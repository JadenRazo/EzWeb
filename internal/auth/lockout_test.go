package auth

import (
	"testing"
	"time"
)

func TestLockout_NewTrackerNotLocked(t *testing.T) {
	lt := NewLockoutTracker(3, 5*time.Minute)
	if lt.IsLocked("192.168.1.1") {
		t.Error("new tracker should not have any locked IPs")
	}
}

func TestLockout_LocksAfterMaxFailures(t *testing.T) {
	lt := NewLockoutTracker(3, 5*time.Minute)
	ip := "192.168.1.1"

	lt.RecordFailure(ip)
	lt.RecordFailure(ip)
	if lt.IsLocked(ip) {
		t.Error("should not be locked after 2 failures (max is 3)")
	}

	lt.RecordFailure(ip)
	if !lt.IsLocked(ip) {
		t.Error("should be locked after 3 failures")
	}
}

func TestLockout_UnlocksAfterDuration(t *testing.T) {
	lt := NewLockoutTracker(2, 50*time.Millisecond)
	ip := "10.0.0.1"

	lt.RecordFailure(ip)
	lt.RecordFailure(ip)
	if !lt.IsLocked(ip) {
		t.Error("should be locked after max failures")
	}

	time.Sleep(60 * time.Millisecond)
	if lt.IsLocked(ip) {
		t.Error("should be unlocked after lockout duration expires")
	}
}

func TestLockout_ResetClearsLockout(t *testing.T) {
	lt := NewLockoutTracker(2, 5*time.Minute)
	ip := "10.0.0.2"

	lt.RecordFailure(ip)
	lt.RecordFailure(ip)
	if !lt.IsLocked(ip) {
		t.Error("should be locked")
	}

	lt.Reset(ip)
	if lt.IsLocked(ip) {
		t.Error("should not be locked after reset")
	}
}
