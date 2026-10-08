package network

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/calebhabesh/orbit/internal/pairing"
	p "github.com/calebhabesh/orbit/internal/protocol"
)

type PairingStatus struct {
	Code    string `json:"code,omitempty"` // deliberate owner transfer only
	State   string `json:"state"`
	Expires string `json:"expires"`
	Error   string `json:"error,omitempty"`
}
type pairingOffer struct {
	status  PairingStatus
	expires time.Time
}

func (c *ServiceClient) PairingExchange(ctx context.Context, q p.PairingRequest) (p.PairingResult, error) {
	work, done, e := c.beginSigned(ctx)
	if e != nil {
		return p.PairingResult{}, e
	}
	defer done()
	q.Proof, e = c.proof(work, p.NetworkProof{Kind: "pairing", Purpose: "enrollment"}, q.Canonical())
	if e != nil {
		return p.PairingResult{}, e
	}
	var out p.PairingResult
	if e = c.post(work, "pairing", q, &out); e != nil {
		return out, e
	}
	if out.Version != "1" {
		return out, errors.New(p.NetworkInvalidRequest)
	}
	return out, nil
}

// StartPairing owns the PAKE attempt beyond the requesting CLI/TUI lifetime.
// It keeps bounded, ephemeral state; daemon or service restart requires a new
// code. Replaying the same operation cannot start another password attempt.
func (c *ServiceClient) StartPairing(ctx context.Context, operation string, invitation []byte, until time.Time) (PairingStatus, error) {
	c.pairMu.Lock()
	defer c.pairMu.Unlock()
	if c.pairOffers == nil {
		c.pairOffers = map[string]*pairingOffer{}
	}
	for id, o := range c.pairOffers {
		if !c.now().Before(o.expires) {
			delete(c.pairOffers, id)
		}
	}
	if o := c.pairOffers[operation]; o != nil {
		return o.status, nil
	}
	if len(c.pairOffers) >= 4 || len(invitation) > 16<<10 {
		return PairingStatus{}, ErrBackpressure
	}
	expires := c.now().Add(10 * time.Minute)
	if until.Before(expires) {
		expires = until
	}
	if !expires.After(c.now()) {
		return PairingStatus{}, errors.New(p.PairingUnavailable)
	}
	code := pairing.Code()
	normalized, _ := pairing.Normalize(code)
	sid := randomNetworkID()
	sidBytes, _ := hex.DecodeString(sid)
	x := pairing.New([]byte(normalized[4:]), pairing.Context(c.origin, normalized[:4]), sidBytes)
	q := p.PairingRequest{Action: "create", Mailbox: normalized[:4], Session: sid, Expires: p.NetworkUint(expires.Unix()), Data: hex.EncodeToString(x.Public())}
	if _, e := c.PairingExchange(ctx, q); e != nil {
		return PairingStatus{}, e
	}
	o := &pairingOffer{PairingStatus{Code: code, State: "waiting", Expires: expires.UTC().Format(time.RFC3339)}, expires}
	c.pairOffers[operation] = o
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return PairingStatus{}, ErrClosed
	}
	c.wg.Add(1)
	c.mu.Unlock()
	go c.servePairing(o, q, x, append([]byte(nil), invitation...))
	return o.status, nil
}

func (c *ServiceClient) PairingStatus(operation string) PairingStatus {
	c.pairMu.Lock()
	defer c.pairMu.Unlock()
	if o := c.pairOffers[operation]; o != nil && c.now().Before(o.expires) {
		s := o.status
		s.Code = ""
		return s
	}
	return PairingStatus{State: "expired", Error: p.PairingUnavailable}
}

