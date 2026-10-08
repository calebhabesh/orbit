package control

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
		return StartService(ctx, stateDir, c.db, nil)
	case "stop":
		return StopService(ctx, stateDir, c.db, nil)
	case "restart":
		return RestartService(ctx, stateDir, c.db, nil)
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
		ManualCommand: fmt.Sprintf("orbit serve --state=%s --control-listen=127.0.0.1:0", stateDir),
	}

	// 1. Check systemctl availability and user bus reachability
	systemctlPath, err := exec.LookPath("systemctl")
	if err != nil {
		res.SystemdAvailable = false
		res.StatusDetail = "systemctl command not found in PATH"
	} else if !userManagerReachable(ctx, systemctlPath) {
		res.SystemdAvailable = false
		res.StatusDetail = "systemd user session bus unavailable"
	} else {
		res.SystemdAvailable = true
	}

	// 2. Unit file existence
	if _, err := effectiveUnit(); err == nil {
		res.UnitInstalled = true
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
	// 4. Currently running, and who runs it
	if db != nil {
		res.CurrentlyRunning = true
	} else {
		testLock, err := state.Acquire(stateDir)
		if errors.Is(err, state.ErrLocked) {
			res.CurrentlyRunning = true
		} else if err == nil {
			_ = testLock.Close()
		}
	}
	if res.CurrentlyRunning {
		res.Owner = OwnerManual
		if res.SystemdAvailable && serviceOwnsState(ctx, systemctlPath, stateDir) {
			res.Owner = OwnerService
		} else if b, e := state.ReadPrivate(stateDir, originFile, 64); e == nil && strings.TrimSpace(string(b)) == OwnerTerminal {
			res.Owner = OwnerTerminal
		}
	}
	if res.SystemdAvailable && res.UnitInstalled {
		res.UnitState = unitActiveState(ctx, systemctlPath)
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

	// 7. Lingering status. Orbit only shows the command; it never runs it.
	if username := currentUsername(); username != "" {
		res.LingeringInstruction = "sudo loginctl enable-linger " + username
		res.LingeringEnabled = lingeringEnabled(ctx, username)
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
	migrateLegacyUserUnit(ctx, systemctlPath)
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

// DaemonStopper gracefully stops a daemon that runs outside the user unit so
// the unit can take over the state (F14). Clients pass app.StopAgent; a nil
// stopper keeps the refusal, since a daemon cannot hand over its own state.
type DaemonStopper func(stateDir string) error

// systemdAction checks availability and selection before a unit action.
func systemdAction(ctx context.Context, action, stateDir string, db *repository.DB) (*ServiceStatusResult, string, error) {
	st, err := CheckServiceStatus(ctx, stateDir, db)
	if err != nil {
		return nil, "", err
	}
	if !st.SystemdAvailable {
		return st, "", &ControlError{
			Code:      "SYSTEMD_UNAVAILABLE",
			Message:   "systemd user session is unavailable in this environment",
			Retryable: false,
			Action:    "Orbit runs while a terminal starts it; to run it yourself: " + st.ManualCommand,
		}
	}
	if err := validateSelectedService(stateDir); err != nil {
		return st, "", err
	}
	systemctlPath, err := exec.LookPath("systemctl")
	if err != nil {
		return st, "", err
	}
	return st, systemctlPath, nil
}

// handOver stops a daemon that the unit does not own, so the unit can start.
// It returns the previous owner ("" when nothing needed stopping).
func handOver(ctx context.Context, st *ServiceStatusResult, stateDir string, stop DaemonStopper) (string, error) {
	if !st.CurrentlyRunning || st.Owner == OwnerService {
		return "", nil
	}
	if stop == nil {
		return "", manualDaemonError()
	}
	if err := stop(stateDir); err != nil {
		return "", &ControlError{Code: "HANDOVER_FAILED", Message: "could not stop the " + st.Owner + " daemon: " + err.Error(), Action: "run 'orbit stop', then 'orbit service start'"}
	}
	return st.Owner, nil
}

func handOverMessage(base, previous string) string {
	if previous == "" {
		return base
	}
	return base + "; took over from the " + previous + " daemon"
}

// ServiceCanStart reports whether a user manager is reachable and the effective
// orbit.service serves stateDir, so a launcher may start the daemon through it.
func ServiceCanStart(ctx context.Context, stateDir string) bool {
	systemctl, err := exec.LookPath("systemctl")
	return err == nil && userManagerReachable(ctx, systemctl) && validateSelectedService(stateDir) == nil
}

// StartServiceUnit starts orbit.service for a launcher and reports whether the
// unit took ownership of stateDir. It never hands over: the caller holds the
// launch lock and has verified no daemon runs. On failure the unit is stopped
// so its restart policy cannot fight the caller's fallback daemon.
func StartServiceUnit(ctx context.Context, stateDir string) bool {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return false
	}
	migrateLegacyUserUnit(ctx, systemctl)
	startCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if exec.CommandContext(startCtx, systemctl, "--user", "start", "orbit.service").Run() == nil {
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) && ctx.Err() == nil {
			if serviceOwnsState(ctx, systemctl, stateDir) {
				return true
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()
	_ = exec.CommandContext(stopCtx, systemctl, "--user", "stop", "orbit.service").Run()
	return false
}

// StartService activates the user unit with systemctl --user start. A daemon
// started outside the unit is first stopped through stop (handover).
func StartService(ctx context.Context, stateDir string, db *repository.DB, stop DaemonStopper) (*ServiceActionResult, error) {
	st, systemctlPath, err := systemdAction(ctx, "start", stateDir, db)
	if err != nil {
		var e *ControlError
		if st != nil && errors.As(err, &e) && e.Code == "SYSTEMD_UNAVAILABLE" {
			return &ServiceActionResult{Action: "start", Status: *st, Message: "systemd user service unavailable; run manual command instead: " + st.ManualCommand}, err
		}
		return nil, err
	}
	migrateLegacyUserUnit(ctx, systemctlPath)
	previous, err := handOver(ctx, st, stateDir, stop)
	if err != nil {
		return nil, err
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
		Message: handOverMessage("orbit user service started", previous),
	}, nil
}

// StopService stops the user unit and, through stop, a daemon running
// outside it, so "orbit service stop" leaves nothing running (F14).
func StopService(ctx context.Context, stateDir string, db *repository.DB, stop DaemonStopper) (*ServiceActionResult, error) {
	st, systemctlPath, err := systemdAction(ctx, "stop", stateDir, db)
	if err != nil {
		var e *ControlError
		if st != nil && errors.As(err, &e) && e.Code == "SYSTEMD_UNAVAILABLE" {
			if stop != nil && st.CurrentlyRunning {
				if stopErr := stop(stateDir); stopErr != nil {
					return nil, stopErr
				}
				updated, statusErr := CheckServiceStatus(ctx, stateDir, db)
				if statusErr != nil {
					return nil, statusErr
				}
				return &ServiceActionResult{Action: "stop", Success: !updated.CurrentlyRunning, Status: *updated, Message: "stopped the " + st.Owner + " daemon"}, nil
			}
			e.Action = "stop background process manually"
			return &ServiceActionResult{Action: "stop", Status: *st, Message: "systemd user service unavailable"}, err
		}
		return nil, err
	}
	cmd := exec.CommandContext(ctx, systemctlPath, "--user", "stop", "orbit.service")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("systemctl --user stop: %w (%s)", err, stderr.String())
	}
	message := "orbit user service stopped"
	if st.CurrentlyRunning && st.Owner != OwnerService && stop != nil {
		after, statusErr := CheckServiceStatus(ctx, stateDir, db)
		if statusErr != nil {
			return nil, statusErr
		}
		if after.CurrentlyRunning && after.Owner != OwnerService {
			if err := stop(stateDir); err != nil {
				return nil, err
			}
			message += "; stopped the " + after.Owner + " daemon"
		}
	}

	updatedStatus, statusErr := CheckServiceStatus(ctx, stateDir, db)
	if statusErr != nil {
		return nil, statusErr
	}
	return &ServiceActionResult{
		Action:  "stop",
		Success: !updatedStatus.CurrentlyRunning,
		Status:  *updatedStatus,
		Message: message,
	}, nil
}

// RestartService restarts the user unit, handing over from a daemon started
// outside it through stop.
func RestartService(ctx context.Context, stateDir string, db *repository.DB, stop DaemonStopper) (*ServiceActionResult, error) {
	st, systemctlPath, err := systemdAction(ctx, "restart", stateDir, db)
	if err != nil {
		var e *ControlError
		if st != nil && errors.As(err, &e) && e.Code == "SYSTEMD_UNAVAILABLE" {
			e.Action = "restart background process manually: " + st.ManualCommand
			return &ServiceActionResult{Action: "restart", Status: *st, Message: "systemd user service unavailable; restart manually: " + st.ManualCommand}, err
		}
		return nil, err
	}
	migrateLegacyUserUnit(ctx, systemctlPath)
	previous, err := handOver(ctx, st, stateDir, stop)
	if err != nil {
		return nil, err
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
		Message: handOverMessage("orbit user service restarted", previous),
	}, nil
}

// A daemon launched outside systemd keeps the state lock, so the unit would
// fail and restart in a loop while status still reported a running daemon.
// Success requires the unit's main process to be the recorded lock owner.
func manualDaemonError() error {
	return &ControlError{Code: "MANUAL_DAEMON_RUNNING", Message: "a daemon started outside the user service owns this state", Action: "run 'orbit service start' from a terminal to hand it over to the service"}
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

// userUnitContent is the unit "orbit service enable" installs. The control
// listener is ephemeral; clients read control.addr (F01).
func userUnitContent(binPath, stateDir, controlListen string) string {
	return fmt.Sprintf(`[Unit]
Description=Orbit Background Engine
Documentation=https://github.com/calebhabesh/orbit
After=network.target

[Service]
Type=simple
ExecStart=%s serve --state=%s --control-listen=%s --allow-init
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
`, binPath, stateDir, controlListen, binPath, stateDir)
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

	content := userUnitContent(binPath, stateDir, "127.0.0.1:0")
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

// migrateLegacyUserUnit rewrites a unit that earlier "orbit service enable"
// generated with the fixed 127.0.0.1:8080 control port. Only a byte-exact
// earlier Orbit-generated file is replaced; anything edited is preserved.
func migrateLegacyUserUnit(ctx context.Context, systemctlPath string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	path := filepath.Join(home, ".config", "systemd", "user", "orbit.service")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var bin, dir string
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "ExecStart="); ok {
			fields := strings.Fields(v)
			if len(fields) > 2 {
				bin, dir = fields[0], strings.TrimPrefix(fields[2], "--state=")
			}
		}
	}
	if bin == "" || string(data) != userUnitContent(bin, dir, "127.0.0.1:8080") {
		return false
	}
	if err := config.WritePrivate(filepath.Dir(path), "orbit.service", []byte(userUnitContent(bin, dir, "127.0.0.1:0"))); err != nil {
		return false
	}
	_ = os.Chmod(path, 0644)
	_ = exec.CommandContext(ctx, systemctlPath, "--user", "daemon-reload").Run()
	return true
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

// userUnitDirs are systemd's user unit search paths, highest precedence first.
func userUnitDirs() []string {
	dirs := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "systemd", "user"))
	}
	return append(dirs, "/etc/systemd/user", "/usr/local/lib/systemd/user", "/usr/lib/systemd/user")
}

// effectiveUnit returns the ExecStart line systemd would use for orbit.service:
// the highest-precedence unit file, then drop-ins in name order, where an empty
// ExecStart= resets the command (as the trial's control-port drop-ins do).
func effectiveUnit() (string, error) {
	var unit []byte
	for _, dir := range userUnitDirs() {
		b, err := os.ReadFile(filepath.Join(dir, "orbit.service"))
		if err == nil {
			unit = b
			break
		}
	}
	if unit == nil {
		return "", os.ErrNotExist
	}
	dropins := map[string]string{}
	for _, dir := range slices.Backward(userUnitDirs()) {
		matches, _ := filepath.Glob(filepath.Join(dir, "orbit.service.d", "*.conf"))
		for _, m := range matches {
			dropins[filepath.Base(m)] = m
		}
	}
	execStart := ""
	apply := func(b []byte) {
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), "ExecStart="); ok {
				execStart = strings.TrimSpace(v)
			}
		}
	}
	apply(unit)
	for _, name := range slices.Sorted(maps.Keys(dropins)) {
		if b, err := os.ReadFile(dropins[name]); err == nil {
			apply(b)
		}
	}
	return execStart, nil
}

