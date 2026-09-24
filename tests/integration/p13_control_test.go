package integration_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

// TestP13CLIDiagnosticsAndDoctor verifies filesync doctor, metrics, and logs CLI commands.
func TestP13CLIDiagnosticsAndDoctor(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)
	stateDir := filepath.Join(disposable, "state")

	// 1. Initialize
	initCmd := exec.Command(binary, "init", "--state", stateDir)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}

	// 2. Doctor CLI
	doctorCmd := exec.Command(binary, "doctor", "--state", stateDir, "--json")
	out, err := doctorCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor --json failed: %v\n%s", err, out)
	}
	var report control.DoctorReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("unmarshal doctor report: %v\noutput: %s", err, out)
	}
	if report.OverallStatus != control.StatusOk {
		t.Fatalf("doctor overall status = %s, want %s", report.OverallStatus, control.StatusOk)
	}
	if len(report.Checks) == 0 {
		t.Fatal("expected at least one doctor check in report")
	}

	// 3. Plaintext doctor
	doctorPlain := exec.Command(binary, "doctor", "--state", stateDir)
	plainOut, err := doctorPlain.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor plaintext failed: %v\n%s", err, plainOut)
	}
	if !strings.Contains(string(plainOut), "doctor report: overall=OK") {
		t.Fatalf("unexpected plaintext output:\n%s", plainOut)
	}

	// 4. Metrics CLI
	metricsCmd := exec.Command(binary, "metrics", "--state", stateDir, "--json")
	mOut, err := metricsCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("metrics --json failed: %v\n%s", err, mOut)
	}
	var metrics control.OperationalMetrics
	if err := json.Unmarshal(mOut, &metrics); err != nil {
		t.Fatalf("unmarshal metrics: %v\noutput: %s", err, mOut)
	}

	// 5. Logs CLI
	logsCmd := exec.Command(binary, "logs", "--state", stateDir, "--json")
	lOut, err := logsCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("logs --json failed: %v\n%s", err, lOut)
	}
	var events []repository.EventLogEntry
	if err := json.Unmarshal(lOut, &events); err != nil {
		t.Fatalf("unmarshal events: %v\noutput: %s", err, lOut)
	}
}

