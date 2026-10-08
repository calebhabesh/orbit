package network

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/model"
)

func managerServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *tls.Config, Target) {
	t.Helper()
	s := httptest.NewUnstartedServer(handler)
	s.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	s.StartTLS()
	t.Cleanup(s.Close)
	cert := s.Certificate()
	pin := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	trust := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "example.com", VerifyConnection: func(cs tls.ConnectionState) error {
		if sha256.Sum256(cs.PeerCertificates[0].RawSubjectPublicKeyInfo) != pin {
			return errors.New("wrong pin")
		}
		return nil
	}}
	return s, trust, Target{Device: history.ID{1}, Pin: pin, Purpose: PeerData}
}
func round(t *testing.T, rt http.RoundTripper, endpoint string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", endpoint, nil)
	res, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
func TestWANW02ManagerReuseScopeIdentityAndShutdown(t *testing.T) {
	var dials, handled atomic.Int32
	s, trust, target := managerServer(t, func(w http.ResponseWriter, r *http.Request) { handled.Add(1); w.Write([]byte("synthetic")) })
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	m := NewManager(ManagerOptions{Now: func() time.Time { return now }, DialContext: func(ctx context.Context, n, a string) (net.Conn, error) {
		dials.Add(1)
		return (&net.Dialer{}).DialContext(ctx, n, a)
	}})
	defer m.Close()
	if err := m.SetManual(target, s.URL); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		rt, err := m.Transport(context.Background(), target, trust)
		if err != nil {
			t.Fatal(err)
		}
		res := round(t, rt, s.URL)
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}
	if dials.Load() != 1 {
		t.Fatalf("pool not reused: %d dials", dials.Load())
	}
	o := m.Observe(target)
	if o.Route != "direct" || o.ObservedAt != now || o.Generation != 1 {
		t.Fatal(o)
	}
	rt, _ := m.Transport(context.Background(), target, trust)
	for _, u := range []string{"http://example.com", "https://example.com:1", "https://user@example.com"} {
		req, _ := http.NewRequest("GET", u, nil)
		if _, err := rt.RoundTrip(req); err == nil {
			t.Fatal("request chose a destination")
		}
	}
	wrong := target
	wrong.Pin[0] ^= 1
	if m.SetManual(wrong, s.URL) == nil {
		t.Fatal("competing pin overwrote route")
	}
	permissive := trust.Clone()
	permissive.InsecureSkipVerify = true
	if _, err := m.Transport(context.Background(), target, permissive); err == nil {
		t.Fatal("cached pool bypassed trust validation")
	}
	different := trust.Clone()
	different.RootCAs = x509.NewCertPool()
	if _, err := m.Transport(context.Background(), target, different); err == nil {
		t.Fatal("cached pool changed roots")
	}
	denied := errors.New("borrower rejected cached peer")
	stricter := trust.Clone()
	stricter.VerifyConnection = func(tls.ConnectionState) error { return denied }
	restricted, err := m.Transport(context.Background(), target, stricter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restricted.RoundTrip(mustRequest(s.URL)); !errors.Is(err, denied) {
		t.Fatal("cached pool bypassed borrower verifier", err)
	}
	if handled.Load() != 4 {
		t.Fatal("rejected borrower disclosed HTTP request", handled.Load())
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.RoundTrip(mustRequest(s.URL)); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connMu.Lock()
	defer m.connMu.Unlock()
	if len(m.requests) != 0 || len(m.connections) != 0 || len(m.pools) != 0 {
		t.Fatal("shutdown retained work")
	}
}
func mustRequest(u string) *http.Request { r, _ := http.NewRequest("GET", u, nil); return r }
func TestWANW02GenerationMatchesIndependentModel(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	s, trust, target := managerServer(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
			w.Write([]byte("old"))
		case <-r.Context().Done():
		}
	})
	m := NewManager(ManagerOptions{})
	defer m.Close()
	m.SetManual(target, s.URL)
	oracle := model.NewWANConnection()
	oracle.Start("manual", "direct")
	rt, _ := m.Transport(context.Background(), target, trust)
	done := make(chan error, 1)
	go func() {
		res, err := rt.RoundTrip(mustRequest(s.URL))
		if err == nil {
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
		}
		done <- err
	}()
	<-entered
	oracle.Change()
	generation := m.AdvanceGeneration()
	close(release)
	if err := <-done; err != nil {
		t.Fatal("inflight did not drain", err)
	}
	if oracle.Complete("manual", generation-1, true) || m.Observe(target).Route != "not_tested" {
		t.Fatal("old completion published route")
	}
	if _, err := rt.RoundTrip(mustRequest(s.URL)); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	for i := 0; i < 10000; i++ {
		oracle.Change()
		if m.AdvanceGeneration() != oracle.Generation {
			t.Fatal("generation diverged")
		}
	}
	m.mu.Lock()
	if len(m.pools) != 0 || len(m.observations) != 0 {
		t.Fatal("flapping retained stale pools")
	}
	m.mu.Unlock()
	oracle.Cancel()
	m.Close()
	if _, err := m.Transport(context.Background(), target, trust); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
func TestWANW02SlowPeerBoundsCancellationAndOtherProgress(t *testing.T) {
	entered := make(chan struct{}, MaxRequestsPerTarget)
	slow, trust, target := managerServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		entered <- struct{}{}
		<-r.Context().Done()
	})
	m := NewManager(ManagerOptions{})
	defer m.Close()
	m.SetManual(target, slow.URL)
	rt, _ := m.Transport(context.Background(), target, trust)
	a := round(t, rt, slow.URL)
	<-entered
	b := round(t, rt, slow.URL)
	<-entered
	// Six additional bounded callers wait behind the two live HTTP sockets.
	pending := make(chan error, MaxRequestsPerTarget-2)
	for i := 2; i < MaxRequestsPerTarget; i++ {
		go func() {
			res, err := rt.RoundTrip(mustRequest(slow.URL))
			if err == nil {
				res.Body.Close()
			}
			pending <- err
		}()
	}
	deadline := time.Now().Add(time.Second)
	for {
		m.mu.Lock()
		active := len(m.requests)
		m.mu.Unlock()
		if active == MaxRequestsPerTarget {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("queued requests not admitted")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := rt.RoundTrip(mustRequest(slow.URL)); !errors.Is(err, ErrBackpressure) {
		t.Fatal("per-peer bound", err)
	}
	fast, fastTrust, other := managerServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("progress")) })
	other.Device[0] = 2
	m.SetManual(other, fast.URL)
	fastRT, err := m.Transport(context.Background(), other, fastTrust)
	if err != nil {
		t.Fatal(err)
	}
	res := round(t, fastRT, fast.URL)
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(data) != "progress" {
		t.Fatal("slow peer blocked other peer")
	}
	before := time.Now()
	m.Close()
	if time.Since(before) > time.Second {
		t.Fatal("shutdown exceeded bound")
	}
	if _, err := a.Body.Read(make([]byte, 1)); err == nil {
		t.Fatal("abandoned body survived shutdown")
	}
	b.Body.Close()
	for i := 2; i < MaxRequestsPerTarget; i++ {
		if <-pending == nil {
			t.Fatal("queued request survived shutdown")
		}
	}
}
func TestWANW02PoolLRUAndManualCandidateBounds(t *testing.T) {
	s, trust, target := managerServer(t, func(w http.ResponseWriter, r *http.Request) {})
	m := NewManager(ManagerOptions{})
	defer m.Close()
	for _, u := range []string{"http://example.com", "https://example.com/path", "https://example.com?x=1", "https://example.com#x", "https://user@example.com"} {
		if m.SetManual(target, u) == nil {
			t.Fatal("invalid origin", u)
		}
	}
	var first http.RoundTripper
	for i := 0; i < 128; i++ {
		candidate := target
		candidate.Device = history.ID{byte(i + 1)}
		if err := m.SetManual(candidate, s.URL); err != nil {
			t.Fatal(err)
		}
		rt, err := m.Transport(context.Background(), candidate, trust)
		if err != nil {
			t.Fatal(err)
		}
		res := round(t, rt, s.URL)
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		if i == 0 {
			first = rt
		}
	}
	extra := target
	extra.Device = history.ID{1, 1}
	if !errors.Is(m.SetManual(extra, s.URL), ErrBackpressure) {
		t.Fatal("unbounded candidate/observation state")
	}
	m.mu.Lock()
	n := len(m.pools)
	m.mu.Unlock()
	if n != MaxActivePeerSlots {
		t.Fatal(n)
	}
	if _, err := first.RoundTrip(mustRequest(s.URL)); !errors.Is(err, ErrStale) {
		t.Fatal("evicted pool usable", err)
	}
}
func TestWANW02ShutdownJoinsPendingDial(t *testing.T) {
	entered := make(chan struct{})
	exited := make(chan struct{})
	m := NewManager(ManagerOptions{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		close(entered)
		<-ctx.Done()
		close(exited)
		return nil, ctx.Err()
	}})
	defer m.Close()
	_, trust, target := managerServer(t, func(http.ResponseWriter, *http.Request) {})
	m.SetManual(target, "https://example.com")
	rt, _ := m.Transport(context.Background(), target, trust)
	done := make(chan error, 1)
	go func() { _, err := rt.RoundTrip(mustRequest("https://example.com")); done <- err }()
	<-entered
	m.Close()
	select {
	case <-exited:
	default:
		t.Fatal("dial not joined")
	}
	if <-done == nil {
		t.Fatal("pending request survived close")
	}
}

