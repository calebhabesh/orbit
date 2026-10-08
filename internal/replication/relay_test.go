package replication

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/rendezvous"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func productionRelayService(t *testing.T, stunServers ...string) (*rendezvous.Service, network.ProfileSelection, string, *x509.CertPool, func() *rendezvous.Service) {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	var ip net.IP
	for _, a := range addrs {
		host, _, _ := net.ParseCIDR(a.String())
		if host != nil && host.To4() != nil && host.IsPrivate() && !host.IsLoopback() {
			ip = host
			break
		}
	}
	if ip == nil {
		t.Fatal("nonloopback private interface required")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	origin := "https://" + listener.Addr().String()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "disposable directory"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IPAddresses: []net.IP{ip}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	profile := p.NetworkProfile{Version: "1", Operator: "Development directory loss fixture", Authority: hex.EncodeToString(pub), ServiceKey: hex.EncodeToString(pub), Epoch: 1, Expires: p.NetworkUint(now.Unix() + 3600), Origins: []string{origin, "wss" + origin[5:]}, STUN: append([]string{}, stunServers...), Privacy: "Development only; metadata expires in memory; no logs or folder data."}
	b, _ := profile.Canonical(true)
	profile.Signature = hex.EncodeToString(ed25519.Sign(key, b))
	selection := network.ProfileSelection{Profile: profile, Authority: profile.Authority, HighestEpoch: 1, Environment: "development"}
	service, err := rendezvous.New(rendezvous.Options{Selection: selection, Origin: origin, ServiceKey: key})
	if err != nil {
		t.Fatal(err)
	}
	var live atomic.Pointer[rendezvous.Service]
	live.Store(service)
	t.Cleanup(func() { _ = live.Load().Close() })
	server := service.Server()
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { live.Load().ServeHTTP(w, r) })
	server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, NextProtos: []string{"http/1.1"}}
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(rendezvous.BoundedListener(listener), "", "") }()
	t.Cleanup(func() { _ = server.Close(); <-done })
	roots := x509.NewCertPool()
	roots.AddCert(leaf)

	restart := func() *rendezvous.Service {
		_ = live.Load().Close()
		next, err := rendezvous.New(rendezvous.Options{Selection: selection, Origin: origin, ServiceKey: key})
		if err != nil {
			t.Fatal(err)
		}
		live.Store(next)
		return next
	}
	return service, selection, origin, roots, restart
}

type productionRelay struct {
	service *rendezvous.Service
	a, b    *network.ConnectionManager
	ca, cb  *network.ServiceClient
	ea, eb  *network.RelayEndpoint
	ta, tb  network.Target
	restart func() *rendezvous.Service
}

