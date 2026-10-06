package network

import (
	"context"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"sync/atomic"
	"testing"
	"time"
)

func TestWANW11ReviewedTimingControlsRuntimeDeadlines(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	m := NewManager(ManagerOptions{Now: func() time.Time { return time.Unix(0, clock.Load()) }})
	defer m.Close()
	timing := protocol.RouteTiming{HeadStartMS: 250, ProbeMS: 10000, CooldownMS: 20000, PollMS: 500, QuietMS: 2000}
	if err := m.ConfigureTiming(timing); err != nil {
		t.Fatal(err)
	}
	target := Target{Device: [32]byte{1}, Pin: [32]byte{2}, Profile: [32]byte{3}, Purpose: PeerData}
	if err := m.RegisterDirect(target, nil); err != nil {
		t.Fatal(err)
	}
	if err := m.ConfigureTiming(protocol.RouteTiming{}); err != ErrBackpressure {
		t.Fatal("live timing change", err)
	}
	for _, want := range []time.Duration{12 * time.Second, 22 * time.Second, 22 * time.Second} {
		m.directProbeFailed(target)
		next := m.directCooldowns[target].next
		if next.Sub(m.now()) != want {
			t.Fatal(next.Sub(m.now()), want)
		}
		clock.Store(next.UnixNano())
	}
	// Slow candidate lookup receives the reviewed bounded head-start context.
	m.RegisterDirect(target, func(ctx context.Context, _ Target, _ uint64) (protocol.NetworkAnnouncement, error) {
		<-ctx.Done()
		return protocol.NetworkAnnouncement{}, ctx.Err()
	})
	start := time.Now()
	m.directCandidates(context.Background(), target)
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond || elapsed > time.Second {
		t.Fatal(elapsed)
	}
}
