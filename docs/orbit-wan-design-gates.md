# Orbit WAN design gates

Status: 2026-10-05: **WG1–WG3 closed for local design composition** by
[W01 outcomes](implementation/wan-contracts.md) and
[executed evidence](evidence/wan-w01-20261005/summary.md). W02 executes the manual manager/state-model integration; W03 authenticates the
production local directory/profile/control path; W04 records live encrypted relay acceptance. Production W05 routed-enrollment acceptance is recorded in the tracker; ordinary
CLI/TUI setup acceptance remains with W06/W07; W12 owns qualified status/doctor and privacy-control presentation. WG4 full local composition closed by W09/W10; WG6 pending; WG5 has W01/W02 model/manual integration evidence and
retains W11 native roaming/timing closure ownership. The owner approved native WAN and hosted
defaults; these gates resolve implementation compositions inside that direction.
Execute them in marked disposable environments. Planning/research alone closes
none. Record canonical fixtures, commands, versions, observed results, decision,
owning-spec edits and limitations under the owning W packet.

## WG1 — Peer transport and inner encryption

Owner W01; required before W02/W04. Default: `http.RoundTripper` injection,
direct HTTPS and WSS binary stream with existing inner pinned device TLS/HTTP.

Build a two-peer disposable spike through production TLS/HTTP handlers: direct
and relayed request/chunk, correct/wrong pin, missing client certificate,
enrollment isolation, timeout/cancellation and relay leg closure. Inspect broker
input/output at its decrypted outer-TLS boundary using synthetic fixtures: no
plaintext invitation token, filename, manifest or content can cross it. Verify
inner TLS ALPN and HTTP transport behavior; forbid redirects/proxy fallback and
zero-match tests. Demonstrate fixed-buffer backpressure under a slow receiver.

Freeze transport/authentication ownership, listener adapters, TLS cloning and
handler isolation; verify the public seam against actual direct and relay adapters.
Close only with packet/interface tests and bounded-resource results. Fix a failed
composition in its adapter rather than bypassing pinning or changing sync semantics.

## WG2 — Registration, routing, profiles and privacy

Owner W01; required before W03/W04. Default: existing ID plus key pin as directory
key, signed proof/challenges, signed operator profile and purpose-bound relay legs.

Deliver strict wire schema/canonical signature fixtures and an independent small
admission/replay model. Exercise competing keys for the same claimed DeviceID,
wrong pin/profile/purpose, duplicate/replayed/expired nonce, registry restart,
lease replacement, over-limit allocations, stale candidate generation, URL/SSRF
classes, absent/expired profile and untrusted forwarded headers. Freeze public
versus LAN candidate scope, metadata visibility, initial numeric bounds and the
profile authority/rotation process. State which caches are ephemeral.

Acceptance: another key cannot overwrite a known ID/pin route; unauthenticated
lookup/registration is refused; a directory record cannot establish initial peer
trust; the broker cannot dial a supplied URL; a profile cannot expose owner control.
Freeze exact role-bound token encoding and library/provider choices before W03.

## WG3 — First-time routed enrollment and version coexistence

Owner W01; required before W05. Default: additive invitation/enrollment v3 with
logical routes; preserved v2 transcript and manual behavior.

Extend an independent admission model and executable TLS spike for devices never
sharing a LAN. Test unknown requester relay admission without folder access,
inviter authentication before capability disclosure, exact reviewed approval,
wrong folder, separate same-key folder requests, retired identity, replay,
expiry/revocation, lost response, daemon restart and changed candidate addresses.
Verify approval/retirement artifacts through existing membership code.

Freeze v3 canonical bytes, profile/certificate transfer, private persisted fields,
capabilities and explicit unsupported-mode errors. Demonstrate v2/v3 coexistence
without changing old signed bytes or device IDs. Close the design gate with a
model plus executable transport evidence; W05 still owes production acceptance.

## WG4 — QUIC, HTTP3 and ICE packet ownership

Owner W09 (transport subgate), completed by W10 (traversal); required before W11.
Default: native HTTP3 over quic-go using a packet adapter of a Pion-owned ICE pair.

