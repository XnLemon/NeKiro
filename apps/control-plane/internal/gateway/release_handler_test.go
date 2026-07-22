package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/contracts"
)

type fakeReleaseCatalog struct {
	release catalog.AgentRelease
	err     error
	request contracts.CreateAgentReleaseRequest
}

func (service *fakeReleaseCatalog) CreateRelease(_ context.Context, _ catalog.AuthenticatedCaller, _, _ string, request contracts.CreateAgentReleaseRequest) (catalog.AgentRelease, error) {
	service.request = request
	return service.release, service.err
}
func (service *fakeReleaseCatalog) GetRelease(context.Context, catalog.AuthenticatedCaller, string) (catalog.AgentRelease, error) {
	return service.release, service.err
}
func (service *fakeReleaseCatalog) VerifyRelease(context.Context, catalog.AuthenticatedCaller, string) (catalog.AgentRelease, error) {
	return service.release, service.err
}
func (service *fakeReleaseCatalog) PublishRelease(context.Context, catalog.AuthenticatedCaller, string) (catalog.AgentRelease, error) {
	return service.release, service.err
}
func (service *fakeReleaseCatalog) SuspendRelease(context.Context, catalog.AuthenticatedCaller, string) (catalog.AgentRelease, error) {
	return service.release, service.err
}
func (service *fakeReleaseCatalog) RevokeRelease(context.Context, catalog.AuthenticatedCaller, string) (catalog.AgentRelease, error) {
	return service.release, service.err
}

func TestReleaseHandlerCreatesExactReleaseWithoutProofMaterial(t *testing.T) {
	now := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	digest := [32]byte{1, 2, 3}
	evidence := [32]byte{4, 5, 6}
	service := &fakeReleaseCatalog{release: catalog.AgentRelease{ReleaseID: "release-a", ProviderID: "provider-a", AgentID: "agent-a", AgentCardVersion: "1.0.0", CardDigest: digest, EndpointBindingID: "binding-a", EndpointOrigin: "https://agent.example", EndpointPath: "/a2a", VerificationMethod: catalog.VerificationMethodHTTPWellKnown, VerificationEvidenceDigest: &evidence, State: catalog.ReleaseVerified, CreatedAt: now, UpdatedAt: now, VerifiedAt: &now}}
	handler := newReleaseTestHandler(t, service)
	request := httptest.NewRequest(http.MethodPost, "/v4/providers/provider-a/agents/agent-a/releases", strings.NewReader(`{"version":"1.0.0","endpointBindingId":"binding-a"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response contracts.AgentReleaseResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ReleaseID != "release-a" || response.AgentCardVersion != "1.0.0" || response.State != string(catalog.ReleaseVerified) || response.VerificationEvidenceDigest == nil || strings.Contains(recorder.Body.String(), "proof") {
		t.Fatalf("response=%#v body=%s", response, recorder.Body.String())
	}
}

func TestReleaseHandlerMapsIllegalTransitionToTypedConflict(t *testing.T) {
	handler := newReleaseTestHandler(t, &fakeReleaseCatalog{err: catalog.ErrReleaseConflict})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v4/releases/release-a/publish", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var publicError contracts.TrustedPublicationError
	if err := json.Unmarshal(recorder.Body.Bytes(), &publicError); err != nil {
		t.Fatal(err)
	}
	if publicError.Code != contracts.TrustedErrorConflict || strings.Contains(recorder.Body.String(), catalog.ErrReleaseConflict.Error()) {
		t.Fatalf("public error=%#v body=%s", publicError, recorder.Body.String())
	}
}

func newReleaseTestHandler(t *testing.T, service ReleaseCatalogService) http.Handler {
	t.Helper()
	traces, err := NewTraceGenerator()
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewReleaseHandler(fakeAuthenticator{caller: catalog.AuthenticatedCaller{ID: "owner-a"}}, service, traces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}
