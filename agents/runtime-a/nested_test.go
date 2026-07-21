package runtimea

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	agentsdk "github.com/Nene7ko/NeKiro/sdks/agent-sdk"
	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
)

func httptestNewServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func newClient(t *testing.T, server *httptest.Server, headers map[string]string) *a2aclient.Client {
	t.Helper()
	meta := make(a2aclient.CallMeta, len(headers))
	for name, value := range headers {
		meta[name] = []string{value}
	}
	client, err := a2aclient.NewFromEndpoints(t.Context(), []a2a.AgentInterface{{
		URL: server.URL, Transport: a2a.TransportProtocolJSONRPC,
	}}, a2aclient.WithJSONRPCTransport(server.Client()), a2aclient.WithInterceptors(a2aclient.NewStaticCallMetaInjector(meta)))
	if err != nil {
		t.Fatalf("create A2A client: %v", err)
	}
	t.Cleanup(func() { _ = client.Destroy() })
	return client
}

type recordingInvoker struct {
	mu     sync.Mutex
	calls  []nestedCall
	err    error
	result func(agentsdk.PlatformContext) *agentsdk.NestedResult
}

type nestedCall struct {
	Context agentsdk.PlatformContext
	Request agentsdk.NestedRequest
}

func (invoker *recordingInvoker) Invoke(_ context.Context, platformContext agentsdk.PlatformContext, request agentsdk.NestedRequest) (*agentsdk.NestedResult, error) {
	invoker.mu.Lock()
	invoker.calls = append(invoker.calls, nestedCall{Context: platformContext, Request: request})
	invoker.mu.Unlock()
	if invoker.err != nil {
		return nil, invoker.err
	}
	return invoker.result(platformContext), nil
}

func TestRuntimeAUsesOneManagedNestedCallAndReturnsCombinedResult(t *testing.T) {
	config, err := LoadConfig(lookupEnvironment(validEnvironment()))
	if err != nil {
		t.Fatal(err)
	}
	invoker := &recordingInvoker{result: func(contextValue agentsdk.PlatformContext) *agentsdk.NestedResult {
		return &agentsdk.NestedResult{
			InvocationID: "child-1",
			RootTaskID:   contextValue.RootTaskID,
			TraceID:      contextValue.TraceID,
			Status:       "succeeded",
			Result:       json.RawMessage(`{"agent":"runtime-b","value":"ok"}`),
		}
	}}
	handler, err := newHandlerWithInvoker(config, invoker)
	if err != nil {
		t.Fatal(err)
	}
	server := httptestNewServer(t, NewHTTPHandler(handler))
	client := newClient(t, server, map[string]string{
		"x-nek-trace-id":      "trace-1",
		"x-nek-invocation-id": "root-1",
		"x-nek-root-task-id":  "task-1",
		"x-nek-workspace-id":  "workspace-1",
	})
	result, err := client.SendMessage(t.Context(), &a2a.MessageSendParams{Message: &a2a.Message{
		ID: "root-1", Role: a2a.MessageRoleUser,
		Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{"fixture": "success", "value": "cross-runtime"}}},
	}})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	message, ok := result.(*a2a.Message)
	if !ok || len(message.Parts) != 1 {
		t.Fatalf("result = %#v", result)
	}
	data, ok := message.Parts[0].(a2a.DataPart)
	if !ok || data.Data["agent"] != "runtime-a" || data.Data["childInvocationId"] != "child-1" {
		t.Fatalf("result data = %#v", data.Data)
	}
	invoker.mu.Lock()
	defer invoker.mu.Unlock()
	if len(invoker.calls) != 1 || invoker.calls[0].Context.RootTaskID != "task-1" || invoker.calls[0].Request.TargetAgentID != config.TargetAgentID || invoker.calls[0].Request.Stream {
		t.Fatalf("nested calls = %#v", invoker.calls)
	}
}

func TestRootInputRejectsUnexpectedShape(t *testing.T) {
	tests := []*a2a.Message{
		{ID: "missing-part"},
		{ID: "wrong-role", Role: a2a.MessageRoleAgent, Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{"fixture": "success", "value": "x"}}}},
		{ID: "wrong-fixture", Role: a2a.MessageRoleUser, Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{"fixture": "failure", "value": "x"}}}},
		{ID: "extra-field", Role: a2a.MessageRoleUser, Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{"fixture": "success", "value": "x", "extra": true}}}},
	}
	for _, message := range tests {
		if _, err := rootInput(message); err == nil {
			t.Errorf("rootInput(%s) accepted invalid message", message.ID)
		}
	}
}

