package launcher

import (
	"context"
	"encoding/json"
	"errors"
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

	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/state"
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

	// 2. Validate state directory integrity
	if err := ValidateExistingState(stateDir); err != nil {
		return nil, err
	}

	// 3. Detect if daemon is already running via exclusive lock check
	daemonRunning := false
	if _, statErr := os.Stat(stateDir); statErr == nil {
		testLock, err := state.Acquire(stateDir)
		if errors.Is(err, state.ErrLocked) {
			daemonRunning = true
		} else if err == nil {
			_ = testLock.Close()
		} else {
			return nil, fmt.Errorf("check state lock in %s: %w", stateDir, err)
		}
	}

	// 4. Start daemon if not running
	ctrlAddr := opts.ControlAddress
	if ctrlAddr == "" {
		ctrlAddr = "127.0.0.1:0"
	}

	if !daemonRunning {
		starter := opts.DaemonStarter
		if starter == nil {
			starter = defaultDaemonStarter
		}
		if err := starter(ctx, stateDir, ctrlAddr); err != nil {
			return nil, fmt.Errorf("start daemon: %w", err)
		}

		// Wait for daemon to become responsive
		if err := waitForDaemonReady(ctx, stateDir, 5*time.Second); err != nil {
			return nil, fmt.Errorf("daemon failed to become ready: %w", err)
		}
		daemonRunning = true
	}

	// 5. Read control address and control token
	addrFile := filepath.Join(stateDir, "control.addr")
	addrBytes, err := os.ReadFile(addrFile)
	if err != nil {
		return nil, fmt.Errorf("read control.addr: %w", err)
	}
	serverAddr := strings.TrimSpace(string(addrBytes))
	httpAddr := serverAddr
	if !strings.HasPrefix(httpAddr, "http://") && !strings.HasPrefix(httpAddr, "https://") {
		httpAddr = "http://" + httpAddr
	}

	tokenFile := filepath.Join(stateDir, "control.token")
	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("read control.token: %w", err)
	}
	cliToken := strings.TrimSpace(string(tokenBytes))

	// 6. Request single-use bootstrap token from daemon
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, httpAddr+"/api/v1/auth/bootstrap-token", nil)
	if err != nil {
		return nil, fmt.Errorf("prepare bootstrap request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cliToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to control server at %s: %w", httpAddr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var ctrlErr control.ControlError
		if err := json.NewDecoder(resp.Body).Decode(&ctrlErr); err == nil && ctrlErr.Code != "" {
			return nil, fmt.Errorf("control server error: %s - %s", ctrlErr.Code, ctrlErr.Message)
		}
		return nil, fmt.Errorf("control server returned HTTP %d", resp.StatusCode)
	}

	var bootstrapRes struct {
		BootstrapToken string `json:"bootstrap_token"`
		ExpiresInSecs  int    `json:"expires_in_secs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bootstrapRes); err != nil {
		return nil, fmt.Errorf("decode bootstrap token response: %w", err)
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

func defaultDaemonStarter(_ context.Context, stateDir, ctrlAddr string) error {
	bin, err := os.Executable()
	if err != nil {
		bin = "filesync"
	}
	cmd := exec.Command(bin, "serve", "--state="+stateDir, "--control-listen="+ctrlAddr, "--allow-init")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

func waitForDaemonReady(ctx context.Context, stateDir string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addrFile := filepath.Join(stateDir, "control.addr")
	client := &http.Client{Timeout: 500 * time.Millisecond}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if data, err := os.ReadFile(addrFile); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			addr := strings.TrimSpace(string(data))
			if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
				addr = "http://" + addr
			}
			resp, err := client.Get(addr + "/api/v1/health")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("timeout waiting for control listener readiness")
}
