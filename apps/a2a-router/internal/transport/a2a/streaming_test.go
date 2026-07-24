package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
	a2ago "github.com/a2aproject/a2a-go/a2a"
)

func TestBoundedSSEBodyRequiresOneJSONDataLine(t *testing.T) {
	valid := `id: event-1
data: {"jsonrpc":"2.0","id":"1","result":{"kind":"task","id":"task-a","contextId":"ctx-a","status":{"state":"working"}}}

`
	body := newBoundedSSEBody(io.NopCloser(strings.NewReader(valid)), 4096, []byte(`"1"`))
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read valid SSE: %v", err)
	}
	if !strings.Contains(string(data), "data: ") {
		t.Fatalf("data=%q", data)
	}

	for _, test := range []struct {
		name string
		body string
	}{
		{name: "multiple data lines", body: "data: {}\ndata: {}\n\n"},
		{name: "missing id", body: "data: {}\n\n"},
		{name: "other field", body: "event: message\ndata: {}\n\n"},
		{name: "missing delimiter", body: "data: {}\n"},
		{name: "id separator", body: "id:event-1\ndata: {}\n\n"},
		{name: "data separator", body: "id: event-1\ndata:{}\n\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream := newBoundedSSEBody(io.NopCloser(strings.NewReader(test.body)), 4096, []byte(`"1"`))
			if _, err := io.ReadAll(stream); err == nil {
				t.Fatal("invalid SSE stream succeeded")
			}
		})
	}
}

func TestBoundedSSEBodyRejectsEventLimitWithoutTruncation(t *testing.T) {
	data := `data: {"jsonrpc":"2.0","id":"1","result":{"kind":"task","id":"task-a","contextId":"ctx-a","status":{"state":"working"}}}

`
	body := newBoundedSSEBody(io.NopCloser(strings.NewReader(data)), int64(len(data)-1), []byte(`"1"`))
	if _, err := io.ReadAll(body); errorCode(err) != "AGENT_RESPONSE_TOO_LARGE" {
		t.Fatalf("error=%v, want AGENT_RESPONSE_TOO_LARGE", err)
	}
}

func TestMapA2AStreamEventCoversProfileEventKinds(t *testing.T) {
	message := &a2ago.Message{
		ID:        "message-a",
		TaskID:    "task-a",
		ContextID: "context-a",
		Role:      a2ago.MessageRoleAgent,
		Parts:     []a2ago.Part{a2ago.DataPart{Data: map[string]any{"value": "ok"}}},
	}
	artifact := &a2ago.TaskArtifactUpdateEvent{
		TaskID:    "task-a",
		ContextID: "context-a",
		Artifact: &a2ago.Artifact{
			ID:    "artifact-a",
			Parts: []a2ago.Part{a2ago.DataPart{Data: map[string]any{"value": "chunk"}}},
		},
		Append:    true,
		LastChunk: true,
	}
	for _, test := range []struct {
		name           string
		event          a2ago.Event
		kind           string
		terminal       contracts.ResultStreamEventType
		status         string
		artifactID     string
		artifactAppend bool
		artifactLast   bool
	}{
		{name: "message", event: message, kind: "message"},
		{name: "task working", event: &a2ago.Task{ID: "task-a", ContextID: "context-a", Status: a2ago.TaskStatus{State: a2ago.TaskStateWorking}}, kind: "task"},
		{name: "task completed", event: &a2ago.Task{ID: "task-a", ContextID: "context-a", Status: a2ago.TaskStatus{State: a2ago.TaskStateCompleted}}, kind: "task", terminal: contracts.ResultStreamEventCompleted, status: "succeeded"},
		{name: "task canceled", event: &a2ago.Task{ID: "task-a", ContextID: "context-a", Status: a2ago.TaskStatus{State: a2ago.TaskStateCanceled}}, kind: "task", terminal: contracts.ResultStreamEventCanceled, status: "canceled"},
		{name: "task failed", event: &a2ago.Task{ID: "task-a", ContextID: "context-a", Status: a2ago.TaskStatus{State: a2ago.TaskStateFailed}}, kind: "task", terminal: contracts.ResultStreamEventFailed, status: "failed"},
		{name: "status working", event: &a2ago.TaskStatusUpdateEvent{TaskID: "task-a", ContextID: "context-a", Status: a2ago.TaskStatus{State: a2ago.TaskStateWorking}}, kind: "status-update"},
		{name: "status completed", event: &a2ago.TaskStatusUpdateEvent{TaskID: "task-a", ContextID: "context-a", Status: a2ago.TaskStatus{State: a2ago.TaskStateCompleted}, Final: true}, kind: "status-update", terminal: contracts.ResultStreamEventCompleted, status: "succeeded"},
		{name: "artifact", event: artifact, kind: "artifact-update", artifactID: "artifact-a", artifactAppend: true, artifactLast: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			mapped, err := mapA2AStreamEvent(test.event)
			if err != nil {
				t.Fatalf("mapA2AStreamEvent() error = %v", err)
			}
			if mapped.Kind != test.kind || mapped.TaskID != "task-a" || mapped.ContextID != "context-a" {
				t.Fatalf("mapped event = %#v", mapped)
			}
			if mapped.TerminalType != test.terminal || mapped.TerminalStatus != test.status {
				t.Fatalf("terminal mapping = %#v", mapped)
			}
			if mapped.ArtifactID != test.artifactID || mapped.ArtifactAppend != test.artifactAppend || mapped.ArtifactLast != test.artifactLast {
				t.Fatalf("artifact mapping = %#v", mapped)
			}
		})
	}
}

