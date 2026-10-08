package integration

import (
	"archive/tar"
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
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
	_ "modernc.org/sqlite"
)

func findOrbitBinary(t *testing.T) string {
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

// TestOrbitPackagingArtifacts verifies that release packages for amd64 and arm64
// are built, contain valid checksums in SHA256SUMS, and include release-manifest.json.
func TestOrbitPackagingArtifacts(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	distDir := filepath.Join(root, "dist")

	// Run build_packages.go
	cmd := exec.Command("go", "run", "./scripts/build_packages.go")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build_packages failed: %v (%s)", err, string(out))
	}

	expectedPackages := []string{
		"orbit-v2.1.0-linux-amd64.tar.gz",
		"orbit_2.1.0_amd64.deb",
		"orbit-2.1.0-1.x86_64.rpm",
		"orbit-v2.1.0-linux-arm64.tar.gz",
		"orbit_2.1.0_arm64.deb",
		"orbit-2.1.0-1.aarch64.rpm",
		"release-manifest.json",
	}

	for _, p := range expectedPackages {
		pkgPath := filepath.Join(distDir, p)
		fi, err := os.Stat(pkgPath)
		if err != nil {
			t.Fatalf("expected artifact %s missing: %v", p, err)
		}
		if fi.Size() == 0 {
			t.Fatalf("artifact %s has zero size", p)
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
}

// TestOrbitTarballContentsAndSymlinks inspects the extracted tarball to verify
// that the orbit binary, desktop launcher, SVG icon, and manifest are present.
func TestOrbitTarballContentsAndSymlinks(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	tarGzPath := filepath.Join(root, "dist", "orbit-v2.1.0-linux-amd64.tar.gz")

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

	foundEntries := make(map[string]tar.Header)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		foundEntries[hdr.Name] = *hdr
	}

	requiredFiles := []string{
		"orbit",
		"systemd/orbit.service",
		"desktop/orbit.desktop",
		"icons/orbit.svg",
		"install.sh",
		"uninstall.sh",
		"LICENSE",
		"NOTICE",
		"LICENSES.md",
		"README.md",
		"release-manifest.json",
	}

	for _, req := range requiredFiles {
		hdr, ok := foundEntries[req]
		if !ok {
			t.Errorf("tar.gz missing expected file %s", req)
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			t.Errorf("%s must be a regular file, got type=%v", req, hdr.Typeflag)
		}
	}
}

// TestOrbitVersionAndManifestMetadata tests CLI version commands and API endpoint.
func TestOrbitVersionAndManifestMetadata(t *testing.T) {
	bin := findOrbitBinary(t)

	// 1. Text version
	cmd := exec.Command(bin, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit version: %v (%s)", err, string(out))
	}
	outStr := string(out)
	if !strings.Contains(outStr, "Orbit Personal File Manager v2.1.0") {
		t.Errorf("missing Orbit brand string, got: %s", outStr)
	}
	if !strings.Contains(outStr, "Schema: SQLite user_version 13") {
		t.Errorf("missing Schema 13, got: %s", outStr)
	}
	if !strings.Contains(outStr, "pure-Go SQLite, zero Node runtime") {
		t.Errorf("missing pure-Go runtime notice, got: %s", outStr)
	}

	// 2. JSON version
	cmdJSON := exec.Command(bin, "version", "--json")
	outJSON, err := cmdJSON.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit version --json: %v (%s)", err, string(outJSON))
	}
	var verInfo map[string]any
	if err := json.Unmarshal(outJSON, &verInfo); err != nil {
		t.Fatalf("parse orbit version json: %v", err)
	}
	if verInfo["product"] != "Orbit" {
		t.Errorf("expected product Orbit, got: %v", verInfo["product"])
	}
	if verInfo["schema_version"] != float64(13) {
		t.Errorf("expected schema_version 13, got: %v", verInfo["schema_version"])
	}
	assets, ok := verInfo["embedded_assets"].(map[string]any)
	if !ok || assets["total_files"].(float64) < 1 {
		t.Errorf("expected valid embedded_assets in json, got: %v", verInfo["embedded_assets"])
	}

	// 3. API endpoint GET /api/v1/version
	stateDir := testkit.NewDisposable(t)
	if err := os.Chmod(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := repository.Open(context.Background(), stateDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws)
	srv, err := control.NewServer(ctrl, stateDir)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	httpSrv := &http.Server{Handler: srv.Handler()}
	go httpSrv.Serve(l)
	defer httpSrv.Close()

	resp, err := http.Get(fmt.Sprintf("http://%s/api/v1/version", l.Addr().String()))
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("version status: %d", resp.StatusCode)
	}
	var apiVer map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&apiVer); err != nil {
		t.Fatalf("decode api version: %v", err)
	}
	if apiVer["product"] != "Orbit" {
		t.Errorf("expected api product Orbit, got: %v", apiVer["product"])
	}
	if apiVer["schema_version"] != float64(13) {
		t.Errorf("expected schema_version 13, got: %v", apiVer["schema_version"])
	}
}

