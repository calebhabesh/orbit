package control

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/repository"
)

type LeaveOrbitRequest struct {
	Folder       history.ID `json:"folder"`
	ExpectedRoot string     `json:"expected_root"`
}

type RemoveDeviceRequest struct {
	Folder           history.ID     `json:"folder"`
	DeviceID         history.ID     `json:"device_id"`
	OperationID      string         `json:"operation_id"`
	ConfirmName      string         `json:"confirm_name"`
	MembershipDigest history.Digest `json:"membership_digest"`
	SnapshotDigest   history.Digest `json:"snapshot_digest"`
}

type RemoveDeviceResult = tc.RemovalResult

type removalOperation struct {
	Request  RemoveDeviceRequest           `json:"request"`
	Proposal repository.RetirementProposal `json:"proposal"`
}

func (c *Controller) LeaveOrbit(ctx context.Context, req LeaveOrbitRequest) error {
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	c.contentMu.Lock()
	defer c.contentMu.Unlock()
	if req.Folder == (history.ID{}) {
		return terminalError("INVALID_REQUEST")
	}
	reg, err := c.db.Root(ctx, req.Folder)
	if errors.Is(err, repository.ErrRootNotRegistered) {
		if left, e := c.db.FolderLeft(ctx, req.Folder); e == nil && left {
			return nil
		}
	}
	if err != nil {
		return err
	}
	if reg.Path != req.ExpectedRoot {
		return terminalError("STALE_VIEW")
	}
	joining, err := c.JoiningFolders(ctx)
	if err != nil {
		return err
	}
	if joining[req.Folder] {
		return &ControlError{Code: "SETUP_IN_PROGRESS", Message: "joining is still in progress", Action: "finish the join before leaving"}
	}
	operation, _, phase, _, err := c.db.GetResumableMaintenance(ctx, req.Folder)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && !strings.EqualFold(phase, "COMPLETED") && !strings.EqualFold(phase, "ABORTED") {
		return &ControlError{Code: "REMOVAL_PENDING", Message: "device removal is still in progress", Action: "resume operation " + operation + " before leaving"}
	}
	return c.db.EndFolderExchanges(ctx, req.Folder, func(ctx context.Context) error { return c.ws.Leave(ctx, req.Folder, c.options.Now()) })
}

func (c *Controller) removalPeer(ctx context.Context, folder, peer history.ID) (*replication.Client, error) {
	ident, err := replication.LoadOrCreateIdentity(c.db.StateDir(), c.options.LocalDevice, c.options.Now())
	if err != nil {
		return nil, err
	}
	routes, err := config.LoadPeerRoutes(c.db.StateDir())
	if err != nil {
		return nil, err
	}
	for _, r := range routes {
		if r.Folder == hex.EncodeToString(folder[:]) && r.Device == hex.EncodeToString(peer[:]) {
			der, e := base64.StdEncoding.DecodeString(r.CertificateDER)
			if e != nil {
				return nil, e
			}
			cert, e := x509.ParseCertificate(der)
			if e != nil {
				return nil, e
			}
			return c.peerClient(ctx, "", ident, cert, peer)
		}
	}
	peers, err := config.LoadPeerEndpoints(c.db.StateDir())
	if err != nil {
		return nil, err
	}
	for _, p := range peers {
		if p.Folder == hex.EncodeToString(folder[:]) && p.Device == hex.EncodeToString(peer[:]) {
			pem, e := os.ReadFile(p.Certificate)
			if e != nil {
				return nil, e
			}
			cert, e := replication.ParsePeerCertificate(pem)
			if e != nil {
				return nil, e
			}
			return c.peerClient(ctx, p.URL, ident, cert, peer)
		}
	}
	return nil, errors.New("no approved route; connect this surviving device first")
}

