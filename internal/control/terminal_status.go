package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
)

func attentionID(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func parseFolderHex(s string) (history.ID, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		return history.ID{}, fmt.Errorf("invalid folder ID %q", s)
	}
	var id history.ID
	copy(id[:], b)
	return id, nil
}

// FolderReadiness computes qualified, truthful readiness for a registered folder
// without hidden scans, GC, or membership mutations.
func (c *Controller) FolderReadiness(ctx context.Context, folder history.ID) (tc.Readiness, error) {
	var r tc.Readiness
	if err := ctx.Err(); err != nil {
		return r, err
	}

	// 1. Membership approval & current
	active, _, _, _, err := c.db.PeerMembers(ctx, folder)
	if err == nil {
		if c.options.LocalDevice == ([32]byte{}) {
			r.Approved = true
		} else {
			for _, m := range active {
				if m.Device == c.options.LocalDevice {
					r.Approved = true
					break
				}
			}
		}
	}
	hasFork, _ := c.db.HasMembershipFork(ctx, folder)
	if !hasFork && r.Approved {
		r.MembershipCurrent = true
	}

	// 2. Root availability
	reg, err := c.db.Root(ctx, folder)
	if err == nil && !reg.Paused {
		if info, statErr := os.Stat(reg.Path); statErr == nil && info.IsDir() {
			if revalErr := c.ws.Revalidate(ctx, folder); revalErr == nil {
				r.RootAvailable = true
			}
		}
	}

	// 3. Blocked paths from path_projections
	blocked, err := c.db.BlockedPaths(ctx, folder)
	if err == nil {
		for _, b := range blocked {
			low := strings.ToLower(b.Reason)
			if strings.Contains(low, "unsupported") {
				r.Unsupported++
			} else if strings.Contains(low, "unreadable") || strings.Contains(low, "permission") {
				r.Unreadable++
			}
		}
	}

	// 4. Scan complete if root available and no blocked objects
	if r.RootAvailable && r.Unsupported == 0 && r.Unreadable == 0 {
		r.ScanComplete = true
	}

	// 5. OnboardingReadiness fills conflicts, missing content, pending publication, uncaptured
	if err := c.db.OnboardingReadiness(ctx, folder, &r); err != nil {
		return r, err
	}

	// 6. Storage blocked check
	if usage, err := c.db.DetailedStorageUsage(ctx); err == nil {
		if (usage.DataBudgetBytes > 0 && usage.ObjectBytes > usage.DataBudgetBytes) ||
			(usage.MetadataBudgetBytes > 0 && usage.MetadataBytes > usage.MetadataBudgetBytes) ||
			(usage.StateFilesystem.AvailableBytes < usage.FreeSpaceReserveBytes) {
			r.StorageBlocked = true
		}
	}

	return r, nil
}

