package terminal

// E00 onboarding baseline reproductions (2026-10-08 trial findings). Each test
// asserts the approved behavior, so it deliberately fails on the current
// implementation. Ordinary runs skip them; the owning E packet promotes each
// case into an ordinary passing regression when it repairs the behavior.

import (
	"os"
	"strings"
	"testing"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/charmbracelet/x/ansi"
)

func onboardingBaseline(t *testing.T) {
	t.Helper()
	if os.Getenv("ORBIT_ONBOARDING_BASELINE") != "1" {
		t.Skip("deliberately failing E00 baseline; opt in with ORBIT_ONBOARDING_BASELINE=1")
	}
}

// A synthetic invitation whose one-line code has the trial's ~1.7 KB size.
// The capability is a fixture value, not a credential.
func e00Invitation() (tc.Invitation, string) {
	inv := tc.Invitation{Version: "2", Folder: strings.Repeat("f", 64), Inviter: strings.Repeat("a", 64),
		CertificateDER: strings.Repeat("Q", 900), KeyPin: strings.Repeat("b", 64),
		EnrollmentEndpoint: "https://192.0.2.10:7001", PeerEndpoint: "https://192.0.2.10:7000",
		Capability: strings.Repeat("c", 64), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}
	code, _ := tc.InvitationCode(inv)
	return inv, code
}

// F05: the revealed invitation is shown so that it copies as one exact line.
func TestOnboardingE00F05RevealedInvitationCopiesWhole(t *testing.T) {
	onboardingBaseline(t)
	inv, code := e00Invitation()
	t.Logf("fixture code length %d", len(code))
	for _, size := range [][2]int{{80, 24}, {200, 50}} {
		m, _ := workflowModel()
		m.width, m.height = size[0], size[1]
		m.flow = &workflow{screen: "invitation_out", invitation: inv, reveal: true}
		// Collect every row reachable by scrolling, in order, without duplicates.
		var rows []string
		seen := map[int]bool{}
		for step := range 200 {
			m.flow.scroll = step
			frame := strings.Split(ansi.Strip(m.View().Content), "\n")
			key := len(strings.Join(frame, "\n")) ^ step
			if seen[key] {
				break
			}
			seen[key] = true
			rows = append(rows, frame...)
		}
		joined := strings.Join(rows, "\n")
		if strings.Contains(joined, "…") {
			t.Errorf("%dx%d: invitation screen contains an ellipsis", size[0], size[1])
		}
		copied := false
		for _, r := range rows {
			if strings.TrimSpace(r) == code {
				copied = true
			}
		}
		if !copied {
			t.Errorf("%dx%d: no rendered row copies as the exact %d-character code (approved: unboxed block that copies as one line)", size[0], size[1], len(code))
		}
		if !strings.Contains(joined, "characters") {
			t.Errorf("%dx%d: no character count shown with the invitation", size[0], size[1])
		}
	}
}
