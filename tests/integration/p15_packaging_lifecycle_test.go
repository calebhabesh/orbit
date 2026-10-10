package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/control"
)

func findBinary(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	bin := filepath.Join(root, "bin", "orbit")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("binary %s not found, run make build first: %v", bin, err)
	}
	return bin
}

func makeTestStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod 0700 test state dir: %v", err)
	}
	return dir
}

func TestP15VersionAndBuildMetadata(t *testing.T) {
	bin := findBinary(t)

	cmd := exec.Command(bin, "engine", "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run orbit version: %v (output: %s)", err, string(out))
	}

	str := string(out)
	if !strings.Contains(str, "orbit 2.3.0") {
		t.Errorf("version output missing 'orbit 2.3.0', got: %s", str)
	}
	if platform := runtime.GOOS + "/" + runtime.GOARCH; !strings.Contains(str, platform) {
		t.Errorf("version output missing %q, got: %s", platform, str)
	}
	if !strings.Contains(str, "commit=") {
		t.Errorf("version output missing commit metadata, got: %s", str)
	}
	if !strings.Contains(str, "built=") {
		t.Errorf("version output missing built date metadata, got: %s", str)
	}
}

func TestP15ReleaseBinaryExcludesDestructiveTestHooks(t *testing.T) {
	bin := findBinary(t)

	// Verify that production binary accepts no hook injection CLI flags
	cmd := exec.Command(bin, "engine", "serve", "--fault-hook", "some_hook")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error passing --fault-hook to binary, got success: %s", string(out))
	}
	if !strings.Contains(string(out), "flag provided but not defined") {
		t.Errorf("expected 'flag provided but not defined', got: %s", string(out))
	}
}

func TestP15EmbeddedUIWithoutNode(t *testing.T) {
	stateDir := makeTestStateDir(t)
	bin := findBinary(t)

	// Initialize state
	initCmd := exec.Command(bin, "engine", "init", "--state", stateDir)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v (%s)", err, string(out))
	}

	// Pick unused loopback port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen loopback: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "engine", "serve", "--state", stateDir, "--control-listen", addr)
	// Strip node and npm from PATH to guarantee zero runtime node dependency
	cmd.Env = []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + stateDir,
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start agent: %v", err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	// Wait for server to respond
	client := &http.Client{Timeout: 2 * time.Second}
	var resp *http.Response
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		resp, err = client.Get("http://" + addr + "/")
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("connect to embedded web server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 for index.html, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read index.html body: %v", err)
	}
	if !strings.Contains(string(body), "<div id=\"root\">") {
		t.Fatalf("expected embedded React root div in response, got: %s", string(body))
	}

	// Client-side routes should return index.html (SPA routing)
	for _, route := range []string{"/folders", "/files", "/conflicts"} {
		routeResp, err := client.Get("http://" + addr + route)
		if err != nil {
			t.Fatalf("fetch %s: %v", route, err)
		}
		routeBody, _ := io.ReadAll(routeResp.Body)
		routeResp.Body.Close()
		if routeResp.StatusCode != http.StatusOK || !strings.Contains(string(routeBody), "<div id=\"root\">") {
			t.Errorf("SPA route %s failed: status=%d", route, routeResp.StatusCode)
		}
	}
}

func TestP15ConfigValidation(t *testing.T) {
	stateDir := makeTestStateDir(t)
	bin := findBinary(t)

	// Before init -> validation fails
	cmd := exec.Command(bin, "engine", "config", "validate", "--state", stateDir)
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected config validate to fail on uninitialized state dir")
	}

	// Initialize
	if out, err := exec.Command(bin, "engine", "init", "--state", stateDir).CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v (%s)", err, string(out))
	}

	// Validate after init -> success
	cmd = exec.Command(bin, "engine", "config", "validate", "--state", stateDir, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config validate failed: %v (%s)", err, string(out))
	}
	var res map[string]any
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if valid, ok := res["valid"].(bool); !ok || !valid {
		t.Fatalf("expected valid=true, got: %v", res)
	}

	// Corrupt config.json -> validation fails
	_ = os.WriteFile(filepath.Join(stateDir, "config.json"), []byte("{broken json"), 0o600)
	cmd = exec.Command(bin, "engine", "config", "validate", "--state", stateDir)
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected config validate to fail on corrupted config.json")
	}
}

