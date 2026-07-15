package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/contracts"
)

type memoryStore struct {
	workspaces    map[string]contracts.Workspace
	installations map[string]contracts.Installation
}

func newMemoryStore() *memoryStore {
	return &memoryStore{workspaces: map[string]contracts.Workspace{}, installations: map[string]contracts.Installation{}}
}

func (store *memoryStore) CreateWorkspace(_ context.Context, value contracts.Workspace) (contracts.Workspace, error) {
	if _, exists := store.workspaces[value.WorkspaceID]; exists {
		return contracts.Workspace{}, ErrConflict
	}
	store.workspaces[value.WorkspaceID] = value
	return value, nil
}
func (store *memoryStore) GetWorkspace(_ context.Context, id string) (contracts.Workspace, error) {
	value, exists := store.workspaces[id]
	if !exists {
		return contracts.Workspace{}, ErrNotFound
	}
	return value, nil
}
func (store *memoryStore) HasCurrentInstallation(_ context.Context, workspaceID, agentID string) (bool, error) {
	for _, value := range store.installations {
		if value.WorkspaceID == workspaceID && value.AgentID == agentID && value.Status != "uninstalled" {
			return true, nil
		}
	}
	return false, nil
}
func (store *memoryStore) CreateInstallation(_ context.Context, callerID string, value contracts.Installation) (contracts.Installation, error) {
	workspace, exists := store.workspaces[value.WorkspaceID]
	if !exists {
		return contracts.Installation{}, ErrNotFound
	}
	if workspace.OwnerID != callerID {
		return contracts.Installation{}, ErrForbidden
	}
	current, _ := store.HasCurrentInstallation(context.Background(), value.WorkspaceID, value.AgentID)
	if current {
		return contracts.Installation{}, ErrConflict
	}
	store.installations[value.InstallationID] = value
	return value, nil
}
func (store *memoryStore) GetInstallation(_ context.Context, workspaceID, installationID string) (contracts.Installation, error) {
	value, exists := store.installations[installationID]
	if !exists || value.WorkspaceID != workspaceID {
		return contracts.Installation{}, ErrNotFound
	}
	return value, nil
}
func (store *memoryStore) GetCurrentInstallation(_ context.Context, workspaceID, agentID string) (contracts.Installation, error) {
	for _, value := range store.installations {
		if value.WorkspaceID == workspaceID && value.AgentID == agentID && value.Status != "uninstalled" {
			return value, nil
		}
	}
	return contracts.Installation{}, ErrNotFound
}
func (store *memoryStore) ListInstallations(_ context.Context, workspaceID string, limit int, after *InstallationPosition) ([]contracts.Installation, bool, error) {
	items := make([]contracts.Installation, 0)
	for _, value := range store.installations {
		if value.WorkspaceID == workspaceID && (after == nil || value.InstalledAt.After(after.InstalledAt) || value.InstalledAt.Equal(after.InstalledAt) && value.InstallationID > after.InstallationID) {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].InstalledAt.Equal(items[j].InstalledAt) {
			return items[i].InstallationID < items[j].InstallationID
		}
		return items[i].InstalledAt.Before(items[j].InstalledAt)
	})
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}
func (store *memoryStore) ChangeInstallationStatus(_ context.Context, workspaceID, installationID, status string, at time.Time) (contracts.Installation, error) {
	value, err := store.GetInstallation(context.Background(), workspaceID, installationID)
	if err != nil {
		return contracts.Installation{}, err
	}
	if value.Status == "uninstalled" || value.Status == status || value.Status == "enabled" && status != "disabled" || value.Status == "disabled" && status != "enabled" {
		return contracts.Installation{}, ErrConflict
	}
	value.Status, value.UpdatedAt = status, at
	store.installations[installationID] = value
	return value, nil
}
func (store *memoryStore) UninstallInstallation(_ context.Context, workspaceID, installationID string, at time.Time) (contracts.Installation, error) {
	value, err := store.GetInstallation(context.Background(), workspaceID, installationID)
	if err != nil {
		return contracts.Installation{}, err
	}
	if value.Status != "disabled" {
		return contracts.Installation{}, ErrConflict
	}
	value.Status, value.UpdatedAt, value.UninstalledAt = "uninstalled", at, &at
	store.installations[installationID] = value
	return value, nil
}
func (store *memoryStore) Check(context.Context) error { return nil }