func (c *Controller) terminalAttention(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	if err := ctx.Err(); err != nil {
		return r, err
	}

	var allAttention []tc.Attention

	// Determine folders to inspect
	var foldersToCheck []history.ID
	if q.Folder != "" {
		fID, err := parseFolderHex(q.Folder)
		if err != nil {
			return r, err
		}
		foldersToCheck = append(foldersToCheck, fID)
	} else {
		registered, err := c.db.RegisteredFolders(ctx)
		if err != nil {
			return r, err
		}
		for _, reg := range registered {
			foldersToCheck = append(foldersToCheck, reg.Folder)
		}
	}

	now := c.options.Now()

	// 1. Folder-specific attention items
	for _, folder := range foldersToCheck {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		folderHex := hex.EncodeToString(folder[:])

		// A. Root availability and paused state
		reg, regErr := c.db.Root(ctx, folder)
		if regErr == nil {
			if reg.Paused {
				allAttention = append(allAttention, tc.Attention{
					ID:     attentionID("folder_paused", folderHex),
					Folder: folderHex,
					Code:   "FOLDER_PAUSED",
					Action: fmt.Sprintf("Resume folder with 'orbit folders resume %s'", folderHex),
				})
			} else if _, statErr := os.Stat(reg.Path); errors.Is(statErr, os.ErrNotExist) {
				allAttention = append(allAttention, tc.Attention{
					ID:     attentionID("root_unavailable", folderHex),
					Folder: folderHex,
					Code:   "ROOT_UNAVAILABLE",
					Action: "Restore access to the root or relocate with 'orbit folders relocate'",
				})
			} else if revalErr := c.ws.Revalidate(ctx, folder); revalErr != nil {
				allAttention = append(allAttention, tc.Attention{
					ID:     attentionID("stale_root", folderHex),
					Folder: folderHex,
					Code:   "STALE_ROOT",
					Action: "Revalidate root with 'orbit folders relocate'",
				})
			}
		}

		// B. Blocked paths in path_projections
		blocked, bErr := c.db.BlockedPaths(ctx, folder)
		if bErr == nil {
			for _, b := range blocked {
				allAttention = append(allAttention, tc.Attention{
					ID:     attentionID("blocked_path", folderHex, b.Path),
					Folder: folderHex,
					Path:   b.Path,
					Code:   "BLOCKED_PATH",
					Action: fmt.Sprintf("Fix unreadable or unsupported object: %s", b.Reason),
				})
			}
		}

		// C. Content conflicts
		conflicts, cErr := c.db.Conflicts(ctx, folder)
		if cErr == nil {
			for _, cf := range conflicts {
				allAttention = append(allAttention, tc.Attention{
					ID:     attentionID("conflict", folderHex, cf.Path),
					Folder: folderHex,
					Path:   cf.Path,
					Code:   "CONFLICT",
					Action: fmt.Sprintf("Review competing versions with 'orbit conflicts resolve %s'", cf.Path),
				})
			}
		}

		// D. Structural conflicts
		sConflicts, sErr := c.db.StructuralConflicts(ctx, folder)
		if sErr == nil {
			for _, sc := range sConflicts {
				allAttention = append(allAttention, tc.Attention{
					ID:     attentionID("structural_conflict", folderHex, sc.DescendantPath),
					Folder: folderHex,
					Path:   sc.DescendantPath,
					Code:   "STRUCTURAL_CONFLICT",
					Action: fmt.Sprintf("Review structural conflict with 'orbit conflicts resolve %s'", sc.DescendantPath),
				})
			}
		}

		// E. Membership forks
		if hasFork, _ := c.db.HasMembershipFork(ctx, folder); hasFork {
			allAttention = append(allAttention, tc.Attention{
				ID:     attentionID("membership_fork", folderHex),
				Folder: folderHex,
				Code:   "MEMBERSHIP_FORK",
				Action: "Review competing membership revisions.",
			})
		}

		// F. Offline peers (>24h without contact)
		progress, pErr := c.db.PeerProgress(ctx, folder)
		if pErr == nil {
			for _, p := range progress {
				if !p.LastContact.IsZero() && now.Sub(p.LastContact) > 24*time.Hour {
					peerHex := hex.EncodeToString(p.Peer[:])
					allAttention = append(allAttention, tc.Attention{
						ID:     attentionID("offline", folderHex, peerHex),
						Folder: folderHex,
						Code:   "OFFLINE",
						Action: "Check the configured peer address.",
					})
				}
			}
		}
	}

	// 2. Global attention items (when not scoped to a single folder)
	if q.Folder == "" {
		// A. Exhausted durable tasks
		tasks, tErr := c.db.ListDurableTasks(ctx, repository.TaskFilter{State: "exhausted"})
		if tErr == nil {
			for _, task := range tasks {
				folderHex := ""
				if task.Folder != (history.ID{}) {
					folderHex = hex.EncodeToString(task.Folder[:])
				}
				allAttention = append(allAttention, tc.Attention{
					ID:          attentionID("exhausted_work", task.ID),
					Folder:      folderHex,
					OperationID: task.ID,
					Code:        "EXHAUSTED_WORK",
					Action:      fmt.Sprintf("Retry task with 'filesync work retry --task %s'", task.ID),
				})
			}
		}

		// B. Storage limits
		if usage, uErr := c.db.DetailedStorageUsage(ctx); uErr == nil {
			if usage.StateFilesystem.AvailableBytes < usage.FreeSpaceReserveBytes {
				allAttention = append(allAttention, tc.Attention{
					ID:     attentionID("disk_budget"),
					Code:   "DISK_BUDGET",
					Action: "Free space or review budgets.",
				})
			}
			if usage.DataBudgetBytes > 0 && usage.ObjectBytes > usage.DataBudgetBytes {
				allAttention = append(allAttention, tc.Attention{
					ID:     attentionID("data_budget"),
					Code:   "DISK_BUDGET",
					Action: "Free space or review budgets.",
				})
			}
			if usage.MetadataBudgetBytes > 0 && usage.MetadataBytes > usage.MetadataBudgetBytes {
				allAttention = append(allAttention, tc.Attention{
					ID:     attentionID("metadata_budget"),
					Code:   "METADATA_BUDGET",
					Action: "Checkpoint WAL or review metadata budgets.",
				})
			}
		}

		// C. Pending enrollment requests awaiting approval
		reqResult, rErr := c.terminalRequests(ctx, tc.Query{Version: tc.Version, Kind: "requests", Limit: tc.MaxPage})
		if rErr == nil {
			for _, req := range reqResult.Requests {
				if req.State == "pending" || req.State == "pending_approval" {
					allAttention = append(allAttention, tc.Attention{
						ID:          attentionID("awaiting_approval", req.ID),
						Folder:      req.Folder,
						OperationID: req.ID,
						Code:        "AWAITING_APPROVAL",
						Action:      fmt.Sprintf("Review pending enrollment request from %s with 'orbit devices requests'", req.Requester),
					})
				}
			}
		}
		for _, f := range foldersToCheck {
			reqs, lErr := c.db.ListEnrollmentRequests(ctx, f, "pending")
			if lErr == nil {
				for _, req := range reqs {
					folderHex := hex.EncodeToString(req.Folder[:])
					label := req.SuggestedLabel
					if label == "" {
						label = hex.EncodeToString(req.DeviceID[:8])
					}
					allAttention = append(allAttention, tc.Attention{
						ID:          attentionID("awaiting_approval", req.RequestID),
						Folder:      folderHex,
						OperationID: req.RequestID,
						Code:        "AWAITING_APPROVAL",
						Action:      fmt.Sprintf("Review pending enrollment request from %s with 'orbit devices requests'", label),
					})
				}
			}
		}

		// D. Incomplete setup operations
		ops, oErr := c.db.TerminalOperations(ctx)
		if oErr == nil {
			for _, op := range ops {
				if (op.Mutation.Kind == "setup" || op.Mutation.Kind == "adopt" || op.Mutation.Kind == "join") &&
					(op.Result.Operation != nil && (op.Result.Operation.State == "pending" || op.Result.Operation.State == "running" || op.Result.Operation.State == "blocked" || op.Result.Operation.State == "partial")) {
					allAttention = append(allAttention, tc.Attention{
						ID:          attentionID("incomplete_setup", op.Mutation.OperationID),
						OperationID: op.Mutation.OperationID,
						Code:        "INCOMPLETE_SETUP",
						Action:      "Resume setup with 'orbit setup' or 'orbit join'",
					})
				}
			}
		}
	}

	sessions, err := c.db.TerminalSessions(ctx)
	if err != nil {
		return r, err
	}
	for _, session := range sessions {
		if q.Folder != "" && session.Context.Folder != q.Folder {
			continue
		}
		expiry, e := time.Parse(time.RFC3339Nano, session.ExpiresAt)
		if session.State == "recovery" || (session.State == "active" && (e != nil || !now.Before(expiry))) {
			allAttention = append(allAttention, tc.Attention{ID: session.ID, Folder: session.Context.Folder, Path: session.Context.Path, Code: "EDITOR_RECOVERY", Action: "inspect retained editor result with orbit conflicts session --session " + session.ID})
		}
	}

	// 3. Deterministic sort
	sort.Slice(allAttention, func(i, j int) bool {
		if allAttention[i].Folder != allAttention[j].Folder {
			return allAttention[i].Folder < allAttention[j].Folder
		}
		if allAttention[i].Path != allAttention[j].Path {
			return allAttention[i].Path < allAttention[j].Path
		}
		if allAttention[i].Code != allAttention[j].Code {
			return allAttention[i].Code < allAttention[j].Code
		}
		return allAttention[i].ID < allAttention[j].ID
	})

	// 4. Bounded pagination via Limit and Cursor
	limit := int(q.Limit)
	if limit <= 0 {
		limit = 50
	}
	if limit > tc.MaxPage {
		limit = tc.MaxPage
	}

	start := 0
	if q.Cursor != "" {
		if offset, err := strconv.Atoi(q.Cursor); err == nil && offset >= 0 && offset < len(allAttention) {
			start = offset
		} else {
			for i, it := range allAttention {
				if it.ID == q.Cursor {
					start = i + 1
					break
				}
			}
		}
	}

	end := start + limit
	if end > len(allAttention) {
		end = len(allAttention)
	}

	if start < len(allAttention) {
		r.Attention = allAttention[start:end]
	} else {
		r.Attention = []tc.Attention{}
	}

	if end < len(allAttention) {
		r.Cursor = strconv.Itoa(end)
	} else {
		r.Cursor = ""
	}

	// 5. Compute State based on attention items
	r.State = computeStateFromAttention(allAttention, len(foldersToCheck) == 0)

	return r, nil
}