// TestP13SupportExportSanitizationAndRedaction verifies that:
// 1. Support export archive is created locally.
// 2. Private keys, session tokens, and file content are completely excluded.
// 3. User filesystem paths are redacted to pseudonyms by default (I15).
func TestP13SupportExportSanitizationAndRedaction(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)
	stateDir := filepath.Join(disposable, "state")
	rootDir := filepath.Join(disposable, "workspace_secret_folder")
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Initialize
	if out, err := exec.Command(binary, "init", "--state", stateDir).CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	// Register a folder
	var folderID history.ID
	folderID[0] = 0xAA
	folderID[31] = 0xBB
	folderHex := hex.EncodeToString(folderID[:])

	secretFile := filepath.Join(rootDir, "financial_records.pdf")
	if err := os.WriteFile(secretFile, []byte("TOP_SECRET_USER_PLAINTEXT_PAYROLL_DATA"), 0o600); err != nil {
		t.Fatal(err)
	}

	regCmd := exec.Command(binary, "register", "--state", stateDir, "--folder", folderHex, "--root", rootDir)
	if out, err := regCmd.CombinedOutput(); err != nil {
		t.Fatalf("register: %v\n%s", err, out)
	}

	// Scan to capture file
	scanCmd := exec.Command(binary, "scan", "--state", stateDir, "--folder", folderHex)
	if out, err := scanCmd.CombinedOutput(); err != nil {
		t.Fatalf("scan: %v\n%s", err, out)
	}

	// Export support bundle
	bundleTarget := filepath.Join(disposable, "support_bundle.tar.gz")
	exportCmd := exec.Command(binary, "support-export", "--state", stateDir, "--out", bundleTarget, "--json")
	out, err := exportCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("support-export: %v\n%s", err, out)
	}

	var res control.SupportExportResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal support export result: %v\noutput: %s", err, out)
	}
	if !res.RedactedPaths {
		t.Fatal("expected RedactedPaths = true by default")
	}

	// Inspect the tar.gz archive directly
	archiveFile, err := os.Open(bundleTarget)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer archiveFile.Close()

	gz, err := gzip.NewReader(archiveFile)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	fileNames := make(map[string]bool)
	var allArchiveBytes bytes.Buffer

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read next: %v", err)
		}
		fileNames[header.Name] = true
		content, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read tar file %s: %v", header.Name, err)
		}
		allArchiveBytes.Write(content)
	}

	archiveString := allArchiveBytes.String()

	// Verify required diagnostics files are present
	if !fileNames["manifest.json"] {
		t.Fatal("missing manifest.json in support bundle")
	}
	if !fileNames["doctor.json"] {
		t.Fatal("missing doctor.json in support bundle")
	}
	if !fileNames["metrics.json"] {
		t.Fatal("missing metrics.json in support bundle")
	}

	// Invariant I15 checks:
	// 1. No raw keys: check for "BEGIN PRIVATE KEY" or "PRIVATE KEY"
	if strings.Contains(archiveString, "PRIVATE KEY") {
		t.Fatal("INVARIANT VIOLATION: support bundle contains private key material")
	}

	// 2. No session tokens: read control.token from stateDir
	if tokenData, err := os.ReadFile(filepath.Join(stateDir, "control.token")); err == nil {
		tok := strings.TrimSpace(string(tokenData))
		if len(tok) >= 32 && strings.Contains(archiveString, tok) {
			t.Fatal("INVARIANT VIOLATION: support bundle contains CLI control.token")
		}
	}

	// 3. No plaintext file content: check for the secret payload
	if strings.Contains(archiveString, "TOP_SECRET_USER_PLAINTEXT_PAYROLL_DATA") {
		t.Fatal("INVARIANT VIOLATION: support bundle contains plaintext file content")
	}

	// 4. User filesystem paths are redacted: real filenames and folder names must NOT appear
	if strings.Contains(archiveString, "financial_records.pdf") {
		t.Fatal("INVARIANT VIOLATION: user file name was not redacted in support export")
	}
	if strings.Contains(archiveString, "workspace_secret_folder") {
		t.Fatal("INVARIANT VIOLATION: user root path was not redacted in support export")
	}
}

