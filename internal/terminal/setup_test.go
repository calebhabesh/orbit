package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/charmbracelet/x/ansi"
)

type workflowFixture struct {
	query    func(context.Context, tc.Query) (tc.Result, error)
	mutation tc.Mutation
	calls    int
	retried  []control.WorkRetryRequest
}

func (w *workflowFixture) Query(ctx context.Context, q tc.Query) (tc.Result, error) {
	if w.query != nil {
		return w.query(ctx, q)
	}
	return tc.Result{}, nil
}
func (w *workflowFixture) Mutate(_ context.Context, m tc.Mutation) (tc.Result, error) {
	w.calls++
	w.mutation = m
	return tc.Result{Operation: &tc.Operation{ID: m.OperationID, State: "completed"}}, nil
}
func (w *workflowFixture) Setup(ctx context.Context, m tc.Mutation) (tc.Result, error) {
	return w.Mutate(ctx, m)
}
func (*workflowFixture) ManageFolder(context.Context, string, string, string, string) error {
	return nil
}
func (*workflowFixture) SaveInvitation(context.Context, string, tc.Invitation) error { return nil }
func (w *workflowFixture) RetryWork(_ context.Context, req control.WorkRetryRequest) (*control.WorkRetryResult, error) {
	w.retried = append(w.retried, req)
	return &control.WorkRetryResult{RetriedCount: 1}, nil
}
func (*workflowFixture) RetirementPreview(context.Context, string, string) (control.RetireDevicePreviewResult, error) {
	return control.RetireDevicePreviewResult{}, nil
}
func workflowModel() (*model, *workflowFixture) {
	w := &workflowFixture{}
	return newModel(context.Background(), w, Options{Colorless: true}), w
}
func runReply(m *model, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	_, next := m.Update(cmd())
	return next
}
func keyCode(m *model, code rune) tea.Cmd { return m.key(tea.KeyPressMsg{Code: code}) }
func loadedForm(t *testing.T) (*model, *workflowFixture) {
	t.Helper()
	m, w := workflowModel()
	w.query = func(_ context.Context, q tc.Query) (tc.Result, error) {
		s := config.DefaultRuntimeSettings()
		return tc.Result{Settings: &s}, nil
	}
	runReply(m, m.setupForm("adopt"))
	if m.flow.screen != "form" {
		t.Fatal("setup form not loaded")
	}
	m.flow.fields[2].input.SetValue("/private/root")
	return m, w
}
func TestTerminalT10FormKeyboardReviewAndErrors(t *testing.T) {
	m, w := loadedForm(t)
	f := m.flow
	m.focusField(0)
	f.fields[0].input.SetValue("")
	for _, key := range []string{"j", "k", "q", "?"} {
		press(m, key)
	}
	if f.fields[0].input.Value() != "jkq?" || m.quitting {
		t.Fatal("shortcut consumed focused text")
	}
	m.Update(tea.PasteMsg{Content: strings.Repeat("x", 17000)})
	if len(f.fields[0].input.Value()) > 256 || f.err == "" {
		t.Fatal("paste admission")
	}
	f.fields[4].input.SetValue("0")
	keyCode(m, tea.KeyEnter)
	if f.screen != "form" || f.err == "" || w.calls != 0 {
		t.Fatal("invalid budgets submitted")
	}
	f.fields[4].input.SetValue("4000000000")
	w.query = func(_ context.Context, q tc.Query) (tc.Result, error) {
		if q.Kind != "root_preview" || q.RootPlan.DeviceName != "jkq?" {
			t.Fatal("wrong plan")
		}
		return tc.Result{}, &control.ControlError{Code: "INVALID_ROOT", Message: "SECRET"}
	}
	runReply(m, keyCode(m, tea.KeyEnter))
	if f.screen != "form" || f.fields[0].input.Value() != "jkq?" || strings.Contains(m.View().Content, "SECRET") {
		t.Fatal("failed preview lost draft or exposed error")
	}
	review := tc.Review{Token: strings.Repeat("a", 64), Generation: strings.Repeat("b", 64), ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)}
	w.query = func(context.Context, tc.Query) (tc.Result, error) {
		return tc.Result{Review: &review, Preview: &tc.RootPreview{Complete: true, Files: 3, Bytes: 123, CapacityKnown: true}}, nil
	}
	runReply(m, keyCode(m, tea.KeyEnter))
	if f.screen != "review" || f.plan.Preview != review || w.calls != 0 {
		t.Fatal("preview auto submitted")
	}
	keyCode(m, tea.KeyEscape)
	if f.screen != "form" || f.fields[0].input.Value() != "jkq?" {
		t.Fatal("back/edit draft")
	}
	runReply(m, keyCode(m, tea.KeyEnter))
	cmd := keyCode(m, tea.KeyEnter)
	if !f.busy {
		t.Fatal("no mutation admission feedback")
	}
	if keyCode(m, tea.KeyEnter) != nil {
		t.Fatal("double submission")
	}
	runReply(m, cmd)
	if w.calls != 1 || w.mutation.Setup.Preview != review || f.screen != "progress" {
		t.Fatal("not exact reviewed setup")
	}
}
func TestTerminalT10ExactApprovalAndScopedSharing(t *testing.T) {
	m, w := workflowModel()
	p := tc.EnrollmentRequest{ID: strings.Repeat("1", 64), Folder: strings.Repeat("2", 64), Requester: strings.Repeat("3", 64), KeyPin: strings.Repeat("4", 64), TranscriptDigest: strings.Repeat("5", 64), ExpectedMembership: strings.Repeat("6", 64), State: "pending_approval", VerificationCode: "owner-code"}
	m.flow = &workflow{screen: "approval", request: p}
	cmd := m.decideRequest(true)
	id := m.flow.mutation.OperationID
	runReply(m, cmd)
	m.flow.screen = "approval"
	m.flow.err = "STALE_VIEW: obtain fresh review"
	m.acceptFlow("approval", tc.Result{Requests: []tc.EnrollmentRequest{{ID: "different"}}}, nil)
	if m.flow.request.ID != p.ID || m.flow.err == "" {
		t.Fatal("poll replaced exact review or erased its error")
	}
	if w.mutation.Approval.Request != p.ID || w.mutation.Approval.TranscriptDigest != p.TranscriptDigest || w.mutation.Approval.ExpectedMembership != p.ExpectedMembership {
		t.Fatal("approval scope changed")
	}
	m.flow.screen = "approval"
	runReply(m, m.decideRequest(true))
	if w.mutation.OperationID != id {
		t.Fatal("lost-response retry allocated new ID")
	}
	m.flow = &workflow{screen: "invite_review", folder: p.Folder, device: p.Requester, result: tc.Result{FolderManagement: &tc.FolderManagement{MembershipDigest: p.ExpectedMembership}}}
	cmd = m.makeInvitation()
	id = m.flow.mutation.OperationID
	runReply(m, cmd)
	if w.mutation.Kind != "share" || w.mutation.Invite.Device != p.Requester || w.mutation.Invite.Folder != p.Folder {
		t.Fatal("sharing did not bind known device/folder")
	}
	runReply(m, m.makeInvitation())
	if w.mutation.OperationID != id {
		t.Fatal("invitation retry changed identity")
	}
}
func TestTerminalT10LateRepliesAndResumeReadiness(t *testing.T) {
	m, w := workflowModel()
	w.query = func(context.Context, tc.Query) (tc.Result, error) {
		return tc.Result{Operation: &tc.Operation{State: "running", Phase: "awaiting_approval"}, Readiness: &tc.Readiness{}}, nil
	}
	cmd := m.openFlow(&workflow{screen: "progress", operation: strings.Repeat("a", 64)})
	old := cmd()
	m.closeFlow()
	m.Update(old)
	if m.flow != nil {
		t.Fatal("late progress resurrected abandoned flow")
	}
	m.flow = &workflow{screen: "progress", result: tc.Result{Operation: &tc.Operation{State: "running", Phase: "awaiting_approval"}, Readiness: &tc.Readiness{Approved: true, MembershipCurrent: false}}}
	view := m.View().Content
	if !strings.Contains(view, "Waiting for approval") || strings.Contains(view, "Locally ready") || !strings.Contains(view, "readiness incomplete") {
		t.Fatal("invented readiness")
	}
	// Reopening uses the actual operation identity from bounded discovery.
	m.flow = nil
	m.pending = 7
	m.firstLoad = false
	m.Update(queryReply{request: 7, generation: m.generation, result: tc.Result{}, setups: []tc.NamedItem{{ID: strings.Repeat("b", 64)}}})
	if m.flow == nil || m.flow.operation != strings.Repeat("b", 64) {
		t.Fatal("unfinished setup not resumed")
	}
}
func TestTerminalT10WrappedInvitationPaste(t *testing.T) {
	m, _ := workflowModel()
	m.flow = &workflow{screen: "invitation", fields: []field{newField("Private invitation", "", true)}}
	m.Update(tea.PasteMsg{Content: "orbit-invitation:v2:abc\r\ndef\n"})
	if m.flow.fields[0].input.Value() != "orbit-invitation:v2:abcdef" {
		t.Fatal("wrapped invitation was corrupted on paste")
	}
	if strings.Contains(m.View().Content, "abcdef") {
		t.Fatal("private paste exposed")
	}
}

