package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace"
	"github.com/Nene7ko/NeKiro/contracts"
)

type workspaceTestAuthenticator struct {
	caller catalog.AuthenticatedCaller
	err    error
}

func (auth workspaceTestAuthenticator) Authenticate(*http.Request) (catalog.AuthenticatedCaller, error) {
	if auth.err != nil {
		return catalog.AuthenticatedCaller{}, auth.err
	}
	return auth.caller, nil
}

type workspaceTestService struct {
	workspace          contracts.Workspace
	installation       contracts.Installation
	resolveErr         error
	createErr          error
	getErr             error
	installErr         error
	updateErr          error
	uninstallErr       error
	createCalls        int
	getCalls           int
	installCalls       int
	updateCalls        int
	uninstallCalls     int
	lastInstallRequest contracts.InstallAgentRequest
	lastUpdate         struct {
		workspaceID, installationID, status string
	}
	lastUninstall struct {
		workspaceID, installationID string
	}
}

func (service *workspaceTestService) CreateWorkspace(_ context.Context, caller workspace.AuthenticatedCaller, request contracts.CreateWorkspaceRequest) (contracts.Workspace, error) {
	service.createCalls++
	if service.createErr != nil {
		return contracts.Workspace{}, service.createErr
	}
	service.workspace = contracts.Workspace{WorkspaceID: request.WorkspaceID, OwnerID: caller.ID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	return service.workspace, nil
}
func (service *workspaceTestService) GetWorkspace(context.Context, workspace.AuthenticatedCaller, string) (contracts.Workspace, error) {
	service.getCalls++
	if service.getErr != nil {
		return contracts.Workspace{}, service.getErr
	}
	return service.workspace, nil
}
func (service *workspaceTestService) Install(_ context.Context, _ workspace.AuthenticatedCaller, _ string, request contracts.InstallAgentRequest) (contracts.Installation, error) {
	service.installCalls++
	service.lastInstallRequest = request
	if service.installErr != nil {
		return contracts.Installation{}, service.installErr
	}
	if service.installation.InstallationID == "" {
		service.installation = contracts.Installation{
			InstallationID:      "installation-a",
			WorkspaceID:         "workspace-a",
			AgentID:             request.AgentID,
			VersionConstraint:   request.VersionConstraint,
			InstalledVersion:    "1.0.0",
			AcceptedPermissions: request.AcceptedPermissions,
			Status:              "enabled",
			InstalledAt:         time.Now().UTC(),
			UpdatedAt:           time.Now().UTC(),
		}
	}
	return service.installation, nil
}
func (service *workspaceTestService) GetInstallation(context.Context, workspace.AuthenticatedCaller, string, string) (contracts.Installation, error) {
	return contracts.Installation{}, nil
}
func (service *workspaceTestService) ListInstallations(context.Context, workspace.AuthenticatedCaller, string, int, *string) (contracts.InstallationList, error) {
	return contracts.InstallationList{Items: []contracts.Installation{}}, nil
}
func (service *workspaceTestService) UpdateInstallation(_ context.Context, _ workspace.AuthenticatedCaller, workspaceID, installationID, status string) (contracts.Installation, error) {
	service.updateCalls++
	service.lastUpdate.workspaceID = workspaceID
	service.lastUpdate.installationID = installationID
	service.lastUpdate.status = status
	if service.updateErr != nil {
		return contracts.Installation{}, service.updateErr
	}
	service.installation.Status = status
	return service.installation, nil
}
func (service *workspaceTestService) Uninstall(_ context.Context, _ workspace.AuthenticatedCaller, workspaceID, installationID string) (contracts.Installation, error) {
	service.uninstallCalls++
	service.lastUninstall.workspaceID = workspaceID
	service.lastUninstall.installationID = installationID
	if service.uninstallErr != nil {
		return contracts.Installation{}, service.uninstallErr
	}
	service.installation.Status = "uninstalled"
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	service.installation.UpdatedAt = now
	service.installation.UninstalledAt = &now
	return service.installation, nil
}
func (service *workspaceTestService) Resolve(context.Context, contracts.ResolveAgentRequest) (contracts.ResolveAgentResponse, error) {
	return contracts.ResolveAgentResponse{}, service.resolveErr
}

func TestWorkspaceHandlerRequiresBearerAndRequiredListLimit(t *testing.T) {
	service := &workspaceTestService{}
	handler := newWorkspaceTestHandler(t, workspaceTestAuthenticator{caller: catalog.AuthenticatedCaller{ID: "owner-a"}}, service)
	request := httptest.NewRequest(http.MethodPost, "/v3/workspaces", strings.NewReader(`{"workspaceId":"workspace-a"}`))
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusCreated || service.workspace.OwnerID != "owner-a" {
		t.Fatalf("create response = %d, workspace = %#v", response.Code, service.workspace)
	}

	request = httptest.NewRequest(http.MethodGet, "/v3/workspaces/workspace-a/installations", nil)
	request.Header.Set("Authorization", "Bearer token")
	response = httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing limit status = %d", response.Code)
	}

	unauthenticated := newWorkspaceTestHandler(t, workspaceTestAuthenticator{err: ErrUnauthenticated}, service)
	request = httptest.NewRequest(http.MethodGet, "/v3/workspaces/workspace-a", nil)
	response = httptest.NewRecorder()
	unauthenticated.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.Code)
	}
}