func TestWANW02PurposePoolsAndDrainingGenerationBound(t *testing.T) {
	s, trust, target := managerServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("bounded old response")) })
	m := NewManager(ManagerOptions{})
	defer m.Close()
	m.SetManual(target, s.URL)
	old, _ := m.Transport(context.Background(), target, trust)
	a := round(t, old, s.URL)
	m.AdvanceGeneration()
	current, err := m.Transport(context.Background(), target, trust)
	if err != nil {
		t.Fatal(err)
	}
	b := round(t, current, s.URL)
	m.AdvanceGeneration()
	if _, err := m.Transport(context.Background(), target, trust); !errors.Is(err, ErrBackpressure) {
		t.Fatal("more than two draining pools", err)
	}
	a.Body.Close()
	b.Body.Close()
	if _, err := m.Transport(context.Background(), target, trust); err != nil {
		t.Fatal("drain failed to release generation pool", err)
	}
	enrollment := target
	enrollment.Purpose = Enrollment
	m.SetManual(enrollment, s.URL)
	enrollmentRT, err := m.Transport(context.Background(), enrollment, trust)
	if err != nil {
		t.Fatal(err)
	}
	if enrollmentRT == current {
		t.Fatal("enrollment reused data pool")
	}
	if enrollmentRT.(*borrowedPool).pool.transport.transport.ResponseHeaderTimeout != 5*time.Second {
		t.Fatal("enrollment deadline weakened")
	}
	m.Close()
	if m.Observe(target).Code != "NETWORK_CLOSED" {
		t.Fatal("shutdown retained current route authority")
	}
	restarted := NewManager(ManagerOptions{})
	defer restarted.Close()
	restarted.SetManual(target, s.URL)
	rt, err := restarted.Transport(context.Background(), target, trust)
	if err != nil {
		t.Fatal(err)
	}
	res := round(t, rt, s.URL)
	res.Body.Close()
}

