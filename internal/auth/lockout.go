package auth

import (
	"sync"
	"time"
)

type loginAttempt struct {
	Count    int
	LastFail time.Time
	LockedAt time.Time
}

type LockoutTracker struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
	maxFails int
	lockDur  time.Duration
}

func NewLockoutTracker(maxFailedAttempts int, lockoutDuration time.Duration) *LockoutTracker {
	return &LockoutTracker{
		attempts: make(map[string]*loginAttempt),
		maxFails: maxFailedAttempts,
		lockDur:  lockoutDuration,
	}
}

// IsLocked returns true if the given key (IP or username) is currently locked out.
func (lt *LockoutTracker) IsLocked(key string) bool {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	a, ok := lt.attempts[key]
	if !ok {
		return false
	}

	if a.Count >= lt.maxFails && time.Since(a.LockedAt) < lt.lockDur {
		return true
	}

	if a.Count >= lt.maxFails && time.Since(a.LockedAt) >= lt.lockDur {
		delete(lt.attempts, key)
	}

	return false
}

// RecordFailure increments the failure count for a key.
func (lt *LockoutTracker) RecordFailure(key string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	a, ok := lt.attempts[key]
	if !ok {
		a = &loginAttempt{}
		lt.attempts[key] = a
	}

	a.Count++
	a.LastFail = time.Now()

	if a.Count >= lt.maxFails {
		a.LockedAt = time.Now()
	}
}

// Reset clears the failure count for a key (called on successful login).
func (lt *LockoutTracker) Reset(key string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	delete(lt.attempts, key)
}
