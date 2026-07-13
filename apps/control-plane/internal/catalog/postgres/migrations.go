package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/tern/v2/migrate"
)

const ExpectedSchemaVersion int32 = 2

var ErrSchemaVersionMismatch = errors.New("catalog schema version mismatch")

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

var migrationFiles = fstest.MapFS{
	"001_catalog.sql":   &fstest.MapFile{Data: []byte(migration001), Mode: 0o444},
	"002_card_text.sql": &fstest.MapFile{Data: []byte(migration002), Mode: 0o444},
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
	err := db.QueryRow(ctx, `
SELECT version,
       to_regclass('catalog.agent_identities') IS NOT NULL,
       to_regclass('catalog.publication_clock') IS NOT NULL,
       to_regclass('catalog.agent_versions') IS NOT NULL,
       to_regclass('catalog.agent_version_capabilities') IS NOT NULL,
       (SELECT count(*) = 1
        FROM catalog.publication_clock
        WHERE singleton = true AND last_sequence >= 0)
FROM catalog.schema_version`).Scan(
		&version,
		&identitiesPresent,
		&clockPresent,
		&versionsPresent,
		&capabilitiesPresent,
		&clockReady,
	)
	if err != nil {
		return fmt.Errorf("read catalog schema version: %w", err)
	}
	if version != ExpectedSchemaVersion || !identitiesPresent || !clockPresent || !versionsPresent || !capabilitiesPresent || !clockReady {
		return ErrSchemaVersionMismatch
	}
	return nil
}
