package a2a

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"mime"
	"net/http"
	"time"

	streammodel "github.com/Nene7ko/NeKiro/apps/a2a-router/internal/stream"
	"github.com/Nene7ko/NeKiro/contracts"
	a2ago "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
)

const streamCancelAttemptTimeout = time.Second

type streamingRoundTripper struct {
	base          http.RoundTripper
	maxEventBytes int64
	body          *boundedSSEBody
}

func (transport *streamingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	requestBody, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, classify(contracts.ErrorCodeA2AProtocol, err)
	}
	if closeErr := request.Body.Close(); closeErr != nil {
		return nil, classify(contracts.ErrorCodeA2AProtocol, closeErr)
	}
	request.Body = io.NopCloser(bytes.NewReader(requestBody))
	expectedID, err := streamingRequestID(requestBody)
	if err != nil {
		return nil, classify(contracts.ErrorCodeA2AProtocol, err)
	}
	response, err := transport.base.RoundTrip(request)
	if err != nil || response.StatusCode != http.StatusOK {
		return response, err
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "text/event-stream" {
		_ = response.Body.Close()
		return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A streaming response media type is invalid"))
	}
	transport.body = newBoundedSSEBody(response.Body, transport.maxEventBytes, expectedID)
	response.Body = transport.body
	return response, nil
}

type boundedSSEBody struct {
	body       io.ReadCloser
	reader     *bufio.Reader
	limit      int64
	expectedID json.RawMessage
	seenIDs    map[string]struct{}
	lastResult json.RawMessage
	pending    []byte
	closed     bool
}

func newBoundedSSEBody(body io.ReadCloser, limit int64, expectedID json.RawMessage) *boundedSSEBody {
	return &boundedSSEBody{body: body, reader: bufio.NewReader(body), limit: limit, expectedID: append(json.RawMessage(nil), expectedID...), seenIDs: make(map[string]struct{})}
}

func (body *boundedSSEBody) Read(destination []byte) (int, error) {
	if body.closed {
		return 0, io.ErrClosedPipe
	}
	if len(body.pending) == 0 {
		frame, err := body.readFrame()
		if err != nil {
			return 0, err
		}
		body.pending = frame
	}
	read := copy(destination, body.pending)
	body.pending = body.pending[read:]
	return read, nil
}

func (body *boundedSSEBody) Close() error {
	body.closed = true
	return body.body.Close()
}

func (body *boundedSSEBody) readFrame() ([]byte, error) {
	frame := make([]byte, 0, minInt64(body.limit, 4096))
	lineBuffer := make([]byte, 0, 4096)
	dataLines := 0
	idLines := 0
	for {
		line, err := body.reader.ReadSlice('\n')
		if int64(len(frame)+len(line)) > body.limit {
			return nil, classify(contracts.ErrorCodeAgentResponseTooLarge, errors.New("A2A streaming event exceeds the configured limit"))
		}
		frame = append(frame, line...)
		lineBuffer = append(lineBuffer, line...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(frame) == 0 {
				return nil, io.EOF
			}
			return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A SSE stream ended before an event delimiter"))
		}
		if err != nil {
			return nil, classify(contracts.ErrorCodeAgentUnavailable, err)
		}
		line = lineBuffer
		lineBuffer = lineBuffer[:0]
		trimmed := bytes.TrimSuffix(line, []byte("\n"))
		trimmed = bytes.TrimSuffix(trimmed, []byte("\r"))
		if len(trimmed) == 0 {
			if dataLines != 1 || idLines != 1 {
				return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A SSE event must contain exactly one id and data line"))
			}
			return frame, nil
		}
		if bytes.HasPrefix(trimmed, []byte("id: ")) {
			if idLines != 0 {
				return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A SSE event contains duplicate id lines"))
			}
			id := bytes.TrimSpace(trimmed[len("id: "):])
			if len(id) == 0 {
				return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A SSE event id is empty"))
			}
			idValue := string(id)
			if _, exists := body.seenIDs[idValue]; exists {
				return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A SSE event id is repeated"))
			}
			body.seenIDs[idValue] = struct{}{}
			idLines++
			continue
		}
		if dataLines != 0 || !bytes.HasPrefix(trimmed, []byte("data: ")) {
			return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A SSE event framing is invalid"))
		}
		data := trimmed[len("data: "):]
		if len(data) == 0 || !json.Valid(data) {
			return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A SSE data must be one JSON value"))
		}
		result, err := streamingJSONRPCResult(data, body.expectedID)
		if err != nil {
			return nil, classify(contracts.ErrorCodeA2AProtocol, err)
		}
		body.lastResult = append(body.lastResult[:0], result...)
		dataLines++
	}
}

