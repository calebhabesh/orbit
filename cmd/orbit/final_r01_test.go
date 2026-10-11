package main

import (
	"bytes"
	"strings"
	"testing"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

func TestFinalR01NetworkStatusRoutesOutput(t *testing.T) {
	var out bytes.Buffer
	printRouteUsage(&out, nil)
	if !strings.Contains(out.String(), "no completed peer requests in the last 30 days (observed by this device)") {
		t.Fatal(out.String())
	}
	out.Reset()
	printRouteUsage(&out, []tc.NetworkRouteUsage{{Device: "dev", Purpose: "peer_data", Window: "24h", Since: "2026-10-10T12:00:00Z", Routes: []tc.NetworkRouteTotals{{Route: "lan"}, {Route: "wan_tcp"}, {Route: "quic", Requests: 2, BytesSent: 100, BytesReceived: 2048}, {Route: "relay"}}}})
	for _, want := range []string{"completed peer requests this device made", "Device dev (peer_data), last 24 hours since 2026-10-10T12:00:00Z:", "  quic    requests=2 sent=100 B received=2 KiB", "  relay   requests=0 sent=0 B received=0 B"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in\n%s", want, out.String())
		}
	}
}