func computeStateFromAttention(attention []tc.Attention, noFolders bool) string {
	if len(attention) == 0 {
		if noFolders {
			return "empty"
		}
		return "success"
	}
	hasConflict := false
	hasStorageBlocked := false
	hasRootUnavailable := false
	hasStale := false
	hasFork := false
	hasAwaiting := false
	hasOffline := false
	for _, it := range attention {
		switch it.Code {
		case "CONFLICT", "STRUCTURAL_CONFLICT":
			hasConflict = true
		case "DISK_BUDGET", "DATA_BUDGET", "METADATA_BUDGET", "STORAGE_BLOCKED":
			hasStorageBlocked = true
		case "ROOT_UNAVAILABLE":
			hasRootUnavailable = true
		case "STALE_ROOT", "STALE_VIEW":
			hasStale = true
		case "MEMBERSHIP_FORK":
			hasFork = true
		case "AWAITING_APPROVAL":
			hasAwaiting = true
		case "OFFLINE":
			hasOffline = true
		}
	}
	if hasConflict {
		return "conflict"
	}
	if hasStorageBlocked {
		return "storage_blocked"
	}
	if hasRootUnavailable {
		return "root_unavailable"
	}
	if hasStale {
		return "stale"
	}
	if hasFork {
		return "fork"
	}
	if hasAwaiting {
		return "awaiting_approval"
	}
	if hasOffline {
		return "offline"
	}
	return "partial"
}

