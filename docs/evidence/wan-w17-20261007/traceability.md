# W17 traceability: acceptance → evidence

Each row links the record that establishes the item and carries forward
whatever that record left unexecuted. "Local" means one development host
(fixtures, user/network namespaces or in-process emulators). "Native" means
real devices and networks. Single-run timings are observations, not limits.

## Packets

| Packet | Accepted on | Evidence | Carried-forward limitations |
| --- | --- | --- | --- |
| W00 Baseline | Recorded baseline revision, reproductions | [W00](../wan-w00-20261005/summary.md) | Historical P/O/T limits retained |
| W01 Contracts/gates | Local WG1–WG3 composition | [W01](../wan-w01-20261005/summary.md), [outcomes](../../implementation/wan-contracts.md) | Closed later: N07 QUIC/ICE by W09/W10, operator custody by W13/WG6 |
| W02 Manager/HTTPS | Local; manual direct HTTPS kept through the seam | [W02](../wan-w02-20261005/summary.md) | — |
| W03 Rendezvous/profiles | Local service/TLS/DNS fixtures | [W03](../wan-w03-20261005/summary.md) | Real operator: W13 |
| W04 Encrypted relay | Local, development CA | [W04](../wan-w04-20261005/summary.md) | Native relay: W13/W16 |
| W05 Routed enrollment | Local marked roots | [W05](../wan-w05-20261005/summary.md) | Native: W14/W16 |
| W06 CLI setup | Local and aggregate validation | [W06](../wan-w06-20261005/summary.md) | — |
| W07 TUI setup | Local PTY relay milestone | [W07](../wan-w07-20261005/summary.md) | Native TUI: W16 run 19 |
| W08 LAN/direct TCP | Local fixtures, isolated public IPv4/IPv6 fixtures | [W08](../wan-w08-20261006/summary.md) | Native LAN-direct between physical devices not observed (W14 same-LAN run stayed on relay) |
| W09 QUIC/HTTP3 | Local native transport subgate | [W09](../wan-w09-20261006/summary.md) | — |
| W10 ICE/STUN | Local native/emulated NAT matrix | [W10](../wan-w10-20261006/summary.md) | NAT types other than the two native networks are emulated only |
| W11 Policy/roaming | Local native/emulator and Pi | [W11](../wan-w11-20261006/summary.md), [follow-up](../wan-w11-followup-20261006/summary.md) | — |
| W12 Status/diagnostics/privacy | CLI/TUI, local privacy/diagnostics | [W12](../wan-w12-20261006/summary.md) | — |
| W13 Operated service | Live hosted service, rotation, self-host rehearsal | [W13](../wan-w13-20261006/summary.md), [deployment](../wan-w13-20261006/deployment.md), [WG6 readiness](../wan-wg6-readiness-20261006/summary.md) | Authority-key restore check waived by owner (unexecuted) |
| W14 Packages/migration | Packaged default, mixed versions, laptop/Pi same-LAN journey | [W14](../wan-w14-20261006/summary.md), [closeout](../wan-w14-closeout-20261006/manifest.json) | Native expired packaged profile: unit tests only |
| W15 Failure/resources | Local/native namespaces, emulators, Pi | [W15](../wan-w15-20261006/summary.md), [matrix](../wan-w15-20261006/scenario-matrix.md) | Physical power loss / Pi media reset out of scope |
| W16 Native WAN | Home NAT ↔ Oracle VPS, hosted default, CLI and TUI, forced relay, address change, service restart, three devices | [W16](../wan-w16-20261006/summary.md), [native runs](../wan-w16-20261006/native-hosted/summary.md), [fix](../wan-w16-20261006/quota-fix/summary.md) | School/corporate/CGNAT/IPv6-only unexecuted; no third separate network; address-change recovery ~30 s |
| W17 Combined release | This record | [summary](summary.md) | See the open list in the summary |

## Gates

| Gate | State | Evidence |
| --- | --- | --- |
| WG1 Inner encryption/transport | Closed (local) | [W01](../wan-w01-20261005/summary.md) |
| WG2 Registration/profile/privacy | Closed (local) | [W01](../wan-w01-20261005/summary.md) |
| WG3 Routed enrollment | Closed (local) | [W01](../wan-w01-20261005/summary.md) |
| WG4 QUIC/ICE composition | Closed (local native/emulator) | [W09](../wan-w09-20261006/summary.md), [W10](../wan-w10-20261006/summary.md) |
| WG5 Connection/roaming policy | Closed (production composition, local, Pi) | [W11](../wan-w11-20261006/summary.md) |
| WG6 Operated defaults | Closed 2026-10-07 | [gate outcome](../../orbit-wan-design-gates.md#wg6-w13-outcome--closed-2026-10-07), [alert evidence](../wan-w16-20261006/wg6-alert/summary.md) |

## Requirements

| Requirement | Native evidence | Local/emulated evidence |
| --- | --- | --- |
| U06/S23 Automatic address discovery | W16: no addresses typed; direct UDP found home ↔ VPS | W08, W10, W11 |
| S24 Relay confidentiality/isolation | W16 forced relay; W13 journal privacy check | W01, W04, W05, W15 (wrong pins, opaque streams, isolation) |
| S25 Direct traversal/fallback | W16 direct UDP, UDP-blocked relay, address change, service restart | W10 NAT matrix, W11, W15 |
| U17/S26 Hosted/self-host modes | W13 live operator, W14/W16 packaged profile | W13 self-host rehearsal, W14 operator switch |
| U18/S27 Simple CLI/TUI onboarding | W14 laptop/Pi; W16 CLI and TUI, run 19 | W06, W07 |
| S28 Bounded operable networking | W16 resources (daemons 30–36 MiB, `orbit-net` ~15 MiB); W13 budgets | W15 campaign, W10 bounds |
| S01–S22, I01–I28 (engine) | T13 native campaigns; W16 run 20 forwarding/conflict/restore | [T13 invariant map](../terminal-t13-20261004/invariant-map.md); W17 clean reproduction |
| N01–N10 (WAN invariants) | W16 | [verification](../../verification.md#native-wan-verification), W15 matrix; N10 physical WAN covered by W16 for the two reachable networks only |

## Inherited T13 technical checks

| Check | State | Evidence |
| --- | --- | --- |
| Ordinary two-device onboarding, restart/resume | Passed | [T13](../terminal-t13-20261004/summary.md) |
| LAN, existing Tailscale, three hosts, forwarding, retirement/replacement | Passed | [T13 follow-up](../terminal-t13-20261005/summary.md) |
| Failure/recovery, VM resets, storage exhaustion | Passed (virtual ext4) | [T13](../terminal-t13-20261004/summary.md) |
| TUI/CLI real PTY, packages/adoption | Passed | T13 records; W17 clean reproduction |
| Native start-at-login, logout, unattended boot | Passed in an owner-designated disposable KVM guest (W17); physical-hardware boot unexecuted | [lifecycle runs](lifecycle-vm/) |
| Personal use and unaided explanation | Deferred by the 2026-10-04 owner amendment | [P17 record](../../implementation/status.md) |
