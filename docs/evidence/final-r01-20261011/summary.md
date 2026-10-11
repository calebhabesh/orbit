# R01 summary: route accounting (RG2)

Date: 2026-10-11, base `4de12fd`. RG2 is closed with the design below.

## What shipped

- `internal/network/route_usage.go`: `RouteUsage` counts completed peer requests
  this device made and their body bytes per peer, purpose and class (`lan`,
  `wan_tcp`, `quic`, `relay`). Counting happens in `managedPool.roundTrip`
  (`manager.go`): the class comes from the socket that carried the request.
- `internal/config/route_usage.go`: owner-only `route-usage.json`. The daemon
  (`internal/app/app.go`) restores it at start, saves it every 5 minutes if
  changed, and saves it after the manager closes.
- `network_status` carries `route_usage` for the "24h" and "30d" windows
  (`internal/control/terminal_network.go`). The contract helper
  `tc.RouteShare` produces the qualified line used by the CLI, doctor and the
  TUI's Connection details.
- `orbit network status --routes` gives per-peer detail; `--json` carries
  everything.

## Evidence

| Acceptance item | Result |
| --- | --- |
| Classification per route class | LAN TCP, WAN TCP, QUIC and production relay each counted 9 requests and ≥ the 3 MiB pulled in their own class, zero in the others (`logs/r01-tests.log`). Classification table (private, ULA, link-local, mapped IPv4, CGNAT, public) passes. Real daemons: W08 (LAN TCP) and W09 (QUIC) production-binary tests now also assert the class in `network_status` and pass (`logs/binary-daemons.log`) |
| Rollup survives restart | Save → load → restore keeps totals, file mode 0600, unknown fields rejected; daemons wrote `route-usage.json` at shutdown in the binary tests |
| Retention and bounds | Hour buckets merge into days after 48 h, buckets go after 90 days, at most 40,000 records; damaged records are rejected whole |
| Rendering | CLI `--routes` output, TUI Connection details line, control windows (24h since the hour, 30d since the UTC day) |
| Overhead on 1 GiB | 1-GiB LAN pull in 17.3 s (59 MiB/s on tmpfs); counted 1,073,885,780 B received (+0.013% protocol bodies) in 1,334 requests. Counting costs 3.7 ns per body read and 105 ns per request, at most about 1.1 ms for the transfer, under 0.01% |

## Limitations

- The classification tests use in-process two-device fixtures, not network
  namespaces. The `wan_tcp` case fakes the remote address on a loopback socket;
  a real public-address run is R05 work.
- The overhead figure is computed from microbenchmarks and an assumed minimum
  body read of 4 KiB, not an A/B run with counting disabled.
- Counts cover this device's own requests only. Served requests, service
  control traffic, doctor probes and TLS/HTTP/WebSocket framing are excluded.
- RFC 6598 addresses (CGNAT, Tailscale) count as `wan_tcp`, by design.
- A crash can lose up to 5 minutes of counts.
- `make check` was not run; the M1 milestone falls at R02.
