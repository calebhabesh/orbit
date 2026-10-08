package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/calebhabesh/orbit/internal/rendezvous"
)

// metricsHandler serves sanitized totals in Prometheus text format on a
// loopback listener. No label carries an address, identity, pin or session.
func (p *prepared) metricsHandler(listener *rendezvous.CappedListener, stun *rendezvous.STUNServer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		for _, profile := range p.service.Metrics().Profiles {
			if profile.Valid {
				fmt.Fprintln(w, "ok")
				return
			}
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, "no valid profile")
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(p.metricsText(listener, stun, time.Now())))
	})
	return mux
}

type reasonValue struct {
	reason string
	value  uint64
}

func (p *prepared) metricsText(listener *rendezvous.CappedListener, stun *rendezvous.STUNServer, now time.Time) string {
	m := p.service.Metrics()
	var b strings.Builder
	metric := func(name, kind, help string, value any, labels ...string) {
		if !strings.Contains(b.String(), "# TYPE "+name+" ") {
			fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
		}
		label := ""
		if len(labels) > 0 {
			label = "{" + strings.Join(labels, ",") + "}"
		}
		fmt.Fprintf(&b, "%s%s %v\n", name, label, value)
	}
	metric("orbit_net_build_info", "gauge", "Build version.", 1, fmt.Sprintf("version=%q", version))
	for _, profile := range m.Profiles {
		epoch := fmt.Sprintf("epoch=\"%d\"", profile.Epoch)
		metric("orbit_net_profile_expiry_seconds", "gauge", "Seconds until a served profile epoch expires.", int64(profile.Expires)-now.Unix(), epoch)
		valid := 0
		if profile.Valid {
			valid = 1
		}
		metric("orbit_net_profile_valid", "gauge", "Whether a served profile epoch admits new work.", valid, epoch)
	}
	if cert := p.cert.current.Load(); cert != nil && cert.Leaf != nil {
		metric("orbit_net_certificate_expiry_seconds", "gauge", "Seconds until the served TLS certificate expires.", int64(cert.Leaf.NotAfter.Sub(now)/time.Second))
	}
	metric("orbit_net_certificate_reloads_total", "counter", "Successful SIGHUP certificate reloads.", p.cert.reloads.Load())
	metric("orbit_net_certificate_reload_failures_total", "counter", "Rejected certificate reloads; the previous certificate stays active.", p.cert.failures.Load())
	metric("orbit_net_connections_active", "gauge", "Accepted TCP sockets (limit 256).", listener.Active())
	metric("orbit_net_connections_refused_total", "counter", "Sockets closed before TLS at the connection limit.", listener.Refused())
	metric("orbit_net_directory_records", "gauge", "Live signed directory leases (limit 128).", m.Records)
	metric("orbit_net_sessions", "gauge", "Live offers and relay sessions (limit 128).", m.Sessions)
	metric("orbit_net_relay_sessions", "gauge", "Sessions with an attached relay.", m.Relays)
	metric("orbit_net_controls", "gauge", "Authenticated control channels.", m.Controls)
	metric("orbit_net_websockets_pending", "gauge", "Control or relay websockets in handshake or service.", m.Pending)
	metric("orbit_net_challenges", "gauge", "Outstanding challenges (limit 128).", m.Challenges)
	metric("orbit_net_rate_keys", "gauge", "Per-device metadata rate buckets (limit 1024).", m.RateKeys)
	metric("orbit_net_source_buckets", "gauge", "Pre-auth source buckets (limit 128).", m.SourceBuckets)
	for _, r := range []reasonValue{{"quota", m.RefusedQuota}, {"invalid", m.RefusedInvalid}, {"expired", m.RefusedExpired}, {"untrusted", m.RefusedUntrusted}, {"budget", m.RefusedBudget}, {"other", m.RefusedOther}} {
		metric("orbit_net_refusals_total", "counter", "Refused requests by stable reason class.", r.value, fmt.Sprintf("reason=%q", r.reason))
	}
	metric("orbit_net_relay_bytes_total", "counter", "Relay payload bytes written to receiving devices, both forwarding directions; excludes TLS/WebSocket framing.", m.RelayBytes)
	if m.RelayMonthLimit > 0 {
		metric("orbit_net_relay_month_bytes", "gauge", "Relay payload bytes forwarded this UTC month (the monthly budget's count).", m.RelayMonthBytes)
		metric("orbit_net_relay_month_limit_bytes", "gauge", "Configured monthly relay allowance (relay_month_bytes).", m.RelayMonthLimit)
	}
	metric("orbit_net_relay_limit_bytes_per_second", "gauge", "Configured aggregate relay ceiling.", m.ServiceBytesPerSecond)
	metric("orbit_net_relay_device_limit_bytes_per_second", "gauge", "Configured per-device relay ceiling.", m.DeviceBytesPerSecond)
	if stun != nil {
		answered, limited, invalid, full := stun.Stats()
		metric("orbit_net_stun_answered_total", "counter", "STUN binding responses sent.", answered)
		for _, r := range []reasonValue{{"rate", limited}, {"invalid", invalid}, {"table_full", full}} {
			metric("orbit_net_stun_dropped_total", "counter", "STUN requests dropped without a response.", r.value, fmt.Sprintf("reason=%q", r.reason))
		}
	}
	return b.String()
}
