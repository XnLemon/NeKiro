package postgres

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/tern/v2/migrate"
)

const ExpectedSchemaVersion int32 = 4

var ErrSchemaVersionMismatch = errors.New("catalog schema version mismatch")

// migration004 is kept beside the embedded migration source so the migration
// runner and the owned apps/control-plane/migrations file cannot drift.
//
//go:embed 004_agent_release.sql
var migration004 []byte

// migration001 is generated from apps/control-plane/migrations/001_catalog.sql.
const migration001 = `CREATE SCHEMA IF NOT EXISTS catalog;

CREATE TABLE catalog.agent_identities (
    agent_id varchar(128) COLLATE "C" PRIMARY KEY,
    owner_id varchar(128) COLLATE "C" NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT agent_identities_agent_id_format CHECK (agent_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT agent_identities_owner_id_format CHECK (owner_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$')
);

CREATE INDEX agent_identities_owner_idx
    ON catalog.agent_identities (owner_id, agent_id);

CREATE TABLE catalog.publication_clock (
    singleton boolean PRIMARY KEY,
    last_sequence bigint NOT NULL,
    CONSTRAINT publication_clock_singleton CHECK (singleton),
    CONSTRAINT publication_clock_non_negative CHECK (last_sequence >= 0)
);

INSERT INTO catalog.publication_clock (singleton, last_sequence)
VALUES (true, 0);

CREATE TABLE catalog.agent_versions (
    agent_id varchar(128) COLLATE "C" NOT NULL,
    version text COLLATE "C" NOT NULL,
    schema_version varchar(16) NOT NULL,
    card jsonb NOT NULL,
    card_digest bytea NOT NULL,
    publication_status varchar(16) NOT NULL,
    registered_at timestamptz NOT NULL,
    published_at timestamptz,
    publication_sequence bigint,
    disabled_at timestamptz,
    PRIMARY KEY (agent_id, version),
    CONSTRAINT agent_versions_identity_fk
        FOREIGN KEY (agent_id) REFERENCES catalog.agent_identities (agent_id),
    CONSTRAINT agent_versions_agent_id_format CHECK (agent_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT agent_versions_schema_version CHECK (schema_version = '0.2'),
    CONSTRAINT agent_versions_card_digest_length CHECK (octet_length(card_digest) = 32),
    CONSTRAINT agent_versions_publication_status CHECK (publication_status IN ('draft', 'published', 'disabled')),
    CONSTRAINT agent_versions_state_timestamps CHECK (
        (publication_status = 'draft'
            AND published_at IS NULL
            AND publication_sequence IS NULL
            AND disabled_at IS NULL)
        OR
        (publication_status = 'published'
            AND published_at IS NOT NULL
            AND publication_sequence IS NOT NULL
            AND disabled_at IS NULL)
        OR
        (publication_status = 'disabled'
            AND disabled_at IS NOT NULL
            AND ((published_at IS NULL AND publication_sequence IS NULL)
                OR (published_at IS NOT NULL AND publication_sequence IS NOT NULL)))
    )
);

CREATE UNIQUE INDEX agent_versions_publication_sequence_idx
    ON catalog.agent_versions (publication_sequence)
    WHERE publication_sequence IS NOT NULL;

CREATE INDEX agent_versions_published_order_idx
    ON catalog.agent_versions (published_at DESC, agent_id, version)
    WHERE publication_status = 'published';

CREATE TABLE catalog.agent_version_capabilities (
    agent_id varchar(128) COLLATE "C" NOT NULL,
    version text COLLATE "C" NOT NULL,
    capability_id varchar(128) COLLATE "C" NOT NULL,
    PRIMARY KEY (agent_id, version, capability_id),
    CONSTRAINT agent_version_capabilities_version_fk
        FOREIGN KEY (agent_id, version)
        REFERENCES catalog.agent_versions (agent_id, version),
    CONSTRAINT agent_version_capabilities_id_format
        CHECK (capability_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$')
);

CREATE INDEX agent_version_capabilities_lookup_idx
    ON catalog.agent_version_capabilities (capability_id, agent_id, version);

---- create above / drop below ----

DROP TABLE catalog.agent_version_capabilities;
DROP TABLE catalog.agent_versions;
DROP TABLE catalog.agent_identities;
DROP TABLE catalog.publication_clock;
`

