# E09 summary — monthly relay egress budget and busy-relay UX (2026-10-08)

**Complete; EG4 closed. Deployment is pending the owner's confirmation at E10.**
[Commands](commands.md), [results](results.json).

- **Accounting (EG4):** the relay counter is egress payload in both
  forwarding directions. TLS/WebSocket framing adds 0.22% for bulk transfer.
- **Budget:** `relay_month_bytes` (2 TiB default) is counted in a private
  `relay-month.json` under the unit's new `StateDirectory`. It is written at
  most every 30 s and on stop, survives restarts and resets at each UTC month.
  At the budget, new data relays are refused with `RELAY_BUDGET` and live ones
  end at the next chunk. Pairing relays stay available.
- **Monitoring:** `orbit_net_relay_month_bytes`, `orbit_net_relay_month_limit_bytes`
  and `orbit_net_refusals_total{reason="budget"}` are exposed, and
  `orbit-net alert` reports the 80% and 100% transitions.
- **Devices:** they show "Relay unavailable until <next month>; direct
  connections still work" and ask the relay again hourly. `orbit network
  status` reports the direct/relay/not-connected route share.

Open item: `TestWANW12BinaryDoctorPrivacyAndPTY` fails intermittently (relay
probe timeout). It reproduced on the unmodified E07 commit, so it is not an E09
regression; its cause is listed for E10.
