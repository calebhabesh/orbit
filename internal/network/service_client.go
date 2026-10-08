package network

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	p "github.com/calebhabesh/orbit/internal/protocol"
	"github.com/coder/websocket"
)

type ServiceClientOptions struct {
	Selection      ProfileSelection
	Origin, Device string
	Certificate    tls.Certificate
	Roots          *x509.CertPool // nil uses system roots, explicit CA never disables TLS
	Now            func() time.Time
	Resolver       interface {
		LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
	}
	Wait func(context.Context, time.Duration) error // nil uses a cancellable timer
}

type ServiceClient struct {
	selection                                ProfileSelection
	origin, digest, device, pin, certificate string
	key                                      ed25519.PrivateKey
	now                                      func() time.Time
	http                                     *http.Client
	transport                                *http.Transport
	mu                                       sync.Mutex
	closed                                   bool
	slots                                    chan struct{}
	lifetime                                 context.Context
	cancel                                   context.CancelFunc
	wg                                       sync.WaitGroup
	controls                                 map[*ServiceControl]bool
	dialSlots                                chan struct{}
	connections                              map[*serviceConn]bool
	relayPending                             map[relayKey]int
	relays                                   map[*relayConn]bool
	wait                                     func(context.Context, time.Duration) error
	// signed bounds challenge-holding operations; budget paces them to the
	// service's per-device metadata rate using wall-clock time.
	signed       chan struct{}
	budgetMu     sync.Mutex
	budgetTokens float64
	budgetAt     time.Time
}

// servedProfile admits a peer proof made under this client's profile or an
// adjacent epoch served by the same origin during rotation. The service admits
// only epochs it serves under one authority, environment and origin; device
// authenticity still comes from signed proofs and pinned inner TLS.
func (c *ServiceClient) servedProfile(digest string) bool {
	return digest == c.digest || p.NetworkHex(digest, 32) == nil
}

