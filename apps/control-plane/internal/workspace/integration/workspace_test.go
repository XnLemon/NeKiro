//go:build integration

package workspace_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestConcurrentWorkspaceCreateLeavesOneCommittedRow(t *testing.T) {
	ctx := context.Background()
	_, workspaceService := integrationServices(t, ctx)
	owner := workspace.AuthenticatedCaller{ID: "owner-a"}

	type createResult struct {
		value contracts.Workspace
		err   error
	}
	const callers = 100
	results := make(chan createResult, callers)
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			value, err := workspaceService.CreateWorkspace(ctx, owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-create-race"})
			results <- createResult{value: value, err: err}
		}()
	}
	wait.Wait()
	close(results)

	successes, conflicts := 0, 0
	var created contracts.Workspace
	for result := range results {
		switch {
		case result.err == nil:
			successes++
			created = result.value
		case errors.Is(result.err, workspace.ErrConflict):
			if result.value.WorkspaceID != "" {
				t.Fatalf("conflicting create returned a Workspace: %#v", result.value)
			}
			conflicts++
		default:
			t.Fatalf("concurrent create error: %v", result.err)
		}
	}
	if successes != 1 || conflicts != callers-1 {
		t.Fatalf("concurrent creates successes=%d conflicts=%d", successes, conflicts)
	}
	if created.WorkspaceID != "workspace-create-race" || created.OwnerID != owner.ID {
		t.Fatalf("created Workspace = %#v", created)
	}
	stored, err := workspaceService.GetWorkspace(ctx, owner, created.WorkspaceID)
	if err != nil {
		t.Fatalf("read committed Workspace: %v", err)
	}
	if !sameWorkspace(stored, created) {
		t.Fatalf("stored Workspace = %#v, want %#v", stored, created)
	}
}

func TestWorkspaceCreateReadSurvivesStoreReconstruction(t *testing.T) {
	ctx := context.Background()
	catalogService, workspaceService := integrationServices(t, ctx)
	owner := workspace.AuthenticatedCaller{ID: "owner-a"}
	created, err := workspaceService.CreateWorkspace(ctx, owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-root"})
	if err != nil {
		t.Fatalf("create Workspace: %v", err)
	}
	if _, err := workspaceService.CreateWorkspace(ctx, workspace.AuthenticatedCaller{ID: "owner-b"}, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-root"}); !errors.Is(err, workspace.ErrConflict) {
		t.Fatalf("duplicate Workspace = %v, want conflict", err)
	}

	initial, err := workspaceService.GetWorkspace(ctx, owner, "workspace-root")
	if err != nil || !sameWorkspace(initial, created) {
		t.Fatalf("initial Workspace read = %#v, %v", initial, err)
	}
	if _, err := workspaceService.GetWorkspace(ctx, workspace.AuthenticatedCaller{ID: "owner-b"}, "workspace-root"); !errors.Is(err, workspace.ErrForbidden) {
		t.Fatalf("non-owner Workspace read = %v, want forbidden", err)
	}
	if _, err := workspaceService.GetWorkspace(ctx, owner, "missing-root"); !errors.Is(err, workspace.ErrNotFound) {
		t.Fatalf("unknown Workspace read = %v, want not found", err)
	}

	databaseURL := integrationDatabaseURL(t)
	reconstructedPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("reopen Workspace database pool: %v", err)
	}
	t.Cleanup(reconstructedPool.Close)
	reconstructedStore, err := workspacepostgres.NewStore(reconstructedPool)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	reconstructedService, err := workspace.NewService(reconstructedStore, catalogService, workspace.OwnerPolicy{}, validator, time.Now, workspace.NewRandomID)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := reconstructedService.GetWorkspace(ctx, owner, "workspace-root")
	if err != nil {
		t.Fatalf("reconstructed Workspace read: %v", err)
	}
	if !sameWorkspace(restarted, created) {
		t.Fatalf("reconstructed Workspace = %#v, want %#v", restarted, created)
	}

	failedContext, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := reconstructedService.GetWorkspace(failedContext, owner, "workspace-root"); !errors.Is(err, workspace.ErrDependency) {
		t.Fatalf("canceled Workspace read = %v, want dependency", err)
	}
}