// managerExecStart asks the user manager for orbit.service's ExecStart, or "".
func managerExecStart() string {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, systemctl, "--user", "show", "-p", "ExecStart", "--value", "orbit.service").Output()
	if err != nil {
		return ""
	}
	// With no unit loaded the manager prints an empty line, which is no command.
	return strings.TrimSpace(strings.NewReplacer(";", " ", "{", " ", "}", " ").Replace(string(out)))
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
	// The manager's own view is authoritative: it resolves %h and its config
	// home from the account, not from $HOME, so a disposable HOME can never
	// select the owner's real unit. Unit files are read only when the manager
	// reports no orbit command (no unit loaded, or a test stand-in).
	execStart := managerExecStart()
	if !strings.Contains(execStart, "--state=") {
		execStart, err = effectiveUnit()
		if err != nil {
			return &ControlError{Code: "SERVICE_SELECTION_REQUIRED", Message: "selected state has no verified user unit", Action: "install a user unit for the selected state"}
		}
	}
	// Packaged units name the state with systemd's %h home specifier.
	expected := filepath.Clean(stateDir)
	for _, field := range strings.Fields(execStart) {
		if value, ok := strings.CutPrefix(field, "--state="); ok {
			if rest, ok := strings.CutPrefix(value, "%h/"); ok {
				value = filepath.Join(home, rest)
			}
			if filepath.Clean(value) == expected {
				return nil
			}
		}
	}
	return &ControlError{Code: "SERVICE_SELECTION_REQUIRED", Message: "existing service targets a different state", Action: "preserve the existing service and configure a separate selected unit"}
}
