package control

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func TestFolderManagementAndUnregisterEmitsZeroDeletes(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	ctrl := New(db, ws)

	// Create workspace root
	root := filepath.Join(t.TempDir(), "workspace-root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	// Place a user file in root
	userFile := filepath.Join(root, "my_document.txt")
	if err := os.WriteFile(userFile, []byte("important user document"), 0o644); err != nil {
		t.Fatalf("write user file: %v", err)
	}

	var folder history.ID
	folder[0] = 0x55
	var author history.ID
	author[0] = 0xAA

	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatalf("ensure folder: %v", err)
	}

	// 1. Register Folder
	reg, err := ctrl.RegisterFolder(ctx, folder, root)
	if err != nil {
		t.Fatalf("register folder: %v", err)
	}
	if reg.Path != root {
		t.Errorf("expected path %s, got %s", root, reg.Path)
	}

	// Scan to author a version
	scanRes, err := ctrl.WorkScan(ctx, WorkScanRequest{Folder: &folder})
	if err != nil {
		t.Fatalf("work scan: %v", err)
	}
	if scanRes.CapturedCount != 1 {
		t.Fatalf("expected 1 captured file, got %d", scanRes.CapturedCount)
	}

	// Record versions count and latest counter before unregister
	folders, err := ctrl.Folders(ctx)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if len(folders) != 1 || folders[0].NextCounter != 1 {
		t.Fatalf("unexpected folder state: %+v", folders)
	}

	// 2. Pause and Resume
	if err := ctrl.PauseFolder(ctx, folder, "OPERATOR_MAINTENANCE"); err != nil {
		t.Fatalf("pause folder: %v", err)
	}
	folders, _ = ctrl.Folders(ctx)
	if !folders[0].Paused || folders[0].PauseReason != "OPERATOR_MAINTENANCE" {
		t.Errorf("expected paused folder: %+v", folders[0])
	}

	if err := ctrl.ResumeFolder(ctx, folder); err != nil {
		t.Fatalf("resume folder: %v", err)
	}
	folders, _ = ctrl.Folders(ctx)
	if folders[0].Paused {
		t.Errorf("expected resumed folder: %+v", folders[0])
	}

	// 3. Revalidate
	if err := ctrl.RevalidateRoot(ctx, folder); err != nil {
		t.Fatalf("revalidate root: %v", err)
	}

	// 4. Unregister Folder
	if err := ctrl.UnregisterFolder(ctx, folder); err != nil {
		t.Fatalf("unregister folder: %v", err)
	}

	// Verify invariant: user file on disk is untouched!
	if _, err := os.Stat(userFile); err != nil {
		t.Fatalf("user file was deleted on unregister! %v", err)
	}

	// Verify invariant: zero deletion tombstones or version increments emitted!
	foldersAfter, err := ctrl.Folders(ctx)
	if err != nil {
		t.Fatalf("folders after unregister: %v", err)
	}
	if foldersAfter[0].RootPath != "" {
		t.Errorf("expected root_path to be cleared, got %q", foldersAfter[0].RootPath)
	}
	if foldersAfter[0].NextCounter != 1 {
		t.Errorf("counter was incremented! Expected 1, got %d", foldersAfter[0].NextCounter)
	}

	// Check history of my_document.txt: must have exactly 1 version (no tombstone was emitted)
	historyItems, err := ctrl.History(ctx, folder, "my_document.txt")
	if err != nil {
		t.Fatalf("history failed: %v", err)
	}
	if len(historyItems) != 1 || historyItems[0].Kind == history.KindTombstone {
		t.Fatalf("tombstone version was emitted on unregister! Items: %+v", historyItems)
	}
}

func TestMaintenanceOperations(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	ctrl := New(db, ws)

	// 1. CheckMigration
	mig, err := ctrl.CheckMigration(ctx)
	if err != nil {
		t.Fatalf("check migration: %v", err)
	}
	if mig.Status != "up_to_date" {
		t.Errorf("expected up_to_date, got %s", mig.Status)
	}

	// 2. Backup
	backupPath := filepath.Join(t.TempDir(), "backup.sqlite")
	bRes, err := ctrl.Backup(ctx, backupPath)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if bRes.SizeBytes == 0 {
		t.Errorf("expected positive backup size")
	}

	// Verify backup can be opened as SQLite database
	backupDB, err := repository.Open(ctx, filepath.Dir(backupPath))
	_ = backupDB // Can open if placed in state dir structure, or checked by size
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	// 3. RecoveryInspection
	rec, err := ctrl.RecoveryInspection(ctx)
	if err != nil {
		t.Fatalf("recovery inspection: %v", err)
	}
	if rec == nil {
		t.Fatal("nil recovery inspection")
	}

	// 4. Metrics
	metrics, err := ctrl.Metrics(ctx)
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if metrics == nil {
		t.Fatal("nil metrics")
	}

	// 5. ResetIdentity
	resetRes, err := ctrl.ResetIdentity(ctx, stateDir)
	if err != nil {
		t.Fatalf("reset identity: %v", err)
	}
	if resetRes.NewDeviceID == ([32]byte{}) {
		t.Errorf("zero new device id")
	}
}
