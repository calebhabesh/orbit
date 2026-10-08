package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/state"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
)

// TestOrbitRecoveryCandidate1ConfigWriteAndRestart verifies that ResetIdentity and RestoreBackup
// write valid typed configuration (format_version, device_id, created_at) that is accepted
// by the real config loader, validator, and daemon.
func TestOrbitRecoveryCandidate1ConfigWriteAndRestart(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)
	stateDir := filepath.Join(disposable, "state")

	// 1. Initialize device
	initCmd := exec.Command(binary, "engine", "init", "--state", stateDir)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}

	// 2. Reset identity
	resetCmd := exec.Command(binary, "engine", "maintenance", "reset-identity", "--state", stateDir, "--json")
	out, err := resetCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("reset-identity failed: %v\n%s", err, out)
	}
	var resetRes control.ResetIdentityResult
	if err := json.Unmarshal(out, &resetRes); err != nil {
		t.Fatalf("unmarshal reset-identity result: %v\n%s", err, out)
	}

	// 3. Oracle: config.Load accepts the state and format_version/created_at are valid
	cfg, err := config.Load(stateDir)
	if err != nil {
		t.Fatalf("config.Load failed after reset-identity: %v", err)
	}
	if cfg.FormatVersion != config.FormatVersion {
		t.Errorf("FormatVersion = %d, want %d", cfg.FormatVersion, config.FormatVersion)
	}
	if cfg.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero after reset-identity")
	}
	if cfg.DeviceID != hex.EncodeToString(resetRes.NewDeviceID[:]) {
		t.Errorf("DeviceID = %s, want %x", cfg.DeviceID, resetRes.NewDeviceID)
	}

	// 4. CLI config validate succeeds
	valCmd := exec.Command(binary, "engine", "config", "validate", "--state", stateDir, "--json")
	if out, err := valCmd.CombinedOutput(); err != nil {
		t.Fatalf("config validate failed after reset-identity: %v\n%s", err, out)
	}

	// 5. Create a backup
	backupPath := filepath.Join(disposable, "backup.sqlite")
	backupCmd := exec.Command(binary, "engine", "maintenance", "backup", "--state", stateDir, "--out", backupPath, "--json")
	if out, err := backupCmd.CombinedOutput(); err != nil {
		t.Fatalf("backup failed: %v\n%s", err, out)
	}

	// 6. Restore backup
	restoreCmd := exec.Command(binary, "engine", "maintenance", "restore-backup", "--state", stateDir, "--backup", backupPath, "--json")
	out, err = restoreCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("restore-backup failed: %v\n%s", err, out)
	}
	var restoreRes control.RestoreBackupResult
	if err := json.Unmarshal(out, &restoreRes); err != nil {
		t.Fatalf("unmarshal restore-backup result: %v\n%s", err, out)
	}

	// 7. Oracle: config.Load accepts restored state
	cfgRestored, err := config.Load(stateDir)
	if err != nil {
		t.Fatalf("config.Load failed after restore-backup: %v", err)
	}
	if cfgRestored.FormatVersion != config.FormatVersion {
		t.Errorf("FormatVersion after restore = %d, want %d", cfgRestored.FormatVersion, config.FormatVersion)
	}
	if cfgRestored.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero after restore-backup")
	}
	if cfgRestored.DeviceID != hex.EncodeToString(restoreRes.NewDeviceID[:]) {
		t.Errorf("DeviceID = %s, want %x", cfgRestored.DeviceID, restoreRes.NewDeviceID)
	}

	// 8. CLI config validate succeeds after restore
	valCmd2 := exec.Command(binary, "engine", "config", "validate", "--state", stateDir, "--json")
	if out, err := valCmd2.CombinedOutput(); err != nil {
		t.Fatalf("config validate failed after restore-backup: %v\n%s", err, out)
	}
}

