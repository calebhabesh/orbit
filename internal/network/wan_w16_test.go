package network

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	p "github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/coder/websocket"
)

// W16 native roaming: after an address change the old control channel fails
// silently while the service still holds it, refusing a second channel for the
// device until its heartbeat drops the first. The runtime must rebuild, back
// off on that refusal and become ready again on a fresh channel.
func TestWANW16AddressChangeRebuildsControlWithBackoff(t *testing.T) {
	client := clientFixture(t, &testResolver{})
	client.selection.Profile.Origins = append(client.selection.Profile.Origins, "wss://directory.orbit.invalid")
	canonical, err := client.selection.Profile.Canonical(false)
	if err != nil {
		t.Fatal(err)
	}
	client.selection.Profile.Signature = hex.EncodeToString(ed25519.Sign(client.key, canonical))
	client.digest, _ = client.selection.Digest()
	var controls, refusedControls atomic.Int64
	var holdOld atomic.Bool
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/network/v1/challenge":
			_ = json.NewEncoder(w).Encode(p.NetworkChallengeResult{Version: "1", Profile: client.digest, Origin: client.origin, Challenge: randomNetworkID(), Expires: p.NetworkUint(time.Now().Unix() + 60)})
		case "/network/v1/announce":
			var request p.NetworkAnnounceRequest
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Error("invalid announcement")
				return
			}
			_ = json.NewEncoder(w).Encode(p.NetworkResult{Version: "1", Operation: request.Proof.Operation, State: "accepted"})
		case "/network/v1/control":
			conn, e := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
			if e != nil {
				return
			}
			defer conn.CloseNow()
			_, body, e := conn.Read(r.Context())
			if e != nil {
				return
			}
			var request p.NetworkLookupRequest
			if json.Unmarshal(body, &request) != nil {
				return
			}
			n := controls.Add(1)
			if n > 2 && holdOld.Load() {
				// The service still holds the previous channel and, like the real
				// one, closes the new channel without a typed reason.
				refusedControls.Add(1)
				return
			}
			ack, _ := json.Marshal(p.NetworkResult{Version: "1", Operation: request.Proof.Operation, State: "accepted"})
			if conn.Write(r.Context(), websocket.MessageText, ack) != nil {
				return
			}
			_, _, _ = conn.Read(r.Context())
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	defer close(release)
	destination, _ := url.Parse(server.URL)
	client.http = &http.Client{Transport: w15ServiceTransport{server.Client().Transport, destination}}
	manager := NewManager(ManagerOptions{})
	defer manager.Close()
	runtime, err := NewRelayRuntime(context.Background(), client, manager, 1, client.digest)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err = runtime.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	before := runtime.endpoints[PeerData]
	runtime.mu.Unlock()
	holdOld.Store(true)
	runtime.NetworkChanged()
	runtime.ReconnectControl()
	// Refusals while the old channel is held back off rather than retrying every
	// two seconds; then the service frees it and a fresh channel becomes ready.
	time.Sleep(10 * time.Second)
	// Two purposes, each backing off 2+4+8s: at most three attempts apiece.
	if n := refusedControls.Load(); n == 0 || n > 6 {
		t.Fatalf("refused control attempts in 10s: %d (want backoff, not a tight loop)", n)
	}
	holdOld.Store(false)
	for ctx.Err() == nil {
		runtime.mu.Lock()
		after := runtime.endpoints[PeerData]
		runtime.mu.Unlock()
		if after != nil && after != before && runtime.Ready() {
			t.Logf("rebuilt control after address change; %d refused attempt(s) while the old channel was held", refusedControls.Load())
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("runtime did not rebuild a fresh ready control")
}

// W16 native forced relay: the client and service allow two relay tunnels per
// device pair, shared by both directions. An initiator holding two warm tunnels
// to a peer locked that peer out of its own direction. One initiator tunnel per
// target leaves the other direction a slot; a concurrent request waits for the
// open tunnel instead of dialing (and spending service budget on) another.
func TestWANW16OneRelayTunnelPerTargetDirection(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var inFlight, peakHandlers atomic.Int32
	s, trust, target := managerServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			old := peakHandlers.Load()
			if n <= old || peakHandlers.CompareAndSwap(old, n) {
				break
			}
		}
		if r.URL.Path == "/slow" {
			<-release
		}
		_, _ = w.Write([]byte("ok"))
	})
	target.Profile = [32]byte{1}
	var dials, open, peakOpen atomic.Int32
	m := NewManager(ManagerOptions{})
	defer m.Close()
	defer unblock() // runs first: the blocked handler must finish before Close joins
	if err := m.SetRelay(target, func(ctx context.Context, _ Target) (net.Conn, error) {
		dials.Add(1)
		c, err := (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(s.URL, "https://"))
		if err != nil {
			return nil, err
		}
		n := open.Add(1)
		for {
			old := peakOpen.Load()
			if n <= old || peakOpen.CompareAndSwap(old, n) {
				break
			}
		}
		return &countingConn{Conn: c, open: &open}, nil
	}); err != nil {
		t.Fatal(err)
	}
	rt, err := m.Transport(context.Background(), target, trust)
	if err != nil {
		t.Fatal(err)
	}
	slow := make(chan error, 1)
	go func() {
		res, err := rt.RoundTrip(mustRequest(LogicalOrigin(target) + "/slow"))
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}
		slow <- err
	}()
	for inFlight.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	fast := make(chan error, 1)
	go func() {
		res, err := rt.RoundTrip(mustRequest(LogicalOrigin(target) + "/fast"))
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}
		fast <- err
	}()
	time.Sleep(300 * time.Millisecond)
	if d := dials.Load(); d != 1 {
		t.Fatalf("second relay tunnel dialed while the first was open: dials=%d", d)
	}
	unblock()
	if err := <-slow; err != nil {
		t.Fatal(err)
	}
	if err := <-fast; err != nil {
		t.Fatal("waiting request failed instead of reusing the tunnel:", err)
	}
	if peakOpen.Load() != 1 || peakHandlers.Load() != 1 {
		t.Fatalf("tunnels open at once=%d concurrent handlers=%d", peakOpen.Load(), peakHandlers.Load())
	}
}

type countingConn struct {
	net.Conn
	open *atomic.Int32
	once sync.Once
}

func (c *countingConn) Close() error {
	c.once.Do(func() { c.open.Add(-1) })
	return c.Conn.Close()
}
