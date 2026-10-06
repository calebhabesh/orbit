package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	p "github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/pion/ice/v4"
	"github.com/pion/logging"
	"github.com/pion/stun/v4"
	"github.com/pion/transport/v5/vnet"
	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

func iceVirtualPair(t *testing.T, natType *vnet.NATType, blocked bool, doubleNAT ...bool) (*ICESession, *ICESession, *vnet.Router) {
	t.Helper()
	root := t.TempDir()
	if e := os.WriteFile(filepath.Join(root, ".filesync-disposable"), []byte("W10 in-process virtual network only\n"), 0600); e != nil {
		t.Fatal(e)
	}
	log := logging.NewDefaultLoggerFactory()
	wan, e := vnet.NewRouter(&vnet.RouterConfig{CIDR: "0.0.0.0/0", LoggerFactory: log})
	if e != nil {
		t.Fatal(e)
	}
	serverNet, e := vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{"11.1.1.1"}})
	if e != nil {
		t.Fatal(e)
	}
	if e = wan.AddNet(serverNet); e != nil {
		t.Fatal(e)
	}
	nets := make([]*vnet.Net, 2)
	for n := range 2 {
		ip := fmt.Sprintf("11.2.%d.1", n+1)
		local := ip
		if natType != nil {
			local = fmt.Sprintf("10.%d.0.2", n+1)
			lan, err := vnet.NewRouter(&vnet.RouterConfig{CIDR: fmt.Sprintf("10.%d.0.0/24", n+1), StaticIPs: []string{ip}, NATType: natType, LoggerFactory: log})
			if err != nil {
				t.Fatal(err)
			}
			nets[n], err = vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{local}})
			if err != nil {
				t.Fatal(err)
			}
			if err = lan.AddNet(nets[n]); err != nil {
				t.Fatal(err)
			}
			if len(doubleNAT) > 0 && doubleNAT[0] {
				outer, err := vnet.NewRouter(&vnet.RouterConfig{CIDR: fmt.Sprintf("172.16.%d.0/24", n+1), StaticIPs: []string{ip}, NATType: natType, LoggerFactory: log})
				if err != nil {
					t.Fatal(err)
				}
				// The inner router's externally assigned address is private to the outer.
				// Reconstruct the inner router before linking to the outer WAN.
				inner, err := vnet.NewRouter(&vnet.RouterConfig{CIDR: fmt.Sprintf("10.%d.0.0/24", n+1), StaticIPs: []string{fmt.Sprintf("172.16.%d.2", n+1)}, NATType: &vnet.NATType{MappingBehavior: vnet.EndpointIndependent, FilteringBehavior: vnet.EndpointIndependent}, LoggerFactory: log})
				if err != nil {
					t.Fatal(err)
				}
				// A fresh Net avoids attaching one stack to two routers.
				nets[n], err = vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{local}})
				if err != nil {
					t.Fatal(err)
				}
				if err = inner.AddNet(nets[n]); err != nil {
					t.Fatal(err)
				}
				if err = outer.AddRouter(inner); err != nil {
					t.Fatal(err)
				}
				if err = wan.AddRouter(outer); err != nil {
					t.Fatal(err)
				}
			} else if err = wan.AddRouter(lan); err != nil {
				t.Fatal(err)
			}
		} else {
			nets[n], e = vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{local}})
			if e != nil {
				t.Fatal(e)
			}
			if e = wan.AddNet(nets[n]); e != nil {
				t.Fatal(e)
			}
		}
	}
	if blocked {
		wan.AddChunkFilter(func(c vnet.Chunk) bool {
			return strings.HasPrefix(c.SourceAddr().String(), "11.1.1.1:") || strings.HasPrefix(c.DestinationAddr().String(), "11.1.1.1:")
		})
	}
	if e = wan.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = wan.Stop() })
	socket, e := serverNet.ListenPacket("udp", "11.1.1.1:3478")
	if e != nil {
		t.Fatal(e)
	}
	server, e := NewSTUNServer(socket)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = server.Close() })
	sessions := make([]*ICESession, 2)
	for n := range 2 {
		opts := []ice.AgentOption{ice.WithNet(nets[n]), ice.WithNetworkTypes([]ice.NetworkType{ice.NetworkTypeUDP4}), ice.WithCandidateTypes([]ice.CandidateType{ice.CandidateTypeHost, ice.CandidateTypeServerReflexive}), ice.WithMulticastDNSMode(ice.MulticastDNSModeDisabled), ice.WithUrls([]*stun.URI{{Scheme: stun.SchemeTypeSTUN, Host: "11.1.1.1", Port: 3478, Proto: stun.ProtoTypeUDP}}), ice.WithSTUNGatherTimeout(time.Second), ice.WithRemoteIPFilter(func(ip net.IP) bool {
			a, ok := netip.AddrFromSlice(ip)
			return ok && p.AllowedAddress(a.Unmap(), false)
		})}
		sessions[n], e = newICESession(context.Background(), opts)
		if e != nil {
			t.Fatal("gather", e)
		}
		t.Cleanup(func() { _ = sessions[n].Close() })
	}
	t.Logf("simulated topology: NAT=%+v blocked-peer-UDP=%v; STUN=11.1.1.1:3478; no host network mutations", natType, blocked)
	return sessions[0], sessions[1], wan
}
func connectICEPair(t *testing.T, a, b *ICESession, success bool) (*PairPacketConn, *PairPacketConn) {
	t.Helper()
	var pa, pb *PairPacketConn
	var ea, eb error
	var wg sync.WaitGroup
	wg.Go(func() { pa, ea = a.Connect(context.Background(), b.Description("accept"), true) })
	wg.Go(func() { pb, eb = b.Connect(context.Background(), a.Description("offer"), false) })
	wg.Wait()
	if success && (ea != nil || eb != nil) {
		t.Fatal("connect", ea, eb)
	}
	if !success && (ea == nil || eb == nil) {
		t.Fatal("expected check failure", ea, eb)
	}
	return pa, pb
}
func TestWANW10ICEHTTP3NATMatrix(t *testing.T) {
	cases := []struct {
		name             string
		nat              *vnet.NATType
		blocked, success bool
	}{
		{"no NAT", nil, false, true},
		{"endpoint independent", &vnet.NATType{MappingBehavior: vnet.EndpointIndependent, FilteringBehavior: vnet.EndpointIndependent}, false, true},
		{"address dependent filtering", &vnet.NATType{MappingBehavior: vnet.EndpointIndependent, FilteringBehavior: vnet.EndpointAddrDependent}, false, true},
		{"address port dependent filtering", &vnet.NATType{MappingBehavior: vnet.EndpointIndependent, FilteringBehavior: vnet.EndpointAddrPortDependent}, false, true},
		{"incompatible address port mappings", &vnet.NATType{MappingBehavior: vnet.EndpointAddrPortDependent, FilteringBehavior: vnet.EndpointAddrPortDependent}, false, false},
		{"blocked peer UDP", &vnet.NATType{MappingBehavior: vnet.EndpointIndependent, FilteringBehavior: vnet.EndpointIndependent}, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, b, wan := iceVirtualPair(t, c.nat, c.blocked)
			pa, pb := connectICEPair(t, a, b, c.success)
			if !c.success {
				return
			}
			s, trust, _ := managerServer(t, func(http.ResponseWriter, *http.Request) {})
			defer s.Close()
			server := s.TLS.Clone()
			server.ClientAuth = tls.RequireAnyClientCert
			trust.Certificates = server.Certificates
			trust.NextProtos = []string{"h3"}
			want := bytes.Repeat([]byte("bounded actual ICE HTTP3 bytes\n"), 50000)
			digest := sha256.Sum256(want)
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.TLS == nil || len(r.TLS.PeerCertificates) != 1 {
					t.Error("missing TLS")
				}
				_, _ = io.Copy(w, bytes.NewReader(want))
			})
			ea, e := NewQUICEndpoint(pa, server, h)
			if e != nil {
				t.Fatal(e)
			}
			defer ea.Close()
			eb, e := NewQUICEndpoint(pb, server, h)
			if e != nil {
				t.Fatal(e)
			}
			defer eb.Close()
			// One actual encrypted QUIC datagram is lost after checks; HTTP3 must recover.
			var drops atomic.Int32
			wan.AddChunkFilter(func(chunk vnet.Chunk) bool {
				data := chunk.UserData()
				if len(data) > 0 && !stun.IsMessage(data) && data[0]&0x80 == 0 && drops.CompareAndSwap(0, 1) {
					return false
				}
				return true
			})
			for _, direction := range []struct {
				from, to *QUICEndpoint
				address  net.Addr
			}{{ea, eb, pa.remoteAddr()}, {eb, ea, pb.remoteAddr()}} {
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				conn, e := direction.from.transport.Dial(ctx, direction.address, trust, peerQUICConfig())
				if e != nil {
					cancel()
					t.Fatal("QUIC over ICE", e)
				}
				req, _ := http.NewRequestWithContext(ctx, "GET", "https://example.com/", nil)
				res, e := (&http3.Transport{MaxResponseHeaderBytes: 16 << 10}).NewClientConn(conn).RoundTrip(req)
				if e != nil {
					cancel()
					t.Fatal(e)
				}
				got, e := io.ReadAll(res.Body)
				_ = res.Body.Close()
				cancel()
				_ = conn.CloseWithError(0, "")
				if e != nil || sha256.Sum256(got) != digest {
					t.Fatal("body hash", e)
				}
			}
			if drops.Load() != 1 {
				t.Fatal("loss not exercised")
			}
			a.pairChanged("changed-pair")
			select {
			case <-a.changed:
			default:
				t.Fatal("pair change not invalidated")
			}
			_ = a.Close()
			if _, _, e := pa.ReadFrom(make([]byte, 1200)); e == nil {
				t.Fatal("changed read")
			}
		})
	}
}
func TestWANW10ICEGatherCancellationAndCredentials(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s, e := NewICESession(ctx, nil, nil); e == nil {
		_ = s.Close()
		t.Fatal("canceled gather")
	}
	a, _, _ := iceVirtualPair(t, nil, false)
	d := a.Description("offer")
	if len(d.Password) < 22 {
		t.Fatal("weak credentials")
	}
	_ = a.Close()
	if a.Description("offer").Password != "" {
		t.Fatal("retained credentials")
	}
}

