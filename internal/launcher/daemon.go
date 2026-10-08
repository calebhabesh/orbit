package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		starter := opts.DaemonStarter
		if starter == nil {
			starter = defaultDaemonStarter
		}
		address := opts.ControlAddress
		if address == "" {
			address = "127.0.0.1:0"
		}
		if err = starter(ctx, dir, address); err != nil {
			return nil, fmt.Errorf("start daemon: %w", err)
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
