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

func TestEmbeddedMigrationsMatchOwnedSQLFiles(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		embedded string
	}{
		{name: "schema v1", filename: "001_catalog.sql", embedded: migration001},
		{name: "schema v2", filename: "002_card_text.sql", embedded: migration002},
		{name: "schema v3", filename: "003_trusted_publication.sql", embedded: migration003},
		{name: "schema v4", filename: "004_agent_release.sql", embedded: string(migration004)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", test.filename))
			if err != nil {
				t.Fatalf("read owned migration: %v", err)
			}
			want := strings.ReplaceAll(string(data), "\r\n", "\n")
			got := strings.ReplaceAll(test.embedded, "\r\n", "\n")
			if got != want {
				t.Fatalf("embedded migration differs from apps/control-plane/migrations/%s", test.filename)
			}
		})
	}
}
