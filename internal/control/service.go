package control

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/state"
)

// ServiceStatus inspects the system environment, systemd user service state,
// lingering configuration, and engine data readiness.
func (c *Controller) ServiceStatus(ctx context.Context) (*ServiceStatusResult, error) {
	return CheckServiceStatus(ctx, c.db.StateDir(), c.db)
}

// ServiceAction dispatches service lifecycle actions (enable, start, stop).
func (c *Controller) ServiceAction(ctx context.Context, req ServiceActionRequest) (*ServiceActionResult, error) {
	stateDir := c.db.StateDir()
	switch req.Action {
	case "enable":
		bin, _ := os.Executable()
		return EnableService(ctx, stateDir, bin, c.db)
	case "start":
		return StartService(ctx, stateDir, c.db)
	case "stop":
		return StopService(ctx, stateDir, c.db)
	case "restart":
		return RestartService(ctx, stateDir, c.db)
	default:
		return nil, &ControlError{
			Code:      "INVALID_ACTION",
			Message:   fmt.Sprintf("unknown service action %q; valid actions: enable, start, stop, restart", req.Action),
			Retryable: false,
			Action:    "specify a valid service action (enable, start, stop, restart)",
		}
	}
}

// CheckServiceStatus inspects the system environment, systemd user service state,
// lingering configuration, and engine data readiness.
func CheckServiceStatus(ctx context.Context, stateDir string, db *repository.DB) (*ServiceStatusResult, error) {
	if stateDir == "" {
		stateDir = config.DefaultStateDir()
	}

	res := &ServiceStatusResult{
		ManualCommand: fmt.Sprintf("orbit serve --state=%s --control-listen=127.0.0.1:8080", stateDir),
	}

	// 1. Check systemctl availability and user bus reachability
	systemctlPath, err := exec.LookPath("systemctl")
	if err != nil {
		res.SystemdAvailable = false
		res.StatusDetail = "systemctl command not found in PATH"
	} else {
		// Ping user bus with a short timeout
		busCtx, busCancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer busCancel()
		cmd := exec.CommandContext(busCtx, systemctlPath, "--user", "is-system-running")
		out, runErr := cmd.CombinedOutput()
		outStr := strings.TrimSpace(string(out))

		if runErr != nil && (busCtx.Err() != nil || strings.Contains(outStr, "Failed to connect to bus") || strings.Contains(outStr, "bus connection refused") || strings.Contains(outStr, "No such file or directory")) {
			res.SystemdAvailable = false
			res.StatusDetail = "systemd user session bus unavailable"
		} else {
			res.SystemdAvailable = true
		}
	}

	// 2. Unit file existence
	var userUnitPath string
	if home, err := os.UserHomeDir(); err == nil {
		userUnitPath = filepath.Join(home, ".config", "systemd", "user", "orbit.service")
		if _, err := os.Stat(userUnitPath); err == nil {
			res.UnitInstalled = true
		}
	}
	if !res.UnitInstalled {
		for _, p := range []string{
			"/usr/lib/systemd/user/orbit.service",
			"/usr/local/lib/systemd/user/orbit.service",
			"/etc/systemd/user/orbit.service",
		} {
			if _, err := os.Stat(p); err == nil {
				res.UnitInstalled = true
				break
			}
		}
	}

	// 3. Enabled on login
	if res.SystemdAvailable {
		enabledCtx, enabledCancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer enabledCancel()
		cmd := exec.CommandContext(enabledCtx, systemctlPath, "--user", "is-enabled", "orbit.service")
		out, _ := cmd.CombinedOutput()
		if strings.TrimSpace(string(out)) == "enabled" {
			res.EnabledOnLogin = true
		}
	}

	if res.UnitInstalled {
		if err := validateSelectedService(stateDir); err != nil {
			res.EnabledOnLogin = false
			res.UnitInstalled = false
			res.StatusDetail = "service belongs to another or unverified state"
		}
	}
	// 4. Currently running
	if db != nil {
		res.CurrentlyRunning = true
	} else {
		// Test lock or systemctl status
		testLock, err := state.Acquire(stateDir)
		if errors.Is(err, state.ErrLocked) {
			res.CurrentlyRunning = true
		} else if err == nil {
			_ = testLock.Close()
		}

	}

	// 5. Root verified
	if db != nil {
		registered, err := db.RegisteredFolders(ctx)
		if err == nil && len(registered) > 0 {
			allValid := true
			for _, reg := range registered {
				info, statErr := os.Lstat(reg.Path)
				if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					allValid = false
					break
				}
				stat, ok := info.Sys().(*syscall.Stat_t)
				if !ok || uint64(stat.Dev) != reg.Device || stat.Ino != reg.Inode {
					allValid = false
					break
				}
			}
			res.RootVerified = allValid
		}
	}

	// 6. Capture successful
	if db != nil {
		hasCaptured, err := db.HasAnyCapturedVersions(ctx)
		if err == nil && hasCaptured {
			res.CaptureSuccessful = true
		}
	}

	// 7. Lingering status
	currentUsername := os.Getenv("USER")
	if currentUsername == "" {
		if u, err := user.Current(); err == nil {
			currentUsername = u.Username
		}
	}
	if currentUsername != "" {
		res.LingeringInstruction = "loginctl enable-linger " + currentUsername
		// Check linger file
		if _, err := os.Stat("/var/lib/systemd/linger/" + currentUsername); err == nil {
			res.LingeringEnabled = true
		} else if loginctl, err := exec.LookPath("loginctl"); err == nil {
			lingerCtx, lingerCancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer lingerCancel()
			out, err := exec.CommandContext(lingerCtx, loginctl, "show-user", currentUsername, "-p", "Linger").CombinedOutput()
			if err == nil && strings.Contains(string(out), "Linger=yes") {
				res.LingeringEnabled = true
			}
		}
	}

	return res, nil
}

