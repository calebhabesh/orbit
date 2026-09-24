package faults

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

var p16Folder, p16AuthorA, p16AuthorB = faultID('F'), faultID('A'), faultID('B')

// TestP16CheckpointBoundaries exercises pre-operation and post-operation
// crash boundaries around SQLite WAL checkpoints:
// 1. HookBeforeCheckpoint ("sql.checkpoint.before"): crash before WAL truncate/checkpoint.
// 2. HookAfterCheckpoint ("sql.checkpoint.after"): crash after checkpoint completes.
func TestP16CheckpointBoundaries(t *testing.T) {
	for _, hook := range []string{
		repository.HookBeforeCheckpoint,
		repository.HookAfterCheckpoint,
	} {
		t.Run(hook, func(t *testing.T) {
			disposable := testkit.NewDisposable(t)
			state := filepath.Join(disposable, "state")
			if err := os.Mkdir(state, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := testkit.ValidateDestructiveTarget(disposable, state); err != nil {
				t.Fatal(err)
			}

			// Prepare state with folder and committed data in WAL
			ctx := context.Background()
			db, err := repository.Open(ctx, state)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.EnsureFolder(ctx, p16Folder, p16AuthorA, 1); err != nil {
				t.Fatal(err)
			}
			data := []byte("checkpoint boundary data payload")
			manifest, err := db.StoreFile(ctx, bytes.NewReader(data), false)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
				Folder:           p16Folder,
				Path:             "checkpoint_test.txt",
				Kind:             history.KindFile,
				Manifest:         manifest,
				AuthoredRevision: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			db.Close()

			// Launch helper that calls db.Checkpoint() and gets SIGKILLed at hook
			cmd := exec.Command(os.Args[0], "-test.run=^TestP16CheckpointHelper$")
			cmd.Env = append(os.Environ(),
				"FILESYNC_P16_CHECKPOINT_HELPER=1",
				"FILESYNC_P16_STATE="+state,
				"FILESYNC_P16_HOOK="+hook,
			)
			err = cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ProcessState.Success() {
				t.Fatalf("checkpoint helper was not killed at %s: %v", hook, err)
			}

			// Verify post-crash recovery
			reopened, err := repository.Open(ctx, state)
			if err != nil {
				t.Fatalf("failed to reopen repository after crash at %s: %v", hook, err)
			}
			defer reopened.Close()

			verID := history.VersionID{Folder: p16Folder, Author: p16AuthorA, Counter: 1}
			known, err := reopened.MetadataKnown(ctx, verID)
			if err != nil {
				t.Fatal(err)
			}
			if !known {
				t.Fatalf("version lost after checkpoint crash at %s", hook)
			}
			if err := reopened.VerifyVersionContent(ctx, verID); err != nil {
				t.Fatalf("corrupt content after checkpoint crash at %s: %v", hook, err)
			}
		})
	}
}

func TestP16CheckpointHelper(t *testing.T) {
	if os.Getenv("FILESYNC_P16_CHECKPOINT_HELPER") != "1" {
		return
	}
	ctx := context.Background()
	hook := os.Getenv("FILESYNC_P16_HOOK")
	db, err := repository.OpenWithOptions(ctx, os.Getenv("FILESYNC_P16_STATE"), repository.Options{
		FaultHook: func(name string) error {
			if name == hook {
				_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			}
			return nil
		},
	})
	if err != nil {
		os.Exit(80)
	}
	defer db.Close()

	if err := db.Checkpoint(ctx); err != nil {
		os.Exit(81)
	}
	os.Exit(82)
}

// TestP16GCBoundaries exercises process kill and restart across all GC phases:
// 1. HookGCIntent ("gc.intent"): crash after intent recorded in SQLite, before object unlink.
// 2. HookGCUnlink ("gc.unlink"): crash after object file unlinked from disk, before metadata finalization.
// 3. HookGCFinalization ("gc.finalization"): crash before finalization transaction commits.
func TestP16GCBoundaries(t *testing.T) {
	for _, hook := range []string{
		repository.HookGCIntent,
		repository.HookGCUnlink,
		repository.HookGCFinalization,
	} {
		t.Run(hook, func(t *testing.T) {
			disposable := testkit.NewDisposable(t)
			state := filepath.Join(disposable, "state")
			if err := os.Mkdir(state, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := testkit.ValidateDestructiveTarget(disposable, state); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()
			db, err := repository.Open(ctx, state)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.EnsureFolder(ctx, p16Folder, p16AuthorA, 1); err != nil {
				t.Fatal(err)
			}

			// Install an unreferenced object eligible for GC
			unrefBytes := []byte("unreferenced garbage candidate bytes")
			unrefDigest := sha256.Sum256(unrefBytes)
			if err := db.InstallChunk(ctx, unrefDigest, uint64(len(unrefBytes)), bytes.NewReader(unrefBytes)); err != nil {
				t.Fatal(err)
			}
			db.Close()

			// Launch helper that triggers GC and gets killed at hook
			cmd := exec.Command(os.Args[0], "-test.run=^TestP16GCHelper$")
			cmd.Env = append(os.Environ(),
				"FILESYNC_P16_GC_HELPER=1",
				"FILESYNC_P16_STATE="+state,
				"FILESYNC_P16_HOOK="+hook,
			)
			err = cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ProcessState.Success() {
				t.Fatalf("GC helper was not killed at %s: %v", hook, err)
			}

			// Verify post-crash recovery on repository restart
			reopened, err := repository.Open(ctx, state)
			if err != nil {
				t.Fatalf("failed to reopen repository after GC crash at %s: %v", hook, err)
			}
			defer reopened.Close()

			hexDigest := hex.EncodeToString(unrefDigest[:])
			objFile := filepath.Join(state, "objects", "sha256", hexDigest[:2], hexDigest[2:])
			_, fileErr := os.Lstat(objFile)

			switch hook {
			case repository.HookGCIntent:
				// Object was not unlinked yet. Startup reconciliation cancels intent and preserves file.
				if fileErr != nil {
					t.Fatalf("expected object file preserved after crash at HookGCIntent: %v", fileErr)
				}
			case repository.HookGCUnlink, repository.HookGCFinalization:
				// Object was unlinked before crash. Startup reconciliation reconciles missing file,
				// purges metadata reference and removes intent safely (I10).
				if fileErr == nil {
					t.Fatalf("expected object file unlinked after crash at %s", hook)
				}
				orphans, err := reopened.OrphanObjects(ctx)
				if err != nil {
					t.Fatal(err)
				}
				for _, o := range orphans {
					if o == unrefDigest {
						t.Fatalf("unlinked object remains in objects table after startup recovery: %s", hexDigest)
					}
				}
			}
		})
	}
}

func TestP16GCHelper(t *testing.T) {
	if os.Getenv("FILESYNC_P16_GC_HELPER") != "1" {
		return
	}
	ctx := context.Background()
	hook := os.Getenv("FILESYNC_P16_HOOK")
	db, err := repository.OpenWithOptions(ctx, os.Getenv("FILESYNC_P16_STATE"), repository.Options{
		FaultHook: func(name string) error {
			if name == hook {
				_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			}
			return nil
		},
	})
	if err != nil {
		os.Exit(80)
	}
	defer db.Close()

	// Trigger GC on unreferenced objects
	policy := &repository.RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	_, _ = db.RunGC(ctx, p16Folder, policy, time.Now().Add(24*time.Hour))
	os.Exit(82)
}

// TestP16ControlResolutionBoundaries exercises crash and restart during
// conflict resolution and restore operations:
//  1. control.select.committed: crash after resolution envelope committed to DAG
//     and published to workspace, before idempotency record is marked complete.
//  2. control.restore.committed: crash after restore version committed, before idempotency saved.
func TestP16ControlResolutionBoundaries(t *testing.T) {
	for _, hook := range []string{
		"control.select.committed",
		"control.restore.committed",
	} {
		t.Run(hook, func(t *testing.T) {
			disposable := testkit.NewDisposable(t)
			state := filepath.Join(disposable, "state")
			root := filepath.Join(disposable, "root")
			if err := os.Mkdir(state, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := testkit.ValidateDestructiveTarget(disposable, root); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()
			db, err := repository.Open(ctx, state)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.EnsureFolder(ctx, p16Folder, p16AuthorA, 1); err != nil {
				t.Fatal(err)
			}
			ws := workspace.New(db, workspace.Options{})
			if _, err := ws.Register(ctx, p16Folder, root); err != nil {
				t.Fatal(err)
			}

			// Create two concurrent conflict versions A1 and B1 for path "doc.txt"
			contentA := []byte("version from author A")
			if err := os.WriteFile(filepath.Join(root, "doc.txt"), contentA, 0o600); err != nil {
				t.Fatal(err)
			}
			scanReport, err := ws.Scan(ctx, p16Folder)
			if err != nil || len(scanReport.Captured) != 1 {
				t.Fatalf("scan failed: %v, report: %+v", err, scanReport)
			}

			contentB := []byte("concurrent version from author B")
			digestB := history.Digest(sha256.Sum256(contentB))
			manifestB := &history.Manifest{
				Size:       uint64(len(contentB)),
				Digest:     digestB,
				Executable: false,
				Chunks:     []history.Chunk{{Digest: digestB, Length: uint64(len(contentB))}},
			}
			if err := db.InstallChunk(ctx, digestB, uint64(len(contentB)), bytes.NewReader(contentB)); err != nil {
				t.Fatal(err)
			}
			envB := history.Envelope{
				ID:               history.VersionID{Folder: p16Folder, Author: p16AuthorB, Counter: 1},
				Path:             "doc.txt",
				Parents:          nil,
				Vector:           []history.ClockEntry{{Author: p16AuthorB, Counter: 1}},
				Kind:             history.KindFile,
				Manifest:         manifestB,
				AuthoredRevision: 1,
				DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
			}
			if err := db.ImportMetadata(ctx, envB); err != nil {
				t.Fatal(err)
			}
			if err := db.MarkContentReady(ctx, envB.ID); err != nil {
				t.Fatal(err)
			}
			db.Close()

			// Launch helper that executes resolve select or restore, getting SIGKILLed at hook
			cmd := exec.Command(os.Args[0], "-test.run=^TestP16ControlHelper$")
			cmd.Env = append(os.Environ(),
				"FILESYNC_P16_CONTROL_HELPER=1",
				"FILESYNC_P16_STATE="+state,
				"FILESYNC_P16_ROOT="+root,
				"FILESYNC_P16_HOOK="+hook,
			)
			err = cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ProcessState.Success() || exitErr.ExitCode() != -1 {
				t.Fatalf("control helper was not SIGKILLed at %s: %v", hook, err)
			}

			// Verify post-crash recovery on restart
			reopened, err := repository.Open(ctx, state)
			if err != nil {
				t.Fatalf("failed to reopen repository after control crash at %s: %v", hook, err)
			}
			defer reopened.Close()

			// Check that the DAG has progressed to counter 2 covering the operation
			resID := history.VersionID{Folder: p16Folder, Author: p16AuthorA, Counter: 2}
			known, err := reopened.MetadataKnown(ctx, resID)
			if err != nil {
				t.Fatal(err)
			}
			if !known {
				t.Fatalf("expected resolution/restore version committed before crash at %s", hook)
			}

			// Verify workspace contains applied content
			data, err := os.ReadFile(filepath.Join(root, "doc.txt"))
			if err != nil {
				t.Fatalf("expected resolved file in workspace: %v", err)
			}
			if !bytes.Equal(data, contentA) {
				t.Fatalf("workspace content mismatch after restart: got %q, want %q", data, contentA)
			}
		})
	}
}

func TestP16ControlHelper(t *testing.T) {
	if os.Getenv("FILESYNC_P16_CONTROL_HELPER") != "1" {
		return
	}
	ctx := context.Background()
	state := os.Getenv("FILESYNC_P16_STATE")
	hook := os.Getenv("FILESYNC_P16_HOOK")

	db, err := repository.Open(ctx, state)
	if err != nil {
		os.Exit(80)
	}
	ws := workspace.New(db, workspace.Options{})

	ctrl := control.New(db, ws, control.Options{
		FaultHook: func(name string) error {
			if name == hook {
				_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			}
			return nil
		},
		Now: time.Now,
	})

	switch hook {
	case "control.select.committed":
		conflicts, _, err := ctrl.Conflicts(ctx, p16Folder)
		if err != nil || len(conflicts) == 0 {
			os.Exit(82)
		}
		item := conflicts[0]
		var reviewed []history.VersionID
		for _, h := range item.Heads {
			reviewed = append(reviewed, h.ID)
		}
		_, err = ctrl.ResolveSelect(ctx, control.ResolveSelectRequest{
			Folder:            p16Folder,
			Path:              "doc.txt",
			Reviewed:          reviewed,
			ExpectedHeadToken: item.HeadToken,
			Selected:          history.VersionID{Folder: p16Folder, Author: p16AuthorA, Counter: 1},
			IdempotencyKey:    "test-select-idempotency",
		})
		if err != nil {
			os.Exit(83)
		}
	case "control.restore.committed":
		verAID := history.VersionID{Folder: p16Folder, Author: p16AuthorA, Counter: 1}
		conflicts, _, _ := ctrl.Conflicts(ctx, p16Folder)
		var reviewed []history.VersionID
		var headToken history.Digest
		if len(conflicts) > 0 {
			for _, h := range conflicts[0].Heads {
				reviewed = append(reviewed, h.ID)
			}
			headToken = conflicts[0].HeadToken
		}
		_, err := ctrl.Restore(ctx, control.RestoreRequest{
			Folder:            p16Folder,
			Path:              "doc.txt",
			SourceVersion:     verAID,
			Reviewed:          reviewed,
			ExpectedHeadToken: headToken,
			IdempotencyKey:    "test-restore-idempotency",
		})
		if err != nil {
			os.Exit(84)
		}
	}
	os.Exit(85)
}

// TestP16IntegrityQuarantineRepairBoundaries exercises crash and restart
// across chunk quarantine and verified peer repair installation:
// 1. HookChunkQuarantined ("integrity.chunk.quarantined"): crash after corrupt chunk moved to quarantine.
// 2. HookRepairInstalled ("repair.installed"): crash after repair chunk installed into objects/.
func TestP16IntegrityQuarantineRepairBoundaries(t *testing.T) {
	for _, hook := range []string{
		repository.HookChunkQuarantined,
		repository.HookRepairInstalled,
	} {
		t.Run(hook, func(t *testing.T) {
			disposable := testkit.NewDisposable(t)
			state := filepath.Join(disposable, "state")
			if err := os.Mkdir(state, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := testkit.ValidateDestructiveTarget(disposable, state); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()
			db, err := repository.Open(ctx, state)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.EnsureFolder(ctx, p16Folder, p16AuthorA, 1); err != nil {
				t.Fatal(err)
			}

			cleanData := []byte("clean chunk data before quarantine test")
			digest := sha256.Sum256(cleanData)
			if err := db.InstallChunk(ctx, digest, uint64(len(cleanData)), bytes.NewReader(cleanData)); err != nil {
				t.Fatal(err)
			}
			manifest, err := db.StoreFile(ctx, bytes.NewReader(cleanData), false)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
				Folder:           p16Folder,
				Path:             "corruptible.txt",
				Kind:             history.KindFile,
				Manifest:         manifest,
				AuthoredRevision: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			db.Close()

			// Launch helper that executes quarantine or repair and gets SIGKILLed at hook
			cmd := exec.Command(os.Args[0], "-test.run=^TestP16IntegrityHelper$")
			cmd.Env = append(os.Environ(),
				"FILESYNC_P16_INTEGRITY_HELPER=1",
				"FILESYNC_P16_STATE="+state,
				"FILESYNC_P16_HOOK="+hook,
				"FILESYNC_P16_DIGEST="+hex.EncodeToString(digest[:]),
			)
			err = cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ProcessState.Success() {
				t.Fatalf("integrity helper was not killed at %s: %v", hook, err)
			}

			// Verify post-crash recovery on restart
			reopened, err := repository.Open(ctx, state)
			if err != nil {
				t.Fatalf("failed to reopen repository after integrity crash at %s: %v", hook, err)
			}
			defer reopened.Close()

			isQuarantined, _, err := reopened.IsChunkQuarantined(ctx, digest)
			if err != nil {
				t.Fatal(err)
			}

			hexDigest := hex.EncodeToString(digest[:])
			switch hook {
			case repository.HookChunkQuarantined:
				// Chunk was moved to quarantine before crash; metadata reflects quarantine
				if !isQuarantined {
					t.Fatalf("expected chunk in quarantine after HookChunkQuarantined crash")
				}
				// Verify chunk is not in objects/
				objPath := filepath.Join(state, "objects", "sha256", hexDigest[:2], hexDigest)
				if _, err := os.Lstat(objPath); err == nil {
					t.Fatalf("quarantined chunk still exists in objects/ directory")
				}
			case repository.HookRepairInstalled:
				// Chunk unquarantined and repaired in objects/
				if isQuarantined {
					t.Fatalf("expected chunk repaired after HookRepairInstalled crash")
				}
			}
		})
	}
}

func TestP16IntegrityHelper(t *testing.T) {
	if os.Getenv("FILESYNC_P16_INTEGRITY_HELPER") != "1" {
		return
	}
	ctx := context.Background()
	hook := os.Getenv("FILESYNC_P16_HOOK")
	state := os.Getenv("FILESYNC_P16_STATE")
	dHex := os.Getenv("FILESYNC_P16_DIGEST")

	raw, err := hex.DecodeString(dHex)
	if err != nil || len(raw) != 32 {
		os.Exit(80)
	}
	var digest history.Digest
	copy(digest[:], raw)

	db, err := repository.OpenWithOptions(ctx, state, repository.Options{
		FaultHook: func(name string) error {
			if name == hook {
				_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			}
			return nil
		},
	})
	if err != nil {
		os.Exit(81)
	}
	defer db.Close()

	switch hook {
	case repository.HookChunkQuarantined:
		_, _ = db.QuarantineChunk(ctx, digest, "bit rot detected")
	case repository.HookRepairInstalled:
		// First quarantine, then unquarantine/repair
		_, _ = db.QuarantineChunk(ctx, digest, "bit rot detected")
		_ = db.UnquarantineChunk(ctx, digest)
	}
	os.Exit(82)
}
