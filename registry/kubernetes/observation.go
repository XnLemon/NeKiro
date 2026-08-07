package kubernetes

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sync"

	"github.com/NeKiro-project/NeKiro/registry"
)

type observationSession struct {
	directory *Directory
	binding   Binding
	target    registry.ReleaseTarget

	inner     registry.InstanceWatch
	publisher registry.InstanceWatchPublisher
	watch     *kubernetesWatch

	mu            sync.Mutex
	state         topologyState
	current       registry.InstanceSnapshot
	terminal      error
	targetDeleted bool
	opened        map[watchSource]bool
	barrier       chan struct{}
	barrierClosed bool
	bodies        map[watchSource]io.ReadCloser

	stopOnce sync.Once
}

func newObservationSession(
	directory *Directory,
	binding Binding,
	target registry.ReleaseTarget,
	state topologyState,
	initial registry.InstanceSnapshot,
	inner registry.InstanceWatch,
	publisher registry.InstanceWatchPublisher,
) *observationSession {
	session := &observationSession{
		directory: directory,
		binding:   binding,
		target:    target,
		inner:     inner,
		publisher: publisher,
		state:     state,
		current:   initial,
		opened:    make(map[watchSource]bool, 2),
		barrier:   make(chan struct{}),
		bodies:    make(map[watchSource]io.ReadCloser, 2),
	}
	session.watch = &kubernetesWatch{session: session, inner: inner}
	return session
}

func (s *observationSession) attachBody(source watchSource, body io.ReadCloser) bool {
	s.mu.Lock()
	if s.terminal != nil {
		s.mu.Unlock()
		_ = closeBody(body)
		return false
	}
	if s.targetDeleted {
		s.mu.Unlock()
		_ = closeBody(body)
		return true
	}
	s.bodies[source] = body
	s.mu.Unlock()
	return true
}

func (s *observationSession) markWatchOpen(source watchSource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminal != nil {
		return s.terminal
	}
	if s.opened[source] {
		return invalidInput()
	}
	s.opened[source] = true
	if len(s.opened) == 2 && !s.barrierClosed {
		s.barrierClosed = true
		close(s.barrier)
	}
	return nil
}

func (s *observationSession) awaitBarrier(ctx context.Context) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return canceledError(ctx.Err())
	case <-s.barrier:
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.terminal
	}
}

func (s *observationSession) terminalError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminal != nil {
		return s.terminal
	}
	if s.targetDeleted {
		return registry.ErrClosed
	}
	return registry.ErrClosed
}

func (s *observationSession) readSource(source watchSource, body io.ReadCloser) {
	reader := &bufioReader{reader: bufio.NewReader(body)}
	for {
		if s.finished() {
			return
		}
		payload, err := readWatchEnvelope(reader, s.binding.bounds.WatchEnvelopeBytes)
		if err != nil {
			switch {
			case errors.Is(err, errWatchEnvelopeTooLarge):
				s.fail(registry.NewOutcomeError(registry.OutcomeWatchInterrupted, registry.CauseWatchEventTooLarge))
			case errors.Is(err, errWatchEnvelopeInvalid):
				s.fail(registry.NewOutcomeError(registry.OutcomeWatchInterrupted, registry.CauseWatchEventInvalid))
			default:
				s.fail(registry.NewOutcomeError(registry.OutcomeWatchInterrupted, registry.CauseStreamEOF))
			}
			return
		}
		event, err := decodeWatchEvent(payload)
		if err != nil {
			s.fail(registry.NewOutcomeError(registry.OutcomeWatchInterrupted, registry.CauseWatchEventInvalid))
			return
		}
		if event.Type == "ERROR" {
			status, err := decodeWatchStatus(event.Object)
			if err != nil {
				s.fail(registry.NewOutcomeError(registry.OutcomeWatchInterrupted, registry.CauseWatchEventInvalid))
				return
			}
			s.fail(outcomeFromWatchStatus(status))
			return
		}
		if err := s.acceptEvent(source, event); err != nil {
			s.fail(err)
			return
		}
	}
}

