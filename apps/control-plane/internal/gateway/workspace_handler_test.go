package gateway

import (
	"context"
	"encoding/json"
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
	workspace  contracts.Workspace
	resolveErr error
}

func (service *workspaceTestService) CreateWorkspace(_ context.Context, caller workspace.AuthenticatedCaller, request contracts.CreateWorkspaceRequest) (contracts.Workspace, error) {
	service.workspace = contracts.Workspace{WorkspaceID: request.WorkspaceID, OwnerID: caller.ID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	return service.workspace, nil
}
func (service *workspaceTestService) GetWorkspace(context.Context, workspace.AuthenticatedCaller, string) (contracts.Workspace, error) {
	return service.workspace, nil
}
func (service *workspaceTestService) Install(context.Context, workspace.AuthenticatedCaller, string, contracts.InstallAgentRequest) (contracts.Installation, error) {
	return contracts.Installation{}, nil
}
func (service *workspaceTestService) GetInstallation(context.Context, workspace.AuthenticatedCaller, string, string) (contracts.Installation, error) {
	return contracts.Installation{}, nil
}
func (service *workspaceTestService) ListInstallations(context.Context, workspace.AuthenticatedCaller, string, int, *string) (contracts.InstallationList, error) {
	return contracts.InstallationList{Items: []contracts.Installation{}}, nil
}
func (service *workspaceTestService) UpdateInstallation(context.Context, workspace.AuthenticatedCaller, string, string, string) (contracts.Installation, error) {
	return contracts.Installation{}, nil
}
func (service *workspaceTestService) Uninstall(context.Context, workspace.AuthenticatedCaller, string, string) (contracts.Installation, error) {
	return contracts.Installation{}, nil
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