func streamingRequestID(data []byte) (json.RawMessage, error) {
	if err := rejectDuplicateJSONMembers(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := decoder.Decode(&request); err != nil {
		return nil, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	if request.JSONRPC != "2.0" || request.Method != "message/stream" {
		return nil, errors.New("A2A streaming request envelope is invalid")
	}
	if err := validateJSONRPCID(request.ID); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), request.ID...), nil
}

func streamingJSONRPCResult(data, expectedID json.RawMessage) (json.RawMessage, error) {
	if err := rejectDuplicateJSONMembers(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var response jsonRPCEnvelope
	if err := decoder.Decode(&response); err != nil {
		return nil, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	if response.JSONRPC != "2.0" {
		return nil, errors.New("A2A JSON-RPC streaming response version is invalid")
	}
	if err := validateJSONRPCID(response.ID); err != nil {
		return nil, err
	}
	if !equalJSONRPCID(expectedID, response.ID) {
		return nil, errors.New("A2A JSON-RPC streaming response id does not match the request")
	}
	hasResult := len(response.Result) > 0 && !bytes.Equal(bytes.TrimSpace(response.Result), []byte("null"))
	hasError := len(response.Error) > 0 && !bytes.Equal(bytes.TrimSpace(response.Error), []byte("null"))
	if hasResult == hasError {
		return nil, errors.New("A2A JSON-RPC streaming response must contain exactly one result or error")
	}
	return append(json.RawMessage(nil), response.Result...), nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("A2A JSON-RPC envelope contains trailing data")
	}
	return err
}

func minInt64(left, right int64) int {
	if left < right {
		return int(left)
	}
	return int(right)
}

func (client *Client) SendStreaming(ctx context.Context, dispatch contracts.DispatchInvocationRequestV3, resolved contracts.ResolveAgentResponse) iter.Seq2[streammodel.Event, error] {
	return func(yield func(streammodel.Event, error) bool) {
		target, err := NewTarget(resolved, dispatch.Capability)
		if err != nil {
			yield(streammodel.Event{}, err)
			return
		}
		if !target.Streaming {
			yield(streammodel.Event{}, classify(contracts.ErrorCodeRouteNotFound, errors.New("resolved Agent Card does not enable streaming")))
			return
		}
		inputLimit := client.inputLimitBytes
		if target.MaxInputBytes < inputLimit {
			inputLimit = target.MaxInputBytes
		}
		if int64(len(dispatch.Input)) > inputLimit {
			yield(streammodel.Event{}, classify(contracts.ErrorCodePayloadTooLarge, errors.New("dispatch input exceeds the resolved Agent limit")))
			return
		}
		params, err := messageSendParams(dispatch)
		if err != nil {
			yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, err))
			return
		}

		httpClient := *client.httpClient
		base := httpClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		eventLimit := client.a2aEventLimitBytes
		if target.MaxOutputBytes < eventLimit {
			eventLimit = target.MaxOutputBytes
		}
		streamTransport := &streamingRoundTripper{base: base, maxEventBytes: eventLimit}
		httpClient.Transport = streamTransport
		a2aClient, err := a2aclient.NewFromEndpoints(ctx, []a2ago.AgentInterface{{Transport: a2ago.TransportProtocolJSONRPC, URL: target.Endpoint}}, a2aclient.WithJSONRPCTransport(&httpClient))
		if err != nil {
			yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, err))
			return
		}
		a2aClient.AddCallInterceptor(a2aclient.NewStaticCallMetaInjector(a2aclient.CallMeta{
			HeaderTraceID:            []string{string(dispatch.TraceID)},
			HeaderInvocationID:       []string{dispatch.InvocationID},
			HeaderRootTaskID:         []string{dispatch.RootTaskID},
			HeaderParentInvocationID: []string{""},
			HeaderWorkspaceID:        []string{dispatch.WorkspaceID},
		}))
		cancelHTTPClient := *client.httpClient
		cancelBase := cancelHTTPClient.Transport
		if cancelBase == nil {
			cancelBase = http.DefaultTransport
		}
		cancelHTTPClient.Transport = cancelBase
		cancelClient, cancelClientErr := a2aclient.NewFromEndpoints(context.Background(), []a2ago.AgentInterface{{Transport: a2ago.TransportProtocolJSONRPC, URL: target.Endpoint}}, a2aclient.WithJSONRPCTransport(&cancelHTTPClient))
		if cancelClientErr != nil {
			yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, cancelClientErr))
			return
		}
		cancelClient.AddCallInterceptor(a2aclient.NewStaticCallMetaInjector(a2aclient.CallMeta{
			HeaderTraceID:            []string{string(dispatch.TraceID)},
			HeaderInvocationID:       []string{dispatch.InvocationID},
			HeaderRootTaskID:         []string{dispatch.RootTaskID},
			HeaderParentInvocationID: []string{""},
			HeaderWorkspaceID:        []string{dispatch.WorkspaceID},
		}))

		var taskID, contextID string
		artifactLast := make(map[string]bool)
		artifactSeen := make(map[string]bool)
		terminalSeen := false
		var pendingTerminal *streammodel.Event
		defer func() {
			if taskID == "" || ctx.Err() == nil {
				return
			}
			// A local deadline/disconnect may leave a known remote task running.
			// Make one bounded cancellation attempt; never retry or reroute.
			cancelCtx, cancel := context.WithTimeout(context.Background(), streamCancelAttemptTimeout)
			defer cancel()
			// Cancellation failure cannot replace the already committed local
			// timeout/cancel outcome and is intentionally not retried or promoted.
			_, _ = cancelClient.CancelTask(cancelCtx, &a2ago.TaskIDParams{ID: a2ago.TaskID(taskID)})
		}()
		for event, eventErr := range a2aClient.SendStreamingMessage(ctx, params) {
			if eventErr != nil {
				yield(streammodel.Event{}, classifyTransportError(eventErr))
				return
			}
			mapped, mapErr := mapA2AStreamEvent(event)
			if mapErr != nil {
				yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, mapErr))
				return
			}
			if terminalSeen {
				yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A stream emitted an event after terminal")))
				return
			}
			if mapped.Kind == "artifact-update" {
				if mapped.ArtifactID == "" {
					yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A artifact identity is required")))
					return
				}
				if mapped.ArtifactAppend && !artifactSeen[mapped.ArtifactID] {
					yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A artifact append has no base chunk")))
					return
				}
				if artifactLast[mapped.ArtifactID] {
					yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A artifact emitted after its last chunk")))
					return
				}
				if !mapped.ArtifactAppend && artifactSeen[mapped.ArtifactID] {
					yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A artifact base chunk was repeated")))
					return
				}
				artifactSeen[mapped.ArtifactID] = true
				if mapped.ArtifactLast {
					artifactLast[mapped.ArtifactID] = true
				}
			}
			if streamTransport.body == nil || len(streamTransport.body.lastResult) == 0 {
				yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A stream result payload is unavailable")))
				return
			}
			payload := append(json.RawMessage(nil), streamTransport.body.lastResult...)
			if int64(len(payload)) > eventLimit {
				yield(streammodel.Event{}, classify(contracts.ErrorCodeAgentResponseTooLarge, errors.New("A2A streaming event exceeds the configured limit")))
				return
			}
			if taskID == "" {
				taskID, contextID = mapped.TaskID, mapped.ContextID
			} else if mapped.TaskID != taskID || mapped.ContextID != contextID {
				yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A stream task or context identity changed")))
				return
			}
			if mapped.TerminalType != "" {
				for artifactID := range artifactSeen {
					if !artifactLast[artifactID] {
						yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A artifact stream ended before last chunk")))
						return
					}
				}
				terminalSeen = true
				pending := mapped
				pending.Payload = payload
				pendingTerminal = &pending
				continue
			}
			mapped.Payload = payload
			if !yield(mapped, nil) {
				return
			}
		}
		for artifactID := range artifactSeen {
			if !artifactLast[artifactID] {
				yield(streammodel.Event{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A artifact stream ended before lastChunk")))
				return
			}
		}
		if pendingTerminal != nil {
			if ctx.Err() != nil {
				yield(streammodel.Event{}, ctx.Err())
				return
			}
			if !yield(*pendingTerminal, nil) {
				return
			}
			return
		}
		if ctx.Err() != nil {
			yield(streammodel.Event{}, ctx.Err())
			return
		}
		yield(streammodel.Event{}, streammodel.ErrInterrupted)
	}
}

