package rendezvous

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/network"
	p "github.com/calebhabesh/file-sync/internal/protocol"
)

// W16 native runs showed a daemon locked out of the operated service: the
// client allowed four concurrent signed operations against the service's two
// outstanding challenges per device, abandoned issued challenges when callers
// cancelled, and outran the one-per-second metadata budget.

// liveClock keeps the fixture's service clock on wall time so its metadata
// bucket refills as it would in production.
func liveClock(t *testing.T, f *fixture) {
	t.Helper()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				f.now.Store(time.Now().Unix())
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-done })
}

func outstandingChallenges(f *fixture, d device) int {
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	n := 0
	for _, c := range f.s.challenges {
		if c.who == (actor{d.id, d.pin}) {
			n++
		}
	}
	return n
}

func quotaFixture(t *testing.T) (*fixture, device, *network.ServiceClient) {
	t.Helper()
	f := newFixture(t, true)
	liveClock(t, f)
	d := identity(t, 1, nil)
	c := f.client(t, d, f.roots)
	if err := c.Announce(context.Background(), "peer_data", nonce(), announcement(f, 1)); err != nil {
		t.Fatal(err)
	}
	return f, d, c
}

func TestWANW16ConcurrentSignedOperationsStayWithinChallengeBound(t *testing.T) {
	f, d, c := quotaFixture(t)
	refusedBefore := f.s.stats.quota.Load()
	var wg sync.WaitGroup
	var completed, backpressure atomic.Int64
	errs := make(chan error, 64)
	for range 8 {
		wg.Go(func() {
			for range 4 {
				_, _, err := c.Lookup(context.Background(), d.id, d.pin, "peer_data", 1)
				var service *network.ServiceError
				switch {
				case err == nil:
					completed.Add(1)
				case errors.Is(err, network.ErrBackpressure), errors.As(err, &service) && service.Code == p.NetworkQuota:
					// Local overload is typed and sends nothing to the service.
					backpressure.Add(1)
					time.Sleep(100 * time.Millisecond)
				default:
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal("unexpected signed-operation failure:", err)
	}
	if refused := f.s.stats.quota.Load() - refusedBefore; refused != 0 || completed.Load() == 0 || backpressure.Load() == 0 {
		t.Fatalf("service refusals=%d completed=%d local backpressure=%d", refused, completed.Load(), backpressure.Load())
	}
	// The reserved token keeps announcement renewal available under that load.
	if err := c.Announce(context.Background(), "peer_data", nonce(), announcement(f, 2)); err != nil {
		t.Fatal("renewal under lookup load:", err)
	}
}

func TestWANW16CancelledCallersLeaveNoOutstandingChallenge(t *testing.T) {
	// Hold each challenge response after the service has issued it, so the
	// caller's deadline always expires inside the window that used to strand it.
	f := newWrappedFixture(t, true, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			if r.URL.Path == "/network/v1/challenge" {
				time.Sleep(150 * time.Millisecond)
			}
		})
	})
	liveClock(t, f)
	d := identity(t, 1, nil)
	c := f.client(t, d, f.roots)
	for range 3 {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
		_, _, _ = c.Lookup(ctx, d.id, d.pin, "peer_data", 1)
		cancel()
	}
	if n := outstandingChallenges(f, d); n != 0 {
		t.Fatalf("%d issued challenge(s) abandoned by cancelled callers", n)
	}
	if err := c.Announce(context.Background(), "peer_data", nonce(), announcement(f, 1)); err != nil {
		t.Fatal("renewal after cancelled callers:", err)
	}
	// Before a service connection exists, cancellation still returns promptly.
	blocked, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := c.Lookup(blocked, d.id, d.pin, "peer_data", 1); err == nil {
		t.Fatal("cancelled caller was admitted")
	}
}

func TestWANW16SignedOperationsArePacedToTheServiceBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("slow; runs in the full suite")
	}
	f, d, c := quotaFixture(t)
	refusedBefore := f.s.stats.quota.Load()
	started := time.Now()
	for i := range network.MetadataBurst + 4 {
		if _, _, err := c.Lookup(context.Background(), d.id, d.pin, "peer_data", 1); err != nil {
			t.Fatalf("sequential operation %d failed after %s: %v", i, time.Since(started), err)
		}
	}
	// A caller that cannot wait for budget fails locally and sends nothing.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	failed := false
	for range network.MetadataBurst {
		if _, _, err := c.Lookup(ctx, d.id, d.pin, "peer_data", 1); err != nil {
			failed = true
			break
		}
	}
	if !failed {
		t.Fatal("exhausted budget admitted every operation")
	}
	if refused := f.s.stats.quota.Load() - refusedBefore; refused != 0 {
		t.Fatalf("service refused %d paced operation(s)", refused)
	}
}

func TestWANW16RelaySetupIsAdmittedWholeOrNotAtAll(t *testing.T) {
	f, d, c := quotaFixture(t)
	refusedBefore := f.s.stats.quota.Load()
	drain := func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
			_, _, err := c.Lookup(ctx, d.id, d.pin, "peer_data", 1)
			cancel()
			if err != nil {
				return
			}
		}
	}
	drain()
	// An exhausted budget refuses a new setup before it spends anything.
	short, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := c.BeginSetup(short, 3); err == nil {
		t.Fatal("setup admitted without budget for all its steps")
	}
	// Once admitted, every step completes by waiting rather than failing midway,
	// even while another caller competes for the same budget.
	setup, err := c.BeginSetup(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	competitor := make(chan struct{})
	go func() { defer close(competitor); drain() }()
	for step := range 3 {
		if _, _, err := c.Lookup(setup, d.id, d.pin, "peer_data", 1); err != nil {
			t.Fatalf("admitted setup failed at step %d: %v", step, err)
		}
	}
	<-competitor
	if refused := f.s.stats.quota.Load() - refusedBefore; refused != 0 {
		t.Fatalf("service refused %d operation(s)", refused)
	}
}
