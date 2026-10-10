package terminal

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/protocol"
)

func reviewModel(kind string) *model {
	m, _ := workflowModel()
	m.width, m.height = 120, 60
	packaged := strings.Repeat("d", 64)
	m.flow = &workflow{screen: "review", kind: kind,
		builtin: &tc.BuiltinProfile{Digest: packaged, Operator: "Fixture Op", Privacy: "LONG PRIVACY STATEMENT"},
		plan: tc.SetupIntent{FolderName: "Synced", DeviceName: "Laptop", Root: "/data/Synced",
			Network: &tc.NetworkPolicy{Mode: "automatic", Profile: packaged}},
		result: tc.Result{Preview: &tc.RootPreview{Root: "/data/Synced", Missing: true, Complete: true}},
	}
	if kind == "join" {
		m.flow.network.Profile = packaged
		m.flow.invitation = tc.Invitation{InviterName: "Desk", Inviter: "abcd", KeyPin: "PINPINPIN",
			ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano),
			Route:     &protocol.EnrollmentRoute{Profile: packaged},
			Profile:   &protocol.NetworkProfile{Operator: "Fixture Op", Privacy: "LONG PRIVACY STATEMENT"}}
	}
	return m
}

// E11: the review card is short and plain; technical detail waits behind d.
func TestOnboardingE11ReviewCardHidesDetailsUntilD(t *testing.T) {
	for _, kind := range []string{"setup", "join"} {
		m := reviewModel(kind)
		v := m.View().Content
		want := map[string]string{"setup": "Review & Create", "join": "Review & Join"}[kind]
		for _, s := range []string{want, "Synced", "/data/Synced", "created when you confirm", "Fixture Op", "never sees file names", "Enter Create Orbit"[:5]} {
			if !strings.Contains(v, s) {
				t.Fatalf("%s: missing %q in\n%s", kind, s, v)
			}
		}
		for _, s := range []string{"LONG PRIVACY STATEMENT", "PINPINPIN", "Measured", "LAN advertising", "concurrency=", "adoption"} {
			if strings.Contains(v, s) {
				t.Fatalf("%s: %q shown before Details:\n%s", kind, s, v)
			}
		}
		if kind == "join" && !strings.Contains(v, "Invited by: Desk") {
			t.Fatalf("join card lacks inviter:\n%s", v)
		}
		m.key(tea.KeyPressMsg{Code: 'd', Text: "d"})
		v = m.View().Content
		for _, s := range []string{"Details", "LONG PRIVACY STATEMENT", "concurrency="} {
			if !strings.Contains(v, s) {
				t.Fatalf("%s details: missing %q", kind, s)
			}
		}
		if kind == "join" && !strings.Contains(v, "PINPINPIN") {
			t.Fatal("join details lack the key pin")
		}
	}
}

// E11 / F08: switching to the inviter's different operator shows its full
// privacy statement on the card itself.
func TestOnboardingE11OperatorSwitchShowsPrivacyInline(t *testing.T) {
	m := reviewModel("join")
	other := strings.Repeat("e", 64)
	m.flow.network.Profile = ""
	m.flow.builtin = nil
	m.flow.plan.Network.Profile = other
	m.flow.invitation.Route.Profile = other
	m.flow.invitation.Profile = &protocol.NetworkProfile{Operator: "Other Op", Privacy: "OTHER PRIVACY"}
	if v := m.View().Content; !strings.Contains(v, "OTHER PRIVACY") || !strings.Contains(v, "Other Op") {
		t.Fatalf("operator switch not explained:\n%s", v)
	}
}

func TestOnboardingE11TitlesAreTitleCase(t *testing.T) {
	for in, want := range map[string]string{"pause": "Pause", "keep_copies": "Keep Copies", "save invitation": "Save Invitation"} {
		if got := titleWord(in); got != want {
			t.Fatalf("titleWord(%q)=%q", in, got)
		}
	}
}
