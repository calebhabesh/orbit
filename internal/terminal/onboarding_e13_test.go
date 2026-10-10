package terminal

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

func TestOnboardingE13LeaveRequiresConfirmationAndKeepsRootInRequest(t *testing.T) {
	m, w := workflowModel()
	m.width, m.height = 100, 28
	folder := strings.Repeat("f", 64)
	m.flow = &workflow{screen: "folder", folder: folder, folderName: "Synced", folderRoot: "/data/Synced", result: tc.Result{FolderManagement: &tc.FolderManagement{Root: "/data/Synced"}}}
	press(m, "L")
	if len(w.left) != 0 || m.flow.screen != "leave_review" {
		t.Fatal("Leave ran before confirmation")
	}
	view := m.View().Content
	for _, text := range []string{"Leave Orbit", "Files stay in", "stop sending and receiving", "Unsynced edits"} {
		if !strings.Contains(view, text) {
			t.Fatalf("missing %q: %s", text, view)
		}
	}
	cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("missing Leave operation")
	}
	runReply(m, cmd)
	if len(w.left) != 1 || w.left[0].ExpectedRoot != "/data/Synced" || m.flow != nil || !strings.Contains(m.notice, "files kept") {
		t.Fatal("Leave result", w.left, m.notice)
	}
}

func TestOnboardingE13RemovalNameAndExactRetry(t *testing.T) {
	m, w := workflowModel()
	m.width, m.height = 80, 28
	m.flow = &workflow{screen: "remove_form", folder: strings.Repeat("f", 64), device: strings.Repeat("d", 64), folderName: "Synced", fields: []field{newField("Type the device name", "Wrong", false)}, result: tc.Result{Retirement: &tc.RetirementReview{DeviceName: "Laptop", ReceivedChanges: 17, MembershipDigest: strings.Repeat("a", 64), SnapshotDigest: strings.Repeat("b", 64)}}}
	press(m, "enter")
	if m.flow.work != nil || len(w.removed) != 0 {
		t.Fatal("wrong name admitted removal")
	}
	m.flow.fields[0].input.SetValue("")
	m.Update(tea.PasteMsg{Content: "Laptop"})
	view := m.View().Content
	for _, text := range []string{"17 recorded changes received here", "Total on that device: unknown", "Unseen edits", "Files stay"} {
		if !strings.Contains(view, text) {
			t.Fatal("review missing", text, view)
		}
	}
	m.flow.err = ""
	cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("missing removal operation")
	}
	runReply(m, cmd)
	if m.flow.screen != "remove_result" || len(w.removed) != 1 {
		t.Fatal("missing result")
	}
	cmd = press(m, "r")
	if cmd == nil {
		t.Fatal("missing retry")
	}
	runReply(m, cmd)
	if len(w.removed) != 2 || w.removed[0] != w.removed[1] {
		t.Fatal("retry changed reviewed request")
	}
}

func TestOnboardingE13RemovalPickerArrowsAndRemovedScreen(t *testing.T) {
	m, _ := workflowModel()
	m.width, m.height = 100, 28
	m.flow = &workflow{screen: "folder", folderName: "Synced", folderRoot: "/data/Synced", result: tc.Result{FolderManagement: &tc.FolderManagement{LocalDevice: "self", Members: []tc.NamedItem{{ID: "self", Name: "PC"}, {ID: "b", Name: "Laptop"}, {ID: "c", Name: "Pi"}}}}}
	press(m, "X")
	if len(m.flow.items) != 2 {
		t.Fatal("local device offered for removal")
	}
	press(m, "down")
	if m.flow.selected != 1 {
		t.Fatal("arrow did not select Pi")
	}
	m.flow.screen = "removed"
	m.flow.result.FolderManagement.RemovedBy = "CalebPC"
	view := m.View().Content
	if !strings.Contains(view, "removed by CalebPC") || !strings.Contains(view, "files stay") {
		t.Fatal("removed screen", view)
	}
	m.flow.result.FolderManagement.RemovedBy = ""
	m.flow.result.FolderManagement.RemovalReporter = "Pi"
	view = m.View().Content
	if !strings.Contains(view, "Reported by Pi") || strings.Contains(view, "removed by Pi") {
		t.Fatal("reporter invented as removal actor", view)
	}
	press(m, "enter")
	if m.flow.screen != "leave_review" {
		t.Fatal("removed screen cannot leave")
	}
}
