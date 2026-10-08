package network

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"

	"github.com/calebhabesh/orbit/internal/protocol"
	"sync"
	"time"
)

var (
	ErrClosed       = errors.New("NETWORK_CLOSED")
	ErrBackpressure = errors.New("NETWORK_BUSY")
	ErrStale        = errors.New("STALE_NETWORK_GENERATION")
	ErrNoRoute      = errors.New("NO_ROUTE")
)

// ManagerOptions supplies deterministic clocks and socket adapters. No discovery
// or global service is enabled by constructing a manager.
type ManagerOptions struct {
	Generation  uint64
	Now         func() time.Time
	DialContext func(context.Context, string, string) (net.Conn, error)
}

type manualRoute struct {
	origin, address string
	revision        uint64
	stream          StreamDialer
}
type poolKey struct {
	target               Target
	revision, generation uint64
}
type managedPool struct {
	manager    *ConnectionManager
	key        poolKey
	origin     string
	transport  *Transport
	active     int
	probeAfter time.Time
	used       uint64
}
type requestWork struct {
	cancel context.CancelFunc
	body   *managedBody
}

// ConnectionManager retains explicit routes independently from its bounded LRU
// pools. Busy peers cannot evict another peer's active work. Overflow is typed
// backpressure for the scheduler's existing bounded retries, not a new queue.
type ConnectionManager struct {
	timing               protocol.RouteTiming
	candidateRevisions   map[Target]uint64
	directCooldowns      map[Target]directCooldown
	iceFailures          map[Target]error
	quicSlots            chan struct{}
	lifetime             context.Context
	cancel               context.CancelFunc
	dialWG               sync.WaitGroup
	tlsDialWG            sync.WaitGroup
	activeDials          int
	peerDials            map[Target]int
	mu                   sync.Mutex
	connMu               sync.Mutex
	now                  func() time.Time
	dial                 func(context.Context, string, string) (net.Conn, error)
	generation, sequence uint64
	closed               bool
	routes               map[Target]manualRoute
	lan                  map[lanKey]candidateLease
	public               map[Target]candidateLease
	sources              map[Target]candidateSource
	pools                map[poolKey]*managedPool
	observations         map[Target]Observation
	requests             map[*requestWork]bool
	connections          map[*ownedConn]bool
	relaySlots           map[Target]chan struct{}
	wg                   sync.WaitGroup
	done                 chan struct{}
	iceProvider          func(context.Context, Target) (*QUICEndpoint, net.Addr, error)
	quicEndpoint         *QUICEndpoint
	dataListener         *StreamListener
	enrollmentListener   *StreamListener
}

