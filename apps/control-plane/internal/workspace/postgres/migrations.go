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
	var workspacePresent, workspaceColumnsPresent, workspaceConstraintsPresent bool
	var installationPresent, installationColumnsPresent, installationConstraintsPresent bool
	var currentIndexPresent, orderIndexPresent bool
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
              AND ordinal_position = CASE column_name
                  WHEN 'workspace_id' THEN 1
                  WHEN 'owner_id' THEN 2
                  WHEN 'created_at' THEN 3
                  WHEN 'updated_at' THEN 4
              END
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
             AND convalidated
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
        (
            SELECT COUNT(*) = 10
            FROM information_schema.columns
            WHERE table_schema = 'workspace'
              AND table_name = 'installations'
        )
        AND (
            SELECT COUNT(*) = 10
            FROM information_schema.columns
            WHERE table_schema = 'workspace'
              AND table_name = 'installations'
              AND (
                  column_name IN ('installation_id', 'workspace_id', 'agent_id')
                  AND data_type = 'character varying'
                  AND character_maximum_length = 128
                  AND collation_name = 'C'
                  AND is_nullable = 'NO'
                  OR column_name = 'status'
                  AND data_type = 'character varying'
                  AND character_maximum_length = 16
                  AND collation_name = 'C'
                  AND is_nullable = 'NO'
                  OR column_name IN ('version_constraint', 'installed_version')
                  AND data_type = 'text'
                  AND collation_name = 'C'
                  AND is_nullable = 'NO'
                  OR column_name = 'accepted_permissions'
                  AND data_type = 'ARRAY'
                  AND udt_name = '_text'
                  AND is_nullable = 'NO'
                  OR column_name IN ('installed_at', 'updated_at')
                  AND data_type = 'timestamp with time zone'
                  AND datetime_precision = 6
                  AND is_nullable = 'NO'
                  OR column_name = 'uninstalled_at'
                  AND data_type = 'timestamp with time zone'
                  AND datetime_precision = 6
                  AND is_nullable = 'YES'
              )
              AND ordinal_position = CASE column_name
                  WHEN 'installation_id' THEN 1
                  WHEN 'workspace_id' THEN 2
                  WHEN 'agent_id' THEN 3
                  WHEN 'version_constraint' THEN 4
                  WHEN 'installed_version' THEN 5
                  WHEN 'accepted_permissions' THEN 6
                  WHEN 'status' THEN 7
                  WHEN 'installed_at' THEN 8
                  WHEN 'updated_at' THEN 9
                  WHEN 'uninstalled_at' THEN 10
              END
        ),
        (
            SELECT COUNT(*) = 8
            FROM pg_constraint
            WHERE conrelid = to_regclass('workspace.installations')
        )
        AND (
            SELECT COUNT(*) = 8
           FROM pg_constraint
           WHERE conrelid = to_regclass('workspace.installations')
             AND convalidated
             AND (
                  conname = 'installations_pkey'
                  AND contype = 'p'
                  AND pg_get_constraintdef(oid) = 'PRIMARY KEY (installation_id)'
                  OR conname = 'installations_workspace_fk'
                  AND contype = 'f'
                  AND confrelid = to_regclass('workspace.workspaces')
                  AND conkey = ARRAY[2]::smallint[]
                  AND confkey = ARRAY[1]::smallint[]
                  AND confmatchtype = 's'
                  AND confupdtype = 'a'
                  AND confdeltype = 'a'
                  AND condeferrable = FALSE
                  AND condeferred = FALSE
                  AND convalidated
                  OR conname = 'installations_id_format'
                  AND contype = 'c'
                  AND pg_get_constraintdef(oid) = 'CHECK (((installation_id)::text ~ ''^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$''::text))'
                  OR conname = 'installations_workspace_id_format'
                  AND contype = 'c'
                  AND pg_get_constraintdef(oid) = 'CHECK (((workspace_id)::text ~ ''^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$''::text))'
                  OR conname = 'installations_agent_id_format'
                  AND contype = 'c'
                  AND pg_get_constraintdef(oid) = 'CHECK (((agent_id)::text ~ ''^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$''::text))'
                  OR conname = 'installations_status'
                  AND contype = 'c'
                  AND pg_get_constraintdef(oid) = 'CHECK (((status)::text = ANY ((ARRAY[''enabled''::character varying, ''disabled''::character varying, ''uninstalled''::character varying])::text[])))'
                  OR conname = 'installations_timestamp_order'
                  AND contype = 'c'
                  AND pg_get_constraintdef(oid) = 'CHECK ((installed_at <= updated_at))'
                  OR conname = 'installations_state_timestamps'
                  AND contype = 'c'
                  AND pg_get_constraintdef(oid) = 'CHECK (((((status)::text = ANY ((ARRAY[''enabled''::character varying, ''disabled''::character varying])::text[])) AND (uninstalled_at IS NULL)) OR (((status)::text = ''uninstalled''::text) AND (uninstalled_at IS NOT NULL) AND (uninstalled_at = updated_at))))'
              )
        ),
       EXISTS (
           SELECT 1
           FROM pg_index index_definition
           JOIN pg_class index_relation ON index_relation.oid = index_definition.indexrelid
           JOIN pg_am access_method ON access_method.oid = index_relation.relam
           JOIN pg_opclass varchar_opclass ON varchar_opclass.opcmethod = access_method.oid
               AND varchar_opclass.opcnamespace = 'pg_catalog'::regnamespace
               AND varchar_opclass.opcname = 'varchar_ops'
           WHERE index_definition.indexrelid = to_regclass('workspace.installations_current_agent_idx')
             AND index_definition.indrelid = to_regclass('workspace.installations')
             AND access_method.amname = 'btree'
             AND index_definition.indisunique
             AND index_definition.indisvalid
             AND index_definition.indkey = '2 3'::int2vector
             AND index_definition.indoption = '0 0'::int2vector
             AND index_definition.indclass = ARRAY[
                 varchar_opclass.oid,
                 varchar_opclass.oid
             ]::oidvector
             AND index_definition.indcollation = ARRAY[
                 to_regcollation('pg_catalog."C"')::oid,
                 to_regcollation('pg_catalog."C"')::oid
             ]::oidvector
             AND pg_get_expr(index_definition.indpred, index_definition.indrelid) = '((status)::text <> ''uninstalled''::text)'
       ),
       EXISTS (
           SELECT 1
           FROM pg_index index_definition
           JOIN pg_class index_relation ON index_relation.oid = index_definition.indexrelid
           JOIN pg_am access_method ON access_method.oid = index_relation.relam
           JOIN pg_opclass varchar_opclass ON varchar_opclass.opcmethod = access_method.oid
               AND varchar_opclass.opcnamespace = 'pg_catalog'::regnamespace
               AND varchar_opclass.opcname = 'varchar_ops'
           JOIN pg_opclass timestamptz_opclass ON timestamptz_opclass.opcmethod = access_method.oid
               AND timestamptz_opclass.opcnamespace = 'pg_catalog'::regnamespace
               AND timestamptz_opclass.opcname = 'timestamptz_ops'
           WHERE index_definition.indexrelid = to_regclass('workspace.installations_workspace_order_idx')
             AND index_definition.indrelid = to_regclass('workspace.installations')
             AND access_method.amname = 'btree'
             AND NOT index_definition.indisunique
             AND index_definition.indisvalid
             AND index_definition.indkey = '2 8 1'::int2vector
             AND index_definition.indoption = '0 0 0'::int2vector
             AND index_definition.indclass = ARRAY[
                 varchar_opclass.oid,
                 timestamptz_opclass.oid,
                 varchar_opclass.oid
             ]::oidvector
             AND index_definition.indcollation = ARRAY[
                 to_regcollation('pg_catalog."C"')::oid,
                 0::oid,
                 to_regcollation('pg_catalog."C"')::oid
             ]::oidvector
             AND index_definition.indpred IS NULL
       )
FROM workspace.schema_version`).Scan(
		&version, &workspacePresent, &workspaceColumnsPresent, &workspaceConstraintsPresent, &installationPresent, &installationColumnsPresent, &installationConstraintsPresent, &currentIndexPresent, &orderIndexPresent,
	); err != nil {
		return fmt.Errorf("read workspace schema version: %w", err)
	}
	if version != ExpectedSchemaVersion || !workspacePresent || !workspaceColumnsPresent || !workspaceConstraintsPresent || !installationPresent || !installationColumnsPresent || !installationConstraintsPresent || !currentIndexPresent || !orderIndexPresent {
		return ErrSchemaVersionMismatch
	}
	return nil
}
