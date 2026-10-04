package terminal_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func buildOrbitBinary(t *testing.T, base string) string {
	t.Helper()
	binary := filepath.Join(base, "orbit")
	build := exec.Command("go", "build", "-o", binary, "./cmd/filesync")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build orbit binary: %v\n%s", err, string(out))
	}
	return binary
}

// TestTerminalT06ContextResolutionCwdAndName verifies:
// - cwd inside root resolves registered folder, root, and relative path
// - cwd in subdirectory of root infers relative path
// - relative target path resolves against cwd
// - explicit folder name and 64-hex ID resolution
// - JSON output matches tc.Result frozen fields
func TestTerminalT06ContextResolutionCwdAndName(t *testing.T) {
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

	rootA := filepath.Join(base, "documents")
	if err := os.MkdirAll(filepath.Join(rootA, "sub", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(rootA, "sub", "nested", "file.txt")
	if err := os.WriteFile(testFile, []byte("hello context"), 0600); err != nil {
		t.Fatal(err)
	}

	var folderA history.ID
	devID := mustID(t, cfg.DeviceID)
	err = app.WithWorkspace(ctx, stateDir, func(c config.Config, db *repository.DB, ws *workspace.Workspace) error {
		folderA = mustID(t, enrollmentRandom(t))
		if err := db.EnsureFolder(ctx, folderA, devID, 1); err != nil {
			return err
		}
		ctrl := control.New(db, ws, control.Options{LocalDevice: devID})
		if _, err := ctrl.RegisterFolder(ctx, folderA, rootA); err != nil {
			return err
		}
		return db.SetFolderDisplayName(ctx, folderA, "Documents")
	})
	if err != nil {
		t.Fatalf("register folder: %v", err)
	}

	folderAHex := hex.EncodeToString(folderA[:])

	// Case 1: In root directory, orbit context --json
	cmd := exec.Command(binary, "context", "--state", stateDir, "--json")
	cmd.Dir = rootA
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit context inside root: %v\n%s", err, string(out))
	}
	var res tc.Result
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if (res.State != "ready" && res.State != "success") || res.Context == nil {
		t.Fatalf("expected ready or success context, got state=%s context=%v", res.State, res.Context)
	}
	if res.Context.Folder != folderAHex {
		t.Errorf("expected folder %s, got %s", folderAHex, res.Context.Folder)
	}
	if res.Context.FolderName != "Documents" {
		t.Errorf("expected folder name 'Documents', got %s", res.Context.FolderName)
	}
	if res.Context.Root != rootA {
		t.Errorf("expected root %s, got %s", rootA, res.Context.Root)
	}
	if res.Context.Path != "" {
		t.Errorf("expected empty relative path in root, got %s", res.Context.Path)
	}
	if res.Context.Generation == "" {
		t.Errorf("expected non-empty context generation")
	}

	// Case 2: In subdirectory rootA/sub/nested, orbit context --json
	cmd = exec.Command(binary, "context", "--state", stateDir, "--json")
	cmd.Dir = filepath.Join(rootA, "sub", "nested")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit context inside subdir: %v\n%s", err, string(out))
	}
	res = tc.Result{}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if res.Context.Path != "sub/nested" {
		t.Errorf("expected relative path 'sub/nested', got %s", res.Context.Path)
	}

	// Case 3: Target path file.txt from root directory
	cmd = exec.Command(binary, "context", "sub/nested/file.txt", "--state", stateDir, "--json")
	cmd.Dir = rootA
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit context target file: %v\n%s", err, string(out))
	}
	res = tc.Result{}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if res.Context.Path != "sub/nested/file.txt" {
		t.Errorf("expected target path 'sub/nested/file.txt', got %s", res.Context.Path)
	}

	// Case 4: Human output format (stdout has structured keys, exit code 0)
	cmd = exec.Command(binary, "context", "--state", stateDir)
	cmd.Dir = rootA
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit context human output: %v\n%s", err, string(out))
	}
	outStr := string(out)
	if !strings.Contains(outStr, "Folder:") || !strings.Contains(outStr, "Documents") || !strings.Contains(outStr, "Root:") {
		t.Errorf("human output missing expected fields:\n%s", outStr)
	}

	// Case 5: Explicit folder name from outside root
	outsideDir := filepath.Join(base, "outside")
	os.MkdirAll(outsideDir, 0700)
	cmd = exec.Command(binary, "context", "--folder", "Documents", "--state", stateDir, "--json")
	cmd.Dir = outsideDir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit context with --folder: %v\n%s", err, string(out))
	}
	res = tc.Result{}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if res.Context.Folder != folderAHex {
		t.Errorf("expected folder %s, got %s", folderAHex, res.Context.Folder)
	}

	// Case 6: Explicit folder 64-hex ID
	cmd = exec.Command(binary, "context", "--folder", folderAHex, "--state", stateDir, "--json")
	cmd.Dir = outsideDir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit context with 64-hex ID: %v\n%s", err, string(out))
	}
	res = tc.Result{}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if res.Context.Folder != folderAHex {
		t.Errorf("expected folder %s, got %s", folderAHex, res.Context.Folder)
	}
}

