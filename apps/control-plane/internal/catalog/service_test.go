package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

type fakeStore struct {
	registered   AgentVersion
	registerErr  error
	getEntry     AgentVersion
	getErr       error
	published    []AgentVersion
	publishedErr error
	publish      AgentVersion
	publishErr   error
	publishID    string
	publishTime  time.Time
	disable      AgentVersion
	disableErr   error
	disableID    string
	disableTime  time.Time
	snapshot     int64
	discovery    DiscoveryResult
	discoverErr  error
	registers    int
}

func (store *fakeStore) Register(_ context.Context, version AgentVersion) (AgentVersion, error) {
	store.registers++
	store.registered = version
	return version, store.registerErr
}
func (store *fakeStore) Get(context.Context, string, string) (AgentVersion, error) {
	return store.getEntry, store.getErr
}
func (store *fakeStore) InstallationCandidates(context.Context, string) ([]AgentVersion, error) {
	return store.published, store.publishedErr
}
func (store *fakeStore) Publish(_ context.Context, _, _ string, callerID string, at time.Time) (AgentVersion, error) {
	store.publishID = callerID
	store.publishTime = at
	return store.publish, store.publishErr
}
func (store *fakeStore) Disable(_ context.Context, _, _ string, callerID string, at time.Time) (AgentVersion, error) {
	store.disableID = callerID
	store.disableTime = at
	return store.disable, store.disableErr
}
func (store *fakeStore) DiscoverFirstPage(context.Context, DiscoveryFilter) (int64, DiscoveryResult, error) {
	return store.snapshot, store.discovery, store.discoverErr
}
func (store *fakeStore) Discover(context.Context, DiscoveryQuery) (DiscoveryResult, error) {
	return store.discovery, store.discoverErr
}
func (store *fakeStore) Check(context.Context) error { return nil }

func TestServiceRegisterValidatesOwnerAndImmutabilityInput(t *testing.T) {
	store := &fakeStore{}
	registeredAt := time.Date(2026, 7, 14, 2, 0, 0, 0, time.UTC)
	service := newTestService(t, store, registeredAt)
	card := testCard("agent-a", "owner-a", "1.0.0")
	body := registerBody(t, card)

	entry, err := service.Register(context.Background(), AuthenticatedCaller{ID: "owner-a"}, body)
	if err != nil {
		t.Fatalf("register valid Card: %v", err)
	}
	if entry.PublicationStatus != "draft" || !entry.RegisteredAt.Equal(registeredAt) {
		t.Fatalf("registered entry = %#v", entry)
	}
	if store.registered.CardDigest == ([32]byte{}) || store.registered.CardJSON == nil {
		t.Fatal("canonical Card digest or JSON was not assigned")
	}

	if _, err := service.Register(context.Background(), AuthenticatedCaller{ID: "other-owner"}, body); !errors.Is(err, ErrForbidden) {
		t.Fatalf("owner mismatch error = %v, want forbidden", err)
	}
	if store.registers != 1 {
		t.Fatalf("store registrations = %d, want 1", store.registers)
	}

	invalid := append(body[:len(body)-1], []byte(`,"unknown":true}`)...)
	if _, err := service.Register(context.Background(), AuthenticatedCaller{ID: "owner-a"}, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown request field error = %v, want invalid", err)
	}
	duplicate := []byte(`{"card":` + string(mustJSON(t, card)) + `,"card":` + string(mustJSON(t, card)) + `}`)
	if _, err := service.Register(context.Background(), AuthenticatedCaller{ID: "owner-a"}, duplicate); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate request member error = %v, want invalid", err)
	}
}

