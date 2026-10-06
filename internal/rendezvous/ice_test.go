package rendezvous

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/network"
	p "github.com/calebhabesh/file-sync/internal/protocol"
)

func TestWANW10SignedICESessionReplayAndIsolation(t *testing.T) {
	f := newFixture(t, true)
	a, b := identity(t, 1, nil), identity(t, 2, nil)
	ca, cb := f.client(t, a, f.roots), f.client(t, b, f.roots)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for _, c := range []*network.ServiceClient{ca, cb} {
		ann := announcement(f, 1)
		ann.Capabilities = append(ann.Capabilities, "quic_ice_v1")
		if e := c.Announce(ctx, "peer_data", nonce(), ann); e != nil {
			t.Fatal(e)
		}
	}
	ac, e := ca.Control(ctx, "peer_data", map[string]string{b.id: b.pin}, false)
	if e != nil {
		t.Fatal(e)
	}
	defer ac.Close()
	bc, e := cb.Control(ctx, "peer_data", map[string]string{a.id: a.pin}, false)
	if e != nil {
		t.Fatal(e)
	}
	defer bc.Close()
	desc := p.NetworkICE{Mode: "offer", Ufrag: "abcd", Password: strings.Repeat("a", 32), Candidates: []string{"1 1 udp 2130706431 11.23.45.1 12345 typ host"}}
	offer := p.NetworkOffer{SenderGeneration: 1, TargetGeneration: 1, Candidates: []p.NetworkCandidate{}, ICE: &desc}
	q := p.NetworkProof{Kind: "offer", Target: b.id, TargetPin: b.pin, Purpose: "peer_data", Session: nonce(), Role: "initiator", Operation: nonce()}
	if e = ca.Exchange(ctx, q, offer); e != nil {
		t.Fatal(e)
	}
	event, e := bc.Next(ctx)
	if e != nil {
		t.Fatal(e)
	}
	// Signed fields are immutable even when the attacker retains a valid proof.
	for _, change := range []func(*p.NetworkOfferRequest){func(r *p.NetworkOfferRequest) { r.Proof.TargetPin = strings.Repeat("f", 64) }, func(r *p.NetworkOfferRequest) { r.Proof.Session = nonce() }, func(r *p.NetworkOfferRequest) { r.Proof.Role = "responder" }, func(r *p.NetworkOfferRequest) { r.Offer.SenderGeneration++ }, func(r *p.NetworkOfferRequest) {
		r.Offer.ICE = &p.NetworkICE{Mode: "offer", Ufrag: "abcd", Password: strings.Repeat("b", 32), Candidates: desc.Candidates}
	}} {
		bad := event
		change(&bad)
		if bad.Verify(true, uint64(f.now.Load())) == nil {
			t.Fatal("changed signed ICE accepted")
		}
	}
	if event.Verify(true, uint64(event.Proof.Expires)) == nil {
		t.Fatal("expired proof")
	}
	if e = ca.Exchange(ctx, q, offer); e != nil {
		t.Fatal("exact replay", e)
	}
	short, end := context.WithTimeout(ctx, 50*time.Millisecond)
	if _, e = bc.Next(short); e != context.DeadlineExceeded {
		t.Fatal("replay emitted another offer", e)
	}
	end()
	f.now.Add(2)
	reply := desc
	reply.Mode = "accept"
	answer := offer
	answer.ICE = &reply
	accept := p.NetworkProof{Kind: "accept", Target: a.id, TargetPin: a.pin, Purpose: "peer_data", Session: q.Session, Role: "responder", Operation: nonce()}
	wrong := accept
	wrong.Session = nonce()
	if e = cb.Exchange(ctx, wrong, answer); e == nil {
		t.Fatal("wrong session")
	}
	wrong = accept
	wrong.TargetPin = identity(t, 3, nil).pin
	if e = cb.Exchange(ctx, wrong, answer); e == nil {
		t.Fatal("wrong pin")
	}
	if e = cb.Exchange(ctx, accept, answer); e != nil {
		t.Fatal(e)
	}
	if _, e = ac.Next(ctx); e != nil {
		t.Fatal(e)
	}
	f.now.Add(2)
	if _, e = ca.Reserve(ctx, q); e == nil {
		t.Fatal("ICE became relay attachment")
	}
	if e = ca.Release(ctx, q); e != nil {
		t.Fatal(e)
	}
	accept.Operation = nonce()
	if e = cb.Exchange(ctx, accept, answer); e == nil {
		t.Fatal("released session replay")
	}
	// A valid new signature with obsolete generation cannot authorize candidates.
	f.now.Add(2)
	q.Operation = nonce()
	q.Session = nonce()
	offer.TargetGeneration = 2
	if e = ca.Exchange(ctx, q, offer); e == nil {
		t.Fatal("wrong target generation")
	}
}
