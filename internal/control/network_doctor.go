package control

import (
	"context"
	"encoding/hex"
	"github.com/calebhabesh/orbit/internal/rendezvous"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/network"
)

func networkAction(code string) string {
	switch code {
	case "CONNECTED", "VERIFIED", "SERVICE_READY", "LOCAL_ONLY", "MANUAL":
		return "No connection action needed; inspect folder copies separately."
	case "NOT_TESTED":
		return "Run an explicit network doctor check if needed."
	case "PROFILE_MISSING", "PROFILE_EXPIRED", "PROFILE_INVALID", "PROFILE_MISSING_OR_EXPIRED", "PROFILE_UNTRUSTED":
		return "Review an updated signed service profile; local capture remains available."
	case "IDENTITY_MISMATCH", "TLS_IDENTITY_FAILED":
		return "Stop and inspect the reviewed device or service identity; do not bypass verification."
	case "QUOTA_EXCEEDED":
		return "Retry later or review an alternative service profile."
	case "RELAY_BUDGET":
		// The operator's monthly relay allowance is spent (E09).
		return "Relay unavailable until " + rendezvous.NextMonth(time.Now()).Format("2 January 2006") + " (the service's monthly relay limit); direct connections still work and Orbit keeps trying them."
	case "PEER_OFFLINE":
		return "Wait for the device to come online; captured changes remain saved locally."
	case "NETWORK_RESTART_REQUIRED":
		return "Restart the daemon to activate the reviewed policy."
	case "CANCELLED":
		return "The check was cancelled; retry explicitly when ready."
	case "DAEMON_STOPPED":
		return "Start the daemon to check live connections."
	default:
		return "Check device and service availability; a timeout does not identify a NAT or firewall type."
	}
}

// Explicit diagnostics are bounded to 20 seconds, one selected peer, and the
// reviewed active policy. Passive queries and support export never call this.
func (c *Controller) terminalNetworkDoctor(ctx context.Context, q tc.Query) (tc.Result, error) {
	r, err := c.terminalNetworkQuery(ctx, tc.Query{Version: tc.Version, Kind: "network_status", ID: q.ID})
	if err != nil {
		return r, err
	}
	n := r.Network
	if n.RestartRequired || c.options.StoppedAdapter {
		return r, nil
	}
	work, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	internet := n.ActivePolicy.Mode == "automatic" || n.ActivePolicy.Mode == "self_hosted"
	if internet && c.options.NetworkService != nil && c.options.Network != nil {
		n.Probes = append(n.Probes, c.options.NetworkService.ProbeService(work)...)
		cfg, e := config.Load(c.db.StateDir())
		if e == nil {
			// An authenticated self-lookup verifies the directory independently of peer presence.
			targets := c.options.Network.KnownTargets()
			device, pin := cfg.DeviceID, ""
			if c.options.NetworkSelfPin != "" {
				pin = c.options.NetworkSelfPin
			}
			for _, t := range targets {
				if q.ID != "" && hex.EncodeToString(t.Device[:]) == q.ID {
					device = q.ID
					pin = hex.EncodeToString(t.Pin[:])
					break
				}
			}
			if pin != "" {
				probe, stop := context.WithTimeout(work, 3*time.Second)
				_, found, e := c.options.NetworkService.Lookup(probe, device, pin, "peer_data", 0)
				code := network.ProbeCode(probe, e)
				if e == nil && !found {
					code = "PEER_OFFLINE"
				}
				n.Probes = append(n.Probes, network.ProbeResult{Kind: "directory", Code: code, ObservedAt: c.options.Now().UTC().Format(time.RFC3339Nano)})
				stop()
			}
		}
	}
	// No implicit peer fan-out. Selecting a device is an explicit probe of that identity.
	if q.ID != "" && c.options.Network != nil && c.options.NetworkPeerTLS != nil {
		for _, t := range c.options.Network.KnownTargets() {
			if t.Purpose != network.PeerData || hex.EncodeToString(t.Device[:]) != q.ID {
				continue
			}
			trust, e := c.options.NetworkPeerTLS(t)
			if e != nil {
				n.Probes = append(n.Probes, network.ProbeResult{Kind: "peer_identity", Code: "IDENTITY_MISMATCH"})
				break
			}
			n.Probes = append(n.Probes, c.options.Network.ProbeTCP(work, t, trust, false))
			if internet {
				n.Probes = append(n.Probes, c.options.Network.ProbeTCP(work, t, trust, true))
			}
			break
		}
	}
	if internet && c.options.NetworkService != nil {
		selection, e := config.LoadNetworkProfile(c.db.StateDir(), uint64(c.options.Now().Unix()))
		settings, se := config.LoadDirectSettings(c.db.StateDir())
		if e == nil && se == nil && !settings.UDPDisabled {
			n.Probes = append(n.Probes, network.ProbeUDP(work, selection.Profile.STUN, settings.Interfaces))
		}
	}
	for _, kind := range []string{"service_dns_tcp", "service_tls", "directory", "direct_tls", "relay_inner_tls", "udp_stun"} {
		present := false
		for _, probe := range n.Probes {
			if probe.Kind == kind {
				present = true
				break
			}
		}
		if !present {
			code := "NOT_TESTED"
			if !internet && (kind != "direct_tls") {
				code = "DISABLED_BY_POLICY"
			}
			n.Probes = append(n.Probes, network.ProbeResult{Kind: kind, Code: code})
		}
	}
	return r, nil
}
