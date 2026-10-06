package replication

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/protocol"
	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

func quicHost(t *testing.T) (string, string) {
	t.Helper()
	interfaces, err := network.SelectedInterfaces(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range interfaces {
		for _, prefix := range iface.Prefixes {
			if prefix.Addr().Is4() && prefix.Addr().IsPrivate() {
				return prefix.Addr().String(), iface.Name
			}
		}
	}
	t.Fatal("native private IPv4 interface required")
	return "", ""
}
func quicFixture(t *testing.T, paired ...bool) (*syncFixture, *network.ConnectionManager, *network.ConnectionManager, *network.QUICEndpoint, *network.QUICEndpoint) {
	t.Helper()
	f, _ := routedFixture(t)
	host, iface := quicHost(t)
	a, b := network.NewManager(network.ManagerOptions{}), network.NewManager(network.ManagerOptions{})
	t.Cleanup(func() { a.Close(); b.Close() })

	sa, e := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(host)})
	if e != nil {
		t.Fatal(e)
	}
	sb, e := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(host)})
	if e != nil {
		t.Fatal(e)
	}
	var ca, cb net.PacketConn = sa, sb
	if len(paired) > 0 && paired[0] {
		pa, pb := &quicPairFixture{UDPConn: sa, remote: sb.LocalAddr()}, &quicPairFixture{UDPConn: sb, remote: sa.LocalAddr()}
		t.Cleanup(func() {
			if pa.shortPackets.Load() < 2 || pb.shortPackets.Load() < 2 {
				t.Error("loss/duplicate injection not exercised", pa.shortPackets.Load(), pb.shortPackets.Load())
			} else {
				t.Log("adapter lost and duplicated one encrypted short-header packet in each direction")
			}
		})
		ca, e = network.NewPairPacketConn(pa)
		if e != nil {
			t.Fatal(e)
		}
		cb, e = network.NewPairPacketConn(pb)
		if e != nil {
			t.Fatal(e)
		}
	}
	ea, e := network.NewQUICEndpoint(ca, f.senderID.ServerTLSConfig(), NewServer(f.senderRepo, f.senderID))
	if e != nil {
		t.Fatal(e)
	}
	eb, e := network.NewQUICEndpoint(cb, f.receiverID.ServerTLSConfig(), NewServer(f.receiverRepo, f.receiverID))
	if e != nil {
		t.Fatal(e)
	}

	if err := a.EnableQUIC(ea); err != nil {
		t.Fatal(err)
	}
	if err := b.EnableQUIC(eb); err != nil {
		t.Fatal(err)
	}
	ta := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
	tb := network.Target{Device: f.receiverID.DeviceID, Pin: f.receiverID.KeyPin, Purpose: network.PeerData}
	for _, route := range []struct {
		m    *network.ConnectionManager
		t    network.Target
		addr string
	}{{a, tb, eb.LocalAddr().String()}, {b, ta, ea.LocalAddr().String()}} {
		if e := route.m.RegisterDirect(route.t, nil); e != nil {
			t.Fatal(e)
		}
		if e := route.m.SetCandidates(route.t, []protocol.NetworkCandidate{{Transport: "udp", Address: route.addr, Scope: "lan", Interface: iface}}, 1, time.Now().Add(time.Minute), false); e != nil {
			t.Fatal(e)
		}
	}
	client, e := NewRoutedClient(context.Background(), network.LogicalOrigin(ta), f.receiverID, f.senderID.Leaf, ta, b)
	if e != nil {
		t.Fatal(e)
	}
	f.client = client
	return f, a, b, ea, eb
}

type quicPairFixture struct {
	*net.UDPConn
	remote       net.Addr
	shortPackets atomic.Int32
}

