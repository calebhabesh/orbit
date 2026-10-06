package control

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"path/filepath"
	"strings"

	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
)

// This compatibility record supplies pinned transport to the earlier join flow.
// T04 replaces its setup phases with reviewed, durable joining jobs.
type pinnedJoin struct {
	Invitation tc.Invitation                   `json:"invitation"`
	Wire       protocol.TerminalEnrollmentWire `json:"wire"`
	Operation  string                          `json:"operation"`
	Root       string                          `json:"root"`
}

func (c *Controller) submitPinnedJoin(ctx context.Context, req JoinFlowSubmitRequest) (*JoinFlowSubmitResult, error) {
	var inv tc.Invitation
	if req.Invitation != nil {
		inv = *req.Invitation
	} else if strings.HasPrefix(req.InvitationToken, "orbit-invitation:v2:") {
		b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(req.InvitationToken, "orbit-invitation:v2:"))
		if err != nil {
			return nil, terminalError("INVALID_REQUEST")
		}
		if err := tc.Decode(b, &inv); err != nil {
			return nil, err
		}
	} else {
		return nil, terminalError("IDENTITY_REVIEW_REQUIRED")
	}
	if err := inv.Validate(); err != nil {
		return nil, err
	}
	if req.TargetFolder != inv.Folder || req.RemoteEndpoint != inv.EnrollmentEndpoint {
		return nil, terminalError("FOLDER_MISMATCH")
	}
	return c.submitReviewedCompatibilityJoin(ctx, req, inv)
}

func (c *Controller) pinnedJoinStatus(ctx context.Context, endpoint, request string) (*EnrollmentStatusResult, error) {
	if err := (tc.Query{Version: "1", Kind: "operation", ID: request}).Validate(); err != nil {
		return nil, err
	}
	var record pinnedJoin
	if err := c.db.TerminalRecord(ctx, "joinflow/"+request, &record); err != nil {
		return nil, err
	}
	if endpoint != record.Invitation.EnrollmentEndpoint {
		return nil, terminalError("IDENTITY_MISMATCH")
	}
	id, err := replication.LoadOrCreateIdentity(c.db.StateDir(), c.options.LocalDevice, c.options.Now())
	if err != nil {
		return nil, err
	}
	client, err := c.enrollmentClient(ctx, record.Invitation, id)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	result, err := client.Status(ctx, request)
	if err != nil {
		return nil, err
	}
	out := &EnrollmentStatusResult{RequestID: request, Folder: terminalID(record.Wire.Folder), DeviceID: id.DeviceID, SuggestedLabel: record.Wire.Label, Status: result.State}
	if out.Status == "pending_approval" {
		out.Status = "pending"
	}
	if result.State == "approved" {
		b, err := hex.DecodeString(result.MembershipHex)
		if err != nil {
			return nil, err
		}
		m, err := protocol.DecodeMembership(b)
		if err != nil {
			return nil, err
		}
		out.Membership = &m
		out.Revision = m.Revision
	}
	return out, nil
}

func (c *Controller) enrollmentV2Record(ctx context.Context, request string) (replication.EnrollmentRecord, bool, error) {
	var record replication.EnrollmentRecord
	err := c.db.EnrollmentTransaction(ctx, func(t *repository.EnrollmentTx) error { return t.Get("request/"+request, &record) })
	if errors.Is(err, repository.ErrOperationNotFound) {
		return record, false, nil
	}
	return record, err == nil, err
}
func compatibilityStatus(record replication.EnrollmentRecord) (*EnrollmentStatusResult, error) {
	w := record.Wire
	r := record.Result
	out := &EnrollmentStatusResult{RequestID: r.Request, Folder: terminalID(w.Folder), DeviceID: terminalID(w.Requester), SuggestedLabel: w.Label, Status: r.State}
	if out.Status == "pending_approval" {
		out.Status = "pending"
	}
	if r.State == "approved" {
		b, err := hex.DecodeString(r.MembershipHex)
		if err != nil {
			return nil, err
		}
		m, err := protocol.DecodeMembership(b)
		if err != nil {
			return nil, err
		}
		out.Membership = &m
		out.Revision = m.Revision
	}
	return out, nil
}
func (c *Controller) approveEnrollmentV2Compatibility(ctx context.Context, req ApproveEnrollmentRequest, record replication.EnrollmentRecord) (*ApproveEnrollmentResult, error) {
	if req.Reviewed == nil || req.Reviewed.Request != req.RequestID || req.Reviewed.Folder != hex.EncodeToString(req.Folder[:]) || req.Reviewed.Decision != "approve" {
		return nil, terminalError("STALE_VIEW")
	}
	if req.Endpoint != "" && req.Endpoint != record.Wire.RequesterEndpoint {
		return nil, terminalError("STALE_VIEW")
	}
	result, err := c.TerminalMutate(ctx, tc.Mutation{Version: "1", Kind: "approval", OperationID: req.OperationID, Approval: req.Reviewed})
	if err != nil {
		return nil, err
	}
	_ = result
	if req.SuggestedLabel != "" {
		if err := c.db.SetDeviceDisplayName(ctx, terminalID(record.Wire.Requester), req.SuggestedLabel); err != nil {
			return nil, err
		}
	}
	endpoint := req.Endpoint
	if endpoint == "" {
		endpoint = record.Wire.RequesterEndpoint
	}
	if endpoint != "" {
		der, err := base64.StdEncoding.DecodeString(record.Wire.CertificateDER)
		if err != nil {
			return nil, err
		}
		name := "enrolled-" + record.Wire.RequesterPin + ".crt"
		if err := config.WritePrivate(c.db.StateDir(), name, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); err != nil {
			return nil, err
		}
		if err := config.SetPeerEndpoint(c.db.StateDir(), config.PeerEndpoint{Folder: record.Wire.Folder, Device: record.Wire.Requester, URL: endpoint, Certificate: filepath.Join(c.db.StateDir(), name)}); err != nil {
			return nil, err
		}
	}
	m, a, err := c.db.GetMembership(ctx, req.Folder)
	if err != nil {
		return nil, err
	}
	return &ApproveEnrollmentResult{RequestID: req.RequestID, Status: "approved", Revision: m.Revision, Digest: a.Digest}, nil
}

func (c *Controller) persistRequesterEndpoint(record replication.EnrollmentRecord) error {
	w := record.Wire
	der, err := base64.StdEncoding.DecodeString(w.CertificateDER)
	if err != nil {
		return err
	}
	name := "enrolled-" + w.RequesterPin + ".crt"
	if err = config.WritePrivate(c.db.StateDir(), name, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); err != nil {
		return err
	}
	return config.SetPeerEndpoint(c.db.StateDir(), config.PeerEndpoint{Folder: w.Folder, Device: w.Requester, URL: w.RequesterEndpoint, Certificate: filepath.Join(c.db.StateDir(), name)})
}
