//go:build integration

package postgres

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCheckSchemaRejectsIncompleteSchemaV4(t *testing.T) {
	ctx := context.Background()
	databaseURL := os.Getenv("NEKIRO_TEST_DATABASE_URL")
	if strings.TrimSpace(databaseURL) == "" {
		t.Fatal("NEKIRO_TEST_DATABASE_URL is required for integration tests")
	}
	configuration, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("NEKIRO_TEST_DATABASE_URL is invalid")
	}
	if !strings.HasSuffix(configuration.Database, "_test") {
		t.Fatalf("integration database %q must end in _test", configuration.Database)
	}

	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect dedicated test database: %v", err)
	}
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, `DROP SCHEMA IF EXISTS catalog CASCADE`); err != nil {
		t.Fatalf("reset dedicated Catalog schema: %v", err)
	}
	if err := Migrate(ctx, connection, "up"); err != nil {
		t.Fatalf("migrate Catalog schema: %v", err)
	}
	if err := CheckSchema(ctx, connection); err != nil {
		t.Fatalf("valid schema was not ready: %v", err)
	}

	tests := []struct {
		name    string
		degrade string
		restore string
	}{
		{
			name:    "Card type",
			degrade: `ALTER TABLE catalog.agent_versions ALTER COLUMN card TYPE varchar USING card::varchar`,
			restore: `ALTER TABLE catalog.agent_versions ALTER COLUMN card TYPE text USING card::text`,
		},
		{
			name:    "Card name nullability",
			degrade: `ALTER TABLE catalog.agent_versions ALTER COLUMN card_name DROP NOT NULL`,
			restore: `ALTER TABLE catalog.agent_versions ALTER COLUMN card_name SET NOT NULL`,
		},
		{
			name:    "Card description nullability",
			degrade: `ALTER TABLE catalog.agent_versions ALTER COLUMN card_description DROP NOT NULL`,
			restore: `ALTER TABLE catalog.agent_versions ALTER COLUMN card_description SET NOT NULL`,
		},
		{
			name:    "Trusted publication foreign key",
			degrade: `ALTER TABLE catalog.endpoint_bindings DROP CONSTRAINT endpoint_bindings_provider_fk`,
			restore: `ALTER TABLE catalog.endpoint_bindings ADD CONSTRAINT endpoint_bindings_provider_fk FOREIGN KEY (provider_id) REFERENCES catalog.providers(provider_id)`,
		},
		{
			name:    "Trusted provider state check",
			degrade: `ALTER TABLE catalog.providers DROP CONSTRAINT providers_state_timestamps`,
			restore: `ALTER TABLE catalog.providers ADD CONSTRAINT providers_state_timestamps CHECK ((verification_status = 'unverified' AND verified_at IS NULL) OR (verification_status = 'verified' AND verified_at IS NOT NULL) OR verification_status = 'suspended')`,
		},
		{
			name:    "Trusted publication state check",
			degrade: `ALTER TABLE catalog.endpoint_bindings DROP CONSTRAINT endpoint_bindings_state_timestamps`,
			restore: `ALTER TABLE catalog.endpoint_bindings ADD CONSTRAINT endpoint_bindings_state_timestamps CHECK ((verification_status = 'pending' AND verification_evidence_digest IS NULL AND verification_failure_code IS NULL AND verified_at IS NULL AND revoked_at IS NULL) OR (verification_status = 'verified' AND verification_evidence_digest IS NOT NULL AND verification_failure_code IS NULL AND verified_at IS NOT NULL AND revoked_at IS NULL) OR (verification_status = 'failed' AND verification_evidence_digest IS NULL AND verification_failure_code IS NOT NULL AND verified_at IS NULL AND revoked_at IS NULL) OR (verification_status = 'revoked' AND verification_evidence_digest IS NULL AND verification_failure_code IS NULL AND verified_at IS NULL AND revoked_at IS NOT NULL))`,
		},
		{
			name:    "Trusted publication digest check",
			degrade: `ALTER TABLE catalog.verification_challenges DROP CONSTRAINT verification_challenges_proof_digest_length`,
			restore: `ALTER TABLE catalog.verification_challenges ADD CONSTRAINT verification_challenges_proof_digest_length CHECK (octet_length(proof_digest) = 32)`,
		},
		{
			name:    "Legacy marker nullability",
			degrade: `ALTER TABLE catalog.agent_versions ALTER COLUMN legacy_unverified DROP NOT NULL`,
			restore: `ALTER TABLE catalog.agent_versions ALTER COLUMN legacy_unverified SET NOT NULL`,
		},
		{
			name:    "Agent Release Card foreign key",
			degrade: `ALTER TABLE catalog.agent_releases DROP CONSTRAINT agent_releases_card_fk`,
			restore: `ALTER TABLE catalog.agent_releases ADD CONSTRAINT agent_releases_card_fk FOREIGN KEY (agent_id, agent_card_version) REFERENCES catalog.agent_versions(agent_id, version)`,
		},
		{
			name:    "Agent Release state check",
			degrade: `ALTER TABLE catalog.agent_releases DROP CONSTRAINT agent_releases_state`,
			restore: `ALTER TABLE catalog.agent_releases ADD CONSTRAINT agent_releases_state CHECK (state IN ('draft', 'pending_verification', 'verified', 'published', 'suspended', 'revoked'))`,
		},
		{
			name:    "Agent Release immutable trigger",
			degrade: `DROP TRIGGER agent_releases_bound_immutable ON catalog.agent_releases`,
			restore: `CREATE TRIGGER agent_releases_bound_immutable BEFORE UPDATE ON catalog.agent_releases FOR EACH ROW EXECUTE FUNCTION catalog.reject_agent_release_bound_mutation()`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := connection.Exec(ctx, test.degrade); err != nil {
				t.Fatalf("degrade schema: %v", err)
			}
			if err := CheckSchema(ctx, connection); err == nil {
				t.Fatal("incomplete schema was reported ready")
			}
			if _, err := connection.Exec(ctx, test.restore); err != nil {
				t.Fatalf("restore schema: %v", err)
			}
			if err := CheckSchema(ctx, connection); err != nil {
				t.Fatalf("restored schema was not ready: %v", err)
			}
		})
	}
}
