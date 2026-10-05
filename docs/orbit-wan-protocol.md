# Orbit WAN network protocol contract

Status: 2026-10-05 design baseline; exact wire schemas, golden bytes and executable
gate evidence are W01/W09 deliverables. Existing [causal protocol](protocol.md),
membership encoding, version IDs, chunk hashes and receipt boundaries remain
authoritative. This document owns the new routing protocol, not file causality.

## Identities and trust

Use the existing persistent random 32-byte DeviceID and existing Ed25519 device
key. The pin is SHA-256 of certificate SPKI as today. Directory records are keyed
by `(DeviceID, key pin)`; verify proof of possession of that key. No new identity
derivation, key rotation or approval occurs on an address/route change.

Outer infrastructure connections authenticate the approved service origin with
standard TLS certificate verification. Application registration/challenge proofs
authenticate devices, allowing a new device to reach enrollment without being a
folder member. The operator's profile authority authenticates service profiles;
it has no authority over device keys or membership. Admission to a network service
is distinct from admission to a folder.

Pinned inner TLS authenticates the inviter before transmitting any invitation
capability. Enrolled data sessions require existing device mTLS and the existing
per-request folder/member/revision checks. Relayed bytes and direct HTTP3 requests
must reach the same checks. Keep enrollment and data destinations distinct.

## Encoding and signed messages

W01 delivers `schemas/network-v1.md`, additive control schema/capabilities and
golden fixtures. Use bounded strict JSON with duplicate/unknown-field rejection;
canonical unsigned decimal strings for counters, sizes and timestamps; lowercase
hex fixed-size IDs/pins/nonces/digests. Endpoint types explicitly distinguish
service origin, TCP candidate, UDP candidate and relay route. Canonical signature
input is length-prefixed binary fields, not arbitrary JSON serialization.

Every authenticated control exchange binds the domain `orbit-network-v1`, message
kind, profile/service origin, sender ID/pin, target ID/pin when applicable, purpose,
fresh server challenge, operation/attempt ID, expiration and canonical payload
digest. Initial authentication includes certificate DER and Ed25519 key-possession
proof. Subsequent signed peer-coordination messages bind both peers, a unique
session ID, candidate generation, role and expiration. WG2 freezes exact field
order/limits, key representation and signature vectors before W03.

Replay cache entries expire safely within a bounded window; accepted nonce reuse
cannot create duplicate reservations or change a signed record. Invalid signatures,
unknown versions, wrong purpose/profile, over-limit fields and expired challenges
fail before expensive work. Do not silently accept missing authentication fields.
State-changing registration/reservation operations are idempotent under an
operation ID; repeated IDs with changed inputs fail. Session credentials are
not long-lived folder capabilities.

## Rendezvous and coordination

The baseline offers HTTPS control plus an authenticated WSS channel. Logical
operations (exact paths/envelopes frozen at WG2): authenticate, announce, look up
the exact known identity/pin, exchange an addressed connection offer/accept, reserve
a relay session, and release. There is no directory enumeration operation.

An announcement contains signed identity/pin, bounded transport capabilities,
generation, lease expiration, eligible public candidates and relay availability.
Lookup returns candidate records with their signature/lease and service-scoped
relay hints. Stale/invalid records are ignored and diagnosed. A service's observed
source IP is an observation, not proof that a TCP/UDP peer listener is reachable.
Only tested reachability or a successful pinned handshake validates a usable path.

Initially all service operations require authenticated device proof; lookup of a
known identity/pin is rate limited rather than treated as folder authorization.
Device IDs and pins are not private lookup capabilities. Someone who knows them
may learn published address/availability metadata; disclose that in the profile
privacy text. The directory receives no folder names, paths, invitation secrets,
version inventories or content. It can observe device IDs, addresses and timing.

Connection offers carry sender/target identities/pins, purpose, generation and
session ID. An enrolled receiver verifies the sender against locally known peers;
it can allow a route while folder data still requires current per-request authority.
Enrollment offers from unknown devices enter a separate bounded admission path.
The requester proves its own key, but that proof does not approve a folder.

Only the initial invitation and inner enrollment exchange carry folder/capability
information. A relay offer exposes peer routing identities and `enrollment` purpose,
not the invitation token. The inviter can decline an unknown route at capacity;
the joining device retains its reviewed operation and reports a retryable block.