type memoryCatalog struct {
	candidates  []catalog.AgentVersion
	versions    map[string]catalog.AgentVersion
	selectErr   error
	selectCalls int
	getCalls    int
}

func (reader *memoryCatalog) SelectPublished(_ context.Context, agentID, constraint string) (catalog.AgentVersion, error) {
	reader.selectCalls++
	if reader.selectErr != nil {
		return catalog.AgentVersion{}, reader.selectErr
	}
	validator, err := contracts.NewValidator()
	if err != nil {
		return catalog.AgentVersion{}, err
	}
	service, err := catalog.NewService(&selectionStore{reader: reader}, validator, time.Now)
	if err != nil {
		return catalog.AgentVersion{}, err
	}
	return service.SelectPublished(context.Background(), agentID, constraint)
}
func (reader *memoryCatalog) GetVersion(_ context.Context, agentID, version string) (catalog.AgentVersion, error) {
	reader.getCalls++
	value, exists := reader.versions[agentID+"/"+version]
	if !exists {
		return catalog.AgentVersion{}, catalog.ErrNotFound
	}
	return value, nil
}

type selectionStore struct{ reader *memoryCatalog }

func (store *selectionStore) Register(context.Context, catalog.AgentVersion) (catalog.AgentVersion, error) {
	return catalog.AgentVersion{}, nil
}
func (store *selectionStore) Get(_ context.Context, agentID, version string) (catalog.AgentVersion, error) {
	return store.reader.GetVersion(context.Background(), agentID, version)
}
func (store *selectionStore) Published(context.Context, string) ([]catalog.AgentVersion, error) {
	return store.reader.candidates, nil
}
func (store *selectionStore) Publish(context.Context, string, string, string, time.Time) (catalog.AgentVersion, error) {
	return catalog.AgentVersion{}, nil
}
func (store *selectionStore) Disable(context.Context, string, string, string, time.Time) (catalog.AgentVersion, error) {
	return catalog.AgentVersion{}, nil
}
func (store *selectionStore) DiscoverFirstPage(context.Context, catalog.DiscoveryFilter) (int64, catalog.DiscoveryResult, error) {
	return 0, catalog.DiscoveryResult{}, nil
}
func (store *selectionStore) Discover(context.Context, catalog.DiscoveryQuery) (catalog.DiscoveryResult, error) {
	return catalog.DiscoveryResult{}, nil
}
func (store *selectionStore) Check(context.Context) error { return nil }

type failingWorkspaceStore struct {
	Store
	createErr       error
	getWorkspaceErr error
}

func (store *failingWorkspaceStore) CreateWorkspace(context.Context, contracts.Workspace) (contracts.Workspace, error) {
	return contracts.Workspace{}, store.createErr
}

func (store *failingWorkspaceStore) GetWorkspace(context.Context, string) (contracts.Workspace, error) {
	return contracts.Workspace{}, store.getWorkspaceErr
}

func TestWorkspaceRootTrustsOwnerAndPreservesDuplicate(t *testing.T) {
	store := newMemoryStore()
	service := newWorkspaceTestService(t, store, &memoryCatalog{})
	created, err := service.CreateWorkspace(context.Background(), AuthenticatedCaller{ID: "owner-a"}, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("create Workspace: %v", err)
	}
	if created.WorkspaceID != "workspace-a" || created.OwnerID != "owner-a" || created.CreatedAt.IsZero() || !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("created Workspace = %#v", created)
	}

	if _, err := service.CreateWorkspace(context.Background(), AuthenticatedCaller{ID: "owner-b"}, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-a"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate Workspace = %v, want conflict", err)
	}
	read, err := service.GetWorkspace(context.Background(), AuthenticatedCaller{ID: "owner-a"}, "workspace-a")
	if err != nil {
		t.Fatalf("read Workspace: %v", err)
	}
	if read != created {
		t.Fatalf("duplicate changed Workspace: created=%#v read=%#v", created, read)
	}
	if _, err := service.GetWorkspace(context.Background(), AuthenticatedCaller{ID: "owner-b"}, "workspace-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-owner read = %v, want forbidden", err)
	}
	if _, err := service.GetWorkspace(context.Background(), AuthenticatedCaller{ID: "owner-a"}, "missing-workspace"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown read = %v, want not found", err)
	}
	if _, err := service.CreateWorkspace(context.Background(), AuthenticatedCaller{}, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-b"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing owner = %v, want invalid", err)
	}
}

