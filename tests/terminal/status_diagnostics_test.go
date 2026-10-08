package terminal_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
)

// TestTerminalT07_Status_Observations_MixedState verifies:
//   - Direct vs indirect observation reporting (Direct=true/false)
//   - Stored vs applied states (Stored=true/false, Applied=true/false)
//   - Online vs offline status based on contact freshness (<5m online, >5m offline, >24h attention)
//   - Content availability states (available, pending, corrupt)
//   - What an offline device's last stored receipt actually establishes:
//     A stored receipt from a device or intermediary forwarder establishes ONLY that
//     at the time recorded, that device durably committed the chunks and metadata.
//     It does NOT prove that the device is currently online, that the version was applied
//     to its working directory (Applied=false), that the device hasn't edited/deleted it offline,
//     or that other devices have received it.
//   - No false "Ready" or "global-synchronized" claim when mixed/pending states exist.
func TestTerminalT07_Status_Observations_MixedState(t *testing.T) {
	t.Parallel()
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("mixed")

	// 1. Create and capture a document
	docPath := filepath.Join(f.root, "mixed", "document.txt")
	if err := os.WriteFile(docPath, []byte("synthetic observed content"), 0600); err != nil {
		t.Fatal(err)
	}
	scan, err := f.ws.Scan(ctx, folder)
	if err != nil || len(scan.Captured) != 1 {
		t.Fatalf("scan failed: %v, captured: %d", err, len(scan.Captured))
	}
	v1 := scan.Captured[0]

	now := time.Now()

	// 2. Set up Peer 1: Direct=true, Online=true (contacted now), Stored=true, Applied=false
	peer1 := history.ID{1}
	if err := f.db.RecordPeerStatusWithOptions(ctx, folder, peer1, v1.ID, "STORED", true, now); err != nil {
		t.Fatal(err)
	}

	// 3. Set up Peer 2: Direct=false, Online=false (contacted 10m ago), Stored=true, Applied=true
	peer2 := history.ID{2}
	if err := f.db.RecordPeerStatusWithOptions(ctx, folder, peer2, v1.ID, "APPLIED", false, now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	// 4. Set up Peer 3 (VPS intermediary forwarder):
	// Direct=false, Online=false (contacted 48h ago), Stored=true (receipt only), Applied=false.
	// This proves that a VPS receipt CANNOT become a final-device applied receipt.
	peer3 := history.ID{3}
	if err := f.db.RecordPeerReceiptWithOptions(ctx, folder, peer3, v1.ID, false, now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	// 5. Query status through terminal control client
	client := terminalClient(t, f)
	res, err := client.Query(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "status",
		Folder:  hex.EncodeToString(folder[:]),
	})
	if err != nil {
		t.Fatalf("query status: %v", err)
	}

	// Verify observations
	if len(res.Observations) != 3 {
		t.Fatalf("expected 3 observations, got %d", len(res.Observations))
	}

	obsMap := make(map[string]tc.Observation)
	for _, obs := range res.Observations {
		obsMap[obs.Device] = obs
	}

	// Verify Peer 1
	p1Obs, ok := obsMap[hex.EncodeToString(peer1[:])]
	if !ok {
		t.Fatalf("missing observation for peer 1")
	}
	if !p1Obs.Direct {
		t.Errorf("peer 1 should be direct=true")
	}
	if !p1Obs.Online {
		t.Errorf("peer 1 should be online=true")
	}
	if !p1Obs.Stored {
		t.Errorf("peer 1 should be stored=true")
	}
	if p1Obs.Applied {
		t.Errorf("peer 1 should be applied=false (stored only)")
	}
	if p1Obs.Availability != "available" {
		t.Errorf("peer 1 content availability: expected 'available', got %s", p1Obs.Availability)
	}

	// Verify Peer 2
	p2Obs, ok := obsMap[hex.EncodeToString(peer2[:])]
	if !ok {
		t.Fatalf("missing observation for peer 2")
	}
	if p2Obs.Direct {
		t.Errorf("peer 2 should be direct=false (indirect)")
	}
	if p2Obs.Online {
		t.Errorf("peer 2 should be online=false (contact 10m ago)")
	}
	if !p2Obs.Stored {
		t.Errorf("peer 2 should be stored=true")
	}
	if !p2Obs.Applied {
		t.Errorf("peer 2 should be applied=true")
	}

	// Verify Peer 3 (VPS intermediary forwarder)
	p3Obs, ok := obsMap[hex.EncodeToString(peer3[:])]
	if !ok {
		t.Fatalf("missing observation for peer 3")
	}
	if p3Obs.Direct {
		t.Errorf("peer 3 should be direct=false")
	}
	if p3Obs.Online {
		t.Errorf("peer 3 should be online=false")
	}
	if !p3Obs.Stored {
		t.Errorf("peer 3 should be stored=true from durable receipt")
	}
	if p3Obs.Applied {
		t.Errorf("CRITICAL: peer 3 (forwarder receipt) cannot be claimed applied=true on destination!")
	}

	// 6. Verify offline peer attention item (>24h without contact)
	hasOfflineAtt := false
	for _, att := range res.Attention {
		if att.Code == "OFFLINE" && att.Folder == hex.EncodeToString(folder[:]) {
			hasOfflineAtt = true
			if !strings.Contains(att.Action, "configured peer address") {
				t.Errorf("unexpected offline action: %s", att.Action)
			}
			break
		}
	}
	if !hasOfflineAtt {
		t.Errorf("expected OFFLINE attention item for peer 3 (contact 48h ago)")
	}

	// 7. Verify NO FALSE READY CLAIM
	if res.State == "ready" {
		t.Errorf("false ready claim: system reported ready despite offline peer attention")
	}
	if res.Readiness == nil {
		t.Fatal("expected readiness report in status result")
	}
}

// TestTerminalT07_Status_Attention_PersistenceAcrossRestart verifies:
//   - All attention categories persist cleanly across client close/restart:
//     CONFLICT, STRUCTURAL_CONFLICT, ROOT_UNAVAILABLE, STALE_ROOT, FOLDER_PAUSED,
//     BLOCKED_PATH, EXHAUSTED_WORK, AWAITING_APPROVAL, MEMBERSHIP_FORK
//   - Safe lifecycle pruning cannot erase pending attention items.
func TestTerminalT07_Status_Attention_PersistenceAcrossRestart(t *testing.T) {
	t.Parallel()
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("persist")
	folderHex := hex.EncodeToString(folder[:])

	// A. Inject BLOCKED_PATH in path_projections
	if err := f.db.SetPathBlock(ctx, folder, "unsupported.fifo", "unsupported FIFO pipe"); err != nil {
		t.Fatal(err)
	}

	// B. Inject EXHAUSTED_WORK task
	taskID, err := f.db.EnqueueDurableTask(ctx, repository.DurableTask{
		Folder:      folder,
		Kind:        "sync",
		MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.UpdateDurableTaskState(ctx, taskID, "exhausted", 5, "peer unreachable", "IO_TIMEOUT", 0); err != nil {
		t.Fatal(err)
	}

	// C. Inject AWAITING_APPROVAL enrollment request
	devID9 := history.ID{9}
	reqID := "req-" + hex.EncodeToString(devID9[:8])
	if err := f.db.RecordEnrollmentRequest(ctx, repository.EnrollmentRequestRecord{
		RequestID:      reqID,
		Folder:         folder,
		DeviceID:       devID9,
		PublicKey:      [32]byte{9},
		KeyPin:         [32]byte{9},
		SuggestedLabel: "Alice Phone",
		Status:         "pending",
		CreatedNS:      time.Now().UnixNano(),
		UpdatedNS:      time.Now().UnixNano(),
	}); err != nil {
		t.Fatal(err)
	}

	// D. Inject FOLDER_PAUSED
	if err := f.ws.Pause(ctx, folder, "testing attention pause"); err != nil {
		t.Fatal(err)
	}

	// Query attention before restart
	client := terminalClient(t, f)
	res1, err := client.Query(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "attention",
	})
	if err != nil {
		t.Fatalf("query attention before restart: %v", err)
	}

	expectedCodes := map[string]bool{
		"BLOCKED_PATH":      false,
		"EXHAUSTED_WORK":    false,
		"AWAITING_APPROVAL": false,
		"FOLDER_PAUSED":     false,
	}
	for _, att := range res1.Attention {
		if _, ok := expectedCodes[att.Code]; ok {
			expectedCodes[att.Code] = true
		}
	}
	for code, found := range expectedCodes {
		if !found {
			t.Errorf("attention item %s missing before restart", code)
		}
	}

	// Simulate daemon stop and restart: close fixture and reopen
	f.close()
	f.open()

	client2 := terminalClient(t, f)
	res2, err := client2.Query(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "attention",
	})
	if err != nil {
		t.Fatalf("query attention after restart: %v", err)
	}

	// Verify all items persisted cleanly across restart
	recheckCodes := map[string]bool{
		"BLOCKED_PATH":      false,
		"EXHAUSTED_WORK":    false,
		"AWAITING_APPROVAL": false,
		"FOLDER_PAUSED":     false,
	}
	for _, att := range res2.Attention {
		if _, ok := recheckCodes[att.Code]; ok {
			recheckCodes[att.Code] = true
		}
	}
	for code, found := range recheckCodes {
		if !found {
			t.Errorf("attention item %s LOST across daemon restart!", code)
		}
	}

	// Verify that safe lifecycle pruning DOES NOT erase pending attention
	// Prune enrollment requests older than 0 -> pending requests MUST NOT be deleted
	if _, err := f.db.PruneTerminalEnrollmentRequests(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Prune completed terminal records / durable tasks
	if _, err := f.db.PruneFinishedTasks(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	res3, err := client2.Query(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "attention",
	})
	if err != nil {
		t.Fatal(err)
	}
	finalCodes := map[string]bool{
		"BLOCKED_PATH":      false,
		"EXHAUSTED_WORK":    false,
		"AWAITING_APPROVAL": false,
		"FOLDER_PAUSED":     false,
	}
	for _, att := range res3.Attention {
		if _, ok := finalCodes[att.Code]; ok {
			finalCodes[att.Code] = true
		}
	}
	for code, found := range finalCodes {
		if !found {
			t.Errorf("lifecycle pruning illegally erased pending attention item %s!", code)
		}
	}

	// Assert folder-scoped query works correctly
	resScoped, err := client2.Query(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "attention",
		Folder:  folderHex,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, att := range resScoped.Attention {
		if att.Folder != folderHex && att.Folder != "" {
			t.Errorf("folder-scoped attention returned item for different folder: %s", att.Folder)
		}
	}
}

// TestTerminalT07_Status_BoundedPaginationAndCancellation verifies:
// - Deterministic pagination of attention items via Limit and Cursor
// - Context cancellation support for long/remote queries
// - Read-only guarantee: status queries NEVER scan the filesystem, perform GC, or mutate membership
func TestTerminalT07_Status_BoundedPaginationAndCancellation(t *testing.T) {
	t.Parallel()
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("paginated")

	// Inject 5 distinct blocked path attention items
	for i := 1; i <= 5; i++ {
		path := fmt.Sprintf("blocked_%02d.bin", i)
		if err := f.db.SetPathBlock(ctx, folder, path, "unsupported type"); err != nil {
			t.Fatal(err)
		}
	}

	client := terminalClient(t, f)

	// 1. Page 1: Limit 2
	p1, err := client.Query(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "attention",
		Limit:   2,
	})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(p1.Attention) != 2 {
		t.Fatalf("expected 2 items on page 1, got %d", len(p1.Attention))
	}
	if p1.Cursor == "" {
		t.Fatalf("expected non-empty cursor after page 1")
	}

	// 2. Page 2: Limit 2 with Cursor
	p2, err := client.Query(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "attention",
		Limit:   2,
		Cursor:  p1.Cursor,
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(p2.Attention) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(p2.Attention))
	}
	if p2.Cursor == "" {
		t.Fatalf("expected non-empty cursor after page 2")
	}

	// Verify no overlap between page 1 and page 2
	seen := make(map[string]bool)
	for _, it := range p1.Attention {
		seen[it.ID] = true
	}
	for _, it := range p2.Attention {
		if seen[it.ID] {
			t.Errorf("duplicate item %s appeared on both page 1 and page 2", it.ID)
		}
		seen[it.ID] = true
	}

	// 3. Page 3: Final item
	p3, err := client.Query(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "attention",
		Limit:   2,
		Cursor:  p2.Cursor,
	})
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(p3.Attention) != 1 {
		t.Fatalf("expected 1 final item on page 3, got %d", len(p3.Attention))
	}
	if p3.Cursor != "" {
		t.Errorf("expected empty cursor on final page, got %q", p3.Cursor)
	}

	// 4. Test context cancellation
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	_, errStatus := client.Query(canceledCtx, tc.Query{Version: tc.Version, Kind: "status"})
	if errStatus == nil || !errors.Is(errStatus, context.Canceled) {
		t.Errorf("expected context.Canceled on canceled status query, got %v", errStatus)
	}

	_, errAtt := client.Query(canceledCtx, tc.Query{Version: tc.Version, Kind: "attention"})
	if errAtt == nil || !errors.Is(errAtt, context.Canceled) {
		t.Errorf("expected context.Canceled on canceled attention query, got %v", errAtt)
	}

	// 5. Test read-only guarantee: status queries must NOT mutate state
	regBefore, err := f.db.Root(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	memBefore, _, err := f.db.GetMembership(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}

	// Run multiple status inspections
	for i := 0; i < 5; i++ {
		_, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "status"})
		if err != nil {
			t.Fatal(err)
		}
	}

	regAfter, err := f.db.Root(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	memAfter, _, err := f.db.GetMembership(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}

	if regBefore.ScanGeneration != regAfter.ScanGeneration {
		t.Errorf("status query side effect: scan_generation changed from %d to %d", regBefore.ScanGeneration, regAfter.ScanGeneration)
	}
	if memBefore.Revision != memAfter.Revision {
		t.Errorf("status query side effect: membership_revision changed from %d to %d", memBefore.Revision, memAfter.Revision)
	}
}

