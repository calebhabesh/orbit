package rendezvous

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
	"github.com/coder/websocket"
)

// RelayLimits are operator ceilings, independent of admission to any folder.
// Zero selects finite defaults; invalid or unlimited values fail closed.
type RelayLimits struct {
	ServiceBytesPerSecond int64
	DeviceBytesPerSecond  int64
	SessionBytes          int64
	Lifetime              time.Duration
	Idle                  time.Duration
	Drain                 time.Duration
}

func (l RelayLimits) defaults() (RelayLimits, error) {
	if l.ServiceBytesPerSecond == 0 {
		l.ServiceBytesPerSecond = 20 << 20
	}
	if l.DeviceBytesPerSecond == 0 {
		l.DeviceBytesPerSecond = 5 << 20
	}
	if l.SessionBytes == 0 {
		l.SessionBytes = 16 << 30
	}
	if l.Lifetime == 0 {
		l.Lifetime = network.TunnelLifetime
	}
	if l.Drain == 0 {
		l.Drain = network.TunnelDrainTimeout
	}
	if l.Idle == 0 {
		l.Idle = network.TunnelIdleTimeout
	}
	if l.ServiceBytesPerSecond < 1 || l.DeviceBytesPerSecond < 1 || l.SessionBytes < 1 || l.Lifetime < time.Millisecond || l.Lifetime > network.TunnelLifetime || l.Idle < time.Millisecond || l.Idle > network.TunnelIdleTimeout || l.Drain < time.Millisecond || l.Drain > network.TunnelDrainTimeout {
		return l, errors.New("INVALID_RELAY_LIMIT")
	}
	return l, nil
}

type relaySession struct {
	ctx       context.Context
	cancel    context.CancelFunc
	legs      map[string]net.Conn
	done      chan struct{}
	limits    []*byteLimiter
	bytesMu   sync.Mutex
	remaining int64
}

