package rendezvous

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
)

// Member sponsorship: listed devices admit the peers in their orbits.

type memorySponsors struct {
	mu    sync.Mutex
	saved []SponsoredDevice
	saves int
	fail  bool
}

func (m *memorySponsors) SaveSponsored(d []SponsoredDevice) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("disk full")
	}
	m.saved, m.saves = append([]SponsoredDevice(nil), d...), m.saves+1
	return nil
}

func refusedWith(err error, code string) bool {
	var s *network.ServiceError
	return errors.As(err, &s) && s.Code == code
}

func sponsoring(t *testing.T, f *fixture, sponsored []SponsoredDevice, revoked []string, devices ...device) *memorySponsors {
	t.Helper()
	a, err := allowlist(t, devices...).WithSponsorship(sponsored, revoked)
	if err != nil {
		t.Fatal(err)
	}
	store := &memorySponsors{}
	f.s.mu.Lock()
	f.s.sponsors = store
	f.s.mu.Unlock()
	f.s.SetAdmission(a)
	return store
}

func TestSponsorshipAdmitsOrbitPeers(t *testing.T) {
	f := newFixture(t, true)
	liveClock(t, f)
	owner, laptop, phone, other, revoked := identity(t, 1, nil), identity(t, 2, nil), identity(t, 3, nil), identity(t, 4, nil), identity(t, 5, nil)
	store := sponsoring(t, f, nil, []string{revoked.id}, owner)
	co, cl, cp := f.client(t, owner, f.roots), f.client(t, laptop, f.roots), f.client(t, phone, f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := cl.Announce(ctx, "peer_data", nonce(), announcement(f, 1)); !notAdmitted(err) {
		t.Fatal("unlisted data announce before sponsorship", err)
	}
	// An unlisted device cannot sponsor anyone, itself included.
	if err := cl.Sponsor(ctx, phone.id, phone.pin); !notAdmitted(err) {
		t.Fatal("unlisted sponsor", err)
	}
	if err := co.Sponsor(ctx, laptop.id, laptop.pin); err != nil {
		t.Fatal("listed sponsor", err)
	}
	if err := cl.Announce(ctx, "peer_data", nonce(), announcement(f, 2)); err != nil {
		t.Fatal("sponsored device not served", err)
	}
	// Transitive: a sponsored device sponsors its own orbit peers.
	if err := cl.Sponsor(ctx, phone.id, phone.pin); err != nil {
		t.Fatal("sponsored device sponsoring", err)
	}
	if err := cp.Announce(ctx, "peer_data", nonce(), announcement(f, 1)); err != nil {
		t.Fatal("transitively sponsored device not served", err)
	}
	// Repeats change nothing and write nothing.
	if err := co.Sponsor(ctx, laptop.id, laptop.pin); err != nil || store.saves != 2 {
		t.Fatal("repeat sponsor", err, store.saves)
	}
	if len(store.saved) != 2 || store.saved[0].Device != laptop.id || store.saved[0].Sponsor != owner.id || store.saved[1].Sponsor != laptop.id {
		t.Fatalf("stored sponsorship %+v", store.saved)
	}
	// Refusals: a revoked device, a listed device under another pin, oneself.
	if err := co.Sponsor(ctx, revoked.id, revoked.pin); !notAdmitted(err) {
		t.Fatal("revoked device sponsored", err)
	}
	if err := cl.Sponsor(ctx, owner.id, other.pin); !refusedWith(err, p.NetworkIdentityMismatch) {
		t.Fatal("static device re-pinned by sponsorship", err)
	}
	if err := co.Sponsor(ctx, owner.id, owner.pin); !refusedWith(err, p.NetworkInvalidRequest) {
		t.Fatal("self sponsorship", err)
	}
	if err := cl.Sponsor(ctx, owner.id, owner.pin); err != nil {
		t.Fatal("sponsoring an already listed device", err)
	}
	// A store failure admits nobody.
	store.fail = true
	if err := co.Sponsor(ctx, other.id, other.pin); !refusedWith(err, p.NetworkServiceUnavailable) {
		t.Fatal("sponsor with failing store", err)
	}
	store.fail = false
	if f.s.Admission().listedActor(actor{other.id, other.pin}) {
		t.Fatal("device admitted although its sponsorship was not saved")
	}
	m := f.s.Metrics()
	if !m.Sponsoring || m.SponsoredSize != 2 || m.SponsoredAdmitted != 2 || m.AllowlistSize != 3 {
		t.Fatalf("sponsorship metrics %+v", m)
	}
}

func TestSponsorshipUnsupportedWithoutIt(t *testing.T) {
	f := newFixture(t, true)
	liveClock(t, f)
	owner, laptop := identity(t, 1, nil), identity(t, 2, nil)
	co := f.client(t, owner, f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := co.Sponsor(ctx, laptop.id, laptop.pin); !refusedWith(err, p.NetworkUnsupported) {
		t.Fatal("open admission sponsor", err)
	}
	f.s.SetAdmission(allowlist(t, owner))
	if err := co.Sponsor(ctx, laptop.id, laptop.pin); !refusedWith(err, p.NetworkUnsupported) {
		t.Fatal("allowlist without sponsorship", err)
	}
}

func TestSponsorshipLoadAndLimits(t *testing.T) {
	owner, laptop := identity(t, 1, nil), identity(t, 2, nil)
	base := allowlist(t, owner)
	// Stored entries for revoked or statically listed devices are dropped.
	a, err := base.WithSponsorship([]SponsoredDevice{{laptop.id, laptop.pin, owner.id, 1}, {owner.id, laptop.pin, owner.id, 1}}, nil)
	if err != nil || len(a.Sponsored()) != 1 || !a.listedActor(actor{laptop.id, laptop.pin}) {
		t.Fatal("load", err, a.Sponsored())
	}
	if a, _ = base.WithSponsorship(a.Sponsored(), []string{laptop.id}); len(a.Sponsored()) != 0 || a.listedActor(actor{laptop.id, laptop.pin}) {
		t.Fatal("revoked entry kept")
	}
	if _, err = OpenAdmission().WithSponsorship(nil, nil); err == nil {
		t.Fatal("sponsorship over open admission")
	}
	// A device with a new identity replaces its sponsored pin.
	a, _ = base.WithSponsorship(nil, nil)
	a, _ = a.sponsor(actor{laptop.id, laptop.pin}, owner.id, 1)
	fresh := identity(t, 3, nil)
	a, code := a.sponsor(actor{laptop.id, fresh.pin}, owner.id, 2)
	if code != "" || len(a.Sponsored()) != 1 || a.listedActor(actor{laptop.id, laptop.pin}) || !a.listedActor(actor{laptop.id, fresh.pin}) {
		t.Fatal("re-pinned sponsored device", code, a.Sponsored())
	}
	// The 64-device limit covers listed and sponsored devices together.
	for i := 0; a.Size() < MaxAllowedDevices; i++ {
		d := identity(t, 100+i, nil)
		if a, code = a.sponsor(actor{d.id, d.pin}, owner.id, 3); code != "" {
			t.Fatal(i, code)
		}
	}
	d := identity(t, 999, nil)
	if _, code = a.sponsor(actor{d.id, d.pin}, owner.id, 4); code != p.NetworkQuota {
		t.Fatal("sponsorship past the device limit", code)
	}
}

func peerTarget(t *testing.T, f *fixture, d device) network.Target {
	t.Helper()
	var target network.Target
	raw, _ := hex.DecodeString(d.id)
	copy(target.Device[:], raw)
	raw, _ = hex.DecodeString(d.pin)
	copy(target.Pin[:], raw)
	raw, _ = hex.DecodeString(f.s.digest)
	target.Profile = history.Digest(raw)
	target.Purpose = network.PeerData
	return target
}

func sponsorRuntime(t *testing.T, f *fixture, d device) *network.RelayRuntime {
	t.Helper()
	m := network.NewManager(network.ManagerOptions{})
	t.Cleanup(func() { _ = m.Close() })
	r, err := network.NewRelayRuntime(context.Background(), f.client(t, d, f.roots), m, 1, f.s.digest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// The joining flow on production runtimes: a new, unlisted device is refused,
// joins an orbit with a listed device, and is served within a minute with no
// operator step.
func TestSponsorshipRuntimeJoinNeedsNoOperator(t *testing.T) {
	if testing.Short() {
		t.Skip("real retry timers; runs in the full suite")
	}
	f := newFixture(t, true)
	liveClock(t, f)
	owner, joiner := identity(t, 1, nil), identity(t, 2, nil)
	store := sponsoring(t, f, nil, nil, owner)
	ro, rj := sponsorRuntime(t, f, owner), sponsorRuntime(t, f, joiner)
	deadline := time.Now().Add(15 * time.Second)
	for (!ro.Ready() || rj.ServiceRefusal() == "") && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !ro.Ready() || rj.ServiceRefusal() != p.NetworkNotAdmitted {
		t.Fatal("start: owner ready", ro.Ready(), "joiner refusal", rj.ServiceRefusal())
	}
	// Joining an orbit registers each side as the other's peer.
	start := time.Now()
	if err := rj.Register(peerTarget(t, f, owner)); err != nil {
		t.Fatal(err)
	}
	if err := ro.Register(peerTarget(t, f, joiner)); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(90 * time.Second)
	for !rj.Ready() && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if !rj.Ready() || rj.ServiceRefusal() != "" {
		t.Fatal("joiner not served after joining", rj.ServiceRefusal())
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.saved) != 1 || store.saved[0].Device != joiner.id || store.saved[0].Sponsor != owner.id {
		t.Fatalf("stored %+v", store.saved)
	}
	t.Logf("joiner served %s after joining, no operator step", time.Since(start).Round(time.Second))
}
