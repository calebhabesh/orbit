package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/testkit"
)

// TestOrbitService_StatusReporting tests Invariant I19 & Requirement U12:
// Service status reports distinct boolean results:
// SystemdAvailable, UnitInstalled, EnabledOnLogin, CurrentlyRunning, RootVerified, CaptureSuccessful, LingeringEnabled.
func TestOrbitService_StatusReporting(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "service-state")

	ctrlAddr, stopDaemon := startTestDaemon(t, stateDir)
	defer stopDaemon()

	tokenBytes, _ := os.ReadFile(filepath.Join(stateDir, "control.token"))
	cliToken := strings.TrimSpace(string(tokenBytes))

	// 1. Initial status before setup: RootVerified=false, CaptureSuccessful=false, CurrentlyRunning=true
	statusReq, _ := http.NewRequest(http.MethodGet, ctrlAddr+"/api/v1/system/service/status", nil)
	statusReq.Header.Set("Authorization", "Bearer "+cliToken)

	client := &http.Client{}
	resp, err := client.Do(statusReq)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/system/service/status failed: %v (code %d)", err, resp.StatusCode)
	}
	var st1 control.ServiceStatusResult
	if err := json.NewDecoder(resp.Body).Decode(&st1); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if !st1.CurrentlyRunning {
		t.Errorf("expected CurrentlyRunning=true for running daemon")
	}
	if st1.RootVerified {
		t.Errorf("expected RootVerified=false before folder setup")
	}
	if st1.CaptureSuccessful {
		t.Errorf("expected CaptureSuccessful=false before initial scan")
	}
	if !strings.Contains(st1.ManualCommand, "orbit serve") {
		t.Errorf("expected ManualCommand containing orbit serve, got: %s", st1.ManualCommand)
	}
	if !strings.Contains(st1.LingeringInstruction, "loginctl enable-linger") {
		t.Errorf("expected LingeringInstruction containing loginctl enable-linger, got: %s", st1.LingeringInstruction)
	}

	// 2. Perform setup with preexisting file
	userRoot := filepath.Join(disposable, "UserDocs")
	if err := os.MkdirAll(userRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userRoot, "test.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	setupReqBody, _ := json.Marshal(control.StartSetupRequest{
		RootPath:      userRoot,
		DeviceLabel:   "Service-Test-Laptop",
		WorkspaceName: "Docs",
	})
	setupReq, _ := http.NewRequest(http.MethodPost, ctrlAddr+"/api/v1/setup/start", bytes.NewReader(setupReqBody))
	setupReq.Header.Set("Authorization", "Bearer "+cliToken)
	setupReq.Header.Set("Content-Type", "application/json")

	setupResp, err := client.Do(setupReq)
	if err != nil || setupResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/setup/start failed: %v", err)
	}
	setupResp.Body.Close()

	// 3. Post-setup service status: RootVerified=true and CaptureSuccessful=true
	statusReq2, _ := http.NewRequest(http.MethodGet, ctrlAddr+"/api/v1/system/service/status", nil)
	statusReq2.Header.Set("Authorization", "Bearer "+cliToken)

	resp2, err := client.Do(statusReq2)
	if err != nil || resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/system/service/status failed: %v", err)
	}
	var st2 control.ServiceStatusResult
	if err := json.NewDecoder(resp2.Body).Decode(&st2); err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	if !st2.CurrentlyRunning {
		t.Errorf("expected CurrentlyRunning=true")
	}
	if !st2.RootVerified {
		t.Errorf("expected RootVerified=true after setup")
	}
	if !st2.CaptureSuccessful {
		t.Errorf("expected CaptureSuccessful=true after setup capture")
	}
}

// TestOrbitService_ActionAndFallback_HTTP tests the service action endpoint
// and validates graceful handling when systemd user session is unavailable.
func TestOrbitService_ActionAndFallback_HTTP(t *testing.T) {
	t.Setenv("HOME", testkit.NewDisposable(t))
	t.Setenv("PATH", t.TempDir())
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "action-state")

	ctrlAddr, stopDaemon := startTestDaemon(t, stateDir)
	defer stopDaemon()

	tokenBytes, _ := os.ReadFile(filepath.Join(stateDir, "control.token"))
	cliToken := strings.TrimSpace(string(tokenBytes))
	client := &http.Client{}

	// Case 1: Invalid action
	invBody, _ := json.Marshal(control.ServiceActionRequest{Action: "invalid-action"})
	invReq, _ := http.NewRequest(http.MethodPost, ctrlAddr+"/api/v1/system/service/action", bytes.NewReader(invBody))
	invReq.Header.Set("Authorization", "Bearer "+cliToken)
	invReq.Header.Set("Content-Type", "application/json")

	invResp, err := client.Do(invReq)
	if err != nil {
		t.Fatal(err)
	}
	if invResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid action, got: %d", invResp.StatusCode)
	}
	invResp.Body.Close()

	// Case 2: Enable action
	actBody, _ := json.Marshal(control.ServiceActionRequest{Action: "enable"})
	actReq, _ := http.NewRequest(http.MethodPost, ctrlAddr+"/api/v1/system/service/action", bytes.NewReader(actBody))
	actReq.Header.Set("Authorization", "Bearer "+cliToken)
	actReq.Header.Set("Content-Type", "application/json")

	actResp, err := client.Do(actReq)
	if err != nil {
		t.Fatal(err)
	}
	defer actResp.Body.Close()

	// The child inherits a disposable home and no systemctl; this negative
	// check cannot install/enable a personal service on a developer workstation.
	var errRes control.ControlError
	if err := json.NewDecoder(actResp.Body).Decode(&errRes); err != nil {
		t.Fatal(err)
	}
	if actResp.StatusCode == http.StatusOK || errRes.Code != "SYSTEMD_UNAVAILABLE" {
		t.Fatalf("missing systemd was not reported: HTTP %d code=%s", actResp.StatusCode, errRes.Code)
	}
}