func mapA2AStreamEvent(event a2ago.Event) (streammodel.Event, error) {
	switch value := event.(type) {
	case *a2ago.Message:
		if err := contracts.ValidateA2AMessageResult(value); err != nil {
			return streammodel.Event{}, err
		}
		if value.TaskID == "" || value.ContextID == "" {
			return streammodel.Event{}, errors.New("A2A stream message task and context IDs are required")
		}
		return streammodel.Event{Kind: "message", TaskID: string(value.TaskID), ContextID: value.ContextID}, nil
	case *a2ago.Task:
		mapping, err := contracts.ValidateA2ATask(value)
		if err != nil {
			return streammodel.Event{}, err
		}
		mapped := streammodel.Event{Kind: "task", TaskID: string(value.ID), ContextID: value.ContextID}
		if mapping.Classification == contracts.A2ATaskStateTerminal {
			mapped.TerminalStatus = mapping.InvocationStatus
			mapped.ErrorCode = mapping.ErrorCode
			if mapping.InvocationStatus == "succeeded" {
				mapped.TerminalType = contracts.ResultStreamEventCompleted
			} else if mapping.InvocationStatus == "canceled" {
				mapped.TerminalType = contracts.ResultStreamEventCanceled
			} else {
				mapped.TerminalType = contracts.ResultStreamEventFailed
			}
		}
		return mapped, nil
	case *a2ago.TaskStatusUpdateEvent:
		if value.TaskID == "" || value.ContextID == "" {
			return streammodel.Event{}, errors.New("A2A status event task and context IDs are required")
		}
		mapping, err := contracts.MapA2ATaskState(value.Status.State)
		if err != nil {
			return streammodel.Event{}, err
		}
		if mapping.Classification == contracts.A2ATaskStateTerminal && !value.Final || mapping.Classification == contracts.A2ATaskStateTransient && value.Final {
			return streammodel.Event{}, errors.New("A2A status event final flag contradicts task state")
		}
		mapped := streammodel.Event{Kind: "status-update", TaskID: string(value.TaskID), ContextID: value.ContextID}
		if mapping.Classification == contracts.A2ATaskStateTerminal {
			mapped.TerminalStatus = mapping.InvocationStatus
			mapped.ErrorCode = mapping.ErrorCode
			switch mapping.InvocationStatus {
			case "succeeded":
				mapped.TerminalType = contracts.ResultStreamEventCompleted
			case "canceled":
				mapped.TerminalType = contracts.ResultStreamEventCanceled
			default:
				mapped.TerminalType = contracts.ResultStreamEventFailed
			}
		}
		return mapped, nil
	case *a2ago.TaskArtifactUpdateEvent:
		if value.TaskID == "" || value.ContextID == "" || value.Artifact == nil || value.Artifact.ID == "" || len(value.Artifact.Parts) == 0 {
			return streammodel.Event{}, errors.New("A2A artifact event is incomplete")
		}
		return streammodel.Event{Kind: "artifact-update", TaskID: string(value.TaskID), ContextID: value.ContextID, ArtifactID: string(value.Artifact.ID), ArtifactAppend: value.Append, ArtifactLast: value.LastChunk}, nil
	default:
		return streammodel.Event{}, errors.New("A2A stream event kind is unsupported")
	}
}