func NewServiceClient(opts ServiceClientOptions) (*ServiceClient, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if err := opts.Selection.Validate(uint64(opts.Now().Unix())); err != nil {
		return nil, err
	}
	if p.NetworkHex(opts.Device, 32) != nil {
		return nil, errors.New(p.NetworkIdentityMismatch)
	}
	found := false
	for _, o := range opts.Selection.Profile.Origins {
		if o == opts.Origin && strings.HasPrefix(o, "https://") {
			found = true
		}
	}
	if !found {
		return nil, errors.New(p.NetworkProfileUntrusted)
	}
	if len(opts.Certificate.Certificate) != 1 {
		return nil, errors.New(p.NetworkIdentityMismatch)
	}
	cert, err := x509.ParseCertificate(opts.Certificate.Certificate[0])
	if err != nil {
		return nil, err
	}
	key, ok := opts.Certificate.PrivateKey.(ed25519.PrivateKey)
	if !ok || len(key) != 64 {
		return nil, errors.New(p.NetworkIdentityMismatch)
	}
	pub, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok || !bytes.Equal(pub, key.Public().(ed25519.PublicKey)) {
		return nil, errors.New(p.NetworkIdentityMismatch)
	}
	hash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	if opts.Resolver == nil {
		opts.Resolver = &net.Resolver{PreferGo: true, StrictErrors: true}
	}
	if opts.Wait == nil {
		opts.Wait = func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		}
	}
	opts.Selection.Profile.Origins = append([]string{}, opts.Selection.Profile.Origins...)
	opts.Selection.Profile.STUN = append([]string{}, opts.Selection.Profile.STUN...)
	digest, _ := opts.Selection.Digest()
	lifetime, cancel := context.WithCancel(context.Background())
	c := &ServiceClient{relayPending: map[relayKey]int{}, relays: map[*relayConn]bool{}, selection: opts.Selection, origin: opts.Origin, digest: digest, device: opts.Device, pin: hex.EncodeToString(hash[:]), certificate: base64.StdEncoding.EncodeToString(cert.Raw), key: append(ed25519.PrivateKey{}, key...), now: opts.Now, slots: make(chan struct{}, 4), lifetime: lifetime, cancel: cancel, wait: opts.Wait, controls: map[*ServiceControl]bool{}, dialSlots: make(chan struct{}, 4), connections: map[*serviceConn]bool{}, signed: make(chan struct{}, MaxOutstandingChallenges), budgetTokens: ClientMetadataBurst, budgetAt: time.Now()}
	roots := opts.Roots
	if roots != nil {
		roots = roots.Clone()
	}
	c.transport = &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}, MaxConnsPerHost: 4, MaxIdleConns: 4, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 5 * time.Second, TLSHandshakeTimeout: 5 * time.Second, MaxResponseHeaderBytes: MaxNetworkHeaderBytes, ForceAttemptHTTP2: false}
	c.transport.DialContext = func(ctx context.Context, kind, address string) (net.Conn, error) {
		u, _ := url.Parse(c.origin)
		port := u.Port()
		if port == "" {
			port = "443"
		}
		if kind != "tcp" || address != net.JoinHostPort(u.Hostname(), port) {
			return nil, errors.New(p.NetworkProfileUntrusted)
		}
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, ErrClosed
		}
		select {
		case c.dialSlots <- struct{}{}:
		default:
			c.mu.Unlock()
			return nil, ErrBackpressure
		}
		c.wg.Add(1)
		c.mu.Unlock()
		defer func() { <-c.dialSlots; c.wg.Done() }()
		dialCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		stop := context.AfterFunc(c.lifetime, cancel)
		defer stop()
		conn, err := safeServiceDial(dialCtx, opts.Resolver, u.Hostname(), port, c.selection.Private())
		if err != nil {
			return nil, err
		}
		owned := &serviceConn{Conn: conn, client: c}
		c.mu.Lock()
		if c.closed || len(c.connections) >= 14 {
			c.mu.Unlock()
			_ = conn.Close()
			return nil, ErrBackpressure
		}
		c.connections[owned] = true
		c.mu.Unlock()
		return owned, nil
	}
	c.http = &http.Client{Transport: c.transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New(p.NetworkProfileUntrusted) }}
	return c, nil
}
func safeServiceDial(ctx context.Context, resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}, host, port string, private bool) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip}
	} else {
		var err error
		ips, err = resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, errors.New(p.NetworkServiceUnavailable)
		}
	}
	if len(ips) == 0 || len(ips) > 16 {
		return nil, errors.New(p.NetworkProfileUntrusted)
	}
	for _, ip := range ips {
		if !p.AllowedAddress(ip, private) {
			return nil, errors.New(p.NetworkProfileUntrusted)
		}
	}
	// All answers are checked before connecting. Original hostname remains in TLS;
	// no second resolver/dial gets to substitute an unchecked private destination.
	for _, ip := range ips {
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.New(p.NetworkServiceUnavailable)
}
func randomNetworkID() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func (c *ServiceClient) begin(ctx context.Context) (context.Context, func(), error) {
	work, _, done, err := c.beginDetachable(ctx)
	return work, done, err
}

// beginDetachable also returns detach, which stops the caller's cancellation
// from reaching work; the operation's own bound and client lifetime remain.
func (c *ServiceClient) beginDetachable(ctx context.Context) (context.Context, func(), func(), error) {
	if ctx.Err() != nil {
		return nil, nil, nil, errors.New(p.NetworkCanceled)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, nil, nil, ErrClosed
	}
	if err := c.selection.Validate(uint64(c.now().Unix())); err != nil {
		return nil, nil, nil, err
	}
	select {
	case c.slots <- struct{}{}:
	default:
		return nil, nil, nil, ErrBackpressure
	}
	c.wg.Add(1)
	// Infrastructure TLS must not inherit the peer transport's httptrace: its
	// GotConn verifier belongs to inner peer TLS, never the outer service socket.
	// Preserve cancellation explicitly while dropping request-scoped values.
	work, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	parentStop := context.AfterFunc(ctx, cancel)
	stop := context.AfterFunc(c.lifetime, cancel)
	return work, func() { parentStop() }, func() { parentStop(); stop(); cancel(); <-c.slots; c.wg.Done() }, nil
}

type detachKey struct{}

// beginSigned admits one challenge-bound operation within the service's
// per-device bounds: at most MaxOutstandingChallenges issued challenges and its
// metadata rate. Ordinary operations take a challenge slot without queueing
// (typed backpressure when both are busy) and wait at most maxBudgetWait for
// budget, then fail locally with QUOTA_EXCEEDED without contacting the service.
// One token stays reserved for announcement renewal, the single priority
// caller per purpose, which may wait for its slot and budget.
//
// The caller can cancel while the service connection is being established.
// Once the challenge request is about to be written, the operation finishes
// within its own ten-second bound even if the caller gives up, because an
// issued challenge that is never used blocks this device's metadata for up to
// a minute.
func (c *ServiceClient) beginSigned(ctx context.Context) (context.Context, func(), error) {
	return c.beginSignedPriority(ctx, false)
}