func (p *quicPairFixture) RemoteAddr() net.Addr { return p.remote }
func (p *quicPairFixture) Write(b []byte) (int, error) {
	if len(b) > 0 && b[0]&0x80 == 0 {
		switch p.shortPackets.Add(1) {
		case 1:
			return len(b), nil
		case 2:
			_, _ = p.UDPConn.WriteTo(b, p.remote)
		}
	}
	return p.UDPConn.WriteTo(b, p.remote)
}
func TestWANW09NativeQUICBothPullDirectionsAndResume(t *testing.T) {
	for _, paired := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "pair adapter"}[paired], func(t *testing.T) { verifyQUICBothDirections(t, paired) })
	}
}
func verifyQUICBothDirections(t *testing.T, paired bool) {
	f, a, b, _, _ := quicFixture(t, paired)
	ctx := context.Background()
	verifyInterruptedResume(t, f)
	tb := network.Target{Device: f.receiverID.DeviceID, Pin: f.receiverID.KeyPin, Purpose: network.PeerData}
	reverse, err := NewRoutedClient(ctx, network.LogicalOrigin(tb), f.senderID, f.receiverID.Leaf, tb, a)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte("reverse native UDP verified bytes\n"), 50000)
	if e := os.WriteFile(filepath.Join(f.receiverRoot, "reverse-quic.txt"), want, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := f.receiverWork.Scan(ctx, f.folder); e != nil {
		t.Fatal(e)
	}
	if _, e := NewSyncer(f.senderRepo, f.senderWork, reverse, f.senderID.DeviceID, f.receiverID.DeviceID, f.folder, f.approved, TransferOptions{}).Sync(ctx); e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(filepath.Join(f.senderRoot, "reverse-quic.txt"))
	if e != nil || !bytes.Equal(got, want) {
		t.Fatal("reverse bytes", e)
	}
	for _, path := range []string{"resume.bin", "reverse-quic.txt"} {
		ah, e := f.senderRepo.Heads(ctx, f.folder, path)
		if e != nil {
			t.Fatal(e)
		}
		bh, e := f.receiverRepo.Heads(ctx, f.folder, path)
		if e != nil {
			t.Fatal(e)
		}
		left, _ := json.Marshal(ah)
		right, _ := json.Marshal(bh)
		if !bytes.Equal(left, right) || len(ah) != 1 || f.senderRepo.VerifyManifest(ah[0].Manifest) != nil || f.receiverRepo.VerifyManifest(bh[0].Manifest) != nil {
			t.Fatal("heads/hash", path)
		}
	}
	ta := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
	if a.Observe(tb).Route != "quic" || b.Observe(ta).Route != "quic" {
		t.Fatal("native HTTP3 not selected", a.Observe(tb), b.Observe(ta))
	}
	t.Log("native UDP on", quicAddress(f.client), "both endpoints listen/dial; verified chunk reuse and exact heads/hashes")
}
func quicAddress(c *Client) string { return c.baseURL }