func (c *Controller) terminalStatus(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	if err := ctx.Err(); err != nil {
		return r, err
	}

	// 1. Attention items
	attRes, err := c.terminalAttention(ctx, q)
	if err != nil {
		return r, err
	}
	r.Attention = attRes.Attention
	r.Cursor = attRes.Cursor

	// 2. Folders / Items
	foldersRes, err := c.terminalFolders(ctx, q)
	if err != nil {
		return r, err
	}
	r.Items = foldersRes.Items

	// 3. Service status
	running := !c.options.StoppedAdapter
	svcStatus, _ := CheckServiceStatus(ctx, c.db.StateDir(), c.db)
	mode := "manual"
	enabled := false
	unattendedVerified := false
	if svcStatus != nil {
		enabled = svcStatus.EnabledOnLogin || svcStatus.CurrentlyRunning
		unattendedVerified = svcStatus.LingeringEnabled
	}
	if s, sErr := config.LoadRuntimeSettings(c.db.StateDir()); sErr == nil && s.Startup != "" {
		mode = s.Startup
	}

	allRootsHealthy := true
	allCaptureHealthy := true
	for _, it := range r.Items {
		fID, pErr := parseFolderHex(it.ID)
		if pErr == nil {
			rd, rdErr := c.FolderReadiness(ctx, fID)
			if rdErr != nil || !rd.RootAvailable {
				allRootsHealthy = false
			}
			if rdErr != nil || rd.Uncaptured > 0 || rd.MissingContent > 0 || rd.Conflicts > 0 || rd.StorageBlocked {
				allCaptureHealthy = false
			}
		}
	}
	r.Service = &tc.Service{
		Running:            running,
		Enabled:            enabled,
		Mode:               mode,
		UnattendedVerified: unattendedVerified,
		RootHealthy:        allRootsHealthy,
		CaptureHealthy:     allCaptureHealthy,
	}

	// 4. Readiness
	if q.Folder != "" {
		fID, pErr := parseFolderHex(q.Folder)
		if pErr == nil {
			folderRd, rdErr := c.FolderReadiness(ctx, fID)
			if rdErr == nil {
				r.Readiness = &folderRd
			}
		}
	} else if len(r.Items) > 0 {
		var aggRd tc.Readiness
		aggRd.Approved = true
		aggRd.MembershipCurrent = true
		aggRd.RootAvailable = true
		aggRd.ScanComplete = true
		for _, it := range r.Items {
			fID, pErr := parseFolderHex(it.ID)
			if pErr == nil {
				folderRd, rdErr := c.FolderReadiness(ctx, fID)
				if rdErr == nil {
					if !folderRd.Approved {
						aggRd.Approved = false
					}
					if !folderRd.MembershipCurrent {
						aggRd.MembershipCurrent = false
					}
					if !folderRd.RootAvailable {
						aggRd.RootAvailable = false
					}
					if !folderRd.ScanComplete {
						aggRd.ScanComplete = false
					}
					aggRd.Unsupported += folderRd.Unsupported
					aggRd.Unreadable += folderRd.Unreadable
					aggRd.Uncaptured += folderRd.Uncaptured
					aggRd.MissingContent += folderRd.MissingContent
					aggRd.PendingPublication += folderRd.PendingPublication
					aggRd.Conflicts += folderRd.Conflicts
					if folderRd.StorageBlocked {
						aggRd.StorageBlocked = true
					}
				}
			}
		}
		r.Readiness = &aggRd
	} else {
		r.Readiness = &tc.Readiness{}
	}

	// 5. Observations
	var observations []tc.Observation
	now := c.options.Now()
	for _, it := range r.Items {
		fID, pErr := parseFolderHex(it.ID)
		if pErr != nil {
			continue
		}
		progress, pErr := c.db.PeerProgress(ctx, fID)
		if pErr != nil {
			continue
		}
		for _, p := range progress {
			avail, _ := c.db.ContentAvailability(ctx, p.Version)
			availStr := "unknown"
			switch avail {
			case repository.ContentReady:
				availStr = "available"
			case repository.ContentPending:
				availStr = "pending"
			case repository.ContentCorrupt:
				availStr = "corrupt"
			case repository.ContentUnavailable:
				availStr = "unavailable"
			}
			isOnline := false
			lastContactTime := ""
			if !p.LastContact.IsZero() {
				lastContactTime = p.LastContact.UTC().Format(time.RFC3339)
				if now.Sub(p.LastContact) < 5*time.Minute {
					isOnline = true
				}
			}
			obs := tc.Observation{
				Device: hex.EncodeToString(p.Peer[:]),
				Folder: it.ID,
				Version: tc.VersionID{
					Folder:  it.ID,
					Author:  hex.EncodeToString(p.Version.Author[:]),
					Counter: tc.Uint(p.Version.Counter),
				},
				Saved:        true,
				Stored:       p.Receipt || p.RemoteState == "STORED" || p.RemoteState == "APPLIED",
				Applied:      p.RemoteState == "APPLIED",
				Direct:       p.Direct,
				ObservedAt:   now.UTC().Format(time.RFC3339),
				LastContact:  lastContactTime,
				Availability: availStr,
				Online:       isOnline,
			}
			observations = append(observations, obs)
		}
	}
	sort.Slice(observations, func(i, j int) bool {
		if observations[i].Folder != observations[j].Folder {
			return observations[i].Folder < observations[j].Folder
		}
		if observations[i].Device != observations[j].Device {
			return observations[i].Device < observations[j].Device
		}
		return observations[i].Version.Counter < observations[j].Version.Counter
	})
	r.Observations = observations

	// 6. Set overall state
	r.State = computeStateFromAttention(r.Attention, len(r.Items) == 0)

	return r, nil
}

func (c *Controller) terminalDoctor(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	if err := ctx.Err(); err != nil {
		return r, err
	}

	report, err := c.Doctor(ctx)
	if err != nil {
		return r, err
	}

	var attention []tc.Attention
	for _, ch := range report.Checks {
		if ch.Status != StatusOk {
			attention = append(attention, tc.Attention{
				ID:     attentionID("doctor", ch.Category, ch.Name),
				Code:   ch.Name,
				Action: ch.Remediation,
			})
		}
	}
	r.Attention = attention

	if report.OverallStatus == StatusFail {
		r.State = "failed"
		r.Error = &tc.Error{
			Code:      "DOCTOR_FAILED",
			Message:   "one or more diagnostic health checks reported failure",
			Retryable: false,
			Action:    "run 'orbit doctor' and follow remediation instructions",
		}
	} else if report.OverallStatus == StatusWarn {
		r.State = "partial"
	} else {
		r.State = "success"
	}

	return r, nil
}
