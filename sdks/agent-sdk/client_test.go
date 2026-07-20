package agentsdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func validContext() PlatformContext {
	return PlatformContext{
		InvocationID: "inv_parent123",
		RootTaskID:   "task_root456",
		TraceID:      "trc_abc123_1",
		WorkspaceID:  "ws_test789",
		AgentID:      "agent_caller01",
	}
}

func validNestedRequest() NestedRequest {
	return NestedRequest{
		TargetAgentID: "agent_target02",
		Capability:    "summarize",
		Input:         json.RawMessage(`{"text":"hello"}`),
		Stream:        false,
	}
}

func TestPlatformContextValidate(t *testing.T) {
	tests := []struct {
		name    string
		context PlatformContext
		wantErr bool
	}{
		{"valid", validContext(), false},
		{"missing invocationId", PlatformContext{RootTaskID: "task_root456", TraceID: "trc_abc123_1", WorkspaceID: "ws_test789", AgentID: "agent_caller01"}, true},
		{"missing rootTaskId", PlatformContext{InvocationID: "inv_parent123", TraceID: "trc_abc123_1", WorkspaceID: "ws_test789", AgentID: "agent_caller01"}, true},
		{"missing traceId", PlatformContext{InvocationID: "inv_parent123", RootTaskID: "task_root456", WorkspaceID: "ws_test789", AgentID: "agent_caller01"}, true},
		{"missing workspaceId", PlatformContext{InvocationID: "inv_parent123", RootTaskID: "task_root456", TraceID: "trc_abc123_1", AgentID: "agent_caller01"}, true},
		{"missing agentId", PlatformContext{InvocationID: "inv_parent123", RootTaskID: "task_root456", TraceID: "trc_abc123_1", WorkspaceID: "ws_test789"}, true},
		{"invalid identifier", PlatformContext{InvocationID: "inv parent", RootTaskID: "task_root456", TraceID: "trc_abc123_1", WorkspaceID: "ws_test789", AgentID: "agent_caller01"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.context.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("PlatformContext.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNestedRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		request NestedRequest
		wantErr bool
	}{
		{"valid", validNestedRequest(), false},
		{"missing targetAgentId", NestedRequest{Capability: "summarize", Input: json.RawMessage(`{}`)}, true},
		{"missing capability", NestedRequest{TargetAgentID: "agent_target02", Input: json.RawMessage(`{}`)}, true},
		{"missing input", NestedRequest{TargetAgentID: "agent_target02", Capability: "summarize"}, true},
		{"input not object", NestedRequest{TargetAgentID: "agent_target02", Capability: "summarize", Input: json.RawMessage(`"string"`)}, true},
		{"input null", NestedRequest{TargetAgentID: "agent_target02", Capability: "summarize", Input: json.RawMessage(`null`)}, true},
		{"invalid targetAgentId", NestedRequest{TargetAgentID: "agent target", Capability: "summarize", Input: json.RawMessage(`{}`)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("NestedRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewClientValidation(t *testing.T) {
	tests := []struct {
		name      string
		doer      HTTPDoer
		routerURL string
		token     string
		wantErr   bool
	}{
		{"valid", http.DefaultClient, "https://router.example.dev", "token123", false},
		{"nil doer", nil, "https://router.example.dev", "token123", true},
		{"empty url", http.DefaultClient, "", "token123", true},
		{"empty token", http.DefaultClient, "https://router.example.dev", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.doer, tt.routerURL, tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientInvokeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/v1/invocations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected accept: %s", r.Header.Get("Accept"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if body["parentInvocationId"] != "inv_parent123" {
			t.Errorf("unexpected parentInvocationId: %v", body["parentInvocationId"])
		}
		if body["targetAgentId"] != "agent_target02" {
			t.Errorf("unexpected targetAgentId: %v", body["targetAgentId"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schemaVersion": "1",
			"invocationId":  "inv_child999",
			"rootTaskId":    "task_root456",
			"traceId":       "trc_abc123_1",
			"status":        "succeeded",
			"result":        map[string]any{"answer": "42"},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	result, err := client.Invoke(context.Background(), validContext(), validNestedRequest())
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.InvocationID != "inv_child999" {
		t.Errorf("unexpected invocationId: %s", result.InvocationID)
	}
	if result.Status != "succeeded" {
		t.Errorf("unexpected status: %s", result.Status)
	}
}

func TestClientInvokeInvalidContext(t *testing.T) {
	client, err := NewClient(http.DefaultClient, "https://router.example.dev", "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.Invoke(context.Background(), PlatformContext{}, validNestedRequest())
	if err == nil {
		t.Error("Invoke() should fail with invalid context")
	}
}

func TestClientInvokeRouterError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "UNAUTHENTICATED",
			"message": "Authentication is required.",
			"traceId": "trc_test123_1",
		})
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "bad-token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.Invoke(context.Background(), validContext(), validNestedRequest())
	if err == nil {
		t.Error("Invoke() should fail with router error")
	}
	var routerErr *RouterError
	if !errors.As(err, &routerErr) {
		t.Errorf("expected RouterError, got %T", err)
	}
	if routerErr != nil && routerErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("unexpected status code: %d", routerErr.StatusCode)
	}
}

func TestClientInvokeSSEAccept(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("expected text/event-stream accept, got: %s", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {}\n\n"))
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	req := validNestedRequest()
	req.Stream = true
	result, err := client.Invoke(context.Background(), validContext(), req)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(result.Body) == 0 {
		t.Error("expected non-empty SSE body")
	}
}