func TestMapA2AStreamEventRejectsInvalidEvents(t *testing.T) {
	for _, test := range []struct {
		name  string
		event a2ago.Event
	}{
		{name: "unsupported", event: &a2ago.Message{}},
		{name: "nil", event: nil},
		{name: "missing status identity", event: &a2ago.TaskStatusUpdateEvent{Status: a2ago.TaskStatus{State: a2ago.TaskStateWorking}}},
		{name: "contradictory final flag", event: &a2ago.TaskStatusUpdateEvent{TaskID: "task-a", ContextID: "context-a", Status: a2ago.TaskStatus{State: a2ago.TaskStateWorking}, Final: true}},
		{name: "incomplete artifact", event: &a2ago.TaskArtifactUpdateEvent{TaskID: "task-a", ContextID: "context-a"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := mapA2AStreamEvent(test.event); err == nil {
				t.Fatal("mapA2AStreamEvent() succeeded for invalid event")
			}
		})
	}
}

func TestStreamingJSONRPCValidationRejectsEnvelopeViolations(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
	}{
		{name: "wrong version", data: `{"jsonrpc":"1.0","id":"1","result":{}}`},
		{name: "mismatched id", data: `{"jsonrpc":"2.0","id":"2","result":{}}`},
		{name: "result and error", data: `{"jsonrpc":"2.0","id":"1","result":{},"error":{"code":-1,"message":"failed"}}`},
		{name: "duplicate member", data: `{"jsonrpc":"2.0","id":"1","id":"2","result":{}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := streamingJSONRPCResult([]byte(test.data), []byte(`"1"`)); err == nil {
				t.Fatal("streamingJSONRPCResult() accepted invalid envelope")
			}
		})
	}
}

func TestBoundedSSEBodyRejectsDuplicateAndRepeatedIDs(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "duplicate id", body: "id: event-1\nid: event-2\ndata: {}\n\n"},
		{name: "empty id", body: "id: \ndata: {}\n\n"},
		{name: "repeated id", body: "id: event-1\ndata: {}\n\nid: event-1\ndata: {}\n\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := newBoundedSSEBody(io.NopCloser(strings.NewReader(test.body)), 4096, []byte(`"1"`))
			if _, err := io.ReadAll(body); err == nil {
				t.Fatal("invalid SSE stream succeeded")
			}
		})
	}
}

func TestClientStreamingMakesOneCancelAttemptAfterDeadline(t *testing.T) {
	var cancelCount atomic.Int32
	cancelSeen := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var envelope struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		if envelope.Method == "tasks/cancel" {
			cancelCount.Add(1)
			select {
			case cancelSeen <- struct{}{}:
			default:
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":"cancel","result":{"kind":"task","id":"task-a","contextId":"ctx-a","status":{"state":"canceled"}}}`))
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Error("test writer is not flushable")
			return
		}
		_, _ = fmt.Fprintf(writer, "id: stream-1\ndata: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"kind\":\"task\",\"id\":\"task-a\",\"contextId\":\"ctx-a\",\"status\":{\"state\":\"working\"}}}\n\n", envelope.ID)
		flusher.Flush()
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)
	issuer := &recordingCredentialIssuer{}
	client, err := NewClient(server.Client(), issuer, 4096, 4096, 4096, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	dispatch := contracts.DispatchInvocationRequestV4{
		InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
		Caller: contracts.Caller{Type: "user", ID: "owner-a"}, WorkspaceID: "workspace-a",
		TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", Capability: "capability-a",
		Input: json.RawMessage(`{"fixture":"stream-success","value":"stream"}`), Stream: true,
	}
	seenEvent := false
	for event, streamErr := range client.SendStreaming(ctx, dispatch, resolvedTarget(targetCard(server.URL, "none", "capability-a"))) {
		if streamErr != nil {
			break
		}
		seenEvent = seenEvent || event.TaskID == "task-a"
	}
	if !seenEvent {
		t.Fatal("stream did not yield the known task")
	}
	select {
	case <-cancelSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("tasks/cancel was not attempted")
	}
	if cancelCount.Load() != 1 {
		t.Fatalf("cancel attempts=%d, want 1", cancelCount.Load())
	}
	if tokens := issuer.tokensSnapshot(); len(tokens) != 2 || tokens[0] == tokens[1] {
		t.Fatalf("stream/cancel credentials = %v, want two fresh values", tokens)
	}
}

type recordingCredentialIssuer struct {
	mu     sync.Mutex
	count  int
	tokens []string
}

func (issuer *recordingCredentialIssuer) Issue(contracts.RouterInvocationCredentialContextV1) (string, error) {
	issuer.mu.Lock()
	defer issuer.mu.Unlock()
	issuer.count++
	token := fmt.Sprintf("test-credential-%d", issuer.count)
	issuer.tokens = append(issuer.tokens, token)
	return token, nil
}

func (issuer *recordingCredentialIssuer) tokensSnapshot() []string {
	issuer.mu.Lock()
	defer issuer.mu.Unlock()
	return append([]string(nil), issuer.tokens...)
}
