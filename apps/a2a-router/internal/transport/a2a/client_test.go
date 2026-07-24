package a2a

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	runtimeb "github.com/Nene7ko/NeKiro/agents/runtime-b"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/credential"
	streammodel "github.com/Nene7ko/NeKiro/apps/a2a-router/internal/stream"
	"github.com/Nene7ko/NeKiro/contracts"
	a2ago "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
)

func TestClientSendMessageCallsRuntimeBWithPlatformContext(t *testing.T) {
	captured := make(http.Header)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		captured = request.Header.Clone()
		a2asrv.NewJSONRPCHandler(runtimeb.NewHandler()).ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)

	client, err := newTestClient(server.Client())
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	target, err := NewTarget(resolvedTarget(targetCard(server.URL, "none", "capability-a")), "capability-a")
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
	if values := captured.Values(contracts.RouterAgentAuthorizationHeader); len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		t.Fatalf("Authorization values = %v", values)
	}
}

func TestClientDoesNotFollowAgentRedirects(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalls++
	}))
	t.Cleanup(target.Close)
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", target.URL)
		writer.WriteHeader(http.StatusTemporaryRedirect)
	}))
	t.Cleanup(source.Close)

	client, err := newTestClient(source.Client())
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	_, err = client.SendNonStreaming(t.Context(), contracts.DispatchInvocationRequestV4{
		InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
		Caller: contracts.Caller{Type: "user", ID: "owner-a"}, WorkspaceID: "workspace-a",
		TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", Capability: "capability-a",
		Input: json.RawMessage(`{"fixture":"success","value":{"exact":true}}`),
	}, resolvedTarget(targetCard(source.URL, "none", "capability-a")))
	if err == nil {
		t.Fatal("Agent redirect accepted")
	}
	if targetCalls != 0 {
		t.Fatalf("redirect target calls = %d, want 0", targetCalls)
	}
}

func TestClassifyTransportCancellation(t *testing.T) {
	err := classifyTransportError(context.Canceled)
	var coded interface {
		PlatformErrorCode() contracts.PlatformErrorCode
	}
	if !errors.As(err, &coded) || coded.PlatformErrorCode() != contracts.ErrorCodeCanceled {
		t.Fatalf("classified error = %v", err)
	}
}

func TestClientSendNonStreamingMapsDispatchToRuntimeB(t *testing.T) {
	captured := make(http.Header)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		captured = request.Header.Clone()
		a2asrv.NewJSONRPCHandler(runtimeb.NewHandler()).ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)

	client, err := newTestClient(server.Client())
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	result, err := client.SendNonStreaming(t.Context(), contracts.DispatchInvocationRequestV4{
		InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
		Caller: contracts.Caller{Type: "user", ID: "owner-a"}, WorkspaceID: "workspace-a",
		TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", Capability: "capability-a",
		Input: json.RawMessage("{\"fixture\":\"success\",\"value\":{\"exact\":true}}"),
	}, resolvedTarget(targetCard(server.URL, "none", "capability-a")))
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

func TestClientSendStreamingMapsRuntimeBEventsAndTrustedHeaders(t *testing.T) {
	captured := make(http.Header)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		captured = request.Header.Clone()
		a2asrv.NewJSONRPCHandler(runtimeb.NewHandler()).ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)
	client, err := newTestClient(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	dispatch := contracts.DispatchInvocationRequestV4{
		InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
		Caller: contracts.Caller{Type: "user", ID: "owner-a"}, WorkspaceID: "workspace-a",
		TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", Capability: "capability-a",
		Input: json.RawMessage(`{"fixture":"stream-success","value":"stream"}`), Stream: true,
	}
	events := make([]streammodel.Event, 0, 5)
	for event, streamErr := range client.SendStreaming(t.Context(), dispatch, resolvedTarget(targetCard(server.URL, "none", "capability-a"))) {
		if streamErr != nil {
			t.Fatalf("stream error: %v", streamErr)
		}
		events = append(events, event)
	}
	if len(events) != 5 || events[0].Kind != "task" || events[len(events)-1].TerminalType != contracts.ResultStreamEventCompleted {
		t.Fatalf("events=%#v", events)
	}
	for index, event := range events {
		if event.TaskID == "" || event.ContextID == "" || len(event.Payload) == 0 || !json.Valid(event.Payload) {
			t.Fatalf("event %d=%#v", index, event)
		}
	}
	assertHeader(t, captured, HeaderTraceID, "trace-a")
	assertHeader(t, captured, HeaderInvocationID, "inv-a")
	assertHeader(t, captured, HeaderRootTaskID, "task-a")
	assertHeader(t, captured, HeaderWorkspaceID, "workspace-a")
}

