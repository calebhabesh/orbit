package network

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	p "github.com/calebhabesh/file-sync/internal/protocol"
)

func TestWANW12DoctorCancellationAndAdmission(t *testing.T) {
	resolver := &testResolver{block: true}
	client := clientFixture(t, resolver)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	probes := client.ProbeService(ctx)
	if time.Since(started) > time.Second || len(probes) != 2 || probes[0].Code != "TIMEOUT" || probes[1].Code != "NOT_TESTED" {
		t.Fatal(probes, time.Since(started))
	}
	if resolver.active.Load() != 0 {
		t.Fatal("resolver leaked")
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if code := client.ProbeService(ctx)[0].Code; code != "CANCELLED" {
		t.Fatal(code)
	}
	for range 4 {
		client.slots <- struct{}{}
	}
	probes = client.ProbeService(context.Background())
	if probes[0].Code != "QUOTA_EXCEEDED" {
		t.Fatal(probes)
	}
	for range 4 {
		<-client.slots
	}
}
func TestWANW12PassiveCandidateFreshness(t *testing.T) {
	now := time.Now()
	m := NewManager(ManagerOptions{Now: func() time.Time { return now }})
	defer m.Close()
	target := Target{Purpose: PeerData}
	target.Device[0] = 1
	target.Pin[0] = 2
	if e := m.RegisterDirect(target, nil); e != nil {
		t.Fatal(e)
	}
	candidates := []p.NetworkCandidate{{Transport: "tcp", Address: "8.8.8.8:443", Scope: "public"}}
	if e := m.SetCandidates(target, candidates, 1, now.Add(time.Minute), true); e != nil {
		t.Fatal(e)
	}
	if summary := m.Candidates(target); summary.Public != 1 || summary.Expired != 0 {
		t.Fatal(summary)
	}
	now = now.Add(2 * time.Minute)
	if summary := m.Candidates(target); summary.Public != 0 || summary.Expired != 1 {
		t.Fatal(summary)
	}
	if o := m.Observe(target); o.Code != "NOT_TESTED" {
		t.Fatal(o)
	}
}
func TestWANW12UDPProbeUsesActualSTUNResponse(t *testing.T) {
	interfaces, err := SelectedInterfaces(nil)
	if err != nil {
		t.Fatal(err)
	}
	var address netip.Addr
	for _, i := range interfaces {
		for _, p := range i.Prefixes {
			if p.Addr().Is4() && p.Addr().IsPrivate() {
				address = p.Addr()
				break
			}
		}
	}
	if !address.IsValid() {
		t.Fatal("fixture needs a private IPv4 interface")
	}
	socket, err := net.ListenPacket("udp4", net.JoinHostPort(address.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewSTUNServer(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	result := ProbeUDP(context.Background(), []string{server.LocalAddr().String()}, nil)
	if result.Code != "VERIFIED" || result.ObservedAt == "" {
		t.Fatal(result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := ProbeUDP(ctx, []string{server.LocalAddr().String()}, nil); result.Code != "CANCELLED" {
		t.Fatal(result)
	}
	if result := ProbeUDP(context.Background(), nil, nil); result.Code != "NOT_TESTED" {
		t.Fatal(result)
	}
	t.Log("UDP response verified; no NAT classification inferred")
}
func TestWANW12ProbeCodesDoNotLeakRawErrors(t *testing.T) {
	if code := ProbeCode(context.Background(), errors.New("private_filename and ice_password")); code != "UNAVAILABLE" {
		t.Fatal(code)
	}
}

func TestWANW12PinnedProbesNeverSendHTTPOrAlterObservedRoute(t *testing.T) {
	var handled atomic.Int64
	server, trust, target := managerServer(t, func(http.ResponseWriter, *http.Request) { handled.Add(1) })
	manager := NewManager(ManagerOptions{})
	defer manager.Close()
	if e := manager.SetManual(target, server.URL); e != nil {
		t.Fatal(e)
	}
	result := manager.ProbeTCP(context.Background(), target, trust, false)
	if result.Code != "VERIFIED" || handled.Load() != 0 || manager.Observe(target).Code != "NOT_TESTED" {
		t.Fatal(result, handled.Load(), manager.Observe(target))
	}
	bad := trust.Clone()
	bad.VerifyConnection = func(tls.ConnectionState) error { return errors.New("peer public key pin mismatch") }
	result = manager.ProbeTCP(context.Background(), target, bad, false)
	if result.Code != "IDENTITY_MISMATCH" || handled.Load() != 0 {
		t.Fatal(result, handled.Load())
	}
	manager.connMu.Lock()
	defer manager.connMu.Unlock()
	if len(manager.connections) != 0 {
		t.Fatal("probe socket retained")
	}
}