func TestWANW10ICESocketAndSessionAdmission(t *testing.T) {
	socket := &iceSocket{servers: map[string]bool{}, sources: map[string]bool{}}
	for n := range 1000 {
		addr := &net.UDPAddr{IP: net.ParseIP("11.23.45.1"), Port: n + 1}
		allowed := socket.allowed([]byte{0x40}, addr)
		if allowed != (n < 32) {
			t.Fatal("source cap", n, allowed)
		}
	}
	if len(socket.sources) != 32 || socket.allowed(make([]byte, 1201), &net.UDPAddr{IP: net.ParseIP("11.23.45.1"), Port: 1}) || socket.allowed([]byte{0x40}, &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1}) {
		t.Fatal("socket admission")
	}
	i := &iceCoordinator{owner: &RelayEndpoint{ctx: context.Background()}, routes: map[Target]*iceRoute{}, workers: make(chan struct{}, 2)}
	targets := []Target{{Device: [32]byte{1}}, {Device: [32]byte{2}}, {Device: [32]byte{3}}}
	a, start, e := i.allocate(targets[0], 1, 2, "one")
	if e != nil || !start {
		t.Fatal(e)
	}
	same, start, e := i.allocate(targets[0], 1, 2, "two")
	if e != nil || start || same != a {
		t.Fatal("not coalesced", e)
	}
	if _, _, e = i.allocate(targets[0], 2, 2, "stale"); e != ErrStale {
		t.Fatal("generation", e)
	}
	b, _, e := i.allocate(targets[1], 1, 2, "two")
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = i.allocate(targets[2], 1, 2, "three"); e != ErrBackpressure {
		t.Fatal("check cap", e)
	}
	i.complete(targets[0], a, ErrICEChecks)
	i.complete(targets[1], b, ErrICEChecks)
	if len(i.routes) != 0 || len(i.workers) != 0 {
		t.Fatal("failure leak")
	}
	resident := make([]*iceRoute, 0, 8)
	for n := range 8 {
		tgt := Target{Device: [32]byte{byte(n + 1)}}
		r, _, err := i.allocate(tgt, 1, 2, "resident")
		if err != nil {
			t.Fatal(err)
		}
		resident = append(resident, r)
		i.complete(tgt, r, nil)
	}
	if _, _, err := i.allocate(Target{Device: [32]byte{99}}, 1, 2, "overflow"); err != ErrBackpressure {
		t.Fatal("resident pair cap", err)
	}
	i.retireGeneration(2)
	if len(i.routes) != 0 {
		t.Fatal("generation retained pairs")
	}
	for _, r := range resident {
		select {
		case <-r.ctx.Done():
		default:
			t.Fatal("generation did not cancel pair")
		}
	}
	i.close()
	if _, _, e = i.allocate(targets[0], 1, 2, "closed"); e != ErrClosed {
		t.Fatal("shutdown", e)
	}
}

