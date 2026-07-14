package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/contracts"
)

type Service struct {
	store      Store
	catalog    CatalogReader
	authorizer Authorizer
	validator  *contracts.Validator
	clock      Clock
	newID      IDGenerator
}

func NewService(store Store, catalogReader CatalogReader, authorizer Authorizer, validator *contracts.Validator, clock Clock, newID IDGenerator) (*Service, error) {
	if store == nil || catalogReader == nil || authorizer == nil || validator == nil || clock == nil || newID == nil {
		return nil, errors.New("workspace service dependencies are required")
	}
	return &Service{store: store, catalog: catalogReader, authorizer: authorizer, validator: validator, clock: clock, newID: newID}, nil
}

func NewRandomID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate identifier: %w", err)
	}
	return "ws_" + hex.EncodeToString(data), nil
}

func (service *Service) CreateWorkspace(ctx context.Context, caller AuthenticatedCaller, request contracts.CreateWorkspaceRequest) (contracts.Workspace, error) {
	if !ValidIdentifier(request.WorkspaceID) || caller.ID == "" || !ValidIdentifier(caller.ID) {
		return contracts.Workspace{}, ErrInvalid
	}
	now := service.clock().UTC()
	value := contracts.Workspace{
		WorkspaceID: request.WorkspaceID, OwnerID: caller.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err := service.validator.ValidateWorkspace(value); err != nil {
		return contracts.Workspace{}, ErrInvalid
	}
	return service.store.CreateWorkspace(ctx, value)
}

func (service *Service) GetWorkspace(ctx context.Context, caller AuthenticatedCaller, workspaceID string) (contracts.Workspace, error) {
	if !ValidIdentifier(workspaceID) || !ValidIdentifier(caller.ID) {
		return contracts.Workspace{}, ErrInvalid
	}
	workspace, err := service.store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return contracts.Workspace{}, err
	}
	if err := service.authorizer.Authorize(workspace.OwnerID, caller.ID); err != nil {
		return contracts.Workspace{}, err
	}
	return workspace, nil
}

func (service *Service) Install(ctx context.Context, caller AuthenticatedCaller, workspaceID string, request contracts.InstallAgentRequest) (contracts.Installation, error) {
	if !ValidIdentifier(workspaceID) || !ValidIdentifier(caller.ID) || !ValidIdentifier(request.AgentID) || !validConstraint(request.VersionConstraint) || !validPermissionInput(request.AcceptedPermissions) {
		return contracts.Installation{}, ErrInvalid
	}
	workspace, err := service.store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return contracts.Installation{}, err
	}
	if err := service.authorizer.Authorize(workspace.OwnerID, caller.ID); err != nil {
		return contracts.Installation{}, err
	}
	current, err := service.store.HasCurrentInstallation(ctx, workspaceID, request.AgentID)
	if err != nil {
		return contracts.Installation{}, err
	}
	if current {
		return contracts.Installation{}, ErrConflict
	}
	selected, err := service.catalog.SelectPublished(ctx, request.AgentID, request.VersionConstraint)
	if err != nil {
		return contracts.Installation{}, mapCatalogError(err)
	}
	if selected.Status != catalog.PublicationPublished {
		return contracts.Installation{}, ErrDependency
	}
	permissions, err := validatePermissionSubset(selected.Card, request.AcceptedPermissions)
	if err != nil {
		return contracts.Installation{}, err
	}
	installationID, err := service.newID()
	if err != nil {
		return contracts.Installation{}, fmt.Errorf("generate Installation identifier: %w", errors.Join(ErrDependency, err))
	}
	if !ValidIdentifier(installationID) {
		return contracts.Installation{}, fmt.Errorf("generate Installation identifier: %w", ErrDependency)
	}
	now := service.clock().UTC()
	installation := contracts.Installation{
		InstallationID: installationID, WorkspaceID: workspaceID, AgentID: request.AgentID,
		VersionConstraint: request.VersionConstraint, InstalledVersion: selected.Card.Version,
		AcceptedPermissions: permissions, Status: "enabled", InstalledAt: now, UpdatedAt: now,
	}
	if err := service.validator.ValidateInstallation(installation); err != nil {
		return contracts.Installation{}, ErrInvalid
	}
	return service.store.CreateInstallation(ctx, caller.ID, installation)
}

func (service *Service) GetInstallation(ctx context.Context, caller AuthenticatedCaller, workspaceID, installationID string) (contracts.Installation, error) {
	if !ValidIdentifier(workspaceID) || !ValidIdentifier(installationID) || !ValidIdentifier(caller.ID) {
		return contracts.Installation{}, ErrInvalid
	}
	workspace, err := service.store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return contracts.Installation{}, err
	}
	if err := service.authorizer.Authorize(workspace.OwnerID, caller.ID); err != nil {
		return contracts.Installation{}, err
	}
	return service.store.GetInstallation(ctx, workspaceID, installationID)
}

func (service *Service) ListInstallations(ctx context.Context, caller AuthenticatedCaller, workspaceID string, limit int, cursor *string) (contracts.InstallationList, error) {
	if !ValidIdentifier(workspaceID) || !ValidIdentifier(caller.ID) || limit < contracts.InstallationMinimumLimit || limit > contracts.InstallationMaximumLimit {
		return contracts.InstallationList{}, ErrInvalid
	}
	var after *InstallationPosition
	if cursor != nil {
		position, err := DecodeInstallationCursor(*cursor, workspaceID, limit)
		if err != nil {
			return contracts.InstallationList{}, err
		}
		after = &position
	}
	workspace, err := service.store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return contracts.InstallationList{}, err
	}
	if err := service.authorizer.Authorize(workspace.OwnerID, caller.ID); err != nil {
		return contracts.InstallationList{}, err
	}
	items, hasMore, err := service.store.ListInstallations(ctx, workspaceID, limit, after)
	if err != nil {
		return contracts.InstallationList{}, err
	}
	result := contracts.InstallationList{Items: items}
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		cursorValue, err := EncodeInstallationCursor(workspaceID, limit, InstallationPosition{InstalledAt: last.InstalledAt, InstallationID: last.InstallationID})
		if err != nil {
			return contracts.InstallationList{}, fmt.Errorf("encode installation cursor: %w", ErrDependency)
		}
		result.NextCursor = &cursorValue
	}
	return result, nil
}