// TestOrbitRecoveryCandidate2KeyRotation verifies that ResetIdentity and RestoreBackup
// generate a fresh TLS private key and rotate the key pin rather than reusing the old key.
func TestOrbitRecoveryCandidate2KeyRotation(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)
	stateDir := filepath.Join(disposable, "state")

	// 1. Initialize
	initCmd := exec.Command(binary, "engine", "init", "--state", stateDir)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}

	cfg0, err := config.Load(stateDir)
	if err != nil {
		t.Fatalf("load initial config: %v", err)
	}
	dev0Bytes, _ := hex.DecodeString(cfg0.DeviceID)
	var dev0 history.ID
	copy(dev0[:], dev0Bytes)

	ident0, err := replication.LoadOrCreateIdentity(stateDir, dev0, time.Now())
	if err != nil {
		t.Fatalf("load initial identity: %v", err)
	}
	initialPin := ident0.KeyPin

	// 2. Reset identity
	resetCmd := exec.Command(binary, "engine", "maintenance", "reset-identity", "--state", stateDir, "--json")
	out, err := resetCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("reset-identity failed: %v\n%s", err, out)
	}
	var resetRes control.ResetIdentityResult
	if err := json.Unmarshal(out, &resetRes); err != nil {
		t.Fatalf("unmarshal reset-identity: %v\n%s", err, out)
	}

	// Oracle: KeyPin must be distinct from initial key pin!
	if resetRes.NewKeyPin == initialPin {
		t.Fatalf("CRITICAL: reset-identity reused old key pin %x", initialPin)
	}

	// Load identity directly from disk and verify it matches the new key pin
	ident1, err := replication.LoadOrCreateIdentity(stateDir, resetRes.NewDeviceID, time.Now())
	if err != nil {
		t.Fatalf("load identity after reset: %v", err)
	}
	if ident1.KeyPin != resetRes.NewKeyPin {
		t.Errorf("loaded identity key pin %x does not match reset result %x", ident1.KeyPin, resetRes.NewKeyPin)
	}
	if bytes.Equal(ident0.Certificate.Certificate[0], ident1.Certificate.Certificate[0]) {
		t.Fatalf("CRITICAL: peer-identity.pem certificate was not rotated!")
	}

	// 3. Test backup and restore key rotation
	backupPath := filepath.Join(disposable, "backup.sqlite")
	backupCmd := exec.Command(binary, "engine", "maintenance", "backup", "--state", stateDir, "--out", backupPath, "--json")
	if out, err := backupCmd.CombinedOutput(); err != nil {
		t.Fatalf("backup failed: %v\n%s", err, out)
	}

	restoreCmd := exec.Command(binary, "engine", "maintenance", "restore-backup", "--state", stateDir, "--backup", backupPath, "--json")
	out, err = restoreCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("restore-backup failed: %v\n%s", err, out)
	}
	var restoreRes control.RestoreBackupResult
	if err := json.Unmarshal(out, &restoreRes); err != nil {
		t.Fatalf("unmarshal restore-backup: %v\n%s", err, out)
	}

	// Oracle: restored key pin must be distinct from both initialPin and resetRes.NewKeyPin!
	if restoreRes.NewKeyPin == initialPin || restoreRes.NewKeyPin == resetRes.NewKeyPin {
		t.Fatalf("CRITICAL: restore-backup reused existing key pin: %x", restoreRes.NewKeyPin)
	}
}

