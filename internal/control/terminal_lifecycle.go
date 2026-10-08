package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/state"
)

func terminalResult() tc.Result {
	return tc.Result{Version: tc.Version, Capabilities: []string{tc.Capability, tc.NetworkCapability, tc.NetworkDiagnosticsCapability, "lifecycle_settings_v1", "enrollment_v2", tc.RoutedEnrollmentCapability, tc.ShortPairingCapability, tc.PackagedProfileCapability, "reviewed_setup_v1", "folder_sharing_v1", "context_v1", "reviewed_content_v1", "onboarding_management_v1", "everyday_management_v1"}, State: "completed", Items: []tc.NamedItem{}, Requests: []tc.EnrollmentRequest{}, Observations: []tc.Observation{}, Attention: []tc.Attention{}, Effects: []tc.Effect{}, Versions: []tc.VersionSummary{}}
}
func terminalError(code string) error {
	return &ControlError{Code: code, Message: code, Action: "inspect state and obtain a fresh review"}
}

type terminalReview struct {
	Kind   string    `json:"kind"`
	Review tc.Review `json:"review"`
}

func generation(in any) string {
	b, _ := json.Marshal(in)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (c *Controller) terminalSnapshot(ctx context.Context, kind string) (tc.Result, string, error) {
	r := terminalResult()
	settings, err := config.LoadRuntimeSettings(c.db.StateDir())
	if err != nil {
		return r, "", err
	}
	if kind == "settings" {
		// A fresh device proposes the host-class startup default (EG3); saved
		// settings keep the owner's reviewed choice.
		host := HostStartupFor(ctx, c.db.StateDir())
		r.Host = &host
		if !config.HasRuntimeSettings(c.db.StateDir()) {
			settings.Startup = host.Suggested
		}
		r.Settings = &settings
		if settings.DataBudget == 0 || settings.MetadataBudget == 0 || settings.ReserveBytes == 0 {
			r.State = "blocked"
			r.Error = &tc.Error{Code: "LIMITS_REVIEW_REQUIRED", Message: "existing state has no finite storage limits", Action: "review and save finite settings"}
		}
		return r, generation(settings), nil
	}
	st, err := c.ServiceStatus(ctx)
	if err != nil {
		return r, "", err
	}
	if c.options.StoppedAdapter {
		st.CurrentlyRunning = false
	}
	mode := "manual"
	if st.EnabledOnLogin {
		mode = "login"
		// Unattended without lingering stops at logout, so it is reported as login.
		if settings.Startup == "unattended" && st.LingeringEnabled {
			mode = "unattended"
		}
	}
	r.Service = &tc.Service{Running: st.CurrentlyRunning, Enabled: st.EnabledOnLogin, Mode: mode, UnattendedVerified: false, RootHealthy: st.RootVerified, CaptureHealthy: false, Owner: st.Owner, UnitState: st.UnitState}
	if c.options.StoppedAdapter {
		r.Service.Owner = ""
	}
	return r, generation(r.Service), nil
}
func (c *Controller) TerminalQuery(ctx context.Context, q tc.Query) (tc.Result, error) {
	if err := q.Validate(); err != nil {
		return tc.Result{}, err
	}
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	if err := c.RecoverTerminalOperations(ctx); err != nil {
		return tc.Result{}, err
	}
	r := terminalResult()
	switch q.Kind {
	case "pairing":
		if c.options.NetworkService == nil {
			return r, terminalError("PAIRING_UNAVAILABLE")
		}
		s := c.options.NetworkService.PairingStatus(q.ID)
		r.Pairing = &s
		return r, nil
	case "network_doctor":
		return c.terminalNetworkDoctor(ctx, q)
	case "network_status", "network_preview":
		return c.terminalNetworkQuery(ctx, q)
	case "paths", "storage", "maintenance":
		return c.terminalEveryday(ctx, q)
	case "files":
		return c.terminalFiles(ctx, q)
	case "file_details":
		return c.terminalFileDetails(ctx, q)
	case "setups":
		return c.terminalSetups(ctx, q)
	case "folder_management":
		return c.terminalFolderManagement(ctx, q)
	case "session":
		return c.terminalSessionQuery(ctx, q)
	case "capabilities":
		return r, nil
	case "context":
		return c.terminalContext(ctx, q)
	case "folders":
		return c.terminalFolders(ctx, q)
	case "devices":
		return c.terminalDevices(ctx, q)
	case "root_preview":
		return c.terminalRootPreview(ctx, q)
	case "requests":
		return c.terminalRequests(ctx, q)
	case "operation":
		var record repository.TerminalRecord
		if err := c.db.TerminalRecord(ctx, "operation/"+q.ID, &record); err != nil {
			if !errors.Is(err, repository.ErrOperationNotFound) {
				return r, err
			}
			err = c.db.EnrollmentTransaction(ctx, func(t *repository.EnrollmentTx) error { return t.Get("operation/"+q.ID, &record) })
			if err != nil {
				return r, err
			}
		}
		result, err := c.replayTerminal(record, "")
		if err != nil || result.Error != nil {
			return result, err
		}
		if record.Mutation.Kind == "content" && result.Operation.State == "pending" && result.Operation.Phase == "publication" {
			return c.observeContentPublication(ctx, &record)
		}
		return result, nil
	case "settings", "service":
		r, g, err := c.terminalSnapshot(ctx, q.Kind)
		if err != nil {
			return r, err
		}
		var token [32]byte
		if _, err := rand.Read(token[:]); err != nil {
			return r, err
		}
		review := tc.Review{Token: hex.EncodeToString(token[:]), Generation: g, ExpiresAt: c.options.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339Nano)}
		if err := c.db.SaveTerminalRecord(ctx, "review/"+review.Token, terminalReview{Kind: q.Kind, Review: review}, true); err != nil {
			return r, err
		}
		r.Review = &review
		return r, nil
	case "status":
		return c.terminalStatus(ctx, q)
	case "attention":
		return c.terminalAttention(ctx, q)
	case "history", "deleted", "content_review", "conflicts":
		return c.terminalContentQuery(ctx, q)
	case "doctor":
		return c.terminalDoctor(ctx, q)
	default:
		return r, terminalError("UNSUPPORTED_CAPABILITY")
	}
}
func (c *Controller) replayTerminal(record repository.TerminalRecord, fingerprint string) (tc.Result, error) {
	r := record.Result
	if fingerprint != "" && r.Operation.Fingerprint != fingerprint {
		return r, terminalError("IDEMPOTENCY_CONFLICT")
	}
	when := record.CreatedAt
	if record.CompletedAt != "" {
		when = record.CompletedAt
	}
	created, err := time.Parse(time.RFC3339Nano, when)
	if err != nil {
		return r, err
	}
	if (r.Operation.State == "completed" || r.Operation.State == "failed" || r.Operation.State == "canceled") && c.options.Now().Sub(created) > 24*time.Hour {
		return r, terminalError("EXPIRED_REPLAY")
	}
	return r, nil
}
func (c *Controller) TerminalMutate(ctx context.Context, m tc.Mutation) (tc.Result, error) {
	fingerprint, err := m.Fingerprint()
	if err != nil {
		return tc.Result{}, err
	}
	if m.Kind == "pairing" {
		return c.terminalPairing(ctx, m, fingerprint)
	}
	if m.Kind == "network" {
		return c.terminalNetworkMutation(ctx, m)
	}
	if m.Kind == "cancel" {
		return c.terminalSessionCancel(ctx, m)
	}
	if m.Kind == "session" {
		return c.terminalSessionMutation(ctx, m)
	}
	if m.Kind == "content" {
		return c.terminalContentMutation(ctx, m)
	}
	if m.Kind == "setup" || m.Kind == "adopt" || m.Kind == "join" {
		return c.terminalSetupMutation(ctx, m)
	}
	if m.Kind == "invite" || m.Kind == "share" || m.Kind == "approval" {
		return c.terminalEnrollmentMutation(ctx, m)
	}
	if m.Kind != "settings" && m.Kind != "service" {
		return tc.Result{}, terminalError("UNSUPPORTED_CAPABILITY")
	}
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	var existing repository.TerminalRecord
	err = c.db.TerminalRecord(ctx, "operation/"+m.OperationID, &existing)
	if err == nil {
		return c.replayTerminal(existing, fingerprint)
	}
	if !errors.Is(err, repository.ErrOperationNotFound) {
		return tc.Result{}, err
	}
	var review tc.Review
	if m.Kind == "settings" {
		review = m.Settings.Review
		if err := config.ValidateRuntimeSettings(m.Settings.Settings); err != nil {
			return tc.Result{}, err
		}
	} else {
		review = m.Service.Review
	}
	var saved terminalReview
	if err := c.db.TerminalRecord(ctx, "review/"+review.Token, &saved); err != nil {
		return tc.Result{}, terminalError("STALE_VIEW")
	}
	_, current, err := c.terminalSnapshot(ctx, m.Kind)
	if err != nil {
		return tc.Result{}, err
	}
	expiry, err := time.Parse(time.RFC3339Nano, review.ExpiresAt)
	if err != nil || saved.Kind != m.Kind || saved.Review != review || current != review.Generation || !c.options.Now().Before(expiry) {
		return tc.Result{}, terminalError("STALE_VIEW")
	}
	r := terminalResult()
	r.State = "running"
	r.Operation = &tc.Operation{ID: m.OperationID, Fingerprint: fingerprint, Kind: m.Kind, State: "running", Phase: "accepted", CommittedEffects: []tc.Effect{}}
	cfg, err := config.Load(c.db.StateDir())
	if err != nil {
		return r, err
	}
	record := repository.TerminalRecord{Mutation: m, Result: r, CreatedAt: c.options.Now().UTC().Format(time.RFC3339Nano), Owner: cfg.DeviceID}
	if b, e := state.ReadPrivate(c.db.StateDir(), ".agent.instance", 128); e == nil {
		record.BeforeInstance = strings.TrimSpace(string(b))
	}
	// From this durable admission onward client cancellation changes waiting only.
	owned, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.db.SaveTerminalRecord(owned, "operation/"+m.OperationID, record, true); err != nil {
		return r, err
	}
	if err := c.callHook("terminal.operation.accepted"); err != nil {
		return r, err
	}
	// Start, stop and restart run in the client: the daemon may be the one a
	// start hands over to the unit, and it cannot stop itself mid-request.
	if m.Kind == "service" && (c.options.StoppedAdapter || m.Service.Action == "start" || m.Service.Action == "stop" || m.Service.Action == "restart") {
		r.State = "running"
		return r, nil
	}
	return c.executeTerminal(owned, record)
}
func (c *Controller) executeTerminal(ctx context.Context, record repository.TerminalRecord) (tc.Result, error) {
	m := record.Mutation
	r := record.Result
	var err error
	if m.Kind == "network" {
		err = c.saveNetworkIntent(*m.Network)
		if err == nil {
			r.Operation.CommittedEffects = append(r.Operation.CommittedEffects, tc.Effect{State: "network_saved_restart_required"})
		}
	} else if m.Kind == "settings" {
		err = config.SaveRuntimeSettings(c.db.StateDir(), m.Settings.Settings)
		if err == nil {
			r.Settings = &m.Settings.Settings
			r.Operation.CommittedEffects = append(r.Operation.CommittedEffects, tc.Effect{State: "settings_saved_restart_required"})
		}
	} else {
		var result *ServiceActionResult
		result, err = ExecuteTerminalService(ctx, c.db.StateDir(), *m.Service, nil)
		if err == nil && result != nil && result.Success {
			r.Operation.CommittedEffects = append(r.Operation.CommittedEffects, tc.Effect{State: "service_" + m.Service.Action})
		} else if err == nil {
			err = errors.New("service command returned without the requested observed state")
		}

	}
	r.State = "completed"
	r.Operation.State = "completed"
	r.Operation.Phase = "completed"
	if err != nil {
		r.State = "failed"
		r.Operation.State = "failed"
		r.Operation.Phase = "failed"
		r.Error = &tc.Error{Code: "IO_ERROR", Message: err.Error(), Action: "inspect service/settings and submit a fresh review"}
		var e *ControlError
		if errors.As(err, &e) {
			r.Error.Code = e.Code
			r.Error.Message = e.Message
			r.Error.Action = e.Action
		}
		r.Operation.Error = r.Error
	}
	record.Result = r
	record.CompletedAt = c.options.Now().UTC().Format(time.RFC3339Nano)
	if saveErr := c.db.SaveTerminalRecord(ctx, "operation/"+m.OperationID, record, false); saveErr != nil {
		return r, saveErr
	}
	return r, nil
}

