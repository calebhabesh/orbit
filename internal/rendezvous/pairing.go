package rendezvous

import (
	"net"
	"net/http"

	p "github.com/calebhabesh/orbit/internal/protocol"
)

const MaxPairingMailboxes = 128
const MaxPairingPerKey = 4

type mailbox struct {
	owner, joiner                                    actor
	session, profile, state, first, reply, encrypted string
	expires                                          uint64
}

// pairing runs under Service.mu. A claimed mailbox can never return to open,
// and can receive only one PAKE proof. No password/verifier is stored here.
func (s *Service) pairing(q p.PairingRequest, r *http.Request, now uint64) (any, string) {
	if !q.Valid() || q.Proof.Kind != "pairing" || q.Proof.Purpose != "enrollment" || q.Proof.Generation != 0 {
		return nil, p.NetworkInvalidRequest
	}
	if code := s.admit(q.Proof, q.Canonical(), now); code != "" {
		return nil, code
	}
	if s.mailboxes == nil {
		s.mailboxes = map[string]*mailbox{}
		s.pairSources = map[string]bucket{}
	}
	for name, m := range s.mailboxes {
		if now >= m.expires {
			delete(s.mailboxes, name)
		}
	}
	if q.Action == "create" || q.Action == "claim" {
		host, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil {
			host = r.RemoteAddr
		}
		for k, v := range s.pairSources {
			if s.now().Sub(v.at).Seconds() > 600 {
				delete(s.pairSources, k)
			}
		}
		b, ok := s.pairSources[host]
		if !ok && len(s.pairSources) >= 128 {
			return nil, p.NetworkQuota
		}
		b, ok = consume(b, s.now(), 5, 1.0/10)
		s.pairSources[host] = b
		if !ok {
			return nil, p.NetworkQuota
		}
	}
	m := s.mailboxes[q.Mailbox]
	if q.Action == "create" {
		if m != nil {
			return nil, p.PairingUsed
		}
		if len(s.mailboxes) >= MaxPairingMailboxes {
			return nil, p.NetworkQuota
		}
		count := 0
		for _, other := range s.mailboxes {
			if other.owner.pin == q.Proof.SenderPin {
				count++
			}
		}
		if count >= MaxPairingPerKey {
			return nil, p.NetworkQuota
		}
		if q.Session == "" || uint64(q.Expires) <= now || uint64(q.Expires)-now > 600 || len(q.Data) != 64 {
			return nil, p.NetworkInvalidRequest
		}
		m = &mailbox{owner: who(q.Proof), session: q.Session, profile: q.Proof.Profile, state: "open", first: q.Data, expires: uint64(q.Expires)}
		s.mailboxes[q.Mailbox] = m
		return p.PairingResult{Version: "1", State: m.state, Session: m.session, Expires: p.NetworkUint(m.expires)}, ""
	}
	if m == nil || m.profile != q.Proof.Profile {
		return nil, p.PairingUnavailable
	}
	caller := who(q.Proof)
	if q.Action == "claim" {
		if m.state != "open" {
			return nil, p.PairingUsed
		}
		if q.Session != "" || q.Data != "" || q.Expires != 0 || caller.pin == m.owner.pin {
			return nil, p.NetworkInvalidRequest
		}
		m.joiner, m.state = caller, "claimed"
		return p.PairingResult{Version: "1", State: m.state, Session: m.session, Expires: p.NetworkUint(m.expires), Device: m.owner.device, Pin: m.owner.pin, Data: m.first}, ""
	}
	if q.Session != m.session || (caller != m.owner && caller != m.joiner) {
		return nil, p.PairingUnavailable
	}
	out := p.PairingResult{Version: "1", State: m.state, Session: m.session, Expires: p.NetworkUint(m.expires)}
	switch q.Action {
	case "poll":
		if q.Data != "" {
			return nil, p.NetworkInvalidRequest
		}
		if caller == m.owner {
			out.Device, out.Pin, out.Data = m.joiner.device, m.joiner.pin, m.reply
		} else {
			out.Device, out.Pin = m.owner.device, m.owner.pin
			if m.state == "delivered" {
				out.Data = m.encrypted
				delete(s.mailboxes, q.Mailbox)
			}
		}
	case "respond":
		if caller != m.joiner || m.state != "claimed" || len(q.Data) != 128 {
			return nil, p.PairingUsed
		}
		m.reply, m.state = q.Data, "proof"
	case "deliver":
		if caller != m.owner || m.state != "proof" || len(q.Data) < 56 {
			return nil, p.PairingUsed
		}
		m.encrypted, m.state = q.Data, "delivered"
		m.first, m.reply = "", ""
	case "burn":
		if caller != m.owner {
			return nil, p.PairingUnavailable
		}
		delete(s.mailboxes, q.Mailbox)
	default:
		return nil, p.NetworkInvalidRequest
	}
	return out, ""
}
