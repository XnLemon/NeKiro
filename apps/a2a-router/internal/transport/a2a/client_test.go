package a2a

import (
	"net/http"
	"net/http/httptest"
	"testing"

	runtimeb "github.com/Nene7ko/NeKiro/agents/runtime-b"
	"github.com/Nene7ko/NeKiro/contracts"
	a2ago "github.com/a2aproject/a2a-go/a2a"
)

func TestClientSendMessageCallsRuntimeBWithPlatformContext(t *testing.T) {
	captured := make(http.Header)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		captured = request.Header.Clone()
		runtimeb.NewHTTPHandler(runtimeb.NewHandler()).ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.Client())
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	target, err := NewTarget(contracts.ResolveAgentResponse{Card: targetCard(server.URL, "none", "capability-a")}, "capability-a")
	if err != nil {
		t.Fatalf("NewTarget = %v", err)
	}
	result, err := client.SendMessage(t.Context(), target, ContextHeaders{
		TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a",
		ParentInvocationID: "parent-a", WorkspaceID: "workspace-a",
	}, runtimeBMessageParams("message-a", "success", map[string]any{"value": "ok"}))
	if err != nil {
		t.Fatalf("SendMessage = %v", err)
	}
	message, ok := result.(*a2ago.Message)
	if !ok {
		t.Fatalf("result type = %T, want *a2a.Message", result)
	}
	if message.Role != a2ago.MessageRoleAgent || message.ContextID == "" {
		t.Fatalf("message = %#v", message)
	}
	assertHeader(t, captured, HeaderTraceID, "trace-a")
	assertHeader(t, captured, HeaderInvocationID, "inv-a")
	assertHeader(t, captured, HeaderRootTaskID, "task-a")
	assertHeader(t, captured, HeaderParentInvocationID, "parent-a")
	assertHeader(t, captured, HeaderWorkspaceID, "workspace-a")
}

func TestClientSendMessageRequiresExplicitDependencies(t *testing.T) {
	if _, err := NewClient(nil); err == nil {
		t.Fatal("NewClient(nil) succeeded, want error")
	}
	client, err := NewClient(http.DefaultClient)
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	if _, err := client.SendMessage(t.Context(), Target{}, ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, runtimeBMessageParams("message-a", "success", "ok")); err == nil {
		t.Fatal("SendMessage without target endpoint succeeded, want error")
	}
	target := Target{Endpoint: "http://127.0.0.1:1"}
	if _, err := client.SendMessage(t.Context(), target, ContextHeaders{}, runtimeBMessageParams("message-a", "success", "ok")); err == nil {
		t.Fatal("SendMessage without context headers succeeded, want error")
	}
	if _, err := client.SendMessage(t.Context(), target, ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, nil); err == nil {
		t.Fatal("SendMessage without params succeeded, want error")
	}
}

func runtimeBMessageParams(messageID, kind string, value any) *a2ago.MessageSendParams {
	return &a2ago.MessageSendParams{Message: &a2ago.Message{
		ID: messageID, Role: a2ago.MessageRoleUser,
		Parts: []a2ago.Part{a2ago.DataPart{Data: map[string]any{"fixture": kind, "value": value}}},
	}}
}

func assertHeader(t *testing.T, header http.Header, key, want string) {
	t.Helper()
	if got := header.Get(key); got != want {
		t.Fatalf("header %s = %q, want %q", key, got, want)
	}
}