// EnableService installs the user unit file if necessary and enables it with systemctl --user.
func EnableService(ctx context.Context, stateDir, binPath string, db *repository.DB) (*ServiceActionResult, error) {
	st, err := CheckServiceStatus(ctx, stateDir, db)
	if err != nil {
		return nil, err
	}
	if !st.SystemdAvailable {
		return &ServiceActionResult{
			Action:  "enable",
			Success: false,
			Status:  *st,
			Message: "systemd user service unavailable; run manual command instead: " + st.ManualCommand,
		}, &ControlError{
			Code:      "SYSTEMD_UNAVAILABLE",
			Message:   "systemd user session is unavailable in this environment",
			Retryable: false,
			Action:    "run manually: " + st.ManualCommand,
		}
	}

	// Ensure unit file is installed
	if !st.UnitInstalled {
		if err := InstallUserUnit(stateDir, binPath); err != nil {
			if errors.Is(err, os.ErrExist) {
				return nil, &ControlError{Code: "SERVICE_SELECTION_REQUIRED", Message: "existing unit is preserved", Action: "inspect the existing service selection"}
			}
			return nil, fmt.Errorf("install user unit: %w", err)
		}
	}

	if err := validateSelectedService(stateDir); err != nil {
		return nil, err
	}

	systemctlPath, _ := exec.LookPath("systemctl")
	// Daemon-reload
	if out, err := exec.CommandContext(ctx, systemctlPath, "--user", "daemon-reload").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("systemctl daemon-reload: %w (%s)", err, out)
	}

	// Enable unit
	cmd := exec.CommandContext(ctx, systemctlPath, "--user", "enable", "orbit.service")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("systemctl --user enable: %w (%s)", err, stderr.String())
	}

	updatedStatus, statusErr := CheckServiceStatus(ctx, stateDir, db)
	if statusErr != nil {
		return nil, statusErr
	}
	return &ServiceActionResult{
		Action:  "enable",
		Success: updatedStatus.EnabledOnLogin,
		Status:  *updatedStatus,
		Message: "orbit user service enabled successfully",
	}, nil
}

// StartService activates the user unit with systemctl --user start.
func StartService(ctx context.Context, stateDir string, db *repository.DB) (*ServiceActionResult, error) {
	st, err := CheckServiceStatus(ctx, stateDir, db)
	if err != nil {
		return nil, err
	}
	if !st.SystemdAvailable {
		return &ServiceActionResult{
			Action:  "start",
			Success: false,
			Status:  *st,
			Message: "systemd user service unavailable; run manual command instead: " + st.ManualCommand,
		}, &ControlError{
			Code:      "SYSTEMD_UNAVAILABLE",
			Message:   "systemd user session is unavailable in this environment",
			Retryable: false,
			Action:    "run manually: " + st.ManualCommand,
		}
	}

	if err := validateSelectedService(stateDir); err != nil {
		return nil, err
	}

	systemctlPath, _ := exec.LookPath("systemctl")
	if st.CurrentlyRunning && !serviceOwnsState(ctx, systemctlPath, stateDir) {
		return nil, manualDaemonError()
	}
	cmd := exec.CommandContext(ctx, systemctlPath, "--user", "start", "orbit.service")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("systemctl --user start: %w (%s)", err, stderr.String())
	}

	owned := waitServiceOwnsState(ctx, systemctlPath, stateDir)
	updatedStatus, statusErr := CheckServiceStatus(ctx, stateDir, db)
	if statusErr != nil {
		return nil, statusErr
	}
	return &ServiceActionResult{
		Action:  "start",
		Success: owned && updatedStatus.CurrentlyRunning,
		Status:  *updatedStatus,
		Message: "orbit user service started successfully",
	}, nil
}

