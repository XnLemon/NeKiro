package testkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/NeKiro-project/NeKiro/registry"
)

func TestFakeDirectoryConformance(t *testing.T) {
	fixture := fakeFixture(t)
	directory := newFakeDirectory(t)
	if err := directory.Bind(fixture.Target, fixture.Initial); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	RunDirectoryConformance(t, directory, fixture)
}

func TestFakeDirectoryRejectsImpossibleTransitionDelta(t *testing.T) {
	fixture := fakeFixture(t)
	directory := newFakeDirectory(t)
	if err := directory.Bind(fixture.Target, fixture.Initial); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	empty := fakeSnapshot(t, fixture.Target, 1, registry.SnapshotStateEmpty, nil)
	badDeletion, err := registry.NewInstanceChange(registry.InstanceChangeInput{
		Kind:               registry.InstanceChangeInstancesChanged,
		Revision:           empty.Revision(),
		DeletedInstanceIDs: []string{"uid-not-in-previous-snapshot"},
		Snapshot:           empty,
	})
	if err != nil {
		t.Fatalf("NewInstanceChange bad deletion fixture: %v", err)
	}
	if err := directory.Emit(fixture.Target, badDeletion); !errors.Is(err, registry.ErrInvalid) {
		t.Fatalf("Emit impossible deletion error = %v, want invalid", err)
	}

	if snapshot, err := directory.Snapshot(context.Background(), fixture.Target); err != nil || !snapshot.Equal(fixture.Initial) {
		t.Fatalf("snapshot after rejected transition = %#v / %v, want initial unchanged", snapshot, err)
	}
}

func TestFakeDirectoryRequiresStateChangeToMatchCurrentSnapshot(t *testing.T) {
	directory := newFakeDirectory(t)
	target := fakeTarget(t, "agent-a", "release-a")
	current := fakeSnapshot(t, target, 0, registry.SnapshotStateEmpty, nil)
	if err := directory.Bind(target, current); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	empty := fakeSnapshot(t, target, 1, registry.SnapshotStateEmpty, nil)
	change, err := registry.NewInstanceChange(registry.InstanceChangeInput{
		Kind:          registry.InstanceChangeStateChanged,
		Revision:      empty.Revision(),
		PreviousState: registry.SnapshotStateMissing,
		Snapshot:      empty,
	})
	if err != nil {
		t.Fatalf("NewInstanceChange state fixture: %v", err)
	}
	if err := directory.Emit(target, change); !errors.Is(err, registry.ErrInvalid) {
		t.Fatalf("Emit mismatched previous state error = %v, want invalid", err)
	}
}

func TestFakeDirectoryRebasesRevisionPerObservation(t *testing.T) {
	fixture := fakeFixture(t)
	directory := newFakeDirectory(t)
	if err := directory.Bind(fixture.Target, fixture.Initial); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	first, err := directory.Observe(context.Background(), fixture.Target)
	if err != nil {
		t.Fatalf("first Observe: %v", err)
	}
	if err := directory.Emit(fixture.Target, fixture.Change); err != nil {
		t.Fatalf("first Emit: %v", err)
	}
	firstChange, err := first.Watch().Next(context.Background())
	if err != nil {
		t.Fatalf("first Next: %v", err)
	}
	if firstChange.Revision().LocalOrder() != 1 {
		t.Fatalf("first change local order = %d, want 1", firstChange.Revision().LocalOrder())
	}

	second, err := directory.Observe(context.Background(), fixture.Target)
	if err != nil {
		t.Fatalf("second Observe: %v", err)
	}
	if second.Initial().Revision().LocalOrder() != 0 {
		t.Fatalf("second initial local order = %d, want 0", second.Initial().Revision().LocalOrder())
	}
	if got, want := second.Initial().Instances(), fixture.Change.Snapshot().Instances(); !sameInstances(got, want) {
		t.Fatalf("second initial instances = %#v, want %#v", got, want)
	}

	nextInstance := fakeInstance(t, "uid-a", false, false, true)
	nextSnapshot := fakeSnapshot(t, fixture.Target, 1, registry.SnapshotStatePopulated, []registry.Instance{nextInstance})
	nextChange, err := registry.NewInstanceChange(registry.InstanceChangeInput{
		Kind:     registry.InstanceChangeInstancesChanged,
		Revision: nextSnapshot.Revision(),
		Upserts:  []registry.Instance{nextInstance},
		Snapshot: nextSnapshot,
	})
	if err != nil {
		t.Fatalf("NewInstanceChange second transition: %v", err)
	}
	if err := directory.Emit(fixture.Target, nextChange); err != nil {
		t.Fatalf("second Emit: %v", err)
	}
	firstNextChange, err := first.Watch().Next(context.Background())
	if err != nil {
		t.Fatalf("first second Next: %v", err)
	}
	if firstNextChange.Revision().LocalOrder() != 2 {
		t.Fatalf("first second change local order = %d, want 2", firstNextChange.Revision().LocalOrder())
	}
	if got, want := firstNextChange.Upserts(), []registry.Instance{nextInstance}; !sameInstances(got, want) {
		t.Fatalf("first second change upserts = %#v, want %#v", got, want)
	}
	if got, want := firstNextChange.Snapshot().Instances(), []registry.Instance{nextInstance}; !sameInstances(got, want) {
		t.Fatalf("first second change instances = %#v, want %#v", got, want)
	}

	secondChange, err := second.Watch().Next(context.Background())
	if err != nil {
		t.Fatalf("second Next: %v", err)
	}
	if secondChange.Revision().LocalOrder() != 1 {
		t.Fatalf("second change local order = %d, want 1", secondChange.Revision().LocalOrder())
	}
	if got, want := secondChange.Upserts(), []registry.Instance{nextInstance}; !sameInstances(got, want) {
		t.Fatalf("second change upserts = %#v, want %#v", got, want)
	}
	if got, want := secondChange.Snapshot().Instances(), []registry.Instance{nextInstance}; !sameInstances(got, want) {
		t.Fatalf("second change instances = %#v, want %#v", got, want)
	}
}

