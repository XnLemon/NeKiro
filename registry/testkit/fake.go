// Package testkit provides deterministic backend-neutral Instance Registry
// fixtures. It is intended for tests of directories and their consumers.
package testkit

import (
	"context"
	"sync"

	"github.com/NeKiro-project/NeKiro/registry"
)

// FakeConfig configures a deterministic FakeDirectory. QueueCapacity and the
// snapshot/observe capabilities are deliberately explicit.
type FakeConfig struct {
	Capabilities  registry.Capabilities
	QueueCapacity int
}

// FakeDirectory is a deterministic in-memory InstanceDirectory for tests. It
// has no selection, persistence, retries, relists, or fallback behavior.
type FakeDirectory struct {
	mu sync.Mutex

	capabilities  registry.Capabilities
	queueCapacity int
	closed        bool

	bindings map[registry.ReleaseTarget]registry.InstanceSnapshot
	watches  map[registry.ReleaseTarget]map[*fakeWatch]struct{}
}

// NewFakeDirectory constructs a fake with explicit supported capabilities and
// an explicit positive delivery queue bound.
func NewFakeDirectory(config FakeConfig) (*FakeDirectory, error) {
	if config.QueueCapacity <= 0 {
		return nil, registry.NewOutcomeError(registry.OutcomeInvalid, registry.CauseInvalidInput)
	}
	if !config.Capabilities.Supports(registry.CapabilitySnapshot) || !config.Capabilities.Supports(registry.CapabilityObserve) {
		return nil, registry.NewOutcomeError(registry.OutcomeInvalid, registry.CauseInvalidInput)
	}
	return &FakeDirectory{
		capabilities:  cloneCapabilities(config.Capabilities),
		queueCapacity: config.QueueCapacity,
		bindings:      make(map[registry.ReleaseTarget]registry.InstanceSnapshot),
		watches:       make(map[registry.ReleaseTarget]map[*fakeWatch]struct{}),
	}, nil
}

// NewFake is a short alias for NewFakeDirectory.
func NewFake(config FakeConfig) (*FakeDirectory, error) {
	return NewFakeDirectory(config)
}

// Bind configures one exact target and its current immutable snapshot. Initial
// snapshots must use local order zero. Rebinding while observations are active
// is invalid because it would bypass an explicit change event.
func (d *FakeDirectory) Bind(target registry.ReleaseTarget, snapshot registry.InstanceSnapshot) error {
	if err := target.Validate(); err != nil {
		return err
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if !snapshot.Target().Equal(target) {
		return registry.NewOutcomeError(registry.OutcomeInvalid, registry.CauseInvalidInput)
	}
	if snapshot.Revision().LocalOrder() != 0 {
		return registry.NewOutcomeError(registry.OutcomeInvalid, registry.CauseInvalidInput)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return registry.ErrClosed
	}
	if len(d.watches[target]) != 0 {
		return registry.NewOutcomeError(registry.OutcomeInvalid, registry.CauseInvalidInput)
	}
	d.bindings[target] = snapshot
	return nil
}

// SetSnapshot is an alias for Bind used by fixtures that describe their setup
// in terms of a current snapshot.
func (d *FakeDirectory) SetSnapshot(target registry.ReleaseTarget, snapshot registry.InstanceSnapshot) error {
	return d.Bind(target, snapshot)
}

func (d *FakeDirectory) Snapshot(ctx context.Context, target registry.ReleaseTarget) (registry.InstanceSnapshot, error) {
	if err := validateContext(ctx); err != nil {
		return registry.InstanceSnapshot{}, err
	}
	if err := target.Validate(); err != nil {
		return registry.InstanceSnapshot{}, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return registry.InstanceSnapshot{}, registry.ErrClosed
	}
	snapshot, found := d.bindings[target]
	if !found {
		return registry.InstanceSnapshot{}, registry.ErrMissing
	}
	return snapshot, nil
}

func (d *FakeDirectory) Observe(ctx context.Context, target registry.ReleaseTarget) (registry.InstanceObservation, error) {
	if err := validateContext(ctx); err != nil {
		return registry.InstanceObservation{}, err
	}
	if err := target.Validate(); err != nil {
		return registry.InstanceObservation{}, err
	}

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return registry.InstanceObservation{}, registry.ErrClosed
	}
	snapshot, found := d.bindings[target]
	if !found {
		d.mu.Unlock()
		return registry.InstanceObservation{}, registry.ErrMissing
	}
	watch, publisher, err := registry.NewInstanceWatch(d.queueCapacity)
	if err != nil {
		d.mu.Unlock()
		return registry.InstanceObservation{}, err
	}
	activeWatch := &fakeWatch{
		directory: d,
		target:    target,
		watch:     watch,
		publisher: publisher,
	}
	if d.watches[target] == nil {
		d.watches[target] = make(map[*fakeWatch]struct{})
	}
	d.watches[target][activeWatch] = struct{}{}
	d.mu.Unlock()

	observation, err := registry.NewInstanceObservation(snapshot, activeWatch)
	if err != nil {
		_ = activeWatch.Close()
		return registry.InstanceObservation{}, err
	}
	return observation, nil
}

