package rendezvous

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/network"
	p "github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/model"
	"github.com/coder/websocket"
)

type fixture struct {
	s         *Service
	selection network.ProfileSelection
	now       atomic.Int64
	roots     *x509.CertPool
	server    *http.Server
	origin    string
}
type device struct {
	id, pin, der string
	key          ed25519.PrivateKey
	cert         tls.Certificate
}

func identity(t *testing.T, id int, host net.IP) device {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(int64(id + 1)), Subject: pkix.Name{CommonName: "synthetic"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, BasicConstraintsValid: true, IsCA: true}
	if host != nil {
		template.IPAddresses = []net.IP{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return device{fmt.Sprintf("%064x", id+1), hex.EncodeToString(pin[:]), base64.StdEncoding.EncodeToString(der), key, tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: cert}}
}
func newFixture(t *testing.T, live bool) *fixture {
	t.Helper()
	f := &fixture{origin: "https://directory.orbit.invalid"}
	f.now.Store(time.Now().Unix())
	var listener net.Listener
	var serviceCert device
	if live {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			t.Fatal(err)
		}
		var host net.IP
		for _, a := range addrs {
			ip, _, _ := net.ParseCIDR(a.String())
			if ip != nil && ip.To4() != nil && ip.IsPrivate() && !ip.IsLoopback() {
				host = ip
				break
			}
		}
		if host == nil {
			t.Fatal("test requires a private nonloopback development interface")
		}
		listener, err = net.Listen("tcp", net.JoinHostPort(host.String(), "0"))
		if err != nil {
			t.Fatal(err)
		}
		f.origin = "https://" + listener.Addr().String()
		serviceCert = identity(t, 9000, host)
		f.roots = x509.NewCertPool()
		f.roots.AddCert(serviceCert.cert.Leaf)
	}
	authority, signer, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	online, onlinePrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profile := p.NetworkProfile{Version: "1", Operator: "Local development fixture", Authority: hex.EncodeToString(authority), ServiceKey: hex.EncodeToString(online), Epoch: 1, Expires: p.NetworkUint(f.now.Load() + 3600), Origins: []string{f.origin, "wss://" + strings.TrimPrefix(f.origin, "https://")}, STUN: []string{}, Privacy: "Development only; routing metadata is ephemeral and never logged; records expire within 600 seconds."}
	b, err := profile.Canonical(true)
	if err != nil {
		t.Fatal(err)
	}
	profile.Signature = hex.EncodeToString(ed25519.Sign(signer, b))
	f.selection = network.ProfileSelection{Profile: profile, Authority: profile.Authority, HighestEpoch: 1, Environment: "development"}
	f.s, err = New(Options{Selection: f.selection, Origin: f.origin, ServiceKey: onlinePrivate, Now: func() time.Time { return time.Unix(f.now.Load(), 0) }})
	if err != nil {
		t.Fatal(err)
	}
	if live {
		f.server = f.s.Server()
		f.server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serviceCert.cert}, NextProtos: []string{"http/1.1"}}
		go func() { _ = f.server.ServeTLS(BoundedListener(listener), "", "") }()
	}
	t.Cleanup(func() {
		_ = f.s.Close()
		if f.server != nil {
			_ = f.server.Close()
		}
	})
	return f
}
func (f *fixture) client(t *testing.T, d device, roots *x509.CertPool) *network.ServiceClient {
	t.Helper()
	c, err := network.NewServiceClient(network.ServiceClientOptions{Selection: f.selection, Origin: f.origin, Device: d.id, Certificate: d.cert, Roots: roots, Now: func() time.Time { return time.Unix(f.now.Load(), 0) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
func (f *fixture) call(t *testing.T, path string, in, out any) (int, string) {
	t.Helper()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	return f.raw(t, path, b, out)
}
func (f *fixture) raw(t *testing.T, path string, b []byte, out any) (int, string) {
	t.Helper()
	r := httptest.NewRequest("POST", f.origin+"/network/v1/"+path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	r.TLS = &tls.ConnectionState{}
	r.RemoteAddr = "192.168.1.10:12345"
	w := httptest.NewRecorder()
	f.s.ServeHTTP(w, r)
	if w.Code != 200 {
		var fail p.NetworkFailure
		if err := p.NetworkDecode(w.Body.Bytes(), &fail); err != nil {
			t.Fatal(err, w.Body.String())
		}
		return w.Code, fail.Code
	}
	if out != nil {
		if err := p.NetworkDecode(w.Body.Bytes(), out); err != nil {
			t.Fatal(err, w.Body.String())
		}
	}
	return w.Code, ""
}
func (f *fixture) proof(t *testing.T, d device, kind, purpose, operation string, a *p.NetworkAnnouncement) p.NetworkProof {
	t.Helper()
	var ch p.NetworkChallengeResult
	if status, code := f.call(t, "challenge", p.NetworkChallengeRequest{Version: "1", Profile: f.s.digest, Sender: d.id, SenderPin: d.pin, CertificateDER: d.der}, &ch); status != 200 {
		t.Fatal("challenge", code)
	}
	q := p.NetworkProof{Version: "1", Kind: kind, Profile: f.s.digest, Origin: f.origin, Sender: d.id, SenderPin: d.pin, Purpose: purpose, Challenge: ch.Challenge, Operation: operation, Expires: ch.Expires, CertificateDER: d.der}
	if operation == "" {
		q.Operation = nonce()
	}
	var b []byte
	var err error
	if a != nil {
		q.Generation = a.Generation
		b, err = a.Canonical(uint64(f.now.Load()), true)
	} else {
		if kind == "lookup" {
			q.Target = d.id
			q.TargetPin = d.pin
		}
		b, err = q.Intent(true)
	}
	if err != nil {
		t.Fatal(err)
	}
	q.Payload = p.NetworkDigest(b)
	sign(t, d, &q)
	return q
}
func sign(t *testing.T, d device, q *p.NetworkProof) {
	t.Helper()
	b, err := q.Canonical(true)
	if err != nil {
		t.Fatal(err)
	}
	q.Signature = hex.EncodeToString(ed25519.Sign(d.key, b))
}
func announcement(f *fixture, g uint64) p.NetworkAnnouncement {
	return p.NetworkAnnouncement{Generation: p.NetworkUint(g), Expires: p.NetworkUint(f.now.Load() + 600), Candidates: []p.NetworkCandidate{{Transport: "tcp", Address: "8.8.8.8:443", Scope: "public"}}, Capabilities: []string{"direct_https_v1"}, Relay: false}
}
func (f *fixture) announce(t *testing.T, d device, a p.NetworkAnnouncement, op string) (p.NetworkProof, string) {
	q := f.proof(t, d, "announce", "peer_data", op, &a)
	_, code := f.call(t, "announce", p.NetworkAnnounceRequest{Proof: q, Announcement: a}, &p.NetworkResult{})
	return q, code
}

func TestWANW03RegistrationAndReplay(t *testing.T) {
	f := newFixture(t, false)
	d := identity(t, 1, nil)
	a := announcement(f, 1)
	q, code := f.announce(t, d, a, nonce())
	if code != "" {
		t.Fatal(code)
	}
	if _, code = f.call(t, "announce", p.NetworkAnnounceRequest{Proof: q, Announcement: a}, nil); code != p.NetworkReplay {
		t.Fatal("reused nonce", code)
	}
	_, code = f.announce(t, d, a, q.Operation)
	if code != "" {
		t.Fatal("lost-response replay", code)
	}
	changed := a
	changed.Relay = true
	_, code = f.announce(t, d, changed, q.Operation)
	if code != p.NetworkIdempotencyConflict {
		t.Fatal(code)
	}
	_, code = f.announce(t, d, a, nonce())
	if code != p.NetworkStaleGeneration {
		t.Fatal(code)
	}
	impostor := identity(t, 1, nil)
	_, code = f.announce(t, impostor, announcement(f, 2), nonce())
	if code != "" {
		t.Fatal(code)
	}
	if f.s.records[routeKey{actor{d.id, d.pin}, "peer_data"}].proof.SenderPin != d.pin || len(f.s.records) != 2 {
		t.Fatal("fake same-ID key replaced pinned route")
	}
	oracle := model.NewWANAdmission("epoch")
	who := model.WANIdentity{Device: d.id, Pin: d.pin}
	if oracle.Challenge("a", who, uint64(f.now.Load())) != "accepted" || oracle.Announce(who, "a", "operation", "payload", 1, uint64(a.Expires), uint64(f.now.Load()), true, true) != "accepted" {
		t.Fatal("model mismatch")
	}
}
func TestWANW03AuthenticationBindings(t *testing.T) {
	for _, test := range []string{"signature", "key", "origin", "profile", "purpose", "expired", "path", "payload"} {
		t.Run(test, func(t *testing.T) {
			f := newFixture(t, false)
			d := identity(t, 1, nil)
			a := announcement(f, 1)
			q := f.proof(t, d, "announce", "peer_data", nonce(), &a)
			path := "announce"
			switch test {
			case "signature":
				q.Signature = strings.Repeat("1", 128)
			case "key":
				q.CertificateDER = identity(t, 2, nil).der
			case "origin":
				q.Origin = "https://other.orbit.invalid"
				sign(t, d, &q)
			case "profile":
				q.Profile = strings.Repeat("1", 64)
				sign(t, d, &q)
			case "purpose":
				q.Purpose = "owner_control"
			case "expired":
				f.now.Add(60)
			case "path":
				path = "offer"
			case "payload":
				a.Relay = true
			}
			if status, _ := f.call(t, path, p.NetworkAnnounceRequest{Proof: q, Announcement: a}, nil); status == 200 {
				t.Fatal("accepted invalid exchange")
			}
			if len(f.s.records) != 0 {
				t.Fatal("invalid mutation allocated route")
			}
		})
	}
	f := newFixture(t, false)
	if status, _ := f.raw(t, "lookup", []byte(`{"proof":{}}`), nil); status == 200 {
		t.Fatal("unauthenticated lookup")
	}
}
func TestWANW03LeaseExpiryAndRestart(t *testing.T) {
	f := newFixture(t, true)
	a, b := identity(t, 1, nil), identity(t, 2, nil)
	ca, cb := f.client(t, a, f.roots), f.client(t, b, f.roots)
	lease := announcement(f, 1)
	if err := ca.Announce(context.Background(), "peer_data", nonce(), lease); err != nil {
		t.Fatal(err)
	}
	f.now.Add(61) // Authentication expires; the candidate's signed lease remains valid.
	got, found, err := cb.Lookup(context.Background(), a.id, a.pin, "peer_data", 1)
	if err != nil || !found || got.Generation != 1 {
		t.Fatal(got, found, err)
	}
	if _, found, err = cb.Lookup(context.Background(), a.id, a.pin, "enrollment", 1); err != nil || found {
		t.Fatal("cross-purpose lookup", found, err)
	}
	if _, _, err = cb.Lookup(context.Background(), a.id, a.pin, "peer_data", 2); err == nil {
		t.Fatal("stale generation accepted")
	}
	f.now.Add(539)
	if _, found, err = cb.Lookup(context.Background(), a.id, a.pin, "peer_data", 1); err != nil || found {
		t.Fatal("lease expiry", found, err)
	}
	before := f.s.epoch
	service, err := New(Options{Selection: f.selection, Origin: f.origin, ServiceKey: f.s.key, Now: func() time.Time { return time.Unix(f.now.Load(), 0) }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if service.epoch == before || len(service.records) != 0 || len(service.challenges) != 0 {
		t.Fatal("restart retained ephemeral trust")
	}
}
func TestWANW03CandidateAndBodyBounds(t *testing.T) {
	for _, address := range []string{"127.0.0.1:1", "10.0.0.1:443", "169.254.1.1:80", "100.64.0.1:80", "[::1]:443", "[::ffff:8.8.8.8]:443", "0.0.0.0:443"} {
		t.Run(address, func(t *testing.T) {
			f := newFixture(t, false)
			d := identity(t, 1, nil)
			a := announcement(f, 1)
			q := f.proof(t, d, "announce", "peer_data", nonce(), &a)
			a.Candidates[0].Address = address
			if status, _ := f.call(t, "announce", p.NetworkAnnounceRequest{Proof: q, Announcement: a}, nil); status == 200 || len(f.s.records) != 0 {
				t.Fatal("unsafe candidate admitted")
			}
		})
	}
	f := newFixture(t, false)
	d := identity(t, 1, nil)
	a := announcement(f, 1)
	a.Candidates = []p.NetworkCandidate{}
	for i := 0; i < 16; i++ {
		a.Candidates = append(a.Candidates, p.NetworkCandidate{Transport: "tcp", Address: fmt.Sprintf("8.8.8.8:%d", i+1), Scope: "public"})
	}
	q := f.proof(t, d, "announce", "peer_data", nonce(), &a)
	body, _ := json.Marshal(p.NetworkAnnounceRequest{Proof: q, Announcement: a})
	body = append(body, bytes.Repeat([]byte(" "), p.NetworkMaxBytes-len(body))...)
	if status, code := f.raw(t, "announce", body, &p.NetworkResult{}); status != 200 {
		t.Fatal("maximum message", code)
	}
	if status, _ := f.raw(t, "announce", append(body, ' '), nil); status == 200 {
		t.Fatal("oversized message")
	}
	a.Candidates = append(a.Candidates, p.NetworkCandidate{Transport: "tcp", Address: "8.8.8.8:17", Scope: "public"})
	if status, _ := f.call(t, "announce", p.NetworkAnnounceRequest{Proof: q, Announcement: a}, nil); status == 200 {
		t.Fatal("oversized candidates")
	}
}
func TestWANW03SharedNATAndCacheAdmission(t *testing.T) {
	f := newFixture(t, false)
	// Twenty independent keys behind the exact same source IP pass; a key has its
	// own challenge/rate ceiling and cannot monopolize all challenge slots.
	for i := 0; i < 20; i++ {
		d := identity(t, i, nil)
		if _, code := f.announce(t, d, announcement(f, 1), nonce()); code != "" {
			t.Fatal("shared NAT", i, code)
		}
	}
	d := identity(t, 100, nil)
	for i := 0; i < 3; i++ {
		var ch p.NetworkChallengeResult
		status, code := f.call(t, "challenge", p.NetworkChallengeRequest{Version: "1", Profile: f.s.digest, Sender: d.id, SenderPin: d.pin, CertificateDER: d.der}, &ch)
		if (i < 2 && status != 200) || (i == 2 && code != p.NetworkQuota) {
			t.Fatal("per-key challenge admission", i, code)
		}
	}
	f.now.Add(61)
	f.s.mu.Lock()
	f.s.expire(uint64(f.now.Load()))
	f.s.mu.Unlock()
	if len(f.s.challenges) != 0 {
		t.Fatal("challenge cleanup")
	}
	// A saturated cache refuses new mutation without evicting replay protection.
	f.s.mu.Lock()
	f.s.operations = map[string]replay{}
	for i := 0; i < network.MaxReplayEntries; i++ {
		f.s.operations[fmt.Sprint(i)] = replay{semantic: "synthetic bounded cache entry", expires: uint64(f.now.Load() + 300)}
	}
	f.s.mu.Unlock()
	_, code := f.announce(t, d, announcement(f, 1), nonce())
	if code != p.NetworkQuota || len(f.s.operations) != network.MaxReplayEntries {
		t.Fatal("cache bound", code)
	}
	f.now.Add(300)
	_, code = f.announce(t, d, announcement(f, 1), nonce())
	if code != "" || len(f.s.operations) != 1 {
		t.Fatal("cache expiry", code)
	}
}
func TestWANW03MaximumRoutesAndChallenges(t *testing.T) {
	f := newFixture(t, false)
	// Use actual verified HTTP mutations; a short clock advance refills the global
	// and shared-source burst without expiring the ten-minute route leases.
	for i := 0; i < MaxRecords+1; i++ {
		if i%20 == 0 {
			f.now.Add(20)
		}
		d := identity(t, i, nil)
		_, code := f.announce(t, d, announcement(f, 1), nonce())
		if (i < MaxRecords && code != "") || (i == MaxRecords && code != p.NetworkQuota) {
			t.Fatal("route bound", i, code)
		}
	}
	if len(f.s.records) != MaxRecords {
		t.Fatal("maximum route cache")
	}
	f.now.Add(600)
	f.s.mu.Lock()
	f.s.expire(uint64(f.now.Load()))
	f.s.mu.Unlock()
	if len(f.s.records) != 0 {
		t.Fatal("route cleanup")
	}
	// Fill the challenge table at one timestamp with independently certified actors.
	f.s.mu.Lock()
	for i := 0; i < 128; i++ {
		d := identity(t, 1000+i, nil)
		_, code := f.s.challenge(p.NetworkChallengeRequest{Version: "1", Profile: f.s.digest, Sender: d.id, SenderPin: d.pin, CertificateDER: d.der}, uint64(f.now.Load()))
		if code != "" {
			t.Fatal(code)
		}
	}
	d := identity(t, 2000, nil)
	_, code := f.s.challenge(p.NetworkChallengeRequest{Version: "1", Profile: f.s.digest, Sender: d.id, SenderPin: d.pin, CertificateDER: d.der}, uint64(f.now.Load()))
	f.s.mu.Unlock()
	if code != p.NetworkQuota || len(f.s.challenges) != 128 {
		t.Fatal("global challenge ceiling", code)
	}
}
func TestWANW03ControlOffersAndReservationIntent(t *testing.T) {
	f := newFixture(t, true)
	a, b := identity(t, 1, nil), identity(t, 2, nil)
	ca, cb := f.client(t, a, f.roots), f.client(t, b, f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, c := range []*network.ServiceClient{ca, cb} {
		if err := c.Announce(ctx, "peer_data", nonce(), announcement(f, 1)); err != nil {
			t.Fatal(err)
		}
	}
	ac, err := ca.Control(ctx, "peer_data", map[string]string{b.id: b.pin}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ac.Close()
	bc, err := cb.Control(ctx, "peer_data", map[string]string{a.id: a.pin}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer bc.Close()
	sessionID := nonce()
	intent := p.NetworkProof{Target: b.id, TargetPin: b.pin, Purpose: "peer_data", Session: sessionID, Role: "initiator"}
	if _, err = ca.Reserve(ctx, intent); err == nil {
		t.Fatal("reservation without partner acceptance")
	}
	offer := p.NetworkOffer{SenderGeneration: 1, TargetGeneration: 1, Candidates: []p.NetworkCandidate{}}
	intent.Kind = "offer"
	intent.Operation = nonce()
	if err = ca.Exchange(ctx, intent, offer); err != nil {
		t.Fatal(err)
	}
	event, err := bc.Next(ctx)
	if err != nil || event.Proof.Sender != a.id || event.Proof.TargetPin != b.pin {
		t.Fatal("addressed event", err)
	}
	if _, err = ca.Reserve(ctx, intent); err == nil {
		t.Fatal("unaccepted offer reservation")
	}
	accept := p.NetworkProof{Kind: "accept", Target: a.id, TargetPin: a.pin, Purpose: "peer_data", Session: sessionID, Role: "responder", Operation: nonce()}
	if err = cb.Exchange(ctx, accept, offer); err != nil {
		t.Fatal(err)
	}
	if _, err = ac.Next(ctx); err != nil {
		t.Fatal(err)
	}
	// reserve uses a different kind's operation namespace; each exact leg gets its
	// own signed short-lived credential. Changed peer/purpose cannot release it.
	intent.Operation = nonce()
	token, err := ca.Reserve(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	if token.Epoch != f.s.epoch || token.PartnerPin != b.pin || token.Role != "initiator" || uint64(token.Expires) > uint64(f.now.Load()+30) {
		t.Fatal("credential binding")
	}
	other, err := cb.Reserve(ctx, accept)
	if err != nil || other.Role != "responder" {
		t.Fatal("responder reservation", err)
	}
	wrong := intent
	wrong.Purpose = "enrollment"
	wrong.Operation = nonce()
	if err = ca.Release(ctx, wrong); err == nil {
		t.Fatal("cross-purpose release")
	}
	if err = ca.Release(ctx, intent); err != nil {
		t.Fatal(err)
	}
	if _, err = cb.Reserve(ctx, p.NetworkProof{Target: a.id, TargetPin: a.pin, Purpose: "peer_data", Session: sessionID, Role: "responder"}); err == nil {
		t.Fatal("released reservation survived")
	}
}
func TestWANW03UnknownEnrollmentIsSeparate(t *testing.T) {
	f := newFixture(t, true)
	receiver := identity(t, 1, nil)
	rc := f.client(t, receiver, f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := rc.Announce(ctx, "enrollment", nonce(), announcement(f, 1)); err != nil {
		t.Fatal(err)
	}
	control, err := rc.Control(ctx, "enrollment", map[string]string{}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	for i := 0; i < 3; i++ {
		d := identity(t, 10+i, nil)
		c := f.client(t, d, f.roots)
		err = c.Exchange(ctx, p.NetworkProof{Kind: "offer", Target: receiver.id, TargetPin: receiver.pin, Purpose: "enrollment", Session: nonce(), Role: "initiator"}, p.NetworkOffer{SenderGeneration: 1, TargetGeneration: 1, Candidates: []p.NetworkCandidate{}})
		if i < 2 {
			if err != nil {
				t.Fatal(err)
			}
			event, err := control.Next(ctx)
			if err != nil || event.Proof.Sender != d.id {
				t.Fatal(err)
			}
		} else if err == nil || err.Error() != p.NetworkQuota {
			t.Fatal("unknown enrollment ceiling", err)
		}
	}
	// Service presence authenticates routing only; it advertises no v3 handlers,
	// no relay availability and has no folder or owner-control path.
	if status, _ := f.raw(t, "../../control/v1/status", []byte(`{}`), nil); status == 200 {
		t.Fatal("control handler exposed")
	}
}
func TestWANW03ControlFloodAndShutdown(t *testing.T) {
	f := newFixture(t, true)
	d := identity(t, 1, nil)
	c := f.client(t, d, f.roots)
	ctx := context.Background()
	ctrl, err := c.Control(ctx, "peer_data", map[string]string{}, false)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var accepted atomic.Int64
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := c.Lookup(ctx, d.id, d.pin, "peer_data", 0)
			if err == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	f.s.mu.Lock()
	if len(f.s.controls) != 1 || len(f.s.challenges) > 128 || len(f.s.rates) > 1024 || len(f.s.sources) > 128 {
		t.Fatal("flood exceeded finite tables")
	}
	f.s.mu.Unlock()
	done := make(chan struct{})
	go func() { _ = c.Close(); _ = f.s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bounded shutdown failed")
	}
	if _, err = ctrl.Next(ctx); err == nil {
		t.Fatal("closed control returned event")
	}
	t.Logf("200 concurrent lookups; %d admitted completions; client slots <=4, controls=1; shutdown joined", accepted.Load())
}
func TestWANW03TLSAndHeaderOrigins(t *testing.T) {
	f := newFixture(t, true)
	d := identity(t, 1, nil)
	untrusted := f.client(t, d, nil)
	if _, _, err := untrusted.Lookup(context.Background(), d.id, d.pin, "peer_data", 0); err == nil {
		t.Fatal("untrusted outer TLS bypassed")
	}
	for _, header := range []string{"Origin", "Forwarded", "X-Forwarded-For", "X-Forwarded-Host"} {
		r := httptest.NewRequest("POST", f.origin+"/network/v1/challenge", strings.NewReader(`{}`))
		r.TLS = &tls.ConnectionState{}
		r.Header.Set(header, "synthetic")
		w := httptest.NewRecorder()
		f.s.ServeHTTP(w, r)
		if w.Code == 200 {
			t.Fatal("header accepted", header)
		}
	}
	r := httptest.NewRequest("POST", f.origin+"/network/v1/challenge", strings.NewReader(`{}`))
	r.TLS = &tls.ConnectionState{}
	validBody, _ := json.Marshal(p.NetworkChallengeRequest{Version: "1", Profile: f.s.digest, Sender: d.id, SenderPin: d.pin, CertificateDER: d.der})
	dupRequest := httptest.NewRequest("POST", f.origin+"/network/v1/challenge", bytes.NewReader(validBody))
	dupRequest.TLS = &tls.ConnectionState{}
	dupRequest.Header.Set("Content-Type", "application/json")
	dupRequest.Header["Origin"] = []string{"", "https://browser.invalid"}
	duplicate := httptest.NewRecorder()
	f.s.ServeHTTP(duplicate, dupRequest)
	if duplicate.Code == 200 {
		t.Fatal("later nonempty Origin accepted on otherwise valid challenge")
	}
	validRequest := httptest.NewRequest("POST", f.origin+"/network/v1/challenge", bytes.NewReader(validBody))
	validRequest.TLS = &tls.ConnectionState{}
	validRequest.Header.Set("Content-Type", "application/json")
	validResponse := httptest.NewRecorder()
	f.s.ServeHTTP(validResponse, validRequest)
	if validResponse.Code != 200 {
		t.Fatal("valid control challenge baseline failed")
	}
	r.Host = "wrong.origin"
	w := httptest.NewRecorder()
	f.s.ServeHTTP(w, r)
	if w.Code == 200 {
		t.Fatal("wrong HTTP host")
	}
	// Actual server header admission must reject over-limit input before the handler.
	conn, err := tls.Dial("tcp", strings.TrimPrefix(f.origin, "https://"), &tls.Config{RootCAs: f.roots, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	_, _ = fmt.Fprintf(conn, "POST /network/v1/challenge HTTP/1.1\r\nHost: %s\r\nX-Large: %s\r\nContent-Length: 2\r\n\r\n{}", strings.TrimPrefix(f.origin, "https://"), strings.Repeat("a", 32<<10))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || !bytes.Contains(buf[:n], []byte("431")) {
		t.Fatal("header bound", string(buf[:n]), err)
	}
}
func TestWANW03ProfilePersistenceAndRollback(t *testing.T) {
	f := newFixture(t, false)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveNetworkProfile(dir, f.selection, uint64(f.now.Load())); err != nil {
		t.Fatal(err)
	}
	got, err := config.LoadNetworkProfile(dir, uint64(f.now.Load()))
	if err != nil || got.Environment != "development" || got.Authority != f.selection.Authority {
		t.Fatal(got, err)
	}
	info, err := os.Stat(dir + "/network-profile.json")
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private profile", err)
	}
	altered := got
	altered.Profile.Operator = "untrusted replacement"
	if err = config.SaveNetworkProfile(dir, altered, uint64(f.now.Load())); err == nil {
		t.Fatal("altered profile accepted")
	}
	changed := got
	changed.Environment = "release"
	if err = config.SaveNetworkProfile(dir, changed, uint64(f.now.Load())); err == nil {
		t.Fatal("development profile promoted without review")
	}
	if _, err = config.LoadNetworkProfile(dir, uint64(got.Profile.Expires)); err == nil {
		t.Fatal("expired persisted profile")
	}
	if policy, err := config.LoadNetworkPolicy(dir); err != nil || policy.Mode != "manual" {
		t.Fatal("saving a profile activated public traffic", err)
	}
}

// Interface assertion documents the resolver seam used by the client tests.
var _ interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
} = net.DefaultResolver

func TestWANW03IdleSocketAdmissionAndClose(t *testing.T) {
	f := newFixture(t, true)
	before := runtime.NumGoroutine()
	sockets := make([]net.Conn, 0, MaxConnections+16)
	for i := 0; i < MaxConnections+16; i++ {
		conn, err := net.DialTimeout("tcp", strings.TrimPrefix(f.origin, "https://"), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		sockets = append(sockets, conn)
	}
	defer func() {
		for _, c := range sockets {
			_ = c.Close()
		}
	}()
	// No TLS bytes: finite pre-handler sockets, no uncontrolled read workers.
	time.Sleep(50 * time.Millisecond)
	delta := runtime.NumGoroutine() - before
	if delta > MaxConnections+32 {
		t.Fatal("unbounded idle socket goroutines", delta)
	}
	if err := f.server.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before+8 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if runtime.NumGoroutine() > before+8 {
		t.Fatal("idle sockets escaped server shutdown")
	}
	t.Logf("%d raw sockets; goroutine delta=%d (ceiling %d); server close joins idle TLS work", len(sockets), delta, MaxConnections)
}
func TestWANW03ProfilesRejectRollbackAndAuthority(t *testing.T) {
	f := newFixture(t, false)
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profile := f.selection.Profile
	profile.Authority = hex.EncodeToString(pub)
	profile.Epoch = 2
	b, err := profile.Canonical(true)
	if err != nil {
		t.Fatal(err)
	}
	profile.Signature = hex.EncodeToString(ed25519.Sign(key, b))
	selection, err := network.ReviewProfile(nil, profile, profile.Authority, "development", uint64(f.now.Load()))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	if err = config.SaveNetworkProfile(dir, selection, uint64(f.now.Load())); err != nil {
		t.Fatal(err)
	}
	lower := profile
	lower.Epoch = 1
	b, _ = lower.Canonical(true)
	lower.Signature = hex.EncodeToString(ed25519.Sign(key, b))
	rollback := selection
	rollback.Profile = lower
	rollback.HighestEpoch = 1
	if err = config.SaveNetworkProfile(dir, rollback, uint64(f.now.Load())); err == nil {
		t.Fatal("valid signed rollback accepted")
	}
	fork := selection
	fork.Profile.Privacy = "altered same-epoch signed metadata"
	b, _ = fork.Profile.Canonical(true)
	fork.Profile.Signature = hex.EncodeToString(ed25519.Sign(key, b))
	if err = config.SaveNetworkProfile(dir, fork, uint64(f.now.Load())); err == nil {
		t.Fatal("same-epoch fork accepted")
	}
	if _, err = network.ReviewProfile(&selection, f.selection.Profile, f.selection.Authority, "development", uint64(f.now.Load())); err == nil {
		t.Fatal("self-supplied authority replaced trusted authority")
	}
	updated := selection
	updated.Profile.Epoch = 3
	updated.HighestEpoch = 3
	b, _ = updated.Profile.Canonical(true)
	updated.Profile.Signature = hex.EncodeToString(ed25519.Sign(key, b))
	if err = config.SaveNetworkProfile(dir, updated, uint64(f.now.Load())); err != nil {
		t.Fatal("valid authority update", err)
	}
	loaded, err := config.LoadNetworkProfile(dir, uint64(f.now.Load()))
	if err != nil || loaded.HighestEpoch != 3 {
		t.Fatal(loaded, err)
	}
}
func TestWANW03NoIdleMetadataRetention(t *testing.T) {
	f := newFixture(t, false)
	d := identity(t, 1, nil)
	if _, code := f.announce(t, d, announcement(f, 1), nonce()); code != "" {
		t.Fatal(code)
	}
	f.now.Add(601)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.s.mu.Lock()
		empty := len(f.s.records) == 0 && len(f.s.operations) == 0 && len(f.s.rates) == 0 && len(f.s.sources) == 0
		f.s.mu.Unlock()
		if empty {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("idle service retained expired metadata without incoming request")
}

func TestWANW03LocalPinsGateDataOffers(t *testing.T) {
	f := newFixture(t, true)
	a, b := identity(t, 1, nil), identity(t, 2, nil)
	ca, cb := f.client(t, a, f.roots), f.client(t, b, f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, c := range []*network.ServiceClient{ca, cb} {
		if err := c.Announce(ctx, "peer_data", nonce(), announcement(f, 1)); err != nil {
			t.Fatal(err)
		}
	}
	control, err := cb.Control(ctx, "peer_data", map[string]string{a.id: identity(t, 3, nil).pin}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	if err = ca.Exchange(ctx, p.NetworkProof{Kind: "offer", Target: b.id, TargetPin: b.pin, Purpose: "peer_data", Role: "initiator", Session: nonce()}, p.NetworkOffer{SenderGeneration: 1, TargetGeneration: 1, Candidates: []p.NetworkCandidate{}}); err != nil {
		t.Fatal(err)
	}
	short, end := context.WithTimeout(ctx, 100*time.Millisecond)
	defer end()
	if _, err = control.Next(short); err != context.DeadlineExceeded {
		t.Fatal("directory key replaced receiver's local pin", err)
	}
}
func TestWANW03ReannouncementJitterAndStableRenewal(t *testing.T) {
	f := newFixture(t, true)
	d := identity(t, 1, nil)
	waits := make(chan time.Duration, 2)
	resume := make(chan struct{}, 2)
	c, err := network.NewServiceClient(network.ServiceClientOptions{Selection: f.selection, Origin: f.origin, Device: d.id, Certificate: d.cert, Roots: f.roots, Now: func() time.Time { return time.Unix(f.now.Load(), 0) }, Wait: func(ctx context.Context, delay time.Duration) error {
		waits <- delay
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-resume:
			return nil
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- c.Reannounce(ctx, "self_hosted", "peer_data", func() p.NetworkAnnouncement { return announcement(f, 1) })
	}()
	for generation := p.NetworkUint(1); generation <= 2; generation++ {
		select {
		case delay := <-waits:
			if delay < 100*time.Second || delay > 140*time.Second {
				t.Fatal("unbounded renewal jitter", delay)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("renewal worker failed")
		}
		f.s.mu.Lock()
		record, ok := f.s.records[routeKey{actor{d.id, d.pin}, "peer_data"}]
		f.s.mu.Unlock()
		if !ok || record.announcement.Generation != generation || record.proof.SenderPin != d.pin {
			t.Fatal("renewal changed identity or stale generation", record)
		}
		if generation == 1 {
			f.now.Add(120)
			resume <- struct{}{}
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("renewal did not join cancellation")
	}
}
func TestWANW03ControlHeartbeatPreservesIdleSession(t *testing.T) {
	f := newFixture(t, true)
	d := identity(t, 1, nil)
	c := f.client(t, d, f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	control, err := c.Control(ctx, "peer_data", map[string]string{}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	// Cross the real 25-second heartbeat. The client consumes the ack instead of
	// expiring an otherwise healthy WSS connection or exposing it as a peer offer.
	time.Sleep(26 * time.Second)
	f.s.mu.Lock()
	_, alive := f.s.controls[routeKey{actor{d.id, d.pin}, "peer_data"}]
	f.s.mu.Unlock()
	if !alive {
		t.Fatal("idle authenticated control died before heartbeat")
	}
	short, end := context.WithTimeout(ctx, 100*time.Millisecond)
	defer end()
	if _, err = control.Next(short); err != context.DeadlineExceeded {
		t.Fatal("heartbeat exposed as peer event", err)
	}
}

func TestWANW03WSSAdmissionBounds(t *testing.T) {
	f := newFixture(t, true)
	d := identity(t, 1, nil)
	for _, size := range []int{16 << 10, (16 << 10) + 1} {
		q := f.proof(t, d, "authenticate", "peer_data", nonce(), nil)
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: f.roots, MinVersion: tls.VersionTLS13}, Proxy: nil}, Timeout: 2 * time.Second}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		conn, _, err := websocket.Dial(ctx, "wss://"+strings.TrimPrefix(f.origin, "https://")+"/network/v1/control", &websocket.DialOptions{HTTPClient: client, CompressionMode: websocket.CompressionDisabled})
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		body, _ := json.Marshal(p.NetworkLookupRequest{Proof: q})
		body = append(body, bytes.Repeat([]byte(" "), size-len(body))...)
		_ = conn.Write(ctx, websocket.MessageText, body)
		_, body, err = conn.Read(ctx)
		if size == 16<<10 {
			var result p.NetworkResult
			if err != nil || p.NetworkDecode(body, &result) != nil || result.Operation != q.Operation {
				t.Fatal("maximum control admission", err)
			}
		} else if err == nil {
			t.Fatal("oversized control admitted")
		}
		_ = conn.CloseNow()
		cancel()
		client.CloseIdleConnections()
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			f.s.mu.Lock()
			empty := len(f.s.controls) == 0
			f.s.mu.Unlock()
			if empty {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}
}

func TestWANW03ServiceBinaryRestart(t *testing.T) {
	f := newFixture(t, true)
	cert := f.server.TLSConfig.Certificates[0]
	_ = f.s.Close()
	_ = f.server.Close()
	root := testkit.NewDisposable(t)
	_ = os.Chmod(root, 0700)
	binary := filepath.Join(root, "orbit-net")
	build := exec.Command("go", "build", "-trimpath", "-o", binary, "../../cmd/orbit-net")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatal("service build", err, string(output))
	}
	profile, _ := json.Marshal(f.selection)
	private, _ := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	files := map[string][]byte{"selection.json": profile, "service-key": []byte(hex.EncodeToString(f.s.key)), "tls-cert.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), "tls-key.pem": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := testkit.ValidateDestructiveTarget(root, binary); err != nil {
		t.Fatal(err)
	}
	start := func() (*exec.Cmd, chan error) {
		command := exec.Command(binary, "--listen", strings.TrimPrefix(f.origin, "https://"), "--profile", filepath.Join(root, "selection.json"), "--origin", f.origin, "--tls-cert", filepath.Join(root, "tls-cert.pem"), "--tls-key", filepath.Join(root, "tls-key.pem"), "--service-key", filepath.Join(root, "service-key"))
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			conn, err := net.DialTimeout("tcp", strings.TrimPrefix(f.origin, "https://"), 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return command, done
			}
			select {
			case err := <-done:
				t.Fatal("service exited", err)
			default:
			}
			time.Sleep(10 * time.Millisecond)
		}
		_ = command.Process.Kill()
		<-done
		t.Fatal("service startup timeout")
		return nil, nil
	}
	stop := func(command *exec.Cmd, done chan error) {
		if err := testkit.ValidateDestructiveTarget(root, binary); err != nil {
			t.Fatal(err)
		}
		_ = command.Process.Signal(syscall.SIGTERM)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal("service shutdown", err)
			}
		case <-time.After(2 * time.Second):
			_ = command.Process.Kill()
			<-done
			t.Fatal("service shutdown did not join")
		}
	}
	d := identity(t, 1, nil)
	c, err := network.NewServiceClient(network.ServiceClientOptions{Selection: f.selection, Origin: f.origin, Device: d.id, Certificate: d.cert, Roots: f.roots})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	command, done := start()
	if err := c.Announce(context.Background(), "peer_data", nonce(), announcement(f, 1)); err != nil {
		stop(command, done)
		t.Fatal(err)
	}
	if _, found, err := c.Lookup(context.Background(), d.id, d.pin, "peer_data", 1); err != nil || !found {
		stop(command, done)
		t.Fatal("binary lookup", found, err)
	}
	stop(command, done)
	command, done = start()
	defer stop(command, done)
	if _, found, err := c.Lookup(context.Background(), d.id, d.pin, "peer_data", 1); err != nil || found {
		t.Fatal("restart invented retained reachability", found, err)
	}
	if err := c.Announce(context.Background(), "peer_data", nonce(), announcement(f, 1)); err != nil {
		t.Fatal("binary reannouncement after restart", err)
	}
}
