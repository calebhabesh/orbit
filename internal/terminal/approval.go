package terminal

import (
	"context"

	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

func (m *model) requestsKey(k string) tea.Cmd {
	f := m.flow
	switch k {
	case "j", "down":
		f.selected = min(f.selected+1, max(0, len(f.result.Requests)-1))
	case "k", "up":
		f.selected = max(0, f.selected-1)
	case "]":
		if f.result.Cursor != "" {
			f.cursor = f.result.Cursor
			f.selected = 0
			return m.invalidate()
		}
	case "[":
		f.cursor = ""
		f.selected = 0
		return m.invalidate()
	case "r":
		return m.startQuery()
	case "enter":
		if len(f.result.Requests) > 0 {
			f.request = f.result.Requests[f.selected]
			f.screen = "approval"
			f.err = ""
			f.mutation = tc.Mutation{}
			return m.invalidate()
		}
	}
	return nil
}
func (m *model) decideRequest(approve bool) tea.Cmd {
	f := m.flow
	w, _ := m.workflows()
	p := f.request
	if p.State != "pending_approval" {
		f.err = "Request is not pending; obtain a fresh invitation/review."
		return nil
	}
	decision := "decline"
	if approve {
		decision = "approve"
	}
	if f.mutation.OperationID == "" || f.mutation.Approval.Decision != decision {
		id, err := newID()
		if err != nil {
			f.err = "Could not allocate operation identity."
			return nil
		}
		f.mutation = tc.Mutation{Version: tc.Version, OperationID: id, Kind: "approval", Approval: &tc.ApprovalIntent{Request: p.ID, Folder: p.Folder, Requester: p.Requester, KeyPin: p.KeyPin, TranscriptDigest: p.TranscriptDigest, ExpectedMembership: p.ExpectedMembership, Decision: decision}}
	}
	mutation := f.mutation
	f.task = decision
	f.work = func(ctx context.Context) (tc.Result, error) { return w.Mutate(ctx, mutation) }
	return m.invalidate()
}
