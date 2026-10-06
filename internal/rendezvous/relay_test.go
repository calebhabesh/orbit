package rendezvous

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
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/network"
	p "github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/coder/websocket"
)

type relayFixture struct {
	f      *fixture
	a, b   device
	ca, cb *network.ServiceClient
	ta, tb p.RelayAttachment
	ctx    context.Context
}

func newRelayFixture(t *testing.T, purpose string) *relayFixture {
	t.Helper()
	f := newFixture(t, true)
	a, b := relayIdentity(t, 101), relayIdentity(t, 102)
	ca, cb := f.client(t, a, f.roots), f.client(t, b, f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	for _, c := range []*network.ServiceClient{ca, cb} {
		if err := c.Announce(ctx, purpose, nonce(), announcement(f, 1)); err != nil {
			t.Fatal(err)
		}
	}
	ac, err := ca.Control(ctx, purpose, map[string]string{b.id: b.pin}, purpose == "enrollment")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ac.Close() })
	bc, err := cb.Control(ctx, purpose, map[string]string{a.id: a.pin}, purpose == "enrollment")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bc.Close() })
	offer := p.NetworkOffer{SenderGeneration: 1, TargetGeneration: 1, Candidates: []p.NetworkCandidate{}}
	qa := p.NetworkProof{Kind: "offer", Target: b.id, TargetPin: b.pin, Purpose: purpose, Session: nonce(), Role: "initiator"}
	if err = ca.Exchange(ctx, qa, offer); err != nil {
		t.Fatal(err)
	}
	if _, err = bc.Next(ctx); err != nil {
		t.Fatal(err)
	}
	qb := p.NetworkProof{Kind: "accept", Target: a.id, TargetPin: a.pin, Purpose: purpose, Session: qa.Session, Role: "responder"}
	if err = cb.Exchange(ctx, qb, offer); err != nil {
		t.Fatal(err)
	}
	if _, err = ac.Next(ctx); err != nil {
		t.Fatal(err)
	}
	ta, err := ca.Reserve(ctx, qa)
	if err != nil {
		t.Fatal(err)
	}
	tb, err := cb.Reserve(ctx, qb)
	if err != nil {
		t.Fatal(err)
	}
	return &relayFixture{f, a, b, ca, cb, ta, tb, ctx}
}
func (r *relayFixture) legs(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	a, err := r.ca.Attach(r.ctx, r.ta)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	b, err := r.cb.Attach(r.ctx, r.tb)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return a, b
}
func waitRelayEmpty(t *testing.T, f *fixture) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.s.mu.Lock()
		n := len(f.s.sessions)

		f.s.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Fatal("reservation retained after close")
}
func TestWANW04OutboundLegsAndBidirectionalBytes(t *testing.T) {
	r := newRelayFixture(t, "peer_data")
	a, b := r.legs(t)
	payload := bytes.Repeat([]byte("synthetic-ciphertext"), 9000)
	done := make(chan error, 1)
	go func() { _, err := a.Write(payload); done <- err }()
	got := make([]byte, len(payload))
	_ = b.SetReadDeadline(time.Now().Add(time.Second * 3))
	if _, err := io.ReadFull(b, got); err != nil || !bytes.Equal(got, payload) {
		t.Fatal("forward", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	go func() { _, err := b.Write(payload); done <- err }()
	if _, err := io.ReadFull(a, got); err != nil || !bytes.Equal(got, payload) {
		t.Fatal("reverse", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// Attachment expiry must not terminate an active stream.
	r.f.now.Add(31)
	r.f.s.mu.Lock()
	r.f.s.expire(uint64(r.f.now.Load()))
	n := len(r.f.s.sessions)
	r.f.s.mu.Unlock()
	if n != 1 {
		t.Fatal("active tunnel expired with credential")
	}
	_ = a.Close()
	waitRelayEmpty(t, r.f)
}
func TestWANW04WrongAttachmentsAndReplay(t *testing.T) {
	for _, kind := range []string{"role", "pin", "purpose", "epoch", "signature", "expired", "released", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			r := newRelayFixture(t, "peer_data")
			token := r.ta
			switch kind {
			case "role":
				token.Role = "responder"
			case "pin":
				token.Pin = r.b.pin
			case "purpose":
				token.Purpose = "enrollment"
			case "epoch":
				token.Epoch = nonce()
			case "signature":
				token.Signature = hex.EncodeToString(make([]byte, ed25519.SignatureSize))
			case "expired":
				r.f.now.Add(31)
			case "released":
				if err := r.ca.Release(r.ctx, p.NetworkProof{Target: r.b.id, TargetPin: r.b.pin, Purpose: "peer_data", Session: token.Session, Role: "initiator"}); err != nil {
					t.Fatal(err)
				}
			case "duplicate":
				a, err := r.ca.Attach(r.ctx, token)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
			}
			if c, err := r.ca.Attach(r.ctx, token); err == nil {
				_ = c.Close()
				t.Fatal("unsafe attachment admitted")
			}
		})
	}
}
func TestWANW04UnpairedExpiryAndAbruptLoss(t *testing.T) {
	for _, kind := range []string{"expiry", "close", "service"} {
		t.Run(kind, func(t *testing.T) {
			r := newRelayFixture(t, "peer_data")
			a, err := r.ca.Attach(r.ctx, r.ta)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			switch kind {
			case "expiry":
				r.f.now.Add(31)
				r.f.s.mu.Lock()
				r.f.s.expire(uint64(r.f.now.Load()))
				r.f.s.mu.Unlock()
			case "close":
				_ = a.Close()
			case "service":
				_ = r.f.s.Close()
			}
			waitRelayEmpty(t, r.f)
		})
	}
}
func TestWANW04IdleLifetimeByteQuotaAndCancel(t *testing.T) {
	for _, kind := range []string{"idle", "lifetime", "bytes", "cancel", "slow"} {
		t.Run(kind, func(t *testing.T) {
			r := newRelayFixture(t, "peer_data")
			r.f.s.mu.Lock()
			switch kind {
			case "idle":
				r.f.s.relayLimits.Idle = 40 * time.Millisecond
			case "lifetime":
				r.f.s.relayLimits.Lifetime = 40 * time.Millisecond
				r.f.s.relayLimits.Drain = time.Millisecond
			case "bytes":
				r.f.s.relayLimits.SessionBytes = 100
			case "slow":
				r.f.s.relayLimits.Idle = 80 * time.Millisecond
			}
			r.f.s.mu.Unlock()
			a, b := r.legs(t)
			switch kind {
			case "bytes":
				_, _ = a.Write(make([]byte, 101))
			case "cancel":
				_ = r.ca.Close()
			case "slow":
				go func() {
					buf := make([]byte, network.StreamBufferBytes)
					for i := 0; i < 4096; i++ {
						if _, err := a.Write(buf); err != nil {
							return
						}
					}
				}()
			}
			_ = b.SetReadDeadline(time.Now().Add(3 * time.Second))
			buf := make([]byte, 4096)
			if kind == "slow" {
				time.Sleep(150 * time.Millisecond)
				_ = b.Close()
			} else {
				for {
					if _, err := b.Read(buf); err != nil {
						break
					}
				}
			}
			waitRelayEmpty(t, r.f)
		})
	}
}
func TestWANW04BandwidthCeilingAndCancellation(t *testing.T) {
	limiter := newByteLimiter(10000)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := limiter.wait(ctx, network.StreamBufferBytes); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := limiter.wait(ctx, 1000); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 90*time.Millisecond {
		t.Fatal("bandwidth ceiling exceeded")
	}
	blocked, end := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- limiter.wait(blocked, network.StreamBufferBytes) }()
	end()
	if err := <-done; err == nil {
		t.Fatal("quota wait ignored cancellation")
	}
}
func TestWANW04ConcurrentDuplicateLegAdmission(t *testing.T) {
	r := newRelayFixture(t, "peer_data")
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	var conns []net.Conn
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			c, err := r.ca.Attach(r.ctx, r.ta)
			if err == nil {
				mu.Lock()
				successes++
				conns = append(conns, c)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()
	if successes != 1 {
		t.Fatalf("same role admitted %d times", successes)
	}
}
func TestWANW04PurposeListenerIsolation(t *testing.T) {
	r := newRelayFixture(t, "enrollment")
	l := network.NewStreamListener()
	defer l.Close()
	if err := r.cb.OfferRelay(r.ctx, r.tb, network.PeerData, l); err == nil {
		t.Fatal("cross purpose listener")
	}
	a, b := r.legs(t)
	_ = a.Close()
	_ = b.Close()
	waitRelayEmpty(t, r.f)
}

func seededRelay(t *testing.T, f *fixture, index int, purpose string) (device, device, p.RelayAttachment, p.RelayAttachment) {
	t.Helper()
	a, b := identity(t, 1000+index*2, nil), identity(t, 1001+index*2, nil)
	q := p.NetworkProof{Profile: f.s.digest, Sender: a.id, SenderPin: a.pin, Target: b.id, TargetPin: b.pin, Purpose: purpose, Session: nonce()}
	ta := p.RelayAttachment{Profile: f.s.digest, Origin: f.s.relayOrigin, Epoch: f.s.epoch, Session: q.Session, Device: a.id, Pin: a.pin, Partner: b.id, PartnerPin: b.pin, Purpose: purpose, Role: "initiator", Expires: p.NetworkUint(f.now.Load() + 30)}
	tb := ta
	tb.Device = b.id
	tb.Pin = b.pin
	tb.Partner = a.id
	tb.PartnerPin = a.pin
	tb.Role = "responder"
	for _, token := range []*p.RelayAttachment{&ta, &tb} {
		canonical, err := token.Canonical(true)
		if err != nil {
			t.Fatal(err)
		}
		token.Signature = hex.EncodeToString(ed25519.Sign(f.s.key, canonical))
	}
	f.s.mu.Lock()
	f.s.sessions[q.Session] = &session{proof: q, accepted: true, expires: uint64(f.now.Load() + 30), attachments: map[string]p.RelayAttachment{"initiator": ta, "responder": tb}}
	f.s.mu.Unlock()
	return a, b, ta, tb
}
func relayResources() (uint64, int, int) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fds, _ := os.ReadDir("/proc/self/fd")
	return m.HeapAlloc, len(fds), runtime.NumGoroutine()
}
func TestWANW04ServiceCapacityResourcesAndEnrollmentIsolation(t *testing.T) {
	f := newFixture(t, true)
	baselineHeap, baselineFD, baselineG := relayResources()
	var clients []*network.ServiceClient
	var streams []net.Conn
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for i := 0; i < network.MaxServiceDataTunnels+MaxUnknownOffers; i++ {
		// Shared-source authentication has its own 128-burst/2-per-second
		// W03 ceiling. Pace additional keys rather than bypassing that bound.
		if i == network.MaxServiceDataTunnels {
			time.Sleep(3 * time.Second)
			f.now.Store(time.Now().Unix())
		}
		purpose := "peer_data"
		if i >= network.MaxServiceDataTunnels {
			purpose = "enrollment"
		}
		a, b, ta, tb := seededRelay(t, f, i, purpose)
		ca, cb := f.client(t, a, f.roots), f.client(t, b, f.roots)
		clients = append(clients, ca, cb)
		for _, leg := range []struct {
			c *network.ServiceClient
			t p.RelayAttachment
		}{{ca, ta}, {cb, tb}} {
			stream, err := leg.c.Attach(ctx, leg.t)
			if err != nil {
				t.Fatalf("session %d: %v", i, err)
			}
			streams = append(streams, stream)
			leg.c.CloseIdleConnections()
		}
	}
	heap, fd, g := relayResources()
	t.Logf("66 live tunnels: heap=%d delta=%d FDs=%d delta=%d goroutines=%d delta=%d", heap, heap-baselineHeap, fd, fd-baselineFD, g, g-baselineG)
	if heap > baselineHeap+(128<<20) || fd > baselineFD+600 || g > baselineG+2500 {
		t.Fatal("declared local resource budget exceeded")
	}
	a, _, ta, _ := seededRelay(t, f, 80, "peer_data")
	extra := f.client(t, a, f.roots)
	clients = append(clients, extra)
	if c, err := extra.Attach(ctx, ta); err == nil || err.Error() != p.NetworkQuota {
		if c != nil {
			_ = c.Close()
		}
		t.Fatal("data capacity not enforced", err)
	}
	a, _, ta, _ = seededRelay(t, f, 81, "enrollment")
	extra = f.client(t, a, f.roots)
	clients = append(clients, extra)
	if c, err := extra.Attach(ctx, ta); err == nil || err.Error() != p.NetworkQuota {
		if c != nil {
			_ = c.Close()
		}
		t.Fatal("enrollment capacity not enforced", err)
	}
	// Already admitted data progresses despite exhausted unknown/data admission.
	start := time.Now()
	streamed := int64(8 << 20)
	streamDone := make(chan error, 1)
	go func() {
		buf := make([]byte, network.StreamBufferBytes)
		for n := int64(0); n < streamed; n += int64(len(buf)) {
			if _, err := streams[0].Write(buf); err != nil {
				streamDone <- err
				return
			}
		}
		streamDone <- nil
	}()
	if _, err := io.CopyN(io.Discard, streams[1], streamed); err != nil {
		t.Fatal(err)
	}
	if err := <-streamDone; err != nil {
		t.Fatal(err)
	}
	streamHeap, streamFD, streamG := relayResources()
	t.Logf("streamed %d bytes in %s; heap=%d FDs=%d goroutines=%d", streamed, time.Since(start), streamHeap, streamFD, streamG)
	if streamHeap > baselineHeap+(128<<20) || streamFD > baselineFD+600 || streamG > baselineG+2500 {
		t.Fatal("streaming resource budget exceeded")
	}
	done := make(chan error, 1)
	go func() { _, err := streams[0].Write([]byte("live-data")); done <- err }()
	buf := make([]byte, 9)
	_ = streams[1].SetReadDeadline(time.Now().Add(time.Second))
	if _, err := io.ReadFull(streams[1], buf); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for _, c := range clients {
		_ = c.Close()
	}
	_ = extra.Close()
	_ = f.s.Close()
	_ = f.server.Close()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, fd, g = relayResources()
		if fd <= baselineFD+4 && g <= baselineG+8 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, fd, g = relayResources()
	t.Logf("joined: FDs=%d goroutines=%d", fd, g)
	if fd > baselineFD+4 || g > baselineG+8 {
		t.Fatal("relay workers or sockets leaked")
	}
}
func TestWANW04RestartEpochAndFreshChallenge(t *testing.T) {
	r := newRelayFixture(t, "peer_data")
	restarted, err := New(Options{Selection: r.f.selection, Origin: r.f.origin, ServiceKey: r.f.s.key, Now: r.f.s.now})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	// A valid old signed token cannot attach after a restart, even if a challenge
	// is issued by the new epoch and the same operator signing key is retained.
	q := r.f.proof(t, r.a, "authenticate", "peer_data", nonce(), nil)
	q.Kind = "attach"
	q.Origin = r.ta.Origin
	q.Target = r.b.id
	q.TargetPin = r.b.pin
	q.Session = r.ta.Session
	q.Role = "initiator"
	intent, _ := q.Intent(true)
	q.Payload = p.NetworkDigest(intent)
	sign(t, r.a, &q)
	req := p.NetworkRelayAttachRequest{Proof: q, Attachment: r.ta}
	restarted.mu.Lock()
	_, code := restarted.attach(req, uint64(r.f.now.Load()))
	restarted.mu.Unlock()
	if code == "" {
		t.Fatal("old epoch accepted")
	}
	r.f.s.mu.Lock()
	v, code := r.f.s.attach(req, uint64(r.f.now.Load()))
	r.f.s.mu.Unlock()
	if code != "" || v == nil {
		t.Fatal("valid proof rejected", code)
	}
	r.f.s.mu.Lock()
	_, code = r.f.s.attach(req, uint64(r.f.now.Load()))
	r.f.s.mu.Unlock()
	if code != p.NetworkReplay {
		t.Fatal("challenge reuse", code)
	}
}
func TestWANW04BandwidthReconnectRetention(t *testing.T) {
	r := newRelayFixture(t, "peer_data")
	a, b := r.legs(t)
	r.f.s.mu.Lock()
	bucket := r.f.s.deviceBandwidth[actor{r.a.id, r.a.pin}]
	r.f.s.mu.Unlock()
	_ = a.Close()
	_ = b.Close()
	waitRelayEmpty(t, r.f)
	r.f.s.mu.Lock()
	retained := r.f.s.deviceBandwidth[actor{r.a.id, r.a.pin}]
	r.f.s.mu.Unlock()
	if retained != bucket {
		t.Fatal("reconnect resets device quota")
	}
	bucket.mu.Lock()
	bucket.at = time.Now().Add(-61 * time.Second)
	bucket.mu.Unlock()
	r.f.s.mu.Lock()
	r.f.s.expireBandwidth()
	retained = r.f.s.deviceBandwidth[actor{r.a.id, r.a.pin}]
	r.f.s.mu.Unlock()
	if retained != nil {
		t.Fatal("idle quota key unbounded")
	}
}

type recordedRelayLeg struct {
	net.Conn
	mu       sync.Mutex
	captured bytes.Buffer
}

func (c *recordedRelayLeg) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.mu.Lock()
	_, _ = c.captured.Write(b[:n])
	c.mu.Unlock()
	return n, err
}
func TestWANW04BrokerSeesOnlyInnerTLSCiphertext(t *testing.T) {
	r := newRelayFixture(t, "peer_data")
	a, err := r.ca.Attach(r.ctx, r.ta)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	r.f.s.mu.Lock()
	relay := r.f.s.sessions[r.ta.Session].relay
	captured := &recordedRelayLeg{Conn: relay.legs["initiator"]}
	relay.legs["initiator"] = captured
	r.f.s.mu.Unlock()
	b, err := r.cb.Attach(r.ctx, r.tb)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	client := tls.Client(a, relayTestTrust(t, r.a, r.b))
	server := tls.Server(b, &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{r.b.cert}, ClientAuth: tls.RequireAnyClientCert})
	done := make(chan error, 1)
	secret := []byte("PRIVATE-SYNTHETIC-FILENAME invitation-secret-token PRIVATE-SYNTHETIC-CONTENT")
	go func() {
		if err := server.HandshakeContext(r.ctx); err != nil {
			done <- err
			return
		}
		got := make([]byte, len(secret))
		_, err := io.ReadFull(server, got)
		if err == nil && !bytes.Equal(got, secret) {
			err = errors.New("inner bytes differ")
		}
		done <- err
	}()
	if err = client.HandshakeContext(r.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Write(secret); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	captured.mu.Lock()
	defer captured.mu.Unlock()
	if captured.captured.Len() == 0 {
		t.Fatal("empty broker inspection")
	}
	for _, s := range [][]byte{[]byte("PRIVATE-SYNTHETIC-FILENAME"), []byte("invitation-secret-token"), []byte("PRIVATE-SYNTHETIC-CONTENT")} {
		if bytes.Contains(captured.captured.Bytes(), s) {
			t.Fatal("plaintext crossed broker boundary")
		}
	}
	t.Logf("inspected %d inner-TLS bytes at decrypted outer-WSS boundary", captured.captured.Len())
}
func relayTestTrust(t *testing.T, a, b device) *tls.Config {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(b.cert.Leaf)
	// Ordinary hostname/root verification plus the exact synthetic peer pin.
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{a.cert}, RootCAs: roots, ServerName: "synthetic.orbit.invalid", VerifyConnection: func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 || hex.EncodeToString(sha256Sum(cs.PeerCertificates[0].RawSubjectPublicKeyInfo)) != b.pin {
			return errors.New("pin mismatch")
		}
		return nil
	}}
}
func sha256Sum(b []byte) []byte { h := sha256.Sum256(b); return h[:] }

