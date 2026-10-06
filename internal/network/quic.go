package network

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	p "github.com/calebhabesh/file-sync/internal/protocol"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

func peerQUICConfig() *quic.Config {
	return &quic.Config{Allow0RTT: false, EnableDatagrams: false, HandshakeIdleTimeout: 5 * time.Second, MaxIdleTimeout: 30 * time.Second,
		MaxIncomingStreams: 2, MaxIncomingUniStreams: 4, InitialStreamReceiveWindow: 64 << 10, MaxStreamReceiveWindow: 256 << 10,
		InitialConnectionReceiveWindow: 128 << 10, MaxConnectionReceiveWindow: 512 << 10, InitialPacketSize: QUICPacketMTU, DisablePathMTUDiscovery: true}
}

// QUICEndpoint owns the sole reader of its packet socket and supports independent
// inbound and outbound HTTP/3 connections. The supplied handler is peer data only.
type QUICEndpoint struct {
	transport *quic.Transport
	listener  *quic.Listener
	socket    net.PacketConn
	server    *http3.Server
	cancel    context.CancelFunc
	done      chan struct{}
	once      sync.Once
	slots     chan struct{}
}

func NewQUICEndpoint(socket net.PacketConn, trust *tls.Config, peer http.Handler) (*QUICEndpoint, error) {
	return newQUICEndpoint(socket, trust, peer, nil)
}
func newQUICEndpoint(socket net.PacketConn, trust *tls.Config, peer http.Handler, admission chan struct{}) (*QUICEndpoint, error) {
	if socket == nil || trust == nil || trust.MinVersion < tls.VersionTLS13 || (trust.ClientAuth != tls.RequireAnyClientCert && trust.ClientAuth != tls.RequireAndVerifyClientCert) || len(trust.Certificates) == 0 || peer == nil {
		return nil, errors.New("INVALID_QUIC_SERVER")
	}
	ctx, cancel := context.WithCancel(context.Background())
	if admission == nil {
		admission = make(chan struct{}, MaxActivePeerSlots)
	}
	e := &QUICEndpoint{socket: socket, cancel: cancel, done: make(chan struct{}), slots: admission}
	e.transport = &quic.Transport{Conn: socket, VerifySourceAddress: func(net.Addr) bool { return true }, ConnContext: func(ctx context.Context, _ *quic.ClientInfo) (context.Context, error) {
		select {
		case e.slots <- struct{}{}:
			context.AfterFunc(ctx, func() { <-e.slots })
			return ctx, nil
		default:
			return nil, ErrBackpressure
		}
	}}
	cfg := trust.Clone()
	cfg.NextProtos = []string{http3.NextProtoH3}
	cfg.SessionTicketsDisabled = true
	e.server = &http3.Server{MaxHeaderBytes: 32 << 10, IdleTimeout: 30 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controller := http.NewResponseController(w)
		// Headers were bounded before parsing. Reset the read deadline for the body;
		// request cancellation and write deadline cover the entire handler/response.
		_ = controller.SetReadDeadline(time.Now().Add(15 * time.Second))
		_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
		requestCtx, stop := context.WithTimeout(r.Context(), 30*time.Second)
		defer stop()
		peer.ServeHTTP(w, r.WithContext(requestCtx))
	})}
	listener, err := e.transport.Listen(cfg, peerQUICConfig())
	if err != nil {
		cancel()
		_ = e.transport.Close()
		return nil, err
	}
	e.listener = listener
	go func() {
		defer close(e.done)
		var connections sync.WaitGroup
		defer connections.Wait()
		for {
			c, err := listener.Accept(ctx)
			if err != nil {
				return
			}
			connections.Go(func() { e.serve(ctx, c) })
		}
	}()
	return e, nil
}
func (e *QUICEndpoint) serve(ctx context.Context, c *quic.Conn) {
	raw, err := e.server.NewRawServerConn(c)
	if err != nil {
		_ = c.CloseWithError(0, "")
		return
	}
	defer raw.CloseWithError(0, "")
	var workers sync.WaitGroup
	workers.Go(func() {
		var uni sync.WaitGroup
		defer uni.Wait()
		for {
			stream, err := c.AcceptUniStream(ctx)
			if err != nil {
				return
			}
			uni.Go(func() { raw.HandleUnidirectionalStream(stream) })
		}
	})
	for {
		stream, err := c.AcceptStream(ctx)
		if err != nil {
			break
		}
		_ = stream.SetReadDeadline(time.Now().Add(5 * time.Second))
		_ = stream.SetWriteDeadline(time.Now().Add(30 * time.Second))
		workers.Go(func() { raw.HandleRequestStream(stream) })
	}
	_ = raw.CloseWithError(0, "")
	workers.Wait()
}
func (e *QUICEndpoint) Done() <-chan struct{} { return e.done }
func (e *QUICEndpoint) LocalAddr() net.Addr   { return e.socket.LocalAddr() }
func (e *QUICEndpoint) Close() error {
	e.once.Do(func() { e.cancel(); _ = e.listener.Close(); _ = e.transport.Close(); _ = e.socket.Close(); <-e.done })
	return nil
}