// TestP13FolderLifecycleAndZeroReplicatedDeletes verifies:
// 1. Folder pause, resume, and revalidate.
// 2. Unregistering a folder preserves working files on disk.
// 3. Unregistering authors ZERO deletion tombstones into versions table (I09).
func TestP13FolderLifecycleAndZeroReplicatedDeletes(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)
	stateDir := filepath.Join(disposable, "state")
	rootDir := filepath.Join(disposable, "my_important_files")
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Initialize
	if out, err := exec.Command(binary, "init", "--state", stateDir).CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	var folderID history.ID
	folderID[0] = 0x11
	folderID[31] = 0x22
	folderHex := hex.EncodeToString(folderID[:])

	// Create user file
	userFile := filepath.Join(rootDir, "important.txt")
	userContent := []byte("this user data must never be deleted on unregister")
	if err := os.WriteFile(userFile, userContent, 0o600); err != nil {
		t.Fatal(err)
	}

	// 1. Add folder
	addCmd := exec.Command(binary, "folders", "add", "--state", stateDir, "--folder", folderHex, "--root", rootDir)
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("folders add: %v\n%s", err, out)
	}

	// 2. Scan folder to author initial version
	scanCmd := exec.Command(binary, "scan", "--state", stateDir, "--folder", folderHex)
	if out, err := scanCmd.CombinedOutput(); err != nil {
		t.Fatalf("scan: %v\n%s", err, out)
	}

	// Count versions before pause/remove
	db, err := repository.Open(context.Background(), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	versionsBefore, err := db.VersionIDs(context.Background(), folderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versionsBefore) == 0 {
		t.Fatal("expected at least one version authored after scan")
	}
	db.Close()

	// 3. Pause folder
	pauseCmd := exec.Command(binary, "folders", "pause", "--state", stateDir, "--folder", folderHex, "--reason", "maintenance test")
	if out, err := pauseCmd.CombinedOutput(); err != nil {
		t.Fatalf("folders pause: %v\n%s", err, out)
	}

	// List folders: verify paused
	listCmd := exec.Command(binary, "folders", "list", "--state", stateDir, "--json")
	listOut, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("folders list: %v\n%s", err, listOut)
	}
	var folders []repository.FolderRecord
	if err := json.Unmarshal(listOut, &folders); err != nil {
		t.Fatalf("unmarshal folders: %v\n%s", err, listOut)
	}
	if len(folders) != 1 || !folders[0].Paused || folders[0].PauseReason != "maintenance test" {
		t.Fatalf("expected folder paused with reason, got: %+v", folders)
	}

	// 4. Resume folder
	resumeCmd := exec.Command(binary, "folders", "resume", "--state", stateDir, "--folder", folderHex)
	if out, err := resumeCmd.CombinedOutput(); err != nil {
		t.Fatalf("folders resume: %v\n%s", err, out)
	}

	// 5. Revalidate root (via safety command)
	revalCmd := exec.Command(binary, "safety", "root-revalidate", "--state", stateDir, "--folder", folderHex)
	if out, err := revalCmd.CombinedOutput(); err != nil {
		t.Fatalf("safety root-revalidate: %v\n%s", err, out)
	}

	// 6. Remove registration (CRITICAL TEST)
	removeCmd := exec.Command(binary, "folders", "remove", "--state", stateDir, "--folder", folderHex, "--json")
	if out, err := removeCmd.CombinedOutput(); err != nil {
		t.Fatalf("folders remove: %v\n%s", err, out)
	}

	// Invariant I09 / Requirement: "Removal of folder registration and uninstall paths do not emit replicated deletes."
	// 1. Files on disk MUST still exist and have unchanged contents
	remaining, err := os.ReadFile(userFile)
	if err != nil {
		t.Fatalf("user file was deleted from disk on unregister! error: %v", err)
	}
	if !bytes.Equal(remaining, userContent) {
		t.Fatalf("user file content corrupted! got %s, want %s", remaining, userContent)
	}

	// 2. Check versions in database: exactly matches versionsBefore; ZERO deletion tombstones authored!
	db, err = repository.Open(context.Background(), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	versionsAfter, err := db.VersionIDs(context.Background(), folderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versionsAfter) != len(versionsBefore) {
		t.Fatalf("versions count changed on unregister: before=%d, after=%d (tombstones were author-emitted!)",
			len(versionsBefore), len(versionsAfter))
	}
	for _, v := range versionsAfter {
		env, err := db.Envelope(context.Background(), v)
		if err == nil && env.Kind == history.KindTombstone {
			t.Fatalf("found tombstone version %v created on folder unregister!", v)
		}
	}
}

// TestP13MaintenanceBackupCheckRecoveryAndReset verifies database backup, migration check,
// recovery inspection, and identity reset.
func TestP13MaintenanceBackupCheckRecoveryAndReset(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)
	stateDir := filepath.Join(disposable, "state")

	// Initialize
	if out, err := exec.Command(binary, "init", "--state", stateDir).CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	// 1. Backup
	backupPath := filepath.Join(disposable, "backup.sqlite")
	backupCmd := exec.Command(binary, "maintenance", "backup", "--state", stateDir, "--out", backupPath, "--json")
	out, err := backupCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("maintenance backup: %v\n%s", err, out)
	}
	var backupRes control.BackupResult
	if err := json.Unmarshal(out, &backupRes); err != nil {
		t.Fatalf("unmarshal backup result: %v\n%s", err, out)
	}
	if backupRes.SizeBytes <= 0 {
		t.Fatalf("backup size = %d, expected > 0", backupRes.SizeBytes)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file does not exist: %v", err)
	}

	// 2. Migration Check
	checkCmd := exec.Command(binary, "maintenance", "check", "--state", stateDir, "--json")
	out, err = checkCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("maintenance check: %v\n%s", err, out)
	}
	var checkRes control.MigrationCheckResult
	if err := json.Unmarshal(out, &checkRes); err != nil {
		t.Fatalf("unmarshal check result: %v\n%s", err, out)
	}
	if checkRes.Status != "up_to_date" && checkRes.Status != "COMPATIBLE" {
		t.Fatalf("check status = %s, want up_to_date or COMPATIBLE", checkRes.Status)
	}

	// 3. Recovery Inspection
	recCmd := exec.Command(binary, "maintenance", "recovery", "--state", stateDir, "--json")
	out, err = recCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("maintenance recovery: %v\n%s", err, out)
	}
	var recRes control.RecoveryInspectionResult
	if err := json.Unmarshal(out, &recRes); err != nil {
		t.Fatalf("unmarshal recovery result: %v\n%s", err, out)
	}

	// 4. Reset Identity
	resetCmd := exec.Command(binary, "maintenance", "reset-identity", "--state", stateDir, "--json")
	out, err = resetCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("maintenance reset-identity: %v\n%s", err, out)
	}
	var resetRes control.ResetIdentityResult
	if err := json.Unmarshal(out, &resetRes); err != nil {
		t.Fatalf("unmarshal reset-identity result: %v\n%s", err, out)
	}
	if resetRes.OldDeviceID == resetRes.NewDeviceID {
		t.Fatal("expected new device ID to differ from old device ID")
	}
}