Pin exact versions and inspect actual exported APIs/implementation rather than
assuming documentation signatures match. First prove pinned HTTP3 handlers on
ordinary UDP in W09; W10 proves the full ICE adapter. Exercise MTU boundaries,
datagram preservation/truncation refusal, remote/local addresses, one socket
reader, deadlines/cancellation, duplicate packets, ICE role conflict, pair-change
rebuild, req.TLS and certificate verification, header/body limits, no0RTT and
resource cleanup on Pi-compatible builds. Freeze adapter addresses within a
generation and rebuild on selected-pair changes. Verify separate ICE roles and
HTTP3 listen/dial roles, both pull directions, and deterministic offer deduplication.
Map HTTP3 request/body/header deadlines explicitly; a net/http configuration is
not sufficient. Measure large-stream progress.

If native HTTP3 fails the security/handler seam, evaluate only the documented raw
QUIC-stream/inner-TLS alternative. Record costs and revise architecture/protocol
before proceeding. If neither passes, leave the direct milestone incomplete;
relay remains a usable earlier milestone. Do not disguise raw ICE as a byte stream.

## WG5 — Connection policy, roaming and bounded progress

Owner W02 for state model, W11 for production evidence. Default: finite direct
head start, relay fallback, per-peer/global limits, network generations and draining
in-flight requests rather than seamless migration.

Model stale responses, concurrent candidates, route failure after allocation,
outages, reordered network changes, cancellation and flapping. Prove no downgrade,
no pin change, no new logical mutation ID after uncertain delivery, no false
receipt, bounded jobs/resources and continued local capture. W11 validates timing
defaults under latency/loss, slow relay and Pi load, plus fair progress for large
and small transfers. Separate model evidence from native route-switch evidence.

## WG6 — Operated defaults and release exposure

Owner W13; required before W14 default distribution and W16 hosted-default tests.
Default: a concrete signed hosted profile with optional separate self-host profile;
one operator instance with declared finite budgets and monitoring.

Record actual operator/service origins, DNS/TLS ownership, profile signing and
rotation, hosting/egress limits, admission policy, privacy/retention text, backup
needs, monitoring, incident/rollback and service shutdown procedures. Produce
packages and local rehearsal before any external activation. Resolve missing
deployment credentials/authority using the authorization available at execution
time; planning does not provision accounts, spend money or alter a live VPS.

Exercise expired profile/certificate, endpoint rotation, overload, service restart,
STUN amplification limits and self-host custom trust. Default release requires
reachable validated origins; placeholders and overridden localhost profiles are
explicitly development-only. Failure to arrange an operator leaves hosted-default
release incomplete, not an excuse to claim effortless self-host setup.

### WG6 W13 interim record — open, 2026-10-06

