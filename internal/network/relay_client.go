package network

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"time"

	p "github.com/calebhabesh/orbit/internal/protocol"
	"github.com/coder/websocket"
)

type relayKey struct{ device, pin, purpose string }
type relayConn struct {
	net.Conn
	client     *ServiceClient
	key        relayKey
	once       sync.Once
	attachedAt time.Time
}

func (r *relayConn) Close() error {
	err := r.Conn.Close()
	r.once.Do(func() {
		r.client.mu.Lock()
		delete(r.client.relays, r)
		r.client.relayPending[r.key]--
		if r.client.relayPending[r.key] == 0 {
			delete(r.client.relayPending, r.key)
		}
		r.client.mu.Unlock()
	})
	return err
}

// Attach builds one outbound leg. The token is not a folder capability; callers
// must run pinned inner TLS before disclosing any application bytes. Successful
// streams outlive the dial context, but never the owning service client.
func (c *ServiceClient) Attach(ctx context.Context, t p.RelayAttachment) (net.Conn, error) {
	serviceKey, _ := hex.DecodeString(c.selection.Profile.ServiceKey)
	origin := "wss://" + strings.TrimPrefix(c.origin, "https://")
	if t.Profile != c.digest || t.Origin != origin || t.Device != c.device || t.Pin != c.pin || t.Verify(ed25519.PublicKey(serviceKey), uint64(c.now().Unix()), c.selection.Private()) != nil {
		return nil, &ServiceError{Code: p.NetworkIdentityMismatch}
	}
	approved := false
	for _, o := range c.selection.Profile.Origins {
		if o == origin {
			approved = true
		}
	}
	if !approved {
		return nil, &ServiceError{Code: p.NetworkProfileUntrusted}
	}
	key := relayKey{t.Partner, t.PartnerPin, t.Purpose}
	c.mu.Lock()
	data, enrollment := 0, 0
	for k, n := range c.relayPending {
		if k.purpose == "peer_data" {
			data += n
		} else {
			enrollment += n
		}
	}
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	if c.relayPending[key] >= MaxTunnelsPerPeer || (t.Purpose == "peer_data" && data >= MaxDataTunnels) || (t.Purpose == "enrollment" && enrollment >= MaxUnknownEnrollmentTunnels) {
		c.mu.Unlock()
		return nil, ErrBackpressure
	}
	c.relayPending[key]++
	c.mu.Unlock()
	admitted := false
	defer func() {
		if !admitted {
			c.mu.Lock()
			c.relayPending[key]--
			if c.relayPending[key] == 0 {
				delete(c.relayPending, key)
			}
			c.mu.Unlock()
		}
	}()
	work, done, err := c.beginSigned(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	q, err := c.proof(work, p.NetworkProof{Kind: "attach", Target: t.Partner, TargetPin: t.PartnerPin, Purpose: t.Purpose, Session: t.Session, Role: t.Role}, nil)
	if err != nil {
		return nil, err
	}
	// Challenge belongs to HTTPS coordination, but the signed attachment binds the
	// approved WSS origin. Recompute intent/signature; do not rewrite signed bytes.
	q.Origin = t.Origin
	intent, err := q.Intent(c.selection.Private())
	if err != nil {
		return nil, err
	}
	q.Payload = p.NetworkDigest(intent)
	canonical, err := q.Canonical(c.selection.Private())
	if err != nil {
		return nil, err
	}
	q.Signature = hex.EncodeToString(ed25519.Sign(c.key, canonical))
	ws, resp, err := websocket.Dial(work, origin+"/network/v1/relay", &websocket.DialOptions{HTTPClient: c.http, CompressionMode: websocket.CompressionDisabled})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		if resp != nil && resp.StatusCode == 429 {
			return nil, &ServiceError{Code: p.NetworkQuota}
		}
		return nil, &ServiceError{Code: p.NetworkServiceUnavailable}
	}
	success := false
	defer func() {
		if !success {
			_ = ws.CloseNow()
		}
	}()
	ws.SetReadLimit(16 << 10)
	b, _ := json.Marshal(p.NetworkRelayAttachRequest{Proof: q, Attachment: t})
	if err = ws.Write(work, websocket.MessageText, b); err != nil {
		return nil, &ServiceError{Code: p.NetworkUnavailable}
	}
	kind, b, err := ws.Read(work)
	if err != nil || kind != websocket.MessageText {
		return nil, &ServiceError{Code: p.NetworkUnavailable}
	}
	var ack p.NetworkResult
	if p.NetworkDecode(b, &ack) != nil || validResult(q, ack) != nil {
		var failure p.NetworkFailure
		if p.NetworkDecode(b, &failure) == nil && failure.Version == "1" {
			return nil, &ServiceError{Code: failure.Code}
		}
		return nil, &ServiceError{Code: p.NetworkInvalidRequest}
	}
	stream := BinaryStream(c.lifetime, ws)
	owned := &relayConn{Conn: stream, client: c, key: key, attachedAt: time.Now()}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = stream.Close()
		return nil, ErrClosed
	}
	c.relays[owned] = true
	c.wg.Add(1)
	admitted = true
	c.mu.Unlock()
	success = true
	timer := time.AfterFunc(TunnelLifetime+TunnelDrainTimeout, func() { _ = owned.Close() })
	go func() {
		defer c.wg.Done()
		defer timer.Stop()
		<-stream.(interface{ Failed() <-chan struct{} }).Failed()
		_ = owned.Close()
	}()
	return owned, nil
}

// RelayDialer supplies an already-coordinated credential per new socket. The
// credential source must preserve exact peer/purpose/profile scope. A fresh
// accepted session is required after a previous stream closes.
func (c *ServiceClient) RelayDialer(reserve func(context.Context, Target) (p.RelayAttachment, error)) StreamDialer {
	return func(ctx context.Context, target Target) (net.Conn, error) {
		if reserve == nil || target.Validate() != nil {
			return nil, &ServiceError{Code: p.NetworkInvalidRequest}
		}
		if hex.EncodeToString(target.Profile[:]) != c.digest {
			return nil, &ServiceError{Code: p.NetworkProfileUntrusted}
		}
		t, err := reserve(ctx, target)
		if err != nil {
			return nil, err
		}
		if t.Partner != hex.EncodeToString(target.Device[:]) || t.PartnerPin != hex.EncodeToString(target.Pin[:]) || t.Purpose != string(target.Purpose) {
			return nil, &ServiceError{Code: p.NetworkPurposeMismatch}
		}
		return c.Attach(ctx, t)
	}
}

// OfferRelay keeps inbound enrollment and data queues distinct. The caller owns
// the matching isolated TLS HTTP server; owner control has no listener here.
func (c *ServiceClient) OfferRelay(ctx context.Context, t p.RelayAttachment, purpose Purpose, listener *StreamListener) error {
	if listener == nil || string(purpose) != t.Purpose || (purpose != PeerData && purpose != Enrollment) {
		return &ServiceError{Code: p.NetworkPurposeMismatch}
	}
	stream, err := c.Attach(ctx, t)
	if err != nil {
		return err
	}
	offer, cancel := context.WithTimeout(ctx, InnerHandshakeTimeout)
	defer cancel()
	return listener.Offer(offer, stream)
}

// LogicalOrigin carries no dialable address. The manager binds it to a Target.
func LogicalOrigin(t Target) string {
	return "https://" + hex.EncodeToString(t.Device[:]) + ".peer.orbit.invalid"
}

// Retired prevents a pooled socket from starting another request at lifetime expiry.
func (r *relayConn) Retired() bool { return !time.Now().Before(r.attachedAt.Add(TunnelLifetime)) }
