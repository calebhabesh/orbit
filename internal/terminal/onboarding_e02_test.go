package terminal

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

// F09: Enter routes by attention code first, for every code the daemon emits.
// An item's OperationID (an approval request ID, a task ID) never sends it to
// the generic operation screen.
func TestOnboardingE02F09EnterRoutesEveryAttentionCode(t *testing.T) {
	want := map[string]string{
		"remove": "folder", "removal": "remove_resume_review", "requests": "requests", "retry": "retry_review", "review": "day_load_review",
		"session": "day_load_session", "progress": "progress", "folder": "folder", "storage": "day_storage",
	}
	folder, id := strings.Repeat("f", 64), strings.Repeat("e", 64)
	for _, code := range control.AttentionCodes {
		route, ok := attentionRoutes[code]
		if !ok {
			t.Errorf("attention code %s has no route", code)
			continue
		}
		m, _ := dailyModel()
		m.section = 2
		m.result = tc.Result{Attention: []tc.Attention{{ID: "att-" + code, Folder: folder, OperationID: id, Path: "notes.txt", Code: code, Action: "x"}}}
		m.restoreSelection()
		if len(m.rows()) == 0 {
			t.Fatalf("%s: attention row missing", code)
		}
		m.inspect()
		if m.flow == nil || m.flow.screen != want[route] {
			got := "nothing"
			if m.flow != nil {
				got = m.flow.screen
			}
			t.Errorf("Enter on %s opened %s, want %s", code, got, want[route])
		}
	}
	// A code with no route shows the item's details, not an operation view.
	m, _ := workflowModel()
	m.section = 2
	m.result = tc.Result{Attention: []tc.Attention{{ID: "att-new", OperationID: id, Code: "SOMETHING_NEW", Action: "x"}}}
	m.restoreSelection()
	m.inspect()
	if m.flow != nil || !m.detail {
		t.Fatalf("unknown code opened a flow")
	}
}

// F03: the retry screen re-queues the reviewed task through control.
func TestOnboardingE02F03RetryReviewCallsControl(t *testing.T) {
	m, w := workflowModel()
	task := strings.Repeat("a", 32)
	m.section = 2
	m.result = tc.Result{Attention: []tc.Attention{{ID: "att-1", OperationID: task, Code: "EXHAUSTED_WORK", Action: "scan stopped after 3 attempts (ROOT_UNAVAILABLE); press Enter to retry"}}}
	m.restoreSelection()
	m.inspect()
	if m.flow == nil || m.flow.screen != "retry_review" {
		t.Fatal("retry review not opened")
	}
	m.width, m.height = 120, 30
	if view := m.View().Content; !strings.Contains(view, "ROOT_UNAVAILABLE") || !strings.Contains(view, "Enter retry") {
		t.Fatalf("retry review lacks the failure or key:\n%s", view)
	}
	cmd := m.key(tea.KeyPressMsg{Code: tea.KeyEnter})
	for i := 0; cmd != nil && i < 5; i++ {
		cmd = runReply(m, cmd)
	}
	if len(w.retried) != 1 || w.retried[0].TaskID != task {
		t.Fatalf("retried %+v", w.retried)
	}
	if m.flow != nil || !strings.Contains(m.notice, "re-queued") {
		t.Fatalf("flow %v notice %q", m.flow, m.notice)
	}
}

// F13: no error shows the shared "Retry; use orbit doctor" advice; unknown
// codes use the daemon's own action, else say where the details are.
func TestOnboardingE02F13SpecificErrorAdvice(t *testing.T) {
	generic := "Retry; use orbit doctor"
	for _, code := range []string{"INVALID_REQUEST", "INTERNAL_ERROR", "IO_ERROR", "CONTROL_UNAVAILABLE", "UNAUTHORIZED",
		"INVITATION_INVALID", "PAYLOAD_TOO_LARGE", "SETUP_BLOCKED", "QUOTA_EXCEEDED", "SOMETHING_NEW", "MANUAL_DAEMON_RUNNING", "HANDOVER_FAILED"} {
		if got := workflowError(tc.Result{Error: &tc.Error{Code: code}}, nil); strings.Contains(got, generic) || !strings.HasPrefix(got, code+": ") {
			t.Errorf("%s: %q", code, got)
		}
	}
	if got := workflowError(tc.Result{}, context.DeadlineExceeded); strings.Contains(got, generic) || !strings.Contains(got, "did not answer") {
		t.Errorf("transport error: %q", got)
	}
	if got := workflowError(tc.Result{Error: &tc.Error{Code: "SOMETHING_NEW", Action: "do the specific thing"}}, nil); !strings.Contains(got, "do the specific thing") {
		t.Errorf("unknown code ignores the daemon's action: %q", got)
	}
	if got := workflowError(tc.Result{Error: &tc.Error{Code: "SOMETHING_NEW"}}, nil); !strings.Contains(got, "journalctl") {
		t.Errorf("unknown code without action does not say where details are: %q", got)
	}
}
