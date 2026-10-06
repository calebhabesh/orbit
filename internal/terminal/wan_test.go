package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
)

func TestWANW07ReviewedPrivacyAndAdvancedFocus(t *testing.T) {
	m, w := workflowModel()
	m.opts.FreshInstall = true
	w.query = func(_ context.Context, q tc.Query) (tc.Result, error) {
		if q.Kind == "settings" {
			s := config.DefaultRuntimeSettings()
			return tc.Result{Settings: &s}, nil
		}
		return tc.Result{Network: &tc.NetworkStatus{Policy: tc.NetworkPolicy{Mode: "manual", Generation: 1}}}, nil
	}
	runReply(m, m.setupForm("setup"))
	f := m.flow
	if f.fields[13].input.Value() != "automatic" {
		t.Fatal("fresh setup must review Automatic")
	}
	view := m.View().Content
	if strings.Contains(view, "Peer listen") || strings.Contains(view, "Advertised peer") {
		t.Fatal("address prompt in ordinary flow")
	}
	m.formKey(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	m.focusField(7)
	m.formKey(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if f.focus != 0 {
		t.Fatal("hidden focused advanced field")
	}
	m.formKey(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	f.fields[2].input.SetValue("/private/root")
	w.query = func(_ context.Context, q tc.Query) (tc.Result, error) {
		if q.RootPlan == nil || q.RootPlan.Network.Mode != "local_only" || q.RootPlan.Network.Generation != 2 {
			t.Fatal("privacy not bound to root review")
		}
		return tc.Result{Review: &tc.Review{Token: strings.Repeat("a", 64)}, Preview: &tc.RootPreview{Complete: true}}, nil
	}
	runReply(m, m.previewSetup())
	if w.calls != 0 {
		t.Fatal("review announced before confirm")
	}
	runReply(m, m.submitSetup())
	if w.mutation.Setup.Network.Mode != "local_only" {
		t.Fatal("confirmation lost reviewed policy")
	}
}

func TestWANW07ExistingPolicyAndExactJoinRetry(t *testing.T) {
	m, w := loadedForm(t)
	if m.flow.fields[13].input.Value() != "manual" {
		t.Fatal("manual installation opted in silently")
	}
	f := m.flow
	f.kind = "join"
	f.plan = tc.SetupIntent{Root: "/private/root", Network: &tc.NetworkPolicy{Mode: "self_hosted", Profile: strings.Repeat("a", 64), Generation: 3}}
	runReply(m, m.submitSetup())
	first := w.mutation
	f.err = "NETWORK_RESTART_REQUIRED"
	runReply(m, m.flowKey(tea.KeyPressMsg{Code: 'r'}))
	if w.mutation.OperationID != first.OperationID || w.mutation.Join.Attempt != first.Join.Attempt || w.mutation.Join.Network != first.Join.Network {
		t.Fatal("retry replaced reviewed identity/policy")
	}
}

func TestWANW07PollingPreservesSelectionAndLateContexts(t *testing.T) {
	m, w := workflowModel()
	a, b := tc.EnrollmentRequest{ID: "a"}, tc.EnrollmentRequest{ID: "b"}
	m.flow = &workflow{screen: "requests", selected: 1, result: tc.Result{Requests: []tc.EnrollmentRequest{a, b}}}
	m.acceptFlow("requests", tc.Result{Requests: []tc.EnrollmentRequest{b, a}}, nil)
	if m.flow.selected != 0 {
		t.Fatal("poll moved selected request")
	}
	w.query = func(context.Context, tc.Query) (tc.Result, error) {
		return tc.Result{Network: &tc.NetworkStatus{Code: "OLD"}}, nil
	}
	cmd := m.openFlow(&workflow{screen: "network"})
	reply := cmd()
	m.closeFlow()
	m.Update(reply)
	if m.flow != nil {
		t.Fatal("late network reply resurrected screen")
	}
}

func TestWANW07ObservedRelaySeparateFromReadiness(t *testing.T) {
	m, _ := workflowModel()
	n := &tc.NetworkStatus{Policy: tc.NetworkPolicy{Mode: "automatic"}, Ready: true, Code: "SERVICE_READY", Observations: []tc.NetworkObservation{{Device: "Pi", Route: "relay", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}}}
	m.flow = &workflow{screen: "progress", result: tc.Result{Network: n, Operation: &tc.Operation{State: "running", Phase: "awaiting_approval"}, Readiness: &tc.Readiness{}}}
	m.width, m.height = 120, 40
	v := m.View().Content
	if !strings.Contains(v, "Connected via relay") || !strings.Contains(v, "readiness incomplete") || strings.Contains(v, "Locally ready") {
		t.Fatal("service/route invented file completion", v)
	}
	m.flow.screen = "network"
	for _, size := range [][2]int{{40, 16}, {20, 10}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if len(strings.Split(m.View().Content, "\n")) > size[1] {
			t.Fatal("resize overflow")
		}
	}
	if code, _ := tc.InvitationCode(tc.Invitation{Version: "3"}); !strings.HasPrefix(code, "orbit-invitation:v3:") {
		t.Fatal("v3 transfer mislabeled")
	}
}

func TestWANW07ExpiredV3PasteRetainsInput(t *testing.T) {
	m, w := workflowModel()
	m.flow = &workflow{screen: "invitation", fields: []field{newField("Private invitation", "orbit-invitation:v3:eyJ2ZXJzaW9uIjoiMyIsImV4cGlyZXNfYXQiOiIyMDAwLTAxLTAxVDAwOjAwOjAwWiJ9", true)}}
	value := m.flow.fields[0].input.Value()
	runReply(m, m.parseInvitation())
	if !strings.Contains(m.flow.err, "INVITATION_EXPIRED") || m.flow.fields[0].input.Value() != value || w.calls != 0 {
		t.Fatal("expired fresh v3 input changed identity/input or wrong error", m.flow.err)
	}
	if strings.Contains(m.View().Content, value) {
		t.Fatal("secret input exposed")
	}
}