func (service *Service) UpdateInstallation(ctx context.Context, caller AuthenticatedCaller, workspaceID, installationID, status string) (contracts.Installation, error) {
	if !ValidIdentifier(workspaceID) || !ValidIdentifier(installationID) || !ValidIdentifier(caller.ID) || !currentInstallationStatus(status) {
		return contracts.Installation{}, ErrInvalid
	}
	workspace, err := service.store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return contracts.Installation{}, err
	}
	if err := service.authorizer.Authorize(workspace.OwnerID, caller.ID); err != nil {
		return contracts.Installation{}, err
	}
	return service.store.ChangeInstallationStatus(ctx, workspaceID, installationID, status, service.clock().UTC())
}

func (service *Service) Uninstall(ctx context.Context, caller AuthenticatedCaller, workspaceID, installationID string) (contracts.Installation, error) {
	if !ValidIdentifier(workspaceID) || !ValidIdentifier(installationID) || !ValidIdentifier(caller.ID) {
		return contracts.Installation{}, ErrInvalid
	}
	workspace, err := service.store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return contracts.Installation{}, err
	}
	if err := service.authorizer.Authorize(workspace.OwnerID, caller.ID); err != nil {
		return contracts.Installation{}, err
	}
	return service.store.UninstallInstallation(ctx, workspaceID, installationID, service.clock().UTC())
}

func (service *Service) Resolve(ctx context.Context, request contracts.ResolveAgentRequest) (contracts.ResolveAgentResponse, error) {
	if err := contracts.ValidateResolveAgentRequestV1(request); err != nil {
		return contracts.ResolveAgentResponse{}, ErrInvalid
	}
	installation, err := service.store.GetCurrentInstallation(ctx, request.WorkspaceID, request.AgentID)
	if errors.Is(err, ErrNotFound) {
		return contracts.ResolveAgentResponse{}, ErrAgentNotInstalled
	}
	if err != nil {
		return contracts.ResolveAgentResponse{}, err
	}
	if installation.InstalledVersion != request.Version {
		return contracts.ResolveAgentResponse{}, ErrAgentNotInstalled
	}
	if installation.Status != "enabled" {
		return contracts.ResolveAgentResponse{}, ErrInstallationDisabled
	}
	version, err := service.catalog.GetVersion(ctx, request.AgentID, request.Version)
	if errors.Is(err, catalog.ErrNotFound) {
		return contracts.ResolveAgentResponse{}, ErrDependency
	}
	if err != nil {
		return contracts.ResolveAgentResponse{}, mapCatalogError(err)
	}
	if version.Status != catalog.PublicationPublished {
		return contracts.ResolveAgentResponse{}, ErrAgentDisabled
	}
	var skill *contracts.AgentSkill
	for index := range version.Card.Skills {
		if version.Card.Skills[index].ID == request.Capability {
			skill = &version.Card.Skills[index]
			break
		}
	}
	if skill == nil || !containsAll(installation.AcceptedPermissions, skill.RequiredPermissions) {
		return contracts.ResolveAgentResponse{}, ErrCapabilityNotAllowed
	}
	response := contracts.ResolveAgentResponse{Card: version.Card, Installation: contracts.ResolvedInstallation{
		InstallationID: installation.InstallationID, WorkspaceID: installation.WorkspaceID,
		AgentID: installation.AgentID, InstalledVersion: installation.InstalledVersion,
		AcceptedPermissions: installation.AcceptedPermissions, Status: installation.Status,
	}}
	if err := service.validator.ValidateResolveAgentResponseForRequest(request, response); err != nil {
		return contracts.ResolveAgentResponse{}, ErrDependency
	}
	return response, nil
}

func validConstraint(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	_, err := semver.NewConstraint(value)
	return err == nil
}

func validPermissionInput(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !ValidIdentifier(value) {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validatePermissionSubset(card contracts.AgentCard, accepted []string) ([]string, error) {
	declared := make(map[string]struct{}, len(card.Permissions))
	for _, permission := range card.Permissions {
		declared[permission.ID] = struct{}{}
	}
	for _, permission := range accepted {
		if _, exists := declared[permission]; !exists {
			return nil, ErrInvalid
		}
	}
	canonical := append([]string(nil), accepted...)
	sort.Strings(canonical)
	return canonical, nil
}

func containsAll(accepted, required []string) bool {
	set := make(map[string]struct{}, len(accepted))
	for _, permission := range accepted {
		set[permission] = struct{}{}
	}
	for _, permission := range required {
		if _, exists := set[permission]; !exists {
			return false
		}
	}
	return true
}

func mapCatalogError(err error) error {
	switch {
	case errors.Is(err, catalog.ErrInvalid):
		return ErrInvalid
	case errors.Is(err, catalog.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, catalog.ErrConflict):
		return ErrConflict
	case errors.Is(err, catalog.ErrForbidden):
		return ErrForbidden
	case errors.Is(err, catalog.ErrDependency):
		return ErrDependency
	default:
		return ErrDependency
	}
}
