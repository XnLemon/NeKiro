package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/invocation"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace"
	"github.com/Nene7ko/NeKiro/contracts"
)

type invocationAuthenticatorStub struct {
	caller catalog.AuthenticatedCaller
	err    error
}

func (stub invocationAuthenticatorStub) Authenticate(*http.Request) (catalog.AuthenticatedCaller, error) {
	return stub.caller, stub.err
}

type invocationDispatcherStub struct {
	caller      workspace.AuthenticatedCaller
	traceID     contracts.TraceID
	workspaceID string
	request     contracts.InvokeAgentRequest
	input       []byte
	mode        contracts.InvocationResultMode
	response    *invocation.RouterResponse
	err         error
	calls       int
}

func (stub *invocationDispatcherStub) Dispatch(_ context.Context, caller workspace.AuthenticatedCaller, traceID contracts.TraceID, workspaceID string, request contracts.InvokeAgentRequest, input []byte, mode contracts.InvocationResultMode) (*invocation.RouterResponse, error) {
	stub.calls++
	stub.caller, stub.traceID, stub.workspaceID, stub.request, stub.input, stub.mode = caller, traceID, workspaceID, request, append([]byte(nil), input...), mode
	return stub.response, stub.err
}

func TestInvocationHandlerStrictlyRejectsPreDispatchRequests(t *testing.T) {
	tests := []struct {
		name        string
		authErr     error
		contentType string
		accept      string
		body        string
		limit       int64
		status      int
		code        contracts.PlatformErrorCode
	}{
		{name: "authentication first", authErr: ErrUnauthenticated, contentType: "text/plain", accept: "", body: "bad", limit: 1024, status: 401, code: contracts.ErrorCodeUnauthenticated},
		{name: "content type parameters", contentType: "application/json; charset=utf-8", accept: "application/json", body: validInvokeBody(false), limit: 1024, status: 400, code: contracts.ErrorCodeValidationError},
		{name: "unknown member", contentType: "application/json", accept: "application/json", body: `{"agentId":"agent-a","capability":"cap-a","input":{},"stream":false,"extra":1}`, limit: 1024, status: 400, code: contracts.ErrorCodeValidationError},
		{name: "duplicate member", contentType: "application/json", accept: "application/json", body: `{"agentId":"agent-a","agentId":"agent-b","capability":"cap-a","input":{},"stream":false}`, limit: 1024, status: 400, code: contracts.ErrorCodeValidationError},
		{name: "null input", contentType: "application/json", accept: "application/json", body: `{"agentId":"agent-a","capability":"cap-a","input":null,"stream":false}`, limit: 1024, status: 400, code: contracts.ErrorCodeValidationError},
		{name: "accept mismatch", contentType: "application/json", accept: "text/event-stream", body: validInvokeBody(false), limit: 1024, status: 406, code: contracts.ErrorCodeNotAcceptable},
		{name: "payload overflow", contentType: "application/json", accept: "application/json", body: validInvokeBody(false), limit: 16, status: 413, code: contracts.ErrorCodePayloadTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dispatcher := &invocationDispatcherStub{}
			handler := newInvocationTestHandler(t, invocationAuthenticatorStub{caller: catalog.AuthenticatedCaller{ID: "owner-a"}, err: test.authErr}, dispatcher, test.limit)
			request := httptest.NewRequest(http.MethodPost, "/v4/workspaces/workspace-a/invocations", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			request.Header.Set("Accept", test.accept)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status || dispatcher.calls != 0 {
				t.Fatalf("status=%d dispatch calls=%d body=%s", response.Code, dispatcher.calls, response.Body.String())
			}
			var platformError contracts.PreCorrelationPlatformErrorV4
			if err := json.Unmarshal(response.Body.Bytes(), &platformError); err != nil || platformError.Code != test.code || platformError.TraceID == "" {
				t.Fatalf("platform error=%#v decode=%v", platformError, err)
			}
			var document map[string]any
			_ = json.Unmarshal(response.Body.Bytes(), &document)
			if _, exists := document["invocationId"]; exists {
				t.Fatal("pre-dispatch error contains Invocation ID")
			}
		})
	}
}

