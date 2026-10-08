package terminal

import (
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/network"
	"strings"
	"testing"
)

func TestOnboardingE06NegotiatesShortCodeAndShowsFailure(t *testing.T) {
	for _, capable := range []bool{false, true} {
		m, w := workflowModel()
		m.flow = &workflow{screen: "invite_review", folder: e08Folder, result: tc.Result{FolderManagement: &tc.FolderManagement{MembershipDigest: strings.Repeat("a", 64)}}}
		if capable {
			m.flow.result.Capabilities = []string{tc.ShortPairingCapability}
		}
		runReply(m, m.makeInvitation())
		if w.mutation.Invite == nil || w.mutation.Invite.ShortCode != capable {
			t.Fatalf("capability %v: %+v", capable, w.mutation)
		}
	}
	inv, _ := e00Invitation()
	for _, state := range []string{"waiting", "sent", "failed", "expired"} {
		m, _ := workflowModel()
		m.width, m.height = 100, 30
		m.flow = &workflow{screen: "invitation_out", invitation: inv, result: tc.Result{Pairing: &network.PairingStatus{Code: "ABCD-2345", State: state, Expires: "2026-10-08T16:00:00Z"}}}
		if state == "failed" || state == "expired" {
			m.flow.result.Pairing.Error = "PAIRING_UNAVAILABLE"
		}
		view := m.View().Content
		for _, want := range []string{"ABCD-2345", state, "Owner approval", "v long invitation"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%s missing %q: %s", state, want, view)
			}
		}
		if m.flow.result.Pairing.Error != "" && !strings.Contains(view, "Press r for a new code") {
			t.Fatal("no recovery action")
		}
	}
}