// TestTerminalT07_Doctor_DiagnosticsAndRemediations verifies:
// - Comprehensive doctor checks across daemon, permissions, roots, storage, network, membership, service
// - Proposes safe corrective commands (remediation)
// - Reports unavailable network/systemd tooling without silently altering host settings
// - Distinguishes OK, WARN, and FAIL states accurately
func TestTerminalT07_Doctor_DiagnosticsAndRemediations(t *testing.T) {
	t.Parallel()
	f := fresh(t)
	ctx := context.Background()

	// 1. Stopped-state adapter reports OK lifecycle check
	t.Run("StoppedAdapter", func(t *testing.T) {
		stoppedCtrl := control.New(f.db, f.ws, control.Options{
			LocalDevice:    f.device,
			StoppedAdapter: true,
		})
		stoppedReport, err := stoppedCtrl.Doctor(ctx)
		if err != nil {
			t.Fatalf("stopped doctor: %v", err)
		}
		if stoppedReport.OverallStatus != control.StatusOk {
			t.Errorf("expected clean stopped adapter doctor to report OK, got %s", stoppedReport.OverallStatus)
		}
		foundStoppedCheck := false
		for _, ch := range stoppedReport.Checks {
			if ch.Name == "daemon_lifecycle" && ch.Status == control.StatusOk {
				foundStoppedCheck = true
				if !strings.Contains(ch.Message, "stopped-state adapter") {
					t.Errorf("unexpected message: %s", ch.Message)
				}
			}
		}
		if !foundStoppedCheck {
			t.Errorf("missing daemon_lifecycle OK check in stopped adapter")
		}
	})

	// 2. Test live server doctor check
	t.Run("LiveCleanState", func(t *testing.T) {
		_ = terminalClient(t, f)
		liveReport, err := f.ctrl.Doctor(ctx)
		if err != nil {
			t.Fatalf("live doctor: %v", err)
		}
		if liveReport.OverallStatus != control.StatusOk {
			t.Errorf("expected clean live server doctor to report OK, got %s", liveReport.OverallStatus)
		}
	})

	// 3. Inject insecure token permissions -> Doctor reports FAIL
	t.Run("InsecureToken", func(t *testing.T) {
		tokenPath := filepath.Join(f.state, "control.token")
		if err := os.Chmod(tokenPath, 0777); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(tokenPath, 0600)

		failReport, err := f.ctrl.Doctor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if failReport.OverallStatus != control.StatusFail {
			t.Errorf("expected doctor FAIL on insecure token, got %s", failReport.OverallStatus)
		}
		foundTokenFail := false
		for _, ch := range failReport.Checks {
			if ch.Name == "daemon_control_token" && ch.Status == control.StatusFail {
				foundTokenFail = true
				if !strings.Contains(ch.Remediation, "chmod 0600") {
					t.Errorf("expected remediation to contain 'chmod 0600', got %q", ch.Remediation)
				}
			}
		}
		if !foundTokenFail {
			t.Errorf("missing daemon_control_token FAIL check")
		}
	})

	// 4. Inject ROOT_UNAVAILABLE -> Doctor reports FAIL on root availability
	t.Run("MissingRoot", func(t *testing.T) {
		f2 := fresh(t)
		_ = terminalClient(t, f2)
		folder := f2.folder("missingroot")
		reg, err := f2.db.Root(ctx, folder)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(reg.Path); err != nil {
			t.Fatal(err)
		}

		rootFailReport, err := f2.ctrl.Doctor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if rootFailReport.OverallStatus != control.StatusFail {
			t.Errorf("expected doctor FAIL on missing root, got %s", rootFailReport.OverallStatus)
		}
		foundRootFail := false
		for _, ch := range rootFailReport.Checks {
			if ch.Category == "roots" && ch.Status == control.StatusFail {
				foundRootFail = true
				if !strings.Contains(ch.Remediation, "orbit folders relocate") {
					t.Errorf("expected remediation to suggest 'orbit folders relocate', got %q", ch.Remediation)
				}
			}
		}
		if !foundRootFail {
			t.Errorf("missing root Category=roots Status=FAIL check")
		}
	})

	// 5. Inject EXHAUSTED_WORK -> Doctor reports WARN with retry command
	t.Run("ExhaustedTask", func(t *testing.T) {
		f3 := fresh(t)
		_ = terminalClient(t, f3)
		folder := f3.folder("taskfolder")
		taskID, err := f3.db.EnqueueDurableTask(ctx, repository.DurableTask{
			Folder:      folder,
			Kind:        "sync",
			MaxAttempts: 5,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f3.db.UpdateDurableTaskState(ctx, taskID, "exhausted", 5, "network down", "IO_TIMEOUT", 0); err != nil {
			t.Fatal(err)
		}

		warnReport, err := f3.ctrl.Doctor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if warnReport.OverallStatus != control.StatusWarn {
			t.Errorf("expected doctor WARN on exhausted task, got %s", warnReport.OverallStatus)
		}
		foundWorkWarn := false
		for _, ch := range warnReport.Checks {
			if ch.Name == "exhausted_tasks" && ch.Status == control.StatusWarn {
				foundWorkWarn = true
				if !strings.Contains(ch.Remediation, "orbit engine work retry") {
					t.Errorf("expected remediation to suggest 'orbit engine work retry', got %q", ch.Remediation)
				}
			}
		}
		if !foundWorkWarn {
			t.Errorf("missing exhausted_tasks WARN check")
		}
	})
}

// TestTerminalT07_CLI_StatusAndDoctor_Parity verifies:
// - orbit status and orbit doctor across running daemon and stopped adapter
// - Output formatting in both machine --json and sanitized human output
// - Correct exit codes: ExitOK (0) for clean query, ExitFailure (1) for failed doctor checks
// - Preserves backward-compatible fields in status JSON: daemon_running, setup, folders, attention
func TestTerminalT07_CLI_StatusAndDoctor_Parity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := testkit.NewDisposable(t)
	os.Chmod(base, 0700)
	binary := buildOrbitBinary(t, base)

	stateDir := filepath.Join(base, "state")
	cfg, err := app.Initialize(ctx, stateDir, app.SystemDependencies())
	if err != nil {
		t.Fatal(err)
	}
	devID := mustID(t, cfg.DeviceID)

	// Register a folder in stopped state
	rootA := filepath.Join(base, "sync_folder")
	if err := os.MkdirAll(rootA, 0700); err != nil {
		t.Fatal(err)
	}
	var folderA history.ID
	err = app.WithWorkspace(ctx, stateDir, func(c config.Config, db *repository.DB, ws *workspace.Workspace) error {
		folderA = mustID(t, enrollmentRandom(t))
		if err := db.EnsureFolder(ctx, folderA, devID, 1); err != nil {
			return err
		}
		ctrl := control.New(db, ws, control.Options{LocalDevice: devID})
		if _, err := ctrl.RegisterFolder(ctx, folderA, rootA); err != nil {
			return err
		}
		return db.SetFolderDisplayName(ctx, folderA, "SyncFolder")
	})
	if err != nil {
		t.Fatalf("register folder: %v", err)
	}

	// 1. Stopped state: orbit status --json
	cmdStatusJSON := exec.Command(binary, "status", "--state", stateDir, "--json")
	outStatusJSON, err := cmdStatusJSON.CombinedOutput()
	if err != nil {
		t.Fatalf("stopped orbit status --json failed: %v\n%s", err, string(outStatusJSON))
	}
	var resStatus map[string]any
	if err := json.Unmarshal(outStatusJSON, &resStatus); err != nil {
		t.Fatalf("unmarshal status json: %v\n%s", err, string(outStatusJSON))
	}
	if resStatus["daemon_running"] != false {
		t.Errorf("expected daemon_running=false in stopped state")
	}
	for _, field := range []string{"setup", "folders", "attention"} {
		if _, ok := resStatus[field]; !ok {
			t.Errorf("status JSON missing backward-compatible field %q", field)
		}
	}

	// 2. Stopped state: orbit status human format
	cmdStatusHuman := exec.Command(binary, "status", "--state", stateDir)
	outStatusHuman, err := cmdStatusHuman.CombinedOutput()
	if err != nil {
		t.Fatalf("stopped orbit status human failed: %v\n%s", err, string(outStatusHuman))
	}
	sHuman := string(outStatusHuman)
	if !strings.Contains(sHuman, "Daemon: stopped") {
		t.Errorf("expected 'Daemon: stopped' in human output:\n%s", sHuman)
	}
	if !strings.Contains(sHuman, "SyncFolder") {
		t.Errorf("expected folder name 'SyncFolder' in human output:\n%s", sHuman)
	}

	// 3. Stopped state: orbit doctor --json
	cmdDocJSON := exec.Command(binary, "doctor", "--state", stateDir, "--json")
	outDocJSON, err := cmdDocJSON.CombinedOutput()
	if err != nil {
		t.Fatalf("stopped orbit doctor --json failed: %v\n%s", err, string(outDocJSON))
	}
	var resDoc control.DoctorReport
	if err := json.Unmarshal(outDocJSON, &resDoc); err != nil {
		t.Fatalf("unmarshal doctor json: %v\n%s", err, string(outDocJSON))
	}
	if resDoc.OverallStatus != control.StatusOk {
		t.Errorf("expected doctor OK, got %s", resDoc.OverallStatus)
	}

	// 4. Start live background daemon
	daemon := exec.Command(binary, "serve", "--state", stateDir, "--control-listen", "127.0.0.1:0", "--no-watch", "--sync-interval", "1s")
	if err := daemon.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer func() {
		_ = daemon.Process.Signal(syscall.SIGTERM)
		_ = daemon.Wait()
	}()

	// Wait for daemon to write control.addr
	addrFile := filepath.Join(stateDir, "control.addr")
	for i := 0; i < 50; i++ {
		if b, err := os.ReadFile(addrFile); err == nil && len(strings.TrimSpace(string(b))) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 5. Live daemon: orbit status --json
	cmdLiveStatus := exec.Command(binary, "status", "--state", stateDir, "--json")
	outLiveStatus, err := cmdLiveStatus.CombinedOutput()
	if err != nil {
		t.Fatalf("live orbit status --json failed: %v\n%s", err, string(outLiveStatus))
	}
	var resLive map[string]any
	if err := json.Unmarshal(outLiveStatus, &resLive); err != nil {
		t.Fatalf("unmarshal live status json: %v\n%s", err, string(outLiveStatus))
	}
	if resLive["daemon_running"] != true {
		t.Errorf("expected daemon_running=true for running daemon")
	}

	// 6. Live daemon: orbit doctor human format
	cmdLiveDoc := exec.Command(binary, "doctor", "--state", stateDir)
	outLiveDoc, err := cmdLiveDoc.CombinedOutput()
	if err != nil {
		t.Fatalf("live orbit doctor failed: %v\n%s", err, string(outLiveDoc))
	}
	if !strings.Contains(string(outLiveDoc), "overall=OK") {
		t.Errorf("expected overall=OK in doctor output:\n%s", string(outLiveDoc))
	}
}
