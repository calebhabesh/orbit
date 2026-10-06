package rendezvous

import (
	"sync/atomic"

	p "github.com/calebhabesh/file-sync/internal/protocol"
)

// counters are sanitized operator totals. They never retain addresses,
// identities, pins, sessions, timing of individual devices or payload bytes.
type counters struct {
	quota, invalid, expired, untrusted, other atomic.Uint64
	relayBytes                                atomic.Uint64
}

func (c *counters) refused(code string) {
	switch code {
	case p.NetworkQuota:
		c.quota.Add(1)
	case p.NetworkInvalidRequest, p.NetworkUnsupported:
		c.invalid.Add(1)
	case p.NetworkProfileExpired:
		c.expired.Add(1)
	case p.NetworkProfileUntrusted:
		c.untrusted.Add(1)
	default:
		c.other.Add(1)
	}
}

// ProfileMetrics describes one served epoch; all fields are public profile data.
type ProfileMetrics struct {
	Epoch   uint64
	Expires uint64
	Valid   bool
}

// Metrics is a bounded snapshot for monitoring and alerting.
type Metrics struct {
	Profiles                      []ProfileMetrics
	Records, Sessions, Relays     int
	Controls, Pending, Challenges int
	RateKeys, SourceBuckets       int
	RefusedQuota, RefusedInvalid  uint64
	RefusedExpired, RefusedOther  uint64
	RefusedUntrusted, RelayBytes  uint64
	ServiceBytesPerSecond         int64
	DeviceBytesPerSecond          int64
}

func (s *Service) Metrics() Metrics {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := uint64(s.now().Unix())
	m := Metrics{Records: len(s.records), Sessions: len(s.sessions), Controls: len(s.controls), Pending: len(s.pending), Challenges: len(s.challenges), RateKeys: len(s.rates), SourceBuckets: len(s.sources), ServiceBytesPerSecond: s.relayLimits.ServiceBytesPerSecond, DeviceBytesPerSecond: s.relayLimits.DeviceBytesPerSecond}
	for _, v := range s.sessions {
		if v.relay != nil {
			m.Relays++
		}
	}
	for _, sp := range s.served {
		_, code := s.profile(digestOf(sp), now)
		m.Profiles = append(m.Profiles, ProfileMetrics{Epoch: uint64(sp.selection.Profile.Epoch), Expires: uint64(sp.selection.Profile.Expires), Valid: code == ""})
	}
	if len(m.Profiles) == 2 && m.Profiles[0].Epoch > m.Profiles[1].Epoch {
		m.Profiles[0], m.Profiles[1] = m.Profiles[1], m.Profiles[0]
	}
	m.RefusedQuota, m.RefusedInvalid, m.RefusedExpired = s.stats.quota.Load(), s.stats.invalid.Load(), s.stats.expired.Load()
	m.RefusedUntrusted, m.RefusedOther, m.RelayBytes = s.stats.untrusted.Load(), s.stats.other.Load(), s.stats.relayBytes.Load()
	return m
}
func digestOf(sp *servedProfile) string { d, _ := sp.selection.Digest(); return d }