func relayIdentity(t *testing.T, id int) device {
	t.Helper()
	d := identity(t, id, nil)
	template := *d.cert.Leaf
	template.DNSNames = []string{"synthetic.orbit.invalid"}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, d.key.Public(), d.key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	d.cert.Certificate = [][]byte{der}
	d.cert.Leaf = leaf
	d.der = base64.StdEncoding.EncodeToString(der)
	return d
}

func TestWANW04PostAuthenticationFrameBounds(t *testing.T) {
	for _, kind := range []string{"maximum", "one-over", "text"} {
		t.Run(kind, func(t *testing.T) {
			r := newRelayFixture(t, "peer_data")
			q := r.f.proof(t, r.a, "authenticate", "peer_data", nonce(), nil)
			q.Kind = "attach"
			q.Origin = r.ta.Origin
			q.Target = r.b.id
			q.TargetPin = r.b.pin
			q.Session = r.ta.Session
			q.Role = "initiator"
			intent, _ := q.Intent(true)
			q.Payload = p.NetworkDigest(intent)
			sign(t, r.a, &q)
			transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: r.f.roots}}
			defer transport.CloseIdleConnections()
			ws, _, err := websocket.Dial(r.ctx, r.ta.Origin+"/network/v1/relay", &websocket.DialOptions{HTTPClient: &http.Client{Transport: transport}, CompressionMode: websocket.CompressionDisabled})
			if err != nil {
				t.Fatal(err)
			}
			defer ws.CloseNow()
			auth, _ := json.Marshal(p.NetworkRelayAttachRequest{Proof: q, Attachment: r.ta})
			if err = ws.Write(r.ctx, websocket.MessageText, auth); err != nil {
				t.Fatal(err)
			}
			_, ack, err := ws.Read(r.ctx)
			var result p.NetworkResult
			if err != nil || p.NetworkDecode(ack, &result) != nil || result.State != "accepted" {
				t.Fatal("authentication", err)
			}
			b, err := r.cb.Attach(r.ctx, r.tb)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			size := network.StreamBufferBytes
			typ := websocket.MessageBinary
			if kind == "one-over" {
				size++
			}
			if kind == "text" {
				typ = websocket.MessageText
				size = 1
			}
			sent := make(chan error, 1)
			go func() { sent <- ws.Write(r.ctx, typ, make([]byte, size)) }()
			_ = b.SetReadDeadline(time.Now().Add(time.Second))
			got := make([]byte, size)
			_, err = io.ReadFull(b, got)
			if kind == "maximum" {
				if err != nil {
					t.Fatal("maximum frame", err)
				}
			} else {
				if err == nil {
					t.Fatal("invalid frame forwarded")
				}
				waitRelayEmpty(t, r.f)
			}
			_ = ws.CloseNow()
			<-sent
		})
	}
}