func (c *ServiceClient) beginSignedPriority(ctx context.Context, priority bool) (context.Context, func(), error) {
	if ctx.Err() != nil {
		return nil, nil, errors.New(p.NetworkCanceled)
	}
	continuation := ctx.Value(setupKey{}) != nil
	if priority || continuation {
		select {
		case c.signed <- struct{}{}:
		case <-ctx.Done():
			return nil, nil, errors.New(p.NetworkCanceled)
		case <-c.lifetime.Done():
			return nil, nil, ErrClosed
		}
	} else {
		select {
		case c.signed <- struct{}{}:
		default:
			return nil, nil, ErrBackpressure
		}
	}
	if err := c.spendBudget(ctx, priority, continuation); err != nil {
		<-c.signed
		return nil, nil, err
	}
	work, detach, done, err := c.beginDetachable(ctx)
	if err != nil {
		<-c.signed
		return nil, nil, err
	}
	return context.WithValue(work, detachKey{}, detach), func() { done(); <-c.signed }, nil
}

const maxBudgetWait = 1500 * time.Millisecond

type setupKey struct{}

// BeginSetup admits a coordination sequence of steps signed operations (offer
// or accept, reserve, attach) only when the budget covers all of them, waiting
// at most maxBudgetWait plus the refill time of those steps. Later steps made with the returned context wait for
// budget and a challenge slot instead of failing locally: abandoning a half-made
// relay setup wastes both devices' budget and the session.
func (c *ServiceClient) BeginSetup(ctx context.Context, steps int) (context.Context, error) {
	limit := maxBudgetWait + time.Duration(steps)*time.Minute/MaxMetadataOperationsPerMinute
	if err := c.waitBudget(ctx, float64(steps)+1, false, limit); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, setupKey{}, true), nil
}

func (c *ServiceClient) spendBudget(ctx context.Context, priority, continuation bool) error {
	reserve := 1.0
	if priority {
		reserve = 0
	}
	limit := maxBudgetWait
	if priority || continuation {
		limit = -1
	}
	return c.waitBudget(ctx, 1+reserve, true, limit)
}

// waitBudget waits until at least need tokens exist, spending one when spend
// is set. It gives up locally after limit; a negative limit waits for ctx.
func (c *ServiceClient) waitBudget(ctx context.Context, need float64, spend bool, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		c.budgetMu.Lock()
		now := time.Now()
		c.budgetTokens = min(ClientMetadataBurst, c.budgetTokens+now.Sub(c.budgetAt).Seconds()*MaxMetadataOperationsPerMinute/60)
		c.budgetAt = now
		if c.budgetTokens >= need {
			if spend {
				c.budgetTokens--
			}
			c.budgetMu.Unlock()
			return nil
		}
		delay := time.Duration((need - c.budgetTokens) * float64(time.Minute) / MaxMetadataOperationsPerMinute)
		c.budgetMu.Unlock()
		if limit >= 0 && now.Add(delay).After(deadline) {
			return &ServiceError{Code: p.NetworkQuota}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.New(p.NetworkCanceled)
		case <-c.lifetime.Done():
			timer.Stop()
			return ErrClosed
		case <-timer.C:
		}
	}
}
func (c *ServiceClient) post(ctx context.Context, path string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil || len(b) > p.NetworkMaxBytes {
		return errors.New(p.NetworkInvalidRequest)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.origin+"/network/v1/"+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return errors.New(p.NetworkCanceled)
		}
		return errors.New(p.NetworkServiceUnavailable)
	}
	defer resp.Body.Close()
	b, err = io.ReadAll(io.LimitReader(resp.Body, p.NetworkMaxBytes+1))
	if err != nil || len(b) > p.NetworkMaxBytes {
		return errors.New(p.NetworkInvalidRequest)
	}
	if resp.StatusCode != 200 {
		var f p.NetworkFailure
		if p.NetworkDecode(b, &f) != nil || f.Version != "1" {
			return errors.New(p.NetworkInvalidRequest)
		}
		return &ServiceError{Code: f.Code}
	}
	return p.NetworkDecode(b, out)
}

