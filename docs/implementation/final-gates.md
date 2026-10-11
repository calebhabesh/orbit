# Final-scope design gates

Each gate stays **open** until its owning packet records the selected design,
executable evidence, the updated owning specification and remaining limitations
([plan](../orbit-final-implementation-plan.md#design-gates)). Candidates are
starting options from planning on 2026-10-11 at `5e61ac2`, not decisions. R00
refined them on 2026-10-11 at `b2172e2` (each gate's **R00 scoping** block,
evidence in `docs/evidence/final-r00-20261011/`); the gates stay open.

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

**R00 scoping (2026-10-11, code read at `b2172e2`):**

- *Where to check.* `/network/v1/challenge` carries only sender, pin and
  certificate (no kind or purpose), so it cannot tell a pairing joiner from an
  unlisted data device. `admitOrigin` (`internal/rendezvous/service.go`) sees
  the verified proof's kind and purpose and runs before every directory,
  offer, relay and pairing allocation: the allowlist check belongs there.
- *Challenge-table exposure.* Challenges are bounded per actor (2) and globally
  (128), with no listed/unlisted split. Unlisted actors (one key pair each, at the
  pre-auth source limits) could hold the global table and lock listed devices
  out. RG1 must also choose either an unlisted share of the challenge table
  (proposed: at most 32 of 128 slots) or a refusal at challenge time for
  unlisted actors except while they hold a claimable pairing mailbox.
- *2.3 clients and an unknown code.* `ServiceClient` passes any failure code
  through as `ServiceError`. Only `QUOTA_EXCEEDED`, `RELAY_BUDGET`,
  `SERVICE_UNAVAILABLE` and `CANCELED` count as transient (`relay_runtime.go`).
  An unknown `NOT_ADMITTED` withdraws readiness, so an unlisted 2.3 device
  retries announcements every 10 s and controls on a 2/4/8 s back-off, per
  purpose, indefinitely (about 25 signed attempts/min, all refused before
  allocation). `RELAY_BUDGET` would buy an hourly back-off on 2.3 clients but
  shows "relay unavailable until next month", which is false. Candidates:
  (a) `NOT_ADMITTED` plus a new-client hourly back-off, accepting the 2.3 retry
  rate as bounded cost (it stops when the release profile expires on 2027-10-07,
  epoch 1 on 2027-01-04); (b) `RELAY_BUDGET` to clients that do not advertise
  the new code (needs a version signal 2.3 does not send). (a) is proposed.
  R02 must measure the 2.3 retry rate on a fixture.
- *Who would be refused.* Every 2.x release embeds the hosted profile
  (`internal/network/release-profile.json`, epoch 2, operator Caleb Habesh), so
  unknown installs keep reaching the VPS until the allowlist is deployed (R06)
  or the epoch expires. R04 alone does not stop them.
- *Pairing mailbox.* `pairing` already calls `admit` and rate-limits
  create/claim by source (burst 5, 1 per 10 s). The proposed joiner exception is
  "kind `pairing`, purpose `enrollment`, mailbox created by a listed owner",
  plus enrollment relay toward that owner under `MaxUnknownOffers`.

## RG2 — Route accounting (R01)

Question: what Orbit counts so direct versus relayed use is visible and honest.

Candidates: HTTP body bytes and request counts of completed peer requests
(payload, not TLS or framing) by actual socket provenance after authentication;
classes `lan`, `wan_tcp`, `quic`, `relay`; daily rollups for 90 days in
owner-only state; displayed as shares with the window and "observed by this
device".

Evidence needed: classification per route; restart persistence; display tests;
overhead negligible on a 1-GiB transfer.

**R00 scoping:** R00 confirmed only the counting point. Route provenance is
known after authenticated HTTP in `internal/network/manager.go` (W08/W11
observations). Counters should hook the completed peer request there, not the
service client: service control traffic is separate and small (see below), and
counting it would mislabel the relay share. Keep the measured idle service
cost below as a reference line in `--routes` output only if R01 decides to.

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

**R00 scoping:** `config.AdoptPackagedProfile` only *writes* the stored
selection from the embedded one (when Automatic is awaiting a profile, or to
advance an epoch under the same authority). `app.go` calls it only when
`network.BuiltinProfile()` returns a profile. The daemon then reads the stored
selection. This supports candidate 1. R04 must still prove two things with a
build lacking the embedded file: (i) a stored release selection under
`ReleaseAuthority` stays usable (the constant stays in the binary); (ii) an
Automatic install with `awaiting_profile` and no embedded profile reports "No
Orbit service configured" rather than waiting. Epoch advances for the owner's
devices would then come from `orbit network set`/review, not from packages.

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

**R00 scoping:** existing namespace tooling is `scripts/validation/wan_netns.sh`
(root netns + veth + NAT; rootless namespaces are blocked on the laptop and
VPS) and `scripts/validation/wan_native.py` (never run two runners at once).
W16 namespaces are all torn down; recreating any on the Pi or laptop needs fresh
owner approval. The owner's PC firewall drops inbound except SSH, which makes
"PC ↔ laptop/Pi" a natural punch-or-relay cell without extra rules. Reusable
earlier evidence: W16 native runs, E09 relay framing overhead (0.22%), W13
Pi/rehearsal resource figures. Cite these rather than re-running.
