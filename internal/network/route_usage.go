package network

import (
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"slices"
	"sort"
	"sync"
	"time"
)

// Route classes counted by RouteUsage (RG2). They name the socket that carried
// a completed peer request, not the route the device hoped to use.
const (
	RouteLAN    = "lan"     // direct TCP to a private, loopback or link-local address
	RouteWANTCP = "wan_tcp" // direct TCP to any other address
	RouteQUIC   = "quic"    // HTTP/3, native UDP or ICE
	RouteRelay  = "relay"   // encrypted tunnel through the Orbit service
)

// RouteClasses lists the classes in display order.
var RouteClasses = []string{RouteLAN, RouteWANTCP, RouteQUIC, RouteRelay}

const (
	// RouteUsageRetention bounds the durable record; older buckets are dropped.
	RouteUsageRetention = 90 * 24 * time.Hour
	// RouteUsageHourly keeps hour buckets this long so a 24-hour window is
	// exact to the hour; older buckets are merged into UTC days.
	RouteUsageHourly = 48 * time.Hour
	// MaxRouteUsageRecords bounds memory and the state file (32 peers × 2
	// purposes × 4 classes × (48 hours + 90 days) is about 35,000).
	MaxRouteUsageRecords = 40000
)

// RouteUsageRecord is one bucket. Start is the bucket's UTC start (an hour, or
// a day once older than RouteUsageHourly). It holds no address or file name.
type RouteUsageRecord struct {
	Start    int64  `json:"start"`
	Device   string `json:"device"`
	Purpose  string `json:"purpose"`
	Route    string `json:"route"`
	Requests uint64 `json:"requests"`
	Sent     uint64 `json:"bytes_sent"`
	Received uint64 `json:"bytes_received"`
}

// RouteTotals sums a window for one peer and class.
type RouteTotals struct {
	Requests, Sent, Received uint64
}

// PeerRouteUsage is one peer's totals per class over a window.
type PeerRouteUsage struct {
	Device, Purpose string
	Routes          map[string]RouteTotals
}

type usageKey struct {
	start                  int64
	device, purpose, route string
}

// RouteUsage counts peer HTTP requests this device made and their body bytes
// by the socket that carried them. Counts are this device's observation only.
type RouteUsage struct {
	mu      sync.Mutex
	now     func() time.Time
	buckets map[usageKey]*RouteTotals
	dirty   bool
}

func NewRouteUsage(now func() time.Time) *RouteUsage {
	if now == nil {
		now = time.Now
	}
	return &RouteUsage{now: now, buckets: map[usageKey]*RouteTotals{}}
}

// routeClass maps a connection's route label and remote address to a class.
func routeClass(route string, remote net.Addr) string {
	switch route {
	case "relay":
		return RouteRelay
	case "quic":
		return RouteQUIC
	}
	if remote != nil {
		if ap, err := netip.ParseAddrPort(remote.String()); err == nil {
			a := ap.Addr().Unmap()
			if a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
				return RouteLAN
			}
		}
	}
	return RouteWANTCP
}

func (u *RouteUsage) add(t Target, route string, requests, sent, received uint64) {
	if u == nil || requests+sent+received == 0 {
		return
	}
	start := u.now().UTC().Truncate(time.Hour).Unix()
	key := usageKey{start, hex.EncodeToString(t.Device[:]), string(t.Purpose), route}
	u.mu.Lock()
	defer u.mu.Unlock()
	b := u.buckets[key]
	if b == nil {
		if len(u.buckets) >= MaxRouteUsageRecords {
			u.compactLocked()
		}
		b = &RouteTotals{}
		u.buckets[key] = b
	}
	b.Requests += requests
	b.Sent += sent
	b.Received += received
	u.dirty = true
}

