# Network v1 and routed enrollment v3

Frozen by W01 local gate evidence; W02 manual/W03 directory integration is implemented;
relay/routed enrollment integration remains W04–W05.
Owners: `internal/protocol/network.go`, `routed_enrollment.go`,
`internal/network`, and `internal/control/terminalcontract/network.go`.
[Protocol](../docs/orbit-wan-protocol.md), [gate outcomes](../docs/implementation/wan-contracts.md)
and [independent fixtures](fixtures/network-v1/README.md) are complementary.

## Common encoding

JSON is uncompressed UTF-8, at most **65,536 bytes**, with exactly the specified
keys (case sensitive), including empty string/array sentinels. Unknown, duplicate,
missing, alias-case and null fields, trailing values and noncanonical integers
fail. `NetworkDecode` checks this closed shape; callers then validate semantic
fields and authentication **before allocation/admission**. Bodies are bounded
before reading; headers are at most 16,384 bytes. JSON whitespace counts toward
limits. The invitation envelope separately permits at most 16,384 bytes.

All integers, including versions, generations, epochs, expiry, retry delay and
array counts in canonical bytes, use unsigned decimal strings, no sign/leading
zeros except `"0"`. Integer data types span 0..18446744073709551615. Nonzero fields
below reject zero. Expiry is whole Unix UTC seconds; certificates use X.509 UTC
validity. Expiry is exclusive: `now >= expires` refuses admission. Window checks
subtract only after ordering to avoid overflow. All ID, SPKI pin, nonce, digest,
operation, session and restart-epoch fields are nonzero 32-byte lowercase hex
(64 characters). Ed25519 public keys are raw 32-byte lowercase hex; signatures
are 64-byte lowercase hex. Certificates are one X.509 DER certificate in
canonical padded standard base64, at most 4,096 DER bytes. Its public key must be
Ed25519; SHA-256 of DER SubjectPublicKeyInfo must equal the bound pin. DeviceIDs
remain existing independently generated random IDs, not public-key hashes.

Canonical bytes encode each field as uint32 big-endian **UTF-8 byte length**,
followed by its UTF-8 bytes. This includes the initial fields `orbit-network-v1`
and message kind. Hex/integer fields remain their canonical ASCII strings in
this new format. These rules do not change the v1 membership/version or v2
transcript format. Sign Ed25519 over these bytes directly; payload/transcript/
profile digests are SHA-256 of canonical bytes. JSON property order has no effect;
array order is signed and preserved. No delimiter concatenation or JSON signing.

## Origins, candidates and profiles

A service origin is at most 256 bytes, lowercase `https://host[:port]` or
`wss://host[:port]`, with no user, path (including `/`), query, fragment, percent
escape, trailing dot or explicit default 443. Ports are canonical decimal 1..65535.
DNS host labels are lowercase ASCII LDH, 1..63 bytes, without edge hyphens.
Loopback/localhost, unspecified, multicast, link-local and IPv4-mapped IPv6 are
always refused. Public addresses exclude private, shared CGNAT, documentation,
benchmark, special-use and transition ranges; public IPv6 is in 2000::/3 with the
listed exclusions in the owning validator. A reviewed self-host profile may use
private global-unicast addresses; this never permits owner-control loopback.

DNS syntax alone is not SSRF protection. W03 must resolve with bounded work,
check **all** answers with the same address policy, connect to a validated numeric
answer with original TLS hostname, and revalidate every reconnect. No redirects,
environment proxy, unpinned destination or supplied broker dialing URL is allowed.
No incoming forwarded header provides identity or a rate exemption.

Candidate object: `transport` (`tcp`/`udp`), `address` (canonical numeric
`netip.AddrPort`, nonzero port), `scope` (`public`/`lan`), `interface` (empty for
public, explicit UTF-8 local-interface name, at most 64 bytes, for LAN). Public
announcement rejects LAN candidates; LAN coordination requires a permitted local
interface and private address. Link-local candidates are unsupported initially,
including zone-bearing IPv6; scoped link-local support needs a later owning
validator change. No DNS candidate, malformed/mapped address or duplicate tuple.