func NewManager(opts ManagerOptions) *ConnectionManager {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.DialContext == nil {
		opts.DialContext = (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	if opts.Generation == 0 {
		opts.Generation = 1
	}
	lifetime, cancel := context.WithCancel(context.Background())
	return &ConnectionManager{timing: (protocol.RouteTiming{}).Effective(), candidateRevisions: map[Target]uint64{}, directCooldowns: map[Target]directCooldown{}, iceFailures: map[Target]error{}, quicSlots: make(chan struct{}, MaxActivePeerSlots), lifetime: lifetime, cancel: cancel, now: opts.Now, dial: opts.DialContext, generation: opts.Generation,
		lan: map[lanKey]candidateLease{}, public: map[Target]candidateLease{}, sources: map[Target]candidateSource{}, peerDials: map[Target]int{}, routes: map[Target]manualRoute{}, pools: map[poolKey]*managedPool{}, observations: map[Target]Observation{},
		requests: map[*requestWork]bool{}, connections: map[*ownedConn]bool{}, relaySlots: map[Target]chan struct{}{}, done: make(chan struct{}), dataListener: NewStreamListener(), enrollmentListener: NewStreamListener()}
}

// SetManual imports reviewed v1 intent; the endpoint never supplies trust. It
// accepts private/loopback routes for legacy/manual use, unlike public discovery.
// New pins for a known device/purpose cannot overwrite that routing identity.
func (m *ConnectionManager) SetManual(target Target, endpoint string) error {
	if err := target.Validate(); err != nil {
		return err
	}
	if target.Profile != ([32]byte{}) {
		return errors.New("PROFILE_MISMATCH")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("INVALID_CANDIDATE")
	}
	address := u.Host
	if u.Port() == "" {
		address = net.JoinHostPort(u.Hostname(), "443")
	}
	if _, _, err := net.SplitHostPort(address); err != nil || u.Hostname() == "" {
		return errors.New("INVALID_CANDIDATE")
	}
	origin := "https://" + u.Host
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	for known := range m.routes {
		if known.Device == target.Device && known.Purpose == target.Purpose && known.Pin != target.Pin {
			return errors.New("IDENTITY_MISMATCH")
		}
	}
	old, exists := m.routes[target]
	if exists && old.origin == origin {
		return nil
	}
	// v1 has 64 folder/device entries; separate enrollment routes need another 64.
	if !exists && len(m.routes) >= 128 {
		return ErrBackpressure
	}
	m.sequence++
	m.routes[target] = manualRoute{origin: origin, address: address, revision: m.sequence}
	delete(m.observations, target)
	m.pruneLocked()
	return nil
}

// AdvanceGeneration invalidates new-route authority. Finite in-flight requests
// may drain; no old completion can publish an observation in the new generation.
// W03/W08/W11 add authenticated candidate leases and actual network-change input.
func (m *ConnectionManager) AdvanceGeneration() uint64 { return m.advanceGeneration(false) }

// NetworkChanged also invalidates address leases tied to the previous interfaces.
func (m *ConnectionManager) NetworkChanged() uint64 { return m.advanceGeneration(true) }

func (m *ConnectionManager) advanceGeneration(dropCandidates bool) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.generation++
		m.observations = map[Target]Observation{}
		m.iceFailures = map[Target]error{}
		if dropCandidates {
			m.directCooldowns = map[Target]directCooldown{}
			for key, lease := range m.lan {
				lease.invalidated = true
				m.lan[key] = lease
			}
			for key, lease := range m.public {
				lease.invalidated = true
				m.public[key] = lease
			}
		}
		m.pruneLocked()
	}
	return m.generation
}
func (m *ConnectionManager) currentLocked(p *managedPool) bool {
	r, ok := m.routes[p.key.target]
	return ok && p.key.generation == m.generation && r.revision == p.key.revision
}
func (m *ConnectionManager) pruneLocked() {
	for k, p := range m.pools {
		if p.active == 0 && !m.currentLocked(p) {
			p.transport.CloseIdleConnections()
			delete(m.pools, k)
		}
	}
}
func (m *ConnectionManager) Transport(ctx context.Context, target Target, trust *tls.Config) (http.RoundTripper, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := target.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	r, ok := m.routes[target]
	if !ok {
		return nil, ErrNoRoute
	}
	key := poolKey{target, r.revision, m.generation}
	// Validate supplied trust even when a pool already exists. All daemon requests
	// use the same persistent local identity; never accept a permissive caller.
	tr, err := NewTransport(target, trust, func(ctx context.Context, _ Target) (net.Conn, error) {
		return m.connect(ctx, target, r.address, r.stream)
	})
	if err != nil {
		return nil, err
	}
	if _, enabled := m.sources[target]; enabled {
		tr.routeManager, tr.routeTarget, tr.route, tr.routeTrust = m, target, r, tr.transport.TLSClientConfig
		tr.transport.DialTLSContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			if ready, ok := ctx.Value(preparedTLSKey{}).(*preparedTLS); ok {
				if c := ready.take(); c != nil {
					return c, nil
				}
			}
			return m.dialTLS(ctx, target, r, tr.transport.TLSClientConfig)
		}
	}
	if (m.quicEndpoint != nil || m.iceProvider != nil) && target.Purpose == PeerData {
		cfg := tr.transport.TLSClientConfig.Clone()
		cfg.NextProtos = []string{"h3"}
		cfg.ClientSessionCache = nil
		tr.quic = &quicPool{endpoint: m.quicEndpoint, manager: m, target: target, generation: m.generation, trust: cfg}
	}
	if p := m.pools[key]; p != nil && p.active == 0 && m.observations[target].Route == "relay" && !m.now().Before(p.probeAfter) {
		// Retire only idle pools: streamed requests keep their existing deadline.
		// The next borrower gets fresh direct probes without changing peer trust.
		p.transport.CloseIdleConnections()
		delete(m.pools, key)
	}
	if p := m.pools[key]; p != nil {
		saved := p.transport.transport.TLSClientConfig
		// A reusable pool cannot silently borrow another caller's client identity or
		// roots. Certificate renewal must drain/rebuild explicitly.
		if !saved.RootCAs.Equal(trust.RootCAs) || saved.ServerName != trust.ServerName || saved.MinVersion != trust.MinVersion || saved.MaxVersion != trust.MaxVersion || !sameCertificates(saved.Certificates, trust.Certificates) {
			return nil, errors.New("TLS_TRUST_CHANGED")
		}
		return &borrowedPool{pool: p, verify: trust.VerifyConnection}, nil
	}
	m.pruneLocked()
	n := 0
	for k := range m.pools {
		if k.target == target {
			n++
		}
	}
	if n >= MaxPoolsPerTarget {
		return nil, ErrBackpressure
	}
	if len(m.pools) >= MaxActivePeerSlots {
		var oldest *managedPool
		for _, p := range m.pools {
			if p.active == 0 && (oldest == nil || p.used < oldest.used) {
				oldest = p
			}
		}
		if oldest == nil {
			return nil, ErrBackpressure
		}
		oldest.transport.CloseIdleConnections()
		delete(m.pools, oldest.key)
	}
	m.sequence++
	p := &managedPool{manager: m, key: key, origin: r.origin, transport: tr, probeAfter: m.now().Add(time.Duration(m.timing.ProbeMS) * time.Millisecond), used: m.sequence}
	m.pools[key] = p
	return &borrowedPool{pool: p, verify: trust.VerifyConnection}, nil
}
func (m *ConnectionManager) Observe(target Target) Observation {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o, ok := m.observations[target]; ok {
		return o
	}
	code := "NOT_TESTED"
	if m.closed {
		code = "NETWORK_CLOSED"
	}
	return Observation{Target: target, Route: "not_tested", Code: code, Generation: m.generation}
}