func TestWorkspaceRootPropagatesPersistenceFailures(t *testing.T) {
	createFailure := &failingWorkspaceStore{createErr: ErrDependency}
	createService := newWorkspaceTestService(t, createFailure, &memoryCatalog{})
	if _, err := createService.CreateWorkspace(context.Background(), AuthenticatedCaller{ID: "owner-a"}, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-a"}); !errors.Is(err, ErrDependency) {
		t.Fatalf("create dependency failure = %v, want dependency", err)
	}

	readFailure := &failingWorkspaceStore{getWorkspaceErr: ErrDependency}
	readService := newWorkspaceTestService(t, readFailure, &memoryCatalog{})
	if _, err := readService.GetWorkspace(context.Background(), AuthenticatedCaller{ID: "owner-a"}, "workspace-a"); !errors.Is(err, ErrDependency) {
		t.Fatalf("read dependency failure = %v, want dependency", err)
	}
}

func TestInstallPreservesExplicitEmptyPermissions(t *testing.T) {
	card := testWorkspaceCard("agent-empty", "1.0.0", nil, nil)
	reader := &memoryCatalog{candidates: []catalog.AgentVersion{{Card: card, Status: catalog.PublicationPublished}}}
	store := newMemoryStore()
	service := newWorkspaceTestService(t, store, reader)
	caller := AuthenticatedCaller{ID: "owner-a"}
	if _, err := service.CreateWorkspace(context.Background(), caller, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-empty"}); err != nil {
		t.Fatal(err)
	}
	installation, err := service.Install(context.Background(), caller, "workspace-empty", contracts.InstallAgentRequest{
		AgentID: "agent-empty", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{},
	})
	if err != nil {
		t.Fatalf("empty-permission install: %v", err)
	}
	if installation.AcceptedPermissions == nil || len(installation.AcceptedPermissions) != 0 {
		t.Fatalf("empty permission snapshot = %#v", installation.AcceptedPermissions)
	}
	if _, err := service.Install(context.Background(), caller, "workspace-empty", contracts.InstallAgentRequest{
		AgentID: "agent-empty", VersionConstraint: "^1.0.0", AcceptedPermissions: nil,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing permission slice = %v, want invalid", err)
	}
}

func TestInstallRejectsUnknownPermissionBeforePersistence(t *testing.T) {
	card := testWorkspaceCard("agent-permission", "1.0.0", []string{"declared"}, nil)
	reader := &memoryCatalog{candidates: []catalog.AgentVersion{{Card: card, Status: catalog.PublicationPublished}}}
	store := newMemoryStore()
	service := newWorkspaceTestService(t, store, reader)
	caller := AuthenticatedCaller{ID: "owner-a"}
	if _, err := service.CreateWorkspace(context.Background(), caller, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-permission"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Install(context.Background(), caller, "workspace-permission", contracts.InstallAgentRequest{
		AgentID: "agent-permission", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"unknown"},
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown permission = %v, want invalid", err)
	}
	if len(store.installations) != 0 {
		t.Fatalf("unknown permission persisted installations: %#v", store.installations)
	}
}

func TestInstallPropagatesCatalogDependencyFailure(t *testing.T) {
	store := newMemoryStore()
	service := newWorkspaceTestService(t, store, &memoryCatalog{selectErr: catalog.ErrDependency})
	caller := AuthenticatedCaller{ID: "owner-a"}
	if _, err := service.CreateWorkspace(context.Background(), caller, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-dependency"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Install(context.Background(), caller, "workspace-dependency", contracts.InstallAgentRequest{
		AgentID: "agent-dependency", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{},
	}); !errors.Is(err, ErrDependency) {
		t.Fatalf("Catalog dependency failure = %v, want dependency", err)
	}
	if len(store.installations) != 0 {
		t.Fatalf("Catalog dependency failure persisted installations: %#v", store.installations)
	}
}

