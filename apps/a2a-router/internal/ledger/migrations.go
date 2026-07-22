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

const ExpectedSchemaVersion int32 = 2

var ErrSchemaVersionMismatch = errors.New("ledger schema version mismatch")

//go:embed 001_ledger.sql
var migration001 []byte

//go:embed 002_release_provenance.sql
var migration002 []byte

var migrationFiles = fstest.MapFS{
	"001_ledger.sql":             &fstest.MapFile{Data: migration001, Mode: 0o444},
	"002_release_provenance.sql": &fstest.MapFile{Data: migration002, Mode: 0o444},
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
	var projectionConstraintsReady, eventConstraintsReady, indexesReady, immutableTrigger bool
	err := db.QueryRow(ctx, `
SELECT version,
       (SELECT count(*) FROM information_schema.columns WHERE table_schema = 'ledger' AND table_name = 'invocations'),
       (SELECT count(*) FROM information_schema.columns WHERE table_schema = 'ledger' AND table_name = 'invocation_events'),
       (SELECT count(*) FROM information_schema.columns
        WHERE table_schema = 'ledger' AND table_name = 'invocations'
          AND ((column_name IN ('invocation_id','root_task_id','parent_invocation_id','trace_id','caller_id','workspace_id','target_agent_id','agent_card_version','agent_release_id','capability')
                AND data_type = 'character varying' AND character_maximum_length = 128 AND collation_name = 'C')
            OR (column_name IN ('caller_type','status') AND data_type = 'character varying' AND character_maximum_length = 16 AND collation_name = 'C')
            OR (column_name = 'agent_card_digest' AND data_type = 'bytea')
            OR (column_name = 'error_code' AND data_type = 'character varying' AND character_maximum_length = 64 AND collation_name = 'C')
            OR (column_name = 'latency_ms' AND data_type = 'bigint')
            OR (column_name IN ('created_at','updated_at') AND data_type = 'timestamp with time zone' AND datetime_precision = 6))
          AND is_nullable = CASE WHEN column_name IN ('parent_invocation_id','agent_release_id','agent_card_digest','latency_ms','error_code') THEN 'YES' ELSE 'NO' END),
       (SELECT count(*) FROM information_schema.columns
        WHERE table_schema = 'ledger' AND table_name = 'invocation_events'
          AND ((column_name IN ('event_id','invocation_id','root_task_id','parent_invocation_id','trace_id','caller_id','workspace_id','target_agent_id','agent_card_version','agent_release_id','capability')
                AND data_type = 'character varying' AND character_maximum_length = 128 AND collation_name = 'C')
            OR (column_name IN ('event_type','status','caller_type') AND data_type = 'character varying' AND character_maximum_length = 16 AND collation_name = 'C')
            OR (column_name = 'agent_card_digest' AND data_type = 'bytea')
            OR (column_name = 'error_code' AND data_type = 'character varying' AND character_maximum_length = 64 AND collation_name = 'C')
            OR (column_name IN ('sequence','chunk_index','chunk_bytes','latency_ms') AND data_type = 'bigint')
            OR (column_name = 'occurred_at' AND data_type = 'timestamp with time zone' AND datetime_precision = 6))
          AND is_nullable = CASE WHEN column_name IN ('parent_invocation_id','agent_release_id','agent_card_digest','chunk_index','chunk_bytes','latency_ms','error_code') THEN 'YES' ELSE 'NO' END),
       (SELECT count(*) = 13 AND bool_and(
            convalidated AND NOT condeferrable AND NOT condeferred AND
            CASE conname
              WHEN 'invocations_pkey' THEN contype = 'p' AND conkey = ARRAY[1]::smallint[]
              WHEN 'invocations_parent_fk' THEN contype = 'f' AND conkey = ARRAY[3]::smallint[]
                   AND confrelid = to_regclass('ledger.invocations') AND confkey = ARRAY[1]::smallint[]
                   AND confupdtype = 'a' AND confdeltype = 'a' AND confmatchtype = 's'
              WHEN 'invocations_identifier_format' THEN contype = 'c' AND conkey = ARRAY[1,2,3,6,7,8,10]::smallint[] AND NOT connoinherit
              WHEN 'invocations_trace_format' THEN contype = 'c' AND conkey = ARRAY[4]::smallint[] AND NOT connoinherit
              WHEN 'invocations_caller_type' THEN contype = 'c' AND conkey = ARRAY[5]::smallint[] AND NOT connoinherit
              WHEN 'invocations_status' THEN contype = 'c' AND conkey = ARRAY[11]::smallint[] AND NOT connoinherit
              WHEN 'invocations_latency_nonnegative' THEN contype = 'c' AND conkey = ARRAY[12]::smallint[] AND NOT connoinherit
              WHEN 'invocations_error_code' THEN contype = 'c' AND conkey = ARRAY[13]::smallint[] AND NOT connoinherit
              WHEN 'invocations_timestamp_order' THEN contype = 'c' AND conkey = ARRAY[14,15]::smallint[] AND NOT connoinherit
              WHEN 'invocations_terminal_shape' THEN contype = 'c' AND conkey = ARRAY[11,12,13]::smallint[] AND NOT connoinherit
              WHEN 'invocations_release_id_format' THEN contype = 'c' AND conkey = ARRAY[16]::smallint[] AND NOT connoinherit
              WHEN 'invocations_release_digest_length' THEN contype = 'c' AND conkey = ARRAY[17]::smallint[] AND NOT connoinherit
              WHEN 'invocations_release_provenance_pair' THEN contype = 'c' AND conkey = ARRAY[16,17]::smallint[] AND NOT connoinherit
              ELSE false
            END
            AND (contype <> 'c' OR pg_get_constraintdef(oid, true) <> 'CHECK (true)')
        ) FROM pg_constraint WHERE conrelid = to_regclass('ledger.invocations')),
       (SELECT count(*) = 15 AND bool_and(
            convalidated AND NOT condeferrable AND NOT condeferred AND
            CASE conname
              WHEN 'invocation_events_pkey' THEN contype = 'p' AND conkey = ARRAY[1]::smallint[]
              WHEN 'invocation_events_invocation_fk' THEN contype = 'f' AND conkey = ARRAY[2]::smallint[]
                   AND confrelid = to_regclass('ledger.invocations') AND confkey = ARRAY[1]::smallint[]
                   AND confupdtype = 'a' AND confdeltype = 'a' AND confmatchtype = 's'
              WHEN 'invocation_events_sequence_unique' THEN contype = 'u' AND conkey = ARRAY[2,3]::smallint[]
              WHEN 'invocation_events_sequence_nonnegative' THEN contype = 'c' AND conkey = ARRAY[3]::smallint[] AND NOT connoinherit
              WHEN 'invocation_events_identifier_format' THEN contype = 'c' AND conkey = ARRAY[1,2,7,8,11,12,13,15]::smallint[] AND NOT connoinherit
              WHEN 'invocation_events_trace_format' THEN contype = 'c' AND conkey = ARRAY[9]::smallint[] AND NOT connoinherit
              WHEN 'invocation_events_caller_type' THEN contype = 'c' AND conkey = ARRAY[10]::smallint[] AND NOT connoinherit
              WHEN 'invocation_events_counter_nonnegative' THEN contype = 'c' AND conkey = ARRAY[16,17,18]::smallint[] AND NOT connoinherit
              WHEN 'invocation_events_type_status' THEN contype = 'c' AND conkey = ARRAY[5,6]::smallint[] AND NOT connoinherit
              WHEN 'invocation_events_field_shape' THEN contype = 'c' AND conkey = ARRAY[5,16,17,18,19]::smallint[] AND NOT connoinherit
              WHEN 'invocation_events_terminal_error' THEN contype = 'c' AND conkey = ARRAY[5,19]::smallint[] AND NOT connoinherit
              WHEN 'invocation_events_error_code' THEN contype = 'c' AND conkey = ARRAY[19]::smallint[] AND NOT connoinherit
              WHEN 'invocation_events_release_id_format' THEN contype = 'c' AND conkey = ARRAY[20]::smallint[] AND NOT connoinherit
              WHEN 'invocation_events_release_digest_length' THEN contype = 'c' AND conkey = ARRAY[21]::smallint[] AND NOT connoinherit
              WHEN 'invocation_events_release_provenance_pair' THEN contype = 'c' AND conkey = ARRAY[20,21]::smallint[] AND NOT connoinherit
              ELSE false
            END
            AND (contype <> 'c' OR pg_get_constraintdef(oid, true) <> 'CHECK (true)')
        ) FROM pg_constraint WHERE conrelid = to_regclass('ledger.invocation_events')),
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
           SELECT 1 FROM pg_trigger t
           JOIN pg_proc p ON p.oid = t.tgfoid
           JOIN pg_language l ON l.oid = p.prolang
           WHERE t.tgrelid = to_regclass('ledger.invocation_events')
             AND t.tgname = 'invocation_events_immutable'
             AND t.tgenabled = 'O' AND NOT t.tgisinternal AND t.tgtype = 27
             AND p.pronamespace = 'ledger'::regnamespace
             AND p.proname = 'reject_invocation_event_mutation'
             AND p.prorettype = 'trigger'::regtype AND p.prokind = 'f'
             AND l.lanname = 'plpgsql'
             AND regexp_replace(p.prosrc, '\s+', '', 'g') =
                 'BEGINRAISEEXCEPTION''invocationeventsareimmutable''USINGERRCODE=''55000'';END;'
       )
FROM ledger.schema_version`).Scan(
		&version, &projectionColumns, &eventColumns, &projectionColumnShape, &eventColumnShape,
		&projectionConstraintsReady, &eventConstraintsReady, &indexesReady, &immutableTrigger,
	)
	if err != nil {
		return fmt.Errorf("read ledger schema: %w", err)
	}
	if version != ExpectedSchemaVersion || projectionColumns != 17 || eventColumns != 21 ||
		projectionColumnShape != 17 || eventColumnShape != 21 ||
		!projectionConstraintsReady || !eventConstraintsReady || !indexesReady || !immutableTrigger {
		return ErrSchemaVersionMismatch
	}
	return nil
}
