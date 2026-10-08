package terminal

// E00 onboarding baseline reproductions (2026-10-08 trial findings). Each test
// asserts the approved behavior, so it deliberately fails on the current
// implementation. Ordinary runs skip them; the owning E packet promotes each
// case into an ordinary passing regression when it repairs the behavior.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/calebhabesh/orbit/internal/config"
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

// F07: an incomplete (truncated) code reports that it is incomplete, not a
// generic or INVALID_REQUEST failure.
func TestOnboardingE00F07TruncatedInvitationIsSpecific(t *testing.T) {
	onboardingBaseline(t)
	_, code := e00Invitation()
	for _, cut := range []int{len(code) - 1, len(code) - 2, len(code) / 2, 300} {
		m, w := workflowModel()
		m.flow = &workflow{screen: "invitation", kind: "join", fields: []field{newField("Private invitation", code[:cut], true)}}
		runReply(m, keyCode(m, tea.KeyEnter))
		got := m.flow.err
		t.Logf("cut at %d of %d: %q", cut, len(code), got)
		low := strings.ToLower(got)
		if got == "" {
			t.Errorf("cut %d: truncated code accepted", cut)
			continue
		}
		if strings.Contains(got, "INVALID_REQUEST") || strings.Contains(got, "CONTROL_UNAVAILABLE") || strings.Contains(got, "orbit doctor") {
			t.Errorf("cut %d: generic failure %q", cut, got)
		}
		if !strings.Contains(low, "incomplete") && !strings.Contains(low, "damaged") {
			t.Errorf("cut %d: message does not say the code is incomplete/damaged", cut)
		}
		if w.calls != 0 {
			t.Fatal("mutation sent for a rejected code")
		}
	}
}

// F08: a join on a device with no folders preselects Automatic connection even
// when the daemon already wrote its config (FreshInstall false), as on the
// trial laptop whose installed service had created the identity first.
func TestOnboardingE00F08JoinDefaultsToAutomatic(t *testing.T) {
	onboardingBaseline(t)
	m, w := workflowModel()
	m.opts.FreshInstall = false
	w.query = func(_ context.Context, q tc.Query) (tc.Result, error) {
		s := config.DefaultRuntimeSettings()
		switch q.Kind {
		case "settings":
			return tc.Result{Settings: &s}, nil
		case "network_status":
			return tc.Result{Network: &tc.NetworkStatus{Policy: tc.NetworkPolicy{Mode: "manual", Generation: 1},
				Builtin: &tc.BuiltinProfile{Digest: strings.Repeat("d", 64), Operator: "Orbit"}}}, nil
		}
		return tc.Result{}, nil
	}
	runReply(m, m.setupForm("join"))
	if m.flow == nil || len(m.flow.fields) < 14 {
		t.Fatal("join form not loaded")
	}
	if got := m.flow.fields[13].input.Value(); got != "automatic" {
		t.Errorf("join connection default %q, want automatic", got)
	}
}