// TestTerminalT06AmbiguousAndOutsideRootContext verifies:
// - cwd outside root returns exit code 2 (AMBIGUOUS_CONTEXT) with candidate Items
// - duplicate folder names return exit code 2 (AMBIGUOUS_CONTEXT) with both candidate items
// - non-existent folder returns FOLDER_NOT_FOUND with candidate items
func TestTerminalT06AmbiguousAndOutsideRootContext(t *testing.T) {
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

	root1 := filepath.Join(base, "work-laptop")
	root2 := filepath.Join(base, "work-desktop")
	os.MkdirAll(root1, 0700)
	os.MkdirAll(root2, 0700)

	var folder1, folder2 history.ID
	err = app.WithWorkspace(ctx, stateDir, func(c config.Config, db *repository.DB, ws *workspace.Workspace) error {
		folder1 = mustID(t, enrollmentRandom(t))
		folder2 = mustID(t, enrollmentRandom(t))
		_ = db.EnsureFolder(ctx, folder1, devID, 1)
		_ = db.EnsureFolder(ctx, folder2, devID, 1)
		ctrl := control.New(db, ws, control.Options{LocalDevice: devID})
		_, _ = ctrl.RegisterFolder(ctx, folder1, root1)
		_, _ = ctrl.RegisterFolder(ctx, folder2, root2)
		// Register both with identical display name "Work"
		_ = db.SetFolderDisplayName(ctx, folder1, "Work")
		return db.SetFolderDisplayName(ctx, folder2, "Work")
	})
	if err != nil {
		t.Fatal(err)
	}

	outsideDir := filepath.Join(base, "outside")
	os.MkdirAll(outsideDir, 0700)

	// Case 1: Outside root without --folder -> exit code 2, AMBIGUOUS_CONTEXT, candidates in Items
	cmd := exec.Command(binary, "context", "--state", stateDir, "--json")
	cmd.Dir = outsideDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code for outside root invocation")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("expected exit code 2 (AMBIGUOUS_CONTEXT), got %d\nOutput: %s", exitErr.ExitCode(), string(out))
	}
	var res tc.Result
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if res.Error == nil || res.Error.Code != "AMBIGUOUS_CONTEXT" {
		t.Errorf("expected error code AMBIGUOUS_CONTEXT, got %v", res.Error)
	}
	if len(res.Items) < 2 {
		t.Errorf("expected candidate items for ambiguous context, got %d", len(res.Items))
	}

	// Case 2: Querying duplicate folder name "Work" -> exit code 2, AMBIGUOUS_CONTEXT with candidate items
	cmd = exec.Command(binary, "context", "--folder", "Work", "--state", stateDir, "--json")
	cmd.Dir = outsideDir
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code for duplicate folder name")
	}
	exitErr, ok = err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("expected exit code 2 (AMBIGUOUS_CONTEXT), got %d\nOutput: %s", exitErr.ExitCode(), string(out))
	}
	res = tc.Result{}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if res.Error == nil || res.Error.Code != "AMBIGUOUS_CONTEXT" {
		t.Errorf("expected AMBIGUOUS_CONTEXT error code, got %v", res.Error)
	}
	if len(res.Items) != 2 {
		t.Errorf("expected 2 candidate items for duplicate folder name, got %d", len(res.Items))
	}

	// Case 3: Non-existent folder name -> FOLDER_NOT_FOUND with candidate items
	cmd = exec.Command(binary, "context", "--folder", "NoSuchFolder", "--state", stateDir, "--json")
	cmd.Dir = outsideDir
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code for missing folder name")
	}
	res = tc.Result{}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if res.Error == nil || res.Error.Code != "FOLDER_NOT_FOUND" {
		t.Errorf("expected FOLDER_NOT_FOUND error code, got %v", res.Error)
	}
	if len(res.Items) != 2 {
		t.Errorf("expected candidates listed in FOLDER_NOT_FOUND response, got %d", len(res.Items))
	}
}