// Directory returns this fake through the provider-neutral interface and lets
// it satisfy DirectoryConformanceDriver directly.
func (d *FakeDirectory) Directory() registry.InstanceDirectory { return d }

// Capabilities returns an immutable copy of the fake's advertised capabilities.
func (d *FakeDirectory) Capabilities() registry.Capabilities {
	d.mu.Lock()
	defer d.mu.Unlock()
	return cloneCapabilities(d.capabilities)
}

// Emit delivers one ordered change to every current observation for target and
// advances the configured current snapshot. It never coalesces changes.
func (d *FakeDirectory) Emit(target registry.ReleaseTarget, change registry.InstanceChange) error {
	if err := target.Validate(); err != nil {
		return err
	}
	if err := change.Validate(); err != nil {
		return err
	}
	if !change.Snapshot().Target().Equal(target) {
		return registry.NewOutcomeError(registry.OutcomeInvalid, registry.CauseInvalidInput)
	}

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return registry.ErrClosed
	}
	current, found := d.bindings[target]
	if !found {
		d.mu.Unlock()
		return registry.ErrMissing
	}
	if change.Revision().LocalOrder() != current.Revision().LocalOrder()+1 {
		d.mu.Unlock()
		return registry.NewOutcomeError(registry.OutcomeInvalid, registry.CauseInvalidInput)
	}
	d.bindings[target] = change.Snapshot()
	watches := make([]*fakeWatch, 0, len(d.watches[target]))
	for watch := range d.watches[target] {
		watches = append(watches, watch)
	}
	d.mu.Unlock()

	var firstErr error
	for _, watch := range watches {
		if err := watch.publisher.Publish(change); err != nil && firstErr == nil {
			firstErr = err
			watch.detach()
		}
	}
	return firstErr
}

// Terminate latches a typed terminal outcome for every current observation of
// target. It does not modify the configured binding snapshot.
func (d *FakeDirectory) Terminate(target registry.ReleaseTarget, terminal error) error {
	if err := target.Validate(); err != nil {
		return err
	}
	if _, ok := registry.OutcomeOf(terminal); !ok {
		return registry.NewOutcomeError(registry.OutcomeInvalid, registry.CauseTerminalOutcomeRequired)
	}

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return registry.ErrClosed
	}
	if _, found := d.bindings[target]; !found {
		d.mu.Unlock()
		return registry.ErrMissing
	}
	watches := make([]*fakeWatch, 0, len(d.watches[target]))
	for watch := range d.watches[target] {
		watches = append(watches, watch)
	}
	d.mu.Unlock()

	for _, watch := range watches {
		watch.publisher.Terminate(terminal)
		watch.detach()
	}
	return nil
}

// Close is idempotent and closes every accepted observation.
func (d *FakeDirectory) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	watches := make([]*fakeWatch, 0)
	for _, watchesForTarget := range d.watches {
		for watch := range watchesForTarget {
			watches = append(watches, watch)
		}
	}
	d.watches = make(map[registry.ReleaseTarget]map[*fakeWatch]struct{})
	d.mu.Unlock()

	for _, watch := range watches {
		_ = watch.watch.Close()
	}
	return nil
}

func (d *FakeDirectory) detach(watch *fakeWatch) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if watchesForTarget := d.watches[watch.target]; watchesForTarget != nil {
		delete(watchesForTarget, watch)
		if len(watchesForTarget) == 0 {
			delete(d.watches, watch.target)
		}
	}
}

type fakeWatch struct {
	directory *FakeDirectory
	target    registry.ReleaseTarget
	watch     registry.InstanceWatch
	publisher registry.InstanceWatchPublisher

	detachOnce sync.Once
}

func (w *fakeWatch) Next(ctx context.Context) (registry.InstanceChange, error) {
	change, err := w.watch.Next(ctx)
	if err != nil {
		if outcome, ok := registry.OutcomeOf(err); ok && outcome != registry.OutcomeCanceled && outcome != registry.OutcomeInvalid {
			w.detach()
		}
	}
	return change, err
}

func (w *fakeWatch) Close() error {
	err := w.watch.Close()
	w.detach()
	return err
}

func (w *fakeWatch) detach() {
	w.detachOnce.Do(func() { w.directory.detach(w) })
}

func cloneCapabilities(capabilities registry.Capabilities) registry.Capabilities {
	copyCapabilities, err := registry.NewCapabilities(capabilities.Values()...)
	if err != nil {
		panic("registry testkit received invalid capabilities")
	}
	return copyCapabilities
}

func validateContext(ctx context.Context) error {
	if ctx == nil {
		return registry.NewOutcomeError(registry.OutcomeInvalid, registry.CauseInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return registry.NewOutcomeError(registry.OutcomeCanceled, registry.CauseNone)
	}
	return nil
}
