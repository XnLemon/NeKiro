package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	runtimeb "github.com/Nene7ko/NeKiro/agents/runtime-b"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/auth"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/credential"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/resolution"
	streammodel "github.com/Nene7ko/NeKiro/apps/a2a-router/internal/stream"
	a2atransport "github.com/Nene7ko/NeKiro/apps/a2a-router/internal/transport/a2a"
	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/a2aproject/a2a-go/a2asrv"
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
	delay    time.Duration
}

func (stub *resolverStub) Resolve(ctx context.Context, request contracts.ResolveAgentRequest) (contracts.ResolveAgentResponse, error) {
	stub.calls++
	stub.request = request
	if stub.delay > 0 {
		timer := time.NewTimer(stub.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return contracts.ResolveAgentResponse{}, ctx.Err()
		}
	}
	return stub.response, stub.err
}

type transportStub struct {
	dispatch  contracts.DispatchInvocationRequestV4
	resolved  contracts.ResolveAgentResponse
	result    json.RawMessage
	calls     int
	err       error
	targetErr error
}

func TestValidateDispatchRejectsRootParentLineage(t *testing.T) {
	request := contracts.DispatchInvocationRequestV4{
		InvocationID: "inv-root", RootTaskID: "task-root", ParentInvocationID: "inv-parent",
		TraceID: "trc_root_1", Caller: contracts.Caller{Type: "user", ID: "user-1"},
		WorkspaceID: "workspace-1", TargetAgentID: "agent-1", AgentCardVersion: "1.0.0",
		Capability: "capability-1", Input: json.RawMessage(`{}`), Stream: false,
	}
	if err := validateDispatch(request); err == nil {
		t.Fatal("root dispatch accepted parentInvocationId")
	}
}

func TestDispatchV3RouteIsRetired(t *testing.T) {
	resolver := &resolverStub{}
	handler := newDispatchTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, 1024)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/internal/v3/invocations", strings.NewReader(validDispatchBody(false))))
	if response.Code != http.StatusNotFound || resolver.calls != 0 {
		t.Fatalf("status=%d resolver calls=%d", response.Code, resolver.calls)
	}
}

type streamingTransportStub struct {
	transportStub
	events []streammodel.Event
	err    error
}

func (stub *streamingTransportStub) SendStreaming(_ context.Context, _ contracts.DispatchInvocationRequestV4, _ contracts.ResolveAgentResponse) iter.Seq2[streammodel.Event, error] {
	return func(yield func(streammodel.Event, error) bool) {
		for _, event := range stub.events {
			if !yield(event, nil) {
				return
			}
		}
		if stub.err != nil {
			yield(streammodel.Event{}, stub.err)
		}
	}
}

func (stub *streamingTransportStub) ValidateStreamingTarget(_ contracts.DispatchInvocationRequestV4, _ contracts.ResolveAgentResponse) error {
	return stub.targetErr
}

func (stub *streamingTransportStub) ValidateStreamingInput(_ contracts.DispatchInvocationRequestV4, _ contracts.ResolveAgentResponse) error {
	return nil
}

type inputPreflightTransportStub struct {
	transportStub
	preflightErr   error
	preflightCalls int
}

type deadlineTransportStub struct {
	transportStub
	deadlineAt time.Time
}

func (stub *deadlineTransportStub) SendNonStreaming(ctx context.Context, dispatch contracts.DispatchInvocationRequestV4, resolved contracts.ResolveAgentResponse) (json.RawMessage, error) {
	stub.calls++
	stub.dispatch = dispatch
	stub.resolved = resolved
	stub.deadlineAt, _ = ctx.Deadline()
	<-ctx.Done()
	return nil, ctx.Err()
}