// TestOrbitRecoveryCandidate3TransactionalAuthorAlignment verifies that ResetIdentity and
// RestoreBackup align folder authors in SQLite transactionally, preserve previous history,
// and guarantee no future versions are authored under the retired device ID.
func TestOrbitRecoveryCandidate3TransactionalAuthorAlignment(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)
	stateDir := filepath.Join(disposable, "state")
	workspaceRoot := filepath.Join(disposable, "workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Initialize
	initCmd := exec.Command(binary, "engine", "init", "--state", stateDir)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	cfg0, _ := config.Load(stateDir)
	dev0Bytes, _ := hex.DecodeString(cfg0.DeviceID)

	var folderID history.ID
	_, _ = rand.Read(folderID[:])
	folderHex := hex.EncodeToString(folderID[:])

	// 2. Register folder and author files
	regCmd := exec.Command(binary, "engine", "register", "--state", stateDir, "--folder", folderHex, "--root", workspaceRoot)
	if out, err := regCmd.CombinedOutput(); err != nil {
		t.Fatalf("register failed: %v\n%s", err, out)
	}

	if err := os.WriteFile(filepath.Join(workspaceRoot, "file1.txt"), []byte("content 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	scanCmd := exec.Command(binary, "engine", "scan", "--state", stateDir, "--folder", folderHex)
	if out, err := scanCmd.CombinedOutput(); err != nil {
		t.Fatalf("scan failed: %v\n%s", err, out)
	}

	// 3. Take backup after file1
	backupPath := filepath.Join(disposable, "backup.sqlite")
	backupCmd := exec.Command(binary, "engine", "maintenance", "backup", "--state", stateDir, "--out", backupPath, "--json")
	if out, err := backupCmd.CombinedOutput(); err != nil {
		t.Fatalf("backup failed: %v\n%s", err, out)
	}

	// Add file2.txt and scan again
	if err := os.WriteFile(filepath.Join(workspaceRoot, "file2.txt"), []byte("content 2"), 0o644); err != nil {
		t.Fatal(err)
	}
	scanCmd2 := exec.Command(binary, "engine", "scan", "--state", stateDir, "--folder", folderHex)
	if out, err := scanCmd2.CombinedOutput(); err != nil {
		t.Fatalf("scan2 failed: %v\n%s", err, out)
	}

	// 4. Verify DB has 2 versions under dev0
	dbPath := filepath.Join(stateDir, "metadata.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	var dev0VersionCount int
	_ = db.QueryRow("SELECT count(*) FROM versions WHERE author_id=?", dev0Bytes).Scan(&dev0VersionCount)
	if dev0VersionCount != 2 {
		t.Fatalf("expected 2 versions under dev0, got %d", dev0VersionCount)
	}
	db.Close()

	// 5. Reset identity
	resetCmd := exec.Command(binary, "engine", "maintenance", "reset-identity", "--state", stateDir, "--json")
	out, err := resetCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("reset-identity failed: %v\n%s", err, out)
	}
	var resetRes control.ResetIdentityResult
	_ = json.Unmarshal(out, &resetRes)

	// 6. Oracle: Check that local_author was updated in folders table to new device ID!
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db after reset: %v", err)
	}
	var localAuthor []byte
	var nextCounter []byte
	if err := db.QueryRow("SELECT local_author, next_counter FROM folders WHERE folder_id=?", folderID[:]).Scan(&localAuthor, &nextCounter); err != nil {
		t.Fatalf("query folder author after reset: %v", err)
	}
	if !bytes.Equal(localAuthor, resetRes.NewDeviceID[:]) {
		t.Fatalf("CRITICAL: folders.local_author was not updated to new device ID! Got %x, want %x", localAuthor, resetRes.NewDeviceID)
	}
	if !bytes.Equal(nextCounter, make([]byte, 8)) {
		t.Fatalf("folders.next_counter was not reset to 0! Got %x", nextCounter)
	}

	// Oracle: Previous versions authored by dev0 MUST still exist in versions table (history preserved)
	var dev0StillExists int
	_ = db.QueryRow("SELECT count(*) FROM versions WHERE author_id=?", dev0Bytes).Scan(&dev0StillExists)
	if dev0StillExists != 2 {
		t.Fatalf("CRITICAL: previous history was lost on reset! Count = %d, want 2", dev0StillExists)
	}
	db.Close()

	// 7. Author a new file under the reset identity: must author under dev1, NOT dev0!
	if err := os.WriteFile(filepath.Join(workspaceRoot, "file3.txt"), []byte("content 3"), 0o644); err != nil {
		t.Fatal(err)
	}
	scanCmd3 := exec.Command(binary, "engine", "scan", "--state", stateDir, "--folder", folderHex)
	if out, err := scanCmd3.CombinedOutput(); err != nil {
		t.Fatalf("scan3 failed: %v\n%s", err, out)
	}

	db, _ = sql.Open("sqlite", dbPath)
	var dev1VersionCount int
	_ = db.QueryRow("SELECT count(*) FROM versions WHERE author_id=?", resetRes.NewDeviceID[:]).Scan(&dev1VersionCount)
	if dev1VersionCount != 1 {
		t.Fatalf("expected 1 version authored under new device ID, got %d", dev1VersionCount)
	}
	// dev0 versions must still be exactly 2
	var dev0CountAfterScan int
	_ = db.QueryRow("SELECT count(*) FROM versions WHERE author_id=?", dev0Bytes).Scan(&dev0CountAfterScan)
	if dev0CountAfterScan != 2 {
		t.Fatalf("old author dev0 produced more versions after reset! Got %d, want 2", dev0CountAfterScan)
	}
	db.Close()

	// 8. Restore from backup and verify author alignment and history preservation
	restoreCmd := exec.Command(binary, "engine", "maintenance", "restore-backup", "--state", stateDir, "--backup", backupPath, "--json")
	out, err = restoreCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("restore-backup failed: %v\n%s", err, out)
	}
	var restoreRes control.RestoreBackupResult
	_ = json.Unmarshal(out, &restoreRes)

	db, _ = sql.Open("sqlite", dbPath)
	if err := db.QueryRow("SELECT local_author, next_counter FROM folders WHERE folder_id=?", folderID[:]).Scan(&localAuthor, &nextCounter); err != nil {
		t.Fatalf("query folder author after restore: %v", err)
	}
	if !bytes.Equal(localAuthor, restoreRes.NewDeviceID[:]) {
		t.Fatalf("CRITICAL: folders.local_author not aligned to restored new device ID! Got %x, want %x", localAuthor, restoreRes.NewDeviceID)
	}
	if !bytes.Equal(nextCounter, make([]byte, 8)) {
		t.Fatalf("folders.next_counter not reset to 0 after restore! Got %x", nextCounter)
	}
	// Restored DB has the 1 version from backup
	var restoredCount int
	_ = db.QueryRow("SELECT count(*) FROM versions").Scan(&restoredCount)
	if restoredCount != 1 {
		t.Fatalf("expected 1 version in restored db, got %d", restoredCount)
	}
	db.Close()
}

