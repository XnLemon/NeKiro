package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/auth"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/resolution"
	"github.com/Nene7ko/NeKiro/contracts"
)

type authStub struct {
	caller auth.Caller
	err    error
}

func (stub authStub) Authenticate(*http.Request) (auth.Caller, error) {
	return stub.caller, stub.err
}

type resolverStub struct {
	request  contracts.ResolveAgentRequest
	response contracts.ResolveAgentResponse
	calls    int
	err      error
}

func (stub *resolverStub) Resolve(_ context.Context, request contracts.ResolveAgentRequest) (contracts.ResolveAgentResponse, error) {
	stub.calls++
	stub.request = request
	return stub.response, stub.err
}

type transportStub struct {
	dispatch  contracts.DispatchInvocationRequestV3
	resolved  contracts.ResolveAgentResponse
	result    json.RawMessage
	calls     int
	err       error
	targetErr error
}

type inputPreflightTransportStub struct {
	transportStub
	preflightErr   error
	preflightCalls int
}

func (stub *inputPreflightTransportStub) ValidateNonStreamingInput(contracts.DispatchInvocationRequestV3, contracts.ResolveAgentResponse) error {
	stub.preflightCalls++
	return stub.preflightErr
}

type codedTransportError struct {
	code  contracts.PlatformErrorCode
	cause error
}

func (err codedTransportError) Error() string { return string(err.code) }
func (err codedTransportError) Unwrap() error { return err.cause }
func (err codedTransportError) PlatformErrorCode() contracts.PlatformErrorCode {
	return err.code
}

func (stub *transportStub) SendNonStreaming(_ context.Context, dispatch contracts.DispatchInvocationRequestV3, resolved contracts.ResolveAgentResponse) (json.RawMessage, error) {
	stub.calls++
	stub.dispatch = dispatch
	stub.resolved = resolved
	return stub.result, stub.err
}

func (stub *transportStub) ValidateNonStreamingTarget(contracts.DispatchInvocationRequestV3, contracts.ResolveAgentResponse) error {
	return stub.targetErr
}

func (stub *transportStub) ValidateNonStreamingInput(contracts.DispatchInvocationRequestV3, contracts.ResolveAgentResponse) error {
	return nil
}

type ledgerRecorder struct {
	events       []contracts.InvocationEventV03
	failSequence int64
	err          error
}

type deadlineAwareLedgerRecorder struct {
	events []contracts.InvocationEventV03
}

func (recorder *deadlineAwareLedgerRecorder) Append(ctx context.Context, event contracts.InvocationEventV03) error {
	if event.Sequence >= 3 && ctx.Err() != nil {
		return ctx.Err()
	}
	recorder.events = append(recorder.events, event)
	return nil
}

type deadlineTransportStub struct{}

func (deadlineTransportStub) ValidateNonStreamingTarget(contracts.DispatchInvocationRequestV3, contracts.ResolveAgentResponse) error {
	return nil
}

func (deadlineTransportStub) ValidateNonStreamingInput(contracts.DispatchInvocationRequestV3, contracts.ResolveAgentResponse) error {
	return nil
}

func (deadlineTransportStub) SendNonStreaming(ctx context.Context, _ contracts.DispatchInvocationRequestV3, _ contracts.ResolveAgentResponse) (json.RawMessage, error) {
	<-ctx.Done()
	return nil, codedTransportError{code: contracts.ErrorCodeTimeout, cause: ctx.Err()}
}

func (recorder *ledgerRecorder) Append(_ context.Context, event contracts.InvocationEventV03) error {
	if recorder.err != nil && event.Sequence == recorder.failSequence {
		return recorder.err
	}
	recorder.events = append(recorder.events, event)
	return nil
}

