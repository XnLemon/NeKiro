package testkit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/NeKiro-project/NeKiro/registry"
)

// DirectoryConformanceDriver supplies backend-specific control actions to the
// reusable provider-neutral directory conformance checks.
type DirectoryConformanceDriver interface {
	Directory() registry.InstanceDirectory
	Emit(registry.ReleaseTarget, registry.InstanceChange) error
	Terminate(registry.ReleaseTarget, error) error
}

// DirectoryConformanceFixture is the exact scenario data used by
// RunDirectoryConformance. Initial must have local order zero; Change must be
// its next revision.
type DirectoryConformanceFixture struct {
	Target        registry.ReleaseTarget
	UnboundTarget registry.ReleaseTarget
	Initial       registry.InstanceSnapshot
	Change        registry.InstanceChange
	Terminal      error
}

// RunDirectoryConformance verifies the root v1 Snapshot/Observe contract
// against a backend-neutral driver. It intentionally has no provider retry or
// recovery assumptions.
func RunDirectoryConformance(t testing.TB, driver DirectoryConformanceDriver, fixture DirectoryConformanceFixture) {
	t.Helper()
	if driver == nil {
		t.Fatal("directory conformance driver is nil")
	}
	if err := fixture.Target.Validate(); err != nil {
		t.Fatalf("fixture target: %v", err)
	}
	if err := fixture.UnboundTarget.Validate(); err != nil {
		t.Fatalf("fixture unbound target: %v", err)
	}
	if fixture.Target.Equal(fixture.UnboundTarget) {
		t.Fatal("fixture unbound target equals bound target")
	}
	if err := fixture.Initial.Validate(); err != nil {
		t.Fatalf("fixture initial snapshot: %v", err)
	}
	if !fixture.Initial.Target().Equal(fixture.Target) || fixture.Initial.Revision().LocalOrder() != 0 {
		t.Fatal("fixture initial snapshot must target the bound target at local order zero")
	}
	if err := fixture.Change.Validate(); err != nil {
		t.Fatalf("fixture change: %v", err)
	}
	if !fixture.Change.Snapshot().Target().Equal(fixture.Target) || fixture.Change.Revision().LocalOrder() != 1 {
		t.Fatal("fixture change must target the bound target at local order one")
	}
	if _, ok := registry.OutcomeOf(fixture.Terminal); !ok {
		t.Fatal("fixture terminal must be a typed registry outcome")
	}

	directory := driver.Directory()
	if directory == nil {
		t.Fatal("driver returned nil directory")
	}
	capabilities := directory.Capabilities()
	if !capabilities.Supports(registry.CapabilitySnapshot) || !capabilities.Supports(registry.CapabilityObserve) {
		t.Fatal("directory does not advertise snapshot and observe")
	}

	snapshot, err := directory.Snapshot(context.Background(), fixture.Target)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !snapshot.Equal(fixture.Initial) {
		t.Fatal("snapshot differs from initial fixture")
	}
	assertSnapshotImmutable(t, directory, fixture.Target, snapshot)

	if _, err := directory.Snapshot(context.Background(), fixture.UnboundTarget); !errors.Is(err, registry.ErrMissing) {
		t.Fatalf("unbound snapshot error = %v, want missing", err)
	}

	observation, err := directory.Observe(context.Background(), fixture.Target)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if !observation.Initial().Equal(fixture.Initial) {
		t.Fatal("observation initial differs from fixture")
	}
	watch := observation.Watch()

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := watch.Next(canceledContext); !errors.Is(err, registry.ErrCanceled) {
		t.Fatalf("canceled next error = %v, want canceled", err)
	}

	firstResult := make(chan error, 1)
	firstContext := newEnteredContext(context.Background())
	go func() {
		change, err := watch.Next(firstContext)
		if err == nil && !change.Equal(fixture.Change) {
			err = errors.New("first concurrent next returned the wrong change")
		}
		firstResult <- err
	}()
	firstContext.awaitEntered(t)
	if _, err := watch.Next(context.Background()); !errors.Is(err, registry.ErrInvalid) {
		t.Fatalf("concurrent next error = %v, want invalid", err)
	}
	if err := driver.Emit(fixture.Target, fixture.Change); err != nil {
		t.Fatalf("emit change: %v", err)
	}
	if err := awaitResult(firstResult); err != nil {
		t.Fatal(err)
	}

	if err := driver.Terminate(fixture.Target, fixture.Terminal); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	if _, err := watch.Next(context.Background()); !errors.Is(err, fixture.Terminal) {
		t.Fatalf("terminal next error = %v, want %v", err, fixture.Terminal)
	}

	secondObservation, err := directory.Observe(context.Background(), fixture.Target)
	if err != nil {
		t.Fatalf("observe after terminal: %v", err)
	}
	blockedResult := make(chan error, 1)
	blockedContext := newEnteredContext(context.Background())
	go func() {
		_, err := secondObservation.Watch().Next(blockedContext)
		blockedResult <- err
	}()
	blockedContext.awaitEntered(t)
	if err := directory.Close(); err != nil {
		t.Fatalf("directory close: %v", err)
	}
	if err := awaitResult(blockedResult); !errors.Is(err, registry.ErrClosed) {
		t.Fatalf("close next error = %v, want closed", err)
	}
	if err := directory.Close(); err != nil {
		t.Fatalf("second directory close: %v", err)
	}
}

func assertSnapshotImmutable(t testing.TB, directory registry.InstanceDirectory, target registry.ReleaseTarget, snapshot registry.InstanceSnapshot) {
	t.Helper()
	instances := snapshot.Instances()
	if len(instances) == 0 {
		return
	}
	metadata := instances[0].Metadata()
	for key := range metadata {
		metadata[key] = "changed"
		break
	}
	instances[0] = registry.Instance{}
	fresh, err := directory.Snapshot(context.Background(), target)
	if err != nil {
		t.Fatalf("fresh snapshot after mutation attempt: %v", err)
	}
	if !fresh.Equal(snapshot) {
		t.Fatal("returned snapshot mutation changed retained topology")
	}
}

func awaitResult(result <-chan error) error {
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		return errors.New("timed out waiting for next")
	}
}

type enteredContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func newEnteredContext(ctx context.Context) *enteredContext {
	return &enteredContext{Context: ctx, entered: make(chan struct{})}
}

func (c *enteredContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

func (c *enteredContext) awaitEntered(t testing.TB) {
	t.Helper()
	select {
	case <-c.entered:
	case <-time.After(time.Second):
		t.Fatal("next did not reach its blocking point")
	}
}
