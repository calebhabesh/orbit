package network

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"time"

	p "github.com/calebhabesh/file-sync/internal/protocol"
)

// RelayEndpoint coordinates a purpose-specific listener, using reviewed pins.
// Construct only after explicit network-policy/profile review. Folder authority
// stays in the isolated HTTP server. The caller owns announcement renewal and
// rebuilding on generation/service changes (the roaming packet).
type RelayEndpoint struct {
	ownsICE    bool
	ice        *iceCoordinator
	client     *ServiceClient
	purpose    Purpose
	generation uint64
	known      map[string]string
	listener   *StreamListener
	control    *ServiceControl
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	pending    map[string]chan p.NetworkOfferRequest
	workers    chan struct{}
	wg         sync.WaitGroup
	done       chan struct{}
}

func NewRelayEndpoint(ctx context.Context, client *ServiceClient, purpose Purpose, generation uint64, known map[string]string, allowUnknown bool, listener *StreamListener, iceOptions ...*ICEOptions) (*RelayEndpoint, error) {
	if client == nil || listener == nil || generation == 0 || (purpose != PeerData && purpose != Enrollment) {
		return nil, &ServiceError{Code: p.NetworkInvalidRequest}
	}
	if len(iceOptions) > 1 || (len(iceOptions) == 1 && iceOptions[0] != nil && (iceOptions[0].TLS == nil || iceOptions[0].Peer == nil)) {
		return nil, &ServiceError{Code: p.NetworkInvalidRequest}
	}
	pins := map[string]string{}
	for id, pin := range known {
		pins[id] = pin
	}
	life, cancel := context.WithCancel(ctx)
	control, err := client.Control(life, string(purpose), pins, allowUnknown)
	if err != nil {
		cancel()
		return nil, err
	}
	slots := MaxDataTunnels
	if purpose == Enrollment {
		slots = MaxUnknownEnrollmentTunnels
	}
	e := &RelayEndpoint{client: client, purpose: purpose, generation: generation, known: pins, listener: listener, control: control, ctx: life, cancel: cancel, pending: map[string]chan p.NetworkOfferRequest{}, workers: make(chan struct{}, slots), done: make(chan struct{})}
	if purpose == PeerData && len(iceOptions) > 0 && iceOptions[0] != nil {
		if iceOptions[0].shared != nil {
			e.ice = iceOptions[0].shared
		} else {
			e.ice = newICECoordinator(e, *iceOptions[0])
			e.ownsICE = true
		}
	}
	go e.run()
	return e, nil
}
func (e *RelayEndpoint) run() {
	defer close(e.done)
	defer e.wg.Wait()
	defer func() {
		if e.ice != nil && e.ownsICE {
			e.ice.close()
		}
	}()
	defer e.cancel()
	for {
		event, err := e.control.Next(e.ctx)
		if err != nil {
			return
		}
		if event.Offer.ICE != nil {
			if e.ice != nil {
				e.ice.event(event)
			}
			continue
		}
		if event.Proof.Kind == "accept" {
			e.mu.Lock()
			waiting := e.pending[event.Proof.Session]
			e.mu.Unlock()
			if waiting != nil {
				select {
				case waiting <- event:
				default:
				}
			}
			continue
		}
		e.mu.Lock()
		generation := e.generation
		e.mu.Unlock()
		if uint64(event.Offer.TargetGeneration) != generation {
			continue
		}
		select {
		case e.workers <- struct{}{}:
		default:
			continue
		}
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			defer func() { <-e.workers }()
			work, cancel := context.WithTimeout(e.ctx, AttachmentLifetime)
			defer cancel()
			// Accept, reserve and attach: refuse locally now rather than midway.
			work, err := e.client.BeginSetup(work, 3)
			if err != nil {
				return
			}
			q := p.NetworkProof{Kind: "accept", Target: event.Proof.Sender, TargetPin: event.Proof.SenderPin, Purpose: string(e.purpose), Session: event.Proof.Session, Role: "responder"}
			offer := p.NetworkOffer{SenderGeneration: p.NetworkUint(generation), TargetGeneration: event.Offer.SenderGeneration, Candidates: []p.NetworkCandidate{}}
			if e.retryQuota(work, func() error { return e.client.Exchange(work, q, offer) }) != nil {
				return
			}
			attached := false
			defer func() {
				if !attached {
					cleanup, end := context.WithTimeout(e.ctx, InnerHandshakeTimeout)
					defer end()
					_ = e.client.Release(cleanup, q)
				}
			}()
			var t p.RelayAttachment
			err = e.retryQuota(work, func() error {
				var reserveErr error
				t, reserveErr = e.client.Reserve(work, q)
				return reserveErr
			})
			if err != nil {
				return
			}
			attached = e.retryQuota(work, func() error { return e.client.OfferRelay(work, t, e.purpose, e.listener) }) == nil
		}()
	}
}
func (e *RelayEndpoint) Dial(ctx context.Context, t Target) (net.Conn, error) {
	return e.dial(ctx, t, nil)
}

