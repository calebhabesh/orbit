package rendezvous

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
	"github.com/coder/websocket"
)

type controlConn interface{ CloseNow() error }

func (s *Service) serveControl(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Encoding") != "" {
		s.failure(w, p.NetworkInvalidRequest)
		return
	}
	// Reset finite HTTP deadlines for the owned websocket lifetime.
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = c.CloseNow()
		return
	}
	s.pending[c] = true
	s.controlWG.Add(1)
	s.mu.Unlock()
	defer s.controlWG.Done()
	defer c.CloseNow()
	defer func() { s.mu.Lock(); delete(s.pending, c); s.mu.Unlock() }()
	c.SetReadLimit(16 << 10)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	authCtx, stop := context.WithTimeout(ctx, 10*time.Second)
	kind, body, err := c.Read(authCtx)
	stop()
	var req p.NetworkLookupRequest
	if err != nil || kind != websocket.MessageText || p.NetworkDecode(body, &req) != nil {
		return
	}
	key := routeKey{who(req.Proof), req.Proof.Purpose}
	s.mu.Lock()
	now := uint64(s.now().Unix())
	s.expire(now)
	code := s.intent(req.Proof, "authenticate", now)
	if _, expired := s.profile(req.Proof.Profile, now); expired != "" {
		code = expired
	}
	if _, exists := s.controls[key]; exists || len(s.controls) >= network.MaxServiceControls {
		code = p.NetworkQuota
	}
	entry := &control{conn: c, events: make(chan p.NetworkOfferRequest, 8), profile: req.Proof.Profile}
	if code == "" {
		s.controls[key] = entry
	}
	s.mu.Unlock()
	if code != "" {
		s.stats.refused(code)
		return
	}
	defer func() {
		s.mu.Lock()
		if s.controls[key] == entry {
			delete(s.controls, key)
		}
		s.mu.Unlock()
	}()
	ack, _ := json.Marshal(result(req.Proof, "accepted"))
	writeCtx, done := context.WithTimeout(ctx, 5*time.Second)
	err = c.Write(writeCtx, websocket.MessageText, ack)
	done()
	if err != nil {
		return
	}
	// One bounded reader detects close and services websocket ping/pong. Clients
	// send no further messages; all mutations use fresh signed HTTPS exchanges.
	readerDone := make(chan struct{})
	go func() { defer close(readerDone); _, _, _ = c.Read(ctx); cancel() }()
	defer func() { cancel(); _ = c.CloseNow(); <-readerDone }()
	ticker := time.NewTicker(network.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-entry.events:
			body, _ := json.Marshal(event)
			work, end := context.WithTimeout(ctx, 5*time.Second)
			err = c.Write(work, websocket.MessageText, body)
			end()
			if err != nil {
				return
			}
		case <-ticker.C:
			s.mu.Lock()
			_, expired := s.profile(entry.profile, uint64(s.now().Unix()))
			invalid := expired != ""
			s.mu.Unlock()
			if invalid {
				return
			}
			work, end := context.WithTimeout(ctx, 5*time.Second)
			err = c.Ping(work)
			if err == nil {
				err = c.Write(work, websocket.MessageText, ack)
			}
			end()
			if err != nil {
				return
			}
		}
	}
}

func (s *Service) Close() error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	for c := range s.pending {
		_ = c.CloseNow()
	}
	s.challenges = map[string]challenge{}
	s.records = map[routeKey]record{}
	s.operations = map[string]replay{}
	for id := range s.sessions {
		s.dropSession(id)
	}
	s.deviceBandwidth = map[actor]*byteLimiter{}
	s.rates = map[actor]bucket{}
	s.sources = map[string]bucket{}
	s.mu.Unlock()
	s.controlWG.Wait()
	s.sweepWG.Wait()
	return nil
}

// Server supplies mandatory transport bounds. Listen is also capped before TLS
// or HTTP starts goroutines; overload sockets close without a queued handler.
func (s *Service) Server() *http.Server {
	return &http.Server{ErrorLog: log.New(io.Discard, "", 0), Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: network.MaxNetworkHeaderBytes}
}

// CappedListener closes sockets beyond MaxConnections before TLS starts.
type CappedListener struct {
	net.Listener
	slots   chan struct{}
	refused atomic.Uint64
}
type boundedConn struct {
	net.Conn
	once  sync.Once
	slots chan struct{}
}

func (c *boundedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { <-c.slots })
	return err
}
func BoundedListener(l net.Listener) *CappedListener {
	return &CappedListener{Listener: l, slots: make(chan struct{}, MaxConnections)}
}

// Active and Refused are sanitized socket totals for operator monitoring.
func (l *CappedListener) Active() int     { return len(l.slots) }
func (l *CappedListener) Refused() uint64 { return l.refused.Load() }
func (l *CappedListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.slots <- struct{}{}:
			return &boundedConn{Conn: c, slots: l.slots}, nil
		default:
			l.refused.Add(1)
			_ = c.Close()
		}
	}
}