func TestDispatchRejectsInvalidRequestsBeforeResolution(t *testing.T) {
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
		{name: "missing auth", authErr: auth.ErrUnauthenticated, contentType: "text/plain", accept: "", body: "bad", limit: 1024, status: 401, code: contracts.ErrorCodeUnauthenticated},
		{name: "wrong content type", contentType: "text/plain", accept: "application/json", body: validDispatchBody(false), limit: 1024, status: 400, code: contracts.ErrorCodeValidationError},
		{name: "duplicate field", contentType: "application/json", accept: "application/json", body: `{"invocationId":"inv-a","invocationId":"inv-b","rootTaskId":"task-a","traceId":"trace-a","caller":{"type":"user","id":"owner-a"},"workspaceId":"workspace-a","targetAgentId":"agent-a","agentCardVersion":"1.0.0","capability":"capability-a","input":{},"stream":false}`, limit: 1024, status: 400, code: contracts.ErrorCodeValidationError},
		{name: "accept mismatch", contentType: "application/json", accept: "text/event-stream", body: validDispatchBody(false), limit: 1024, status: 406, code: contracts.ErrorCodeNotAcceptable},
		{name: "payload too large", contentType: "application/json", accept: "application/json", body: validDispatchBody(false), limit: 10, status: 413, code: contracts.ErrorCodePayloadTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := &resolverStub{}
			handler := newDispatchTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}, err: test.authErr}, resolver, test.limit)
			response := invokeDispatch(handler, test.contentType, test.accept, test.body)
			if response.Code != test.status || resolver.calls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, resolver.calls, response.Body.String())
			}
			var platformError contracts.PreCorrelationPlatformErrorV4
			if err := json.Unmarshal(response.Body.Bytes(), &platformError); err != nil || platformError.Code != test.code {
				t.Fatalf("error=%#v decode=%v", platformError, err)
			}
			var document map[string]any
			_ = json.Unmarshal(response.Body.Bytes(), &document)
			if _, exists := document["invocationId"]; exists {
				t.Fatal("pre-correlation error contains invocationId")
			}
		})
	}
}

func TestDispatchResolvesExactRequestAndReturnsRouteNotFoundPlaceholder(t *testing.T) {
	resolver := &resolverStub{}
	handler := newDispatchTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, 4096)
	response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
	if response.Code != http.StatusServiceUnavailable || resolver.calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, resolver.calls, response.Body.String())
	}
	if resolver.request.InvocationID != "inv-a" || resolver.request.RootTaskID != "task-a" || resolver.request.TraceID != "trace-a" || resolver.request.WorkspaceID != "workspace-a" || resolver.request.AgentID != "agent-a" || resolver.request.Version != "1.0.0" || resolver.request.Capability != "capability-a" {
		t.Fatalf("resolve request=%#v", resolver.request)
	}
	var platformError contracts.CorrelatedPlatformErrorV4
	if json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeRouteNotFound || platformError.InvocationID != "inv-a" || platformError.RootTaskID != "task-a" || response.Header().Get(TraceHeader) != "trace-a" {
		t.Fatalf("error=%#v headers=%#v", platformError, response.Header())
	}
}

func TestDispatchAcceptsExactSemVerBuildMetadata(t *testing.T) {
	resolver := &resolverStub{}
	handler := newDispatchTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, 4096)
	body := strings.Replace(validDispatchBody(false), `"agentCardVersion":"1.0.0"`, `"agentCardVersion":"1.0.0+build.7"`, 1)
	response := invokeDispatch(handler, "application/json", "application/json", body)
	if response.Code != http.StatusServiceUnavailable || resolver.request.Version != "1.0.0+build.7" {
		t.Fatalf("status=%d version=%q body=%s", response.Code, resolver.request.Version, response.Body.String())
	}
}

func TestDispatchUsesNonStreamingTransportAndReturnsInvocationResult(t *testing.T) {
	resolved := contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}
	resolver := &resolverStub{response: resolved}
	transport := &transportStub{result: json.RawMessage("{\"kind\":\"message\",\"messageId\":\"agent-message\",\"role\":\"agent\",\"parts\":[{\"kind\":\"data\",\"data\":{\"ok\":true}}]}")}
	handler := newDispatchTransportTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, 4096)
	response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
	if response.Code != http.StatusOK || resolver.calls != 1 || transport.calls != 1 {
		t.Fatalf("status=%d resolver=%d transport=%d body=%s", response.Code, resolver.calls, transport.calls, response.Body.String())
	}
	var result contracts.InvocationResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v body=%s", err, response.Body.String())
	}
	if result.SchemaVersion != contracts.InvocationResultSchemaVersion || result.InvocationID != "inv-a" || result.RootTaskID != "task-a" || result.TraceID != "trace-a" || result.Status != "succeeded" || response.Header().Get(TraceHeader) != "trace-a" {
		t.Fatalf("result=%#v headers=%#v", result, response.Header())
	}
	if string(result.Result) != string(transport.result) {
		t.Fatalf("result payload=%s want %s", result.Result, transport.result)
	}
	if transport.dispatch.InvocationID != "inv-a" || len(transport.dispatch.Input) == 0 || transport.resolved.Card.AgentID != "agent-a" {
		t.Fatalf("transport dispatch=%#v resolved=%#v", transport.dispatch, transport.resolved)
	}
}

