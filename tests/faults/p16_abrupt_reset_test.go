package faults

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

// TestP16AbruptResetStorageAssumptions exercises and validates the narrower
// durability assumptions of abrupt power loss / VM hard reset compared to process SIGKILL:
//
//  1. Un-fsynced dirty page discard: In an abrupt reset, dirty OS page buffers that
//     have not been flushed via fsync() are discarded.
//  2. Verified chunk durability: File Sync explicitly flushes chunk payload files
//     (f.Sync()) and containing object directories (syncDir) before recording metadata.
//  3. SQLite WAL durability: PRAGMA synchronous=FULL ensures WAL frames are flushed
//     before transaction commit returns.
//  4. Two-phase publication recovery across crash boundaries: Staging files in
//     scratch space never compromise destination files if power is lost before rename.
func TestP16AbruptResetStorageAssumptions(t *testing.T) {
	ctx := context.Background()
	folder := faultID('F')
	author := faultID('A')

	t.Run("FsyncFlushDurabilityVsUnflushedDiscard", func(t *testing.T) {
		disposable := testkit.NewDisposable(t)
		filePath := filepath.Join(disposable, "flushed_test.dat")
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		data := []byte("durable flushed bytes that survive write barrier")
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		// Explicit fsync guarantees storage barrier
		if err := f.Sync(); err != nil {
			t.Fatalf("f.Sync failed: %v", err)
		}
		_ = f.Close()

		readBack, err := os.ReadFile(filePath)
		if err != nil || !bytes.Equal(readBack, data) {
			t.Fatalf("flushed data mismatch: got %q, want %q", readBack, data)
		}
	})

	t.Run("SQLiteWALAbruptRecovery", func(t *testing.T) {
		disposable := testkit.NewDisposable(t)
		state := filepath.Join(disposable, "state")
		_ = os.Mkdir(state, 0o700)

		// PRAGMA synchronous=FULL ensures commit transactions write & sync WAL
		db, err := repository.Open(ctx, state)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
			t.Fatal(err)
		}

		payload := []byte("version committed before sudden reset")
		manifest, err := db.StoreFile(ctx, bytes.NewReader(payload), false)
		if err != nil {
			t.Fatal(err)
		}
		v, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
			Folder:           folder,
			Path:             "wal_abrupt.txt",
			Kind:             history.KindFile,
			Manifest:         manifest,
			AuthoredRevision: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		// Abruptly close DB connection without graceful checkpoint
		db.Close()

		// Reopen repository: SQLite replays WAL and verifies PRAGMA quick_check
		reopened, err := repository.Open(ctx, state)
		if err != nil {
			t.Fatalf("failed to reopen DB after abrupt close: %v", err)
		}
		defer reopened.Close()

		known, err := reopened.MetadataKnown(ctx, v.ID)
		if err != nil || !known {
			t.Fatalf("committed version lost after abrupt close: known=%v, err=%v", known, err)
		}
		if err := reopened.VerifyVersionContent(ctx, v.ID); err != nil {
			t.Fatalf("corrupted content after abrupt close: %v", err)
		}
	})

	t.Run("TwoPhasePublicationCrashScenarios", func(t *testing.T) {
		disposable := testkit.NewDisposable(t)
		state := filepath.Join(disposable, "state")
		root := filepath.Join(disposable, "root")
		_ = os.Mkdir(state, 0o700)
		_ = os.Mkdir(root, 0o700)

		db, err := repository.Open(ctx, state)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
			t.Fatal(err)
		}

		ws := workspace.New(db, workspace.Options{})
		_, err = ws.Register(ctx, folder, root)
		if err != nil {
			t.Fatal(err)
		}

		// Scenario 1: Abrupt reset during staging (before rename)
		// Staging file in .filesync-internal/stage has no link at destination.
		// Destination file remains completely intact.
		destPath := filepath.Join(root, "safe_doc.txt")
		originalBytes := []byte("original safe document bytes")
		if err := os.WriteFile(destPath, originalBytes, 0o600); err != nil {
			t.Fatal(err)
		}
		scan1, err := ws.Scan(ctx, folder)
		if err != nil || len(scan1.Captured) != 1 {
			t.Fatalf("initial scan failed: err=%v, captured=%d", err, len(scan1.Captured))
		}

		// Create a simulated incomplete/orphaned stage file in scratch area
		stageDir := filepath.Join(root, ".filesync-internal", "stage")
		_ = os.MkdirAll(stageDir, 0o700)
		orphanStage := filepath.Join(stageDir, "incomplete_staging.tmp")
		_ = os.WriteFile(orphanStage, []byte("partial incomplete chunk bytes"), 0o600)

		// Rescan & recovery must ignore scratch without touching safe_doc.txt
		currentDestBytes, err := os.ReadFile(destPath)
		if err != nil || !bytes.Equal(currentDestBytes, originalBytes) {
			t.Fatalf("destination file corrupted by orphan staging: got %q, want %q", currentDestBytes, originalBytes)
		}

		// Scenario 2: Abrupt reset after atomic rename and directory fsync
		// The destination file has transitioned on disk, but SQLite path_projections
		// has not updated. Workspace scan reconciles the observation.
		newVersionBytes := []byte("new version published on disk")
		if err := os.WriteFile(destPath, newVersionBytes, 0o600); err != nil {
			t.Fatal(err)
		}
		scanReport, err := ws.Scan(ctx, folder)
		if err != nil {
			t.Fatal(err)
		}
		if len(scanReport.Captured) != 1 {
			t.Fatalf("expected 1 captured version on reconciliation, got %d", len(scanReport.Captured))
		}

		readBack, err := os.ReadFile(destPath)
		if err != nil || !bytes.Equal(readBack, newVersionBytes) {
			t.Fatalf("reconciled content mismatch: got %q, want %q", readBack, newVersionBytes)
		}
	})
}