type ownedConn struct {
	net.Conn
	manager *ConnectionManager
	target  Target
	once    sync.Once
	route   string
	release func()
}

func (c *ownedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() {
		c.manager.connMu.Lock()
		delete(c.manager.connections, c)
		c.manager.connMu.Unlock()
		if c.release != nil {
			c.release()
		}
	})
	return err
}

// relayProbeKey marks an explicit diagnostic relay handshake: it closes at
// once and must not wait behind the pool's tunnel; pair limits still apply.
type relayProbeKey struct{}

// relaySlot is this device's single initiator relay tunnel toward a target.
// The client and service allow two tunnels per device pair, shared by both
// directions; holding one per direction leaves the peer's direction a slot.
// A concurrent request waits for the open tunnel (net/http hands it over when
// idle) rather than dialing another and spending service budget.
func (m *ConnectionManager) relaySlot(ctx context.Context, target Target) (func(), error) {
	m.mu.Lock()
	slot := m.relaySlots[target]
	if slot == nil {
		slot = make(chan struct{}, 1)
		m.relaySlots[target] = slot
	}
	m.mu.Unlock()
	select {
	case slot <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-slot }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.lifetime.Done():
		return nil, ErrClosed
	}
}
func (m *ConnectionManager) connect(ctx context.Context, target Target, address string, streams ...StreamDialer) (net.Conn, error) {
	var release func()
	if len(streams) > 0 && streams[0] != nil && ctx.Value(relayProbeKey{}) == nil {
		var err error
		if release, err = m.relaySlot(ctx, target); err != nil {
			return nil, err
		}
	}
	keep := false
	defer func() {
		if release != nil && !keep {
			release()
		}
	}()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrClosed
	}
	if m.activeDials >= MaxActivePeerSlots || m.peerDials[target] >= MaxDirectAttempts {
		m.mu.Unlock()
		return nil, ErrBackpressure
	}
	m.activeDials++
	m.peerDials[target]++
	m.dialWG.Add(1)
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.activeDials--
		m.peerDials[target]--
		if m.peerDials[target] == 0 {
			delete(m.peerDials, target)
		}
		m.mu.Unlock()
		m.dialWG.Done()
	}()
	dialTimeout := 3 * time.Second
	if len(streams) > 0 && streams[0] != nil {
		dialTimeout = time.Duration(m.timing.CycleMS) * time.Millisecond
	}
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	stop := context.AfterFunc(m.lifetime, cancel)
	defer func() { stop(); cancel() }()
	var c net.Conn
	var err error
	if len(streams) > 0 && streams[0] != nil {
		c, err = streams[0](dialCtx, target)
	} else {
		c, err = m.dial(dialCtx, "tcp", address)
	}
	if err != nil {
		return nil, err
	}
	route := "direct"
	if len(streams) > 0 && streams[0] != nil {
		route = "relay"
	}
	owned := &ownedConn{Conn: c, manager: m, target: target, route: route, release: release}
	m.mu.Lock()
	if m.closed || ctx.Err() != nil {
		m.mu.Unlock()
		c.Close()
		return nil, ErrClosed
	}
	m.connMu.Lock()
	peerConnections := 0
	for connection := range m.connections {
		if connection.target == target {
			peerConnections++
		}
	}
	if len(m.connections) >= MaxActivePeerSlots*MaxDirectAttempts || peerConnections >= MaxPoolsPerTarget*MaxDirectAttempts {
		m.connMu.Unlock()
		m.mu.Unlock()
		c.Close()
		return nil, ErrBackpressure
	}
	m.connections[owned] = true
	m.connMu.Unlock()
	m.mu.Unlock()
	keep = true
	return owned, nil
}

