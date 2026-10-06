package control

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

type rootReview struct {
	Walk workspace.AdoptionWalk `json:"walk"`
	Plan tc.SetupIntent         `json:"plan"`
	Kind string                 `json:"kind"`
	Used string                 `json:"used"`
}
type setupJob struct {
	Routed      *protocol.RoutedEnrollmentRequest `json:"routed,omitempty"`
	NextContact string                            `json:"next_contact"`
	Folder      string                            `json:"folder"`
	Root        workspace.AdoptionWalk            `json:"root"`
	Wire        *protocol.TerminalEnrollmentWire  `json:"wire,omitempty"`
	Membership  *protocol.Membership              `json:"membership,omitempty"`
	Registered  bool                              `json:"registered"`
	Configured  bool                              `json:"configured"`
	Captured    bool                              `json:"captured"`
}

func randomTerminalID() (string, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func setupPlan(m tc.Mutation) tc.SetupIntent {
	if m.Setup != nil {
		return *m.Setup
	}
	p := m.Join
	return tc.SetupIntent{DeviceName: p.DeviceName, FolderName: p.FolderName, Root: p.Root, Network: p.Network, Preview: p.Preview, Settings: p.Settings}
}
func (c *Controller) terminalRootPreview(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	var saved rootReview
	var key string
	if q.Cursor != "" {
		if _, err := hex.DecodeString(q.Cursor); err != nil || len(q.Cursor) != 64 {
			return r, terminalError("INVALID_REQUEST")
		}
		key = q.Cursor
		if err := c.db.TerminalRecord(ctx, "rootreview/"+key, &saved); err != nil {
			return r, err
		}
		if saved.Walk.Preview.Root != q.Path {
			return r, terminalError("STALE_VIEW")
		}
	} else {
		plan := tc.SetupIntent{DeviceName: "Orbit Device", FolderName: "Orbit", Root: q.Path}
		var err error
		plan.Settings, err = config.LoadRuntimeSettings(c.db.StateDir())
		if err != nil {
			return r, err
		}
		if q.RootPlan != nil {
			plan = *q.RootPlan
			plan.Preview = tc.Review{}
			if plan.Root != q.Path {
				return r, terminalError("INVALID_REQUEST")
			}
		}
		if err = config.ValidateRuntimeSettings(plan.Settings); err != nil {
			return r, err
		}
		// Reuse product protected-path validation, then owning descriptor traversal.
		prev, err := c.previewRoot(ctx, PreviewCreateRootRequest{Path: q.Path}, true)
		if err != nil {
			return r, err
		}
		if prev.Disallowed {
			return r, terminalError("INVALID_ROOT")
		}
		walk, err := c.ws.BeginAdoption(ctx, q.Path)
		if err != nil {
			return r, err
		}
		key, err = randomTerminalID()
		if err != nil {
			return r, err
		}
		kind := q.Name
		if kind == "" {
			kind = "setup"
		}
		if kind != "setup" && kind != "adopt" && kind != "join" {
			return r, terminalError("INVALID_REQUEST")
		}
		saved = rootReview{Walk: walk, Plan: plan, Kind: kind}
		saved.Walk.Preview.Review = tc.Review{Token: key, ExpiresAt: c.options.Now().Add(300 * time.Second).UTC().Format(time.RFC3339Nano)}
		if err = c.db.SaveTerminalRecord(ctx, "rootreview/"+key, saved, true); err != nil {
			return r, err
		}
	}
	expires, _ := time.Parse(time.RFC3339Nano, saved.Walk.Preview.Review.ExpiresAt)
	if saved.Used != "" || !c.options.Now().Before(expires) {
		return r, terminalError("STALE_VIEW")
	}
	saved.Walk.Preview.Issues = []tc.Issue{} // issues page for this slice, totals retained
	if err := c.ws.AdoptionSlice(ctx, &saved.Walk); err != nil {
		return r, err
	}
	p := &saved.Walk.Preview
	p.Review.ExpiresAt = c.options.Now().Add(300 * time.Second).UTC().Format(time.RFC3339Nano)
	p.Review.Generation = generation(struct {
		Tree string
		Plan tc.SetupIntent
		Kind string
	}{saved.Walk.Generation(), saved.Plan, saved.Kind})
	avail, err := c.db.AdoptionCapacity(ctx, p.Root, uint64(p.Bytes), uint64(p.Files+p.Directories), saved.Plan.Settings)
	p.CapacityObservedAt = c.options.Now().UTC().Format(time.RFC3339Nano)
	p.CapacityKnown = err == nil
	p.AvailableBytes = tc.Uint(avail)
	if err != nil {
		r.State = "blocked"
		r.Error = &tc.Error{Code: "STORAGE_BLOCKED", Message: err.Error(), Action: "review finite budgets and available capacity"}
	}
	if !p.Complete {
		p.Cursor = key
		r.Cursor = key
		r.State = "running"
	} else {
		p.Cursor = ""
	}
	if err := c.db.SaveTerminalRecord(ctx, "rootreview/"+key, saved, false); err != nil {
		return r, err
	}
	r.Preview = p
	r.Review = &p.Review
	return r, nil
}
func (c *Controller) terminalSetupMutation(ctx context.Context, m tc.Mutation) (tc.Result, error) {
	if m.Join != nil {
		copyJoin := *m.Join
		m.Join = &copyJoin
	}
	fp, err := m.Fingerprint()
	if err != nil {
		return tc.Result{}, err
	}
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	var old repository.TerminalRecord
	if err = c.db.TerminalRecord(ctx, "operation/"+m.OperationID, &old); err == nil {
		r, err := c.replayTerminal(old, fp)
		if err != nil {
			return r, err
		}
		if old.Result.Operation.State != "completed" {
			return c.advanceSetup(ctx, old)
		}
		return r, nil
	} else if !errors.Is(err, repository.ErrOperationNotFound) {
		return tc.Result{}, err
	}
	if c.options.StoppedAdapter && m.Kind == "join" {
		return tc.Result{}, terminalError("DAEMON_REQUIRED")
	}
	plan := setupPlan(m)
	if plan.Network != nil {
		current, e := config.LoadNetworkPolicy(c.db.StateDir())
		if e != nil {
			return tc.Result{}, e
		}
		if *plan.Network != current && (current.Generation == ^tc.Uint(0) || plan.Network.Generation != current.Generation+1) {
			return tc.Result{}, terminalError("STALE_VIEW")
		}
		if e = c.validateNetworkIntent(tc.NetworkIntent{Policy: *plan.Network}); e != nil {
			return tc.Result{}, e
		}
	}
	var saved rootReview
	if err = c.db.TerminalRecord(ctx, "rootreview/"+plan.Preview.Token, &saved); err != nil {
		return tc.Result{}, terminalError("STALE_VIEW")
	}
	expiry, _ := time.Parse(time.RFC3339Nano, saved.Walk.Preview.Review.ExpiresAt)
	bare := plan
	bare.Preview = tc.Review{}
	if saved.Used != "" || saved.Kind != m.Kind || generation(saved.Plan) != generation(bare) || saved.Walk.Preview.Review != plan.Preview || !c.options.Now().Before(expiry) {
		return tc.Result{}, terminalError("STALE_VIEW")
	}
	if !saved.Walk.Preview.Complete || saved.Walk.Preview.Unsupported != 0 || saved.Walk.Preview.Unreadable != 0 {
		return tc.Result{}, terminalError("ROOT_REVIEW_INCOMPLETE")
	}
	if err = c.revalidateAdoption(ctx, saved.Walk); err != nil {
		return tc.Result{}, err
	}
	if _, err = c.db.AdoptionCapacity(ctx, plan.Root, uint64(saved.Walk.Preview.Bytes), uint64(saved.Walk.Preview.Files+saved.Walk.Preview.Directories), plan.Settings); err != nil {
		return tc.Result{}, err
	}
	folder, err := randomTerminalID()
	if err != nil {
		return tc.Result{}, err
	}
	if m.Kind == "join" {
		folder = m.Join.Invitation.Folder
	}
	job := setupJob{Folder: folder, Root: saved.Walk}
	r := terminalResult()
	r.State = "running"
	r.Operation = &tc.Operation{ID: m.OperationID, Fingerprint: fp, Kind: m.Kind, State: "running", Phase: "reviewed", CommittedEffects: []tc.Effect{}}
	r.Join = &tc.JoinRecord{Operation: *r.Operation, Folder: folder, Root: plan.Root, Preview: plan.Preview}
	if m.Join != nil {
		inv := m.Join.Invitation
		r.Join.Attempt = m.Join.Attempt
		r.Join.Inviter = inv.Inviter
		r.Join.KeyPin = inv.KeyPin
		r.Join.CertificateDER = inv.CertificateDER
		r.Join.EnrollmentEndpoint = inv.EnrollmentEndpoint
		r.Join.PeerEndpoint = inv.PeerEndpoint
	}
	cfg, err := config.Load(c.db.StateDir())
	if err != nil {
		return r, err
	}
	record := repository.TerminalRecord{Mutation: m, Result: r, Owner: cfg.DeviceID, CreatedAt: c.options.Now().UTC().Format(time.RFC3339Nano)}
	// Job first is harmless if admission fails; operation identity is authoritative.
	owned, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = c.db.SaveOnboarding(owned, m.OperationID, job, record, true); err != nil {
		return r, err
	}
	folderID := terminalID(folder)
	if err = c.db.SaveSetupState(owned, repository.SetupStateRecord{Phase: "reviewed", RootPath: plan.Root, DefaultFolderID: &folderID}); err != nil {
		return r, err
	}
	if err = c.callHook("terminal.setup.reviewed"); err != nil {
		return r, err
	}
	return c.advanceSetup(owned, record)
}
func (c *Controller) revalidateAdoption(ctx context.Context, want workspace.AdoptionWalk) error {
	next, err := c.ws.BeginAdoption(ctx, want.Preview.Root)
	if err != nil {
		return err
	}
	for !next.Preview.Complete {
		if err = c.ws.AdoptionSlice(ctx, &next); err != nil {
			return err
		}
	}
	if next.Generation() != want.Generation() {
		return terminalError("STALE_VIEW")
	}
	return nil
}
func (c *Controller) saveSetupPhase(ctx context.Context, record *repository.TerminalRecord, job setupJob, phase string) error {
	r := &record.Result
	r.Operation.Phase = phase
	r.Join.Operation = *r.Operation
	if r.Readiness != nil {
		r.Join.Readiness = *r.Readiness
	}
	if err := c.db.SaveOnboarding(ctx, record.Mutation.OperationID, job, *record, false); err != nil {
		return err
	}
	folder := terminalID(job.Folder)
	if err := c.db.SaveSetupState(ctx, repository.SetupStateRecord{Phase: phase, RootPath: r.Join.Root, DefaultFolderID: &folder, Completed: phase == "ready"}); err != nil {
		return err
	}
	return c.callHook("terminal.setup." + phase)
}
func (c *Controller) advanceSetup(ctx context.Context, record repository.TerminalRecord) (tc.Result, error) {
	r := &record.Result
	m := record.Mutation
	plan := setupPlan(m)
	folder := terminalID(r.Join.Folder)
	var job setupJob
	if err := c.db.TerminalRecord(ctx, "setupjob/"+m.OperationID, &job); err != nil {
		return *r, err
	}
	if job.NextContact != "" {
		next, _ := time.Parse(time.RFC3339Nano, job.NextContact)
		if c.options.Now().Before(next) {
			return *r, nil
		}
	}
	r.State = "running"
	r.Operation.State = "running"
	r.Error = nil
	r.Operation.Error = nil
	r.Readiness = &tc.Readiness{}
	block := func(err error) (tc.Result, error) {
		code := "SETUP_BLOCKED"
		if m.Join != nil && !job.Registered {
			job.NextContact = c.options.Now().Add(25 * time.Second).UTC().Format(time.RFC3339Nano)
		}
		if err.Error() == "RATE_LIMITED" {
			expires := ""
			if job.Wire != nil {
				expires = job.Wire.ExpiresUnix
			}
			delay := preparedRequestThrottleDelay(r.Operation.Phase, expires, c.options.Now())
			job.NextContact = c.options.Now().Add(delay).UTC().Format(time.RFC3339Nano)
		}
		switch err.Error() {
		case "STALE_VIEW", "RATE_LIMITED", "PROFILE_MISSING", "PROFILE_EXPIRED", "PROFILE_UNTRUSTED", "PROFILE_MISMATCH", "IDENTITY_MISMATCH", "EXPIRED_ATTEMPT", "EXPIRED_OR_DECLINED_ATTEMPT":
			code = err.Error()
		}
		var se *network.ServiceError
		if errors.As(err, &se) {
			code = se.Code
		}
		action := "inspect operation; correct the cause and resume the same operation, or explicitly review a new attempt"
		var ce *ControlError
		if errors.As(err, &ce) {
			code = ce.Code
			if ce.Action != "" && ce.Action != "inspect state and obtain a fresh review" {
				action = ce.Action
			}
		}
		if code == "SERVICE_UNAVAILABLE" && m.Join != nil && !job.Registered {
			// This device's own service connection is not ready yet (typically
			// just after a daemon restart). Nothing reached the inviter, so its
			// request budget is untouched; retry soon instead of in 25 s.
			job.NextContact = c.options.Now().Add(3 * time.Second).UTC().Format(time.RFC3339Nano)
		}
		if repository.IsAdmissionError(err) {
			code = "STORAGE_BLOCKED"
			r.Readiness.StorageBlocked = true
		}
		r.State = "blocked"
		r.Operation.State = "blocked"
		r.Error = &tc.Error{Code: code, Message: err.Error(), Retryable: code != "STALE_VIEW" && code != "EXPIRED_ATTEMPT" && code != "EXPIRED_OR_DECLINED_ATTEMPT", Action: action}
		r.Operation.Error = r.Error
		if e := c.saveSetupPhase(context.WithoutCancel(ctx), &record, job, r.Operation.Phase); e != nil {
			return *r, e
		}
		return *r, nil
	}
	if !job.Configured {
		if plan.Network != nil {
			// Reviewed Automatic naming the packaged digest installs that profile
			// first; a retry finds it stored and only saves the policy.
			if in := c.resolvePackaged(tc.NetworkIntent{Policy: *plan.Network}); in.Profile != nil {
				s := network.ProfileSelection{Profile: *in.Profile, Authority: in.Authority, Environment: in.Environment, HighestEpoch: in.Profile.Epoch}
				if err := config.SaveNetworkProfile(c.db.StateDir(), s, uint64(c.options.Now().Unix())); err != nil {
					return block(err)
				}
			}
			if err := config.SaveNetworkPolicy(c.db.StateDir(), *plan.Network); err != nil {
				return block(err)
			}
		}
		if err := config.SaveRuntimeSettings(c.db.StateDir(), plan.Settings); err != nil {
			return block(err)
		}
		if err := c.db.ReloadStorageLimits(); err != nil {
			return block(err)
		}
		job.Configured = true
		if err := c.saveSetupPhase(ctx, &record, job, r.Operation.Phase); err != nil {
			return *r, err
		}
	}
	if plan.Settings.Startup != "manual" {
		if err := c.setupStartup(ctx, record); err != nil {
			return block(err)
		}
	}
	if m.Join != nil && job.Membership == nil {
		id, err := replication.LoadOrCreateIdentity(c.db.StateDir(), c.options.LocalDevice, c.options.Now())
		if err != nil {
			return block(err)
		}
		client, err := c.enrollmentClient(ctx, m.Join.Invitation, id)
		if err != nil {
			return block(err)
		}
		defer client.Close()
		if job.Wire == nil {
			if err := c.revalidateAdoption(ctx, job.Root); err != nil {
				return block(err)
			}
			expiry, _ := time.Parse(time.RFC3339Nano, m.Join.Invitation.ExpiresAt)
			if !c.options.Now().Before(expiry) {
				return block(terminalError("EXPIRED_ATTEMPT"))
			}
			endpoint := ""
			if plan.Settings.AdvertisedPeer != "" {
				endpoint = "https://" + plan.Settings.AdvertisedPeer
			}
			if m.Join.Invitation.Version == "3" {
				wire, e := client.PrepareV3(ctx, m.Join.Attempt, plan.DeviceName)
				if e != nil {
					return block(e)
				}
				job.Routed = &wire
				projected := replication.RoutedRecordWire(wire)
				job.Wire = &projected
				request, e := wire.Transcript.RequestID()
				if e != nil {
					return block(e)
				}
				r.Join.Request = request
				code, e := wire.Transcript.VerificationCode()
				if e != nil {
					return block(e)
				}
				bytes, e := wire.Transcript.Canonical()
				if e != nil {
					return block(e)
				}
				r.Requests = []tc.EnrollmentRequest{{ID: request, Folder: wire.Transcript.Folder, Requester: wire.Transcript.Requester.Device, KeyPin: wire.Transcript.Requester.Pin, TranscriptDigest: protocol.NetworkDigest(bytes), VerificationCode: code, State: "pending_approval"}}
			} else {
				wire, e := client.Prepare(ctx, m.Join.Attempt, plan.DeviceName, endpoint)
				if e != nil {
					return block(e)
				}
				job.Wire = &wire
				tr, e := wire.Transcript()
				if e != nil {
					return block(e)
				}
				r.Join.Request = tr.RequestID()
				code, e := tr.VerificationCode()
				if e != nil {
					return block(e)
				}
				digest, e := tr.Digest()
				if e != nil {
					return block(e)
				}
				r.Requests = []tc.EnrollmentRequest{{ID: r.Join.Request, Folder: wire.Folder, Requester: wire.Requester, KeyPin: wire.RequesterPin, TranscriptDigest: hex.EncodeToString(digest[:]), VerificationCode: code, State: "pending_approval"}}
			}

			if err = c.saveSetupPhase(ctx, &record, job, "request_prepared"); err != nil {
				return *r, err
			}
		}
		var recoveredStatus *protocol.TerminalEnrollmentResult
		if r.Operation.Phase == "request_prepared" || r.Operation.Phase == "reviewed" {
			expires, parseErr := strconv.ParseInt(job.Wire.ExpiresUnix, 10, 64)
			if parseErr != nil || c.options.Now().Unix() >= expires {
				// A lost acknowledgement may hide an already accepted request. Inspect by
				// possession before classifying an expired unsent transcript; never renew it.
				status, e := setupEnrollmentStatus(ctx, client, job, r.Join.Request)
				if e != nil {
					if e.Error() == "RATE_LIMITED" || (job.Routed != nil && e.Error() != "UNAUTHORIZED" && e.Error() != "INVITATION_INVALID" && e.Error() != "EXPIRED_REPLAY") {
						// Transport failure cannot establish that an uncertain
						// submission was never accepted. Preserve the signed
						// v3 request and inspect again after reconnection.
						return block(e)
					}
					return block(terminalError("EXPIRED_ATTEMPT"))
				}
				recoveredStatus = &status
			} else {
				submitted, submitErr := setupEnrollmentSubmit(ctx, client, job)
				if submitErr != nil {
					return block(submitErr)
				}
				// The authenticated submit response already reports pending approval.
				// Avoid spending two extra possession-status tokens immediately;
				// lost responses still use the original status recovery path.
				recoveredStatus = &submitted
				if err = c.callHook("terminal.setup.request_accepted"); err != nil {
					return *r, err
				}
			}
			job.Wire.Token = ""
			if job.Routed != nil {
				job.Routed.Capability = ""
			}
			record.Mutation.Join.Invitation.Capability = strings.Repeat("1", 64)
			if err = c.saveSetupPhase(ctx, &record, job, "awaiting_approval"); err != nil {
				return *r, err
			}
		}
		job.NextContact = c.options.Now().Add(25 * time.Second).UTC().Format(time.RFC3339Nano)
		var status protocol.TerminalEnrollmentResult
		if recoveredStatus != nil {
			status = *recoveredStatus
		} else {
			status, err = setupEnrollmentStatus(ctx, client, job, r.Join.Request)
			if err != nil {
				return block(err)
			}
		}
		if status.VerificationCode != "" {
			if len(r.Requests) == 0 {
				r.Requests = []tc.EnrollmentRequest{{ID: r.Join.Request, Folder: r.Join.Folder}}
			}
			r.Requests[0].VerificationCode = status.VerificationCode
			r.Requests[0].State = status.State
		}
		if status.State != "approved" {
			if status.State != "pending_approval" {
				return block(terminalError("EXPIRED_OR_DECLINED_ATTEMPT"))
			}
			if err = c.saveSetupPhase(ctx, &record, job, "awaiting_approval"); err != nil {
				return *r, err
			}
			return *r, nil
		}
		job.NextContact = ""
		b, err := hex.DecodeString(status.MembershipHex)
		if err != nil {
			return block(err)
		}
		membership, err := protocol.DecodeMembership(b)
		if err != nil {
			return block(err)
		}
		if membership.Folder != folder {
			return block(terminalError("FOLDER_MISMATCH"))
		}
		job.Membership = &membership
		if err = c.saveSetupPhase(ctx, &record, job, "membership_received"); err != nil {
			return *r, err
		}
	}
	if m.Join != nil && m.Join.Invitation.Version == "3" {
		inv := m.Join.Invitation
		if err := c.persistLogicalRoute(job.Folder, *inv.Route, inv.CertificateDER); err != nil {
			return block(err)
		}
	}
	var retirementSnapshots []protocol.RetirementSnapshot
	if job.Membership != nil && len(job.Membership.Retired) > 0 {
		if m.Join == nil {
			return block(terminalError("INVALID_REQUEST"))
		}
		var err error
		retirementSnapshots, err = c.onboardingRetirementSnapshots(ctx, *job.Membership, m.Join.Invitation)
		if err != nil {
			return block(err)
		}
	}
	if !job.Registered {
		// Recovery after registration commit but before journal acknowledgement uses
		// the full registration verifier rather than a string-matched error.
		reg, err := c.db.Root(ctx, folder)
		if err == nil {
			if reg.Path != plan.Root || (!job.Root.Missing && (reg.Device != uint64(job.Root.Preview.Device) || reg.Inode != uint64(job.Root.Preview.Inode))) {
				return block(terminalError("STALE_VIEW"))
			}
			if err = c.ws.Revalidate(ctx, folder); err != nil {
				return block(err)
			}
		} else if errors.Is(err, repository.ErrRootNotRegistered) || errors.Is(err, repository.ErrFolderUnknown) {
			if err = c.revalidateAdoption(ctx, job.Root); err != nil {
				return block(err)
			}
			if _, err = c.db.AdoptionCapacity(ctx, plan.Root, uint64(job.Root.Preview.Bytes), uint64(job.Root.Preview.Files+job.Root.Preview.Directories), plan.Settings); err != nil {
				return block(err)
			}
			if err = c.ws.CreateAdoptionRoot(job.Root); err != nil {
				return block(err)
			}
			if err = c.db.EnsureFolder(ctx, folder, c.options.LocalDevice, 1); err != nil {
				return block(err)
			}
			if job.Membership != nil {
				if _, err = c.db.ApproveMembership(ctx, *job.Membership, retirementSnapshots...); err != nil {
					return block(err)
				}
			}
			dev, ino := uint64(job.Root.Preview.Device), uint64(job.Root.Preview.Inode)
			if job.Root.Missing {
				dev, ino = 0, 0
			}
			if _, err = c.ws.RegisterReviewed(ctx, folder, plan.Root, dev, ino); err != nil {
				return block(err)
			}
		} else {
			return block(err)
		}
		job.Registered = true
		if err = c.saveSetupPhase(ctx, &record, job, "bootstrap_capture"); err != nil {
			return *r, err
		}
	}
	if err := c.ws.Revalidate(ctx, folder); err != nil {
		return block(err)
	}
	r.Readiness.RootAvailable = true
	// Membership alone imports no remote file history. Capture uses the approved
	// authored revision before any remote projection.
	if job.Membership != nil {
		if _, err := c.db.ApproveMembership(ctx, *job.Membership, retirementSnapshots...); err != nil {
			return block(err)
		}
	}
	// Local bootstrap precedes remote history import and all publication.
	scan, err := c.ws.Scan(ctx, folder)
	if err != nil {
		r.Readiness.Uncaptured++
		return block(err)
	}
	for _, issue := range scan.Issues {
		switch issue.Code {
		case "UNSUPPORTED_OBJECT", "HARD_LINK_UNSUPPORTED", "NESTED_MOUNT", "INVALID_PATH":
			r.Readiness.Unsupported++
		case "INCOMPLETE_SUBTREE":
			r.Readiness.Unreadable++
		default:
			r.Readiness.Uncaptured++
		}
	}
	r.Readiness.ScanComplete = len(scan.Issues) == 0 && scan.Deletion == nil
	if !r.Readiness.ScanComplete {
		return block(terminalError("SCAN_INCOMPLETE"))
	}
	job.Captured = true
	if job.Membership == nil {
		id, err := replication.LoadOrCreateIdentity(c.db.StateDir(), c.options.LocalDevice, c.options.Now())
		if err != nil {
			return block(err)
		}
		job.Membership = &protocol.Membership{Folder: folder, Revision: 1, Active: []protocol.ActiveMember{{Device: c.options.LocalDevice, KeyPin: id.KeyPin}}}
	}
	if _, err := c.db.ApproveMembership(ctx, *job.Membership, retirementSnapshots...); err != nil {
		return block(err)
	}
	r.Readiness.Approved = true
	r.Readiness.MembershipCurrent = true
	if err = c.db.SetFolderDisplayName(ctx, folder, plan.FolderName); err != nil {
		return block(err)
	}
	if _, err = c.UpdateSettings(ctx, UpdateSettingsRequest{DeviceLabel: &plan.DeviceName, DefaultWorkspace: &job.Folder, WorkspaceNames: map[string]string{job.Folder: plan.FolderName}}); err != nil {
		return block(err)
	}
	if err = c.saveSetupPhase(ctx, &record, job, "content_pending"); err != nil {
		return *r, err
	}
	if m.Join != nil {
		inv := m.Join.Invitation
		der, err := base64.StdEncoding.DecodeString(inv.CertificateDER)
		if err != nil {
			return block(err)
		}
		name := "enrolled-" + inv.KeyPin + ".crt"
		if err = config.WritePrivate(c.db.StateDir(), name, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); err != nil {
			return block(err)
		}
		if inv.Version != "3" {
			if err = config.SetPeerEndpoint(c.db.StateDir(), config.PeerEndpoint{Folder: job.Folder, Device: inv.Inviter, URL: inv.PeerEndpoint, Certificate: filepath.Join(c.db.StateDir(), name)}); err != nil {
				return block(err)
			}
		}
		cert, err := replication.ParsePeerCertificate(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
		if err != nil {
			return block(err)
		}
		id, err := replication.LoadOrCreateIdentity(c.db.StateDir(), c.options.LocalDevice, c.options.Now())
		if err != nil {
			return block(err)
		}
		client, err := c.peerClient(ctx, inv.PeerEndpoint, id, cert, terminalID(inv.Inviter))
		if err != nil {
			return block(err)
		}
		defer client.CloseIdleConnections()
		approved, err := c.db.Membership(ctx, folder)
		if err != nil {
			return block(err)
		}
		// First pull imports/fetches without publishing. Capture diagnostics have
		// already been checked. A second pass publishes through the owning workspace.
		if _, err = replication.NewSyncer(c.db, nil, client, c.options.LocalDevice, terminalID(inv.Inviter), folder, approved, replication.TransferOptions{}).Sync(ctx); err != nil {
			return block(err)
		}
		if err = c.saveSetupPhase(ctx, &record, job, "publishing"); err != nil {
			return *r, err
		}
		if _, err = replication.NewSyncer(c.db, c.ws, client, c.options.LocalDevice, terminalID(inv.Inviter), folder, approved, replication.TransferOptions{}).Sync(ctx); err != nil {
			return block(err)
		}
	}
	if err = c.db.OnboardingReadiness(ctx, folder, r.Readiness); err != nil {
		return block(err)
	}
	if !r.Readiness.Ready() {
		return block(terminalError("READINESS_PENDING"))
	}
	r.State = "completed"
	r.Operation.State = "completed"
	r.Operation.CommittedEffects = []tc.Effect{{Path: plan.Root, State: "local_capture_and_readiness_observed"}}
	record.CompletedAt = c.options.Now().UTC().Format(time.RFC3339Nano)
	if err = c.saveSetupPhase(ctx, &record, job, "ready"); err != nil {
		return *r, err
	}
	return *r, nil
}

// Enrollment's approved membership hashes name the exact retirement artifacts.
// Fetch them through the already-pinned peer interface before importing history.
// Naming the predecessor requests the exact admitted revision even if the
// inviter has since approved another additive enrollment.
func (c *Controller) onboardingRetirementSnapshots(ctx context.Context, membership protocol.Membership, invitation tc.Invitation) ([]protocol.RetirementSnapshot, error) {
	existing, err := c.db.ListRetirementSnapshots(ctx, membership.Folder, membership.Revision)
	if err != nil {
		return nil, err
	}
	validate := func(snapshots []protocol.RetirementSnapshot) error {
		if len(snapshots) != len(membership.Retired) {
			return repository.ErrMembershipMismatch
		}
		for _, member := range membership.Retired {
			matches := 0
			for _, snapshot := range snapshots {
				if snapshot.Folder != membership.Folder || snapshot.RetiredDevice != member.Device {
					continue
				}
				digest, err := protocol.RetirementSnapshotDigest(snapshot)
				if err != nil || digest != member.SnapshotDigest || snapshot.ConfigurationRev != member.RetiredAt-1 {
					return repository.ErrMembershipMismatch
				}
				matches++
			}
			if matches != 1 {
				return repository.ErrMembershipMismatch
			}
		}
		return nil
	}
	if validate(existing) == nil {
		return existing, nil
	}
	if membership.Revision < 2 {
		return nil, repository.ErrMembershipMismatch
	}
	der, err := base64.StdEncoding.DecodeString(invitation.CertificateDER)
	if err != nil {
		return nil, err
	}
	certificate, err := replication.ParsePeerCertificate(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	if err != nil {
		return nil, err
	}
	identity, err := replication.LoadOrCreateIdentity(c.db.StateDir(), c.options.LocalDevice, c.options.Now())
	if err != nil {
		return nil, err
	}
	client, err := c.peerClient(ctx, invitation.PeerEndpoint, identity, certificate, terminalID(invitation.Inviter))
	if err != nil {
		return nil, err
	}
	defer client.CloseIdleConnections()
	response, err := client.MembershipGet(ctx, replication.MembershipGetRequest{ProtocolVersion: replication.ProtocolVersion,
		DeviceID: hex.EncodeToString(c.options.LocalDevice[:]), FolderID: hex.EncodeToString(membership.Folder[:]),
		FromRevision: strconv.FormatUint(membership.Revision-1, 10), ExpectedDigest: hex.EncodeToString(membership.PriorDigest[:])})
	if err != nil {
		return nil, err
	}
	expected, err := protocol.MembershipDigest(membership)
	if err != nil {
		return nil, err
	}
	actual, err := protocol.MembershipDigest(response.Membership)
	if err != nil || response.ProtocolVersion != replication.ProtocolVersion || response.FolderID != hex.EncodeToString(membership.Folder[:]) || actual != expected {
		return nil, repository.ErrMembershipMismatch
	}
	if err = validate(response.Snapshots); err != nil {
		return nil, err
	}
	return response.Snapshots, nil
}

// ResumeSetupJobs is owned by the daemon; client cancellation only stops waiting.
func (c *Controller) ResumeSetupJobs(ctx context.Context) error {
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	if err := c.recoverApprovedRoutes(ctx); err != nil {
		return err
	}
	records, err := c.db.TerminalOperations(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Mutation.Kind != "setup" && record.Mutation.Kind != "adopt" && record.Mutation.Kind != "join" {
			continue
		}
		if record.Result.Operation.State == "completed" {
			continue
		}
		if record.Result.Error != nil && (record.Result.Error.Code == "STALE_VIEW" || record.Result.Error.Code == "EXPIRED_ATTEMPT" || record.Result.Error.Code == "EXPIRED_OR_DECLINED_ATTEMPT") {
			continue
		}
		if _, err = c.advanceSetup(ctx, record); err != nil {
			return fmt.Errorf("resume setup job: %w", err)
		}
	}
	return nil
}

// The setup review binds startup mode. Its child operation has a deterministic
// durable ID, and uses the existing service dispatcher/recovery rules.
func (c *Controller) setupStartup(ctx context.Context, parent repository.TerminalRecord) error {
	plan := setupPlan(parent.Mutation)
	id := generation(struct{ Operation, Action string }{parent.Mutation.OperationID, "setup-enable"})
	var saved repository.TerminalRecord
	err := c.db.TerminalRecord(ctx, "operation/"+id, &saved)
	if err == nil {
		if saved.Result.Operation.State == "completed" {
			return nil
		}
		snapshot, _, e := c.terminalSnapshot(ctx, "service")
		if e != nil {
			return e
		}
		if snapshot.Service.Enabled {
			return nil
		}
		return terminalError("STARTUP_REVIEW_REQUIRED")
	}
	if !errors.Is(err, repository.ErrOperationNotFound) {
		return err
	}
	m := tc.Mutation{Version: tc.Version, OperationID: id, Kind: "service", Service: &tc.ServiceIntent{Action: "enable", Mode: plan.Settings.Startup, Review: plan.Preview}}
	fp, err := m.Fingerprint()
	if err != nil {
		return err
	}
	r := terminalResult()
	r.State = "running"
	r.Operation = &tc.Operation{ID: id, Fingerprint: fp, Kind: "service", State: "running", Phase: "accepted", CommittedEffects: []tc.Effect{}}
	saved = repository.TerminalRecord{Mutation: m, Result: r, Owner: parent.Owner, CreatedAt: c.options.Now().UTC().Format(time.RFC3339Nano)}
	if err = c.db.SaveTerminalRecord(ctx, "operation/"+id, saved, true); err != nil {
		return err
	}
	r, err = c.executeTerminal(ctx, saved)
	if err != nil {
		return err
	}
	if r.Error != nil {
		return &ControlError{Code: r.Error.Code, Message: r.Error.Message, Action: r.Error.Action, Retryable: r.Error.Retryable}
	}
	return nil
}

func setupEnrollmentStatus(ctx context.Context, client *replication.EnrollmentClient, job setupJob, request string) (protocol.TerminalEnrollmentResult, error) {
	if job.Routed == nil {
		return client.Status(ctx, request)
	}
	out, err := client.StatusV3(ctx, *job.Routed)
	return protocol.TerminalEnrollmentResult{Version: out.Version, Request: out.Request, State: out.State, TranscriptDigest: out.TranscriptDigest, VerificationCode: out.VerificationCode, MembershipHex: out.MembershipHex}, err
}
func setupEnrollmentSubmit(ctx context.Context, client *replication.EnrollmentClient, job setupJob) (protocol.TerminalEnrollmentResult, error) {
	if job.Routed == nil {
		return client.Submit(ctx, *job.Wire)
	}
	out, err := client.SubmitV3(ctx, *job.Routed)
	return protocol.TerminalEnrollmentResult{Version: out.Version, Request: out.Request, State: out.State, TranscriptDigest: out.TranscriptDigest, VerificationCode: out.VerificationCode, MembershipHex: out.MembershipHex}, err
}

// Two source-admission tokens refill in 25 seconds. Waiting a full minute after
// a refused prepared submit would expire its one-minute proof before any retry.
// Retain that exact proof; expired or nearly expired requests keep status recovery.
func preparedRequestThrottleDelay(phase, expires string, now time.Time) time.Duration {
	if phase == "request_prepared" {
		deadline, err := strconv.ParseInt(expires, 10, 64)
		if err == nil && time.Unix(deadline, 0).After(now.Add(25*time.Second)) {
			return 25 * time.Second
		}
	}
	return 60 * time.Second
}