func TestInstallationInspectionHistorySurvivesStoreReconstruction(t *testing.T) {
	ctx := context.Background()
	catalogService, workspaceService := integrationServices(t, ctx)
	owner := workspace.AuthenticatedCaller{ID: "owner-a"}
	if _, err := workspaceService.CreateWorkspace(ctx, owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-inspection-restart"}); err != nil {
		t.Fatalf("create Workspace: %v", err)
	}

	var expected []contracts.Installation
	expectedByID := make(map[string]contracts.Installation)
	for index := 0; index < 3; index++ {
		created, err := workspaceService.Install(ctx, owner, "workspace-inspection-restart", contracts.InstallAgentRequest{AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"}})
		if err != nil {
			t.Fatalf("install history row %d: %v", index, err)
		}
		if index < 2 {
			if _, err := workspaceService.UpdateInstallation(ctx, owner, "workspace-inspection-restart", created.InstallationID, "disabled"); err != nil {
				t.Fatalf("disable history row %d: %v", index, err)
			}
			if _, err := workspaceService.Uninstall(ctx, owner, "workspace-inspection-restart", created.InstallationID); err != nil {
				t.Fatalf("uninstall history row %d: %v", index, err)
			}
		}
		stored, err := workspaceService.GetInstallation(ctx, owner, "workspace-inspection-restart", created.InstallationID)
		if err != nil {
			t.Fatalf("read history row %d: %v", index, err)
		}
		expected = append(expected, stored)
		expectedByID[stored.InstallationID] = stored
	}

	beforeRestart, err := listAllInspectionInstallations(ctx, workspaceService, owner, "workspace-inspection-restart", 1)
	if err != nil {
		t.Fatalf("list history before restart: %v", err)
	}
	if len(beforeRestart) != len(expected) {
		t.Fatalf("history before restart count = %d, want %d", len(beforeRestart), len(expected))
	}
	seen := make(map[string]struct{}, len(beforeRestart))
	for index, row := range beforeRestart {
		if _, exists := seen[row.InstallationID]; exists {
			t.Fatalf("history before restart contains duplicate %s", row.InstallationID)
		}
		seen[row.InstallationID] = struct{}{}
		want, exists := expectedByID[row.InstallationID]
		if !exists || !sameInstallation(row, want) {
			t.Fatalf("history before restart[%d] = %#v, want committed row %#v", index, row, want)
		}
	}

	databaseURL := integrationDatabaseURL(t)
	reconstructedPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("reopen Workspace database pool: %v", err)
	}
	t.Cleanup(reconstructedPool.Close)
	reconstructedStore, err := workspacepostgres.NewStore(reconstructedPool)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	reconstructedService, err := workspace.NewService(reconstructedStore, catalogService, workspace.OwnerPolicy{}, validator, time.Now, workspace.NewRandomID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range expected {
		actual, err := reconstructedService.GetInstallation(ctx, owner, "workspace-inspection-restart", row.InstallationID)
		if err != nil {
			t.Fatalf("restarted read %s: %v", row.InstallationID, err)
		}
		if !sameInstallation(actual, row) {
			t.Fatalf("restarted read = %#v, want %#v", actual, row)
		}
	}
	afterRestart, err := listAllInspectionInstallations(ctx, reconstructedService, owner, "workspace-inspection-restart", 1)
	if err != nil {
		t.Fatalf("list history after restart: %v", err)
	}
	if len(afterRestart) != len(beforeRestart) {
		t.Fatalf("history after restart count = %d, want %d", len(afterRestart), len(beforeRestart))
	}
	for index := range afterRestart {
		if !sameInstallation(afterRestart[index], beforeRestart[index]) {
			t.Fatalf("history after restart[%d] = %#v, want %#v", index, afterRestart[index], beforeRestart[index])
		}
	}
}

func listAllInspectionInstallations(ctx context.Context, service *workspace.Service, caller workspace.AuthenticatedCaller, workspaceID string, limit int) ([]contracts.Installation, error) {
	var result []contracts.Installation
	var cursor *string
	for {
		page, err := service.ListInstallations(ctx, caller, workspaceID, limit, cursor)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if page.NextCursor == nil {
			return result, nil
		}
		cursor = page.NextCursor
	}
}

func TestInstallPinsCommittedFieldsAndIgnoresNewPublication(t *testing.T) {
	ctx := context.Background()
	catalogService, workspaceService := integrationServices(t, ctx)
	owner := workspace.AuthenticatedCaller{ID: "owner-a"}
	if _, err := workspaceService.CreateWorkspace(ctx, owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-pin"}); err != nil {
		t.Fatal(err)
	}
	installation, err := workspaceService.Install(ctx, owner, "workspace-pin", contracts.InstallAgentRequest{
		AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
	})
	if err != nil {
		t.Fatalf("install pinned Agent: %v", err)
	}
	if installation.InstalledVersion != "1.0.0" || installation.Status != "enabled" || installation.AcceptedPermissions[0] != "document.read" {
		t.Fatalf("installation = %#v", installation)
	}
	stored, err := workspaceService.GetInstallation(ctx, owner, "workspace-pin", installation.InstallationID)
	if err != nil || !sameInstallation(stored, installation) {
		t.Fatalf("stored installation = %#v, %v; created = %#v", stored, err, installation)
	}

	newCard := integrationCard()
	newCard.Version = "1.1.0"
	if err := registerPublishedCard(ctx, catalogService, newCard); err != nil {
		t.Fatalf("publish newer matching Card: %v", err)
	}
	unchanged, err := workspaceService.GetInstallation(ctx, owner, "workspace-pin", installation.InstallationID)
	if err != nil || !sameInstallation(unchanged, installation) {
		t.Fatalf("new publication mutated installation = %#v, %v; original = %#v", unchanged, err, installation)
	}

	emptyCard := integrationCard()
	emptyCard.AgentID = "runtime-empty"
	emptyCard.Name = "Runtime Empty Permission"
	emptyCard.Skills[0].RequiredPermissions = []string{}
	emptyCard.Permissions = []contracts.PermissionDeclaration{}
	if err := registerPublishedCard(ctx, catalogService, emptyCard); err != nil {
		t.Fatalf("publish empty-permission Card: %v", err)
	}
	if _, err := workspaceService.CreateWorkspace(ctx, owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-empty-install"}); err != nil {
		t.Fatal(err)
	}
	emptyInstallation, err := workspaceService.Install(ctx, owner, "workspace-empty-install", contracts.InstallAgentRequest{
		AgentID: "runtime-empty", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{},
	})
	if err != nil {
		t.Fatalf("install empty-permission Agent: %v", err)
	}
	if emptyInstallation.AcceptedPermissions == nil || len(emptyInstallation.AcceptedPermissions) != 0 {
		t.Fatalf("empty accepted permissions = %#v", emptyInstallation.AcceptedPermissions)
	}
	emptyStored, err := workspaceService.GetInstallation(ctx, owner, "workspace-empty-install", emptyInstallation.InstallationID)
	if err != nil || emptyStored.AcceptedPermissions == nil || len(emptyStored.AcceptedPermissions) != 0 {
		t.Fatalf("stored empty accepted permissions = %#v, %v", emptyStored.AcceptedPermissions, err)
	}

	failedContext, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := workspaceService.Install(failedContext, owner, "workspace-pin", contracts.InstallAgentRequest{
		AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
	}); !errors.Is(err, workspace.ErrDependency) {
		t.Fatalf("canceled install = %v, want dependency", err)
	}
}

func TestInstallationLifecyclePersistsCommittedHistoryAcrossRestart(t *testing.T) {
	ctx := context.Background()
	catalogService, workspaceService := integrationServices(t, ctx)
	owner := workspace.AuthenticatedCaller{ID: "owner-a"}
	if _, err := workspaceService.CreateWorkspace(ctx, owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-lifecycle-restart"}); err != nil {
		t.Fatal(err)
	}
	original, err := workspaceService.Install(ctx, owner, "workspace-lifecycle-restart", contracts.InstallAgentRequest{
		AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	disabled, err := workspaceService.UpdateInstallation(ctx, owner, "workspace-lifecycle-restart", original.InstallationID, "disabled")
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !disabled.UpdatedAt.After(original.UpdatedAt) || disabled.Status != "disabled" || disabled.UninstalledAt != nil {
		t.Fatalf("disabled committed row = %#v", disabled)
	}
	terminal, err := workspaceService.Uninstall(ctx, owner, "workspace-lifecycle-restart", original.InstallationID)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if terminal.Status != "uninstalled" || terminal.UninstalledAt == nil || !terminal.UninstalledAt.Equal(terminal.UpdatedAt) || !terminal.UpdatedAt.After(disabled.UpdatedAt) {
		t.Fatalf("terminal committed row = %#v", terminal)
	}
	terminalValidator, err := contracts.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	if err := terminalValidator.ValidateInstallation(terminal); err != nil {
		t.Fatalf("terminal Installation contract = %v", err)
	}

	databaseURL := integrationDatabaseURL(t)
	restartedPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("reopen Workspace database: %v", err)
	}
	t.Cleanup(restartedPool.Close)
	restartedStore, err := workspacepostgres.NewStore(restartedPool)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	restartedService, err := workspace.NewService(restartedStore, catalogService, workspace.OwnerPolicy{}, validator, time.Now, workspace.NewRandomID)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := restartedService.GetInstallation(ctx, owner, "workspace-lifecycle-restart", original.InstallationID)
	if err != nil {
		t.Fatalf("read terminal after restart: %v", err)
	}
	if !sameInstallation(restarted, terminal) {
		t.Fatalf("restarted terminal = %#v, want %#v", restarted, terminal)
	}

	reinstalled, err := workspaceService.Install(ctx, owner, "workspace-lifecycle-restart", contracts.InstallAgentRequest{
		AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
	})
	if err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if reinstalled.InstallationID == original.InstallationID || reinstalled.Status != "enabled" {
		t.Fatalf("reinstall identity/state = %#v", reinstalled)
	}
	history, err := restartedService.ListInstallations(ctx, owner, "workspace-lifecycle-restart", 100, nil)
	if err != nil || len(history.Items) != 2 {
		t.Fatalf("lifecycle history = %#v, %v", history, err)
	}
	if history.Items[0].Status != "uninstalled" || history.Items[1].Status != "enabled" {
		t.Fatalf("lifecycle history states = %#v", history.Items)
	}
}

func TestConcurrentLifecycleAndReinstallRequestsPreserveOneCurrentRow(t *testing.T) {
	ctx := context.Background()
	_, workspaceService := integrationServices(t, ctx)
	owner := workspace.AuthenticatedCaller{ID: "owner-a"}
	if _, err := workspaceService.CreateWorkspace(ctx, owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-lifecycle-race"}); err != nil {
		t.Fatal(err)
	}
	original, err := workspaceService.Install(ctx, owner, "workspace-lifecycle-race", contracts.InstallAgentRequest{
		AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
	})
	if err != nil {
		t.Fatal(err)
	}

	type raceResult struct {
		operation    string
		installation contracts.Installation
		err          error
	}

	var wait sync.WaitGroup
	disableResults := make(chan raceResult, 100)
	for index := 0; index < 100; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			value, err := workspaceService.UpdateInstallation(ctx, owner, "workspace-lifecycle-race", original.InstallationID, "disabled")
			disableResults <- raceResult{operation: "disable", installation: value, err: err}
		}()
	}
	wait.Wait()
	close(disableResults)
	disableSuccesses, disableConflicts := 0, 0
	var disabled contracts.Installation
	for result := range disableResults {
		switch {
		case result.err == nil:
			disableSuccesses++
			disabled = result.installation
		case errors.Is(result.err, workspace.ErrConflict):
			if result.installation.InstallationID != "" {
				t.Fatalf("conflicting disable returned an Installation: %#v", result.installation)
			}
			disableConflicts++
		default:
			t.Fatalf("concurrent %s error: %v", result.operation, result.err)
		}
	}
	if disableSuccesses != 1 || disableConflicts != 99 {
		t.Fatalf("concurrent disable successes=%d conflicts=%d", disableSuccesses, disableConflicts)
	}
	if disabled.InstallationID != original.InstallationID || disabled.Status != "disabled" || disabled.UninstalledAt != nil || !disabled.UpdatedAt.After(original.UpdatedAt) || !sameInstallationImmutable(disabled, original) {
		t.Fatalf("concurrent disable result = %#v, original = %#v", disabled, original)
	}

	results := make(chan raceResult, 100)
	for index := 0; index < 100; index++ {
		wait.Add(1)
		if index%2 == 0 {
			go func() {
				defer wait.Done()
				value, err := workspaceService.Uninstall(ctx, owner, "workspace-lifecycle-race", original.InstallationID)
				results <- raceResult{operation: "uninstall", installation: value, err: err}
			}()
			continue
		}
		go func() {
			defer wait.Done()
			value, err := workspaceService.Install(ctx, owner, "workspace-lifecycle-race", contracts.InstallAgentRequest{
				AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
			})
			results <- raceResult{operation: "reinstall", installation: value, err: err}
		}()
	}
	wait.Wait()
	close(results)
	uninstallSuccesses, reinstallSuccesses, conflicts := 0, 0, 0
	var terminal contracts.Installation
	var reinstalled contracts.Installation
	for result := range results {
		switch {
		case result.err == nil:
			switch result.operation {
			case "uninstall":
				uninstallSuccesses++
				terminal = result.installation
			case "reinstall":
				reinstallSuccesses++
				reinstalled = result.installation
			default:
				t.Fatalf("unknown successful race operation %q", result.operation)
			}
		case errors.Is(result.err, workspace.ErrConflict):
			if result.installation.InstallationID != "" {
				t.Fatalf("conflicting %s returned an Installation: %#v", result.operation, result.installation)
			}
			conflicts++
		default:
			t.Fatalf("concurrent %s error: %v", result.operation, result.err)
		}
	}
	if uninstallSuccesses != 1 || reinstallSuccesses > 1 || conflicts != 99-reinstallSuccesses {
		t.Fatalf("concurrent uninstall/reinstall outcomes: uninstalls=%d reinstalls=%d conflicts=%d", uninstallSuccesses, reinstallSuccesses, conflicts)
	}
	if terminal.InstallationID != original.InstallationID || terminal.Status != "uninstalled" || terminal.UninstalledAt == nil || !terminal.UninstalledAt.Equal(terminal.UpdatedAt) || !terminal.UpdatedAt.After(disabled.UpdatedAt) || !sameInstallationImmutable(terminal, original) {
		t.Fatalf("concurrent uninstall result = %#v, original = %#v, disabled = %#v", terminal, original, disabled)
	}
	if reinstallSuccesses == 1 && (reinstalled.InstallationID == original.InstallationID || reinstalled.Status != "enabled" || reinstalled.UninstalledAt != nil || !sameInstallationPin(reinstalled, original)) {
		t.Fatalf("concurrent reinstall result = %#v, original = %#v", reinstalled, original)
	}

	listed, err := workspaceService.ListInstallations(ctx, owner, "workspace-lifecycle-race", 100, nil)
	if err != nil {
		t.Fatalf("list lifecycle race history: %v", err)
	}
	current := 0
	terminalCount := 0
	seen := make(map[string]struct{}, len(listed.Items))
	for _, value := range listed.Items {
		if _, exists := seen[value.InstallationID]; exists {
			t.Fatalf("lifecycle race history contains duplicate %s", value.InstallationID)
		}
		seen[value.InstallationID] = struct{}{}
		switch value.Status {
		case "enabled", "disabled":
			current++
		case "uninstalled":
			terminalCount++
			if value.InstallationID != original.InstallationID || !sameInstallationImmutable(value, original) || value.UninstalledAt == nil || !value.UninstalledAt.Equal(value.UpdatedAt) {
				t.Fatalf("unexpected replacement terminal row: %#v", value)
			}
		default:
			t.Fatalf("unexpected lifecycle race status %q in %#v", value.Status, value)
		}
	}
	if terminalCount != 1 || current != reinstallSuccesses || len(listed.Items) != 1+reinstallSuccesses {
		t.Fatalf("lifecycle race history = %#v", listed.Items)
	}
}

func TestLifecycleDependencyFailuresRemainExplicit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, workspaceService := integrationServices(t, context.Background())
	owner := workspace.AuthenticatedCaller{ID: "owner-a"}
	if _, err := workspaceService.CreateWorkspace(context.Background(), owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-lifecycle-dependency"}); err != nil {
		t.Fatal(err)
	}
	installation, err := workspaceService.Install(context.Background(), owner, "workspace-lifecycle-dependency", contracts.InstallAgentRequest{
		AgentID: "runtime-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspaceService.UpdateInstallation(ctx, owner, "workspace-lifecycle-dependency", installation.InstallationID, "disabled"); !errors.Is(err, workspace.ErrDependency) {
		t.Fatalf("canceled disable = %v, want dependency", err)
	}
	if _, err := workspaceService.Uninstall(ctx, owner, "workspace-lifecycle-dependency", installation.InstallationID); !errors.Is(err, workspace.ErrDependency) {
		t.Fatalf("canceled uninstall = %v, want dependency", err)
	}
}

func integrationServices(t *testing.T, ctx context.Context) (*catalog.Service, *workspace.Service) {
	t.Helper()
	databaseURL := integrationDatabaseURL(t)
	if _, err := pgx.ParseConfig(databaseURL); err != nil {
		t.Fatal("integration database URL was rejected")
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
	if err := registerPublishedCard(ctx, catalogService, integrationCard()); err != nil {
		t.Fatalf("publish fixture Card: %v", err)
	}
	workspaceService, err := workspace.NewService(workspaceStore, catalogService, workspace.OwnerPolicy{}, validator, time.Now, workspace.NewRandomID)
	if err != nil {
		t.Fatal(err)
	}
	return catalogService, workspaceService
}

func integrationDatabaseURL(t *testing.T) string {
	t.Helper()
	databaseURL := os.Getenv("NEKIRO_TEST_DATABASE_URL")
	if strings.TrimSpace(databaseURL) == "" {
		t.Fatal("NEKIRO_TEST_DATABASE_URL is required")
	}
	configuration, err := pgx.ParseConfig(databaseURL)
	if err != nil || !strings.HasSuffix(configuration.Database, "_test") {
		t.Fatal("integration database must end in _test")
	}
	return databaseURL
}

func sameWorkspace(left, right contracts.Workspace) bool {
	return left.WorkspaceID == right.WorkspaceID && left.OwnerID == right.OwnerID &&
		left.CreatedAt.Equal(right.CreatedAt) && left.UpdatedAt.Equal(right.UpdatedAt)
}

func sameInstallation(left, right contracts.Installation) bool {
	if left.InstallationID != right.InstallationID || left.WorkspaceID != right.WorkspaceID ||
		left.AgentID != right.AgentID || left.VersionConstraint != right.VersionConstraint ||
		left.InstalledVersion != right.InstalledVersion || left.Status != right.Status ||
		!left.InstalledAt.Equal(right.InstalledAt) || !left.UpdatedAt.Equal(right.UpdatedAt) ||
		(left.UninstalledAt == nil) != (right.UninstalledAt == nil) ||
		left.UninstalledAt != nil && !left.UninstalledAt.Equal(*right.UninstalledAt) ||
		len(left.AcceptedPermissions) != len(right.AcceptedPermissions) {
		return false
	}
	for index := range left.AcceptedPermissions {
		if left.AcceptedPermissions[index] != right.AcceptedPermissions[index] {
			return false
		}
	}
	return true
}

func sameInstallationImmutable(left, right contracts.Installation) bool {
	return left.InstallationID == right.InstallationID && sameInstallationPin(left, right)
}

func sameInstallationPin(left, right contracts.Installation) bool {
	if left.WorkspaceID != right.WorkspaceID || left.AgentID != right.AgentID || left.VersionConstraint != right.VersionConstraint || left.InstalledVersion != right.InstalledVersion || len(left.AcceptedPermissions) != len(right.AcceptedPermissions) {
		return false
	}
	for index := range left.AcceptedPermissions {
		if left.AcceptedPermissions[index] != right.AcceptedPermissions[index] {
			return false
		}
	}
	return true
}

func registerPublishedCard(ctx context.Context, service *catalog.Service, card contracts.AgentCard) error {
	body, err := json.Marshal(contracts.RegisterAgentRequest{Card: card})
	if err != nil {
		return err
	}
	if _, err := service.Register(ctx, catalog.AuthenticatedCaller{ID: "owner-a"}, body); err != nil {
		return fmt.Errorf("register Card: %w", err)
	}
	if _, err := service.Publish(ctx, catalog.AuthenticatedCaller{ID: "owner-a"}, card.AgentID, card.Version); err != nil {
		return fmt.Errorf("publish Card: %w", err)
	}
	return nil
}

func integrationCard() contracts.AgentCard {
	return contracts.AgentCard{SchemaVersion: "0.2", AgentID: "runtime-a", Name: "Runtime A", Description: "Integration fixture", Owner: contracts.AgentOwner{ID: "owner-a", DisplayName: "Owner"}, Version: "1.0.0", Protocol: contracts.AgentProtocol{Type: "a2a", Version: "0.3.0", Transport: "JSONRPC", Endpoint: "https://agent.example.test/a2a"}, Skills: []contracts.AgentSkill{{ID: "document.read", Name: "Read", Description: "Read", InputSchema: contracts.JSONSchema{"type": "object"}, OutputSchema: contracts.JSONSchema{"type": "object"}, RequiredPermissions: []string{"document.read"}}}, Authentication: contracts.AgentAuthentication{Type: "none"}, Permissions: []contracts.PermissionDeclaration{{ID: "document.read", Description: "Read"}}, Limits: contracts.AgentLimits{TimeoutMS: 1000, MaxInputBytes: json.Number("1000"), MaxOutputBytes: json.Number("1000"), Streaming: false}}
}
