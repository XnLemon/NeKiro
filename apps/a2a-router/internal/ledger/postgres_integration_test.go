//go:build integration

package ledger

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresLedgerLifecycleLineageAndRestart(t *testing.T) {
	ctx := context.Background()
	databaseURL := requiredIntegrationDatabase(t)
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect Ledger integration database: %v", err)
	}
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, `DROP SCHEMA IF EXISTS ledger CASCADE`); err != nil {
		t.Fatalf("reset Ledger schema: %v", err)
	}
	if err := Migrate(ctx, connection, "up"); err != nil {
		t.Fatalf("migrate Ledger schema: %v", err)
	}
	if err := Migrate(ctx, connection, "up"); err != nil {
		t.Fatalf("repeat Ledger migration: %v", err)
	}
	if err := CheckSchema(ctx, connection); err != nil {
		t.Fatalf("valid Ledger schema was not ready: %v", err)
	}
	assertStrictReadiness(t, ctx, connection)

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open Ledger pool: %v", err)
	}
	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf("construct Ledger store: %v", err)
	}

	base := baseEvent("inv-root", "task-root", "trace-root", "workspace-a", time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC))
	appendEvent(t, store, base)
	appendEvent(t, store, nextEvent(base, 1, "routing", "routing"))
	running := nextEvent(base, 2, "started", "running")
	appendEvent(t, store, running)
	chunkIndex, chunkBytes := int64(0), int64(17)
	stream := nextEvent(base, 3, "stream", "running")
	stream.ChunkIndex, stream.ChunkBytes = &chunkIndex, &chunkBytes
	appendEvent(t, store, stream)
	latency := int64(42)
	succeeded := nextEvent(base, 4, "succeeded", "succeeded")
	succeeded.LatencyMS = &latency
	appendEvent(t, store, succeeded)

	detail, err := store.GetInvocation(ctx, "workspace-a", "inv-root")
	if err != nil {
		t.Fatalf("read completed Invocation: %v", err)
	}
	if detail.Invocation.Status != "succeeded" || len(detail.Events) != 5 || detail.Events[3].ChunkBytes == nil || *detail.Events[3].ChunkBytes != 17 {
		t.Fatalf("completed Invocation detail = %#v", detail)
	}
	trusted := baseEvent("inv-trusted-release", "task-trusted-release", "trace-trusted-release", "workspace-a", time.Date(2026, 7, 16, 12, 0, 10, 0, time.UTC))
	trusted.AgentReleaseID = "release-trusted"
	trusted.AgentCardDigest = strings.Repeat("a", 64)
	appendEvent(t, store, trusted)
	appendEvent(t, store, nextEvent(trusted, 1, "routing", "routing"))
	appendEvent(t, store, nextEvent(trusted, 2, "started", "running"))
	trustedSuccess := nextEvent(trusted, 3, "succeeded", "succeeded")
	trustedSuccess.LatencyMS = &latency
	appendEvent(t, store, trustedSuccess)
	trustedDetail, err := store.GetInvocation(ctx, "workspace-a", trusted.InvocationID)
	if err != nil || trustedDetail.Invocation.AgentReleaseID != trusted.AgentReleaseID || trustedDetail.Invocation.AgentCardDigest != trusted.AgentCardDigest || trustedDetail.Events[3].AgentReleaseID != trusted.AgentReleaseID || trustedDetail.Events[3].AgentCardDigest != trusted.AgentCardDigest {
		t.Fatalf("trusted release provenance = %#v, %v", trustedDetail, err)
	}
	beforeCount := eventCount(t, ctx, pool, "inv-root")
	late := nextEvent(base, 5, "failed", "failed")
	late.LatencyMS = &latency
	late.Error = platformError(t, contracts.ErrorCodeAgentExecutionFailed, late)
	if err := store.Append(ctx, late); !errors.Is(err, ErrValidation) {
		t.Fatalf("post-terminal append error = %v, want validation", err)
	}
	if got := eventCount(t, ctx, pool, "inv-root"); got != beforeCount {
		t.Fatalf("post-terminal append changed event count to %d", got)
	}
	if _, err := pool.Exec(ctx, `UPDATE ledger.invocation_events SET status = 'failed' WHERE event_id = $1`, base.EventID); err == nil {
		t.Fatal("direct immutable event update succeeded")
	}

	assertConcurrentTerminal(t, ctx, store, pool)
	assertNestedLineage(t, ctx, store)
	assertPersistenceShape(t, ctx, pool)

	pool.Close()
	restartedPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("reconstruct Ledger pool: %v", err)
	}
	restarted, err := NewStore(restartedPool)
	if err != nil {
		t.Fatalf("reconstruct Ledger store: %v", err)
	}
	defer restartedPool.Close()
	restartedDetail, err := restarted.GetInvocation(ctx, "workspace-a", "inv-root")
	if err != nil || restartedDetail.Invocation.Status != "succeeded" || len(restartedDetail.Events) != 5 {
		t.Fatalf("restart Invocation detail = %#v, %v", restartedDetail, err)
	}
	if _, err := restarted.GetInvocation(ctx, "workspace-b", "inv-root"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign Workspace Invocation read = %v, want not found", err)
	}
	trace, err := restarted.GetTrace(ctx, "workspace-a", "trace-nested")
	if err != nil {
		t.Fatalf("restart nested Trace read: %v", err)
	}
	if len(trace.Invocations) != 2 || trace.Invocations[0].InvocationID != "inv-parent" || trace.Invocations[1].ParentInvocationID != "inv-parent" {
		t.Fatalf("nested Trace order = %#v", trace.Invocations)
	}
	if _, err := restarted.GetTrace(ctx, "workspace-b", "trace-nested"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign Workspace Trace read = %v, want not found", err)
	}
}

