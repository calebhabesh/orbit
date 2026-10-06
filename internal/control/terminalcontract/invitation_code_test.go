package terminalcontract_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
)

func w14Invitation(t *testing.T, s network.ProfileSelection) tc.Invitation {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var device history.ID
	device[0] = 7
	id, err := replication.LoadOrCreateIdentity(dir, device, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := s.Digest()
	p := s.Profile
	inviter := hex.EncodeToString(device[:])
	pin := hex.EncodeToString(id.KeyPin[:])
	return tc.Invitation{Version: "3", Folder: strings.Repeat("a", 64), Inviter: inviter, KeyPin: pin, CertificateDER: base64.StdEncoding.EncodeToString(id.Leaf.Raw), Capability: strings.Repeat("b", 64), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), Route: &protocol.EnrollmentRoute{Device: inviter, Pin: pin, Profile: digest, Purpose: "enrollment"}, Profile: &p}
}

func w14Signed(t *testing.T, env string) network.ProfileSelection {
	_, signer, _ := ed25519.GenerateKey(rand.Reader)
	service, _, _ := ed25519.GenerateKey(rand.Reader)
	p := protocol.NetworkProfile{Version: "1", Operator: "W14", Authority: hex.EncodeToString(signer.Public().(ed25519.PublicKey)), ServiceKey: hex.EncodeToString(service), Epoch: 1, Expires: protocol.NetworkUint(time.Now().Add(time.Hour).Unix()), Origins: []string{"https://orbit.example.net:8443", "wss://orbit.example.net:8443"}, STUN: []string{}, Privacy: "Ephemeral."}
	b, _ := p.Canonical(env != "release")
	p.Signature = hex.EncodeToString(ed25519.Sign(signer, b))
	return network.ProfileSelection{Profile: p, Authority: p.Authority, HighestEpoch: 1, Environment: env}
}

func decode(t *testing.T, code string) tc.Invitation {
	t.Helper()
	b, ok, err := tc.DecodeInvitationCode(code)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	var inv tc.Invitation
	if err = tc.Decode(b, &inv); err != nil {
		t.Fatal(err)
	}
	return inv
}

// A routed invitation under the packaged profile becomes a shorter code; the
// receiver restores the identical profile and the route digest still binds it.
func TestWANW14CompactInvitationCode(t *testing.T) {
	packaged := w14Signed(t, "release")
	restore := network.OverrideBuiltinProfile(&packaged)
	defer restore()
	inv := w14Invitation(t, packaged)
	if err := inv.Validate(); err != nil {
		t.Fatal(err)
	}
	full, _ := json.Marshal(inv)
	code, err := tc.InvitationCode(inv)
	if err != nil || !strings.HasPrefix(code, tc.InvitationCodeV3) {
		t.Fatal(code, err)
	}
	t.Logf("full v3 code %d chars; compact %d chars", len(tc.InvitationCodeV3)+base64.RawURLEncoding.EncodedLen(len(full)), len(code))
	if len(code) >= len(tc.InvitationCodeV3)+base64.RawURLEncoding.EncodedLen(len(full)) {
		t.Fatal("packaged invitation not compacted")
	}
	got := decode(t, code)
	if got.Profile != nil {
		t.Fatal("compact code carried profile")
	}
	if err = got.ExpandPackaged(); err != nil {
		t.Fatal(err)
	}
	if err = got.Validate(); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(got)
	if string(a) != string(full) {
		t.Fatal("expanded invitation differs")
	}

	// A receiver whose build packages another profile gets a precise error.
	other := w14Signed(t, "release")
	network.OverrideBuiltinProfile(&other)
	got = decode(t, code)
	if err = got.ExpandPackaged(); !errors.Is(err, tc.ErrProfileNotPackaged) {
		t.Fatal(err)
	}
	// Self-hosted invitations keep their profile in the code.
	self := w14Signed(t, "self_hosted")
	selfInv := w14Invitation(t, self)
	code, _ = tc.InvitationCode(selfInv)
	if decode(t, code).Profile == nil {
		t.Fatal("non-packaged profile dropped")
	}
	// Legacy v2 codes are unchanged.
	if _, ok, err := tc.DecodeInvitationCode("orbit-invitation:v2:" + base64.RawURLEncoding.EncodeToString([]byte(`{}`))); !ok || err != nil {
		t.Fatal(ok, err)
	}
}

func TestWANW14FutureInvitationVersionIsPrecise(t *testing.T) {
	if _, ok, err := tc.DecodeInvitationCode("orbit-invitation:v4:AAAA"); !ok || !errors.Is(err, tc.ErrNewerInvitation) {
		t.Fatal(ok, err)
	}
	if err := (tc.Invitation{Version: "4"}).Validate(); !errors.Is(err, tc.ErrNewerInvitation) {
		t.Fatal(err)
	}
	if _, ok, err := tc.DecodeInvitationCode("/home/user/invite.json"); ok || err != nil {
		t.Fatal("file path treated as code", ok, err)
	}
}
