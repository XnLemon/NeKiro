//go:build integration

package workspace_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	catalogpostgres "github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog/postgres"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/config"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/gateway"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace"
	workspacepostgres "github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace/postgres"
	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	acceptanceOwnerToken    = "issue-9-owner-token"
	acceptanceOtherToken    = "issue-9-other-token"
	acceptanceInternalToken = "issue-9-internal-token"
)

type acceptanceHTTPHarness struct {
	handler          http.Handler
	catalog          *catalog.Service
	workspace        *workspace.Service
	pool             *pgxpool.Pool
	agentEndpoint    string
	ownerToken       string
	otherToken       string
	internalToken    string
	agentCallCounter *atomic.Int64
}

type acceptanceReadiness struct {
	catalog   gateway.ReadinessChecker
	workspace gateway.ReadinessChecker
}

func (readiness acceptanceReadiness) Check(ctx context.Context) error {
	if err := readiness.catalog.Check(ctx); err != nil {
		return err
	}
	return readiness.workspace.Check(ctx)
}

func newAcceptanceHTTPHarness(t *testing.T) *acceptanceHTTPHarness {
	t.Helper()
	ctx := context.Background()
	databaseURL := integrationDatabaseURL(t)
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect acceptance database: %v", err)
	}
	if _, err := connection.Exec(ctx, `DROP SCHEMA IF EXISTS workspace CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `DROP SCHEMA IF EXISTS catalog CASCADE`); err != nil {
		t.Fatal(err)
	}
	if err := catalogpostgres.Migrate(ctx, connection, "up"); err != nil {
		t.Fatalf("migrate Catalog schema: %v", err)
	}
	if err := workspacepostgres.Migrate(ctx, connection, "up"); err != nil {
		t.Fatalf("migrate Workspace schema: %v", err)
	}
	if err := connection.Close(ctx); err != nil {
		t.Fatalf("close migration connection: %v", err)
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open acceptance pool: %v", err)
	}
	t.Cleanup(pool.Close)
	catalogStore, err := catalogpostgres.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	workspaceStore, err := workspacepostgres.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	catalogService, err := catalog.NewService(catalogStore, validator, testClock)
	if err != nil {
		t.Fatal(err)
	}
	workspaceService, err := workspace.NewService(workspaceStore, catalogService, workspace.OwnerPolicy{}, validator, testClock, workspace.NewRandomID)
	if err != nil {
		t.Fatal(err)
	}

	var agentCallCounter atomic.Int64
	agentServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		agentCallCounter.Add(1)
	}))
	t.Cleanup(agentServer.Close)
	card := integrationCard()
	card.Protocol.Endpoint = agentServer.URL
	if err := registerLegacyPublishedCard(ctx, pool, catalogService, card); err != nil {
		t.Fatalf("publish acceptance fixture Card: %v", err)
	}

	publicAuth, err := gateway.NewDevelopmentStaticAuthenticator([]config.StaticPrincipal{
		{ID: "owner-a", TokenSHA256: acceptanceTokenDigest(acceptanceOwnerToken)},
		{ID: "owner-b", TokenSHA256: acceptanceTokenDigest(acceptanceOtherToken)},
	})
	if err != nil {
		t.Fatal(err)
	}
	internalAuth, err := gateway.NewDevelopmentStaticAuthenticator([]config.StaticPrincipal{
		{ID: "router-a", TokenSHA256: acceptanceTokenDigest(acceptanceInternalToken)},
	})
	if err != nil {
		t.Fatal(err)
	}
	traces, err := gateway.NewTraceGenerator()
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	catalogHandler, err := gateway.NewHandler(publicAuth, catalogService, acceptanceReadiness{catalog: catalogStore, workspace: workspaceStore}, traces, logger)
	if err != nil {
		t.Fatal(err)
	}
	workspaceHandler, err := gateway.NewWorkspaceHandler(publicAuth, internalAuth, workspaceService, traces, logger, 1048576)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	catalogHandler.RegisterRoutes(mux)
	workspaceHandler.RegisterRoutes(mux)

	return &acceptanceHTTPHarness{
		handler: mux, catalog: catalogService, workspace: workspaceService,
		pool: pool, agentEndpoint: agentServer.URL,
		ownerToken: acceptanceOwnerToken, otherToken: acceptanceOtherToken,
		internalToken: acceptanceInternalToken, agentCallCounter: &agentCallCounter,
	}
}

func testClock() time.Time {
	return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
}

func acceptanceTokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func (harness *acceptanceHTTPHarness) request(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	authorization := ""
	if token != "" {
		authorization = "Bearer " + token
	}
	return harness.requestWithContext(t, context.Background(), method, path, authorization, body)
}

func (harness *acceptanceHTTPHarness) requestWithAuthorization(t *testing.T, method, path, authorization string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return harness.requestWithContext(t, context.Background(), method, path, authorization, body)
}

func (harness *acceptanceHTTPHarness) requestWithContext(t *testing.T, ctx context.Context, method, path, authorization string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader = http.NoBody
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, reader)
	request = request.WithContext(ctx)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)
	return response
}

func decodeAcceptanceJSON(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatalf("decode response body %q: %v", response.Body.String(), err)
	}
}

func requireAcceptanceTrace(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	traceID := response.Header().Get(gateway.TraceHeader)
	if traceID == "" {
		t.Fatalf("response status=%d has no %s header", response.Code, gateway.TraceHeader)
	}
	return traceID
}

func requireAcceptanceError(t *testing.T, harness *acceptanceHTTPHarness, response *httptest.ResponseRecorder, status int, code contracts.PlatformErrorCode) contracts.PlatformErrorV3 {
	t.Helper()
	if response.Code != status {
		t.Fatalf("error status=%d, want %d body=%s", response.Code, status, response.Body.String())
	}
	rawBody := append([]byte(nil), response.Body.Bytes()...)
	var payload contracts.PlatformErrorV3
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		t.Fatalf("decode error response body %q: %v", string(rawBody), err)
	}
	if payload.Code != code {
		t.Fatalf("error code=%q, want %q body=%s", payload.Code, code, response.Body.String())
	}
	traceID := requireAcceptanceTrace(t, response)
	if payload.TraceID == "" || string(payload.TraceID) != string(traceID) {
		t.Fatalf("error trace payload=%#v header=%q", payload, traceID)
	}
	body := strings.ToLower(string(rawBody))
	for _, secret := range []string{harness.ownerToken, harness.otherToken, harness.internalToken, "postgres://"} {
		if strings.Contains(body, strings.ToLower(secret)) {
			t.Fatalf("error response leaked secret %q: %s", secret, response.Body.String())
		}
	}
	return payload
}

func TestAcceptanceWorkspaceControlPlaneHTTPWorkflow(t *testing.T) {
	harness := newAcceptanceHTTPHarness(t)

	searchResponse := harness.request(t, http.MethodGet, "/v3/agents?capability=document.read", harness.ownerToken, nil)
	if searchResponse.Code != http.StatusOK {
		t.Fatalf("discover status=%d body=%s", searchResponse.Code, searchResponse.Body.String())
	}
	requireAcceptanceTrace(t, searchResponse)
	var search contracts.SearchAgentsResponse
	decodeAcceptanceJSON(t, searchResponse, &search)
	if len(search.Items) != 1 || search.Items[0].Card.AgentID != "runtime-a" || search.Items[0].Card.Version != "1.0.0" || search.Items[0].PublicationStatus != "published" {
		t.Fatalf("discover response = %#v", search)
	}

	createResponse := harness.request(t, http.MethodPost, "/v3/workspaces", harness.ownerToken, contracts.CreateWorkspaceRequest{WorkspaceID: "acceptance-workspace"})
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	requireAcceptanceTrace(t, createResponse)
	var created contracts.Workspace
	decodeAcceptanceJSON(t, createResponse, &created)
	if created.WorkspaceID != "acceptance-workspace" || created.OwnerID != "owner-a" {
		t.Fatalf("created Workspace = %#v", created)
	}

	installResponse := harness.request(t, http.MethodPost, "/v3/workspaces/acceptance-workspace/installations", harness.ownerToken, contracts.InstallAgentRequest{
		AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
	})
	if installResponse.Code != http.StatusCreated {
		t.Fatalf("install status=%d body=%s", installResponse.Code, installResponse.Body.String())
	}
	requireAcceptanceTrace(t, installResponse)
	var installed contracts.Installation
	decodeAcceptanceJSON(t, installResponse, &installed)
	if installed.AgentID != "runtime-a" || installed.InstalledVersion != "1.0.0" || installed.Status != "enabled" || len(installed.AcceptedPermissions) != 1 || installed.AcceptedPermissions[0] != "document.read" {
		t.Fatalf("installed = %#v", installed)
	}

	listPath := "/v3/workspaces/acceptance-workspace/installations?limit=25"
	listResponse := harness.request(t, http.MethodGet, listPath, harness.ownerToken, nil)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	requireAcceptanceTrace(t, listResponse)
	var listed contracts.InstallationList
	decodeAcceptanceJSON(t, listResponse, &listed)
	if len(listed.Items) != 1 || !sameInstallation(listed.Items[0], installed) {
		t.Fatalf("listed = %#v, installed = %#v", listed, installed)
	}

	detailResponse := harness.request(t, http.MethodGet, "/v3/workspaces/acceptance-workspace/installations/"+installed.InstallationID, harness.ownerToken, nil)
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailResponse.Code, detailResponse.Body.String())
	}
	requireAcceptanceTrace(t, detailResponse)
	var detailed contracts.Installation
	decodeAcceptanceJSON(t, detailResponse, &detailed)
	if !sameInstallation(detailed, installed) {
		t.Fatalf("detailed = %#v, installed = %#v", detailed, installed)
	}

	for _, agentID := range []string{"runtime-b", "runtime-c"} {
		card := integrationCard()
		card.AgentID = agentID
		card.Name = "Acceptance " + agentID
		card.Protocol.Endpoint = harness.agentEndpoint
		if err := registerLegacyPublishedCard(context.Background(), harness.pool, harness.catalog, card); err != nil {
			t.Fatalf("publish %s: %v", agentID, err)
		}
		response := harness.request(t, http.MethodPost, "/v3/workspaces/acceptance-workspace/installations", harness.ownerToken, contracts.InstallAgentRequest{
			AgentID: agentID, VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
		})
		if response.Code != http.StatusCreated {
			t.Fatalf("install %s status=%d body=%s", agentID, response.Code, response.Body.String())
		}
		requireAcceptanceTrace(t, response)
	}

	seen := make(map[string]struct{})
	cursor := ""
	for {
		path := "/v3/workspaces/acceptance-workspace/installations?limit=1"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		response := harness.request(t, http.MethodGet, path, harness.ownerToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("paged list status=%d body=%s", response.Code, response.Body.String())
		}
		requireAcceptanceTrace(t, response)
		var page contracts.InstallationList
		decodeAcceptanceJSON(t, response, &page)
		if len(page.Items) > 1 {
			t.Fatalf("paged list returned %d items for limit=1", len(page.Items))
		}
		for _, item := range page.Items {
			if _, exists := seen[item.InstallationID]; exists {
				t.Fatalf("paged list repeated Installation %s", item.InstallationID)
			}
			seen[item.InstallationID] = struct{}{}
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatalf("paged list returned %d unique Installations, want 3", len(seen))
	}

	disabledResponse := harness.request(t, http.MethodPatch, "/v3/workspaces/acceptance-workspace/installations/"+installed.InstallationID, harness.ownerToken, contracts.UpdateInstallationRequest{Status: "disabled"})
	if disabledResponse.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabledResponse.Code, disabledResponse.Body.String())
	}
	requireAcceptanceTrace(t, disabledResponse)
	var disabled contracts.Installation
	decodeAcceptanceJSON(t, disabledResponse, &disabled)
	if disabled.Status != "disabled" || !sameInstallationImmutable(disabled, installed) {
		t.Fatalf("disabled = %#v, installed = %#v", disabled, installed)
	}

	enabledResponse := harness.request(t, http.MethodPatch, "/v3/workspaces/acceptance-workspace/installations/"+installed.InstallationID, harness.ownerToken, contracts.UpdateInstallationRequest{Status: "enabled"})
	if enabledResponse.Code != http.StatusOK {
		t.Fatalf("enable status=%d body=%s", enabledResponse.Code, enabledResponse.Body.String())
	}
	requireAcceptanceTrace(t, enabledResponse)
	var enabled contracts.Installation
	decodeAcceptanceJSON(t, enabledResponse, &enabled)
	if enabled.Status != "enabled" || !sameInstallationImmutable(enabled, installed) || !enabled.UpdatedAt.After(disabled.UpdatedAt) {
		t.Fatalf("enabled = %#v, disabled = %#v", enabled, disabled)
	}

	resolveRequest := contracts.ResolveAgentRequest{
		InvocationID: "invocation-acceptance", RootTaskID: "root-task-acceptance", TraceID: "trace-acceptance",
		WorkspaceID: "acceptance-workspace", AgentID: "runtime-a", Version: "1.0.0", Capability: "document.read",
	}
	resolveResponse := harness.request(t, http.MethodPost, "/internal/v2/resolve-agent", harness.internalToken, resolveRequest)
	if resolveResponse.Code != http.StatusOK {
		t.Fatalf("resolve status=%d body=%s", resolveResponse.Code, resolveResponse.Body.String())
	}
	if got := requireAcceptanceTrace(t, resolveResponse); got != string(resolveRequest.TraceID) {
		t.Fatalf("resolve response trace=%q, want %q", got, resolveRequest.TraceID)
	}
	var resolved contracts.ResolveAgentResponse
	decodeAcceptanceJSON(t, resolveResponse, &resolved)
	if resolved.Card.AgentID != "runtime-a" || resolved.Card.Version != "1.0.0" || resolved.Installation.InstallationID != installed.InstallationID || resolved.Installation.Status != "enabled" {
		t.Fatalf("resolved = %#v", resolved)
	}

	disabledAgainResponse := harness.request(t, http.MethodPatch, "/v3/workspaces/acceptance-workspace/installations/"+installed.InstallationID, harness.ownerToken, contracts.UpdateInstallationRequest{Status: "disabled"})
	if disabledAgainResponse.Code != http.StatusOK {
		t.Fatalf("disable before uninstall status=%d body=%s", disabledAgainResponse.Code, disabledAgainResponse.Body.String())
	}
	requireAcceptanceTrace(t, disabledAgainResponse)
	var disabledAgain contracts.Installation
	decodeAcceptanceJSON(t, disabledAgainResponse, &disabledAgain)
	uninstallResponse := harness.request(t, http.MethodDelete, "/v3/workspaces/acceptance-workspace/installations/"+installed.InstallationID, harness.ownerToken, nil)
	if uninstallResponse.Code != http.StatusOK {
		t.Fatalf("uninstall status=%d body=%s", uninstallResponse.Code, uninstallResponse.Body.String())
	}
	requireAcceptanceTrace(t, uninstallResponse)
	var terminal contracts.Installation
	decodeAcceptanceJSON(t, uninstallResponse, &terminal)
	if terminal.Status != "uninstalled" || terminal.UninstalledAt == nil || !terminal.UninstalledAt.Equal(terminal.UpdatedAt) || !sameInstallationImmutable(terminal, installed) {
		t.Fatalf("terminal = %#v, installed = %#v", terminal, installed)
	}

	reinstallResponse := harness.request(t, http.MethodPost, "/v3/workspaces/acceptance-workspace/installations", harness.ownerToken, contracts.InstallAgentRequest{
		AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
	})
	if reinstallResponse.Code != http.StatusCreated {
		t.Fatalf("reinstall status=%d body=%s", reinstallResponse.Code, reinstallResponse.Body.String())
	}
	requireAcceptanceTrace(t, reinstallResponse)
	var reinstalled contracts.Installation
	decodeAcceptanceJSON(t, reinstallResponse, &reinstalled)
	if reinstalled.InstallationID == installed.InstallationID || reinstalled.Status != "enabled" || !sameInstallationPin(reinstalled, installed) {
		t.Fatalf("reinstalled = %#v, installed = %#v", reinstalled, installed)
	}

	terminalDetail := harness.request(t, http.MethodGet, "/v3/workspaces/acceptance-workspace/installations/"+installed.InstallationID, harness.ownerToken, nil)
	if terminalDetail.Code != http.StatusOK {
		t.Fatalf("terminal detail status=%d body=%s", terminalDetail.Code, terminalDetail.Body.String())
	}
	requireAcceptanceTrace(t, terminalDetail)
	var terminalAfterReinstall contracts.Installation
	decodeAcceptanceJSON(t, terminalDetail, &terminalAfterReinstall)
	if !sameInstallation(terminalAfterReinstall, terminal) {
		t.Fatalf("terminal changed after reinstall = %#v, before = %#v", terminalAfterReinstall, terminal)
	}
	if calls := harness.agentCallCounter.Load(); calls != 0 {
		t.Fatalf("acceptance workflow contacted Agent endpoint %d times", calls)
	}
}

func TestAcceptanceHTTPFailureBoundaries(t *testing.T) {
	harness := newAcceptanceHTTPHarness(t)
	createResponse := harness.request(t, http.MethodPost, "/v3/workspaces", harness.ownerToken, contracts.CreateWorkspaceRequest{WorkspaceID: "acceptance-errors"})
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create error fixture status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	requireAcceptanceTrace(t, createResponse)
	installResponse := harness.request(t, http.MethodPost, "/v3/workspaces/acceptance-errors/installations", harness.ownerToken, contracts.InstallAgentRequest{
		AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
	})
	if installResponse.Code != http.StatusCreated {
		t.Fatalf("install error fixture status=%d body=%s", installResponse.Code, installResponse.Body.String())
	}
	requireAcceptanceTrace(t, installResponse)
	var installed contracts.Installation
	decodeAcceptanceJSON(t, installResponse, &installed)
	otherWorkspaceResponse := harness.request(t, http.MethodPost, "/v3/workspaces", harness.ownerToken, contracts.CreateWorkspaceRequest{WorkspaceID: "acceptance-other"})
	if otherWorkspaceResponse.Code != http.StatusCreated {
		t.Fatalf("create wrong-workspace fixture status=%d body=%s", otherWorkspaceResponse.Code, otherWorkspaceResponse.Body.String())
	}
	requireAcceptanceTrace(t, otherWorkspaceResponse)

	requireAcceptanceError(t, harness, harness.request(t, http.MethodGet, "/v3/workspaces/acceptance-errors", "", nil), http.StatusUnauthorized, contracts.ErrorCodeUnauthenticated)
	requireAcceptanceError(t, harness, harness.requestWithAuthorization(t, http.MethodGet, "/v3/workspaces/acceptance-errors", "Basic "+harness.ownerToken, nil), http.StatusUnauthorized, contracts.ErrorCodeUnauthenticated)
	requireAcceptanceError(t, harness, harness.request(t, http.MethodGet, "/v3/workspaces/acceptance-errors/installations/"+installed.InstallationID, harness.otherToken, nil), http.StatusForbidden, contracts.ErrorCodeForbidden)
	requireAcceptanceError(t, harness, harness.request(t, http.MethodPatch, "/v3/workspaces/acceptance-errors/installations/"+installed.InstallationID, harness.otherToken, contracts.UpdateInstallationRequest{Status: "disabled"}), http.StatusForbidden, contracts.ErrorCodeForbidden)
	requireAcceptanceError(t, harness, harness.request(t, http.MethodGet, "/v3/workspaces/missing-workspace", harness.ownerToken, nil), http.StatusNotFound, contracts.ErrorCodeNotFound)
	requireAcceptanceError(t, harness, harness.request(t, http.MethodGet, "/v3/workspaces/acceptance-other/installations/"+installed.InstallationID, harness.ownerToken, nil), http.StatusNotFound, contracts.ErrorCodeNotFound)
	requireAcceptanceError(t, harness, harness.request(t, http.MethodPatch, "/v3/workspaces/acceptance-errors/installations/"+installed.InstallationID, harness.ownerToken, map[string]any{"status": "disabled", "unexpected": true}), http.StatusBadRequest, contracts.ErrorCodeValidationError)
	requireAcceptanceError(t, harness, harness.request(t, http.MethodPost, "/v3/workspaces/acceptance-errors/installations", harness.ownerToken, map[string]any{"agentId": "runtime-a", "versionConstraint": "^1.0.0"}), http.StatusBadRequest, contracts.ErrorCodeValidationError)
	requireAcceptanceError(t, harness, harness.request(t, http.MethodPatch, "/v3/workspaces/acceptance-errors/installations/"+installed.InstallationID, harness.ownerToken, contracts.UpdateInstallationRequest{Status: "enabled"}), http.StatusConflict, contracts.ErrorCodeConflict)

	unchangedResponse := harness.request(t, http.MethodGet, "/v3/workspaces/acceptance-errors/installations/"+installed.InstallationID, harness.ownerToken, nil)
	if unchangedResponse.Code != http.StatusOK {
		t.Fatalf("read Installation after rejected requests status=%d body=%s", unchangedResponse.Code, unchangedResponse.Body.String())
	}
	requireAcceptanceTrace(t, unchangedResponse)
	var unchanged contracts.Installation
	decodeAcceptanceJSON(t, unchangedResponse, &unchanged)
	if !sameInstallation(unchanged, installed) {
		t.Fatalf("rejected requests mutated Installation = %#v, before = %#v", unchanged, installed)
	}

	resolveRequest := contracts.ResolveAgentRequest{
		InvocationID: "invocation-errors", RootTaskID: "root-task-errors", TraceID: "trace-errors",
		WorkspaceID: "acceptance-errors", AgentID: "runtime-a", Version: "1.0.0", Capability: "document.read",
	}
	requireAcceptanceError(t, harness, harness.request(t, http.MethodPost, "/internal/v2/resolve-agent", "", resolveRequest), http.StatusUnauthorized, contracts.ErrorCodeUnauthenticated)

	disabledResponse := harness.request(t, http.MethodPatch, "/v3/workspaces/acceptance-errors/installations/"+installed.InstallationID, harness.ownerToken, contracts.UpdateInstallationRequest{Status: "disabled"})
	if disabledResponse.Code != http.StatusOK {
		t.Fatalf("disable error fixture status=%d body=%s", disabledResponse.Code, disabledResponse.Body.String())
	}
	requireAcceptanceTrace(t, disabledResponse)
	disabledError := requireAcceptanceError(t, harness, harness.request(t, http.MethodPost, "/internal/v2/resolve-agent", harness.internalToken, resolveRequest), http.StatusForbidden, contracts.ErrorCodeInstallationDisabled)
	if disabledError.InvocationID != resolveRequest.InvocationID || disabledError.RootTaskID != resolveRequest.RootTaskID || disabledError.TraceID != resolveRequest.TraceID {
		t.Fatalf("disabled correlated error = %#v", disabledError)
	}

	enabledResponse := harness.request(t, http.MethodPatch, "/v3/workspaces/acceptance-errors/installations/"+installed.InstallationID, harness.ownerToken, contracts.UpdateInstallationRequest{Status: "enabled"})
	if enabledResponse.Code != http.StatusOK {
		t.Fatalf("enable error fixture status=%d body=%s", enabledResponse.Code, enabledResponse.Body.String())
	}
	requireAcceptanceTrace(t, enabledResponse)
	unknownCapability := resolveRequest
	unknownCapability.Capability = "document.write"
	requireAcceptanceError(t, harness, harness.request(t, http.MethodPost, "/internal/v2/resolve-agent", harness.internalToken, unknownCapability), http.StatusForbidden, contracts.ErrorCodeCapabilityNotAllowed)

	catalogDisableResponse := harness.request(t, http.MethodPost, "/v3/agents/runtime-a/versions/1.0.0/disable", harness.ownerToken, nil)
	if catalogDisableResponse.Code != http.StatusOK {
		t.Fatalf("Catalog disable status=%d body=%s", catalogDisableResponse.Code, catalogDisableResponse.Body.String())
	}
	requireAcceptanceTrace(t, catalogDisableResponse)
	requireAcceptanceError(t, harness, harness.request(t, http.MethodPost, "/internal/v2/resolve-agent", harness.internalToken, resolveRequest), http.StatusForbidden, contracts.ErrorCodeAgentDisabled)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	requireAcceptanceError(t, harness, harness.requestWithContext(t, canceled, http.MethodGet, "/v3/workspaces/acceptance-errors", "Bearer "+harness.ownerToken, nil), http.StatusServiceUnavailable, contracts.ErrorCodeDependency)
	if _, err := harness.workspace.GetWorkspace(canceled, workspace.AuthenticatedCaller{ID: "owner-a"}, "acceptance-errors"); !errors.Is(err, workspace.ErrDependency) {
		t.Fatalf("canceled acceptance dependency = %v, want dependency", err)
	}
	if calls := harness.agentCallCounter.Load(); calls != 0 {
		t.Fatalf("failure acceptance contacted Agent endpoint %d times", calls)
	}

	if _, err := harness.pool.Exec(context.Background(), `ALTER SCHEMA workspace RENAME TO workspace_unavailable`); err != nil {
		t.Fatalf("degrade Workspace schema: %v", err)
	}
	schemaFailure := requireAcceptanceError(t, harness, harness.request(t, http.MethodGet, "/v3/workspaces/acceptance-errors", harness.ownerToken, nil), http.StatusServiceUnavailable, contracts.ErrorCodeDependency)
	if schemaFailure.Code != contracts.ErrorCodeDependency {
		t.Fatalf("schema failure code = %q", schemaFailure.Code)
	}
	if _, err := harness.pool.Exec(context.Background(), `ALTER SCHEMA workspace_unavailable RENAME TO workspace`); err != nil {
		t.Fatalf("restore Workspace schema: %v", err)
	}

	if _, err := harness.pool.Exec(context.Background(), `
CREATE OR REPLACE FUNCTION workspace.issue_9_reject_workspace_insert()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'issue-9 transaction failure';
END;
$$`); err != nil {
		t.Fatalf("create transaction failure trigger function: %v", err)
	}
	if _, err := harness.pool.Exec(context.Background(), `
CREATE TRIGGER issue_9_reject_workspace_insert
BEFORE INSERT ON workspace.workspaces
FOR EACH ROW EXECUTE FUNCTION workspace.issue_9_reject_workspace_insert()`); err != nil {
		t.Fatalf("create transaction failure trigger: %v", err)
	}
	transactionFailure := requireAcceptanceError(t, harness, harness.request(t, http.MethodPost, "/v3/workspaces", harness.ownerToken, contracts.CreateWorkspaceRequest{WorkspaceID: "acceptance-transaction-failure"}), http.StatusServiceUnavailable, contracts.ErrorCodeDependency)
	if transactionFailure.Code != contracts.ErrorCodeDependency {
		t.Fatalf("transaction failure code = %q", transactionFailure.Code)
	}
	if _, err := harness.pool.Exec(context.Background(), `DROP TRIGGER issue_9_reject_workspace_insert ON workspace.workspaces`); err != nil {
		t.Fatalf("drop transaction failure trigger: %v", err)
	}
	if _, err := harness.pool.Exec(context.Background(), `DROP FUNCTION workspace.issue_9_reject_workspace_insert()`); err != nil {
		t.Fatalf("drop transaction failure trigger function: %v", err)
	}
}
