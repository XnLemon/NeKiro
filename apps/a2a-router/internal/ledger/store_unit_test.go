package ledger

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestNewStoreRequiresPool(t *testing.T) {
	if _, err := NewStore(nil); err == nil {
		t.Fatal("nil Ledger pool accepted")
	}
}

func TestOrderTraceProjectionsBuildsParentBeforeChild(t *testing.T) {
	values := []contracts.InvocationRecordV4{
		{InvocationID: "child", ParentInvocationID: "parent"},
		{InvocationID: "parent"},
		{InvocationID: "root"},
	}
	ordered, err := orderTraceProjections(values)
	if err != nil {
		t.Fatalf("order trace: %v", err)
	}
	if got := []string{ordered[0].InvocationID, ordered[1].InvocationID, ordered[2].InvocationID}; !reflect.DeepEqual(got, []string{"parent", "root", "child"}) {
		t.Fatalf("ordered trace = %v", got)
	}
}

func TestOrderTraceProjectionsRejectsMissingOrCyclicLineage(t *testing.T) {
	for _, values := range [][]contracts.InvocationRecordV4{
		{{InvocationID: "child", ParentInvocationID: "missing"}},
		{{InvocationID: "a", ParentInvocationID: "b"}, {InvocationID: "b", ParentInvocationID: "a"}},
	} {
		if _, err := orderTraceProjections(values); err == nil {
			t.Fatalf("invalid lineage accepted: %#v", values)
		}
	}
}

func TestStoreValidateCandidateChecksEventAndTimestampPolicy(t *testing.T) {
	validator, err := contracts.NewRuntimeContractValidator()
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{validator: validator}
	valid := ledgerUnitEvent()
	if occurredAt, err := store.validateCandidate(valid); err != nil || !occurredAt.Equal(time.Date(2026, 7, 19, 12, 0, 0, 123000, time.UTC)) {
		t.Fatalf("valid candidate = %s, %v", occurredAt, err)
	}

	invalid := valid
	invalid.EventID = ""
	if _, err := store.validateCandidate(invalid); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid event error = %v", err)
	}
	tooPrecise := valid
	tooPrecise.OccurredAt = time.Date(2026, 7, 19, 12, 0, 0, 123456789, time.UTC).Format(time.RFC3339Nano)
	if _, err := store.validateCandidate(tooPrecise); !errors.Is(err, ErrValidation) {
		t.Fatalf("over-precise timestamp error = %v", err)
	}
	nonInitial := valid
	nonInitial.Sequence = 1
	nonInitial.Type = "routing"
	nonInitial.Status = "routing"
	if _, err := store.validateCandidate(nonInitial); err != nil {
		t.Fatalf("non-initial candidate rejected before persistence: %v", err)
	}
}

func TestScanProjectionAndEventDecodeNullableFields(t *testing.T) {
	projection, err := scanProjection(valueScanner{values: []any{
		"inv-a", "task-a", sql.NullString{String: "parent-a", Valid: true}, contracts.TraceID("trace-a"),
		"user", "caller-a", "workspace-a", "agent-a", "1.0.0", sql.NullString{}, []byte(nil), "capability-a", "succeeded",
		sql.NullInt64{Int64: 7, Valid: true}, sql.NullString{String: string(contracts.ErrorCodeAgentExecutionFailed), Valid: true},
		time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC), time.Date(2026, 7, 19, 12, 0, 1, 0, time.UTC),
	}})
	if err != nil || projection.ParentInvocationID != "parent-a" || projection.LatencyMS == nil || *projection.LatencyMS != 7 || projection.ErrorCode != contracts.ErrorCodeAgentExecutionFailed {
		t.Fatalf("projection = %#v, %v", projection, err)
	}

	event, err := scanEvent(valueScanner{values: []any{
		"event-a", "inv-a", int64(3), time.Date(2026, 7, 19, 12, 0, 1, 0, time.UTC), "failed", "failed",
		"task-a", sql.NullString{}, contracts.TraceID("trace-a"), "user", "caller-a", "workspace-a", "agent-a", "1.0.0", sql.NullString{}, []byte(nil), "capability-a",
		sql.NullInt64{Int64: 1, Valid: true}, sql.NullInt64{Int64: 10, Valid: true}, sql.NullInt64{Int64: 7, Valid: true}, sql.NullString{String: string(contracts.ErrorCodeAgentExecutionFailed), Valid: true},
	}})
	if err != nil || event.SchemaVersion != contracts.RuntimeInvocationEventSchemaVersion || event.Error == nil || event.Error.Code != contracts.ErrorCodeAgentExecutionFailed || event.ChunkIndex == nil || *event.ChunkIndex != 1 {
		t.Fatalf("event = %#v, %v", event, err)
	}
}

func TestLedgerHelpersClassifyWriteFailuresAndOptionalValues(t *testing.T) {
	if nullableText("") != nil || nullableText("value") != "value" {
		t.Fatal("nullable text mapping is incorrect")
	}
	left, right := int64(7), int64(7)
	if !sameOptionalInt64(&left, &right) || sameOptionalInt64(&left, nil) || sameOptionalInt64(nil, &right) || !sameOptionalInt64(nil, nil) {
		t.Fatal("optional integer comparison is incorrect")
	}
	event := ledgerUnitEvent()
	if eventErrorCode(event) != "" {
		t.Fatal("non-terminal event unexpectedly has an error code")
	}
	terminalError := contracts.ErrorCodeAgentExecutionFailed
	event.Error = &contracts.PlatformErrorV4{Code: terminalError}
	if eventErrorCode(event) != terminalError {
		t.Fatal("terminal error code was not extracted")
	}
	for _, test := range []struct {
		code string
		want error
	}{
		{code: "23503", want: ErrConflict},
		{code: "23505", want: ErrConflict},
		{code: "23514", want: ErrValidation},
		{code: "XX000", want: ErrDependency},
	} {
		err := classifyWriteError("write", &pgconn.PgError{Code: test.code})
		if !errors.Is(err, test.want) {
			t.Fatalf("code %s error=%v want=%v", test.code, err, test.want)
		}
	}
	dependency := dependencyError("read", errors.New("offline"))
	if dependency.Error() != "read: "+ErrDependency.Error() || !errors.Is(dependency, ErrDependency) {
		t.Fatalf("dependency error = %v", dependency)
	}
}

type valueScanner struct{ values []any }

func (scanner valueScanner) Scan(dest ...any) error {
	if len(dest) != len(scanner.values) {
		return errors.New("scanner destination count mismatch")
	}
	for index, value := range scanner.values {
		target := reflect.ValueOf(dest[index])
		if target.Kind() != reflect.Pointer || target.IsNil() {
			return errors.New("scanner destination is not a pointer")
		}
		valueOf := reflect.ValueOf(value)
		if !valueOf.IsValid() || !valueOf.Type().AssignableTo(target.Elem().Type()) {
			return errors.New("scanner value type mismatch")
		}
		target.Elem().Set(valueOf)
	}
	return nil
}

func ledgerUnitEvent() contracts.InvocationEventV03 {
	return contracts.InvocationEventV03{
		SchemaVersion: contracts.RuntimeInvocationEventSchemaVersion,
		EventID:       "event-a", Sequence: 0, OccurredAt: time.Date(2026, 7, 19, 12, 0, 0, 123000, time.UTC).Format(time.RFC3339Nano),
		Type: "created", Status: "pending", InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
		Caller: contracts.Caller{Type: "user", ID: "caller-a"}, WorkspaceID: "workspace-a", TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", Capability: "capability-a",
	}
}
