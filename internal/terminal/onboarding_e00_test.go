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
