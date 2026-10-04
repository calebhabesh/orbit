package terminal_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func enrollmentRandom(t *testing.T) string {
	t.Helper()
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[:])
}
func networkIP(t *testing.T) string {
	t.Helper()
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range addresses {
		ip, _, err := net.ParseCIDR(a.String())
		if err == nil && ip.To4() != nil && !ip.IsLoopback() {
			return ip.String()
		}
	}
	t.Fatal("fresh nonloopback test requires a local IPv4 interface")
	return ""
}
func mustID(t *testing.T, s string) history.ID {
	t.Helper()
	var id history.ID
	if err := id.UnmarshalText([]byte(s)); err != nil {
		t.Fatal(err)
	}
	return id
}

type enrollmentFixture struct {
	owner, joiner     *fixture
	ownerID, joinerID replication.Identity
	folder            history.ID
	client            *replication.EnrollmentClient
	inv               tc.Invitation
	mutation          tc.Mutation
	control           *controlclient.Client
	peerURL           string
}

func newEnrollmentFixture(t *testing.T) *enrollmentFixture {
	t.Helper()
	ctx := context.Background()
	f := &enrollmentFixture{owner: fresh(t), joiner: fresh(t)}
	var err error
	f.ownerID, err = replication.LoadOrCreateIdentity(f.owner.state, f.owner.device, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	f.joinerID, err = replication.LoadOrCreateIdentity(f.joiner.state, f.joiner.device, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	f.folder = mustID(t, enrollmentRandom(t))
	if err := f.owner.db.EnsureFolder(ctx, f.folder, f.owner.device, 1); err != nil {
		t.Fatal(err)
	}
	approved, err := f.owner.db.ApproveMembership(ctx, protocol.Membership{Folder: f.folder, Revision: 1, Active: []protocol.ActiveMember{{Device: f.owner.device, KeyPin: f.ownerID.KeyPin}}})
	if err != nil {
		t.Fatal(err)
	}
	ip := networkIP(t)
	peer, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		peer.Close()
		t.Fatal(err)
	}
	networkCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 2)
	go func() { done <- replication.NewServer(f.owner.db, f.ownerID).Serve(networkCtx, peer) }()
	go func() { done <- replication.NewEnrollmentServer(f.owner.db, f.ownerID).Serve(networkCtx, enrollment) }()
	t.Cleanup(func() {
		cancel()
		for i := 0; i < 2; i++ {
			if err := <-done; err != nil {
				t.Error(err)
			}
		}
	})
	f.peerURL = "https://" + peer.Addr().String()
	s := config.DefaultRuntimeSettings()
	s.PeerListen = peer.Addr().String()
	s.EnrollmentListen = enrollment.Addr().String()
	s.AdvertisedPeer = s.PeerListen
	s.AdvertisedEnrollment = s.EnrollmentListen
	if err := config.SaveRuntimeSettings(f.owner.state, s); err != nil {
		t.Fatal(err)
	}
	f.control = terminalClient(t, f.owner)
	f.mutation = tc.Mutation{Version: tc.Version, Kind: "invite", OperationID: enrollmentRandom(t), Invite: &tc.InviteIntent{Folder: hex.EncodeToString(f.folder[:]), ExpectedMembership: hex.EncodeToString(approved.Digest[:]), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}}
	result, err := f.control.Mutate(ctx, f.mutation)
	if err != nil {
		t.Fatal(err)
	}
	if result.Invitation == nil {
		t.Fatal("missing invitation")
	}
	f.inv = *result.Invitation
	f.client, err = replication.NewEnrollmentClient(f.inv, f.joinerID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.client.Close)
	return f
}
func prepareEnrollment(t *testing.T, f *enrollmentFixture) protocol.TerminalEnrollmentWire {
	t.Helper()
	w, err := f.client.Prepare(context.Background(), enrollmentRandom(t), "Joining laptop", "")
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func approveEnrollment(t *testing.T, f *enrollmentFixture, result protocol.TerminalEnrollmentResult, w protocol.TerminalEnrollmentWire) tc.Mutation {
	t.Helper()
	m := tc.Mutation{Version: tc.Version, Kind: "approval", OperationID: enrollmentRandom(t), Approval: &tc.ApprovalIntent{Request: result.Request, Folder: w.Folder, Requester: w.Requester, KeyPin: w.RequesterPin, TranscriptDigest: result.TranscriptDigest, ExpectedMembership: w.PriorMembership, Decision: "approve"}}
	r, err := f.control.Mutate(context.Background(), m)
	if err != nil || r.Operation.State != "completed" {
		t.Fatalf("reviewed approval failed: %v", err)
	}
	return m
}
func postEnrollment(t *testing.T, f *enrollmentFixture, path string, in, out any) int {
	t.Helper()
	cfg, err := f.joinerID.ClientTLSConfig(f.ownerID.Leaf, f.ownerID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	tr := &http.Transport{TLSClientConfig: cfg, Proxy: nil}
	defer tr.CloseIdleConnections()
	c := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Post(f.inv.EnrollmentEndpoint+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal("enrollment transport failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16385))
	if err != nil || len(data) > 16384 {
		t.Fatal("response bound exceeded")
	}
	if out != nil {
		if err := protocol.DecodeStrict(data, out); err != nil {
			t.Fatal("invalid bounded response")
		}
	}
	return resp.StatusCode
}

func TestTerminalT03NetworkEnrollmentApprovalAndReplay(t *testing.T) {
	f := newEnrollmentFixture(t)
	ctx := context.Background()
	w := prepareEnrollment(t, f)
	pending, err := f.client.Submit(ctx, w)
	if err != nil || pending.State != "pending_approval" {
		t.Fatalf("request failed: %v", err)
	}
	replay, err := f.client.Submit(ctx, w)
	if err != nil || replay != pending {
		t.Fatal("lost-response retry changed admission")
	}
	q, err := f.control.Query(ctx, tc.Query{Version: tc.Version, Kind: "requests", Folder: w.Folder})
	if err != nil || len(q.Requests) != 1 || q.Requests[0].VerificationCode != pending.VerificationCode || q.Requests[0].KeyPin != w.RequesterPin {
		t.Fatal("owner review bindings missing")
	}
	approval := approveEnrollment(t, f, pending, w)
	r, err := f.control.Mutate(ctx, approval)
	if err != nil || len(r.Operation.CommittedEffects) != 1 {
		t.Fatal("approval replay failed")
	}
	status, err := f.client.Status(ctx, pending.Request)
	if err != nil || status.State != "approved" {
		t.Fatalf("signed status failed: %v", err)
	}
	b, err := hex.DecodeString(status.MembershipHex)
	if err != nil {
		t.Fatal(err)
	}
	membership, err := protocol.DecodeMembership(b)
	if err != nil || membership.Revision != 2 || membership.PriorDigest != history.Digest(mustID(t, w.PriorMembership)) {
		t.Fatal("approved membership chain invalid")
	}
	if len(membership.Active) != 2 {
		t.Fatal("membership does not contain requester")
	}
	found := false
	for _, a := range membership.Active {
		if a.Device == f.joiner.device && a.KeyPin == f.joinerID.KeyPin {
			found = true
		}
	}
	if !found {
		t.Fatal("approved key differs")
	}
	// Durable records contain verifier only, including operation replay/query.
	err = f.owner.db.EnrollmentTransaction(ctx, func(tx *repository.EnrollmentTx) error {
		records, err := tx.Records("")
		if err != nil {
			return err
		}
		for _, data := range records {
			if bytes.Contains(data, []byte(f.inv.Capability)) {
				t.Fatal("raw capability persisted")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := f.control.Query(ctx, tc.Query{Version: tc.Version, Kind: "operation", ID: f.mutation.OperationID})
	if err != nil || inspected.Invitation.Capability != "" {
		t.Fatal("inspection disclosed capability")
	}
}
func TestTerminalT03InviterPinBeforeDisclosure(t *testing.T) {
	f := newEnrollmentFixture(t)
	var calls atomic.Int32
	impostor := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	impostor.TLS = &tls.Config{Certificates: []tls.Certificate{f.joinerID.Certificate}}
	impostor.StartTLS()
	defer impostor.Close()
	inv := f.inv
	inv.EnrollmentEndpoint = impostor.URL
	c, err := replication.NewEnrollmentClient(inv, f.joinerID)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Prepare(context.Background(), enrollmentRandom(t), "Device", ""); err == nil || calls.Load() != 0 {
		t.Fatal("inviter mismatch disclosed capability")
	}
	inv.KeyPin = hex.EncodeToString(f.joinerID.KeyPin[:])
	if _, err := replication.NewEnrollmentClient(inv, f.joinerID); err == nil {
		t.Fatal("certificate pin mismatch accepted")
	}
}
func TestTerminalT03ScopeAndProofRejections(t *testing.T) {
	for _, kind := range []string{"scope", "signature", "certificate", "inviter", "endpoint", "expired_nonce", "altered_label"} {
		t.Run(kind, func(t *testing.T) {
			f := newEnrollmentFixture(t)
			w := prepareEnrollment(t, f)
			switch kind {
			case "scope":
				w.Folder = enrollmentRandom(t)
			case "signature":
				w.Signature = strings.Repeat("0", 128)
			case "certificate":
				w.CertificateDER = f.inv.CertificateDER
			case "inviter":
				w.Inviter = enrollmentRandom(t)
			case "endpoint":
				w.PeerEndpoint = "https://192.0.2.1:443"
			case "expired_nonce":
				w.ExpiresUnix = "1"
			case "altered_label":
				w.Label = "altered"
			}
			if kind != "signature" && kind != "certificate" && kind != "altered_label" {
				transcript, err := w.Transcript()
				if err != nil {
					t.Fatal(err)
				}
				data, err := transcript.Canonical()
				if err != nil {
					t.Fatal(err)
				}
				w.Signature = hex.EncodeToString(ed25519.Sign(f.joinerID.Certificate.PrivateKey.(ed25519.PrivateKey), data))
			}
			if _, err := f.client.Submit(context.Background(), w); err == nil {
				t.Fatal("invalid proof admitted")
			}
			// Rejection never consumes the invitation.
			q := protocol.TerminalChallengeRequest{Version: "2", Folder: f.inv.Folder, Token: f.inv.Capability, Attempt: enrollmentRandom(t), Requester: hex.EncodeToString(f.joiner.device[:])}
			var result protocol.TerminalChallengeResult
			if postEnrollment(t, f, "/enrollment/v2/challenge", q, &result) != 200 {
				t.Fatal("invalid request consumed capability")
			}
		})
	}
}
func TestTerminalT03WrongScopeChallengeAndRevocation(t *testing.T) {
	f := newEnrollmentFixture(t)
	q := protocol.TerminalChallengeRequest{Version: "2", Folder: enrollmentRandom(t), Token: f.inv.Capability, Attempt: enrollmentRandom(t), Requester: hex.EncodeToString(f.joiner.device[:])}
	if postEnrollment(t, f, "/enrollment/v2/challenge", q, nil) == 200 {
		t.Fatal("wrong-scope challenge granted")
	}
	w := prepareEnrollment(t, f)
	token, _ := hex.DecodeString(f.inv.Capability)
	d := sha256.Sum256(token)
	if err := f.control.Call(context.Background(), "POST", "/api/v1/invitations/revoke", control.RevokeInvitationRequest{Digest: d}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.client.Submit(context.Background(), w); err == nil {
		t.Fatal("revoked invitation accepted")
	}
}
func TestTerminalT03ConcurrentSingleUse(t *testing.T) {
	f := newEnrollmentFixture(t)
	w := prepareEnrollment(t, f)
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.client.Submit(context.Background(), w)
			if err == nil && r.State == "pending_approval" {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 3 {
		t.Fatal("identical concurrent retries not idempotent")
	}
	token, _ := hex.DecodeString(f.inv.Capability)
	d := sha256.Sum256(token)
	err := f.owner.db.EnrollmentTransaction(context.Background(), func(tx *repository.EnrollmentTx) error {
		var inv replication.EnrollmentInvitation
		if err := tx.Get("invite/"+hex.EncodeToString(d[:]), &inv); err != nil {
			return err
		}
		if inv.Uses != 1 {
			t.Fatal("capability consumed more than once")
		}
		rs, err := tx.Records("request/")
		if len(rs) != 1 {
			t.Fatal("duplicate admission")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestTerminalT03StatusRequiresPossession(t *testing.T) {
	for _, kind := range []string{"wrong_key", "replay", "arbitrary_id"} {
		t.Run(kind, func(t *testing.T) {
			f := newEnrollmentFixture(t)
			w := prepareEnrollment(t, f)
			r, err := f.client.Submit(context.Background(), w)
			if err != nil {
				t.Fatal(err)
			}
			request := r.Request
			if kind == "arbitrary_id" {
				request = enrollmentRandom(t)
			}
			q := protocol.TerminalEnrollmentStatusRequest{Version: "2", Request: request, ExpiresUnix: "0"}
			var challenge protocol.TerminalChallengeResult
			if postEnrollment(t, f, "/enrollment/v2/status", q, &challenge) != 200 {
				t.Fatal("public nonce failed")
			}
			expires, _ := strconv.ParseUint(challenge.ExpiresUnix, 10, 64)
			req := mustID(t, request)
			nonce := mustID(t, challenge.Challenge)
			msg, _ := protocol.TerminalStatusTranscript([32]byte(req), [32]byte(nonce), expires)
			key := f.joinerID.Certificate.PrivateKey.(ed25519.PrivateKey)
			if kind == "wrong_key" {
				key = f.ownerID.Certificate.PrivateKey.(ed25519.PrivateKey)
			}
			q.Nonce = challenge.Challenge
			q.ExpiresUnix = challenge.ExpiresUnix
			q.Signature = hex.EncodeToString(ed25519.Sign(key, msg))
			status := postEnrollment(t, f, "/enrollment/v2/status", q, nil)
			if kind != "replay" {
				if status == 200 {
					t.Fatal("status artifact disclosed without possession")
				}
			} else {
				if status != 200 {
					t.Fatal("valid proof failed")
				}
				if postEnrollment(t, f, "/enrollment/v2/status", q, nil) == 200 {
					t.Fatal("status nonce replayed")
				}
			}
		})
	}
}
func TestTerminalT03IsolationAndDataAuthorization(t *testing.T) {
	f := newEnrollmentFixture(t)
	for _, path := range []string{"/peer/v1/inventory", "/api/v1/settings", "/control/terminal/v1/query"} {
		if postEnrollment(t, f, path, struct{}{}, nil) != 404 {
			t.Fatal("enrollment mounts unrelated route")
		}
	}
	client, err := replication.NewClient(f.peerURL, f.joinerID, f.ownerID.Leaf, f.ownerID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	_, err = client.Hello(context.Background(), replication.HelloRequest{ProtocolVersion: "1", DeviceID: hex.EncodeToString(f.joiner.device[:]), Folders: []replication.FolderHandshake{{FolderID: f.inv.Folder, Revision: "1", MembershipDigest: f.mutation.Invite.ExpectedMembership}}, Limits: replication.Limits{MetadataBytes: "8388608", InventoryPage: "128"}})
	if err == nil {
		t.Fatal("pending requester authorized data")
	}
	cfg, err := f.joinerID.ClientTLSConfig(f.ownerID.Leaf, f.ownerID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Certificates = nil
	tr := &http.Transport{TLSClientConfig: cfg}
	defer tr.CloseIdleConnections()
	c := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	resp, err := c.Post(f.peerURL+"/peer/v1/hello", "application/json", strings.NewReader("{}"))
	if err == nil {
		resp.Body.Close()
		t.Fatal("peer listener admitted certificate-free client")
	}
	if status := api(t, f.owner.server.URL, "", "POST", "/control/terminal/v1/mutate", f.mutation, nil); status != 401 {
		t.Fatal("owner control admitted network capability")
	}
}
func TestTerminalT03BoundsAndThrottle(t *testing.T) {
	f := newEnrollmentFixture(t)
	if postEnrollment(t, f, "/enrollment/v2/request", strings.Repeat("x", 16385), nil) != 413 {
		t.Fatal("oversized request admitted")
	}
	for i := 0; i < 4; i++ {
		postEnrollment(t, f, "/enrollment/v2/request", struct{}{}, nil)
	}
	if postEnrollment(t, f, "/enrollment/v2/request", struct{}{}, nil) != 429 {
		t.Fatal("per-IP limit not enforced")
	}
}
func TestTerminalT03ReviewedApprovalStaleAndReplay(t *testing.T) {
	f := newEnrollmentFixture(t)
	w := prepareEnrollment(t, f)
	r, err := f.client.Submit(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	p := tc.ApprovalIntent{Request: r.Request, Folder: w.Folder, Requester: w.Requester, KeyPin: w.RequesterPin, TranscriptDigest: r.TranscriptDigest, ExpectedMembership: w.PriorMembership, Decision: "approve"}
	bad := p
	bad.KeyPin = enrollmentRandom(t)
	if _, err := f.control.Mutate(context.Background(), tc.Mutation{Version: "1", Kind: "approval", OperationID: enrollmentRandom(t), Approval: &bad}); err == nil {
		t.Fatal("unreviewed key approved")
	}
	m := approveEnrollment(t, f, r, w)
	p.Decision = "decline"
	m.Approval = &p
	if _, err := f.control.Mutate(context.Background(), m); err == nil || !strings.Contains(err.Error(), "IDEMPOTENCY_CONFLICT") {
		t.Fatal("changed approval retry accepted")
	}
	replay, err := f.control.Mutate(context.Background(), f.mutation)
	if err != nil || replay.Invitation.Capability != f.inv.Capability {
		t.Fatal("invitation replay changed capability")
	}
}

func TestTerminalT03DaemonProcess(t *testing.T) {
	dir := os.Getenv("ORBIT_T03_TEST_STATE")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	if err := testkit.ValidateDestructiveTarget(filepath.Dir(dir), dir); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	workerDone := make(chan error, 1)
	go func() { workerDone <- enrollmentProcessWorker(ctx, dir) }()
	defer func() {
		cancel()
		if err := <-workerDone; err != nil {
			t.Error(err)
		}
	}()

	if err := app.ServeWithOptions(ctx, dir, app.ServeOptions{ControlAddress: "127.0.0.1:0", NoWatch: true}); err != nil {
		t.Fatal(err)
	}
}

// The receiving daemon's process signs and transmits its own request/status.
func enrollmentProcessWorker(ctx context.Context, dir string) error {
	waitFile := func(name string) ([]byte, error) {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err == nil {
				return b, nil
			}
			if !os.IsNotExist(err) {
				return nil, err
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-ticker.C:
			}
		}
	}
	data, err := waitFile("t03.invitation.json")
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	var inv tc.Invitation
	if err := tc.Decode(data, &inv); err != nil {
		return err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	var device history.ID
	if err := device.UnmarshalText([]byte(cfg.DeviceID)); err != nil {
		return err
	}
	id, err := replication.LoadOrCreateIdentity(dir, device, time.Now())
	if err != nil {
		return err
	}
	client, err := replication.NewEnrollmentClient(inv, id)
	if err != nil {
		return err
	}
	defer client.Close()
	var attempt [32]byte
	if _, err := rand.Read(attempt[:]); err != nil {
		return err
	}
	wire, err := client.Prepare(ctx, hex.EncodeToString(attempt[:]), "Second process", "")
	if err != nil {
		return err
	}
	b, err := json.Marshal(wire)
	if err != nil {
		return err
	}
	if err := config.WritePrivate(dir, "t03.wire.json", b); err != nil {
		return err
	}
	pending, err := client.Submit(ctx, wire)
	if err != nil {
		return err
	}
	b, err = json.Marshal(pending)
	if err != nil {
		return err
	}
	if err := config.WritePrivate(dir, "t03.pending.json", b); err != nil {
		return err
	}
	if _, err := waitFile("t03.approved"); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	status, err := client.Status(ctx, pending.Request)
	if err != nil {
		return err
	}
	b, err = json.Marshal(status)
	if err != nil {
		return err
	}
	return config.WritePrivate(dir, "t03.status.json", b)
}
func processFile(t *testing.T, dir, name string) []byte {
	t.Helper()
	until := time.Now().Add(10 * time.Second)
	for time.Now().Before(until) {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			return b
		}
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("process did not publish private test record")
	return nil
}
func startEnrollmentProcess(t *testing.T, dir string) *controlclient.Client {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestTerminalT03DaemonProcess$")
	cmd.Env = append(os.Environ(), "ORBIT_T03_TEST_STATE="+dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Error(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Error("test daemon failed")
			}
		case <-time.After(10 * time.Second):
			t.Error("test daemon did not stop")
		}
	})
	c := &controlclient.Client{StateDir: dir}
	until := time.Now().Add(10 * time.Second)
	for time.Now().Before(until) {
		var ready tc.Result
		if err := c.Call(context.Background(), "POST", "/control/terminal/v1/query", tc.Query{Version: "1", Kind: "capabilities"}, &ready); err == nil {
			return c
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("test daemon not ready")
	return nil
}
func TestTerminalT03TwoProcessEnrollment(t *testing.T) {
	ctx := context.Background()
	ip := networkIP(t)
	dirs := []string{filepath.Join(testkit.NewDisposable(t), "state"), filepath.Join(testkit.NewDisposable(t), "state")}
	ids := make([]replication.Identity, 2)
	var folder history.ID
	rand.Read(folder[:])
	var expected history.Digest
	for index, dir := range dirs {
		cfg, err := app.Initialize(ctx, dir, app.SystemDependencies())
		if err != nil {
			t.Fatal(err)
		}
		dev := mustID(t, cfg.DeviceID)
		ids[index], err = replication.LoadOrCreateIdentity(dir, dev, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		s := config.DefaultRuntimeSettings()
		addresses := make([]string, 2)
		listeners := make([]net.Listener, 2)
		for i := range addresses {
			l, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
			if err != nil {
				t.Fatal(err)
			}
			addresses[i] = l.Addr().String()
			listeners[i] = l
		}
		for _, l := range listeners {
			l.Close()
		}
		s.PeerListen = addresses[0]
		s.EnrollmentListen = addresses[1]
		s.AdvertisedPeer = s.PeerListen
		s.AdvertisedEnrollment = s.EnrollmentListen
		if err := config.SaveRuntimeSettings(dir, s); err != nil {
			t.Fatal(err)
		}
		err = app.WithWorkspace(ctx, dir, func(_ config.Config, db *repository.DB, _ *workspace.Workspace) error {
			if err := db.EnsureFolder(ctx, folder, dev, 1); err != nil {
				return err
			}
			if index == 0 {
				a, err := db.ApproveMembership(ctx, protocol.Membership{Folder: folder, Revision: 1, Active: []protocol.ActiveMember{{Device: dev, KeyPin: ids[index].KeyPin}}})
				expected = a.Digest
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	owner := startEnrollmentProcess(t, dirs[0])
	_ = startEnrollmentProcess(t, dirs[1])
	inviteMutation := tc.Mutation{Version: "1", OperationID: enrollmentRandom(t), Kind: "invite", Invite: &tc.InviteIntent{Folder: hex.EncodeToString(folder[:]), ExpectedMembership: hex.EncodeToString(expected[:]), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}}
	r, err := owner.Mutate(ctx, inviteMutation)
	if err != nil {
		t.Fatal(err)
	}

	invBytes, err := json.Marshal(r.Invitation)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.WritePrivate(dirs[1], "t03.invitation.json", invBytes); err != nil {
		t.Fatal(err)
	}
	var pending protocol.TerminalEnrollmentResult
	if err := protocol.DecodeStrict(processFile(t, dirs[1], "t03.pending.json"), &pending); err != nil {
		t.Fatal(err)
	}
	var w protocol.TerminalEnrollmentWire
	if err := protocol.DecodeStrict(processFile(t, dirs[1], "t03.wire.json"), &w); err != nil {
		t.Fatal(err)
	}
	m := tc.Mutation{Version: "1", Kind: "approval", OperationID: enrollmentRandom(t), Approval: &tc.ApprovalIntent{Request: pending.Request, Folder: w.Folder, Requester: w.Requester, KeyPin: w.RequesterPin, TranscriptDigest: pending.TranscriptDigest, ExpectedMembership: w.PriorMembership, Decision: "approve"}}
	if _, err := owner.Mutate(ctx, m); err != nil {
		t.Fatal(err)
	}

	if err := config.WritePrivate(dirs[1], "t03.approved", []byte("approved")); err != nil {
		t.Fatal(err)
	}
	var status protocol.TerminalEnrollmentResult
	if err := protocol.DecodeStrict(processFile(t, dirs[1], "t03.status.json"), &status); err != nil {
		t.Fatal(err)
	}
	if status.State != "approved" || status.MembershipHex == "" {
		t.Fatal("two-process approval artifact exchange failed")
	}
	canonical, err := hex.DecodeString(status.MembershipHex)
	if err != nil {
		t.Fatal(err)
	}
	membership, err := protocol.DecodeMembership(canonical)
	if err != nil {
		t.Fatal(err)
	}
	receiver := &controlclient.Client{StateDir: dirs[1]}
	err = receiver.Call(ctx, "POST", "/api/v1/membership/import", struct {
		Folder     history.ID          `json:"folder"`
		Membership protocol.Membership `json:"membership"`
		Approve    bool                `json:"approve"`
	}{folder, membership, true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := replication.NewClient(status.PeerEndpoint, ids[1], ids[0].Leaf, ids[0].KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.CloseIdleConnections()
	digest, err := protocol.MembershipDigest(membership)
	if err != nil {
		t.Fatal(err)
	}
	_, err = peer.Hello(ctx, replication.HelloRequest{ProtocolVersion: "1", DeviceID: hex.EncodeToString(ids[1].DeviceID[:]), Folders: []replication.FolderHandshake{{FolderID: hex.EncodeToString(folder[:]), Revision: "2", MembershipDigest: hex.EncodeToString(digest[:])}}, Limits: replication.Limits{MetadataBytes: "8388608", InventoryPage: "128"}})
	if err != nil {
		t.Fatal("approved peer membership handshake failed")
	}
	// Stop/reopen the real owner database; committed invitation/approval effects
	// and possession-based artifact retrieval survive the process boundary.
	if err := app.StopAgent(dirs[0], 5*time.Second); err != nil {
		t.Fatal(err)
	}
	owner = startEnrollmentProcess(t, dirs[0])
	replay, err := owner.Mutate(ctx, inviteMutation)
	if err != nil || replay.Invitation.Capability != r.Invitation.Capability {
		t.Fatal("restart changed invitation replay")
	}
	if _, err := owner.Mutate(ctx, m); err != nil {
		t.Fatal("restart lost approval replay")
	}
	inspector, err := replication.NewEnrollmentClient(*r.Invitation, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	defer inspector.Close()
	retained, err := inspector.Status(ctx, pending.Request)
	if err != nil || retained.MembershipHex != status.MembershipHex {
		t.Fatal("restart lost authenticated approval artifact")
	}
}

func TestTerminalT03ExpiredAndRevokedPending(t *testing.T) {
	for _, kind := range []string{"expired_invite", "expired_pending", "revoked_pending", "changed_membership"} {
		t.Run(kind, func(t *testing.T) {
			f := newEnrollmentFixture(t)
			ctx := context.Background()
			if kind == "expired_invite" {
				raw, _ := hex.DecodeString(f.inv.Capability)
				digest := sha256.Sum256(raw)
				err := f.owner.db.EnrollmentTransaction(ctx, func(tx *repository.EnrollmentTx) error {
					var inv replication.EnrollmentInvitation
					key := "invite/" + hex.EncodeToString(digest[:])
					if err := tx.Get(key, &inv); err != nil {
						return err
					}
					inv.Invitation.ExpiresAt = time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
					return tx.Put(key, inv)
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.client.Prepare(ctx, enrollmentRandom(t), "Device", ""); err == nil {
					t.Fatal("expired invitation granted challenge")
				}
				return
			}
			w := prepareEnrollment(t, f)
			r, err := f.client.Submit(ctx, w)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "expired_pending":
				err = f.owner.db.EnrollmentTransaction(ctx, func(tx *repository.EnrollmentTx) error {
					var record replication.EnrollmentRecord
					key := "request/" + r.Request
					if err := tx.Get(key, &record); err != nil {
						return err
					}
					record.Expires = time.Now().Unix() - 1
					return tx.Put(key, record)
				})
			case "revoked_pending":
				raw, _ := hex.DecodeString(f.inv.Capability)
				digest := sha256.Sum256(raw)
				err = f.control.Call(ctx, "POST", "/api/v1/invitations/revoke", control.RevokeInvitationRequest{Digest: digest}, nil)
			case "changed_membership":
				m, a, e := f.owner.db.GetMembership(ctx, f.folder)
				if e != nil {
					t.Fatal(e)
				}
				m.PriorDigest = a.Digest
				m.Revision++
				_, err = f.owner.db.ApproveMembership(ctx, m)
			}
			if err != nil {
				t.Fatal(err)
			}
			m := tc.Mutation{Version: "1", Kind: "approval", OperationID: enrollmentRandom(t), Approval: &tc.ApprovalIntent{Request: r.Request, Folder: w.Folder, Requester: w.Requester, KeyPin: w.RequesterPin, TranscriptDigest: r.TranscriptDigest, ExpectedMembership: w.PriorMembership, Decision: "approve"}}
			if _, err := f.control.Mutate(ctx, m); err == nil {
				t.Fatal("invalidated approval accepted")
			}
			current, _, err := f.owner.db.GetMembership(ctx, f.folder)
			if err != nil || len(current.Active) != 1 {
				t.Fatal("rejected approval changed membership")
			}
		})
	}
}
func TestTerminalT03AdmissionCapsAndChangedReplay(t *testing.T) {
	for _, kind := range []string{"nonces", "pending", "changed_retry"} {
		t.Run(kind, func(t *testing.T) {
			f := newEnrollmentFixture(t)
			ctx := context.Background()
			w := prepareEnrollment(t, f)
			if kind == "changed_retry" {
				if _, err := f.client.Submit(ctx, w); err != nil {
					t.Fatal(err)
				}
				original := w
				w.Label = "new label"
				transcript, err := w.Transcript()
				if err != nil {
					t.Fatal(err)
				}
				msg, _ := transcript.Canonical()
				w.Signature = hex.EncodeToString(ed25519.Sign(f.joinerID.Certificate.PrivateKey.(ed25519.PrivateKey), msg))
				if _, err := f.client.Submit(ctx, w); err == nil || !strings.Contains(err.Error(), "IDEMPOTENCY_CONFLICT") {
					t.Fatal("changed retry accepted")
				}
				alternate := original
				alternate.CertificateDER += "\n"
				if _, err := f.client.Submit(ctx, alternate); err == nil || !strings.Contains(err.Error(), "IDEMPOTENCY_CONFLICT") {
					t.Fatal("changed certificate encoding replay accepted")
				}
				return
			}
			err := f.owner.db.EnrollmentTransaction(ctx, func(tx *repository.EnrollmentTx) error {
				for i := 0; i < 128; i++ {
					if kind == "nonces" {
						err := tx.Put("status/"+enrollmentRandom(t), map[string]any{"request": protocol.TerminalChallengeRequest{}, "result": protocol.TerminalChallengeResult{Version: "2", ExpiresUnix: strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)}})
						if err != nil {
							return err
						}
					} else {
						err := tx.Put("request/"+enrollmentRandom(t), replication.EnrollmentRecord{Expires: time.Now().Add(time.Hour).Unix(), Result: protocol.TerminalEnrollmentResult{State: "pending_approval"}})
						if err != nil {
							return err
						}
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "nonces" {
				if postEnrollment(t, f, "/enrollment/v2/status", protocol.TerminalEnrollmentStatusRequest{Version: "2", Request: enrollmentRandom(t), ExpiresUnix: "0"}, nil) != 429 {
					t.Fatal("outstanding nonce limit exceeded")
				}
			} else {
				if _, err := f.client.Submit(ctx, w); err == nil || !strings.Contains(err.Error(), "RATE_LIMITED") {
					t.Fatal("pending admission cap exceeded")
				}
			}
			raw, _ := hex.DecodeString(f.inv.Capability)
			digest := sha256.Sum256(raw)
			err = f.owner.db.EnrollmentTransaction(ctx, func(tx *repository.EnrollmentTx) error {
				var inv replication.EnrollmentInvitation
				if err := tx.Get("invite/"+hex.EncodeToString(digest[:]), &inv); err != nil {
					return err
				}
				if inv.Uses != 0 {
					t.Fatal("admission cap consumed capability")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestTerminalT03LegacyBoundaries(t *testing.T) {
	f := newEnrollmentFixture(t)
	for _, path := range []string{"/api/v1/enrollment/request", "/api/v1/enrollment/status?request_id=" + enrollmentRandom(t)} {
		method := "POST"
		if strings.Contains(path, "status") {
			method = "GET"
		}
		if status := api(t, f.owner.server.URL, "", method, path, struct{}{}, nil); status != 401 {
			t.Fatal("legacy enrollment bypasses owner authentication")
		}
	}
	var disclosures atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { disclosures.Add(1) }))
	defer server.Close()
	_, err := f.joiner.ctrl.SubmitJoinFlow(context.Background(), control.JoinFlowSubmitRequest{InvitationToken: f.inv.Capability, TargetFolder: f.inv.Folder, RemoteEndpoint: server.URL, RootPath: filepath.Join(f.joiner.root, "data")})
	if err == nil || disclosures.Load() != 0 {
		t.Fatal("unpinned legacy helper disclosed capability")
	}
}
