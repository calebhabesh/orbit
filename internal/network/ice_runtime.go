package network

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	p "github.com/calebhabesh/orbit/internal/protocol"
	"github.com/pion/transport/v5"
)

// ICEOptions is daemon-owned peer TLS and interface intent, never service trust.
type ICEOptions struct {
	shared    *iceCoordinator
	admission chan struct{}
	// Net supplies an isolated socket backend for disposable emulator tests.
	// The daemon leaves it nil, which selects native OS sockets.
	Net        transport.Net
	TLS        *tls.Config
	Peer       http.Handler
	Interfaces []string
}
type iceRoute struct {
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	accepts       chan p.NetworkOfferRequest
	session       string
	local, remote p.NetworkUint
	agent         *ICESession
	endpoint      *QUICEndpoint
	address       net.Addr
	err           error
}
type iceCoordinator struct {
	owner   *RelayEndpoint
	options ICEOptions
	mu      sync.Mutex
	routes  map[Target]*iceRoute
	workers chan struct{}
	wg      sync.WaitGroup
	closed  bool
}

func newICECoordinator(owner *RelayEndpoint, opts ICEOptions) *iceCoordinator {
	opts.Interfaces = append([]string{}, opts.Interfaces...)
	opts.TLS = opts.TLS.Clone()
	return &iceCoordinator{owner: owner, options: opts, routes: map[Target]*iceRoute{}, workers: make(chan struct{}, 2)}
}
func (i *iceCoordinator) target(id, pin string) Target {
	var t Target
	b, _ := hex.DecodeString(id)
	copy(t.Device[:], b)
	b, _ = hex.DecodeString(pin)
	copy(t.Pin[:], b)
	b, _ = hex.DecodeString(i.owner.client.digest)
	copy(t.Profile[:], b)
	t.Purpose = PeerData
	return t
}
func (i *iceCoordinator) generations() (uint64, bool) {
	i.owner.mu.Lock()
	defer i.owner.mu.Unlock()
	return i.owner.generation, i.owner.ctx.Err() == nil
}
func (i *iceCoordinator) smaller(t Target) bool {
	return i.owner.client.device+"/"+i.owner.client.pin < hex.EncodeToString(t.Device[:])+"/"+hex.EncodeToString(t.Pin[:])
}
func (i *iceCoordinator) allocate(t Target, local, remote p.NetworkUint, session string) (*iceRoute, bool, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil, false, ErrClosed
	}
	if old := i.routes[t]; old != nil {
		if old.local != local || old.remote != remote {
			return nil, false, ErrStale
		}
		return old, false, nil
	}
	if len(i.routes) >= 8 {
		return nil, false, ErrBackpressure
	}
	select {
	case i.workers <- struct{}{}:
	default:
		return nil, false, ErrBackpressure
	}
	life, cancel := context.WithCancel(i.owner.ctx)
	r := &iceRoute{ctx: life, cancel: cancel, done: make(chan struct{}), accepts: make(chan p.NetworkOfferRequest, 1), session: session, local: local, remote: remote}
	i.routes[t] = r
	i.wg.Add(1)
	return r, true, nil
}
func (i *iceCoordinator) complete(t Target, r *iceRoute, err error) {
	i.mu.Lock()
	r.err = err
	close(r.done)
	if err != nil && i.routes[t] == r {
		delete(i.routes, t)
	}
	i.mu.Unlock()
	if err != nil {
		r.cancel()
		if r.endpoint != nil {
			_ = r.endpoint.Close()
		}
		if r.agent != nil {
			_ = r.agent.Close()
		}
	}
	<-i.workers
	i.wg.Done()
}
func (i *iceCoordinator) Dial(ctx context.Context, t Target) (*QUICEndpoint, net.Addr, error) {
	if t.Purpose != PeerData || hex.EncodeToString(t.Profile[:]) != i.owner.client.digest {
		return nil, nil, ErrNoRoute
	}
	id, pin := hex.EncodeToString(t.Device[:]), hex.EncodeToString(t.Pin[:])
	i.owner.mu.Lock()
	known := i.owner.known[id] == pin
	i.owner.mu.Unlock()
	if !known {
		return nil, nil, errors.New(p.NetworkIdentityMismatch)
	}
	// Reuse an established pair during directory outage. Its TLS still verifies
	// the current borrower, and pair changes close its entire QUIC transport.
	i.mu.Lock()
	r := i.routes[t]
	if r != nil {
		select {
		case <-r.done:
			if r.agent != nil {
				select {
				case <-r.agent.changed:
					delete(i.routes, t)
					r.cancel()
					r = nil
				default:
				}
			}
		default:
		}
	}
	i.mu.Unlock()
	generation, alive := i.generations()
	if !alive {
		return nil, nil, ErrClosed
	}
	if r != nil && r.local == p.NetworkUint(generation) {
		return i.wait(ctx, r)
	}
	a, found, err := i.owner.client.Lookup(ctx, id, pin, string(PeerData), 0)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, ErrNoRoute
	}
	capable := false
	for _, c := range a.Capabilities {
		if c == "quic_ice_v1" {
			capable = true
		}
	}
	if !capable {
		return nil, nil, ErrNoRoute
	}
	r, start, err := i.allocate(t, p.NetworkUint(generation), a.Generation, randomNetworkID())
	if err != nil {
		return nil, nil, err
	}
	if start {
		go i.establish(t, r, nil)
	}
	return i.wait(ctx, r)
}
func (i *iceCoordinator) wait(ctx context.Context, r *iceRoute) (*QUICEndpoint, net.Addr, error) {
	select {
	case <-r.done:
		if r.err != nil {
			return nil, nil, r.err
		}
		select {
		case <-r.agent.changed:
			return nil, nil, ErrStale
		default:
		}
		return r.endpoint, r.address, nil
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-r.ctx.Done():
		return nil, nil, ErrStale
	case <-i.owner.ctx.Done():
		return nil, nil, ErrClosed
	}
}
func (i *iceCoordinator) event(event p.NetworkOfferRequest) {
	if event.Offer.ICE == nil || event.VerifyICE() != nil {
		return
	}
	t := i.target(event.Proof.Sender, event.Proof.SenderPin)
	i.owner.mu.Lock()
	known := i.owner.known[event.Proof.Sender] == event.Proof.SenderPin
	i.owner.mu.Unlock()
	if !known {
		return
	}
	generation, alive := i.generations()
	if !alive || event.Offer.TargetGeneration != p.NetworkUint(generation) {
		return
	}
	if event.Offer.ICE.Mode == "accept" {
		i.mu.Lock()
		r := i.routes[t]
		if r != nil && r.session == event.Proof.Session && r.local == event.Offer.TargetGeneration && r.remote == event.Offer.SenderGeneration {
			select {
			case r.accepts <- event:
			default:
			}
		}
		i.mu.Unlock()
		return
	}
	session := event.Proof.Session
	if event.Offer.ICE.Mode == "request" {
		session = randomNetworkID()
	}
	r, start, err := i.allocate(t, event.Offer.TargetGeneration, event.Offer.SenderGeneration, session)
	if err != nil {
		return
	}
	// A request made by the controlled peer and the controlling offer are distinct
	// service operations, but reuse one local pair slot and one controlling nonce.
	if !start && event.Offer.ICE.Mode == "offer" && !i.smaller(t) {
		select {
		case r.accepts <- event:
		default:
		}
	}
	if start {
		go i.establish(t, r, &event)
	}
}
func (i *iceCoordinator) establish(t Target, r *iceRoute, incoming *p.NetworkOfferRequest) {
	var err error
	defer func() { i.complete(t, r, err) }()
	work, cancel := context.WithTimeout(r.ctx, 12*time.Second)
	defer cancel()
	q := p.NetworkProof{Kind: "offer", Target: hex.EncodeToString(t.Device[:]), TargetPin: hex.EncodeToString(t.Pin[:]), Purpose: string(PeerData), Session: r.session, Role: "initiator"}
	// Refuse an empty local gather before asking the other role for an offer.
	// Otherwise a private-only/no-STUN peer waits the entire coordination budget
	// even though it can never supply a usable candidate, delaying safe fallback.
	r.agent, err = NewICESession(work, i.owner.client.selection.Profile.STUN, i.options.Interfaces, i.options.Net)
	if err != nil {
		return
	}
	// Release the bounded service coordination record after establishment. Active
	// pair lifetime is independent of service credentials and directory leases.
	defer func() {
		cleanup, end := context.WithTimeout(i.owner.ctx, time.Second)
		defer end()
		_ = i.owner.client.Release(cleanup, q)
	}()
	local := r.agent.Description("offer")
	controlling := i.smaller(t)
	var remote p.NetworkICE
	if !controlling && incoming == nil {
		request := p.NetworkICE{Mode: "request", Candidates: []string{}}
		err = i.owner.client.Exchange(work, q, p.NetworkOffer{SenderGeneration: r.local, TargetGeneration: r.remote, Candidates: []p.NetworkCandidate{}, ICE: &request})
		if err != nil {
			return
		}
		select {
		case event := <-r.accepts:
			if event.Offer.ICE == nil || event.Offer.ICE.Mode != "offer" || event.Offer.SenderGeneration != r.remote || event.Offer.TargetGeneration != r.local {
				err = ErrStale
				return
			}
			incoming = &event
			i.mu.Lock()
			r.session = event.Proof.Session
			i.mu.Unlock()
		case <-work.Done():
			err = ErrICEChecks
			return
		}
	}
	if controlling {
		err = i.owner.client.Exchange(work, q, p.NetworkOffer{SenderGeneration: r.local, TargetGeneration: r.remote, Candidates: []p.NetworkCandidate{}, ICE: &local})
		if err != nil {
			return
		}
		select {
		case event := <-r.accepts:
			remote = *event.Offer.ICE
		case <-work.Done():
			err = ErrICEChecks
			return
		}
	} else {
		remote = *incoming.Offer.ICE
		local.Mode = "accept"
		q.Kind = "accept"
		q.Role = "responder"
		q.Session = r.session
		err = i.owner.client.Exchange(work, q, p.NetworkOffer{SenderGeneration: r.local, TargetGeneration: r.remote, Candidates: []p.NetworkCandidate{}, ICE: &local})
		if err != nil {
			return
		}
	}
	var packet *PairPacketConn
	packet, err = r.agent.Connect(work, remote, controlling)
	if err != nil {
		return
	}
	r.endpoint, err = newQUICEndpoint(packet, i.options.TLS, i.options.Peer, i.options.admission)
	if err != nil {
		return
	}
	r.address = packet.remoteAddr()
	generation, alive := i.generations()
	if !alive || p.NetworkUint(generation) != r.local {
		err = ErrStale
		return
	}
	// This worker owns retirement and is joined on endpoint shutdown. Avoid closing
	// from inside Pion callbacks; doing so would deadlock its event loop.
	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		err = ErrClosed
		return
	}
	i.wg.Add(1)
	i.mu.Unlock()
	go func() {
		defer i.wg.Done()
		select {
		case <-r.agent.changed:
		case <-r.ctx.Done():
		}
		i.mu.Lock()
		if i.routes[t] == r {
			delete(i.routes, t)
		}
		i.mu.Unlock()
		_ = r.endpoint.Close()
		_ = r.agent.Close()
	}()
}
func (i *iceCoordinator) close() {
	i.mu.Lock()
	i.closed = true
	i.mu.Unlock()
	i.wg.Wait()
}

func (i *iceCoordinator) retireGeneration(generation uint64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for t, r := range i.routes {
		if uint64(r.local) != generation {
			r.cancel()
			delete(i.routes, t)
		}
	}
}

func (i *iceCoordinator) addPeer(id, pin string) {
	i.owner.mu.Lock()
	i.owner.known[id] = pin
	i.owner.mu.Unlock()
}
func (i *iceCoordinator) setGeneration(generation uint64) {
	i.owner.mu.Lock()
	i.owner.generation = generation
	i.owner.mu.Unlock()
	i.retireGeneration(generation)
}
