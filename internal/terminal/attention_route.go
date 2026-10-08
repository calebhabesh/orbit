package terminal

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

// attentionRoutes decides what Enter opens for each attention code the daemon
// emits (F09). Routing is by code first; an item's OperationID is only data
// for the chosen screen, so an approval request ID can never open a generic
// operation view. Every code in control.AttentionCodes must appear here.
var attentionRoutes = map[string]string{
	"AWAITING_APPROVAL":   "requests",
	"EXHAUSTED_WORK":      "retry",
	"CONFLICT":            "review",
	"STRUCTURAL_CONFLICT": "review",
	"EDITOR_RECOVERY":     "session",
	"INCOMPLETE_SETUP":    "progress",
	"ROOT_UNAVAILABLE":    "folder",
	"STALE_ROOT":          "folder",
	"FOLDER_PAUSED":       "folder",
	"BLOCKED_PATH":        "folder",
	"MEMBERSHIP_FORK":     "folder",
	"OFFLINE":             "folder",
	"DISK_BUDGET":         "storage",
	"METADATA_BUDGET":     "storage",
}

// openAttention opens the screen for one attention item. Unknown codes show
// the item's details instead of guessing at an operation.
func (m *model) openAttention(a tc.Attention) tea.Cmd {
	switch attentionRoutes[a.Code] {
	case "requests":
		return m.openFlow(&workflow{screen: "requests", folder: a.Folder})
	case "retry":
		return m.openFlow(&workflow{screen: "retry_review", operation: a.OperationID, folder: a.Folder})
	case "review":
		return m.everyday(a.Folder, "load_review", a.Path)
	case "session":
		return m.openFlow(&workflow{screen: "day_load_session", operation: a.ID, folder: a.Folder, daily: &dailyState{path: a.Path, limit: 1 << 20}})
	case "progress":
		return m.openFlow(&workflow{screen: "progress", operation: a.OperationID})
	case "storage":
		return m.everyday(a.Folder, "storage", "")
	case "folder":
		if a.Folder != "" {
			return m.openFlow(&workflow{screen: "folder", folder: a.Folder})
		}
	}
	return nil
}

// retryWork re-queues the reviewed exhausted task through control (F03).
func (m *model) retryWork() tea.Cmd {
	f := m.flow
	w, _ := m.workflows()
	task := f.operation
	f.task = "retry_work"
	f.work = func(ctx context.Context) (tc.Result, error) {
		_, err := w.RetryWork(ctx, control.WorkRetryRequest{TaskID: task})
		return tc.Result{}, err
	}
	return m.invalidate()
}