func TestPostgresLedgerDependencyFailureIsExplicit(t *testing.T) {
	ctx := context.Background()
	databaseURL := requiredIntegrationDatabase(t)
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open Ledger dependency pool: %v", err)
	}
	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf("construct Ledger dependency store: %v", err)
	}
	pool.Close()
	if _, err := store.GetInvocation(ctx, "workspace-a", "inv-root"); !errors.Is(err, ErrDependency) {
		t.Fatalf("closed database read = %v, want dependency failure", err)
	}
}

func assertStrictReadiness(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	assertSchemaMutationRejected(t, ctx, connection, `DROP INDEX ledger.invocations_trace_order_idx`)
	assertSchemaMutationRejected(t, ctx, connection,
		`ALTER TABLE ledger.invocations DROP CONSTRAINT invocations_status`,
		`ALTER TABLE ledger.invocations ADD CONSTRAINT invocations_status CHECK (true)`)
	assertSchemaMutationRejected(t, ctx, connection, `
CREATE OR REPLACE FUNCTION ledger.reject_invocation_event_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$ BEGIN RETURN OLD; END; $$`)
	assertSchemaMutationRejected(t, ctx, connection,
		`DROP TRIGGER invocation_events_immutable ON ledger.invocation_events`,
		`CREATE TRIGGER invocation_events_immutable
         BEFORE UPDATE ON ledger.invocation_events
         FOR EACH ROW EXECUTE FUNCTION ledger.reject_invocation_event_mutation()`)
	if _, err := connection.Exec(ctx, `UPDATE ledger.schema_version SET version = $1`, ExpectedSchemaVersion+1); err != nil {
		t.Fatalf("make Ledger schema stale: %v", err)
	}
	if err := CheckSchema(ctx, connection); !errors.Is(err, ErrSchemaVersionMismatch) {
		t.Fatalf("stale Ledger readiness = %v", err)
	}
	if _, err := connection.Exec(ctx, `UPDATE ledger.schema_version SET version = $1`, ExpectedSchemaVersion); err != nil {
		t.Fatalf("restore Ledger schema version: %v", err)
	}
}

func assertSchemaMutationRejected(t *testing.T, ctx context.Context, connection *pgx.Conn, statements ...string) {
	t.Helper()
	tx, err := connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin readiness mutation: %v", err)
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("apply readiness mutation: %v", err)
		}
	}
	if err := CheckSchema(ctx, tx); !errors.Is(err, ErrSchemaVersionMismatch) {
		_ = tx.Rollback(ctx)
		t.Fatalf("weakened Ledger schema readiness = %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback readiness mutation: %v", err)
	}
}

func assertConcurrentTerminal(t *testing.T, ctx context.Context, store *Store, pool *pgxpool.Pool) {
	t.Helper()
	base := baseEvent("inv-race", "task-race", "trace-race", "workspace-a", time.Date(2026, 7, 16, 12, 1, 0, 0, time.UTC))
	appendEvent(t, store, base)
	appendEvent(t, store, nextEvent(base, 1, "routing", "routing"))
	appendEvent(t, store, nextEvent(base, 2, "started", "running"))
	latency := int64(7)
	success := nextEvent(base, 3, "succeeded", "succeeded")
	success.EventID = "event-inv-race-success"
	success.LatencyMS = &latency
	failure := nextEvent(base, 3, "failed", "failed")
	failure.EventID = "event-inv-race-failure"
	failure.LatencyMS = &latency
	failure.Error = platformError(t, contracts.ErrorCodeAgentExecutionFailed, failure)
	results := make(chan error, 2)
	var start sync.WaitGroup
	start.Add(2)
	for _, event := range []contracts.InvocationEventV03{success, failure} {
		event := event
		go func() {
			start.Done()
			start.Wait()
			results <- store.Append(ctx, event)
		}()
	}
	committed := 0
	for range 2 {
		if err := <-results; err == nil {
			committed++
		} else if !errors.Is(err, ErrValidation) {
			t.Fatalf("concurrent terminal error = %v", err)
		}
	}
	if committed != 1 || eventCount(t, ctx, pool, "inv-race") != 4 {
		t.Fatalf("concurrent terminal committed=%d count=%d", committed, eventCount(t, ctx, pool, "inv-race"))
	}
}

