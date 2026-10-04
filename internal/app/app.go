package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	cfg, err := initializeLocked(stateDir, deps)
	if err != nil {
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

// initializeLocked is shared by init and serve (launcher/service) under ownership.
// Limits precede the identity: retries after interruption never replace a key.
func initializeLocked(dir string, deps Dependencies) (config.Config, error) {
	if _, err := config.Load(dir); errors.Is(err, os.ErrNotExist) {
		for _, name := range []string{"metadata.sqlite", "identity/peer-identity.pem", "peer-identity.pem"} {
			if _, e := os.Lstat(filepath.Join(dir, name)); e == nil {
				return config.Config{}, errors.New("identity configuration missing from existing state; use recovery, not initialization")
			} else if !errors.Is(e, os.ErrNotExist) {
				return config.Config{}, e
			}
		}
		if err := config.InitializeStorageLimits(dir); err != nil {
			return config.Config{}, err
		}
	} else if err != nil {
		return config.Config{}, err
	}
	return config.Initialize(dir, deps.Now, deps.Random)
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
	if err := config.WritePrivate(stateDir, ".agent.pid", []byte(strconv.Itoa(os.Getpid())+"\n")); err != nil {
		return err
	}
	defer os.Remove(pidPath)
	var instance [32]byte
	if _, err := rand.Read(instance[:]); err != nil {
		return err
	}
	if err := config.WritePrivate(stateDir, ".agent.instance", []byte(hex.EncodeToString(instance[:]))); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(stateDir, ".agent.instance"))

	cfg, err := config.Load(stateDir)
	if err != nil {
		if opts.AllowInitialize && errors.Is(err, os.ErrNotExist) {
			cfg, err = initializeLocked(stateDir, SystemDependencies())
			if err != nil {
				return fmt.Errorf("auto-initialize state: %w", err)
			}
		} else {
			return err
		}
	}
	// Existing missing budgets are preserved for explicit settings review.
	if _, err := config.LoadStorageLimits(stateDir); err != nil {
		return err
	}
	runtimeSettings, err := config.LoadRuntimeSettings(stateDir)
	if err != nil {
		return err
	}
	if opts.ControlAddress != "" {
		host, _, e := net.SplitHostPort(opts.ControlAddress)
		if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return errors.New("owner control requires numeric loopback listener")
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
	ctrl := control.New(db, ws, control.Options{LocalDevice: deviceID})
	if err := ctrl.RecoverTerminalOperations(ctx); err != nil {
		return err
	}
	// Recovery may have installed desired budgets/listeners after interruption.
	if err := db.Close(); err != nil {
		return err
	}
	db, err = repository.Open(ctx, stateDir)
	if err != nil {
		return err
	}
	defer db.Close()
	ws = workspace.New(db, workspace.Options{})
	ctrl = control.New(db, ws, control.Options{LocalDevice: deviceID})
	runtimeSettings, err = config.LoadRuntimeSettings(stateDir)
	if err != nil {
		return err
	}
	if opts.PeerAddress == "" {
		opts.PeerAddress = runtimeSettings.PeerListen
	}
	registered, err := db.RegisteredFolders(ctx)
	if err != nil {
		return err
	}
	for _, reg := range registered {
		if err := ws.Recover(ctx, reg.Folder); err != nil {
			// Unavailable/ambiguous roots are paused by their owning operations.
			if opts.Ready != nil {
				fmt.Fprintf(opts.Ready, "recovery blocked: folder=%s error=%v\n", reg.Folder, err)
			}
		}
	}
	peerTargets := func() ([]scheduler.PeerTarget, error) {
		endpoints, err := config.LoadPeerEndpoints(stateDir)
		if err != nil {
			return nil, err
		}
		var targets []scheduler.PeerTarget
		for _, endpoint := range endpoints {
			folder, err := decodeDeviceID(endpoint.Folder)
			if err != nil {
				return nil, err
			}
			peer, err := decodeDeviceID(endpoint.Device)
			if err != nil {
				return nil, err
			}
			targets = append(targets, scheduler.PeerTarget{Folder: folder, Peer: peer})
		}
		return targets, nil
	}
	targets, err := peerTargets()
	if err != nil {
		return err
	}
	if opts.ClientFactory == nil {
		opts.ClientFactory = func(folder, peer history.ID) (replication.PeerClient, error) {
			endpoints, err := config.LoadPeerEndpoints(stateDir)
			if err != nil {
				return nil, err
			}
			for _, endpoint := range endpoints {
				if endpoint.Folder != hex.EncodeToString(folder[:]) || endpoint.Device != hex.EncodeToString(peer[:]) {
					continue
				}
				data, err := os.ReadFile(endpoint.Certificate)
				if err != nil {
					return nil, err
				}
				cert, err := replication.ParsePeerCertificate(data)
				if err != nil {
					return nil, err
				}
				pin := replication.PublicKeyPin(cert)
				membership, err := db.Membership(ctx, folder)
				if err != nil {
					return nil, err
				}
				if err = db.AuthorizePeer(ctx, folder, peer, pin, membership.Revision, membership.Digest); err != nil {
					return nil, err
				}
				return replication.NewClient(endpoint.URL, identity, cert, pin)
			}
			return nil, errors.New("peer endpoint is not configured")
		}
	}

	profileType := scheduler.ProfileLaptop
	if opts.Profile == string(scheduler.ProfilePi) {
		profileType = scheduler.ProfilePi
	}
	prof := scheduler.GetProfile(profileType)
	if _, err := os.Stat(filepath.Join(stateDir, "runtime.json")); err == nil {
		prof.TransferWorkers = int(runtimeSettings.Concurrency)
		if opts.BandwidthLimitBps == 0 {
			opts.BandwidthLimitBps = int64(runtimeSettings.BandwidthBytesPerSecond)
		}
	}
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
		PeerTargets:   peerTargets,
	})
	if err != nil {
		return fmt.Errorf("create scheduler: %w", err)
	}

	var peerListener net.Listener
	if opts.PeerAddress != "" {
		peerListener, err = net.Listen("tcp", opts.PeerAddress)
		if err != nil {
			return fmt.Errorf("listen for peers: %w", err)
		}
		defer peerListener.Close()
		if err := config.WritePrivate(stateDir, "peer.addr", []byte(peerListener.Addr().String()+"\n")); err != nil {
			return err
		}
		defer os.Remove(filepath.Join(stateDir, "peer.addr"))
	}
	var enrollmentListener net.Listener
	if runtimeSettings.EnrollmentListen != "" {
		enrollmentListener, err = net.Listen("tcp", runtimeSettings.EnrollmentListen)
		if err != nil {
			return fmt.Errorf("listen for enrollment: %w", err)
		}
		defer enrollmentListener.Close()
		if err := config.WritePrivate(stateDir, "enrollment.addr", []byte(enrollmentListener.Addr().String()+"\n")); err != nil {
			return err
		}
		defer os.Remove(filepath.Join(stateDir, "enrollment.addr"))
	}
	setupCtx, setupCancel := context.WithCancel(ctx)
	setupDone := make(chan struct{})
	go func() {
		defer close(setupDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-setupCtx.Done():
				return
			case <-ticker.C:
				slice, cancel := context.WithTimeout(setupCtx, 8*time.Second)
				_ = ctrl.ResumeSetupJobs(slice)
				cancel()
			}
		}
	}()
	defer func() { setupCancel(); <-setupDone }()
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

		ctrlServer, err = control.NewServer(ctrl, stateDir)
		if err != nil {
			return fmt.Errorf("create control server: %w", err)
		}

		addrFile := filepath.Join(stateDir, "control.addr")
		if err := config.WritePrivate(stateDir, "control.addr", []byte(ctrlAddr+"\n")); err != nil {
			return err
		}
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

	networkCtx, networkCancel := context.WithCancel(ctx)
	defer networkCancel()
	networkDone := make(chan error, 2)
	count := 0
	if enrollmentListener != nil {
		count++
		go func() {
			networkDone <- replication.NewEnrollmentServer(db, identity).Serve(networkCtx, enrollmentListener)
		}()
	}
	if peerListener != nil {
		count++
		go func() { networkDone <- replication.NewServer(db, identity).Serve(networkCtx, peerListener) }()
	}
	if opts.Ready != nil {
		peerAddress := "disabled"
		if peerListener != nil {
			peerAddress = peerListener.Addr().String()
		}
		enrollmentAddress := "disabled"
		if enrollmentListener != nil {
			enrollmentAddress = enrollmentListener.Addr().String()
		}
		fmt.Fprintf(opts.Ready, "agent ready: device=%s schema=%d peer-listener=%s key-pin=%x enrollment-listener=%s%s\n", cfg.DeviceID, repository.CurrentSchema, peerAddress, identity.KeyPin, enrollmentAddress, ctrlStatus)
	}
	if count == 0 {
		<-ctx.Done()
		return nil
	}
	first := <-networkDone
	networkCancel()
	for i := 1; i < count; i++ {
		if err := <-networkDone; first == nil {
			first = err
		}
	}
	return first
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
	ownership, lockErr := state.Acquire(stateDir)
	if lockErr == nil {
		ownership.Close()
		return errors.New("agent is not running")
	}
	if !errors.Is(lockErr, state.ErrLocked) {
		return lockErr
	}
	pidPath := filepath.Join(stateDir, ".agent.pid")
	data, err := state.ReadPrivate(stateDir, ".agent.pid", 64)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 1 {
		return errors.New("invalid daemon PID")
	}
	// pidfd pins this process instance across verification and signalling.
	pidfd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return fmt.Errorf("open daemon process: %w", err)
	}
	defer unix.Close(pidfd)
	lockInfo, err := os.Stat(filepath.Join(stateDir, ".agent.lock"))
	if err != nil {
		return err
	}
	descriptors, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	if err != nil {
		return err
	}
	selected := false
	for _, descriptor := range descriptors {
		info, err := os.Stat(fmt.Sprintf("/proc/%d/fd/%s", pid, descriptor.Name()))
		if err == nil && os.SameFile(info, lockInfo) {
			selected = true
			break
		}
	}
	if !selected {
		return errors.New("stale daemon PID does not own selected state lock")
	}
	if err := unix.PidfdSendSignal(pidfd, unix.SIGTERM, nil, 0); err != nil {
		return fmt.Errorf("signal selected daemon: %w", err)
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
