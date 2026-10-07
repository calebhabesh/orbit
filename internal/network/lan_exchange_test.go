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
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	p "github.com/calebhabesh/file-sync/internal/protocol"
)

// peerLAN is one side of an exchange: its signing key, pin and a discovery that
// signs records without sockets.
type peerLAN struct {
	key ed25519.PrivateKey
	pin [32]byte
	d   *LANDiscovery
}

func newPeerLAN(t *testing.T, m *ConnectionManager, device history.ID, iface LocalInterface, candidates []p.NetworkCandidate) peerLAN {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(pub)
	pin := sha256.Sum256(der)
	d := &LANDiscovery{manager: m, device: hex.EncodeToString(device[:]), pin: hex.EncodeToString(pin[:]), key: key, interfaces: []LocalInterface{iface}, candidates: candidates}
	if m != nil {
		d.known = m.KnownTargets
	}
	return peerLAN{key: key, pin: pin, d: d}
}

func lanCandidate(address, iface string) p.NetworkCandidate {
	return p.NetworkCandidate{Transport: "tcp", Address: address, Scope: "lan", Interface: iface}
}

func TestLANExchangeAcceptPeerScopesCandidates(t *testing.T) {
	m := NewManager(ManagerOptions{})
	defer m.Close()
	local := LocalInterface{Index: 3, Name: "enp6s0", Prefixes: []netip.Prefix{netip.MustParsePrefix("192.168.88.171/24"), netip.MustParsePrefix("172.18.0.1/16")}}
	self := newPeerLAN(t, m, history.ID{1}, local, nil)
	// The remote record mixes its LAN address, a container bridge address that
	// equals one of ours, and a subnet this device does not have.
	remoteIface := LocalInterface{Index: 9, Name: "eth0", Prefixes: []netip.Prefix{netip.MustParsePrefix("192.168.88.63/24")}}
	remote := newPeerLAN(t, nil, history.ID{2}, remoteIface, []p.NetworkCandidate{lanCandidate("192.168.88.63:4567", "eth0"), lanCandidate("172.18.0.1:4567", "eth0"), lanCandidate("10.9.9.9:4567", "eth0")})
	target := Target{Device: history.ID{2}, Pin: remote.pin, Purpose: PeerData}
	if err := m.RegisterDirect(target, nil); err != nil {
		t.Fatal(err)
	}
	records := remote.d.Records()
	if len(records) != 1 {
		t.Fatal("remote records", records)
	}
	if n := self.d.AcceptPeer(records, [32]byte{9}); n != 0 {
		t.Fatal("records accepted under another session pin", n)
	}
	if n := self.d.AcceptPeer(records, remote.pin); n != 1 {
		t.Fatal("installed", n)
	}
	got := m.directCandidates(context.Background(), target)
	if len(got) != 1 || got[0].Address != "192.168.88.63:4567" || got[0].Interface != "enp6s0" {
		t.Fatal("scoped peer candidates", got)
	}
	m.mu.Lock()
	lease := m.lan[lanKey{target, 3}]
	m.mu.Unlock()
	if !lease.peer {
		t.Fatal("peer-sent lease not marked")
	}
}