// Proof obtains a fresh one-use server challenge. Keep a mutation's operation ID
// across uncertain delivery; fresh authentication does not change its intent.
func (c *ServiceClient) proof(ctx context.Context, q p.NetworkProof, payload []byte) (p.NetworkProof, error) {
	var ch p.NetworkChallengeResult
	if detach, ok := ctx.Value(detachKey{}).(func()); ok {
		// The connection is ready and the challenge request is about to be
		// written: from here the caller's cancellation could strand it.
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { detach() }})
	}
	err := c.post(ctx, "challenge", p.NetworkChallengeRequest{Version: "1", Profile: c.digest, Sender: c.device, SenderPin: c.pin, CertificateDER: c.certificate}, &ch)
	if err != nil {
		return q, err
	}
	if ch.Version != "1" || ch.Profile != c.digest || ch.Origin != c.origin || p.NetworkHex(ch.Challenge, 32) != nil || uint64(ch.Expires) <= uint64(c.now().Unix()) || uint64(ch.Expires)-uint64(c.now().Unix()) > 60 {
		return q, errors.New(p.NetworkInvalidRequest)
	}
	q.Version = "1"
	q.Profile = c.digest
	q.Origin = c.origin
	q.Sender = c.device
	q.SenderPin = c.pin
	q.CertificateDER = c.certificate
	q.Challenge = ch.Challenge
	q.Expires = ch.Expires
	if q.Operation == "" {
		q.Operation = randomNetworkID()
	}
	if payload == nil {
		payload, err = q.Intent(c.selection.Private())
		if err != nil {
			return q, err
		}
	}
	q.Payload = p.NetworkDigest(payload)
	b, err := q.Canonical(c.selection.Private())
	if err != nil {
		return q, err
	}
	q.Signature = hex.EncodeToString(ed25519.Sign(c.key, b))
	return q, nil
}
func (c *ServiceClient) Announce(ctx context.Context, purpose, operation string, a p.NetworkAnnouncement) error {
	work, done, err := c.beginSignedPriority(ctx, true)
	if err != nil {
		return err
	}
	defer done()
	b, err := a.Canonical(uint64(c.now().Unix()), true)
	if err != nil {
		return err
	}
	q, err := c.proof(work, p.NetworkProof{Kind: "announce", Purpose: purpose, Operation: operation, Generation: a.Generation}, b)
	if err != nil {
		return err
	}
	var out p.NetworkResult
	if err = c.post(work, "announce", p.NetworkAnnounceRequest{Proof: q, Announcement: a}, &out); err != nil {
		return err
	}
	return validResult(q, out)
}
func validResult(q p.NetworkProof, r p.NetworkResult) error {
	if r.Version != "1" || r.Operation != q.Operation || (r.State != "accepted" && r.State != "replayed" && r.State != "released") {
		return errors.New(p.NetworkInvalidRequest)
	}
	return nil
}