type borrowedPool struct {
	pool   *managedPool
	verify func(tls.ConnectionState) error
}

func (b *borrowedPool) RoundTrip(r *http.Request) (*http.Response, error) {
	return b.pool.roundTrip(r, b.verify)
}
func (b *borrowedPool) CloseIdleConnections() {}
func (p *managedPool) roundTrip(r *http.Request, verify func(tls.ConnectionState) error) (*http.Response, error) {
	if r == nil || r.URL == nil || r.URL.Scheme != "https" || r.URL.User != nil || "https://"+r.URL.Host != p.origin {
		return nil, errors.New("ROUTE_SCOPE_MISMATCH")
	}
	m := p.manager
	timeout := 30 * time.Second
	if p.key.target.Purpose == Enrollment {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	w := &requestWork{cancel: cancel}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		cancel()
		return nil, ErrClosed
	}
	if !m.currentLocked(p) || m.pools[p.key] != p {
		m.mu.Unlock()
		cancel()
		return nil, ErrStale
	}
	peerActive := 0
	for _, pool := range m.pools {
		if pool.key.target == p.key.target {
			peerActive += pool.active
		}
	}
	if len(m.requests) >= MaxActivePeerSlots || peerActive >= MaxRequestsPerTarget {
		m.mu.Unlock()
		cancel()
		return nil, ErrBackpressure
	}
	m.requests[w] = true
	p.active++
	m.sequence++
	p.used = m.sequence
	m.wg.Add(1)
	m.mu.Unlock()
	defer m.wg.Done()
	// Reusing a socket must retain this borrower's verifier as well as the
	// original handshake policy. GotConn runs before net/http writes the request;
	// rejection closes/cancels that connection before any HTTP secret is sent.
	var verifyMu sync.Mutex
	var verifyErr error
	actualRoute := "direct"
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		conn, ok := info.Conn.(*tls.Conn)
		var err error
		if !ok {
			err = errors.New("TLS_REQUIRED")
		} else {
			err = verify(conn.ConnectionState())
			if owned, ok := conn.NetConn().(*ownedConn); ok {
				actualRoute = owned.route
				if retiring, ok := owned.Conn.(interface{ Retired() bool }); ok && retiring.Retired() {
					err = ErrStale
				}
			}
		}
		if err != nil {
			verifyMu.Lock()
			verifyErr = err
			verifyMu.Unlock()
			cancel()
			info.Conn.Close()
		}
	}})
	ctx = context.WithValue(ctx, requestVerifierKey{}, verify)
	response, err := p.transport.RoundTrip(r.Clone(ctx))
	if response != nil && response.TLS != nil && response.TLS.NegotiatedProtocol == "h3" {
		actualRoute = "quic"
	}
	verifyMu.Lock()
	if verifyErr != nil {
		err = verifyErr
	}
	verifyMu.Unlock()
	m.mu.Lock()
	if !m.closed && m.currentLocked(p) {
		route, code := actualRoute, "CONNECTED"
		if err != nil {
			route, code = "unavailable", "CONNECTION_FAILED"
			var service *ServiceError
			if errors.As(err, &service) {
				code = service.Code
			}
			if ctx.Err() != nil {
				code = "CANCELLED"
			} else if errors.Is(err, ErrBackpressure) {
				code = "QUOTA_EXCEEDED"
			}
		}
		m.observations[p.key.target] = Observation{p.key.target, route, code, m.now(), m.generation}
		if err == nil && route == "relay" {
			if cooldown := m.directCooldowns[p.key.target]; cooldown.next.After(p.probeAfter) {
				p.probeAfter = cooldown.next
			}
		}
	}
	if err != nil {
		m.releaseLocked(p, w)
		m.mu.Unlock()
		cancel()
		return nil, err
	}
	body := &managedBody{ReadCloser: response.Body, manager: m, pool: p, work: w}
	w.body = body
	response.Body = body
	closed := m.closed
	m.mu.Unlock()
	if closed {
		body.Close()
		return nil, ErrClosed
	}
	return response, nil
}
func (m *ConnectionManager) releaseLocked(p *managedPool, w *requestWork) {
	if m.requests[w] {
		delete(m.requests, w)
		p.active--
		m.pruneLocked()
	}
}