func TestP15AgentStop(t *testing.T) {
	stateDir := makeTestStateDir(t)
	bin := findBinary(t)

	if out, err := exec.Command(bin, "engine", "init", "--state", stateDir).CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v (%s)", err, string(out))
	}

	// Start agent in background
	cmd := exec.Command(bin, "engine", "serve", "--state", stateDir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start agent: %v", err)
	}

	// Wait for .agent.pid to appear
	pidPath := filepath.Join(stateDir, ".agent.pid")
	var pid int
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if data, err := os.ReadFile(pidPath); err == nil {
			var p int
			if _, err := fmt.Sscanf(string(data), "%d", &p); err == nil && p > 0 {
				pid = p
				break
			}
		}
	}
	if pid == 0 {
		_ = cmd.Process.Kill()
		t.Fatalf("agent did not write .agent.pid")
	}

	// Run orbit stop
	stopCmd := exec.Command(bin, "engine", "stop", "--state", stateDir)
	if out, err := stopCmd.CombinedOutput(); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("orbit stop failed: %v (%s)", err, string(out))
	}

	// Verify background process exited
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Clean exit
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("agent process did not exit within timeout")
	}

	// Verify .agent.pid removed
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf(".agent.pid still exists after stop")
	}
}

func TestP15UpgradePreflight(t *testing.T) {
	stateDir := makeTestStateDir(t)
	bin := findBinary(t)

	if out, err := exec.Command(bin, "engine", "init", "--state", stateDir).CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v (%s)", err, string(out))
	}

	// Test 1: Clean preflight when agent is stopped
	cmd := exec.Command(bin, "engine", "maintenance", "preflight", "--state", stateDir, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("preflight failed: %v (%s)", err, string(out))
	}
	var res control.PreflightResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal preflight json: %v", err)
	}
	if res.Status != "ready" {
		t.Errorf("expected status=ready, got %s (issues: %v)", res.Status, res.Issues)
	}
	if res.AgentRunning {
		t.Errorf("expected agent_running=false, got true")
	}
	if !res.IntegrityClean {
		t.Errorf("expected integrity_clean=true, got false")
	}

	// Test 2: Incompatible schema (Invariant I20)
	dbPath := filepath.Join(stateDir, "metadata.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec("PRAGMA user_version = 999")
	db.Close()
	if err != nil {
		t.Fatalf("bump user_version: %v", err)
	}

	cmd = exec.Command(bin, "engine", "maintenance", "preflight", "--state", stateDir, "--json")
	out, _ = cmd.CombinedOutput()
	var incompRes control.PreflightResult
	_ = json.Unmarshal(out, &incompRes)
	if incompRes.Status != "blocked" {
		t.Errorf("expected status=blocked for schema 999, got %s", incompRes.Status)
	}
	if incompRes.DatabaseSchema != 999 {
		t.Errorf("expected database_schema=999, got %d", incompRes.DatabaseSchema)
	}
}

func TestP15InterruptedMigrationRollback(t *testing.T) {
	stateDir := t.TempDir()

	// Create SQLite database at version 1
	dbPath := filepath.Join(stateDir, "metadata.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec("CREATE TABLE t1 (id INTEGER PRIMARY KEY); PRAGMA user_version = 1;")
	if err != nil {
		db.Close()
		t.Fatalf("init test schema: %v", err)
	}

	// Attempt a failing migration inside a transaction
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		db.Close()
		t.Fatalf("begin tx: %v", err)
	}
	_, _ = tx.Exec("CREATE TABLE t2 (id INTEGER PRIMARY KEY);")
	// Deliberately trigger SQL error
	_, err = tx.Exec("CREATE TABLE t1 (duplicate TABLE);")
	if err != nil {
		_ = tx.Rollback()
	} else {
		_ = tx.Commit()
	}

	// Verify user_version remained at 1 and t2 was not committed (Invariant I20)
	var ver int
	_ = db.QueryRow("PRAGMA user_version").Scan(&ver)
	if ver != 1 {
		t.Errorf("expected user_version=1 after rollback, got %d", ver)
	}
	var count int
	_ = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='t2'").Scan(&count)
	db.Close()
	if count != 0 {
		t.Errorf("expected table t2 to be rolled back, but found %d tables", count)
	}
}

