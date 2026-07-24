package clientsdk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Nene7ko/NeKiro/contracts"
)

// StreamEvent is the active Result Stream Event v2 contract exposed after SDK
// framing, shape, sequence, and correlation validation.
type StreamEvent = contracts.InvocationResultStreamEventV2

// Stream owns one successful live Gateway response. It has a single consumer
// and is not safe for concurrent Recv or Close calls.
type Stream struct {
	ctx             context.Context
	body            io.ReadCloser
	reader          *bufio.Reader
	eventLimitBytes int64
	traceID         contracts.TraceID
	runtime         *contracts.RuntimeContractValidator
	sequence        *contracts.RuntimeResultStreamSequenceValidator
	terminal        bool
	finished        bool
	failure         error
	closed          bool
	closeOutcome    error
	bodyClosed      bool
}

// InvokeStream performs exactly one streaming Gateway invocation and returns
// after the HTTP status, media type, and Trace header have been validated.
func (client *Client) InvokeStream(ctx context.Context, request InvokeRequest) (*Stream, error) {
	response, err := client.do(ctx, request, true, "text/event-stream")
	if err != nil {
		return nil, err
	}
	if response == nil || response.Body == nil {
		return nil, errors.New("clientsdk: Gateway response is empty")
	}
	if response.StatusCode != http.StatusOK {
		return nil, client.decodePlatformError(response)
	}
	if err := requireMediaType(response.Header, "text/event-stream"); err != nil {
		return nil, closeWithError(response.Body, err)
	}
	traceID, err := requireTraceHeader(response.Header)
	if err != nil {
		return nil, closeWithError(response.Body, err)
	}
	return &Stream{
		ctx: ctx, body: response.Body, reader: bufio.NewReader(response.Body),
		eventLimitBytes: client.streamEventLimitBytes,
		traceID:         traceID,
		runtime:         client.runtime,
	}, nil
}

// Recv returns the next validated stream event. A clean io.EOF is returned
// only after one terminal event has been returned and transport EOF is then
// observed.
func (stream *Stream) Recv() (StreamEvent, error) {
	if stream.closed {
		return StreamEvent{}, fmt.Errorf("clientsdk: result stream is closed: %w", contracts.ErrRuntimeStreamClosed)
	}
	if stream.failure != nil {
		return StreamEvent{}, stream.failure
	}
	if stream.finished {
		return StreamEvent{}, io.EOF
	}
	if err := stream.ctx.Err(); err != nil {
		return StreamEvent{}, stream.recordFailure(errors.Join(fmt.Errorf("clientsdk: result stream context ended: %w", err), contracts.ErrRuntimeStreamInterrupted))
	}
	frame, err := readSSEFrame(stream.reader, stream.eventLimitBytes)
	if errors.Is(err, io.EOF) {
		if stream.sequence == nil || !stream.terminal {
			return StreamEvent{}, stream.recordFailure(fmt.Errorf("clientsdk: result stream ended before a terminal event: %w", contracts.ErrRuntimeStreamInterrupted))
		}
		if err := stream.sequence.Finish(); err != nil {
			return StreamEvent{}, stream.recordFailure(fmt.Errorf("clientsdk: finish result stream: %w", err))
		}
		if err := stream.closeBody(); err != nil {
			return StreamEvent{}, stream.recordFailure(fmt.Errorf("clientsdk: close completed result stream: %w", err))
		}
		stream.finished = true
		return StreamEvent{}, io.EOF
	}
	if err != nil {
		if contextErr := stream.ctx.Err(); contextErr != nil {
			return StreamEvent{}, stream.recordFailure(errors.Join(fmt.Errorf("clientsdk: result stream context ended: %w", contextErr), contracts.ErrRuntimeStreamInterrupted))
		}
		return StreamEvent{}, stream.recordFailure(fmt.Errorf("clientsdk: read SSE frame: %w", err))
	}

	var event StreamEvent
	if err := rejectDuplicateJSONMembers(frame); err != nil {
		return StreamEvent{}, stream.recordFailure(errors.New("clientsdk: Gateway stream event is invalid"))
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return StreamEvent{}, stream.recordFailure(errors.New("clientsdk: Gateway stream event is invalid"))
	}
	if err := requireEOF(decoder); err != nil {
		return StreamEvent{}, stream.recordFailure(errors.New("clientsdk: Gateway stream event is invalid"))
	}
	if stream.sequence == nil {
		if event.Type != contracts.ResultStreamEventAccepted {
			return StreamEvent{}, stream.recordFailure(errors.New("clientsdk: result stream must begin with accepted"))
		}
		if event.TraceID != stream.traceID {
			return StreamEvent{}, stream.recordFailure(errors.New("clientsdk: first stream event Trace does not match response header"))
		}
		sequence, err := contracts.NewRuntimeResultStreamSequenceValidator(stream.runtime, event.InvocationID, event.RootTaskID, event.TraceID)
		if err != nil {
			return StreamEvent{}, stream.recordFailure(errors.New("clientsdk: Gateway stream event is invalid"))
		}
		stream.sequence = sequence
	}
	if err := stream.sequence.Accept(event); err != nil {
		return StreamEvent{}, stream.recordFailure(errors.New("clientsdk: Gateway stream event is invalid"))
	}
	stream.terminal = stream.sequence.IsTerminal()
	return event, nil
}

