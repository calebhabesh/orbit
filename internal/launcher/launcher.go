package launcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/calebhabesh/orbit/internal/controlclient"
)

// DaemonStarter encapsulates starting the background daemon process or runner.
type DaemonStarter func(ctx context.Context, stateDir, ctrlAddr string) error

// LaunchOptions configures the Orbit desktop launcher invocation.
type LaunchOptions struct {
	StateDir       string
	ControlAddress string
	NoBrowser      bool
	BrowserOpener  func(url string) error
	DaemonStarter  DaemonStarter
	Stdout         io.Writer
	Stderr         io.Writer
}

// LaunchResult details the outcome of an Orbit launch sequence.
type LaunchResult struct {
	StateDir       string `json:"state_dir"`
	DaemonRunning  bool   `json:"daemon_running"`
	DaemonPID      int    `json:"daemon_pid,omitempty"`
	ControlAddress string `json:"control_address"`
	BootstrapURL   string `json:"bootstrap_url"`
	BootstrapToken string `json:"bootstrap_token,omitempty"`
	BrowserOpened  bool   `json:"browser_opened"`
	Message        string `json:"message"`
}

// Launch executes the Orbit launcher workflow: discovers state, verifies validity,
// reuses or starts the daemon safely, generates a 1-use bootstrap handoff, and opens the UI.
func Launch(ctx context.Context, opts LaunchOptions) (*LaunchResult, error) {
	// 1. Discover state directory
	stateDir, err := DiscoverState(opts.StateDir)
	if err != nil {
		return nil, err
	}

	ready, err := EnsureDaemon(ctx, opts)
	if err != nil {
		return nil, err
	}
	daemonRunning := true
	serverAddr := ready.ControlAddress
	httpAddr := "http://" + serverAddr
	// 6. Request single-use bootstrap token from daemon
	var bootstrapRes struct {
		BootstrapToken string `json:"bootstrap_token"`
		ExpiresInSecs  int    `json:"expires_in_secs"`
	}
	if err := (&controlclient.Client{StateDir: stateDir}).Call(ctx, http.MethodPost, "/api/v1/auth/bootstrap-token", nil, &bootstrapRes); err != nil {
		return nil, err
	}

	bootstrapURL := fmt.Sprintf("%s/#bootstrap=%s", httpAddr, bootstrapRes.BootstrapToken)

	// 7. Get PID if available
	var daemonPID int
	if pidBytes, err := os.ReadFile(filepath.Join(stateDir, ".agent.pid")); err == nil {
		daemonPID, _ = strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	}

	// 8. Open browser via desktop helper or output URL
	browserOpened := false
	if opts.NoBrowser {
		browserOpened = false
		if opts.Stdout != nil {
			fmt.Fprintf(opts.Stdout, "Orbit control interface ready:\n%s\n", bootstrapURL)
		}
	} else if opts.BrowserOpener != nil {
		if err := opts.BrowserOpener(bootstrapURL); err == nil {
			browserOpened = true
		} else if opts.Stdout != nil {
			fmt.Fprintf(opts.Stdout, "Orbit control interface ready:\n%s\n", bootstrapURL)
		}
	} else {
		hasDisplay := os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
		opener, err := exec.LookPath("xdg-open")
		if !hasDisplay || err != nil {
			browserOpened = false
			if opts.Stdout != nil {
				fmt.Fprintf(opts.Stdout, "Orbit control interface ready:\n%s\n", bootstrapURL)
			}
		} else {
			cmd := exec.Command(opener, bootstrapURL)
			if err := cmd.Start(); err != nil {
				browserOpened = false
				if opts.Stdout != nil {
					fmt.Fprintf(opts.Stdout, "Orbit control interface ready:\n%s\n", bootstrapURL)
				}
			} else {
				browserOpened = true
			}
		}
	}

	return &LaunchResult{
		StateDir:       stateDir,
		DaemonRunning:  daemonRunning,
		DaemonPID:      daemonPID,
		ControlAddress: serverAddr,
		BootstrapURL:   bootstrapURL,
		BootstrapToken: bootstrapRes.BootstrapToken,
		BrowserOpened:  browserOpened,
		Message:        "Orbit launched successfully",
	}, nil
}

// detachedLog keeps a terminal-started daemon's output (F14). The previous
// run's log is kept as daemon.log.1 so a crash stays inspectable once.
const detachedLog = "daemon.log"

func defaultDaemonStarter(_ context.Context, stateDir, ctrlAddr string) error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	logPath := filepath.Join(stateDir, detachedLog)
	_ = os.Rename(logPath, logPath+".1")
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_APPEND|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(bin, "serve", "--state="+stateDir, "--control-listen="+ctrlAddr, "--allow-init", "--started-by=terminal")
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func waitForDaemonReady(ctx context.Context, stateDir string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var result map[string]any
		err := (&controlclient.Client{StateDir: stateDir}).Call(ctx, http.MethodGet, "/api/v1/settings", nil, &result)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("daemon readiness: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
