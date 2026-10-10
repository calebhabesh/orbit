package replication

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
)

const retirementPath = "/peer/v1/membership/retirement"

// RecordDeviceRemoval applies an authenticated peer refusal to every sync
// caller, including direct CLI/control use. Call only after releasing this
// caller's exchange so ending participation can drain the other exchanges.
func RecordDeviceRemoval(ctx context.Context, db *repository.DB, folder, peer history.ID, syncErr error) error {
	var ending *WireError
	if !errors.As(syncErr, &ending) || ending.Body.Code != DeviceRemovedCode {
		return nil
	}
	retired, err := db.IsDeviceRetired(ctx, folder, peer)
	if err != nil || retired {
		return err
	}
	by, reportedOnly := peer, true
	if raw, err := hex.DecodeString(ending.Body.RemovedBy); err == nil && len(raw) == len(by) {
		var actor history.ID
		copy(actor[:], raw)
		if actor != (history.ID{}) {
			by, reportedOnly = actor, false
		}
	}
	return db.EndFolderExchanges(ctx, folder, func(ctx context.Context) error {
		return db.MarkDeviceRemovedFromPeer(ctx, folder, peer, by, reportedOnly)
	})
}

type RetirementRequest struct {
	ProtocolVersion string                        `json:"protocol_version"`
	DeviceID        string                        `json:"device_id"`
	Action          string                        `json:"action"`
	Proposal        repository.RetirementProposal `json:"proposal"`
}

type RetirementResponse struct {
	ProtocolVersion string `json:"protocol_version"`
	State           string `json:"state"`
	Digest          string `json:"digest"`
}

func (client *Client) Retirement(ctx context.Context, req RetirementRequest) (RetirementResponse, error) {
	var r RetirementResponse
	return r, client.postJSON(ctx, retirementPath, req, &r)
}

func (server *Server) handleRetirement(w http.ResponseWriter, r *http.Request) {
	var body RetirementRequest
	if !readRequest(w, r, &body) {
		return
	}
	if body.ProtocolVersion != ProtocolVersion || (body.Action != "prepare" && body.Action != "commit") {
		writeWireError(w, 400, "INVALID_REQUEST", "invalid retirement request", false, "update Orbit and review again")
		return
	}
	device, pin, err := server.requestIdentity(r, body.DeviceID)
	p := body.Proposal
	if err != nil || device != p.Initiator {
		server.writeAuthorizationError(w, repository.ErrUnauthorized)
		return
	}
	// Authorize against the immutable reviewed predecessor, even on commit replay.
	prior, app, err := server.repo.GetMembership(r.Context(), p.Membership.Folder, p.Snapshot.ConfigurationRev)
	if err != nil || app.Digest != p.Membership.PriorDigest {
		server.writeAuthorizationError(w, repository.ErrMembershipMismatch)
		return
	}
	allowed := false
	for _, m := range prior.Active {
		if m.Device == device && m.KeyPin == pin {
			allowed = true
		}
	}
	retired, e := server.repo.IsDeviceRetired(r.Context(), p.Membership.Folder, device)
	if !allowed || e != nil || retired {
		server.writeAuthorizationError(w, repository.ErrUnauthorized)
		return
	}
	// The common exchange barrier prevents a Leave during this request.
	if err = server.holdExchange(r, p.Membership.Folder); err != nil {
		server.writeAuthorizationError(w, err)
		return
	}
	state := "prepared"
	if body.Action == "prepare" {
		err = server.repo.PrepareRetirement(r.Context(), p)
	} else {
		_, err = server.repo.CommitRetirement(r.Context(), p)
		state = "completed"
	}
	if err != nil {
		code := "RETIREMENT_CHANGED"
		if errors.Is(err, repository.ErrMembershipFork) {
			code = "MEMBERSHIP_FORK"
		}
		writeWireError(w, http.StatusConflict, code, "surviving device's retirement review differs", false, "sync surviving devices and review removal again")
		return
	}
	digest, err := protocol.MembershipDigest(p.Membership)
	if err != nil {
		writeWireError(w, 400, "INVALID_REQUEST", "invalid membership", false, "review removal again")
		return
	}
	writeJSON(w, 200, RetirementResponse{ProtocolVersion: ProtocolVersion, State: state, Digest: hex.EncodeToString(digest[:])})
}
