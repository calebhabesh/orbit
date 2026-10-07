package network

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"reflect"
	"time"

	p "github.com/calebhabesh/file-sync/internal/protocol"
)

type lanKey struct {
	target Target
	iface  int
}

type candidateLease struct {
	invalidated bool
	// peer marks LAN candidates received from an approved peer over its pinned
	// session rather than heard on the local link. They add direct attempts but
	// prove no locality, so they never suppress the public directory lookup.
	peer       bool
	candidates []p.NetworkCandidate
	generation uint64
	expires    time.Time
}
type candidateSource func(context.Context, Target, uint64) (p.NetworkAnnouncement, error)

// RegisterDirect accepts reviewed identity intent, never discovery-owned trust.
// Local-only callers leave source nil, so no public lookup can occur.
func (m *ConnectionManager) RegisterDirect(t Target, source candidateSource) error {
	if t.Validate() != nil {
		return errors.New("INVALID_TARGET")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	for known := range m.routes {
		if known.Device == t.Device && known.Purpose == t.Purpose && known.Pin != t.Pin {
			return errors.New("IDENTITY_MISMATCH")
		}
	}
	if _, ok := m.routes[t]; !ok {
		if len(m.routes) >= 128 {
			return ErrBackpressure
		}
		m.sequence++
		m.routes[t] = manualRoute{origin: LogicalOrigin(t), revision: m.sequence}
	}
	if _, exists := m.sources[t]; !exists {
		r := m.routes[t]
		m.sequence++
		r.revision = m.sequence
		m.routes[t] = r
		m.pruneLocked()
	}
	m.sources[t] = source
	return nil
}

// SetCandidates installs only verified, scope-checked discovery/service records
// for a previously reviewed target. It cannot create a route or change a pin.
func (m *ConnectionManager) SetCandidates(t Target, candidates []p.NetworkCandidate, generation uint64, expires time.Time, public bool, interfaceIndex ...int) error {

	return m.setCandidates(t, candidates, generation, expires, public, false, 0, interfaceIndex...)
}

// SetPeerLANCandidates installs LAN candidates that an approved peer sent over
// its pinned session, scoped to the local interface whose prefix holds them.
func (m *ConnectionManager) SetPeerLANCandidates(t Target, candidates []p.NetworkCandidate, generation uint64, expires time.Time, interfaceIndex int) error {
	if len(candidates) > MaxPeerLANCandidates {
		return ErrBackpressure
	}
	return m.setCandidates(t, candidates, generation, expires, false, true, 0, interfaceIndex)
}
func (m *ConnectionManager) setCandidates(t Target, candidates []p.NetworkCandidate, generation uint64, expires time.Time, public, peer bool, localGeneration uint64, interfaceIndex ...int) error {
	if len(candidates) > p.NetworkMaxCandidates || generation == 0 || !m.now().Before(expires) || expires.After(m.now().Add(CandidateLifetime)) {
		return errors.New("INVALID_CANDIDATE")
	}
	seen := map[string]bool{}
	for _, c := range candidates {
		if c.Validate(public) != nil || (c.Transport != "tcp" && c.Transport != "udp") || seen[c.Transport+"/"+c.Address] || (public && c.Scope != "public") || (!public && c.Scope != "lan") {
			return errors.New("INVALID_CANDIDATE")
		}
		seen[c.Transport+"/"+c.Address] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if localGeneration != 0 && localGeneration != m.generation {
		return ErrStale
	}
	if _, ok := m.routes[t]; !ok {
		return ErrNoRoute
	}
	var old candidateLease
	var key lanKey
	if public {
		old = m.public[t]
		count := len(candidates)
		for key, lease := range m.lan {
			if key.target == t && !lease.peer && !lease.invalidated && m.now().Before(lease.expires) {
				count += len(lease.candidates)
			}
		}
		if count > p.NetworkMaxCandidates {
			return ErrBackpressure
		}
	} else {
		index := 0
		if len(interfaceIndex) > 0 {
			index = interfaceIndex[0]
		}
		if index < 0 {
			return errors.New("INTERFACE_SCOPE_MISMATCH")
		}
		key = lanKey{t, index}
		old = m.lan[key]
		if _, exists := m.lan[key]; !exists && len(m.lan) >= 128*MaxDiscoveryInterfaces {
			return ErrBackpressure
		}
		count := len(candidates)
		if lease := m.public[t]; m.now().Before(lease.expires) {
			count += len(lease.candidates)
		}
		for scoped, lease := range m.lan {
			if !m.now().Before(lease.expires) {
				lease.candidates = nil
				m.lan[scoped] = lease
			}
			if scoped.target == t && scoped != key && !lease.peer {
				count += len(lease.candidates)
			}
		}
		// Peer-sent leases are separately capped and excluded from this budget.
		if !peer && count > p.NetworkMaxCandidates {
			return ErrBackpressure
		}
	}
	// A live lease heard on the link already proves locality; keep it.
	if peer && !old.peer && !old.invalidated && m.now().Before(old.expires) && len(old.candidates) > 0 {
		return nil
	}
	if generation < old.generation || (generation == old.generation && (!old.expires.Equal(expires) || !reflect.DeepEqual(old.candidates, append([]p.NetworkCandidate(nil), candidates...)))) {
		return ErrStale
	}
	if !reflect.DeepEqual(old.candidates, append([]p.NetworkCandidate(nil), candidates...)) {
		m.candidateRevisions[t]++
		delete(m.directCooldowns, t)
		for key, pool := range m.pools {
			if key.target == t {
				pool.probeAfter = m.now()
			}
		}
	}
	lease := candidateLease{peer: peer && !public, candidates: append([]p.NetworkCandidate(nil), candidates...), generation: generation, expires: expires}
	if public {
		m.public[t] = lease
	} else {
		m.lan[key] = lease
	}
	return nil
}

func (m *ConnectionManager) directCandidates(ctx context.Context, t Target) []p.NetworkCandidate {
	m.mu.Lock()
	var out []p.NetworkCandidate
	linkLocal := false
	for key, lease := range m.lan {
		if key.target == t && !lease.invalidated && m.now().Before(lease.expires) {
			out = append(out, lease.candidates...)
			linkLocal = linkLocal || !lease.peer
		}
	}
	if lease := m.public[t]; !lease.invalidated && m.now().Before(lease.expires) {
		out = append(out, lease.candidates...)
	}
	localGeneration := m.generation
	source := m.sources[t]
	minimum := m.public[t].generation
	publicLeaseLive := !m.public[t].invalidated && m.now().Before(m.public[t].expires)
	m.mu.Unlock()
	// A live LAN lease heard on the link needs no directory, including during an
	// outage. Peer-sent LAN candidates may belong to an unrelated network with the
	// same private prefix, so public candidates are still gathered after them.
	if !linkLocal && !publicLeaseLive && source != nil {
		work, cancel := context.WithTimeout(ctx, time.Duration(m.timing.HeadStartMS)*time.Millisecond)
		defer cancel()
		a, err := source(work, t, minimum)
		if err == nil && uint64(a.Generation) >= minimum {
			candidates := []p.NetworkCandidate{}
			valid := len(a.Candidates) <= p.NetworkMaxCandidates
			nativeQUIC := false
			for _, capability := range a.Capabilities {
				if capability == "quic_http3_v1" {
					nativeQUIC = true
				}
			}
			for _, c := range a.Candidates {
				if c.Validate(true) != nil {
					valid = false
					break
				}
				if c.Transport == "tcp" || (c.Transport == "udp" && nativeQUIC && t.Purpose == PeerData) {
					candidates = append(candidates, c)
				}
			}
			if valid && m.setCandidates(t, candidates, uint64(a.Generation), time.Unix(int64(a.Expires), 0), true, false, localGeneration) == nil {
				out = append(out, candidates...)
			}
		}
	}
	if len(out) > p.NetworkMaxCandidates {
		out = out[:p.NetworkMaxCandidates]
	}
	// Alternate families, preserving order within each family.
	var v4, v6 []p.NetworkCandidate
	for _, c := range out {
		host, _, _ := net.SplitHostPort(c.Address)
		if net.ParseIP(host).To4() != nil {
			v4 = append(v4, c)
		} else {
			v6 = append(v6, c)
		}
	}
	out = nil
	for len(v4) > 0 || len(v6) > 0 {
		if len(v6) > 0 {
			out = append(out, v6[0])
			v6 = v6[1:]
		}
		if len(v4) > 0 {
			out = append(out, v4[0])
			v4 = v4[1:]
		}
	}
	return out
}

// dialTLS races at most two complete pinned TLS handshakes. A TCP accept alone
// never wins; losing sockets are closed and all workers joined before returning.
func (m *ConnectionManager) dialTLS(ctx context.Context, t Target, r manualRoute, trust *tls.Config) (net.Conn, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrClosed
	}
	m.tlsDialWG.Add(1)
	m.mu.Unlock()
	defer m.tlsDialWG.Done()
	workLife, cancelLife := context.WithCancel(ctx)
	stopLife := context.AfterFunc(m.lifetime, cancelLife)
	defer func() { stopLife(); cancelLife() }()
	ctx = workLife
	if r.stream != nil && !m.directProbeAllowed(t) {
		return m.dialTLSCandidates(ctx, t, r, trust, nil)
	}
	return m.dialTLSCandidates(ctx, t, r, trust, m.directCandidates(ctx, t))
}

func (m *ConnectionManager) dialTLSCandidates(ctx context.Context, t Target, r manualRoute, trust *tls.Config, allCandidates []p.NetworkCandidate) (net.Conn, error) {
	var candidates []p.NetworkCandidate
	if !m.directProbeAllowed(t) {
		allCandidates = nil
	}
	for _, c := range allCandidates {
		if c.Transport == "tcp" {
			candidates = append(candidates, c)
		}
	}
	if len(candidates) > 0 {
		budget := 3 * time.Second
		if r.stream != nil {
			budget = time.Duration(m.timing.HeadStartMS) * time.Millisecond
			if deadline, ok := ctx.Value(directDeadlineKey{}).(time.Time); ok {
				budget = time.Until(deadline)
			}
		}
		work, cancel := context.WithTimeout(ctx, budget)
		type result struct {
			c   net.Conn
			err error
		}
		results := make(chan result, MaxDirectAttempts)
		next, active := 0, 0
		start := func() {
			address := candidates[next].Address
			next++
			active++
			go func() {
				c, err := m.connect(work, t, address)
				if err == nil {
					tc := tls.Client(c, trust)
					err = tc.HandshakeContext(work)
					if err != nil {
						c.Close()
						c = nil
					} else {
						c = tc
					}
				}
				results <- result{c, err}
			}()
		}
		start()
		timer := time.NewTimer(200 * time.Millisecond)
		var winner net.Conn
		for active > 0 {
			select {
			case result := <-results:
				active--
				if result.err == nil && winner == nil && work.Err() == nil {
					winner = result.c
					cancel()
				} else if result.c != nil {
					result.c.Close()
				}
				if winner == nil && work.Err() == nil && next < len(candidates) {
					start()
				}
			case <-timer.C:
				if winner == nil && work.Err() == nil && active < MaxDirectAttempts && next < len(candidates) {
					start()
				}
			}
		}
		timer.Stop()
		cancel()
		if winner != nil {
			m.directProbeSucceeded(t)
			return winner, nil
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if r.stream == nil && r.address == "" {
		return nil, ErrNoRoute
	}
	if r.stream != nil {
		m.directProbeFailed(t)
	}
	c, err := m.connect(ctx, t, r.address, r.stream)
	if err != nil {
		return nil, err
	}
	tc := tls.Client(c, trust)
	work, cancel := context.WithTimeout(ctx, InnerHandshakeTimeout)
	defer cancel()
	if err = tc.HandshakeContext(work); err != nil {
		c.Close()
		return nil, err
	}
	return tc, nil
}

// KnownTargets copies bounded reviewed routing identities for local discovery.
func (m *ConnectionManager) KnownTargets() []Target {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Target, 0, len(m.routes))
	for t := range m.routes {
		out = append(out, t)
	}
	return out
}
