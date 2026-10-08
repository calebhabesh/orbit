package replication

import (
	"os"
	"testing"
	"time"
)

// E00 F10 diagnosis (2026-10-08 trial): a joining device awaiting approval saw
// RATE_LIMITED. orbit-net refuses with QUOTA_EXCEEDED, not RATE_LIMITED; this
// code comes from the inviter's enrollment server per-source bucket (5/min,
// burst 5). The joiner's status poll runs every 15 s
// (control/terminal_setup.go) and each StatusV3 makes two POSTs (challenge,
// then signed status), i.e. 8 requests per minute.
//
// Asserts approved behavior (a waiting joiner is never refused); deliberately
// fails until E07. Opt in with ORBIT_ONBOARDING_BASELINE=1.
func TestOnboardingE00F10StatusPollingFitsInviterSourceBucket(t *testing.T) {
	if os.Getenv("ORBIT_ONBOARDING_BASELINE") != "1" {
		t.Skip("deliberately failing E00 baseline; opt in with ORBIT_ONBOARDING_BASELINE=1")
	}
	s := NewEnrollmentServer(nil, Identity{})
	start := time.Unix(1_790_000_000, 0)
	source := "203.0.113.7"
	// The routed join first spends a challenge and the request submission.
	requests := []time.Duration{0, time.Second}
	// Then, while waiting for approval: one status (two POSTs) every 15 s.
	for poll := 1; poll <= 40; poll++ {
		at := time.Duration(poll) * 15 * time.Second
		requests = append(requests, at, at+200*time.Millisecond)
	}
	for i, at := range requests {
		if !s.allow(source, start.Add(at)) {
			t.Fatalf("request %d at %s of the approval wait refused with RATE_LIMITED (per-source 5/min, burst 5; client sends 8/min)", i+1, at)
		}
	}
}
