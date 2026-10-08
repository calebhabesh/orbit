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

// F06: a paste into the secret invitation field replaces its content and
// reports how much was received without revealing it.
func TestOnboardingE00F06SecretPasteReplacesAndReportsLength(t *testing.T) {
	onboardingBaseline(t)
	_, code := e00Invitation()
	m, _ := workflowModel()
	m.flow = &workflow{screen: "invitation", fields: []field{newField("Private invitation", "", true)}}
	m.Update(tea.PasteMsg{Content: code[:800]}) // first, incomplete attempt
	m.Update(tea.PasteMsg{Content: code})       // retry with the whole code
	got := m.flow.fields[0].input.Value()
	if got != code {
		t.Errorf("second paste produced %d characters, want exactly the pasted %d (approved: paste replaces)", len(got), len(code))
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "characters") {
		t.Error("secret field shows no received-length status line")
	}
	if strings.Contains(view, code[20:60]) {
		t.Fatal("secret revealed")
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

// F09: Enter on an AWAITING_APPROVAL attention item opens the request review,
// even though the item carries the request ID in OperationID.
func TestOnboardingE00F09AwaitingApprovalOpensRequests(t *testing.T) {
	onboardingBaseline(t)
	m, _ := workflowModel()
	folder, request := strings.Repeat("f", 64), strings.Repeat("e", 64)
	m.section = 2
	m.result = tc.Result{Attention: []tc.Attention{{ID: "att-1", Folder: folder, OperationID: request, Code: "AWAITING_APPROVAL", Action: "Review pending enrollment request"}}}
	m.restoreSelection()
	if len(m.rows()) == 0 {
		t.Fatal("attention row missing")
	}
	m.inspect()
	if m.flow == nil {
		t.Fatal("Enter opened nothing")
	}
	if m.flow.screen != "requests" {
		t.Errorf("Enter on AWAITING_APPROVAL opened %q, want requests review", m.flow.screen)
	}
}

// F12: startup is a selector, not a typed word, and its default is not manual.
func TestOnboardingE00F12StartupIsSelector(t *testing.T) {
	onboardingBaseline(t)
	m, _ := loadedForm(t)
	f := m.flow
	label := f.fields[3].label
	t.Logf("startup field %q default %q", label, f.fields[3].input.Value())
	if strings.Contains(label, "(manual/login/unattended)") {
		t.Error("startup offered as typed enumeration")
	}
	m.focusField(3)
	before := f.fields[3].input.Value()
	press(m, "z")
	if got := f.fields[3].input.Value(); got != before {
		t.Errorf("typing changed startup to %q (approved: selector)", got)
	}
	if before == "manual" {
		t.Error("setup default startup is manual (approved: login on desktops, unattended on headless with linger guidance)")
	}
	f.fields[3].input.SetValue(before)
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if got := f.fields[3].input.Value(); got == before || (got != "manual" && got != "login" && got != "unattended") {
		t.Errorf("right arrow left startup at %q (approved: selector moves to the next choice)", got)
	}
}

// F13: no error code falls back to the shared "Retry; use orbit doctor" advice.
func TestOnboardingE00F13NoGenericErrorAdvice(t *testing.T) {
	onboardingBaseline(t)
	generic := "Retry; use orbit doctor to inspect local control/network reachability."
	codes := []string{"INVALID_REQUEST", "INTERNAL_ERROR", "IO_ERROR", "CONTROL_UNAVAILABLE", "UNAUTHORIZED",
		"INVITATION_INVALID", "PAYLOAD_TOO_LARGE", "SETUP_BLOCKED", "QUOTA_EXCEEDED", "SOMETHING_NEW"}
	var fallbacks []string
	for _, code := range codes {
		if strings.Contains(workflowError(tc.Result{Error: &tc.Error{Code: code}}, nil), generic) {
			fallbacks = append(fallbacks, code)
		}
	}
	if strings.Contains(workflowError(tc.Result{}, context.DeadlineExceeded), generic) {
		fallbacks = append(fallbacks, "transport error")
	}
	if len(fallbacks) != 0 {
		t.Errorf("%d of %d sampled failures show the generic advice: %v", len(fallbacks), len(codes)+1, fallbacks)
	}
}