Profile object fields, all required:
`version="1"`, `operator` (1..128 UTF-8 bytes), `authority` (profile Ed25519 public
key), `service_key` (online relay Ed25519 signer public key), `epoch` (nonzero),
`expires` (nonzero), `origins` (1..4 distinct service origins), `stun` (0..4 distinct
numeric AddrPorts), `privacy` (1..4096 UTF-8 bytes), `signature`.
Text excludes NUL/CR/LF; terminal clients must safely render all remaining text.
Canonical kind `profile`; order:
`version, operator, authority, service_key, epoch, expires, len(origins), origins..., len(stun), stun..., privacy`.

The profile key cannot authenticate itself: verify against a previously trusted
distributed authority or explicitly reviewed self-host key. Persist the highest
reviewed epoch and reject rollback, invalid signature, missing/expired profile.
The initial authority cannot rotate itself to an unrelated key inside this
object: distribute an authorized software/profile trust update or explicit review.
Online service-key/origin rotation increments the signed epoch and profile digest;
in-flight routes drain, new reservations bind the new profile. W13 records actual
signer custody, deployment, lifetime and privacy/retention; fixtures are synthetic.

Announcement object: `generation` (nonzero), `expires` (now < expiry <= now+600),
`candidates` (0..16), `capabilities` (0..5 distinct values:
`direct_https_v1`, `relay_inner_tls_v1`, `enrollment_v3`, `quic_ice_v1`, `quic_http3_v1`), `relay`
(boolean). Advertise only implemented capabilities; defining a type is not runtime
support. Canonical kind `announcement`; order:
`generation, expires, len(candidates), [transport,address,scope,interface]..., len(capabilities), capabilities..., relay` (last value `true`/`false`).

## Authenticated service operations

NetworkProof's required fields in canonical order:
`version="1", profile, origin, sender, sender_pin, target, target_pin, purpose,
challenge, operation, session, generation, role, expires, payload`.
JSON additionally includes `kind`, `certificate_der`, `signature`. Canonical kind
is the operation name below. The certificate and signature are not separately
signed fields: the verified key's SPKI pin is signed; the key verifies the proof.
Purpose is `peer_data` or `enrollment`, never owner control. Challenge lifetime
and proof expiry are at most 60 seconds; the service checks the exact issued
challenge, actor, profile, origin and expiry. Generation is unsigned, zero only
for non-candidate operations. Operation IDs always nonzero. Profile is its canonical
digest; origin must be one of the selected profile's origins of the correct kind.

`authenticate`/`announce`: target, target_pin, session, role are empty.
`lookup`: target and target_pin required; session/role empty.
`offer`/`accept`/`reserve`/`attach`/`release`: both target fields and session required;
role is `initiator` or `responder`. Exact pair/purpose/role is checked against the
admitted operation/reservation, not merely the signature. A competing key for the
same random ID can have a distinct `(ID,pin)` entry; it cannot overwrite/query as
the known pinned key. Initial directory trust never approves folder membership.

POST paths and closed envelopes:

| Path | Request | Response |
| --- | --- | --- |
| `/network/v1/challenge` | `version, profile, sender, sender_pin, certificate_der` | `version, profile, origin, challenge, expires` |
| `/network/v1/authenticate` | `proof` | `version, state, operation` |
| `/network/v1/announce` | `proof, announcement` | `version, state, operation` |
| `/network/v1/lookup` | `proof` | `version, found, proofs, announcements` |
| `/network/v1/offer`, `/network/v1/accept` | `proof, offer` | `version, state, operation` |
| `/network/v1/reserve` | `proof` | `version, attachments` |
| `/network/v1/release` | `proof` | `version, state, operation` |

Challenge is a bounded pre-auth operation, not lookup/registration; public TLS
and the selected profile precede it. Returned proof/announcement arrays have
zero elements on absent record, exactly one matching signed record on found.
The proof's historical authentication expiry does not extend a candidate lease;
lookup checks the record signature and announcement lease. Offers and acceptance
use `sender_generation, target_generation, candidates` (0..8, public or explicitly
scoped LAN). Canonical kind `offer`; order those two generation strings,
`len(candidates), [transport,address,scope,interface]...`. ICE credentials/proofs
and HTTP3 negotiation are a WG4 extension; no QUIC/ICE capability ships in W01.

