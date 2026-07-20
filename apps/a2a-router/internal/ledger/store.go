package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool      *pgxpool.Pool
	validator *contracts.RuntimeContractValidator
}

func NewStore(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, errors.New("ledger database pool is required")
	}
	validator, err := contracts.NewRuntimeContractValidator()
	if err != nil {
		return nil, fmt.Errorf("initialize ledger contract validator: %w", err)
	}
	return &Store{pool: pool, validator: validator}, nil
}

func (store *Store) Append(ctx context.Context, event contracts.InvocationEventV03) (returnErr error) {
	occurredAt, err := store.validateCandidate(event)
	if err != nil {
		return err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return dependencyError("begin append", err)
	}
	defer rollback(ctx, tx, &returnErr, "append")

	if event.Sequence == 0 {
		if event.ParentInvocationID != "" {
			parent, err := lockProjection(ctx, tx, event.ParentInvocationID)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrConflict
			}
			if err != nil {
				return dependencyError("lock parent Invocation", err)
			}
			if err := contracts.ValidateNestedInvocationCorrelation(parent, event); err != nil {
				return fmt.Errorf("%w: nested lineage", ErrValidation)
			}
			if occurredAt.Before(parent.UpdatedAt) {
				return fmt.Errorf("%w: child timestamp precedes parent state", ErrValidation)
			}
		}
		if err := insertProjection(ctx, tx, event, occurredAt); err != nil {
			return classifyWriteError("insert Invocation projection", err)
		}
	} else {
		projection, err := lockProjection(ctx, tx, event.InvocationID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return dependencyError("lock Invocation projection", err)
		}
		if occurredAt.Before(projection.UpdatedAt) {
			return fmt.Errorf("%w: event timestamp precedes projection", ErrValidation)
		}
		history, err := readEvents(ctx, tx, event.InvocationID)
		if err != nil {
			return dependencyError("read Invocation history", err)
		}
		if err := store.validator.ValidateInvocationDetailResponseV4(projection.WorkspaceID, contracts.InvocationDetailResponseV4{
			Invocation: projection,
			Events:     history,
		}); err != nil {
			return dependencyError("validate committed projection", err)
		}
		lastOccurredAt, err := time.Parse(time.RFC3339Nano, history[len(history)-1].OccurredAt)
		firstOccurredAt, firstErr := time.Parse(time.RFC3339Nano, history[0].OccurredAt)
		if err != nil || firstErr != nil || !projection.UpdatedAt.Equal(lastOccurredAt) || !projection.CreatedAt.Equal(firstOccurredAt) ||
			!sameOptionalInt64(projection.LatencyMS, history[len(history)-1].LatencyMS) ||
			projection.ErrorCode != eventErrorCode(history[len(history)-1]) {
			return dependencyError("validate projection state", errors.Join(err, firstErr))
		}
		sequence, err := contracts.NewRuntimeInvocationSequenceValidator(store.validator)
		if err != nil {
			return dependencyError("initialize lifecycle validation", err)
		}
		for _, prior := range history {
			if err := sequence.Accept(prior); err != nil {
				return dependencyError("validate committed lifecycle", err)
			}
		}
		if err := sequence.Accept(event); err != nil {
			return fmt.Errorf("%w: lifecycle transition", ErrValidation)
		}
	}

	if err := insertEvent(ctx, tx, event, occurredAt); err != nil {
		return classifyWriteError("insert Invocation event", err)
	}
	if event.Sequence > 0 {
		if err := updateProjection(ctx, tx, event, occurredAt); err != nil {
			return classifyWriteError("update Invocation projection", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return dependencyError("commit append", err)
	}
	return nil
}

func (store *Store) GetInvocation(ctx context.Context, workspaceID, invocationID string) (result contracts.InvocationDetailResponseV4, returnErr error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, dependencyError("begin Invocation read", err)
	}
	defer rollback(ctx, tx, &returnErr, "Invocation read")
	result.Invocation, err = scanProjection(tx.QueryRow(ctx, projectionSelect+`
WHERE workspace_id = $1 AND invocation_id = $2`, workspaceID, invocationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, dependencyError("read Invocation projection", err)
	}
	result.Events, err = readEvents(ctx, tx, invocationID)
	if err != nil {
		return result, dependencyError("read Invocation events", err)
	}
	if err := store.validator.ValidateInvocationDetailResponseV4(workspaceID, result); err != nil {
		return result, dependencyError("validate stored Invocation detail", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return result, dependencyError("commit Invocation read", err)
	}
	return result, nil
}

// GetInvocationByParentID reads a committed Invocation by its ID alone,
// without workspace scoping. This is a trusted internal lookup used by the
// nested Agent handler. Cross-Workspace isolation is not enforced here but
// by the caller: (a) DeriveChildContext requires the parent target to match
// the authenticated Agent, (b) the child inherits the parent's Workspace,
// and (c) Control Plane resolution validates the installation in that
// Workspace before dispatch.
func (store *Store) GetInvocationByParentID(ctx context.Context, invocationID string) (result contracts.InvocationDetailResponseV4, returnErr error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, dependencyError("begin parent Invocation read", err)
	}
	defer rollback(ctx, tx, &returnErr, "parent Invocation read")
	result.Invocation, err = scanProjection(tx.QueryRow(ctx, projectionSelect+`
WHERE invocation_id = $1`, invocationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, dependencyError("read parent Invocation projection", err)
	}
	result.Events, err = readEvents(ctx, tx, invocationID)
	if err != nil {
		return result, dependencyError("read parent Invocation events", err)
	}
	if err := store.validator.ValidateInvocationDetailResponseV4(result.Invocation.WorkspaceID, result); err != nil {
		return result, dependencyError("validate stored parent Invocation detail", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return result, dependencyError("commit parent Invocation read", err)
	}
	return result, nil
}

func (store *Store) GetTrace(ctx context.Context, workspaceID string, traceID contracts.TraceID) (contracts.TraceResponseV4, error) {
	rows, err := store.pool.Query(ctx, projectionSelect+`
WHERE workspace_id = $1 AND trace_id = $2
ORDER BY created_at ASC, invocation_id ASC`, workspaceID, traceID)
	if err != nil {
		return contracts.TraceResponseV4{}, dependencyError("read Trace projections", err)
	}
	defer rows.Close()
	result := contracts.TraceResponseV4{TraceID: traceID}
	for rows.Next() {
		projection, err := scanProjection(rows)
		if err != nil {
			return contracts.TraceResponseV4{}, dependencyError("scan Trace projection", err)
		}
		result.Invocations = append(result.Invocations, projection)
	}
	if err := rows.Err(); err != nil {
		return contracts.TraceResponseV4{}, dependencyError("iterate Trace projections", err)
	}
	if len(result.Invocations) == 0 {
		return contracts.TraceResponseV4{}, ErrNotFound
	}
	result.Invocations, err = orderTraceProjections(result.Invocations)
	if err != nil {
		return contracts.TraceResponseV4{}, dependencyError("order stored Trace", err)
	}
	if err := contracts.ValidateTraceResponseV4(workspaceID, traceID, result); err != nil {
		return contracts.TraceResponseV4{}, dependencyError("validate stored Trace", err)
	}
	return result, nil
}

func orderTraceProjections(values []contracts.InvocationRecordV4) ([]contracts.InvocationRecordV4, error) {
	ordered := make([]contracts.InvocationRecordV4, 0, len(values))
	emitted := make(map[string]struct{}, len(values))
	remaining := append([]contracts.InvocationRecordV4(nil), values...)
	for len(remaining) > 0 {
		next := remaining[:0]
		progress := false
		for _, value := range remaining {
			if value.ParentInvocationID == "" {
				ordered = append(ordered, value)
				emitted[value.InvocationID] = struct{}{}
				progress = true
				continue
			}
			if _, exists := emitted[value.ParentInvocationID]; exists {
				ordered = append(ordered, value)
				emitted[value.InvocationID] = struct{}{}
				progress = true
				continue
			}
			next = append(next, value)
		}
		if !progress {
			return nil, errors.New("trace contains missing or cyclic parent lineage")
		}
		remaining = next
	}
	return ordered, nil
}

func (store *Store) Check(ctx context.Context) error { return CheckSchema(ctx, store.pool) }

func (store *Store) validateCandidate(event contracts.InvocationEventV03) (time.Time, error) {
	if err := store.validator.ValidateInvocationEventV03(event); err != nil {
		return time.Time{}, fmt.Errorf("%w: Event 0.3", ErrValidation)
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, event.OccurredAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: occurredAt", ErrValidation)
	}
	if !occurredAt.Equal(occurredAt.Truncate(time.Microsecond)) {
		return time.Time{}, fmt.Errorf("%w: occurredAt exceeds PostgreSQL precision", ErrValidation)
	}
	if event.Sequence == 0 {
		sequence, err := contracts.NewRuntimeInvocationSequenceValidator(store.validator)
		if err != nil {
			return time.Time{}, dependencyError("initialize lifecycle validation", err)
		}
		if err := sequence.Accept(event); err != nil {
			return time.Time{}, fmt.Errorf("%w: initial lifecycle event", ErrValidation)
		}
	}
	return occurredAt.UTC(), nil
}

const projectionSelect = `
SELECT invocation_id, root_task_id, parent_invocation_id, trace_id,
       caller_type, caller_id, workspace_id, target_agent_id,
       agent_card_version, capability, status, latency_ms, error_code,
       created_at, updated_at
FROM ledger.invocations `

func lockProjection(ctx context.Context, tx pgx.Tx, invocationID string) (contracts.InvocationRecordV4, error) {
	return scanProjection(tx.QueryRow(ctx, projectionSelect+`WHERE invocation_id = $1 FOR UPDATE`, invocationID))
}

func insertProjection(ctx context.Context, tx pgx.Tx, event contracts.InvocationEventV03, occurredAt time.Time) error {
	_, err := tx.Exec(ctx, `
INSERT INTO ledger.invocations (
  invocation_id, root_task_id, parent_invocation_id, trace_id, caller_type,
  caller_id, workspace_id, target_agent_id, agent_card_version, capability,
  status, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)`,
		event.InvocationID, event.RootTaskID, nullableText(event.ParentInvocationID), event.TraceID,
		event.Caller.Type, event.Caller.ID, event.WorkspaceID, event.TargetAgentID,
		event.AgentCardVersion, event.Capability, event.Status, occurredAt)
	return err
}

func insertEvent(ctx context.Context, tx pgx.Tx, event contracts.InvocationEventV03, occurredAt time.Time) error {
	var errorCode any
	if event.Error != nil {
		errorCode = event.Error.Code
	}
	_, err := tx.Exec(ctx, `
INSERT INTO ledger.invocation_events (
  event_id, invocation_id, sequence, occurred_at, event_type, status,
  root_task_id, parent_invocation_id, trace_id, caller_type, caller_id,
  workspace_id, target_agent_id, agent_card_version, capability,
  chunk_index, chunk_bytes, latency_ms, error_code
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		event.EventID, event.InvocationID, event.Sequence, occurredAt, event.Type, event.Status,
		event.RootTaskID, nullableText(event.ParentInvocationID), event.TraceID,
		event.Caller.Type, event.Caller.ID, event.WorkspaceID, event.TargetAgentID,
		event.AgentCardVersion, event.Capability, event.ChunkIndex, event.ChunkBytes,
		event.LatencyMS, errorCode)
	return err
}

func updateProjection(ctx context.Context, tx pgx.Tx, event contracts.InvocationEventV03, occurredAt time.Time) error {
	var errorCode any
	if event.Error != nil {
		errorCode = event.Error.Code
	}
	command, err := tx.Exec(ctx, `
UPDATE ledger.invocations
SET status = $2, latency_ms = $3, error_code = $4, updated_at = $5
WHERE invocation_id = $1`, event.InvocationID, event.Status, event.LatencyMS, errorCode, occurredAt)
	if err == nil && command.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return err
}

func readEvents(ctx context.Context, tx pgx.Tx, invocationID string) ([]contracts.InvocationEventV03, error) {
	rows, err := tx.Query(ctx, `
SELECT event_id, invocation_id, sequence, occurred_at, event_type, status,
       root_task_id, parent_invocation_id, trace_id, caller_type, caller_id,
       workspace_id, target_agent_id, agent_card_version, capability,
       chunk_index, chunk_bytes, latency_ms, error_code
FROM ledger.invocation_events
WHERE invocation_id = $1 ORDER BY sequence ASC`, invocationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]contracts.InvocationEventV03, 0)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

type scanner interface{ Scan(...any) error }

func scanProjection(row scanner) (contracts.InvocationRecordV4, error) {
	var value contracts.InvocationRecordV4
	var parent, errorCode sql.NullString
	var latency sql.NullInt64
	err := row.Scan(
		&value.InvocationID, &value.RootTaskID, &parent, &value.TraceID,
		&value.Caller.Type, &value.Caller.ID, &value.WorkspaceID, &value.TargetAgentID,
		&value.AgentCardVersion, &value.Capability, &value.Status, &latency, &errorCode,
		&value.CreatedAt, &value.UpdatedAt,
	)
	if err != nil {
		return value, err
	}
	if parent.Valid {
		value.ParentInvocationID = parent.String
	}
	if latency.Valid {
		value.LatencyMS = &latency.Int64
	}
	if errorCode.Valid {
		value.ErrorCode = contracts.PlatformErrorCode(errorCode.String)
	}
	return value, nil
}

func scanEvent(row scanner) (contracts.InvocationEventV03, error) {
	var event contracts.InvocationEventV03
	var occurredAt time.Time
	var parent, errorCode sql.NullString
	var chunkIndex, chunkBytes, latency sql.NullInt64
	err := row.Scan(
		&event.EventID, &event.InvocationID, &event.Sequence, &occurredAt, &event.Type, &event.Status,
		&event.RootTaskID, &parent, &event.TraceID, &event.Caller.Type, &event.Caller.ID,
		&event.WorkspaceID, &event.TargetAgentID, &event.AgentCardVersion, &event.Capability,
		&chunkIndex, &chunkBytes, &latency, &errorCode,
	)
	if err != nil {
		return event, err
	}
	event.SchemaVersion = contracts.RuntimeInvocationEventSchemaVersion
	event.OccurredAt = occurredAt.UTC().Format(time.RFC3339Nano)
	if parent.Valid {
		event.ParentInvocationID = parent.String
	}
	if chunkIndex.Valid {
		event.ChunkIndex = &chunkIndex.Int64
	}
	if chunkBytes.Valid {
		event.ChunkBytes = &chunkBytes.Int64
	}
	if latency.Valid {
		event.LatencyMS = &latency.Int64
	}
	if errorCode.Valid {
		platformError, err := contracts.NewCorrelatedPlatformErrorV4(
			contracts.PlatformErrorCode(errorCode.String), event.TraceID, event.InvocationID, event.RootTaskID,
		)
		if err != nil {
			return event, err
		}
		event.Error = &platformError
	}
	return event, nil
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func sameOptionalInt64(left, right *int64) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func eventErrorCode(event contracts.InvocationEventV03) contracts.PlatformErrorCode {
	if event.Error == nil {
		return ""
	}
	return event.Error.Code
}

func classifyWriteError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23503", "23505":
			return ErrConflict
		case "23514":
			return ErrValidation
		}
	}
	return dependencyError(operation, err)
}

type dependencyFailure struct {
	operation string
	cause     error
}

func (failure *dependencyFailure) Error() string {
	return failure.operation + ": " + ErrDependency.Error()
}
func (failure *dependencyFailure) Unwrap() error { return ErrDependency }

func dependencyError(operation string, cause error) error {
	return &dependencyFailure{operation: operation, cause: cause}
}

func rollback(ctx context.Context, tx pgx.Tx, returnErr *error, operation string) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		*returnErr = errors.Join(*returnErr, dependencyError("rollback "+operation, err))
	}
}
