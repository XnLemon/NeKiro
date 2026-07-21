package runtimea

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/a2aproject/a2a-go/a2a"
)

// TestRuntimeARootToRouterEndToEnd covers the complete child slice boundary:
// A2A root request -> trpc Runner -> real Agent SDK HTTP request -> Router
// Agent v1 fixture -> deterministic callee result -> A2A root response.
func TestRuntimeARootToRouterEndToEnd(t *testing.T) {
	var mu sync.Mutex
	var nestedRequest contracts.NestedInvocationRequestV1
	requestCount := 0
	router := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/agent/v1/invocations" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer opaque-token" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil || json.Unmarshal(body, &nestedRequest) != nil {
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		requestCount++
		mu.Unlock()
		var nestedInput map[string]json.RawMessage
		if err := json.Unmarshal(nestedRequest.Input, &nestedInput); err != nil || string(nestedInput["fixture"]) != `"success"` || string(nestedInput["value"]) != `"cross-runtime"` {
			http.Error(writer, "invalid nested input", http.StatusBadRequest)
			return
		}
		childMessage := json.RawMessage(`{"agent":"runtime-b","fixture":"success","value":"cross-runtime"}`)
		result, err := json.Marshal(contracts.InvocationResult{
			SchemaVersion: contracts.InvocationResultSchemaVersion,
			InvocationID:  "child-1",
			RootTaskID:    "task-e2e",
			TraceID:       "trace-e2e",
			Status:        "succeeded",
			Result:        childMessage,
		})
		if err != nil {
			http.Error(writer, "encode result", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("x-nek-trace-id", "trace-e2e")
		_, _ = writer.Write(result)
	}))
	t.Cleanup(router.Close)

	config, err := LoadConfig(lookupEnvironment(validEnvironment()))
	if err != nil {
		t.Fatal(err)
	}
	config.RouterURL = router.URL
	handler, err := NewHandler(config, router.Client())
	if err != nil {
		t.Fatal(err)
	}
	runtimeA := httptestNewServer(t, NewHTTPHandler(handler))
	rootClient := newClient(t, runtimeA, map[string]string{
		"x-nek-trace-id":      "trace-e2e",
		"x-nek-invocation-id": "root-e2e",
		"x-nek-root-task-id":  "task-e2e",
		"x-nek-workspace-id":  "workspace-e2e",
	})
	result, err := rootClient.SendMessage(t.Context(), &a2a.MessageSendParams{Message: &a2a.Message{
		ID: "root-e2e", Role: a2a.MessageRoleUser,
		Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{"fixture": "success", "value": "cross-runtime"}}},
	}})
	if err != nil {
		t.Fatalf("root SendMessage() error = %v", err)
	}
	message, ok := result.(*a2a.Message)
	if !ok || len(message.Parts) != 1 {
		t.Fatalf("root result = %#v", result)
	}
	data, ok := message.Parts[0].(a2a.DataPart)
	childData, childOK := data.Data["childResult"].(map[string]any)
	if !ok || data.Data["agent"] != "runtime-a" || data.Data["childInvocationId"] != "child-1" || !childOK || childData["agent"] != "runtime-b" || childData["value"] != "cross-runtime" {
		t.Fatalf("root combined data = %#v", data.Data)
	}
	mu.Lock()
	defer mu.Unlock()
	if requestCount != 1 || nestedRequest.ParentInvocationID != "root-e2e" || nestedRequest.TargetAgentID != config.TargetAgentID || nestedRequest.Capability != config.Capability || nestedRequest.Stream {
		t.Fatalf("Router nested request = (%d, %#v)", requestCount, nestedRequest)
	}
}