func TestWANW04PerPairAdmissionCannotBeBypassedByNewClient(t *testing.T) {
	r := newRelayFixture(t, "peer_data")
	first, err := r.ca.Attach(r.ctx, r.ta)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	for i := 0; i < 2; i++ {
		token := r.ta
		token.Session = nonce()
		canonical, _ := token.Canonical(true)
		token.Signature = hex.EncodeToString(ed25519.Sign(r.f.s.key, canonical))
		q := p.NetworkProof{Profile: r.f.s.digest, Sender: r.a.id, SenderPin: r.a.pin, Target: r.b.id, TargetPin: r.b.pin, Purpose: "peer_data", Session: token.Session}
		r.f.s.mu.Lock()
		r.f.s.sessions[q.Session] = &session{proof: q, accepted: true, expires: uint64(token.Expires), attachments: map[string]p.RelayAttachment{"initiator": token}}
		r.f.s.mu.Unlock()
		client := r.f.client(t, r.a, r.f.roots)
		c, err := client.Attach(r.ctx, token)
		if i == 0 {
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
		} else {
			if err == nil || err.Error() != p.NetworkQuota {
				if c != nil {
					_ = c.Close()
				}
				t.Fatal("new client bypassed pair limit", err)
			}
			r.f.s.mu.Lock()
			_, retained := r.f.s.sessions[token.Session]
			r.f.s.mu.Unlock()
			if retained {
				t.Fatal("refused reservation retained")
			}
		}
	}
}