func (s *observationSession) acceptEvent(source watchSource, event wireWatchEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminal != nil || s.targetDeleted {
		return nil
	}
	switch source {
	case watchService:
		switch event.Type {
		case "ADDED", "MODIFIED":
			service, err := decodeServiceObject(event.Object, s.binding, true)
			if err != nil {
				return err
			}
			s.state.servicePresent = true
			s.state.serviceResourceVersion = service.Metadata.ResourceVersion
			return s.emitCurrentStateLocked()
		case "DELETED":
			service, err := decodeServiceObject(event.Object, s.binding, true)
			if err != nil {
				return err
			}
			s.state.servicePresent = false
			s.state.serviceResourceVersion = service.Metadata.ResourceVersion
			return s.emitTargetDeletedLocked()
		default:
			return invalidInput()
		}
	case watchEndpointSlice:
		switch event.Type {
		case "ADDED", "MODIFIED":
			record, err := decodeEndpointSliceObject(event.Object, s.binding, true)
			if err != nil {
				return err
			}
			if len(s.state.slices) >= s.binding.bounds.EndpointSliceCount {
				if _, exists := s.state.slices[record.uid]; !exists {
					return invalidInput()
				}
			}
			endpointCount := countEndpoints(s.state.slices) - endpointCountForSlice(s.state.slices[record.uid]) + len(record.endpoints)
			if endpointCount > s.binding.bounds.EndpointCount {
				return invalidInput()
			}
			s.state.slices[record.uid] = record
			s.state.endpointResourceVersion = resourceVersionFromEvent(event.Object)
			if !validResourceVersion(s.state.endpointResourceVersion) {
				return invalidInput()
			}
			return s.emitCurrentStateLocked()
		case "DELETED":
			uid, resourceVersion, err := decodeEndpointSliceDelete(event.Object, s.binding, true)
			if err != nil {
				return err
			}
			delete(s.state.slices, uid)
			s.state.endpointResourceVersion = resourceVersion
			return s.emitCurrentStateLocked()
		default:
			return invalidInput()
		}
	default:
		return invalidInput()
	}
}

func (s *observationSession) emitCurrentStateLocked() error {
	nextOrder := s.current.Revision().LocalOrder() + 1
	next, err := s.state.snapshot(s.target, nextOrder, s.binding)
	if err != nil {
		return err
	}
	if samePublicTopology(s.current, next) {
		return nil
	}
	upserts, deleted := topologyDelta(s.current, next)
	input := registry.InstanceChangeInput{
		Revision:           next.Revision(),
		Upserts:            upserts,
		DeletedInstanceIDs: deleted,
		Snapshot:           next,
	}
	if len(upserts) == 0 && len(deleted) == 0 {
		input.Kind = registry.InstanceChangeStateChanged
		input.PreviousState = s.current.State()
	} else {
		input.Kind = registry.InstanceChangeInstancesChanged
	}
	change, err := registry.NewInstanceChange(input)
	if err != nil {
		return err
	}
	if err := s.publisher.Publish(change); err != nil {
		return err
	}
	s.current = next
	return nil
}

func (s *observationSession) emitTargetDeletedLocked() error {
	nextOrder := s.current.Revision().LocalOrder() + 1
	next, err := s.state.snapshot(s.target, nextOrder, s.binding)
	if err != nil {
		return err
	}
	change, err := registry.NewInstanceChange(registry.InstanceChangeInput{
		Kind:     registry.InstanceChangeTargetDeleted,
		Revision: next.Revision(),
		Snapshot: next,
	})
	if err != nil {
		return err
	}
	if err := s.publisher.Publish(change); err != nil {
		return err
	}
	s.current = next
	s.targetDeleted = true
	go s.finishTargetDeleted()
	return nil
}

func (s *observationSession) fail(err error) {
	if err == nil {
		err = registry.ErrClosed
	}
	s.mu.Lock()
	if s.targetDeleted {
		s.mu.Unlock()
		return
	}
	if s.terminal == nil {
		s.terminal = safeTerminal(err)
	}
	terminal := s.terminal
	s.mu.Unlock()
	s.stopOnce.Do(func() {
		s.publisher.Terminate(terminal)
		s.closeBodies()
		s.directory.unregisterSession(s)
	})
}