Proof payload is SHA-256 of canonical announcement/offer bytes where present.
For authentication, lookup, reserve, attach and release it is SHA-256 of
`NetworkCanonical(kind+"-intent", profile, origin, sender, sender_pin, target,
target_pin, purpose, operation, session, generation, role)` (same outer proof
bindings excluding challenge/expiry). Only exact signed payloads are admitted.
State-changing operation replay keys include actor ID/pin, profile, purpose, kind
and operation; semantic digest includes all intent bindings and payload. A fresh
challenge may retrieve the same result; changed semantic inputs conflict. Consumed
nonces cannot allocate again. Cache lifetime is 300 seconds, maximum 1024 entries;
new work at capacity is refused rather than evicting live replay protection.
These ephemeral routing operations expire/restart independently of durable file
mutations/enrollment, whose original idempotency semantics remain unchanged.

GET WSS `/network/v1/control` and `/network/v1/relay` use HTTP/1.1 upgrade,
compression disabled and reject **every nonempty browser Origin**. Control first
receives a bounded authenticated proof. Relay first receives `proof, attachment`
with proof kind `attach`; no payload is forwarded before admission. Admission JSON
is at most 16 KiB, then all forwarded frames are binary and at most 32,768 bytes.
Reservations require partner acceptance; never put credentials in URLs/logs.

RelayAttachment required fields, canonical kind `relay-attachment`, order:
`profile, origin, epoch, session, device, pin, partner, partner_pin, purpose, role,
expires`. JSON adds `signature`, signed by the reviewed profile's **service_key**.
Epoch is a fresh server-restart nonce. Attach within 30 seconds, match both exact
pins/role/purpose/current profile/epoch, each leg once. Abort/release closes both
legs. An attached tunnel has a separate maximum 60-minute lifetime and 60-second
idle limit, finite request drain and quotas; token expiry is not active-tunnel
expiry. Broker forwarding never means durable storage or approval.

Errors: `version="1", code, retryable, retry_after` (decimal seconds).
Codes: INVALID_REQUEST, UNSUPPORTED_CAPABILITY, PROFILE_MISSING, PROFILE_EXPIRED,
PROFILE_UNTRUSTED, IDENTITY_MISMATCH, PURPOSE_MISMATCH, CHALLENGE_EXPIRED,
REPLAY_REJECTED, STALE_GENERATION, IDEMPOTENCY_CONFLICT, QUOTA_EXCEEDED,
ROUTE_UNAVAILABLE, SERVICE_UNAVAILABLE, CANCELED. Invalid inputs are 400,
auth/purpose failures 403, stale/idempotency conflicts 409, quotas 429 with bounded
Retry-After, unavailable service 503. No raw secrets/addresses in errors.

## Routed enrollment v3

Route object: `device, pin, profile, purpose="enrollment"`. No mutable candidate
address. Both routes in a transcript use the same reviewed profile digest.
Invitation: `version="3", folder, inviter_route, certificate_der, capability,
expires, profile` (full signed profile). Explicit private transfer establishes
inviter certificate/pin; separate profile trust/review is mandatory before use.
Invitation Decode/Validate enforces 16 KiB, exact certificate pin and expiry.
Changing an address does not renew an invitation or alter a reviewed route/pin.

Request: `transcript, capability, certificate_der, signature` (16 KiB HTTP bound).
Transcript canonical kind `enrollment-request-v3`, fields:
`version="3", folder, inviter.[device,pin,profile,purpose],
requester.[device,pin,profile,purpose], capability_digest, attempt, challenge,
public_key, prior_membership, expires, label` (0..256 UTF-8 bytes).
Capability digest is SHA-256 of decoded 32-byte capability. Requester certificate
pin and raw key must match the transcript; Ed25519 signs these canonical bytes.
Inviter TLS authentication precedes any capability-bearing challenge/request.
Approval expiry/challenge/capability usage remain checked by durable admission.

Request ID = SHA-256 of canonical kind `enrollment-attempt-v3`, fields
`folder, inviter.device, inviter.pin, requester.device, requester.pin, attempt,
inviter.profile`. Different folders using one key cannot collide. Verification
code is the first 80 bits of canonical transcript digest, hex in four groups of
five. Request ID is stable across a refreshed challenge; approval review binds
the exact refreshed transcript digest, not merely its request ID.