func (c *ServiceClient) servePairing(o *pairingOffer, q p.PairingRequest, x *pairing.Exchange, invitation []byte) {
	defer c.wg.Done()
	defer clear(invitation)
	ctx, cancel := context.WithDeadline(c.lifetime, o.expires)
	defer cancel()
	finish := func(state, code string) {
		c.pairMu.Lock()
		o.status.State, o.status.Error = state, code
		c.pairMu.Unlock()
	}
	q.Action, q.Data = "poll", ""
	for {
		if e := c.wait(ctx, 3*time.Second); e != nil {
			finish("expired", p.PairingUnavailable)
			return
		}
		r, e := c.PairingExchange(ctx, q)
		if e != nil {
			if e.Error() == p.NetworkQuota || errors.Is(e, ErrBackpressure) {
				continue
			}
			finish("expired", p.PairingUnavailable)
			return
		}
		if r.Session != q.Session {
			finish("failed", p.PairingUnavailable)
			return
		}
		if r.State != "proof" {
			continue
		}
		data, e := hex.DecodeString(r.Data)
		if e != nil || len(data) != 64 || p.NetworkHex(r.Device, 32) != nil || p.NetworkHex(r.Pin, 32) != nil {
			finish("failed", p.PairingWrong)
			return
		}
		key, e := x.Finish(data[:32], pairing.Identity(c.device, c.pin), pairing.Identity(r.Device, r.Pin), true)
		if e != nil || !pairing.Confirm(key, data[32:]) {
			q.Action = "burn"
			_, _ = c.PairingExchange(ctx, q)
			finish("failed", p.PairingWrong)
			return
		}
		q.Action, q.Data = "deliver", hex.EncodeToString(pairing.Seal(key, invitation))
		clear(key)
		if _, e = c.PairingExchange(ctx, q); e != nil {
			finish("failed", p.PairingUnavailable)
			return
		}
		finish("sent", "")
		return
	}
}

// JoinPairing claims once. Neither a bad password nor transport uncertainty
// retries the claim; the owner requests a fresh code instead.
func (c *ServiceClient) JoinPairing(ctx context.Context, code string) ([]byte, string, string, error) {
	n, e := pairing.Normalize(code)
	if e != nil {
		return nil, "", "", e
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	q := p.PairingRequest{Action: "claim", Mailbox: n[:4]}
	r, e := c.PairingExchange(ctx, q)
	if e != nil {
		return nil, "", "", e
	}
	sid, e := hex.DecodeString(r.Session)
	if e != nil || len(sid) != 32 || r.State != "claimed" || p.NetworkHex(r.Device, 32) != nil || p.NetworkHex(r.Pin, 32) != nil || uint64(r.Expires) <= uint64(c.now().Unix()) || uint64(r.Expires) > uint64(c.now().Add(10*time.Minute).Unix()) {
		return nil, "", "", errors.New(p.NetworkInvalidRequest)
	}
	peer, e := hex.DecodeString(r.Data)
	if e != nil {
		return nil, "", "", pairing.ErrProof
	}
	x := pairing.New([]byte(n[4:]), pairing.Context(c.origin, n[:4]), sid)
	key, e := x.Finish(peer, pairing.Identity(r.Device, r.Pin), pairing.Identity(c.device, c.pin), false)
	if e != nil {
		return nil, "", "", e
	}
	defer clear(key)
	q.Session, q.Action, q.Data = r.Session, "respond", hex.EncodeToString(append(x.Public(), pairing.Confirmation(key)...))
	if _, e = c.PairingExchange(ctx, q); e != nil {
		return nil, "", "", e
	}
	q.Action, q.Data = "poll", ""
	for {
		if e = c.wait(ctx, 3*time.Second); e != nil {
			return nil, "", "", errors.New(p.PairingUnavailable)
		}
		got, e := c.PairingExchange(ctx, q)
		if e != nil {
			if e.Error() == p.NetworkQuota || errors.Is(e, ErrBackpressure) {
				continue
			}
			return nil, "", "", e
		}
		if got.Session != q.Session || got.Device != r.Device || got.Pin != r.Pin {
			return nil, "", "", pairing.ErrProof
		}
		if got.State != "delivered" {
			continue
		}
		ciphertext, e := hex.DecodeString(got.Data)
		if e != nil {
			return nil, "", "", pairing.ErrProof
		}
		plain, e := pairing.Open(key, ciphertext)
		return plain, r.Device, r.Pin, e
	}
}