func TestFakeDistinguishesMissingBindingFromMissingSnapshot(t *testing.T) {
	directory := newFakeDirectory(t)
	target := fakeTarget(t, "agent-a", "release-a")
	unbound := fakeTarget(t, "agent-b", "release-b")
	missing := fakeSnapshot(t, target, 0, registry.SnapshotStateMissing, nil)
	if err := directory.Bind(target, missing); err != nil {
		t.Fatalf("Bind missing snapshot: %v", err)
	}

	snapshot, err := directory.Snapshot(context.Background(), target)
	if err != nil {
		t.Fatalf("Snapshot bound missing: %v", err)
	}
	if snapshot.State() != registry.SnapshotStateMissing {
		t.Fatalf("bound state = %q, want missing", snapshot.State())
	}
	observation, err := directory.Observe(context.Background(), target)
	if err != nil {
		t.Fatalf("Observe bound missing: %v", err)
	}
	if observation.Initial().State() != registry.SnapshotStateMissing {
		t.Fatalf("observation state = %q, want missing", observation.Initial().State())
	}
	if _, err := directory.Snapshot(context.Background(), unbound); !errors.Is(err, registry.ErrMissing) {
		t.Fatalf("unbound Snapshot error = %v, want missing outcome", err)
	}
	if _, err := directory.Observe(context.Background(), unbound); !errors.Is(err, registry.ErrMissing) {
		t.Fatalf("unbound Observe error = %v, want missing outcome", err)
	}
}