## Encrypted relay sessions

Both endpoints attach outbound WSS binary streams to an admitted rendezvous
session. Short-lived attachment credentials bind session, exact ID/pin, leg/role,
purpose, service origin and expiration; each leg attaches at most once. The
broker checks proof, purpose, partner acceptance and quotas before forwarding.
Its configured service directory selects destinations; it never opens arbitrary
caller-supplied network URLs. Session expiry/abort closes both legs and releases
buffers and reservations. A service restart closes sessions and invalidates tokens.

The attachment token/reservation expires after 30 seconds if unused; a successfully
attached active tunnel uses a separate 60-minute maximum lifetime, idle deadline
and byte/quota limits. At lifetime expiry drain the current finite request, then
close. Subsequent verified-chunk work may allocate a new tunnel. A long transfer
must not fail every 30 seconds because the original attachment credential expired.

Inner device TLS negotiates only after routing succeeds. A wrong inviter pin fails
before a challenge containing the folder invitation secret is sent. Peer data
uses mTLS; enrollment uses the existing isolated possession-proof protocol and
explicit approval. The relay cannot substitute a device certificate or downgrade
to plaintext. Public control/owner routes are not registered or tunnelable.

Broker storage is transient bounded buffers. Its acknowledgements mean tunnel
allocation or forwarding, never durable file receipt. Peers remain responsible
for verified chunks, durability, application progress and idempotent operations.
A relay route preserves the end-device identity of a direct receipt.

## Routed enrollment v3

The current v2 transcript binds numeric endpoints and is preserved unchanged for
legacy/manual operation. Introduce an explicitly negotiated invitation/enrollment
v3 for logical routing; **do not reinterpret v2 URL strings** as service routes.

The v3 invitation transfers the existing folder ID, exact inviter ID/pin and public
certificate, expiring scoped request capability, approved profile reference and
logical enrollment route. It includes sufficient trusted profile data for a
self-hosted receiver to review/select that operator. A downloaded directory record
cannot supply the inviter's initial trust anchor. Secret input uses a prompt,
stdin or private file, never ordinary argv/history/logs. Set a bounded invitation
size (initially 16 KiB); compact encoding is a convenience, not a short numeric PIN.

V3's signed request/status/approval transcript binds both identities/pins, exact
folder, prior membership digest, attempt/challenge, capability digest, profile and
logical inviter/requester route references, expiration and the existing review
context. Mutable candidate addresses are outside folder authority and may change
without renewing enrollment. A route reference never changes the pin/profile in
the already-reviewed transcript.

Existing single-use capability, replay guards, explicit owner approval, exact
membership artifact, retired-identity rejection, bootstrap retirement snapshots,
per-folder sharing and receiving-root preview remain in force. V3 network failure
cannot replace an identity/attempt, extend authorization or mark setup complete.
The private receiving setup record persists what is necessary to resume the same
workflow; ordinary status omits capability bytes. WG3 freezes proof/approval
canonical bytes and an independent admission model before W05.

New devices negotiate v3 explicitly. V2 peers continue reachable manual/private
endpoints. Unsupported automatic routing returns a specific capability error and
an explicit manual-mode path, without unpinned retries or silent proof downgrade.
The version envelope and canonical membership schema stay v1.

## Direct routes and traversal

Local discovery sends minimal bounded announcements with protocol version,
device ID/pin, signed candidates, generation/expiry and capabilities. No folder
names or invitation secrets. Only the selected local interfaces/scopes receive
broadcast/multicast; advertise actual usable addresses, never loopback/unspecified.
A received identity is a candidate, not an approval. Automatic dialing is limited
to known pinned peers; unknown devices appear only when the owner explicitly opens
a nearby-device discovery view. Nearby discovery does not auto-share files.

Direct public TCP/IPv6 uses the existing pinned HTTPS exchange. Race candidate
families with finite limits and delays rather than sequentially waiting on every
dead address. Direct reachability is permitted where network policy allows it.

For UDP, exchange ICE credentials/candidates only over authenticated coordination
and bind their envelope to both known peer pins and an expiring session/generation.
Order the exact `(DeviceID, pin)` pairs lexicographically: the smaller is the ICE
controlling/session-offer initiator. The other sends a signed connect request;
the initiator creates at most one offer/session nonce for the peer pair, profile,
purpose and signed two-sided network-generation tuple. Concurrent requests reuse
that offer; superseded offers/credentials are cancelled. Library ICE tie handling
is supplementary rather than the session deduplication mechanism.