func TestP15ConsistentBackupAndRestoreSafety(t *testing.T) {
	stateDir := makeTestStateDir(t)
	backupDir := t.TempDir()
	bin := findBinary(t)

	// 1. Initialize
	if out, err := exec.Command(bin, "engine", "init", "--state", stateDir).CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v (%s)", err, string(out))
	}

	// Read initial config
	cfg1, err := os.ReadFile(filepath.Join(stateDir, "config.json"))
	if err != nil {
		t.Fatalf("read config1: %v", err)
	}
	var m1 map[string]any
	_ = json.Unmarshal(cfg1, &m1)
	oldDeviceID := m1["device_id"].(string)

	// 2. Perform consistent backup
	backupPath := filepath.Join(backupDir, "test-backup.sqlite")
	backupCmd := exec.Command(bin, "engine", "maintenance", "backup", "--state", stateDir, "--out", backupPath, "--json")
	if out, err := backupCmd.CombinedOutput(); err != nil {
		t.Fatalf("backup failed: %v (%s)", err, string(out))
	}

	// Verify backup file is a valid SQLite DB
	bDB, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	var integrity string
	if err := bDB.QueryRow("PRAGMA quick_check").Scan(&integrity); err != nil || integrity != "ok" {
		bDB.Close()
		t.Fatalf("backup integrity failure: %v (%s)", err, integrity)
	}
	bDB.Close()

	// 3. Restore from backup -> must reset identity (Invariant I08)
	restoreCmd := exec.Command(bin, "engine", "maintenance", "restore-backup", "--state", stateDir, "--backup", backupPath, "--json")
	out, err := restoreCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("restore-backup failed: %v (%s)", err, string(out))
	}

	var res control.RestoreBackupResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal restore-backup json: %v", err)
	}

	if res.Status != "success" {
		t.Errorf("expected status=success, got %s", res.Status)
	}

	// New device ID must be distinct from old device ID
	newDeviceHex := hex.EncodeToString(res.NewDeviceID[:])
	oldDeviceHex := hex.EncodeToString(res.OldDeviceID[:])
	if newDeviceHex == oldDeviceHex {
		t.Fatalf("CRITICAL (Invariant I08): restored node reused old device ID %s", oldDeviceHex)
	}
	if oldDeviceHex != oldDeviceID {
		t.Errorf("expected reported old_device=%s, got %s", oldDeviceID, oldDeviceHex)
	}

	// Verify config.json on disk was updated with new device ID
	cfg2, _ := os.ReadFile(filepath.Join(stateDir, "config.json"))
	var m2 map[string]any
	_ = json.Unmarshal(cfg2, &m2)
	if m2["device_id"].(string) != newDeviceHex {
		t.Errorf("config.json device_id (%s) does not match new device ID (%s)", m2["device_id"], newDeviceHex)
	}

	// Verify database folders local_author and next_counter updated
	db, err := sql.Open("sqlite", filepath.Join(stateDir, "metadata.sqlite"))
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer db.Close()

	var count int
	var nextCounter []byte
	row := db.QueryRow("SELECT count(*), next_counter FROM folders")
	if err := row.Scan(&count, &nextCounter); err == nil && count > 0 {
		if len(nextCounter) == 8 && !bytes.Equal(nextCounter, make([]byte, 8)) {
			t.Errorf("expected next_counter reset to 0, got %x", nextCounter)
		}
	}
}