func TestServiceExactReadVisibilityAndLifecycleErrors(t *testing.T) {
	owner := AuthenticatedCaller{ID: "owner-a"}
	other := AuthenticatedCaller{ID: "owner-b"}
	for _, status := range []PublicationStatus{PublicationDraft, PublicationDisabled} {
		store := &fakeStore{getEntry: AgentVersion{Card: testCard("agent-a", owner.ID, "1.0.0"), Status: status}}
		service := newTestService(t, store, time.Now())
		if _, err := service.Get(context.Background(), other, "agent-a", "1.0.0"); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s non-owner read error = %v", status, err)
		}
		if _, err := service.Get(context.Background(), owner, "agent-a", "1.0.0"); err != nil {
			t.Fatalf("%s owner read: %v", status, err)
		}
	}

	store := &fakeStore{getEntry: AgentVersion{Card: testCard("agent-a", owner.ID, "1.0.0"), Status: PublicationPublished}}
	service := newTestService(t, store, time.Now())
	if _, err := service.Get(context.Background(), other, "agent-a", "1.0.0"); err != nil {
		t.Fatalf("published non-owner read: %v", err)
	}
	if _, err := service.Publish(context.Background(), owner, "bad/id", "1.0.0"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid publish identity error = %v", err)
	}
	store.publishErr = ErrConflict
	if _, err := service.Publish(context.Background(), owner, "agent-a", "1.0.0"); !errors.Is(err, ErrConflict) {
		t.Fatalf("publish conflict = %v", err)
	}
}

func TestServiceSearchReturnsExplicitItemsAndCursor(t *testing.T) {
	publishedAt := time.Date(2026, 7, 14, 2, 0, 0, 0, time.UTC)
	store := &fakeStore{
		snapshot: 8,
		discovery: DiscoveryResult{HasMore: true, Versions: []AgentVersion{{
			Card: testCard("agent-a", "owner-a", "1.0.0"), Status: PublicationPublished, PublishedAt: &publishedAt,
		}}},
	}
	service := newTestService(t, store, publishedAt)
	limit := 1
	result, err := service.Search(context.Background(), contracts.SearchAgentsQuery{Limit: &limit})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Entries) != 1 || result.NextCursor == nil {
		t.Fatalf("search result = %#v", result)
	}

	store.discovery = DiscoveryResult{Versions: []AgentVersion{}}
	result, err = service.Search(context.Background(), contracts.SearchAgentsQuery{})
	if err != nil {
		t.Fatalf("empty search: %v", err)
	}
	if result.Entries == nil || len(result.Entries) != 0 || result.NextCursor != nil {
		t.Fatalf("empty search result = %#v", result)
	}
}

func TestSelectInstallableGatesHighestMatchingReleaseState(t *testing.T) {
	card := testCard("agent-release-gate", "owner-a", "1.0.0")
	digest := [32]byte{1}
	tests := []struct {
		name  string
		state ReleaseState
		want  error
	}{
		{name: "unverified", want: ErrReleaseUnpublished},
		{name: "pending", state: ReleasePendingVerification, want: ErrReleaseUnpublished},
		{name: "suspended", state: ReleaseSuspended, want: ErrReleaseSuspended},
		{name: "revoked", state: ReleaseRevoked, want: ErrReleaseRevoked},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := AgentVersion{Card: card, CardDigest: digest, Status: PublicationPublished}
			if test.name == "unverified" {
				candidate.Release = nil
			} else {
				candidate.Release = &AgentRelease{ReleaseID: "release-" + test.name, AgentID: card.AgentID, AgentCardVersion: card.Version, CardDigest: digest, State: test.state}
			}
			service := newTestService(t, &fakeStore{published: []AgentVersion{candidate}}, time.Now())
			if _, err := service.SelectInstallable(context.Background(), card.AgentID, "^1.0.0"); !errors.Is(err, test.want) {
				t.Fatalf("selection error = %v, want %v", err, test.want)
			}
		})
	}

	trusted := AgentVersion{Card: card, CardDigest: digest, Status: PublicationPublished, Release: &AgentRelease{
		ReleaseID: "release-trusted", AgentID: card.AgentID, AgentCardVersion: card.Version, CardDigest: digest, State: ReleasePublished,
	}}
	higher := card
	higher.Version = "2.0.0"
	service := newTestService(t, &fakeStore{published: []AgentVersion{
		trusted,
		{Card: higher, Status: PublicationPublished},
	}}, time.Now())
	if _, err := service.SelectInstallable(context.Background(), card.AgentID, ">=1.0.0"); !errors.Is(err, ErrReleaseUnpublished) {
		t.Fatalf("highest untrusted version error = %v, want unpublished", err)
	}
}