Status required fields: `version="3", request, transcript_digest, requester,
nonce, expires, signature`. Canonical kind `enrollment-status-v3`, order:
`version, request, transcript_digest, requester.[device,pin,profile,purpose], nonce,
expires`. Fresh one-use status challenge; verify the admitted requester key.
Approval review: `version="3", request, transcript_digest, prior_membership,
membership_digest`. Canonical kind `enrollment-approval-v3`, same order. It binds
the exact reviewed artifact encoded by existing canonical v1 membership rules.
Do not install a model-approved artifact without the existing membership checks.

Closed v3 POST paths/envelopes are `/enrollment/v3/challenge` with
`version, folder, inviter, requester, capability, attempt`; response
`version, challenge, expires, prior_membership`; `/enrollment/v3/request` with
RoutedEnrollmentRequest; `/enrollment/v3/status` with RoutedEnrollmentStatus.
An initial status nonce request uses the same exact fields with empty nonce/signature
and expiry zero, and receives RoutedChallengeResult; it discloses no artifact.
Final request/status result: `version, request, state, transcript_digest,
verification_code, membership_hex, retirement_snapshots, inviter_route`.
MembershipHex is empty until approved; retirement_snapshots is a present array
of canonical v1 retirement snapshot bytes encoded lowercase hex, checked by the
existing decoder. Inviter route echoes the reviewed enrollment reference; a data
Target using that same key is permitted only after installing approved membership.
All HTTP envelopes have the 16 KiB enrollment bound; W05 must use the existing
bounded peer artifact transfer when retirement snapshots exceed a single result,
rather than dropping required retirement evidence or increasing the body limit.

The v3 production challenge/status/approval and artifact bootstrap handler
integration are W05, using these frozen signed bindings and existing membership/
retirement snapshots. Gate fixture HTTP routes do not add production v3 support.
V2 stays `/enrollment/v2/...` with unchanged signed bytes/manual endpoints;
explicit version/capability negotiation refuses unsupported automatic routing,
never silently translates or downgrades a proof.

## Additive terminal contracts and migration

Keep terminal-control-v1's existing version and result unchanged. New capability
names `network_control_v1` and `enrollment_v3` are **reserved**, not advertised in
W01. NetworkPolicy: `mode` (automatic/local_only/manual/self_hosted), `profile`
(digest required for automatic/self_hosted, empty otherwise), `lan_advertising`
(boolean), `generation` (unsigned decimal). Query: `device` (empty for known-peer
page), `purpose`, `cursor` (0..256 bytes), `limit` (1..200). Query is passive.
Observation: `device, pin, purpose, route, code, observed_at, generation`; route
is direct/relay/finding/unavailable/not_tested; freshness is separate from file
capture/stored/applied facts. No receipt inference from service connectivity.

Preview: `policy, review, effects`; apply: `operation, policy, review`, using the
existing expiring, generation-bound review and operation-id semantics. Require
a running daemon; no stopped-controller DB fallback for networking operations.

PrivateNetworkSetup: `version, operation, attempt, root, phase, policy, invitation,
transcript, request`. Persist only in existing exclusive private state ownership;
receiving invitation contains a secret and must never appear in normal status or
exports. Keep phases/receiving root from existing reviewed setup; retries preserve
identity/attempt/root and authorization expiry. W05 owns its additive durable
implementation. Invitations are cleared/redacted at the same existing boundaries.

W02/W14 import peers.json v1 as manual intent and preserve all existing keys,
counters/history. No upgrade opts an existing install into public announcements.
W06 fresh setup reviews Automatic; W12 changes mode/profile only after preview.
Candidate leases, challenges, replay caches, relay tokens, ICE credentials,
network generation and observations are ephemeral. No schema migration occurs in
W01; v1 membership/version and v2 persisted namespaces remain unchanged.

## W03 implemented service selection and WSS control behavior

