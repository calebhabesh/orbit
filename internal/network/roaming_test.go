package network

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	p "github.com/calebhabesh/orbit/internal/protocol"
)

func TestWANW11RelayDirectReprobeAndGenerationDrain(t *testing.T) {
	s, trust, target := managerServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("pinned bytes")) })
	target.Profile[0] = 1
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	var usable atomic.Bool
	var direct, relay atomic.Int32
	m := NewManager(ManagerOptions{Now: func() time.Time { return time.Unix(0, clock.Load()) }, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		direct.Add(1)
		if !usable.Load() {
			return nil, errors.New("blocked fixture route")
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", s.Listener.Addr().String())
	}})
	defer m.Close()
	if err := m.SetRelay(target, func(ctx context.Context, _ Target) (net.Conn, error) {
		relay.Add(1)
		return (&net.Dialer{}).DialContext(ctx, "tcp", s.Listener.Addr().String())
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterDirect(target, nil); err != nil {
		t.Fatal(err)
	}
	expires := m.now().Add(5 * time.Minute)
	candidate := []p.NetworkCandidate{{Transport: "tcp", Address: "192.168.10.2:4567", Scope: "lan", Interface: "test"}}
	if err := m.SetCandidates(target, candidate, 1, expires, false); err != nil {
		t.Fatal(err)
	}
	request := func() (http.RoundTripper, *http.Response) {
		t.Helper()
		rt, err := m.Transport(context.Background(), target, trust)
		if err != nil {
			t.Fatal(err)
		}
		return rt, round(t, rt, LogicalOrigin(target))
	}
	old, res := request()
	if m.Observe(target).Route != "relay" {
		t.Fatal(m.Observe(target))
	}
	// Busy response is protected even when a probe interval expires.
	usable.Store(true)
	clock.Add(int64(DirectProbeInterval + DirectProbeJitter + time.Second))
	rt, err := m.Transport(context.Background(), target, trust)
	if err != nil || rt.(*borrowedPool).pool != old.(*borrowedPool).pool {
		t.Fatal("active pool retired", err)
	}
	data, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil || string(data) != "pinned bytes" {
		t.Fatal(string(data), err)
	}
	fresh, res := request()
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if fresh.(*borrowedPool).pool == old.(*borrowedPool).pool || m.Observe(target).Route != "direct" {
		t.Fatal("idle relay failed to reprobe", m.Observe(target))
	}
	// Old-generation bodies may drain, but old borrowers cannot submit new work.
	_, res = request()
	m.NetworkChanged()
	data, err = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil || string(data) != "pinned bytes" {
		t.Fatal("drain", err)
	}
	req, _ := http.NewRequest("GET", LogicalOrigin(target), nil)
	if _, err = fresh.RoundTrip(req); !errors.Is(err, ErrStale) {
		t.Fatal("stale borrower", err)
	}
	if m.Observe(target).Code != "NOT_TESTED" {
		t.Fatal("stale observation", m.Observe(target))
	}
	// Network change drops interface candidates; a fresh pool safely falls back.
	_, res = request()
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if m.Observe(target).Route != "relay" || relay.Load() != 2 || direct.Load() != 2 {
		t.Fatal(m.Observe(target), relay.Load(), direct.Load())
	}
	t.Log("authenticated relay/direct/relay routing used real TLS/HTTP; addresses/dial outcome and time were injected")
}

func TestWANW11LateLookupCannotPublishIntoNewGeneration(t *testing.T) {
	m := NewManager(ManagerOptions{})
	defer m.Close()
	target := Target{Device: [32]byte{1}, Pin: [32]byte{2}, Purpose: PeerData}
	entered, release := make(chan struct{}), make(chan struct{})
	if err := m.RegisterDirect(target, func(context.Context, Target, uint64) (p.NetworkAnnouncement, error) {
		close(entered)
		<-release
		return p.NetworkAnnouncement{Generation: 1, Expires: p.NetworkUint(time.Now().Add(time.Minute).Unix()), Candidates: []p.NetworkCandidate{{Transport: "tcp", Address: "8.8.8.8:443", Scope: "public"}}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	done := make(chan []p.NetworkCandidate, 1)
	go func() { done <- m.directCandidates(context.Background(), target) }()
	<-entered
	m.NetworkChanged()
	close(release)
	if got := <-done; len(got) != 0 {
		t.Fatal("late lookup escaped", got)
	}
	m.mu.Lock()
	lease := m.public[target]
	m.mu.Unlock()
	if len(lease.candidates) != 0 {
		t.Fatal("late lookup cached", lease)
	}
}

func TestWANW11FailedICERetryIsTimedAndQuotaQuietPeriodDoesNotSlide(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	m := NewManager(ManagerOptions{Now: func() time.Time { return time.Unix(0, clock.Load()) }})
	defer m.Close()
	var attempts atomic.Int32
	if err := m.enableICE(func(context.Context, Target) (*QUICEndpoint, net.Addr, error) {
		attempts.Add(1)
		return nil, nil, ErrNoRoute
	}); err != nil {
		t.Fatal(err)
	}
	pool := &quicPool{manager: m, target: Target{Device: [32]byte{1}, Pin: [32]byte{2}, Purpose: PeerData}}
	for range 20 {
		_, _, _ = pool.connection(context.Background())
	}
	if attempts.Load() != 1 {
		t.Fatal("unbounded failed checks", attempts.Load())
	}
	clock.Add(int64(DirectProbeInterval))
	_, _, _ = pool.connection(context.Background())
	if attempts.Load() != 2 {
		t.Fatal("sticky ICE failure", attempts.Load())
	}
	r := &RelayRuntime{}
	r.recordServiceFailure(&ServiceError{Code: p.NetworkQuota})
	deadline := r.retryAfter
	for range 20 {
		r.recordServiceFailure(r.serviceCooldown())
	}
	if r.retryAfter != deadline {
		t.Fatal("local retries extended cooldown")
	}
	r.retryAfter = time.Now().Add(-time.Second)
	if err := r.serviceCooldown(); err != nil {
		t.Fatal("cooldown never expires", err)
	}
	r.recordServiceFailure(&ServiceError{Code: p.NetworkIdentityMismatch})
	if err := r.serviceCooldown(); err != nil {
		t.Fatal("pin error became quota", err)
	}
}

func TestWANW11NetworkWatcherCoalescesAndJoins(t *testing.T) {
	var snapshot atomic.Int32
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	stop := WatchNetwork(context.Background(), func() (string, error) {
		switch snapshot.Load() {
		case 0:
			return "original", nil
		case 1:
			return "changed", nil
		default:
			return "", errors.New("sampling failed")
		}
	}, func() { calls.Add(1); close(entered); <-release }, 5*time.Millisecond)
	snapshot.Store(1)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("stable change missed")
	}
	joined := make(chan struct{})
	go func() { stop(); close(joined) }()
	select {
	case <-joined:
		t.Fatal("watcher did not join callback")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("watcher leaked")
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	if _, err := NetworkSnapshot(nil); err != nil {
		t.Fatal("native read-only snapshot", err)
	}
}

func TestWANW11ColdRaceRetainsQuotaAfterCanceledICEWaiter(t *testing.T) {
	m := NewManager(ManagerOptions{})
	defer m.Close()
	target := Target{Device: [32]byte{1}, Pin: [32]byte{2}, Profile: [32]byte{3}, Purpose: PeerData}
	entered := make(chan struct{})
	if err := m.SetRelay(target, func(context.Context, Target) (net.Conn, error) { return nil, &ServiceError{Code: p.NetworkQuota} }); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterDirect(target, nil); err != nil {
		t.Fatal(err)
	}
	if err := m.enableICE(func(ctx context.Context, _ Target) (*QUICEndpoint, net.Addr, error) {
		close(entered)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	_, trust, _ := managerServer(t, func(http.ResponseWriter, *http.Request) { t.Error("failed route disclosed HTTP") })
	rt, err := m.Transport(context.Background(), target, trust)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", LogicalOrigin(target), nil)
	started := time.Now()
	_, err = rt.RoundTrip(req)
	var refusal *ServiceError
	if !errors.As(err, &refusal) || refusal.Code != p.NetworkQuota {
		t.Fatal("quota obscured by losing waiter", err)
	}
	select {
	case <-entered:
	default:
		t.Fatal("competing waiter did not run")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("losing waiter delayed quota", time.Since(started))
	}
}

func TestWANW11ResponderQuotaRetryIsBoundedAndKeepsSession(t *testing.T) {
	endpoint := &RelayEndpoint{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	attempts := 0
	started := time.Now()
	if err := endpoint.retryQuota(ctx, func() error {
		attempts++
		if attempts == 1 {
			return &ServiceError{Code: p.NetworkQuota}
		}
		return nil
	}); err != nil || attempts != 2 || time.Since(started) < ServiceRetryQuietPeriod {
		t.Fatal("quiet retry", attempts, err)
	}
	attempts = 0
	if err := endpoint.retryQuota(ctx, func() error { attempts++; return &ServiceError{Code: p.NetworkIdentityMismatch} }); err == nil || attempts != 1 {
		t.Fatal("authentication refusal retried", attempts, err)
	}
	short, end := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer end()
	attempts = 0
	if err := endpoint.retryQuota(short, func() error { attempts++; return &ServiceError{Code: p.NetworkQuota} }); !errors.Is(err, context.DeadlineExceeded) || attempts != 1 {
		t.Fatal("cancellation ignored", attempts, err)
	}
}

func TestWANW11DirectCooldownGrowsAndFreshNetworkClearsIt(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	m := NewManager(ManagerOptions{Now: func() time.Time { return time.Unix(0, clock.Load()) }})
	defer m.Close()
	target := Target{Device: [32]byte{1}, Pin: [32]byte{2}, Profile: [32]byte{3}, Purpose: PeerData}
	if err := m.SetRelay(target, func(context.Context, Target) (net.Conn, error) { return nil, ErrNoRoute }); err != nil {
		t.Fatal(err)
	}
	for _, delay := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 4 * time.Minute} {
		m.directProbeFailed(target)
		deadline := m.directCooldowns[target].next
		if deadline.Sub(m.now()) != delay+2*time.Second || m.directProbeAllowed(target) {
			t.Fatal("wrong cooldown", deadline.Sub(m.now()))
		}
		m.directProbeFailed(target)
		if m.directCooldowns[target].next != deadline {
			t.Fatal("sliding cooldown")
		}
		clock.Store(deadline.UnixNano())
	}
	m.NetworkChanged()
	if !m.directProbeAllowed(target) || len(m.directCooldowns) != 0 {
		t.Fatal("new network retained negative routing cache")
	}
}

func TestWANW11RepeatedFlappingBoundsBusyGenerations(t *testing.T) {
	s, trust, target := managerServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("protected response")) })
	manager := NewManager(ManagerOptions{})
	defer manager.Close()
	var targets []Target
	var bodies []io.ReadCloser
	for peer := range 8 {
		known := target
		known.Device[0] = byte(peer + 1)
		if err := manager.SetManual(known, s.URL); err != nil {
			t.Fatal(err)
		}
		targets = append(targets, known)
		rt, err := manager.Transport(context.Background(), known, trust)
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, round(t, rt, s.URL).Body)
	}
	manager.AdvanceGeneration()
	for _, known := range targets {
		rt, err := manager.Transport(context.Background(), known, trust)
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, round(t, rt, s.URL).Body)
	}
	for range 200 {
		manager.AdvanceGeneration()
		for _, known := range targets {
			if _, err := manager.Transport(context.Background(), known, trust); !errors.Is(err, ErrBackpressure) {
				t.Fatal("busy generations exceeded per-target pool bound", err)
			}
		}
	}
	manager.mu.Lock()
	pools, requests := len(manager.pools), len(manager.requests)
	manager.mu.Unlock()
	if pools != 16 || requests != 16 {
		t.Fatal("flapping grew retained work", pools, requests)
	}
	for _, body := range bodies {
		got, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || string(got) != "protected response" {
			t.Fatal("flapping damaged draining bytes", string(got), err)
		}
	}
	manager.mu.Lock()
	pools, requests = len(manager.pools), len(manager.requests)
	manager.mu.Unlock()
	if pools != 0 || requests != 0 {
		t.Fatal("drained generations leaked", pools, requests)
	}
	t.Log("200 reordered generations across eight pinned TCP targets retained exactly two pools/requests per target; body drain pruned all old work")
}

func TestWANW11FreshLookupRevalidatesUnchangedRemoteLeaseAfterRoam(t *testing.T) {
	manager := NewManager(ManagerOptions{})
	defer manager.Close()
	target := Target{Device: [32]byte{1}, Pin: [32]byte{2}, Purpose: PeerData}
	candidate := p.NetworkCandidate{Transport: "tcp", Address: "8.8.8.8:443", Scope: "public"}
	announcement := p.NetworkAnnouncement{Generation: 1, Expires: p.NetworkUint(time.Now().Add(time.Minute).Unix()), Candidates: []p.NetworkCandidate{candidate}}
	var lookups atomic.Int32
	if err := manager.RegisterDirect(target, func(context.Context, Target, uint64) (p.NetworkAnnouncement, error) {
		lookups.Add(1)
		return announcement, nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(manager.directCandidates(context.Background(), target)) != 1 {
		t.Fatal("initial lease")
	}
	manager.NetworkChanged()
	if len(manager.directCandidates(context.Background(), target)) != 1 || lookups.Load() != 2 {
		t.Fatal("unchanged remote lease could not be freshly authenticated after local roam", lookups.Load())
	}
}

func TestWANW11CandidateArrivalInvalidatesEarlyEmptyProbe(t *testing.T) {
	manager := NewManager(ManagerOptions{})
	defer manager.Close()
	target := Target{Device: [32]byte{1}, Pin: [32]byte{2}, Purpose: PeerData}
	if err := manager.RegisterDirect(target, nil); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int32
	if err := manager.enableICE(func(context.Context, Target) (*QUICEndpoint, net.Addr, error) {
		attempts.Add(1)
		return nil, nil, ErrNoRoute
	}); err != nil {
		t.Fatal(err)
	}
	pool := &quicPool{manager: manager, target: target}
	_, _, _ = pool.connection(context.Background())
	if attempts.Load() != 1 {
		t.Fatal("initial probe", attempts.Load())
	}
	if err := manager.SetCandidates(target, []p.NetworkCandidate{{Transport: "tcp", Address: "192.168.10.2:4567", Scope: "lan", Interface: "test"}}, 1, time.Now().Add(time.Minute), false); err != nil {
		t.Fatal(err)
	}
	_, _, _ = pool.connection(context.Background())
	if attempts.Load() != 2 {
		t.Fatal("fresh candidate arrival retained empty-probe cooldown", attempts.Load())
	}
}

// Slow service operations must leave when the manager closes, including during
// candidate lookup before either authenticated transport race can start.
func TestWANW11SlowLookupAndRelayShutdownJoin(t *testing.T) {
	for _, phase := range []string{"lookup", "relay"} {
		t.Run(phase, func(t *testing.T) {
			_, trust, target := managerServer(t, func(http.ResponseWriter, *http.Request) { t.Error("unselected route received HTTP") })
			target.Profile[0] = 1
			m := NewManager(ManagerOptions{})
			defer m.Close()
			entered := make(chan struct{}, 1)
			wait := func(ctx context.Context) error {
				select {
				case entered <- struct{}{}:
				default:
				}
				<-ctx.Done()
				return ctx.Err()
			}
			if err := m.SetRelay(target, func(ctx context.Context, _ Target) (net.Conn, error) { return nil, wait(ctx) }); err != nil {
				t.Fatal(err)
			}
			var source candidateSource
			if phase == "lookup" {
				source = func(ctx context.Context, _ Target, _ uint64) (p.NetworkAnnouncement, error) {
					return p.NetworkAnnouncement{}, wait(ctx)
				}
			}
			if err := m.RegisterDirect(target, source); err != nil {
				t.Fatal(err)
			}
			if err := m.enableICE(func(context.Context, Target) (*QUICEndpoint, net.Addr, error) { return nil, nil, ErrNoRoute }); err != nil {
				t.Fatal(err)
			}
			rt, err := m.Transport(context.Background(), target, trust)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				req, _ := http.NewRequest("GET", LogicalOrigin(target), nil)
				_, err := rt.RoundTrip(req)
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("slow operation did not start")
			}
			closed := make(chan error, 1)
			go func() { closed <- m.Close() }()
			select {
			case err := <-closed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("shutdown did not join slow service")
			}
			if err := <-done; err == nil {
				t.Fatal("failed route reported success")
			}
			m.mu.Lock()
			remaining := m.activeDials + len(m.requests) + len(m.pools)
			m.mu.Unlock()
			if remaining != 0 {
				t.Fatal("shutdown retained work", remaining)
			}
		})
	}
}
