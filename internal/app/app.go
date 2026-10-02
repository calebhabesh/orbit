package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/scheduler"
	"github.com/calebhabesh/file-sync/internal/state"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

// Dependencies makes identity creation deterministic in tests without weakening
// production randomness or introducing a global clock.
type Dependencies struct {
	Now    func() time.Time
	Random io.Reader
}

func SystemDependencies() Dependencies {
	return Dependencies{Now: time.Now, Random: rand.Reader}
}

func Initialize(ctx context.Context, stateDir string, deps Dependencies) (config.Config, error) {
	if err := state.EnsureDirectory(stateDir); err != nil {
		return config.Config{}, err
	}
	lock, err := state.Acquire(stateDir)
	if err != nil {
		return config.Config{}, err
	}
	defer lock.Close()

	cfg, err := config.Initialize(stateDir, deps.Now, deps.Random)
	if err != nil {
		return config.Config{}, err
	}
	if err := config.InitializeStorageLimits(stateDir); err != nil {
		return config.Config{}, err
	}
	deviceID, err := decodeDeviceID(cfg.DeviceID)
	if err != nil {
		return config.Config{}, err
	}
	if _, err := replication.LoadOrCreateIdentity(stateDir, deviceID, deps.Now()); err != nil {
		return config.Config{}, err
	}
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		return config.Config{}, err
	}
	if err := db.Close(); err != nil {
		return config.Config{}, fmt.Errorf("close metadata database: %w", err)
	}
	return cfg, nil
}

type ServeOptions struct {
	PeerAddress       string
	ControlAddress    string // loopback control listener, e.g. "127.0.0.1:8080"
	Ready             io.Writer
	Profile           string        // "laptop" or "pi"
	BandwidthLimitBps int64         // 0 = unlimited
	SyncInterval      time.Duration // default 5m
	FullScanInterval  time.Duration // default 24h
	NoWatch           bool
	ClientFactory     scheduler.ClientFactory
	AllowInitialize   bool // auto-initialize clean uninitialized state directory
}

func Serve(ctx context.Context, stateDir, peerAddress string, ready io.Writer) error {
	return ServeWithOptions(ctx, stateDir, ServeOptions{
		PeerAddress: peerAddress,
		Ready:       ready,
	})
}

