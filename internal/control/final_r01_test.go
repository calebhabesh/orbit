package control

import (
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
)

func TestFinalR01StatusRouteUsageWindows(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 30, 0, 0, time.UTC)
	u := network.NewRouteUsage(func() time.Time { return now })
	device := "0101010101010101010101010101010101010101010101010101010101010101"
	other := "0202020202020202020202020202020202020202020202020202020202020202"
	if err := u.Restore([]network.RouteUsageRecord{
		{Start: now.Truncate(time.Hour).Unix(), Device: device, Purpose: "peer_data", Route: network.RouteRelay, Requests: 1, Received: 10},
		{Start: now.Add(-10 * 24 * time.Hour).Truncate(24 * time.Hour).Unix(), Device: device, Purpose: "peer_data", Route: network.RouteLAN, Requests: 1, Received: 90},
		{Start: now.Truncate(time.Hour).Unix(), Device: other, Purpose: "peer_data", Route: network.RouteQUIC, Requests: 1, Received: 5},
	}); err != nil {
		t.Fatal(err)
	}
	got := routeUsage(u, now, device)
	if len(got) != 2 || got[0].Window != "24h" || got[1].Window != "30d" || got[0].Since != "2026-10-10T12:00:00Z" || got[1].Since != "2026-09-11T00:00:00Z" {
		t.Fatalf("windows: %+v", got)
	}
	if len(got[0].Routes) != 4 || got[0].Routes[3].Route != "relay" || got[0].Routes[3].BytesReceived != 10 || got[0].Routes[0].BytesReceived != 0 || got[1].Routes[0].BytesReceived != 90 {
		t.Fatalf("totals: %+v", got)
	}
	if all := routeUsage(u, now, ""); len(all) != 4 {
		t.Fatalf("unfiltered: %+v", all)
	}
	if empty := routeUsage(nil, now, ""); empty == nil || len(empty) != 0 {
		t.Fatal("nil usage")
	}
}
