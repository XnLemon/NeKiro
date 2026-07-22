package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

// ReleaseStore owns immutable Agent Release facts and their state transitions.
// It is separate from Store so the legacy Card publication port remains
// compatible for existing sample Agents.
type ReleaseStore interface {
	CreateRelease(context.Context, AgentRelease) (AgentRelease, error)
	GetRelease(context.Context, string) (AgentRelease, error)
	TransitionRelease(context.Context, string, ReleaseState, *[32]byte, time.Time) (AgentRelease, error)
}

// ReleaseService validates the cross-boundary facts before asking Registry
// persistence to create or transition a release. Endpoint and provider facts
// are read through the TrustStore port; no trust table is accessed directly.
type ReleaseService struct {
	store    ReleaseStore
	versions AgentVersionReader
	trust    TrustStore
	clock    Clock
	newID    func(string) (string, error)
}

func NewReleaseService(store ReleaseStore, versions AgentVersionReader, trust TrustStore, clock Clock) (*ReleaseService, error) {
	if store == nil || versions == nil || trust == nil || clock == nil {
		return nil, errors.New("agent release dependencies are required")
	}
	return &ReleaseService{store: store, versions: versions, trust: trust, clock: clock, newID: newTrustID}, nil
}

func (service *ReleaseService) CreateRelease(ctx context.Context, caller AuthenticatedCaller, providerID, agentID string, request contracts.CreateAgentReleaseRequest) (AgentRelease, error) {
	if !ValidIdentifier(caller.ID) || !ValidIdentifier(providerID) || !ValidIdentifier(agentID) || !validAgentVersionIdentity(agentID, request.Version) || !ValidIdentifier(request.EndpointBindingID) {
		return AgentRelease{}, ErrReleaseInvalid
	}
	provider, binding, version, err := service.authorizeBinding(ctx, caller, providerID, agentID, request.Version, request.EndpointBindingID)
	if err != nil {
		return AgentRelease{}, err
	}
	state := ReleasePendingVerification
	if binding.VerificationStatus == VerificationVerified {
		state = ReleaseVerified
	}
	if binding.VerificationStatus != VerificationPending && binding.VerificationStatus != VerificationVerified {
		return AgentRelease{}, ErrReleaseConflict
	}
	now := service.clock().UTC()
	releaseID, err := service.newID("release")
	if err != nil {
		return AgentRelease{}, fmt.Errorf("generate Agent Release identifier: %w", ErrDependency)
	}
	release := AgentRelease{
		ReleaseID: releaseID, ProviderID: provider.ProviderID, AgentID: agentID,
		AgentCardVersion: request.Version, CardDigest: version.CardDigest,
		EndpointBindingID: binding.BindingID, EndpointOrigin: binding.Origin,
		EndpointPath: binding.Path, VerificationMethod: binding.VerificationMethod,
		VerificationEvidenceDigest: binding.VerificationEvidenceDigest,
		State:                      state, CreatedAt: now, UpdatedAt: now,
	}
	if state == ReleaseVerified {
		release.VerifiedAt = binding.VerifiedAt
	}
	return service.store.CreateRelease(ctx, release)
}

func (service *ReleaseService) GetRelease(ctx context.Context, caller AuthenticatedCaller, releaseID string) (AgentRelease, error) {
	if !ValidIdentifier(caller.ID) || !ValidIdentifier(releaseID) {
		return AgentRelease{}, ErrReleaseInvalid
	}
	release, err := service.store.GetRelease(ctx, releaseID)
	if err != nil {
		return AgentRelease{}, err
	}
	provider, err := service.trust.GetProvider(ctx, release.ProviderID)
	if err != nil {
		return AgentRelease{}, err
	}
	if provider.OwnerIdentity != caller.ID && release.State != ReleasePublished {
		return AgentRelease{}, ErrForbidden
	}
	return release, nil
}

func (service *ReleaseService) VerifyRelease(ctx context.Context, caller AuthenticatedCaller, releaseID string) (AgentRelease, error) {
	return service.transitionAfterBinding(ctx, caller, releaseID, ReleaseVerified)
}

func (service *ReleaseService) PublishRelease(ctx context.Context, caller AuthenticatedCaller, releaseID string) (AgentRelease, error) {
	release, err := service.authorizeRelease(ctx, caller, releaseID)
	if err != nil {
		return AgentRelease{}, err
	}
	if release.State != ReleaseVerified {
		return AgentRelease{}, ErrReleaseConflict
	}
	if err := service.requireCurrentVerifiedBinding(ctx, release); err != nil {
		return AgentRelease{}, err
	}
	return service.store.TransitionRelease(ctx, releaseID, ReleasePublished, nil, service.clock().UTC())
}