// RecoverTerminalOperations precedes networking and scans. Settings are an atomic
// desired-state write; ambiguous external service effects require observation/review.
func (c *Controller) RecoverTerminalOperations(ctx context.Context) error {
	records, err := c.db.TerminalOperations(ctx)
	if err != nil {
		return err
	}
	cfg, err := config.Load(c.db.StateDir())
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Owner != cfg.DeviceID {
			return fmt.Errorf("terminal ledger owner mismatch")
		}
		if record.Mutation.Kind == "content" || record.Mutation.Kind == "session" || record.Mutation.Kind == "cancel" {
			continue
		}
		if record.Mutation.Kind == "setup" || record.Mutation.Kind == "adopt" || record.Mutation.Kind == "join" {
			continue
		}
		if record.Result.Operation.State != "running" {
			continue
		}
		if record.Mutation.Kind == "settings" || record.Mutation.Kind == "network" {
			if _, err := c.executeTerminal(ctx, record); err != nil {
				return err
			}
		} else {
			snapshot, _, err := c.terminalSnapshot(ctx, "service")
			if err != nil {
				return err
			}
			intent := record.Mutation.Service
			observed := false
			instance := ""
			if b, e := state.ReadPrivate(c.db.StateDir(), ".agent.instance", 128); e == nil {
				instance = strings.TrimSpace(string(b))
			}
			switch intent.Action {
			case "start":
				observed = snapshot.Service.Running
			case "stop":
				observed = !snapshot.Service.Running
			case "enable":
				observed = snapshot.Service.Enabled
			case "disable":
				observed = !snapshot.Service.Enabled
			case "restart":
				observed = snapshot.Service.Running && instance != "" && record.BeforeInstance != "" && instance != record.BeforeInstance
			}
			if observed {
				record.Result.State = "completed"
				record.Result.Operation.State = "completed"
				record.Result.Operation.Phase = "observed_after_recovery"
				record.Result.Service = snapshot.Service
				record.Result.Operation.CommittedEffects = append(record.Result.Operation.CommittedEffects, tc.Effect{State: "service_" + intent.Action + "_observed"})
				record.CompletedAt = c.options.Now().UTC().Format(time.RFC3339Nano)
			} else {
				record.Result.State = "blocked"
				record.Result.Operation.State = "blocked"
				record.Result.Operation.Phase = "reconcile_service"
				record.Result.Error = &tc.Error{Code: "SERVICE_REVIEW_REQUIRED", Message: "service command outcome interrupted", Action: "inspect actual service state before another reviewed action"}
			}
			if err := c.db.SaveTerminalRecord(ctx, "operation/"+record.Mutation.OperationID, record, false); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Server) registerTerminal(mux *http.ServeMux) {
	s.registerTerminalContent(mux)
	mux.HandleFunc("POST /control/terminal/v1/service/claim", func(w http.ResponseWriter, r *http.Request) {
		var req TerminalServiceCompletion
		if err := decodeTerminalBody(w, r, &req); err != nil {
			writeError(w, err)
			return
		}
		granted, err := s.ctrl.ClaimTerminalService(r.Context(), req.ID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Granted bool `json:"granted"`
		}{granted})
	})
	mux.HandleFunc("POST /control/terminal/v1/service/complete", func(w http.ResponseWriter, r *http.Request) {
		var req TerminalServiceCompletion
		if err := decodeTerminalBody(w, r, &req); err != nil {
			writeError(w, err)
			return
		}
		result, err := s.ctrl.CompleteTerminalService(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("POST /control/terminal/v1/query", func(w http.ResponseWriter, r *http.Request) {
		var q tc.Query
		if err := decodeTerminalBody(w, r, &q); err != nil {
			writeError(w, err)
			return
		}
		result, err := s.ctrl.TerminalQuery(r.Context(), q)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("POST /control/terminal/v1/mutate", func(w http.ResponseWriter, r *http.Request) {
		var m tc.Mutation
		if err := decodeTerminalBody(w, r, &m); err != nil {
			writeError(w, err)
			return
		}
		if m.Kind == "pairing" {
			// The bounded PAKE rendezvous may take 45 seconds. Ordinary
			// control calls retain the server's 30-second write deadline.
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(50 * time.Second))
		}
		result, err := s.ctrl.TerminalMutate(r.Context(), m)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}
func decodeTerminalBody(w http.ResponseWriter, r *http.Request, out any) error {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, tc.MaxMetadata))
	if err != nil {
		return PayloadTooLargeError("terminal metadata exceeds bound")
	}
	if err := tc.Decode(b, out); err != nil {
		return &ControlError{Code: "INVALID_REQUEST", Message: err.Error()}
	}
	return nil
}

// ExecuteTerminalService runs after the stopped adapter has relinquished ownership.
// stop hands a daemon started outside the unit over to it; nil refuses instead.
func ExecuteTerminalService(ctx context.Context, dir string, intent tc.ServiceIntent, stop DaemonStopper) (*ServiceActionResult, error) {
	switch intent.Action {
	case "enable":
		if intent.Mode == "unattended" {
			st, err := CheckServiceStatus(ctx, dir, nil)
			if err != nil {
				return nil, err
			}
			if !st.LingeringEnabled {
				return nil, &ControlError{Code: "UNATTENDED_PREREQUISITE", Message: "lingering is not enabled", Action: st.LingeringInstruction}
			}
		}
		bin, _ := os.Executable()
		return EnableService(ctx, dir, bin, nil)
	case "disable":
		return DisableService(ctx, dir, nil)
	case "start":
		return StartService(ctx, dir, nil, stop)
	case "stop":
		return StopService(ctx, dir, nil, stop)
	case "restart":
		return RestartService(ctx, dir, nil, stop)
	}
	return nil, terminalError("INVALID_REQUEST")
}

type TerminalServiceCompletion struct {
	ID      string        `json:"id"`
	Failure *ControlError `json:"failure,omitempty"`
}

func (c *Controller) CompleteTerminalService(ctx context.Context, completion TerminalServiceCompletion) (tc.Result, error) {
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	if err := (tc.Query{Version: tc.Version, Kind: "operation", ID: completion.ID}).Validate(); err != nil {
		return tc.Result{}, err
	}
	var record repository.TerminalRecord
	if err := c.db.TerminalRecord(ctx, "operation/"+completion.ID, &record); err != nil {
		return tc.Result{}, err
	}
	if record.Mutation.Kind != "service" {
		return tc.Result{}, terminalError("INVALID_REQUEST")
	}
	if (record.Result.Operation.State == "completed" && (completion.Failure == nil || record.Result.Operation.Phase != "observed_after_recovery")) || record.Result.Operation.State == "failed" {
		return record.Result, nil
	}
	r := record.Result
	r.Error = nil
	r.Operation.Error = nil
	r.State = "completed"
	r.Operation.State = "completed"
	r.Operation.Phase = "completed"
	if completion.Failure != nil {
		r.State = "failed"
		r.Operation.State = "failed"
		r.Operation.Phase = "failed"
		r.Error = &tc.Error{Code: completion.Failure.Code, Message: completion.Failure.Message, Action: completion.Failure.Action}
		r.Operation.Error = r.Error
		if len(r.Operation.CommittedEffects) > 0 {
			r.State = "partial"
			r.Operation.State = "partial"
		}
	} else {
		snapshot, _, err := c.terminalSnapshot(ctx, "service")
		if err != nil {
			return r, err
		}
		r.Service = snapshot.Service
		r.Operation.CommittedEffects = append(r.Operation.CommittedEffects, tc.Effect{State: "service_" + record.Mutation.Service.Action})
	}
	record.Result = r
	record.CompletedAt = c.options.Now().UTC().Format(time.RFC3339Nano)
	if err := c.db.SaveTerminalRecord(ctx, "operation/"+completion.ID, record, false); err != nil {
		return r, err
	}
	return r, nil
}

// ClaimTerminalService durably fences external dispatch across concurrent retries.
// It performs no recovery: a replayed running/blocked operation cannot reclaim
// dispatch. Startup/query recovery observes effects or requests explicit review.
func (c *Controller) ClaimTerminalService(ctx context.Context, id string) (bool, error) {
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	if err := (tc.Query{Version: tc.Version, Kind: "operation", ID: id}).Validate(); err != nil {
		return false, err
	}
	var record repository.TerminalRecord
	if err := c.db.TerminalRecord(ctx, "operation/"+id, &record); err != nil {
		return false, err
	}
	if record.Mutation.Kind != "service" {
		return false, terminalError("INVALID_REQUEST")
	}
	if record.Result.Operation.State != "running" || record.Result.Operation.Phase != "accepted" {
		return false, nil
	}
	record.Result.Operation.Phase = "external_dispatched"
	if err := c.db.SaveTerminalRecord(ctx, "operation/"+id, record, false); err != nil {
		return false, err
	}
	return true, nil
}
