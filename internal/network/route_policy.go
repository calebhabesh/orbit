package network

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

type preparedTLSKey struct{}
type directDeadlineKey struct{}
type preparedTLS struct {
	mu   sync.Mutex
	conn net.Conn
}

func (p *preparedTLS) take() net.Conn {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.conn
	p.conn = nil
	return c
}
func (p *preparedTLS) closeUnused() {
	if c := p.take(); c != nil {
		_ = c.Close()
	}
}

// selectRoute races only authenticated handshakes. Exactly one HTTP transport
// receives the request; uncertain HTTP delivery is never replayed across routes.
// ICE's runtime-owned, bounded gathering may finish after relay wins so a later
// direct reprobe can borrow that pair. Cancellation still joins these dialers.
func (t *Transport) selectRoute(ctx context.Context) (*preparedTLS, bool, error) {
	t.routeMu.Lock()
	defer t.routeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	revision := t.routeManager.candidateRevision(t.routeTarget)
	if revision != t.candidateRevision {
		t.candidateRevision = revision
		t.tcpUntil = time.Time{}
	}
	if t.routeManager.now().Before(t.tcpUntil) || (t.route.stream != nil && !t.routeManager.directProbeAllowed(t.routeTarget)) {
		return nil, false, nil
	}
	work, cancel := context.WithTimeout(ctx, time.Duration(t.routeManager.timing.CycleMS)*time.Millisecond)
	stop := context.AfterFunc(t.routeManager.lifetime, cancel)
	defer func() { stop(); cancel() }()
	candidates := t.routeManager.directCandidates(work, t.routeTarget)
	work = context.WithValue(work, directDeadlineKey{}, time.Now().Add(time.Duration(t.routeManager.timing.HeadStartMS)*time.Millisecond))
	type result struct {
		conn net.Conn
		quic bool
		err  error
	}
	results := make(chan result, 2)
	go func() {
		_, _, err := t.quic.connectionCandidates(work, candidates, true)
		results <- result{quic: true, err: err}
	}()
	go func() {
		hasTCP, hasUDP := false, false
		for _, candidate := range candidates {
			if candidate.Transport == "tcp" {
				hasTCP = true
			}
			if candidate.Transport == "udp" {
				hasUDP = true
			}
		}
		if hasTCP && hasUDP {
			// Give advertised native QUIC the same bounded stagger as families.
			// TCP still participates before the shared relay head-start deadline.
			timer := time.NewTimer(DirectTransportStagger)
			select {
			case <-work.Done():
				timer.Stop()
				results <- result{err: work.Err()}
				return
			case <-timer.C:
			}
		}
		if !hasTCP && t.route.stream != nil {
			timer := time.NewTimer(time.Duration(t.routeManager.timing.HeadStartMS) * time.Millisecond)
			select {
			case <-work.Done():
				timer.Stop()
				results <- result{err: work.Err()}
				return
			case <-timer.C:
			}
		}
		c, err := t.routeManager.dialTLSCandidates(work, t.routeTarget, t.route, t.routeTrust, candidates)
		results <- result{conn: c, err: err}
	}()
	var winner result
	chosen := false
	var lastErr error
	for range 2 {
		next := <-results
		if !chosen && next.err == nil && work.Err() == nil {
			chosen = true
			winner = next
			cancel()
		} else {
			if next.conn != nil {
				_ = next.conn.Close()
			}
			if next.err != nil {
				// A typed quota/service refusal must survive cancellation of the
				// competing ICE waiter; the scheduler needs its quiet refill.
				if lastErr == nil || errors.Is(lastErr, context.Canceled) || errors.Is(lastErr, context.DeadlineExceeded) {
					lastErr = next.err
				}
				var service *ServiceError
				if errors.As(next.err, &service) {
					lastErr = next.err
					if service.Code == "QUOTA_EXCEEDED" {
						cancel()
					}
				}
			}
			if next.quic && next.err == nil {
				t.quic.close()
			}
		}
	}
	if !chosen {
		if lastErr == nil {
			lastErr = work.Err()
		}
		return nil, false, lastErr
	}
	if winner.quic {
		return nil, true, nil
	}
	t.tcpUntil = t.routeManager.now().Add(time.Duration(t.routeManager.timing.ProbeMS) * time.Millisecond)
	return &preparedTLS{conn: winner.conn}, false, nil
}

type directCooldown struct {
	failures uint8
	next     time.Time
}

func (m *ConnectionManager) directProbeAllowed(t Target) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.now().Before(m.directCooldowns[t].next)
}
func (m *ConnectionManager) directProbeSucceeded(t Target) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.directCooldowns, t)
}
func (m *ConnectionManager) directProbeFailed(t Target) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, known := m.routes[t]; !known || m.closed {
		return
	}
	cooldown := m.directCooldowns[t]
	// Multiple requests sharing this generation cannot slide the deadline.
	if m.now().Before(cooldown.next) {
		return
	}
	if cooldown.failures < 3 {
		cooldown.failures++
	}
	delay := time.Duration(m.timing.ProbeMS) * time.Millisecond * time.Duration(1<<uint(cooldown.failures-1))
	if delay > time.Duration(m.timing.CooldownMS)*time.Millisecond {
		delay = time.Duration(m.timing.CooldownMS) * time.Millisecond
	}
	// Stable peer jitter separates simultaneous probes without new persistent data.
	jitter := time.Duration(t.Pin[0]%16) * time.Second
	cooldown.next = m.now().Add(delay + jitter)
	m.directCooldowns[t] = cooldown
}

func (m *ConnectionManager) candidateRevision(t Target) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.candidateRevisions[t]
}