func TestDispatchWithLedgerCommitsTerminalSuccessBeforeResult(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &transportStub{result: json.RawMessage("{\"kind\":\"message\",\"messageId\":\"agent-message\",\"role\":\"agent\",\"parts\":[{\"kind\":\"data\",\"data\":{\"ok\":true}}]}")}
	ledger := &ledgerRecorder{}
	handler := newDispatchLedgerTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096)

	response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
	if response.Code != http.StatusOK || transport.calls != 1 {
		t.Fatalf("status=%d transport=%d body=%s", response.Code, transport.calls, response.Body.String())
	}
	assertLedgerLifecycle(t, ledger.events, []string{"created", "routing", "started", "succeeded"})
	terminal := ledger.events[len(ledger.events)-1]
	if terminal.Error != nil || terminal.LatencyMS == nil {
		t.Fatalf("terminal event=%#v", terminal)
	}
	var result contracts.InvocationResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Status != "succeeded" {
		t.Fatalf("result=%#v err=%v body=%s", result, err, response.Body.String())
	}
}

func TestDispatchWithLedgerRecordsTerminalFailureForTransportError(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &transportStub{err: errors.New("agent endpoint offline")}
	ledger := &ledgerRecorder{}
	handler := newDispatchLedgerTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096)

	response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
	var platformError contracts.CorrelatedPlatformErrorV4
	if response.Code != http.StatusServiceUnavailable || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeDependency {
		t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
	}
	assertLedgerLifecycle(t, ledger.events, []string{"created", "routing", "started", "failed"})
	terminal := ledger.events[len(ledger.events)-1]
	if terminal.Error == nil || terminal.Error.Code != contracts.ErrorCodeDependency || terminal.LatencyMS == nil {
		t.Fatalf("terminal event=%#v", terminal)
	}
}

func TestDispatchWithLedgerCommitsTimeoutTerminalAfterContextDeadline(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	ledger := &deadlineAwareLedgerRecorder{}
	handlerValue, err := NewDispatchHandlerWithTransportAndLedger(
		authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, deadlineTransportStub{}, ledger, 4096, 20*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handlerValue.RegisterRoutes(mux)
	response := invokeDispatch(mux, "application/json", "application/json", validDispatchBody(false))
	var platformError contracts.CorrelatedPlatformErrorV4
	if response.Code != http.StatusGatewayTimeout || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeTimeout {
		t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
	}
	assertLedgerLifecycle(t, ledger.events, []string{"created", "routing", "started", "timed_out"})
	if terminal := ledger.events[len(ledger.events)-1]; terminal.Error == nil || terminal.Error.Code != contracts.ErrorCodeTimeout {
		t.Fatalf("terminal event=%#v", terminal)
	}
}

func TestDispatchWithLedgerFailureDoesNotExposeSuccessfulResult(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &transportStub{result: json.RawMessage("{\"kind\":\"message\",\"messageId\":\"agent-message\",\"role\":\"agent\",\"parts\":[{\"kind\":\"data\",\"data\":{\"ok\":true}}]}")}
	ledger := &ledgerRecorder{failSequence: 3, err: errors.New("ledger down")}
	handler := newDispatchLedgerTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096)

	response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
	var platformError contracts.CorrelatedPlatformErrorV4
	if response.Code != http.StatusServiceUnavailable || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeDependency {
		t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "\"status\":\"succeeded\"") || strings.Contains(response.Body.String(), "\"result\"") {
		t.Fatalf("successful result exposed after terminal Ledger failure: %s", response.Body.String())
	}
	assertLedgerLifecycle(t, ledger.events, []string{"created", "routing", "started"})
	if transport.calls != 1 {
		t.Fatalf("transport calls=%d", transport.calls)
	}
}

func TestDispatchWithLedgerPreTransportFailureSkipsAgentCall(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &transportStub{result: json.RawMessage("{\"kind\":\"message\"}")}
	ledger := &ledgerRecorder{failSequence: 1, err: errors.New("ledger down")}
	handler := newDispatchLedgerTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096)

	response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
	var platformError contracts.CorrelatedPlatformErrorV4
	if response.Code != http.StatusServiceUnavailable || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeDependency {
		t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
	}
	assertLedgerLifecycle(t, ledger.events, []string{"created"})
	if transport.calls != 0 {
		t.Fatalf("transport calls=%d, want 0", transport.calls)
	}
}

