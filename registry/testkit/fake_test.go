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
