package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInstanceWatchCancellationConsumesNoQueuedChange(t *testing.T) {
	target := mustTarget(t, validTargetInput())
	instance := mustInstance(t, "uid-a", true, true, false)
	change := mustChange(t, target, 1, InstanceChangeInstancesChanged, []Instance{instance}, []Instance{instance}, nil)
	watch, publisher, err := NewInstanceWatch(2)
	if err != nil {
		t.Fatalf("NewInstanceWatch: %v", err)
	}
	if err := publisher.Publish(change); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := watch.Next(canceled); !errors.Is(err, ErrCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Next error = %v, want canceled and context.Canceled", err)
	}
	got, err := watch.Next(context.Background())
	if err != nil {
		t.Fatalf("Next after cancellation: %v", err)
	}
	if !got.Equal(change) {
		t.Fatal("canceled call consumed or changed queued event")
	}
}

func TestInstanceWatchRejectsConcurrentNext(t *testing.T) {
	watch, _, err := NewInstanceWatch(1)
	if err != nil {
		t.Fatalf("NewInstanceWatch: %v", err)
	}
	firstContext, cancelFirst := context.WithCancel(context.Background())
	blockingContext := newEnteredContext(firstContext)
	firstResult := make(chan error, 1)
	go func() {
		_, err := watch.Next(blockingContext)
		firstResult <- err
	}()
	blockingContext.awaitEntered(t)
	if _, err := watch.Next(context.Background()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("concurrent Next error = %v, want invalid", err)
	}
	cancelFirst()
	if err := awaitWatchError(t, firstResult); !errors.Is(err, ErrCanceled) {
		t.Fatalf("first Next error = %v, want canceled", err)
	}
}

func TestInstanceWatchTerminalCauseLatchesAndDiscardsQueuedChanges(t *testing.T) {
	target := mustTarget(t, validTargetInput())
	instance := mustInstance(t, "uid-a", true, true, false)
	change := mustChange(t, target, 1, InstanceChangeInstancesChanged, []Instance{instance}, []Instance{instance}, nil)
	watch, publisher, err := NewInstanceWatch(2)
	if err != nil {
		t.Fatalf("NewInstanceWatch: %v", err)
	}
	if err := publisher.Publish(change); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	terminal := NewOutcomeError(OutcomeStale, CauseResourceVersionExpired)
	publisher.Terminate(terminal)
	if err := watch.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := watch.Next(context.Background()); !errors.Is(err, terminal) {
			t.Fatalf("Next %d error = %v, want latched stale", attempt, err)
		}
	}
}

func TestInstanceWatchTerminalDropsUnsafeWrappedText(t *testing.T) {
	watch, publisher, err := NewInstanceWatch(1)
	if err != nil {
		t.Fatalf("NewInstanceWatch: %v", err)
	}
	publisher.Terminate(fmt.Errorf("provider response carried token=secret: %w", NewOutcomeError(OutcomeStale, CauseResourceVersionExpired)))
	_, err = watch.Next(context.Background())
	if !errors.Is(err, ErrStale) {
		t.Fatalf("Next error = %v, want stale", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("terminal error leaked wrapped provider text: %q", err)
	}
}

func TestInstanceWatchCloseUnblocksAndIsIdempotent(t *testing.T) {
	watch, _, err := NewInstanceWatch(1)
	if err != nil {
		t.Fatalf("NewInstanceWatch: %v", err)
	}
	result := make(chan error, 1)
	blockingContext := newEnteredContext(context.Background())
	go func() {
		_, err := watch.Next(blockingContext)
		result <- err
	}()
	blockingContext.awaitEntered(t)
	if err := watch.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := watch.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := awaitWatchError(t, result); !errors.Is(err, ErrClosed) {
		t.Fatalf("blocked Next error = %v, want closed", err)
	}
}

func TestInstanceWatchOverflowDiscardsQueuedSuccess(t *testing.T) {
	target := mustTarget(t, validTargetInput())
	ready := mustInstance(t, "uid-a", true, true, false)
	draining := mustInstance(t, "uid-a", false, true, true)
	first := mustChange(t, target, 1, InstanceChangeInstancesChanged, []Instance{ready}, []Instance{ready}, nil)
	second := mustChange(t, target, 2, InstanceChangeInstancesChanged, []Instance{draining}, []Instance{draining}, nil)
	watch, publisher, err := NewInstanceWatch(1)
	if err != nil {
		t.Fatalf("NewInstanceWatch: %v", err)
	}
	if err := publisher.Publish(first); err != nil {
		t.Fatalf("Publish first: %v", err)
	}
	err = publisher.Publish(second)
	if !errors.Is(err, ErrWatchInterrupted) {
		t.Fatalf("Publish overflow error = %v, want watch_interrupted", err)
	}
	var outcomeError *OutcomeError
	if !errors.As(err, &outcomeError) || outcomeError.Cause() != "delivery_overflow" {
		t.Fatalf("overflow cause = %v, want delivery_overflow", err)
	}
	if _, err := watch.Next(context.Background()); !errors.Is(err, ErrWatchInterrupted) {
		t.Fatalf("Next after overflow = %v, want watch_interrupted", err)
	}
}

func TestTargetDeletedDeliveredOnceThenClosed(t *testing.T) {
	target := mustTarget(t, validTargetInput())
	change := mustChange(t, target, 1, InstanceChangeTargetDeleted, nil, nil, nil)
	watch, publisher, err := NewInstanceWatch(1)
	if err != nil {
		t.Fatalf("NewInstanceWatch: %v", err)
	}
	if err := publisher.Publish(change); err != nil {
		t.Fatalf("Publish target_deleted: %v", err)
	}
	publisher.Terminate(NewOutcomeError(OutcomeStale, CauseResourceVersionExpired))
	got, err := watch.Next(context.Background())
	if err != nil {
		t.Fatalf("Next target_deleted: %v", err)
	}
	if !got.Equal(change) {
		t.Fatal("target_deleted event changed")
	}
	if _, err := watch.Next(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Next after target_deleted = %v, want closed", err)
	}
}

func awaitWatchError(t testing.TB, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Next")
		return nil
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
		t.Fatal("Next did not reach its blocking point")
	}
}
