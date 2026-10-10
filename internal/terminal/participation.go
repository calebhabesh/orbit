package terminal

import (
	"context"
	"encoding/hex"

	tea "charm.land/bubbletea/v2"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
)

type participationWorkflows interface {
	LeaveOrbit(context.Context, control.LeaveOrbitRequest) error
	RemoveDevice(context.Context, control.RemoveDeviceRequest) (control.RemoveDeviceResult, error)
}

func (m *model) pickRemovalDevice() tea.Cmd {
	f := m.flow
	info := f.result.FolderManagement
	if info == nil {
		return nil
	}
	if f.device != "" && f.device != info.LocalDevice {
		return m.previewRemoval()
	}
	f.items = nil
	for _, member := range info.Members {
		if member.ID != info.LocalDevice {
			f.items = append(f.items, member)
		}
	}
	f.screen = "pick_removal_device"
	f.selected = 0
	if len(f.items) == 0 {
		f.err = "This Orbit has no other device to remove. Use Leave to stop syncing here."
	}
	return nil
}

func (m *model) removalPickerKey(k string) tea.Cmd {
	f := m.flow
	switch k {
	case "down", "j":
		f.selected = min(f.selected+1, max(0, len(f.items)-1))
	case "up", "k":
		f.selected = max(0, f.selected-1)
	case "enter":
		if len(f.items) > 0 {
			f.device = f.items[f.selected].ID
			return m.previewRemoval()
		}
	}
	return nil
}

func (m *model) previewRemoval() tea.Cmd {
	f := m.flow
	w, _ := m.workflows()
	folder, device := f.folder, f.device
	f.task = "retirement_preview"
	f.work = func(ctx context.Context) (tc.Result, error) {
		r, err := w.RetirementPreview(ctx, folder, device)
		return tc.Result{Retirement: &tc.RetirementReview{DeviceName: r.DeviceName, ReceivedChanges: r.ReceivedChanges, MembershipDigest: hex.EncodeToString(r.MembershipDigest[:]), SnapshotDigest: hex.EncodeToString(r.SnapshotDigest[:]), Warning: r.Warning, Disclaimer: r.Disclaimer}}, err
	}
	return m.invalidate()
}

func identity(s string) history.ID {
	b, _ := hex.DecodeString(s)
	var id history.ID
	copy(id[:], b)
	return id
}

func (m *model) leaveOrbit() tea.Cmd {
	f := m.flow
	w, ok := m.client.(participationWorkflows)
	if !ok {
		f.err = "Update Orbit to use Leave."
		return nil
	}
	req := control.LeaveOrbitRequest{Folder: identity(f.folder), ExpectedRoot: f.folderRoot}
	f.task = "leave"
	f.work = func(ctx context.Context) (tc.Result, error) { return tc.Result{}, w.LeaveOrbit(ctx, req) }
	return m.invalidate()
}

func (m *model) submitRemoval() tea.Cmd {
	f := m.flow
	w, ok := m.client.(participationWorkflows)
	if !ok {
		f.err = "Update Orbit to remove a device."
		return nil
	}
	if f.removalRequest == nil {
		r := f.result.Retirement
		if r == nil {
			f.err = "Review the device again."
			return nil
		}
		if f.fields[0].input.Value() != r.DeviceName {
			f.err = "Type " + safe(r.DeviceName) + " exactly to confirm."
			return nil
		}
		id, err := newID()
		if err != nil {
			f.err = "Could not allocate removal identity."
			return nil
		}
		f.removalRequest = &control.RemoveDeviceRequest{Folder: identity(f.folder), DeviceID: identity(f.device), OperationID: id, ConfirmName: f.fields[0].input.Value(), MembershipDigest: history.Digest(identity(r.MembershipDigest)), SnapshotDigest: history.Digest(identity(r.SnapshotDigest))}
	}
	req := *f.removalRequest
	f.task = "remove_device"
	f.work = func(ctx context.Context) (tc.Result, error) {
		out, err := w.RemoveDevice(ctx, req)
		return tc.Result{Removal: &out}, err
	}
	return m.invalidate()
}

func (m *model) resumeRemoval() tea.Cmd {
	f := m.flow
	w, ok := m.client.(interface {
		ResumeRemoval(context.Context, control.ResumeRemovalRequest) (control.RemoveDeviceResult, error)
	})
	if !ok {
		f.err = "Update Orbit to resume removal."
		return nil
	}
	req := control.ResumeRemovalRequest{Folder: identity(f.folder), OperationID: f.operation}
	f.task = "remove_device"
	f.work = func(ctx context.Context) (tc.Result, error) {
		out, err := w.ResumeRemoval(ctx, req)
		return tc.Result{Removal: &out}, err
	}
	return m.invalidate()
}