func TestDispatchWithLedgerRecordsTargetValidationFailureWithoutStartingAgent(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &inputPreflightTransportStub{
		transportStub: transportStub{targetErr: codedTransportError{code: contracts.ErrorCodeAgentAuthUnsupported}},
		preflightErr:  codedTransportError{code: contracts.ErrorCodePayloadTooLarge},
	}
	ledger := &ledgerRecorder{}
	handler := newDispatchLedgerTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096)

	response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
	var platformError contracts.CorrelatedPlatformErrorV4
	if response.Code != http.StatusBadGateway || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeAgentAuthUnsupported {
		t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
	}
	assertLedgerLifecycle(t, ledger.events, []string{"created", "routing", "failed"})
	terminal := ledger.events[len(ledger.events)-1]
	if terminal.Error == nil || terminal.Error.Code != contracts.ErrorCodeAgentAuthUnsupported || terminal.LatencyMS == nil {
		t.Fatalf("terminal event=%#v", terminal)
	}
	if transport.calls != 0 {
		t.Fatalf("transport calls=%d, want 0", transport.calls)
	}
	if transport.preflightCalls != 0 {
		t.Fatalf("input preflight calls=%d, want 0", transport.preflightCalls)
	}
}

func TestDispatchPreservesTypedResolutionFailures(t *testing.T) {
	body := []byte(`{"code":"CAPABILITY_NOT_ALLOWED","message":"The requested capability is not allowed.","traceId":"trace-control","invocationId":"inv-a","rootTaskId":"task-a"}`)
	resolver := &resolverStub{err: &resolution.Failure{StatusCode: http.StatusForbidden, Code: contracts.ErrorCodeCapabilityNotAllowed, TraceID: "trace-control", Body: body}}
	handler := newDispatchTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, 4096)
	response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
	if response.Code != http.StatusForbidden || response.Body.String() != string(body) || response.Header().Get(TraceHeader) != "trace-control" || resolver.calls != 1 {
		t.Fatalf("status=%d body=%q headers=%#v calls=%d", response.Code, response.Body.String(), response.Header(), resolver.calls)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }

func TestDispatchFailsClosedWhenPreCorrelationTraceCannotBeGenerated(t *testing.T) {
	previous := traceSource
	traceSource = errReader{}
	defer func() { traceSource = previous }()
	resolver := &resolverStub{}
	handler := newDispatchTestHandler(t, authStub{err: auth.ErrUnauthenticated}, resolver, 4096)
	response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
	if response.Code != http.StatusInternalServerError || resolver.calls != 0 || strings.Contains(response.Body.String(), "trc_00000000000000000000000000000000_1") {
		t.Fatalf("status=%d calls=%d body=%q", response.Code, resolver.calls, response.Body.String())
	}
}

func TestDispatchMapsResolutionDependencyWithoutRetry(t *testing.T) {
	resolver := &resolverStub{err: errors.New("offline")}
	handler := newDispatchTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, 4096)
	response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
	var platformError contracts.CorrelatedPlatformErrorV4
	if response.Code != http.StatusServiceUnavailable || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeDependency || resolver.calls != 1 {
		t.Fatalf("status=%d error=%#v calls=%d", response.Code, platformError, resolver.calls)
	}
}

func TestDispatchMapsTransportFailureMatrix(t *testing.T) {
	tests := []struct {
		name   string
		code   contracts.PlatformErrorCode
		status int
		cause  error
	}{
		{name: "unsupported auth", code: contracts.ErrorCodeAgentAuthUnsupported, status: http.StatusBadGateway},
		{name: "response too large", code: contracts.ErrorCodeAgentResponseTooLarge, status: http.StatusBadGateway},
		{name: "protocol", code: contracts.ErrorCodeA2AProtocol, status: http.StatusBadGateway},
		{name: "agent unavailable", code: contracts.ErrorCodeAgentUnavailable, status: http.StatusServiceUnavailable},
		{name: "agent execution", code: contracts.ErrorCodeAgentExecutionFailed, status: http.StatusBadGateway},
		{name: "timeout", code: contracts.ErrorCodeTimeout, status: http.StatusGatewayTimeout, cause: context.DeadlineExceeded},
		{name: "canceled", code: contracts.ErrorCodeCanceled, status: http.StatusConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
			transport := &transportStub{err: codedTransportError{code: test.code, cause: test.cause}}
			handler := newDispatchTransportTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, 4096)
			response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
			var platformError contracts.CorrelatedPlatformErrorV4
			if response.Code != test.status || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != test.code {
				t.Fatalf("status=%d code=%q want status=%d code=%q body=%s", response.Code, platformError.Code, test.status, test.code, response.Body.String())
			}
		})
	}
}

