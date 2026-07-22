package ledger

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCheckSchemaRequiresDatabaseAndPropagatesQueryFailure(t *testing.T) {
	if err := CheckSchema(context.Background(), nil); err == nil {
		t.Fatal("nil readiness database accepted")
	}
	queryErr := errors.New("database unavailable")
	if err := CheckSchema(context.Background(), rowQuerierStub{row: scanRow{err: queryErr}}); !errors.Is(err, queryErr) {
		t.Fatalf("query error = %v, want %v", err, queryErr)
	}
}

func TestCheckSchemaAcceptsExactShapeAndRejectsMismatch(t *testing.T) {
	ready := []any{int32(ExpectedSchemaVersion), 17, 21, 17, 21, true, true, true, true}
	if err := CheckSchema(context.Background(), rowQuerierStub{row: scanRow{values: ready}}); err != nil {
		t.Fatalf("exact schema rejected: %v", err)
	}
	weakened := append([]any(nil), ready...)
	weakened[7] = false
	if err := CheckSchema(context.Background(), rowQuerierStub{row: scanRow{values: weakened}}); !errors.Is(err, ErrSchemaVersionMismatch) {
		t.Fatalf("weakened schema error = %v", err)
	}
}

func TestMigrateRequiresConnection(t *testing.T) {
	if err := Migrate(context.Background(), nil, "up"); err == nil {
		t.Fatal("nil migration connection accepted")
	}
}

type rowQuerierStub struct{ row pgx.Row }

func (stub rowQuerierStub) QueryRow(context.Context, string, ...any) pgx.Row { return stub.row }

type scanRow struct {
	values []any
	err    error
}

func (row scanRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	return (valueScanner{values: row.values}).Scan(dest...)
}
