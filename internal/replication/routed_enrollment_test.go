package replication

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/network"
	p "github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/repository"
)

type routedAdmissionFixture struct {
	f         *syncFixture
	s         *EnrollmentServer
	inv       tc.Invitation
	requester Identity
	selection network.ProfileSelection
}

func routedAdmission(t *testing.T) *routedAdmissionFixture {
	t.Helper()
	f := newSyncFixture(t)
	_, selection, _, _, _ := productionRelayService(t)
	id, err := LoadOrCreateIdentity(t.TempDir(), fixedID('U'), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	profile, _ := selection.Digest()
	token := strings.Repeat("8", 64)
	inv := tc.Invitation{Version: "3", Folder: hex.EncodeToString(f.folder[:]), Inviter: hex.EncodeToString(f.senderID.DeviceID[:]), KeyPin: hex.EncodeToString(f.senderID.KeyPin[:]), CertificateDER: base64.StdEncoding.EncodeToString(f.senderID.Leaf.Raw), Capability: token, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), Route: &p.EnrollmentRoute{Device: hex.EncodeToString(f.senderID.DeviceID[:]), Pin: hex.EncodeToString(f.senderID.KeyPin[:]), Profile: profile, Purpose: "enrollment"}, Profile: &selection.Profile}
	verifier, _ := tokenVerifier(token)
	if err = f.senderRepo.EnrollmentTransaction(f.ctx, func(tx *repository.EnrollmentTx) error {
		safe := inv
		safe.Capability = ""
		return tx.Put("invite/"+verifier, EnrollmentInvitation{Invitation: safe, Digest: verifier})
	}); err != nil {
		t.Fatal(err)
	}
	return &routedAdmissionFixture{f, NewEnrollmentServer(f.senderRepo, f.senderID), inv, id, selection}
}
func (f *routedAdmissionFixture) prepare(t *testing.T) p.RoutedEnrollmentRequest {
	t.Helper()
	requester := p.EnrollmentRoute{Device: hex.EncodeToString(f.requester.DeviceID[:]), Pin: hex.EncodeToString(f.requester.KeyPin[:]), Profile: f.inv.Route.Profile, Purpose: "enrollment"}
	attempt, _ := enrollmentRandom()
	ch, err := f.s.routedChallenge(f.f.ctx, p.RoutedChallengeRequest{Version: "3", Folder: f.inv.Folder, Inviter: *f.inv.Route, Requester: requester, Capability: f.inv.Capability, Attempt: attempt})
	if err != nil {
		t.Fatal(err)
	}
	key := f.requester.Certificate.PrivateKey.(ed25519.PrivateKey)
	w := p.RoutedEnrollmentRequest{Transcript: p.RoutedEnrollmentTranscript{Version: "3", Folder: f.inv.Folder, Inviter: *f.inv.Route, Requester: requester, CapabilityDigest: mustTokenVerifier(f.inv.Capability), Attempt: attempt, Challenge: ch.Challenge, PublicKey: hex.EncodeToString(key.Public().(ed25519.PublicKey)), PriorMembership: ch.PriorMembership, Expires: ch.Expires, Label: "reviewed requester"}, Capability: f.inv.Capability, CertificateDER: base64.StdEncoding.EncodeToString(f.requester.Leaf.Raw)}
	signRouted(t, &w, key)
	return w
}
func signRouted(t *testing.T, w *p.RoutedEnrollmentRequest, key ed25519.PrivateKey) {
	t.Helper()
	b, err := w.Transcript.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	w.Signature = hex.EncodeToString(ed25519.Sign(key, b))
}
func TestWANW05RoutedAdmissionExactReplayRevocationAndExpiry(t *testing.T) {
	f := routedAdmission(t)
	w := f.prepare(t)
	first, err := f.s.routedSubmit(f.f.ctx, w)
	if err != nil || first.State != "pending_approval" {
		t.Fatal(err)
	}
	// Lost responses may replay the exact signed request after its minute-long
	// challenge expires, but never after the accepted invitation's own lifetime.
	base := f.s.now()
	f.s.now = func() time.Time { return base.Add(2 * time.Minute) }
	again, err := f.s.routedSubmit(f.f.ctx, w)
	if err != nil || again.Request != first.Request {
		t.Fatal("accepted replay lost", err)
	}
	changed := w
	changed.Transcript.Label = "changed review"
	signRouted(t, &changed, f.requester.Certificate.PrivateKey.(ed25519.PrivateKey))
	if _, err = f.s.routedSubmit(f.f.ctx, changed); err == nil || err.Error() != "IDEMPOTENCY_CONFLICT" {
		t.Fatal("changed replay admitted", err)
	}
	if err = f.f.senderRepo.EnrollmentTransaction(f.f.ctx, func(tx *repository.EnrollmentTx) error {
		var inv EnrollmentInvitation
		v := mustTokenVerifier(f.inv.Capability)
		if e := tx.Get("invite/"+v, &inv); e != nil {
			return e
		}
		inv.Revoked = true
		return tx.Put("invite/"+v, inv)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.routedSubmit(f.f.ctx, w); err == nil || err.Error() != "INVITATION_INVALID" {
		t.Fatal("revoked replay admitted", err)
	}
	f.s.now = func() time.Time { return base.Add(2 * time.Hour) }
	if _, err = f.s.routedSubmit(f.f.ctx, w); err == nil || err.Error() != "EXPIRED_REPLAY" {
		t.Fatal("expired replay admitted", err)
	}
}
func TestWANW05RoutedWrongFolderRouteSignatureAndV2Downgrade(t *testing.T) {
	for _, kind := range []string{"folder", "inviter", "requester", "profile", "signature", "v2"} {
		t.Run(kind, func(t *testing.T) {
			f := routedAdmission(t)
			w := f.prepare(t)
			key := f.requester.Certificate.PrivateKey.(ed25519.PrivateKey)
			switch kind {
			case "folder":
				w.Transcript.Folder = strings.Repeat("7", 64)
			case "inviter":
				w.Transcript.Inviter.Device = strings.Repeat("7", 64)
			case "requester":
				w.Transcript.Requester.Device = strings.Repeat("7", 64)
			case "profile":
				w.Transcript.Inviter.Profile = strings.Repeat("7", 64)
				w.Transcript.Requester.Profile = w.Transcript.Inviter.Profile
			case "signature":
				w.Signature = strings.Repeat("0", 128)
			case "v2":
				if _, err := f.s.challenge(f.f.ctx, p.TerminalChallengeRequest{Version: "2", Folder: f.inv.Folder, Token: f.inv.Capability, Attempt: w.Transcript.Attempt, Requester: w.Transcript.Requester.Device}); err == nil {
					t.Fatal("v3 capability downgraded")
				}
				return
			}
			if kind != "signature" {
				signRouted(t, &w, key)
			}
			if _, err := f.s.routedSubmit(f.f.ctx, w); err == nil {
				t.Fatal("mutated signed request admitted")
			}
		})
	}
}
func TestWANW05RoutedStatusPossessionNonceAndDigest(t *testing.T) {
	f := routedAdmission(t)
	w := f.prepare(t)
	out, err := f.s.routedSubmit(f.f.ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	q := p.RoutedEnrollmentStatus{Version: "3", Request: out.Request, TranscriptDigest: out.TranscriptDigest, Requester: w.Transcript.Requester}
	chAny, err := f.s.routedStatus(f.f.ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	ch := chAny.(p.RoutedChallengeResult)
	q.Nonce = ch.Challenge
	q.Expires = ch.Expires
	b, _ := q.Canonical()
	q.Signature = hex.EncodeToString(ed25519.Sign(f.requester.Certificate.PrivateKey.(ed25519.PrivateKey), b))
	wrong := q
	wrong.TranscriptDigest = strings.Repeat("5", 64)
	if _, err = f.s.routedStatus(f.f.ctx, wrong); err == nil {
		t.Fatal("status digest ignored")
	}
	wrong = q
	wrong.Signature = strings.Repeat("0", 128)
	if _, err = f.s.routedStatus(f.f.ctx, wrong); err == nil {
		t.Fatal("status without possession")
	}
	if _, err = f.s.routedStatus(f.f.ctx, q); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.routedStatus(f.f.ctx, q); err == nil {
		t.Fatal("nonce replay admitted")
	}
}
func TestWANW05RoutedWrongInviterTLSBeforeDisclosure(t *testing.T) {
	f := routedAdmission(t)
	var hits atomic.Int64
	server := f.s.HTTPServer()
	handler := server.Handler
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); handler.ServeHTTP(w, r) })
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(tls.NewListener(l, server.TLSConfig)) }()
	t.Cleanup(func() { _ = server.Close(); <-done })
	// Deliberately transfer a different valid certificate/pin. The routing fixture
	// still reaches the actual inviter, so only inner TLS can prevent disclosure.
	inv := f.inv
	route := *inv.Route
	route.Pin = hex.EncodeToString(f.requester.KeyPin[:])
	inv.Route = &route
	inv.KeyPin = route.Pin
	inv.CertificateDER = base64.StdEncoding.EncodeToString(f.requester.Leaf.Raw)
	manager := network.NewManager(network.ManagerOptions{})
	defer manager.Close()
	target := network.Target{Device: f.f.senderID.DeviceID, Pin: f.requester.KeyPin, Profile: history.Digest(mustNetworkID(route.Profile)), Purpose: network.Enrollment}
	if err = manager.SetRelay(target, func(ctx context.Context, _ network.Target) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", l.Addr().String())
	}); err != nil {
		t.Fatal(err)
	}
	client, err := NewV3EnrollmentClient(context.Background(), inv, f.requester, f.selection, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.PrepareV3(context.Background(), strings.Repeat("5", 64), "never disclosed"); err == nil {
		t.Fatal("wrong inviter accepted")
	}
	if hits.Load() != 0 {
		t.Fatal("HTTP capability disclosed before pin verification")
	}
}
func TestWANW05RoutedUnsentExpiryAndSingleUse(t *testing.T) {
	f := routedAdmission(t)
	w := f.prepare(t)
	base := f.s.now()
	f.s.now = func() time.Time { return base.Add(2 * time.Minute) }
	if _, err := f.s.routedSubmit(f.f.ctx, w); err == nil || err.Error() != "EXPIRED_REPLAY" {
		t.Fatal("unsent expired transcript renewed", err)
	}
	f.s.now = time.Now
	if _, err := f.s.routedSubmit(f.f.ctx, w); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.routedChallenge(f.f.ctx, p.RoutedChallengeRequest{Version: "3", Folder: f.inv.Folder, Inviter: *f.inv.Route, Requester: w.Transcript.Requester, Capability: f.inv.Capability, Attempt: strings.Repeat("9", 64)}); err == nil {
		t.Fatal("single-use capability reused")
	}
	var record EnrollmentRecord
	if err := f.f.senderRepo.EnrollmentTransaction(f.f.ctx, func(tx *repository.EnrollmentTx) error { return tx.Get("request/"+mustRequestID(w), &record) }); err != nil {
		t.Fatal(err)
	}
	if record.Routed.Capability != "" || record.Wire.Token != "" {
		t.Fatal("inviter stored raw capability")
	}
}
func mustRequestID(w p.RoutedEnrollmentRequest) string { id, _ := w.Transcript.RequestID(); return id }

