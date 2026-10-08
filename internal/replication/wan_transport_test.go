package replication

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/model"
	"github.com/coder/websocket"
)

// Fixture broker has two independently outbound WSS legs; it forwards only
// synthetic inner-TLS bytes to an isolated production peer/enrollment listener.
type wanCapture struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (c *wanCapture) add(p []byte) { c.mu.Lock(); defer c.mu.Unlock(); c.b.Write(p) }
func (c *wanCapture) contains(s []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return bytes.Contains(c.b.Bytes(), s)
}

type captureConn struct {
	net.Conn
	c *wanCapture
}

func (c captureConn) Read(p []byte) (int, error) { n, e := c.Conn.Read(p); c.c.add(p[:n]); return n, e }

func wanRelay(t *testing.T, server *http.Server) (network.StreamDialer, *wanCapture) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	capture := &wanCapture{}
	legs := make(chan net.Conn, 2)
	broker := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			http.Error(w, "browser forbidden", 403)
			return
		}
		ws, e := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if e != nil {
			return
		}
		c := network.BinaryStream(ctx, ws)
		select {
		case legs <- captureConn{c, capture}:
		case <-ctx.Done():
			_ = c.Close()
		}
	}))
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		var a, b net.Conn
		select {
		case a = <-legs:
		case <-ctx.Done():
			return
		}
		select {
		case b = <-legs:
		case <-ctx.Done():
			a.Close()
			return
		}
		_ = network.Forward(ctx, a, b, time.Second*5)
	}()
	dialWS := func() (net.Conn, error) {
		ws, _, e := websocket.Dial(ctx, "wss"+broker.URL[5:], &websocket.DialOptions{HTTPClient: broker.Client(), CompressionMode: websocket.CompressionDisabled})
		if e != nil {
			return nil, e
		}
		return network.BinaryStream(ctx, ws), nil
	}
	listener := network.NewStreamListener()
	served := make(chan error, 1)
	go func() { served <- server.Serve(tls.NewListener(listener, server.TLSConfig)) }()
	receiver, e := dialWS()
	if e != nil {
		t.Fatal(e)
	}
	if e = listener.Offer(ctx, receiver); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		cancel()
		listener.Close()
		server.Close()
		broker.Close()
		select {
		case <-joined:
		case <-time.After(2 * time.Second):
			t.Error("broker pumps not joined")
		}
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Error("HTTP server not joined")
		}
	})
	return func(context.Context, network.Target) (net.Conn, error) { return dialWS() }, capture
}

func wanDirect(t *testing.T, s *http.Server) network.StreamDialer {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve(tls.NewListener(l, s.TLSConfig)) }()
	t.Cleanup(func() { s.Close(); l.Close(); <-done })
	return func(ctx context.Context, _ network.Target) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", l.Addr().String())
	}
}

