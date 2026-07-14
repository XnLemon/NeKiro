//go:build integration

package workspace_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	catalogpostgres "github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog/postgres"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace"
	workspacepostgres "github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace/postgres"
	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWorkspaceInstallationPersistenceLifecycleAndResolution(t *testing.T) {
	ctx := context.Background()
	catalogService, workspaceService := integrationServices(t, ctx)
	owner := workspace.AuthenticatedCaller{ID: "owner-a"}
	if _, err := workspaceService.CreateWorkspace(ctx, owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-integration"}); err != nil {
		t.Fatalf("create Workspace: %v", err)
	}
	installation, err := workspaceService.Install(ctx, owner, "workspace-integration", contracts.InstallAgentRequest{
		AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
	})
	if err != nil {
		t.Fatalf("install Agent: %v", err)
	}
	if installation.InstalledVersion != "1.0.0" || installation.Status != "enabled" {
		t.Fatalf("installation = %#v", installation)
	}
	resolved, err := workspaceService.Resolve(ctx, contracts.ResolveAgentRequest{
		InvocationID: "invocation-integration", RootTaskID: "task-integration", TraceID: "trace-integration",
		WorkspaceID: "workspace-integration", AgentID: "runtime-a", Version: "1.0.0", Capability: "document.read",
	})
	if err != nil || resolved.Card.Version != "1.0.0" || resolved.Installation.InstallationID != installation.InstallationID {
		t.Fatalf("exact resolution = %#v, %v", resolved, err)
	}
	if _, err := workspaceService.UpdateInstallation(ctx, owner, "workspace-integration", installation.InstallationID, "disabled"); err != nil {
		t.Fatalf("disable Installation: %v", err)
	}
	if _, err := workspaceService.Uninstall(ctx, owner, "workspace-integration", installation.InstallationID); err != nil {
		t.Fatalf("uninstall Installation: %v", err)
	}
	listed, err := workspaceService.ListInstallations(ctx, owner, "workspace-integration", 1, nil)
	if err != nil || len(listed.Items) != 1 || listed.Items[0].Status != "uninstalled" {
		t.Fatalf("historical Installation list = %#v, %v", listed, err)
	}
	second, err := workspaceService.Install(ctx, owner, "workspace-integration", contracts.InstallAgentRequest{AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"}})
	if err != nil {
		t.Fatalf("reinstall Agent: %v", err)
	}
	if _, err := catalogService.Disable(ctx, catalog.AuthenticatedCaller{ID: "owner-a"}, "runtime-a", "1.0.0"); err != nil {
		t.Fatalf("disable pinned Catalog version: %v", err)
	}
	if _, err := workspaceService.Resolve(ctx, contracts.ResolveAgentRequest{
		InvocationID: "invocation-after-disable", RootTaskID: "task-after-disable", TraceID: "trace-after-disable",
		WorkspaceID: "workspace-integration", AgentID: "runtime-a", Version: second.InstalledVersion, Capability: "document.read",
	}); !errors.Is(err, workspace.ErrAgentDisabled) {
		t.Fatalf("resolution after Catalog disable = %v", err)
	}
	current, err := workspaceService.GetInstallation(ctx, owner, "workspace-integration", installation.InstallationID)
	if err != nil || current.Status != "uninstalled" {
		t.Fatalf("historical Installation changed = %#v, %v", current, err)
	}

}

func TestConcurrentInstallLeavesOneCurrentInstallation(t *testing.T) {
	ctx := context.Background()
	_, workspaceService := integrationServices(t, ctx)
	owner := workspace.AuthenticatedCaller{ID: "owner-a"}
	if _, err := workspaceService.CreateWorkspace(ctx, owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-race"}); err != nil {
		t.Fatalf("create Workspace: %v", err)
	}
	var wait sync.WaitGroup
	results := make(chan error, 100)
	for index := 0; index < 100; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := workspaceService.Install(ctx, owner, "workspace-race", contracts.InstallAgentRequest{AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"}})
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	successes := 0
	conflicts := 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, workspace.ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("concurrent install error: %v", err)
		}
	}
	if successes != 1 || conflicts != 99 {
		t.Fatalf("concurrent installs successes=%d conflicts=%d", successes, conflicts)
	}
	listed, err := workspaceService.ListInstallations(ctx, owner, "workspace-race", 100, nil)
	if err != nil || len(listed.Items) != 1 {
		t.Fatalf("current Installation count = %#v, %v", listed, err)
	}
}

func integrationServices(t *testing.T, ctx context.Context) (*catalog.Service, *workspace.Service) {
	t.Helper()
	databaseURL := os.Getenv("NEKIRO_TEST_DATABASE_URL")
	if strings.TrimSpace(databaseURL) == "" {
		t.Fatal("NEKIRO_TEST_DATABASE_URL is required")
	}
	configuration, err := pgx.ParseConfig(databaseURL)
	if err != nil || !strings.HasSuffix(configuration.Database, "_test") {
		t.Fatal("integration database must end in _test")
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	if _, err := connection.Exec(ctx, `DROP SCHEMA IF EXISTS workspace CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `DROP SCHEMA IF EXISTS catalog CASCADE`); err != nil {
		t.Fatal(err)
	}
	if err := catalogpostgres.Migrate(ctx, connection, "up"); err != nil {
		t.Fatal(err)
	}
	if err := workspacepostgres.Migrate(ctx, connection, "up"); err != nil {
		t.Fatal(err)
	}
	_ = connection.Close(ctx)

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
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
	catalogService, err := catalog.NewService(catalogStore, validator, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	card := integrationCard()
	body, err := json.Marshal(contracts.RegisterAgentRequest{Card: card})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalogService.Register(ctx, catalog.AuthenticatedCaller{ID: "owner-a"}, body); err != nil && !errors.Is(err, catalog.ErrConflict) {
		t.Fatalf("register fixture Card: %v", err)
	}
	if _, err := catalogService.Publish(ctx, catalog.AuthenticatedCaller{ID: "owner-a"}, card.AgentID, card.Version); err != nil && !errors.Is(err, catalog.ErrConflict) {
		t.Fatalf("publish fixture Card: %v", err)
	}
	workspaceService, err := workspace.NewService(workspaceStore, catalogService, workspace.OwnerPolicy{}, validator, time.Now, workspace.NewRandomID)
	if err != nil {
		t.Fatal(err)
	}
	return catalogService, workspaceService
}

func integrationCard() contracts.AgentCard {
	return contracts.AgentCard{SchemaVersion: "0.2", AgentID: "runtime-a", Name: "Runtime A", Description: "Integration fixture", Owner: contracts.AgentOwner{ID: "owner-a", DisplayName: "Owner"}, Version: "1.0.0", Protocol: contracts.AgentProtocol{Type: "a2a", Version: "0.3.0", Transport: "JSONRPC", Endpoint: "https://agent.example.test/a2a"}, Skills: []contracts.AgentSkill{{ID: "document.read", Name: "Read", Description: "Read", InputSchema: contracts.JSONSchema{"type": "object"}, OutputSchema: contracts.JSONSchema{"type": "object"}, RequiredPermissions: []string{"document.read"}}}, Authentication: contracts.AgentAuthentication{Type: "none"}, Permissions: []contracts.PermissionDeclaration{{ID: "document.read", Description: "Read"}}, Limits: contracts.AgentLimits{TimeoutMS: 1000, MaxInputBytes: json.Number("1000"), MaxOutputBytes: json.Number("1000"), Streaming: false}}
}