// migration002 is generated from apps/control-plane/migrations/002_card_text.sql.
const migration002 = `ALTER TABLE catalog.agent_versions
    ADD COLUMN card_name text,
    ADD COLUMN card_description text;

UPDATE catalog.agent_versions
SET card_name = card->>'name',
    card_description = card->>'description';

ALTER TABLE catalog.agent_versions
    ALTER COLUMN card_name SET NOT NULL,
    ALTER COLUMN card_description SET NOT NULL,
    ALTER COLUMN card TYPE text USING card::text;

---- create above / drop below ----

ALTER TABLE catalog.agent_versions
    ALTER COLUMN card TYPE jsonb USING card::jsonb,
    DROP COLUMN card_description,
    DROP COLUMN card_name;
`

// migration003 is generated from apps/control-plane/migrations/003_trusted_publication.sql.
const migration003 = `CREATE TABLE catalog.providers (
    provider_id varchar(128) COLLATE "C" PRIMARY KEY,
    owner_identity varchar(128) COLLATE "C" NOT NULL,
    verification_status varchar(16) NOT NULL,
    verification_method varchar(64) NOT NULL,
    verified_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT providers_provider_id_format CHECK (provider_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT providers_owner_identity_format CHECK (owner_identity ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT providers_status CHECK (verification_status IN ('unverified', 'verified', 'suspended')),
    CONSTRAINT providers_method CHECK (verification_method = 'http_well_known'),
    CONSTRAINT providers_state_timestamps CHECK ((verification_status = 'unverified' AND verified_at IS NULL) OR (verification_status = 'verified' AND verified_at IS NOT NULL) OR verification_status = 'suspended')
);

ALTER TABLE catalog.agent_identities
    ADD COLUMN provider_id varchar(128) COLLATE "C",
    ADD CONSTRAINT agent_identities_provider_fk FOREIGN KEY (provider_id) REFERENCES catalog.providers(provider_id);

CREATE INDEX agent_identities_provider_idx ON catalog.agent_identities (provider_id);

CREATE TABLE catalog.endpoint_bindings (
    binding_id varchar(128) COLLATE "C" PRIMARY KEY,
    provider_id varchar(128) COLLATE "C" NOT NULL,
    agent_id varchar(128) COLLATE "C" NOT NULL,
    agent_card_version text COLLATE "C" NOT NULL,
    endpoint text NOT NULL,
    endpoint_origin text NOT NULL,
    endpoint_path text NOT NULL,
    verification_method varchar(64) NOT NULL,
    verification_status varchar(16) NOT NULL,
    verification_evidence_digest bytea,
    verification_failure_code varchar(64),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    verified_at timestamptz,
    revoked_at timestamptz,
    CONSTRAINT endpoint_bindings_provider_fk FOREIGN KEY (provider_id) REFERENCES catalog.providers(provider_id),
    CONSTRAINT endpoint_bindings_agent_version_fk FOREIGN KEY (agent_id, agent_card_version) REFERENCES catalog.agent_versions(agent_id, version),
    CONSTRAINT endpoint_bindings_identifier_format CHECK (binding_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$' AND agent_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT endpoint_bindings_evidence_digest_length CHECK (verification_evidence_digest IS NULL OR octet_length(verification_evidence_digest) = 32),
    CONSTRAINT endpoint_bindings_status CHECK (verification_status IN ('pending', 'verified', 'failed', 'revoked')),
    CONSTRAINT endpoint_bindings_method CHECK (verification_method = 'http_well_known'),
    CONSTRAINT endpoint_bindings_state_timestamps CHECK (
        (verification_status = 'pending' AND verification_evidence_digest IS NULL AND verification_failure_code IS NULL AND verified_at IS NULL AND revoked_at IS NULL)
        OR (verification_status = 'verified' AND verification_evidence_digest IS NOT NULL AND verification_failure_code IS NULL AND verified_at IS NOT NULL AND revoked_at IS NULL)
        OR (verification_status = 'failed' AND verification_evidence_digest IS NULL AND verification_failure_code IS NOT NULL AND verified_at IS NULL AND revoked_at IS NULL)
        OR (verification_status = 'revoked' AND verification_evidence_digest IS NULL AND verification_failure_code IS NULL AND verified_at IS NULL AND revoked_at IS NOT NULL)
    )
);

CREATE INDEX endpoint_bindings_provider_idx ON catalog.endpoint_bindings (provider_id, agent_id);

CREATE TABLE catalog.verification_challenges (
    challenge_id varchar(128) COLLATE "C" PRIMARY KEY,
    binding_id varchar(128) COLLATE "C" NOT NULL,
    proof_digest bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL,
    CONSTRAINT verification_challenges_binding_fk FOREIGN KEY (binding_id) REFERENCES catalog.endpoint_bindings(binding_id),
    CONSTRAINT verification_challenges_identifier_format CHECK (challenge_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT verification_challenges_proof_digest_length CHECK (octet_length(proof_digest) = 32)
);

CREATE INDEX verification_challenges_binding_idx ON catalog.verification_challenges (binding_id, created_at DESC);

---- create above / drop below ----

DROP TABLE catalog.verification_challenges;
DROP TABLE catalog.endpoint_bindings;
DROP INDEX catalog.agent_identities_provider_idx;
ALTER TABLE catalog.agent_identities DROP CONSTRAINT agent_identities_provider_fk, DROP COLUMN provider_id;
DROP TABLE catalog.providers;
`

