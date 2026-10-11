package terminalcontract

import "testing"

func TestFinalR01RouteShareQualifiedSummary(t *testing.T) {
	usage := []NetworkRouteUsage{
		{Device: "a", Purpose: "peer_data", Window: "24h", Routes: []NetworkRouteTotals{{Route: "lan", Requests: 3, BytesReceived: 750}, {Route: "wan_tcp"}, {Route: "quic", Requests: 1, BytesReceived: 50}, {Route: "relay", Requests: 2, BytesReceived: 200}}},
		{Device: "b", Purpose: "peer_data", Window: "24h", Routes: []NetworkRouteTotals{{Route: "relay", Requests: 1, BytesReceived: 1000}}},
		{Device: "a", Purpose: "peer_data", Window: "30d", Routes: []NetworkRouteTotals{{Route: "relay", Requests: 9, BytesReceived: 9}}},
	}
	if got, want := RouteShare(usage, "24h", "a"), "Peer traffic observed by this device, last 24 hours: 1000 B in 6 requests; 80% direct (LAN 75%, WAN TCP 0%, QUIC 5%), 20% relayed"; got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if got, want := RouteShare(usage, "24h", ""), "Peer traffic observed by this device, last 24 hours: 2 KiB in 7 requests; 40% direct (LAN 38%, WAN TCP 0%, QUIC 3%), 60% relayed"; got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if RouteShare(usage, "30d", "b") != "" || RouteShare(nil, "24h", "") != "" {
		t.Fatal("empty window rendered")
	}
}
