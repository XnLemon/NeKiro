package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/tern/v2/migrate"
)

const ExpectedSchemaVersion int32 = 1

var ErrSchemaVersionMismatch = errors.New("workspace schema version mismatch")

// migration001 is generated from apps/control-plane/migrations/003_workspace.sql.
const migration001 = `CREATE SCHEMA IF NOT EXISTS workspace;

CREATE TABLE workspace.workspaces (
    workspace_id varchar(128) COLLATE "C" PRIMARY KEY,
    owner_id varchar(128) COLLATE "C" NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT workspaces_id_format CHECK (workspace_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT workspaces_owner_format CHECK (owner_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT workspaces_timestamp_order CHECK (created_at <= updated_at)
);

CREATE TABLE workspace.installations (
    installation_id varchar(128) COLLATE "C" PRIMARY KEY,
    workspace_id varchar(128) COLLATE "C" NOT NULL,
    agent_id varchar(128) COLLATE "C" NOT NULL,
    version_constraint text COLLATE "C" NOT NULL,
    installed_version text COLLATE "C" NOT NULL,
    accepted_permissions text[] NOT NULL,
    status varchar(16) COLLATE "C" NOT NULL,
    installed_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    uninstalled_at timestamptz,
    CONSTRAINT installations_workspace_fk
        FOREIGN KEY (workspace_id) REFERENCES workspace.workspaces (workspace_id),
    CONSTRAINT installations_id_format CHECK (installation_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT installations_workspace_id_format CHECK (workspace_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT installations_agent_id_format CHECK (agent_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT installations_status CHECK (status IN ('enabled', 'disabled', 'uninstalled')),
    CONSTRAINT installations_timestamp_order CHECK (installed_at <= updated_at),
    CONSTRAINT installations_state_timestamps CHECK (
        (status IN ('enabled', 'disabled') AND uninstalled_at IS NULL)
        OR (status = 'uninstalled' AND uninstalled_at IS NOT NULL AND uninstalled_at = updated_at)
    )
);

CREATE UNIQUE INDEX installations_current_agent_idx
    ON workspace.installations (workspace_id, agent_id)
    WHERE status <> 'uninstalled';

CREATE INDEX installations_workspace_order_idx
    ON workspace.installations (workspace_id, installed_at ASC, installation_id ASC);

---- create above / drop below ----

DROP TABLE workspace.installations;
DROP TABLE workspace.workspaces;
`

var migrationFiles = fstest.MapFS{
	"001_workspace.sql": &fstest.MapFile{Data: []byte(migration001), Mode: 0o444},
}

type RowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Migrate(ctx context.Context, conn *pgx.Conn, direction string) error {
	if direction != "up" {
		return fmt.Errorf("workspace migration direction %q is unsupported", direction)
	}
	if _, err := conn.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS workspace`); err != nil {
		return fmt.Errorf("create workspace migration schema: %w", err)
	}
	migrator, err := migrate.NewMigrator(ctx, conn, "workspace.schema_version")
	if err != nil {
		return fmt.Errorf("initialize workspace migrator: %w", err)
	}
	if err := migrator.LoadMigrations(migrationFiles); err != nil {
		return fmt.Errorf("load embedded workspace migrations: %w", err)
	}
	if len(migrator.Migrations) != int(ExpectedSchemaVersion) {
		return fmt.Errorf("embedded workspace migration count: %w", ErrSchemaVersionMismatch)
	}
	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate workspace up: %w", err)
	}
	return nil
}

func CheckSchema(ctx context.Context, db RowQuerier) error {
	var version int32
	var workspacePresent, workspaceColumnsPresent, workspaceConstraintsPresent, installationPresent, currentIndexPresent, orderIndexPresent bool
	if err := db.QueryRow(ctx, `
SELECT version,
       to_regclass('workspace.workspaces') IS NOT NULL,
       (
           SELECT COUNT(*) = 4
           FROM information_schema.columns
           WHERE table_schema = 'workspace'
             AND table_name = 'workspaces'
             AND (
                 column_name IN ('workspace_id', 'owner_id')
                 AND data_type = 'character varying'
                 AND character_maximum_length = 128
                 AND collation_name = 'C'
                 AND is_nullable = 'NO'
                 OR column_name IN ('created_at', 'updated_at')
                 AND data_type = 'timestamp with time zone'
                 AND datetime_precision = 6
                 AND is_nullable = 'NO'
             )
       )
       AND (
           SELECT COUNT(*) = 4
           FROM information_schema.columns
           WHERE table_schema = 'workspace'
             AND table_name = 'workspaces'
       ),
       (
           SELECT COUNT(*) = 4
           FROM pg_constraint
           WHERE conrelid = to_regclass('workspace.workspaces')
             AND (
                 conname = 'workspaces_id_format'
                 AND contype = 'c'
                 AND pg_get_constraintdef(oid) = 'CHECK (((workspace_id)::text ~ ''^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$''::text))'
                 OR conname = 'workspaces_owner_format'
                 AND contype = 'c'
                 AND pg_get_constraintdef(oid) = 'CHECK (((owner_id)::text ~ ''^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$''::text))'
                 OR conname = 'workspaces_timestamp_order'
                 AND contype = 'c'
                 AND pg_get_constraintdef(oid) = 'CHECK ((created_at <= updated_at))'
                 OR conname = 'workspaces_pkey'
                 AND contype = 'p'
                 AND pg_get_constraintdef(oid) = 'PRIMARY KEY (workspace_id)'
             )
       ),
       to_regclass('workspace.installations') IS NOT NULL,
       EXISTS (
           SELECT 1
           FROM pg_index index_definition
           WHERE index_definition.indexrelid = to_regclass('workspace.installations_current_agent_idx')
             AND index_definition.indrelid = to_regclass('workspace.installations')
             AND index_definition.indisunique
             AND index_definition.indisvalid
             AND index_definition.indkey = '2 3'::int2vector
             AND pg_get_expr(index_definition.indpred, index_definition.indrelid) = '((status)::text <> ''uninstalled''::text)'
       ),
       to_regclass('workspace.installations_workspace_order_idx') IS NOT NULL
FROM workspace.schema_version`).Scan(
		&version, &workspacePresent, &workspaceColumnsPresent, &workspaceConstraintsPresent, &installationPresent, &currentIndexPresent, &orderIndexPresent,
	); err != nil {
		return fmt.Errorf("read workspace schema version: %w", err)
	}
	if version != ExpectedSchemaVersion || !workspacePresent || !workspaceColumnsPresent || !workspaceConstraintsPresent || !installationPresent || !currentIndexPresent || !orderIndexPresent {
		return ErrSchemaVersionMismatch
	}
	return nil
}
