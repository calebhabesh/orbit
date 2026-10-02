package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

// TestOrbitPruning_BoundedLifecycleRecords verifies Invariant I28:
//  1. Finished lifecycle records (completed/canceled tasks, expired invitations,
//     terminal enrollment requests, expired read leases, finished operations, expired control operations)
//     are safely and boundingly pruned.
//  2. Pending work (queued, running, retry) and diagnostic failures (exhausted)
//     are STRICTLY PRESERVED so operators never miss work or diagnostic errors.
//  3. Causal DAG metadata (versions, heads) is NEVER pruned.
func TestOrbitPruning_BoundedLifecycleRecords(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	rootDir := filepath.Join(disposable, "workspace")
	_ = os.MkdirAll(stateDir, 0o700)
	_ = os.MkdirAll(rootDir, 0o755)

	var devID history.ID
	rand.Read(devID[:])
	_ = config.Save(stateDir, config.Config{
		FormatVersion: 1,
		DeviceID:      hex.EncodeToString(devID[:]),
		CreatedAt:     time.Now().UTC(),
	})

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{LocalDevice: devID})

	var folderID history.ID
	rand.Read(folderID[:])
	if err := db.EnsureFolder(ctx, folderID, devID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.RegisterFolder(ctx, folderID, rootDir); err != nil {
		t.Fatal(err)
	}

	oldTime := time.Now().Add(-48 * time.Hour) // 2 days ago

	// 1. Insert tasks in various states directly to avoid coalescing
	taskStates := []string{"completed", "canceled", "queued", "running", "retry", "exhausted"}
	for _, st := range taskStates {
		tID := "task-" + st
		targetPath := "file-" + st + ".txt"
		_, err := db.ExecRaw(ctx, `INSERT INTO durable_work_tasks(task_id, folder_id, task_kind, target_path, state, created_ns, updated_ns) VALUES(?,?,?,?,?,?,?)`,
			tID, folderID[:], "scan", targetPath, st, oldTime.UnixNano(), oldTime.UnixNano())
		if err != nil {
			t.Fatalf("insert task %s: %v", st, err)
		}
	}

	// 2. Insert invitations: one expired, one active
	var expDigest, actDigest [32]byte
	rand.Read(expDigest[:])
	rand.Read(actDigest[:])
	if _, err := db.ExecRaw(ctx, `INSERT INTO invitations(digest, folder_id, created_ns, expires_ns, max_uses, uses_count, revoked) VALUES(?,?,?,?,1,0,0)`,
		expDigest[:], folderID[:], oldTime.UnixNano(), oldTime.UnixNano()); err != nil {
		t.Fatalf("insert expired invitation: %v", err)
	}
	if _, err := db.ExecRaw(ctx, `INSERT INTO invitations(digest, folder_id, created_ns, expires_ns, max_uses, uses_count, revoked) VALUES(?,?,?,?,1,0,0)`,
		actDigest[:], folderID[:], time.Now().UnixNano(), time.Now().Add(24*time.Hour).UnixNano()); err != nil {
		t.Fatalf("insert active invitation: %v", err)
	}

	// 3. Insert enrollment requests: approved, declined, pending
	dummyBytes := make([]byte, 32)
	for _, st := range []string{"approved", "declined", "pending"} {
		reqID := "req-" + st
		var joiner history.ID
		rand.Read(joiner[:])
		if _, err := db.ExecRaw(ctx, `INSERT INTO enrollment_requests(request_id, folder_id, device_id, public_key, key_pin, suggested_label, status, created_ns, updated_ns) VALUES(?,?,?,?,?,?,?,?,?)`,
			reqID, folderID[:], joiner[:], dummyBytes, dummyBytes, "dev", st, oldTime.UnixNano(), oldTime.UnixNano()); err != nil {
			t.Fatalf("insert enrollment request %s: %v", st, err)
		}
	}

	// 4. Insert operation progress: finished vs running
	if _, err := db.ExecRaw(ctx, `INSERT INTO operation_progress(operation_id, kind, phase, created_ns, updated_ns) VALUES('op-finished', 'copy', 'COMPLETED', ?, ?)`,
		oldTime.UnixNano(), oldTime.UnixNano()); err != nil {
		t.Fatalf("insert finished operation: %v", err)
	}
	if _, err := db.ExecRaw(ctx, `INSERT INTO operation_progress(operation_id, kind, phase, created_ns, updated_ns) VALUES('op-running', 'copy', 'RUNNING', ?, ?)`,
		oldTime.UnixNano(), oldTime.UnixNano()); err != nil {
		t.Fatalf("insert running operation: %v", err)
	}

	// 5. Insert control operations (idempotency records): expired vs active
	var d history.Digest
	_ = db.PutControlOperation(ctx, "idemp-expired", d, 0, &repository.ControlOpRecord{Action: "test", Status: "completed"}, oldTime)
	_ = db.PutControlOperation(ctx, "idemp-active", d, 0, &repository.ControlOpRecord{Action: "test", Status: "completed"}, time.Now().Add(24*time.Hour))

	// 6. Execute bounded pruning via Controller (cutoff = 24 hours ago)
	maxAge := int64(86400) // 24 hours
	pruneRes, err := ctrl.PruneLifecycleRecords(ctx, control.PruneRecordsRequest{CutoffSeconds: maxAge})
	if err != nil {
		t.Fatalf("PruneLifecycleRecords failed: %v", err)
	}

	rep := pruneRes.Report
	if rep.TasksPruned != 2 { // completed and canceled
		t.Errorf("TasksPruned = %d, want 2", rep.TasksPruned)
	}
	if rep.InvitationsPruned != 1 {
		t.Errorf("InvitationsPruned = %d, want 1", rep.InvitationsPruned)
	}
	if rep.EnrollmentRequestsPruned != 2 { // approved, declined
		t.Errorf("EnrollmentRequestsPruned = %d, want 2", rep.EnrollmentRequestsPruned)
	}
	if rep.OperationsPruned != 1 { // op-finished
		t.Errorf("OperationsPruned = %d, want 1", rep.OperationsPruned)
	}
	if rep.ControlOpsPruned != 1 { // idemp-expired
		t.Errorf("ControlOpsPruned = %d, want 1", rep.ControlOpsPruned)
	}

	// 7. INVARIANT I28 ORACLES: Verify preserved records!

	// Check durable work tasks
	taskList, err := ctrl.WorkList(ctx, control.WorkListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	survivingTasks := make(map[string]bool)
	for _, tk := range taskList.Tasks {
		survivingTasks[tk.State] = true
	}
	// 'queued', 'running', 'retry' MUST survive
	if !survivingTasks["queued"] {
		t.Errorf("Invariant I28 violated: 'queued' task was prematurely pruned!")
	}
	if !survivingTasks["running"] {
		t.Errorf("Invariant I28 violated: 'running' task was prematurely pruned!")
	}
	if !survivingTasks["retry"] {
		t.Errorf("Invariant I28 violated: 'retry' task was prematurely pruned!")
	}
	// 'exhausted' diagnostic task MUST survive
	if !survivingTasks["exhausted"] {
		t.Errorf("Invariant I28 violated: 'exhausted' diagnostic task was prematurely pruned!")
	}
	// 'completed' and 'canceled' MUST NOT survive
	if survivingTasks["completed"] || survivingTasks["canceled"] {
		t.Errorf("Finished task was not pruned: completed=%v, canceled=%v", survivingTasks["completed"], survivingTasks["canceled"])
	}

	// Check invitations
	var actCount int
	_ = db.QueryRowRaw(ctx, `SELECT count(*) FROM invitations WHERE digest=?`, actDigest[:]).Scan(&actCount)
	if actCount != 1 {
		t.Errorf("Active invitation was erroneously pruned!")
	}
	var expCount int
	_ = db.QueryRowRaw(ctx, `SELECT count(*) FROM invitations WHERE digest=?`, expDigest[:]).Scan(&expCount)
	if expCount != 0 {
		t.Errorf("Expired invitation was not pruned!")
	}

	// Check enrollment requests
	var pendingCount int
	_ = db.QueryRowRaw(ctx, `SELECT count(*) FROM enrollment_requests WHERE request_id='req-pending'`).Scan(&pendingCount)
	if pendingCount != 1 {
		t.Errorf("Pending enrollment request was erroneously pruned!")
	}
	var approvedCount int
	_ = db.QueryRowRaw(ctx, `SELECT count(*) FROM enrollment_requests WHERE request_id='req-approved'`).Scan(&approvedCount)
	if approvedCount != 0 {
		t.Errorf("Approved enrollment request was not pruned!")
	}

	// Check operations
	var runningOpCount int
	_ = db.QueryRowRaw(ctx, `SELECT count(*) FROM operation_progress WHERE operation_id='op-running'`).Scan(&runningOpCount)
	if runningOpCount != 1 {
		t.Errorf("Running operation progress was erroneously pruned!")
	}

	// Check idempotency records
	rec, _, _, err := db.GetControlOperation(ctx, "idemp-active")
	if err != nil || rec == nil {
		t.Errorf("Active idempotency record was erroneously pruned!")
	}
}

// TestOrbitPruning_IdempotencyExpiryAndReplaySafety verifies Invariant I28:
// Defined replay lifetime with explicit expired replay rejection (ErrExpiredReplay / EXPIRED_REPLAY).
func TestOrbitPruning_IdempotencyExpiryAndReplaySafety(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	_ = os.MkdirAll(stateDir, 0o700)

	var devID history.ID
	rand.Read(devID[:])
	_ = config.Save(stateDir, config.Config{
		FormatVersion: 1,
		DeviceID:      hex.EncodeToString(devID[:]),
		CreatedAt:     time.Now().UTC(),
	})

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{LocalDevice: devID})

	var folderID history.ID
	rand.Read(folderID[:])
	_ = db.EnsureFolder(ctx, folderID, devID, 1)

	// 1. Save an unexpired control operation
	keyActive := "idemp-key-active"
	digest := sha256.Sum256([]byte("test-payload"))
	recActive := &repository.ControlOpRecord{Action: "gc_run", Status: "completed"}
	err = db.PutControlOperation(ctx, keyActive, digest, 0, recActive, time.Now().Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	// Query before expiry: returns record successfully
	gotRec, gotDigest, _, err := db.GetControlOperation(ctx, keyActive)
	if err != nil || gotRec == nil || gotDigest != digest {
		t.Fatalf("Failed to retrieve active control operation: %v", err)
	}

	// 2. Save an already expired control operation
	keyExpired := "idemp-key-expired"
	recExpired := &repository.ControlOpRecord{Action: "gc_run", Status: "completed"}
	err = db.PutControlOperation(ctx, keyExpired, digest, 0, recExpired, time.Now().Add(-10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	// Query expired: MUST return ErrExpiredReplay!
	_, _, _, err = db.GetControlOperation(ctx, keyExpired)
	if !errors.Is(err, repository.ErrExpiredReplay) {
		t.Fatalf("Expected ErrExpiredReplay for expired idempotency key, got: %v", err)
	}

	// 3. Controller idempotency check returns ExpiredReplayError (code EXPIRED_REPLAY)
	_, err = ctrl.GCRun(ctx, control.GCRunRequest{
		Folder:         folderID,
		IdempotencyKey: keyExpired,
	})
	if err == nil {
		t.Fatalf("Expected error for expired replay")
	}
	var ctrlErr *control.ControlError
	if errors.As(err, &ctrlErr) {
		if ctrlErr.Code != "EXPIRED_REPLAY" {
			t.Errorf("Expected error code EXPIRED_REPLAY, got %s", ctrlErr.Code)
		}
	} else {
		t.Errorf("Expected ControlError, got %v", err)
	}
}
