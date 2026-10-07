# Same-LAN direct paths and profile epoch 2 — 2026-10-07

Post-W17 change, recorded in [WAN status](../../implementation/wan-status.md#post-w17--same-lan-direct-paths-and-profile-epoch-2-2026-10-07).
Base commit: `77c099e` (working-tree changes on top, listed there).

## Cause of the W14 same-LAN result

- The owner's laptop runs `ufw` with `DEFAULT_INPUT_POLICY="DROP"` and no Orbit
  rule. Orbit's discovery (UDP 22027 multicast) and its random direct ports were
  therefore dropped inbound. The kernel log recorded a dropped TCP SYN from the Pi
  (192.168.88.63) to a random laptop port on 2026-10-05.
- LAN addresses travel only by multicast; the service never carries them, so the
  laptop never learned the Pi's LAN address.
- Correction: the [W14 summary](../wan-w14-20261006/summary.md) states the route
  was `relay CONNECTED`, but its [log](../wan-w14-20261006/logs/native-journey.log)
  records only one route observation, `quic; code=CONNECTED`, at the end of the
  run. That direct UDP path used public (service-coordinated) candidates through
  the home router. The historical files are unchanged.

## Native rerun ([script](lan-native.sh), [log](logs/native-lan.log))

Release packages from the final source (epoch 2 profile), fresh disposable state on
the laptop (amd64, firewall unchanged) and Pi 4B (arm64), same home LAN, hosted
service, ordinary use only.

| Observation | Result |
| --- | --- |
| Pi route after first sync, before an exchange reached it | `relay`, LAN candidates 0 |
| Laptop LAN candidates for the Pi (multicast from the Pi is dropped by the laptop firewall) | 4, present within one 15 s exchange tick |
| Both routes after the exchange | `quic; code=CONNECTED`, LAN candidates 4 on each side |
| Laptop → Pi / Pi → laptop after the exchange | 1.7 s / 1.7 s (W14: ≈29 s first file, 5.7 s edit) |
| File hashes | identical on both devices |

Limits: one run on one home network. The route label `quic` does not say whether
the LAN or the router's public address carried the UDP; LAN candidate counts are
the evidence that the exchange worked. No other router or firewall combination
was tested.

## Release check

`make check` passed (exit 0, [log](logs/make-check.log)). Its terminal stage
compiled before the LAN exchange edits; later stages (unit, integration, model,
faults, harness, packages) ran on the changed source. The terminal stage was
rerun on the final source and passed in 1,511 s ([log](logs/test-terminal.log)).
