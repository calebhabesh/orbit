package config_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/replication"
)

func w14Profile(t *testing.T, signer ed25519.PrivateKey, epoch, expires uint64, env, privacy string) network.ProfileSelection {
	t.Helper()
	service, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p := protocol.NetworkProfile{Version: "1", Operator: "W14 operator", Authority: hex.EncodeToString(signer.Public().(ed25519.PublicKey)), ServiceKey: hex.EncodeToString(service), Epoch: protocol.NetworkUint(epoch), Expires: protocol.NetworkUint(expires), Origins: []string{"https://orbit.example.net:8443", "wss://orbit.example.net:8443"}, STUN: []string{}, Privacy: privacy}
	b, err := p.Canonical(env != "release")
	if err != nil {
		t.Fatal(err)
	}
	p.Signature = hex.EncodeToString(ed25519.Sign(signer, b))
	return network.ProfileSelection{Profile: p, Authority: p.Authority, HighestEpoch: p.Epoch, Environment: env}
}

func w14Dir(t *testing.T) string {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestWANW14PackagedProfileAdoptionMatrix(t *testing.T) {
	now := uint64(time.Now().Unix())
	_, signer, _ := ed25519.GenerateKey(rand.Reader)
	epoch1 := w14Profile(t, signer, 1, now+3600, "release", "Ephemeral routing metadata.")
	epoch2 := w14Profile(t, signer, 2, now+7200, "release", "Ephemeral routing metadata.")
	reworded := w14Profile(t, signer, 2, now+7200, "release", "Routing metadata kept for 30 days.")
	d1, _ := epoch1.Digest()
	d2, _ := epoch2.Digest()

	t.Run("automatic awaiting adopts once", func(t *testing.T) {
		dir := w14Dir(t)
		if err := config.SaveNetworkPolicy(dir, tc.NetworkPolicy{Mode: "automatic", AwaitingProfile: true, LANAdvertising: true, Generation: 2}); err != nil {
			t.Fatal(err)
		}
		out, err := config.AdoptPackagedProfile(dir, epoch1, now)
		if err != nil || out != config.PackagedAdopted {
			t.Fatal(out, err)
		}
		policy, _ := config.LoadNetworkPolicy(dir)
		stored, err := config.LoadNetworkProfile(dir, now)
		if err != nil || policy.Profile != d1 || policy.AwaitingProfile || policy.Generation != 3 || !policy.LANAdvertising || stored.Profile.Epoch != 1 {
			t.Fatal(policy, stored, err)
		}
		if out, err = config.AdoptPackagedProfile(dir, epoch1, now); err != nil || out != "" {
			t.Fatal("second start changed state", out, err)
		}
	})

	t.Run("manual local-only and legacy installs untouched", func(t *testing.T) {
		for _, policy := range []*tc.NetworkPolicy{nil, {Mode: "manual", Generation: 4}, {Mode: "local_only", LANAdvertising: true, Generation: 2}} {
			dir := w14Dir(t)
			if policy != nil {
				if err := config.SaveNetworkPolicy(dir, *policy); err != nil {
					t.Fatal(err)
				}
			}
			out, err := config.AdoptPackagedProfile(dir, epoch1, now)
			if err != nil || out != "" {
				t.Fatal(out, err)
			}
			if _, err = os.Stat(dir + "/network-profile.json"); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("profile written for non-automatic install")
			}
			if policy == nil {
				if _, err = os.Stat(dir + "/network.json"); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("legacy install acquired a policy file")
				}
			}
		}
	})

	t.Run("same text newer epoch updates and rebinds routes", func(t *testing.T) {
		dir := w14Dir(t)
		if err := config.SaveNetworkProfile(dir, epoch1, now); err != nil {
			t.Fatal(err)
		}
		if err := config.SaveNetworkPolicy(dir, tc.NetworkPolicy{Mode: "automatic", Profile: d1, Generation: 2}); err != nil {
			t.Fatal(err)
		}
		var device history.ID
		device[0] = 1
		id, err := replication.LoadOrCreateIdentity(dir, device, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		route := config.PeerRoute{Folder: strings.Repeat("1", 64), Device: hex.EncodeToString(device[:]), Pin: hex.EncodeToString(id.KeyPin[:]), Profile: d1, CertificateDER: base64.StdEncoding.EncodeToString(id.Leaf.Raw)}
		if err := config.SetPeerRoute(dir, route); err != nil {
			t.Fatal(err)
		}
		out, err := config.AdoptPackagedProfile(dir, epoch2, now)
		if err != nil || out != config.PackagedUpdated {
			t.Fatal(out, err)
		}
		policy, _ := config.LoadNetworkPolicy(dir)
		routes, _ := config.LoadPeerRoutes(dir)
		if policy.Profile != d2 || policy.Generation != 3 || len(routes) != 1 || routes[0].Profile != d2 {
			t.Fatal(policy, routes)
		}
		// An older packaged epoch (downgraded package) never rolls back.
		if out, err = config.AdoptPackagedProfile(dir, epoch1, now); err != nil || out != "" {
			t.Fatal("downgrade adopted", out, err)
		}
	})

	t.Run("changed privacy text waits for review", func(t *testing.T) {
		dir := w14Dir(t)
		if err := config.SaveNetworkProfile(dir, epoch1, now); err != nil {
			t.Fatal(err)
		}
		if err := config.SaveNetworkPolicy(dir, tc.NetworkPolicy{Mode: "automatic", Profile: d1, Generation: 2}); err != nil {
			t.Fatal(err)
		}
		out, err := config.AdoptPackagedProfile(dir, reworded, now)
		stored, _ := config.LoadNetworkProfile(dir, now)
		if err != nil || out != config.PackagedReview || stored.Profile.Epoch != 1 {
			t.Fatal(out, err, stored.Profile.Epoch)
		}
	})

	t.Run("expired stored profile is replaced by current packaged one", func(t *testing.T) {
		dir := w14Dir(t)
		old := w14Profile(t, signer, 1, now+60, "release", "Ephemeral routing metadata.")
		if err := config.SaveNetworkProfile(dir, old, now); err != nil {
			t.Fatal(err)
		}
		od, _ := old.Digest()
		if err := config.SaveNetworkPolicy(dir, tc.NetworkPolicy{Mode: "automatic", Profile: od, Generation: 2}); err != nil {
			t.Fatal(err)
		}
		if out, err := config.AdoptPackagedProfile(dir, epoch2, now+120); err != nil || out != config.PackagedUpdated {
			t.Fatal(out, err)
		}
	})

	t.Run("interrupted update completes on next start", func(t *testing.T) {
		dir := w14Dir(t)
		if err := config.SaveNetworkProfile(dir, epoch1, now); err != nil {
			t.Fatal(err)
		}
		if err := config.SaveNetworkPolicy(dir, tc.NetworkPolicy{Mode: "automatic", Profile: d1, Generation: 2}); err != nil {
			t.Fatal(err)
		}
		if err := config.SaveNetworkProfile(dir, epoch2, now); err != nil { // crash before policy write
			t.Fatal(err)
		}
		out, err := config.AdoptPackagedProfile(dir, epoch2, now)
		policy, _ := config.LoadNetworkPolicy(dir)
		if err != nil || out != config.PackagedCompleted || policy.Profile != d2 {
			t.Fatal(out, err, policy)
		}
	})

	t.Run("other authority and expired packaged profile untouched", func(t *testing.T) {
		_, other, _ := ed25519.GenerateKey(rand.Reader)
		self := w14Profile(t, other, 5, now+3600, "self_hosted", "Self-hosted.")
		sd, _ := self.Digest()
		dir := w14Dir(t)
		if err := config.SaveNetworkProfile(dir, self, now); err != nil {
			t.Fatal(err)
		}
		if err := config.SaveNetworkPolicy(dir, tc.NetworkPolicy{Mode: "self_hosted", Profile: sd, Generation: 2}); err != nil {
			t.Fatal(err)
		}
		if out, err := config.AdoptPackagedProfile(dir, epoch2, now); err != nil || out != "" {
			t.Fatal(out, err)
		}
		dir = w14Dir(t)
		if err := config.SaveNetworkPolicy(dir, tc.NetworkPolicy{Mode: "automatic", AwaitingProfile: true, Generation: 2}); err != nil {
			t.Fatal(err)
		}
		if out, err := config.AdoptPackagedProfile(dir, epoch1, now+7200); err != nil || out != "" {
			t.Fatal("expired packaged profile adopted", out, err)
		}
	})
}