func TestInstallHonorsCatalogPrereleasePolicy(t *testing.T) {
	stable := testWorkspaceCard("agent-prerelease", "1.0.0", []string{}, []string{})
	preRelease := testWorkspaceCard("agent-prerelease", "1.1.0-rc.1", []string{}, []string{})
	reader := &memoryCatalog{candidates: []catalog.AgentVersion{
		{Card: stable, Status: catalog.PublicationPublished},
		{Card: preRelease, Status: catalog.PublicationPublished},
	}}
	store := newMemoryStore()
	service := newWorkspaceTestService(t, store, reader)
	caller := AuthenticatedCaller{ID: "owner-a"}
	if _, err := service.CreateWorkspace(context.Background(), caller, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-stable"}); err != nil {
		t.Fatal(err)
	}
	stableInstallation, err := service.Install(context.Background(), caller, "workspace-stable", contracts.InstallAgentRequest{
		AgentID: "agent-prerelease", VersionConstraint: ">=1.0.0 <2.0.0", AcceptedPermissions: []string{},
	})
	if err != nil || stableInstallation.InstalledVersion != "1.0.0" {
		t.Fatalf("stable-only selection = %#v, %v", stableInstallation, err)
	}

	if _, err := service.CreateWorkspace(context.Background(), caller, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-prerelease"}); err != nil {
		t.Fatal(err)
	}
	preReleaseInstallation, err := service.Install(context.Background(), caller, "workspace-prerelease", contracts.InstallAgentRequest{
		AgentID: "agent-prerelease", VersionConstraint: ">=1.1.0-0 <2.0.0", AcceptedPermissions: []string{},
	})
	if err != nil || preReleaseInstallation.InstalledVersion != "1.1.0-rc.1" {
		t.Fatalf("pre-release selection = %#v, %v", preReleaseInstallation, err)
	}
}

func TestInstallPinsHighestVersionAndCanonicalPermissions(t *testing.T) {
	card := testWorkspaceCard("agent-a", "1.0.0", []string{"read", "write"}, []string{"read"})
	cardBuildA := testWorkspaceCard("agent-a", "1.0.1+a", []string{"read", "write"}, []string{"read"})
	cardBuildZ := testWorkspaceCard("agent-a", "1.0.1+z", []string{"read", "write"}, []string{"read"})
	reader := &memoryCatalog{candidates: []catalog.AgentVersion{{Card: card, Status: catalog.PublicationPublished}, {Card: cardBuildA, Status: catalog.PublicationPublished}, {Card: cardBuildZ, Status: catalog.PublicationPublished}}, versions: map[string]catalog.AgentVersion{"agent-a/1.0.1+z": {Card: cardBuildZ, Status: catalog.PublicationPublished}}}
	store := newMemoryStore()
	service := newWorkspaceTestService(t, store, reader)
	if _, err := service.CreateWorkspace(context.Background(), AuthenticatedCaller{ID: "owner-a"}, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-a"}); err != nil {
		t.Fatal(err)
	}
	installation, err := service.Install(context.Background(), AuthenticatedCaller{ID: "owner-a"}, "workspace-a", contracts.InstallAgentRequest{AgentID: "agent-a", VersionConstraint: ">=1.0.0", AcceptedPermissions: []string{"write", "read"}})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if installation.InstalledVersion != "1.0.1+z" || installation.AcceptedPermissions[0] != "read" || installation.AcceptedPermissions[1] != "write" {
		t.Fatalf("installation pin = %#v", installation)
	}
	if _, err := service.Install(context.Background(), AuthenticatedCaller{ID: "owner-b"}, "workspace-a", contracts.InstallAgentRequest{AgentID: "agent-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{}}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-owner install = %v", err)
	}
}

