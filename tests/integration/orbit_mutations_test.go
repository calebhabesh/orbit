package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/state"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
)

// setupMutationNode creates an initialized Orbit test node and registered folder with sync root.
func setupMutationNode(t *testing.T, label string) (*control.Controller, *workspace.Workspace, *repository.DB, string, string, history.ID, func()) {
	t.Helper()
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)

	stateDir := filepath.Join(disposable, "state-"+label)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	syncRoot := filepath.Join(disposable, "sync-"+label)
	if err := os.MkdirAll(syncRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	rawDev := make([]byte, 32)
	_, _ = rand.Read(rawDev)
	var localDevice history.ID
	copy(localDevice[:], rawDev)

	coreCfg := config.Config{
		FormatVersion: 1,
		DeviceID:      hex.EncodeToString(rawDev),
		CreatedAt:     time.Now().UTC(),
	}
	if err := config.Save(stateDir, coreCfg); err != nil {
		t.Fatal(err)
	}

	if _, err := replication.LoadOrCreateIdentity(stateDir, localDevice, time.Now()); err != nil {
		t.Fatal(err)
	}

	st := config.ProductSettings{
		FormatVersion: 1,
		DeviceLabel:   label,
	}
	if err := config.SaveSettings(stateDir, st); err != nil {
		t.Fatal(err)
	}

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{
		LocalDevice: localDevice,
	})

	var folderID history.ID
	_, _ = rand.Read(folderID[:])
	if err := db.EnsureFolder(ctx, folderID, localDevice, 1); err != nil {
		t.Fatal(err)
	}

	pin := sha256.Sum256(localDevice[:])
	if _, err := db.ApproveMembership(ctx, protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active:      []protocol.ActiveMember{{Device: localDevice, KeyPin: pin}},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := ctrl.RegisterFolder(ctx, folderID, syncRoot); err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		_ = db.Close()
	}

	return ctrl, ws, db, stateDir, syncRoot, folderID, cleanup
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	return string(b)
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestOrbitImport verifies file import, destination collision, recovery displacement, and idempotency.
func TestOrbitImport(t *testing.T) {
	ctx := context.Background()
	ctrl, ws, _, _, syncRoot, folderID, cleanup := setupMutationNode(t, "node-import")
	defer cleanup()

	// 1. Fresh file import
	content1 := "Hello Orbit Import v1"
	res1, err := ctrl.ImportFile(ctx, control.ImportFileRequest{
		Folder:         folderID,
		Path:           "docs/readme.txt",
		IdempotencyKey: "import-1",
	}, strings.NewReader(content1), uint64(len(content1)))
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}
	if !res1.Completed {
		t.Fatal("expected import to be completed")
	}

	targetPath := filepath.Join(syncRoot, "docs", "readme.txt")
	if got := mustReadFile(t, targetPath); got != content1 {
		t.Fatalf("expected content %q, got %q", content1, got)
	}

	// Invariant I17: scan-after-action does not fabricate feedback edits
	scanRes, err := ws.Scan(ctx, folderID)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(scanRes.Captured) != 0 {
		t.Fatalf("expected 0 captured files on scan-after-import, got %d", len(scanRes.Captured))
	}

	// 2. Collision without overwrite approval must be rejected (HTTP 409 / DESTINATION_EXISTS)
	_, err = ctrl.ImportFile(ctx, control.ImportFileRequest{
		Folder:    folderID,
		Path:      "docs/readme.txt",
		Overwrite: false,
	}, strings.NewReader("Attempted overwrite"), uint64(len("Attempted overwrite")))
	if err == nil {
		t.Fatal("expected ErrDestinationExists when overwrite is false")
	}
	var ce *control.ControlError
	if !errors.As(err, &ce) || ce.Code != "DESTINATION_EXISTS" {
		t.Fatalf("expected DESTINATION_EXISTS error code, got: %v", err)
	}
	// Verify content on disk was NOT modified
	if got := mustReadFile(t, targetPath); got != content1 {
		t.Fatalf("expected original content to remain, got %q", got)
	}

	// 3. Overwrite approved: displaces previous destination into recovery scratch
	content2 := "Hello Orbit Import v2 Overwritten"
	res2, err := ctrl.ImportFile(ctx, control.ImportFileRequest{
		Folder:         folderID,
		Path:           "docs/readme.txt",
		Overwrite:      true,
		OperationID:    "op-import-ovw",
		IdempotencyKey: "import-2",
	}, strings.NewReader(content2), uint64(len(content2)))
	if err != nil {
		t.Fatalf("ImportFile with overwrite failed: %v", err)
	}
	if !res2.Completed {
		t.Fatal("expected overwrite import to complete")
	}
	if got := mustReadFile(t, targetPath); got != content2 {
		t.Fatalf("expected updated content %q, got %q", content2, got)
	}

	// Verify displaced destination is preserved in recovery storage!
	scratchRecovery := filepath.Join(syncRoot, ".orbit-internal", "recovery-op-import-ovw")
	if got := mustReadFile(t, scratchRecovery); got != content1 {
		t.Fatalf("expected recovery file to preserve original bytes %q, got %q", content1, got)
	}

	// 4. Idempotent replay: exact duplicate request returns cached result
	resReplay, err := ctrl.ImportFile(ctx, control.ImportFileRequest{
		Folder:         folderID,
		Path:           "docs/readme.txt",
		Overwrite:      true,
		OperationID:    "op-import-ovw",
		IdempotencyKey: "import-2",
	}, strings.NewReader(content2), uint64(len(content2)))
	if err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}
	if resReplay.VersionID != res2.VersionID {
		t.Fatalf("expected identical VersionID on replay, got %v vs %v", resReplay.VersionID, res2.VersionID)
	}

	// 5. Changed parameters with same idempotency key fails with IDEMPOTENCY_CONFLICT
	_, err = ctrl.ImportFile(ctx, control.ImportFileRequest{
		Folder:         folderID,
		Path:           "docs/other.txt", // changed path
		IdempotencyKey: "import-2",
	}, strings.NewReader("different"), 9)
	if err == nil {
		t.Fatal("expected IDEMPOTENCY_CONFLICT on changed parameters")
	}
	if !errors.As(err, &ce) || ce.Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("expected IDEMPOTENCY_CONFLICT, got: %v", err)
	}
}

