package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	p "github.com/calebhabesh/file-sync/internal/protocol"
)

func lanRecord(t *testing.T) (p.LANAnnouncement, ed25519.PrivateKey, Target, LocalInterface) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(pub)
	pin := sha256.Sum256(der)
	target := Target{Device: history.ID{7}, Pin: pin, Purpose: PeerData}
	a := p.LANAnnouncement{Version: "1", Device: hex.EncodeToString(target.Device[:]), Pin: hex.EncodeToString(pin[:]), Key: hex.EncodeToString(pub), Generation: 1, Expires: p.NetworkUint(time.Now().Unix() + 600), Capabilities: []string{"direct_https_v1"}, Candidates: []p.NetworkCandidate{{Transport: "tcp", Address: "192.168.10.2:4567", Scope: "lan", Interface: "remote-name"}}}
	return a, key, target, LocalInterface{Index: 1, Name: "selected", Prefixes: []netip.Prefix{netip.MustParsePrefix("192.168.10.1/24")}}
}
func signLAN(t *testing.T, a p.LANAnnouncement, key ed25519.PrivateKey) []byte {
	t.Helper()
	b, err := a.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	a.Signature = hex.EncodeToString(ed25519.Sign(key, b))
	b, err = json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestWANW08ScopedSignedDiscoveryAdmission(t *testing.T) {
	a, key, target, iface := lanRecord(t)
	m := NewManager(ManagerOptions{})
	defer m.Close()
	if err := m.RegisterDirect(target, nil); err != nil {
		t.Fatal(err)
	}
	d := &LANDiscovery{manager: m, device: "local-device", known: m.KnownTargets}
	source := &net.UDPAddr{IP: net.ParseIP("192.168.10.2"), Port: DiscoveryPort}
	b := signLAN(t, a, key)
	if len(b) > MaxDiscoveryDatagramBytes || d.Accept(b, source, iface) != nil {
		t.Fatal("valid bounded announcement refused")
	}
	if len(m.directCandidates(context.Background(), target)) != 1 {
		t.Fatal("missing scoped route")
	}
	for _, kind := range []string{"unknown", "pin", "signature", "expired", "generation", "public", "multicast", "loopback", "wrong-prefix", "interface", "source", "oversized", "unknown-field"} {
		t.Run(kind, func(t *testing.T) {
			changed := a
			changed.Candidates = append([]p.NetworkCandidate(nil), a.Candidates...)
			input := b
			src := source
			local := iface
			switch kind {
			case "unknown":
				changed.Device = hex.EncodeToString(bytesOf(8))
				input = signLAN(t, changed, key)
			case "pin":
				changed.Pin = hex.EncodeToString(bytesOf(8))
				input = signLAN(t, changed, key)
			case "signature":
				changed.Signature = hex.EncodeToString(make([]byte, 64))
				input, _ = json.Marshal(changed)
			case "expired":
				changed.Expires = p.NetworkUint(time.Now().Unix() - 1)
				input = signLAN(t, changed, key)
			case "generation":
				changed.Generation = 0
				input, _ = json.Marshal(changed)
			case "public", "multicast", "loopback":
				changed.Candidates[0].Address = map[string]string{"public": "8.8.8.8:4567", "multicast": "239.1.1.1:4567", "loopback": "127.0.0.1:4567"}[kind]
				input, _ = json.Marshal(changed)
			case "wrong-prefix":
				changed.Candidates[0].Address = "192.168.11.2:4567"
				input = signLAN(t, changed, key)
			case "interface":
				local = LocalInterface{Index: 2, Prefixes: []netip.Prefix{netip.MustParsePrefix("10.0.0.1/24")}}
			case "source":
				src = &net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: 1}
			case "oversized":
				input = make([]byte, MaxDiscoveryDatagramBytes+1)
			case "unknown-field":
				input = append(append([]byte(nil), b[:len(b)-1]...), []byte(",\"folders\":[\"secret\"]}")...)
			}
			if d.Accept(input, src, local) == nil {
				t.Fatal("invalid record admitted")
			}
		})
	}
	// A signed move supersedes the old address; old or same-generation changed
	// records cannot put the old candidate back into the cache.
	a.Generation = 2
	a.Candidates[0].Address = "192.168.10.3:4567"
	if err := d.Accept(signLAN(t, a, key), source, iface); err != nil {
		t.Fatal(err)
	}
	a.Candidates[0].Address = "192.168.10.4:4567"
	if err := d.Accept(signLAN(t, a, key), source, iface); !errors.Is(err, ErrStale) {
		t.Fatal("same-generation mutation", err)
	}
}