func TestRuntimeAHidesRawNestedFailureAndDoesNotRetry(t *testing.T) {
	config, err := LoadConfig(lookupEnvironment(validEnvironment()))
	if err != nil {
		t.Fatal(err)
	}
	invoker := &recordingInvoker{err: errors.New("raw dependency secret opaque-token")}
	handler, err := newHandlerWithInvoker(config, invoker)
	if err != nil {
		t.Fatal(err)
	}
	server := httptestNewServer(t, NewHTTPHandler(handler))
	client := newClient(t, server, map[string]string{
		"x-nek-trace-id":      "trace-1",
		"x-nek-invocation-id": "root-1",
		"x-nek-root-task-id":  "task-1",
		"x-nek-workspace-id":  "workspace-1",
	})
	_, err = client.SendMessage(t.Context(), &a2a.MessageSendParams{Message: &a2a.Message{
		ID: "root-1", Role: a2a.MessageRoleUser,
		Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{"fixture": "success", "value": "cross-runtime"}}},
	}})
	if err == nil {
		t.Fatal("nested dependency failure was returned as success")
	}
	if strings.Contains(err.Error(), "opaque-token") || strings.Contains(err.Error(), "raw dependency") {
		t.Fatalf("raw nested failure escaped: %v", err)
	}
	invoker.mu.Lock()
	defer invoker.mu.Unlock()
	if len(invoker.calls) != 1 {
		t.Fatalf("nested call count = %d, want 1", len(invoker.calls))
	}
}

func TestRuntimeARejectsMissingManagedContextBeforeNestedCall(t *testing.T) {
	config, err := LoadConfig(lookupEnvironment(validEnvironment()))
	if err != nil {
		t.Fatal(err)
	}
	invoker := &recordingInvoker{result: func(contextValue agentsdk.PlatformContext) *agentsdk.NestedResult {
		return &agentsdk.NestedResult{InvocationID: "unexpected", RootTaskID: contextValue.RootTaskID, TraceID: contextValue.TraceID, Status: "succeeded", Result: json.RawMessage(`{}`)}
	}}
	handler, err := newHandlerWithInvoker(config, invoker)
	if err != nil {
		t.Fatal(err)
	}
	server := httptestNewServer(t, NewHTTPHandler(handler))
	client := newClient(t, server, map[string]string{
		"x-nek-trace-id":      "trace-1",
		"x-nek-invocation-id": "root-1",
		"x-nek-root-task-id":  "task-1",
	})
	_, err = client.SendMessage(t.Context(), &a2a.MessageSendParams{Message: &a2a.Message{
		ID: "root-1", Role: a2a.MessageRoleUser,
		Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{"fixture": "success", "value": "x"}}},
	}})
	if err == nil {
		t.Fatal("missing Workspace context was accepted")
	}
	invoker.mu.Lock()
	defer invoker.mu.Unlock()
	if len(invoker.calls) != 0 {
		t.Fatalf("nested call count = %d, want 0", len(invoker.calls))
	}
}

type capturedDoer struct {
	request *http.Request
	body    string
}

func (doer *capturedDoer) Do(request *http.Request) (*http.Response, error) {
	doer.request = request
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-nek-trace-id": []string{"trace-1"}},
		Body:       io.NopCloser(strings.NewReader(doer.body)),
	}, nil
}

