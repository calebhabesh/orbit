package app

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
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

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/scheduler"
	"github.com/calebhabesh/orbit/internal/state"
	"github.com/calebhabesh/orbit/internal/workspace"
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
	TransferFaultHook   func(string) error // deterministic disposable transfer campaigns
	NetworkRoots        *x509.CertPool     // independently configured self-host/development service trust
	ControllerFaultHook control.FaultHook  // deterministic disposable fault campaigns
	PeerAddress         string
	ControlAddress      string // loopback control listener, e.g. "127.0.0.1:8080"
	Ready               io.Writer
	Profile             string        // "laptop" or "pi"
	BandwidthLimitBps   int64         // 0 = unlimited
	SyncInterval        time.Duration // default 5m
	FullScanInterval    time.Duration // default 24h
	NoWatch             bool
	ClientFactory       scheduler.ClientFactory
	AllowInitialize     bool // auto-initialize clean uninitialized state directory
	// StartedBy records who started this daemon (only "terminal" is recorded),
	// so status can name the owner separately from the startup mode.
	StartedBy string
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
	_ = os.Remove(filepath.Join(stateDir, ".agent.origin"))
	if opts.StartedBy == "terminal" {
		if err := config.WritePrivate(stateDir, ".agent.origin", []byte("terminal\n")); err != nil {
			return err
		}
		defer os.Remove(filepath.Join(stateDir, ".agent.origin"))
	}

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
	if packaged, ok := network.BuiltinProfile(); ok {
		outcome, e := config.AdoptPackagedProfile(stateDir, packaged, uint64(time.Now().Unix()))
		if e != nil {
			return e
		}
		if outcome != "" && outcome != config.PackagedReview && opts.Ready != nil {
			fmt.Fprintf(opts.Ready, "network profile: %s packaged release epoch %d\n", outcome, packaged.Profile.Epoch)
		}
	}
	networkPolicy, err := config.LoadNetworkPolicy(stateDir)
	if err != nil {
		return err
	}

	manager := network.NewManager(network.ManagerOptions{Generation: uint64(networkPolicy.Generation)})
	if err := manager.ConfigureTiming(networkPolicy.Timing); err != nil {
		return err
	}
	defer manager.Close() // after setup/scheduler joins, before SQLite closes
	// Peer LAN exchange shares signed LAN records with approved peers over their
	// pinned session, so a host firewall on one side does not force the relay.
	var lanExchange *network.LANExchange
	if networkPolicy.Mode != "manual" {
		lanExchange = network.NewLANExchange(manager)
		defer lanExchange.Close()
	}
	peerServer := func() *replication.Server { return replication.NewServer(db, identity).WithLAN(lanExchange) }
	ws = workspace.New(db, workspace.Options{})
	var directListener net.Listener
	var quicEndpoint *network.QUICEndpoint
	var lanDiscovery *network.LANDiscovery
	var directInterfaces []network.LocalInterface
	if networkPolicy.Mode != "manual" {
		settings, e := config.LoadDirectSettings(stateDir)
		if e == nil {
			directInterfaces, e = network.SelectedInterfaces(settings.Interfaces)
		}
		if e == nil {
			directListener, e = network.OptionalDirectListener(settings)
		}
		// UDP bind failure is independent of TCP and never blocks capture/relay.
		if len(directInterfaces) > 0 {
			var socket net.PacketConn
			var udpErr error
			if networkPolicy.Mode == "local_only" {
				socket, udpErr = network.OptionalLocalQUICSocket(settings, directInterfaces)
			} else {
				socket, udpErr = network.OptionalQUICSocket(settings)
			}
			if udpErr == nil && socket != nil {
				quicEndpoint, udpErr = manager.NewPeerQUICEndpoint(socket, identity.ServerTLSConfig(), peerServer())
				if udpErr != nil {
					_ = socket.Close()
				} else {
					defer quicEndpoint.Close()
					if udpErr = manager.EnableQUIC(quicEndpoint); udpErr != nil {
						return udpErr
					}
				}
			}
			if udpErr != nil && opts.Ready != nil {
				fmt.Fprintf(opts.Ready, "UDP route limitation: %v\n", udpErr)
			}
		}
		if e != nil && opts.Ready != nil {
			fmt.Fprintf(opts.Ready, "direct route limitation: %v\n", e)
		}
		if directListener != nil {
			if networkPolicy.Mode == "local_only" {
				directListener = network.LocalListener(directListener, directInterfaces)
			}
			defer directListener.Close()
		}
	}
	// Reviewed durable identities remain eligible for LAN-only routing even if a
	// public service profile is missing or unavailable. Addresses supply no trust.
	durableRoutes, e := config.LoadPeerRoutes(stateDir)
	if e != nil {
		return e
	}
	for _, route := range durableRoutes {
		if networkPolicy.Mode == "manual" {
			break
		}
		target := network.Target{Device: controlID(route.Device), Pin: history.Digest(controlID(route.Pin)), Profile: history.Digest(controlID(route.Profile)), Purpose: network.PeerData}
		if e = manager.RegisterDirect(target, nil); e != nil {
			return e
		}
	}
	var lanCandidates []protocol.NetworkCandidate
	if directListener != nil {
		lanCandidates = network.GatherCandidates(directInterfaces, directListener.Addr(), false)
	}
	if quicEndpoint != nil {
		lanCandidates = network.CombineDirectCandidates(lanCandidates, network.GatherCandidates(directInterfaces, quicEndpoint.LocalAddr(), false))
	}
	if networkPolicy.LANAdvertising && len(lanCandidates) > 0 {
		lanDiscovery, e = network.NewLANDiscovery(ctx, manager, cfg.DeviceID, hex.EncodeToString(identity.KeyPin[:]), identity.Certificate, directInterfaces, lanCandidates, manager.KnownTargets)
		if e != nil && opts.Ready != nil {
			fmt.Fprintf(opts.Ready, "local discovery limitation: %v\n", e)
		}
		if lanDiscovery != nil {
			defer lanDiscovery.Close()
		}
		lanExchange.Set(lanDiscovery)
	}

	var relayRuntime *network.RelayRuntime
	var serviceClient *network.ServiceClient
	networkError := ""
	activateNetwork := func() error {
		if networkPolicy.Mode == "automatic" || networkPolicy.Mode == "self_hosted" {
			selection, e := config.LoadNetworkProfile(stateDir, uint64(time.Now().Unix()))
			if e != nil {
				return e
			}
			if networkPolicy.Mode == "automatic" && selection.Environment != "release" {
				return errors.New("PROFILE_UNTRUSTED")
			}
			digest, e := selection.Digest()
			if e != nil {
				return e
			}
			if digest != networkPolicy.Profile {
				return errors.New("PROFILE_MISMATCH")
			}
			// Completes a rotation interrupted between profile and route writes.
			if e = config.RebindPeerRoutes(stateDir, digest); e != nil {
				return e
			}
			origin := ""
			for _, o := range selection.Profile.Origins {
				if strings.HasPrefix(o, "https://") {
					origin = o
					break
				}
			}
			// Release profiles verify against system roots only; reviewed custom
			// trust applies to self-hosted/development services.
			roots := opts.NetworkRoots
			if roots == nil && selection.Environment != "release" {
				if roots, _, e = config.LoadServiceRoots(stateDir, time.Now()); e != nil {
					return e
				}
			}
			serviceClient, e = network.NewServiceClient(network.ServiceClientOptions{Selection: selection, Origin: origin, Device: cfg.DeviceID, Certificate: identity.Certificate, Roots: roots})
			if e != nil {
				return e
			}

			var iceOptions *network.ICEOptions
			settings, settingsErr := config.LoadDirectSettings(stateDir)
			if settingsErr == nil && !settings.UDPDisabled {
				iceOptions = &network.ICEOptions{TLS: identity.ServerTLSConfig(), Peer: peerServer(), Interfaces: settings.Interfaces}
			}
			relayRuntime, e = network.NewRelayRuntime(ctx, serviceClient, manager, uint64(networkPolicy.Generation), digest, iceOptions)
			if e != nil {
				_ = serviceClient.Close()
				return e
			}

			var publicCandidates []protocol.NetworkCandidate
			if directListener != nil {
				publicCandidates = network.GatherCandidates(directInterfaces, directListener.Addr(), true)
			}
			if quicEndpoint != nil {
				publicCandidates = network.CombineDirectCandidates(publicCandidates, network.GatherCandidates(directInterfaces, quicEndpoint.LocalAddr(), true))
			}
			_ = relayRuntime.SetPublicCandidates(publicCandidates)
			routes, e := config.LoadPeerRoutes(stateDir)
			if e != nil {
				return e
			}
			for _, route := range routes {
				target := network.Target{Device: controlID(route.Device), Pin: history.Digest(controlID(route.Pin)), Profile: history.Digest(controlID(route.Profile)), Purpose: network.PeerData}
				if e = relayRuntime.Register(target); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e := activateNetwork(); e != nil {
		networkError = "PROFILE_MISSING_OR_EXPIRED"
		if !errors.Is(e, os.ErrNotExist) {
			networkError = "PROFILE_INVALID"
			switch e.Error() {
			case "PROFILE_EXPIRED", "PROFILE_UNTRUSTED", "PROFILE_MISMATCH":
				networkError = e.Error()
			}
		}
	}
	if serviceClient != nil {
		defer serviceClient.Close()
	}
	if relayRuntime != nil {
		defer relayRuntime.Close()
	}

	peerTLS := func(t network.Target) (*tls.Config, error) {
		routes, e := config.LoadPeerRoutes(stateDir)
		if e != nil {
			return nil, e
		}
		for _, route := range routes {
			if route.Device == hex.EncodeToString(t.Device[:]) && route.Pin == hex.EncodeToString(t.Pin[:]) {
				cert, _, e := protocol.NetworkCertificate(route.CertificateDER, route.Pin, uint64(time.Now().Unix()))
				if e != nil {
					return nil, e
				}
				return identity.ClientTLSConfig(cert, t.Pin)
			}
		}
		endpoints, e := config.LoadPeerEndpoints(stateDir)
		if e != nil {
			return nil, e
		}
		for _, endpoint := range endpoints {
			if endpoint.Device == hex.EncodeToString(t.Device[:]) {
				b, e := os.ReadFile(endpoint.Certificate)
				if e != nil {
					return nil, e
				}
				cert, e := replication.ParsePeerCertificate(b)
				if e != nil {
					return nil, e
				}
				return identity.ClientTLSConfig(cert, t.Pin)
			}
		}
		return nil, errors.New("IDENTITY_MISMATCH")
	}
	ctrl = control.New(db, ws, control.Options{LocalDevice: deviceID, Network: manager, Relay: relayRuntime, NetworkPolicy: &networkPolicy, NetworkError: networkError, NetworkService: serviceClient, NetworkSelfPin: hex.EncodeToString(identity.KeyPin[:]), NetworkPeerTLS: peerTLS, StopInternet: func() error {
		if relayRuntime != nil {
			_ = relayRuntime.Close()
		}
		if serviceClient != nil {
			_ = serviceClient.Close()
		}
		// Keep the local control listener and daemon alive so the reviewed
		// mutation can return its restart-required state. Invalidate all
		// WAN leases and drain idle pools; the next daemon generation binds
		// only the selected local/manual policy.
		manager.NetworkChanged()
		return nil
	}, FaultHook: opts.ControllerFaultHook})
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
		routes, err := config.LoadPeerRoutes(stateDir)
		if err != nil {
			return nil, err
		}
		for _, route := range routes {
			endpoints = append(endpoints, config.PeerEndpoint{Folder: route.Folder, Device: route.Device})
		}
		var targets []scheduler.PeerTarget
		seen := map[string]bool{}
		for _, endpoint := range endpoints {
			folder, err := decodeDeviceID(endpoint.Folder)
			if err != nil {
				return nil, err
			}
			peer, err := decodeDeviceID(endpoint.Device)
			if err != nil {
				return nil, err
			}
			mem, _, err := db.GetMembership(ctx, folder)
			if err != nil {
				continue
			}
			active := false
			for _, m := range mem.Active {
				if m.Device == peer {
					active = true
					break
				}
			}
			if !active {
				continue
			}
			key := endpoint.Folder + "/" + endpoint.Device
			if seen[key] {
				continue
			}
			seen[key] = true
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
			routes, e := config.LoadPeerRoutes(stateDir)
			if e != nil {
				return nil, e
			}
			for _, route := range routes {
				if route.Folder == hex.EncodeToString(folder[:]) && route.Device == hex.EncodeToString(peer[:]) {

					cert, _, e := protocol.NetworkCertificate(route.CertificateDER, route.Pin, uint64(time.Now().Unix()))
					if e != nil {
						return nil, e
					}
					membership, e := db.Membership(ctx, folder)
					if e != nil {
						return nil, e
					}
					pin := replication.PublicKeyPin(cert)
					if e = db.AuthorizePeer(ctx, folder, peer, pin, membership.Revision, membership.Digest); e != nil {
						return nil, e
					}
					target := network.Target{Device: peer, Pin: pin, Profile: history.Digest(controlID(route.Profile)), Purpose: network.PeerData}
					if relayRuntime != nil {
						e = relayRuntime.Register(target)
					} else if networkPolicy.Mode != "manual" {
						e = manager.RegisterDirect(target, nil)
					} else {
						e = errors.New("UNSUPPORTED_CAPABILITY")
					}
					if e != nil {
						return nil, e
					}
					return replication.NewRoutedClient(ctx, network.LogicalOrigin(target), identity, cert, target, manager)
				}
			}

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
				target := network.Target{Device: peer, Pin: pin, Purpose: network.PeerData}
				if networkPolicy.Mode == "local_only" && !network.LocalEndpoint(endpoint.URL) {
					if err = manager.RegisterDirect(target, nil); err != nil {
						return nil, err
					}
					return replication.NewRoutedClient(ctx, network.LogicalOrigin(target), identity, cert, target, manager)
				}
				if err := manager.SetManual(target, endpoint.URL); err != nil {
					return nil, err
				}
				return replication.NewRoutedClient(ctx, endpoint.URL, identity, cert, target, manager)
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
	// Routed onboarding admits new roots and peer routes after startup. Refresh
	// them promptly rather than waiting for the legacy five-minute cadence.
	if opts.SyncInterval == 0 && (networkPolicy.Mode == "automatic" || networkPolicy.Mode == "self_hosted" || networkPolicy.Mode == "local_only") {
		opts.SyncInterval = 5 * time.Second
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
		TransferHook:  opts.TransferFaultHook,
		Profile:       prof,
		Limiter:       limiter,
		NoWatch:       opts.NoWatch,
		ClientFactory: opts.ClientFactory,
		LocalDevice:   deviceID,
		Peers:         targets,
		PeerTargets:   peerTargets,
		Joining:       ctrl.JoiningFolders,
	})
	if err != nil {
		return fmt.Errorf("create scheduler: %w", err)
	}

	var peerListener net.Listener
	if opts.PeerAddress != "" && (networkPolicy.Mode != "local_only" || network.LocalEndpoint("https://"+opts.PeerAddress)) {
		peerListener, err = net.Listen("tcp", opts.PeerAddress)
		if err != nil {
			return fmt.Errorf("listen for peers: %w", err)
		}
		if networkPolicy.Mode == "local_only" {
			peerListener = network.LocalListener(peerListener, directInterfaces)
		}
		defer peerListener.Close()
		if err := config.WritePrivate(stateDir, "peer.addr", []byte(peerListener.Addr().String()+"\n")); err != nil {
			return err
		}
		defer os.Remove(filepath.Join(stateDir, "peer.addr"))
	}
	var enrollmentListener net.Listener
	if runtimeSettings.EnrollmentListen != "" && (networkPolicy.Mode != "local_only" || network.LocalEndpoint("https://"+runtimeSettings.EnrollmentListen)) {
		enrollmentListener, err = net.Listen("tcp", runtimeSettings.EnrollmentListen)
		if err != nil {
			return fmt.Errorf("listen for enrollment: %w", err)
		}
		if networkPolicy.Mode == "local_only" {
			enrollmentListener = network.LocalListener(enrollmentListener, directInterfaces)
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
				_ = ctrl.ResumeSetupJobs(setupCtx)
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
	networkDone := make(chan error, 5)
	count := 2
	virtualData, _ := manager.IncomingListener(network.PeerData)
	virtualEnrollment, _ := manager.IncomingListener(network.Enrollment)
	go func() { networkDone <- peerServer().Serve(networkCtx, virtualData) }()
	go func() {
		networkDone <- replication.NewEnrollmentServer(db, identity).Serve(networkCtx, virtualEnrollment)
	}()
	if directListener != nil {
		count++
		go func() { networkDone <- peerServer().Serve(networkCtx, directListener) }()
	}
	if enrollmentListener != nil {
		count++
		go func() {
			networkDone <- replication.NewEnrollmentServer(db, identity).Serve(networkCtx, enrollmentListener)
		}()
	}
	if peerListener != nil {
		count++
		go func() { networkDone <- peerServer().Serve(networkCtx, peerListener) }()
	}
	lanExchange.Start(networkCtx, peerTLS)
	// Interface and default-route observations are transient. One joined watcher
	// refreshes actual candidates; it never writes policy, trust or sync history.
	if networkPolicy.Mode != "manual" {
		if settings, settingsErr := config.LoadDirectSettings(stateDir); settingsErr == nil {
			stopNetwork := network.WatchNetworkTiming(ctx, func() (string, error) {
				return network.NetworkSnapshot(settings.Interfaces)
			}, func() {
				interfaces, _ := network.SelectedInterfaces(settings.Interfaces)
				manager.NetworkChanged()
				network.RefreshLocalListener(directListener, interfaces)
				network.RefreshLocalListener(peerListener, interfaces)
				network.RefreshLocalListener(enrollmentListener, interfaces)
				if networkPolicy.Mode == "local_only" {
					// Retire the old concrete binding before rebinding a fixed port.
					// Failed requests retain existing chunk/operation retry rules.
					_ = manager.ReplaceQUIC(nil)
					// Local UDP is bound to one concrete address; replace it on roaming.
					var next *network.QUICEndpoint
					if socket, socketErr := network.OptionalLocalQUICSocket(settings, interfaces); socketErr == nil && socket != nil {
						next, socketErr = manager.NewPeerQUICEndpoint(socket, identity.ServerTLSConfig(), peerServer())
						if socketErr != nil {
							_ = socket.Close()
						}
					}
					if replaceErr := manager.ReplaceQUIC(next); replaceErr != nil && next != nil {
						_ = next.Close()
					}
					quicEndpoint = next
				}
				var localCandidates, publicCandidates []protocol.NetworkCandidate
				if directListener != nil {
					localCandidates = network.GatherCandidates(interfaces, directListener.Addr(), false)
					publicCandidates = network.GatherCandidates(interfaces, directListener.Addr(), true)
				}
				if quicEndpoint != nil {
					localCandidates = network.CombineDirectCandidates(localCandidates, network.GatherCandidates(interfaces, quicEndpoint.LocalAddr(), false))
					publicCandidates = network.CombineDirectCandidates(publicCandidates, network.GatherCandidates(interfaces, quicEndpoint.LocalAddr(), true))
				}
				lanExchange.Set(nil)
				if lanDiscovery != nil {
					_ = lanDiscovery.Close()
					lanDiscovery = nil
				}
				if networkPolicy.LANAdvertising && len(localCandidates) > 0 {
					lanDiscovery, _ = network.NewLANDiscovery(ctx, manager, cfg.DeviceID, hex.EncodeToString(identity.KeyPin[:]), identity.Certificate, interfaces, localCandidates, manager.KnownTargets)
				}
				lanExchange.Set(lanDiscovery)
				if relayRuntime != nil {
					_ = relayRuntime.SetPublicCandidates(publicCandidates)
					relayRuntime.NetworkChanged()
					relayRuntime.ReconnectControl()
				}
			}, time.Duration(networkPolicy.Timing.Effective().PollMS)*time.Millisecond, time.Duration(networkPolicy.Timing.Effective().QuietMS)*time.Millisecond)
			defer func() {
				stopNetwork()
				if lanDiscovery != nil {
					_ = lanDiscovery.Close()
				}
			}()
		}
	}
	if opts.Ready != nil {
		peerAddress := "disabled"
		if peerListener != nil {
			peerAddress = peerListener.Addr().String()
		} else if directListener != nil {
			peerAddress = directListener.Addr().String()
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

func controlID(s string) history.ID {
	var id history.ID
	b, _ := hex.DecodeString(s)
	copy(id[:], b)
	return id
}
