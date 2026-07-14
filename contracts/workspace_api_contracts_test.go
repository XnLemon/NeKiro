package contracts

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestWorkspaceAndInstallationV2Schemas(t *testing.T) {
	validator := mustValidator(t)
	if _, err := fs.ReadFile(ContractFiles(), "installation/v2/semantic-rules.md"); err != nil {
		t.Fatalf("Installation v2 semantic rules are not embedded: %v", err)
	}
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	workspace := Workspace{
		WorkspaceID: "workspace-1",
		OwnerID:     "owner-1",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := validator.ValidateWorkspace(workspace); err != nil {
		t.Fatalf("valid Workspace rejected: %v", err)
	}
	workspace.OwnerID = ""
	if err := validator.ValidateWorkspace(workspace); err == nil {
		t.Fatal("Workspace without owner was accepted")
	}

	installation := validInstallation()
	if err := validator.ValidateInstallation(installation); err != nil {
		t.Fatalf("valid current Installation rejected: %v", err)
	}
	installation.UninstalledAt = &now
	if err := validator.ValidateInstallation(installation); err == nil {
		t.Fatal("current Installation with uninstalledAt was accepted")
	}
	installation.Status = "uninstalled"
	installation.UninstalledAt = nil
	if err := validator.ValidateInstallation(installation); err == nil {
		t.Fatal("uninstalled Installation without uninstalledAt was accepted")
	}
	installation.UpdatedAt = now
	installation.UninstalledAt = &now
	if err := validator.ValidateInstallation(installation); err != nil {
		t.Fatalf("valid uninstalled Installation rejected: %v", err)
	}
	installation.UpdatedAt = now.Add(time.Minute)
	if err := validator.ValidateInstallation(installation); err == nil {
		t.Fatal("uninstalled Installation with a stale uninstalledAt was accepted")
	}
	installation.UpdatedAt = now
	installation.AcceptedPermissions = []string{"z.permission", "a.permission"}
	if err := validator.ValidateInstallation(installation); err == nil {
		t.Fatal("Installation with non-canonical permissions was accepted")
	}
	installation.AcceptedPermissions = []string{"document.read"}
	installation.InstalledAt = now.Add(time.Minute)
	if err := validator.ValidateInstallation(installation); err == nil {
		t.Fatal("Installation with installedAt after updatedAt was accepted")
	}
	installation.InstalledAt = now
	unchanged := installation
	changed := installation
	changed.InstalledVersion = "1.3.0"
	if err := ValidateInstallationImmutablePin(unchanged, changed); err == nil {
		t.Fatal("Installation immutable pin mutation was accepted")
	}
	installation.VersionConstraint = ">=1.0.0+" + strings.Repeat("a", 249)
	if err := validator.ValidateInstallation(installation); err != nil {
		t.Fatalf("long version constraint rejected: %v", err)
	}
	installation.VersionConstraint = ">=1.0.0+" + strings.Repeat("a", 250)
	if err := validator.ValidateInstallation(installation); err != nil {
		t.Fatalf("parser-valid version constraint was rejected: %v", err)
	}
}

func TestWorkspaceV3OperationsDeclareSecurityTraceAndExactErrors(t *testing.T) {
	document := loadOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v3.yaml"))
	tests := []struct {
		path     string
		method   string
		success  int
		failures map[int][]string
	}{
		{
			path: "/v3/workspaces", method: "POST", success: 201,
			failures: map[int][]string{400: {"VALIDATION_ERROR"}, 401: {"UNAUTHENTICATED"}, 409: {"CONFLICT"}, 503: {"DEPENDENCY_ERROR"}},
		},
		{
			path: "/v3/workspaces/{workspaceId}", method: "GET", success: 200,
			failures: workspaceReadFailures(),
		},
		{
			path: "/v3/workspaces/{workspaceId}/installations", method: "POST", success: 201,
			failures: map[int][]string{400: {"VALIDATION_ERROR"}, 401: {"UNAUTHENTICATED"}, 403: {"FORBIDDEN"}, 404: {"NOT_FOUND"}, 409: {"CONFLICT"}, 503: {"DEPENDENCY_ERROR"}},
		},
		{
			path: "/v3/workspaces/{workspaceId}/installations", method: "GET", success: 200,
			failures: workspaceReadFailures(),
		},
		{
			path: "/v3/workspaces/{workspaceId}/installations/{installationId}", method: "GET", success: 200,
			failures: workspaceReadFailures(),
		},
		{
			path: "/v3/workspaces/{workspaceId}/installations/{installationId}", method: "PATCH", success: 200,
			failures: workspaceMutationFailures(),
		},
		{
			path: "/v3/workspaces/{workspaceId}/installations/{installationId}", method: "DELETE", success: 200,
			failures: workspaceMutationFailures(),
		},
	}

	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			operation := findOperation(t, document, test.path, test.method)
			if operation.Security == nil || len(*operation.Security) != 1 {
				t.Fatal("Bearer security requirement is missing")
			}
			if _, exists := (*operation.Security)[0]["bearerAuth"]; !exists {
				t.Fatalf("security = %#v, want bearerAuth", (*operation.Security)[0])
			}
			if operation.Responses.Len() != len(test.failures)+1 {
				t.Fatalf("response count = %d, want %d", operation.Responses.Len(), len(test.failures)+1)
			}
			assertTraceHeader(t, operation, test.success)
			for status, codes := range test.failures {
				assertExactResponseErrorCodes(t, operation, status, codes)
				assertTraceHeader(t, operation, status)
			}
		})
	}
}