// TestOrbitService_AbsentSystemdGracefulFallback tests that control methods
// return actionable instructions without panic when systemd is not present.
func TestOrbitService_AbsentSystemdGracefulFallback(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "fallback-state")

	st, err := control.CheckServiceStatus(ctx, stateDir, nil)
	if err != nil {
		t.Fatalf("CheckServiceStatus failed: %v", err)
	}

	if st.ManualCommand == "" {
		t.Errorf("expected non-empty ManualCommand")
	}

	// If systemd is unavailable, verify Enable, Start, Stop return ControlError with SYSTEMD_UNAVAILABLE
	if !st.SystemdAvailable {
		_, err := control.EnableService(ctx, stateDir, "/usr/bin/orbit", nil)
		if err == nil {
			t.Fatal("expected error from EnableService when systemd unavailable")
		}
		var ctrlErr *control.ControlError
		if ok := errorAs(err, &ctrlErr); ok {
			if ctrlErr.Code != "SYSTEMD_UNAVAILABLE" {
				t.Errorf("expected code SYSTEMD_UNAVAILABLE, got %s", ctrlErr.Code)
			}
			if ctrlErr.Action == "" {
				t.Errorf("expected actionable instruction in Action field")
			}
		}

		_, startErr := control.StartService(ctx, stateDir, nil, nil)
		if startErr == nil {
			t.Fatal("expected error from StartService when systemd unavailable")
		}

		_, stopErr := control.StopService(ctx, stateDir, nil, nil)
		if stopErr == nil {
			t.Fatal("expected error from StopService when systemd unavailable")
		}
	}
}

// TestOrbitService_LingeringDocumentedNotSilent verifies that lingering configuration
// is strictly documented as an optional administrative command and never executed silently (Requirement U12).
func TestOrbitService_LingeringDocumentedNotSilent(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "lingering-state")

	st, err := control.CheckServiceStatus(ctx, stateDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(st.LingeringInstruction, "sudo loginctl enable-linger ") {
		t.Fatalf("expected instruction format 'sudo loginctl enable-linger <user>' (shown, never run), got: %s", st.LingeringInstruction)
	}

	// Verify CheckServiceStatus did NOT create any privileged files
	user := os.Getenv("USER")
	if user != "" && !st.LingeringEnabled {
		if _, statErr := os.Stat("/var/lib/systemd/linger/" + user); statErr == nil {
			t.Fatal("lingering was enabled silently without explicit user administration")
		}
	}
}

// TestOrbitService_InstallUserUnit verifies unit file installation into ~/.config/systemd/user.
func TestOrbitService_InstallUserUnit(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "unit-state")
	binPath := "/opt/orbit/bin/orbit"

	if err := control.InstallUserUnit(stateDir, binPath); err != nil {
		t.Fatalf("InstallUserUnit failed: %v", err)
	}

	unitFile := filepath.Join(tempHome, ".config", "systemd", "user", "orbit.service")
	data, err := os.ReadFile(unitFile)
	if err != nil {
		t.Fatalf("unit file not found at %s: %v", unitFile, err)
	}

	content := string(data)
	expectedTokens := []string{
		"Description=Orbit Background Engine",
		"ExecStart=" + binPath + " serve --state=" + stateDir + " --control-listen=127.0.0.1:0 --allow-init",
		"ExecStop=" + binPath + " stop --state=" + stateDir,
		"Restart=on-failure",
		"LimitNOFILE=65536",
		"MemoryHigh=512M",
		"MemoryMax=1G",
		"NoNewPrivileges=yes",
		"WantedBy=default.target",
	}

	for _, token := range expectedTokens {
		if !strings.Contains(content, token) {
			t.Errorf("missing expected token in unit file: %q", token)
		}
	}
}

// TestOrbitService_CLI_ServiceStatus verifies the CLI command 'orbit service status --json'.
func TestOrbitService_CLI_ServiceStatus(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	orbitBin := filepath.Join(root, "bin", "orbit")
	if _, err := os.Stat(orbitBin); err != nil {
		t.Skip("bin/orbit not found; skipping CLI test")
	}

	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "cli-state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(orbitBin, "service", "status", "--state="+stateDir, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit service status failed: %v (%s)", err, string(out))
	}

	var res control.ServiceStatusResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("failed to parse JSON from orbit service status: %v (%s)", err, string(out))
	}

	if res.ManualCommand == "" {
		t.Errorf("expected ManualCommand in CLI JSON output")
	}
}

func errorAs(err error, target **control.ControlError) bool {
	if err == nil {
		return false
	}
	var ce *control.ControlError
	if ok := errorAsDirect(err, &ce); ok {
		*target = ce
		return true
	}
	return false
}

func errorAsDirect(err error, target **control.ControlError) bool {
	for err != nil {
		if ce, ok := err.(*control.ControlError); ok {
			*target = ce
			return true
		}
		if u, ok := err.(interface{ Unwrap() error }); ok {
			err = u.Unwrap()
		} else {
			break
		}
	}
	return false
}