// Called under Service.mu. Cancellation wakes handlers; socket closes and joins
// happen outside the service lock. Never hold that lock across stream shutdown.
func (s *Service) dropSession(id string) {
	if v := s.sessions[id]; v != nil {
		delete(s.sessions, id)
		if v.relay != nil {
			v.relay.cancel()
		}
	}

}
func (s *Service) attach(req p.NetworkRelayAttachRequest, now uint64) (*session, string) {
	sp, code := s.profile(req.Attachment.Profile, now)
	if code != "" {
		return nil, code
	}
	if req.Verify(sp.key.Public().(ed25519.PublicKey), req.Attachment.Profile, s.relayOrigin, s.epoch, now, s.selection.Private()) != nil || req.Proof.Generation != 0 {
		return nil, p.NetworkIdentityMismatch
	}
	intent, _ := req.Proof.Intent(s.selection.Private())
	if code := s.admitOrigin(req.Proof, intent, now, s.relayOrigin); code != "" {
		return nil, code
	}
	v := s.sessions[req.Proof.Session]
	if v == nil || !v.accepted || v.attachments[req.Proof.Role] != req.Attachment {
		return nil, p.NetworkUnavailable
	}
	if v.relay != nil && v.relay.legs[req.Proof.Role] != nil {
		return nil, p.NetworkReplay
	}
	if v.relay == nil {
		data, unknown, perSender, perTarget, perPair := 0, 0, 0, 0, 0
		for _, old := range s.sessions {
			if old.relay == nil {
				continue
			}
			if old.proof.Purpose == "enrollment" {
				unknown++
				continue
			}
			data++
			if who(old.proof) == who(v.proof) || target(old.proof) == who(v.proof) {
				perSender++
			}
			if who(old.proof) == target(v.proof) || target(old.proof) == target(v.proof) {
				perTarget++
			}
			if (who(old.proof) == who(v.proof) && target(old.proof) == target(v.proof)) || (who(old.proof) == target(v.proof) && target(old.proof) == who(v.proof)) {
				perPair++
			}
		}
		if (v.proof.Purpose == "enrollment" && unknown >= MaxUnknownOffers) || (v.proof.Purpose == "peer_data" && (data >= network.MaxServiceDataTunnels || perSender >= network.MaxDataTunnels || perTarget >= network.MaxDataTunnels || perPair >= network.MaxTunnelsPerPeer)) {
			s.dropSession(req.Proof.Session)
			return nil, p.NetworkQuota
		}
		missing := 0
		for _, a := range []actor{who(v.proof), target(v.proof)} {
			if s.deviceBandwidth[a] == nil {
				missing++
			}
		}
		if len(s.deviceBandwidth)+missing > 1024 {
			s.dropSession(req.Proof.Session)
			return nil, p.NetworkQuota
		}
		ctx, cancel := context.WithCancel(context.Background())
		relay := &relaySession{ctx: ctx, cancel: cancel, legs: map[string]net.Conn{}, done: make(chan struct{}), remaining: s.relayLimits.SessionBytes, limits: []*byteLimiter{s.bandwidth}}
		for _, a := range []actor{who(v.proof), target(v.proof)} {
			limiter := s.deviceBandwidth[a]
			if limiter == nil {
				limiter = newByteLimiter(s.relayLimits.DeviceBytesPerSecond)
				s.deviceBandwidth[a] = limiter
			}
			relay.limits = append(relay.limits, limiter)
		}
		v.relay = relay
	}
	return v, ""
}
func (s *Service) serveRelay(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Encoding") != "" {
		s.failure(w, p.NetworkInvalidRequest)
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = ws.CloseNow()
		return
	}
	s.pending[ws] = true
	s.controlWG.Add(1)
	s.mu.Unlock()
	defer s.controlWG.Done()
	defer ws.CloseNow()
	defer func() { s.mu.Lock(); delete(s.pending, ws); s.mu.Unlock() }()
	ws.SetReadLimit(16 << 10)
	auth, cancel := context.WithTimeout(r.Context(), network.InnerHandshakeTimeout)
	kind, body, err := ws.Read(auth)
	cancel()
	var req p.NetworkRelayAttachRequest
	if err != nil || kind != websocket.MessageText || p.NetworkDecode(body, &req) != nil {
		return
	}
	s.mu.Lock()
	now := uint64(s.now().Unix())
	s.expire(now)
	v, code := s.attach(req, now)
	// Occupy the role before releasing the lock, including the acknowledgement.
	if code == "" {
		v.relay.legs[req.Proof.Role] = &reservedLeg{}
	}
	s.mu.Unlock()
	if code != "" {
		s.stats.refused(code)
		b, _ := json.Marshal(p.NetworkFailure{Version: "1", Code: code, Retryable: code == p.NetworkQuota})
		work, end := context.WithTimeout(r.Context(), time.Second)
		_ = ws.Write(work, websocket.MessageText, b)
		end()
		return
	}
	relay := v.relay
	defer func() {
		s.mu.Lock()
		if s.sessions[req.Proof.Session] == v {
			s.dropSession(req.Proof.Session)
		}
		s.mu.Unlock()
	}()
	b, _ := json.Marshal(result(req.Proof, "accepted"))
	work, end := context.WithTimeout(relay.ctx, 5*time.Second)
	err = ws.Write(work, websocket.MessageText, b)
	end()
	if err != nil {
		return
	}
	stream := network.BinaryStream(relay.ctx, ws)
	defer stream.Close()
	stop := context.AfterFunc(relay.ctx, func() { _ = stream.Close() })
	defer stop()
	s.mu.Lock()
	if relay.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	relay.legs[req.Proof.Role] = stream
	a, bleg := relay.legs["initiator"], relay.legs["responder"]
	_, pendingA := a.(*reservedLeg)
	_, pendingB := bleg.(*reservedLeg)
	paired := a != nil && bleg != nil && !pendingA && !pendingB
	if paired {
		// Active lifetime differs from one-use attachment expiry. Profile expiry
		// and the finite hard lifetime still bound every established tunnel.
		v.expires = now + uint64((s.relayLimits.Lifetime+s.relayLimits.Drain)/time.Second) + 1
	}
	s.mu.Unlock()
	if paired {
		defer close(relay.done)
		lifetime, finish := context.WithTimeout(relay.ctx, s.relayLimits.Lifetime+s.relayLimits.Drain)
		defer finish()
		stopLife := context.AfterFunc(lifetime, relay.cancel)
		defer stopLife()
		_ = network.Forward(lifetime, &quotaConn{Conn: a, relay: relay, ctx: lifetime, bytes: &s.stats.relayBytes}, &quotaConn{Conn: bleg, relay: relay, ctx: lifetime, bytes: &s.stats.relayBytes}, s.relayLimits.Idle)
		return
	}
	select {
	case <-relay.ctx.Done():
	case <-stream.(interface{ Failed() <-chan struct{} }).Failed():
	case <-relay.done:
	}
}