// Lookup requires the caller's preexisting peer pin. Returned records supply
// candidates only. Historical challenge expiry is separate from the signed lease.
func (c *ServiceClient) Lookup(ctx context.Context, device, pin, purpose string, minGeneration uint64) (p.NetworkAnnouncement, bool, error) {
	work, done, err := c.beginSigned(ctx)
	if err != nil {
		return p.NetworkAnnouncement{}, false, err
	}
	defer done()
	q, err := c.proof(work, p.NetworkProof{Kind: "lookup", Purpose: purpose, Target: device, TargetPin: pin}, nil)
	if err != nil {
		return p.NetworkAnnouncement{}, false, err
	}
	var out p.NetworkLookupResult
	if err = c.post(work, "lookup", p.NetworkLookupRequest{Proof: q}, &out); err != nil {
		return p.NetworkAnnouncement{}, false, err
	}
	return c.validateLookup(out, device, pin, purpose, minGeneration)
}
func (c *ServiceClient) validateLookup(out p.NetworkLookupResult, device, pin, purpose string, minGeneration uint64) (p.NetworkAnnouncement, bool, error) {
	invalid := func() (p.NetworkAnnouncement, bool, error) {
		return p.NetworkAnnouncement{}, false, errors.New(p.NetworkInvalidRequest)
	}
	if out.Version != "1" {
		return invalid()
	}
	if !out.Found {
		if len(out.Proofs) != 0 || len(out.Announcements) != 0 {
			return invalid()
		}
		return p.NetworkAnnouncement{}, false, nil
	}
	if len(out.Proofs) != 1 || len(out.Announcements) != 1 {
		return invalid()
	}
	q, a := out.Proofs[0], out.Announcements[0]
	now := uint64(c.now().Unix())
	if q.Kind != "announce" || !c.servedProfile(q.Profile) || q.Origin != c.origin || q.Sender != device || q.SenderPin != pin || q.Purpose != purpose || q.Generation != a.Generation || uint64(a.Generation) < minGeneration {
		return invalid()
	}
	// Verify at its signed challenge time; current lease validity is checked
	// independently. A record's expired auth proof cannot extend its lease.
	if q.Expires < 60 || q.Verify(c.selection.Private(), uint64(q.Expires)-1) != nil {
		return invalid()
	}
	b, err := a.Canonical(now, true)
	if err != nil || q.VerifyPayload(b) != nil {
		return invalid()
	}
	return a, true, nil
}
func (c *ServiceClient) Exchange(ctx context.Context, q p.NetworkProof, offer p.NetworkOffer) error {
	work, done, err := c.beginSigned(ctx)
	if err != nil {
		return err
	}
	defer done()
	if q.Kind != "offer" && q.Kind != "accept" {
		return errors.New(p.NetworkInvalidRequest)
	}
	for _, candidate := range offer.Candidates {
		if err := candidate.Validate(true); err != nil {
			return err
		}
	}
	q.Generation = offer.SenderGeneration
	b, err := offer.Canonical()
	if err != nil {
		return err
	}
	q, err = c.proof(work, q, b)
	if err != nil {
		return err
	}
	var out p.NetworkResult
	if err = c.post(work, q.Kind, p.NetworkOfferRequest{Proof: q, Offer: offer}, &out); err != nil {
		return err
	}
	return validResult(q, out)
}
func (c *ServiceClient) Reserve(ctx context.Context, q p.NetworkProof) (p.RelayAttachment, error) {
	work, done, err := c.beginSigned(ctx)
	if err != nil {
		return p.RelayAttachment{}, err
	}
	defer done()
	q.Kind = "reserve"
	q.Generation = 0
	q, err = c.proof(work, q, nil)
	if err != nil {
		return p.RelayAttachment{}, err
	}
	var out p.NetworkRelayResult
	if err = c.post(work, "reserve", p.NetworkLookupRequest{Proof: q}, &out); err != nil {
		return p.RelayAttachment{}, err
	}
	if out.Version != "1" || len(out.Attachments) != 1 {
		return p.RelayAttachment{}, errors.New(p.NetworkInvalidRequest)
	}
	t := out.Attachments[0]
	serviceKey, _ := hex.DecodeString(c.selection.Profile.ServiceKey)
	originOK := false
	for _, o := range c.selection.Profile.Origins {
		if o == t.Origin && strings.HasPrefix(o, "wss://") {
			originOK = true
		}
	}
	if !originOK || t.Profile != c.digest || t.Session != q.Session || t.Device != c.device || t.Pin != c.pin || t.Partner != q.Target || t.PartnerPin != q.TargetPin || t.Purpose != q.Purpose || t.Role != q.Role || t.Verify(ed25519.PublicKey(serviceKey), uint64(c.now().Unix()), c.selection.Private()) != nil {
		return t, errors.New(p.NetworkInvalidRequest)
	}
	return t, nil
}
func (c *ServiceClient) Release(ctx context.Context, q p.NetworkProof) error {
	work, done, err := c.beginSigned(ctx)
	if err != nil {
		return err
	}
	defer done()
	q.Kind = "release"
	q.Generation = 0
	q, err = c.proof(work, q, nil)
	if err != nil {
		return err
	}
	var out p.NetworkResult
	if err = c.post(work, "release", p.NetworkLookupRequest{Proof: q}, &out); err != nil {
		return err
	}
	return validResult(q, out)
}
func (c *ServiceClient) Close() error {
	c.mu.Lock()
	c.closed = true
	c.cancel()
	controls := make([]*ServiceControl, 0, len(c.controls))
	for control := range c.controls {
		controls = append(controls, control)
	}
	c.mu.Unlock()
	for _, control := range controls {
		_ = control.conn.CloseNow()
	}
	c.transport.CloseIdleConnections()
	c.mu.Lock()
	connections := make([]*serviceConn, 0, len(c.connections))
	for conn := range c.connections {
		connections = append(connections, conn)
	}
	c.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
	c.wg.Wait()
	c.transport.CloseIdleConnections()
	return nil
}