func (s *observationSession) close() error {
	s.mu.Lock()
	targetDeleted := s.targetDeleted
	if !targetDeleted && s.terminal == nil {
		s.terminal = registry.ErrClosed
	}
	terminal := s.terminal
	s.mu.Unlock()
	if targetDeleted {
		s.finishTargetDeleted()
		return nil
	}
	s.stopOnce.Do(func() {
		s.publisher.Terminate(terminal)
		s.closeBodies()
		s.directory.unregisterSession(s)
	})
	return nil
}

func (s *observationSession) finishTargetDeleted() {
	s.stopOnce.Do(func() {
		s.closeBodies()
		s.directory.unregisterSession(s)
	})
}

func (s *observationSession) closeBodies() {
	s.mu.Lock()
	bodies := make([]io.ReadCloser, 0, len(s.bodies))
	for source, body := range s.bodies {
		bodies = append(bodies, body)
		delete(s.bodies, source)
	}
	s.mu.Unlock()
	for _, body := range bodies {
		_ = body.Close()
	}
}

func (s *observationSession) finished() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminal != nil || s.targetDeleted
}

type kubernetesWatch struct {
	session *observationSession
	inner   registry.InstanceWatch
}

func (w *kubernetesWatch) Next(ctx context.Context) (registry.InstanceChange, error) {
	change, err := w.inner.Next(ctx)
	if err != nil {
		if outcome, ok := registry.OutcomeOf(err); ok && outcome != registry.OutcomeCanceled && outcome != registry.OutcomeInvalid {
			_ = w.session.close()
		}
		return registry.InstanceChange{}, err
	}
	if change.Kind() == registry.InstanceChangeTargetDeleted {
		w.session.finishTargetDeleted()
	}
	return change, nil
}

func (w *kubernetesWatch) Close() error { return w.session.close() }

func samePublicTopology(left, right registry.InstanceSnapshot) bool {
	if left.State() != right.State() {
		return false
	}
	leftInstances := left.Instances()
	rightInstances := right.Instances()
	if len(leftInstances) != len(rightInstances) {
		return false
	}
	for index := range leftInstances {
		if !leftInstances[index].Equal(rightInstances[index]) {
			return false
		}
	}
	return true
}

func topologyDelta(previous, next registry.InstanceSnapshot) ([]registry.Instance, []string) {
	previousByID := make(map[string]registry.Instance)
	for _, instance := range previous.Instances() {
		previousByID[instance.ID()] = instance
	}
	nextByID := make(map[string]registry.Instance)
	upserts := make([]registry.Instance, 0)
	for _, instance := range next.Instances() {
		nextByID[instance.ID()] = instance
		if prior, found := previousByID[instance.ID()]; !found || !prior.Equal(instance) {
			upserts = append(upserts, instance)
		}
	}
	deleted := make([]string, 0)
	for id := range previousByID {
		if _, found := nextByID[id]; !found {
			deleted = append(deleted, id)
		}
	}
	return upserts, deleted
}

func countEndpoints(slices map[string]sliceRecord) int {
	count := 0
	for _, slice := range slices {
		count += len(slice.endpoints)
	}
	return count
}

func endpointCountForSlice(slice sliceRecord) int { return len(slice.endpoints) }

func resourceVersionFromEvent(payload []byte) string {
	var slice wireEndpointSlice
	if err := decodeOneJSON(payload, &slice); err != nil {
		return ""
	}
	return slice.Metadata.ResourceVersion
}

func safeTerminal(err error) error {
	var outcome *registry.OutcomeError
	if errors.As(err, &outcome) && outcome != nil {
		return registry.NewOutcomeError(outcome.Outcome(), outcome.Cause())
	}
	return registry.NewOutcomeError(registry.OutcomeInvalid, registry.CauseTerminalOutcomeRequired)
}

var _ registry.InstanceWatch = (*kubernetesWatch)(nil)
var _ = context.Canceled
