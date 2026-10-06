package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"math/big"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	p "github.com/calebhabesh/file-sync/internal/protocol"
)

type testResolver struct {
	addresses []netip.Addr
	calls     atomic.Int64
	active    atomic.Int64
	peak      atomic.Int64
	block     bool
}

func (r *testResolver) LookupNetIP(ctx context.Context, _, _ string) ([]netip.Addr, error) {
	r.calls.Add(1)
	active := r.active.Add(1)
	defer r.active.Add(-1)
	for {
		peak := r.peak.Load()
		if active <= peak || r.peak.CompareAndSwap(peak, active) {
			break
		}
	}
	if r.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return r.addresses, nil
}
func clientFixture(t *testing.T, resolver *testResolver) *ServiceClient {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "synthetic"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	profile := p.NetworkProfile{Version: "1", Operator: "Development test", Authority: hex.EncodeToString(pub), ServiceKey: hex.EncodeToString(pub), Epoch: 1, Expires: p.NetworkUint(time.Now().Unix() + 3600), Origins: []string{"https://directory.orbit.invalid"}, STUN: []string{}, Privacy: "Synthetic development operator; ephemeral metadata only."}
	b, err := profile.Canonical(false)
	if err != nil {
		t.Fatal(err)
	}
	profile.Signature = hex.EncodeToString(ed25519.Sign(key, b))
	client, err := NewServiceClient(ServiceClientOptions{Selection: ProfileSelection{Profile: profile, Authority: profile.Authority, HighestEpoch: 1, Environment: "development"}, Origin: profile.Origins[0], Device: hex.EncodeToString(pub), Certificate: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}
func TestWANW03DNSAllAnswersAndRebinding(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "100.64.0.1", "::1", "::ffff:8.8.8.8", "0.0.0.0", "192.0.2.1"} {
		r := &testResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr(address)}}
		if conn, err := safeServiceDial(context.Background(), r, "directory.orbit.invalid", "443", false); err == nil || conn != nil || err.Error() != p.NetworkProfileUntrusted {
			t.Fatal("unchecked DNS answer", address, err)
		}
	}
	r := &testResolver{addresses: make([]netip.Addr, 17)}
	for i := range r.addresses {
		r.addresses[i] = netip.MustParseAddr("8.8.8.8")
	}
	if _, err := safeServiceDial(context.Background(), r, "directory.orbit.invalid", "443", false); err == nil {
		t.Fatal("unbounded DNS answers")
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	var ip netip.Addr
	for _, a := range addrs {
		prefix, err := netip.ParsePrefix(a.String())
		if err == nil && prefix.Addr().Is4() && prefix.Addr().IsPrivate() && !prefix.Addr().IsLoopback() {
			ip = prefix.Addr()
			break
		}
	}
	if !ip.IsValid() {
		t.Fatal("nonloopback private interface required")
	}
	l, err := net.Listen("tcp", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	r.addresses = []netip.Addr{ip}
	conn, err := safeServiceDial(context.Background(), r, "directory.orbit.invalid", port, true)
	if err != nil {
		t.Fatal("reviewed private DNS", err)
	}
	_ = conn.Close()
	r.addresses = []netip.Addr{netip.MustParseAddr("127.0.0.1")}
	if _, err = safeServiceDial(context.Background(), r, "directory.orbit.invalid", port, true); err == nil {
		t.Fatal("reconnect accepted rebound loopback")
	}
	if r.calls.Load() != 3 {
		t.Fatal("resolver did not run on each connection", r.calls.Load())
	}
}
func TestWANW03CanceledResolverFloodIsBounded(t *testing.T) {
	resolver := &testResolver{block: true}
	client := clientFixture(t, resolver)
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			go func() { time.Sleep(time.Millisecond); cancel() }()
			_, _, _ = client.Lookup(ctx, client.device, client.pin, "peer_data", 0)
		}()
	}
	wg.Wait()
	done := make(chan struct{})
	go func() { _ = client.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("detached resolver escaped shutdown")
	}
	if resolver.peak.Load() > 4 || resolver.active.Load() != 0 || len(client.connections) != 0 {
		t.Fatal("resolver/socket ceiling", resolver.peak.Load(), resolver.active.Load(), len(client.connections))
	}
	t.Logf("1,000 canceled callers: resolver peak=%d; after close active=%d sockets=%d", resolver.peak.Load(), resolver.active.Load(), len(client.connections))
}
func TestWANW03PrivacyPolicyHasNoServiceTraffic(t *testing.T) {
	resolver := &testResolver{}
	client := clientFixture(t, resolver)
	for _, mode := range []string{"manual", "local_only"} {
		err := client.Reannounce(context.Background(), mode, "peer_data", func() p.NetworkAnnouncement {
			t.Fatal("privacy mode generated public candidates")
			return p.NetworkAnnouncement{}
		})
		if err == nil {
			t.Fatal("privacy policy activated")
		}
	}
	if err := client.Reannounce(context.Background(), "automatic", "peer_data", nil); err == nil || err.Error() != p.NetworkProfileUntrusted {
		t.Fatal("development profile used as automatic release default", err)
	}
	if resolver.calls.Load() != 0 {
		t.Fatal("privacy mode contacted DNS/service")
	}
}
func TestWANW03LookupRejectsSubstitution(t *testing.T) {
	resolver := &testResolver{}
	client := clientFixture(t, resolver)
	var proof p.NetworkProof
	var a p.NetworkAnnouncement
	if _, _, err := client.validateLookup(p.NetworkLookupResult{Version: "1", Found: false, Proofs: []p.NetworkProof{proof}, Announcements: []p.NetworkAnnouncement{a}}, client.device, client.pin, "peer_data", 0); err == nil {
		t.Fatal("absent response smuggled route")
	}
	for _, out := range []p.NetworkLookupResult{{Version: "2"}, {Version: "1", Found: true, Proofs: []p.NetworkProof{}, Announcements: []p.NetworkAnnouncement{}}, {Version: "1", Found: true, Proofs: []p.NetworkProof{proof}, Announcements: []p.NetworkAnnouncement{a}}} {
		if _, _, err := client.validateLookup(out, client.device, client.pin, "peer_data", 0); err == nil {
			t.Fatal("invalid/substituted response accepted")
		}
	}
	if _, err := client.transport.DialContext(context.Background(), "tcp", "127.0.0.1:443"); err == nil {
		t.Fatal("arbitrary URL dial permitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := client.Lookup(ctx, client.device, client.pin, "peer_data", 0); err == nil || err.Error() != p.NetworkCanceled {
		t.Fatal("canceled caller misreported service loss", err)
	}
	if !errors.Is(client.Close(), nil) {
		t.Fatal("close")
	}
}
