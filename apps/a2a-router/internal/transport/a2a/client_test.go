package a2a

import (
	"context"
	"encoding/json"
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
		writer.Header().Set("Content-Type", "application/json")
		captured = request.Header.Clone()
		runtimeb.NewHTTPHandler(runtimeb.NewHandler()).ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)

	client, err := newTestClient(server.Client())
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

func TestClientSendNonStreamingMapsDispatchToRuntimeB(t *testing.T) {
	captured := make(http.Header)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		captured = request.Header.Clone()
		runtimeb.NewHTTPHandler(runtimeb.NewHandler()).ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)

	client, err := newTestClient(server.Client())
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	result, err := client.SendNonStreaming(t.Context(), contracts.DispatchInvocationRequestV3{
		InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
		Caller: contracts.Caller{Type: "user", ID: "owner-a"}, WorkspaceID: "workspace-a",
		TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", Capability: "capability-a",
		Input: json.RawMessage("{\"fixture\":\"success\",\"value\":{\"exact\":true}}"),
	}, contracts.ResolveAgentResponse{Card: targetCard(server.URL, "none", "capability-a")})
	if err != nil {
		t.Fatalf("SendNonStreaming = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(result, &document); err != nil {
		t.Fatalf("decode result: %v body=%s", err, result)
	}
	if document["kind"] != "message" || document["role"] != "agent" {
		t.Fatalf("result document = %#v", document)
	}
	assertHeader(t, captured, HeaderTraceID, "trace-a")
	assertHeader(t, captured, HeaderInvocationID, "inv-a")
	assertHeader(t, captured, HeaderRootTaskID, "task-a")
	assertHeader(t, captured, HeaderWorkspaceID, "workspace-a")
}

func TestClientSendMessageRequiresExplicitDependencies(t *testing.T) {
	if _, err := NewClient(nil, 4096, 4096); err == nil {
		t.Fatal("NewClient(nil) succeeded, want error")
	}
	client, err := newTestClient(http.DefaultClient)
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	if _, err := client.SendMessage(t.Context(), Target{}, ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, runtimeBMessageParams("message-a", "success", "ok")); err == nil {
		t.Fatal("SendMessage without target endpoint succeeded, want error")
	}
	target := testTarget("http://127.0.0.1:1")
	if _, err := client.SendMessage(t.Context(), target, ContextHeaders{}, runtimeBMessageParams("message-a", "success", "ok")); err == nil {
		t.Fatal("SendMessage without context headers succeeded, want error")
	}
	if _, err := client.SendMessage(t.Context(), target, ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, nil); err == nil {
		t.Fatal("SendMessage without params succeeded, want error")
	}
}

func TestClientClassifiesA2AJSONRPCFailureAsAgentExecutionFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		var call struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"jsonrpc": "2.0", "id": call.ID,
			"error": map[string]any{"code": -32603, "message": "agent failed"},
		})
	}))
	t.Cleanup(server.Close)
	client, err := newTestClient(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendMessage(t.Context(), testTarget(server.URL), ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, runtimeBMessageParams("message-a", "success", "ok"))
	if got := errorCode(err); got != contracts.ErrorCodeAgentExecutionFailed {
		t.Fatalf("error code = %q, want %q, err=%v", got, contracts.ErrorCodeAgentExecutionFailed, err)
	}
}

func TestClientClassifiesMalformedA2AResultAsProtocolError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		var call struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"jsonrpc": "2.0", "id": call.ID,
			"result": map[string]any{"kind": "unsupported"},
		})
	}))
	t.Cleanup(server.Close)
	client, err := newTestClient(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendMessage(t.Context(), testTarget(server.URL), ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, runtimeBMessageParams("message-a", "success", "ok"))
	if got := errorCode(err); got != contracts.ErrorCodeA2AProtocol {
		t.Fatalf("error code = %q, want %q, err=%v", got, contracts.ErrorCodeA2AProtocol, err)
	}
}

func TestClientRejectsMalformedMessageResultInNonStreamingDispatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		var call struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"jsonrpc": "2.0", "id": call.ID,
			"result": map[string]any{"kind": "message", "id": "agent-message", "role": "agent", "parts": []any{}},
		})
	}))
	t.Cleanup(server.Close)
	client, err := newTestClient(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendNonStreaming(t.Context(), contracts.DispatchInvocationRequestV3{
		InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
		Caller: contracts.Caller{Type: "user", ID: "owner-a"}, WorkspaceID: "workspace-a",
		TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", Capability: "capability-a",
		Input: json.RawMessage(`{"fixture":"success"}`),
	}, contracts.ResolveAgentResponse{Card: targetCard(server.URL, "none", "capability-a")})
	if got := errorCode(err); got != contracts.ErrorCodeA2AProtocol {
		t.Fatalf("error code = %q, want %q, err=%v", got, contracts.ErrorCodeA2AProtocol, err)
	}
}

func TestClientClassifiesHTTPFailureAsAgentUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "offline", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	client, err := newTestClient(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendMessage(t.Context(), testTarget(server.URL), ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, runtimeBMessageParams("message-a", "success", "ok"))
	if got := errorCode(err); got != contracts.ErrorCodeAgentUnavailable {
		t.Fatalf("error code = %q, want %q, err=%v", got, contracts.ErrorCodeAgentUnavailable, err)
	}
}

func TestClientClassifiesOversizedAgentResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var call struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"jsonrpc": "2.0", "id": call.ID,
			"result": map[string]any{"kind": "message", "id": "agent-message", "role": "agent", "parts": []any{map[string]any{"kind": "text", "text": "response larger than configured limit"}}},
		})
	}))
	defer server.Close()
	client, err := NewClient(server.Client(), 4096, 32)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendMessage(t.Context(), testTarget(server.URL), ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, runtimeBMessageParams("message-a", "success", "ok"))
	if got := errorCode(err); got != contracts.ErrorCodeAgentResponseTooLarge {
		t.Fatalf("error code = %q, want %q, err=%v", got, contracts.ErrorCodeAgentResponseTooLarge, err)
	}
}

func TestClientRejectsMalformedJSONRPCEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		jsonrpc    string
		responseID string
		both       bool
		unknown    bool
	}{
		{name: "invalid version", jsonrpc: "1.0", responseID: "match"},
		{name: "mismatched id", jsonrpc: "2.0", responseID: "other"},
		{name: "result and error", jsonrpc: "2.0", responseID: "match", both: true},
		{name: "unknown member", jsonrpc: "2.0", responseID: "match", unknown: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				var call struct {
					ID string `json:"id"`
				}
				if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				responseID := call.ID
				if test.responseID == "other" {
					responseID = "other"
				}
				response := map[string]any{
					"jsonrpc": test.jsonrpc,
					"id":      responseID,
					"result":  map[string]any{"kind": "message", "id": "agent-message", "role": "agent", "parts": []any{map[string]any{"kind": "text", "text": "ok"}}},
				}
				if test.both {
					response["error"] = map[string]any{"code": -32603, "message": "agent failed"}
				}
				if test.unknown {
					response["extra"] = true
				}
				_ = json.NewEncoder(writer).Encode(response)
			}))
			defer server.Close()
			client, err := newTestClient(server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.SendMessage(t.Context(), testTarget(server.URL), ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, runtimeBMessageParams("message-a", "success", "ok"))
			if got := errorCode(err); got != contracts.ErrorCodeA2AProtocol {
				t.Fatalf("error code = %q, want %q, err=%v", got, contracts.ErrorCodeA2AProtocol, err)
			}
		})
	}
}

func TestClientRejectsActiveA2ANegativeCorpus(t *testing.T) {
	tests := []struct {
		name string
		body func(requestID string) string
	}{
		{
			name: "missing result and error",
			body: func(requestID string) string {
				return `{"jsonrpc":"2.0","id":` + requestID + `}`
			},
		},
		{
			name: "boolean response id",
			body: func(string) string {
				return `{"jsonrpc":"2.0","id":true,"result":{"kind":"message"}}`
			},
		},
		{
			name: "object response id",
			body: func(string) string {
				return `{"jsonrpc":"2.0","id":{"request":"message-send-1"},"result":{"kind":"message"}}`
			},
		},
		{
			name: "array response id",
			body: func(string) string {
				return `{"jsonrpc":"2.0","id":["message-send-1"],"result":{"kind":"message"}}`
			},
		},
		{
			name: "trailing data",
			body: func(requestID string) string {
				return `{"jsonrpc":"2.0","id":` + requestID + `,"result":{"kind":"message"}} trailing`
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var call struct {
					ID json.RawMessage `json:"id"`
				}
				if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(test.body(string(call.ID))))
			}))
			defer server.Close()

			client, err := newTestClient(server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.SendMessage(t.Context(), testTarget(server.URL), ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, runtimeBMessageParams("message-a", "success", "ok"))
			if got := errorCode(err); got != contracts.ErrorCodeA2AProtocol {
				t.Fatalf("error code = %q, want %q, err=%v", got, contracts.ErrorCodeA2AProtocol, err)
			}
		})
	}
}

func TestClientRejectsDuplicateJSONRPCEnvelopeMember(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var call struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","jsonrpc":"2.0","id":"` + call.ID + `","result":{"kind":"message","id":"agent-message","role":"agent","parts":[{"kind":"text","text":"ok"}]}}`))
	}))
	defer server.Close()
	client, err := newTestClient(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendMessage(t.Context(), testTarget(server.URL), ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, runtimeBMessageParams("message-a", "success", "ok"))
	if got := errorCode(err); got != contracts.ErrorCodeA2AProtocol {
		t.Fatalf("error code = %q, want %q, err=%v", got, contracts.ErrorCodeA2AProtocol, err)
	}
}

func TestClientRejectsNonJSONRPCResponseMediaType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var call struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "text/plain")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"jsonrpc": "2.0", "id": call.ID,
			"result": map[string]any{"kind": "message", "id": "agent-message", "role": "agent", "parts": []any{map[string]any{"kind": "text", "text": "ok"}}},
		})
	}))
	defer server.Close()
	client, err := newTestClient(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendMessage(t.Context(), testTarget(server.URL), ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, runtimeBMessageParams("message-a", "success", "ok"))
	if got := errorCode(err); got != contracts.ErrorCodeA2AProtocol {
		t.Fatalf("error code = %q, want %q, err=%v", got, contracts.ErrorCodeA2AProtocol, err)
	}
}

func TestClientClassifiesDeadlineAsTimeout(t *testing.T) {
	client, err := newTestClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendMessage(t.Context(), testTarget("http://agent.example/a2a"), ContextHeaders{TraceID: "trace-a", InvocationID: "inv-a", RootTaskID: "task-a", WorkspaceID: "workspace-a"}, runtimeBMessageParams("message-a", "success", "ok"))
	if got := errorCode(err); got != contracts.ErrorCodeTimeout {
		t.Fatalf("error code = %q, want %q, err=%v", got, contracts.ErrorCodeTimeout, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func newTestClient(httpClient *http.Client) (*Client, error) {
	return NewClient(httpClient, 4096, 4096)
}

func testTarget(endpoint string) Target {
	return Target{Endpoint: endpoint, MaxInputBytes: 4096, MaxOutputBytes: 4096}
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