// TestOrbitMkdir verifies directory creation, scaffolding, and structural conflict handling.
func TestOrbitMkdir(t *testing.T) {
	ctx := context.Background()
	ctrl, _, _, _, syncRoot, folderID, cleanup := setupMutationNode(t, "node-mkdir")
	defer cleanup()

	// 1. Nested directory creation
	res, err := ctrl.CreateDir(ctx, control.CreateDirRequest{
		Folder:         folderID,
		Path:           "work/projects/alpha",
		IdempotencyKey: "mkdir-1",
	})
	if err != nil {
		t.Fatalf("CreateDir failed: %v", err)
	}
	if !res.Completed {
		t.Fatal("expected completed mkdir")
	}

	dirPath := filepath.Join(syncRoot, "work", "projects", "alpha")
	st, err := os.Stat(dirPath)
	if err != nil || !st.IsDir() {
		t.Fatalf("expected directory at %s, err: %v", dirPath, err)
	}

	// 2. Repeat mkdir on existing directory returns AlreadyExists without error
	resRepeat, err := ctrl.CreateDir(ctx, control.CreateDirRequest{
		Folder: folderID,
		Path:   "work/projects/alpha",
	})
	if err != nil {
		t.Fatalf("repeat CreateDir failed: %v", err)
	}
	if !resRepeat.AlreadyExists {
		t.Fatal("expected AlreadyExists = true")
	}

	// 3. Collision with regular file returns STRUCTURAL_CONFLICT
	fileCollisionPath := filepath.Join(syncRoot, "work", "file.txt")
	mustWriteFile(t, fileCollisionPath, "regular file")
	_, err = ctrl.CreateDir(ctx, control.CreateDirRequest{
		Folder: folderID,
		Path:   "work/file.txt",
	})
	if err == nil {
		t.Fatal("expected STRUCTURAL_CONFLICT when mkdir conflicts with file")
	}
	var ce *control.ControlError
	if !errors.As(err, &ce) || ce.Code != "STRUCTURAL_CONFLICT" {
		t.Fatalf("expected STRUCTURAL_CONFLICT, got: %v", err)
	}
}