func TestWANW01PinnedTransport(t *testing.T) {
	for _, route := range []string{"direct", "relay"} {
		t.Run(route, func(t *testing.T) {
			f := newTwoPeerFixture(t)
			idA, e := LoadOrCreateIdentity(filepath.Join(filepath.Dir(f.stateA), "id-a"), f.devA, time.Now())
			if e != nil {
				t.Fatal(e)
			}
			idB, e := LoadOrCreateIdentity(filepath.Join(filepath.Dir(f.stateB), "id-b"), f.devB, time.Now())
			if e != nil {
				t.Fatal(e)
			}
			data := bytes.Repeat([]byte("SYNTHETIC-WAN-CONTENT-ONLY-"), 10000)
			if e = os.WriteFile(filepath.Join(f.rootA, "synthetic-wan-filename.txt"), data, 0600); e != nil {
				t.Fatal(e)
			}
			scan, e := f.workA.Scan(f.ctx, f.folder)
			if e != nil || len(scan.Captured) != 1 {
				t.Fatalf("capture: %v", e)
			}
			s := NewServer(f.repoA, idA).HTTPServer()
			var dial network.StreamDialer
			var capture *wanCapture
			if route == "direct" {
				dial = wanDirect(t, s)
			} else {
				dial, capture = wanRelay(t, s)
			}
			cfg, e := idB.ClientTLSConfig(idA.Leaf, idA.KeyPin)
			if e != nil {
				t.Fatal(e)
			}
			rt, e := network.NewTransport(network.Target{Device: f.devA, Pin: idA.KeyPin, Purpose: network.PeerData}, cfg, dial)
			if e != nil {
				t.Fatal(e)
			}
			defer rt.CloseIdleConnections()
			if len(cfg.NextProtos) != 0 {
				t.Fatal("mutated caller TLS config")
			}
			c := &Client{baseURL: "https://peer.orbit.invalid", http: network.HTTPClient(rt)}
			syncer := NewSyncer(f.repoB, f.workB, c, f.devB, f.devA, f.folder, f.approved, TransferOptions{})
			if _, e = syncer.Sync(f.ctx); e != nil {
				t.Fatal(e)
			}
			got, e := os.ReadFile(filepath.Join(f.rootB, "synthetic-wan-filename.txt"))
			if e != nil || !bytes.Equal(got, data) {
				t.Fatalf("chunk sync %v", e)
			}
			if capture != nil {
				for _, secret := range [][]byte{[]byte("synthetic-wan-filename.txt"), data[:128], []byte(`"manifest"`)} {
					if capture.contains(secret) {
						t.Fatal("broker saw plaintext")
					}
				}
			}
		})
	}
}