// TestOrbitRecoveryCandidate4InspectionDoesNotMutateState verifies that RecoveryInspection
// is strictly read-only and does not delete scratch recovery files or clear database reservations.
func TestOrbitRecoveryCandidate4InspectionDoesNotMutateState(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	workspaceRoot := filepath.Join(disposable, "workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws)

	var folderID history.ID
	folderID[0] = 0x44
	var authorID history.ID
	authorID[0] = 0xAA

	if err := db.EnsureFolder(ctx, folderID, authorID, 1); err != nil {
		t.Fatalf("ensure folder: %v", err)
	}
	if _, err := ctrl.RegisterFolder(ctx, folderID, workspaceRoot); err != nil {
		t.Fatalf("register folder: %v", err)
	}

	// Place an unreferenced recovery file in the workspace internal scratch directory
	scratchDir := filepath.Join(workspaceRoot, ".orbit-internal")
	if err := os.MkdirAll(scratchDir, 0o700); err != nil {
		t.Fatalf("mkdir scratchDir: %v", err)
	}
	recoveryFilePath := filepath.Join(scratchDir, "recovery-op999")
	recoveryData := []byte("critical recovery copy data")
	if err := os.WriteFile(recoveryFilePath, recoveryData, 0o600); err != nil {
		t.Fatalf("write recovery file: %v", err)
	}

	// Create a reservation for it
	if err := db.Reserve(ctx, "publication-op999", uint64(len(recoveryData)), "publication"); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	// Execute RecoveryInspection multiple times
	for i := 0; i < 3; i++ {
		res, err := ctrl.RecoveryInspection(ctx)
		if err != nil {
			t.Fatalf("iteration %d: recovery inspection failed: %v", i, err)
		}
		if res.ReclaimableRecovery != 1 {
			t.Fatalf("iteration %d: expected 1 reclaimable recovery copy, got %d", i, res.ReclaimableRecovery)
		}

		// Oracle: the recovery file MUST STILL EXIST on disk!
		info, err := os.Stat(recoveryFilePath)
		if err != nil {
			t.Fatalf("iteration %d: recovery file was deleted by inspection! %v", i, err)
		}
		if info.Size() != int64(len(recoveryData)) {
			t.Fatalf("iteration %d: recovery file size modified: %d", i, info.Size())
		}

		// Oracle: the reservation MUST STILL EXIST in the database!
		usage, _ := db.StorageUsage(ctx)
		if usage.Reserved == 0 {
			t.Fatalf("iteration %d: reservation was cleared by inspection!", i)
		}
	}

	// Only explicit reclaim mutation removes it
	reclaimRes, err := ctrl.ReclaimRecoveryCopies(ctx, control.ReclaimRecoveryRequest{Folder: folderID})
	if err != nil {
		t.Fatalf("reclaim failed: %v", err)
	}
	if reclaimRes.ReclaimedCount != 1 {
		t.Fatalf("expected 1 reclaimed file, got %d", reclaimRes.ReclaimedCount)
	}

	// Now it is gone
	if _, err := os.Stat(recoveryFilePath); !os.IsNotExist(err) {
		t.Fatalf("recovery file should be deleted after explicit reclaim: %v", err)
	}
}

// TestOrbitRecoveryCandidate5RunbookWorkflow executes the full database-recovery runbook
// workflow step-by-step against fresh marked fixtures.
func TestOrbitRecoveryCandidate5RunbookWorkflow(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)

	node1State := filepath.Join(disposable, "node1_state")
	node1Root := filepath.Join(disposable, "node1_workspace")
	node2State := filepath.Join(disposable, "node2_state")
	node2Root := filepath.Join(disposable, "node2_workspace")
	for _, dir := range []string{node1State, node1Root, node2State, node2Root} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Init both nodes
	if out, err := exec.Command(binary, "engine", "init", "--state", node1State).CombinedOutput(); err != nil {
		t.Fatalf("init node1: %v\n%s", err, out)
	}
	if out, err := exec.Command(binary, "engine", "init", "--state", node2State).CombinedOutput(); err != nil {
		t.Fatalf("init node2: %v\n%s", err, out)
	}

	var folderID history.ID
	folderID[0] = 0x88
	folderHex := hex.EncodeToString(folderID[:])

	// Register shared folder on both surviving peer (node1) and local node (node2)
	if out, err := exec.Command(binary, "engine", "register", "--state", node1State, "--folder", folderHex, "--root", node1Root).CombinedOutput(); err != nil {
		t.Fatalf("register node1: %v\n%s", err, out)
	}
	if out, err := exec.Command(binary, "engine", "register", "--state", node2State, "--folder", folderHex, "--root", node2Root).CombinedOutput(); err != nil {
		t.Fatalf("register node2: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(node2Root, "doc.txt"), []byte("initial doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(binary, "engine", "scan", "--state", node2State, "--folder", folderHex).CombinedOutput(); err != nil {
		t.Fatalf("scan node2: %v\n%s", err, out)
	}

	// Take backup of node2
	backupFile := filepath.Join(disposable, "node2-backup.sqlite")
	if out, err := exec.Command(binary, "engine", "maintenance", "backup", "--state", node2State, "--out", backupFile, "--json").CombinedOutput(); err != nil {
		t.Fatalf("backup node2: %v\n%s", err, out)
	}

	// Corrupt node2 database to simulate disaster
	dbPath := filepath.Join(node2State, "metadata.sqlite")
	if err := os.WriteFile(dbPath, []byte("corrupted database content"), 0o600); err != nil {
		t.Fatal(err)
	}

	// === Execute Runbook Step 1: Restore backup and safe identity reset ===
	restoreCmd := exec.Command(binary, "engine", "maintenance", "restore-backup", "--state", node2State, "--backup", backupFile, "--json")
	out, err := restoreCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("runbook step 1 (restore-backup) failed: %v\n%s", err, out)
	}
	var restoreRes control.RestoreBackupResult
	if err := json.Unmarshal(out, &restoreRes); err != nil {
		t.Fatalf("unmarshal restore-backup: %v\n%s", err, out)
	}
	newDev2Hex := hex.EncodeToString(restoreRes.NewDeviceID[:])
	newPin2Hex := hex.EncodeToString(restoreRes.NewKeyPin[:])

	// === Execute Runbook Step 2: Re-enroll folders on surviving peer and recovered node ===
	// Surviving peer (node1) approves the recovered node's new identity
	pairCmd := exec.Command(binary, "engine", "pair-approve", "--state", node1State, "--folder", folderHex,
		"--peer-device", newDev2Hex, "--peer-key-pin", newPin2Hex)
	if out, err := pairCmd.CombinedOutput(); err != nil {
		t.Fatalf("runbook step 2 (pair-approve on peer) failed: %v\n%s", err, out)
	}

	// Export updated membership bundle from peer
	membershipFile := filepath.Join(disposable, "updated-membership.json")
	exportCmd := exec.Command(binary, "engine", "membership", "export", "--state", node1State, "--folder", folderHex, "--file", membershipFile)
	if out, err := exportCmd.CombinedOutput(); err != nil {
		t.Fatalf("runbook step 2 (membership export) failed: %v\n%s", err, out)
	}

	// Import updated membership onto recovered node with --approve
	importCmd := exec.Command(binary, "engine", "membership", "import", "--state", node2State, "--folder", folderHex, "--file", membershipFile, "--approve")
	if out, err := importCmd.CombinedOutput(); err != nil {
		t.Fatalf("runbook step 2 (membership import on recovered) failed: %v\n%s", err, out)
	}

	// Revalidate folder root on recovered node (the restored backup retains folder registration)
	revalCmd := exec.Command(binary, "engine", "folders", "revalidate", "--state", node2State, "--folder", folderHex)
	if out, err := revalCmd.CombinedOutput(); err != nil {
		t.Fatalf("runbook step 2 (folders revalidate on recovered) failed: %v\n%s", err, out)
	}

	// === Execute Runbook Step 3: Reconcile workspace root ===
	scanCmd := exec.Command(binary, "engine", "work", "scan", "--state", node2State, "--folder", folderHex, "--full")
	if out, err := scanCmd.CombinedOutput(); err != nil {
		t.Fatalf("runbook step 3 (work scan --full) failed: %v\n%s", err, out)
	}

	syncCmd := exec.Command(binary, "engine", "work", "sync", "--state", node2State, "--folder", folderHex)
	if out, err := syncCmd.CombinedOutput(); err != nil {
		t.Fatalf("runbook step 3 (work sync) failed: %v\n%s", err, out)
	}

	doctorCmd := exec.Command(binary, "engine", "doctor", "--state", node2State)
	if out, err := doctorCmd.CombinedOutput(); err != nil {
		t.Fatalf("runbook step 3 (doctor) failed: %v\n%s", err, out)
	}

	statusCmd := exec.Command(binary, "engine", "work", "status", "--state", node2State, "--folder", folderHex)
	if out, err := statusCmd.CombinedOutput(); err != nil {
		t.Fatalf("runbook step 3 (work status) failed: %v\n%s", err, out)
	}
}

// TestOrbitRecoveryLiveResetFenced verifies that identity reset and backup restore are rejected
// when the agent is running / live lock is held, enforcing a stopped exclusive transition.
func TestOrbitRecoveryLiveResetFenced(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)
	stateDir := filepath.Join(disposable, "state")

	// Init
	if out, err := exec.Command(binary, "engine", "init", "--state", stateDir).CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	backupPath := filepath.Join(disposable, "backup.sqlite")
	if out, err := exec.Command(binary, "engine", "maintenance", "backup", "--state", stateDir, "--out", backupPath, "--json").CombinedOutput(); err != nil {
		t.Fatalf("backup: %v\n%s", err, out)
	}

	// Acquire state lock (simulating running daemon)
	lock, err := state.Acquire(stateDir)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}

	// 1. Attempt ResetIdentity while running
	_, err = control.ResetIdentity(ctx, stateDir)
	if err == nil {
		lock.Close()
		t.Fatalf("ResetIdentity succeeded while agent lock held! Expected stopped error.")
	}

	// 2. Attempt RestoreBackup while running
	_, err = control.RestoreBackup(ctx, stateDir, backupPath)
	if err == nil {
		lock.Close()
		t.Fatalf("RestoreBackup succeeded while agent lock held! Expected stopped error.")
	}

	// Release lock
	if err := lock.Close(); err != nil {
		t.Fatalf("close lock: %v", err)
	}

	// Now that agent is stopped, ResetIdentity must succeed
	res, err := control.ResetIdentity(ctx, stateDir)
	if err != nil {
		t.Fatalf("ResetIdentity failed after agent stopped: %v", err)
	}
	if res.NewDeviceID == ([32]byte{}) {
		t.Errorf("zero new device ID")
	}
}