func TestInvocationHandlerForwardsExactJSONAndTrustedArguments(t *testing.T) {
	result := `{"schemaVersion":"1","invocationId":"inv-root","rootTaskId":"task-root","traceId":"trc_00000000000000000000000000000000_1","status":"succeeded","result":{"answer":42}}`
	headers := http.Header{}
	headers.Set(TraceHeader, "router-trace")
	dispatcher := &invocationDispatcherStub{response: &invocation.RouterResponse{StatusCode: 200, ContentType: "application/json", Headers: headers, Body: io.NopCloser(strings.NewReader(result))}}
	handler := newInvocationTestHandler(t, invocationAuthenticatorStub{caller: catalog.AuthenticatedCaller{ID: "owner-a", AuthenticationKind: "development-static"}}, dispatcher, 4096)
	request := httptest.NewRequest(http.MethodPost, "/v4/workspaces/workspace-a/invocations", strings.NewReader(validInvokeBody(false)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/*")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || response.Body.String() != result || dispatcher.calls != 1 || dispatcher.caller.ID != "owner-a" || dispatcher.workspaceID != "workspace-a" || dispatcher.request.AgentID != "agent-a" || dispatcher.request.Capability != "capability.read" || dispatcher.request.Stream || string(dispatcher.input) != `{"query":"hello"}` || dispatcher.mode != contracts.InvocationResultModeJSON {
		t.Fatalf("response=%d %q dispatcher=%#v", response.Code, response.Body.String(), dispatcher)
	}
	if response.Header().Get(TraceHeader) != string(dispatcher.traceID) || response.Header().Get(TraceHeader) == "router-trace" {
		t.Fatalf("trace header = %q, Gateway trace = %q", response.Header().Get(TraceHeader), dispatcher.traceID)
	}
}

func TestInvocationHandlerMapsWorkspaceBeforeRootAndRouterAfterRootErrors(t *testing.T) {
	t.Run("Workspace policy is pre-correlation", func(t *testing.T) {
		dispatcher := &invocationDispatcherStub{err: workspace.ErrInstallationDisabled}
		response := invokeWithTestHandler(t, dispatcher)
		if response.Code != http.StatusConflict {
			t.Fatalf("status=%d", response.Code)
		}
		var document map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &document)
		if document["code"] != string(contracts.ErrorCodeInstallationDisabled) || document["invocationId"] != nil {
			t.Fatalf("error=%v", document)
		}
	})
	t.Run("Release state is exact and pre-correlation", func(t *testing.T) {
		dispatcher := &invocationDispatcherStub{err: workspace.ErrReleaseRevoked}
		response := invokeWithTestHandler(t, dispatcher)
		if response.Code != http.StatusConflict {
			t.Fatalf("status=%d", response.Code)
		}
		var document map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &document)
		if document["code"] != string(contracts.ErrorCodeAgentReleaseRevoked) || document["invocationId"] != nil {
			t.Fatalf("error=%v", document)
		}
	})
	t.Run("Router dependency is correlated", func(t *testing.T) {
		dispatcher := &invocationDispatcherStub{err: &invocation.DispatchError{Code: contracts.ErrorCodeDependency, InvocationID: "inv-root", RootTaskID: "task-root", Cause: errors.New("offline")}}
		response := invokeWithTestHandler(t, dispatcher)
		var platformError contracts.CorrelatedPlatformErrorV4
		if response.Code != http.StatusServiceUnavailable || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.InvocationID != "inv-root" || platformError.RootTaskID != "task-root" || platformError.Code != contracts.ErrorCodeDependency {
			t.Fatalf("status=%d error=%#v", response.Code, platformError)
		}
	})
	t.Run("internal failure before root is uncorrelated HTTP 500", func(t *testing.T) {
		dispatcher := &invocationDispatcherStub{err: &invocation.DispatchError{Code: contracts.ErrorCodeInternal, Cause: errors.New("id generation failed")}}
		response := invokeWithTestHandler(t, dispatcher)
		var platformError contracts.PreCorrelationPlatformErrorV4
		if response.Code != http.StatusInternalServerError || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeInternal {
			t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
		}
	})
	t.Run("internal failure after root is correlated HTTP 500", func(t *testing.T) {
		traceID := contracts.TraceID("trc_00000000000000000000000000000000_1")
		platformError, err := contracts.NewCorrelatedPlatformErrorV4(contracts.ErrorCodeInternal, traceID, "inv-root", "task-root")
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(platformError)
		if err != nil {
			t.Fatal(err)
		}
		headers := http.Header{}
		headers.Set(TraceHeader, "router-trace")
		dispatcher := &invocationDispatcherStub{response: &invocation.RouterResponse{StatusCode: http.StatusInternalServerError, ContentType: "application/json", Headers: headers, Body: io.NopCloser(bytes.NewReader(body))}}
		response := invokeWithTestHandler(t, dispatcher)
		var got contracts.CorrelatedPlatformErrorV4
		if response.Code != http.StatusInternalServerError || json.Unmarshal(response.Body.Bytes(), &got) != nil || got.Code != contracts.ErrorCodeInternal || got.InvocationID != "inv-root" || response.Header().Get(TraceHeader) != string(traceID) {
			t.Fatalf("status=%d error=%#v trace=%q", response.Code, got, response.Header().Get(TraceHeader))
		}
	})
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func (recorder *flushRecorder) Flush() {
	recorder.flushes++
	recorder.ResponseRecorder.Flush()
}

func TestProxySSEForwardsOneBoundedEventPerFlush(t *testing.T) {
	source := "data: {\"sequence\":0}\n\ndata: {\"sequence\":1}\n\n"
	recorder := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	if err := proxySSE(recorder, strings.NewReader(source), 64); err != nil {
		t.Fatal(err)
	}
	if recorder.Body.String() != source || recorder.flushes != 2 {
		t.Fatalf("body=%q flushes=%d", recorder.Body.String(), recorder.flushes)
	}
	if err := proxySSE(&flushRecorder{ResponseRecorder: httptest.NewRecorder()}, strings.NewReader("data: {\"oversized\":true}\n\n"), 10); !errors.Is(err, errInvocationPayloadTooLarge) {
		t.Fatalf("oversized SSE error=%v", err)
	}
	for _, invalid := range []string{"event: x\ndata: {}\n\n", "data: {}\r\n\r\n", "data: {}\n"} {
		if err := proxySSE(&flushRecorder{ResponseRecorder: httptest.NewRecorder()}, strings.NewReader(invalid), 64); err == nil {
			t.Fatalf("invalid SSE accepted: %q", invalid)
		}
	}
}

func validInvokeBody(stream bool) string {
	if stream {
		return `{"agentId":"agent-a","capability":"capability.read","input":{"query":"hello"},"stream":true}`
	}
	return `{"agentId":"agent-a","capability":"capability.read","input":{"query":"hello"},"stream":false}`
}

func newInvocationTestHandler(t *testing.T, authenticator Authenticator, dispatcher InvocationDispatcher, requestLimit int64) http.Handler {
	t.Helper()
	traces, err := newTraceGenerator(bytes.NewReader(make([]byte, 16)))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewInvocationHandler(authenticator, dispatcher, traces, slog.New(slog.NewTextHandler(io.Discard, nil)), requestLimit, 4096, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

func invokeWithTestHandler(t *testing.T, dispatcher InvocationDispatcher) *httptest.ResponseRecorder {
	t.Helper()
	handler := newInvocationTestHandler(t, invocationAuthenticatorStub{caller: catalog.AuthenticatedCaller{ID: "owner-a"}}, dispatcher, 4096)
	request := httptest.NewRequest(http.MethodPost, "/v4/workspaces/workspace-a/invocations", strings.NewReader(validInvokeBody(false)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