func TestWANW14OperatorSwitchKeepsRollbackFloors(t *testing.T) {
	now := uint64(time.Now().Unix())
	_, a, _ := ed25519.GenerateKey(rand.Reader)
	_, b, _ := ed25519.GenerateKey(rand.Reader)
	a1 := w14Profile(t, a, 1, now+3600, "release", "A.")
	a3 := w14Profile(t, a, 3, now+3600, "release", "A.")
	b1 := w14Profile(t, b, 1, now+3600, "self_hosted", "B.")
	dir := w14Dir(t)
	if err := config.SaveNetworkProfile(dir, a3, now); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveNetworkProfile(dir, b1, now); !errors.Is(err, network.ErrOperatorChange) {
		t.Fatal("unconfirmed operator switch accepted", err)
	}
	if err := config.SaveNetworkProfileChange(dir, b1, now, true); err != nil {
		t.Fatal(err)
	}
	floors, err := config.LoadProfileFloors(dir)
	if err != nil || floors[a3.Authority] != 3 || floors[b1.Authority] != 1 {
		t.Fatal(floors, err)
	}
	// Switching back cannot reintroduce an epoch below the one already seen.
	if err = config.SaveNetworkProfileChange(dir, a1, now, true); err == nil {
		t.Fatal("rollback through operator switch accepted")
	}
	if err = config.SaveNetworkProfileChange(dir, a3, now, true); err != nil {
		t.Fatal(err)
	}
	stored, err := config.LoadNetworkProfile(dir, now)
	if err != nil || stored.Authority != a3.Authority || stored.Profile.Epoch != 3 {
		t.Fatal(stored, err)
	}
}