func TestServiceLifecycleUsesTrustedCallerAndCommittedStoreResult(t *testing.T) {
	now := time.Date(2026, 7, 14, 2, 3, 4, 567890123, time.UTC)
	publishedAt := now.Truncate(time.Microsecond)
	card := testCard("agent-a", "owner-a", "1.0.0")
	store := &fakeStore{
		publish: AgentVersion{Card: card, Status: PublicationPublished, PublishedAt: &publishedAt},
		disable: AgentVersion{Card: card, Status: PublicationDisabled, PublishedAt: &publishedAt},
	}
	service := newTestService(t, store, now)
	caller := AuthenticatedCaller{ID: "owner-a"}
	published, err := service.Publish(context.Background(), caller, card.AgentID, card.Version)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if store.publishID != caller.ID || !store.publishTime.Equal(now) || published.PublishedAt == nil || !published.PublishedAt.Equal(publishedAt) {
		t.Fatalf("publish delegation/result = caller %q time %s entry %#v", store.publishID, store.publishTime, published)
	}
	disabled, err := service.Disable(context.Background(), caller, card.AgentID, card.Version)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if store.disableID != caller.ID || !store.disableTime.Equal(now) || disabled.PublicationStatus != "disabled" {
		t.Fatalf("disable delegation/result = caller %q time %s entry %#v", store.disableID, store.disableTime, disabled)
	}

	for _, domainErr := range []error{ErrForbidden, ErrNotFound, ErrConflict, ErrDependency} {
		store.publishErr = domainErr
		if _, err := service.Publish(context.Background(), caller, card.AgentID, card.Version); !errors.Is(err, domainErr) {
			t.Fatalf("publish error = %v, want %v", err, domainErr)
		}
	}
}

func newTestService(t *testing.T, store Store, now time.Time) *Service {
	t.Helper()
	validator, err := contracts.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store, validator, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func testCard(agentID, ownerID, version string) contracts.AgentCard {
	return contracts.AgentCard{
		SchemaVersion: "0.2", AgentID: agentID, Name: "Test Agent", Description: "Tests Catalog behavior.",
		Owner: contracts.AgentOwner{ID: ownerID, DisplayName: "Test Owner"}, Version: version,
		Protocol: contracts.AgentProtocol{Type: "a2a", Version: "0.3.0", Transport: "JSONRPC", Endpoint: "https://agent.example.test/a2a"},
		Skills: []contracts.AgentSkill{{
			ID: "document.summarize", Name: "Summarize", Description: "Summarizes a document.",
			InputSchema:  contracts.JSONSchema{"type": "object", "maximum": json.Number("1e400")},
			OutputSchema: contracts.JSONSchema{"type": "object"}, RequiredPermissions: []string{"documents.read"},
		}},
		Authentication: contracts.AgentAuthentication{Type: "none"},
		Permissions:    []contracts.PermissionDeclaration{{ID: "documents.read", Description: "Read documents."}},
		Limits:         contracts.AgentLimits{TimeoutMS: 1000, MaxInputBytes: json.Number("1024"), MaxOutputBytes: json.Number("1024"), Streaming: false},
	}
}

func registerBody(t *testing.T, card contracts.AgentCard) []byte {
	t.Helper()
	return []byte(`{"card":` + string(mustJSON(t, card)) + `}`)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
