package invocation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nene7ko/NeKiro/contracts"
)

func TestRouterClientUsesOnlyFrozenInternalV3Direction(t *testing.T) {
	var received contracts.DispatchInvocationRequestV3
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/internal/v3/invocations" || request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer service-secret" || request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("unexpected Router request: %s %s %#v", request.Method, request.URL.Path, request.Header)
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("x-nek-trace-id", "trace-router")
		_, _ = io.WriteString(writer, "data: {}\n\n")
	}))
	defer server.Close()
	client, err := NewRouterClient(server.Client(), server.URL+"/internal/v3/invocations", "service-secret")
	if err != nil {
		t.Fatal(err)
	}
	request := contracts.DispatchInvocationRequestV3{InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a", Caller: contracts.Caller{Type: "user", ID: "owner-a"}, WorkspaceID: "workspace-a", TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", Capability: "capability-a", Input: []byte(`{}`), Stream: true}
	response, err := client.Dispatch(context.Background(), request, contracts.InvocationResultModeSSE)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if received.InvocationID != request.InvocationID || response.StatusCode != 200 || response.ContentType != "text/event-stream" || response.Headers.Get("x-nek-trace-id") != "trace-router" {
		t.Fatalf("received=%#v response=%#v", received, response)
	}
}

func TestRouterClientRejectsWrongResultMediaWithoutFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{}`)
	}))
	defer server.Close()
	client, _ := NewRouterClient(server.Client(), server.URL, "service-secret")
	if _, err := client.Dispatch(context.Background(), contracts.DispatchInvocationRequestV3{}, contracts.InvocationResultModeSSE); err == nil {
		t.Fatal("wrong Router result media was accepted")
	}
}