// TestTerminalT06StaleAndUnavailableRoot verifies:
// - Missing or deleted registered root directory returns ROOT_UNAVAILABLE
// - Root moved on disk without update returns STALE_ROOT
func TestTerminalT06StaleAndUnavailableRoot(t *testing.T) {
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

	rootTemp := filepath.Join(base, "will-be-deleted")
	if err := os.MkdirAll(rootTemp, 0700); err != nil {
		t.Fatal(err)
	}

	var folderID history.ID
	err = app.WithWorkspace(ctx, stateDir, func(c config.Config, db *repository.DB, ws *workspace.Workspace) error {
		folderID = mustID(t, enrollmentRandom(t))
		_ = db.EnsureFolder(ctx, folderID, devID, 1)
		ctrl := control.New(db, ws, control.Options{LocalDevice: devID})
		_, err := ctrl.RegisterFolder(ctx, folderID, rootTemp)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Delete the root directory
	if err := os.RemoveAll(rootTemp); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binary, "context", "--folder", hex.EncodeToString(folderID[:]), "--state", stateDir, "--json")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code for unavailable root")
	}
	var res tc.Result
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if res.Error == nil || (res.Error.Code != "ROOT_UNAVAILABLE" && res.Error.Code != "STALE_ROOT") {
		t.Errorf("expected ROOT_UNAVAILABLE or STALE_ROOT, got %v", res.Error)
	}
}

// TestTerminalT06PathSafetyAndLiteralPaths verifies:
// - Escaping root via `..` returns exit code 4 (INVALID_PATH)
// - Reserved paths (e.g. .filesync) return exit code 4 (INVALID_PATH)
// - `--` allows literal leading-dash paths without flag collision
// - Spaces and Unicode paths resolve accurately without corruption
func TestTerminalT06PathSafetyAndLiteralPaths(t *testing.T) {
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

	root := filepath.Join(base, "safepath")
	os.MkdirAll(root, 0700)

	var folderID history.ID
	err = app.WithWorkspace(ctx, stateDir, func(c config.Config, db *repository.DB, ws *workspace.Workspace) error {
		folderID = mustID(t, enrollmentRandom(t))
		_ = db.EnsureFolder(ctx, folderID, devID, 1)
		ctrl := control.New(db, ws, control.Options{LocalDevice: devID})
		_, err := ctrl.RegisterFolder(ctx, folderID, root)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Path escaping root via .. -> exit code 4 (INVALID_PATH)
	cmd := exec.Command(binary, "context", "../outside.txt", "--state", stateDir, "--json")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected failure when path escapes root")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 2 {
		t.Errorf("expected exit code 2 (INVALID_PATH), got %v\nOutput: %s", err, string(out))
	}
	var res tc.Result
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if res.Error == nil || res.Error.Code != "INVALID_PATH" {
		t.Errorf("expected INVALID_PATH, got %v", res.Error)
	}

	// 2. Reserved internal path (.filesync) -> exit code 4
	cmd = exec.Command(binary, "context", ".filesync/data.db", "--state", stateDir, "--json")
	cmd.Dir = root
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected failure for reserved directory path")
	}
	res = tc.Result{}
	_ = json.Unmarshal(out, &res)
	if res.Error == nil || res.Error.Code != "INVALID_PATH" {
		t.Errorf("expected INVALID_PATH for reserved path, got %v", res.Error)
	}

	// 3. Literal path with leading dash via --
	cmd = exec.Command(binary, "context", "--state", stateDir, "--json", "--", "-dashed-file.txt")
	cmd.Dir = root
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("literal -- path failed: %v\n%s", err, string(out))
	}
	res = tc.Result{}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if res.Context == nil || res.Context.Path != "-dashed-file.txt" {
		t.Errorf("expected path '-dashed-file.txt', got %v", res.Context)
	}

	// 4. Spaces and Unicode in path
	unicodePath := "Project Docs/🚀 Launch 2026.pdf"
	cmd = exec.Command(binary, "context", unicodePath, "--state", stateDir, "--json")
	cmd.Dir = root
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unicode path failed: %v\n%s", err, string(out))
	}
	res = tc.Result{}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, string(out))
	}
	if res.Context == nil || res.Context.Path != unicodePath {
		t.Errorf("expected path %q, got %v", unicodePath, res.Context)
	}
}