Local composition is closed: packaged `orbit-net`, operator key/profile tooling,
two-epoch rotation overlap, reviewed custom self-host trust, certificate reload,
sanitized monitoring, finite budgets, STUN/overload limits, restart/disable and
rollback behavior pass the [W13 rehearsal](evidence/wan-w13-20261006/summary.md).
The owner then deployed the release service on their Oracle VPS
(`https://connect.calebhabesh.com:8443`, Let's Encrypt with automatic renewal,
release profile epoch 1, finite budgets) and a live laptop↔Pi pairing synced
through it ([deployment](evidence/wan-w13-20261006/deployment.md)). The gate stays
**open** only for a second offline authority-key copy and a configured
alert/on-call destination. The
[operator runbook](orbit-net-operator.md#hosted-default-readiness-wg6) lists the
exact records that close it. W14 default distribution and W16 hosted-default
tests stay blocked on them; self-host mode does not depend on them.

Owner-directed sequencing amendment, 2026-10-06: the offline backup and working
alert destination are deferred operator actions. W15 isolated failure/security/
resource development may proceed on W13's implemented technical evidence.
WG6 stays open; wider package distribution, W16 hosted-default acceptance and
W17 release still require both actions with evidence. See
[the plan amendment](orbit-wan-implementation-plan.md#owner-directed-sequencing-amendment--2026-10-06).


### WG6 W13 outcome — closed, 2026-10-07

Both remaining operator records now exist; the interim record above is retained.

- **Alert/on-call destination:** `orbit-net alert` runs every minute on the VPS
  from a separate binary/timer (the production service was not replaced or
  restarted) and posts firing/resolved transitions to the owner's private ntfy
  topic. On-call contact: the owner (Pixel 6a ntfy subscription). A live drill
  at 2026-10-07T00:28:16Z–00:28:47Z (test, synthetic FIRING down, RESOLVED down)
  was received as live notifications, confirmed by the owner; the earlier
  23:51Z drill was received from the ntfy cache
  ([evidence](evidence/wan-w16-20261006/wg6-alert/summary.md)).
- **Second authority-key copy:** the owner attests that the authority key,
  signed release profile and public authority are stored in an encrypted
  Bitwarden vault item, separate from the workstation copy. The owner waived
  recording the restored-copy `orbit-net key verify` result, so restore
  verification is **unexecuted** and owner-attested custody is the evidence.

Hosted-default W16 tests and W14 package distribution are no longer gated by
WG6. Their own acceptance criteria still apply.

### WG5 W08 TCP defaults (partial integration, gate remains open)

[W08 architecture](orbit-wan-architecture.md#w08-direct-tcp-and-local-discovery-integration)
freezes optional `:0` peer-data listener, 64 pre-TLS incoming sockets, eight selected
interfaces, multicast TTL 1 / UDP 22027, 1,200-byte records, three startup announcements
at five-second spacing followed by 120–135-second renewal, ten-minute scoped leases,
20 admitted datagrams/second, 16 aggregate candidates/target, 200-ms family stagger,
two candidate attempts and existing global 32 raw-dial/64 outgoing-socket bounds.
Direct-only budget is three seconds; relay-enabled direct budget is 750 ms before
pinned fallback. Full handshakes and losing workers are joined on shutdown. Actual
local/race/resource and isolated public-scope evidence is linked from the tracker.

These are supported W08 defaults, not native roaming/Pi timing tuning or full WG5
closure. Restart-to-relay repetitions still expose route/quota recovery failures;
W11 owns their measured cooldown/generation repair. Its fair-progress, direct
reprobe and network-change campaigns remain required. W08 fixture collision
acceptance instead starts fresh ordinary onboarding with the optional port occupied.

W14 (2026-10-06) embeds this release profile in ordinary builds and verified it
natively with the owner's laptop and Pi. Because every package is now a
hosted-default distribution, publishing packages beyond the owner still waits
for the two open WG6 items (offline authority-key copy, alert destination).

## WG4 transport subgate outcome — 2026-10-06

**Closed for W09 native UDP HTTP3 and the established-pair test adapter.**
[W09 evidence](evidence/wan-w09-20261006/summary.md) and
[tagged dependency/API audit](evidence/wan-w09-20261006/dependency.md) select
quic-go v0.63.0 / qpack v0.6.0 with pinned Pion ICE v4.4.6 packet API compatibility.
Native HTTP3 passes replication TLS/req.TLS, mandatory certificate/ALPN, current
borrower pin, per-request authority, size/deadline and no-early-data requirements.
Real native UDP and one socket/transport behind a synthetic pair listen/dial both
pull directions with verified chunks/heads/hashes and actual loss/duplicates.
Packet MTU/truncation/address/deadline/close, session/resource/backpressure and
real daemon/PTY/package checks pass. The raw stream/inner-TLS alternate is not
selected. [Architecture](orbit-wan-architecture.md#w09-native-quic-http3-integration)
freezes exact limits and lifecycle; failed runs stay uncredited.

**WG4 remains incomplete until W10** establishes actual Pion pairs, authenticates
coordination, resolves ICE role/session conflicts, rebuilds on selected-pair/network
changes and executes the NAT/MTU/loss/shutdown campaign through the full composition.
W09's compile-time Pion contract and synthetic pair do not close these conditions.
W11 remains dependent on full WG4, not only this transport outcome.


## WG4 full ICE composition outcome — 2026-10-06

**Closed for W10 production integration and local native/emulator acceptance.**
[W10 evidence](evidence/wan-w10-20261006/summary.md) supersedes the pending full
integration statement in the historical W09 outcome. Pion ICE v4.4.6 owns raw UDP;
one frozen selected-pair adapter feeds one quic-go v0.63.0 transport/HTTP3 endpoint.
The existing replication TLS/authority/receipt rules remain authoritative. Signed
identity/purpose/session/generation/expiry and deterministic controlling roles
coordinate actual agents; concurrent requests coalesce into bounded pairs.
Supported configured NAT/filtering paths pass two-way sync with heads/hashes and
chunk reuse; incompatible/double NAT and blocked peer UDP pass separate WSS
fallback. Actual encrypted loss, MTU/slow-stream/cancellation, consent-loss closure
and fresh-pair rebuild, directory outage, shared QUIC admission and shutdown pass.
Native UDP4 ICE and UDP6 STUN execute twice in a new marked isolated namespace.

[Architecture](orbit-wan-architecture.md#w10-authenticated-ice-integration) records
socket/source/candidate/session/time bounds and runtime ownership; credentials are
ephemeral, cleared on close and never persisted, without a heap-erasure claim.
The controlled shutdown/rebuild fixture closes composition recovery, not W11
interface-detection, native timing/fairness or physical WAN/Pi acceptance. W11 is
now eligible; WG5 native policy and WG6 operated services remain open, as do
inherited T13 technical checks and deferred P17 owner use/explanation.


## WG5 W11 policy core outcome — partial, 2026-10-06

[W11 evidence](evidence/wan-w11-20261006/summary.md) adds authenticated handshake
races, shared outgoing admission, demand-driven direct reprobes/exponential
cooldowns, quota quiet refill, joined actual Linux network observations and
busy-generation draining. Real native QUIC→TCP→QUIC preserves verified chunks,
receipt replay, original heads/authors/hashes and working bytes. An isolated
namespace measures address/default-route detection; queue retry/reload tests
force bounded selection independent of declared size. Numeric local defaults are
frozen in [architecture](orbit-wan-architecture.md#w11-route-policy-and-roaming-integration-partial).

**WG5 remains open.** These are partial local evidence. Advanced timing controls,
Pi/latency/loss tuning, whole-daemon relay/Wi-Fi/membership-polling switches,
slow-service campaigns and measured mixed-transfer bandwidth/combined-resource
fairness still require implementation/evidence. No M3 closure or physical WAN/Pi
performance guarantee is inferred from these tests.


## WG5 completion outcome — 2026-10-06

**Closed for W11 production integration, local native/emulator and actual Pi
acceptance.** The [completion follow-up](evidence/wan-w11-followup-20261006/summary.md)
supersedes the historical partial WG5 outcome above. Reviewed timing, exact legacy
policy serialization, bounded authenticated transport races/reprobes/cooldowns,
network generation rebuilding and queue aging have actual checks. Full daemon
relay/QUIC and native address/default-route campaigns preserve chunk reuse,
committed receipt retry, peer pins, identities and heads/authors/hashes/working
bytes. Production-default timings pass on Raspberry Pi 4B with actual isolated
25 ms/1% netem impairment. Mixed 16 MiB/continuous-small progress across three peers
uses actual WSS/TCP/HTTP3 and measures combined socket/RSS/FD/heap/goroutine/CPU
resources. Thirty-two competing delayed DNS/relay peers cancel/join within bounds.

The initial aggregate integration control-query timeout is retained; minimized
read-only wait regressions, repeated real pairing, full integration and affected
CLI race journeys pass after repair. All remaining check targets pass. No failed
aggregate or fixture receives success credit. This closes WG5/M3 for the declared
native isolated/local/Pi conditions; it does not close WG6 operated defaults,
W16 physical WAN or inherited T13 lifecycle requirements.
