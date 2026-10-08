package rendezvous

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
)

func rotationProfile(t *testing.T, signer ed25519.PrivateKey, origin string, epoch, expires uint64, environment string) ServedProfile {
	t.Helper()
	online, onlinePrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profile := p.NetworkProfile{Version: "1", Operator: "Rotation fixture", Authority: hex.EncodeToString(signer.Public().(ed25519.PublicKey)), ServiceKey: hex.EncodeToString(online), Epoch: p.NetworkUint(epoch), Expires: p.NetworkUint(expires), Origins: []string{origin, "wss://" + origin[len("https://"):]}, STUN: []string{}, Privacy: "Development only; routing metadata is ephemeral."}
	b, err := profile.Canonical(true)
	if err != nil {
		t.Fatal(err)
	}
	profile.Signature = hex.EncodeToString(ed25519.Sign(signer, b))
	return ServedProfile{Selection: network.ProfileSelection{Profile: profile, Authority: profile.Authority, HighestEpoch: profile.Epoch, Environment: environment}, ServiceKey: onlinePrivate}
}

// Rotation serves two adjacent reviewed epochs so devices can review the next
// profile before the current one expires; neither epoch outlives its expiry.
func TestWANW13RotationOverlapAndSanitizedMetrics(t *testing.T) {
	f := &fixture{origin: "https://directory.orbit.invalid"}
	f.now.Store(time.Now().Unix())
	now := uint64(f.now.Load())
	_, signer, _ := ed25519.GenerateKey(rand.Reader)
	current := rotationProfile(t, signer, f.origin, 1, now+3600, "development")
	next := rotationProfile(t, signer, f.origin, 2, now+7200, "development")
	clock := func() time.Time { return time.Unix(f.now.Load(), 0) }

	_, otherSigner, _ := ed25519.GenerateKey(rand.Reader)
	for name, overlap := range map[string][]ServedProfile{
		"foreign authority":  {rotationProfile(t, otherSigner, f.origin, 1, now+3600, "development")},
		"other environment":  {rotationProfile(t, signer, f.origin, 1, now+3600, "self_hosted")},
		"other origin":       {rotationProfile(t, signer, "https://other.orbit.invalid", 1, now+3600, "development")},
		"duplicate epoch":    {next},
		"three epochs":       {current, rotationProfile(t, signer, f.origin, 3, now+3600, "development")},
		"expired at startup": {rotationProfile(t, signer, f.origin, 1, now-1, "development")},
	} {
		if s, err := New(Options{Selection: next.Selection, Origin: f.origin, ServiceKey: next.ServiceKey, Overlap: overlap, Now: clock}); err == nil {
			_ = s.Close()
			t.Fatalf("%s overlap admitted", name)
		}
	}
	var err error
	f.s, err = New(Options{Selection: next.Selection, Origin: f.origin, ServiceKey: next.ServiceKey, Overlap: []ServedProfile{current}, Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.s.Close() })
	digests := map[string]string{}
	for name, sp := range map[string]ServedProfile{"current": current, "next": next} {
		digests[name], _ = sp.Selection.Digest()
	}
	challenge := func(d device, digest string) (int, string) {
		var out p.NetworkChallengeResult
		status, code := f.call(t, "challenge", p.NetworkChallengeRequest{Version: "1", Profile: digest, Sender: d.id, SenderPin: d.pin, CertificateDER: d.der}, &out)
		if status == 200 && out.Profile != digest {
			t.Fatal("challenge bound to another epoch")
		}
		return status, code
	}
	a, b := identity(t, 1, nil), identity(t, 2, nil)
	if status, code := challenge(a, digests["current"]); status != 200 {
		t.Fatal("current epoch refused", code)
	}
	if status, code := challenge(b, digests["next"]); status != 200 {
		t.Fatal("next epoch refused", code)
	}
	if _, code := challenge(a, hex.EncodeToString(make([]byte, 32))); code != p.NetworkProfileUntrusted {
		t.Fatal("unknown profile admitted", code)
	}
	f.now.Store(int64(now + 3601))
	if _, code := challenge(a, digests["current"]); code != p.NetworkProfileExpired {
		t.Fatal("expired overlap epoch admitted", code)
	}
	if status, code := challenge(b, digests["next"]); status != 200 {
		t.Fatal("next epoch refused after overlap expiry", code)
	}
	m := f.s.Metrics()
	if len(m.Profiles) != 2 || m.Profiles[0].Epoch != 1 || m.Profiles[0].Valid || m.Profiles[1].Epoch != 2 || !m.Profiles[1].Valid {
		t.Fatalf("profile metrics %+v", m.Profiles)
	}
	if m.RefusedExpired != 1 || m.RefusedUntrusted != 1 || m.Challenges != 1 { // earlier challenges expired after 60 seconds
		t.Fatalf("refusal metrics %+v", m)
	}
	f.now.Store(int64(now + 7201))
	if _, code := challenge(b, digests["next"]); code != p.NetworkProfileExpired {
		t.Fatal("service admitted after every epoch expired", code)
	}
}
