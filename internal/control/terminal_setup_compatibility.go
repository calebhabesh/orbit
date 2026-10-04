package control

import (
	"context"
	"errors"
	"github.com/calebhabesh/file-sync/internal/repository"
	"strings"

	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
)

// Legacy explicit submit is an adapter over the same reviewed job. Ordinary CLI
// supplies a review file; old callers deliberately submit their current root.
func (c *Controller) submitReviewedCompatibilityJoin(ctx context.Context, req JoinFlowSubmitRequest, inv tc.Invitation) (*JoinFlowSubmitResult, error) {
	settings, err := config.LoadRuntimeSettings(c.db.StateDir())
	if err != nil {
		return nil, err
	}
	label := req.DeviceLabel
	if label == "" {
		label = "Orbit Device"
	}
	plan := tc.SetupIntent{DeviceName: label, FolderName: "Orbit", Root: req.RootPath, Settings: settings}
	q := tc.Query{Version: "1", Kind: "root_preview", Path: req.RootPath, Name: "join", RootPlan: &plan}
	var preview tc.Result
	for {
		preview, err = c.TerminalQuery(ctx, q)
		if err != nil {
			return nil, err
		}
		if preview.Preview.Complete {
			break
		}
		q.Cursor = preview.Cursor
	}
	operation, err := randomTerminalID()
	if err != nil {
		return nil, err
	}
	attempt, err := randomTerminalID()
	if err != nil {
		return nil, err
	}
	m := tc.Mutation{Version: "1", OperationID: operation, Kind: "join", Join: &tc.JoinIntent{Invitation: inv, Attempt: attempt, Root: plan.Root, DeviceName: label, FolderName: plan.FolderName, Settings: settings, Preview: *preview.Review}}
	r, err := c.TerminalMutate(ctx, m)
	if err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, &ControlError{Code: r.Error.Code, Message: r.Error.Message, Action: r.Error.Action, Retryable: r.Error.Retryable}
	}
	var job setupJob
	if err = c.db.TerminalRecord(ctx, "setupjob/"+operation, &job); err != nil {
		return nil, err
	}
	if job.Wire == nil {
		return nil, terminalError("SETUP_BLOCKED")
	}
	inv.Capability = strings.Repeat("1", 64)
	record := pinnedJoin{Invitation: inv, Wire: *job.Wire, Root: req.RootPath, Operation: operation}
	if err = c.db.SaveTerminalRecord(ctx, "joinflow/"+r.Join.Request, record, true); err != nil {
		return nil, err
	}
	cfg, err := config.Load(c.db.StateDir())
	if err != nil {
		return nil, err
	}
	return &JoinFlowSubmitResult{RequestID: r.Join.Request, Status: "pending", TargetFolder: inv.Folder, RemoteEndpoint: inv.EnrollmentEndpoint, RootPath: req.RootPath, DeviceID: cfg.DeviceID, KeyPin: job.Wire.RequesterPin, Message: "awaiting owner approval; resume the durable reviewed operation"}, nil
}

func compatibilitySetupPhase(phase string) string {
	if phase == "ready" {
		return "completed"
	}
	return phase
}
func (c *Controller) startReviewedCompatibilitySetup(ctx context.Context, req StartSetupRequest) (*StartSetupResult, error) {
	label, name := req.DeviceLabel, req.WorkspaceName
	if label == "" {
		label = "Orbit Device"
	}
	if name == "" {
		name = "Orbit"
	}
	var m tc.Mutation
	id, err := randomTerminalID()
	if err != nil {
		return nil, err
	}
	if req.IdempotencyKey != "" {
		id = generation(struct{ Kind, Key string }{"legacy-setup", req.IdempotencyKey})
		var record repository.TerminalRecord
		if err = c.db.TerminalRecord(ctx, "operation/"+id, &record); err == nil {
			p := record.Mutation.Setup
			if p == nil || p.Root != req.RootPath || p.DeviceName != label || p.FolderName != name {
				return nil, terminalError("IDEMPOTENCY_CONFLICT")
			}
			m = record.Mutation
		} else if !errors.Is(err, repository.ErrOperationNotFound) {
			return nil, err
		}
	}
	if m.OperationID == "" {
		settings, err := config.LoadRuntimeSettings(c.db.StateDir())
		if err != nil {
			return nil, err
		}
		if settings.DataBudget == 0 || settings.MetadataBudget == 0 || settings.ReserveBytes == 0 {
			return nil, terminalError("LIMITS_REVIEW_REQUIRED")
		}
		p := tc.SetupIntent{DeviceName: label, FolderName: name, Root: req.RootPath, Settings: settings}
		q := tc.Query{Version: "1", Kind: "root_preview", Path: p.Root, RootPlan: &p}
		var preview tc.Result
		for {
			preview, err = c.TerminalQuery(ctx, q)
			if err != nil {
				return nil, err
			}
			if preview.Preview.Complete {
				break
			}
			q.Cursor = preview.Cursor
		}
		p.Preview = *preview.Review
		m = tc.Mutation{Version: "1", Kind: "setup", OperationID: id, Setup: &p}
	}
	r, err := c.TerminalMutate(ctx, m)
	if err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, &ControlError{Code: r.Error.Code, Message: r.Error.Message, Action: r.Error.Action, Retryable: r.Error.Retryable}
	}
	cfg, err := config.Load(c.db.StateDir())
	if err != nil {
		return nil, err
	}
	return &StartSetupResult{OperationID: r.Operation.ID, DeviceID: cfg.DeviceID, FolderID: terminalID(r.Join.Folder), RootPath: r.Join.Root, Phase: compatibilitySetupPhase(r.Operation.Phase), Message: "reviewed setup and local capture observed"}, nil
}