func quicHTTPClient(t *testing.T, id Identity, peer Identity, addr net.Addr, modify func(*tls.Config)) *http.Client {
	t.Helper()
	cfg, e := id.ClientTLSConfig(peer.Leaf, peer.KeyPin)
	if e != nil {
		t.Fatal(e)
	}
	cfg.NextProtos = []string{"h3"}
	if modify != nil {
		modify(cfg)
	}
	socket, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	qt := &quic.Transport{Conn: socket}
	rt := &http3.Transport{TLSClientConfig: cfg, MaxResponseHeaderBytes: 16 << 10, Dial: func(ctx context.Context, _ string, trust *tls.Config, _ *quic.Config) (*quic.Conn, error) {
		return qt.Dial(ctx, addr, trust, &quic.Config{Allow0RTT: false, MaxIncomingStreams: -1, MaxIncomingUniStreams: 4})
	}}
	t.Cleanup(func() { rt.Close(); qt.Close(); socket.Close() })
	return network.HTTPClient(rt)
}
func TestWANW09HTTP3AuthenticationAuthorizationAndBounds(t *testing.T) {
	f, _, _, ea, _ := quicFixture(t)
	// Use matching address families for the actual native socket.
	client := quicHTTPClient(t, f.receiverID, f.senderID, ea.LocalAddr(), nil)
	post := func(c *http.Client, path string, data string, header string) (int, error) {
		req, _ := http.NewRequest("POST", "https://peer.filesync.invalid"+path, strings.NewReader(data))
		if header != "" {
			req.Header.Set("X-Large", header)
		}
		res, e := c.Do(req)
		if e != nil {
			return 0, e
		}
		defer res.Body.Close()
		_, e = io.Copy(io.Discard, res.Body)
		return res.StatusCode, e
	}
	req := HelloRequest{ProtocolVersion: "1", DeviceID: hex.EncodeToString(f.receiverID.DeviceID[:]), Folders: []FolderHandshake{{FolderID: hex.EncodeToString(f.folder[:]), Revision: "1", MembershipDigest: hex.EncodeToString(f.approved.Digest[:])}}, Limits: Limits{MetadataBytes: "8388608", InventoryPage: "128"}}
	data, _ := json.Marshal(req)
	if code, e := post(client, "/peer/v1/hello", string(data), ""); e != nil || code != 200 {
		t.Fatal("hello TLS", code, e)
	}
	for _, path := range []string{"/control/v1/status", "/enrollment/v3/challenge"} {
		if code, e := post(client, path, "{}", ""); e != nil || code != 404 {
			t.Fatal("handler isolation", path, code, e)
		}
	}
	req.ProtocolVersion = "99"
	data, _ = json.Marshal(req)
	res, e := client.Post("https://peer.filesync.invalid/peer/v1/hello", "application/json", bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !bytes.Contains(body, []byte("INCOMPATIBLE_VERSION")) {
		t.Fatal(string(body))
	}
	inventory := InventoryRequest{ProtocolVersion: "1", DeviceID: hex.EncodeToString(f.receiverID.DeviceID[:]), FolderID: strings.Repeat("f", 64), Revision: "1", MembershipDigest: hex.EncodeToString(f.approved.Digest[:]), PageSize: "1"}
	raw, _ := json.Marshal(inventory)
	res, e = client.Post("https://peer.filesync.invalid/peer/v1/inventory", "application/json", bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if !bytes.Contains(body, []byte("UNAUTHORIZED")) {
		t.Fatal(string(body))
	}
	if code, e := post(client, "/peer/v1/hello", strings.Repeat("x", int(MaxMetadataBytes)+1), ""); e != nil || code != 413 {
		t.Fatal("body bound", code, e)
	}
	if code, e := post(client, "/peer/v1/hello", "{}", strings.Repeat("x", 33<<10)); e == nil && code != 431 {
		t.Fatal("header bound", code)
	}
	for name, modify := range map[string]func(*tls.Config){"missing": func(c *tls.Config) { c.Certificates = nil }, "wrong pin": func(c *tls.Config) {
		c.VerifyConnection = func(tls.ConnectionState) error { return errors.New("wrong pin") }
	}} {
		t.Run(name, func(t *testing.T) {
			bad := quicHTTPClient(t, f.receiverID, f.senderID, ea.LocalAddr(), modify)
			if _, e := post(bad, "/peer/v1/hello", string(data), ""); e == nil {
				t.Fatal("bad trust accepted")
			}
		})
	}
	// A valid unknown certificate is still refused by membership authorization.
	unknown, e := LoadOrCreateIdentity(t.TempDir(), history.ID{99}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	bad := quicHTTPClient(t, unknown, f.senderID, ea.LocalAddr(), nil)
	if code, e := post(bad, "/peer/v1/inventory", string(raw), ""); e != nil || code != 403 {
		t.Fatal("unknown member", code, e)
	}
}

type partialQUICChunkWriter struct{ http.ResponseWriter }

func (w partialQUICChunkWriter) Write(b []byte) (int, error) {
	if len(b) > 1 {
		n, _ := w.ResponseWriter.Write(b[:len(b)/2])
		return n, io.ErrUnexpectedEOF
	}
	return w.ResponseWriter.Write(b)
}
func TestWANW09PartialChunkNoReceiptAndBorrowerPin(t *testing.T) {
	f, _, b, _, _ := quicFixture(t)
	host, iface := quicHost(t)
	socket, e := net.ListenPacket("udp", net.JoinHostPort(host, "0"))
	if e != nil {
		t.Fatal(e)
	}
	server := NewServer(f.senderRepo, f.senderID)
	partial, e := network.NewQUICEndpoint(socket, f.senderID.ServerTLSConfig(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/peer/v1/chunks/get" {
			server.ServeHTTP(partialQUICChunkWriter{w}, r)
			return
		}
		server.ServeHTTP(w, r)
	}))
	if e != nil {
		t.Fatal(e)
	}
	defer partial.Close()
	target := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
	if e = b.SetCandidates(target, []protocol.NetworkCandidate{{Transport: "udp", Address: partial.LocalAddr().String(), Scope: "lan", Interface: iface}}, 2, time.Now().Add(time.Minute), false); e != nil {
		t.Fatal(e)
	}
	b.AdvanceGeneration()
	client, e := NewRoutedClient(context.Background(), network.LogicalOrigin(target), f.receiverID, f.senderID.Leaf, target, b)
	if e != nil {
		t.Fatal(e)
	}
	f.client = client
	want := bytes.Repeat([]byte("partial-native-quic\n"), 10000)
	if e = os.WriteFile(filepath.Join(f.senderRoot, "partial.bin"), want, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = f.senderWork.Scan(context.Background(), f.folder); e != nil {
		t.Fatal(e)
	}
	if _, e = f.newSyncer(TransferOptions{}).Sync(context.Background()); e == nil {
		t.Fatal("partial chunk accepted")
	}
	heads, e := f.senderRepo.Heads(context.Background(), f.folder, "partial.bin")
	if e != nil || len(heads) != 1 {
		t.Fatal(e)
	}
	available, e := f.receiverRepo.VerifiedChunk(context.Background(), heads[0].Manifest.Chunks[0])
	if e != nil || available {
		t.Fatal("partial bytes verified", e)
	}
	ready, e := f.receiverRepo.ContentReady(context.Background(), heads[0].ID)
	if e != nil || ready {
		t.Fatal("partial content ready", e)
	}
	peers, e := f.senderRepo.PeerProgress(context.Background(), f.folder)
	if e != nil {
		t.Fatal(e)
	}
	for _, peer := range peers {
		if peer.Receipt {
			t.Fatal("premature stored receipt")
		}
	}
	if _, e = os.Stat(filepath.Join(f.receiverRoot, "partial.bin")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("partial publication", e)
	}
	// Reusing an authenticated QUIC pool must enforce the current borrower's
	// verifier before encoding another HTTP request, just as the TCP seam does.
	trust, e := f.receiverID.ClientTLSConfig(f.senderID.Leaf, f.senderID.KeyPin)
	if e != nil {
		t.Fatal(e)
	}
	denied := errors.New("borrower rejects cached peer")
	trust.VerifyConnection = func(tls.ConnectionState) error { return denied }
	rt, e := b.Transport(context.Background(), target, trust)
	if e != nil {
		t.Fatal(e)
	}
	req, _ := http.NewRequest("POST", network.LogicalOrigin(target)+"/peer/v1/hello", strings.NewReader("{}"))
	if _, e = rt.RoundTrip(req); !errors.Is(e, denied) {
		t.Fatal("borrowed verifier bypass", e)
	}
}

func TestWANW09UnavailableUDPUsesPreservedPinnedTCP(t *testing.T) {
	f, _, manager, _, _ := quicFixture(t)
	host, iface := quicHost(t)
	target := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
	occupied, err := net.ListenPacket("udp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	// A bound UDP socket that never answers reproduces unavailable QUIC without
	// modifying a host firewall or claiming a particular NAT/filter type.
	if err = manager.SetCandidates(target, []protocol.NetworkCandidate{{Transport: "udp", Address: occupied.LocalAddr().String(), Scope: "lan", Interface: iface}}, 2, time.Now().Add(time.Minute), false); err != nil {
		t.Fatal(err)
	}
	if err = manager.SetManual(target, "https://"+f.listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	client, err := NewRoutedClient(context.Background(), "https://"+f.listener.Addr().String(), f.receiverID, f.senderID.Leaf, target, manager)
	if err != nil {
		t.Fatal(err)
	}
	f.client = client
	data := []byte("optional UDP cannot prevent pinned TCP progress")
	if err = os.WriteFile(filepath.Join(f.senderRoot, "tcp-fallback.txt"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.senderWork.Scan(context.Background(), f.folder); err != nil {
		t.Fatal(err)
	}
	if _, err = f.newSyncer(TransferOptions{}).Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(f.receiverRoot, "tcp-fallback.txt"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal(err)
	}
	if manager.Observe(target).Route != "direct" {
		t.Fatal(manager.Observe(target))
	}
}
