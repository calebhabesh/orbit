package replication

import (
	"testing"
	"time"
)

// F10 (E00 diagnosis): a joining device awaiting approval saw RATE_LIMITED
// from the inviter's per-source enrollment bucket (5/min, burst 5), because
// each status check costs two POSTs (challenge, then signed status) and the
// joiner checked every 15 s (8/min). E07 spaces checks 30–36 s apart
// (control.approvalPollInterval). Even at the fastest spacing, 30 minutes of
// waiting after the prepare and submit requests is never refused.
func TestOnboardingE07F10ApprovalWaitFitsInviterSourceBucket(t *testing.T) {
	s := NewEnrollmentServer(nil, Identity{})
	start := time.Unix(1_790_000_000, 0)
	source := "203.0.113.7"
	requests := []time.Duration{0, time.Second}
	for poll := 1; time.Duration(poll)*30*time.Second <= 30*time.Minute; poll++ {
		at := time.Duration(poll) * 30 * time.Second
		requests = append(requests, at, at+200*time.Millisecond)
	}
	for i, at := range requests {
		if !s.allow(source, start.Add(at)) {
			t.Fatalf("request %d at %s of the approval wait refused", i+1, at)
		}
	}
	// The old 15 s spacing is refused within the first two minutes.
	s = NewEnrollmentServer(nil, Identity{})
	refused := false
	for poll := 0; poll < 8 && !refused; poll++ {
		at := time.Duration(poll) * 15 * time.Second
		refused = !s.allow(source, start.Add(at)) || !s.allow(source, start.Add(at+200*time.Millisecond))
	}
	if !refused {
		t.Fatal("the fixture no longer models the trial's refusal at 15 s spacing")
	}
}
