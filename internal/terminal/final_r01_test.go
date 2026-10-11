package terminal

import (
	"strings"
	"testing"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

// R01: connection details show each peer's qualified share per window.
func TestFinalR01ConnectionDetailsShowRouteShare(t *testing.T) {
	device := strings.Repeat("ab", 32)
	n := &tc.NetworkStatus{Policy: tc.NetworkPolicy{Mode: "automatic"}, Code: "SERVICE_READY", Observations: []tc.NetworkObservation{{Device: device, Route: "relay", Freshness: "recent"}},
		RouteUsage: []tc.NetworkRouteUsage{{Device: device, Purpose: "peer_data", Window: "30d", Routes: []tc.NetworkRouteTotals{{Route: "lan", Requests: 1, BytesReceived: 1 << 20}, {Route: "relay", Requests: 1, BytesReceived: 3 << 20}}}}}
	text := strings.Join(networkLines(n), "\n")
	if !strings.Contains(text, "Peer traffic observed by this device, last 30 days: 4 MiB in 2 requests; 25% direct (LAN 25%, WAN TCP 0%, QUIC 0%), 75% relayed") {
		t.Fatal(text)
	}
	if strings.Contains(text, "last 24 hours") {
		t.Fatal("empty window rendered:", text)
	}
}
