package postgres

import (
	"testing"
	"time"
)

func TestNextLifecycleTimeNeverRegressesLockedInstallationTimestamp(t *testing.T) {
	previous := time.Date(2026, 7, 15, 12, 0, 0, 123456789, time.UTC)
	normalizedPrevious := previous.Truncate(time.Microsecond)
	future := previous.Add(time.Second)

	if got := nextLifecycleTime(previous, future); !got.Equal(future.Truncate(time.Microsecond)) {
		t.Fatalf("future candidate = %s, want %s", got, future.Truncate(time.Microsecond))
	}
	for _, candidate := range []time.Time{previous, previous.Add(time.Nanosecond), previous.Add(-time.Second)} {
		got := nextLifecycleTime(previous, candidate)
		want := normalizedPrevious.Add(time.Microsecond)
		if !got.Equal(want) {
			t.Fatalf("stale candidate %s = %s, want %s", candidate, got, want)
		}
	}
}