func (stub *inputPreflightTransportStub) ValidateNonStreamingInput(contracts.DispatchInvocationRequestV4, contracts.ResolveAgentResponse) error {
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

func (stub *transportStub) SendNonStreaming(_ context.Context, dispatch contracts.DispatchInvocationRequestV4, resolved contracts.ResolveAgentResponse) (json.RawMessage, error) {
	stub.calls++
	stub.dispatch = dispatch
	stub.resolved = resolved
	return stub.result, stub.err
}

func (stub *transportStub) ValidateNonStreamingTarget(contracts.DispatchInvocationRequestV4, contracts.ResolveAgentResponse) error {
	return stub.targetErr
}

func (stub *transportStub) ValidateNonStreamingInput(contracts.DispatchInvocationRequestV4, contracts.ResolveAgentResponse) error {
	return nil
}

type ledgerRecorder struct {
	events       []contracts.InvocationEventV03
	failSequence int64
	err          error
}

type cancelingLedgerRecorder struct {
	events []contracts.InvocationEventV03
	cancel context.CancelFunc
	delay  time.Duration
}

type failingStreamWriter struct {
	header http.Header
}

func (writer *failingStreamWriter) Header() http.Header { return writer.header }
func (writer *failingStreamWriter) WriteHeader(int)     {}
func (writer *failingStreamWriter) Write([]byte) (int, error) {
	return 0, errors.New("caller disconnected")
}
func (writer *failingStreamWriter) Flush() {}

func (recorder *ledgerRecorder) Append(_ context.Context, event contracts.InvocationEventV03) error {
	if recorder.err != nil && event.Sequence == recorder.failSequence {
		return recorder.err
	}
	recorder.events = append(recorder.events, event)
	return nil
}

func (recorder *cancelingLedgerRecorder) Append(ctx context.Context, event contracts.InvocationEventV03) error {
	if event.Type == "created" {
		recorder.events = append(recorder.events, event)
		if recorder.delay > 0 {
			time.Sleep(recorder.delay)
		} else {
			recorder.cancel()
		}
		return nil
	}
	if event.Type == "routing" {
		return ctx.Err()
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

func TestDispatchRequiresResolvedReleaseProvenanceAndPersistsExactPair(t *testing.T) {
	digest := strings.Repeat("a", 64)
	resolved := contracts.ResolveAgentResponse{
		Card: dispatchResolvedCard("https://agent.example/a2a"),
		Installation: contracts.ResolvedInstallation{
			InstalledReleaseID: "release-a", AgentCardDigest: digest,
		},
	}
	transport := &transportStub{result: json.RawMessage("{\"kind\":\"message\",\"messageId\":\"agent-message\",\"role\":\"agent\",\"parts\":[{\"kind\":\"data\",\"data\":{\"ok\":true}}]}")}
	ledger := &ledgerRecorder{}
	handler := newDispatchLedgerTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, &resolverStub{response: resolved}, transport, ledger, 4096)
	response := invokeDispatch(handler, "application/json", "application/json", trustedDispatchBody(false, "release-a", digest))
	if response.Code != http.StatusOK || transport.calls != 1 {
		t.Fatalf("status=%d transport=%d body=%s", response.Code, transport.calls, response.Body.String())
	}
	assertLedgerLifecycle(t, ledger.events, []string{"created", "routing", "started", "succeeded"})
	for index, event := range ledger.events {
		if event.AgentReleaseID != "release-a" || event.AgentCardDigest != digest {
			t.Fatalf("event %d provenance = %q/%q", index, event.AgentReleaseID, event.AgentCardDigest)
		}
	}

	for _, test := range []struct {
		name     string
		body     string
		resolved contracts.ResolveAgentResponse
	}{
		{name: "request omits trusted pair", body: validDispatchBody(false), resolved: resolved},
		{name: "release differs", body: trustedDispatchBody(false, "release-other", digest), resolved: resolved},
		{name: "digest differs", body: trustedDispatchBody(false, "release-a", strings.Repeat("b", 64)), resolved: resolved},
		{name: "resolution omits trusted pair", body: trustedDispatchBody(false, "release-a", digest), resolved: contracts.ResolveAgentResponse{Card: resolved.Card}},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &transportStub{result: json.RawMessage(`{"kind":"message"}`)}
			ledger := &ledgerRecorder{}
			handler := newDispatchLedgerTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, &resolverStub{response: test.resolved}, transport, ledger, 4096)
			response := invokeDispatch(handler, "application/json", "application/json", test.body)
			var platformError contracts.CorrelatedPlatformErrorV4
			if response.Code != http.StatusServiceUnavailable || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeDependency {
				t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
			}
			if transport.calls != 0 || len(ledger.events) != 0 {
				t.Fatalf("transport=%d ledger=%d", transport.calls, len(ledger.events))
			}
		})
	}
}

