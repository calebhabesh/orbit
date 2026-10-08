package network

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"time"

	p "github.com/calebhabesh/orbit/internal/protocol"
)

// RelayRuntime rebuilds service/control state from reviewed durable intent.
// It never persists credentials and never creates folder authority. Each purpose
// has one joined worker; service failure leaves local capture/manual work alive.
type RelayRuntime struct {
	changes          map[Purpose]chan struct{}
	ice              *iceCoordinator
	iceOptions       *ICEOptions
	retryAfter       time.Time
	retryCode        string
	lookupCache      map[Target]p.NetworkAnnouncement
	ctx              context.Context
	cancel           context.CancelFunc
	client           *ServiceClient
	manager          *ConnectionManager
	generation       uint64
	publicCandidates []p.NetworkCandidate
	profile          string
	mu               sync.Mutex
	pins             map[Purpose]map[string]string
	endpoints        map[Purpose]*RelayEndpoint
	wg               sync.WaitGroup
	ready            map[Purpose]bool
}

func NewRelayRuntime(ctx context.Context, client *ServiceClient, manager *ConnectionManager, generation uint64, profile string, iceOptions ...*ICEOptions) (*RelayRuntime, error) {
	if client == nil || manager == nil || generation == 0 || profile != client.digest {
		return nil, errors.New(p.NetworkProfileUntrusted)
	}
	life, cancel := context.WithCancel(ctx)
	if stamp := uint64(time.Now().UnixNano()); stamp > generation {
		generation = stamp
	}
	r := &RelayRuntime{changes: map[Purpose]chan struct{}{PeerData: make(chan struct{}, 1), Enrollment: make(chan struct{}, 1)}, lookupCache: map[Target]p.NetworkAnnouncement{}, ctx: life, cancel: cancel, client: client, manager: manager, generation: generation, profile: profile, pins: map[Purpose]map[string]string{PeerData: {}, Enrollment: {}}, endpoints: map[Purpose]*RelayEndpoint{}, ready: map[Purpose]bool{}}
	if len(iceOptions) > 0 {
		if iceOptions[0] != nil {
			copyOptions := *iceOptions[0]
			copyOptions.admission = manager.quicSlots
			r.iceOptions = &copyOptions
		}
		if r.iceOptions != nil && (r.iceOptions.TLS == nil || r.iceOptions.Peer == nil) {
			cancel()
			return nil, ErrNoRoute
		}
		if r.iceOptions != nil {
			owner := &RelayEndpoint{client: client, purpose: PeerData, ctx: life, generation: generation, known: map[string]string{}}
			r.ice = newICECoordinator(owner, *r.iceOptions)
			r.iceOptions.shared = r.ice
			if err := manager.enableICE(r.ICE); err != nil {
				cancel()
				return nil, err
			}
		}
	}
	for _, purpose := range []Purpose{PeerData, Enrollment} {
		r.wg.Add(1)
		go r.run(purpose)
	}
	return r, nil
}
func (r *RelayRuntime) run(purpose Purpose) {
	defer r.wg.Done()
	generation := r.generation
	failures := 0
	for r.ctx.Err() == nil {
		generation++
		listener, _ := r.manager.IncomingListener(purpose)
		r.mu.Lock()
		pins := map[string]string{}
		for id, pin := range r.pins[purpose] {
			pins[id] = pin
		}
		r.mu.Unlock()
		endpoint, err := NewRelayEndpoint(r.ctx, r.client, purpose, generation, pins, purpose == Enrollment, listener, r.iceOptions)
		if err == nil {
			failures = 0
			r.mu.Lock()
			for id, pin := range r.pins[purpose] {
				_ = endpoint.AddPeer(id, pin)
			}
			r.endpoints[purpose] = endpoint
			r.mu.Unlock()
			announceCtx, cancel := context.WithCancel(r.ctx)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				// The service keeps an accepted record until it expires; this
				// endpoint keeps offering its generation until then.
				var acceptedUntil time.Time
				for announceCtx.Err() == nil {
					generation++
					candidates := r.Candidates(purpose)
					capabilities := []string{"relay_inner_tls_v1", "enrollment_v3"}
					if len(candidates) > 0 {
						capabilities = append(capabilities, "direct_https_v1")
					}
					for _, c := range candidates {
						if c.Transport == "udp" {
							capabilities = append(capabilities, "quic_http3_v1")
							break
						}
					}
					if purpose == PeerData && r.iceOptions != nil {
						capabilities = append(capabilities, "quic_ice_v1")
					}
					announcement := p.NetworkAnnouncement{Generation: p.NetworkUint(generation), Expires: p.NetworkUint(time.Now().Unix() + 600), Candidates: candidates, Capabilities: capabilities, Relay: true}
					operation := randomNetworkID()
					err := r.serviceCooldown()
					if err == nil {
						err = r.client.Announce(announceCtx, string(purpose), operation, announcement)
						r.recordServiceFailure(err)
					}
					r.recordServiceFailure(err)
					if err != nil && announceCtx.Err() == nil && r.serviceCooldown() == nil {
						err = r.client.Announce(announceCtx, string(purpose), operation, announcement)
						r.recordServiceFailure(err)
					}
					// Offers must bind the service-accepted generation, never a refused renewal.
					if err == nil {
						endpoint.SetGeneration(generation)
						acceptedUntil = time.Unix(int64(announcement.Expires), 0).Add(-AnnouncementRenewalMargin)
					}
					// A deferred or overloaded renewal leaves the previously accepted
					// record serving; only a cold start, a semantic refusal or an
					// expiring record withdraws readiness.
					r.mu.Lock()
					r.ready[purpose] = err == nil || (transientServiceFailure(err) && time.Now().Before(acceptedUntil))
					r.mu.Unlock()
					delay := ReannounceInterval
					if err != nil {
						delay = 10 * time.Second
					}
					timer := time.NewTimer(delay)
					select {
					case <-announceCtx.Done():
						timer.Stop()
						return
					case <-timer.C:
					case <-r.changes[purpose]:
						timer.Stop()
					}
				}
			}()
			select {
			case <-endpoint.done:
			case <-r.ctx.Done():
			}
			cancel()
			_ = endpoint.Close()
			<-finished

			r.mu.Lock()
			delete(r.endpoints, purpose)
			r.ready[purpose] = false
			r.mu.Unlock()
		}
		// A refused control (the service still holds this device's previous
		// channel until its heartbeat drops it, and closes the new one without
		// a typed reason) backs off 2, 4, then 8 seconds instead of spending a
		// challenge every two seconds; a quota quiet period is also honored.
		r.recordServiceFailure(err)
		delay := 2 * time.Second
		if err != nil {
			delay <<= min(failures, 2)
			failures++
		}
		r.mu.Lock()
		if wait := time.Until(r.retryAfter); wait > delay {
			delay = wait
		}
		r.mu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-r.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func (r *RelayRuntime) Register(target Target) error {
	if target.Validate() != nil || hex.EncodeToString(target.Profile[:]) != r.profile {
		return errors.New(p.NetworkProfileUntrusted)
	}
	id, pin := hex.EncodeToString(target.Device[:]), hex.EncodeToString(target.Pin[:])
	r.mu.Lock()
	defer r.mu.Unlock()
	peers := r.pins[target.Purpose]
	if old, ok := peers[id]; ok {
		if old != pin {
			return errors.New(p.NetworkIdentityMismatch)
		}
		return nil
	}
	if _, ok := peers[id]; !ok && len(peers) >= 128 {
		return ErrBackpressure
	}
	if endpoint := r.endpoints[target.Purpose]; endpoint != nil {
		if err := endpoint.AddPeer(id, pin); err != nil {
			return err
		}
	}
	if err := r.manager.SetRelay(target, r.Dial); err != nil {
		return err
	}
	if target.Purpose == PeerData {
		if err := r.manager.RegisterDirect(target, r.LookupCandidates); err != nil {
			return err
		}
	}
	peers[id] = pin
	if target.Purpose == PeerData && r.ice != nil {
		r.ice.addPeer(id, pin)
	}
	return nil
}
func (r *RelayRuntime) Dial(ctx context.Context, target Target) (net.Conn, error) {
	if err := r.serviceCooldown(); err != nil {
		return nil, err
	}
	if hex.EncodeToString(target.Profile[:]) != r.profile {
		return nil, errors.New(p.NetworkProfileUntrusted)
	}
	r.mu.Lock()
	endpoint := r.endpoints[target.Purpose]
	a, found := r.lookupCache[target]
	ready := r.ready[target.Purpose]
	r.mu.Unlock()
	if endpoint == nil || !ready {
		return nil, &ServiceError{Code: p.NetworkServiceUnavailable}
	}
	var cached *p.NetworkAnnouncement
	if found {
		cached = &a
	}
	c, err := endpoint.dial(ctx, target, cached)
	r.recordServiceFailure(err)
	var failure *ServiceError
	if errors.As(err, &failure) && (failure.Code == p.NetworkStaleGeneration || failure.Code == p.NetworkUnavailable) {
		r.mu.Lock()
		delete(r.lookupCache, target)
		r.mu.Unlock()
	}
	return c, err
}
func (r *RelayRuntime) Close() error {
	r.cancel()
	r.wg.Wait()
	if r.ice != nil {
		r.ice.close()
	}
	return nil
}

func (r *RelayRuntime) WaitReady(ctx context.Context) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		r.mu.Lock()
		ready := r.ready[Enrollment] && r.ready[PeerData]
		r.mu.Unlock()
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.ctx.Done():
			return ErrClosed
		case <-ticker.C:
		}
	}
}