Both devices start their peer QUIC/HTTP3 listener before initiating client requests.
Each may dial a separate authenticated QUIC connection over the single ICE packet
transport, so both existing pull directions work. ICE role does not select a sole
HTTP client. ICE selects an authenticated-check path, then device
QUIC/TLS still verifies the peer and membership gates still protect requests.
ICE credentials authorize path checks, not file access. STUN services can observe
the source address and remain subject to resource/amplification limits.

Pion owns the UDP receive path. The packet adapter preserves one datagram per
read/write, selected-pair addresses, MTU, deadlines and shutdown; no `tls.Client`
on raw ICE Conn. Preferred QUIC HTTP3 preserves `req.TLS` and existing HTTP request
semantics. Freeze LocalAddr/ReadFrom remote addresses for one generation and rebuild
on selected-pair change; reject misaddressed writes and truncated datagrams.
Explicitly implement equivalent header/body/request deadlines for HTTP3 instead
of assuming net/http Server fields apply. Keep mandatory client-certificate
handshake plus the existing request-level DeviceID/SPKI/membership authorization:
an unenrolled certificate may complete TLS and must still fail data authorization.
Disable early data and 0-RTT. W09/W10 fixtures must verify wrong pins,
malformed packets, header/body caps, retransmission, duplicates and cancellation.
No seamless ICE-pair/network migration guarantee is made: rebuild on generation
changes and use existing safe retries. Relay remains available when UDP is blocked.

## Engineering admission defaults

These are initial finite bounds to verify/tune, not measured service capacity.

| Item | Planning bound |
| --- | --- |
| Network metadata/header | 64 KiB body, 16 KiB headers; validate before allocation |
| Discovery datagram | 1,200 bytes; pagination is not UDP amplification |
| Candidates | 16 per device/purpose, 8 per offer, 32 active peer slots per daemon; retain configured peers and schedule them fairly |
| Authentication | Challenge 60 s, replay window 5 min, bounded caches |
| Candidate lease | 10 min; reannounce every 2 min with jitter |
| Relay reservations | 30 s to attach; 10 s inner handshake deadline |
| Control heartbeat | 25 s; stale connection checked within 75 s |
| Relay forwarding | 32 KiB buffers per direction; bounded library buffers; no file-size allocation |
| Daemon sessions | 8 active data tunnels globally, 2 per peer; 2 pending unknown enrollment tunnels |
| Service sessions | 64 active data tunnels, 128 device control channels, 128 pending reservations per initial operator instance |
| Idle relay | 60 s without traffic; refresh through bounded active protocol traffic |
| Admission rate | 60 metadata operations/min/device, burst 10; stricter unknown enrollment limits from existing contract |

Per-IP limits are supplementary and account for shared NATs. Inner enrollment
adds per-key/global limits; a relay's socket IP cannot be treated as the remote
end-device IP. Untrusted forwarded headers grant no identity/rate exemption.
Operator deployment must choose finite aggregate/per-device bandwidth and egress
budgets (initial test profile 20/5 MiB/s), connection/FD/memory ceilings and monitoring.
Refusal produces bounded typed backpressure. W15 measures actual memory/FD/Pi
behavior; increase a bound only with evidence and matching configuration/spec edits.

## Failure and privacy

Directory/relay/STUN outages affect route availability, not captured history or
membership. Unavailable services do not block local capture or an established
valid direct path. Expiration, overload, profile incompatibility and blocked routes
are distinct typed failures. A fresh connection is not a claim that files are
stored/applied. The [UX](orbit-wan-ux.md) owns their human presentation.

Support exports redact invitation/ICE/relay credentials, private filenames and
full endpoint details by default. Logs use bounded IDs and aggregate diagnostics;
the operator declares exact retention and visible metadata. Local-only stops
internet announcements/coordination/STUN/relay, rejects public WAN candidates,
invalidates cached WAN routes and drains their direct pools as well as tunnels.
Only candidates scoped to permitted local interfaces remain. Mode changes neither
retire peers nor delete local/captured files. Self-hosting does not weaken TLS
verification; custom trust/profile choices are explicit reviewed inputs.