func (e *RelayEndpoint) dial(ctx context.Context, t Target, cached *p.NetworkAnnouncement) (net.Conn, error) {
	if t.Validate() != nil || t.Purpose != e.purpose || hex.EncodeToString(t.Profile[:]) != e.client.digest {
		return nil, &ServiceError{Code: p.NetworkPurposeMismatch}
	}
	id, pin := hex.EncodeToString(t.Device[:]), hex.EncodeToString(t.Pin[:])
	// Initial enrollment targets are also explicitly supplied reviewed pins.
	e.mu.Lock()
	known := e.known[id] == pin
	e.mu.Unlock()
	if !known {
		return nil, &ServiceError{Code: p.NetworkIdentityMismatch}
	}
	work, cancel := context.WithTimeout(ctx, AttachmentLifetime)
	defer cancel()
	stop := context.AfterFunc(e.ctx, cancel)
	defer stop()
	var a p.NetworkAnnouncement
	var err error
	if cached != nil && uint64(cached.Expires) > uint64(time.Now().Unix()) {
		a = *cached
	} else {
		var found bool
		a, found, err = e.client.Lookup(work, id, pin, string(e.purpose), 0)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, &ServiceError{Code: p.NetworkUnavailable}
		}
	}
	if !a.Relay {
		return nil, &ServiceError{Code: p.NetworkUnavailable}
	}
	q := p.NetworkProof{Kind: "offer", Target: id, TargetPin: pin, Purpose: string(e.purpose), Session: randomNetworkID(), Role: "initiator"}
	waiting := make(chan p.NetworkOfferRequest, 1)
	e.mu.Lock()
	if e.ctx.Err() != nil {
		e.mu.Unlock()
		return nil, ErrClosed
	}
	limit := MaxDataTunnels
	if e.purpose == Enrollment {
		limit = MaxUnknownEnrollmentTunnels
	}
	if len(e.pending) >= limit {
		e.mu.Unlock()
		return nil, ErrBackpressure
	}
	e.pending[q.Session] = waiting
	e.mu.Unlock()
	success := false
	defer func() {
		e.mu.Lock()
		delete(e.pending, q.Session)
		e.mu.Unlock()
		if !success {
			cleanup, end := context.WithTimeout(e.ctx, InnerHandshakeTimeout)
			defer end()
			_ = e.client.Release(cleanup, q)
		}
	}()
	e.mu.Lock()
	generation := e.generation
	e.mu.Unlock()
	offer := p.NetworkOffer{SenderGeneration: p.NetworkUint(generation), TargetGeneration: a.Generation, Candidates: []p.NetworkCandidate{}}
	// Offer, reserve and attach: refuse locally now rather than midway.
	if work, err = e.client.BeginSetup(work, 3); err != nil {
		return nil, err
	}
	if err = e.client.Exchange(work, q, offer); err != nil {
		return nil, err
	}
	select {
	case accept := <-waiting:
		if accept.Proof.Sender != id || accept.Proof.SenderPin != pin || accept.Offer.TargetGeneration != offer.SenderGeneration || accept.Offer.SenderGeneration != offer.TargetGeneration {
			return nil, &ServiceError{Code: p.NetworkIdentityMismatch}
		}
	case <-work.Done():
		return nil, &ServiceError{Code: p.NetworkUnavailable}
	}
	token, err := e.client.Reserve(work, q)
	if err != nil {
		return nil, err
	}
	conn, err := e.client.Attach(work, token)
	success = err == nil
	return conn, err
}
func (e *RelayEndpoint) Close() error { e.cancel(); _ = e.control.Close(); <-e.done; return nil }

func (e *RelayEndpoint) AddPeer(device, pin string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.control.AddPeer(device, pin); err != nil {
		return err
	}
	e.known[device] = pin
	if e.ice != nil && !e.ownsICE {
		e.ice.addPeer(device, pin)
	}
	return nil
}

func (e *RelayEndpoint) SetGeneration(generation uint64) {
	e.mu.Lock()
	e.generation = generation
	e.mu.Unlock()
	if e.ice != nil {
		if e.ownsICE {
			e.ice.retireGeneration(generation)
		} else {
			e.ice.setGeneration(generation)
		}
	}
}

// A remote responder cannot report its metadata refusal over the opaque stream
// before attachment exists. One quiet refill retries only an explicit quota
// refusal, keeping the exact session/role/generation, never uncertain peer HTTP.
func (e *RelayEndpoint) retryQuota(ctx context.Context, operation func() error) error {
	err := operation()
	var service *ServiceError
	if !errors.As(err, &service) || service.Code != p.NetworkQuota {
		return err
	}
	timer := time.NewTimer(ServiceRetryQuietPeriod)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	return operation()
}
