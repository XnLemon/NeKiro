//go:build integration

package postgres

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCheckSchemaRejectsIncompleteSchemaV2(t *testing.T) {
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