// Only a complete authenticated handshake may choose QUIC. Failure before that
// point can select TCP/WSS. An HTTP request is never replayed by this adapter.
type quicPool struct {
	candidateRevision uint64
	generation        uint64
	nextProbe         time.Time
	mu                sync.Mutex
	endpoint          *QUICEndpoint
	manager           *ConnectionManager
	target            Target
	trust             *tls.Config
	conn              *quic.Conn
	client            *http3.ClientConn
}
type requestVerifierKey struct{}

func (p *quicPool) connection(ctx context.Context) (*quic.Conn, *http3.ClientConn, error) {
	return p.connectionCandidates(ctx, nil, false)
}
func (p *quicPool) connectionCandidates(ctx context.Context, candidates []p.NetworkCandidate, supplied bool) (*quic.Conn, *http3.ClientConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		if p.conn.Context().Err() == nil {
			return p.conn, p.client, nil
		}
		// A once-usable path gets an immediate rebuild after connection loss.
		p.conn, p.client = nil, nil
		p.nextProbe = time.Time{}
	}
	revision := p.manager.candidateRevision(p.target)
	if revision != p.candidateRevision {
		p.candidateRevision = revision
		p.nextProbe = time.Time{}
	}
	if p.manager.now().Before(p.nextProbe) || !p.manager.directProbeAllowed(p.target) {
		return nil, nil, ErrNoRoute
	}
	p.nextProbe = p.manager.now().Add(time.Duration(p.manager.timing.ProbeMS) * time.Millisecond)
	if !supplied {
		candidates = p.manager.directCandidates(ctx, p.target)
	}
	if c := p.nativeConnection(ctx, candidates); c != nil {
		p.manager.directProbeSucceeded(p.target)
		p.conn = c
		p.client = (&http3.Transport{MaxResponseHeaderBytes: 16 << 10, DisableCompression: true}).NewClientConn(c)
		return c, p.client, nil
	}
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	p.manager.mu.Lock()
	provider := p.manager.iceProvider
	p.manager.mu.Unlock()
	if provider != nil {
		endpoint, addr, err := provider(ctx, p.target)
		if err == nil {
			dialCtx, end := context.WithTimeout(ctx, 5*time.Second)
			defer end()
			c, err := p.dial(dialCtx, endpoint, addr)
			if err == nil && c.ConnectionState().TLS.HandshakeComplete && c.ConnectionState().TLS.NegotiatedProtocol == http3.NextProtoH3 && !c.ConnectionState().Used0RTT {
				p.manager.recordICEFailure(p.target, nil, p.generation)
				p.manager.directProbeSucceeded(p.target)
				p.conn = c
				p.client = (&http3.Transport{MaxResponseHeaderBytes: 16 << 10, DisableCompression: true}).NewClientConn(c)
				return c, p.client, nil
			}
			if c != nil {
				_ = c.CloseWithError(0, "")
			}
			err = ErrICEQUIC
		}
		if ctx.Err() != nil {
			err = &ServiceError{Code: "ICE_PROBE_DEFERRED"}
		}
		p.manager.recordICEFailure(p.target, err, p.generation)
	}
	return nil, nil, ErrNoRoute
}
func (p *quicPool) roundTrip(r *http.Request) (*http.Response, bool, error) {
	c, client, err := p.connection(r.Context())
	if err != nil {
		return nil, false, err
	}
	verify, _ := r.Context().Value(requestVerifierKey{}).(func(tls.ConnectionState) error)
	if verify != nil {
		if err = verify(c.ConnectionState().TLS); err != nil {
			return nil, true, err
		}
	}
	// The deadline remains active until the response body closes; the header timer
	// only bounds waiting for headers, without shortening streaming bodies.
	ctx, cancel := context.WithCancel(r.Context())
	timer := time.AfterFunc(15*time.Second, cancel)
	res, err := client.RoundTrip(r.Clone(ctx))
	timer.Stop()
	if err != nil {
		cancel()
		return nil, true, err
	}
	res.Body = &quicResponseBody{ReadCloser: res.Body, cancel: cancel}
	return res, true, nil
}
func (p *quicPool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		_ = p.conn.CloseWithError(0, "")
		p.conn = nil
		p.client = nil
	}
}

type quicResponseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *quicResponseBody) Close() error { b.cancel(); return b.ReadCloser.Close() }

// Native TCP, relay attachment and QUIC handshakes share one outgoing budget.
// ICE additionally retains its independent two-establishment/eight-pair caps.
func (p *quicPool) dial(ctx context.Context, endpoint *QUICEndpoint, addr net.Addr) (*quic.Conn, error) {
	m := p.manager
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrClosed
	}
	if m.activeDials >= MaxActivePeerSlots || m.peerDials[p.target] >= MaxDirectAttempts {
		m.mu.Unlock()
		return nil, ErrBackpressure
	}
	m.activeDials++
	m.peerDials[p.target]++
	m.dialWG.Add(1)
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.activeDials--
		m.peerDials[p.target]--
		if m.peerDials[p.target] == 0 {
			delete(m.peerDials, p.target)
		}
		m.mu.Unlock()
		m.dialWG.Done()
	}()
	life, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.lifetime, cancel)
	defer func() { stop(); cancel() }()
	return endpoint.transport.Dial(life, addr, p.trust, peerQUICConfig())
}

// Address families race complete pinned QUIC handshakes with the same 200-ms
// stagger as TCP. All losing QUIC connections close before a winner is returned.
func (p *quicPool) nativeConnection(ctx context.Context, candidates []p.NetworkCandidate) *quic.Conn {
	if p.endpoint == nil {
		return nil
	}
	var addresses []*net.UDPAddr
	for _, candidate := range candidates {
		if candidate.Transport != "udp" {
			continue
		}
		if addr, err := net.ResolveUDPAddr("udp", candidate.Address); err == nil {
			addresses = append(addresses, addr)
		}
	}
	if len(addresses) == 0 {
		return nil
	}
	work, cancel := context.WithTimeout(ctx, time.Duration(p.manager.timing.HeadStartMS)*time.Millisecond)
	defer cancel()
	type result struct {
		conn *quic.Conn
		err  error
	}
	results := make(chan result, MaxDirectAttempts)
	next, active := 0, 0
	start := func() {
		address := addresses[next]
		next++
		active++
		go func() { c, err := p.dial(work, p.endpoint, address); results <- result{c, err} }()
	}
	start()
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	var winner *quic.Conn
	for active > 0 {
		select {
		case result := <-results:
			active--
			if result.err == nil && result.conn != nil && winner == nil && work.Err() == nil {
				state := result.conn.ConnectionState()
				if state.TLS.HandshakeComplete && state.TLS.NegotiatedProtocol == http3.NextProtoH3 && !state.Used0RTT {
					winner = result.conn
					cancel()
				}
			}
			if result.conn != nil && result.conn != winner {
				_ = result.conn.CloseWithError(0, "")
			}
			if winner == nil && work.Err() == nil && next < len(addresses) {
				start()
			}
		case <-timer.C:
			if winner == nil && work.Err() == nil && active < MaxDirectAttempts && next < len(addresses) {
				start()
			}
		}
	}
	return winner
}
