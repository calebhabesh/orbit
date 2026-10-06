package control

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/protocol"
)

func mismatchProfile(t *testing.T, signer ed25519.PrivateKey, operator string, epoch uint64) network.ProfileSelection {
	service, _, _ := ed25519.GenerateKey(rand.Reader)
	p := protocol.NetworkProfile{Version: "1", Operator: operator, Authority: hex.EncodeToString(signer.Public().(ed25519.PublicKey)), ServiceKey: hex.EncodeToString(service), Epoch: protocol.NetworkUint(epoch), Expires: protocol.NetworkUint(time.Now().Add(time.Hour).Unix()), Origins: []string{"https://orbit.example.net:8443"}, STUN: []string{}, Privacy: "Ephemeral."}
	b, err := p.Canonical(false)
	if err != nil {
		t.Fatal(err)
	}
	p.Signature = hex.EncodeToString(ed25519.Sign(signer, b))
	return network.ProfileSelection{Profile: p, Authority: p.Authority, HighestEpoch: p.Epoch, Environment: "release"}
}

// Pairing across devices on different service profiles names the cause and
// which device to change instead of a bare PROFILE_MISMATCH.
func TestWANW14ProfileMismatchIsPrecise(t *testing.T) {
	_, a, _ := ed25519.GenerateKey(rand.Reader)
	_, b, _ := ed25519.GenerateKey(rand.Reader)
	local := mismatchProfile(t, a, "Hosted", 2)
	inv := func(s network.ProfileSelection) tc.Invitation {
		d, _ := s.Digest()
		p := s.Profile
		return tc.Invitation{Version: "3", Route: &protocol.EnrollmentRoute{Profile: d}, Profile: &p}
	}
	if err := profileMismatch(local, inv(local)); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		s      network.ProfileSelection
		code   string
		action string
	}{
		{mismatchProfile(t, a, "Hosted", 3), "PROFILE_EPOCH_MISMATCH", "this device"},
		{mismatchProfile(t, a, "Hosted", 1), "PROFILE_EPOCH_MISMATCH", "inviting device"},
		{mismatchProfile(t, b, "Home server", 2), "PROFILE_OPERATOR_MISMATCH", "same operator"},
		{mismatchProfile(t, a, "Hosted", 2), "PROFILE_MISMATCH", "do not bypass"},
	}
	for _, c := range cases {
		var ce *ControlError
		if err := profileMismatch(local, inv(c.s)); !errors.As(err, &ce) || ce.Code != c.code || !strings.Contains(ce.Action, c.action) {
			t.Fatal(c.code, err)
		}
	}
}