func TestWANW08PinnedFamilyRaceAndDirectoryIndependentPool(t *testing.T) {
	var handled, lookup atomic.Int32
	s, trust, target := managerServer(t, func(w http.ResponseWriter, r *http.Request) { handled.Add(1); w.Write([]byte("verified")) })
	var active, peak atomic.Int32
	m := NewManager(ManagerOptions{DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		if address == "[2606:4700:4700::1111]:443" {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", s.Listener.Addr().String())
	}})
	defer m.Close()
	candidates := []p.NetworkCandidate{{Transport: "tcp", Address: "[2606:4700:4700::1111]:443", Scope: "public"}, {Transport: "tcp", Address: "8.8.8.8:443", Scope: "public"}}
	now := time.Now()
	if err := m.RegisterDirect(target, func(context.Context, Target, uint64) (p.NetworkAnnouncement, error) {
		lookup.Add(1)
		return p.NetworkAnnouncement{Generation: 1, Expires: p.NetworkUint(now.Add(time.Minute).Unix()), Candidates: candidates}, nil
	}); err != nil {
		t.Fatal(err)
	}
	rt, err := m.Transport(context.Background(), target, trust)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		response := round(t, rt, LogicalOrigin(target))
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
	}
	if handled.Load() != 3 || lookup.Load() != 1 || peak.Load() != 2 || active.Load() != 0 || m.Observe(target).Route != "direct" {
		t.Fatal("race/cache/join", handled.Load(), lookup.Load(), peak.Load(), active.Load(), m.Observe(target))
	}
	// Existing authenticated connection continues even with an expired candidate
	// and a now-failed source; no lookup occurs for a reusable session.
	m.mu.Lock()
	m.public[target] = candidateLease{}
	m.sources[target] = func(context.Context, Target, uint64) (p.NetworkAnnouncement, error) {
		t.Error("existing session depended on directory")
		return p.NetworkAnnouncement{}, ErrNoRoute
	}
	m.mu.Unlock()
	response := round(t, rt, LogicalOrigin(target))
	response.Body.Close()
}

func TestWANW08WrongPinBlockedTCPFallsBackWithoutDisclosure(t *testing.T) {
	s, trust, target := managerServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("relay result")) })
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"example.com"}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	wrong := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("wrong-pin candidate received HTTP request") }))
	wrong.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	wrong.StartTLS()
	defer wrong.Close()
	for _, kind := range []string{"wrong-pin", "blocked", "ipv6-unavailable"} {
		t.Run(kind, func(t *testing.T) {
			m := NewManager(ManagerOptions{DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
				if kind == "ipv6-unavailable" {
					return nil, syscall.EAFNOSUPPORT
				}
				if kind == "blocked" {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return (&net.Dialer{}).DialContext(ctx, "tcp", wrong.Listener.Addr().String())
			}})
			defer m.Close()
			target.Profile[0] = 1
			if err := m.SetRelay(target, func(ctx context.Context, _ Target) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", s.Listener.Addr().String())
			}); err != nil {
				t.Fatal(err)
			}
			if err := m.RegisterDirect(target, nil); err != nil {
				t.Fatal(err)
			}
			address := "8.8.8.8:443"
			if kind == "ipv6-unavailable" {
				address = "[2606:4700:4700::1111]:443"
			}
			if err := m.SetCandidates(target, []p.NetworkCandidate{{Transport: "tcp", Address: address, Scope: "public"}}, 1, time.Now().Add(time.Minute), true); err != nil {
				t.Fatal(err)
			}
			rt, err := m.Transport(context.Background(), target, trust)
			if err != nil {
				t.Fatal(err)
			}
			res := round(t, rt, LogicalOrigin(target))
			res.Body.Close()
			if m.Observe(target).Route != "relay" {
				t.Fatal(m.Observe(target))
			}
		})
	}
}

