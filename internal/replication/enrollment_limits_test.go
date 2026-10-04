package replication

import (
	"fmt"
	"testing"
	"time"
)

func TestTerminalT03AdmissionLimiterBounds(t *testing.T) {
	s := NewEnrollmentServer(nil, Identity{})
	now := time.Now()
	for i := 0; i < 5; i++ {
		if !s.allow("192.0.2.1", now) {
			t.Fatal("initial IP burst rejected")
		}
	}
	if s.allow("192.0.2.1", now) {
		t.Fatal("IP burst exceeded")
	}
	for i := 0; i < 11; i++ {
		if !s.allow(fmt.Sprintf("192.0.2.%d", i+2), now) {
			t.Fatal("global burst rejected early")
		}
	}
	if s.allow("192.0.2.250", now) {
		t.Fatal("global burst exceeded")
	}
	// Refill is time based; rotating IPs cannot allocate an unbounded map.
	for i := 0; i < 1100; i++ {
		s.allow(fmt.Sprintf("198.51.%d.%d", i/256, i%256), now)
	}
	if len(s.ips) > 1024 {
		t.Fatal("IP accounting grew beyond cap")
	}
	if !s.allow("192.0.2.1", now.Add(time.Minute)) {
		t.Fatal("expired IP buckets not evicted/refilled")
	}
	if len(s.ips) != 1 {
		t.Fatal("expired IP accounting retained")
	}
}
