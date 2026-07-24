package routerauth

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReplayGuardAcceptsAtMostOneConcurrentPresentation(t *testing.T) {
	now := time.Unix(100, 0)
	guard, err := NewReplayGuard(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int64
	var group sync.WaitGroup
	for range 64 {
		group.Add(1)
		go func() {
			defer group.Done()
			if guard.Accept("rtj_concurrent", 130) {
				accepted.Add(1)
			}
		}()
	}
	group.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted = %d", accepted.Load())
	}
}

func TestReplayGuardRemovesOnlyExpiredEntries(t *testing.T) {
	now := time.Unix(100, 0)
	guard, _ := NewReplayGuard(func() time.Time { return now })
	if !guard.Accept("rtj_expiring", 101) || !guard.Accept("rtj_live", 200) {
		t.Fatal("initial credentials rejected")
	}
	now = time.Unix(101, 0)
	if !guard.Accept("rtj_expiring", 130) {
		t.Fatal("expired jti was not removed")
	}
	if guard.Accept("rtj_live", 220) {
		t.Fatal("live jti was removed")
	}
}

func TestReplayGuardRejectsCredentialAtExpirationBoundary(t *testing.T) {
	now := time.Unix(100, 0)
	guard, err := NewReplayGuard(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if !guard.Accept("rtj_boundary", 101) {
		t.Fatal("initial credential rejected")
	}
	now = time.Unix(101, 0)
	if guard.Accept("rtj_boundary", 101) {
		t.Fatal("credential was accepted at its expiration boundary")
	}
}