func ServeWithOptions(ctx context.Context, stateDir string, opts ServeOptions) error {
	if err := state.ValidateDirectory(stateDir); err != nil {
		if opts.AllowInitialize && errors.Is(err, os.ErrNotExist) {
			if err := state.EnsureDirectory(stateDir); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	lock, err := state.Acquire(stateDir)
	if err != nil {
		return err
	}
	defer lock.Close()

	pidPath := filepath.Join(stateDir, ".agent.pid")
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
	defer os.Remove(pidPath)

	cfg, err := config.Load(stateDir)
	if err != nil {
		if opts.AllowInitialize && errors.Is(err, os.ErrNotExist) {
			cfg, err = config.Initialize(stateDir, time.Now, rand.Reader)
			if err != nil {
				return fmt.Errorf("auto-initialize state: %w", err)
			}
		} else {
			return err
		}
	}
	if err := control.VerifyRecoveryConsistency(stateDir); err != nil {
		return err
	}
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		return err
	}
	defer db.Close()
	deviceID, err := decodeDeviceID(cfg.DeviceID)
	if err != nil {
		return err
	}
	identity, err := replication.LoadOrCreateIdentity(stateDir, deviceID, time.Now())
	if err != nil {
		return err
	}

	ws := workspace.New(db, workspace.Options{})
	endpoints, err := config.LoadPeerEndpoints(stateDir)
	if err != nil {
		return fmt.Errorf("load peer endpoints: %w", err)
	}
	var targets []scheduler.PeerTarget
	clients := map[scheduler.PeerTarget]*replication.Client{}
	for _, endpoint := range endpoints {
		folder, err := decodeDeviceID(endpoint.Folder)
		if err != nil {
			return err
		}
		peer, err := decodeDeviceID(endpoint.Device)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(endpoint.Certificate)
		if err != nil {
			return fmt.Errorf("read peer certificate: %w", err)
		}
		certificate, err := replication.ParsePeerCertificate(data)
		if err != nil {
			return err
		}
		membership, err := db.Membership(ctx, folder)
		if err != nil {
			return err
		}
		pin := replication.PublicKeyPin(certificate)
		if err := db.AuthorizePeer(ctx, folder, peer, pin, membership.Revision, membership.Digest); err != nil {
			return fmt.Errorf("configured peer is not approved: %w", err)
		}
		client, err := replication.NewClient(endpoint.URL, identity, certificate, pin)
		if err != nil {
			return err
		}
		defer client.CloseIdleConnections()
		target := scheduler.PeerTarget{Folder: folder, Peer: peer}
		targets = append(targets, target)
		clients[target] = client
	}
	if opts.ClientFactory == nil && len(clients) > 0 {
		opts.ClientFactory = func(folder, peer history.ID) (replication.PeerClient, error) {
			client := clients[scheduler.PeerTarget{Folder: folder, Peer: peer}]
			if client == nil {
				return nil, errors.New("peer endpoint is not configured")
			}
			return client, nil
		}
	}

	profileType := scheduler.ProfileLaptop
	if opts.Profile == string(scheduler.ProfilePi) {
		profileType = scheduler.ProfilePi
	}
	prof := scheduler.GetProfile(profileType)
	if opts.SyncInterval > 0 {
		prof.ReconcileInterval = opts.SyncInterval
	}
	if opts.FullScanInterval > 0 {
		prof.FullScanInterval = opts.FullScanInterval
	}

	var limiter *scheduler.BandwidthLimiter
	if opts.BandwidthLimitBps > 0 {
		limiter = scheduler.NewBandwidthLimiter(opts.BandwidthLimitBps)
	}

	sched, err := scheduler.NewScheduler(db, ws, scheduler.SchedulerOptions{
		Profile:       prof,
		Limiter:       limiter,
		NoWatch:       opts.NoWatch,
		ClientFactory: opts.ClientFactory,
		LocalDevice:   deviceID,
		Peers:         targets,
	})
	if err != nil {
		return fmt.Errorf("create scheduler: %w", err)
	}

	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}
	defer sched.Stop()

	var ctrlServer *control.Server
	var ctrlListener net.Listener
	var ctrlAddr string
	if opts.ControlAddress != "" {
		ctrlListener, err = net.Listen("tcp", opts.ControlAddress)
		if err != nil {
			return fmt.Errorf("listen for control: %w", err)
		}
		defer ctrlListener.Close()
		ctrlAddr = ctrlListener.Addr().String()

		ctrl := control.New(db, ws, control.Options{LocalDevice: deviceID})
		ctrlServer, err = control.NewServer(ctrl, stateDir)
		if err != nil {
			return fmt.Errorf("create control server: %w", err)
		}

		addrFile := filepath.Join(stateDir, "control.addr")
		_ = os.WriteFile(addrFile, []byte(ctrlAddr+"\n"), 0o600)
		defer os.Remove(addrFile)

		go func() {
			_ = ctrlServer.Serve(ctrlListener)
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = ctrlServer.Shutdown(shutdownCtx)
		}()
	}

	ctrlStatus := ""
	if ctrlAddr != "" {
		ctrlStatus = fmt.Sprintf(" control-listener=http://%s", ctrlAddr)
	}

	if opts.PeerAddress == "" {
		if opts.Ready != nil {
			fmt.Fprintf(opts.Ready, "agent ready: device=%s schema=%d peer-listener=disabled%s\n", cfg.DeviceID, repository.CurrentSchema, ctrlStatus)
		}
		<-ctx.Done()
		return nil
	}
	listener, err := net.Listen("tcp", opts.PeerAddress)
	if err != nil {
		return fmt.Errorf("listen for peers: %w", err)
	}
	defer listener.Close()
	if opts.Ready != nil {
		fmt.Fprintf(opts.Ready, "agent ready: device=%s schema=%d peer-listener=%s key-pin=%x%s\n", cfg.DeviceID, repository.CurrentSchema, listener.Addr(), identity.KeyPin, ctrlStatus)
	}
	return replication.NewServer(db, identity).Serve(ctx, listener)
}

// WithWorkspace gives local CLI operations the same locked repository/workspace
// boundary used by the agent. The callback must not retain either handle.
func WithWorkspace(ctx context.Context, stateDir string, run func(config.Config, *repository.DB, *workspace.Workspace) error) error {
	if err := state.ValidateDirectory(stateDir); err != nil {
		return err
	}
	lock, err := state.Acquire(stateDir)
	if err != nil {
		return err
	}
	defer lock.Close()
	cfg, err := config.Load(stateDir)
	if err != nil {
		return err
	}
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		return err
	}
	defer db.Close()
	return run(cfg, db, workspace.New(db, workspace.Options{}))
}

func decodeDeviceID(text string) (history.ID, error) {
	var id history.ID
	raw, err := hex.DecodeString(text)
	if err != nil || len(raw) != len(id) {
		return id, errors.New("configured device ID is invalid")
	}
	copy(id[:], raw)
	return id, nil
}

// StopAgent sends SIGTERM to the running agent identified in stateDir and waits for it to exit.
func StopAgent(stateDir string, timeout time.Duration) error {
	if err := state.ValidateDirectory(stateDir); err != nil {
		return err
	}
	pidPath := filepath.Join(stateDir, ".agent.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		// Check if lock is held anyway
		testLock, lockErr := state.Acquire(stateDir)
		if lockErr == nil {
			_ = testLock.Close()
			return errors.New("agent is not running")
		}
		return fmt.Errorf("read agent pid file %s: %w", pidPath, err)
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid pid %q in %s: %w", pidStr, pidPath, err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal process %d: %w", pid, err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		testLock, lockErr := state.Acquire(stateDir)
		if lockErr == nil {
			_ = testLock.Close()
			_ = os.Remove(pidPath)
			return nil
		}
	}

	return fmt.Errorf("timed out after %v waiting for agent (pid %d) to stop", timeout, pid)
}
