package rendezvous

import (
	"bufio"
	"context"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
)

// R00 baseline: what one idle online device costs the service, and what a new
// device observes when the directory is full. Both run the production
// RelayRuntime (control channel, heartbeats, renewals for both purposes)
// against the production Service on a local TLS listener; nothing here
// contacts an operated service.

// readyDeadline bounds waits for eventual readiness; the measured times are
// logged, not asserted.
const readyDeadline = 90 * time.Second

func r00Runtime(t *testing.T, f *fixture, id int) (*network.RelayRuntime, device) {
	t.Helper()
	d := identity(t, id, nil)
	c := f.client(t, d, f.roots)
	m := network.NewManager(network.ManagerOptions{})
	t.Cleanup(func() { _ = m.Close() })
	r, err := network.NewRelayRuntime(context.Background(), c, m, 1, f.s.digest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	// One paired peer, offline: its pin is carried on the control channel but
	// no lookup or dial happens while the device is idle.
	peer := identity(t, id+1000, nil)
	var target network.Target
	raw, _ := hex.DecodeString(peer.id)
	copy(target.Device[:], raw)
	raw, _ = hex.DecodeString(peer.pin)
	copy(target.Pin[:], raw)
	raw, _ = hex.DecodeString(f.s.digest)
	target.Profile = history.Digest(raw)
	target.Purpose = network.PeerData
	if err = r.Register(target); err != nil {
		t.Fatal(err)
	}
	return r, d
}

// TestFinalR00IdleDeviceControlCost measures raw TLS bytes (both directions,
// at the service socket) for one idle device after it becomes ready. It runs
// for ORBIT_FINAL_R00_WINDOW (default 150s, enough to cross a renewal) and is
// skipped in short mode.
func TestFinalR00IdleDeviceControlCost(t *testing.T) {
	if testing.Short() {
		t.Skip("real heartbeat and renewal timers; runs in the full suite")
	}
	window := 150 * time.Second
	if v := os.Getenv("ORBIT_FINAL_R00_WINDOW"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatal(err)
		}
		window = d
	}
	var announces, challenges atomic.Int64
	f := newWrappedFixture(t, true, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/announce"):
				announces.Add(1)
			case strings.HasSuffix(r.URL.Path, "/challenge"):
				challenges.Add(1)
			}
			next.ServeHTTP(w, r)
		})
	})
	liveClock(t, f)
	r, _ := r00Runtime(t, f, 1)
	// Generous: the race-core CI job runs every package's race-instrumented
	// tests at once, and a starved runtime can take tens of seconds.
	ctx, cancel := context.WithTimeout(context.Background(), readyDeadline)
	defer cancel()
	if err := r.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	startIn, startOut := f.wireIn.Load(), f.wireOut.Load()
	startAnnounces, startChallenges := announces.Load(), challenges.Load()
	time.Sleep(window)
	in, out := f.wireIn.Load()-startIn, f.wireOut.Load()-startOut
	hours := window.Hours()
	f.s.mu.Lock()
	records, controls := len(f.s.records), len(f.s.controls)
	f.s.mu.Unlock()
	if records != 2 || controls != 2 || !r.Ready() {
		t.Fatalf("idle device lost readiness: records=%d controls=%d", records, controls)
	}
	t.Logf("R00 idle window=%s in=%d out=%d bytes; per hour in=%.0f out=%.0f total=%.0f; announces=%d challenges=%d; records=%d controls=%d",
		window, in, out, float64(in)/hours, float64(out)/hours, float64(in+out)/hours,
		announces.Load()-startAnnounces, challenges.Load()-startChallenges, records, controls)
}

// TestFinalR00FullDirectoryRefusesNewDevice fills the directory to one slot
// short, then starts a real device. One purpose fits, the other is refused
// with QUOTA; the device stays not ready, retries at a bounded rate, and
// becomes ready once a slot frees. Controls are admitted independently.
func TestFinalR00FullDirectoryRefusesNewDevice(t *testing.T) {
	var refused atomic.Int64
	f := newWrappedFixture(t, true, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			next.ServeHTTP(rec, r)
			if strings.HasSuffix(r.URL.Path, "/announce") && rec.status != 200 {
				refused.Add(1)
			}
		})
	})
	liveClock(t, f)
	now := uint64(time.Now().Unix())
	f.s.mu.Lock()
	for i := 0; i < MaxRecords-1; i++ {
		d := identity(t, 5000+i, nil)
		a := p.NetworkAnnouncement{Generation: 1, Expires: p.NetworkUint(now + 600)}
		f.s.records[routeKey{actor{d.id, d.pin}, "peer_data"}] = record{p.NetworkProof{Profile: f.s.digest, Sender: d.id, SenderPin: d.pin}, a}
	}
	f.s.mu.Unlock()
	r, _ := r00Runtime(t, f, 1)
	observe := 25 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), observe)
	err := r.WaitReady(ctx)
	cancel()
	if err == nil {
		t.Fatal("device became ready in a full directory")
	}
	f.s.mu.Lock()
	records, controls := len(f.s.records), len(f.s.controls)
	f.s.mu.Unlock()
	n := refused.Load()
	if records != MaxRecords || controls != 2 || n == 0 || n > 8 {
		t.Fatalf("full directory: records=%d controls=%d refused announces=%d in %s", records, controls, n, observe)
	}
	t.Logf("R00 full directory: records=%d controls=%d refused announces=%d in %s; ready=%v", records, controls, n, observe, r.Ready())
	// Free one synthetic slot; the refused purpose's next renewal succeeds.
	f.s.mu.Lock()
	for k := range f.s.records {
		if k.device != identity(t, 1, nil).id {
			delete(f.s.records, k)
			break
		}
	}
	f.s.mu.Unlock()
	start := time.Now()
	ctx, cancel = context.WithTimeout(context.Background(), readyDeadline)
	defer cancel()
	if err = r.WaitReady(ctx); err != nil {
		t.Fatal("device did not recover after a slot freed", err)
	}
	t.Logf("R00 full directory: ready %s after a slot freed", time.Since(start).Round(time.Millisecond))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) { s.status = code; s.ResponseWriter.WriteHeader(code) }

// Hijack lets the control WebSocket upgrade pass through the recorder.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return s.ResponseWriter.(http.Hijacker).Hijack()
}