// ServiceControl owns one WSS reader and a finite event queue. Known data peers
// require local pins; unknown enrollment is explicitly enabled with two slots.
type ServiceControl struct {
	pinsMu sync.RWMutex
	pins   map[string]string
	conn   *websocket.Conn
	events chan p.NetworkOfferRequest
	done   chan struct{}
	err    error
}

func (c *ServiceClient) Control(ctx context.Context, purpose string, known map[string]string, allowUnknown bool) (*ServiceControl, error) {
	work, done, err := c.beginSigned(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	knownCopy := map[string]string{}
	if len(known) > 128 {
		return nil, ErrBackpressure
	}
	for id, pin := range known {
		if p.NetworkHex(id, 32) != nil || p.NetworkHex(pin, 32) != nil {
			return nil, errors.New(p.NetworkIdentityMismatch)
		}
		knownCopy[id] = pin
	}
	q, err := c.proof(work, p.NetworkProof{Kind: "authenticate", Purpose: purpose}, nil)
	if err != nil {
		return nil, err
	}
	origin := "wss://" + strings.TrimPrefix(c.origin, "https://")
	found := false
	for _, o := range c.selection.Profile.Origins {
		if o == origin {
			found = true
		}
	}
	if !found {
		return nil, errors.New(p.NetworkProfileUntrusted)
	}
	// coder/websocket uses the same numeric-validated dial and hostname TLS client.
	conn, resp, err := websocket.Dial(work, origin+"/network/v1/control", &websocket.DialOptions{HTTPClient: c.http, CompressionMode: websocket.CompressionDisabled})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, errors.New(p.NetworkServiceUnavailable)
	}
	conn.SetReadLimit(16 << 10)
	b, _ := json.Marshal(p.NetworkLookupRequest{Proof: q})
	if err = conn.Write(work, websocket.MessageText, b); err != nil {
		_ = conn.CloseNow()
		return nil, err
	}
	kind, b, err := conn.Read(work)
	var ack p.NetworkResult
	if err != nil || kind != websocket.MessageText || p.NetworkDecode(b, &ack) != nil || validResult(q, ack) != nil {
		_ = conn.CloseNow()
		return nil, errors.New(p.NetworkInvalidRequest)
	}
	control := &ServiceControl{conn: conn, events: make(chan p.NetworkOfferRequest, 8), done: make(chan struct{}), pins: knownCopy}
	c.mu.Lock()
	if c.closed || len(c.controls) >= 2 {
		c.mu.Unlock()
		_ = conn.CloseNow()
		return nil, ErrBackpressure
	}
	c.controls[control] = true
	c.wg.Add(1)
	c.mu.Unlock()
	go func() {
		defer c.wg.Done()
		defer close(control.done)
		defer close(control.events)
		defer conn.CloseNow()
		defer func() { c.mu.Lock(); delete(c.controls, control); c.mu.Unlock() }()
		life, cancel := context.WithCancel(ctx)
		defer cancel()
		stop := context.AfterFunc(c.lifetime, cancel)
		defer stop()
		unknown := map[string]uint64{}
		for {
			readCtx, end := context.WithTimeout(life, ControlStaleTimeout)
			kind, b, err := conn.Read(readCtx)
			end()
			if err != nil {
				control.err = err
				return
			}
			var heartbeat p.NetworkResult
			if kind == websocket.MessageText && p.NetworkDecode(b, &heartbeat) == nil && validResult(q, heartbeat) == nil {
				continue
			}
			var event p.NetworkOfferRequest
			if kind != websocket.MessageText || p.NetworkDecode(b, &event) != nil || event.Verify(c.selection.Private(), uint64(c.now().Unix())) != nil || !c.servedProfile(event.Proof.Profile) || event.Proof.Origin != c.origin || event.Proof.Target != c.device || event.Proof.TargetPin != c.pin || event.Proof.Purpose != purpose {
				control.err = errors.New(p.NetworkInvalidRequest)
				return
			}
			valid := true
			for _, candidate := range event.Offer.Candidates {
				if candidate.Validate(true) != nil {
					valid = false
				}
			}
			if !valid {
				control.err = errors.New(p.NetworkInvalidRequest)
				return
			}
			if (event.Proof.Kind == "offer" && event.Proof.Role != "initiator") || (event.Proof.Kind == "accept" && event.Proof.Role != "responder") {
				control.err = errors.New(p.NetworkInvalidRequest)
				return
			}
			if !control.knows(event.Proof.Sender, event.Proof.SenderPin) {
				now := uint64(c.now().Unix())
				for session, expires := range unknown {
					if now >= expires {
						delete(unknown, session)
					}
				}
				if purpose != "enrollment" || !allowUnknown || event.Proof.Kind != "offer" || len(unknown) >= 2 {
					continue
				}
				unknown[event.Proof.Session] = uint64(event.Proof.Expires)
			}
			select {
			case control.events <- event:
			case <-life.Done():
				control.err = life.Err()
				return
			default:
				control.err = ErrBackpressure
				return
			}
		}
	}()
	return control, nil
}
func (c *ServiceControl) Next(ctx context.Context) (p.NetworkOfferRequest, error) {
	select {
	case event, ok := <-c.events:
		if !ok {
			<-c.done
			return event, c.err
		}
		return event, nil
	case <-ctx.Done():
		return p.NetworkOfferRequest{}, ctx.Err()
	}
}
func (c *ServiceControl) Close() error { _ = c.conn.CloseNow(); <-c.done; return nil }

