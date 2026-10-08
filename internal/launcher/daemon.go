package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/state"
	"golang.org/x/sys/unix"
)

// EnsureDaemon serializes launch attempts independently of daemon ownership.
// It never opens the database while another owner holds the agent lock.
func EnsureDaemon(ctx context.Context, opts LaunchOptions) (*LaunchResult, error) {
	dir, err := DiscoverState(opts.StateDir)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		if err := state.EnsureDirectory(dir); err != nil {
			return nil, err
		}
	}
	if err := state.ValidateDirectory(dir); err != nil {
		return nil, err
	}
	fd, err := unix.Open(filepath.Join(dir, ".launch.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	for {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	ownership, err := state.Acquire(dir)
	if err == nil {
		// Validate only under exclusive ownership; live discovery never probes SQLite.
		err = ValidateExistingState(dir)
		_ = ownership.Close()
		if err != nil {
			return nil, err
		}
		address := opts.ControlAddress
		if address == "" {
			address = "127.0.0.1:0"
		}
		starter := opts.DaemonStarter
		started := false
		if starter == nil {
			// One daemon owner (F04/F14): prefer the selected state's user unit so
			// output reaches the journal and orbit service controls it. Otherwise
			// the detached daemon logs to daemon.log and status names it terminal.
			started, err = startThroughService(ctx, dir, opts.ControlAddress)
			if err != nil {
				return nil, err
			}
			starter = defaultDaemonStarter
		}
		if !started {
			if err = starter(ctx, dir, address); err != nil {
				return nil, fmt.Errorf("start daemon: %w", err)
			}
		}
	} else if !errors.Is(err, state.ErrLocked) {
		return nil, err
	}
	if err = waitForDaemonReady(ctx, dir, 5*time.Second); err != nil {
		return nil, err
	}
	address, err := controlclient.PrivateFile(dir, "control.addr", 4096)
	if err != nil {
		return nil, err
	}
	return &LaunchResult{StateDir: dir, DaemonRunning: true, ControlAddress: strings.TrimSpace(string(address))}, nil
}

// startThroughService starts orbit.service when a user manager is reachable
// and the effective unit serves this state. A unit that does not take
// ownership is stopped again so it cannot crash-loop against the detached
// fallback daemon; the caller then starts that fallback.
func startThroughService(ctx context.Context, dir, explicitAddress string) (bool, error) {
	if explicitAddress != "" && explicitAddress != "127.0.0.1:0" {
		return false, nil
	}
	if !control.ServiceCanStart(ctx, dir) {
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); errors.Is(err, os.ErrNotExist) {
		// Packaged units serve without --allow-init; create the identity here,
		// under the launch lock, exactly as a detached --allow-init daemon would.
		if _, err := app.Initialize(ctx, dir, app.SystemDependencies()); err != nil {
			return false, err
		}
	}
	return control.StartServiceUnit(ctx, dir), nil
}
