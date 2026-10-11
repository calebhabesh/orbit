# Final-scope design gates

Each gate stays **open** until its owning packet records the selected design,
executable evidence, the updated owning specification and remaining limitations
([plan](../orbit-final-implementation-plan.md#design-gates)). Candidates are
starting options from planning on 2026-10-11 at `5e61ac2`, not decisions. R00
refines this file before dependent work starts.

## RG1 — Service admission (R02)

Question: how the owner's `orbit-net` serves only the owner's devices, and what
other devices observe.

Facts: every service operation except STUN requires a signed key-possession
proof (W03), so the service knows the device ID and pin before allocating
directory records, offers or relay sessions. Unknown enrollment requesters
already use a separate bounded path (W04). The only data-relay refusal today
besides quotas is `RELAY_BUDGET` (E09). The pairing mailbox (E06) is claimed by
a device that is not yet enrolled anywhere.

Decide:

- Covered operations: announce, lookup, offers, relay reservation/attachment,
  pairing mailbox create and claim. STUN stays unauthenticated and rate-limited
  (proposed) since it carries no device identity.
- New-device pairing (proposed): an unlisted device may claim a pairing mailbox
  created by a listed inviter and use the enrollment path toward that inviter
  under existing enrollment bounds; it gets nothing else until the operator adds
  it. Alternative: the operator adds the joiner first (needs its identity before
  pairing; more steps).
- Refusal code (proposed `NOT_ADMITTED`) and how 2.3 clients treat an unknown
  code: bounded retries, understandable status, no tight loop.
- Allowlist source and reload: inline list or owner-only file; SIGHUP reload.

Evidence needed: per-operation listed/unlisted tests; new-device pairing test;
2.3 client fixture run; reload test.

## RG2 — Route accounting (R01)

Question: what Orbit counts so direct versus relayed use is visible and honest.

Candidates: HTTP body bytes and request counts of completed peer requests
(payload, not TLS or framing) by actual socket provenance after authentication;
classes `lan`, `wan_tcp`, `quic`, `relay`; daily rollups for 90 days in
owner-only state; displayed as shares with the window and "observed by this
device".

Evidence needed: classification per route; restart persistence; display tests;
overhead negligible on a 1-GiB transfer.

## RG3 — Release default (R04)

Question: how public builds ship without a default service while the owner's
devices keep theirs.

Facts: `internal/network/release-profile.json` is embedded and accepted only as
a signed `release` selection; `ORBIT_DISABLE_PACKAGED_PROFILE=1` already makes a
process behave like a build without one; `config.AdoptPackagedProfile` adopts it
for Automatic installs at daemon start; operator switches go through
`ReviewProfileChange` with per-authority rollback floors.

Candidates:

1. **Build without the embedded profile** (proposed); existing stored release
   selections keep working because they are stored and verified against the
   frozen release authority, not re-read from the embedded file. Confirm in code.
2. Keep the embedded profile and rely on the allowlist (rejected: public users
   would see refusals instead of a clear "no service" state).
3. Owner devices switch to `self_hosted` with a re-signed profile (fallback if
   option 1's stored-selection path depends on the embedded file).

Evidence needed: fresh-build setup tests; 2.3 upgrade with stored release
selection; owner-path rehearsal on `orbit-trial`; rollback to 2.3.

## RG4 — Measurement method (R05)

Question: what to measure and how, so results are reproducible and honest.

Candidates:

- Network matrix: same LAN; home (Bell fibre) ↔ VPS; home ↔ phone hotspot
  (likely CGNAT); UDP blocked by local firewall rule in a namespace; emulated
  NAT types and latency/loss with `nftables` and `tc netem` in namespaces.
- Metrics: direct success rate, time to first authenticated request, relay
  fallback time, throughput by route, resume cost, convergence time after heal,
  Pi RSS/CPU/descriptors.
- Repetitions: at least 10 per connectivity cell, at least 3 per throughput cell;
  report median and range, and every failure.
- Safety: namespaces and disposable state only; the VPS only as the owner's own
  allowlisted service, never under flood or fault load.

Evidence needed: the scripts, raw results and summary tables in R05 evidence.