// TestOrbitInstallAndUninstallScriptLifecycle tests standalone installer and uninstaller
// ensuring that binaries, services, desktop entry, and icon are managed, and user
// data is strictly preserved upon uninstall (Invariant S21 / I20).
func TestOrbitInstallAndUninstallScriptLifecycle(t *testing.T) {
	tempHome := t.TempDir()
	workspaceRoot := filepath.Join(tempHome, "PersonalNotes")
	stateDir := filepath.Join(tempHome, ".local", "state", "orbit")

	// 1. Create user files and state
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	noteFile := filepath.Join(workspaceRoot, "my_life_work.txt")
	if err := os.WriteFile(noteFile, []byte("User Critical Files - Never Delete\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, _ := filepath.Abs("../..")
	binPath := filepath.Join(root, "bin", "orbit")
	if out, err := exec.Command(binPath, "init", "--state", stateDir).CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v (%s)", err, string(out))
	}

	// 2. Run install.sh with HOME=tempHome
	installScript := filepath.Join(root, "packaging/scripts/install.sh")
	installCmd := exec.Command("bash", installScript)
	installCmd.Dir = root
	installCmd.Env = []string{
		"HOME=" + tempHome,
		"PATH=/usr/bin:/bin",
		"USER=testuser",
	}
	if out, err := installCmd.CombinedOutput(); err != nil {
		t.Fatalf("install.sh failed: %v (%s)", err, string(out))
	}

	// Verify installed files in user mode
	expectedInstalled := []string{
		filepath.Join(tempHome, ".local/bin/orbit"),
		filepath.Join(tempHome, ".config/systemd/user/orbit.service"),
		filepath.Join(tempHome, ".local/share/applications/orbit.desktop"),
		filepath.Join(tempHome, ".local/share/icons/hicolor/scalable/apps/orbit.svg"),
	}

	for _, p := range expectedInstalled {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("expected installed file %s not found: %v", p, err)
		}
	}

	// 3. Run uninstall.sh with HOME=tempHome
	uninstallScript := filepath.Join(root, "packaging/scripts/uninstall.sh")
	uninstallCmd := exec.Command("bash", uninstallScript)
	uninstallCmd.Dir = root
	uninstallCmd.Env = []string{
		"HOME=" + tempHome,
		"PATH=/usr/bin:/bin",
		"USER=testuser",
	}
	if out, err := uninstallCmd.CombinedOutput(); err != nil {
		t.Fatalf("uninstall.sh failed: %v (%s)", err, string(out))
	}

	// Verify executables and systemd units removed
	for _, p := range expectedInstalled {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("file %s should have been removed on uninstall", p)
		}
	}

	// 4. CRITICAL (Invariant S21 / I20): State directory and user workspace must remain intact!
	if _, err := os.Stat(filepath.Join(stateDir, "metadata.sqlite")); err != nil {
		t.Fatalf("CRITICAL: metadata.sqlite deleted on uninstall!")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "config.json")); err != nil {
		t.Fatalf("CRITICAL: config.json deleted on uninstall!")
	}
	content, err := os.ReadFile(noteFile)
	if err != nil || !strings.Contains(string(content), "User Critical Files") {
		t.Fatalf("CRITICAL: user workspace file corrupted or deleted on uninstall!")
	}
}

// TestOrbitLegacyStateAdoption verifies that an existing SQLite database at schema version 5
// is cleanly adopted and migrated to schema 13 without data loss.
func TestOrbitLegacyStateAdoption(t *testing.T) {
	stateDir := testkit.NewDisposable(t)
	dbPath := filepath.Join(stateDir, "metadata.sqlite")

	// Create legacy database at schema version 5
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	initLegacyV5Schema(t, rawDB)
	rawDB.Close()

	// Create valid config.json
	cfg := config.Config{
		FormatVersion: 1,
		DeviceID:      "1111111111111111111111111111111111111111111111111111111111111111",
		CreatedAt:     time.Now().UTC(),
	}
	if err := config.Save(stateDir, cfg); err != nil {
		t.Fatal(err)
	}

	// Open with current Orbit repository package (migrates 5 -> 13)
	db, err := repository.Open(context.Background(), stateDir)
	if err != nil {
		t.Fatalf("legacy state adoption failed: %v", err)
	}
	defer db.Close()

	// Verify schema is now CurrentSchema (13)
	var finalVersion int
	if err := db.QueryRowRaw(context.Background(), "PRAGMA user_version").Scan(&finalVersion); err != nil {
		t.Fatal(err)
	}
	if finalVersion != repository.CurrentSchema {
		t.Fatalf("expected schema migrated to %d, got %d", repository.CurrentSchema, finalVersion)
	}

	// Verify existing folders record preserved
	folders, err := db.Folders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || hex.EncodeToString(folders[0].Folder[:]) != "f000000000000000000000000000000000000000000000000000000000000001" {
		t.Fatalf("legacy folder records not preserved: %v", folders)
	}
}