func TestWANW05RoutedRetiredIdentityCannotReenroll(t *testing.T) {
	f := routedAdmission(t)
	f.requester = f.f.receiverID
	m, approved, err := f.f.senderRepo.GetMembership(f.f.ctx, f.f.folder)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := p.RetirementSnapshot{Folder: f.f.folder, RetiredDevice: f.requester.DeviceID, ConfigurationRev: m.Revision, AcceptedByRetiree: []p.RetiredVersion{}}
	digest, err := p.RetirementSnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	active := m.Active[:0]
	for _, a := range m.Active {
		if a.Device != f.requester.DeviceID {
			active = append(active, a)
		}
	}
	m.Active = active
	m.PriorDigest = approved.Digest
	m.Revision++
	m.Retired = append(m.Retired, p.RetiredMember{Device: f.requester.DeviceID, RetiredAt: m.Revision, SnapshotDigest: digest})
	if _, err = f.f.senderRepo.ApproveMembership(f.f.ctx, m, snapshot); err != nil {
		t.Fatal(err)
	}
	w := f.prepare(t)
	if _, err = f.s.routedSubmit(f.f.ctx, w); err == nil || err.Error() != "UNAUTHORIZED" {
		t.Fatal("retired identity admitted", err)
	}
}
func TestWANW05RoutedApprovalArtifactBinding(t *testing.T) {
	f := routedAdmission(t)
	w := f.prepare(t)
	pending, err := f.s.routedSubmit(f.f.ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	m, app, err := f.f.senderRepo.GetMembership(f.f.ctx, f.f.folder)
	if err != nil {
		t.Fatal(err)
	}
	m.PriorDigest = app.Digest
	m.Revision++
	m.Active = append(m.Active, p.ActiveMember{Device: f.requester.DeviceID, KeyPin: f.requester.KeyPin})
	raw, err := p.EncodeMembership(m)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.MembershipDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	approval := p.RoutedEnrollmentApproval{Version: "3", Request: pending.Request, TranscriptDigest: pending.TranscriptDigest, PriorMembership: w.Transcript.PriorMembership, MembershipDigest: hex.EncodeToString(d[:])}
	b, _ := approval.Canonical()
	pending.Approval = &approval
	pending.ApprovalSignature = hex.EncodeToString(ed25519.Sign(f.f.senderID.Certificate.PrivateKey.(ed25519.PrivateKey), b))
	pending.MembershipHex = hex.EncodeToString(raw)
	pending.State = "approved"
	client := &EnrollmentClient{invitation: f.inv, identity: f.requester}
	if err = client.validateV3Result(w, pending); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"signature", "request", "transcript", "prior", "membership"} {
		t.Run(kind, func(t *testing.T) {
			changed := pending
			a := *pending.Approval
			changed.Approval = &a
			switch kind {
			case "signature":
				changed.ApprovalSignature = strings.Repeat("0", 128)
			case "request":
				a.Request = strings.Repeat("4", 64)
			case "transcript":
				a.TranscriptDigest = strings.Repeat("4", 64)
			case "prior":
				a.PriorMembership = strings.Repeat("4", 64)
			case "membership":
				a.MembershipDigest = strings.Repeat("4", 64)
			}
			if client.validateV3Result(w, changed) == nil {
				t.Fatal("changed approval artifact accepted")
			}
		})
	}
}
func TestWANW05RoutedPendingRequesterDataAndControlIsolation(t *testing.T) {
	f := routedAdmission(t)
	wire := f.prepare(t)
	if _, err := f.s.routedSubmit(f.f.ctx, wire); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("https://"+f.f.listener.Addr().String(), f.requester, f.f.senderID.Leaf, f.f.senderID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	_, err = client.Inventory(f.f.ctx, InventoryRequest{ProtocolVersion: "1", DeviceID: hex.EncodeToString(f.requester.DeviceID[:]), FolderID: f.inv.Folder, Revision: "1", MembershipDigest: hex.EncodeToString(f.f.approved.Digest[:]), Cursor: "0", PageSize: "128"})
	if err == nil {
		t.Fatal("pending requester fetched inventory")
	}
	for _, path := range []string{"/peer/v1/inventory", "/peer/v1/chunk", "/api/v1/settings"} {
		req, _ := http.NewRequest("POST", "https://inviter.invalid"+path, strings.NewReader("{}"))
		req.RemoteAddr = "127.0.0.1:1234"
		recorder := httptest.NewRecorder()
		f.s.ServeHTTP(recorder, req)
		if recorder.Code != 404 {
			t.Fatal("isolated enrollment handler exposed", path)
		}
	}
}
