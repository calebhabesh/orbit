package terminalcontract

import (
	"fmt"
	"strconv"
	"strings"
)

// RouteWindowLabel names a route usage window for people.
func RouteWindowLabel(window string) string {
	switch window {
	case "24h":
		return "last 24 hours"
	case "30d":
		return "last 30 days"
	}
	return window
}

// RouteShare sums one window's route usage across peers (or for one device)
// and returns a qualified one-line summary, or "" when nothing was counted.
// Shares are by body bytes in both directions; with no bytes, by requests.
func RouteShare(usage []NetworkRouteUsage, window, device string) string {
	totals := map[string][2]uint64{} // route -> bytes, requests
	var bytes, requests uint64
	for _, u := range usage {
		if u.Window != window || (device != "" && u.Device != device) {
			continue
		}
		for _, r := range u.Routes {
			b := uint64(r.BytesSent) + uint64(r.BytesReceived)
			t := totals[r.Route]
			t[0] += b
			t[1] += uint64(r.Requests)
			totals[r.Route] = t
			bytes += b
			requests += uint64(r.Requests)
		}
	}
	if requests == 0 {
		return ""
	}
	index, whole := 0, bytes
	if bytes == 0 {
		index, whole = 1, requests
	}
	pct := func(route string) int { return int((totals[route][index]*100 + whole/2) / whole) }
	// Direct and relayed always sum to 100; per-class figures round separately.
	direct := 100 - pct("relay")
	return fmt.Sprintf("Peer traffic observed by this device, %s: %s in %d requests; %d%% direct (LAN %d%%, WAN TCP %d%%, QUIC %d%%), %d%% relayed",
		RouteWindowLabel(window), HumanBytes(bytes), requests, direct, pct("lan"), pct("wan_tcp"), pct("quic"), pct("relay"))
}

// HumanBytes formats a byte count with binary units (1.5 GiB).
func HumanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatUint(n, 10) + " B"
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit && exp < 4; v /= unit {
		div *= unit
		exp++
	}
	value := strings.TrimSuffix(strconv.FormatFloat(float64(n)/float64(div), 'f', 1, 64), ".0")
	return value + " " + string("KMGTP"[exp]) + "iB"
}