// Close releases the response body. Until terminal followed by actual EOF has
// been observed, Close records and returns an interrupted-stream error.
func (stream *Stream) Close() error {
	if stream.closed {
		return stream.closeOutcome
	}
	stream.closed = true
	if stream.failure != nil {
		stream.closeOutcome = stream.failure
		return stream.closeOutcome
	}
	if stream.finished {
		stream.closeOutcome = nil
		return nil
	}
	interrupted := fmt.Errorf("clientsdk: result stream closed before terminal EOF: %w", contracts.ErrRuntimeStreamInterrupted)
	stream.closeOutcome = errors.Join(interrupted, stream.closeBody())
	return stream.closeOutcome
}

func (stream *Stream) recordFailure(err error) error {
	if stream.failure != nil {
		return stream.failure
	}
	stream.failure = errors.Join(err, stream.closeBody())
	return stream.failure
}

func (stream *Stream) closeBody() error {
	if stream.bodyClosed {
		return nil
	}
	stream.bodyClosed = true
	return stream.body.Close()
}

func readSSEFrame(reader *bufio.Reader, limit int64) ([]byte, error) {
	line, err := readSSELine(reader, limit)
	if errors.Is(err, io.EOF) {
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(line, []byte("data: ")) || bytes.IndexByte(line, '\r') >= 0 || len(line) < len("data: \n") || line[len(line)-1] != '\n' {
		return nil, errors.New("SSE frame must contain exactly one data line")
	}
	blank, err := readSSELine(reader, limit)
	if err != nil || !bytes.Equal(blank, []byte("\n")) {
		return nil, errors.New("SSE frame must end with one blank line")
	}
	if int64(len(line)+len(blank)) > limit {
		return nil, errors.New("SSE frame exceeds the configured limit")
	}
	payload := line[len("data: ") : len(line)-1]
	if len(payload) == 0 {
		return nil, errors.New("SSE data payload is empty")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, payload); err != nil || !bytes.Equal(compact.Bytes(), payload) {
		return nil, errors.New("SSE data payload must be compact JSON")
	}
	return payload, nil
}

func readSSELine(reader *bufio.Reader, limit int64) ([]byte, error) {
	var line []byte
	for {
		part, err := reader.ReadSlice('\n')
		line = append(line, part...)
		if int64(len(line)) > limit {
			return nil, errors.New("SSE frame exceeds the configured limit")
		}
		if err == nil {
			return line, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(line) == 0 {
				return nil, io.EOF
			}
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
}