func TestResolveAgentResponsePreservesExactRequestIdentity(t *testing.T) {
	validator := mustValidator(t)
	card := validAgentCard()
	request := ResolveAgentRequest{InvocationID: "inv-resolve", RootTaskID: "task-resolve", TraceID: "trace-resolve", WorkspaceID: "workspace-resolve", AgentID: card.AgentID, Version: card.Version, Capability: "contract.review"}
	response := ResolveAgentResponse{Card: card, Installation: ResolvedInstallation{InstallationID: "installation-resolve", WorkspaceID: request.WorkspaceID, AgentID: request.AgentID, InstalledVersion: request.Version, AcceptedPermissions: []string{"document.read"}, Status: "enabled"}}
	if err := validator.ValidateResolveAgentResponseForRequest(request, response); err != nil {
		t.Fatalf("valid exact resolution response rejected: %v", err)
	}
	response.Card.Version = "2.0.0"
	if err := validator.ValidateResolveAgentResponseForRequest(request, response); err == nil {
		t.Fatal("resolution response with mismatched Card version was accepted")
	}
}

func TestWorkspaceV3GoMappings(t *testing.T) {
	document := loadOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v3.yaml"))
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	workspace := Workspace{WorkspaceID: "workspace-1", OwnerID: "owner-1", CreatedAt: now, UpdatedAt: now}
	installation := validInstallation()
	uninstalled := installation
	uninstalled.Status = "uninstalled"
	uninstalled.UpdatedAt = now
	uninstalled.UninstalledAt = &now

	create := findOperation(t, document, "/v3/workspaces", "POST")
	validateOpenAPIValue(t, create.RequestBody.Value.Content["application/json"].Schema, CreateWorkspaceRequest{WorkspaceID: workspace.WorkspaceID})
	validateOpenAPIValue(t, create.Responses.Status(201).Value.Content["application/json"].Schema, workspace)

	read := findOperation(t, document, "/v3/workspaces/{workspaceId}", "GET")
	validateOpenAPIValue(t, read.Responses.Status(200).Value.Content["application/json"].Schema, workspace)

	collection := document.Paths.Find("/v3/workspaces/{workspaceId}/installations")
	if collection.Get.Parameters.GetByInAndName("query", "limit") == nil || collection.Get.Parameters.GetByInAndName("query", "cursor") == nil {
		t.Fatal("Installation list limit/cursor parameters are missing")
	}
	if !collection.Get.Parameters.GetByInAndName("query", "limit").Required {
		t.Fatal("Installation list limit must be required")
	}
	validateOpenAPIValue(t, collection.Post.RequestBody.Value.Content["application/json"].Schema, InstallAgentRequest{
		AgentID:             installation.AgentID,
		VersionConstraint:   installation.VersionConstraint,
		AcceptedPermissions: installation.AcceptedPermissions,
	})
	validateOpenAPIValue(t, collection.Post.Responses.Status(201).Value.Content["application/json"].Schema, installation)
	cursor := "opaque-cursor"
	validateOpenAPIValue(t, collection.Get.Responses.Status(200).Value.Content["application/json"].Schema, InstallationList{Items: []Installation{installation, uninstalled}, NextCursor: &cursor})
	validateOpenAPIValue(t, collection.Get.Responses.Status(200).Value.Content["application/json"].Schema, InstallationList{Items: []Installation{}})

	item := document.Paths.Find("/v3/workspaces/{workspaceId}/installations/{installationId}")
	validateOpenAPIValue(t, item.Get.Responses.Status(200).Value.Content["application/json"].Schema, uninstalled)
	validateOpenAPIValue(t, item.Patch.RequestBody.Value.Content["application/json"].Schema, UpdateInstallationRequest{Status: "disabled"})
	validateOpenAPIValue(t, item.Patch.Responses.Status(200).Value.Content["application/json"].Schema, installation)
	validateOpenAPIValue(t, item.Delete.Responses.Status(200).Value.Content["application/json"].Schema, uninstalled)
}