func (service *ReleaseService) SuspendRelease(ctx context.Context, caller AuthenticatedCaller, releaseID string) (AgentRelease, error) {
	release, err := service.authorizeRelease(ctx, caller, releaseID)
	if err != nil {
		return AgentRelease{}, err
	}
	if release.State != ReleaseVerified && release.State != ReleasePublished {
		return AgentRelease{}, ErrReleaseConflict
	}
	return service.store.TransitionRelease(ctx, releaseID, ReleaseSuspended, nil, service.clock().UTC())
}

func (service *ReleaseService) RevokeRelease(ctx context.Context, caller AuthenticatedCaller, releaseID string) (AgentRelease, error) {
	release, err := service.authorizeRelease(ctx, caller, releaseID)
	if err != nil {
		return AgentRelease{}, err
	}
	if release.State != ReleaseVerified && release.State != ReleasePublished && release.State != ReleaseSuspended {
		return AgentRelease{}, ErrReleaseConflict
	}
	return service.store.TransitionRelease(ctx, releaseID, ReleaseRevoked, nil, service.clock().UTC())
}

func (service *ReleaseService) transitionAfterBinding(ctx context.Context, caller AuthenticatedCaller, releaseID string, target ReleaseState) (AgentRelease, error) {
	release, err := service.authorizeRelease(ctx, caller, releaseID)
	if err != nil {
		return AgentRelease{}, err
	}
	if release.State != ReleasePendingVerification {
		return AgentRelease{}, ErrReleaseConflict
	}
	if err := service.requireCurrentVerifiedBinding(ctx, release); err != nil {
		return AgentRelease{}, err
	}
	binding, err := service.trust.GetBinding(ctx, release.ProviderID, release.EndpointBindingID)
	if err != nil || binding.VerificationEvidenceDigest == nil {
		if err != nil {
			return AgentRelease{}, err
		}
		return AgentRelease{}, ErrReleaseConflict
	}
	return service.store.TransitionRelease(ctx, releaseID, target, binding.VerificationEvidenceDigest, service.clock().UTC())
}

func (service *ReleaseService) authorizeRelease(ctx context.Context, caller AuthenticatedCaller, releaseID string) (AgentRelease, error) {
	if !ValidIdentifier(caller.ID) || !ValidIdentifier(releaseID) {
		return AgentRelease{}, ErrReleaseInvalid
	}
	release, err := service.store.GetRelease(ctx, releaseID)
	if err != nil {
		return AgentRelease{}, err
	}
	provider, err := service.trust.GetProvider(ctx, release.ProviderID)
	if err != nil {
		return AgentRelease{}, err
	}
	if provider.OwnerIdentity != caller.ID {
		return AgentRelease{}, ErrForbidden
	}
	return release, nil
}

func (service *ReleaseService) authorizeBinding(ctx context.Context, caller AuthenticatedCaller, providerID, agentID, version, bindingID string) (Provider, EndpointBinding, AgentVersion, error) {
	provider, err := service.trust.GetProvider(ctx, providerID)
	if err != nil {
		return Provider{}, EndpointBinding{}, AgentVersion{}, err
	}
	if provider.OwnerIdentity != caller.ID {
		return Provider{}, EndpointBinding{}, AgentVersion{}, ErrForbidden
	}
	if provider.VerificationStatus == VerificationSuspended {
		return Provider{}, EndpointBinding{}, AgentVersion{}, ErrForbidden
	}
	binding, err := service.trust.GetBinding(ctx, providerID, bindingID)
	if err != nil {
		return Provider{}, EndpointBinding{}, AgentVersion{}, err
	}
	if binding.AgentID != agentID || binding.AgentCardVersion != version {
		return Provider{}, EndpointBinding{}, AgentVersion{}, ErrReleaseInvalid
	}
	versionValue, err := service.versions.Get(ctx, agentID, version)
	if err != nil {
		return Provider{}, EndpointBinding{}, AgentVersion{}, err
	}
	return provider, binding, versionValue, nil
}

func (service *ReleaseService) requireCurrentVerifiedBinding(ctx context.Context, release AgentRelease) error {
	provider, err := service.trust.GetProvider(ctx, release.ProviderID)
	if err != nil {
		return err
	}
	if provider.VerificationStatus != VerificationVerified {
		return ErrReleaseConflict
	}
	binding, err := service.trust.GetBinding(ctx, release.ProviderID, release.EndpointBindingID)
	if err != nil {
		return err
	}
	if binding.VerificationStatus != VerificationVerified || binding.AgentID != release.AgentID || binding.AgentCardVersion != release.AgentCardVersion || binding.Origin != release.EndpointOrigin || binding.Path != release.EndpointPath || binding.VerificationEvidenceDigest == nil {
		return ErrReleaseConflict
	}
	if release.State == ReleasePendingVerification && release.VerificationEvidenceDigest == nil {
		return nil
	}
	if release.VerificationEvidenceDigest == nil || *release.VerificationEvidenceDigest != *binding.VerificationEvidenceDigest {
		return ErrReleaseConflict
	}
	return nil
}
