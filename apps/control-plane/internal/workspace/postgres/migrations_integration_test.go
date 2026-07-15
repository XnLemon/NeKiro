//go:build integration

package postgres

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestWorkspaceMigrationAndReadiness(t *testing.T) {
	ctx := context.Background()
	databaseURL := os.Getenv("NEKIRO_TEST_DATABASE_URL")
	if strings.TrimSpace(databaseURL) == "" {
		t.Fatal("NEKIRO_TEST_DATABASE_URL is required for integration tests")
	}
	configuration, err := pgx.ParseConfig(databaseURL)
	if err != nil || !strings.HasSuffix(configuration.Database, "_test") {
		t.Fatal("integration database must be a valid database ending in _test")
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect dedicated test database: %v", err)
	}
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, `DROP SCHEMA IF EXISTS workspace CASCADE`); err != nil {
		t.Fatalf("reset Workspace schema: %v", err)
	}
	if err := Migrate(ctx, connection, "up"); err != nil {
		t.Fatalf("migrate Workspace schema: %v", err)
	}
	if err := CheckSchema(ctx, connection); err != nil {
		t.Fatalf("valid Workspace schema was not ready: %v", err)
	}

	transaction, err := connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin search-path readiness check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `SET LOCAL search_path = workspace, pg_catalog`); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatalf("set search path for readiness check: %v", err)
	}
	if err := CheckSchema(ctx, transaction); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatalf("valid Workspace schema was rejected with workspace search path: %v", err)
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback search-path readiness check: %v", err)
	}

	if _, err := connection.Exec(ctx, `DROP SCHEMA workspace CASCADE`); err != nil {
		t.Fatalf("drop Workspace schema: %v", err)
	}
	if err := CheckSchema(ctx, connection); err == nil {
		t.Fatal("missing Workspace schema was reported ready")
	}
	if err := Migrate(ctx, connection, "up"); err != nil {
		t.Fatalf("restore Workspace schema after missing-schema check: %v", err)
	}

	if _, err := connection.Exec(ctx, `UPDATE workspace.schema_version SET version = $1`, ExpectedSchemaVersion+1); err != nil {
		t.Fatalf("make Workspace schema stale: %v", err)
	}
	if err := CheckSchema(ctx, connection); err == nil {
		t.Fatal("stale Workspace schema was reported ready")
	}
	if _, err := connection.Exec(ctx, `UPDATE workspace.schema_version SET version = $1`, ExpectedSchemaVersion); err != nil {
		t.Fatalf("restore Workspace schema version: %v", err)
	}

	transaction, err = connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin incomplete-schema check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `ALTER TABLE workspace.workspaces DROP COLUMN owner_id`); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatalf("remove Workspace owner column in transaction: %v", err)
	}
	if err := CheckSchema(ctx, transaction); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("incomplete Workspace columns were reported ready")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback incomplete-schema check: %v", err)
	}

	transaction, err = connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin incomplete-constraint check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `ALTER TABLE workspace.workspaces DROP CONSTRAINT workspaces_timestamp_order`); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatalf("remove Workspace timestamp constraint in transaction: %v", err)
	}
	if err := CheckSchema(ctx, transaction); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("incomplete Workspace constraints were reported ready")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback incomplete-constraint check: %v", err)
	}

	transaction, err = connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin nullable-column check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `ALTER TABLE workspace.workspaces ALTER COLUMN owner_id DROP NOT NULL`); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatalf("make Workspace owner nullable in transaction: %v", err)
	}
	if err := CheckSchema(ctx, transaction); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("nullable Workspace owner was reported ready")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback nullable-column check: %v", err)
	}

	transaction, err = connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin collation check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `ALTER TABLE workspace.workspaces ALTER COLUMN owner_id TYPE varchar(128) COLLATE pg_catalog."C.utf8"`); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatalf("change Workspace owner collation in transaction: %v", err)
	}
	if err := CheckSchema(ctx, transaction); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("non-C Workspace collation was reported ready")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback collation check: %v", err)
	}

	transaction, err = connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin timestamp precision check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `ALTER TABLE workspace.workspaces ALTER COLUMN updated_at TYPE timestamptz(0)`); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatalf("reduce Workspace timestamp precision in transaction: %v", err)
	}
	if err := CheckSchema(ctx, transaction); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("reduced Workspace timestamp precision was reported ready")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback timestamp precision check: %v", err)
	}

	transaction, err = connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin incomplete Installation column check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `ALTER TABLE workspace.installations DROP COLUMN accepted_permissions`); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatalf("remove Installation permissions column in transaction: %v", err)
	}
	if err := CheckSchema(ctx, transaction); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("incomplete Installation columns were reported ready")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback incomplete Installation column check: %v", err)
	}

	transaction, err = connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin incomplete Installation constraint check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `
ALTER TABLE workspace.installations DROP CONSTRAINT installations_state_timestamps;
ALTER TABLE workspace.installations ADD CONSTRAINT installations_state_timestamps CHECK (true)`); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatalf("weaken Installation state constraint in transaction: %v", err)
	}
	if err := CheckSchema(ctx, transaction); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("incomplete Installation constraints were reported ready")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback incomplete Installation constraint check: %v", err)
	}

	transaction, err = connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin extra Installation constraint check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `ALTER TABLE workspace.installations ADD CONSTRAINT issue_9_extra_installation_check CHECK (installation_id IS NOT NULL)`); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatalf("add extra Installation constraint in transaction: %v", err)
	}
	if err := CheckSchema(ctx, transaction); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("extra Installation constraint was reported ready")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback extra Installation constraint check: %v", err)
	}

	transaction, err = connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin unvalidated Installation constraint check: %v", err)
	}
	if _, err := transaction.Exec(ctx, `
ALTER TABLE workspace.installations DROP CONSTRAINT installations_id_format;
ALTER TABLE workspace.installations ADD CONSTRAINT installations_id_format CHECK (installation_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$') NOT VALID`); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatalf("recreate unvalidated Installation constraint in transaction: %v", err)
	}
	if err := CheckSchema(ctx, transaction); err == nil {
		_ = transaction.Rollback(ctx)
		t.Fatal("unvalidated Installation constraint was reported ready")
	}
	if err := transaction.Rollback(ctx); err != nil {
		t.Fatalf("rollback unvalidated Installation constraint check: %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := CheckSchema(canceled, connection); err == nil {
		t.Fatal("unavailable Workspace readiness context was reported ready")
	}

	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_current_agent_idx`); err != nil {
		t.Fatalf("drop current-install index: %v", err)
	}
	if err := CheckSchema(ctx, connection); err == nil {
		t.Fatal("missing current-install index was reported ready")
	}
	if _, err := connection.Exec(ctx, `CREATE UNIQUE INDEX installations_current_agent_idx ON workspace.installations (workspace_id, agent_id) WHERE status <> 'uninstalled'`); err != nil {
		t.Fatalf("restore current-install index: %v", err)
	}
	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_current_agent_idx`); err != nil {
		t.Fatalf("drop current-install uniqueness index: %v", err)
	}
	if _, err := connection.Exec(ctx, `CREATE INDEX installations_current_agent_idx ON workspace.installations (workspace_id, agent_id) WHERE status <> 'uninstalled'`); err != nil {
		t.Fatalf("create degraded current-install index: %v", err)
	}
	if err := CheckSchema(ctx, connection); err == nil {
		t.Fatal("non-unique current-install index was reported ready")
	}
	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_current_agent_idx`); err != nil {
		t.Fatalf("drop degraded current-install index: %v", err)
	}
	if _, err := connection.Exec(ctx, `CREATE UNIQUE INDEX installations_current_agent_idx ON workspace.installations (workspace_id, agent_id) WHERE status <> 'uninstalled'`); err != nil {
		t.Fatalf("restore current-install uniqueness index: %v", err)
	}
	if err := CheckSchema(ctx, connection); err != nil {
		t.Fatalf("restored current-install index was not ready: %v", err)
	}
	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_current_agent_idx`); err != nil {
		t.Fatalf("drop current-install index for ordering check: %v", err)
	}
	if _, err := connection.Exec(ctx, `CREATE UNIQUE INDEX installations_current_agent_idx ON workspace.installations (workspace_id DESC, agent_id ASC) WHERE status <> 'uninstalled'`); err != nil {
		t.Fatalf("create degraded current-install ordering index: %v", err)
	}
	if err := CheckSchema(ctx, connection); err == nil {
		t.Fatal("degraded current-install ordering index was reported ready")
	}
	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_current_agent_idx`); err != nil {
		t.Fatalf("drop degraded current-install ordering index: %v", err)
	}
	if _, err := connection.Exec(ctx, `CREATE UNIQUE INDEX installations_current_agent_idx ON workspace.installations (workspace_id, agent_id) WHERE status <> 'uninstalled'`); err != nil {
		t.Fatalf("restore current-install index after ordering check: %v", err)
	}
	if err := CheckSchema(ctx, connection); err != nil {
		t.Fatalf("restored current-install index after ordering check was not ready: %v", err)
	}
	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_current_agent_idx`); err != nil {
		t.Fatalf("drop current-install index for opclass check: %v", err)
	}
	if _, err := connection.Exec(ctx, `CREATE UNIQUE INDEX installations_current_agent_idx ON workspace.installations (workspace_id varchar_pattern_ops, agent_id varchar_pattern_ops) WHERE status <> 'uninstalled'`); err != nil {
		t.Fatalf("create degraded current-install opclass index: %v", err)
	}
	if err := CheckSchema(ctx, connection); err == nil {
		t.Fatal("degraded current-install opclass index was reported ready")
	}
	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_current_agent_idx`); err != nil {
		t.Fatalf("drop degraded current-install opclass index: %v", err)
	}
	if _, err := connection.Exec(ctx, `CREATE UNIQUE INDEX installations_current_agent_idx ON workspace.installations (workspace_id, agent_id) WHERE status <> 'uninstalled'`); err != nil {
		t.Fatalf("restore current-install index after opclass check: %v", err)
	}
	if err := CheckSchema(ctx, connection); err != nil {
		t.Fatalf("restored current-install index after opclass check was not ready: %v", err)
	}
	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_workspace_order_idx`); err != nil {
		t.Fatalf("drop Workspace ordering index: %v", err)
	}
	if _, err := connection.Exec(ctx, `CREATE INDEX installations_workspace_order_idx ON workspace.installations USING hash (workspace_id)`); err != nil {
		t.Fatalf("create hash Workspace ordering index: %v", err)
	}
	if err := CheckSchema(ctx, connection); err == nil {
		t.Fatal("hash Workspace ordering index was reported ready")
	}
	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_workspace_order_idx`); err != nil {
		t.Fatalf("drop hash Workspace ordering index: %v", err)
	}
	if _, err := connection.Exec(ctx, `CREATE INDEX installations_workspace_order_idx ON workspace.installations (agent_id)`); err != nil {
		t.Fatalf("create degraded Workspace ordering index: %v", err)
	}
	if err := CheckSchema(ctx, connection); err == nil {
		t.Fatal("degraded Workspace ordering index was reported ready")
	}
	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_workspace_order_idx`); err != nil {
		t.Fatalf("drop degraded Workspace ordering index: %v", err)
	}
	if _, err := connection.Exec(ctx, `CREATE INDEX installations_workspace_order_idx ON workspace.installations (workspace_id, installed_at ASC, installation_id ASC)`); err != nil {
		t.Fatalf("restore Workspace ordering index: %v", err)
	}
	if err := CheckSchema(ctx, connection); err != nil {
		t.Fatalf("restored Workspace ordering index was not ready: %v", err)
	}
	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_workspace_order_idx`); err != nil {
		t.Fatalf("degrade Workspace schema: %v", err)
	}
	if err := CheckSchema(ctx, connection); err == nil {
		t.Fatal("incomplete Workspace schema was reported ready")
	}
}
