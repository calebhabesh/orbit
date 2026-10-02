package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

// TestOrbitStorage_AccountingCategories verifies that storage accounting distinctly tracks:
// 1. Working root bytes (actual files in folder)
// 2. Managed object bytes (CAS chunk storage in stateDir/objects)
// 3. Staging and recovery bytes
// 4. Metadata bytes (SQLite database and WAL)
// 5. Filesystem free space with visible default limits and capacity check warnings.
func TestOrbitStorage_AccountingCategories(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	rootDir := filepath.Join(disposable, "workspace")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatal(err)
	}

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

	// 1. Create a file in working root
	testPayload := []byte("Hello, Orbit Storage Accounting World!")
	filePath := filepath.Join(rootDir, "test.txt")
	if err := os.WriteFile(filePath, testPayload, 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Create managed CAS object via db.InstallChunk
	chunkData := []byte("chunk-data-object-payload")
	chunkDigest := sha256.Sum256(chunkData)
	if err := db.InstallChunk(ctx, chunkDigest, uint64(len(chunkData)), bytes.NewReader(chunkData)); err != nil {
		t.Fatal(err)
	}

	// 3. Create stage and recovery files beneath root/.filesync-internal
	internalDir := filepath.Join(rootDir, ".filesync-internal")
	_ = os.MkdirAll(internalDir, 0o755)

	stagePath := filepath.Join(internalDir, "stage-op1")
	_ = os.WriteFile(stagePath, []byte("staging-data"), 0o644)

	recovPath := filepath.Join(internalDir, "recovery-op1")
	_ = os.WriteFile(recovPath, []byte("recovered-data-bytes"), 0o644)

	// 4. Query Detailed Storage Usage via controller
	usageRes, err := ctrl.StorageUsage(ctx)
	if err != nil {
		t.Fatalf("StorageUsage failed: %v", err)
	}
	usage := usageRes.Usage

	// Check 5 distinct categories
	if usage.TotalWorkingRootBytes < uint64(len(testPayload)) {
		t.Errorf("TotalWorkingRootBytes = %d, want at least %d", usage.TotalWorkingRootBytes, len(testPayload))
	}
	if usage.ObjectBytes < uint64(len(chunkData)) {
		t.Errorf("ObjectBytes = %d, want at least %d", usage.ObjectBytes, len(chunkData))
	}
	if usage.StagingBytes < uint64(len("staging-data")) {
		t.Errorf("StagingBytes = %d, want at least %d", usage.StagingBytes, len("staging-data"))
	}
	if usage.RecoveryBytes < uint64(len("recovered-data-bytes")) {
		t.Errorf("RecoveryBytes = %d, want at least %d", usage.RecoveryBytes, len("recovered-data-bytes"))
	}
	if usage.MetadataBytes <= 0 {
		t.Errorf("MetadataBytes = %d, want > 0 (database sqlite file)", usage.MetadataBytes)
	}
	if usage.StateFilesystem.FreeBytes <= 0 {
		t.Errorf("StateFilesystem free space not reported or zero")
	}

	// Check visible default limits
	if usage.MetadataBudgetBytes != 256*1024*1024 {
		t.Errorf("MetadataBudgetBytes = %d, want 256 MiB", usage.MetadataBudgetBytes)
	}
	if usage.FreeSpaceReserveBytes != 512*1024*1024 {
		t.Errorf("FreeSpaceReserveBytes = %d, want 512 MiB", usage.FreeSpaceReserveBytes)
	}
}

