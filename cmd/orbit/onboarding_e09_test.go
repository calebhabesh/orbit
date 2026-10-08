package main

import (
	"testing"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

func TestOnboardingE09RouteShare(t *testing.T) {
	got := routeShare([]tc.NetworkObservation{{Route: "direct"}, {Route: "quic"}, {Route: "relay"}, {Route: "unavailable", Code: "RELAY_BUDGET"}})
	if got != "Routes: 2 direct, 1 relay, 1 not connected (latest observation per device)" {
		t.Fatalf("%q", got)
	}
}