type managedBody struct {
	io.ReadCloser
	manager *ConnectionManager
	pool    *managedPool
	work    *requestWork
	once    sync.Once
}

func (b *managedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.Close()
	}
	return n, err
}
func (b *managedBody) Close() error {
	var err error
	b.once.Do(func() {
		err = b.ReadCloser.Close()
		b.work.cancel()
		b.manager.mu.Lock()
		b.manager.releaseLocked(b.pool, b.work)
		b.manager.mu.Unlock()
	})
	return err
}

func (m *ConnectionManager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		<-m.done
		return nil
	}
	m.closed = true
	m.observations = map[Target]Observation{}
	m.iceFailures = map[Target]error{}
	m.cancel()
	_ = m.dataListener.Close()
	_ = m.enrollmentListener.Close()
	for w := range m.requests {
		w.cancel()
	}
	m.connMu.Lock()
	conns := make([]*ownedConn, 0, len(m.connections))
	for c := range m.connections {
		conns = append(conns, c)
	}
	m.connMu.Unlock()
	m.mu.Unlock()
	for _, c := range conns {
		c.Close()
	}
	if m.quicEndpoint != nil {
		_ = m.quicEndpoint.Close()
	}
	m.wg.Wait()
	m.dialWG.Wait()
	m.tlsDialWG.Wait()
	m.mu.Lock()
	bodies := make([]*managedBody, 0, len(m.requests))
	for w := range m.requests {
		if w.body != nil {
			bodies = append(bodies, w.body)
		}
	}
	m.mu.Unlock()
	for _, b := range bodies {
		b.Close()
	}
	m.mu.Lock()
	for _, p := range m.pools {
		p.transport.CloseIdleConnections()
	}
	m.pools = map[poolKey]*managedPool{}
	m.mu.Unlock()
	close(m.done)
	return nil
}

