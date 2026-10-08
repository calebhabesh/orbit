package rendezvous

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
)

// EG4/E09: the monthly count persists privately, survives a restart (reopen),
// ignores an earlier month's file and resets at the UTC month boundary.
func TestOnboardingE09RelayBudgetPersistsAndResets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay-month.json")
	var clock atomic.Int64
	clock.Store(time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC).Unix())
	now := func() time.Time { return time.Unix(clock.Load(), 0) }
	b, err := OpenRelayBudget(path, 1000, now)
	if err != nil {
		t.Fatal(err)
	}
	b.Add(600)
	b.Add(500)
	if !b.Exhausted() {
		t.Fatal("1100 of 1000 not exhausted")
	}
	if err = b.Flush(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode %v %v", info.Mode(), err)
	}
	// Restart: the count survives.
	b, err = OpenRelayBudget(path, 1000, now)
	if err != nil {
		t.Fatal(err)
	}
	if month, used, _ := b.Snapshot(); month != "2026-10" || used != 1100 || !b.Exhausted() {
		t.Fatalf("after reopen: %s %d", month, used)
	}
	// Month boundary: a new UTC month starts at zero, and is persisted.
	clock.Store(time.Date(2026, 11, 1, 0, 0, 1, 0, time.UTC).Unix())
	if b.Exhausted() {
		t.Fatal("budget did not reset at the month boundary")
	}
	if err = b.Flush(); err != nil {
		t.Fatal(err)
	}
	b, _ = OpenRelayBudget(path, 1000, now)
	if month, used, _ := b.Snapshot(); month != "2026-11" || used != 0 {
		t.Fatalf("after month reset: %s %d", month, used)
	}
	if got := NextMonth(time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)); !got.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("NextMonth across a year: %s", got)
	}
	// A state file readable by others is refused rather than trusted.
	if err = os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenRelayBudget(path, 1000, now); err == nil {
		t.Fatal("non-private state file accepted")
	}
	if _, err = OpenRelayBudget("", 0, now); err == nil {
		t.Fatal("zero (unbounded) budget accepted")
	}
}

// E09: a live data relay ends at the chunk boundary once the budget is spent;
// new data relays are refused with RELAY_BUDGET and counted; pairing
// (enrollment) relays stay available.
func TestOnboardingE09RelayRefusesAndEndsAtBudget(t *testing.T) {
	r := newRelayFixture(t, "peer_data")
	budget, err := OpenRelayBudget(filepath.Join(t.TempDir(), "relay-month.json"), 64<<10, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.f.s.mu.Lock()
	r.f.s.budget = budget
	r.f.s.mu.Unlock()
	a, b := r.legs(t)
	chunk := make([]byte, 16<<10)
	go func() {
		for i := 0; i < 64; i++ {
			if _, err := a.Write(chunk); err != nil {
				return
			}
		}
	}()
	_ = b.SetReadDeadline(time.Now().Add(5 * time.Second))
	received, err := io.Copy(io.Discard, b)
	if err == nil && received >= 64*int64(len(chunk)) {
		t.Fatal("relay forwarded everything past the budget")
	}
	if month, used, limit := budget.Snapshot(); used < limit || month == "" {
		t.Fatalf("budget not spent: %d of %d", used, limit)
	}
	if received > 64<<10+int64(network.StreamBufferBytes)*2 {
		t.Fatalf("forwarded %d bytes, far past the 64 KiB budget", received)
	}
	t.Logf("received %d bytes before the session ended at the budget", received)
	waitRelayEmpty(t, r.f)

	// A new data session is refused with RELAY_BUDGET.
	r2 := newRelayFixture(t, "peer_data")
	r2.f.s.mu.Lock()
	r2.f.s.budget = budget
	r2.f.s.mu.Unlock()
	_, err = r2.ca.Attach(r2.ctx, r2.ta)
	var service *network.ServiceError
	if !errors.As(err, &service) || service.Code != p.NetworkRelayBudget {
		t.Fatalf("new data relay: %v", err)
	}
	if m := r2.f.s.Metrics(); m.RefusedBudget != 1 || m.RelayMonthLimit != 64<<10 || m.RelayMonthBytes < 64<<10 {
		t.Fatalf("metrics %+v", m)
	}
	// Pairing relays stay available.
	r3 := newRelayFixture(t, "enrollment")
	r3.f.s.mu.Lock()
	r3.f.s.budget = budget
	r3.f.s.mu.Unlock()
	a3, b3 := r3.legs(t)
	go func() { _, _ = a3.Write([]byte("pairing")) }()
	got := make([]byte, 7)
	_ = b3.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err = io.ReadFull(b3, got); err != nil || string(got) != "pairing" {
		t.Fatalf("enrollment relay blocked by the data budget: %v", err)
	}
}

// EG4: the relay counter excludes TLS and WebSocket framing. Measure the
// service's raw wire egress for a 4 MiB relayed transfer against the payload
// counter, so the deployed budget can leave matching headroom.
func TestOnboardingE09RelayFramingOverhead(t *testing.T) {
	r := newRelayFixture(t, "peer_data")
	a, b := r.legs(t)
	payload := make([]byte, 4<<20)
	before, wireBefore := r.f.s.stats.relayBytes.Load(), r.f.wireOut.Load()
	go func() { _, _ = a.Write(payload) }()
	_ = b.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.ReadFull(b, make([]byte, len(payload))); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	counted := r.f.s.stats.relayBytes.Load() - before
	wire := r.f.wireOut.Load() - wireBefore
	ratio := float64(wire) / float64(counted)
	t.Logf("payload counter %d bytes, service wire egress %d bytes, overhead %.2f%%", counted, wire, (ratio-1)*100)
	if counted < uint64(len(payload)) || ratio < 1 || ratio > 1.05 {
		t.Fatalf("unexpected accounting: counted %d wire %d ratio %.4f", counted, wire, ratio)
	}
}