// TestP13LoopbackControlSecurityAndBootstrap validates all security guarantees:
// 1. Strict Host header validation (DNS rebinding prevention).
// 2. Strict Origin header validation (cross-origin browser protection).
// 3. Unauthenticated sensitive reads fail with 401 Unauthorized.
// 4. Authenticated CLI requests (Bearer token) succeed.
// 5. One-use bootstrap token exchange to HttpOnly/SameSite session cookie.
// 6. Bootstrap token replay is rejected.
// 7. CSRF token requirement for state-mutating requests (I16).
// 8. Session logout terminates access.
func TestP13LoopbackControlSecurityAndBootstrap(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)
	stateDir := filepath.Join(disposable, "state")

	// Initialize
	if out, err := exec.Command(binary, "init", "--state", stateDir).CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	// Start agent serving control listener on random loopback port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "serve", "--state", stateDir, "--control-listen", "127.0.0.1:0", "--no-watch")
	var stdoutBuf safeBuffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start filesync serve: %v", err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	// Wait for control listener to be ready
	var controlURL string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		output := stdoutBuf.String()
		if strings.Contains(output, "control-listener=") {
			idx := strings.Index(output, "control-listener=")
			rest := output[idx+len("control-listener="):]
			if lineEnd := strings.IndexAny(rest, " \r\n"); lineEnd != -1 {
				controlURL = rest[:lineEnd]
			} else {
				controlURL = strings.TrimSpace(rest)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if controlURL == "" {
		t.Fatalf("timed out waiting for control listener; stdout: %s", stdoutBuf.String())
	}

	// Read local CLI token
	tokenBytes, err := os.ReadFile(filepath.Join(stateDir, "control.token"))
	if err != nil {
		t.Fatalf("read control.token: %v", err)
	}
	cliToken := strings.TrimSpace(string(tokenBytes))

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 3 * time.Second}

	// 1. Test Host Header DNS Rebinding Attack:
	// Request with hostile Host header must be rejected with 403 Forbidden.
	rebindingReq, err := http.NewRequest("GET", controlURL+"/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	rebindingReq.Host = "evil.attacker.com"
	resp, err := client.Do(rebindingReq)
	if err != nil {
		t.Fatalf("rebinding request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("DNS rebinding check: got status %d, want 400 Bad Request or 403 Forbidden", resp.StatusCode)
	}

	// 2. Test Cross-Origin Browser Mutation Attack:
	// Browser sending Origin: http://evil.attacker.com must be rejected with 403 Forbidden (I16).
	corsReq, err := http.NewRequest("POST", controlURL+"/api/v1/folders", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	corsReq.Header.Set("Origin", "http://evil.attacker.com")
	resp, err = client.Do(corsReq)
	if err != nil {
		t.Fatalf("cross-origin request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Cross-Origin check: got status %d, want 403 Forbidden", resp.StatusCode)
	}

	// 3. Test Unauthenticated Sensitive Read:
	// GET /api/v1/doctor without credentials must fail with 401 Unauthorized.
	unauthReq, err := http.NewRequest("GET", controlURL+"/api/v1/doctor", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = client.Do(unauthReq)
	if err != nil {
		t.Fatalf("unauth request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated read: got status %d, want 401 Unauthorized", resp.StatusCode)
	}

	// 4. Test Authenticated Read using local CLI Bearer token:
	cliReq, err := http.NewRequest("GET", controlURL+"/api/v1/doctor", nil)
	if err != nil {
		t.Fatal(err)
	}
	cliReq.Header.Set("Authorization", "Bearer "+cliToken)
	resp, err = client.Do(cliReq)
	if err != nil {
		t.Fatalf("auth read: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated read with CLI token: got status %d, want 200 OK", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Generate Bootstrap Token via CLI command:
	bootCmd := exec.Command(binary, "control", "bootstrap-token", "--state", stateDir, "--json")
	bOut, err := bootCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("control bootstrap-token CLI failed: %v\n%s", err, bOut)
	}
	var bootGen struct {
		BootstrapToken string `json:"bootstrap_token"`
		ExpiresInSecs  int    `json:"expires_in_secs"`
	}
	if err := json.Unmarshal(bOut, &bootGen); err != nil {
		t.Fatalf("unmarshal bootstrap token output: %v\n%s", err, bOut)
	}
	if bootGen.BootstrapToken == "" {
		t.Fatal("received empty bootstrap token")
	}

	// 6. Exchange Bootstrap Token for Browser Session:
	exchangeBody, _ := json.Marshal(map[string]string{"token": bootGen.BootstrapToken})
	exReq, err := http.NewRequest("POST", controlURL+"/api/v1/auth/bootstrap", bytes.NewReader(exchangeBody))
	if err != nil {
		t.Fatal(err)
	}
	exReq.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(exReq)
	if err != nil {
		t.Fatalf("bootstrap exchange: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap exchange: got status %d, want 200 OK", resp.StatusCode)
	}

	var bootRes control.BootstrapResult
	if err := json.NewDecoder(resp.Body).Decode(&bootRes); err != nil {
		t.Fatalf("decode bootstrap result: %v", err)
	}
	if bootRes.SessionID == "" || bootRes.CSRFToken == "" {
		t.Fatalf("empty session or CSRF token: %+v", bootRes)
	}

	// Verify HttpOnly SameSite cookie was set
	parsedURL, _ := url.Parse(controlURL)
	cookies := jar.Cookies(parsedURL)
	var foundCookie bool
	for _, c := range cookies {
		if c.Name == "filesync_session" && c.Value == bootRes.SessionID {
			foundCookie = true
			break
		}
	}
	if !foundCookie {
		t.Fatal("filesync_session cookie was not stored in client jar")
	}

	// 7. Verify Bootstrap Token Burn (Replay Rejected):
	replayReq, _ := http.NewRequest("POST", controlURL+"/api/v1/auth/bootstrap", bytes.NewReader(exchangeBody))
	replayReq.Header.Set("Content-Type", "application/json")
	replayResp, err := client.Do(replayReq)
	if err != nil {
		t.Fatal(err)
	}
	replayResp.Body.Close()
	if replayResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bootstrap replay: got status %d, want 401 Unauthorized (token was not burned!)", replayResp.StatusCode)
	}

	// 8. CSRF Protection for Browser Sessions:
	// A mutating POST with session cookie but MISSING X-CSRF-Token must fail with 403 Forbidden.
	mutatePayload := []byte(`{"folder":"1111111111111111111111111111111111111111111111111111111111111111","reason":"csrf test"}`)
	noCsrfReq, _ := http.NewRequest("POST", controlURL+"/api/v1/folders/pause", bytes.NewReader(mutatePayload))
	noCsrfReq.Header.Set("Content-Type", "application/json")
	noCsrfResp, err := client.Do(noCsrfReq)
	if err != nil {
		t.Fatal(err)
	}
	noCsrfResp.Body.Close()
	if noCsrfResp.StatusCode != http.StatusForbidden {
		t.Fatalf("CSRF missing header: got status %d, want 403 Forbidden", noCsrfResp.StatusCode)
	}

	// A mutating POST with INVALID X-CSRF-Token must fail with 403 Forbidden.
	badCsrfReq, _ := http.NewRequest("POST", controlURL+"/api/v1/folders/pause", bytes.NewReader(mutatePayload))
	badCsrfReq.Header.Set("Content-Type", "application/json")
	badCsrfReq.Header.Set("X-CSRF-Token", "wrong-csrf-token")
	badCsrfResp, err := client.Do(badCsrfReq)
	if err != nil {
		t.Fatal(err)
	}
	badCsrfResp.Body.Close()
	if badCsrfResp.StatusCode != http.StatusForbidden {
		t.Fatalf("CSRF invalid header: got status %d, want 403 Forbidden", badCsrfResp.StatusCode)
	}

	// Mutating POST with VALID X-CSRF-Token passes the CSRF middleware.
	validCsrfReq, _ := http.NewRequest("POST", controlURL+"/api/v1/folders/pause", bytes.NewReader(mutatePayload))
	validCsrfReq.Header.Set("Content-Type", "application/json")
	validCsrfReq.Header.Set("X-CSRF-Token", bootRes.CSRFToken)
	validCsrfResp, err := client.Do(validCsrfReq)
	if err != nil {
		t.Fatal(err)
	}
	validCsrfResp.Body.Close()
	// Passed CSRF middleware (domain logic error about folder not registered is expected and acceptable, but not 403 CSRF error)
	if validCsrfResp.StatusCode == http.StatusForbidden {
		t.Fatalf("valid CSRF token was rejected with 403 Forbidden")
	}

	// 9. Logout terminates session
	logoutReq, _ := http.NewRequest("POST", controlURL+"/api/v1/auth/logout", strings.NewReader(`{}`))
	logoutReq.Header.Set("X-CSRF-Token", bootRes.CSRFToken)
	logoutResp, err := client.Do(logoutReq)
	if err != nil {
		t.Fatal(err)
	}
	logoutResp.Body.Close()
	if logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("logout returned status %d, want 200 OK", logoutResp.StatusCode)
	}

	// Subsequent read with old session cookie must now be rejected
	postLogoutReq, _ := http.NewRequest("GET", controlURL+"/api/v1/doctor", nil)
	postLogoutResp, err := client.Do(postLogoutReq)
	if err != nil {
		t.Fatal(err)
	}
	postLogoutResp.Body.Close()
	if postLogoutResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("post-logout request: got status %d, want 401 Unauthorized", postLogoutResp.StatusCode)
	}
}

// TestP13ErrorCategoriesHaveSafeNextActions verifies all 15 stable error categories.
func TestP13ErrorCategoriesHaveSafeNextActions(t *testing.T) {
	categories := []*control.ControlError{
		control.UnauthorizedError(""),
		control.MembershipMismatchError(""),
		control.IncompatibleVersionError(""),
		control.InvalidManifestError(""),
		control.InvalidPathError("test/path", "too long"),
		control.StaleViewError([]history.VersionID{}, history.Digest{}),
		control.RootUnavailableError("/mnt/data", "not mounted"),
		control.StructuralConflictError("dir/file", "collision"),
		control.ContentPendingError(""),
		control.ContentExpiredError(""),
		control.ContentUnavailableError(""),
		control.DiskBudgetError(""),
		control.IOError(""),
		control.UnstableFileError("foo.txt", "mtime changed"),
		control.RetryExhaustedError("task-1", "connection refused"),
	}

	expectedCodes := map[string]bool{
		"UNAUTHORIZED":         true,
		"MEMBERSHIP_MISMATCH":  true,
		"INCOMPATIBLE_VERSION": true,
		"INVALID_MANIFEST":     true,
		"INVALID_PATH":         true,
		"STALE_VIEW":           true,
		"ROOT_UNAVAILABLE":     true,
		"STRUCTURAL_CONFLICT":  true,
		"CONTENT_PENDING":      true,
		"CONTENT_EXPIRED":      true,
		"CONTENT_UNAVAILABLE":  true,
		"DISK_BUDGET":          true,
		"IO_ERROR":             true,
		"UNSTABLE_FILE":        true,
		"RETRY_EXHAUSTED":      true,
	}

	seenCodes := make(map[string]bool)

	for _, ce := range categories {
		if ce.Code == "" {
			t.Errorf("empty error code for %+v", ce)
		}
		if ce.Message == "" {
			t.Errorf("empty message for code %s", ce.Code)
		}
		if ce.Action == "" {
			t.Errorf("empty action for code %s", ce.Code)
		}
		if !expectedCodes[ce.Code] {
			t.Errorf("unexpected error code: %s", ce.Code)
		}
		seenCodes[ce.Code] = true
	}

	if len(seenCodes) != 15 {
		t.Fatalf("expected 15 unique stable error categories, saw %d", len(seenCodes))
	}
}