// RemoveDevice keeps the exact reviewed proposal across retries. All survivors
// must prepare the same accepted-retiree set before the local membership changes.
// Partial rollout remains pending, with the same operation available to resume.
func (c *Controller) RemoveDevice(ctx context.Context, req RemoveDeviceRequest) (RemoveDeviceResult, error) {
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	out := RemoveDeviceResult{OperationID: req.OperationID, PendingDevices: []string{}, DeviceID: hex.EncodeToString(req.DeviceID[:])}
	if req.DeviceID == c.options.LocalDevice || c.options.LocalDevice == (history.ID{}) || req.Folder == (history.ID{}) || protocol.NetworkHex(req.OperationID, 32) != nil {
		return out, terminalError("INVALID_REQUEST")
	}
	if left, err := c.db.FolderLeft(ctx, req.Folder); err != nil {
		return out, err
	} else if left {
		return out, repository.ErrFolderLeft
	}
	id := "remove-" + req.OperationID
	var raw []byte
	var phase string
	var op removalOperation
	savedPhase, savedRaw, err := c.db.MaintenanceByID(ctx, id)
	if err == nil {
		phase, raw = savedPhase, savedRaw
		if err = protocol.DecodeStrict(raw, &op); err != nil {
			return out, err
		}
		if op.Request != req {
			return out, IdempotencyConflictError(req.OperationID)
		}
	} else {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		savedID, _, pendingPhase, _, pendingErr := c.db.GetResumableMaintenance(ctx, req.Folder)
		if pendingErr != nil && !errors.Is(pendingErr, sql.ErrNoRows) {
			return out, pendingErr
		}
		if pendingErr == nil && !strings.EqualFold(pendingPhase, "COMPLETED") && !strings.EqualFold(pendingPhase, "ABORTED") {
			return out, &ControlError{Code: "REMOVAL_PENDING", Message: "another removal is pending", Action: "resume operation " + savedID}
		}
		name, e := c.db.GetDeviceDisplayName(ctx, req.DeviceID)
		if e != nil {
			return out, e
		}
		if name == "" {
			name = fmt.Sprintf("Device %x", req.DeviceID[:8])
		}
		if req.ConfirmName != name {
			return out, &ControlError{Code: "NAME_MISMATCH", Message: "type the device name exactly", Action: "review the device and type " + name}
		}
		cur, app, e := c.db.GetMembership(ctx, req.Folder)
		if e != nil {
			return out, e
		}
		preview, e := c.RetireMemberPreview(ctx, req.Folder, req.DeviceID)
		if e != nil {
			return out, e
		}
		if app.Digest != req.MembershipDigest || preview.SnapshotDigest != req.SnapshotDigest {
			return out, terminalError("STALE_VIEW")
		}
		versions, e := c.db.VersionsByAuthor(ctx, req.Folder, req.DeviceID)
		if e != nil {
			return out, e
		}
		snap := protocol.RetirementSnapshot{Folder: req.Folder, ConfigurationRev: cur.Revision, RetiredDevice: req.DeviceID, AcceptedByRetiree: versions}
		next := protocol.Membership{Folder: req.Folder, Revision: cur.Revision + 1, PriorDigest: app.Digest, Retired: append([]protocol.RetiredMember(nil), cur.Retired...)}
		for _, m := range cur.Active {
			if m.Device != req.DeviceID {
				next.Active = append(next.Active, m)
			}
		}
		next.Retired = append(next.Retired, protocol.RetiredMember{Device: req.DeviceID, RetiredAt: next.Revision, SnapshotDigest: preview.SnapshotDigest})
		op = removalOperation{Request: req, Proposal: repository.RetirementProposal{Initiator: c.options.LocalDevice, DeviceName: name, Membership: next, Snapshot: snap}}
		raw, e = json.Marshal(op)
		if e != nil {
			return out, e
		}
		phase = "PROPOSED"
		if e = c.db.PrepareRetirement(ctx, op.Proposal); e != nil {
			return out, e
		}
		if e = c.db.SaveResumableMaintenance(ctx, id, req.Folder, req.DeviceID, phase, raw, c.options.Now()); e != nil {
			return out, e
		}
	}
	p := op.Proposal
	out.ReceivedChanges = len(p.Snapshot.AcceptedByRetiree)
	if phase == "ABORTED" {
		out.State = "needs_review"
		out.Message = "The received history changed. Review Remove Device again before confirming."
		return out, nil
	}
	if phase == "COMPLETED" {
		out.State = "completed"
		out.Message = p.DeviceName + " was removed; files on every device stay"
		return out, nil
	}
	changed := func(e error) (RemoveDeviceResult, error) {
		if !errors.Is(e, repository.ErrRetirementChanged) {
			return out, e
		}
		app, readErr := c.db.Membership(ctx, req.Folder)
		if readErr != nil {
			return out, readErr
		}
		// Before rollout, a changed membership invalidates only this proposal.
		// After rollout starts, divergence may describe an irrevocable commit.
		if app.Digest != p.Membership.PriorDigest && phase != "PROPOSED" {
			return out, e
		}
		if err := c.db.SaveResumableMaintenance(ctx, id, req.Folder, req.DeviceID, "ABORTED", raw, c.options.Now()); err != nil {
			return out, err
		}
		out.State = "needs_review"
		out.Message = "The reviewed membership or received history changed. Review Remove Device again before confirming."
		return out, nil
	}
	// Every call is bounded independently; failed calls keep their device listed.
	call := func(peer history.ID, action string) error {
		work, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		client, e := c.removalPeer(work, req.Folder, peer)
		if e != nil {
			return e
		}
		defer client.CloseIdleConnections()
		response, e := client.Retirement(work, replication.RetirementRequest{ProtocolVersion: replication.ProtocolVersion, DeviceID: hex.EncodeToString(c.options.LocalDevice[:]), Action: action, Proposal: p})
		digest, _ := protocol.MembershipDigest(p.Membership)
		state := "prepared"
		if action == "commit" {
			state = "completed"
		}
		if e == nil && (response.ProtocolVersion != replication.ProtocolVersion || response.Digest != hex.EncodeToString(digest[:]) || response.State != state) {
			e = repository.ErrRetirementChanged
		}
		return e
	}
	pending := func(peer history.ID, e error) {
		name, _ := c.db.GetDeviceDisplayName(ctx, peer)
		if name == "" {
			name = fmt.Sprintf("Device %x", peer[:8])
		}
		out.PendingDevices = append(out.PendingDevices, name)
		if out.Message == "" {
			var wire *replication.WireError
			if phase == "ROLLING_OUT" && errors.As(e, &wire) && wire.Body.Code == "RETIREMENT_CHANGED" {
				out.Message = name + " has different received history after removal started. Removal is partially applied; keep all files and follow docs/runbooks/membership-fork.md for reviewed recovery. Retired history cannot be expanded by retrying."
				return
			}
			out.Message = "Waiting for " + name + ": " + e.Error() + ". Connect/sync surviving devices, then retry this operation."
		}
	}
	if phase == "PROPOSED" {
		// A saved review can become stale while survivors are unavailable.
		// Require a fresh review before spending another network attempt.
		if err = c.db.PrepareRetirement(ctx, p); err != nil {
			return changed(err)
		}
		for _, m := range p.Membership.Active {
			if m.Device != c.options.LocalDevice {
				if e := call(m.Device, "prepare"); e != nil {
					pending(m.Device, e)
				}
			}
		}
		if len(out.PendingDevices) > 0 {
			out.State = "pending"
			return out, nil
		}
		if err = c.db.PrepareRetirement(ctx, p); err != nil {
			return changed(err)
		}
		// Persist rollout intent before commit, so a crash can repeat it safely.
		phase = "ROLLING_OUT"
		if err = c.db.SaveResumableMaintenance(ctx, id, req.Folder, req.DeviceID, phase, raw, c.options.Now()); err != nil {
			return out, err
		}
	}
	if _, err = c.db.CommitRetirement(ctx, p); err != nil {
		return changed(err)
	}
	for _, m := range p.Membership.Active {
		if m.Device != c.options.LocalDevice {
			if e := call(m.Device, "commit"); e != nil {
				pending(m.Device, e)
			}
		}
	}
	if len(out.PendingDevices) > 0 {
		out.State = "pending"
		return out, nil
	}
	if err = c.db.SaveResumableMaintenance(ctx, id, req.Folder, req.DeviceID, "COMPLETED", raw, c.options.Now()); err != nil {
		return out, err
	}
	out.State = "completed"
	out.Message = p.DeviceName + " was removed; files on every device stay"
	return out, nil
}

type ResumeRemovalRequest struct {
	Folder      history.ID `json:"folder"`
	OperationID string     `json:"operation_id"`
}

func (c *Controller) ResumeRemoval(ctx context.Context, req ResumeRemovalRequest) (RemoveDeviceResult, error) {
	_, raw, err := c.db.MaintenanceByID(ctx, "remove-"+req.OperationID)
	if err != nil {
		return RemoveDeviceResult{}, err
	}
	var op removalOperation
	if err = protocol.DecodeStrict(raw, &op); err != nil {
		return RemoveDeviceResult{}, err
	}
	if op.Request.Folder != req.Folder {
		return RemoveDeviceResult{}, terminalError("INVALID_REQUEST")
	}
	return c.RemoveDevice(ctx, op.Request)
}