func TestDispatchChildRejectsResolvedReleaseProvenanceMismatchBeforeLedger(t *testing.T) {
	digest := strings.Repeat("a", 64)
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{
		Card: dispatchResolvedCard("https://agent.example/a2a"),
		Installation: contracts.ResolvedInstallation{
			InstalledReleaseID: "release-resolved", AgentCardDigest: digest,
		},
	}}
	transport := &transportStub{result: json.RawMessage(`{"kind":"message"}`)}
	ledger := &ledgerRecorder{}
	handler, err := NewDispatchHandlerWithTransportAndLedger(authStub{caller: auth.Caller{ID: "agent-a"}}, resolver, transport, ledger, 4096, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	dispatch := contracts.DispatchInvocationRequestV4{
		InvocationID: "inv-child", RootTaskID: "task-root", ParentInvocationID: "inv-parent", TraceID: "trace-child",
		Caller: contracts.Caller{Type: "agent", ID: "agent-parent"}, WorkspaceID: "workspace-a",
		TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", AgentReleaseID: "release-request",
		AgentCardDigest: digest, Capability: "capability-a", Input: json.RawMessage(`{}`), Stream: false,
	}
	response := httptest.NewRecorder()
	handler.DispatchChild(response, httptest.NewRequest(http.MethodPost, "/agent/v1/invocations", nil), dispatch, "application/json")
	var platformError contracts.PreCorrelationPlatformErrorV4
	if response.Code != http.StatusServiceUnavailable || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeDependency {
		t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
	}
	if transport.calls != 0 || len(ledger.events) != 0 {
		t.Fatalf("transport=%d ledger=%d", transport.calls, len(ledger.events))
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
	before := time.Now().UTC().Add(-time.Millisecond)
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
	occurredAt, err := time.Parse(time.RFC3339Nano, terminal.OccurredAt)
	if err != nil || occurredAt.Before(before) || occurredAt.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("target failure occurred_at=%s is not current: %v", terminal.OccurredAt, err)
	}
	if transport.calls != 0 {
		t.Fatalf("transport calls=%d, want 0", transport.calls)
	}
	if transport.preflightCalls != 0 {
		t.Fatalf("input preflight calls=%d, want 0", transport.preflightCalls)
	}
}

func TestDispatchWithLedgerCancellationAfterAcceptanceCommitsTerminal(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &transportStub{result: json.RawMessage(`{"kind":"message"}`)}
	ledger := &cancelingLedgerRecorder{cancel: cancel}
	handler := newDispatchLedgerTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096)
	request := httptest.NewRequest(http.MethodPost, "/internal/v4/invocations", strings.NewReader(validDispatchBody(false))).WithContext(requestContext)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var platformError contracts.CorrelatedPlatformErrorV4
	if response.Code != http.StatusConflict || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeCanceled {
		t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
	}
	assertLedgerLifecycle(t, ledger.events, []string{"created", "canceled"})
	if transport.calls != 0 {
		t.Fatalf("transport calls=%d, want 0", transport.calls)
	}
}

func TestDispatchWithLedgerTimeoutAfterAcceptanceCommitsTerminal(t *testing.T) {
	requestContext, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &transportStub{result: json.RawMessage(`{"kind":"message"}`)}
	ledger := &cancelingLedgerRecorder{delay: 25 * time.Millisecond}
	handler := newDispatchLedgerTestHandler(t, authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096)
	request := httptest.NewRequest(http.MethodPost, "/internal/v4/invocations", strings.NewReader(validDispatchBody(false))).WithContext(requestContext)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var platformError contracts.CorrelatedPlatformErrorV4
	if response.Code != http.StatusGatewayTimeout || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeTimeout {
		t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
	}
	assertLedgerLifecycle(t, ledger.events, []string{"created", "timed_out"})
	if transport.calls != 0 {
		t.Fatalf("transport calls=%d, want 0", transport.calls)
	}
}

