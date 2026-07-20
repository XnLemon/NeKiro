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
