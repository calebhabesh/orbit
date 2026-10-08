package integration_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/launcher"
	"github.com/calebhabesh/orbit/internal/state"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func startTestDaemon(t *testing.T, stateDir string) (string, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- app.ServeWithOptions(ctx, stateDir, app.ServeOptions{
			ControlAddress:  "127.0.0.1:0",
			AllowInitialize: true,
		})
	}()

	// Wait for control.addr to appear
	addrFile := filepath.Join(stateDir, "control.addr")
	client := &http.Client{Timeout: 500 * time.Millisecond}
	var ctrlAddr string

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(addrFile); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			addr := strings.TrimSpace(string(data))
			if !strings.HasPrefix(addr, "http://") {
				addr = "http://" + addr
			}
			resp, err := client.Get(addr + "/api/v1/health")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					ctrlAddr = addr
					break
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	if ctrlAddr == "" {
		cancel()
		t.Fatalf("timed out waiting for test daemon to start in %s", stateDir)
	}

	cleanup := func() {
		cancel()
		<-errCh
	}
	return ctrlAddr, cleanup
}

// TestOrbitLaunch_FirstLaunchUninitialized tests launching Orbit on a fresh uninitialized state:
// auto-initializes device identity, starts control server, and mints single-use bootstrap token.
func TestOrbitLaunch_FirstLaunchUninitialized(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "fresh-state")

	var recordedURL string
	opener := func(url string) error {
		recordedURL = url
		return nil
	}

	daemonStarter := func(ctx context.Context, sDir, ctrlAddr string) error {
		go func() {
			_ = app.ServeWithOptions(ctx, sDir, app.ServeOptions{
				ControlAddress:  ctrlAddr,
				AllowInitialize: true,
			})
		}()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	res, err := launcher.Launch(ctx, launcher.LaunchOptions{
		StateDir:       stateDir,
		ControlAddress: "127.0.0.1:0",
		BrowserOpener:  opener,
		DaemonStarter:  daemonStarter,
	})
	if err != nil {
		t.Fatalf("launcher.Launch failed: %v", err)
	}

	if !res.DaemonRunning {
		t.Errorf("expected DaemonRunning=true")
	}
	if !res.BrowserOpened {
		t.Errorf("expected BrowserOpened=true")
	}
	if recordedURL == "" {
		t.Fatal("expected browser opener to receive bootstrap URL")
	}
	if !strings.Contains(recordedURL, "/#bootstrap=") {
		t.Fatalf("expected bootstrap URL fragment, got %s", recordedURL)
	}

	// Verify state directory was cleanly initialized
	cfg, err := config.Load(stateDir)
	if err != nil {
		t.Fatalf("config.Load failed after launch: %v", err)
	}
	if cfg.DeviceID == "" || cfg.FormatVersion != 1 {
		t.Errorf("invalid config after first launch: %+v", cfg)
	}

	// Verify lock is held exclusively (no duplicate daemon permitted - Invariant I21)
	if _, err := state.Acquire(stateDir); !errors.Is(err, state.ErrLocked) {
		t.Errorf("expected state.ErrLocked for second instance, got: %v", err)
	}
}

// TestOrbitLaunch_RepeatedLaunchReusesDaemon tests running the launcher when the daemon
// is already active: reuses the running daemon, acquires fresh token, and preserves lock.
func TestOrbitLaunch_RepeatedLaunchReusesDaemon(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "running-state")

	_, stopDaemon := startTestDaemon(t, stateDir)
	defer stopDaemon()

	// Launch 1
	var url1 string
	res1, err := launcher.Launch(context.Background(), launcher.LaunchOptions{
		StateDir: stateDir,
		BrowserOpener: func(u string) error {
			url1 = u
			return nil
		},
	})
	if err != nil {
		t.Fatalf("first launch failed: %v", err)
	}
	if !res1.DaemonRunning {
		t.Errorf("expected DaemonRunning=true on launch 1")
	}

	// Launch 2 (Repeated launch while daemon still running)
	var url2 string
	res2, err := launcher.Launch(context.Background(), launcher.LaunchOptions{
		StateDir: stateDir,
		BrowserOpener: func(u string) error {
			url2 = u
			return nil
		},
	})
	if err != nil {
		t.Fatalf("repeated launch failed: %v", err)
	}
	if !res2.DaemonRunning {
		t.Errorf("expected DaemonRunning=true on launch 2")
	}

	// URLs should have distinct fresh one-use tokens
	if url1 == url2 {
		t.Errorf("expected distinct bootstrap tokens for repeated launches: %s vs %s", url1, url2)
	}

	// Daemon lock MUST remain held exclusively by the running daemon
	if _, err := state.Acquire(stateDir); !errors.Is(err, state.ErrLocked) {
		t.Errorf("expected state.ErrLocked, got: %v", err)
	}
}