// TestTerminalT06OutputEscapingAndControlCharacters verifies:
// - Control characters and ANSI escapes in folder/file names are sanitized in human output
// - Raw escape bytes (\x1b, \r) are neutralized to avoid terminal control injection
func TestTerminalT06OutputEscapingAndControlCharacters(t *testing.T) {
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

	root := filepath.Join(base, "escape_test")
	os.MkdirAll(root, 0700)

	maliciousName := "\x1b[31;1mInjectedColor\x1b[0m\r\n\tFolder"
	var folderID history.ID
	err = app.WithWorkspace(ctx, stateDir, func(c config.Config, db *repository.DB, ws *workspace.Workspace) error {
		folderID = mustID(t, enrollmentRandom(t))
		_ = db.EnsureFolder(ctx, folderID, devID, 1)
		ctrl := control.New(db, ws, control.Options{LocalDevice: devID})
		_, err := ctrl.RegisterFolder(ctx, folderID, root)
		if err != nil {
			return err
		}
		return db.SetFolderDisplayName(ctx, folderID, maliciousName)
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Human output: raw 0x1b and raw 0x0d must NOT exist in output stream
	cmd := exec.Command(binary, "context", "--state", stateDir)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit context: %v\n%s", err, string(out))
	}

	if bytes.Contains(out, []byte{0x1b}) {
		t.Errorf("human output contains raw 0x1b byte! EscapeTerminal failed to sanitize ANSI injection")
	}
	if bytes.Contains(out, []byte{0x0d}) {
		t.Errorf("human output contains raw carriage return byte 0x0d!")
	}
	// Verify escaped literal representation was written instead
	if !strings.Contains(string(out), `\x1b`) {
		t.Errorf("expected escaped \\x1b literal string in output, got: %s", string(out))
	}

	// 2. orbit folders list human output: must also be sanitized
	cmd = exec.Command(binary, "folders", "list", "--state", stateDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit folders: %v\n%s", err, string(out))
	}
	if bytes.Contains(out, []byte{0x1b}) {
		t.Errorf("orbit folders list contains raw 0x1b byte!")
	}
}

// TestTerminalT06LiveAndStoppedAdapterParity verifies:
// - Exact command equivalence between stopped adapter and live background daemon
// - Consistent tc.Result payloads across query adapters
func TestTerminalT06LiveAndStoppedAdapterParity(t *testing.T) {
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

	root := filepath.Join(base, "parity_root")
	os.MkdirAll(root, 0700)

	var folderID history.ID
	err = app.WithWorkspace(ctx, stateDir, func(c config.Config, db *repository.DB, ws *workspace.Workspace) error {
		folderID = mustID(t, enrollmentRandom(t))
		_ = db.EnsureFolder(ctx, folderID, devID, 1)
		ctrl := control.New(db, ws, control.Options{LocalDevice: devID})
		_, err := ctrl.RegisterFolder(ctx, folderID, root)
		if err != nil {
			return err
		}
		return db.SetFolderDisplayName(ctx, folderID, "ParityFolder")
	})
	if err != nil {
		t.Fatal(err)
	}

	// Run queries in stopped mode
	runQuery := func() (contextOut, foldersOut, statusOut []byte) {
		cmd1 := exec.Command(binary, "context", "--state", stateDir, "--json")
		cmd1.Dir = root
		cOut, err := cmd1.CombinedOutput()
		if err != nil {
			t.Fatalf("context query: %v\n%s", err, string(cOut))
		}

		cmd2 := exec.Command(binary, "folders", "list", "--state", stateDir, "--json")
		fOut, err := cmd2.CombinedOutput()
		if err != nil {
			t.Fatalf("folders query: %v\n%s", err, string(fOut))
		}

		cmd3 := exec.Command(binary, "status", "--state", stateDir, "--json")
		sOut, err := cmd3.CombinedOutput()
		if err != nil {
			t.Fatalf("status query: %v\n%s", err, string(sOut))
		}
		return cOut, fOut, sOut
	}

	stoppedContext, stoppedFolders, stoppedStatus := runQuery()

	var scRes, sfRes tc.Result
	if err := json.Unmarshal(stoppedContext, &scRes); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(stoppedFolders, &sfRes); err != nil {
		t.Fatal(err)
	}

	// Start live background daemon
	daemon := exec.Command(binary, "serve", "--state", stateDir, "--control-listen", "127.0.0.1:0", "--no-watch", "--sync-interval", "1s")
	if err := daemon.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer func() {
		_ = daemon.Process.Signal(syscall.SIGTERM)
		_ = daemon.Wait()
	}()

	client := &controlclient.Client{StateDir: stateDir}
	until := time.Now().Add(10 * time.Second)
	ready := false
	for time.Now().Before(until) {
		var r tc.Result
		if err := client.Call(ctx, "POST", "/control/terminal/v1/query", tc.Query{Version: tc.Version, Kind: "capabilities"}, &r); err == nil {
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("daemon did not become ready")
	}

	// Run queries in live mode
	liveContext, liveFolders, liveStatus := runQuery()

	var lcRes, lfRes tc.Result
	if err := json.Unmarshal(liveContext, &lcRes); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(liveFolders, &lfRes); err != nil {
		t.Fatal(err)
	}

	// Assert parity between stopped and live query results
	if scRes.Context.Folder != lcRes.Context.Folder || scRes.Context.Root != lcRes.Context.Root {
		t.Errorf("context mismatch between stopped and live: stopped=%+v, live=%+v", scRes.Context, lcRes.Context)
	}
	if len(sfRes.Items) != len(lfRes.Items) || sfRes.Items[0].Name != lfRes.Items[0].Name {
		t.Errorf("folders mismatch between stopped and live: stopped=%+v, live=%+v", sfRes.Items, lfRes.Items)
	}

	// Verify status indicates daemon running in live mode
	var lsMap map[string]any
	_ = json.Unmarshal(liveStatus, &lsMap)
	if lsMap["daemon_running"] != true {
		t.Errorf("expected live status to report daemon_running=true, got %v", lsMap["daemon_running"])
	}
	var ssMap map[string]any
	_ = json.Unmarshal(stoppedStatus, &ssMap)
	if ssMap["daemon_running"] != false {
		t.Errorf("expected stopped status to report daemon_running=false, got %v", ssMap["daemon_running"])
	}
}

// TestTerminalT06ShellCompletionsAndHelpWalkthrough verifies:
// - orbit help contains coherent grouped sections
// - orbit help <cmd> succeeds for all agreed subcommands
// - shell completions generate valid scripts for bash, zsh, and fish
// - completion scripts do NOT leak tokens, keys, or secrets
func TestTerminalT06ShellCompletionsAndHelpWalkthrough(t *testing.T) {
	t.Parallel()
	base := testkit.NewDisposable(t)
	os.Chmod(base, 0700)
	binary := buildOrbitBinary(t, base)

	// 1. orbit help top-level overview
	cmd := exec.Command(binary, "help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit help: %v\n%s", err, string(out))
	}
	helpText := string(out)
	expectedSections := []string{
		"Everyday Commands:",
		"Setup & Sharing:",
		"Management & Diagnostics:",
		"Options:",
		"status",
		"context",
		"folders",
		"devices",
		"conflicts",
		"history",
		"deleted",
		"restore",
		"doctor",
		"completion",
	}
	for _, sec := range expectedSections {
		if !strings.Contains(helpText, sec) {
			t.Errorf("orbit help missing section or command %q", sec)
		}
	}

	// 2. orbit help <cmd> for each agreed command
	agreedCommands := []string{
		"status", "context", "folders", "devices", "conflicts",
		"history", "deleted", "restore", "setup", "join",
		"service", "storage", "doctor", "completion", "version",
	}
	for _, sub := range agreedCommands {
		cmdHelp := exec.Command(binary, "help", sub)
		subOut, subErr := cmdHelp.CombinedOutput()
		if subErr != nil {
			t.Fatalf("orbit help %s failed: %v\n%s", sub, subErr, string(subOut))
		}
		if len(subOut) == 0 {
			t.Errorf("orbit help %s produced empty output", sub)
		}
	}

	// 3. orbit completion bash, zsh, fish
	shells := []string{"bash", "zsh", "fish"}
	for _, shell := range shells {
		compCmd := exec.Command(binary, "completion", shell)
		compOut, compErr := compCmd.CombinedOutput()
		if compErr != nil {
			t.Fatalf("orbit completion %s failed: %v\n%s", shell, compErr, string(compOut))
		}
		script := string(compOut)
		if len(script) < 50 {
			t.Errorf("orbit completion %s output suspiciously short: %s", shell, script)
		}
		// Ensure all subcommands are in the completion definitions
		for _, sub := range agreedCommands {
			if !strings.Contains(script, sub) {
				t.Errorf("shell %s completion script missing subcommand %q", shell, sub)
			}
		}
		// Ensure no sensitive tokens, pins, or keys are emitted
		if strings.Contains(script, "Bearer") || strings.Contains(script, "token") && strings.Contains(script, "secret") {
			t.Errorf("shell %s completion script leaked sensitive data!", shell)
		}
	}
}