// Reannounce has one synchronous worker, bounded jitter and finite exchanges.
// Policy is checked before every public operation. Manual/local-only modes never
// instantiate traffic. Caller supplies already scoped candidates and generation.
func (c *ServiceClient) Reannounce(ctx context.Context, mode, purpose string, next func() p.NetworkAnnouncement) error {
	if mode != "automatic" && mode != "self_hosted" {
		return errors.New(p.NetworkUnsupported)
	}
	if mode == "automatic" && c.selection.Environment != "release" {
		return errors.New(p.NetworkProfileUntrusted)
	}
	var generation p.NetworkUint
	for {
		a := next()
		if a.Generation <= generation {
			a.Generation = generation + 1
		}
		if a.Generation == 0 {
			return errors.New(p.NetworkStaleGeneration)
		}
		generation = a.Generation
		if a.Expires == 0 {
			a.Expires = p.NetworkUint(c.now().Unix() + 600)
		}
		operation := randomNetworkID()
		// Retry uncertain delivery with identical mutation bytes and a fresh proof.
		err := c.Announce(ctx, purpose, operation, a)
		if err != nil && ctx.Err() == nil {
			err = c.Announce(ctx, purpose, operation, a)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, ErrClosed) {
			return err
		}
		if err != nil && err.Error() != p.NetworkServiceUnavailable && err.Error() != p.NetworkQuota && !errors.Is(err, ErrBackpressure) {
			return err
		}
		var jitter [1]byte
		_, _ = rand.Read(jitter[:])
		delay := ReannounceInterval + time.Duration(int(jitter[0])%41-20)*time.Second
		waitCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(c.lifetime, cancel)
		err = c.wait(waitCtx, delay)
		stop()
		cancel()
		if err != nil {
			if c.lifetime.Err() != nil {
				return ErrClosed
			}
			return err
		}
	}
}

// Own detached net/http dials and upgraded sockets independently of request
// slots so rapid cancellation cannot grow resolver work or escape shutdown.
type serviceConn struct {
	net.Conn
	client *ServiceClient
	once   sync.Once
}

func (c *serviceConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.client.mu.Lock(); delete(c.client.connections, c); c.client.mu.Unlock() })
	return err
}

// CloseIdleConnections releases metadata keepalives without closing upgraded legs.
func (c *ServiceClient) CloseIdleConnections() { c.transport.CloseIdleConnections() }

func (c *ServiceControl) knows(device, pin string) bool {
	c.pinsMu.RLock()
	defer c.pinsMu.RUnlock()
	return c.pins[device] == pin
}
func (c *ServiceControl) AddPeer(device, pin string) error {
	if p.NetworkHex(device, 32) != nil || p.NetworkHex(pin, 32) != nil {
		return errors.New(p.NetworkIdentityMismatch)
	}
	c.pinsMu.Lock()
	defer c.pinsMu.Unlock()
	if old, ok := c.pins[device]; ok && old != pin {
		return errors.New(p.NetworkIdentityMismatch)
	}
	if _, ok := c.pins[device]; !ok && len(c.pins) >= 128 {
		return ErrBackpressure
	}
	c.pins[device] = pin
	return nil
}