func TestWorkspaceHandlerMapsWorkspaceCreateReadOutcomes(t *testing.T) {
	service := &workspaceTestService{}
	handler := newWorkspaceTestHandler(t, workspaceTestAuthenticator{caller: catalog.AuthenticatedCaller{ID: "owner-a"}}, service)

	request := httptest.NewRequest(http.MethodPost, "/v3/workspaces", strings.NewReader(`{"workspaceId":"workspace-a"}`))
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusCreated || response.Header().Get(TraceHeader) == "" {
		t.Fatalf("create response status=%d trace=%q", response.Code, response.Header().Get(TraceHeader))
	}
	var created contracts.Workspace
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.WorkspaceID != "workspace-a" || created.OwnerID != "owner-a" {
		t.Fatalf("created Workspace = %#v", created)
	}

	request = httptest.NewRequest(http.MethodGet, "/v3/workspaces/workspace-a", nil)
	request.Header.Set("Authorization", "Bearer token")
	response = httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("read response status = %d", response.Code)
	}
	var read contracts.Workspace
	if err := json.Unmarshal(response.Body.Bytes(), &read); err != nil {
		t.Fatal(err)
	}
	if read != created {
		t.Fatalf("read Workspace = %#v, want %#v", read, created)
	}

	service.createErr = workspace.ErrConflict
	request = httptest.NewRequest(http.MethodPost, "/v3/workspaces", strings.NewReader(`{"workspaceId":"workspace-a"}`))
	request.Header.Set("Authorization", "Bearer token")
	response = httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"CONFLICT"`) {
		t.Fatalf("duplicate response status=%d body=%s", response.Code, response.Body.String())
	}

	service.getErr = workspace.ErrForbidden
	request = httptest.NewRequest(http.MethodGet, "/v3/workspaces/workspace-a", nil)
	request.Header.Set("Authorization", "Bearer token")
	response = httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), "owner-a") {
		t.Fatalf("forbidden read status=%d body=%s", response.Code, response.Body.String())
	}
	service.getErr = workspace.ErrNotFound
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v3/workspaces/missing-workspace", nil)
	request.Header.Set("Authorization", "Bearer token")
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("unknown read status=%d body=%s", response.Code, response.Body.String())
	}

	service.getErr = nil
	createCallsBeforeInvalid := service.createCalls
	request = httptest.NewRequest(http.MethodPost, "/v3/workspaces", strings.NewReader(`{"workspaceId":"workspace-b","ownerId":"attacker"}`))
	request.Header.Set("Authorization", "Bearer token")
	response = httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || service.createCalls != createCallsBeforeInvalid {
		t.Fatalf("owner override status=%d create calls=%d before=%d", response.Code, service.createCalls, createCallsBeforeInvalid)
	}
}

func TestWorkspaceHandlerInstallRequiresPermissionArrayAndPreservesEmpty(t *testing.T) {
	service := &workspaceTestService{}
	handler := newWorkspaceTestHandler(t, workspaceTestAuthenticator{caller: catalog.AuthenticatedCaller{ID: "owner-a"}}, service)
	validBody := `{"agentId":"agent-a","versionConstraint":"^1.0.0","acceptedPermissions":[]}`

	request := httptest.NewRequest(http.MethodPost, "/v3/workspaces/workspace-a/installations", strings.NewReader(validBody))
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusCreated || service.installCalls != 1 || service.lastInstallRequest.AcceptedPermissions == nil {
		t.Fatalf("explicit empty permissions status=%d calls=%d request=%#v", response.Code, service.installCalls, service.lastInstallRequest)
	}
	var installation contracts.Installation
	if err := json.Unmarshal(response.Body.Bytes(), &installation); err != nil {
		t.Fatal(err)
	}
	if installation.AcceptedPermissions == nil || len(installation.AcceptedPermissions) != 0 {
		t.Fatalf("empty permission response = %#v", installation.AcceptedPermissions)
	}

	for _, body := range []string{
		`{"agentId":"agent-a","versionConstraint":"^1.0.0"}`,
		`{"agentId":"agent-a","versionConstraint":"^1.0.0","acceptedPermissions":null}`,
		`{"agentId":"agent-a","versionConstraint":"^1.0.0","acceptedPermissions":"read"}`,
	} {
		callsBefore := service.installCalls
		request = httptest.NewRequest(http.MethodPost, "/v3/workspaces/workspace-a/installations", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer token")
		response = httptest.NewRecorder()
		handler.Routes().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || service.installCalls != callsBefore {
			t.Fatalf("invalid permission presence body=%s status=%d calls=%d before=%d", body, response.Code, service.installCalls, callsBefore)
		}
	}

	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{workspace.ErrForbidden, http.StatusForbidden, "FORBIDDEN"},
		{workspace.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{workspace.ErrConflict, http.StatusConflict, "CONFLICT"},
		{workspace.ErrDependency, http.StatusServiceUnavailable, "DEPENDENCY_ERROR"},
	} {
		service.installErr = test.err
		request = httptest.NewRequest(http.MethodPost, "/v3/workspaces/workspace-a/installations", strings.NewReader(validBody))
		request.Header.Set("Authorization", "Bearer token")
		response = httptest.NewRecorder()
		handler.Routes().ServeHTTP(response, request)
		if response.Code != test.status || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
			t.Fatalf("install error=%v status=%d body=%s", test.err, response.Code, response.Body.String())
		}
	}
}

func TestWorkspaceHandlerMapsLifecycleSuccessAndFailures(t *testing.T) {
	service := &workspaceTestService{installation: contracts.Installation{
		InstallationID: "installation-a", WorkspaceID: "workspace-a", AgentID: "agent-a",
		VersionConstraint: "^1.0.0", InstalledVersion: "1.0.0",
		AcceptedPermissions: []string{"document.read"}, Status: "enabled",
		InstalledAt: time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC),
	}}
	handler := newWorkspaceTestHandler(t, workspaceTestAuthenticator{caller: catalog.AuthenticatedCaller{ID: "owner-a"}}, service)

	request := httptest.NewRequest(http.MethodPatch, "/v3/workspaces/workspace-a/installations/installation-a", strings.NewReader(`{"status":"disabled"}`))
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get(TraceHeader) == "" || service.updateCalls != 1 || service.lastUpdate.status != "disabled" {
		t.Fatalf("disable response status=%d trace=%q calls=%d request=%#v", response.Code, response.Header().Get(TraceHeader), service.updateCalls, service.lastUpdate)
	}
	var disabled contracts.Installation
	if err := json.Unmarshal(response.Body.Bytes(), &disabled); err != nil {
		t.Fatal(err)
	}
	if disabled.Status != "disabled" || disabled.InstalledVersion != "1.0.0" || len(disabled.AcceptedPermissions) != 1 {
		t.Fatalf("disable response = %#v", disabled)
	}

	request = httptest.NewRequest(http.MethodDelete, "/v3/workspaces/workspace-a/installations/installation-a", nil)
	request.Header.Set("Authorization", "Bearer token")
	response = httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.uninstallCalls != 1 || service.lastUninstall.installationID != "installation-a" {
		t.Fatalf("uninstall response status=%d calls=%d request=%#v", response.Code, service.uninstallCalls, service.lastUninstall)
	}
	var terminal contracts.Installation
	if err := json.Unmarshal(response.Body.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Status != "uninstalled" || terminal.UninstalledAt == nil || !terminal.UninstalledAt.Equal(terminal.UpdatedAt) {
		t.Fatalf("terminal response = %#v", terminal)
	}

	for _, test := range []struct {
		name       string
		method     string
		body       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "invalid missing status", method: http.MethodPatch, body: `{}`, err: workspace.ErrInvalid, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_ERROR"},
		{name: "invalid terminal target", method: http.MethodPatch, body: `{"status":"uninstalled"}`, err: workspace.ErrInvalid, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_ERROR"},
		{name: "unknown fields", method: http.MethodPatch, body: `{"status":"enabled","secret":"token=secret"}`, err: workspace.ErrInvalid, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_ERROR"},
		{name: "forbidden", method: http.MethodPatch, body: `{"status":"enabled"}`, err: workspace.ErrForbidden, wantStatus: http.StatusForbidden, wantCode: "FORBIDDEN"},
		{name: "not found", method: http.MethodPatch, body: `{"status":"enabled"}`, err: workspace.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "conflict", method: http.MethodPatch, body: `{"status":"enabled"}`, err: workspace.ErrConflict, wantStatus: http.StatusConflict, wantCode: "CONFLICT"},
		{name: "dependency", method: http.MethodDelete, body: ``, err: workspace.ErrDependency, wantStatus: http.StatusServiceUnavailable, wantCode: "DEPENDENCY_ERROR"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service.updateErr = nil
			service.uninstallErr = nil
			if test.method == http.MethodPatch {
				service.updateErr = test.err
			} else {
				service.uninstallErr = test.err
			}
			beforeUpdate := service.updateCalls
			beforeUninstall := service.uninstallCalls
			request := httptest.NewRequest(test.method, "/v3/workspaces/workspace-a/installations/installation-a", strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer token")
			response := httptest.NewRecorder()
			handler.Routes().ServeHTTP(response, request)
			if response.Code != test.wantStatus || response.Header().Get(TraceHeader) == "" || !strings.Contains(response.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("status=%d trace=%q body=%s", response.Code, response.Header().Get(TraceHeader), response.Body.String())
			}
			if test.err == workspace.ErrInvalid && test.method == http.MethodPatch && beforeUpdate != service.updateCalls {
				t.Fatalf("invalid PATCH reached service: before=%d after=%d", beforeUpdate, service.updateCalls)
			}
			if test.method == http.MethodDelete && test.err == workspace.ErrDependency && beforeUninstall == service.uninstallCalls {
				t.Fatalf("dependency DELETE did not reach service")
			}
		})
	}

	unauthenticated := newWorkspaceTestHandler(t, workspaceTestAuthenticator{err: ErrUnauthenticated}, service)
	request = httptest.NewRequest(http.MethodDelete, "/v3/workspaces/workspace-a/installations/installation-a", nil)
	response = httptest.NewRecorder()
	unauthenticated.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || service.uninstallCalls != 2 {
		t.Fatalf("unauthenticated response status=%d uninstallCalls=%d", response.Code, service.uninstallCalls)
	}
}

func TestWorkspaceHandlerRejectsOversizedJSONBeforeService(t *testing.T) {
	service := &workspaceTestService{}
	handler := newWorkspaceTestHandler(t, workspaceTestAuthenticator{caller: catalog.AuthenticatedCaller{ID: "owner-a"}}, service)
	request := httptest.NewRequest(http.MethodPost, "/v3/workspaces", strings.NewReader(strings.Repeat("x", contracts.WorkspaceRequestMaximumBodyBytes+1)))
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized Workspace request status = %d, want 400", response.Code)
	}
	if service.workspace.WorkspaceID != "" {
		t.Fatalf("oversized Workspace request reached service: %#v", service.workspace)
	}
}

func TestWorkspaceHandlerSeparatesPreAndPostCorrelationErrors(t *testing.T) {
	service := &workspaceTestService{resolveErr: workspace.ErrDependency}
	handler := newWorkspaceTestHandler(t, workspaceTestAuthenticator{caller: catalog.AuthenticatedCaller{ID: "owner-a"}}, service)
	request := httptest.NewRequest(http.MethodPost, "/internal/v2/resolve-agent", strings.NewReader(`{"invocationId":"bad id"}`))
	request.Header.Set("Authorization", "Bearer internal")
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("pre-correlation status = %d", response.Code)
	}
	var pre contracts.PlatformErrorV3
	if err := json.Unmarshal(response.Body.Bytes(), &pre); err != nil {
		t.Fatal(err)
	}
	if pre.InvocationID != "" || pre.RootTaskID != "" {
		t.Fatalf("pre-correlation error leaked IDs: %#v", pre)
	}

	request = httptest.NewRequest(http.MethodPost, "/internal/v2/resolve-agent", strings.NewReader(`{"invocationId":"inv-a","rootTaskId":"task-a","traceId":"trace-a","workspaceId":"workspace-a","agentId":"agent-a","version":"bad","capability":"capability-a"}`))
	request.Header.Set("Authorization", "Bearer internal")
	response = httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("post-correlation status = %d", response.Code)
	}
	var post contracts.PlatformErrorV3
	if err := json.Unmarshal(response.Body.Bytes(), &post); err != nil {
		t.Fatal(err)
	}
	if post.InvocationID != "inv-a" || post.RootTaskID != "task-a" || post.TraceID != "trace-a" {
		t.Fatalf("post-correlation error = %#v", post)
	}
}

func TestWorkspaceHandlerMapsUnexpectedErrorsToInternalServerError(t *testing.T) {
	service := &workspaceTestService{resolveErr: errors.New("unexpected service failure")}
	handler := newWorkspaceTestHandler(t, workspaceTestAuthenticator{caller: catalog.AuthenticatedCaller{ID: "router-a"}}, service)
	request := httptest.NewRequest(http.MethodPost, "/internal/v2/resolve-agent", strings.NewReader(`{"invocationId":"inv-a","rootTaskId":"task-a","traceId":"trace-a","workspaceId":"workspace-a","agentId":"agent-a","version":"1.0.0","capability":"capability-a"}`))
	request.Header.Set("Authorization", "Bearer internal")
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected error status = %d, want 500", response.Code)
	}
	var platformError contracts.PlatformErrorV3
	if err := json.Unmarshal(response.Body.Bytes(), &platformError); err != nil {
		t.Fatal(err)
	}
	if platformError.Code != contracts.ErrorCodeInternal || platformError.TraceID != "trace-a" {
		t.Fatalf("unexpected internal error = %#v", platformError)
	}
}

func newWorkspaceTestHandler(t *testing.T, auth Authenticator, service WorkspaceService) *WorkspaceHandler {
	t.Helper()
	traces, err := NewTraceGenerator()
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewWorkspaceHandler(auth, auth, service, traces, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

var _ WorkspaceService = (*workspaceTestService)(nil)
var _ Authenticator = workspaceTestAuthenticator{}