func TestTerminalT10PrivateNarrowViewsAndErrorActions(t *testing.T) {
	m, _ := loadedForm(t)
	for _, screen := range []string{"form", "review", "progress", "approval", "invite_review", "invitation_out", "folder", "unregister_preview"} {
		m.flow.screen = screen
		m.flow.invitation.Capability = "PRIVATE-CAPABILITY"
		m.flow.err = workflowError(tc.Result{}, errors.New("https://SECRET:token@endpoint"))
		for _, size := range [][2]int{{80, 24}, {40, 16}, {20, 10}} {
			m.width, m.height = size[0], size[1]
			v := m.View().Content
			if strings.Contains(v, "PRIVATE-CAPABILITY") || strings.Contains(v, "SECRET") {
				t.Fatal("private/error data leaked")
			}
			ls := strings.Split(v, "\n")
			if len(ls) > m.height {
				t.Fatal("height overflow")
			}
			for _, line := range ls {
				if ansi.StringWidth(line) > m.width {
					t.Fatal("width overflow")
				}
			}
		}
	}
	for _, code := range []string{"IDENTITY_MISMATCH", "EXPIRED_ATTEMPT", "STORAGE_BLOCKED", "STALE_VIEW", "MEMBERSHIP_FORK", "UNATTENDED_PREREQUISITE"} {
		s := workflowError(tc.Result{Error: &tc.Error{Code: code}}, nil)
		if !strings.Contains(s, code) || len(s) < len(code)+20 {
			t.Fatal("missing corrective action")
		}
	}
}