// TestOrbitMove verifies file move, overwrite displacement, concurrent source editor race (Invariant I26),
// directory subtree move, and subtree invalidation.
func TestOrbitMove(t *testing.T) {
	ctx := context.Background()
	ctrl, ws, _, _, syncRoot, folderID, cleanup := setupMutationNode(t, "node-move")
	defer cleanup()

	// 1. Regular file move
	srcRel := "notes/draft.md"
	dstRel := "archive/final_draft.md"
	mustWriteFile(t, filepath.Join(syncRoot, srcRel), "# Draft Content")

	// Capture initial observation via Scan
	_, _ = ws.Scan(ctx, folderID)

	moveRes, err := ctrl.MoveFile(ctx, control.MoveFileRequest{
		Folder:         folderID,
		SourcePath:     srcRel,
		DestPath:       dstRel,
		IdempotencyKey: "move-1",
	})
	if err != nil {
		t.Fatalf("MoveFile failed: %v", err)
	}
	if !moveRes.Completed || moveRes.SourceRetained {
		t.Fatalf("expected clean move without retention: %+v", moveRes)
	}

	// Destination must exist, source must be removed
	if got := mustReadFile(t, filepath.Join(syncRoot, dstRel)); got != "# Draft Content" {
		t.Fatalf("destination content mismatch: %q", got)
	}
	if _, err := os.Stat(filepath.Join(syncRoot, srcRel)); !os.IsNotExist(err) {
		t.Fatalf("expected source to be unlinked, stat returned: %v", err)
	}

	// 2. Invariant I26: Concurrent Source Modification Race
	// If editor modifies source file concurrently during move, source is NOT deleted!
	fileA := "docA.txt"
	fileB := "docB.txt"
	mustWriteFile(t, filepath.Join(syncRoot, fileA), "initial-version-A")
	_, _ = ws.Scan(ctx, folderID)

	// Calculate initial hash
	h := sha256.Sum256([]byte("initial-version-A"))
	reviewedHash := hex.EncodeToString(h[:])

	// Concurrently modify fileA on disk before Move verifies source
	mustWriteFile(t, filepath.Join(syncRoot, fileA), "concurrent-editor-edits-A2")

	raceRes, err := ctrl.MoveFile(ctx, control.MoveFileRequest{
		Folder:        folderID,
		SourcePath:    fileA,
		DestPath:      fileB,
		ReviewedToken: reviewedHash, // basis was initial version
	})
	if err != nil {
		t.Fatalf("MoveFile race failed: %v", err)
	}

	// Must have SourceRetained = true (Invariant I26)
	if !raceRes.SourceRetained {
		t.Fatal("expected SourceRetained = true under concurrent editor race")
	}

	// Both files must be preserved on disk!
	if got := mustReadFile(t, filepath.Join(syncRoot, fileA)); got != "concurrent-editor-edits-A2" {
		t.Fatalf("expected source to retain editor edits, got: %q", got)
	}
	if got := mustReadFile(t, filepath.Join(syncRoot, fileB)); got != "initial-version-A" {
		t.Fatalf("expected destination to retain initial version, got: %q", got)
	}

	// 3. Directory Move
	dirSrc := "projects/v1"
	dirDst := "projects/v2"
	mustWriteFile(t, filepath.Join(syncRoot, dirSrc, "main.go"), "package main")
	mustWriteFile(t, filepath.Join(syncRoot, dirSrc, "sub", "util.go"), "package util")
	_, _ = ws.Scan(ctx, folderID)

	dirRes, err := ctrl.MoveFile(ctx, control.MoveFileRequest{
		Folder:     folderID,
		SourcePath: dirSrc,
		DestPath:   dirDst,
	})
	if err != nil {
		t.Fatalf("Directory move failed: %v", err)
	}
	if !dirRes.Completed {
		t.Fatal("expected directory move to complete")
	}

	// Verify destination tree
	if got := mustReadFile(t, filepath.Join(syncRoot, dirDst, "main.go")); got != "package main" {
		t.Fatalf("dst main.go content mismatch: %q", got)
	}
	if got := mustReadFile(t, filepath.Join(syncRoot, dirDst, "sub", "util.go")); got != "package util" {
		t.Fatalf("dst util.go content mismatch: %q", got)
	}
	// Verify source tree removed
	if _, err := os.Stat(filepath.Join(syncRoot, dirSrc)); !os.IsNotExist(err) {
		t.Fatalf("expected source directory to be removed, got: %v", err)
	}

	// 4. Directory Subtree Invalidation: new child added before completion
	dirA := "watched_module"
	dirB := "watched_module_moved"
	mustWriteFile(t, filepath.Join(syncRoot, dirA, "core.go"), "package core")
	_, _ = ws.Scan(ctx, folderID)

	// Calculate subtree token
	hSub := sha256.New()
	hSub.Write([]byte("core.go:file;"))
	staleToken := hex.EncodeToString(hSub.Sum(nil))

	// Add new child file before move
	mustWriteFile(t, filepath.Join(syncRoot, dirA, "new_extra.go"), "package core; func Extra() {}")

	// Attempt move with stale subtree token
	_, err = ctrl.MoveFile(ctx, control.MoveFileRequest{
		Folder:        folderID,
		SourcePath:    dirA,
		DestPath:      dirB,
		ReviewedToken: staleToken,
	})
	if err == nil {
		t.Fatal("expected SUBTREE_INVALIDATED when new child appears")
	}
	var ce *control.ControlError
	if !errors.As(err, &ce) || ce.Code != "SUBTREE_INVALIDATED" {
		t.Fatalf("expected SUBTREE_INVALIDATED error code, got: %v", err)
	}
	// Verify source directory and its contents remain intact
	if _, err := os.Stat(filepath.Join(syncRoot, dirA, "new_extra.go")); err != nil {
		t.Fatalf("expected source to remain intact: %v", err)
	}
}