func connectProductionRelay(t *testing.T, f *syncFixture, decorate ...func(http.Handler) http.Handler) *productionRelay {
	t.Helper()
	for _, db := range []*repository.DB{f.senderRepo, f.receiverRepo} {
		if err := os.WriteFile(filepath.Join(db.StateDir(), testkit.Marker), []byte("W04 disposable synthetic state\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	service, selection, origin, roots, restart := productionRelayService(t)
	mA, mB := network.NewManager(network.ManagerOptions{}), network.NewManager(network.ManagerOptions{})
	t.Cleanup(func() { _ = mA.Close(); _ = mB.Close() })
	var clients []*network.ServiceClient
	for _, id := range []Identity{f.senderID, f.receiverID} {
		c, err := network.NewServiceClient(network.ServiceClientOptions{Selection: selection, Origin: origin, Device: hex.EncodeToString(id.DeviceID[:]), Certificate: id.Certificate, Roots: roots})
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
		t.Cleanup(func() { _ = c.Close() })
		err = c.Announce(f.ctx, "peer_data", fmt.Sprintf("%064x", len(clients)), p.NetworkAnnouncement{Generation: 1, Expires: p.NetworkUint(time.Now().Unix() + 600), Candidates: []p.NetworkCandidate{}, Capabilities: []string{"relay_inner_tls_v1"}, Relay: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	ca, cb := clients[0], clients[1]
	lA, _ := mA.IncomingListener(network.PeerData)
	lB, _ := mB.IncomingListener(network.PeerData)
	var serverWG sync.WaitGroup
	servers := []*http.Server{NewServer(f.senderRepo, f.senderID).HTTPServer(), NewServer(f.receiverRepo, f.receiverID).HTTPServer()}
	if len(decorate) > 0 {
		servers[0].Handler = decorate[0](servers[0].Handler)
	}
	for i, l := range []*network.StreamListener{lA, lB} {
		server := servers[i]
		serverWG.Go(func() { _ = server.Serve(tls.NewListener(l, server.TLSConfig)) })
	}
	t.Cleanup(func() {
		for _, s := range servers {
			_ = s.Close()
		}
		_ = lA.Close()
		_ = lB.Close()
		serverWG.Wait()
	})
	ea, err := network.NewRelayEndpoint(f.ctx, ca, network.PeerData, 1, map[string]string{hex.EncodeToString(f.receiverID.DeviceID[:]): hex.EncodeToString(f.receiverID.KeyPin[:])}, false, lA)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ea.Close() })
	eb, err := network.NewRelayEndpoint(f.ctx, cb, network.PeerData, 1, map[string]string{hex.EncodeToString(f.senderID.DeviceID[:]): hex.EncodeToString(f.senderID.KeyPin[:])}, false, lB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eb.Close() })
	digest, _ := selection.Digest()
	raw, _ := hex.DecodeString(digest)
	var profile history.Digest
	copy(profile[:], raw)
	ta := network.Target{Device: f.receiverID.DeviceID, Pin: f.receiverID.KeyPin, Purpose: network.PeerData, Profile: profile}
	tb := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData, Profile: profile}
	if err = mA.SetRelay(ta, ea.Dial); err != nil {
		t.Fatal(err)
	}
	if err = mB.SetRelay(tb, eb.Dial); err != nil {
		t.Fatal(err)
	}
	// The direct fixture listener is deliberately unavailable. Logical peer names
	// cannot resolve to a direct address and every dial uses the approved service.
	_ = f.listener.Close()
	f.client, err = NewRoutedClient(f.ctx, network.LogicalOrigin(tb), f.receiverID, f.senderID.Leaf, tb, mB)
	if err != nil {
		t.Fatal(err)
	}
	return &productionRelay{service, mA, mB, ca, cb, ea, eb, ta, tb, restart}
}
func TestWANW04ProductionRelayTwoWaySyncAndInterruptedReuse(t *testing.T) {
	f := newSyncFixture(t)
	r := connectProductionRelay(t, f)
	verifyInterruptedResume(t, f)
	before, err := f.senderRepo.Heads(f.ctx, f.folder, "resume.bin")
	if err != nil {
		t.Fatal(err)
	}
	after, err := f.receiverRepo.Heads(f.ctx, f.folder, "resume.bin")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if !bytes.Equal(a, b) {
		t.Fatal("relay heads differ")
	}
	reverse, err := NewRoutedClient(f.ctx, network.LogicalOrigin(r.ta), f.senderID, f.receiverID.Leaf, r.ta, r.a)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("synthetic reverse pull through production encrypted WSS relay")
	if err = os.WriteFile(filepath.Join(f.receiverRoot, "reverse-relay.txt"), want, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.receiverWork.Scan(f.ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	if _, err = NewSyncer(f.senderRepo, f.senderWork, reverse, f.senderID.DeviceID, f.receiverID.DeviceID, f.folder, f.approved, TransferOptions{}).Sync(f.ctx); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(f.senderRoot, "reverse-relay.txt"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("reverse bytes", err)
	}
	local, _ := f.senderRepo.Heads(f.ctx, f.folder, "reverse-relay.txt")
	remote, _ := f.receiverRepo.Heads(f.ctx, f.folder, "reverse-relay.txt")
	a, _ = json.Marshal(local)
	b, _ = json.Marshal(remote)
	if !bytes.Equal(a, b) || len(local) != 1 || f.senderRepo.VerifyManifest(local[0].Manifest) != nil {
		t.Fatal("reverse head/hash")
	}
	if r.a.Observe(r.ta).Route != "relay" || r.b.Observe(r.tb).Route != "relay" {
		t.Fatal("relay observations")
	}
}
func TestWANW04ProductionRelayFolderAndPinIsolation(t *testing.T) {
	f := newSyncFixture(t)
	r := connectProductionRelay(t, f)
	unshared := fixedID('X')
	req := InventoryRequest{ProtocolVersion: ProtocolVersion, DeviceID: hex.EncodeToString(f.receiverID.DeviceID[:]), FolderID: hex.EncodeToString(unshared[:]), Revision: "1", MembershipDigest: hex.EncodeToString(f.approved.Digest[:]), PageSize: "1"}
	_, err := f.client.Inventory(f.ctx, req)
	var wire *WireError
	if !errors.As(err, &wire) || wire.Body.Code != "UNAUTHORIZED" {
		t.Fatal("unshared folder", err)
	}
	wrong := r.tb
	wrong.Pin = history.Digest{1}
	if err = r.b.SetRelay(wrong, r.eb.Dial); err == nil {
		t.Fatal("pin substitution")
	}
	bad, err := NewRoutedClient(f.ctx, network.LogicalOrigin(r.tb), f.receiverID, f.receiverID.Leaf, r.tb, r.b)
	if err != nil {
		return
	}
	if _, err = bad.Inventory(f.ctx, req); err == nil {
		t.Fatal("wrong certificate accepted")
	}
}
func TestWANW04ProductionRelayLossPreservesCapture(t *testing.T) {
	f := newSyncFixture(t)
	r := connectProductionRelay(t, f)
	want := []byte("saved locally while the relay is down")
	_ = r.service.Close()
	if err := os.WriteFile(filepath.Join(f.receiverRoot, "offline-relay.txt"), want, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.receiverWork.Scan(f.ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, time.Second)
	defer cancel()
	if _, err := f.newSyncer(TransferOptions{}).Sync(ctx); err == nil {
		t.Fatal("relay outage reported sync success")
	}
	heads, err := f.receiverRepo.Heads(f.ctx, f.folder, "offline-relay.txt")
	if err != nil || len(heads) != 1 || f.receiverRepo.VerifyManifest(heads[0].Manifest) != nil {
		t.Fatal("offline capture", err)
	}
}

func (r *productionRelay) reconnect(t *testing.T, f *syncFixture) {
	t.Helper()
	_ = r.ea.Close()
	_ = r.eb.Close()
	r.service = r.restart()
	for _, c := range []*network.ServiceClient{r.ca, r.cb} {
		if err := c.Announce(f.ctx, "peer_data", fmt.Sprintf("%064x", time.Now().UnixNano()), p.NetworkAnnouncement{Generation: 1, Expires: p.NetworkUint(time.Now().Unix() + 600), Candidates: []p.NetworkCandidate{}, Capabilities: []string{"relay_inner_tls_v1"}, Relay: true}); err != nil {
			t.Fatal(err)
		}
	}
	lA, _ := r.a.IncomingListener(network.PeerData)
	lB, _ := r.b.IncomingListener(network.PeerData)
	var err error
	r.ea, err = network.NewRelayEndpoint(f.ctx, r.ca, network.PeerData, 1, map[string]string{hex.EncodeToString(r.ta.Device[:]): hex.EncodeToString(r.ta.Pin[:])}, false, lA)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.ea.Close() })
	r.eb, err = network.NewRelayEndpoint(f.ctx, r.cb, network.PeerData, 1, map[string]string{hex.EncodeToString(r.tb.Device[:]): hex.EncodeToString(r.tb.Pin[:])}, false, lB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.eb.Close() })
	r.a.AdvanceGeneration()
	r.b.AdvanceGeneration()
	if err = r.a.SetRelay(r.ta, r.ea.Dial); err != nil {
		t.Fatal(err)
	}
	if err = r.b.SetRelay(r.tb, r.eb.Dial); err != nil {
		t.Fatal(err)
	}
	f.client, err = NewRoutedClient(f.ctx, network.LogicalOrigin(r.tb), f.receiverID, f.senderID.Leaf, r.tb, r.b)
	if err != nil {
		t.Fatal(err)
	}
}
func TestWANW04ProductionRelayRestartResumesVerifiedChunks(t *testing.T) {
	f := newSyncFixture(t)
	r := connectProductionRelay(t, f)
	content := append(bytes.Repeat([]byte("A"), int(history.ChunkSize)), bytes.Repeat([]byte("B"), int(history.ChunkSize))...)
	if err := os.WriteFile(filepath.Join(f.senderRoot, "relay-restart.bin"), content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.senderWork.Scan(f.ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	verified := 0
	hook := func(name string) error {
		if name == HookChunkVerified {
			verified++
			if verified == 1 {
				_ = r.service.Close()
				return errors.New("disposable relay restart after verified chunk")
			}
		}
		return nil
	}
	if _, err := f.newSyncer(TransferOptions{Workers: 1, Hook: hook}).Sync(f.ctx); err == nil {
		t.Fatal("interruption reported complete")
	}
	heads, err := f.receiverRepo.Heads(f.ctx, f.folder, "relay-restart.bin")
	if err != nil || len(heads) != 1 {
		t.Fatal(err)
	}
	ready, err := f.receiverRepo.ContentReady(f.ctx, heads[0].ID)
	if err != nil || ready {
		t.Fatal("partial content marked ready", err)
	}
	if _, err = os.Stat(filepath.Join(f.receiverRoot, "relay-restart.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial file published", err)
	}
	peers, err := f.senderRepo.PeerProgress(f.ctx, f.folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, peer := range peers {
		if peer.Receipt {
			t.Fatal("broker routing became receipt")
		}
	}
	r.reconnect(t, f)
	result, err := f.newSyncer(TransferOptions{Workers: 1}).Sync(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.ChunksReused != 1 || result.ChunksFetched != 1 || result.VersionsApplied != 1 {
		t.Fatalf("restart resume: %+v", result)
	}
	got, err := os.ReadFile(filepath.Join(f.receiverRoot, "relay-restart.bin"))
	if err != nil || !bytes.Equal(got, content) {
		t.Fatal("restart bytes", err)
	}
	local, _ := f.senderRepo.Heads(f.ctx, f.folder, "relay-restart.bin")
	remote, _ := f.receiverRepo.Heads(f.ctx, f.folder, "relay-restart.bin")
	a, _ := json.Marshal(local)
	b, _ := json.Marshal(remote)
	if !bytes.Equal(a, b) || f.receiverRepo.VerifyManifest(remote[0].Manifest) != nil {
		t.Fatal("restart head/hash/author changed")
	}
}
func TestWANW04ProductionRelayLostReceiptReplay(t *testing.T) {
	f := newSyncFixture(t)
	_ = connectProductionRelay(t, f)
	want := []byte("durable bytes before uncertain receipt through relay")
	if err := os.WriteFile(filepath.Join(f.senderRoot, "relay-receipt.txt"), want, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.senderWork.Scan(f.ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	hook := func(name string) error {
		if name == HookBeforeReceipt {
			return errors.New("disposable lost receipt response")
		}
		return nil
	}
	if _, err := f.newSyncer(TransferOptions{Hook: hook}).Sync(f.ctx); err == nil {
		t.Fatal("lost receipt not observed")
	}
	heads, err := f.receiverRepo.Heads(f.ctx, f.folder, "relay-receipt.txt")
	if err != nil || len(heads) != 1 {
		t.Fatal(err)
	}
	ready, err := f.receiverRepo.ContentReady(f.ctx, heads[0].ID)
	if err != nil || !ready {
		t.Fatal("verified local content lost", err)
	}
	peers, _ := f.senderRepo.PeerProgress(f.ctx, f.folder)
	for _, peer := range peers {
		if peer.Receipt {
			t.Fatal("allocation falsely supplied receipt")
		}
	}
	if _, err = f.newSyncer(TransferOptions{}).Sync(f.ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := f.receiverRepo.Heads(f.ctx, f.folder, "relay-receipt.txt")
	a, _ := json.Marshal(heads)
	b, _ := json.Marshal(after)
	if !bytes.Equal(a, b) {
		t.Fatal("receipt replay changed causal version")
	}
	peers, _ = f.senderRepo.PeerProgress(f.ctx, f.folder)
	found := false
	for _, peer := range peers {
		found = found || peer.Receipt
	}
	if !found {
		t.Fatal("end-device receipt missing after retry")
	}
}

func TestWANW04ProductionUnknownEnrollmentCannotReachDataOrControl(t *testing.T) {
	f := newSyncFixture(t)
	_, selection, origin, roots, _ := productionRelayService(t)
	unknown, err := LoadOrCreateIdentity(t.TempDir(), fixedID('U'), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	newClient := func(id Identity) *network.ServiceClient {
		c, err := network.NewServiceClient(network.ServiceClientOptions{Selection: selection, Origin: origin, Device: hex.EncodeToString(id.DeviceID[:]), Certificate: id.Certificate, Roots: roots})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	ca, cu := newClient(f.senderID), newClient(unknown)
	if err = ca.Announce(f.ctx, "enrollment", strings.Repeat("3", 64), p.NetworkAnnouncement{Generation: 1, Expires: p.NetworkUint(time.Now().Unix() + 600), Candidates: []p.NetworkCandidate{}, Capabilities: []string{"relay_inner_tls_v1"}, Relay: true}); err != nil {
		t.Fatal(err)
	}
	m := network.NewManager(network.ManagerOptions{})
	t.Cleanup(func() { _ = m.Close() })
	l, _ := m.IncomingListener(network.Enrollment)
	server := NewEnrollmentServer(f.senderRepo, f.senderID).HTTPServer()
	done := make(chan error, 1)
	go func() { done <- server.Serve(tls.NewListener(l, server.TLSConfig)) }()
	t.Cleanup(func() { _ = server.Close(); _ = l.Close(); <-done })
	incoming, err := network.NewRelayEndpoint(f.ctx, ca, network.Enrollment, 1, map[string]string{}, true, l)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = incoming.Close() })
	unused := network.NewStreamListener()
	defer unused.Close()
	requester, err := network.NewRelayEndpoint(f.ctx, cu, network.Enrollment, 1, map[string]string{hex.EncodeToString(f.senderID.DeviceID[:]): hex.EncodeToString(f.senderID.KeyPin[:])}, false, unused)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = requester.Close() })
	digest, _ := selection.Digest()
	raw, _ := hex.DecodeString(digest)
	var profile history.Digest
	copy(profile[:], raw)
	target := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.Enrollment, Profile: profile}
	if err = m.SetRelay(target, requester.Dial); err != nil {
		t.Fatal(err)
	}
	cfg, err := unknown.ClientTLSConfig(f.senderID.Leaf, f.senderID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Certificates = nil
	rt, err := m.Transport(f.ctx, target, cfg)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := network.HTTPClient(rt)
	for _, path := range []string{"/peer/v1/hello", "/api/v1/status"} {
		req, _ := http.NewRequestWithContext(f.ctx, "POST", network.LogicalOrigin(target)+path, strings.NewReader(`{}`))
		response, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != 404 {
			t.Fatal("enrollment reached other handler", path, response.StatusCode)
		}
	}
	token := strings.Repeat("1", 64)
	verifier, err := tokenVerifier(token)
	if err != nil {
		t.Fatal(err)
	}
	inv := tc.Invitation{Version: "1", Folder: hex.EncodeToString(f.folder[:]), Inviter: hex.EncodeToString(f.senderID.DeviceID[:]), KeyPin: hex.EncodeToString(f.senderID.KeyPin[:]), CertificateDER: base64.StdEncoding.EncodeToString(f.senderID.Leaf.Raw), Capability: token, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), EnrollmentEndpoint: network.LogicalOrigin(target), PeerEndpoint: "https://peer.orbit.invalid"}
	stored := inv
	stored.Capability = ""
	if err = f.senderRepo.EnrollmentTransaction(f.ctx, func(tx *repository.EnrollmentTx) error {
		return tx.Put("invite/"+verifier, EnrollmentInvitation{Invitation: stored, Digest: verifier})
	}); err != nil {
		t.Fatal(err)
	}
	client := &EnrollmentClient{invitation: inv, identity: unknown, http: httpClient}
	wire, err := client.Prepare(f.ctx, strings.Repeat("2", 64), "Synthetic unknown relay requester", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Submit(f.ctx, wire)
	if err != nil || result.State != "pending_approval" {
		t.Fatal("pending enrollment", result, err)
	}
	membership, _, err := f.senderRepo.GetMembership(f.ctx, f.folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range membership.Active {
		if member.Device == unknown.DeviceID {
			t.Fatal("relay enrollment granted membership")
		}
	}
	// This intentionally exercises the unchanged v2 handlers over the production
	// relay. Logical invitations/v3 durable setup remain the W05 deliverable.
}

func TestWANW04ProductionRelayRequestCancellationJoins(t *testing.T) {
	f := newSyncFixture(t)
	started, joined := make(chan struct{}), make(chan struct{})
	r := connectProductionRelay(t, f, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path != "/synthetic-cancel" {
				next.ServeHTTP(w, req)
				return
			}
			_, _ = io.Copy(io.Discard, req.Body)
			close(started)
			<-req.Context().Done()
			close(joined)
		})
	})
	trust, err := f.receiverID.ClientTLSConfig(f.senderID.Leaf, f.senderID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := r.b.Transport(f.ctx, r.tb, trust)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", network.LogicalOrigin(r.tb)+"/synthetic-cancel", strings.NewReader("synthetic request"))
	done := make(chan error, 1)
	go func() {
		response, err := network.HTTPClient(rt).Do(req)
		if response != nil {
			_ = response.Body.Close()
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("relayed handler did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled request succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request cancellation did not join")
	}
	select {
	case <-joined:
	case <-time.After(2 * time.Second):
		t.Fatal("remote HTTP handler survived canceled relay")
	}
	if err = r.b.Close(); err != nil {
		t.Fatal(err)
	}
}
