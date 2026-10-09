package control

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
)

// FolderRenameRequest renames an Orbit on every member device.
type FolderRenameRequest struct {
	Folder history.ID `json:"folder"`
	Name   string     `json:"name"`
}

// RenameFolder sets the Orbit's shared name and pushes it to its members.
func (c *Controller) RenameFolder(ctx context.Context, folder history.ID, name string) error {
	if err := c.db.RenameFolder(ctx, folder, name, c.options.LocalDevice); err != nil {
		return renameError(err)
	}
	if s, err := config.LoadSettings(c.db.StateDir()); err == nil {
		if s.WorkspaceNames == nil {
			s.WorkspaceNames = map[string]string{}
		}
		s.WorkspaceNames[hex.EncodeToString(folder[:])] = name
		_ = config.SaveSettings(c.db.StateDir(), s)
	}
	c.shareNames(ctx, folder)
	return nil
}

// renameDevice sets a device's shared name and pushes it to every Orbit the
// device shares with this one.
func (c *Controller) renameDevice(ctx context.Context, device history.ID, name string) error {
	if err := c.db.RenameDevice(ctx, device, name, c.options.LocalDevice); err != nil {
		return renameError(err)
	}
	if folders, err := c.db.RegisteredFolders(ctx); err == nil {
		for _, f := range folders {
			c.shareNames(ctx, f.Folder)
		}
	}
	return nil
}

func renameError(err error) error {
	if errors.Is(err, repository.ErrInvalidName) {
		return &ControlError{Code: "INVALID_REQUEST", Message: err.Error(), Action: "use 1 to 128 printable characters"}
	}
	return err
}

// shareNames queues a sync with each other member so a rename spreads now
// rather than at the next reconciliation. Failures leave the periodic sync.
func (c *Controller) shareNames(ctx context.Context, folder history.ID) {
	membership, _, err := c.db.GetMembership(ctx, folder)
	if err != nil {
		return
	}
	queued := false
	for _, m := range membership.Active {
		if m.Device == c.options.LocalDevice {
			continue
		}
		peer := m.Device
		if _, err := c.db.EnqueueDurableTask(ctx, repository.DurableTask{Folder: folder, Peer: &peer, Kind: "sync"}); err == nil {
			queued = true
		}
	}
	if queued && c.options.WorkChanged != nil {
		c.options.WorkChanged()
	}
}

// nameApprovedDevice records the device name from an approval: the owner's
// own choice (different from the joiner's requested label) is shared and
// outranks the joiner's name; otherwise the label stays local until the
// joiner's shared name arrives.
func (c *Controller) nameApprovedDevice(ctx context.Context, device history.ID, requested, chosen string) error {
	if chosen != "" && chosen != requested {
		if err := c.db.RenameDeviceAtLeast(ctx, device, chosen, c.options.LocalDevice, 2); err != nil {
			return renameError(err)
		}
		return nil
	}
	if requested != "" {
		return c.db.SetDeviceDisplayName(ctx, device, requested)
	}
	return nil
}