func TestWANW10IncompatibleDoubleNAT(t *testing.T) {
	a, b, _ := iceVirtualPair(t, &vnet.NATType{MappingBehavior: vnet.EndpointAddrPortDependent, FilteringBehavior: vnet.EndpointAddrPortDependent}, false, true)
	connectICEPair(t, a, b, false)
	t.Log("two NAT layers per endpoint: inner endpoint-independent; outer address/port-dependent mapping/filtering; bounded check failure")
}

func TestWANW10ICESlowStreamCancellation(t *testing.T) {
	a, b, _ := iceVirtualPair(t, &vnet.NATType{MappingBehavior: vnet.EndpointIndependent, FilteringBehavior: vnet.EndpointIndependent}, false)
	pa, pb := connectICEPair(t, a, b, true)
	s, trust, _ := managerServer(t, func(http.ResponseWriter, *http.Request) {})
	defer s.Close()
	server := s.TLS.Clone()
	server.ClientAuth = tls.RequireAnyClientCert
	trust.Certificates = server.Certificates
	trust.NextProtos = []string{"h3"}
	var sent atomic.Int64
	handlerDone := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		w.Header().Set("Content-Length", "33554432")
		buffer := make([]byte, 32<<10)
		for sent.Load() < 32<<20 {
			n, e := w.Write(buffer)
			sent.Add(int64(n))
			if e != nil {
				return
			}
		}
	})
	ea, e := NewQUICEndpoint(pa, server, handler)
	if e != nil {
		t.Fatal(e)
	}
	defer ea.Close()
	eb, e := NewQUICEndpoint(pb, server, http.NotFoundHandler())
	if e != nil {
		t.Fatal(e)
	}
	defer eb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, e := eb.transport.Dial(ctx, pb.remoteAddr(), trust, peerQUICConfig())
	if e != nil {
		t.Fatal(e)
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://example.com/", nil)
	res, e := (&http3.Transport{MaxResponseHeaderBytes: 16 << 10}).NewClientConn(conn).RoundTrip(req)
	if e != nil {
		t.Fatal(e)
	}
	time.Sleep(200 * time.Millisecond)
	if sent.Load() == 0 || sent.Load() > 1<<20 {
		t.Fatal("slow receiver flow-control bound", sent.Load())
	}
	cancel()
	_ = res.Body.Close()
	_ = conn.CloseWithError(0, "")
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("ICE stream cancellation did not join handler")
	}
	t.Log("offered 32 MiB; sent before cancellation", sent.Load(), "bytes; fixed 32 KiB write buffer and unchanged HTTP3 receive windows")
}