func TestFakeDirectoryCloseUnblocksEveryWatch(t *testing.T) {
	directory := newFakeDirectory(t)
	target := fakeTarget(t, "agent-a", "release-a")
	if err := directory.Bind(target, fakeSnapshot(t, target, 0, registry.SnapshotStateEmpty, nil)); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	first, err := directory.Observe(context.Background(), target)
	if err != nil {
		t.Fatalf("first Observe: %v", err)
	}
	second, err := directory.Observe(context.Background(), target)
	if err != nil {
		t.Fatalf("second Observe: %v", err)
	}
	results := make(chan error, 2)
	contexts := []*fakeEnteredContext{
		newFakeEnteredContext(context.Background()),
		newFakeEnteredContext(context.Background()),
	}
	for index, watch := range []registry.InstanceWatch{first.Watch(), second.Watch()} {
		go func(watch registry.InstanceWatch) {
			_, err := watch.Next(contexts[index])
			results <- err
		}(watch)
	}
	for _, ctx := range contexts {
		ctx.awaitEntered(t)
	}
	if err := directory.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for index := 0; index < 2; index++ {
		select {
		case err := <-results:
			if !errors.Is(err, registry.ErrClosed) {
				t.Fatalf("watch %d error = %v, want closed", index, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("watch %d did not unblock", index)
		}
	}
}

func newFakeDirectory(t testing.TB) *FakeDirectory {
	t.Helper()
	capabilities, err := registry.NewCapabilities(registry.CapabilitySnapshot, registry.CapabilityObserve)
	if err != nil {
		t.Fatalf("NewCapabilities: %v", err)
	}
	directory, err := NewFakeDirectory(FakeConfig{Capabilities: capabilities, QueueCapacity: 4})
	if err != nil {
		t.Fatalf("NewFakeDirectory: %v", err)
	}
	return directory
}

func fakeFixture(t testing.TB) DirectoryConformanceFixture {
	t.Helper()
	target := fakeTarget(t, "agent-a", "release-a")
	unbound := fakeTarget(t, "agent-b", "release-b")
	instance := fakeInstance(t, "uid-a", true, true, false)
	initial := fakeSnapshot(t, target, 0, registry.SnapshotStatePopulated, []registry.Instance{instance})
	changed := fakeInstance(t, "uid-a", false, true, true)
	changeSnapshot := fakeSnapshot(t, target, 1, registry.SnapshotStatePopulated, []registry.Instance{changed})
	change, err := registry.NewInstanceChange(registry.InstanceChangeInput{
		Kind:     registry.InstanceChangeInstancesChanged,
		Revision: changeSnapshot.Revision(),
		Upserts:  []registry.Instance{changed},
		Snapshot: changeSnapshot,
	})
	if err != nil {
		t.Fatalf("NewInstanceChange: %v", err)
	}
	return DirectoryConformanceFixture{
		Target:        target,
		UnboundTarget: unbound,
		Initial:       initial,
		Change:        change,
		Terminal:      registry.NewOutcomeError(registry.OutcomeStale, registry.CauseResourceVersionExpired),
	}
}

func fakeTarget(t testing.TB, agentID, releaseID string) registry.ReleaseTarget {
	t.Helper()
	target, err := registry.NewReleaseTarget(registry.ReleaseTargetInput{
		AgentID:           agentID,
		AgentCardVersion:  "1.0.0",
		ReleaseID:         releaseID,
		CardDigest:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CanonicalEndpoint: "https://agent.example/a2a",
		Audience:          "https://agent.example",
	})
	if err != nil {
		t.Fatalf("NewReleaseTarget: %v", err)
	}
	return target
}

func fakeInstance(t testing.TB, id string, ready, serving, terminating bool) registry.Instance {
	t.Helper()
	endpoint, err := registry.NewNetworkEndpoint(registry.NetworkEndpointInput{
		AddressType: registry.AddressTypeIPv4,
		Address:     "10.0.0.1",
		PortName:    "a2a",
		Port:        8080,
		Protocol:    registry.TransportProtocolTCP,
	})
	if err != nil {
		t.Fatalf("NewNetworkEndpoint: %v", err)
	}
	instance, err := registry.NewInstance(registry.InstanceInput{
		ID:          id,
		Endpoints:   []registry.NetworkEndpoint{endpoint},
		Ready:       ready,
		Serving:     serving,
		Terminating: terminating,
		Metadata:    map[string]string{"target_ref_uid": id},
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	return instance
}

func fakeSnapshot(t testing.TB, target registry.ReleaseTarget, order uint64, state registry.SnapshotState, instances []registry.Instance) registry.InstanceSnapshot {
	t.Helper()
	revision, err := registry.NewRevision(registry.RevisionInput{
		SourceTokens: []string{fmt.Sprintf("service-%d", order), fmt.Sprintf("slice-%d", order)},
		LocalOrder:   order,
	})
	if err != nil {
		t.Fatalf("NewRevision: %v", err)
	}
	snapshot, err := registry.NewInstanceSnapshot(registry.InstanceSnapshotInput{
		Target:    target,
		Revision:  revision,
		State:     state,
		Instances: instances,
	})
	if err != nil {
		t.Fatalf("NewInstanceSnapshot: %v", err)
	}
	return snapshot
}

type fakeEnteredContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func newFakeEnteredContext(ctx context.Context) *fakeEnteredContext {
	return &fakeEnteredContext{Context: ctx, entered: make(chan struct{})}
}

func (c *fakeEnteredContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

func (c *fakeEnteredContext) awaitEntered(t testing.TB) {
	t.Helper()
	select {
	case <-c.entered:
	case <-time.After(time.Second):
		t.Fatal("watch did not reach its blocking point")
	}
}
