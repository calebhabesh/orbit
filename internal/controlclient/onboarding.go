package controlclient

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/state"
)

// Setup retains the exact retry intent before submission. A lost response never
// requires a fresh operation identity. Networking changes need a daemon restart,
// as in the CLI setup adapter; committed work outlives the client waiter.
func (c *Client) Setup(ctx context.Context, m tc.Mutation) (tc.Result, error) {
	if m.Kind != "setup" && m.Kind != "adopt" && m.Kind != "join" {
		return tc.Result{}, errors.New("INVALID_REQUEST")
	}
	if err := m.Validate(); err != nil {
		return tc.Result{}, err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return tc.Result{}, err
	}
	if err = state.ValidateDirectory(c.StateDir); err != nil {
		return tc.Result{}, err
	}
	name := "setup-request-" + m.OperationID + ".json"
	old, err := state.ReadPrivate(c.StateDir, name, tc.MaxMetadata)
	if err == nil {
		var prior tc.Mutation
		if err = tc.Decode(old, &prior); err != nil {
			return tc.Result{}, err
		}
		canonical, _ := json.Marshal(prior)
		if string(canonical) != string(b) {
			return tc.Result{}, errors.New("IDEMPOTENCY_CONFLICT")
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return tc.Result{}, err
	}
	if errors.Is(err, os.ErrNotExist) {
		if err = writeNewPrivate(c.StateDir, name, b); err != nil {
			return tc.Result{}, err
		}
	}
	before, err := c.Query(ctx, tc.Query{Version: tc.Version, Kind: "settings"})
	if err != nil {
		return tc.Result{}, err
	}
	desired := tc.Settings{}
	var policy *tc.NetworkPolicy
	if m.Setup != nil {
		desired = m.Setup.Settings
		policy = m.Setup.Network
	} else {
		desired = m.Join.Settings
		policy = m.Join.Network
	}
	restartName := "setup-network-" + m.OperationID + ".json"
	restart := struct{ Required, Completed bool }{}
	saved, e := state.ReadPrivate(c.StateDir, restartName, 1024)
	if e == nil {
		if err = tc.Decode(saved, &restart); err != nil {
			return tc.Result{}, err
		}
	} else if errors.Is(e, os.ErrNotExist) {
		if policy != nil {
			current, e := c.Query(ctx, tc.Query{Version: tc.Version, Kind: "network_status"})
			if e != nil {
				return tc.Result{}, e
			}
			restart.Required = current.Network == nil || current.Network.ActivePolicy != *policy
		}
		restart.Required = restart.Required || before.Settings != nil && (before.Settings.PeerListen != desired.PeerListen || before.Settings.EnrollmentListen != desired.EnrollmentListen || before.Settings.AdvertisedPeer != desired.AdvertisedPeer || before.Settings.AdvertisedEnrollment != desired.AdvertisedEnrollment)
		b, _ := json.Marshal(restart)
		if err = writeNewPrivate(c.StateDir, restartName, b); err != nil {
			return tc.Result{}, err
		}
	} else {
		return tc.Result{}, e
	}
	r, err := c.Mutate(ctx, m)
	if err != nil || r.Operation == nil {
		return r, err
	}
	if restart.Required && !restart.Completed {
		owned, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if c.RestartDaemon == nil {
			return r, errors.New("NETWORK_RESTART_REQUIRED")
		}
		if err = c.RestartDaemon(owned); err != nil {
			return r, err
		}
		restart.Completed = true
		b, _ := json.Marshal(restart)
		err = config.WritePrivate(c.StateDir, restartName, b)
	}
	return r, err
}

// SaveInvitation is an explicit private transfer, never a log or status field.
func (c *Client) SaveInvitation(ctx context.Context, path string, inv tc.Invitation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := inv.Validate(); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return errors.New("absolute private transfer path required")
	}
	if err := state.ValidateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	b, err := json.Marshal(inv)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		old, e := state.ReadPrivate(filepath.Dir(path), filepath.Base(path), 16384)
		if e != nil {
			return e
		}
		if string(old) == string(b) {
			return nil
		}
		return errors.New("IDEMPOTENCY_CONFLICT")
	}
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func parseIdentity(s string) (history.ID, error) {
	var id history.ID
	b, err := decodeID(s)
	if err == nil {
		copy(id[:], b)
	}
	return id, err
}