// StopService deactivates the user unit with systemctl --user stop.
func StopService(ctx context.Context, stateDir string, db *repository.DB) (*ServiceActionResult, error) {
	st, err := CheckServiceStatus(ctx, stateDir, db)
	if err != nil {
		return nil, err
	}
	if !st.SystemdAvailable {
		return &ServiceActionResult{
			Action:  "stop",
			Success: false,
			Status:  *st,
			Message: "systemd user service unavailable",
		}, &ControlError{
			Code:      "SYSTEMD_UNAVAILABLE",
			Message:   "systemd user session is unavailable in this environment",
			Retryable: false,
			Action:    "stop background process manually",
		}
	}

	if err := validateSelectedService(stateDir); err != nil {
		return nil, err
	}

	systemctlPath, _ := exec.LookPath("systemctl")
	cmd := exec.CommandContext(ctx, systemctlPath, "--user", "stop", "orbit.service")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("systemctl --user stop: %w (%s)", err, stderr.String())
	}

	updatedStatus, statusErr := CheckServiceStatus(ctx, stateDir, db)
	if statusErr != nil {
		return nil, statusErr
	}
	return &ServiceActionResult{
		Action:  "stop",
		Success: !updatedStatus.CurrentlyRunning,
		Status:  *updatedStatus,
		Message: "orbit user service stopped successfully",
	}, nil
}

// RestartService restarts the user service using systemctl --user restart.
func RestartService(ctx context.Context, stateDir string, db *repository.DB) (*ServiceActionResult, error) {
	st, err := CheckServiceStatus(ctx, stateDir, db)
	if err != nil {
		return nil, err
	}
	if !st.SystemdAvailable {
		return &ServiceActionResult{
			Action:  "restart",
			Success: false,
			Status:  *st,
			Message: "systemd user service unavailable; restart manually: " + st.ManualCommand,
		}, &ControlError{
			Code:      "SYSTEMD_UNAVAILABLE",
			Message:   "systemd user session is unavailable in this environment",
			Retryable: false,
			Action:    "restart background process manually: " + st.ManualCommand,
		}
	}

	if err := validateSelectedService(stateDir); err != nil {
		return nil, err
	}

	systemctlPath, _ := exec.LookPath("systemctl")
	if st.CurrentlyRunning && !serviceOwnsState(ctx, systemctlPath, stateDir) {
		return nil, manualDaemonError()
	}
	cmd := exec.CommandContext(ctx, systemctlPath, "--user", "restart", "orbit.service")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("systemctl --user restart: %w (%s)", err, stderr.String())
	}

	owned := waitServiceOwnsState(ctx, systemctlPath, stateDir)
	updatedStatus, statusErr := CheckServiceStatus(ctx, stateDir, db)
	if statusErr != nil {
		return nil, statusErr
	}
	return &ServiceActionResult{
		Action:  "restart",
		Success: owned && updatedStatus.CurrentlyRunning,
		Status:  *updatedStatus,
		Message: "orbit user service restarted successfully",
	}, nil
}

// A daemon launched outside systemd keeps the state lock, so the unit would
// fail and restart in a loop while status still reported a running daemon.
// Success requires the unit's main process to be the recorded lock owner.
func manualDaemonError() error {
	return &ControlError{Code: "MANUAL_DAEMON_RUNNING", Message: "a daemon started outside the user service owns this state", Action: "run 'orbit stop', then 'orbit service start'"}
}

func serviceOwnsState(ctx context.Context, systemctlPath, stateDir string) bool {
	if stateDir == "" {
		stateDir = config.DefaultStateDir()
	}
	out, err := exec.CommandContext(ctx, systemctlPath, "--user", "show", "-p", "MainPID", "--value", "orbit.service").Output()
	if err != nil {
		return false
	}
	data, err := state.ReadPrivate(stateDir, ".agent.pid", 64)
	if err != nil {
		return false
	}
	return stateOwnedByUnit(string(out), string(data))
}

