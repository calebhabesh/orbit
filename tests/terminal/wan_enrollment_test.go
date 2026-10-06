package terminal_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/rendezvous"
	"github.com/calebhabesh/file-sync/internal/replication"
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
)

func w05Service(t *testing.T, observe ...func()) (*rendezvous.Service, network.ProfileSelection, string, *x509.CertPool, func() *rendezvous.Service) {
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
	profile := protocol.NetworkProfile{Version: "1", Operator: "Development directory loss fixture", Authority: hex.EncodeToString(pub), ServiceKey: hex.EncodeToString(pub), Epoch: 1, Expires: protocol.NetworkUint(now.Unix() + 3600), Origins: []string{origin, "wss" + origin[5:]}, STUN: []string{}, Privacy: "Development only; metadata expires in memory; no logs or folder data."}
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
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(observe) > 0 {
			observe[0]()
		}
		live.Load().ServeHTTP(w, r)
	})
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

type w05Node struct {
	f         *fixture
	id        replication.Identity
	manager   *network.ConnectionManager
	runtime   *network.RelayRuntime
	client    *network.ServiceClient
	selection network.ProfileSelection
	origin    string
	roots     *x509.CertPool
	stop      func()
}

func newW05Node(t *testing.T, selection network.ProfileSelection, origin string, roots *x509.CertPool) *w05Node {
	t.Helper()
	f := fresh(t)
	id, err := replication.LoadOrCreateIdentity(f.state, f.device, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := selection.Digest()
	if err = config.SaveNetworkProfile(f.state, selection, uint64(time.Now().Unix())); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(tc.NetworkPolicy{Mode: "self_hosted", Profile: digest, Generation: 1})
	if err = config.WritePrivate(f.state, "network.json", b); err != nil {
		t.Fatal(err)
	}
	n := &w05Node{f: f, id: id, selection: selection, origin: origin, roots: roots}
	n.start(t, nil)
	t.Cleanup(func() { n.stop() })
	return n
}
func (n *w05Node) start(t *testing.T, hook control.FaultHook) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	n.manager = network.NewManager(network.ManagerOptions{})
	var err error
	n.client, err = network.NewServiceClient(network.ServiceClientOptions{Selection: n.selection, Origin: n.origin, Device: hex.EncodeToString(n.id.DeviceID[:]), Certificate: n.id.Certificate, Roots: n.roots})
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := n.selection.Digest()
	n.runtime, err = network.NewRelayRuntime(ctx, n.client, n.manager, 1, digest)
	if err != nil {
		t.Fatal(err)
	}
	routes, err := config.LoadPeerRoutes(n.f.state)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range routes {
		target := network.Target{Device: mustID(t, route.Device), Pin: history.Digest(mustID(t, route.Pin)), Profile: history.Digest(mustID(t, route.Profile)), Purpose: network.PeerData}
		if err = n.runtime.Register(target); err != nil {
			t.Fatal(err)
		}
	}
	n.f.ctrl = control.New(n.f.db, n.f.ws, control.Options{LocalDevice: n.f.device, Network: n.manager, Relay: n.runtime, FaultHook: hook, Now: time.Now})
	var servers []*http.Server
	var wg sync.WaitGroup
	for _, purpose := range []network.Purpose{network.Enrollment, network.PeerData} {
		listener, _ := n.manager.IncomingListener(purpose)
		server := replication.NewServer(n.f.db, n.id).HTTPServer()
		if purpose == network.Enrollment {
			server = replication.NewEnrollmentServer(n.f.db, n.id).HTTPServer()
		}
		servers = append(servers, server)
		wg.Go(func() { _ = server.Serve(tls.NewListener(listener, server.TLSConfig)) })
	}
	n.stop = func() {
		cancel()
		_ = n.runtime.Close()
		_ = n.client.Close()
		for _, s := range servers {
			_ = s.Close()
		}
		_ = n.manager.Close()
		wg.Wait()
	}
	readyCtx, end := context.WithTimeout(context.Background(), 15*time.Second)
	defer end()
	if err = n.runtime.WaitReady(readyCtx); err != nil {
		t.Fatal(err)
	}

}
func (n *w05Node) restart(t *testing.T, hook control.FaultHook) {
	t.Helper()
	before := n.id
	n.stop()
	n.f.close()
	n.f.open()
	id, err := replication.LoadOrCreateIdentity(n.f.state, n.f.device, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if before.DeviceID != id.DeviceID || before.KeyPin != id.KeyPin {
		t.Fatal("restart replaced identity")
	}
	n.id = id
	n.start(t, hook)
}
func w05Create(t *testing.T, n *w05Node, name string) history.ID {
	t.Helper()
	root := filepath.Join(n.f.root, name)
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner-file"), []byte("owner verified bytes "+name), 0600); err != nil {
		t.Fatal(err)
	}
	plan, _ := setupReview(t, n.f, root, "setup")
	r, err := n.f.ctrl.TerminalMutate(context.Background(), tc.Mutation{Version: "1", Kind: "setup", OperationID: enrollmentRandom(t), Setup: &plan})
	if err != nil || r.Error != nil {
		t.Fatalf("create: %v %+v", err, r.Error)
	}
	return mustID(t, r.Join.Folder)
}
func w05Invite(t *testing.T, n *w05Node, folder history.ID, target string) tc.Invitation {
	t.Helper()
	m, err := n.f.db.Membership(context.Background(), folder)
	if err != nil {
		t.Fatal(err)
	}
	kind := "invite"
	if target != "" {
		kind = "share"
	}
	r, err := n.f.ctrl.TerminalMutate(context.Background(), tc.Mutation{Version: "1", Kind: kind, OperationID: enrollmentRandom(t), Invite: &tc.InviteIntent{Folder: hex.EncodeToString(folder[:]), ExpectedMembership: hex.EncodeToString(m.Digest[:]), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), Device: target}})
	if err != nil || r.Invitation == nil {
		t.Fatalf("invite: %v", err)
	}
	if r.Invitation.Version != "3" || r.Invitation.EnrollmentEndpoint != "" || r.Invitation.PeerEndpoint != "" {
		t.Fatal("invitation has numeric endpoint")
	}
	return *r.Invitation
}
func w05Join(t *testing.T, n *w05Node, inv tc.Invitation, name string) tc.Mutation {
	t.Helper()
	root := filepath.Join(n.f.root, name)
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "joining-file"), []byte("requester protected bytes "+name), 0600); err != nil {
		t.Fatal(err)
	}
	plan, _ := setupReview(t, n.f, root, "join")
	return tc.Mutation{Version: "1", Kind: "join", OperationID: enrollmentRandom(t), Join: &tc.JoinIntent{Invitation: inv, Attempt: enrollmentRandom(t), DeviceName: plan.DeviceName, FolderName: plan.FolderName, Root: plan.Root, Preview: plan.Preview, Settings: plan.Settings}}
}
func w05Approve(t *testing.T, n *w05Node, request string) tc.Mutation {
	t.Helper()
	r, err := n.f.ctrl.TerminalQuery(context.Background(), tc.Query{Version: "1", Kind: "requests", Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range r.Requests {
		if q.ID == request {
			m := tc.Mutation{Version: "1", Kind: "approval", OperationID: enrollmentRandom(t), Approval: &tc.ApprovalIntent{Request: q.ID, Folder: q.Folder, Requester: q.Requester, KeyPin: q.KeyPin, TranscriptDigest: q.TranscriptDigest, ExpectedMembership: q.ExpectedMembership, Decision: "approve"}}
			if _, err = n.f.ctrl.TerminalMutate(context.Background(), m); err != nil {
				t.Fatal(err)
			}
			return m
		}
	}
	t.Fatal("request missing")
	return tc.Mutation{}
}
func w05Pull(t *testing.T, from, to *w05Node, folder history.ID) {
	t.Helper()
	routes, err := config.LoadPeerRoutes(to.f.state)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range routes {
		if route.Folder != hex.EncodeToString(folder[:]) || route.Device != hex.EncodeToString(from.id.DeviceID[:]) {
			continue
		}
		target := network.Target{Device: from.id.DeviceID, Pin: from.id.KeyPin, Profile: history.Digest(mustID(t, route.Profile)), Purpose: network.PeerData}
		client, err := replication.NewRoutedClient(context.Background(), network.LogicalOrigin(target), to.id, from.id.Leaf, target, to.manager)
		if err != nil {
			t.Fatal(err)
		}
		defer client.CloseIdleConnections()
		membership, err := to.f.db.Membership(context.Background(), folder)
		if err != nil {
			t.Fatal(err)
		}
		for attempt := 0; attempt < 3; attempt++ {
			_, err = replication.NewSyncer(to.f.db, to.f.ws, client, to.id.DeviceID, from.id.DeviceID, folder, membership, replication.TransferOptions{}).Sync(context.Background())
			if err == nil {
				break
			}
			if attempt == 2 {
				t.Fatal(err)
			}
			client.CloseIdleConnections()
			time.Sleep(11 * time.Second)
		}
		return
	}
	t.Fatal("durable reverse route missing")
}
func w05AssertFiles(t *testing.T, a, b *w05Node, folder history.ID, rootA, rootB string) {
	t.Helper()
	for _, name := range []string{"owner-file", "joining-file"} {
		x, e := os.ReadFile(filepath.Join(rootA, name))
		if e != nil {
			t.Fatal(e)
		}
		y, e := os.ReadFile(filepath.Join(rootB, name))
		if e != nil || !bytes.Equal(x, y) {
			t.Fatalf("byte mismatch %s: %v", name, e)
		}
		h1, e := a.f.db.Heads(context.Background(), folder, name)
		if e != nil {
			t.Fatal(e)
		}
		h2, e := b.f.db.Heads(context.Background(), folder, name)
		if e != nil {
			t.Fatal(e)
		}
		p, _ := json.Marshal(h1)
		q, _ := json.Marshal(h2)
		if !bytes.Equal(p, q) || len(h1) != 1 || a.f.db.VerifyManifest(h1[0].Manifest) != nil || b.f.db.VerifyManifest(h2[0].Manifest) != nil {
			t.Fatal("heads/hash oracle failed")
		}
	}
}
func TestWANW05RoutedJoinTwoFoldersRestartAndTwoWayData(t *testing.T) {
	_, selection, origin, roots, _ := w05Service(t)
	a, b := newW05Node(t, selection, origin, roots), newW05Node(t, selection, origin, roots)
	for i := 0; i < 2; i++ {
		folder := w05Create(t, a, fmt.Sprintf("source%d", i))
		target := ""
		if i == 1 {
			target = hex.EncodeToString(b.id.DeviceID[:])
		}
		inv := w05Invite(t, a, folder, target)
		m := w05Join(t, b, inv, fmt.Sprintf("join%d", i))
		r, err := w05MutateRetry(t, b, m)
		if err != nil || r.Error != nil || r.Operation.Phase != "awaiting_approval" {
			t.Fatalf("join: %v %+v", err, r.Error)
		}
		request := r.Join.Request
		safe, _ := json.Marshal(r)
		if strings.Contains(string(safe), inv.Capability) {
			t.Fatal("inspection leaked capability")
		}
		if _, err = b.f.db.Membership(context.Background(), folder); err == nil {
			t.Fatal("pending membership admitted")
		}
		// A real close/reopen restores the same request, root and identity. The
		// production enrollment server/control sessions are rebuilt, resetting the
		// process-local five-request burst rather than bypassing it.
		b.restart(t, nil)
		a.restart(t, nil)
		replay, err := b.f.ctrl.TerminalMutate(context.Background(), m)
		if err != nil || replay.Join.Request != request || replay.Join.Root != m.Join.Root {
			t.Fatal("resume replaced reviewed attempt")
		}
		approval := w05Approve(t, a, request)
		time.Sleep(26 * time.Second)
		r, err = w05MutateRetry(t, b, m)
		if err != nil || r.Error != nil || !r.Readiness.Ready() {
			t.Fatalf("bootstrap: %v %+v", err, r.Error)
		}
		w05Pull(t, b, a, folder)
		w05AssertFiles(t, a, b, folder, filepath.Join(a.f.root, fmt.Sprintf("source%d", i)), m.Join.Root)
		if _, err = a.f.ctrl.TerminalMutate(context.Background(), approval); err != nil {
			t.Fatal(err)
		}
		routes, err := config.LoadPeerRoutes(a.f.state)
		if err != nil || len(routes) != i+1 {
			t.Fatal("folder routes duplicated")
		}
		a.restart(t, nil)
		b.restart(t, nil)
		w05Pull(t, a, b, folder)
		w05Pull(t, b, a, folder)
	}
}

func w05MutateRetry(t *testing.T, n *w05Node, m tc.Mutation) (tc.Result, error) {
	t.Helper()
	var result tc.Result
	var err error
	for i := 0; i < 4; i++ {
		result, err = n.f.ctrl.TerminalMutate(context.Background(), m)
		if err != nil || result.Error == nil || !result.Error.Retryable {
			return result, err
		}
		if i < 3 {
			time.Sleep(26 * time.Second)
		}
	}
	return result, err
}

func TestWANW05ThirdDeviceOfflineRolloutForkAndRetiredBootstrap(t *testing.T) {
	_, selection, origin, roots, _ := w05Service(t)
	a, b := newW05Node(t, selection, origin, roots), newW05Node(t, selection, origin, roots)
	folder := w05Create(t, a, "source")
	inv := w05Invite(t, a, folder, "")
	m := w05Join(t, b, inv, "join")
	r, err := w05MutateRetry(t, b, m)
	if err != nil || r.Error != nil {
		t.Fatalf("first join: %v %+v", err, r.Error)
	}
	w05Approve(t, a, r.Join.Request)
	time.Sleep(26 * time.Second)
	r, err = w05MutateRetry(t, b, m)
	if err != nil || r.Error != nil {
		t.Fatalf("first bootstrap: %v %+v", err, r.Error)
	}
	a.stop()
	a.stop = func() {}
	c := newW05Node(t, selection, origin, roots)
	inv = w05Invite(t, b, folder, "")
	third := w05Join(t, c, inv, "third")
	if err = os.Rename(filepath.Join(third.Join.Root, "joining-file"), filepath.Join(third.Join.Root, "third-file")); err != nil {
		t.Fatal(err)
	}
	plan, _ := setupReview(t, c.f, third.Join.Root, "join")
	third.Join.Preview = plan.Preview
	r, err = w05MutateRetry(t, c, third)
	if err != nil || r.Error != nil {
		t.Fatalf("third join: %v %+v", err, r.Error)
	}
	w05Approve(t, b, r.Join.Request)
	time.Sleep(26 * time.Second)
	r, err = w05MutateRetry(t, c, third)
	if err != nil || r.Error != nil {
		t.Fatalf("third bootstrap: %v %+v", err, r.Error)
	}
	// B forwards A's unchanged authored version while A has no listeners/control.
	heads, err := c.f.db.Heads(context.Background(), folder, "owner-file")
	if err != nil || len(heads) != 1 || heads[0].ID.Author != a.id.DeviceID || c.f.db.VerifyManifest(heads[0].Manifest) != nil {
		t.Fatal("offline forwarding replaced author/history")
	}
	w05Pull(t, c, b, folder)
	a.start(t, nil)
	w05Pull(t, b, a, folder)
	membership, err := a.f.db.Membership(context.Background(), folder)
	if err != nil || membership.Revision != 3 {
		t.Fatal("offline member did not catch up canonical chain", err)
	}
	routes, err := config.LoadPeerRoutes(a.f.state)
	if err != nil {
		t.Fatal(err)
	}
	target := network.Target{Device: b.id.DeviceID, Pin: b.id.KeyPin, Profile: history.Digest(mustID(t, routes[0].Profile)), Purpose: network.PeerData}
	client, err := replication.NewRoutedClient(context.Background(), network.LogicalOrigin(target), a.id, b.id.Leaf, target, a.manager)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	if _, err = client.MembershipGet(context.Background(), replication.MembershipGetRequest{ProtocolVersion: "1", DeviceID: hex.EncodeToString(a.id.DeviceID[:]), FolderID: hex.EncodeToString(folder[:]), FromRevision: "2", ExpectedDigest: strings.Repeat("f", 64)}); err == nil || !strings.Contains(err.Error(), "MEMBERSHIP_FORK") {
		t.Fatal("fork was not distinguished from network error", err)
	}
	// Retirement preserves the original A-authored history. A new receiver imports
	// the exact snapshot through the existing canonical membership handler.
	if _, err = b.f.ctrl.RetireMemberExecute(context.Background(), control.RetireMemberRequest{Folder: folder, TargetDevice: a.id.DeviceID}); err != nil {
		t.Fatal(err)
	}
	a.stop()
	a.stop = func() {}
	d := newW05Node(t, selection, origin, roots)
	inv = w05Invite(t, b, folder, "")
	fourth := w05Join(t, d, inv, "fourth")
	if err = os.Rename(filepath.Join(fourth.Join.Root, "joining-file"), filepath.Join(fourth.Join.Root, "fourth-file")); err != nil {
		t.Fatal(err)
	}
	plan, _ = setupReview(t, d.f, fourth.Join.Root, "join")
	fourth.Join.Preview = plan.Preview
	r, err = w05MutateRetry(t, d, fourth)
	if err != nil || r.Error != nil {
		t.Fatalf("replacement join: %v %+v", err, r.Error)
	}
	w05Approve(t, b, r.Join.Request)
	time.Sleep(26 * time.Second)
	r, err = w05MutateRetry(t, d, fourth)
	if err != nil || r.Error != nil || !r.Readiness.Ready() {
		t.Fatalf("retired bootstrap: %v %+v", err, r.Error)
	}
	heads, err = d.f.db.Heads(context.Background(), folder, "owner-file")
	if err != nil || len(heads) != 1 || heads[0].ID.Author != a.id.DeviceID || d.f.db.VerifyManifest(heads[0].Manifest) != nil {
		t.Fatal("retired history lost or reauthored")
	}
	snapshots, err := d.f.db.ListRetirementSnapshots(context.Background(), folder, 5)
	if err != nil || len(snapshots) != 1 || snapshots[0].RetiredDevice != a.id.DeviceID {
		t.Fatal("canonical retirement snapshot missing", err)
	}
}
