package ledger

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/tern/v2/migrate"
)

const ExpectedSchemaVersion int32 = 1

var ErrSchemaVersionMismatch = errors.New("ledger schema version mismatch")

//go:embed 001_ledger.sql
var migration001 []byte

var migrationFiles = fstest.MapFS{
	"001_ledger.sql": &fstest.MapFile{Data: migration001, Mode: 0o444},
}

type RowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Migrate(ctx context.Context, conn *pgx.Conn, direction string) error {
	if conn == nil {
		return errors.New("ledger migration connection is required")
	}
	if direction != "up" {
		return fmt.Errorf("ledger migration direction %q is unsupported", direction)
	}
	if _, err := conn.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS ledger`); err != nil {
		return fmt.Errorf("create ledger migration schema: %w", err)
	}
	migrator, err := migrate.NewMigrator(ctx, conn, "ledger.schema_version")
	if err != nil {
		return fmt.Errorf("initialize ledger migrator: %w", err)
	}
	if err := migrator.LoadMigrations(migrationFiles); err != nil {
		return fmt.Errorf("load embedded ledger migrations: %w", err)
	}
	if len(migrator.Migrations) != int(ExpectedSchemaVersion) {
		return fmt.Errorf("embedded ledger migration count: %w", ErrSchemaVersionMismatch)
	}
	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate ledger up: %w", err)
	}
	return nil
}

func CheckSchema(ctx context.Context, db RowQuerier) error {
	if db == nil {
		return errors.New("ledger readiness database is required")
	}
	var version int32
	var projectionColumns, eventColumns int
	var projectionColumnShape, eventColumnShape int
	var projectionChecks, eventChecks int
	var projectionConstraintNames, eventConstraintNames, indexesReady, immutableTrigger bool
	err := db.QueryRow(ctx, `
SELECT version,
       (SELECT count(*) FROM information_schema.columns WHERE table_schema = 'ledger' AND table_name = 'invocations'),
       (SELECT count(*) FROM information_schema.columns WHERE table_schema = 'ledger' AND table_name = 'invocation_events'),
       (SELECT count(*) FROM information_schema.columns
        WHERE table_schema = 'ledger' AND table_name = 'invocations'
          AND ((column_name IN ('invocation_id','root_task_id','parent_invocation_id','trace_id','caller_id','workspace_id','target_agent_id','agent_card_version','capability')
                AND data_type = 'character varying' AND character_maximum_length = 128 AND collation_name = 'C')
            OR (column_name IN ('caller_type','status') AND data_type = 'character varying' AND character_maximum_length = 16 AND collation_name = 'C')
            OR (column_name = 'error_code' AND data_type = 'character varying' AND character_maximum_length = 64 AND collation_name = 'C')
            OR (column_name = 'latency_ms' AND data_type = 'bigint')
            OR (column_name IN ('created_at','updated_at') AND data_type = 'timestamp with time zone' AND datetime_precision = 6))
          AND is_nullable = CASE WHEN column_name IN ('parent_invocation_id','latency_ms','error_code') THEN 'YES' ELSE 'NO' END),
       (SELECT count(*) FROM information_schema.columns
        WHERE table_schema = 'ledger' AND table_name = 'invocation_events'
          AND ((column_name IN ('event_id','invocation_id','root_task_id','parent_invocation_id','trace_id','caller_id','workspace_id','target_agent_id','agent_card_version','capability')
                AND data_type = 'character varying' AND character_maximum_length = 128 AND collation_name = 'C')
            OR (column_name IN ('event_type','status','caller_type') AND data_type = 'character varying' AND character_maximum_length = 16 AND collation_name = 'C')
            OR (column_name = 'error_code' AND data_type = 'character varying' AND character_maximum_length = 64 AND collation_name = 'C')
            OR (column_name IN ('sequence','chunk_index','chunk_bytes','latency_ms') AND data_type = 'bigint')
            OR (column_name = 'occurred_at' AND data_type = 'timestamp with time zone' AND datetime_precision = 6))
          AND is_nullable = CASE WHEN column_name IN ('parent_invocation_id','chunk_index','chunk_bytes','latency_ms','error_code') THEN 'YES' ELSE 'NO' END),
       (SELECT count(*) FROM pg_constraint WHERE conrelid = to_regclass('ledger.invocations')),
       (SELECT count(*) FROM pg_constraint WHERE conrelid = to_regclass('ledger.invocation_events')),
       (SELECT array_agg(conname::text ORDER BY conname) = ARRAY[
           'invocations_caller_type','invocations_error_code','invocations_identifier_format',
           'invocations_latency_nonnegative','invocations_parent_fk','invocations_pkey','invocations_status','invocations_terminal_shape',
           'invocations_timestamp_order','invocations_trace_format'
        ] FROM pg_constraint WHERE conrelid = to_regclass('ledger.invocations')),
       (SELECT array_agg(conname::text ORDER BY conname) = ARRAY[
           'invocation_events_caller_type','invocation_events_counter_nonnegative','invocation_events_error_code',
           'invocation_events_field_shape','invocation_events_identifier_format','invocation_events_invocation_fk',
           'invocation_events_pkey','invocation_events_sequence_nonnegative','invocation_events_sequence_unique',
           'invocation_events_terminal_error','invocation_events_trace_format','invocation_events_type_status'
        ] FROM pg_constraint WHERE conrelid = to_regclass('ledger.invocation_events')),
       (SELECT count(*) = 3 FROM pg_index i
        JOIN pg_class idx ON idx.oid = i.indexrelid
        JOIN pg_class tbl ON tbl.oid = i.indrelid
        JOIN pg_am am ON am.oid = idx.relam
        WHERE tbl.oid = to_regclass('ledger.invocations')
          AND idx.relname IN ('invocations_trace_order_idx','invocations_root_order_idx','invocations_parent_order_idx')
          AND i.indisvalid AND NOT i.indisunique AND am.amname = 'btree'
          AND i.indpred IS NULL AND i.indexprs IS NULL AND i.indnkeyatts = 4
          AND i.indoption = '0 0 0 0'::int2vector
          AND i.indkey = CASE idx.relname
              WHEN 'invocations_trace_order_idx' THEN '7 4 14 1'::int2vector
              WHEN 'invocations_root_order_idx' THEN '7 2 14 1'::int2vector
              WHEN 'invocations_parent_order_idx' THEN '7 3 14 1'::int2vector
          END),
       EXISTS (
           SELECT 1 FROM pg_trigger
           WHERE tgrelid = to_regclass('ledger.invocation_events')
             AND tgname = 'invocation_events_immutable'
             AND tgenabled = 'O' AND NOT tgisinternal
       )
FROM ledger.schema_version`).Scan(
		&version, &projectionColumns, &eventColumns, &projectionColumnShape, &eventColumnShape,
		&projectionChecks, &eventChecks,
		&projectionConstraintNames, &eventConstraintNames, &indexesReady, &immutableTrigger,
	)
	if err != nil {
		return fmt.Errorf("read ledger schema: %w", err)
	}
	if version != ExpectedSchemaVersion || projectionColumns != 15 || eventColumns != 19 ||
		projectionColumnShape != 15 || eventColumnShape != 19 ||
		projectionChecks != 10 || eventChecks != 12 || !projectionConstraintNames ||
		!eventConstraintNames || !indexesReady || !immutableTrigger {
		return ErrSchemaVersionMismatch
	}
	return nil
}