func TestLifecycleResolutionAndReinstall(t *testing.T) {
	card := testWorkspaceCard("agent-a", "1.0.0", []string{"read"}, []string{"read"})
	reader := &memoryCatalog{candidates: []catalog.AgentVersion{{Card: card, Status: catalog.PublicationPublished}}, versions: map[string]catalog.AgentVersion{"agent-a/1.0.0": {Card: card, Status: catalog.PublicationPublished}}}
	store := newMemoryStore()
	service := newWorkspaceTestService(t, store, reader)
	caller := AuthenticatedCaller{ID: "owner-a"}
	_, _ = service.CreateWorkspace(context.Background(), caller, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-a"})
	installation, err := service.Install(context.Background(), caller, "workspace-a", contracts.InstallAgentRequest{AgentID: "agent-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	request := contracts.ResolveAgentRequest{InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a", WorkspaceID: "workspace-a", AgentID: "agent-a", Version: "1.0.0", Capability: "capability.read"}
	if _, err := service.Resolve(context.Background(), request); err != nil {
		t.Fatalf("resolve enabled: %v", err)
	}
	if _, err := service.Uninstall(context.Background(), caller, "workspace-a", installation.InstallationID); !errors.Is(err, ErrConflict) {
		t.Fatalf("enabled uninstall = %v", err)
	}
	if _, err := service.UpdateInstallation(context.Background(), caller, "workspace-a", installation.InstallationID, "disabled"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(context.Background(), request); !errors.Is(err, ErrInstallationDisabled) {
		t.Fatalf("disabled resolve = %v", err)
	}
	terminal, err := service.Uninstall(context.Background(), caller, "workspace-a", installation.InstallationID)
	if err != nil || terminal.Status != "uninstalled" || terminal.UninstalledAt == nil {
		t.Fatalf("uninstall = %#v, %v", terminal, err)
	}
	second, err := service.Install(context.Background(), caller, "workspace-a", contracts.InstallAgentRequest{AgentID: "agent-a", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"read"}})
	if err != nil || second.InstallationID == installation.InstallationID {
		t.Fatalf("reinstall = %#v, %v", second, err)
	}
}

func TestLifecycleTransitionTablePreservesImmutableFactsAndTimestamps(t *testing.T) {
	store := newMemoryStore()
	reader := &memoryCatalog{candidates: []catalog.AgentVersion{{
		Card:   testWorkspaceCard("agent-lifecycle", "1.0.0", []string{"read", "write"}, nil),
		Status: catalog.PublicationPublished,
	}}}
	clockValues := []time.Time{
		time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 15, 10, 0, 1, 0, time.UTC),
		time.Date(2026, 7, 15, 10, 0, 2, 0, time.UTC),
		time.Date(2026, 7, 15, 10, 0, 3, 0, time.UTC),
		time.Date(2026, 7, 15, 10, 0, 4, 0, time.UTC),
		time.Date(2026, 7, 15, 10, 0, 5, 0, time.UTC),
		time.Date(2026, 7, 15, 10, 0, 6, 0, time.UTC),
		time.Date(2026, 7, 15, 10, 0, 7, 0, time.UTC),
	}
	clockIndex := 0
	service := newWorkspaceTestServiceWithClock(t, store, reader, func() time.Time {
		value := clockValues[clockIndex]
		clockIndex++
		return value
	})
	owner := AuthenticatedCaller{ID: "owner-a"}
	if _, err := service.CreateWorkspace(context.Background(), owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-lifecycle"}); err != nil {
		t.Fatal(err)
	}
	installation, err := service.Install(context.Background(), owner, "workspace-lifecycle", contracts.InstallAgentRequest{
		AgentID: "agent-lifecycle", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"write", "read"},
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !installation.InstalledAt.Equal(clockValues[1]) || !installation.UpdatedAt.Equal(clockValues[1]) {
		t.Fatalf("initial timestamps = %#v", installation)
	}

	before := installation
	disabled, err := service.UpdateInstallation(context.Background(), owner, "workspace-lifecycle", installation.InstallationID, "disabled")
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if disabled.Status != "disabled" || !disabled.UpdatedAt.Equal(clockValues[2]) || !disabled.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("disabled result = %#v", disabled)
	}
	assertImmutableInstallationFacts(t, before, disabled)

	if _, err := service.UpdateInstallation(context.Background(), owner, "workspace-lifecycle", installation.InstallationID, "disabled"); !errors.Is(err, ErrConflict) {
		t.Fatalf("same-state disable = %v, want conflict", err)
	}
	if _, err := service.Uninstall(context.Background(), owner, "workspace-lifecycle", installation.InstallationID); err != nil {
		t.Fatalf("uninstall disabled Installation: %v", err)
	}
	terminal, err := service.GetInstallation(context.Background(), owner, "workspace-lifecycle", installation.InstallationID)
	if err != nil {
		t.Fatalf("read terminal Installation: %v", err)
	}
	if terminal.Status != "uninstalled" || terminal.UninstalledAt == nil || !terminal.UninstalledAt.Equal(terminal.UpdatedAt) || !terminal.UpdatedAt.Equal(clockValues[4]) {
		t.Fatalf("terminal result = %#v", terminal)
	}
	assertImmutableInstallationFacts(t, before, terminal)
	if reader.selectCalls != 1 || reader.getCalls != 0 {
		t.Fatalf("lifecycle consulted Catalog: select=%d get=%d", reader.selectCalls, reader.getCalls)
	}

	if _, err := service.UpdateInstallation(context.Background(), owner, "workspace-lifecycle", installation.InstallationID, "enabled"); !errors.Is(err, ErrConflict) {
		t.Fatalf("re-enable terminal Installation = %v, want conflict", err)
	}
	if _, err := service.Uninstall(context.Background(), owner, "workspace-lifecycle", installation.InstallationID); !errors.Is(err, ErrConflict) {
		t.Fatalf("repeat terminal uninstall = %v, want conflict", err)
	}

	reinstalled, err := service.Install(context.Background(), owner, "workspace-lifecycle", contracts.InstallAgentRequest{
		AgentID: "agent-lifecycle", VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"read", "write"},
	})
	if err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if reinstalled.InstallationID == terminal.InstallationID || reinstalled.Status != "enabled" || !reinstalled.UpdatedAt.Equal(clockValues[7]) {
		t.Fatalf("reinstall result = %#v", reinstalled)
	}
}