// Ready is a cached observation. It performs no remote query.
func (r *RelayRuntime) Ready() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ctx.Err() == nil && r.ready[Enrollment] && r.ready[PeerData]
}

// SetPublicCandidates accepts only actual bound public listener addresses. LAN
// interfaces and service-observed source addresses are never published here.
func (r *RelayRuntime) SetPublicCandidates(candidates []p.NetworkCandidate) error {
	if len(candidates) > p.NetworkMaxCandidates {
		return ErrBackpressure
	}
	for _, c := range candidates {
		if c.Validate(true) != nil || (c.Transport != "tcp" && c.Transport != "udp") {
			return errors.New("INVALID_CANDIDATE")
		}
	}
	r.mu.Lock()
	r.publicCandidates = append([]p.NetworkCandidate(nil), candidates...)
	r.mu.Unlock()
	return nil
}
func (r *RelayRuntime) Candidates(purpose Purpose) []p.NetworkCandidate {
	r.mu.Lock()
	defer r.mu.Unlock()
	if purpose != PeerData {
		return []p.NetworkCandidate{}
	}
	return append([]p.NetworkCandidate{}, r.publicCandidates...)
}
func (r *RelayRuntime) LookupCandidates(ctx context.Context, t Target, minimum uint64) (p.NetworkAnnouncement, error) {
	if !r.purposeReady(t.Purpose) {
		return p.NetworkAnnouncement{}, &ServiceError{Code: p.NetworkServiceUnavailable}
	}
	if err := r.serviceCooldown(); err != nil {
		return p.NetworkAnnouncement{}, err
	}
	a, found, err := r.client.Lookup(ctx, hex.EncodeToString(t.Device[:]), hex.EncodeToString(t.Pin[:]), string(t.Purpose), minimum)
	r.recordServiceFailure(err)
	if err != nil {
		return p.NetworkAnnouncement{}, err
	}
	if !found {
		return p.NetworkAnnouncement{}, ErrNoRoute
	}
	r.mu.Lock()
	old, exists := r.lookupCache[t]
	if (!exists && len(r.lookupCache) < 128) || (exists && a.Generation >= old.Generation) {
		r.lookupCache[t] = a
	}
	r.mu.Unlock()
	return a, nil
}