// compactLocked merges hour buckets older than RouteUsageHourly into days,
// drops buckets past retention and, if still over the bound, the oldest.
func (u *RouteUsage) compactLocked() {
	before := len(u.buckets)
	changed := false
	now := u.now().UTC()
	hourly := now.Add(-RouteUsageHourly).Unix()
	expired := now.Add(-RouteUsageRetention).Unix()
	for k, v := range u.buckets {
		if k.start < expired {
			delete(u.buckets, k)
			changed = true
			continue
		}
		if k.start < hourly {
			day := time.Unix(k.start, 0).UTC().Truncate(24 * time.Hour).Unix()
			if day != k.start {
				changed = true
				delete(u.buckets, k)
				k.start = day
				merged := u.buckets[k]
				if merged == nil {
					merged = &RouteTotals{}
					u.buckets[k] = merged
				}
				merged.Requests += v.Requests
				merged.Sent += v.Sent
				merged.Received += v.Received
			}
		}
	}
	if over := len(u.buckets) - MaxRouteUsageRecords + 1; over > 0 {
		keys := make([]usageKey, 0, len(u.buckets))
		for k := range u.buckets {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].start < keys[j].start })
		for _, k := range keys[:over] {
			delete(u.buckets, k)
		}
	}
	if changed || len(u.buckets) != before {
		u.dirty = true
	}
}

// Records returns the compacted, sorted record and whether it changed since
// the last call that reported a change.
func (u *RouteUsage) Records() ([]RouteUsageRecord, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.compactLocked()
	out := make([]RouteUsageRecord, 0, len(u.buckets))
	for k, v := range u.buckets {
		out = append(out, RouteUsageRecord{Start: k.start, Device: k.device, Purpose: k.purpose, Route: k.route, Requests: v.Requests, Sent: v.Sent, Received: v.Received})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		if a.Device != b.Device {
			return a.Device < b.Device
		}
		if a.Purpose != b.Purpose {
			return a.Purpose < b.Purpose
		}
		return a.Route < b.Route
	})
	dirty := u.dirty
	u.dirty = false
	return out, dirty
}

// Restore merges a durable record. Invalid entries reject the whole record so
// a damaged file is reported rather than half-loaded.
func (u *RouteUsage) Restore(records []RouteUsageRecord) error {
	if len(records) > MaxRouteUsageRecords {
		return errors.New("ROUTE_USAGE_TOO_LARGE")
	}
	for _, r := range records {
		if !validUsageRecord(r) {
			return errors.New("ROUTE_USAGE_INVALID")
		}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, r := range records {
		k := usageKey{r.Start, r.Device, r.Purpose, r.Route}
		b := u.buckets[k]
		if b == nil {
			b = &RouteTotals{}
			u.buckets[k] = b
		}
		b.Requests += r.Requests
		b.Sent += r.Sent
		b.Received += r.Received
	}
	u.compactLocked()
	u.dirty = false // the restored record is what is on disk
	return nil
}

func validUsageRecord(r RouteUsageRecord) bool {
	if r.Start <= 0 || r.Start%3600 != 0 || (r.Purpose != string(PeerData) && r.Purpose != string(Enrollment)) {
		return false
	}
	if b, err := hex.DecodeString(r.Device); err != nil || len(b) != 32 {
		return false
	}
	return slices.Contains(RouteClasses, r.Route)
}

// Window sums buckets that start at or after since, per peer and purpose.
func (u *RouteUsage) Window(since time.Time) []PeerRouteUsage {
	u.mu.Lock()
	defer u.mu.Unlock()
	from := since.UTC().Unix()
	byPeer := map[[2]string]map[string]RouteTotals{}
	for k, v := range u.buckets {
		if k.start < from {
			continue
		}
		p := [2]string{k.device, k.purpose}
		if byPeer[p] == nil {
			byPeer[p] = map[string]RouteTotals{}
		}
		t := byPeer[p][k.route]
		t.Requests += v.Requests
		t.Sent += v.Sent
		t.Received += v.Received
		byPeer[p][k.route] = t
	}
	out := make([]PeerRouteUsage, 0, len(byPeer))
	for p, routes := range byPeer {
		out = append(out, PeerRouteUsage{Device: p[0], Purpose: p[1], Routes: routes})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Device != out[j].Device {
			return out[i].Device < out[j].Device
		}
		return out[i].Purpose < out[j].Purpose
	})
	return out
}