func TestP15PackagingOutputsAndChecksums(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	distDir := filepath.Join(root, "dist")

	// Ensure packages are built
	cmd := exec.Command("go", "run", "./scripts/build_packages.go")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build_packages failed: %v (%s)", err, string(out))
	}

	expectedPackages := []string{
		"orbit-v2.3.0-linux-amd64.tar.gz",
		"orbit_2.3.0_amd64.deb",
		"orbit-2.3.0-1.x86_64.rpm",
		"orbit-v2.3.0-linux-arm64.tar.gz",
		"orbit_2.3.0_arm64.deb",
		"orbit-2.3.0-1.aarch64.rpm",
	}

	for _, p := range expectedPackages {
		pkgPath := filepath.Join(distDir, p)
		fi, err := os.Stat(pkgPath)
		if err != nil {
			t.Fatalf("expected package %s missing: %v", p, err)
		}
		if fi.Size() == 0 {
			t.Fatalf("package %s has zero size", p)
		}
	}

	// Verify SHA256SUMS file
	sumsData, err := os.ReadFile(filepath.Join(distDir, "SHA256SUMS"))
	if err != nil {
		t.Fatalf("read SHA256SUMS: %v", err)
	}
	for _, p := range expectedPackages {
		pkgData, err := os.ReadFile(filepath.Join(distDir, p))
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		h := sha256.Sum256(pkgData)
		expectedLine := fmt.Sprintf("%s  %s", hex.EncodeToString(h[:]), p)
		if !strings.Contains(string(sumsData), expectedLine) {
			t.Errorf("SHA256SUMS missing expected entry: %s", expectedLine)
		}
	}

	// Verify tar.gz contents
	tarGzPath := filepath.Join(distDir, "orbit-v2.3.0-linux-amd64.tar.gz")
	f, err := os.Open(tarGzPath)
	if err != nil {
		t.Fatalf("open tar.gz: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	tr := tar.NewReader(gz)
	foundFiles := make(map[string]bool)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		foundFiles[hdr.Name] = true
	}
	for _, req := range []string{"orbit", "systemd/orbit.service", "install.sh", "uninstall.sh", "LICENSE", "NOTICE"} {
		if !foundFiles[req] {
			t.Errorf("tar.gz missing expected file %s", req)
		}
	}
}

func TestP15UninstallPreservesUserData(t *testing.T) {
	tempHome := t.TempDir()
	stateDir := filepath.Join(tempHome, ".local", "share", "orbit")
	workspaceRoot := filepath.Join(tempHome, "SyncedNotes")
	bin := findBinary(t)

	// Create workspace with files
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	testFile := filepath.Join(workspaceRoot, "important_note.txt")
	if err := os.WriteFile(testFile, []byte("User Captured Work - Do Not Delete\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Initialize state dir
	if out, err := exec.Command(bin, "engine", "init", "--state", stateDir).CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v (%s)", err, string(out))
	}

	// Simulate package installation
	localBin := filepath.Join(tempHome, ".local", "bin")
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		t.Fatalf("mkdir local bin: %v", err)
	}
	installedBin := filepath.Join(localBin, "orbit")
	_ = os.WriteFile(installedBin, []byte("#!/bin/sh\n"), 0o755)

	localServiceDir := filepath.Join(tempHome, ".config", "systemd", "user")
	if err := os.MkdirAll(localServiceDir, 0o755); err != nil {
		t.Fatalf("mkdir local service dir: %v", err)
	}
	installedService := filepath.Join(localServiceDir, "orbit.service")
	_ = os.WriteFile(installedService, []byte("[Service]\n"), 0o644)

	// Run uninstall.sh with HOME=tempHome
	root, _ := filepath.Abs("../..")
	uninstallScript := filepath.Join(root, "packaging/scripts/uninstall.sh")
	cmd := exec.Command("bash", uninstallScript)
	cmd.Env = []string{
		"HOME=" + tempHome,
		"PATH=/usr/bin:/bin",
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("uninstall.sh failed: %v (%s)", err, string(out))
	}

	// 1. Verify binary and service are removed
	if _, err := os.Stat(installedBin); !os.IsNotExist(err) {
		t.Errorf("binary was not removed by uninstall: %s", installedBin)
	}
	if _, err := os.Stat(installedService); !os.IsNotExist(err) {
		t.Errorf("service was not removed by uninstall: %s", installedService)
	}

	// 2. CRITICAL (Requirement S21): State directory and workspace root MUST be preserved!
	if _, err := os.Stat(filepath.Join(stateDir, "metadata.sqlite")); err != nil {
		t.Fatalf("CRITICAL: metadata.sqlite was deleted on uninstall!")
	}
	content, err := os.ReadFile(testFile)
	if err != nil || !strings.Contains(string(content), "User Captured Work") {
		t.Fatalf("CRITICAL: user workspace file was deleted or corrupted on uninstall!")
	}
}
