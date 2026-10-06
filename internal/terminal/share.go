package terminal

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
)

func (m *model) pickerKey(k string) tea.Cmd {
	f := m.flow
	switch k {
	case "j", "down":
		f.selected = min(f.selected+1, max(0, len(f.items)-1))
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
	case "enter":
		if len(f.items) == 0 {
			return nil
		}
		it := f.items[f.selected]
		switch f.screen {
		case "setups":
			f.operation = it.ID
			f.screen = "progress"
		case "pick_folder":
			f.folder = it.ID
			switch f.kind {
			case "storage":
				return m.everyday(f.folder, "storage", "")
			case "invite":
				f.screen = "invite_review"
			case "share":
				f.screen = "pick_device"
			case "retire":
				f.screen = "retire"
			default:
				f.screen = "folder"
			}
		case "pick_device":
			f.device = it.ID
			f.screen = "invite_review"
		}
		f.cursor = ""
		f.selected = 0
		return m.invalidate()
	}
	return nil
}
func (m *model) makeInvitation() tea.Cmd {
	f := m.flow
	w, _ := m.workflows()
	info := f.result.FolderManagement
	if info == nil {
		f.err = "Membership unavailable; refresh the folder review."
		return nil
	}
	if f.mutation.OperationID == "" {
		id, err := newID()
		if err != nil {
			f.err = "Could not allocate operation identity."
			return nil
		}
		kind := "invite"
		if f.device != "" {
			kind = "share"
		}
		f.mutation = tc.Mutation{Version: tc.Version, OperationID: id, Kind: kind, Invite: &tc.InviteIntent{Folder: f.folder, Device: f.device, ExpectedMembership: info.MembershipDigest, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}}
	}
	mutation := f.mutation
	f.task = "invite"
	f.work = func(ctx context.Context) (tc.Result, error) { return w.Mutate(ctx, mutation) }
	return m.invalidate()
}
func (m *model) folderKey(k string) tea.Cmd {
	f := m.flow
	info := f.result.FolderManagement
	if info == nil {
		return nil
	}
	switch k {
	case "v":
		return m.everyday(f.folder, "status", "")
	case "h":
		return m.everyday(f.folder, "paths", "")
	case "D":
		return m.everyday(f.folder, "deleted", "")
	case "C":
		return m.everyday(f.folder, "conflicts", "")
	case "b":
		return m.everyday(f.folder, "storage", "")
	case "p":
		if info.Paused {
			for _, a := range f.result.Attention {
				if a.Code == "MEMBERSHIP_FORK" {
					f.err = "MEMBERSHIP_FORK: keep the old group paused; follow docs/runbooks/membership-fork.md."
					return nil
				}
			}
		}
		f.task = "pause"
		if info.Paused {
			f.task = "resume"
		}
		f.screen = "folder_action"
	case "l":
		f.fields = []field{newField("Destination (absolute)", "", false)}
		f.focus = 0
		f.screen = "relocate_form"
		return m.focusField(0)
	case "a":
		f.device = ""
		f.kind = "invite"
		f.screen = "invite_review"
		f.mutation = tc.Mutation{}
		return m.invalidate()
	case "s":
		f.kind = "share"
		f.screen = "pick_device"
		f.mutation = tc.Mutation{}
		return m.invalidate()
	case "x":
		f.screen = "unregister_preview"
	case "t":
		if f.device == "" {
			f.err = "Inspect a device, select its shared folder, then preview retirement."
			return nil
		}
		w, _ := m.workflows()
		folder, device := f.folder, f.device
		f.task = "retirement_preview"
		f.work = func(ctx context.Context) (tc.Result, error) {
			r, err := w.RetirementPreview(ctx, folder, device)
			return tc.Result{Items: []tc.NamedItem{{Name: r.Warning, Root: r.Disclaimer}, {Name: r.DeviceName, Root: fmt.Sprintf("revision %d -> %d; %d surviving devices", r.CurrentRevision, r.NextRevision, r.RemainingCount)}}}, err
		}
		return m.invalidate()
	case "r":
		return m.startQuery()
	}
	return nil
}
func (m *model) manageFolder(action string) tea.Cmd {
	f := m.flow
	info := f.result.FolderManagement
	if info == nil {
		f.err = "Folder observation missing; refresh."
		return nil
	}
	folder, expected, destination := f.folder, info.Root, ""
	if action == "relocate" {
		destination = f.fields[0].input.Value()
	}
	w, _ := m.workflows()
	f.task = action
	f.work = func(ctx context.Context) (tc.Result, error) {
		return tc.Result{}, w.ManageFolder(ctx, folder, action, expected, destination)
	}
	return m.invalidate()
}

// Revocation reuses the existing authenticated compatibility control, scoped by token digest.
func (m *model) revokeInvitation() tea.Cmd {
	f := m.flow
	w, ok := m.client.(interface {
		RevokeInvitation(context.Context, tc.Invitation) error
	})
	if !ok {
		f.err = "Invitation revocation unavailable; use orbit invite revoke."
		return nil
	}
	inv := f.invitation
	f.task = "revoke_invitation"
	f.work = func(ctx context.Context) (tc.Result, error) { return tc.Result{}, w.RevokeInvitation(ctx, inv) }
	return m.invalidate()
}