func assertNestedLineage(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	parent := baseEvent("inv-parent", "task-nested", "trace-nested", "workspace-a", time.Date(2026, 7, 16, 12, 2, 0, 0, time.UTC))
	parent.TargetAgentID = "agent-parent"
	appendEvent(t, store, parent)
	appendEvent(t, store, nextEvent(parent, 1, "routing", "routing"))
	appendEvent(t, store, nextEvent(parent, 2, "started", "running"))
	child := baseEvent("inv-child", parent.RootTaskID, parent.TraceID, parent.WorkspaceID, time.Date(2026, 7, 16, 12, 2, 1, 0, time.UTC))
	child.ParentInvocationID = parent.InvocationID
	child.Caller = contracts.Caller{Type: "agent", ID: parent.TargetAgentID}
	appendEvent(t, store, child)

	invalid := child
	invalid.InvocationID = "inv-child-invalid"
	invalid.EventID = "event-inv-child-invalid-0"
	invalid.TraceID = "trace-forged"
	if err := store.Append(ctx, invalid); !errors.Is(err, ErrValidation) {
		t.Fatalf("forged child append = %v, want validation", err)
	}
	if _, err := store.GetInvocation(ctx, "workspace-a", invalid.InvocationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("forged child partial projection read = %v", err)
	}
}

func assertPersistenceShape(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `
SELECT column_name, data_type
FROM information_schema.columns
WHERE table_schema = 'ledger' AND table_name IN ('invocations','invocation_events')`)
	if err != nil {
		t.Fatalf("inspect Ledger columns: %v", err)
	}
	defer rows.Close()
	prohibited := []string{"input", "output", "result", "chunk_value", "credential", "token", "endpoint", "message", "raw", "payload"}
	for rows.Next() {
		var name, dataType string
		if err := rows.Scan(&name, &dataType); err != nil {
			t.Fatalf("scan Ledger column: %v", err)
		}
		for _, fragment := range prohibited {
			if strings.Contains(name, fragment) {
				t.Fatalf("prohibited Ledger column %q", name)
			}
		}
		if dataType == "json" || dataType == "jsonb" {
			t.Fatalf("generic content-capable Ledger column %q", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate Ledger columns: %v", err)
	}
}

func baseEvent(invocationID, rootTaskID string, traceID contracts.TraceID, workspaceID string, at time.Time) contracts.InvocationEventV03 {
	return contracts.InvocationEventV03{
		SchemaVersion: contracts.RuntimeInvocationEventSchemaVersion,
		EventID:       "event-" + invocationID + "-0", Sequence: 0, OccurredAt: at.UTC().Format(time.RFC3339Nano),
		Type: "created", Status: "pending", InvocationID: invocationID, RootTaskID: rootTaskID,
		TraceID: traceID, Caller: contracts.Caller{Type: "user", ID: "user-a"}, WorkspaceID: workspaceID,
		TargetAgentID: "agent-target", AgentCardVersion: "1.0.0", Capability: "document.read",
	}
}

func nextEvent(base contracts.InvocationEventV03, sequence int64, eventType, status string) contracts.InvocationEventV03 {
	event := base
	event.EventID = "event-" + base.InvocationID + "-" + eventType
	event.Sequence = sequence
	event.OccurredAt = mustEventTime(base.OccurredAt).Add(time.Duration(sequence) * time.Microsecond).Format(time.RFC3339Nano)
	event.Type, event.Status = eventType, status
	event.ChunkIndex, event.ChunkBytes, event.LatencyMS, event.Error = nil, nil, nil, nil
	return event
}

func platformError(t *testing.T, code contracts.PlatformErrorCode, event contracts.InvocationEventV03) *contracts.PlatformErrorV4 {
	t.Helper()
	value, err := contracts.NewCorrelatedPlatformErrorV4(code, event.TraceID, event.InvocationID, event.RootTaskID)
	if err != nil {
		t.Fatalf("construct terminal Platform Error: %v", err)
	}
	return &value
}

func appendEvent(t *testing.T, store *Store, event contracts.InvocationEventV03) {
	t.Helper()
	if err := store.Append(context.Background(), event); err != nil {
		t.Fatalf("append %s event: %v", event.Type, err)
	}
}

func eventCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, invocationID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ledger.invocation_events WHERE invocation_id = $1`, invocationID).Scan(&count); err != nil {
		t.Fatalf("count Invocation events: %v", err)
	}
	return count
}

func mustEventTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func requiredIntegrationDatabase(t *testing.T) string {
	t.Helper()
	value := os.Getenv("NEKIRO_TEST_DATABASE_URL")
	if strings.TrimSpace(value) == "" {
		t.Fatal("NEKIRO_TEST_DATABASE_URL is required for Ledger integration tests")
	}
	configuration, err := pgxpool.ParseConfig(value)
	if err != nil || !strings.HasSuffix(configuration.ConnConfig.Database, "_test") {
		t.Fatal("Ledger integration database must be valid and end in _test")
	}
	return value
}
