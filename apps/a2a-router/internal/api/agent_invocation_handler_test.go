package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/auth"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/nested"
	"github.com/Nene7ko/NeKiro/contracts"
)

func agentTokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type mockNestedLedgerReader struct {
	invocation contracts.InvocationDetailResponseV4
	err        error
}

func (m *mockNestedLedgerReader) GetInvocationByParentID(_ context.Context, _ string) (contracts.InvocationDetailResponseV4, error) {
	return m.invocation, m.err
}

type mockVersionResolver struct {
	response contracts.ResolveInstalledVersionResponse
	err      error
}

func (m *mockVersionResolver) ResolveInstalledVersion(_ context.Context, _ contracts.ResolveInstalledVersionRequest) (contracts.ResolveInstalledVersionResponse, error) {
	return m.response, m.err
}

type mockResolver struct{}

func (m *mockResolver) Resolve(_ context.Context, _ contracts.ResolveAgentRequest) (contracts.ResolveAgentResponse, error) {
	return contracts.ResolveAgentResponse{}, errors.New("mock resolver not implemented")
}

func newTestAgentHandler(t *testing.T, ledgerReader NestedLedgerReader, versionResolver VersionResolver) (*AgentInvocationHandler, string) {
	t.Helper()
	token := "agent-test-token"
	binding, err := nested.NewAgentBinding([]nested.AgentPrincipal{
		{AgentID: "agent_caller01", TokenSHA256: agentTokenDigest(token)},
	})
	if err != nil {
		t.Fatalf("NewAgentBinding() error = %v", err)
	}

	serviceAuth, err := auth.NewStaticAuthenticator([]auth.Principal{
		{ID: "router-service", TokenSHA256: agentTokenDigest("service-token")},
	})
	if err != nil {
		t.Fatalf("NewStaticAuthenticator() error = %v", err)
	}

	resolver := &mockResolver{}
	dispatchHandler, err := NewDispatchHandler(serviceAuth, resolver, 1048576, 30000*1000000)
	if err != nil {
		t.Fatalf("NewDispatchHandler() error = %v", err)
	}

	handler, err := NewAgentInvocationHandler(binding, ledgerReader, versionResolver, dispatchHandler, 1048576, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgentInvocationHandler() error = %v", err)
	}
	return handler, token
}

func runningParentDetail() contracts.InvocationDetailResponseV4 {
	return contracts.InvocationDetailResponseV4{
		Invocation: contracts.InvocationRecordV4{
			InvocationID:     "inv_parent123",
			RootTaskID:       "task_root456",
			TraceID:          "trc_abc123_1",
			WorkspaceID:      "ws_test789",
			TargetAgentID:    "agent_caller01",
			AgentCardVersion: "1.0.0",
			Capability:       "process",
			Status:           "running",
			Caller:           contracts.Caller{Type: "user", ID: "user01"},
		},
	}
}

func validNestedBody() string {
	return `{"parentInvocationId":"inv_parent123","targetAgentId":"agent_target02","capability":"summarize","input":{"text":"hello"},"stream":false}`
}

