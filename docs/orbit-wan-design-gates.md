# Orbit WAN design gates

Status: all gates **pending**, 2026-10-05. The owner approved native WAN and hosted
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