// TestOrbitDelete verifies file deletion, non-empty directory rejection without recursive,
// and full recursive directory removal.
func TestOrbitDelete(t *testing.T) {
	ctx := context.Background()
	ctrl, ws, _, _, syncRoot, folderID, cleanup := setupMutationNode(t, "node-delete")
	defer cleanup()

	// 1. Single file deletion
	filePath := "temp/scratch.log"
	mustWriteFile(t, filepath.Join(syncRoot, filePath), "log contents")
	_, _ = ws.Scan(ctx, folderID)

	delRes, err := ctrl.DeleteFile(ctx, control.DeleteFileRequest{
		Folder:         folderID,
		Path:           filePath,
		IdempotencyKey: "del-1",
	})
	if err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}
	if !delRes.Completed || delRes.DeletedCount != 1 {
		t.Fatalf("unexpected DeleteResult: %+v", delRes)
	}
	if _, err := os.Stat(filepath.Join(syncRoot, filePath)); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted, got err: %v", err)
	}

	// 2. Non-empty directory delete without recursive flag fails with DIRECTORY_NOT_EMPTY
	dirRel := "data_bundle"
	mustWriteFile(t, filepath.Join(syncRoot, dirRel, "part1.bin"), "data1")
	mustWriteFile(t, filepath.Join(syncRoot, dirRel, "part2.bin"), "data2")
	_, _ = ws.Scan(ctx, folderID)

	_, err = ctrl.DeleteFile(ctx, control.DeleteFileRequest{
		Folder:    folderID,
		Path:      dirRel,
		Recursive: false,
	})
	if err == nil {
		t.Fatal("expected DIRECTORY_NOT_EMPTY when recursive is false")
	}
	var ce *control.ControlError
	if !errors.As(err, &ce) || ce.Code != "DIRECTORY_NOT_EMPTY" {
		t.Fatalf("expected DIRECTORY_NOT_EMPTY error code, got: %v", err)
	}

	// 3. Recursive directory delete succeeds
	recDelRes, err := ctrl.DeleteFile(ctx, control.DeleteFileRequest{
		Folder:    folderID,
		Path:      dirRel,
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("recursive DeleteFile failed: %v", err)
	}
	if !recDelRes.Completed || recDelRes.DeletedCount < 3 { // part1, part2, and dirRel
		t.Fatalf("unexpected recursive delete result: %+v", recDelRes)
	}
	if _, err := os.Stat(filepath.Join(syncRoot, dirRel)); !os.IsNotExist(err) {
		t.Fatalf("expected directory to be removed from disk, got: %v", err)
	}
}

