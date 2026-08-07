package registry

import (
	"context"
	"sync"
)

// InstanceWatchPublisher is the provider-side half of a locally created
// InstanceWatch. Consumers receive only InstanceWatch and can only pull.
type InstanceWatchPublisher interface {
	Publish(InstanceChange) error
	Terminate(error)
}

// NewInstanceWatch creates a bounded pull watch and its provider-side
// publisher. queueCapacity must be explicit and positive; no queue default is
// inferred. A full queue latches watch_interrupted(delivery_overflow).
func NewInstanceWatch(queueCapacity int) (InstanceWatch, InstanceWatchPublisher, error) {
	if queueCapacity <= 0 {
		return nil, nil, newInvalidError("queue_capacity")
	}
	watch := &localInstanceWatch{
		capacity: queueCapacity,
		signal:   make(chan struct{}),
	}
	return watch, watch, nil
}

// NewWatch is a short alias for NewInstanceWatch.
func NewWatch(queueCapacity int) (InstanceWatch, InstanceWatchPublisher, error) {
	return NewInstanceWatch(queueCapacity)
}

type localInstanceWatch struct {
	mu sync.Mutex

	capacity int
	queue    []InstanceChange

	terminal             error
	targetDeletedPending bool
	nextActive           bool
	signal               chan struct{}
}

func (w *localInstanceWatch) Next(ctx context.Context) (InstanceChange, error) {
	if ctx == nil {
		return InstanceChange{}, newInvalidError("next_context")
	}

	w.mu.Lock()
	if w.nextActive {
		w.mu.Unlock()
		return InstanceChange{}, newInvalidError("concurrent_next")
	}
	w.nextActive = true
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.nextActive = false
		w.mu.Unlock()
	}()

	for {
		w.mu.Lock()
		if err := ctx.Err(); err != nil {
			w.mu.Unlock()
			return InstanceChange{}, newCanceledError(err)
		}
		if len(w.queue) > 0 {
			change := cloneChange(w.queue[0])
			w.queue[0] = InstanceChange{}
			w.queue = w.queue[1:]
			if change.kind == InstanceChangeTargetDeleted && w.targetDeletedPending && len(w.queue) == 0 {
				w.targetDeletedPending = false
				w.terminal = ErrClosed
				w.notifyLocked()
			}
			w.mu.Unlock()
			return change, nil
		}
		if w.terminal != nil {
			err := w.terminal
			w.mu.Unlock()
			return InstanceChange{}, err
		}
		signal := w.signal
		w.mu.Unlock()

		select {
		case <-ctx.Done():
			return InstanceChange{}, newCanceledError(ctx.Err())
		case <-signal:
		}
	}
}

func (w *localInstanceWatch) Close() error {
	w.terminate(ErrClosed)
	return nil
}

func (w *localInstanceWatch) Publish(change InstanceChange) error {
	if err := change.Validate(); err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.targetDeletedPending {
		return ErrClosed
	}
	if w.terminal != nil {
		return w.terminal
	}
	if len(w.queue) >= w.capacity {
		w.terminal = NewOutcomeError(OutcomeWatchInterrupted, CauseDeliveryOverflow)
		w.queue = nil
		w.notifyLocked()
		return w.terminal
	}
	w.queue = append(w.queue, cloneChange(change))
	if change.kind == InstanceChangeTargetDeleted {
		w.targetDeletedPending = true
	}
	w.notifyLocked()
	return nil
}

func (w *localInstanceWatch) Terminate(err error) {
	w.terminate(typedTerminal(err))
}

func (w *localInstanceWatch) terminate(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.terminal != nil || w.targetDeletedPending {
		return
	}
	w.terminal = err
	w.queue = nil
	w.notifyLocked()
}

func (w *localInstanceWatch) notifyLocked() {
	close(w.signal)
	w.signal = make(chan struct{})
}