func TestWANW01WrongPinAndIsolation(t *testing.T) {
	for _, route := range []string{"direct", "relay"} {
		for _, purpose := range []network.Purpose{network.Enrollment, network.PeerData} {
			t.Run(route+"/"+string(purpose), func(t *testing.T) {
				f := newTwoPeerFixture(t)
				idA, e := LoadOrCreateIdentity(filepath.Join(filepath.Dir(f.stateA), "id-a"), f.devA, time.Now())
				if e != nil {
					t.Fatal(e)
				}
				idB, e := LoadOrCreateIdentity(filepath.Join(filepath.Dir(f.stateB), "id-b"), f.devB, time.Now())
				if e != nil {
					t.Fatal(e)
				}
				server := NewServer(f.repoA, idA).HTTPServer()
				if purpose == network.Enrollment {
					server = NewEnrollmentServer(f.repoA, idA).HTTPServer()
				}
				var dial network.StreamDialer
				var capture *wanCapture
				if route == "direct" {
					dial = wanDirect(t, server)
				} else {
					dial, capture = wanRelay(t, server)
				}
				// A different trusted certificate at the actual A route must stop TLS before
				// the capability-bearing challenge is written. Never bypass normal verification.
				cfg, e := idB.ClientTLSConfig(idB.Leaf, idB.KeyPin)
				if e != nil {
					t.Fatal(e)
				}
				if purpose == network.Enrollment {
					cfg.Certificates = nil
				}
				rt, e := network.NewTransport(network.Target{Device: f.devA, Pin: idB.KeyPin, Purpose: purpose}, cfg, dial)
				if e != nil {
					t.Fatal(e)
				}
				defer rt.CloseIdleConnections()
				secret := []byte("SYNTHETIC-INVITATION-CAPABILITY-NEVER-DISCLOSED")
				req, _ := http.NewRequest("POST", "https://peer.orbit.invalid/enrollment/v2/challenge", bytes.NewReader(secret))
				if r, e := network.HTTPClient(rt).Do(req); e == nil {
					r.Body.Close()
					t.Fatal("wrong inviter pin accepted")
				}
				if capture != nil && capture.contains(secret) {
					t.Fatal("invitation disclosed to broker")
				}
			})
		}
	}
	// Correct inviter plus unknown/absent client certificate may reach enrollment
	// only. Production data folder gates still reject an otherwise valid TLS key.
	for _, test := range []struct {
		name    string
		purpose network.Purpose
		path    string
		missing bool
		want    int
	}{
		{"data-missing-cert", network.PeerData, "/peer/v1/hello", true, 0},
		{"enrollment-no-data", network.Enrollment, "/peer/v1/hello", true, 404},
		{"enrollment-no-control", network.Enrollment, "/api/v1/status", true, 404},
		{"data-no-control", network.PeerData, "/api/v1/status", false, 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newTwoPeerFixture(t)
			base := filepath.Dir(f.stateA)
			a, e := LoadOrCreateIdentity(filepath.Join(base, "id-a"), f.devA, time.Now())
			if e != nil {
				t.Fatal(e)
			}
			b, e := LoadOrCreateIdentity(filepath.Join(base, "id-b"), f.devB, time.Now())
			if e != nil {
				t.Fatal(e)
			}
			s := NewServer(f.repoA, a).HTTPServer()
			if test.purpose == network.Enrollment {
				s = NewEnrollmentServer(f.repoA, a).HTTPServer()
			}
			dial, _ := wanRelay(t, s)
			cfg, e := b.ClientTLSConfig(a.Leaf, a.KeyPin)
			if e != nil {
				t.Fatal(e)
			}
			if test.missing {
				cfg.Certificates = nil
			}
			rt, e := network.NewTransport(network.Target{Device: f.devA, Pin: a.KeyPin, Purpose: test.purpose}, cfg, dial)
			if e != nil {
				t.Fatal(e)
			}
			defer rt.CloseIdleConnections()
			r, _ := http.NewRequest("POST", "https://peer.orbit.invalid"+test.path, bytes.NewReader([]byte(`{}`)))
			response, e := network.HTTPClient(rt).Do(r)
			if test.want == 0 {
				if e == nil {
					response.Body.Close()
					t.Fatal("missing mTLS accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			defer response.Body.Close()
			if response.StatusCode != test.want {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("%d %s", response.StatusCode, body)
			}
		})
	}
}

func TestWANW01EnrollmentHandlerThroughRelay(t *testing.T) {
	f := newTwoPeerFixture(t)
	base := filepath.Dir(f.stateA)
	a, e := LoadOrCreateIdentity(filepath.Join(base, "id-a"), f.devA, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	// A fresh requester has never been a member or used a LAN route.
	b, e := LoadOrCreateIdentity(t.TempDir(), fixedID('C'), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	token := strings.Repeat("1", 64)
	digest, e := tokenVerifier(token)
	if e != nil {
		t.Fatal(e)
	}
	inv := tc.Invitation{Version: "1", Folder: hex.EncodeToString(f.folder[:]), Inviter: hex.EncodeToString(f.devA[:]), KeyPin: hex.EncodeToString(a.KeyPin[:]), CertificateDER: base64.StdEncoding.EncodeToString(a.Leaf.Raw), Capability: token, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), EnrollmentEndpoint: "https://peer.orbit.invalid", PeerEndpoint: "https://peer.orbit.invalid"}
	stored := inv
	stored.Capability = ""
	e = f.repoA.EnrollmentTransaction(f.ctx, func(tx *repository.EnrollmentTx) error {
		return tx.Put("invite/"+digest, EnrollmentInvitation{Invitation: stored, Digest: digest})
	})
	if e != nil {
		t.Fatal(e)
	}
	dial, capture := wanRelay(t, NewEnrollmentServer(f.repoA, a).HTTPServer())
	cfg, e := b.ClientTLSConfig(a.Leaf, a.KeyPin)
	if e != nil {
		t.Fatal(e)
	}
	cfg.Certificates = nil
	rt, e := network.NewTransport(network.Target{Device: f.devA, Pin: a.KeyPin, Purpose: network.Enrollment}, cfg, dial)
	if e != nil {
		t.Fatal(e)
	}
	defer rt.CloseIdleConnections()
	c := &EnrollmentClient{invitation: inv, identity: b, http: network.HTTPClient(rt)}
	attempt := strings.Repeat("2", 64)
	wire, e := c.Prepare(f.ctx, attempt, "Synthetic unknown requester", "")
	if e != nil {
		t.Fatal(e)
	}
	result, e := c.Submit(f.ctx, wire)
	if e != nil || result.State != "pending_approval" {
		t.Fatalf("admission %v %+v", e, result)
	}
	if capture.contains([]byte(token)) {
		t.Fatal("broker saw capability")
	}
	// No membership effect follows service/enrollment admission alone.
	membership, _, e := f.repoA.GetMembership(f.ctx, f.folder)
	if e != nil {
		t.Fatal(e)
	}
	for _, member := range membership.Active {
		if member.Device == b.DeviceID {
			t.Fatal("pending admission granted folder")
		}
	}
	dataDial, _ := wanRelay(t, NewServer(f.repoA, a).HTTPServer())
	dataCfg, e := b.ClientTLSConfig(a.Leaf, a.KeyPin)
	if e != nil {
		t.Fatal(e)
	}
	dataRT, e := network.NewTransport(network.Target{Device: f.devA, Pin: a.KeyPin, Purpose: network.PeerData}, dataCfg, dataDial)
	if e != nil {
		t.Fatal(e)
	}
	defer dataRT.CloseIdleConnections()
	client := &Client{baseURL: "https://peer.orbit.invalid", http: network.HTTPClient(dataRT)}
	_, e = client.Inventory(f.ctx, InventoryRequest{ProtocolVersion: "1", DeviceID: hex.EncodeToString(b.DeviceID[:]), FolderID: inv.Folder, Revision: "1", MembershipDigest: hex.EncodeToString(f.approved.Digest[:]), PageSize: "1"})
	var denied *WireError
	if !errors.As(e, &denied) || denied.Body.Code != "UNAUTHORIZED" {
		t.Fatalf("unknown data key must fail: %v", e)
	}
}

// This is a v3 gate fixture, not a production enrollment handler. It composes
// real pinned TLS, the frozen v3 signature contract, an independent admission
// oracle, and the existing membership transaction/retirement rules. W05 owns
// persistent v3 challenge/status/approval handlers and daemon resumption.
func TestWANW01V3EnrollmentTransportAndMembership(t *testing.T) {
	for _, route := range []string{"direct", "relay"} {
		t.Run(route, func(t *testing.T) {
			f := newTwoPeerFixture(t)
			base := filepath.Dir(f.stateA)
			inviter, e := LoadOrCreateIdentity(filepath.Join(base, "id-a"), f.devA, time.Now())
			if e != nil {
				t.Fatal(e)
			}
			requester, e := LoadOrCreateIdentity(t.TempDir(), fixedID('C'), time.Now())
			if e != nil {
				t.Fatal(e)
			}
			p := protocol.NetworkProfile{Version: "1", Operator: "Synthetic fixture", ServiceKey: strings.Repeat("a", 64), Authority: hex.EncodeToString(inviter.Certificate.PrivateKey.(ed25519.PrivateKey).Public().(ed25519.PublicKey)), Epoch: 1, Expires: protocol.NetworkUint(time.Now().Add(time.Hour).Unix()), Origins: []string{"https://rendezvous.example.org", "wss://relay.example.org"}, STUN: []string{}, Privacy: "Synthetic fixture; not an operated profile"}
			pb, e := p.Canonical(false)
			if e != nil {
				t.Fatal(e)
			}
			p.Signature = hex.EncodeToString(ed25519.Sign(inviter.Certificate.PrivateKey.(ed25519.PrivateKey), pb))
			profile, e := p.Digest(false)
			if e != nil {
				t.Fatal(e)
			}
			if e = p.Verify(inviter.Certificate.PrivateKey.(ed25519.PrivateKey).Public().(ed25519.PublicKey), uint64(time.Now().Unix()), 1, false); e != nil {
				t.Fatal(e)
			}
			cap := strings.Repeat("3", 64)
			capbytes, _ := hex.DecodeString(cap)
			transcript := protocol.RoutedEnrollmentTranscript{Version: "3", Folder: hex.EncodeToString(f.folder[:]), Inviter: protocol.EnrollmentRoute{Device: hex.EncodeToString(inviter.DeviceID[:]), Pin: hex.EncodeToString(inviter.KeyPin[:]), Profile: profile, Purpose: "enrollment"}, Requester: protocol.EnrollmentRoute{Device: hex.EncodeToString(requester.DeviceID[:]), Pin: hex.EncodeToString(requester.KeyPin[:]), Profile: profile, Purpose: "enrollment"}, CapabilityDigest: protocol.NetworkDigest(capbytes), Attempt: strings.Repeat("4", 64), Challenge: strings.Repeat("5", 64), PublicKey: hex.EncodeToString(requester.Certificate.PrivateKey.(ed25519.PrivateKey).Public().(ed25519.PublicKey)), PriorMembership: hex.EncodeToString(f.approved.Digest[:]), Expires: protocol.NetworkUint(time.Now().Add(time.Minute).Unix()), Label: "Synthetic requester"}
			canonical, e := transcript.Canonical()
			if e != nil {
				t.Fatal(e)
			}
			request := protocol.RoutedEnrollmentRequest{Transcript: transcript, Capability: cap, CertificateDER: base64.StdEncoding.EncodeToString(requester.Leaf.Raw), Signature: hex.EncodeToString(ed25519.Sign(requester.Certificate.PrivateKey.(ed25519.PrivateKey), canonical))}
			oracle := model.NewWANEnrollment()
			var mu sync.Mutex
			join := model.WANJoin{Folder: transcript.Folder, Attempt: transcript.Attempt, Inviter: transcript.Inviter.Device, Requester: transcript.Requester.Device, Pin: transcript.Requester.Pin, Prior: transcript.PriorMembership, Digest: protocol.NetworkDigest(canonical), Until: uint64(transcript.Expires)}
			fixture := &http.Server{TLSConfig: inviter.ServerTLSConfig(), ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/enrollment/v3/request" {
					http.NotFound(w, r)
					return
				}
				raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
				var got protocol.RoutedEnrollmentRequest
				if e != nil || protocol.NetworkDecode(raw, &got) != nil || got.Verify(uint64(time.Now().Unix())) != nil || got.Transcript != transcript {
					http.Error(w, "rejected", 403)
					return
				}
				mu.Lock()
				state := oracle.Submit(join, got.Capability, uint64(time.Now().Unix()), true, true)
				mu.Unlock()
				w.Write([]byte(state))
			})}
			fixture.TLSConfig.ClientAuth = tls.NoClientCert
			var dial network.StreamDialer
			var capture *wanCapture
			if route == "direct" {
				dial = wanDirect(t, fixture)
			} else {
				dial, capture = wanRelay(t, fixture)
			}
			cfg, e := requester.ClientTLSConfig(inviter.Leaf, inviter.KeyPin)
			if e != nil {
				t.Fatal(e)
			}
			cfg.Certificates = nil
			rt, e := network.NewTransport(network.Target{Device: inviter.DeviceID, Pin: inviter.KeyPin, Purpose: network.Enrollment}, cfg, dial)
			if e != nil {
				t.Fatal(e)
			}
			defer rt.CloseIdleConnections()
			raw, _ := json.Marshal(request)
			for _, want := range []string{"pending", "replayed"} {
				q, _ := http.NewRequest("POST", "https://peer.orbit.invalid/enrollment/v3/request", bytes.NewReader(raw))
				r, e := network.HTTPClient(rt).Do(q)
				if e != nil {
					t.Fatal(e)
				}
				body, e := io.ReadAll(r.Body)
				r.Body.Close()
				if e != nil || string(body) != want {
					t.Fatalf("admission %s %v", body, e)
				}
			}
			if capture != nil && capture.contains([]byte(cap)) {
				t.Fatal("v3 secret disclosed")
			}
			mu.Lock()
			if oracle.FolderAccess(join) {
				t.Fatal("route admission granted data")
			}
			if got := oracle.Approve(join.ID(), join.Folder, join.Requester, join.Pin, join.Prior, join.Digest, cap, uint64(time.Now().Unix())); got != "approved" {
				t.Fatal(got)
			}
			mu.Unlock()
			membership, _, e := f.repoA.GetMembership(f.ctx, f.folder)
			if e != nil {
				t.Fatal(e)
			}
			membership.Revision++
			membership.PriorDigest = f.approved.Digest
			membership.Active = append(membership.Active, protocol.ActiveMember{Device: requester.DeviceID, KeyPin: requester.KeyPin})
			artifact, e := protocol.EncodeMembership(membership)
			if e != nil {
				t.Fatal(e)
			}
			approved, e := f.repoA.ApproveMembership(f.ctx, membership)
			if e != nil {
				t.Fatal(e)
			}
			if approved.Digest != sha256.Sum256(artifact) {
				t.Fatal("changed v1 membership artifact")
			}
			// Existing membership refuses reuse of a retired identity without inventing
			// a new random DeviceID or weakening retirement snapshot requirements.
			retired := membership
			retired.Revision++
			retired.PriorDigest = approved.Digest
			retired.Active = retired.Active[:len(retired.Active)-1]
			retired.Retired = []protocol.RetiredMember{{Device: requester.DeviceID, RetiredAt: retired.Revision, SnapshotDigest: sha256.Sum256([]byte("synthetic-retirement"))}}
			retirement, e := f.repoA.ApproveMembership(f.ctx, retired)
			if e != nil {
				t.Fatal(e)
			}
			revival := membership
			revival.Revision = retired.Revision + 1
			revival.PriorDigest = retirement.Digest
			if _, e = f.repoA.ApproveMembership(f.ctx, revival); e == nil {
				t.Fatal("retired identity revived")
			}
		})
	}
}