// ManageFolder reuses the same compatibility controls as CLI and browser.
// Relocation retains expected-path validation and its owning recovery journal.
func (c *Client) ManageFolder(ctx context.Context, folder, action, expected, destination string) error {
	id, err := parseIdentity(folder)
	if err != nil {
		return err
	}
	return c.WithController(ctx, func() error {
		switch action {
		case "pause":
			return c.Call(ctx, "POST", "/api/v1/folders/pause", control.FolderPauseRequest{Folder: id, Reason: "owner paused in terminal"}, nil)
		case "resume":
			return c.Call(ctx, "POST", "/api/v1/folders/resume", control.FolderResumeRequest{Folder: id}, nil)
		case "relocate":
			return c.Call(ctx, "POST", "/api/v1/folders/relocate", control.RelocateFolderRequest{Folder: id, ExpectedPath: expected, Path: destination}, nil)
		default:
			return errors.New("INVALID_REQUEST")
		}
	}, func(ctrl *control.Controller) error {
		switch action {
		case "pause":
			return ctrl.PauseFolder(ctx, id, "owner paused in terminal")
		case "resume":
			return ctrl.ResumeFolder(ctx, id)
		case "relocate":
			_, err := ctrl.RelocateFolder(ctx, control.RelocateFolderRequest{Folder: id, ExpectedPath: expected, Path: destination})
			return err
		default:
			return errors.New("INVALID_REQUEST")
		}
	})
}
func (c *Client) RetirementPreview(ctx context.Context, folder, device string) (out control.RetireDevicePreviewResult, err error) {
	f, err := parseIdentity(folder)
	if err != nil {
		return out, err
	}
	d, err := parseIdentity(device)
	if err != nil {
		return out, err
	}
	req := control.RetireDevicePreviewRequest{Folder: f, DeviceID: d}
	err = c.WithController(ctx, func() error { return c.Call(ctx, "POST", "/api/v1/peers/retire/preview", req, &out) }, func(ctrl *control.Controller) error {
		r, e := ctrl.PreviewDeviceRetirement(ctx, req)
		if r != nil {
			out = *r
		}
		return e
	})
	return
}

// PrivateInvitation admits only a small private regular transfer file.
func PrivateInvitation(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("INVALID_REQUEST")
	}
	b, err := state.ReadPrivate(filepath.Dir(path), filepath.Base(path), 16384)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(string(b))
	for _, prefix := range []string{"orbit-invitation:v2:", "orbit-invitation:v3:"} {
		if strings.HasPrefix(code, prefix) {
			return base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, prefix))
		}
	}
	return b, nil
}

func writeNewPrivate(dir, name string, b []byte) error {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		old, e := state.ReadPrivate(dir, name, tc.MaxMetadata)
		if e != nil {
			return e
		}
		if string(old) == string(b) {
			return nil
		}
		return errors.New("IDEMPOTENCY_CONFLICT")
	}
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// RevokeInvitation binds deliberate transfer to the owning controller's digest operation.
func (c *Client) RevokeInvitation(ctx context.Context, inv tc.Invitation) error {
	if err := inv.Validate(); err != nil {
		return err
	}
	token, err := hex.DecodeString(inv.Capability)
	if err != nil {
		return err
	}
	req := control.RevokeInvitationRequest{Digest: sha256.Sum256(token)}
	return c.WithController(ctx, func() error {
		var out control.RevokeInvitationResult
		return c.Call(ctx, "POST", "/api/v1/invitations/revoke", req, &out)
	}, func(ctrl *control.Controller) error { _, err := ctrl.RevokeInvitation(ctx, req); return err })
}