func TestControlPlaneInternalResolutionDeclaresTrustedIdentityAndTrace(t *testing.T) {
	document := loadOpenAPIDocument(t, filepath.Join("openapi", "control-plane-internal.v2.yaml"))
	operation := findOperation(t, document, "/internal/v2/resolve-agent", "POST")
	if operation.Security == nil || len(*operation.Security) != 1 {
		t.Fatal("internal Bearer security requirement is missing")
	}
	if _, exists := (*operation.Security)[0]["internalBearerAuth"]; !exists {
		t.Fatalf("security = %#v, want internalBearerAuth", (*operation.Security)[0])
	}
	for _, status := range []int{200, 400, 401, 403, 404, 503} {
		assertTraceHeader(t, operation, status)
	}
}

func workspaceReadFailures() map[int][]string {
	return map[int][]string{
		400: {"VALIDATION_ERROR"},
		401: {"UNAUTHENTICATED"},
		403: {"FORBIDDEN"},
		404: {"NOT_FOUND"},
		503: {"DEPENDENCY_ERROR"},
	}
}

func workspaceMutationFailures() map[int][]string {
	failures := workspaceReadFailures()
	failures[409] = []string{"CONFLICT"}
	return failures
}

func findOperation(t *testing.T, document *openapi3.T, path, method string) *openapi3.Operation {
	t.Helper()
	item := document.Paths.Find(path)
	if item == nil {
		t.Fatalf("path %s is missing", path)
	}
	var operation *openapi3.Operation
	switch method {
	case "GET":
		operation = item.Get
	case "POST":
		operation = item.Post
	case "PATCH":
		operation = item.Patch
	case "DELETE":
		operation = item.Delete
	default:
		t.Fatalf("unsupported test method %s", method)
	}
	if operation == nil {
		t.Fatalf("%s %s is missing", method, path)
	}
	return operation
}

func assertTraceHeader(t *testing.T, operation *openapi3.Operation, status int) {
	t.Helper()
	response := operation.Responses.Status(status)
	if response == nil || response.Value == nil {
		t.Fatalf("response %d is missing", status)
	}
	if response.Value.Headers["x-nek-trace-id"] == nil {
		t.Fatalf("response %d trace header is missing", status)
	}
}
