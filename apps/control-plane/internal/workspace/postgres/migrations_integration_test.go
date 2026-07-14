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
	if _, err := connection.Exec(ctx, `DROP INDEX workspace.installations_workspace_order_idx`); err != nil {
		t.Fatalf("degrade Workspace schema: %v", err)
	}
	if err := CheckSchema(ctx, connection); err == nil {
		t.Fatal("incomplete Workspace schema was reported ready")
	}
}