// TestOrbitStorage_RetentionPreviewVsExplicitGC verifies that:
// 1. Retention preview is a safe inspection read that NEVER deletes files.
// 2. Candidate chunks are clearly separated from protected chunks.
// 3. Cleanup (RunGC/GCRun) is an explicit mutation that deletes candidate objects.
func TestOrbitStorage_RetentionPreviewVsExplicitGC(t *testing.T) {
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

	// 1. Create superseded version 1 with chunk 1
	c1Data := []byte("superseded-historical-content")
	m1, err := db.StoreFile(ctx, bytes.NewReader(c1Data), false)
	if err != nil {
		t.Fatal(err)
	}
	v1, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folderID,
		Path:             "doc.txt",
		Kind:             history.KindFile,
		Manifest:         m1,
		AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Age version 1 by 60 days
	agedTime := time.Now().Add(-60 * 24 * time.Hour)
	if err := db.SetAcquiredTime(ctx, v1.ID, agedTime); err != nil {
		t.Fatal(err)
	}

	// 2. Create current head version 2 with chunk 2
	c2Data := []byte("current-active-head-content")
	m2, err := db.StoreFile(ctx, bytes.NewReader(c2Data), false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folderID,
		Path:             "doc.txt",
		Basis:            []history.VersionID{v1.ID},
		Kind:             history.KindFile,
		Manifest:         m2,
		AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Locate chunk 1 on disk
	hexDigest := hex.EncodeToString(m1.Chunks[0].Digest[:])
	c1Path := filepath.Join(stateDir, "objects", "sha256", hexDigest[:2], hexDigest[2:])
	if _, err := os.Stat(c1Path); err != nil {
		t.Fatalf("chunk 1 not found on disk at %s: %v", c1Path, err)
	}

	// 3. Preview retention (INSPECTION READ with 30 days retention and 0 min superseded)
	previewDays := 30
	zeroMin := 0
	previewRes, err := ctrl.RetentionPreview(ctx, control.RetentionPreviewRequest{
		Folder:        folderID,
		RetentionDays: &previewDays,
		MinSuperseded: &zeroMin,
	})
	if err != nil {
		t.Fatalf("RetentionPreview failed: %v", err)
	}

	if previewRes.Preview.CandidateChunks != 1 {
		t.Errorf("CandidateChunks = %d, want 1", previewRes.Preview.CandidateChunks)
	}
	if previewRes.Preview.ProtectedChunks < 1 {
		t.Errorf("ProtectedChunks = %d, want at least 1", previewRes.Preview.ProtectedChunks)
	}

	// CRITICAL ORACLE: The candidate chunk file MUST STILL EXIST on disk!
	// Retention preview must never delete any data.
	if _, err := os.Stat(c1Path); err != nil {
		t.Fatalf("Retention preview violated inspection safety! File was prematurely deleted: %v", err)
	}

	// 4. Explicit GC execution (MUTATION)
	gcRes, err := ctrl.GCRun(ctx, control.GCRunRequest{
		Folder:        folderID,
		RetentionDays: &previewDays,
		MinSuperseded: &zeroMin,
	})
	if err != nil {
		t.Fatalf("GCRun failed: %v", err)
	}

	if gcRes.Report.UnlinkedObjects != 1 {
		t.Errorf("UnlinkedObjects = %d, want 1", gcRes.Report.UnlinkedObjects)
	}

	// Now c1Path MUST be deleted
	if _, err := os.Stat(c1Path); !os.IsNotExist(err) {
		t.Errorf("Explicit GCRun failed to delete candidate chunk on disk")
	}

	// Protected head chunk c2 MUST still exist
	hexC2 := hex.EncodeToString(m2.Chunks[0].Digest[:])
	c2Path := filepath.Join(stateDir, "objects", "sha256", hexC2[:2], hexC2[2:])
	if _, err := os.Stat(c2Path); err != nil {
		t.Errorf("GCRun deleted protected head chunk: %v", err)
	}
}

// TestOrbitStorage_ReclaimRecovery verifies explicit recovery storage reclamation.
func TestOrbitStorage_ReclaimRecovery(t *testing.T) {
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

	db, _ := repository.Open(ctx, stateDir)
	defer db.Close()
	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{LocalDevice: devID})

	var folderID history.ID
	rand.Read(folderID[:])
	_ = db.EnsureFolder(ctx, folderID, devID, 1)
	if _, err := ctrl.RegisterFolder(ctx, folderID, rootDir); err != nil {
		t.Fatal(err)
	}

	internalDir := filepath.Join(rootDir, ".filesync-internal")
	_ = os.MkdirAll(internalDir, 0o755)
	_ = os.WriteFile(filepath.Join(internalDir, "recovery-op123"), []byte("recovered data 1"), 0o600)
	_ = os.WriteFile(filepath.Join(internalDir, "recovery-op456"), []byte("recovered data 2"), 0o600)

	res, err := ctrl.ReclaimRecoveryCopies(ctx, control.ReclaimRecoveryRequest{Folder: folderID})
	if err != nil {
		t.Fatalf("ReclaimRecoveryCopies failed: %v", err)
	}

	if res.ReclaimedCount != 2 {
		t.Errorf("ReclaimedCount = %d, want 2", res.ReclaimedCount)
	}

	// Verify the recovery files are unlinked
	if _, err := os.Stat(filepath.Join(internalDir, "recovery-op123")); !os.IsNotExist(err) {
		t.Errorf("Expected recovery-op123 to be unlinked")
	}
	if _, err := os.Stat(filepath.Join(internalDir, "recovery-op456")); !os.IsNotExist(err) {
		t.Errorf("Expected recovery-op456 to be unlinked")
	}
}

