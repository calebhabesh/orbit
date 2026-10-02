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
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/state"
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
		ManualCommand: fmt.Sprintf("filesync serve --state=%s --control-listen=127.0.0.1:8080", stateDir),
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

		if runErr != nil && (strings.Contains(outStr, "Failed to connect to bus") || strings.Contains(outStr, "bus connection refused") || strings.Contains(outStr, "No such file or directory")) {
			res.SystemdAvailable = false
			res.StatusDetail = "systemd user session bus unavailable"
		} else {
			res.SystemdAvailable = true
		}
	}

	// 2. Unit file existence
	var userUnitPath string
	if home, err := os.UserHomeDir(); err == nil {
		userUnitPath = filepath.Join(home, ".config", "systemd", "user", "filesync.service")
		if _, err := os.Stat(userUnitPath); err == nil {
			res.UnitInstalled = true
		}
	}
	if !res.UnitInstalled {
		for _, p := range []string{
			"/usr/lib/systemd/user/filesync.service",
			"/usr/local/lib/systemd/user/filesync.service",
			"/etc/systemd/user/filesync.service",
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
		cmd := exec.CommandContext(enabledCtx, systemctlPath, "--user", "is-enabled", "filesync.service")
		out, _ := cmd.CombinedOutput()
		if strings.TrimSpace(string(out)) == "enabled" {
			res.EnabledOnLogin = true
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
		if !res.CurrentlyRunning && res.SystemdAvailable {
			activeCtx, activeCancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer activeCancel()
			cmd := exec.CommandContext(activeCtx, systemctlPath, "--user", "is-active", "filesync.service")
			out, _ := cmd.CombinedOutput()
			if strings.TrimSpace(string(out)) == "active" {
				res.CurrentlyRunning = true
			}
		}
	}

	// 5. Root verified
	if db != nil {
		registered, err := db.RegisteredFolders(ctx)
		if err == nil && len(registered) > 0 {
			allValid := true
			for _, reg := range registered {
				info, statErr := os.Stat(reg.Path)
				if statErr != nil || !info.IsDir() {
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
			return nil, fmt.Errorf("install user unit: %w", err)
		}
	}

	systemctlPath, _ := exec.LookPath("systemctl")
	// Daemon-reload
	_ = exec.CommandContext(ctx, systemctlPath, "--user", "daemon-reload").Run()

	// Enable unit
	cmd := exec.CommandContext(ctx, systemctlPath, "--user", "enable", "filesync.service")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("systemctl --user enable: %w (%s)", err, stderr.String())
	}

	updatedStatus, _ := CheckServiceStatus(ctx, stateDir, db)
	return &ServiceActionResult{
		Action:  "enable",
		Success: true,
		Status:  *updatedStatus,
		Message: "filesync user service enabled successfully",
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

	systemctlPath, _ := exec.LookPath("systemctl")
	cmd := exec.CommandContext(ctx, systemctlPath, "--user", "start", "filesync.service")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("systemctl --user start: %w (%s)", err, stderr.String())
	}

	time.Sleep(300 * time.Millisecond)

	updatedStatus, _ := CheckServiceStatus(ctx, stateDir, db)
	return &ServiceActionResult{
		Action:  "start",
		Success: true,
		Status:  *updatedStatus,
		Message: "filesync user service started successfully",
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

	systemctlPath, _ := exec.LookPath("systemctl")
	cmd := exec.CommandContext(ctx, systemctlPath, "--user", "stop", "filesync.service")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("systemctl --user stop: %w (%s)", err, stderr.String())
	}

	updatedStatus, _ := CheckServiceStatus(ctx, stateDir, db)
	return &ServiceActionResult{
		Action:  "stop",
		Success: true,
		Status:  *updatedStatus,
		Message: "filesync user service stopped successfully",
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

	systemctlPath, _ := exec.LookPath("systemctl")
	cmd := exec.CommandContext(ctx, systemctlPath, "--user", "restart", "filesync.service")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("systemctl --user restart: %w (%s)", err, stderr.String())
	}

	updatedStatus, _ := CheckServiceStatus(ctx, stateDir, db)
	return &ServiceActionResult{
		Action:  "restart",
		Success: true,
		Status:  *updatedStatus,
		Message: "filesync user service restarted successfully",
	}, nil
}

// InstallUserUnit writes ~/.config/systemd/user/filesync.service configured for this binary.
func InstallUserUnit(stateDir, binPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get user home dir: %w", err)
	}
	if binPath == "" {
		if execPath, err := os.Executable(); err == nil {
			binPath = execPath
		} else {
			binPath = "/usr/bin/filesync"
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
Description=File Sync Background Engine
Documentation=https://github.com/calebhabesh/file-sync
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
SyslogIdentifier=filesync

[Install]
WantedBy=default.target
`, binPath, stateDir, binPath, stateDir)

	targetFile := filepath.Join(userDir, "filesync.service")
	return os.WriteFile(targetFile, []byte(content), 0o644)
}