var migrationFiles = fstest.MapFS{
	"001_catalog.sql":             &fstest.MapFile{Data: []byte(migration001), Mode: 0o444},
	"002_card_text.sql":           &fstest.MapFile{Data: []byte(migration002), Mode: 0o444},
	"003_trusted_publication.sql": &fstest.MapFile{Data: []byte(migration003), Mode: 0o444},
	"004_agent_release.sql":       &fstest.MapFile{Data: migration004, Mode: 0o444},
}

type RowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Migrate(ctx context.Context, conn *pgx.Conn, direction string) error {
	if direction != "up" {
		return fmt.Errorf("catalog migration direction %q is unsupported", direction)
	}

	if _, err := conn.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS catalog`); err != nil {
		return fmt.Errorf("create catalog migration schema: %w", err)
	}

	migrator, err := migrate.NewMigrator(ctx, conn, "catalog.schema_version")
	if err != nil {
		return fmt.Errorf("initialize catalog migrator: %w", err)
	}
	if err := migrator.LoadMigrations(migrationFiles); err != nil {
		return fmt.Errorf("load embedded catalog migrations: %w", err)
	}
	if len(migrator.Migrations) != int(ExpectedSchemaVersion) {
		return fmt.Errorf("embedded catalog migration count: %w", ErrSchemaVersionMismatch)
	}

	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate catalog up: %w", err)
	}
	return nil
}

func CheckSchema(ctx context.Context, db RowQuerier) error {
	var version int32
	var identitiesPresent bool
	var clockPresent bool
	var clockReady bool
	var versionsPresent bool
	var capabilitiesPresent bool
	var cardTextReady bool
	var cardNameReady bool
	var cardDescriptionReady bool
	var legacyUnverifiedReady bool
	var providersPresent bool
	var bindingsPresent bool
	var challengesPresent bool
	var trustColumnsReady bool
	var trustForeignKeysReady bool
	var trustStatusChecksReady bool
	var trustDigestChecksReady bool
	var releasesPresent bool
	var releaseColumnsReady bool
	var releaseForeignKeysReady bool
	var releaseChecksReady bool
	var releaseIndexesReady bool
	var releaseImmutableTrigger bool
	err := db.QueryRow(ctx, `
WITH required_columns(table_name, column_name, is_nullable, data_type) AS (VALUES
    ('agent_identities', 'provider_id', 'YES', 'character varying'),
    ('providers', 'provider_id', 'NO', 'character varying'),
    ('providers', 'owner_identity', 'NO', 'character varying'),
    ('providers', 'verification_status', 'NO', 'character varying'),
    ('providers', 'verification_method', 'NO', 'character varying'),
    ('providers', 'verified_at', 'YES', 'timestamp with time zone'),
    ('providers', 'created_at', 'NO', 'timestamp with time zone'),
    ('providers', 'updated_at', 'NO', 'timestamp with time zone'),
    ('endpoint_bindings', 'binding_id', 'NO', 'character varying'),
    ('endpoint_bindings', 'provider_id', 'NO', 'character varying'),
    ('endpoint_bindings', 'agent_id', 'NO', 'character varying'),
    ('endpoint_bindings', 'agent_card_version', 'NO', 'text'),
    ('endpoint_bindings', 'endpoint', 'NO', 'text'),
    ('endpoint_bindings', 'endpoint_origin', 'NO', 'text'),
    ('endpoint_bindings', 'endpoint_path', 'NO', 'text'),
    ('endpoint_bindings', 'verification_method', 'NO', 'character varying'),
    ('endpoint_bindings', 'verification_status', 'NO', 'character varying'),
    ('endpoint_bindings', 'verification_evidence_digest', 'YES', 'bytea'),
    ('endpoint_bindings', 'verification_failure_code', 'YES', 'character varying'),
    ('endpoint_bindings', 'created_at', 'NO', 'timestamp with time zone'),
    ('endpoint_bindings', 'updated_at', 'NO', 'timestamp with time zone'),
    ('endpoint_bindings', 'verified_at', 'YES', 'timestamp with time zone'),
    ('endpoint_bindings', 'revoked_at', 'YES', 'timestamp with time zone'),
    ('verification_challenges', 'challenge_id', 'NO', 'character varying'),
    ('verification_challenges', 'binding_id', 'NO', 'character varying'),
    ('verification_challenges', 'proof_digest', 'NO', 'bytea'),
    ('verification_challenges', 'expires_at', 'NO', 'timestamp with time zone'),
    ('verification_challenges', 'used_at', 'YES', 'timestamp with time zone'),
    ('verification_challenges', 'created_at', 'NO', 'timestamp with time zone'),
    ('agent_versions', 'legacy_unverified', 'NO', 'boolean')
)
SELECT version,
       to_regclass('catalog.agent_identities') IS NOT NULL,
       to_regclass('catalog.publication_clock') IS NOT NULL,
       to_regclass('catalog.agent_versions') IS NOT NULL,
       to_regclass('catalog.agent_version_capabilities') IS NOT NULL,
       (SELECT count(*) = 1
         FROM catalog.publication_clock
         WHERE singleton = true AND last_sequence >= 0),
       (SELECT count(*) = 1
        FROM information_schema.columns
        WHERE table_schema = 'catalog'
          AND table_name = 'agent_versions'
          AND column_name = 'card'
          AND data_type = 'text'
          AND is_nullable = 'NO'),
       (SELECT count(*) = 1
        FROM information_schema.columns
        WHERE table_schema = 'catalog'
          AND table_name = 'agent_versions'
          AND column_name = 'card_name'
          AND data_type = 'text'
          AND is_nullable = 'NO'),
       (SELECT count(*) = 1
        FROM information_schema.columns
        WHERE table_schema = 'catalog'
          AND table_name = 'agent_versions'
          AND column_name = 'card_description'
          AND data_type = 'text'
          AND is_nullable = 'NO'),
       (SELECT count(*) = 1
        FROM information_schema.columns
        WHERE table_schema = 'catalog'
          AND table_name = 'agent_versions'
          AND column_name = 'legacy_unverified'
          AND data_type = 'boolean'
          AND is_nullable = 'NO'),
       to_regclass('catalog.providers') IS NOT NULL,
       to_regclass('catalog.endpoint_bindings') IS NOT NULL,
       to_regclass('catalog.verification_challenges') IS NOT NULL,
       (SELECT count(*) = 30
        FROM required_columns expected
        JOIN information_schema.columns actual
          ON actual.table_schema = 'catalog'
         AND actual.table_name = expected.table_name
         AND actual.column_name = expected.column_name
         AND actual.is_nullable = expected.is_nullable
         AND actual.data_type = expected.data_type),
       (SELECT count(*) = 4
        FROM pg_constraint constraint_row
        JOIN pg_class relation ON relation.oid = constraint_row.conrelid
        JOIN pg_namespace namespace_row ON namespace_row.oid = relation.relnamespace
        WHERE namespace_row.nspname = 'catalog'
          AND constraint_row.conname IN ('agent_identities_provider_fk', 'endpoint_bindings_provider_fk', 'endpoint_bindings_agent_version_fk', 'verification_challenges_binding_fk')
          AND constraint_row.contype = 'f'
          AND constraint_row.convalidated),
       (SELECT count(*) = 12
        FROM pg_constraint constraint_row
        JOIN pg_class relation ON relation.oid = constraint_row.conrelid
        JOIN pg_namespace namespace_row ON namespace_row.oid = relation.relnamespace
        WHERE namespace_row.nspname = 'catalog'
          AND constraint_row.conname IN ('providers_provider_id_format', 'providers_owner_identity_format', 'providers_status', 'providers_method', 'providers_state_timestamps', 'endpoint_bindings_identifier_format', 'endpoint_bindings_evidence_digest_length', 'endpoint_bindings_status', 'endpoint_bindings_method', 'endpoint_bindings_state_timestamps', 'verification_challenges_identifier_format', 'verification_challenges_proof_digest_length')
          AND constraint_row.contype = 'c'
          AND constraint_row.convalidated),
       (SELECT count(*) = 2
        FROM pg_constraint constraint_row
        JOIN pg_class relation ON relation.oid = constraint_row.conrelid
        JOIN pg_namespace namespace_row ON namespace_row.oid = relation.relnamespace
        WHERE namespace_row.nspname = 'catalog'
          AND constraint_row.conname IN ('endpoint_bindings_evidence_digest_length', 'verification_challenges_proof_digest_length')
          AND constraint_row.contype = 'c'
          AND constraint_row.convalidated)
FROM catalog.schema_version`).Scan(
		&version,
		&identitiesPresent,
		&clockPresent,
		&versionsPresent,
		&capabilitiesPresent,
		&clockReady,
		&cardTextReady,
		&cardNameReady,
		&cardDescriptionReady,
		&legacyUnverifiedReady,
		&providersPresent,
		&bindingsPresent,
		&challengesPresent,
		&trustColumnsReady,
		&trustForeignKeysReady,
		&trustStatusChecksReady,
		&trustDigestChecksReady,
	)
	if err != nil {
		return fmt.Errorf("read catalog schema version: %w", err)
	}
	if err := db.QueryRow(ctx, `
WITH required_release_columns(column_name, is_nullable, data_type) AS (VALUES
    ('release_id', 'NO', 'character varying'),
    ('provider_id', 'NO', 'character varying'),
    ('agent_id', 'NO', 'character varying'),
    ('agent_card_version', 'NO', 'text'),
    ('card_digest', 'NO', 'bytea'),
    ('endpoint_binding_id', 'NO', 'character varying'),
    ('endpoint_origin', 'NO', 'text'),
    ('endpoint_path', 'NO', 'text'),
    ('verification_method', 'NO', 'character varying'),
    ('verification_evidence_digest', 'YES', 'bytea'),
    ('state', 'NO', 'character varying'),
    ('created_at', 'NO', 'timestamp with time zone'),
    ('updated_at', 'NO', 'timestamp with time zone'),
    ('verified_at', 'YES', 'timestamp with time zone'),
    ('published_at', 'YES', 'timestamp with time zone'),
    ('suspended_at', 'YES', 'timestamp with time zone'),
    ('revoked_at', 'YES', 'timestamp with time zone')
)
SELECT to_regclass('catalog.agent_releases') IS NOT NULL,
       (SELECT count(*) = 17
        FROM required_release_columns expected
        JOIN information_schema.columns actual
          ON actual.table_schema = 'catalog'
         AND actual.table_name = 'agent_releases'
         AND actual.column_name = expected.column_name
         AND actual.is_nullable = expected.is_nullable
         AND actual.data_type = expected.data_type),
       (SELECT count(*) = 3
        FROM pg_constraint constraint_row
        JOIN pg_class relation ON relation.oid = constraint_row.conrelid
        JOIN pg_namespace namespace_row ON namespace_row.oid = relation.relnamespace
        WHERE namespace_row.nspname = 'catalog'
          AND relation.relname = 'agent_releases'
          AND constraint_row.conname IN ('agent_releases_provider_fk', 'agent_releases_card_fk', 'agent_releases_binding_fk')
          AND constraint_row.contype = 'f' AND constraint_row.convalidated),
       (SELECT count(*) = 9
        FROM pg_constraint constraint_row
        JOIN pg_class relation ON relation.oid = constraint_row.conrelid
        JOIN pg_namespace namespace_row ON namespace_row.oid = relation.relnamespace
        WHERE namespace_row.nspname = 'catalog'
          AND relation.relname = 'agent_releases'
          AND constraint_row.conname IN ('agent_releases_release_id_format', 'agent_releases_provider_id_format', 'agent_releases_agent_id_format', 'agent_releases_card_digest_length', 'agent_releases_evidence_digest_length', 'agent_releases_state', 'agent_releases_method', 'agent_releases_timestamp_order', 'agent_releases_state_timestamps')
          AND constraint_row.contype = 'c' AND constraint_row.convalidated),
       to_regclass('catalog.agent_releases_agent_version_idx') IS NOT NULL
       AND to_regclass('catalog.agent_releases_provider_state_idx') IS NOT NULL,
       EXISTS (
           SELECT 1 FROM pg_trigger trigger_row
           JOIN pg_class relation ON relation.oid = trigger_row.tgrelid
           WHERE relation.oid = to_regclass('catalog.agent_releases')
             AND trigger_row.tgname = 'agent_releases_bound_immutable'
             AND trigger_row.tgenabled = 'O' AND NOT trigger_row.tgisinternal
       )`).Scan(
		&releasesPresent, &releaseColumnsReady, &releaseForeignKeysReady,
		&releaseChecksReady, &releaseIndexesReady, &releaseImmutableTrigger,
	); err != nil {
		return fmt.Errorf("read Agent Release schema: %w", err)
	}
	if version != ExpectedSchemaVersion || !identitiesPresent || !clockPresent || !versionsPresent || !capabilitiesPresent || !clockReady || !cardTextReady || !cardNameReady || !cardDescriptionReady || !legacyUnverifiedReady || !providersPresent || !bindingsPresent || !challengesPresent || !trustColumnsReady || !trustForeignKeysReady || !trustStatusChecksReady || !trustDigestChecksReady || !releasesPresent || !releaseColumnsReady || !releaseForeignKeysReady || !releaseChecksReady || !releaseIndexesReady || !releaseImmutableTrigger {
		return ErrSchemaVersionMismatch
	}
	return nil
}