func TestDispatchStreamingEmitsStrictCorrelatedFramesAndMetadataLedger(t *testing.T) {
	before := time.Now().UTC().Add(-time.Millisecond)
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &streamingTransportStub{events: []streammodel.Event{
		{Kind: "task", Payload: json.RawMessage(`{"kind":"task","id":"task-a","contextId":"ctx-a","status":{"state":"working"}}`)},
		{Kind: "message", Payload: json.RawMessage(`{"kind":"message","messageId":"message-a","taskId":"task-a","contextId":"ctx-a","role":"agent","parts":[{"kind":"data","data":{"value":"ok"}}]}`)},
		{Kind: "status-update", Payload: json.RawMessage(`{"kind":"status-update","taskId":"task-a","contextId":"ctx-a","status":{"state":"completed"},"final":true}`), TerminalType: contracts.ResultStreamEventCompleted, TerminalStatus: "succeeded"},
	}}
	ledger := &ledgerRecorder{}
	handler, err := NewDispatchHandlerWithTransportAndLedgerAndStreaming(authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096, 4096, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	response := invokeDispatch(mux, "application/json", "text/event-stream", validDispatchBody(true))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status=%d content-type=%q body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	frames := bytes.Split(bytes.TrimSuffix(response.Body.Bytes(), []byte("\n\n")), []byte("\n\n"))
	if len(frames) != 5 {
		t.Fatalf("frames=%d body=%s", len(frames), response.Body.String())
	}
	validator, err := contracts.NewRuntimeContractValidator()
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := contracts.NewRuntimeResultStreamSequenceValidator(validator, "inv-a", "task-a", "trace-a")
	if err != nil {
		t.Fatal(err)
	}
	for index, frame := range frames {
		lines := bytes.Split(frame, []byte("\n"))
		if len(lines) != 1 || !bytes.HasPrefix(lines[0], []byte("data: ")) {
			t.Fatalf("frame %d=%q", index, frame)
		}
		var event contracts.InvocationResultStreamEventV2
		if err := json.Unmarshal(bytes.TrimPrefix(lines[0], []byte("data: ")), &event); err != nil {
			t.Fatalf("frame %d decode: %v", index, err)
		}
		if err := sequence.Accept(event); err != nil {
			t.Fatalf("frame %d validation: %v", index, err)
		}
	}
	if err := sequence.Finish(); err != nil {
		t.Fatal(err)
	}
	if len(ledger.events) != 7 {
		t.Fatalf("ledger events=%d want 7", len(ledger.events))
	}
	assertLedgerLifecycle(t, ledger.events[:3], []string{"created", "routing", "started"})
	for index, event := range ledger.events[3:6] {
		if event.Type != "stream" || event.ChunkIndex == nil || event.ChunkBytes == nil || *event.ChunkIndex != int64(index) || *event.ChunkBytes <= 0 {
			t.Fatalf("stream ledger event %d=%#v", index, event)
		}
		if event.Error != nil {
			t.Fatalf("stream ledger event contains content/error: %#v", event)
		}
	}
	if ledger.events[6].Type != "succeeded" || ledger.events[6].Error != nil {
		t.Fatalf("terminal ledger event=%#v", ledger.events[6])
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, ledger.events[6].OccurredAt)
	if err != nil || occurredAt.Before(before) || occurredAt.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("stream terminal occurred_at=%s is not current: %v", ledger.events[6].OccurredAt, err)
	}
}