func TestClientStreamingRejectsInvalidJSONRPCEnvelopeBeforeEventMapping(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "boolean id", data: `{"jsonrpc":"2.0","id":true,"result":{"kind":"task","id":"task-a","contextId":"ctx-a","status":{"state":"working"}}}`},
		{name: "missing result and error", data: `{"jsonrpc":"2.0","id":"ignored"}`},
		{name: "trailing data", data: `{"jsonrpc":"2.0","id":"ignored","result":{"kind":"task"}} trailing`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = writer.Write([]byte("data: " + test.data + "\n\n"))
			}))
			t.Cleanup(server.Close)
			client, err := newTestClient(server.Client())
			if err != nil {
				t.Fatal(err)
			}
			dispatch := contracts.DispatchInvocationRequestV4{
				InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
				Caller: contracts.Caller{Type: "user", ID: "owner-a"}, WorkspaceID: "workspace-a",
				TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", Capability: "capability-a",
				Input: json.RawMessage(`{"fixture":"stream-success","value":"stream"}`), Stream: true,
			}
			for _, streamErr := range client.SendStreaming(t.Context(), dispatch, resolvedTarget(targetCard(server.URL, "none", "capability-a"))) {
				if streamErr == nil || errorCode(streamErr) != contracts.ErrorCodeA2AProtocol {
					t.Fatalf("stream error=%v", streamErr)
				}
				return
			}
			t.Fatal("stream completed without envelope error")
		})
	}
}

func TestClientSendMessageRequiresExplicitDependencies(t *testing.T) {
	if _, err := NewClient(nil, nil, 4096, 4096, 4096, 4096); err == nil {
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
	_, err = client.SendNonStreaming(t.Context(), contracts.DispatchInvocationRequestV4{
		InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
		Caller: contracts.Caller{Type: "user", ID: "owner-a"}, WorkspaceID: "workspace-a",
		TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", Capability: "capability-a",
		Input: json.RawMessage(`{"fixture":"success"}`),
	}, resolvedTarget(targetCard(server.URL, "none", "capability-a")))
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
	issuer, err := credential.NewIssuer(credential.Config{Issuer: "https://a2a-router.nekiro.test", KeyID: "router-key-1", PrivateKey: ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)), TTL: 30 * time.Second}, time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(server.Client(), issuer, 4096, 32, 4096, 4096)
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
		name           string
		body           func(requestID string) string
		causeSubstring string
	}{
		{
			name: "missing result and error",
			body: func(requestID string) string {
				return `{"jsonrpc":"2.0","id":` + requestID + `}`
			},
		},
		{
			name:           "boolean response id",
			causeSubstring: "unsupported JSON type",
			body: func(string) string {
				return `{"jsonrpc":"2.0","id":true,"result":{"kind":"message"}}`
			},
		},
		{
			name:           "object response id",
			causeSubstring: "unsupported JSON type",
			body: func(string) string {
				return `{"jsonrpc":"2.0","id":{"request":"message-send-1"},"result":{"kind":"message"}}`
			},
		},
		{
			name:           "array response id",
			causeSubstring: "unsupported JSON type",
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
			if test.causeSubstring != "" {
				var classified *classifiedError
				if !errors.As(err, &classified) || !strings.Contains(classified.cause.Error(), test.causeSubstring) {
					t.Fatalf("error cause = %v, want substring %q", err, test.causeSubstring)
				}
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
	issuer, err := credential.NewIssuer(credential.Config{Issuer: "https://a2a-router.nekiro.test", KeyID: "router-key-1", PrivateKey: ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)), TTL: 30 * time.Second}, time.Now, rand.Reader)
	if err != nil {
		return nil, err
	}
	return NewClient(httpClient, issuer, 4096, 4096, 4096, 4096)
}

func testTarget(endpoint string) Target {
	return Target{AgentID: "agent-a", Version: "1.0.0", Capability: "capability-a", Endpoint: endpoint, Audience: "http://agent.example", ReleaseID: "release-a", CardDigest: strings.Repeat("a", 64), MaxInputBytes: 4096, MaxOutputBytes: 4096}
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
