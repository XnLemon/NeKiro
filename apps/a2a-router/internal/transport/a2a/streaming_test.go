package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
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
	client, err := newTestClient(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	dispatch := contracts.DispatchInvocationRequestV3{
		InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
		Caller: contracts.Caller{Type: "user", ID: "owner-a"}, WorkspaceID: "workspace-a",
		TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", Capability: "capability-a",
		Input: json.RawMessage(`{"fixture":"stream-success","value":"stream"}`), Stream: true,
	}
	seenEvent := false
	for event, streamErr := range client.SendStreaming(ctx, dispatch, contracts.ResolveAgentResponse{Card: targetCard(server.URL, "none", "capability-a")}) {
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
}