func TestSDKNestedCallUsesRouterOnlyAndExactContractShape(t *testing.T) {
	config, err := LoadConfig(lookupEnvironment(validEnvironment()))
	if err != nil {
		t.Fatal(err)
	}
	doer := &capturedDoer{body: `{"schemaVersion":"1","invocationId":"child-1","rootTaskId":"task-1","traceId":"trace-1","status":"succeeded","result":{"agent":"runtime-b"}}`}
	sdk, err := agentsdk.NewClient(doer, config.RouterURL, config.RouterToken, config.ResponseLimit, config.EventLimit)
	if err != nil {
		t.Fatal(err)
	}
	service, err := newNestedService(config, sdk)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.invokeWithContext(t.Context(), agentsdk.PlatformContext{InvocationID: "root-1", RootTaskID: "task-1", TraceID: "trace-1", WorkspaceID: "workspace-1", AgentID: config.AgentID}, json.RawMessage(`{"fixture":"success","value":"x"}`))
	if err != nil || result.InvocationID != "child-1" {
		t.Fatalf("nested result = (%#v, %v)", result, err)
	}
	if doer.request.URL.String() != config.RouterURL+"/agent/v1/invocations" {
		t.Fatalf("request URL = %s", doer.request.URL)
	}
	if doer.request.Header.Get("Authorization") != "Bearer "+config.RouterToken {
		t.Fatalf("authorization header = %q", doer.request.Header.Get("Authorization"))
	}
	body, err := io.ReadAll(doer.request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"parentInvocationId":"root-1"`)) || bytes.Contains(body, []byte("router.example")) {
		t.Fatalf("nested request body = %s", body)
	}
}

func TestSDKRejectsChangedChildCorrelationWithoutRetry(t *testing.T) {
	config, err := LoadConfig(lookupEnvironment(validEnvironment()))
	if err != nil {
		t.Fatal(err)
	}
	doer := &capturedDoer{body: `{"schemaVersion":"1","invocationId":"child-1","rootTaskId":"other-task","traceId":"trace-1","status":"succeeded","result":{"agent":"runtime-b"}}`}
	sdk, err := agentsdk.NewClient(doer, config.RouterURL, config.RouterToken, config.ResponseLimit, config.EventLimit)
	if err != nil {
		t.Fatal(err)
	}
	service, err := newNestedService(config, sdk)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.invokeWithContext(t.Context(), agentsdk.PlatformContext{InvocationID: "root-1", RootTaskID: "task-1", TraceID: "trace-1", WorkspaceID: "workspace-1", AgentID: config.AgentID}, json.RawMessage(`{"fixture":"success","value":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "correlation") {
		t.Fatalf("correlation mismatch error = %v", err)
	}
	if doer.request == nil {
		t.Fatal("SDK request was not made")
	}
}

func TestSDKRejectsChildResultReusingParentInvocationID(t *testing.T) {
	config, err := LoadConfig(lookupEnvironment(validEnvironment()))
	if err != nil {
		t.Fatal(err)
	}
	doer := &capturedDoer{body: `{"schemaVersion":"1","invocationId":"root-1","rootTaskId":"task-1","traceId":"trace-1","status":"succeeded","result":{"agent":"runtime-b"}}`}
	sdk, err := agentsdk.NewClient(doer, config.RouterURL, config.RouterToken, config.ResponseLimit, config.EventLimit)
	if err != nil {
		t.Fatal(err)
	}
	service, err := newNestedService(config, sdk)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.invokeWithContext(t.Context(), agentsdk.PlatformContext{InvocationID: "root-1", RootTaskID: "task-1", TraceID: "trace-1", WorkspaceID: "workspace-1", AgentID: config.AgentID}, json.RawMessage(`{"fixture":"success","value":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "differ") {
		t.Fatalf("parent/child identity error = %v", err)
	}
}

func TestCombinedResultIsDeterministic(t *testing.T) {
	child := &agentsdk.NestedResult{InvocationID: "child-1", RootTaskID: "task-1", TraceID: "trace-1", Status: "succeeded", Result: json.RawMessage(`{"agent":"runtime-b","value":1}`)}
	first, err := combinedResult(child)
	if err != nil {
		t.Fatal(err)
	}
	second, err := combinedResult(child)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("combined results differ: %s != %s", first, second)
	}
}

func TestCombinedResultPreservesLargeJSONNumberTokens(t *testing.T) {
	child := &agentsdk.NestedResult{InvocationID: "child-1", RootTaskID: "task-1", TraceID: "trace-1", Status: "succeeded", Result: json.RawMessage(`{"value":9007199254740993}`)}
	combined, err := combinedResult(child)
	if err != nil {
		t.Fatal(err)
	}
	data, err := combinedData(combined)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(`9007199254740993`)) {
		t.Fatalf("large JSON number was changed: %s", wire)
	}
}