func TestWANW01TransportCancellationRedirectAndBinding(t *testing.T) {
	for _, route := range []string{"direct", "relay"} {
		for _, mode := range []string{"cancel", "redirect", "target-pin"} {
			t.Run(route+"/"+mode, func(t *testing.T) {
				f := newTwoPeerFixture(t)
				base := filepath.Dir(f.stateA)
				a, e := LoadOrCreateIdentity(filepath.Join(base, "id-a"), f.devA, time.Now())
				if e != nil {
					t.Fatal(e)
				}
				b, e := LoadOrCreateIdentity(filepath.Join(base, "id-b"), f.devB, time.Now())
				if e != nil {
					t.Fatal(e)
				}
				entered := make(chan struct{}, 1)
				released := make(chan struct{}, 1)
				s := &http.Server{TLSConfig: a.ServerTLSConfig(), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 || r.TLS.NegotiatedProtocol != "http/1.1" {
						http.Error(w, "TLS/ALPN mismatch", 500)
						return
					}
					if mode == "cancel" {
						_, _ = io.Copy(io.Discard, r.Body)
						entered <- struct{}{}
						<-r.Context().Done()
						released <- struct{}{}
						return
					}
					http.Redirect(w, r, "https://other.example.org/peer/v1/hello", http.StatusTemporaryRedirect)
				})}
				var dial network.StreamDialer
				if route == "direct" {
					dial = wanDirect(t, s)
				} else {
					dial, _ = wanRelay(t, s)
				}
				cfg, e := b.ClientTLSConfig(a.Leaf, a.KeyPin)
				if e != nil {
					t.Fatal(e)
				}
				target := network.Target{Device: f.devA, Pin: a.KeyPin, Purpose: network.PeerData}
				if mode == "target-pin" {
					target.Pin = b.KeyPin
				}
				rt, e := network.NewTransport(target, cfg, dial)
				if e != nil {
					t.Fatal(e)
				}
				defer rt.CloseIdleConnections()

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				q, _ := http.NewRequestWithContext(ctx, "POST", "https://peer.orbit.invalid/peer/v1/hello", bytes.NewReader([]byte(`{}`)))
				if mode == "cancel" {
					done := make(chan error, 1)
					go func() {
						r, e := network.HTTPClient(rt).Do(q)
						if r != nil {
							r.Body.Close()
						}
						done <- e
					}()
					select {
					case <-entered:
					case <-time.After(time.Second):
						t.Fatal("request not started")
					}
					cancel()
					select {
					case e := <-done:
						if e == nil {
							t.Fatal("cancellation succeeded")
						}
					case <-time.After(time.Second):
						t.Fatal("request not canceled")
					}
					select {
					case <-released:
					case <-time.After(time.Second):
						t.Fatal("handler not canceled")
					}
				} else {
					r, e := network.HTTPClient(rt).Do(q)
					if r != nil {
						r.Body.Close()
					}
					if e == nil {
						t.Fatal("redirect or mismatched target accepted")
					}
				}
			})
		}
	}
}
