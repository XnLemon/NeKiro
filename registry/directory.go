package registry

import "context"

// InstanceDirectory reads and observes ephemeral topology for an exact,
// already-authorized ReleaseTarget. It does not resolve or authorize targets.
type InstanceDirectory interface {
	Snapshot(context.Context, ReleaseTarget) (InstanceSnapshot, error)
	Observe(context.Context, ReleaseTarget) (InstanceObservation, error)
	Capabilities() Capabilities
	Close() error
}

// InstanceWatch is a pull-only, single-consumer topology change stream.
type InstanceWatch interface {
	Next(context.Context) (InstanceChange, error)
	Close() error
}

// InstanceObservation atomically joins an immutable initial snapshot with an
// already-established pull watch.
type InstanceObservation struct {
	initial InstanceSnapshot
	watch   InstanceWatch
}

// NewInstanceObservation constructs an observation from a valid initial
// snapshot and a non-nil established watch.
func NewInstanceObservation(initial InstanceSnapshot, watch InstanceWatch) (InstanceObservation, error) {
	if err := initial.Validate(); err != nil {
		return InstanceObservation{}, err
	}
	if watch == nil {
		return InstanceObservation{}, newInvalidError("watch")
	}
	return InstanceObservation{initial: initial, watch: watch}, nil
}

// Initial returns the observation's immutable initial snapshot.
func (o InstanceObservation) Initial() InstanceSnapshot { return o.initial }

// Watch returns the observation's pull watch.
func (o InstanceObservation) Watch() InstanceWatch { return o.watch }
