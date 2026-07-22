package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

type releaseMemoryStore struct{ values map[string]AgentRelease }

func (store *releaseMemoryStore) CreateRelease(_ context.Context, value AgentRelease) (AgentRelease, error) {
	if _, exists := store.values[value.ReleaseID]; exists {
		return AgentRelease{}, ErrReleaseConflict
	}
	for _, existing := range store.values {
		if existing.AgentID == value.AgentID && existing.AgentCardVersion == value.AgentCardVersion {
			return AgentRelease{}, ErrReleaseConflict
		}
	}
	store.values[value.ReleaseID] = value
	return value, nil
}

func (store *releaseMemoryStore) GetRelease(_ context.Context, releaseID string) (AgentRelease, error) {
	value, exists := store.values[releaseID]
	if !exists {
		return AgentRelease{}, ErrReleaseNotFound
	}
	return value, nil
}

func (store *releaseMemoryStore) TransitionRelease(_ context.Context, releaseID string, target ReleaseState, evidence *[32]byte, at time.Time) (AgentRelease, error) {
	value, err := store.GetRelease(context.Background(), releaseID)
	if err != nil {
		return AgentRelease{}, err
	}
	value.State = target
	value.UpdatedAt = at
	if evidence != nil {
		value.VerificationEvidenceDigest = evidence
	}
	switch target {
	case ReleaseVerified:
		value.VerifiedAt = &at
	case ReleasePublished:
		value.PublishedAt = &at
	case ReleaseSuspended:
		value.SuspendedAt = &at
	case ReleaseRevoked:
		value.RevokedAt = &at
	}
	store.values[releaseID] = value
	return value, nil
}

type releaseVersionReader struct{ value AgentVersion }

func (reader releaseVersionReader) Get(_ context.Context, agentID, version string) (AgentVersion, error) {
	if reader.value.Card.AgentID != agentID || reader.value.Card.Version != version {
		return AgentVersion{}, ErrNotFound
	}
	return reader.value, nil
}

func TestReleaseLifecycleCopiesTrustFactsAndRejectsIllegalTransitions(t *testing.T) {
	clockValue := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return clockValue }
	digest := [32]byte{1, 2, 3}
	evidence := [32]byte{9, 8, 7}
	trust := newMemoryTrustStore()
	trust.providers["provider-a"] = Provider{ProviderID: "provider-a", OwnerIdentity: "owner-a", VerificationStatus: VerificationVerified}
	trust.bindings["binding-a"] = EndpointBinding{BindingID: "binding-a", ProviderID: "provider-a", AgentID: "agent-a", AgentCardVersion: "1.0.0", Origin: "https://agent.example", Path: "/a2a", VerificationMethod: VerificationMethodHTTPWellKnown, VerificationStatus: VerificationPending}
	versions := releaseVersionReader{value: AgentVersion{Card: contracts.AgentCard{AgentID: "agent-a", Version: "1.0.0"}, CardDigest: digest}}
	store := &releaseMemoryStore{values: make(map[string]AgentRelease)}
	service, err := NewReleaseService(store, versions, trust, clock)
	if err != nil {
		t.Fatal(err)
	}
	release, err := service.CreateRelease(context.Background(), AuthenticatedCaller{ID: "owner-a"}, "provider-a", "agent-a", contracts.CreateAgentReleaseRequest{Version: "1.0.0", EndpointBindingID: "binding-a"})
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if release.State != ReleasePendingVerification || release.CardDigest != digest || release.EndpointOrigin != "https://agent.example" || release.EndpointPath != "/a2a" {
		t.Fatalf("release facts = %#v", release)
	}
	if _, err := service.PublishRelease(context.Background(), AuthenticatedCaller{ID: "owner-a"}, release.ReleaseID); !errors.Is(err, ErrReleaseConflict) {
		t.Fatalf("publish pending error = %v, want conflict", err)
	}
	trust.providers["provider-a"] = Provider{ProviderID: "provider-a", OwnerIdentity: "owner-a", VerificationStatus: VerificationVerified}
	verifiedBinding := trust.bindings["binding-a"]
	verifiedBinding.VerificationStatus = VerificationVerified
	verifiedBinding.VerificationEvidenceDigest = &evidence
	verifiedBinding.VerifiedAt = &clockValue
	trust.bindings["binding-a"] = verifiedBinding
	verified, err := service.VerifyRelease(context.Background(), AuthenticatedCaller{ID: "owner-a"}, release.ReleaseID)
	if err != nil || verified.State != ReleaseVerified {
		t.Fatalf("VerifyRelease = %#v, %v", verified, err)
	}
	if verified.VerificationEvidenceDigest == nil || *verified.VerificationEvidenceDigest != evidence {
		t.Fatalf("evidence digest = %#v", verified.VerificationEvidenceDigest)
	}
	published, err := service.PublishRelease(context.Background(), AuthenticatedCaller{ID: "owner-a"}, release.ReleaseID)
	if err != nil || published.State != ReleasePublished {
		t.Fatalf("PublishRelease = %#v, %v", published, err)
	}
	if _, err := service.PublishRelease(context.Background(), AuthenticatedCaller{ID: "owner-a"}, release.ReleaseID); !errors.Is(err, ErrReleaseConflict) {
		t.Fatalf("repeated publish error = %v, want conflict", err)
	}
	suspended, err := service.SuspendRelease(context.Background(), AuthenticatedCaller{ID: "owner-a"}, release.ReleaseID)
	if err != nil || suspended.State != ReleaseSuspended {
		t.Fatalf("SuspendRelease = %#v, %v", suspended, err)
	}
	revoked, err := service.RevokeRelease(context.Background(), AuthenticatedCaller{ID: "owner-a"}, release.ReleaseID)
	if err != nil || revoked.State != ReleaseRevoked {
		t.Fatalf("RevokeRelease = %#v, %v", revoked, err)
	}
	if _, err := service.SuspendRelease(context.Background(), AuthenticatedCaller{ID: "owner-a"}, release.ReleaseID); !errors.Is(err, ErrReleaseConflict) {
		t.Fatalf("suspend revoked error = %v, want conflict", err)
	}
}

func TestCreateReleaseRejectsBindingVersionMismatchAndCrossOwner(t *testing.T) {
	trust := newMemoryTrustStore()
	trust.providers["provider-a"] = Provider{ProviderID: "provider-a", OwnerIdentity: "owner-a", VerificationStatus: VerificationVerified}
	trust.bindings["binding-a"] = EndpointBinding{BindingID: "binding-a", ProviderID: "provider-a", AgentID: "agent-other", AgentCardVersion: "1.0.0", VerificationStatus: VerificationVerified, VerificationEvidenceDigest: func() *[32]byte { value := [32]byte{1}; return &value }()}
	store := &releaseMemoryStore{values: make(map[string]AgentRelease)}
	service, err := NewReleaseService(store, releaseVersionReader{value: AgentVersion{Card: contracts.AgentCard{AgentID: "agent-a", Version: "1.0.0"}}}, trust, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateRelease(context.Background(), AuthenticatedCaller{ID: "owner-a"}, "provider-a", "agent-a", contracts.CreateAgentReleaseRequest{Version: "1.0.0", EndpointBindingID: "binding-a"}); !errors.Is(err, ErrReleaseInvalid) {
		t.Fatalf("binding mismatch error = %v, want invalid", err)
	}
	if _, err := service.CreateRelease(context.Background(), AuthenticatedCaller{ID: "owner-b"}, "provider-a", "agent-a", contracts.CreateAgentReleaseRequest{Version: "1.0.0", EndpointBindingID: "binding-a"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-owner error = %v, want forbidden", err)
	}
}
