package network

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestFinalR01RouteClassification(t *testing.T) {
	for _, c := range []struct {
		route, addr, want string
	}{
		{"direct", "192.168.1.20:41080", RouteLAN},
		{"direct", "10.0.0.5:41080", RouteLAN},
		{"direct", "127.0.0.1:41080", RouteLAN},
		{"direct", "[fd12:3456::1]:41080", RouteLAN},
		{"direct", "[fe80::1%eth0]:41080", RouteLAN},
		{"direct", "[::ffff:192.168.1.20]:41080", RouteLAN},
		{"direct", "203.0.113.7:443", RouteWANTCP},
		{"direct", "[2001:db8::1]:443", RouteWANTCP},
		{"direct", "100.101.0.2:41080", RouteWANTCP}, // RFC 6598 shared space (CGNAT, overlay VPNs) is not LAN
		{"direct", "", RouteWANTCP},
		{"quic", "192.168.1.20:41080", RouteQUIC},
		{"relay", "192.168.1.20:41080", RouteRelay},
	} {
		var remote net.Addr
		if c.addr != "" {
			remote = stringAddr(c.addr)
		}
		if got := routeClass(c.route, remote); got != c.want {
			t.Errorf("%s %s: got %s want %s", c.route, c.addr, got, c.want)
		}
	}
}

type stringAddr string

func (s stringAddr) Network() string { return "tcp" }
func (s stringAddr) String() string  { return string(s) }

func TestFinalR01RouteUsageBucketsWindowsAndRetention(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 30, 0, 0, time.UTC)
	u := NewRouteUsage(func() time.Time { return now })
	var target Target
	target.Device[0], target.Purpose = 1, PeerData
	u.add(target, RouteRelay, 1, 100, 1000)
	u.add(target, RouteLAN, 2, 10, 50)
	if _, dirty := u.Records(); !dirty {
		t.Fatal("new counts not dirty")
	}
	if _, dirty := u.Records(); dirty {
		t.Fatal("unchanged record reported dirty")
	}
	now = now.Add(-30 * time.Hour) // an older hour bucket, then return
	u.add(target, RouteQUIC, 1, 1, 1)
	now = now.Add(30 * time.Hour)
	day := u.Window(now.Add(-24 * time.Hour).Truncate(time.Hour))
	month := u.Window(now.Add(-30 * 24 * time.Hour).Truncate(24 * time.Hour))
	if len(day) != 1 || day[0].Routes[RouteQUIC] != (RouteTotals{}) || day[0].Routes[RouteRelay].Received != 1000 {
		t.Fatalf("24-hour window: %+v", day)
	}
	if len(month) != 1 || month[0].Routes[RouteQUIC].Requests != 1 || month[0].Routes[RouteLAN].Requests != 2 {
		t.Fatalf("30-day window: %+v", month)
	}
	// After 48 hours the hour buckets merge into their UTC day; after 90 days they go.
	now = now.Add(72 * time.Hour)
	records, _ := u.Records()
	for _, r := range records {
		if r.Start%86400 != 0 {
			t.Fatalf("old hour bucket not compacted: %+v", r)
		}
		if strings.Contains(r.Device, ".") || len(r.Device) != 64 {
			t.Fatalf("record device is not a bare ID: %+v", r)
		}
	}
	now = now.Add(RouteUsageRetention)
	if records, _ = u.Records(); len(records) != 0 {
		t.Fatalf("expired buckets retained: %+v", records)
	}
}

func TestFinalR01RouteUsageRestoreRejectsDamage(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	good := RouteUsageRecord{Start: now.Unix(), Device: strings.Repeat("ab", 32), Purpose: "peer_data", Route: RouteRelay, Requests: 1, Received: 5}
	for name, bad := range map[string]RouteUsageRecord{
		"unaligned": {Start: now.Unix() + 1, Device: good.Device, Purpose: "peer_data", Route: RouteRelay},
		"device":    {Start: now.Unix(), Device: "192.168.1.2", Purpose: "peer_data", Route: RouteRelay},
		"purpose":   {Start: now.Unix(), Device: good.Device, Purpose: "files", Route: RouteRelay},
		"route":     {Start: now.Unix(), Device: good.Device, Purpose: "peer_data", Route: "turn"},
	} {
		u := NewRouteUsage(func() time.Time { return now })
		if err := u.Restore([]RouteUsageRecord{good, bad}); err == nil {
			t.Errorf("%s: damaged record accepted", name)
		}
		if records, _ := u.Records(); len(records) != 0 {
			t.Errorf("%s: partial restore", name)
		}
	}
	u := NewRouteUsage(func() time.Time { return now })
	if err := u.Restore([]RouteUsageRecord{good}); err != nil {
		t.Fatal(err)
	}
	if records, dirty := u.Records(); dirty || len(records) != 1 || records[0] != good {
		t.Fatalf("restore: %+v dirty=%v", records, dirty)
	}
}

func TestFinalR01RouteUsageBound(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	u := NewRouteUsage(func() time.Time { return now })
	var target Target
	target.Purpose = PeerData
	for i := 0; i < MaxRouteUsageRecords+10; i++ {
		target.Device[0], target.Device[1], target.Device[2] = byte(i), byte(i>>8), byte(i>>16)
		u.add(target, RouteLAN, 1, 0, 0)
	}
	if records, _ := u.Records(); len(records) > MaxRouteUsageRecords {
		t.Fatalf("unbounded route usage: %d", len(records))
	}
}

// Per-operation costs of route accounting: one atomic add per response body
// Read and one bucket update per completed request (R01 overhead evidence).
func BenchmarkFinalR01BodyReadCount(b *testing.B) {
	body := &managedBody{}
	for i := 0; i < b.N; i++ {
		body.received.Add(32 << 10)
	}
}

func BenchmarkFinalR01RequestBucket(b *testing.B) {
	u := NewRouteUsage(nil)
	var target Target
	target.Purpose = PeerData
	for i := 0; i < b.N; i++ {
		u.add(target, RouteLAN, 1, 100, 4<<20)
	}
}