Private profile selection is the closed object `profile, authority, highest_epoch,
environment`. `profile` is the unchanged signed profile above; `authority` is the
independently reviewed key; `highest_epoch` is a nonzero decimal string equal to
the selected profile's epoch; environment is `release`, `self_hosted` or
`development`. A saved selection refuses rollback/same-epoch fork and unreviewed
authority/environment replacement. Production default authority/origins remain W13.

The WSS control transport must be the approved same-host `wss://` counterpart of
the service's HTTPS origin. Its first text frame is the existing closed `proof`
envelope with kind `authenticate`, binding the challenge-issuing **HTTPS service
origin** exactly (no rewritten challenge/transcript). Within ten seconds authenticate
with a fresh challenge; no credentials in URLs. The server replies with NetworkResult
`accepted` for that operation. Subsequent text frames are the unchanged addressed
NetworkOfferRequest (kind `offer` or `accept`); mutation requests continue over signed
HTTPS. A 25-second server ping plus the same authenticated operation result provides
heartbeat progress; clients consume that result internally, never as a peer offer.
Client unsolicited frames close the channel. All control JSON frames are at most
16 KiB, compression disabled, no browser Origin. The frozen canonical profile/proof/
offer/attachment bytes and W01 golden fixtures remain unchanged.

The public directory rejects LAN candidates even in authenticated coordination;
LAN candidate coordination belongs to scoped later transport integration. This
stricter W03 service path does not enable public arbitrary-address forwarding.
Enrollment offers without a sender lease may be admitted within the separate
finite budget; both peers must have current purpose-scoped leases and outward
controls for two-way acceptance/reservation. Data offer receivers verify local
pins, and directory authentication never grants folder access.

## W04 implemented WSS attachment behavior

`GET /network/v1/relay` on the approved WSS origin accepts one text
`NetworkRelayAttachRequest` of at most 16 KiB within ten seconds, then an accepted
`NetworkResult` or bounded `NetworkFailure` and closure. Attachment proofs use a
fresh HTTPS-issued challenge and bind their canonical origin/intent to WSS;
control proofs retain W03's HTTPS binding. Canonical fixtures are unchanged.
Admission additionally checks the exact live issued token, partner acceptance,
current service restart epoch and one-use role before accepting binary messages.
Only binary ciphertext messages of at most 32 KiB follow authentication.

Unknown enrollment initiators may authenticate a control channel without a public
announcement; their acceptance is bound to the stored exact offered generation.
Data still requires both current public registrations. Purpose and partner pins
cannot be substituted. See [the protocol](../docs/orbit-wan-protocol.md#w04-live-relay-attachment)
and [runtime bounds](../docs/orbit-wan-architecture.md#w04-encrypted-relay-integration).

## W09 native HTTP3 capability

`quic_http3_v1` advertises ordinary direct UDP HTTP3, independently of W10 ICE.
LAN `orbit-lan-v1` records may append this capability after `direct_https_v1`;
UDP candidates require it. TCP-only canonical bytes are unchanged. Duplicate
candidates are checked as transport/address tuples; bounds remain four LAN /
sixteen total candidates and 1,200-byte discovery datagrams.

The same signed records may also travel inside an approved peer session as
`LANExchange` (`version`, `records`, at most eight records); see
[peer LAN exchange](../docs/orbit-wan-protocol.md#peer-lan-exchange-post-w17-2026-10-07).

## Additive ICE extension (W10)

An offer optionally includes a non-null `ice` object, whose exact required keys
are `mode`, `ufrag`, `password`, `candidates`. The historical three offer keys are
still mandatory and legacy canonical bytes are unchanged. `request` has empty
credentials/candidates; `offer` and `accept` carry bounded ICE credentials and
1–8 canonical numeric UDP host/srflx SDP candidates, each at most 512 bytes.
The enclosing ordinary candidate array is empty. Scope/role/generation/session
checks and canonical domains are specified in the
[owning protocol](../docs/orbit-wan-protocol.md#w10-ice-offer-extension).

The independent [golden](fixtures/ice-v1/offer.json), [proof bytes](fixtures/ice-v1/offer.hex),
[payload bytes](fixtures/ice-v1/payload.hex) and [generator](fixtures/ice-v1/generate.py)
use synthetic credentials and a fixed synthetic signing key. They are test data.