func TestAgentHandlerAuthFirst(t *testing.T) {
	handler, _ := newTestAgentHandler(t, &mockNestedLedgerReader{}, &mockVersionResolver{})

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"missing auth", "", http.StatusUnauthorized},
		{"empty bearer", "Bearer ", http.StatusUnauthorized},
		{"unknown token", "Bearer wrong-token", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(validNestedBody()))
			req.Header.Set("Content-Type", "application/json")
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			handler.serve(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestAgentHandlerContentTypeValidation(t *testing.T) {
	handler, token := newTestAgentHandler(t, &mockNestedLedgerReader{}, &mockVersionResolver{})

	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(validNestedBody()))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAgentHandlerForbiddenTrustedFields(t *testing.T) {
	handler, token := newTestAgentHandler(t, &mockNestedLedgerReader{}, &mockVersionResolver{})

	forbiddenFields := []string{
		"invocationId", "rootTaskId", "traceId", "workspaceId",
		"caller", "callerType", "callerId",
		"agentCardVersion", "version",
		"endpoint", "url", "credential", "token", "authorization",
		"childInvocationId", "childId",
	}
	for _, field := range forbiddenFields {
		t.Run("forbidden_"+field, func(t *testing.T) {
			body := `{"parentInvocationId":"inv_parent123","targetAgentId":"agent_target02","capability":"summarize","input":{},"stream":false,"` + field + `":"injected"}`
			req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.serve(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d for field %s", rec.Code, http.StatusBadRequest, field)
			}
		})
	}
}

func TestAgentHandlerInvalidIdentifiers(t *testing.T) {
	handler, token := newTestAgentHandler(t, &mockNestedLedgerReader{}, &mockVersionResolver{})

	tests := []struct {
		name string
		body string
	}{
		{"invalid parentInvocationId", `{"parentInvocationId":"inv parent","targetAgentId":"agent_target02","capability":"summarize","input":{},"stream":false}`},
		{"invalid targetAgentId", `{"parentInvocationId":"inv_parent123","targetAgentId":"agent target","capability":"summarize","input":{},"stream":false}`},
		{"invalid capability", `{"parentInvocationId":"inv_parent123","targetAgentId":"agent_target02","capability":"sum marize","input":{},"stream":false}`},
		{"input not object", `{"parentInvocationId":"inv_parent123","targetAgentId":"agent_target02","capability":"summarize","input":"string","stream":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.serve(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestAgentHandlerModeNegotiation(t *testing.T) {
	handler, token := newTestAgentHandler(t, &mockNestedLedgerReader{}, &mockVersionResolver{})

	// stream=true but Accept=application/json
	body := `{"parentInvocationId":"inv_parent123","targetAgentId":"agent_target02","capability":"summarize","input":{},"stream":true}`
	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)
	if rec.Code != http.StatusNotAcceptable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotAcceptable)
	}
}

func TestAgentHandlerParentNotFound(t *testing.T) {
	ledgerReader := &mockNestedLedgerReader{err: errors.New("not found")}
	handler, token := newTestAgentHandler(t, ledgerReader, &mockVersionResolver{})

	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(validNestedBody()))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAgentHandlerParentNotRunning(t *testing.T) {
	parent := runningParentDetail()
	parent.Invocation.Status = "succeeded"
	ledgerReader := &mockNestedLedgerReader{invocation: parent}
	handler, token := newTestAgentHandler(t, ledgerReader, &mockVersionResolver{})

	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(validNestedBody()))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestAgentHandlerParentTargetMismatch(t *testing.T) {
	parent := runningParentDetail()
	parent.Invocation.TargetAgentID = "agent_different"
	ledgerReader := &mockNestedLedgerReader{invocation: parent}
	handler, token := newTestAgentHandler(t, ledgerReader, &mockVersionResolver{})

	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(validNestedBody()))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAgentHandlerDuplicateMembers(t *testing.T) {
	handler, token := newTestAgentHandler(t, &mockNestedLedgerReader{}, &mockVersionResolver{})

	body := `{"parentInvocationId":"inv_parent123","parentInvocationId":"inv_other","targetAgentId":"agent_target02","capability":"summarize","input":{},"stream":false}`
	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAgentHandlerUnknownFields(t *testing.T) {
	handler, token := newTestAgentHandler(t, &mockNestedLedgerReader{}, &mockVersionResolver{})

	body := `{"parentInvocationId":"inv_parent123","targetAgentId":"agent_target02","capability":"summarize","input":{},"stream":false,"unknownField":"value"}`
	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAgentHandlerMissingStreamField(t *testing.T) {
	handler, token := newTestAgentHandler(t, &mockNestedLedgerReader{}, &mockVersionResolver{})

	// stream field is required by router-agent.v1; omission must be rejected.
	body := `{"parentInvocationId":"inv_parent123","targetAgentId":"agent_target02","capability":"summarize","input":{}}`
	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for missing stream", rec.Code, http.StatusBadRequest)
	}
}

func TestAgentHandlerNullStreamField(t *testing.T) {
	handler, token := newTestAgentHandler(t, &mockNestedLedgerReader{}, &mockVersionResolver{})

	// stream null must be rejected.
	body := `{"parentInvocationId":"inv_parent123","targetAgentId":"agent_target02","capability":"summarize","input":{},"stream":null}`
	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for null stream", rec.Code, http.StatusBadRequest)
	}
}

func TestAgentHandlerPayloadTooLarge(t *testing.T) {
	handler, token := newTestAgentHandler(t, &mockNestedLedgerReader{}, &mockVersionResolver{})

	// Create a body larger than the limit
	largeInput := strings.Repeat("x", 1048577)
	body := `{"parentInvocationId":"inv_parent123","targetAgentId":"agent_target02","capability":"summarize","input":{"data":"` + largeInput + `"},"stream":false}`
	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestAgentHandlerErrorResponseShape(t *testing.T) {
	handler, token := newTestAgentHandler(t, &mockNestedLedgerReader{err: errors.New("not found")}, &mockVersionResolver{})

	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(validNestedBody()))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("content type = %s, want application/json", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("x-nek-trace-id") == "" {
		t.Error("expected x-nek-trace-id header")
	}

	var errorBody map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&errorBody); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errorBody["code"] != "NOT_FOUND" {
		t.Errorf("error code = %v, want NOT_FOUND", errorBody["code"])
	}
	if errorBody["message"] == "" {
		t.Error("expected non-empty error message")
	}
	if errorBody["traceId"] == "" {
		t.Error("expected non-empty traceId")
	}
	// Pre-correlation errors must not have invocationId or rootTaskId
	if _, exists := errorBody["invocationId"]; exists {
		t.Error("pre-correlation error must not contain invocationId")
	}
	if _, exists := errorBody["rootTaskId"]; exists {
		t.Error("pre-correlation error must not contain rootTaskId")
	}
}

func TestNewAgentInvocationHandlerValidation(t *testing.T) {
	token := "test-token"
	binding, _ := nested.NewAgentBinding([]nested.AgentPrincipal{
		{AgentID: "agent01", TokenSHA256: agentTokenDigest(token)},
	})
	serviceAuth, _ := auth.NewStaticAuthenticator([]auth.Principal{
		{ID: "service", TokenSHA256: agentTokenDigest("svc-token")},
	})
	dispatchHandler, _ := NewDispatchHandler(serviceAuth, &mockResolver{}, 1048576, 30000*1000000)
	ledgerReader := &mockNestedLedgerReader{}
	versionResolver := &mockVersionResolver{}

	tests := []struct {
		name            string
		binding         *nested.AgentBinding
		ledgerReader    NestedLedgerReader
		versionResolver VersionResolver
		dispatchHandler *DispatchHandler
		requestLimit    int64
		deadline        time.Duration
		wantErr         bool
	}{
		{"valid", binding, ledgerReader, versionResolver, dispatchHandler, 1048576, 30 * time.Second, false},
		{"nil binding", nil, ledgerReader, versionResolver, dispatchHandler, 1048576, 30 * time.Second, true},
		{"nil ledger reader", binding, nil, versionResolver, dispatchHandler, 1048576, 30 * time.Second, true},
		{"nil version resolver", binding, ledgerReader, nil, dispatchHandler, 1048576, 30 * time.Second, true},
		{"nil dispatch handler", binding, ledgerReader, versionResolver, nil, 1048576, 30 * time.Second, true},
		{"invalid request limit", binding, ledgerReader, versionResolver, dispatchHandler, 0, 30 * time.Second, true},
		{"invalid deadline", binding, ledgerReader, versionResolver, dispatchHandler, 1048576, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAgentInvocationHandler(tt.binding, tt.ledgerReader, tt.versionResolver, tt.dispatchHandler, tt.requestLimit, tt.deadline)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAgentInvocationHandler() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestAgentHandlerNestedJSONSuccessPath exercises the full US1 trusted nested
// call flow: auth -> request -> parent read -> child derivation -> version
// resolution -> DispatchChild -> Ledger -> JSON result. It asserts lineage,
// correlation, and content-exclusion.
func TestAgentHandlerNestedJSONSuccessPath(t *testing.T) {
	agentToken := "agent-test-token"
	binding, err := nested.NewAgentBinding([]nested.AgentPrincipal{
		{AgentID: "agent_caller01", TokenSHA256: agentTokenDigest(agentToken)},
	})
	if err != nil {
		t.Fatalf("NewAgentBinding() error = %v", err)
	}

	serviceAuth, err := auth.NewStaticAuthenticator([]auth.Principal{
		{ID: "router-service", TokenSHA256: agentTokenDigest("service-token")},
	})
	if err != nil {
		t.Fatalf("NewStaticAuthenticator() error = %v", err)
	}

	// Resolver returns a valid card for the target agent.
	targetCard := contracts.AgentCard{
		AgentID: "agent_target02", Version: "2.0.0",
		Protocol:       contracts.AgentProtocol{Type: "a2a", Version: contracts.A2AProtocolVersion, Transport: "JSONRPC", Endpoint: "https://target.example/a2a"},
		Authentication: contracts.AgentAuthentication{Type: "none"},
		Skills:         []contracts.AgentSkill{{ID: "summarize"}},
		Limits:         contracts.AgentLimits{TimeoutMS: 5000, MaxInputBytes: "4096", MaxOutputBytes: "4096", Streaming: false},
	}
	resolver := &resolverStub{response: contracts.ResolveAgentResponse{
		Card: targetCard,
		Installation: contracts.ResolvedInstallation{
			InstallationID: "inst-01", WorkspaceID: "ws_test789",
			AgentID: "agent_target02", InstalledVersion: "2.0.0",
			AcceptedPermissions: []string{}, Status: "enabled",
		},
	}}

	// Transport returns a valid A2A message result.
	transport := &transportStub{result: json.RawMessage(`{"kind":"message","messageId":"msg-01","role":"agent","parts":[{"kind":"data","data":{"answer":"42"}}]}`)}

	// Ledger records all appended events.
	ledgerRec := &ledgerRecorder{}

	dispatchHandler, err := NewDispatchHandlerWithTransportAndLedger(serviceAuth, resolver, transport, ledgerRec, 1048576, 30*time.Second)
	if err != nil {
		t.Fatalf("NewDispatchHandlerWithTransportAndLedger() error = %v", err)
	}

	ledgerReader := &mockNestedLedgerReader{invocation: runningParentDetail()}
	versionResolver := &mockVersionResolver{response: contracts.ResolveInstalledVersionResponse{Version: "2.0.0"}}

	handler, err := NewAgentInvocationHandler(binding, ledgerReader, versionResolver, dispatchHandler, 1048576, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgentInvocationHandler() error = %v", err)
	}

	req := httptest.NewRequest("POST", "/agent/v1/invocations", strings.NewReader(validNestedBody()))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.serve(rec, req)

	// Assert 200 OK.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	// Assert InvocationResult correlation.
	var result contracts.InvocationResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.InvocationID == "" || result.InvocationID == "inv_parent123" {
		t.Errorf("child invocation ID should be new, got %s", result.InvocationID)
	}
	if result.RootTaskID != "task_root456" {
		t.Errorf("root task ID = %s, want task_root456", result.RootTaskID)
	}
	if string(result.TraceID) != "trc_abc123_1" {
		t.Errorf("trace ID = %s, want trc_abc123_1", result.TraceID)
	}
	if result.Status != "succeeded" {
		t.Errorf("status = %s, want succeeded", result.Status)
	}

	// Assert Ledger lifecycle: created -> routing -> started -> succeeded.
	if len(ledgerRec.events) != 4 {
		t.Fatalf("ledger events = %d, want 4", len(ledgerRec.events))
	}
	expectedTypes := []string{"created", "routing", "started", "succeeded"}
	for i, event := range ledgerRec.events {
		if event.Type != expectedTypes[i] {
			t.Errorf("event[%d].Type = %s, want %s", i, event.Type, expectedTypes[i])
		}
		// Assert parent lineage propagation.
		if event.ParentInvocationID != "inv_parent123" {
			t.Errorf("event[%d].ParentInvocationID = %s, want inv_parent123", i, event.ParentInvocationID)
		}
		if event.RootTaskID != "task_root456" {
			t.Errorf("event[%d].RootTaskID = %s, want task_root456", i, event.RootTaskID)
		}
		if event.TraceID != "trc_abc123_1" {
			t.Errorf("event[%d].TraceID = %s, want trc_abc123_1", i, event.TraceID)
		}
		if event.WorkspaceID != "ws_test789" {
			t.Errorf("event[%d].WorkspaceID = %s, want ws_test789", i, event.WorkspaceID)
		}
		if event.Caller.Type != "agent" || event.Caller.ID != "agent_caller01" {
			t.Errorf("event[%d].Caller = %+v, want agent/agent_caller01", i, event.Caller)
		}
		if event.TargetAgentID != "agent_target02" {
			t.Errorf("event[%d].TargetAgentID = %s, want agent_target02", i, event.TargetAgentID)
		}
		if event.AgentCardVersion != "2.0.0" {
			t.Errorf("event[%d].AgentCardVersion = %s, want 2.0.0", i, event.AgentCardVersion)
		}
		// Content-exclusion: no input/output stored in Ledger.
		if event.ChunkIndex != nil || event.ChunkBytes != nil {
			t.Errorf("event[%d] stores content metadata: %+v", i, event)
		}
	}

	// Assert transport received the correct dispatch.
	if transport.calls != 1 {
		t.Fatalf("transport calls = %d, want 1", transport.calls)
	}
	if transport.dispatch.ParentInvocationID != "inv_parent123" {
		t.Errorf("transport dispatch ParentInvocationID = %s, want inv_parent123", transport.dispatch.ParentInvocationID)
	}
	if transport.dispatch.Caller.Type != "agent" {
		t.Errorf("transport dispatch Caller.Type = %s, want agent", transport.dispatch.Caller.Type)
	}
}