// TestOrbitLaunch_InvalidOldStateRefusesOverwrite tests that corrupted configs or newer
// incompatible database schemas are rejected safely without overwriting data (Invariant I20).
func TestOrbitLaunch_InvalidOldStateRefusesOverwrite(t *testing.T) {
	disposable := testkit.NewDisposable(t)

	// Case 1: Corrupted config.json
	corruptState := filepath.Join(disposable, "corrupt-state")
	if err := os.MkdirAll(corruptState, 0o700); err != nil {
		t.Fatal(err)
	}
	corruptCfg := filepath.Join(corruptState, "config.json")
	if err := os.WriteFile(corruptCfg, []byte("{ malformed json"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := launcher.ValidateExistingState(corruptState)
	if !errors.Is(err, launcher.ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState for corrupted config, got: %v", err)
	}

	// Verify launcher refuses to launch or overwrite
	_, launchErr := launcher.Launch(context.Background(), launcher.LaunchOptions{StateDir: corruptState, NoBrowser: true})
	if launchErr == nil {
		t.Fatal("expected launch error for corrupted state")
	}

	// Verify corrupted config file was NOT deleted or overwritten
	content, _ := os.ReadFile(corruptCfg)
	if string(content) != "{ malformed json" {
		t.Fatal("corrupted config was modified unexpectedly")
	}

	// Case 2: Incompatible future SQLite schema version (e.g. 99)
	futureState := filepath.Join(disposable, "future-state")
	if err := os.MkdirAll(futureState, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = config.Save(futureState, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(make([]byte, 32)), CreatedAt: time.Now()})

	futureDBPath := filepath.Join(futureState, "metadata.sqlite")
	futureDB, err := sql.Open("sqlite", "file:"+futureDBPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = futureDB.Exec("CREATE TABLE test (id INT); PRAGMA user_version = 99;")
	_ = futureDB.Close()

	valErr := launcher.ValidateExistingState(futureState)
	if !errors.Is(valErr, launcher.ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState for future schema version, got: %v", valErr)
	}
}

// TestOrbitLaunch_TokenExchangeAndSessionSeparation tests Invariant I21:
// Single-use token exchange, replay prevention, and logging out preserves background daemon.
func TestOrbitLaunch_TokenExchangeAndSessionSeparation(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "auth-state")

	ctrlAddr, stopDaemon := startTestDaemon(t, stateDir)
	defer stopDaemon()

	var launchURL string
	_, err := launcher.Launch(context.Background(), launcher.LaunchOptions{
		StateDir: stateDir,
		BrowserOpener: func(u string) error {
			launchURL = u
			return nil
		},
	})
	if err != nil {
		t.Fatalf("launcher.Launch failed: %v", err)
	}

	// Extract token from fragment (#bootstrap=<token>)
	parts := strings.Split(launchURL, "#bootstrap=")
	if len(parts) != 2 {
		t.Fatalf("invalid bootstrap URL: %s", launchURL)
	}
	token := parts[1]

	// 1. Exchange token via POST /api/v1/auth/bootstrap
	body, _ := json.Marshal(map[string]string{"token": token})
	req, _ := http.NewRequest(http.MethodPost, ctrlAddr+"/api/v1/auth/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap exchange failed: %v (status: %d)", err, resp.StatusCode)
	}

	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "orbit_session" {
			sessionCookie = c
			break
		}
	}
	var bootRes struct {
		CSRFToken string `json:"csrf_token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&bootRes)
	resp.Body.Close()

	if sessionCookie == nil {
		t.Fatal("expected orbit_session cookie in exchange response")
	}
	if !sessionCookie.HttpOnly {
		t.Errorf("expected HttpOnly cookie")
	}

	// 2. Replay attack: Re-exchanging the same token must fail (HTTP 401)
	req2, _ := http.NewRequest(http.MethodPost, ctrlAddr+"/api/v1/auth/bootstrap", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := client.Do(req2)
	if err != nil || resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 on token replay, got: %d (%v)", resp2.StatusCode, err)
	}
	resp2.Body.Close()

	// 3. User logout: Invalidates UI session
	logoutReq, _ := http.NewRequest(http.MethodPost, ctrlAddr+"/api/v1/auth/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutReq.Header.Set("X-CSRF-Token", bootRes.CSRFToken)
	logoutResp, err := client.Do(logoutReq)
	if err != nil || logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("logout failed: %v (code: %d)", err, logoutResp.StatusCode)
	}
	logoutResp.Body.Close()

	// 4. Invariant I21: Background sync daemon remains running and lock held after UI logout
	if _, err := state.Acquire(stateDir); !errors.Is(err, state.ErrLocked) {
		t.Fatal("daemon lock was unexpectedly released on UI session logout")
	}
}

// TestOrbitLaunch_BootstrapSecretNotLeakedInSupportExport verifies that bootstrap tokens,
// control tokens, and key secrets are never included in diagnostic support exports.
func TestOrbitLaunch_BootstrapSecretNotLeakedInSupportExport(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "export-state")

	ctrlAddr, stopDaemon := startTestDaemon(t, stateDir)
	defer stopDaemon()

	tokenBytes, _ := os.ReadFile(filepath.Join(stateDir, "control.token"))
	cliToken := strings.TrimSpace(string(tokenBytes))

	// Request support bundle export into disposable path
	exportDest := filepath.Join(disposable, "support-bundle.tar.gz")
	body, _ := json.Marshal(control.SupportExportRequest{DestinationPath: exportDest})
	exportReq, _ := http.NewRequest(http.MethodPost, ctrlAddr+"/api/v1/support/export", bytes.NewReader(body))
	exportReq.Header.Set("Authorization", "Bearer "+cliToken)
	exportReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(exportReq)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("support export failed: %v", err)
	}
	var exportRes control.SupportExportResult
	_ = json.NewDecoder(resp.Body).Decode(&exportRes)
	resp.Body.Close()

	// Inspect the tar.gz file
	bundlePath := exportRes.ArchivePath
	f, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open support bundle %s: %v", bundlePath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, tr)
		content := buf.String()

		// Verify no leaked CLI token
		if strings.Contains(content, cliToken) {
			t.Fatalf("support bundle leaked control token in %s", header.Name)
		}
		// Verify no private key leaked
		if strings.Contains(content, "PRIVATE KEY") {
			t.Fatalf("support bundle leaked private key in %s", header.Name)
		}
	}
}

// TestOrbitLaunch_CLI_Help runs the real compiled binary's product help and
// its low-level engine usage.
func TestOrbitLaunch_CLI_Help(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	orbitBin := filepath.Join(root, "bin", "orbit")
	if _, err := os.Stat(orbitBin); err != nil {
		t.Skip("bin/orbit not found; run make build first")
	}

	out, err := exec.Command(orbitBin, "help").CombinedOutput()
	if err != nil {
		t.Fatalf("orbit help failed: %v (%s)", err, string(out))
	}
	if !strings.Contains(string(out), "Orbit - Local file synchronization") {
		t.Errorf("unexpected output from orbit help:\n%s", string(out))
	}

	out, err = exec.Command(orbitBin, "engine", "help").CombinedOutput()
	if err != nil {
		t.Fatalf("orbit engine help failed: %v (%s)", err, string(out))
	}
	if !strings.Contains(string(out), "usage: orbit engine <init|serve") {
		t.Errorf("unexpected output from orbit engine help:\n%s", string(out))
	}
}
