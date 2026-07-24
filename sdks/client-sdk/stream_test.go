package clientsdk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Nene7ko/NeKiro/contracts"
)

func TestInvokeStreamNegotiatesExactRequestAndDeliversIncrementallyThroughEOF(t *testing.T) {
	reader, writer := io.Pipe()
	tracked := &streamTrackedBody{ReadCloser: reader}
	var requestBody []byte
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/v4/workspaces/workspace-a/invocations" || request.Header.Get("Accept") != "text/event-stream" || request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Authorization") != "Bearer application-secret" {
			t.Error("streaming request did not match the exact method, route, media, and authorization contract")
		}
		requestBody, _ = io.ReadAll(request.Body)
		header := http.Header{}
		header.Set("Content-Type", "text/event-stream")
		header.Set(traceHeader, "trace-client")
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: tracked}, nil
	})
	client, err := NewClient(validTestConfig(&http.Client{Transport: transport}, "https://gateway.example"))
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.InvokeStream(t.Context(), InvokeRequest{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`{"question":"live"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(requestBody, &members) != nil || len(members) != 4 || string(members["stream"]) != "true" {
		t.Fatalf("stream request body=%s", requestBody)
	}
	events := []StreamEvent{
		streamEvent(t, contracts.ResultStreamEventAccepted, 0, 0, "trace-client"),
		streamEvent(t, contracts.ResultStreamEventChunk, 1, 0, "trace-client"),
		streamEvent(t, contracts.ResultStreamEventChunk, 2, 1, "trace-client"),
		streamEvent(t, contracts.ResultStreamEventCompleted, 3, 0, "trace-client"),
	}
	for _, expected := range events {
		frame := eventFrame(t, expected)
		writeDone := make(chan error, 1)
		go func() {
			_, err := io.WriteString(writer, frame)
			writeDone <- err
		}()
		actual, err := stream.Recv()
		if err != nil || actual.Type != expected.Type || actual.Sequence != expected.Sequence || actual.InvocationID != "inv-client" || actual.RootTaskID != "task-client" || actual.TraceID != "trace-client" {
			t.Fatalf("event=%#v error=%v, want %#v", actual, err, expected)
		}
		if err := <-writeDone; err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("terminal transport EOF=%v", err)
	}
	if !tracked.closed {
		t.Fatal("clean stream body was not closed")
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close after clean EOF=%v", err)
	}
}

func TestStreamAcceptsEveryCorrelatedTerminalOutcome(t *testing.T) {
	for _, terminal := range []contracts.ResultStreamEventType{
		contracts.ResultStreamEventCompleted,
		contracts.ResultStreamEventFailed,
		contracts.ResultStreamEventCanceled,
		contracts.ResultStreamEventTimedOut,
	} {
		t.Run(string(terminal), func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(eventFrame(t, streamEvent(t, contracts.ResultStreamEventAccepted, 0, 0, "trace-client")) + eventFrame(t, streamEvent(t, terminal, 1, 0, "trace-client")))}
			stream := mustOpenStream(t, body, 4096, []string{"trace-client"}, []string{"text/event-stream"})
			accepted, err := stream.Recv()
			if err != nil || accepted.Type != contracts.ResultStreamEventAccepted {
				t.Fatalf("accepted=%#v error=%v", accepted, err)
			}
			got, err := stream.Recv()
			if err != nil || got.Type != terminal || got.InvocationID != "inv-client" || got.RootTaskID != "task-client" || got.TraceID != "trace-client" {
				t.Fatalf("terminal=%#v error=%v", got, err)
			}
			if terminal != contracts.ResultStreamEventCompleted && (got.Error == nil || got.Error.InvocationID != got.InvocationID || got.Error.RootTaskID != got.RootTaskID || got.Error.TraceID != got.TraceID) {
				t.Fatalf("terminal error correlation=%#v", got.Error)
			}
			if _, err := stream.Recv(); !errors.Is(err, io.EOF) || !body.closed {
				t.Fatalf("EOF=%v closed=%v", err, body.closed)
			}
		})
	}
}

func TestInvokeStreamRejectsInvalidHTTPBoundaryAndClosesBody(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		traces  []string
		content []string
	}{
		{name: "non-200", status: http.StatusAccepted, traces: []string{"trace-client"}, content: []string{"text/event-stream"}},
		{name: "missing media", status: 200, traces: []string{"trace-client"}},
		{name: "wrong media", status: 200, traces: []string{"trace-client"}, content: []string{"application/json"}},
		{name: "duplicate media", status: 200, traces: []string{"trace-client"}, content: []string{"text/event-stream", "text/event-stream"}},
		{name: "missing Trace", status: 200, content: []string{"text/event-stream"}},
		{name: "duplicate Trace", status: 200, traces: []string{"trace-client", "trace-client"}, content: []string{"text/event-stream"}},
		{name: "malformed Trace", status: 200, traces: []string{"bad trace"}, content: []string{"text/event-stream"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader("")}
			client := streamClient(t, body, 4096, test.status, test.traces, test.content)
			if _, err := client.InvokeStream(t.Context(), validStreamRequest()); err == nil || !body.closed {
				t.Fatalf("error=%v closed=%v", err, body.closed)
			}
		})
	}
}

func TestStreamRejectsMalformedReorderedAndInterruptedDelivery(t *testing.T) {
	accepted := eventFrame(t, streamEvent(t, contracts.ResultStreamEventAccepted, 0, 0, "trace-client"))
	completed := eventFrame(t, streamEvent(t, contracts.ResultStreamEventCompleted, 1, 0, "trace-client"))
	chunkZero := eventFrame(t, streamEvent(t, contracts.ResultStreamEventChunk, 1, 0, "trace-client"))
	tests := []struct {
		name   string
		body   string
		limit  int64
		reads  int
		isIntr bool
	}{
		{name: "malformed field", body: "event: accepted\n\n", limit: 4096, reads: 1},
		{name: "CRLF", body: strings.ReplaceAll(accepted, "\n", "\r\n"), limit: 4096, reads: 1},
		{name: "multiple fields", body: "data: {}\ndata: {}\n\n", limit: 4096, reads: 1},
		{name: "oversized", body: accepted, limit: int64(len(accepted) - 1), reads: 1},
		{name: "duplicate event member", body: strings.Replace(accepted, `"schemaVersion":"2"`, `"schemaVersion":"2","schemaVersion":"2"`, 1), limit: 4096, reads: 1},
		{name: "unknown event member", body: strings.Replace(accepted, `,"sequence"`, `,"unknown":true,"sequence"`, 1), limit: 4096, reads: 1},
		{name: "trailing event JSON", body: strings.Replace(accepted, "}\n\n", "}{}\n\n", 1), limit: 4096, reads: 1},
		{name: "wrong first event", body: eventFrame(t, streamEvent(t, contracts.ResultStreamEventCompleted, 0, 0, "trace-client")), limit: 4096, reads: 1},
		{name: "first Trace mismatch", body: eventFrame(t, streamEvent(t, contracts.ResultStreamEventAccepted, 0, 0, "trace-other")), limit: 4096, reads: 1},
		{name: "sequence drift", body: accepted + eventFrame(t, streamEvent(t, contracts.ResultStreamEventCompleted, 2, 0, "trace-client")), limit: 4096, reads: 2},
		{name: "chunk drift", body: accepted + eventFrame(t, streamEvent(t, contracts.ResultStreamEventChunk, 1, 1, "trace-client")), limit: 4096, reads: 2},
		{name: "Trace drift", body: accepted + eventFrame(t, streamEvent(t, contracts.ResultStreamEventChunk, 1, 0, "trace-other")), limit: 4096, reads: 2},
		{name: "Invocation drift", body: accepted + strings.Replace(chunkZero, `"invocationId":"inv-client"`, `"invocationId":"inv-other"`, 1), limit: 4096, reads: 2},
		{name: "root Task drift", body: accepted + strings.Replace(chunkZero, `"rootTaskId":"task-client"`, `"rootTaskId":"task-other"`, 1), limit: 4096, reads: 2},
		{name: "early EOF", body: accepted, limit: 4096, reads: 2, isIntr: true},
		{name: "post-terminal event", body: accepted + completed + eventFrame(t, streamEvent(t, contracts.ResultStreamEventChunk, 2, 0, "trace-client")), limit: 4096, reads: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(test.body)}
			stream := mustOpenStream(t, body, test.limit, []string{"trace-client"}, []string{"text/event-stream"})
			var err error
			for range test.reads {
				_, err = stream.Recv()
				if err != nil {
					break
				}
			}
			if err == nil || errors.Is(err, io.EOF) {
				t.Fatalf("stream error=%v", err)
			}
			if test.isIntr && !errors.Is(err, contracts.ErrRuntimeStreamInterrupted) {
				t.Fatalf("stream interruption error=%v", err)
			}
			if !body.closed {
				t.Fatal("failed stream body was not closed")
			}
		})
	}
}

func TestStreamContextCancellationAndCloseStateMatrix(t *testing.T) {
	accepted := eventFrame(t, streamEvent(t, contracts.ResultStreamEventAccepted, 0, 0, "trace-client"))
	completed := eventFrame(t, streamEvent(t, contracts.ResultStreamEventCompleted, 1, 0, "trace-client"))

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		body := &trackedBody{Reader: strings.NewReader(accepted + completed)}
		client := streamClient(t, body, 4096, http.StatusOK, []string{"trace-client"}, []string{"text/event-stream"})
		stream, err := client.InvokeStream(ctx, validStreamRequest())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stream.Recv(); err != nil {
			t.Fatal(err)
		}
		cancel()
		if _, err := stream.Recv(); !errors.Is(err, context.Canceled) || !errors.Is(err, contracts.ErrRuntimeStreamInterrupted) || !body.closed {
			t.Fatalf("cancellation error=%v closed=%v", err, body.closed)
		}
	})

	tests := []struct {
		name       string
		body       string
		reads      int
		clean      bool
		wantClosed bool
	}{
		{name: "before accepted", body: accepted + completed, reads: 0, wantClosed: true},
		{name: "active", body: accepted + completed, reads: 1, wantClosed: true},
		{name: "terminal before EOF", body: accepted + completed, reads: 2, wantClosed: true},
		{name: "after terminal EOF", body: accepted + completed, reads: 3, clean: true, wantClosed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(test.body)}
			stream := mustOpenStream(t, body, 4096, []string{"trace-client"}, []string{"text/event-stream"})
			for index := 0; index < test.reads; index++ {
				_, err := stream.Recv()
				if index < 2 && err != nil {
					t.Fatalf("read %d error=%v", index, err)
				}
				if index == 2 && !errors.Is(err, io.EOF) {
					t.Fatalf("EOF error=%v", err)
				}
			}
			first := stream.Close()
			second := stream.Close()
			if test.clean {
				if first != nil || second != nil {
					t.Fatalf("clean Close outcomes=%v, %v", first, second)
				}
			} else if !errors.Is(first, contracts.ErrRuntimeStreamInterrupted) || first != second {
				t.Fatalf("interrupted Close outcomes=%v, %v", first, second)
			}
			if body.closed != test.wantClosed {
				t.Fatalf("body closed=%v", body.closed)
			}
			if _, err := stream.Recv(); !errors.Is(err, contracts.ErrRuntimeStreamClosed) {
				t.Fatalf("Recv after Close error=%v", err)
			}
		})
	}
}

func TestGatewayStreamValidationDoesNotExposeRawEventContent(t *testing.T) {
	secret := "credential-sentinel-from-stream"
	accepted := eventFrame(t, streamEvent(t, contracts.ResultStreamEventAccepted, 0, 0, "trace-client"))
	body := strings.Replace(accepted, `,"sequence"`, `,"`+secret+`":"`+secret+`","sequence"`, 1)
	stream := mustOpenStream(t, &trackedBody{Reader: strings.NewReader(body)}, 4096, []string{"trace-client"}, []string{"text/event-stream"})
	_, err := stream.Recv()
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), body) {
		t.Fatal("Gateway stream validation exposed raw event content")
	}
}

func validStreamRequest() InvokeRequest {
	return InvokeRequest{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`{}`)}
}

func streamEvent(t *testing.T, eventType contracts.ResultStreamEventType, sequence, chunkIndex int64, traceID contracts.TraceID) StreamEvent {
	t.Helper()
	event := StreamEvent{SchemaVersion: "2", Sequence: sequence, Type: eventType, InvocationID: "inv-client", RootTaskID: "task-client", TraceID: traceID}
	switch eventType {
	case contracts.ResultStreamEventAccepted:
		event.Status = "pending"
	case contracts.ResultStreamEventChunk:
		event.Status = "running"
		event.ChunkIndex = pointer(chunkIndex)
		event.Chunk = json.RawMessage(`{"delta":"x"}`)
	case contracts.ResultStreamEventCompleted:
		event.Status = "succeeded"
	case contracts.ResultStreamEventFailed, contracts.ResultStreamEventCanceled, contracts.ResultStreamEventTimedOut:
		code := contracts.ErrorCodeAgentExecutionFailed
		event.Status = "failed"
		if eventType == contracts.ResultStreamEventCanceled {
			code, event.Status = contracts.ErrorCodeCanceled, "canceled"
		}
		if eventType == contracts.ResultStreamEventTimedOut {
			code, event.Status = contracts.ErrorCodeTimeout, "timed_out"
		}
		platformError, err := contracts.NewCorrelatedPlatformErrorV4(code, traceID, event.InvocationID, event.RootTaskID)
		if err != nil {
			t.Fatal(err)
		}
		event.Error = &platformError
	default:
		t.Fatalf("unsupported test event type %q", eventType)
	}
	return event
}

func eventFrame(t *testing.T, event StreamEvent) string {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return "data: " + string(data) + "\n\n"
}

func pointer(value int64) *int64 { return &value }

func mustOpenStream(t *testing.T, body io.ReadCloser, limit int64, traces, content []string) *Stream {
	t.Helper()
	client := streamClient(t, body, limit, http.StatusOK, traces, content)
	stream, err := client.InvokeStream(t.Context(), validStreamRequest())
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func streamClient(t *testing.T, body io.ReadCloser, limit int64, status int, traces, content []string) *Client {
	t.Helper()
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		header := http.Header{}
		for _, value := range traces {
			header.Add(traceHeader, value)
		}
		for _, value := range content {
			header.Add("Content-Type", value)
		}
		return &http.Response{StatusCode: status, Header: header, Body: body}, nil
	})
	config := validTestConfig(&http.Client{Transport: transport}, "https://gateway.example")
	config.StreamEventLimitBytes = limit
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

type streamTrackedBody struct {
	io.ReadCloser
	closed bool
}

func (body *streamTrackedBody) Close() error {
	body.closed = true
	return body.ReadCloser.Close()
}
