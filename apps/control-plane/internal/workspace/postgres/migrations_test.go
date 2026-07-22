package postgres

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateRejectsUnsupportedDirectionBeforeUsingConnection(t *testing.T) {
	for _, direction := range []string{"down", "sideways", ""} {
		if err := Migrate(context.Background(), nil, direction); err == nil {
			t.Fatalf("Migrate direction %q succeeded", direction)
		}
	}
}

func TestEmbeddedMigrationMatchesOwnedSQLFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "003_workspace.sql"))
	if err != nil {
		t.Fatalf("read Workspace migration: %v", err)
	}
	want := strings.ReplaceAll(string(data), "\r\n", "\n")
	got := strings.ReplaceAll(migration001, "\r\n", "\n")
	if got != want {
		t.Fatal("embedded Workspace migration differs from apps/control-plane/migrations/003_workspace.sql")
	}
}

func TestEmbeddedReleaseMigrationMatchesOwnedSQLFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "004_workspace_installation_release.sql"))
	if err != nil {
		t.Fatalf("read Workspace release migration: %v", err)
	}
	want := strings.ReplaceAll(string(data), "\r\n", "\n")
	got := strings.ReplaceAll(string(migration002), "\r\n", "\n")
	if got != want {
		t.Fatal("embedded Workspace release migration differs from apps/control-plane/migrations/004_workspace_installation_release.sql")
	}
}