func stateOwnedByUnit(mainPID, agentPID string) bool {
	unit, err := strconv.Atoi(strings.TrimSpace(mainPID))
	if err != nil || unit <= 1 {
		return false
	}
	owner, err := strconv.Atoi(strings.TrimSpace(agentPID))
	return err == nil && owner == unit
}

// The started unit records its PID only after acquiring the state lock.
func waitServiceOwnsState(ctx context.Context, systemctlPath, stateDir string) bool {
	deadline := time.Now().Add(15 * time.Second)
	for {
		if serviceOwnsState(ctx, systemctlPath, stateDir) {
			return true
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// InstallUserUnit writes ~/.config/systemd/user/orbit.service configured for this binary.
func InstallUserUnit(stateDir, binPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get user home dir: %w", err)
	}
	if binPath == "" {
		if execPath, err := os.Executable(); err == nil {
			binPath = execPath
		} else {
			binPath = "/usr/bin/orbit"
		}
	}
	if stateDir == "" {
		stateDir = config.DefaultStateDir()
	}

	userDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		return fmt.Errorf("create user systemd directory: %w", err)
	}

	content := fmt.Sprintf(`[Unit]
Description=Orbit Background Engine
Documentation=https://github.com/calebhabesh/orbit
After=network.target

[Service]
Type=simple
ExecStart=%s serve --state=%s --control-listen=127.0.0.1:8080 --allow-init
ExecStop=%s stop --state=%s
Restart=on-failure
RestartSec=5s
TimeoutStopSec=30s

LimitNOFILE=65536
MemoryHigh=512M
MemoryMax=1G
NoNewPrivileges=yes

StandardOutput=journal
StandardError=journal
SyslogIdentifier=orbit

[Install]
WantedBy=default.target
`, binPath, stateDir, binPath, stateDir)

	targetFile := filepath.Join(userDir, "orbit.service")
	if strings.ContainsAny(stateDir+binPath, "\r\n\x00%\"\\") || strings.ContainsAny(stateDir+binPath, " \t") {
		return errors.New("service paths require plain absolute paths without whitespace or systemd specifiers")
	}
	f, err := os.OpenFile(targetFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// DisableService removes login enablement without stopping the selected daemon.
func DisableService(ctx context.Context, stateDir string, db *repository.DB) (*ServiceActionResult, error) {
	st, err := CheckServiceStatus(ctx, stateDir, db)
	if err != nil {
		return nil, err
	}
	if !st.SystemdAvailable {
		return nil, &ControlError{Code: "SYSTEMD_UNAVAILABLE", Message: "systemd user session unavailable", Action: "inspect login startup on the host"}
	}
	if err := validateSelectedService(stateDir); err != nil {
		return nil, err
	}

	path, err := exec.LookPath("systemctl")
	if err != nil {
		return nil, err
	}
	if out, err := exec.CommandContext(ctx, path, "--user", "disable", "orbit.service").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("disable service: %w (%s)", err, out)
	}
	updated, err := CheckServiceStatus(ctx, stateDir, db)
	if err != nil {
		return nil, err
	}
	return &ServiceActionResult{Action: "disable", Success: !updated.EnabledOnLogin, Status: *updated, Message: "login startup disabled"}, nil
}

// Refuse actions on a unit that belongs to another selected state. Existing
// personal units are never overwritten or stopped to configure another state.
func validateSelectedService(stateDir string) error {
	if stateDir == "" {
		stateDir = config.DefaultStateDir()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", "orbit.service"))
	if err != nil {
		return &ControlError{Code: "SERVICE_SELECTION_REQUIRED", Message: "selected state has no verified user unit", Action: "install a user unit for the selected state"}
	}
	// Packaged units name the state with systemd's %h home specifier.
	expected := filepath.Clean(stateDir)
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if value, ok := strings.CutPrefix(field, "--state="); ok {
				if rest, ok := strings.CutPrefix(value, "%h/"); ok {
					value = filepath.Join(home, rest)
				}
				if filepath.Clean(value) == expected {
					return nil
				}
			}
		}
	}
	return &ControlError{Code: "SERVICE_SELECTION_REQUIRED", Message: "existing service targets a different state", Action: "preserve the existing service and configure a separate selected unit"}
}
