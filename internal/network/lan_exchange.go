package network

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	p "github.com/calebhabesh/file-sync/internal/protocol"
)

// LANExchangePath is the peer endpoint carrying p.LANExchange in both directions.
const LANExchangePath = "/peer/v1/lan"

var (
	ErrLANExchangeUnavailable = errors.New("LAN_EXCHANGE_UNAVAILABLE")
	ErrLANExchangePeer        = errors.New("IDENTITY_MISMATCH")
)

// LANExchange lets two approved peers that already share a pinned session
// (usually through the relay) swap their signed LAN records. Multicast alone
// fails when a host firewall drops inbound discovery and direct traffic; after
// an exchange, whichever device accepts inbound connections is dialed directly
// by the other. Both directions are learned from one request. It runs only
// while LAN advertising is enabled and never sends records to the service.
type LANExchange struct {
	manager *ConnectionManager
	mu      sync.Mutex
	current *LANDiscovery
	due     map[Target]time.Time
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewLANExchange(manager *ConnectionManager) *LANExchange {
	return &LANExchange{manager: manager, due: map[Target]time.Time{}}
}

// Set replaces the discovery whose records and interface scope are used, for
// example after roaming, and schedules a prompt exchange with connected peers.
func (x *LANExchange) Set(d *LANDiscovery) {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.current = d
	x.due = map[Target]time.Time{}
}

func (x *LANExchange) discovery() *LANDiscovery {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.current
}

// Start runs one bounded sequential worker; trust supplies the pinned client
// configuration that ordinary peer requests use.
func (x *LANExchange) Start(ctx context.Context, trust func(Target) (*tls.Config, error)) {
	if x == nil || trust == nil {
		return
	}
	life, cancel := context.WithCancel(ctx)
	x.mu.Lock()
	x.cancel = cancel
	x.mu.Unlock()
	x.wg.Go(func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			x.round(life, trust)
			select {
			case <-life.Done():
				return
			case <-ticker.C:
			}
		}
	})
}

func (x *LANExchange) Close() {
	if x == nil {
		return
	}
	x.mu.Lock()
	cancel := x.cancel
	x.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	x.wg.Wait()
}

// round exchanges with peers that the last ordinary request reached, at most
// once per refresh period each. Unreached peers cause no traffic.
func (x *LANExchange) round(ctx context.Context, trust func(Target) (*tls.Config, error)) {
	for _, t := range x.manager.KnownTargets() {
		d := x.discovery()
		if ctx.Err() != nil || d == nil {
			return
		}
		if t.Purpose != PeerData || x.manager.Observe(t).Code != "CONNECTED" {
			continue
		}
		now := time.Now()
		x.mu.Lock()
		due := x.due[t]
		x.mu.Unlock()
		if now.Before(due) {
			continue
		}
		work, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := x.Exchange(work, d, t, trust)
		cancel()
		next := PeerLANRefresh
		if errors.Is(err, ErrLANExchangeUnavailable) {
			next = PeerLANUnsupported
		} else if err != nil {
			next = PeerLANRetry
		}
		x.mu.Lock()
		if x.current == d {
			x.due[t] = now.Add(next)
		}
		x.mu.Unlock()
	}
}

// Exchange sends this device's records to one peer over the manager's normal
// pinned route and installs the records in the reply.
func (x *LANExchange) Exchange(ctx context.Context, d *LANDiscovery, t Target, trust func(Target) (*tls.Config, error)) error {
	cfg, err := trust(t)
	if err != nil {
		return err
	}
	rt, err := x.manager.Transport(ctx, t, cfg)
	if err != nil {
		return err
	}
	body, err := json.Marshal(p.LANExchange{Version: "1", Records: d.Records()})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, LogicalOrigin(t)+LANExchangePath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := rt.RoundTrip(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxPeerLANRequestBytes+1))
	if err != nil {
		return err
	}
	switch {
	case response.StatusCode == http.StatusNotFound:
		// Older peers and peers without LAN advertising have no endpoint.
		return ErrLANExchangeUnavailable
	case response.StatusCode != http.StatusOK || len(data) > MaxPeerLANRequestBytes:
		return errors.New("LAN_EXCHANGE_FAILED")
	}
	var reply p.LANExchange
	if err = p.NetworkDecode(data, &reply); err != nil {
		return err
	}
	if err = reply.Validate(uint64(time.Now().Unix())); err != nil {
		return err
	}
	d.AcceptPeer(reply.Records, t.Pin)
	return nil
}

// ExchangePeerLAN answers the peer endpoint. The caller has already required
// a mutual-TLS client certificate; pin is that certificate's SPKI digest.
func (x *LANExchange) ExchangePeerLAN(pin [32]byte, body []byte) ([]byte, error) {
	d := x.discovery()
	if d == nil {
		return nil, ErrLANExchangeUnavailable
	}
	known := false
	for _, t := range x.manager.KnownTargets() {
		known = known || (t.Purpose == PeerData && t.Pin == pin)
	}
	if !known {
		return nil, ErrLANExchangePeer
	}
	if len(body) > MaxPeerLANRequestBytes {
		return nil, errors.New("INVALID_LAN_EXCHANGE")
	}
	var request p.LANExchange
	if err := p.NetworkDecode(body, &request); err != nil {
		return nil, err
	}
	if err := request.Validate(uint64(time.Now().Unix())); err != nil {
		return nil, err
	}
	d.AcceptPeer(request.Records, pin)
	return json.Marshal(p.LANExchange{Version: "1", Records: d.Records()})
}