func TestDispatchStreamingUsesStreamingTargetValidation(t *testing.T) {
	card := dispatchResolvedCard("https://agent.example/a2a")
	card.Limits.Streaming = false
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: card}}
	transport := &streamingTransportStub{transportStub: transportStub{targetErr: codedTransportError{code: contracts.ErrorCodeRouteNotFound}}, events: []streammodel.Event{{Kind: "message", Payload: json.RawMessage(`{"kind":"message"}`)}}}
	ledger := &ledgerRecorder{}
	handler, err := NewDispatchHandlerWithTransportAndLedgerAndStreaming(authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096, 4096, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	response := invokeDispatch(mux, "application/json", "text/event-stream", validDispatchBody(true))
	var platformError contracts.CorrelatedPlatformErrorV4
	if response.Code != http.StatusServiceUnavailable || json.Unmarshal(response.Body.Bytes(), &platformError) != nil || platformError.Code != contracts.ErrorCodeRouteNotFound {
		t.Fatalf("status=%d error=%#v body=%s", response.Code, platformError, response.Body.String())
	}
	if transport.calls != 0 {
		t.Fatalf("non-streaming transport calls=%d", transport.calls)
	}
	assertLedgerLifecycle(t, ledger.events, []string{"created", "routing", "failed"})
}

func TestDispatchStreamingRuntimeBEndToEnd(t *testing.T) {
	server := httptest.NewServer(a2asrv.NewJSONRPCHandler(runtimeb.NewHandler()))
	t.Cleanup(server.Close)
	issuer, err := credential.NewIssuer(credential.Config{Issuer: "https://a2a-router.nekiro.test", KeyID: "router-key-1", PrivateKey: ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)), TTL: 30 * time.Second}, time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := a2atransport.NewClient(server.Client(), issuer, 4096, 4096, 4096, 4096)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: contracts.AgentCard{
		AgentID: "agent-a", Version: "1.0.0",
		Protocol:       contracts.AgentProtocol{Type: "a2a", Version: contracts.A2AProtocolVersion, Transport: "JSONRPC", Endpoint: server.URL},
		Authentication: contracts.AgentAuthentication{Type: "http_bearer"},
		Skills:         []contracts.AgentSkill{{ID: "capability-a"}},
		Limits:         contracts.AgentLimits{TimeoutMS: 1000, MaxInputBytes: "4096", MaxOutputBytes: "4096", Streaming: true},
	}, Installation: contracts.ResolvedInstallation{InstallationID: "installation-a", WorkspaceID: "workspace-a", AgentID: "agent-a", InstalledVersion: "1.0.0", InstalledReleaseID: "release-a", AgentCardDigest: strings.Repeat("a", 64), Status: "enabled"}}}
	ledger := &ledgerRecorder{}
	handler, err := NewDispatchHandlerWithTransportAndLedgerAndStreaming(authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096, 4096, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := `{"invocationId":"inv-a","rootTaskId":"task-a","traceId":"trace-a","caller":{"type":"user","id":"owner-a"},"workspaceId":"workspace-a","targetAgentId":"agent-a","agentCardVersion":"1.0.0","agentReleaseId":"release-a","agentCardDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","capability":"capability-a","input":{"fixture":"stream-success","value":"e2e"},"stream":true}`
	response := invokeDispatch(mux, "application/json", "text/event-stream", body)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(response.Body.String(), `"type":"completed"`) {
		t.Fatalf("status=%d headers=%#v body=%s", response.Code, response.Header(), response.Body.String())
	}
	if len(ledger.events) != 9 || ledger.events[len(ledger.events)-1].Type != "succeeded" {
		t.Fatalf("ledger events=%#v", ledger.events)
	}
}

