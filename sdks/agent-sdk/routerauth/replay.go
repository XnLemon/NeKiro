package routerauth

import (
	"errors"
	"sync"
	"time"
)

type ReplayGuard struct {
	mu      sync.Mutex
	now     func() time.Time
	expires map[string]int64
}

func NewReplayGuard(now func() time.Time) (*ReplayGuard, error) {
	if now == nil {
		return nil, errors.New("router credential replay clock is required")
	}
	return &ReplayGuard{now: now, expires: make(map[string]int64)}, nil
}

func (guard *ReplayGuard) Accept(jwtID string, expiresAt int64) bool {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	now := guard.now().UTC().Unix()
	for existingID, existingExpiry := range guard.expires {
		if now >= existingExpiry {
			delete(guard.expires, existingID)
		}
	}
	if now >= expiresAt {
		return false
	}
	if existingExpiry, exists := guard.expires[jwtID]; exists && now < existingExpiry {
		return false
	}
	guard.expires[jwtID] = expiresAt
	return true
}
