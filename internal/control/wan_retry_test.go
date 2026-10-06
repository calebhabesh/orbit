package control

import (
	"strconv"
	"testing"
	"time"
)

func TestWANW11PreparedEnrollmentRetryRetainsOriginalExpiry(t *testing.T) {
	now := time.Unix(1700000000, 0)
	for _, tc := range []struct {
		phase     string
		remaining time.Duration
		want      time.Duration
	}{
		{"request_prepared", time.Minute, 25 * time.Second},
		{"request_prepared", 30 * time.Second, 25 * time.Second},
		{"request_prepared", 25 * time.Second, 60 * time.Second},
		{"request_prepared", -time.Second, 60 * time.Second},
		{"awaiting_approval", time.Minute, 60 * time.Second},
	} {
		expiry := strconv.FormatInt(now.Add(tc.remaining).Unix(), 10)
		if got := preparedRequestThrottleDelay(tc.phase, expiry, now); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	if got := preparedRequestThrottleDelay("request_prepared", "invalid", now); got != time.Minute {
		t.Fatal("malformed proof changed recovery", got)
	}
}
