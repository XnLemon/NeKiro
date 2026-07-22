//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTrustedPublicationStorePersistsSingleUseVerification(t *testing.T) {
	ctx := context.Background()
	databaseURL := os.Getenv("NEKIRO_TEST_DATABASE_URL")
	if strings.TrimSpace(databaseURL) == "" {
		t.Fatal("NEKIRO_TEST_DATABASE_URL is required for integration tests")
	}
	configuration, err := pgx.ParseConfig(databaseURL)
	if err != nil || !strings.HasSuffix(configuration.Database, "_test") {
		t.Fatal("integration tests require a dedicated _test database")
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `DROP SCHEMA IF EXISTS catalog CASCADE`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, connection, "up"); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO catalog.agent_identities (agent_id, owner_id, created_at) VALUES ('agent-trust', 'provider-trust', now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO catalog.agent_versions (agent_id, version, schema_version, card, card_name, card_description, card_digest, publication_status, registered_at, legacy_unverified) VALUES ('agent-trust', '1.0.0', '0.2', '{}', 'Trust Agent', 'Trust Agent', $1, 'draft', now(), false)`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	binding, err := store.CreateBinding(ctx, catalog.Provider{ProviderID: "provider-trust", OwnerIdentity: "provider-trust", VerificationStatus: catalog.VerificationUnverified, VerificationMethod: catalog.VerificationMethodHTTPWellKnown, CreatedAt: now, UpdatedAt: now}, catalog.EndpointBinding{BindingID: "binding-trust", ProviderID: "provider-trust", AgentID: "agent-trust", AgentCardVersion: "1.0.0", Endpoint: "https://agent.example/a2a", Origin: "https://agent.example", Path: "/a2a", VerificationMethod: catalog.VerificationMethodHTTPWellKnown, VerificationStatus: catalog.VerificationPending, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	release, err := store.CreateRelease(ctx, catalog.AgentRelease{
		ReleaseID: "release-trust", ProviderID: "provider-trust", AgentID: "agent-trust",
		AgentCardVersion: "1.0.0", EndpointBindingID: binding.BindingID,
		EndpointOrigin: binding.Origin, EndpointPath: binding.Path,
		VerificationMethod: catalog.VerificationMethodHTTPWellKnown,
		State:              catalog.ReleasePendingVerification, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil || release.State != catalog.ReleasePendingVerification {
		t.Fatalf("create pending Release = %#v, %v", release, err)
	}
	if _, err := store.CreateRelease(ctx, catalog.AgentRelease{
		ReleaseID: "release-duplicate-version", ProviderID: "provider-trust", AgentID: "agent-trust",
		AgentCardVersion: "1.0.0", EndpointBindingID: binding.BindingID,
		EndpointOrigin: binding.Origin, EndpointPath: binding.Path,
		VerificationMethod: catalog.VerificationMethodHTTPWellKnown,
		State:              catalog.ReleasePendingVerification, CreatedAt: now, UpdatedAt: now,
	}); !errors.Is(err, catalog.ErrReleaseConflict) {
		t.Fatalf("duplicate Card-version Release error = %v", err)
	}
	proofDigest := sha256.Sum256([]byte("proof"))
	challenge := catalog.VerificationChallenge{ChallengeID: "challenge-trust", BindingID: binding.BindingID, ProofDigest: proofDigest, ExpiresAt: now.Add(time.Minute), CreatedAt: now}
	if err := store.CreateChallenge(ctx, challenge); err != nil {
		t.Fatal(err)
	}
	reserved, _, err := store.ReserveChallenge(ctx, binding.BindingID, challenge.ChallengeID, now.Add(time.Second))
	if err != nil || reserved.UsedAt == nil {
		t.Fatalf("reserve challenge=%#v error=%v", reserved, err)
	}
	if _, _, err := store.ReserveChallenge(ctx, binding.BindingID, challenge.ChallengeID, now.Add(2*time.Second)); err != catalog.ErrChallengeReused {
		t.Fatalf("second reservation error=%v", err)
	}
	verified, err := store.SetBindingVerification(ctx, binding.BindingID, catalog.VerificationVerified, nil, &proofDigest, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if verified.VerificationStatus != catalog.VerificationVerified || verified.VerifiedAt == nil || verified.VerificationEvidenceDigest == nil || *verified.VerificationEvidenceDigest != proofDigest {
		t.Fatalf("verified binding=%#v", verified)
	}
	provider, err := store.GetProvider(ctx, "provider-trust")
	if err != nil || provider.VerificationStatus != catalog.VerificationVerified || provider.VerifiedAt == nil {
		t.Fatalf("verified provider=%#v error=%v", provider, err)
	}
	release, err = store.TransitionRelease(ctx, release.ReleaseID, catalog.ReleaseVerified, &proofDigest, now.Add(3*time.Second))
	if err != nil || release.State != catalog.ReleaseVerified || release.VerifiedAt == nil || release.VerificationEvidenceDigest == nil || *release.VerificationEvidenceDigest != proofDigest {
		t.Fatalf("verify Release = %#v, %v", release, err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO catalog.agent_versions (agent_id, version, schema_version, card, card_name, card_description, card_digest, publication_status, registered_at, legacy_unverified)
VALUES ('agent-trust', '2.0.0', '0.2', '{}', 'Trust Agent v2', 'Trust Agent v2', $1, 'draft', $2, false)`, make([]byte, 32), now); err != nil {
		t.Fatal(err)
	}
	bindingVerified, err := store.CreateBinding(ctx, catalog.Provider{
		ProviderID: "provider-trust", OwnerIdentity: "provider-trust", VerificationStatus: catalog.VerificationVerified,
		VerificationMethod: catalog.VerificationMethodHTTPWellKnown, VerifiedAt: release.VerifiedAt, CreatedAt: now, UpdatedAt: now,
	}, catalog.EndpointBinding{
		BindingID: "binding-trust-verified", ProviderID: "provider-trust", AgentID: "agent-trust", AgentCardVersion: "2.0.0",
		Endpoint: "https://agent.example/a2a-v2", Origin: "https://agent.example", Path: "/a2a-v2",
		VerificationMethod: catalog.VerificationMethodHTTPWellKnown, VerificationStatus: catalog.VerificationPending, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetBindingVerification(ctx, bindingVerified.BindingID, catalog.VerificationVerified, nil, &proofDigest, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	verifiedOnly, err := store.CreateRelease(ctx, catalog.AgentRelease{
		ReleaseID: "release-trust-verified", ProviderID: "provider-trust", AgentID: "agent-trust", AgentCardVersion: "2.0.0",
		EndpointBindingID: bindingVerified.BindingID, EndpointOrigin: bindingVerified.Origin, EndpointPath: bindingVerified.Path,
		VerificationMethod: catalog.VerificationMethodHTTPWellKnown, State: catalog.ReleasePendingVerification, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	verifiedOnly, err = store.TransitionRelease(ctx, verifiedOnly.ReleaseID, catalog.ReleaseVerified, &proofDigest, now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	verifiedOnly, err = store.TransitionRelease(ctx, verifiedOnly.ReleaseID, catalog.ReleaseSuspended, nil, now.Add(5*time.Second))
	if err != nil || verifiedOnly.State != catalog.ReleaseSuspended || verifiedOnly.PublishedAt != nil || verifiedOnly.SuspendedAt == nil {
		t.Fatalf("suspend verified-only Release = %#v, %v", verifiedOnly, err)
	}
	verifiedOnly, err = store.TransitionRelease(ctx, verifiedOnly.ReleaseID, catalog.ReleaseRevoked, nil, now.Add(6*time.Second))
	if err != nil || verifiedOnly.State != catalog.ReleaseRevoked || verifiedOnly.PublishedAt != nil || verifiedOnly.SuspendedAt == nil || verifiedOnly.RevokedAt == nil {
		t.Fatalf("revoke suspended verified-only Release = %#v, %v", verifiedOnly, err)
	}

	var wait sync.WaitGroup
	wait.Add(2)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			defer wait.Done()
			_, transitionErr := store.TransitionRelease(ctx, release.ReleaseID, catalog.ReleasePublished, nil, now.Add(4*time.Second))
			results <- transitionErr
		}()
	}
	wait.Wait()
	close(results)
	var publishedCount, conflictCount int
	for transitionErr := range results {
		switch {
		case transitionErr == nil:
			publishedCount++
		case errors.Is(transitionErr, catalog.ErrReleaseConflict):
			conflictCount++
		default:
			t.Fatalf("concurrent publish error = %v", transitionErr)
		}
	}
	if publishedCount != 1 || conflictCount != 1 {
		t.Fatalf("concurrent publish successes=%d conflicts=%d", publishedCount, conflictCount)
	}
	var publicationStatus string
	var publicationSequence int64
	var legacyUnverified bool
	if err := pool.QueryRow(ctx, `SELECT publication_status, publication_sequence, legacy_unverified FROM catalog.agent_versions WHERE agent_id = 'agent-trust' AND version = '1.0.0'`).Scan(&publicationStatus, &publicationSequence, &legacyUnverified); err != nil {
		t.Fatal(err)
	}
	if publicationStatus != "published" || publicationSequence <= 0 || legacyUnverified {
		t.Fatalf("trusted publication projection status=%q sequence=%v legacy=%t", publicationStatus, publicationSequence, legacyUnverified)
	}
	if _, err := pool.Exec(ctx, `UPDATE catalog.agent_releases SET endpoint_path = '/changed' WHERE release_id = 'release-trust'`); err == nil {
		t.Fatal("immutable Release endpoint mutation succeeded")
	}
	release, err = store.TransitionRelease(ctx, release.ReleaseID, catalog.ReleaseSuspended, nil, now.Add(5*time.Second))
	if err != nil || release.State != catalog.ReleaseSuspended || release.SuspendedAt == nil {
		t.Fatalf("suspend Release = %#v, %v", release, err)
	}
	release, err = store.TransitionRelease(ctx, release.ReleaseID, catalog.ReleaseRevoked, nil, now.Add(6*time.Second))
	if err != nil || release.State != catalog.ReleaseRevoked || release.RevokedAt == nil {
		t.Fatalf("revoke Release = %#v, %v", release, err)
	}
	if _, err := store.TransitionRelease(ctx, release.ReleaseID, catalog.ReleaseSuspended, nil, now.Add(7*time.Second)); !errors.Is(err, catalog.ErrReleaseConflict) {
		t.Fatalf("revoked Release transition error = %v", err)
	}
	failure := "wrong_proof"
	if _, err := store.SetBindingVerification(ctx, binding.BindingID, catalog.VerificationFailed, &failure, nil, now.Add(8*time.Second)); err != catalog.ErrTrustConflict {
		t.Fatalf("verified binding overwrite error=%v", err)
	}
}
