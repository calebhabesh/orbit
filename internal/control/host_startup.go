package control

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

// Daemon owners reported separately from the configured startup mode (F04).
const (
	OwnerService  = "service"  // the user unit's main process holds the state lock
	OwnerTerminal = "terminal" // the launcher started a detached daemon
	OwnerManual   = "manual"   // started some other way, such as orbit serve
)

// originFile records who started the daemon that holds the state lock; the
// service claim itself is always verified against the unit's main PID.
const originFile = ".agent.origin"

// unitActiveState reports systemd's ActiveState for orbit.service (active,
// activating, failed, inactive …), or "" when it cannot be read.
func unitActiveState(ctx context.Context, systemctl string) string {
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	out, _ := exec.CommandContext(ctx, systemctl, "--user", "is-active", "orbit.service").Output()
	s := strings.TrimSpace(string(out))
	switch s {
	case "active", "activating", "deactivating", "reloading", "failed", "inactive":
		return s
	}
	return ""
}

// ProbeHostStartup decides the advisory startup default (EG3). A host is a
// desktop when its default boot target is graphical or a graphical session is
// active for this user; otherwise it is headless. Headless hosts default to
// unattended only when lingering is already on; Orbit never enables lingering
// or runs sudo, it only shows the command. Without a user manager the only
// possible startup is manual.
func ProbeHostStartup(ctx context.Context) tc.HostStartup {
	h := tc.HostStartup{Class: "unknown", Suggested: "manual"}
	username := currentUsername()
	if username != "" {
		h.LingerCommand = "sudo loginctl enable-linger " + username
		h.Lingering = lingeringEnabled(ctx, username)
	}
	systemctl, err := exec.LookPath("systemctl")
	if err != nil || !userManagerReachable(ctx, systemctl) {
		h.Note = "No user service manager is reachable; Orbit runs while a terminal starts it."
		return h
	}
	h.Systemd = true
	run := func(args ...string) string {
		c, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
		out, _ := exec.CommandContext(c, systemctl, args...).Output()
		return strings.TrimSpace(string(out))
	}
	if run("get-default") == "graphical.target" || run("--user", "is-active", "graphical-session.target") == "active" {
		h.Class = "desktop"
		h.Suggested = "login"
		return h
	}
	h.Class = "headless"
	if h.Lingering {
		h.Suggested = "unattended"
		return h
	}
	h.Suggested = "login"
	h.Note = "Unattended startup keeps Orbit running after you log out. It needs lingering: run " + h.LingerCommand + ", then re-check. Until then startup is login."
	return h
}

func currentUsername() string {
	if name := os.Getenv("USER"); name != "" {
		return name
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

func userManagerReachable(ctx context.Context, systemctl string) bool {
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, systemctl, "--user", "is-system-running").CombinedOutput()
	s := strings.TrimSpace(string(out))
	return !(err != nil && (ctx.Err() != nil || strings.Contains(s, "Failed to connect to bus") || strings.Contains(s, "bus connection refused") || strings.Contains(s, "No such file or directory")))
}

func lingeringEnabled(ctx context.Context, username string) bool {
	if _, err := os.Stat("/var/lib/systemd/linger/" + username); err == nil {
		return true
	}
	loginctl, err := exec.LookPath("loginctl")
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, loginctl, "show-user", username, "-p", "Linger").CombinedOutput()
	return err == nil && strings.Contains(string(out), "Linger=yes")
}

// HostStartupFor applies the host proposal to one selected state. Login or
// unattended startup is proposed only when that state can use the user unit:
// the effective unit serves it, or no unit exists and it is the default state
// (enable then installs one). A unit serving another state is never replaced,
// so such a state keeps manual startup and says why.
func HostStartupFor(ctx context.Context, stateDir string) tc.HostStartup {
	h := ProbeHostStartup(ctx)
	if h.Suggested == "manual" {
		return h
	}
	if validateSelectedService(stateDir) == nil {
		return h
	}
	if _, err := effectiveUnit(); errors.Is(err, os.ErrNotExist) && managerExecStart() == "" && filepath.Clean(stateDir) == filepath.Clean(config.DefaultStateDir()) {
		return h
	}
	h.Suggested = "manual"
	h.Note = "The Orbit user service on this account serves a different state directory, so this one starts manually."
	return h
}