// TestOrbitStorage_UnregisterPreservesFiles verifies Invariant I20:
// Unregistering a workspace clears registration but NEVER deletes user files
// and emits NO deletion tombstones in the DAG.
func TestOrbitStorage_UnregisterPreservesFiles(t *testing.T) {
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

	db, _ := repository.Open(ctx, stateDir)
	defer db.Close()
	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{LocalDevice: devID})

	var folderID history.ID
	rand.Read(folderID[:])
	_ = db.EnsureFolder(ctx, folderID, devID, 1)
	if _, err := ctrl.RegisterFolder(ctx, folderID, rootDir); err != nil {
		t.Fatal(err)
	}

	// Create user file
	userFilePath := filepath.Join(rootDir, "important-document.pdf")
	_ = os.WriteFile(userFilePath, []byte("%PDF-1.4 important user content"), 0o644)

	// Scan workspace to create version in DAG
	scanRes, err := ctrl.WorkScan(ctx, control.WorkScanRequest{Folder: &folderID})
	if err != nil {
		t.Fatalf("WorkScan failed: %v", err)
	}
	if scanRes.CapturedCount != 1 {
		t.Fatalf("CapturedCount = %d, want 1", scanRes.CapturedCount)
	}

	// Unregister folder
	err = ctrl.UnregisterFolder(ctx, folderID)
	if err != nil {
		t.Fatalf("UnregisterFolder failed: %v", err)
	}

	// 1. Verify folder registration is cleared
	folders, err := db.Folders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var fRec repository.FolderRecord
	for _, f := range folders {
		if f.Folder == folderID {
			fRec = f
			break
		}
	}
	if fRec.RootPath != "" {
		t.Errorf("RootPath = %q, want empty after unregister", fRec.RootPath)
	}

	// 2. Invariant I20: User file MUST STILL EXIST on disk!
	if _, err := os.Stat(userFilePath); err != nil {
		t.Fatalf("Invariant I20 violated: User file was deleted by unregister: %v", err)
	}

	// 3. Invariant I20: No deletion tombstones were created
	heads, err := db.Heads(ctx, folderID, "important-document.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if len(heads) != 1 || heads[0].Kind == history.KindTombstone {
		t.Errorf("Expected head to remain live file, got heads: %v", heads)
	}
}

// TestOrbitStorage_PauseResume verifies that workspace pause and resume
// update folder status without mutating files or DAG history.
func TestOrbitStorage_PauseResume(t *testing.T) {
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

	db, _ := repository.Open(ctx, stateDir)
	defer db.Close()
	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{LocalDevice: devID})

	var folderID history.ID
	rand.Read(folderID[:])
	_ = db.EnsureFolder(ctx, folderID, devID, 1)
	if _, err := ctrl.RegisterFolder(ctx, folderID, rootDir); err != nil {
		t.Fatal(err)
	}

	// 1. Pause folder
	err := ctrl.PauseFolder(ctx, folderID, "USER_REQUESTED")
	if err != nil {
		t.Fatalf("PauseFolder failed: %v", err)
	}
	folders, err := db.Folders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var fRec repository.FolderRecord
	for _, f := range folders {
		if f.Folder == folderID {
			fRec = f
			break
		}
	}
	if !fRec.Paused || fRec.PauseReason != "USER_REQUESTED" {
		t.Errorf("Folder not paused correctly: paused=%v, reason=%s", fRec.Paused, fRec.PauseReason)
	}

	// 2. Resume folder
	err = ctrl.ResumeFolder(ctx, folderID)
	if err != nil {
		t.Fatalf("ResumeFolder failed: %v", err)
	}
	folders, err = db.Folders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range folders {
		if f.Folder == folderID {
			fRec = f
			break
		}
	}
	if fRec.Paused || fRec.PauseReason != "" {
		t.Errorf("Folder not resumed correctly: paused=%v, reason=%s", fRec.Paused, fRec.PauseReason)
	}
}
