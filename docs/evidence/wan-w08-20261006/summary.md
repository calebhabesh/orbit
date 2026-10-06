# W08 — LAN discovery and reachable direct candidates

**Complete for W08 production integration and local/isolated fixture acceptance.**
Run: 2026-10-06, one Linux amd64 development host, Go 1.27.1. Existing dirty
W00–W07/P/O/T source and evidence are preserved; no host firewall/router/VPS workload
was changed. [Manifest](manifest.json), [commands](commands.md), [results](results.json)
and [preservation](preservation.json) record provenance and limits.

Delivered signed bounded LAN announcements on reviewed selected interfaces;
known-pin-only discovery; independent interface/generation leases; actual bound
TCP ports and eligible public IPv4/IPv6 gathering; optional direct peer listeners;
complete pinned TLS candidate races; expiry, bounded admission and joined shutdown.
The existing replication handlers retain membership, chunk, receipt and enrollment
isolation. Fresh reviewed setup exposes LAN advertising, and existing policy values
remain intact. Local-only supports known-peer LAN sync without service requests.
Exact defaults and settings are frozen in the
[architecture](../../orbit-wan-architecture.md#w08-direct-tcp-and-local-discovery-integration).

Executed acceptance:

- **15 discovered W08 tests** across network, protocol, replication and terminal.
  The normally skipped public-namespace test is separately compiled/run under race
  in a private marked user/network namespace; it is not counted as an ordinary skip.
- Focused race checks pass: spoofed/unknown identity/pin/signature, oversized/strict
  datagrams, prohibited scopes and interface-prefix mismatches; scoped address changes,
  same-generation mutation, expiry and 16-candidate aggregate bounds; complete family
  races, wrong certificate, blocked TCP and unsupported IPv6 fallback; cached empty
  directory leases and existing direct sessions without service access.
- Production engine multicast discovery transfers both ways without configured peer
  IPs, matching bytes/heads/hashes/authors and reusing interrupted verified chunks.
  Actual nonloopback local ULA IPv6 mTLS/HTTP/verified transfer passes. Public-scope
  IPv4/IPv6 uses actual TCP sockets, bound ports, production signed directory lookup
  and pinned engine transfer in a new isolated namespace; a direct session continues
  after actual directory shutdown. These addresses are simulated, with no internet route.
- Race-instrumented production binaries pass **two repetitions of each W08 journey**
  (173.308s total): invitation/pending denial/exact approval before Local-only LAN;
  zero service HTTP/WSS requests during that mode; exact two-way versions/hashes and
  unchanged identities; capture after service shutdown. Fresh ordinary CLI onboarding
  with optional ports occupied preserves verified relay sync; an explicit mandatory
  manual collision still fails startup. Five-second reconciliation is retained.
- Resource tests pass three repetitions plus final focused race: 32 blocked raw TCP
  dials and joined work; 64 admitted incoming sockets before TLS; 1,000 spoof/oversized
  datagrams. Recorded whole-process FD baseline/during/after is **6/8/6**, goroutines
  **2/2** before/after; heap samples are in the logs, not isolated capacity estimates.
- Broader uncached race checks pass network/protocol/replication/config/control/TUI/CLI.
  W06 binary and W07 real PTY compatibility pass (324.337s combined with the earlier
  W08 fixture). The focused Make aggregate passes format, vet, internal/CLI tests,
  integration, models, faults, twelve Python harness checks and amd64/arm64 packages.
  Current packages and both standalone service cross-builds pass. Full `make check`
  was not rerun for this intermediate packet; W07's existing full aggregate is retained.
- Namespace safety refusals, independent Python LAN signature/canonical fixture,
  relative documentation file links, initial-source/evidence preservation and `git diff --check` pass.

The new empty-lease regression first reproduced 20 lookups instead of one in 0.002s;
its repair caches valid empty records and reuses the same verified lookup for relay
allocation, keeping fresh session proofs. A resource experiment added explicit full
TLS-attempt lifecycle joining. Initial compilation/fixture-oracle failures are retained
without acceptance credit. The first namespace ownership check could not inspect an
ancestor `/proc` namespace under mapped credentials; Linux `NS_GET_USERNS` now verifies
actual namespace ownership before any interface action, with passing refusal tests.
The first link-check attempt treated historical `file://` URI references as relative
paths; the corrected relative-file check passes and leaves historical references intact.

**Remaining limits:** restart-to-relay experiments using pre-enrolled W05 fixtures
exposed route/quota recovery failures, including one failed and one passing repeated
race run; all are uncredited for W08 and remain W11 work. Collision acceptance starts
with the optional port occupied in a fresh ordinary journey. IPv6-only LAN multicast
is not implemented; local discovery carries IPv4/ULA IPv6 over IPv4 multicast. Fresh
Local-only initial pairing requires explicit isolated legacy local enrollment settings.
There is no native public internet IPv4/IPv6, physical multi-host/Pi capacity or hosted
operator/default-profile claim. QUIC/ICE/STUN, direct reprobes, native roaming/fairness,
WG4/WG6, T13 login/logout/unattended boot and deferred P17 owner use/explanation remain.

Next: **W09 — QUIC HTTP3 transport and integration subgate**. Read its packet and WG4,
inspect/pin actual quic-go/Pion APIs and licenses, and prove native UDP HTTP3 with
existing pinned TLS/peer handlers and both pull directions before ICE composition.
Preserve these TCP/LAN bounds, failed W11 restart evidence and all historical records.