func (r *RelayRuntime) ICE(ctx context.Context, t Target) (*QUICEndpoint, net.Addr, error) {
	if r.ice == nil || !r.purposeReady(PeerData) {
		return nil, nil, ErrNoRoute
	}
	if err := r.serviceCooldown(); err != nil {
		return nil, nil, err
	}
	endpoint, addr, err := r.ice.Dial(ctx, t)
	r.recordServiceFailure(err)
	return endpoint, addr, err
}

// A quota refusal needs a quiet refill, shared by all targets using this client.
// RelayBudgetRetry spaces relay attempts after a RELAY_BUDGET refusal.
const RelayBudgetRetry = time.Hour

// Refused local retries do not extend the deadline or send more service traffic.
func (r *RelayRuntime) serviceCooldown() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Now().Before(r.retryAfter) {
		return &ServiceError{Code: r.retryCode}
	}
	return nil
}
func (r *RelayRuntime) recordServiceFailure(err error) {
	var service *ServiceError
	if !errors.As(err, &service) || (service.Code != p.NetworkQuota && service.Code != p.NetworkRelayBudget) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Now().Before(r.retryAfter) {
		return
	}
	quiet := ServiceRetryQuietPeriod
	if service.Code == p.NetworkRelayBudget {
		// A spent monthly relay budget lasts until the month resets or the
		// operator raises it; ask again hourly, not every quiet period.
		quiet = RelayBudgetRetry
	}
	r.retryAfter = time.Now().Add(quiet)
	r.retryCode = service.Code
}

// NetworkChanged invalidates cached locations and coalesces fresh announcements.
// Generation counters remain owned by each purpose's single announce worker.
func (r *RelayRuntime) NetworkChanged() {
	r.mu.Lock()
	r.lookupCache = map[Target]p.NetworkAnnouncement{}
	r.mu.Unlock()
	for _, purpose := range []Purpose{PeerData, Enrollment} {
		select {
		case r.changes[purpose] <- struct{}{}:
		default:
		}
	}
}

// ReconnectControl rebuilds each purpose's endpoint after an actual address
// change. Its control channel was bound to the old address and fails silently,
// and both incoming offers and outgoing accepts travel over it. The run loop
// creates a fresh endpoint and announcement; the service admits the new
// channel once its heartbeat drops the old one.
func (r *RelayRuntime) ReconnectControl() {
	r.mu.Lock()
	endpoints := make([]*RelayEndpoint, 0, len(r.endpoints))
	for _, e := range r.endpoints {
		endpoints = append(endpoints, e)
	}
	r.mu.Unlock()
	for _, e := range endpoints {
		go e.Close()
	}
}

// A cold or refused announcement cannot authorize a matching generation offer.
func (r *RelayRuntime) purposeReady(purpose Purpose) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ctx.Err() == nil && r.ready[purpose]
}

// transientServiceFailure is overload or unavailability, not a refusal of the
// announcement's identity, profile or generation.
func transientServiceFailure(err error) bool {
	if errors.Is(err, ErrBackpressure) {
		return true
	}
	code := err.Error()
	var service *ServiceError
	if errors.As(err, &service) {
		code = service.Code
	}
	switch code {
	case p.NetworkQuota, p.NetworkRelayBudget, p.NetworkServiceUnavailable, p.NetworkCanceled:
		return true
	}
	return false
}