func TestDispatchRejectsCardInputOverflowBeforeLedgerAcceptance(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &inputPreflightTransportStub{
		transportStub: transportStub{result: json.RawMessage(`{"kind":"message"}`)},
		preflightErr:  codedTransportError{code: contracts.ErrorCodePayloadTooLarge},
	}
	ledger := &ledgerRecorder{}
	handler := newDispatchLedgerTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096)
	response := invokeDispatch(handler, "application/json", "application/json", validDispatchBody(false))
	var platformError contracts.PreCorrelationPlatformErrorV4
	if response.Code != http.StatusRequestEntityTooLarge || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodePayloadTooLarge {
		t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
	}
	if transport.preflightCalls != 1 || transport.calls != 0 || len(ledger.events) != 0 {
		t.Fatalf("preflight=%d transport=%d ledger=%d", transport.preflightCalls, transport.calls, len(ledger.events))
	}
}

func newDispatchTestHandler(t *testing.T, authenticator Authenticator, resolver Resolver, limit int64) http.Handler {
	t.Helper()
	handler, err := NewDispatchHandler(authenticator, resolver, limit, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

func newDispatchTransportTestHandler(t *testing.T, authenticator Authenticator, resolver Resolver, transport NonStreamingTransport, limit int64) http.Handler {
	t.Helper()
	handler, err := NewDispatchHandlerWithTransport(authenticator, resolver, transport, limit, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

func newDispatchLedgerTestHandler(t *testing.T, authenticator Authenticator, resolver Resolver, transport NonStreamingTransport, ledger InvocationLedgerAppender, limit int64) http.Handler {
	t.Helper()
	handler, err := NewDispatchHandlerWithTransportAndLedger(authenticator, resolver, transport, ledger, limit, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

func assertLedgerLifecycle(t *testing.T, events []contracts.InvocationEventV03, types []string) {
	t.Helper()
	if len(events) != len(types) {
		t.Fatalf("events len=%d want=%d events=%#v", len(events), len(types), events)
	}
	validator, err := contracts.NewRuntimeContractValidator()
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := contracts.NewRuntimeInvocationSequenceValidator(validator)
	if err != nil {
		t.Fatal(err)
	}
	for index, event := range events {
		if event.Type != types[index] {
			t.Fatalf("event %d type=%q want=%q events=%#v", index, event.Type, types[index], events)
		}
		if event.ChunkIndex != nil || event.ChunkBytes != nil {
			t.Fatalf("event %d stores content metadata fields: %#v", index, event)
		}
		if err := sequence.Accept(event); err != nil {
			t.Fatalf("event %d invalid: %v event=%#v", index, err, event)
		}
	}
}

func invokeDispatch(handler http.Handler, contentType, accept, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/internal/v3/invocations", strings.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept", accept)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func validDispatchBody(stream bool) string {
	if stream {
		return `{"invocationId":"inv-a","rootTaskId":"task-a","traceId":"trace-a","caller":{"type":"user","id":"owner-a"},"workspaceId":"workspace-a","targetAgentId":"agent-a","agentCardVersion":"1.0.0","capability":"capability-a","input":{"q":"x"},"stream":true}`
	}
	return `{"invocationId":"inv-a","rootTaskId":"task-a","traceId":"trace-a","caller":{"type":"user","id":"owner-a"},"workspaceId":"workspace-a","targetAgentId":"agent-a","agentCardVersion":"1.0.0","capability":"capability-a","input":{"q":"x"},"stream":false}`
}

func dispatchResolvedCard(endpoint string) contracts.AgentCard {
	return contracts.AgentCard{
		AgentID: "agent-a", Version: "1.0.0",
		Protocol:       contracts.AgentProtocol{Type: "a2a", Version: contracts.A2AProtocolVersion, Transport: "JSONRPC", Endpoint: endpoint},
		Authentication: contracts.AgentAuthentication{Type: "none"},
		Skills:         []contracts.AgentSkill{{ID: "capability-a"}},
	}
}
