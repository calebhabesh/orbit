package terminal

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

func ready() *tc.Readiness {
	return &tc.Readiness{Approved: true, MembershipCurrent: true, RootAvailable: true, ScanComplete: true}
}

// E12: a completed setup shows a success card; Enter lands on the Overview
// with the new Orbit selected and a one-time toast.
func TestOnboardingE12SuccessCardThenOverview(t *testing.T) {
	m, _ := workflowModel()
	m.width, m.height = 120, 40
	m.flow = &workflow{screen: "progress", kind: "setup", plan: tc.SetupIntent{FolderName: "Synced", DeviceName: "Laptop", Root: "/data/Synced"},
		result: tc.Result{Operation: &tc.Operation{ID: "op", State: "completed"}, Readiness: ready()}}
	v := m.View().Content
	for _, s := range []string{"✓ Synced is ready on Laptop", "Enter Go to Overview", "a Invite a device"} {
		if !strings.Contains(v, s) {
			t.Fatalf("missing %q:\n%s", s, v)
		}
	}
	if strings.Contains(v, "Operation: op") || strings.Contains(v, "Approved=") {
		t.Fatalf("details shown without d:\n%s", v)
	}
	press(m, "enter")
	if m.flow != nil || m.section != 0 || m.toast != "✓ Synced is ready" || m.focusRoot != "/data/Synced" {
		t.Fatalf("Enter did not land on the Overview: flow=%v section=%d toast=%q root=%q", m.flow != nil, m.section, m.toast, m.focusRoot)
	}
	m.result = tc.Result{Items: []tc.NamedItem{{ID: "other", Name: "Other", Root: "/data/Other"}, {ID: "new", Name: "Synced", Root: "/data/Synced"}}}
	m.restoreSelection()
	if m.selectedKey != "new" || m.focusRoot != "" {
		t.Fatalf("new Orbit not selected: %q", m.selectedKey)
	}
	if !strings.Contains(strings.Join(m.statusLines(true), "\n"), "✓ Synced is ready") {
		t.Fatal("toast not shown")
	}
	press(m, "j")
	if m.toast != "" {
		t.Fatal("toast survived a key press")
	}
}

// E12: completion without readiness is "set up", never "ready", and says what
// is still pending; a join says it joined.
func TestOnboardingE12SetUpIsNotReady(t *testing.T) {
	m, _ := workflowModel()
	m.width, m.height = 120, 40
	rd := ready()
	rd.MissingContent = 3
	m.flow = &workflow{screen: "progress", kind: "join", plan: tc.SetupIntent{FolderName: "Synced", DeviceName: "Pi"},
		invitation: tc.Invitation{InviterName: "Desk"},
		result:     tc.Result{Operation: &tc.Operation{State: "completed", Kind: "join"}, Readiness: rd}}
	v := m.View().Content
	if !strings.Contains(v, "Joined! Synced is set up on Pi") || strings.Contains(v, "is ready") || !strings.Contains(v, "3 files to download") || !strings.Contains(v, "Syncs with:") {
		t.Fatalf("unexpected card:\n%s", v)
	}
}

func TestOnboardingE12SetupToastRequiresReadiness(t *testing.T) {
	pending := ready()
	pending.MissingContent = 1
	for _, test := range []struct {
		name      string
		readiness *tc.Readiness
		want      string
	}{{"unknown", nil, "✓ Synced is set up"}, {"pending", pending, "✓ Synced is set up"}, {"ready", ready(), "✓ Synced is ready"}} {
		t.Run(test.name, func(t *testing.T) {
			m, _ := workflowModel()
			m.flow = &workflow{screen: "progress", kind: "setup", plan: tc.SetupIntent{FolderName: "Synced"}, result: tc.Result{Operation: &tc.Operation{State: "completed"}, Readiness: test.readiness}}
			press(m, "enter")
			if m.toast != test.want {
				t.Fatalf("toast = %q, want %q", m.toast, test.want)
			}
		})
	}
}