// Marker is never read or written. It prevents simultaneous duplicate-role
// authentication while the bounded acknowledgement is being sent.
type reservedLeg struct{ net.Conn }

// A fixed-size token bucket uses socket backpressure. At most two pumps per
// admitted session wait; timers and byte accounting never scale with file size.
type byteLimiter struct {
	mu     sync.Mutex
	rate   float64
	tokens float64
	at     time.Time
}

func newByteLimiter(rate int64) *byteLimiter {
	return &byteLimiter{rate: float64(rate), tokens: network.StreamBufferBytes, at: time.Now()}
}
func (l *byteLimiter) wait(ctx context.Context, n int) error {
	for {
		l.mu.Lock()
		now := time.Now()
		l.tokens = min(float64(network.StreamBufferBytes), l.tokens+now.Sub(l.at).Seconds()*l.rate)
		l.at = now
		if l.tokens >= float64(n) {
			l.tokens -= float64(n)
			l.mu.Unlock()
			return nil
		}
		delay := time.Duration((float64(n) - l.tokens) / l.rate * float64(time.Second))
		l.mu.Unlock()
		timer := time.NewTimer(max(delay, time.Millisecond))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

type quotaConn struct {
	net.Conn
	relay    *relaySession
	ctx      context.Context
	deadline time.Time
	bytes    *atomic.Uint64
}

func (c *quotaConn) Write(b []byte) (int, error) {
	c.relay.bytesMu.Lock()
	if int64(len(b)) > c.relay.remaining {
		c.relay.bytesMu.Unlock()
		return 0, errors.New(p.NetworkQuota)
	}
	c.relay.remaining -= int64(len(b))
	c.relay.bytesMu.Unlock()
	ctx := c.ctx
	var cancel context.CancelFunc
	if !c.deadline.IsZero() {
		ctx, cancel = context.WithDeadline(ctx, c.deadline)
		defer cancel()
	}
	for _, l := range c.relay.limits {
		if err := l.wait(ctx, len(b)); err != nil {
			return 0, err
		}
	}
	n, err := c.Conn.Write(b)
	c.bytes.Add(uint64(n))
	return n, err
}

func (c *quotaConn) Close() error { c.relay.cancel(); return c.Conn.Close() }
func (c *quotaConn) SetWriteDeadline(t time.Time) error {
	c.deadline = t
	return c.Conn.SetWriteDeadline(t)
}

// Retain exhausted device buckets for one minute across reconnects, so closing
// a tunnel cannot replenish its bandwidth burst. Only inactive keys are pruned.
func (s *Service) expireBandwidth() {
	for a, l := range s.deviceBandwidth {
		l.mu.Lock()
		stale := time.Since(l.at) > time.Minute
		l.mu.Unlock()
		if !stale {
			continue
		}
		live := false
		for _, v := range s.sessions {
			if v.relay != nil && (who(v.proof) == a || target(v.proof) == a) {
				live = true
				break
			}
		}
		if !live {
			delete(s.deviceBandwidth, a)
		}
	}
}