func sameCertificates(a, b []tls.Certificate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i].Certificate) != len(b[i].Certificate) {
			return false
		}
		for j := range a[i].Certificate {
			if !bytes.Equal(a[i].Certificate[j], b[i].Certificate[j]) {
				return false
			}
		}
	}
	return true
}

// SetRelay installs an explicitly reviewed logical route. No service is selected
// or contacted by this method. Pins are still supplied by replication.
func (m *ConnectionManager) SetRelay(target Target, dial StreamDialer) error {
	if target.Validate() != nil || target.Profile == ([32]byte{}) || dial == nil {
		return errors.New("INVALID_RELAY_ROUTE")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	for known := range m.routes {
		if known.Device == target.Device && known.Purpose == target.Purpose && known.Pin != target.Pin {
			return errors.New("IDENTITY_MISMATCH")
		}
	}
	if _, ok := m.routes[target]; !ok && len(m.routes) >= 128 {
		return ErrBackpressure
	}
	m.sequence++
	m.routes[target] = manualRoute{origin: LogicalOrigin(target), revision: m.sequence, stream: dial}
	delete(m.observations, target)
	m.pruneLocked()
	return nil
}

// IncomingListener has no owner-control purpose and keeps enrollment separate.
func (m *ConnectionManager) IncomingListener(purpose Purpose) (*StreamListener, error) {
	switch purpose {
	case PeerData:
		return m.dataListener, nil
	case Enrollment:
		return m.enrollmentListener, nil
	default:
		return nil, errors.New("PURPOSE_MISMATCH")
	}
}

// EnableQUIC attaches one optional daemon-owned packet transport before requests.
// Existing pools are drained under a new generation; enrollment stays isolated.
func (m *ConnectionManager) EnableQUIC(e *QUICEndpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if e == nil || m.quicEndpoint != nil {
		return errors.New("INVALID_QUIC_ENDPOINT")
	}
	m.quicEndpoint = e
	m.generation++
	m.pruneLocked()
	return nil
}

func (m *ConnectionManager) enableICE(provider func(context.Context, Target) (*QUICEndpoint, net.Addr, error)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if provider == nil || m.iceProvider != nil {
		return ErrNoRoute
	}
	m.iceProvider = provider
	m.generation++
	m.pruneLocked()
	return nil
}

// NewPeerQUICEndpoint shares incoming admission across native and ICE transports.
func (m *ConnectionManager) NewPeerQUICEndpoint(socket net.PacketConn, trust *tls.Config, peer http.Handler) (*QUICEndpoint, error) {
	return newQUICEndpoint(socket, trust, peer, m.quicSlots)
}

// ICEFailure reports the last bounded establishment failure without guessing a
// NAT class. W12 owns its presentation; a working fallback keeps its own route.
func (m *ConnectionManager) ICEFailure(t Target) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.iceFailures[t]
}
func (m *ConnectionManager) recordICEFailure(t Target, err error, generation ...uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed && (len(generation) == 0 || generation[0] == m.generation) {
		if err == nil {
			delete(m.iceFailures, t)
		} else if _, ok := m.routes[t]; ok {
			m.iceFailures[t] = err
		}
	}
}

// ConfigureTiming is allowed only before registration or request work starts.
// Reviewed changes activate by daemon restart, keeping each generation immutable.
func (m *ConnectionManager) ConfigureTiming(t protocol.RouteTiming) error {
	if err := t.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if len(m.routes) != 0 || len(m.requests) != 0 {
		return ErrBackpressure
	}
	m.timing = t.Effective()
	return nil
}