func TestLANExchangePeerLeaseKeepsDirectoryAndLinkLease(t *testing.T) {
	a, key, target, iface := lanRecord(t)
	m := NewManager(ManagerOptions{})
	defer m.Close()
	var lookups atomic.Int32
	if err := m.RegisterDirect(target, func(context.Context, Target, uint64) (p.NetworkAnnouncement, error) {
		lookups.Add(1)
		return p.NetworkAnnouncement{Generation: 1, Expires: p.NetworkUint(time.Now().Unix() + 60), Candidates: []p.NetworkCandidate{{Transport: "tcp", Address: "8.8.8.8:443", Scope: "public"}}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(5 * time.Minute)
	if err := m.SetPeerLANCandidates(target, []p.NetworkCandidate{lanCandidate("192.168.10.9:4567", "selected")}, 5, expires, iface.Index); err != nil {
		t.Fatal(err)
	}
	// A same-prefix peer address can belong to another home network: public
	// candidates are still gathered after it.
	if got := m.directCandidates(context.Background(), target); len(got) != 2 || lookups.Load() != 1 {
		t.Fatal("peer lease suppressed directory", got, lookups.Load())
	}
	// A record heard on the link replaces it and is not overwritten by a
	// later peer-sent record.
	m.mu.Lock()
	m.public = map[Target]candidateLease{}
	m.mu.Unlock()
	d := &LANDiscovery{manager: m, device: "local-device", known: m.KnownTargets}
	a.Generation = 6
	if err := d.Accept(signLAN(t, a, key), &net.UDPAddr{IP: net.ParseIP("192.168.10.2"), Port: DiscoveryPort}, iface); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPeerLANCandidates(target, []p.NetworkCandidate{lanCandidate("192.168.10.9:4567", "selected")}, 7, expires, iface.Index); err != nil {
		t.Fatal(err)
	}
	if got := m.directCandidates(context.Background(), target); len(got) != 1 || got[0].Address != "192.168.10.2:4567" || lookups.Load() != 1 {
		t.Fatal("link lease replaced or directory consulted", got, lookups.Load())
	}
	if err := m.SetPeerLANCandidates(target, make([]p.NetworkCandidate, MaxPeerLANCandidates+1), 8, expires, iface.Index); err == nil {
		t.Fatal("peer candidate cap")
	}
}

// The W14 case: the local device's firewall drops inbound multicast and direct
// connections, so only the relay works until the peers exchange records over
// it. Afterwards the local device dials the peer's LAN address directly.
func TestLANExchangeFirewalledPeerMovesFromRelayToDirect(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"example.com"}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pin := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	remoteDevice := history.ID{2}
	remote := &LANDiscovery{device: hex.EncodeToString(remoteDevice[:]), pin: hex.EncodeToString(pin[:]), key: key, interfaces: []LocalInterface{{Index: 2, Name: "eth0", Prefixes: []netip.Prefix{netip.MustParsePrefix("192.168.88.63/24")}}}, candidates: []p.NetworkCandidate{lanCandidate("192.168.88.63:4567", "eth0")}}
	var received atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != LANExchangePath {
			w.Write([]byte("synced"))
			return
		}
		var request p.LANExchange
		body, _ := io.ReadAll(r.Body)
		if p.NetworkDecode(body, &request) != nil || request.Validate(uint64(time.Now().Unix())) != nil || len(request.Records) != 1 {
			t.Error("invalid exchange request", string(body))
		}
		received.Add(1)
		reply, _ := json.Marshal(p.LANExchange{Version: "1", Records: remote.Records()})
		w.Write(reply)
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	trust := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "example.com", VerifyConnection: func(cs tls.ConnectionState) error {
		if sha256.Sum256(cs.PeerCertificates[0].RawSubjectPublicKeyInfo) != pin {
			return errors.New("wrong pin")
		}
		return nil
	}}
	var direct atomic.Int32
	m := NewManager(ManagerOptions{DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
		if address != "192.168.88.63:4567" {
			return nil, errors.New("unexpected dial " + address)
		}
		direct.Add(1)
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}})
	defer m.Close()
	target := Target{Device: remoteDevice, Pin: pin, Purpose: PeerData, Profile: history.Digest{1}}
	if err := m.SetRelay(target, func(ctx context.Context, _ Target) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterDirect(target, nil); err != nil {
		t.Fatal(err)
	}
	request := func() {
		rt, err := m.Transport(context.Background(), target, trust)
		if err != nil {
			t.Fatal(err)
		}
		res := round(t, rt, LogicalOrigin(target))
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}
	request()
	if m.Observe(target).Route != "relay" || direct.Load() != 0 {
		t.Fatal("expected relay before exchange", m.Observe(target))
	}
	local := newPeerLAN(t, m, history.ID{1}, LocalInterface{Index: 4, Name: "enp6s0", Prefixes: []netip.Prefix{netip.MustParsePrefix("192.168.88.171/24")}}, []p.NetworkCandidate{lanCandidate("192.168.88.171:5555", "enp6s0")})
	x := NewLANExchange(m)
	x.Set(local.d)
	if err := x.Exchange(context.Background(), local.d, target, func(Target) (*tls.Config, error) { return trust, nil }); err != nil {
		t.Fatal(err)
	}
	if received.Load() != 1 {
		t.Fatal("exchange not carried over the relay")
	}
	request()
	if m.Observe(target).Route != "direct" || direct.Load() == 0 {
		t.Fatal("still relayed after exchange", m.Observe(target), direct.Load())
	}
}

func TestLANExchangeEndpointAdmitsOnlyApprovedPeers(t *testing.T) {
	m := NewManager(ManagerOptions{})
	defer m.Close()
	x := NewLANExchange(m)
	if _, err := x.ExchangePeerLAN([32]byte{1}, []byte(`{"version":"1","records":[]}`)); !errors.Is(err, ErrLANExchangeUnavailable) {
		t.Fatal("no discovery", err)
	}
	iface := LocalInterface{Index: 1, Name: "eth0", Prefixes: []netip.Prefix{netip.MustParsePrefix("192.168.1.10/24")}}
	local := newPeerLAN(t, m, history.ID{1}, iface, []p.NetworkCandidate{lanCandidate("192.168.1.10:5555", "eth0")})
	x.Set(local.d)
	remote := newPeerLAN(t, nil, history.ID{2}, LocalInterface{Index: 1, Name: "wlan0", Prefixes: []netip.Prefix{netip.MustParsePrefix("192.168.1.20/24")}}, []p.NetworkCandidate{lanCandidate("192.168.1.20:6666", "wlan0")})
	body, _ := json.Marshal(p.LANExchange{Version: "1", Records: remote.d.Records()})
	if _, err := x.ExchangePeerLAN(remote.pin, body); !errors.Is(err, ErrLANExchangePeer) {
		t.Fatal("unapproved peer received records", err)
	}
	target := Target{Device: history.ID{2}, Pin: remote.pin, Purpose: PeerData}
	if err := m.RegisterDirect(target, nil); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"version":"2","records":[]}`, `{"version":"1"}`, `{"version":"1","records":[],"extra":1}`} {
		if _, err := x.ExchangePeerLAN(remote.pin, []byte(bad)); err == nil {
			t.Fatal("malformed exchange accepted", bad)
		}
	}
	reply, err := x.ExchangePeerLAN(remote.pin, body)
	if err != nil {
		t.Fatal(err)
	}
	var decoded p.LANExchange
	if err = p.NetworkDecode(reply, &decoded); err != nil || decoded.Validate(uint64(time.Now().Unix())) != nil || len(decoded.Records) != 1 || decoded.Records[0].Candidates[0].Address != "192.168.1.10:5555" {
		t.Fatal("reply", string(reply), err)
	}
	if got := m.directCandidates(context.Background(), target); len(got) != 1 || got[0].Address != "192.168.1.20:6666" {
		t.Fatal("request records not installed", got)
	}
}