func TestWANW02DialAdmissionSurvivesGenerationFlapping(t *testing.T) {
	entered := make(chan struct{}, MaxActivePeerSlots)
	m := NewManager(ManagerOptions{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	defer m.Close()
	target := Target{Device: history.ID{1}, Pin: history.Digest{1}, Purpose: PeerData}
	done := make(chan error, MaxActivePeerSlots)
	for i := 0; i < MaxActivePeerSlots; i++ {
		peer := target
		peer.Device[0] = byte(i/MaxDirectAttempts + 1)
		go func() { _, err := m.connect(context.Background(), peer, "synthetic:443"); done <- err }()
		<-entered
	}
	for i := 0; i < 10000; i++ {
		m.AdvanceGeneration()
	}
	if _, err := m.connect(context.Background(), target, "synthetic:443"); !errors.Is(err, ErrBackpressure) {
		t.Fatal("cancelled generations exceeded dial admission", err)
	}
	other := target
	other.Device[0] = 128
	if _, err := m.connect(context.Background(), other, "synthetic:443"); !errors.Is(err, ErrBackpressure) {
		t.Fatal("global dial cap", err)
	}
	m.Close()
	for i := 0; i < MaxActivePeerSlots; i++ {
		if <-done == nil {
			t.Fatal("dial survived shutdown")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeDials != 0 || len(m.peerDials) != 0 {
		t.Fatal("dial admission retained after join")
	}
}

func TestWANW02OwnedSocketAdmissionAcrossPools(t *testing.T) {
	m := NewManager(ManagerOptions{DialContext: func(context.Context, string, string) (net.Conn, error) {
		a, b := net.Pipe()
		t.Cleanup(func() { b.Close() })
		return a, nil
	}})
	defer m.Close()
	target := Target{Device: history.ID{1}, Pin: history.Digest{1}, Purpose: PeerData}
	for i := 0; i < MaxActivePeerSlots*MaxDirectAttempts; i++ {
		peer := target
		peer.Device[0] = byte(i/(MaxPoolsPerTarget*MaxDirectAttempts) + 1)
		if _, err := m.connect(context.Background(), peer, "synthetic:443"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.connect(context.Background(), target, "synthetic:443"); !errors.Is(err, ErrBackpressure) {
		t.Fatal("socket cap after generation drain", err)
	}
	other := target
	other.Device[0] = 128
	if _, err := m.connect(context.Background(), other, "synthetic:443"); !errors.Is(err, ErrBackpressure) {
		t.Fatal("global socket cap", err)
	}
	m.Close()
	m.connMu.Lock()
	defer m.connMu.Unlock()
	if len(m.connections) != 0 {
		t.Fatal("socket admission retained after shutdown")
	}
}