func TestOnboardingE12RequestBadge(t *testing.T) {
	m := newModel(t.Context(), &workflowFixture{}, Options{})
	m.result = tc.Result{Attention: []tc.Attention{{ID: "a", Code: "AWAITING_APPROVAL"}, {ID: "a", Code: "AWAITING_APPROVAL"}, {ID: "b", Code: "AWAITING_APPROVAL"}}}
	if b := m.requestBadge(m.theme()); !strings.Contains(b, "2 join requests") {
		t.Fatalf("badge %q", b)
	}
	for _, plain := range []bool{false, true} {
		m.opts.Colorless = plain
		m.width, m.height = 100, 30
		m.flow = &workflow{screen: "leave_review", folder: "f", result: tc.Result{FolderManagement: &tc.FolderManagement{Name: "Synced", Root: "/kept"}}}
		if v := m.View().Content; !strings.Contains(v, "2 join requests") {
			t.Fatalf("badge missing from workflow (plain=%v): %s", plain, v)
		}
		m.flow = nil
		m.width = 40
		if v := m.View().Content; !strings.Contains(v, "2 join requests") {
			t.Fatalf("badge missing from compact view (plain=%v): %s", plain, v)
		}
	}
	m.result.Attention = nil
	if b := m.requestBadge(m.theme()); b != "" {
		t.Fatalf("badge without requests: %q", b)
	}
}

func TestOnboardingE12RequestBadgeAcrossViewsAndIdleForms(t *testing.T) {
	m, w := workflowModel()
	m.firstLoad = true
	count := 2
	w.query = func(_ context.Context, q tc.Query) (tc.Result, error) {
		current := tc.Uint(count)
		return tc.Result{JoinRequestCount: &current}, nil
	}
	for section := range sections {
		runReply(m, m.changeSection(section))
		if badge := m.requestBadge(m.theme()); !strings.Contains(badge, "2 join requests") {
			t.Fatalf("%s lost global badge: %q", sections[section], badge)
		}
	}
	m.flow = &workflow{screen: "form", fields: []field{newField("Device name", "unsaved draft", false)}, err: "retained validation", notice: "retained notice"}
	count = 3
	if cmd := m.startQuery(); cmd != nil {
		t.Fatal("opening an idle form started a badge query")
	}
	_, cmd := m.Update(refreshMsg{})
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("refresh tick did not schedule badge and next tick")
	}
	runReply(m, batch[0])
	if badge := m.requestBadge(m.theme()); !strings.Contains(badge, "3 join requests") {
		t.Fatalf("idle form did not refresh badge: %q", badge)
	}
	if m.flow.fields[0].input.Value() != "unsaved draft" || m.flow.err != "retained validation" || m.flow.notice != "retained notice" || m.flow.busy {
		t.Fatal("badge refresh changed a form draft or workflow state")
	}
	m.flow = &workflow{screen: "invitation_out", invitation: tc.Invitation{Folder: "folder"}}
	count = 1
	runReply(m, m.startQuery())
	if badge := m.requestBadge(m.theme()); !strings.Contains(badge, "1 join request") {
		t.Fatalf("invitation polling did not refresh badge: %q", badge)
	}
	count = 0
	runReply(m, m.startQuery())
	if badge := m.requestBadge(m.theme()); badge != "" {
		t.Fatalf("approved requests left a stale badge: %q", badge)
	}
}

func TestOnboardingE12ActionWaitsForCanceledBadgeReply(t *testing.T) {
	for _, action := range []string{"retry", "rename"} {
		t.Run(action, func(t *testing.T) {
			m, w := workflowModel()
			switch action {
			case "retry":
				m.flow = &workflow{screen: "retry_review", operation: "reviewed-task"}
			case "rename":
				m.flow = &workflow{screen: "rename_form", task: "folder", folder: "reviewed-folder", fields: []field{newField("Orbit name", "Projects", false)}}
			}
			w.query = func(ctx context.Context, q tc.Query) (tc.Result, error) {
				if q.Kind != "capabilities" {
					t.Fatalf("badge queried %q", q.Kind)
				}
				return tc.Result{}, ctx.Err()
			}
			badge := m.startFlowQuery(true)
			if badge == nil {
				t.Fatal("refresh did not start a badge query")
			}
			if cmd := keyCode(m, tea.KeyEnter); cmd != nil {
				t.Fatal("action bypassed the serialized query lane")
			}
			cmd := runReply(m, badge)
			if cmd == nil {
				t.Fatal("canceled badge reply lost the confirmed action")
			}
			runReply(m, cmd)
			if action == "retry" && (len(w.retried) != 1 || w.retried[0].TaskID != "reviewed-task") {
				t.Fatalf("retry = %+v", w.retried)
			}
			if action == "rename" && (len(w.renamed) != 1 || w.renamed[0] != "folder reviewed-folder Projects") {
				t.Fatalf("rename = %+v", w.renamed)
			}
		})
	}
}