func TestLifecycleRejectsNonOwnerInvalidIdentityAndDependency(t *testing.T) {
	store := newMemoryStore()
	reader := &memoryCatalog{}
	service := newWorkspaceTestService(t, store, reader)
	owner := AuthenticatedCaller{ID: "owner-a"}
	if _, err := service.CreateWorkspace(context.Background(), owner, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-policy"}); err != nil {
		t.Fatal(err)
	}
	store.installations["installation-policy"] = contracts.Installation{
		InstallationID: "installation-policy", WorkspaceID: "workspace-policy", AgentID: "agent-policy",
		VersionConstraint: "^1.0.0", InstalledVersion: "1.0.0", AcceptedPermissions: []string{"read"},
		Status: "enabled", InstalledAt: time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC),
	}

	cases := []struct {
		name                                string
		caller                              AuthenticatedCaller
		workspaceID, installationID, status string
		want                                error
	}{
		{name: "non-owner disable", caller: AuthenticatedCaller{ID: "owner-b"}, workspaceID: "workspace-policy", installationID: "installation-policy", status: "disabled", want: ErrForbidden},
		{name: "missing caller", caller: AuthenticatedCaller{}, workspaceID: "workspace-policy", installationID: "installation-policy", status: "disabled", want: ErrInvalid},
		{name: "invalid target", caller: owner, workspaceID: "workspace-policy", installationID: "installation-policy", status: "uninstalled", want: ErrInvalid},
		{name: "unknown installation", caller: owner, workspaceID: "workspace-policy", installationID: "missing-installation", status: "disabled", want: ErrNotFound},
		{name: "wrong workspace", caller: owner, workspaceID: "other-workspace", installationID: "installation-policy", status: "disabled", want: ErrNotFound},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.workspaceID == "other-workspace" {
				if _, err := service.CreateWorkspace(context.Background(), owner, contracts.CreateWorkspaceRequest{WorkspaceID: test.workspaceID}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := service.UpdateInstallation(context.Background(), test.caller, test.workspaceID, test.installationID, test.status)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}

	dependencyStore := &lifecycleDependencyStore{Store: store, workspace: contracts.Workspace{WorkspaceID: "workspace-policy", OwnerID: "owner-a"}, changeErr: ErrDependency, uninstallErr: ErrDependency}
	dependencyService := newWorkspaceTestService(t, dependencyStore, reader)
	if _, err := dependencyService.UpdateInstallation(context.Background(), owner, "workspace-policy", "installation-policy", "disabled"); !errors.Is(err, ErrDependency) {
		t.Fatalf("transition dependency = %v, want dependency", err)
	}
	if _, err := dependencyService.Uninstall(context.Background(), owner, "workspace-policy", "installation-policy"); !errors.Is(err, ErrDependency) {
		t.Fatalf("uninstall dependency = %v, want dependency", err)
	}
}

type lifecycleDependencyStore struct {
	Store
	workspace    contracts.Workspace
	changeErr    error
	uninstallErr error
}

func (store *lifecycleDependencyStore) GetWorkspace(context.Context, string) (contracts.Workspace, error) {
	return store.workspace, nil
}

func (store *lifecycleDependencyStore) ChangeInstallationStatus(context.Context, string, string, string, time.Time) (contracts.Installation, error) {
	return contracts.Installation{}, store.changeErr
}

func (store *lifecycleDependencyStore) UninstallInstallation(context.Context, string, string, time.Time) (contracts.Installation, error) {
	return contracts.Installation{}, store.uninstallErr
}

func assertImmutableInstallationFacts(t *testing.T, before, after contracts.Installation) {
	t.Helper()
	if before.InstallationID != after.InstallationID || before.WorkspaceID != after.WorkspaceID || before.AgentID != after.AgentID || before.VersionConstraint != after.VersionConstraint || before.InstalledVersion != after.InstalledVersion || len(before.AcceptedPermissions) != len(after.AcceptedPermissions) {
		t.Fatalf("immutable fields changed: before=%#v after=%#v", before, after)
	}
	for index := range before.AcceptedPermissions {
		if before.AcceptedPermissions[index] != after.AcceptedPermissions[index] {
			t.Fatalf("permission %d changed: before=%#v after=%#v", index, before, after)
		}
	}
}

func TestListCursorBindsWorkspaceAndLimit(t *testing.T) {
	reader := &memoryCatalog{versions: map[string]catalog.AgentVersion{}}
	store := newMemoryStore()
	service := newWorkspaceTestService(t, store, reader)
	caller := AuthenticatedCaller{ID: "owner-a"}
	_, _ = service.CreateWorkspace(context.Background(), caller, contracts.CreateWorkspaceRequest{WorkspaceID: "workspace-a"})
	for _, id := range []string{"installation-a", "installation-b"} {
		store.installations[id] = contracts.Installation{InstallationID: id, WorkspaceID: "workspace-a", AgentID: id, VersionConstraint: "^1.0.0", InstalledVersion: "1.0.0", Status: "enabled", InstalledAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	}
	first, err := service.ListInstallations(context.Background(), caller, "workspace-a", 1, nil)
	if err != nil || len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatalf("first page = %#v, %v", first, err)
	}
	second, err := service.ListInstallations(context.Background(), caller, "workspace-a", 1, first.NextCursor)
	if err != nil || len(second.Items) != 1 || second.Items[0].InstallationID != "installation-b" {
		t.Fatalf("second page = %#v, %v", second, err)
	}
	if _, err := service.ListInstallations(context.Background(), caller, "workspace-a", 2, first.NextCursor); !errors.Is(err, ErrInvalid) {
		t.Fatalf("mismatched page size = %v", err)
	}
}

func newWorkspaceTestService(t *testing.T, store Store, reader CatalogReader) *Service {
	return newWorkspaceTestServiceWithClock(t, store, reader, func() time.Time {
		return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	})
}

func newWorkspaceTestServiceWithClock(t *testing.T, store Store, reader CatalogReader, clock Clock) *Service {
	t.Helper()
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	service, err := NewService(store, reader, OwnerPolicy{}, validator, clock, func() (string, error) {
		sequence++
		return fmt.Sprintf("installation-generated-%d", sequence), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func testWorkspaceCard(agentID, version string, permissions, required []string) contracts.AgentCard {
	declared := make([]contracts.PermissionDeclaration, 0, len(permissions))
	for _, permission := range permissions {
		declared = append(declared, contracts.PermissionDeclaration{ID: permission, Description: permission})
	}
	return contracts.AgentCard{SchemaVersion: "0.2", AgentID: agentID, Name: "Workspace Agent", Description: "Workspace test agent", Owner: contracts.AgentOwner{ID: "owner-a", DisplayName: "Owner"}, Version: version, Protocol: contracts.AgentProtocol{Type: "a2a", Version: "0.3.0", Transport: "JSONRPC", Endpoint: "https://agent.example.test/a2a"}, Skills: []contracts.AgentSkill{{ID: "capability.read", Name: "Read", Description: "Read", InputSchema: contracts.JSONSchema{"type": "object"}, OutputSchema: contracts.JSONSchema{"type": "object"}, RequiredPermissions: required}}, Authentication: contracts.AgentAuthentication{Type: "none"}, Permissions: declared, Limits: contracts.AgentLimits{TimeoutMS: 1000, MaxInputBytes: json.Number("1000"), MaxOutputBytes: json.Number("1000"), Streaming: false}}
}
