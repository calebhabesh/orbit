package rendezvous

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
)

// R02 service admission (RG1) on the production Service over a local TLS
// listener with real ServiceClients.

func notAdmitted(err error) bool {
	var s *network.ServiceError
	return errors.As(err, &s) && s.Code == p.NetworkNotAdmitted
}

func allowlist(t *testing.T, devices ...device) *Admission {
	t.Helper()
	list := []AllowedDevice{}
	for _, d := range devices {
		list = append(list, AllowedDevice{Device: d.id, Pin: d.pin})
	}
	a, err := NewAllowlist(list)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func enrollmentAnnouncement(f *fixture, g uint64) p.NetworkAnnouncement {
	a := announcement(f, g)
	a.Candidates, a.Capabilities, a.Relay = []p.NetworkCandidate{}, []string{"relay_inner_tls_v1", "enrollment_v3"}, true
	return a
}

func TestFinalR02AllowlistCoversEveryOperation(t *testing.T) {
	f := newFixture(t, true)
	liveClock(t, f)
	listed, unlisted, stranger := identity(t, 1, nil), identity(t, 2, nil), identity(t, 3, nil)
	f.s.SetAdmission(allowlist(t, listed))
	cl, cu, cs := f.client(t, listed, f.roots), f.client(t, unlisted, f.roots), f.client(t, stranger, f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Directory announce: listed for both purposes; unlisted only enrollment.
	if err := cl.Announce(ctx, "peer_data", nonce(), announcement(f, 1)); err != nil {
		t.Fatal("listed announce", err)
	}
	if err := cl.Announce(ctx, "enrollment", nonce(), enrollmentAnnouncement(f, 1)); err != nil {
		t.Fatal("listed enrollment announce", err)
	}
	if err := cu.Announce(ctx, "peer_data", nonce(), announcement(f, 1)); !notAdmitted(err) {
		t.Fatal("unlisted data announce", err)
	}
	if err := cu.Announce(ctx, "enrollment", nonce(), enrollmentAnnouncement(f, 1)); err != nil {
		t.Fatal("unlisted enrollment announce (pairing path)", err)
	}
	// Lookup: unlisted only toward a listed device's enrollment record.
	if _, _, err := cu.Lookup(ctx, listed.id, listed.pin, "peer_data", 0); !notAdmitted(err) {
		t.Fatal("unlisted data lookup", err)
	}
	if _, found, err := cu.Lookup(ctx, listed.id, listed.pin, "enrollment", 0); err != nil || !found {
		t.Fatal("unlisted enrollment lookup toward listed", found, err)
	}
	if _, _, err := cu.Lookup(ctx, stranger.id, stranger.pin, "enrollment", 0); !notAdmitted(err) {
		t.Fatal("unlisted lookup toward unlisted", err)
	}
	// Control: unlisted data control is refused with a typed reason.
	if _, err := cu.Control(ctx, "peer_data", map[string]string{}, false); !notAdmitted(err) {
		t.Fatal("unlisted data control", err)
	}
	lc, err := cl.Control(ctx, "enrollment", map[string]string{}, true)
	if err != nil {
		t.Fatal("listed enrollment control", err)
	}
	defer lc.Close()
	uc, err := cu.Control(ctx, "enrollment", map[string]string{listed.id: listed.pin}, false)
	if err != nil {
		t.Fatal("unlisted enrollment control", err)
	}
	defer uc.Close()
	// Offers and reservations: data refused; enrollment toward listed admitted.
	data := p.NetworkProof{Kind: "offer", Target: listed.id, TargetPin: listed.pin, Purpose: "peer_data", Session: nonce(), Role: "initiator", Operation: nonce()}
	if err = cu.Exchange(ctx, data, p.NetworkOffer{SenderGeneration: 1, TargetGeneration: 1, Candidates: []p.NetworkCandidate{}}); !notAdmitted(err) {
		t.Fatal("unlisted data offer", err)
	}
	data.Kind, data.Operation = "reserve", nonce()
	if _, err = cu.Reserve(ctx, data); !notAdmitted(err) {
		t.Fatal("unlisted data reservation", err)
	}
	session := nonce()
	offer := p.NetworkOffer{SenderGeneration: 1, TargetGeneration: 1, Candidates: []p.NetworkCandidate{}}
	enroll := p.NetworkProof{Kind: "offer", Target: listed.id, TargetPin: listed.pin, Purpose: "enrollment", Session: session, Role: "initiator", Operation: nonce()}
	if err = cu.Exchange(ctx, enroll, offer); err != nil {
		t.Fatal("unlisted enrollment offer toward listed", err)
	}
	if _, err = lc.Next(ctx); err != nil {
		t.Fatal("listed inviter did not receive the offer", err)
	}
	accept := p.NetworkProof{Kind: "accept", Target: unlisted.id, TargetPin: unlisted.pin, Purpose: "enrollment", Session: session, Role: "responder", Operation: nonce()}
	if err = cl.Exchange(ctx, accept, offer); err != nil {
		t.Fatal("listed accept toward unlisted joiner", err)
	}
	if _, err = uc.Next(ctx); err != nil {
		t.Fatal("joiner did not receive the accept", err)
	}
	enroll.Kind, enroll.Operation = "reserve", nonce()
	if _, err = cu.Reserve(ctx, enroll); err != nil {
		t.Fatal("unlisted enrollment reservation", err)
	}
	// Pairing mailbox: only listed devices create; an unlisted joiner claims.
	if _, err = cu.StartPairing(ctx, strings.Repeat("b", 64), []byte("orbit-invitation:v3:synthetic"), time.Now().Add(time.Minute)); !notAdmitted(err) {
		t.Fatal("unlisted mailbox create", err)
	}
	status, err := cl.StartPairing(ctx, strings.Repeat("a", 64), []byte("orbit-invitation:v3:synthetic"), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal("listed mailbox create", err)
	}
	if got, d, _, err := cu.JoinPairing(ctx, strings.ToLower(status.Code)); err != nil || d != listed.id || string(got) != "orbit-invitation:v3:synthetic" {
		t.Fatal("unlisted joiner claim", err)
	}
	// Refusals allocate nothing: the stranger's refused data work leaves no
	// rate bucket, record or control behind.
	if err = cs.Announce(ctx, "peer_data", nonce(), announcement(f, 1)); !notAdmitted(err) {
		t.Fatal("stranger announce", err)
	}
	f.s.mu.Lock()
	_, rate := f.s.rates[actor{stranger.id, stranger.pin}]
	_, record := f.s.records[routeKey{actor{stranger.id, stranger.pin}, "peer_data"}]
	f.s.mu.Unlock()
	if rate || record {
		t.Fatal("refused unlisted work allocated state", rate, record)
	}
	m := f.s.Metrics()
	if !m.Allowlist || m.AllowlistSize != 1 || m.RefusedNotAdmitted < 7 {
		t.Fatalf("admission metrics: %+v", m)
	}
	t.Logf("not_admitted refusals=%d; listed and pairing paths admitted", m.RefusedNotAdmitted)
}

func TestFinalR02UnlistedShareCannotExhaustListed(t *testing.T) {
	f := newFixture(t, false)
	listed := identity(t, 1, nil)
	f.s.SetAdmission(allowlist(t, listed))
	now := uint64(f.now.Load())
	f.s.mu.Lock()
	for i := 0; i < MaxUnlistedChallenges; i++ {
		d := identity(t, 100+i, nil)
		if _, code := f.s.challenge(p.NetworkChallengeRequest{Version: "1", Profile: f.s.digest, Sender: d.id, SenderPin: d.pin, CertificateDER: d.der}, now); code != "" {
			t.Fatal("unlisted challenge within share", i, code)
		}
	}
	d := identity(t, 999, nil)
	_, refused := f.s.challenge(p.NetworkChallengeRequest{Version: "1", Profile: f.s.digest, Sender: d.id, SenderPin: d.pin, CertificateDER: d.der}, now)
	_, admitted := f.s.challenge(p.NetworkChallengeRequest{Version: "1", Profile: f.s.digest, Sender: listed.id, SenderPin: listed.pin, CertificateDER: listed.der}, now)
	f.s.mu.Unlock()
	if refused != p.NetworkQuota || admitted != "" {
		t.Fatal("unlisted challenge share", refused, admitted)
	}
	f.s.mu.Lock()
	f.s.challenges = map[string]challenge{}
	f.s.mu.Unlock()
	// Directory records: unlisted enrollment announcements share 16 slots.
	for i := 0; i <= MaxUnlistedRecords; i++ {
		if i%10 == 0 {
			f.now.Add(20)
		}
		u := identity(t, 200+i, nil)
		a := enrollmentAnnouncement(f, 1)
		q := f.proof(t, u, "announce", "enrollment", nonce(), &a)
		_, code := f.call(t, "announce", p.NetworkAnnounceRequest{Proof: q, Announcement: a}, &p.NetworkResult{})
		if (i < MaxUnlistedRecords && code != "") || (i == MaxUnlistedRecords && code != p.NetworkQuota) {
			t.Fatal("unlisted record share", i, code)
		}
	}
	if _, code := f.announce(t, listed, announcement(f, 1), nonce()); code != "" {
		t.Fatal("listed device refused while unlisted share is full", code)
	}
}

func TestFinalR02ReloadWithdrawsRemovedDevice(t *testing.T) {
	f := newFixture(t, true)
	liveClock(t, f)
	kept, removed := identity(t, 1, nil), identity(t, 2, nil)
	f.s.SetAdmission(allowlist(t, kept, removed))
	ck, cr := f.client(t, kept, f.roots), f.client(t, removed, f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, c := range []*network.ServiceClient{ck, cr} {
		if err := c.Announce(ctx, "peer_data", nonce(), announcement(f, 1)); err != nil {
			t.Fatal(err)
		}
	}
	control, err := cr.Control(ctx, "peer_data", map[string]string{}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	f.s.SetAdmission(allowlist(t, kept))
	f.s.mu.Lock()
	_, record := f.s.records[routeKey{actor{removed.id, removed.pin}, "peer_data"}]
	_, live := f.s.controls[routeKey{actor{removed.id, removed.pin}, "peer_data"}]
	_, keptRecord := f.s.records[routeKey{actor{kept.id, kept.pin}, "peer_data"}]
	f.s.mu.Unlock()
	if record || live || !keptRecord {
		t.Fatal("reload did not withdraw exactly the removed device", record, live, keptRecord)
	}
	short, end := context.WithTimeout(ctx, 5*time.Second)
	defer end()
	if _, err = control.Next(short); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("removed device's control stayed open", err)
	}
	if err = cr.Announce(ctx, "peer_data", nonce(), announcement(f, 2)); !notAdmitted(err) {
		t.Fatal("removed device still admitted", err)
	}
}

// An unlisted device running the production runtime learns NOT_ADMITTED for
// peer data, stops asking for ten minutes, and keeps its enrollment path.
func TestFinalR02UnlistedRuntimeBacksOffAndKeepsPairingPath(t *testing.T) {
	if testing.Short() {
		t.Skip("real retry timers; runs in the full suite")
	}
	f := newFixture(t, true)
	liveClock(t, f)
	listed := identity(t, 900, nil)
	f.s.SetAdmission(allowlist(t, listed))
	r, d := r00Runtime(t, f, 1)
	deadline := time.Now().Add(15 * time.Second)
	for r.ServiceRefusal() == "" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if r.ServiceRefusal() != p.NetworkNotAdmitted {
		t.Fatal("runtime did not learn NOT_ADMITTED")
	}
	// Under a loaded race run the enrollment path can still be starting.
	enrolled := func() (bool, bool) {
		f.s.mu.Lock()
		defer f.s.mu.Unlock()
		_, record := f.s.records[routeKey{actor{d.id, d.pin}, "enrollment"}]
		_, control := f.s.controls[routeKey{actor{d.id, d.pin}, "enrollment"}]
		return record, control
	}
	for deadline := time.Now().Add(readyDeadline); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if record, control := enrolled(); record && control {
			break
		}
	}
	before := f.s.Metrics().RefusedNotAdmitted
	window := 30 * time.Second
	time.Sleep(window)
	after := f.s.Metrics().RefusedNotAdmitted
	f.s.mu.Lock()
	_, enrollment := f.s.records[routeKey{actor{d.id, d.pin}, "enrollment"}]
	_, enrollControl := f.s.controls[routeKey{actor{d.id, d.pin}, "enrollment"}]
	_, data := f.s.records[routeKey{actor{d.id, d.pin}, "peer_data"}]
	f.s.mu.Unlock()
	if after != before || !enrollment || !enrollControl || data {
		t.Fatalf("unlisted runtime: refusals in %s=%d enrollment record=%v control=%v data record=%v", window, after-before, enrollment, enrollControl, data)
	}
	t.Logf("unlisted runtime: %d refusals before back-off, 0 in the next %s; enrollment record and control live", before, window)
}