func TestDispatchStreamingInterruptedEOFIsFailedAndNeverSucceeded(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &streamingTransportStub{events: []streammodel.Event{{Kind: "message", Payload: json.RawMessage(`{"kind":"message","messageId":"message-a","taskId":"task-a","contextId":"ctx-a","role":"agent","parts":[{"kind":"data","data":{"value":"partial"}}]}`)}}}
	ledger := &ledgerRecorder{}
	handler, err := NewDispatchHandlerWithTransportAndLedgerAndStreaming(authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096, 4096, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	response := invokeDispatch(mux, "application/json", "text/event-stream", validDispatchBody(true))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"type":"completed"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"type":"failed"`) || !strings.Contains(response.Body.String(), `"code":"A2A_PROTOCOL_ERROR"`) {
		t.Fatalf("interrupted stream body=%s", response.Body.String())
	}
	if len(ledger.events) != 5 || ledger.events[len(ledger.events)-1].Type != "failed" {
		t.Fatalf("ledger=%#v", ledger.events)
	}
}

func TestDispatchStreamingSSEOverflowEmitsBoundedFailure(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	largePayload := json.RawMessage(`{"kind":"message","messageId":"message-a","taskId":"task-a","contextId":"ctx-a","role":"agent","parts":[{"kind":"data","data":{"value":"` + strings.Repeat("x", 700) + `"}}]}`)
	transport := &streamingTransportStub{events: []streammodel.Event{{Kind: "message", Payload: largePayload}}}
	ledger := &ledgerRecorder{}
	handler, err := NewDispatchHandlerWithTransportAndLedgerAndStreaming(authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 320, 4096, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	response := invokeDispatch(mux, "application/json", "text/event-stream", validDispatchBody(true))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"type":"failed"`) || !strings.Contains(response.Body.String(), `"code":"AGENT_RESPONSE_TOO_LARGE"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(ledger.events) != 5 || ledger.events[len(ledger.events)-1].Type != "failed" || ledger.events[len(ledger.events)-1].Error == nil || ledger.events[len(ledger.events)-1].Error.Code != contracts.ErrorCodeAgentResponseTooLarge {
		t.Fatalf("ledger=%#v", ledger.events)
	}
}

func TestDispatchStreamingClassifiesTimeoutAndCancellation(t *testing.T) {
	for _, test := range []struct {
		name      string
		err       error
		code      contracts.PlatformErrorCode
		typeValue string
	}{
		{name: "timeout", err: context.DeadlineExceeded, code: contracts.ErrorCodeTimeout, typeValue: "timed_out"},
		{name: "canceled", err: context.Canceled, code: contracts.ErrorCodeCanceled, typeValue: "canceled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
			transport := &streamingTransportStub{err: test.err}
			ledger := &ledgerRecorder{}
			handler, err := NewDispatchHandlerWithTransportAndLedgerAndStreaming(authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096, 4096, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)
			response := invokeDispatch(mux, "application/json", "text/event-stream", validDispatchBody(true))
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":"`+string(test.code)+`"`) || !strings.Contains(response.Body.String(), `"type":"`+test.typeValue+`"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if ledger.events[len(ledger.events)-1].Type != test.typeValue || ledger.events[len(ledger.events)-1].Error == nil || ledger.events[len(ledger.events)-1].Error.Code != test.code {
				t.Fatalf("ledger terminal=%#v", ledger.events[len(ledger.events)-1])
			}
		})
	}
}

func TestDispatchNonStreamingUsesResolvedCardDeadline(t *testing.T) {
	card := dispatchResolvedCard("https://agent.example/a2a")
	card.Limits.TimeoutMS = 5
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: card}}
	transport := &deadlineTransportStub{}
	ledger := &ledgerRecorder{}
	handler, err := NewDispatchHandlerWithTransportAndLedger(
		authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport,
		ledger, 4096, time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	started := time.Now()
	response := invokeDispatch(mux, "application/json", "application/json", validDispatchBody(false))
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
		t.Fatalf("Card deadline was not applied: elapsed=%s", elapsed)
	}
	if transport.calls != 1 || len(ledger.events) != 4 || ledger.events[3].Status != "timed_out" {
		t.Fatalf("transport calls=%d ledger=%#v", transport.calls, ledger.events)
	}
}

func TestDispatchNonStreamingCardDeadlineIncludesResolutionTime(t *testing.T) {
	card := dispatchResolvedCard("https://agent.example/a2a")
	card.Limits.TimeoutMS = 100
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: card}, delay: 80 * time.Millisecond}
	transport := &deadlineTransportStub{}
	ledger := &ledgerRecorder{}
	handler, err := NewDispatchHandlerWithTransportAndLedger(
		authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport,
		ledger, 4096, 500*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	started := time.Now()
	response := invokeDispatch(mux, "application/json", "application/json", validDispatchBody(false))
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if transport.deadlineAt.IsZero() || transport.deadlineAt.After(started.Add(160*time.Millisecond)) {
		t.Fatalf("Card deadline started after resolution: started=%s deadline=%s", started, transport.deadlineAt)
	}
	if len(ledger.events) != 4 || ledger.events[3].Status != "timed_out" {
		t.Fatalf("ledger=%#v", ledger.events)
	}
}

func TestResolvedDeadlineContextUsesConfiguredAndCardMinimum(t *testing.T) {
	startedAt := time.Now()
	for _, test := range []struct {
		name       string
		configured time.Duration
		card       int64
		want       time.Duration
	}{
		{name: "Card is shorter", configured: 200 * time.Millisecond, card: 5, want: 5 * time.Millisecond},
		{name: "configured is shorter", configured: 5 * time.Millisecond, card: 200, want: 5 * time.Millisecond},
		{name: "Card limit absent", configured: 5 * time.Millisecond, card: 0, want: 5 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent, cancelParent := context.WithTimeout(context.Background(), test.configured)
			defer cancelParent()
			started := time.Now()
			ctx, cancel := resolvedDeadlineContext(parent, test.card, startedAt)
			defer cancel()
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("resolved context has no deadline")
			}
			got := time.Until(deadline)
			if got < test.want-20*time.Millisecond || got > test.want+20*time.Millisecond {
				t.Fatalf("deadline=%s, want approximately %s (started %s)", got, test.want, time.Since(started))
			}
		})
	}
}

func TestLedgerContextProvidesBoundedGraceAfterCancellation(t *testing.T) {
	var key ledgerContextTestKey
	parent := context.WithValue(context.Background(), key, "correlation")
	canceled, cancelParent := context.WithCancel(parent)
	cancelParent()
	grace, release := ledgerContext(canceled)
	defer release()
	if grace.Err() != nil {
		t.Fatalf("grace context already failed: %v", grace.Err())
	}
	if got := grace.Value(key); got != "correlation" {
		t.Fatalf("grace context lost value: %v", got)
	}
	deadline, ok := grace.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > ledgerCommitGrace {
		t.Fatalf("grace deadline=%s ok=%v, want within %s", deadline, ok, ledgerCommitGrace)
	}
}

type ledgerContextTestKey struct{}

func TestDispatchStreamingLedgerFailureAfterAgentChunkDoesNotFabricateTerminalFact(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &streamingTransportStub{events: []streammodel.Event{{Kind: "message", Payload: json.RawMessage(`{"kind":"message","messageId":"message-a","taskId":"task-a","contextId":"ctx-a","role":"agent","parts":[{"kind":"data","data":{"value":"ok"}}]}`), TerminalType: contracts.ResultStreamEventCompleted, TerminalStatus: "succeeded"}}}
	ledger := &ledgerRecorder{failSequence: 4, err: errors.New("ledger unavailable")}
	handler, err := NewDispatchHandlerWithTransportAndLedgerAndStreaming(authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096, 4096, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	response := invokeDispatch(mux, "application/json", "text/event-stream", validDispatchBody(true))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":"DEPENDENCY_ERROR"`) || strings.Contains(response.Body.String(), `"type":"completed"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(ledger.events) != 4 || ledger.events[len(ledger.events)-1].Type != "stream" {
		t.Fatalf("ledger=%#v", ledger.events)
	}
}

func TestDispatchStreamingChunkLedgerFailureEmitsDeliveryFailure(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &streamingTransportStub{events: []streammodel.Event{{Kind: "message", Payload: json.RawMessage(`{"kind":"message","messageId":"message-a","taskId":"task-a","contextId":"ctx-a","role":"agent","parts":[{"kind":"data","data":{"value":"ok"}}]}`)}}}
	ledger := &ledgerRecorder{failSequence: 3, err: errors.New("ledger unavailable")}
	handler, err := NewDispatchHandlerWithTransportAndLedgerAndStreaming(authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096, 4096, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	response := invokeDispatch(mux, "application/json", "text/event-stream", validDispatchBody(true))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":"DEPENDENCY_ERROR"`) || strings.Contains(response.Body.String(), `"type":"completed"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(ledger.events) != 3 || ledger.events[len(ledger.events)-1].Type != "started" {
		t.Fatalf("ledger=%#v", ledger.events)
	}
}

func TestDispatchStreamingWriterFailureCommitsNonSuccessLedgerTerminal(t *testing.T) {
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{Card: dispatchResolvedCard("https://agent.example/a2a")}}
	transport := &streamingTransportStub{events: []streammodel.Event{{Kind: "message", Payload: json.RawMessage(`{"kind":"message","messageId":"message-a","taskId":"task-a","contextId":"ctx-a","role":"agent","parts":[{"kind":"data","data":{"value":"ok"}}]}`)}}}
	ledger := &ledgerRecorder{}
	handler, err := NewDispatchHandlerWithTransportAndLedgerAndStreaming(authStub{caller: auth.Caller{ID: "control-plane"}}, resolver, transport, ledger, 4096, 4096, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	request := httptest.NewRequest(http.MethodPost, "/internal/v4/invocations", strings.NewReader(validDispatchBody(true)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	writer := &failingStreamWriter{header: make(http.Header)}
	mux.ServeHTTP(writer, request)
	if len(ledger.events) < 4 || ledger.events[len(ledger.events)-1].Type != "failed" || ledger.events[len(ledger.events)-1].Error == nil {
		t.Fatalf("ledger=%#v", ledger.events)
	}
	if ledger.events[len(ledger.events)-1].Error.Code != contracts.ErrorCodeDependency {
		t.Fatalf("writer failure error=%q", ledger.events[len(ledger.events)-1].Error.Code)
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
		{name: "internal", code: contracts.ErrorCodeInternal, status: http.StatusInternalServerError},
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

func TestStreamTerminalTypeMapsAllTerminalOutcomes(t *testing.T) {
	for _, test := range []struct {
		name   string
		input  contracts.ResultStreamEventType
		status string
		want   contracts.ResultStreamEventType
		state  string
	}{
		{name: "completed", input: contracts.ResultStreamEventCompleted, status: "ignored", want: contracts.ResultStreamEventCompleted, state: "succeeded"},
		{name: "canceled", input: contracts.ResultStreamEventCanceled, status: "ignored", want: contracts.ResultStreamEventCanceled, state: "canceled"},
		{name: "timed out", input: contracts.ResultStreamEventTimedOut, status: "ignored", want: contracts.ResultStreamEventTimedOut, state: "timed_out"},
		{name: "failed", input: contracts.ResultStreamEventFailed, status: "failed", want: contracts.ResultStreamEventFailed, state: "failed"},
		{name: "failed with canceled status", input: contracts.ResultStreamEventFailed, status: "canceled", want: contracts.ResultStreamEventCanceled, state: "canceled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, state := streamTerminalType(test.input, test.status)
			if got != test.want || state != test.state {
				t.Fatalf("streamTerminalType(%q, %q) = %q, %q; want %q, %q", test.input, test.status, got, state, test.want, test.state)
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
	request := httptest.NewRequest(http.MethodPost, "/internal/v4/invocations", strings.NewReader(body))
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

func trustedDispatchBody(stream bool, releaseID, digest string) string {
	return strings.Replace(validDispatchBody(stream), `"agentCardVersion":"1.0.0"`, `"agentCardVersion":"1.0.0","agentReleaseId":"`+releaseID+`","agentCardDigest":"`+digest+`"`, 1)
}

func dispatchResolvedCard(endpoint string) contracts.AgentCard {
	return contracts.AgentCard{
		AgentID: "agent-a", Version: "1.0.0",
		Protocol:       contracts.AgentProtocol{Type: "a2a", Version: contracts.A2AProtocolVersion, Transport: "JSONRPC", Endpoint: endpoint},
		Authentication: contracts.AgentAuthentication{Type: "none"},
		Skills:         []contracts.AgentSkill{{ID: "capability-a"}},
		Limits:         contracts.AgentLimits{TimeoutMS: 1000, MaxInputBytes: "4096", MaxOutputBytes: "4096", Streaming: true},
	}
}