// TestOrbitSchemaRollbackRefusal verifies Invariant I20: if database user_version
// is newer than the binary's CurrentSchema, the system refuses to run.
func TestOrbitSchemaRollbackRefusal(t *testing.T) {
	stateDir := testkit.NewDisposable(t)
	dbPath := filepath.Join(stateDir, "metadata.sqlite")

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.Exec(fmt.Sprintf("PRAGMA user_version = %d;", repository.CurrentSchema+1)); err != nil {
		t.Fatal(err)
	}
	rawDB.Close()

	// Attempt to open repository
	_, err = repository.Open(context.Background(), stateDir)
	if err == nil {
		t.Fatal("expected error opening newer schema version, got success")
	}
	if !strings.Contains(err.Error(), "metadata schema is newer than this binary") {
		t.Fatalf("expected incompatible schema error, got: %v", err)
	}
}

// TestOrbitStoppedMetadataRestoreFreshIdentity verifies Invariant I08 & G04:
// restoring an older backup requires stopped exclusive lock and resets causal author counter.
func TestOrbitStoppedMetadataRestoreFreshIdentity(t *testing.T) {
	stateDir := testkit.NewDisposable(t)
	backupDir := t.TempDir()
	bin := findOrbitBinary(t)

	// 1. Initialize state
	initCmd := exec.Command(bin, "init", "--state", stateDir)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v (%s)", err, string(out))
	}

	cfg1, err := os.ReadFile(filepath.Join(stateDir, "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var m1 map[string]any
	_ = json.Unmarshal(cfg1, &m1)
	oldDeviceID := m1["device_id"].(string)

	// 2. Perform consistent backup using orbit maintenance backup
	backupPath := filepath.Join(backupDir, "test-backup.sqlite")
	backupCmd := exec.Command(bin, "maintenance", "backup", "--state", stateDir, "--out", backupPath, "--json")
	if out, err := backupCmd.CombinedOutput(); err != nil {
		t.Fatalf("backup failed: %v (%s)", err, string(out))
	}

	// Verify backup integrity
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

	// 3. Restore backup using orbit maintenance restore-backup -> must reset identity (Invariant I08)
	restoreCmd := exec.Command(bin, "maintenance", "restore-backup", "--state", stateDir, "--backup", backupPath, "--json")
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

	newDeviceHex := hex.EncodeToString(res.NewDeviceID[:])
	oldDeviceHex := hex.EncodeToString(res.OldDeviceID[:])
	if newDeviceHex == oldDeviceHex {
		t.Fatalf("CRITICAL (Invariant I08): restored node reused old device ID %s", oldDeviceHex)
	}
	if oldDeviceHex != oldDeviceID {
		t.Errorf("expected reported old_device=%s, got %s", oldDeviceID, oldDeviceHex)
	}

	// Verify config.json was updated with new device ID
	cfg2, _ := os.ReadFile(filepath.Join(stateDir, "config.json"))
	var m2 map[string]any
	_ = json.Unmarshal(cfg2, &m2)
	if m2["device_id"].(string) != newDeviceHex {
		t.Errorf("config.json device_id (%s) does not match new device ID (%s)", m2["device_id"], newDeviceHex)
	}
}

func initLegacyV5Schema(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := repository.InitSchemaV5ForTest(context.Background(), db); err != nil {
		t.Fatalf("legacy schema v5 init failed: %v", err)
	}

	folderBytes, _ := hex.DecodeString("f000000000000000000000000000000000000000000000000000000000000001")
	authorBytes, _ := hex.DecodeString("1111111111111111111111111111111111111111111111111111111111111111")
	counterBytes := make([]byte, 8)
	counterBytes[7] = 5
	revBytes := make([]byte, 8)
	revBytes[7] = 1

	_, err := db.Exec(`INSERT INTO folders (folder_id, local_author, next_counter, membership_revision, root_path)
		 VALUES (?, ?, ?, ?, ?);`, folderBytes, authorBytes, counterBytes, revBytes, "/tmp/legacy")
	if err != nil {
		t.Fatalf("insert legacy folder: %v", err)
	}
}