func TestWANW08CandidateBoundsExpiryAndOptionalListener(t *testing.T) {
	now := time.Now()
	_, _, target, iface := lanRecord(t)
	m := NewManager(ManagerOptions{Now: func() time.Time { return now }})
	defer m.Close()
	if err := m.RegisterDirect(target, nil); err != nil {
		t.Fatal(err)
	}
	c := p.NetworkCandidate{Transport: "tcp", Address: "192.168.10.2:4567", Scope: "lan", Interface: "selected"}
	for _, candidates := range [][]p.NetworkCandidate{make([]p.NetworkCandidate, p.NetworkMaxCandidates+1), {c, c}} {
		if m.SetCandidates(target, candidates, 1, now.Add(time.Minute), false) == nil {
			t.Fatal("candidate bound")
		}
	}
	if err := m.SetCandidates(target, []p.NetworkCandidate{c}, 1, now.Add(time.Minute), false); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if len(m.directCandidates(context.Background(), target)) != 0 {
		t.Fatal("expired route dialed")
	}
	listener, err := OptionalDirectListener(DirectSettings{})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	local := GatherCandidates([]LocalInterface{iface}, listener.Addr(), false)
	if len(local) != 1 {
		t.Fatal("actual candidate gathering", local, listener.Addr())
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	if local[0].Address != "192.168.10.1:"+port {
		t.Fatal(local)
	}
	if _, err = OptionalDirectListener(DirectSettings{Listen: listener.Addr().String()}); err == nil {
		t.Fatal("occupied optional listener unexpectedly available")
	}
	if public := GatherCandidates([]LocalInterface{iface}, listener.Addr(), true); len(public) != 0 {
		t.Fatal("private public publication")
	}
	if _, err = SelectedInterfaces([]string{"orbit-nonexistent-disposable-interface"}); err == nil {
		t.Fatal("unsupported interface")
	}
	before := runtime.NumGoroutine()
	for i := 0; i < 100; i++ {
		manager := NewManager(ManagerOptions{})
		manager.Close()
	}
	if runtime.NumGoroutine() > before+2 {
		t.Fatal("manager goroutines leaked")
	}
	m.mu.Lock()
	if len(m.lan) != 1 || len(m.public) != 0 {
		t.Error("unbounded caches")
	}
	m.mu.Unlock()
}

func bytesOf(n byte) []byte { b := make([]byte, 32); b[0] = n; return b }

func TestWANW08EmptyPublicLeaseDoesNotConsumeRepeatedLookupQuota(t *testing.T) {
	_, _, target, _ := lanRecord(t)
	m := NewManager(ManagerOptions{})
	defer m.Close()
	var calls atomic.Int32
	if err := m.RegisterDirect(target, func(context.Context, Target, uint64) (p.NetworkAnnouncement, error) {
		calls.Add(1)
		return p.NetworkAnnouncement{Generation: 1, Expires: p.NetworkUint(time.Now().Unix() + 600), Candidates: []p.NetworkCandidate{}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		m.directCandidates(context.Background(), target)
	}
	if calls.Load() != 1 {
		t.Fatalf("valid empty directory lease consumed repeated quota: lookups=%d want=1", calls.Load())
	}
}

func TestWANW08CancellationLimitsAndJoinedResources(t *testing.T) {
	_, trust, target := managerServer(t, func(http.ResponseWriter, *http.Request) { t.Error("blocked dial reached handler") })
	var active, peak atomic.Int32
	m := NewManager(ManagerOptions{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	var wg sync.WaitGroup
	before := runtime.NumGoroutine()
	for i := 0; i < MaxActivePeerSlots*2; i++ {
		peer := target
		peer.Device[0] = byte(i + 1)
		if err := m.RegisterDirect(peer, nil); err != nil {
			t.Fatal(err)
		}
		if err := m.SetCandidates(peer, []p.NetworkCandidate{{Transport: "tcp", Address: "8.8.8.8:443", Scope: "public"}, {Transport: "tcp", Address: "[2606:4700:4700::1111]:443", Scope: "public"}}, 1, time.Now().Add(time.Minute), true); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			rt, err := m.Transport(context.Background(), peer, trust)
			if err != nil {
				return
			}
			req, _ := http.NewRequest("GET", LogicalOrigin(peer), nil)
			_, _ = rt.RoundTrip(req)
		}()
	}
	deadline := time.Now().Add(time.Second)
	for active.Load() < MaxActivePeerSlots && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if active.Load() != MaxActivePeerSlots {
		t.Fatal("global direct dial ceiling not exercised", active.Load())
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	m.mu.Lock()
	dials, requests, pools := m.activeDials, len(m.requests), len(m.pools)
	m.mu.Unlock()
	m.connMu.Lock()
	sockets := len(m.connections)
	m.connMu.Unlock()
	quiescent := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before+4 && time.Now().Before(quiescent) {
		runtime.Gosched()
	}
	if peak.Load() > MaxActivePeerSlots || active.Load() != 0 || dials != 0 || requests != 0 || pools != 0 || sockets != 0 || runtime.NumGoroutine() > before+4 {
		t.Fatal("resource ceiling/join", peak.Load(), active.Load(), dials, requests, pools, sockets)
	}
	t.Logf("peak blocked raw TCP dials=%d; joined dials/requests/pools/sockets=0", peak.Load())
}

func TestWANW08DatagramFloodFixedWorkersAndSocketCleanup(t *testing.T) {
	interfaces, err := SelectedInterfaces(nil)
	if err != nil || len(interfaces) == 0 {
		t.Fatal("real selected interfaces", err)
	}
	var iface LocalInterface
	var ip net.IP
	for _, candidate := range interfaces {
		for _, prefix := range candidate.Prefixes {
			if prefix.Addr().Is4() && prefix.Addr().IsPrivate() {
				iface = candidate
				ip = net.ParseIP(prefix.Addr().String())
				break
			}
		}
		if ip != nil {
			break
		}
	}
	if ip == nil {
		t.Fatal("multicast IPv4 unavailable")
	}
	m := NewManager(ManagerOptions{})
	defer m.Close()
	a, key, _, _ := lanRecord(t)
	var callbacks atomic.Int32
	baselineFD, _ := os.ReadDir("/proc/self/fd")
	baselineG := runtime.NumGoroutine()
	var before, during, after runtime.MemStats
	runtime.ReadMemStats(&before)
	discovery, err := NewLANDiscovery(context.Background(), m, hex.EncodeToString(bytesOf(8)), a.Pin, tls.Certificate{PrivateKey: key}, []LocalInterface{iface}, nil, func() []Target { callbacks.Add(1); return nil })
	if err != nil {
		t.Fatal(err)
	}
	sender, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: ip, Port: DiscoveryPort})
	if err != nil {
		discovery.Close()
		t.Fatal(err)
	}
	spoof := signLAN(t, a, key)
	oversized := make([]byte, MaxDiscoveryDatagramBytes+1)
	for i := 0; i < 1000; i++ {
		packet := spoof
		if i%2 == 0 {
			packet = oversized
		}
		if _, err = sender.Write(packet); err != nil {
			sender.Close()
			discovery.Close()
			t.Fatal(err)
		}
	}
	time.Sleep(100 * time.Millisecond)
	runtime.ReadMemStats(&during)
	activeFD, _ := os.ReadDir("/proc/self/fd")
	sender.Close()
	discovery.Close()
	runtime.ReadMemStats(&after)
	finalFD, _ := os.ReadDir("/proc/self/fd")
	if len(finalFD) > len(baselineFD)+1 || len(activeFD) > len(baselineFD)+2 || runtime.NumGoroutine() > baselineG+2 || callbacks.Load() > 40 || len(m.KnownTargets()) != 0 {
		t.Fatal("discovery flood bounds", len(baselineFD), len(activeFD), len(finalFD), callbacks.Load(), baselineG, runtime.NumGoroutine())
	}
	t.Logf("1000 spoof/oversized datagrams; FD before/during/after=%d/%d/%d, goroutines before/after=%d/%d, admitted identity callbacks=%d, whole-process HeapAlloc before/during/after=%d/%d/%d", len(baselineFD), len(activeFD), len(finalFD), baselineG, runtime.NumGoroutine(), callbacks.Load(), before.HeapAlloc, during.HeapAlloc, after.HeapAlloc)
}

func TestWANW08OptionalIncomingSocketCeiling(t *testing.T) {
	listener, err := OptionalDirectListener(DirectSettings{})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	bounded := listener.(*directListener)
	accepted := make(chan net.Conn, MaxDirectIncomingConnections+1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		for {
			c, e := listener.Accept()
			if e != nil {
				return
			}
			accepted <- c
		}
	}()
	clients := []net.Conn{}
	defer func() {
		for _, c := range clients {
			c.Close()
		}
	}()
	// Connect one more than admission allows; only the kernel backlog retains the
	// excess socket, without another accepted TLS/HTTP worker.
	for i := 0; i < MaxDirectIncomingConnections+1; i++ {
		c, e := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
		if e != nil {
			t.Fatal(e)
		}
		clients = append(clients, c)
	}
	received := []net.Conn{}
	for i := 0; i < MaxDirectIncomingConnections; i++ {
		select {
		case c := <-accepted:
			received = append(received, c)
		case <-time.After(time.Second):
			t.Fatal("admission saturation unavailable")
		}
	}
	select {
	case c := <-accepted:
		c.Close()
		t.Fatal("pre-TLS admission overflow")
	case <-time.After(20 * time.Millisecond):
	}
	if len(bounded.slots) != MaxDirectIncomingConnections {
		t.Fatal("socket ceiling", len(bounded.slots))
	}
	listener.Close()
	<-joined
	for _, c := range received {
		c.Close()
		c.Close()
	}
	if len(bounded.slots) != 0 {
		t.Fatal("admission slot leaked", len(bounded.slots))
	}
	t.Logf("%d raw incoming sockets admitted; excess remains in kernel backlog; slots release on close", MaxDirectIncomingConnections)
}

func TestWANW08IndependentInterfaceLeasesAndAggregateCandidates(t *testing.T) {
	_, _, target, _ := lanRecord(t)
	m := NewManager(ManagerOptions{})
	defer m.Close()
	if err := m.RegisterDirect(target, nil); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Minute)
	for i := 1; i <= MaxDiscoveryInterfaces; i++ {
		c := []p.NetworkCandidate{{Transport: "tcp", Address: fmt.Sprintf("192.168.%d.2:4567", i), Scope: "lan", Interface: fmt.Sprintf("remote%d", i)}}
		if err := m.SetCandidates(target, c, 1, expires, false, i); err != nil {
			t.Fatal(err)
		}
	}
	if len(m.directCandidates(context.Background(), target)) != MaxDiscoveryInterfaces {
		t.Fatal("interface leases overwrote each other")
	}
	// Replacing one interface does not renew or replace another interface's lease.
	c := []p.NetworkCandidate{{Transport: "tcp", Address: "192.168.1.3:4567", Scope: "lan", Interface: "remote1"}}
	if err := m.SetCandidates(target, c, 2, expires, false, 1); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	if m.lan[lanKey{target, 2}].generation != 1 || m.lan[lanKey{target, 1}].generation != 2 {
		t.Error("interface generation mixed")
	}
	m.mu.Unlock()
	over := []p.NetworkCandidate{}
	for i := 0; i < 9; i++ {
		over = append(over, p.NetworkCandidate{Transport: "tcp", Address: fmt.Sprintf("8.8.8.8:%d", i+1), Scope: "public"})
	}
	if err := m.SetCandidates(target, over, 1, expires, true); !errors.Is(err, ErrBackpressure) {
		t.Fatal("aggregate device candidates exceeded 16", err)
	}
}
