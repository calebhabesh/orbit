package terminalcontract

import (
	"errors"
	"strings"
	"testing"

	"github.com/calebhabesh/orbit/internal/protocol"
)

// F08: the joiner's proposed connection for each invitation kind.
func TestOnboardingE04JoinPolicy(t *testing.T) {
	packaged := &BuiltinProfile{Digest: strings.Repeat("d", 64)}
	manual := NetworkPolicy{Mode: "manual", Generation: 3}
	routed := func(profile string) Invitation {
		return Invitation{Version: "3", Route: &protocol.EnrollmentRoute{Profile: profile}}
	}
	for _, c := range []struct {
		name    string
		inv     Invitation
		builtin *BuiltinProfile
		fresh   bool
		mode    string
		profile string
		nextGen bool
	}{
		{"routed packaged", routed(packaged.Digest), packaged, false, "automatic", packaged.Digest, true},
		{"routed self-hosted", routed(strings.Repeat("e", 64)), packaged, false, "self_hosted", strings.Repeat("e", 64), true},
		{"direct fresh", Invitation{Version: "2"}, packaged, true, "automatic", packaged.Digest, true},
		{"direct with folders", Invitation{Version: "2"}, packaged, false, "manual", "", false},
		{"direct fresh no packaged profile", Invitation{Version: "2"}, nil, true, "manual", "", false},
		{"routed packaged but expired", routed(packaged.Digest), &BuiltinProfile{Digest: packaged.Digest, Expired: true}, true, "self_hosted", packaged.Digest, true},
	} {
		p := JoinPolicy(c.inv, manual, c.builtin, c.fresh)
		if p.Mode != c.mode || p.Profile != c.profile || (p.Generation == 4) != c.nextGen {
			t.Errorf("%s: %+v", c.name, p)
		}
	}
}

// F07: each pasted-code failure class is specific; none is INVALID_REQUEST.
func TestOnboardingE04InvitationErrorClasses(t *testing.T) {
	code, err := InvitationCode(Invitation{Version: "2", Folder: strings.Repeat("f", 64)})
	if err != nil {
		t.Fatal(err)
	}
	for _, cut := range []int{len(code) - 1, len(code) - 3, len(code) / 2, len(InvitationCodeV2) + 2} {
		if _, _, err := DecodeInvitationText([]byte(code[:cut])); !errors.Is(err, ErrIncompleteInvitation) {
			t.Errorf("cut %d: %v", cut, err)
		}
	}
	if _, _, err := DecodeInvitationText([]byte("orbit-invitation:v9:AAAA")); !errors.Is(err, ErrNewerInvitation) {
		t.Errorf("newer version: %v", err)
	}
	if err := ShapeError(errors.New("INVALID_REQUEST: routed invitation")); !errors.Is(err, ErrIncompleteInvitation) {
		t.Errorf("shape: %v", err)
	}
	if err := ShapeError(errors.New("IDENTITY_MISMATCH")); err == nil || err.Error() != "IDENTITY_MISMATCH" {
		t.Errorf("identity passes through: %v", err)
	}
}