func TestWANW10SharedQUICAdmission(t *testing.T) {
	s, trust, _ := managerServer(t, func(http.ResponseWriter, *http.Request) {})
	defer s.Close()
	server := s.TLS.Clone()
	server.ClientAuth = tls.RequireAnyClientCert
	trust.Certificates = server.Certificates
	trust.NextProtos = []string{"h3"}
	manager := NewManager(ManagerOptions{})
	defer manager.Close()
	endpoints := make([]*QUICEndpoint, 2)
	for n := range 2 {
		socket, e := net.ListenPacket("udp4", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		endpoints[n], e = manager.NewPeerQUICEndpoint(socket, server, http.NotFoundHandler())
		if e != nil {
			t.Fatal(e)
		}
		defer endpoints[n].Close()
	}
	for n := range MaxActivePeerSlots {
		testQUICDial(t, endpoints[n%2], trust)
	}
	socket, e := net.ListenPacket("udp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	transport := &quic.Transport{Conn: socket}
	defer transport.Close()
	defer socket.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	extra, e := transport.Dial(ctx, endpoints[0].LocalAddr(), trust, peerQUICConfig())
	if e == nil {
		_ = extra.CloseWithError(0, "")
		t.Fatal("33rd daemon QUIC session admitted")
	}
	if len(manager.quicSlots) != MaxActivePeerSlots {
		t.Fatal("shared admission", len(manager.quicSlots))
	}
	for _, endpoint := range endpoints {
		_ = endpoint.Close()
	}
	deadline := time.Now().Add(time.Second)
	for len(manager.quicSlots) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(manager.quicSlots) != 0 {
		t.Fatal("QUIC admission leak", len(manager.quicSlots))
	}
}
