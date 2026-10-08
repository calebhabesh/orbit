package terminal

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/protocol"
)

// F07: an incomplete (truncated) code reports that it is incomplete, not a
// generic or INVALID_REQUEST failure.
func TestOnboardingE04F07TruncatedInvitationIsSpecific(t *testing.T) {
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
func TestOnboardingE04F08JoinDefaultsToAutomatic(t *testing.T) {
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

// F08: a routed invitation proposes the inviter's operator in the join form:
// Automatic for the packaged profile, self-hosted otherwise; the review then
// names the operator and its privacy text.
func TestOnboardingE04F08RoutedInvitationProposesInviterOperator(t *testing.T) {
	packaged := strings.Repeat("d", 64)
	for _, c := range []struct {
		route, mode string
	}{{packaged, "automatic"}, {strings.Repeat("e", 64), "self_hosted"}} {
		m, w := workflowModel()
		w.query = func(_ context.Context, q tc.Query) (tc.Result, error) {
			s := config.DefaultRuntimeSettings()
			if q.Kind == "network_status" {
				return tc.Result{Network: &tc.NetworkStatus{Policy: tc.NetworkPolicy{Mode: "manual", Generation: 4},
					Builtin: &tc.BuiltinProfile{Digest: packaged, Operator: "Orbit"}}}, nil
			}
			return tc.Result{Settings: &s}, nil
		}
		runReply(m, m.setupForm("join"))
		inv := tc.Invitation{Version: "3", Route: &protocol.EnrollmentRoute{Profile: c.route},
			Profile: &protocol.NetworkProfile{Operator: "Fixture operator", Privacy: "fixture privacy"}}
		m.flow.invitation = inv
		m.acceptFlow("parse_invitation", tc.Result{Invitation: &inv}, nil)
		if got := m.flow.fields[13].input.Value(); got != c.mode {
			t.Fatalf("route %s: connection %q, want %q", c.route[:4], got, c.mode)
		}
		if p := m.flow.joinPolicy; p == nil || p.Profile != c.route || p.Generation != 5 {
			t.Fatalf("join policy %+v", p)
		}
	}
}