// TestOrbitMutation exercises fault injection across durable journal phases, ENOSPC/budget checks,
// and CLI stopped vs live daemon parity (Invariant I19).
func TestOrbitMutation(t *testing.T) {
	ctx := context.Background()

	t.Run("FaultHooksAndJournalRecovery", func(t *testing.T) {
		disposable := testkit.NewDisposable(t)
		stateDir := filepath.Join(disposable, "state-fault")
		syncRoot := filepath.Join(disposable, "sync-fault")
		_ = os.MkdirAll(stateDir, 0o700)
		_ = os.MkdirAll(syncRoot, 0o700)

		rawDev := make([]byte, 32)
		_, _ = rand.Read(rawDev)
		var localDevice history.ID
		copy(localDevice[:], rawDev)
		_ = config.Save(stateDir, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(rawDev), CreatedAt: time.Now().UTC()})
		_, _ = replication.LoadOrCreateIdentity(stateDir, localDevice, time.Now())
		_ = config.SaveSettings(stateDir, config.ProductSettings{FormatVersion: 1, DeviceLabel: "fault-node"})

		db, err := repository.Open(ctx, stateDir)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		var folderID history.ID
		_, _ = rand.Read(folderID[:])
		_ = db.EnsureFolder(ctx, folderID, localDevice, 1)
		pin := sha256.Sum256(localDevice[:])
		_, _ = db.ApproveMembership(ctx, protocol.Membership{Folder: folderID, Revision: 1, Active: []protocol.ActiveMember{{Device: localDevice, KeyPin: pin}}})

		// 1. Injected fault at HookFileMutationPlanned
		injectedErr := errors.New("injected crash at PLANNED phase")
		wsWithFault := workspace.New(db, workspace.Options{
			FaultHook: func(hook string) error {
				if hook == workspace.HookFileMutationPlanned {
					return injectedErr
				}
				return nil
			},
		})
		_, _ = wsWithFault.Register(ctx, folderID, syncRoot)

		_, err = wsWithFault.ImportFile(ctx, workspace.ImportRequest{
			Folder:      folderID,
			Path:        "crash_planned.txt",
			Source:      strings.NewReader("content"),
			Size:        7,
			OperationID: "op-fault-planned",
		})
		if !errors.Is(err, injectedErr) {
			t.Fatalf("expected injected fault error, got: %v", err)
		}

		// Reopen/recover workspace: journal recovery aborts incomplete PLANNED mutation
		wsClean := workspace.New(db, workspace.Options{})
		if err := wsClean.Recover(ctx, folderID); err != nil {
			t.Fatalf("Recover failed: %v", err)
		}
		mut, err := db.GetFileMutation(ctx, "op-fault-planned")
		if err != nil {
			t.Fatalf("GetFileMutation failed: %v", err)
		}
		if mut.Phase != "ABORTED" {
			t.Fatalf("expected mutation phase to be ABORTED, got: %s", mut.Phase)
		}

		// 2. Injected fault after destination installed during move
		srcFile := filepath.Join(syncRoot, "move_src.txt")
		mustWriteFile(t, srcFile, "move source bytes")
		_, _ = wsClean.Scan(ctx, folderID)

		wsMoveFault := workspace.New(db, workspace.Options{
			FaultHook: func(hook string) error {
				if hook == workspace.HookFileMutationInstalled {
					return errors.New("injected crash after INSTALLED")
				}
				return nil
			},
		})
		_, err = wsMoveFault.Move(ctx, workspace.MoveRequest{
			Folder:      folderID,
			SourcePath:  "move_src.txt",
			DestPath:    "move_dst.txt",
			OperationID: "op-move-crash",
		})
		if err == nil {
			t.Fatal("expected fault after INSTALLED")
		}

		// Destination was installed on disk
		dstFile := filepath.Join(syncRoot, "move_dst.txt")
		if got := mustReadFile(t, dstFile); got != "move source bytes" {
			t.Fatalf("expected destination to be installed: %q", got)
		}

		// Recover: resume re-verifying source and completing move
		if err := wsClean.Recover(ctx, folderID); err != nil {
			t.Fatalf("Recover failed: %v", err)
		}
		mutMove, err := db.GetFileMutation(ctx, "op-move-crash")
		if err != nil {
			t.Fatalf("GetFileMutation failed: %v", err)
		}
		if mutMove.Phase != "COMPLETED" {
			t.Fatalf("expected recovered mutation phase to be COMPLETED, got: %s", mutMove.Phase)
		}
		// Source file should now be unlinked safely by recovery
		if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
			t.Fatalf("expected source file to be removed after recovery, got err: %v", err)
		}
	})

	t.Run("CLIPartityStoppedAndLiveDaemon", func(t *testing.T) {
		// Test CLI subcommands (mkdir, import, move, delete) under stopped CLI and live daemon (Invariant I19)
		_, _, db, stateDir, syncRoot, folderID, cleanup := setupMutationNode(t, "node-cli")
		defer cleanup()

		folderHex := hex.EncodeToString(folderID[:])
		binPath, err := filepath.Abs("../../bin/orbit")
		if err != nil {
			t.Fatal(err)
		}

		// Prepare local file for import
		localTmp := filepath.Join(testkit.NewDisposable(t), "local_sample.txt")
		mustWriteFile(t, localTmp, "Sample CLI import payload")

		// --- Part A: Stopped CLI ---
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		// 1. orbit mkdir
		cmdMkdir := exec.Command(binPath, "mkdir", "--state", stateDir, "--folder", folderHex, "--path", "cli_test_dir")
		out, err := cmdMkdir.CombinedOutput()
		if err != nil {
			t.Fatalf("orbit mkdir failed: %v\nOutput: %s", err, string(out))
		}
		var mkdirRes control.CreateDirResult
		if err := json.Unmarshal(out, &mkdirRes); err != nil || !mkdirRes.Completed {
			t.Fatalf("unexpected mkdir output: %s", string(out))
		}

		// 2. orbit import
		cmdImport := exec.Command(binPath, "import", "--state", stateDir, "--folder", folderHex, "--path", "cli_test_dir/file1.txt", "--file", localTmp)
		out, err = cmdImport.CombinedOutput()
		if err != nil {
			t.Fatalf("orbit import failed: %v\nOutput: %s", err, string(out))
		}
		var impRes control.ImportFileResult
		if err := json.Unmarshal(out, &impRes); err != nil || !impRes.Completed {
			t.Fatalf("unexpected import output: %s", string(out))
		}

		// 3. orbit move
		cmdMove := exec.Command(binPath, "move", "--state", stateDir, "--folder", folderHex, "--source", "cli_test_dir/file1.txt", "--dest", "cli_test_dir/file1_renamed.txt")
		out, err = cmdMove.CombinedOutput()
		if err != nil {
			t.Fatalf("orbit move failed: %v\nOutput: %s", err, string(out))
		}
		var moveRes control.MoveFileResult
		if err := json.Unmarshal(out, &moveRes); err != nil || !moveRes.Completed {
			t.Fatalf("unexpected move output: %s", string(out))
		}

		// 4. orbit delete
		cmdDelete := exec.Command(binPath, "delete", "--state", stateDir, "--folder", folderHex, "--path", "cli_test_dir", "--recursive")
		out, err = cmdDelete.CombinedOutput()
		if err != nil {
			t.Fatalf("orbit delete failed: %v\nOutput: %s", err, string(out))
		}
		var delRes control.DeleteFileResult
		if err := json.Unmarshal(out, &delRes); err != nil || !delRes.Completed {
			t.Fatalf("unexpected delete output: %s", string(out))
		}
		if _, err := os.Stat(filepath.Join(syncRoot, "cli_test_dir")); !os.IsNotExist(err) {
			t.Fatalf("expected cli_test_dir to be deleted")
		}

		// --- Part B: Live Daemon CLI ---
		// Start control server and write daemon lock
		db, err = repository.Open(context.Background(), stateDir)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		ctrl := control.New(db, workspace.New(db, workspace.Options{}))
		srv, err := control.NewServer(ctrl, stateDir)
		if err != nil {
			t.Fatal(err)
		}
		httpSrv := httptest.NewServer(srv.Handler())
		defer httpSrv.Close()

		ownership, err := state.Acquire(stateDir)
		if err != nil {
			t.Fatal(err)
		}
		defer ownership.Close()
		if err := config.WritePrivate(stateDir, "control.addr", []byte(strings.TrimPrefix(httpSrv.URL, "http://"))); err != nil {
			t.Fatal(err)
		}

		// 1. Live orbit mkdir
		cmdLiveMkdir := exec.Command(binPath, "mkdir", "--state", stateDir, "--folder", folderHex, "--path", "live_dir")
		out, err = cmdLiveMkdir.CombinedOutput()
		if err != nil {
			t.Fatalf("live orbit mkdir failed: %v\nOutput: %s", err, string(out))
		}
		if err := json.Unmarshal(out, &mkdirRes); err != nil || !mkdirRes.Completed {
			t.Fatalf("unexpected live mkdir output: %s", string(out))
		}

		// 2. Live orbit import
		cmdLiveImport := exec.Command(binPath, "import", "--state", stateDir, "--folder", folderHex, "--path", "live_dir/sample.txt", "--file", localTmp)
		out, err = cmdLiveImport.CombinedOutput()
		if err != nil {
			t.Fatalf("live orbit import failed: %v\nOutput: %s", err, string(out))
		}
		if err := json.Unmarshal(out, &impRes); err != nil || !impRes.Completed {
			t.Fatalf("unexpected live import output: %s", string(out))
		}

		// 3. Live orbit move
		cmdLiveMove := exec.Command(binPath, "move", "--state", stateDir, "--folder", folderHex, "--source", "live_dir/sample.txt", "--dest", "live_dir/sample_moved.txt")
		out, err = cmdLiveMove.CombinedOutput()
		if err != nil {
			t.Fatalf("live orbit move failed: %v\nOutput: %s", err, string(out))
		}
		if err := json.Unmarshal(out, &moveRes); err != nil || !moveRes.Completed {
			t.Fatalf("unexpected live move output: %s", string(out))
		}

		// 4. Live orbit delete
		cmdLiveDel := exec.Command(binPath, "delete", "--state", stateDir, "--folder", folderHex, "--path", "live_dir", "--recursive")
		out, err = cmdLiveDel.CombinedOutput()
		if err != nil {
			t.Fatalf("live orbit delete failed: %v\nOutput: %s", err, string(out))
		}
		if err := json.Unmarshal(out, &delRes); err != nil || !delRes.Completed {
			t.Fatalf("unexpected live delete output: %s", string(out))
		}

		_ = db
	})
}
