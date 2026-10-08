# Orbit WAN network protocol contract

Status: 2026-10-05: W01 freezes [strict network/v3 contracts](../schemas/network-v1.md)
and [local WG1–WG3 outcomes](implementation/wan-contracts.md). W02 manual transport and W03 directory/profile integration are complete; production
W04 live relay integration is complete; W05 routed-enrollment integration is recorded in the tracker; W06 adds reviewed
CLI activation without changing signed bytes; WG4 closed in W09/W10, and W13/W16 record the operated service and its client-side signed-operation pacing. W17 (2026-10-07) found no protocol contradiction with the implementation. Existing [causal protocol](protocol.md),
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

W01 delivers [network-v1](../schemas/network-v1.md), additive control contracts and
[independently generated golden fixtures](../schemas/fixtures/network-v1/README.md). Use bounded strict JSON with duplicate/unknown-field rejection;
canonical unsigned decimal strings for counters, sizes and timestamps; lowercase
hex fixed-size IDs/pins/nonces/digests. Endpoint types explicitly distinguish
service origin, TCP candidate, UDP candidate and relay route. Canonical signature
input is length-prefixed binary fields, not arbitrary JSON serialization.

Every authenticated control exchange binds the domain `orbit-network-v1`, message
kind, profile/service origin, sender ID/pin, target ID/pin when applicable, purpose,
fresh server challenge, operation/attempt ID, expiration and canonical payload
digest. Initial authentication includes certificate DER and Ed25519 key-possession
proof. Subsequent signed peer-coordination messages bind both peers, a unique
session ID, candidate generation, role and expiration. [W01 contracts](implementation/wan-contracts.md) freeze exact field
order/limits, profile authority and online service key, and signature vectors for W03.

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
The relay endpoint publishes an offer generation only after the matching
announcement is accepted. A quota-refused renewal does not advance its offer
generation. Until its purpose announcement is ready, the runtime performs no
new lookup, relay offer or ICE dial; established authenticated transports may
continue. This prevents cold-start retries from spending quota on generations
the directory has never accepted. Ready means the directory holds an accepted,
unexpired record: a transiently refused renewal (quota, service unavailable,
local overload) keeps the accepted generation and readiness until one minute
before that record expires, while a semantic refusal withdraws readiness at
once (W16 native evidence: withdrawing on every quota refusal turned overload
into relay outages).

Each daemon keeps its signed operations inside the service's per-device bounds:
at most two outstanding challenges, paced to the metadata rate with a smaller
client burst and one token reserved for announcement renewal. Ordinary callers
get typed local backpressure or `QUOTA_EXCEEDED` instead of queueing or spending
service quota. A caller may cancel until the service connection is ready; once
the challenge request is written, the operation completes within its own bound
so that no issued challenge is abandoned.
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
workflow; ordinary status omits capability bytes. [W01 contracts](implementation/wan-contracts.md) freeze proof/approval
canonical bytes and execute the independent admission model plus local transport
fixtures; W05 still owns production v3 handlers and durable resumption.

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

## W03 production coordination

W03 implements the frozen HTTPS exchanges, same-host authenticated WSS control
and independently reviewed profile selection. The
[control clarification](../schemas/network-v1.md#w03-implemented-service-selection-and-wss-control-behavior)
records HTTPS-origin challenge binding on WSS, addressed offer events and heartbeat
without changing canonical bytes. The
[architecture](orbit-wan-architecture.md#w03-directory-and-profile-integration)
owns concrete bounds, DNS/socket ownership and ephemeral metadata retention.
Purpose-scoped lookup verifies the historical signed announcement proof separately
from its current lease; source address is never a reachability candidate. No relay
forwarding, directory enumeration, folder admission or automatic daemon migration
is supplied by this packet.

## W04 live relay attachment

The W01 canonical bytes and strict fixtures are unchanged. W04 implements
`GET /network/v1/relay` with normal service TLS, no browser Origin/forwarded
headers/query credentials, disabled compression and the same approved HTTPS/WSS
host. The first text message is strict `NetworkRelayAttachRequest`, at most
16 KiB, received within ten seconds. Its fresh proof challenge is issued over
HTTPS, but its signed origin/intent binds the WSS origin. Live admission checks
partner acceptance, exact issued credential, service restart epoch, unexpired
attachment and unused role in addition to the frozen signature/binding checks.
A bounded `NetworkResult` acknowledges accepted routing; refusals return a bounded
`NetworkFailure` then close. Following messages are binary ciphertext only,
at most 32 KiB each. No application invitation is an attachment credential.

A new enrollment requester needs an authenticated addressed control channel but
need not publish a directory record. Acceptance binds its exact live signed offer;
this grants routing only. Data offers/acceptances still require current registered
generations and local known pins. Enrollment and data terminate at separate
existing TLS HTTP listeners; neither destination exposes owner control.

The [implemented architecture](orbit-wan-architecture.md#w04-encrypted-relay-integration)
owns finite sessions, bandwidth, byte budgets and shutdown. At the 60-minute
active lifetime, clients refuse new pooled requests and drain an admitted request
for its existing finite deadline (at most 30 seconds). The opaque broker closes
at the hard lifetime-plus-grace ceiling; it does not inspect HTTP boundaries.
Fault/profile/authorization expiry closes immediately. No routing acknowledgement
is converted into a file receipt, and a new session cannot extend enrollment expiry.

## W08 local announcement encoding

`LANAnnouncement` is additive and never sent to public services. Strict JSON
fields, in canonical struct order: `version`, `device`, `pin`, `key`, `generation`,
`expires`, `capabilities`, `candidates`, `signature`. Version is `"1"`; IDs/pin/raw
Ed25519 key are lowercase nonzero 32-byte hex; generation and Unix-second expiry
use the existing decimal-string `NetworkUint`. Capability is exactly
`["direct_https_v1"]`. There are one through four distinct `tcp`/`lan` candidates
in existing candidate field order, with signed interface labels and canonical
private numeric AddrPorts. Loopback, unspecified, multicast, mapped and link-local
addresses are rejected. Packet size is at most 1,200 bytes, with no fragmentation,
pagination, response amplification, folder names or capabilities for enrollment.

Signature bytes are Ed25519 over `orbit-lan-v1\n` followed by the compact UTF-8
JSON encoding in the stated field order with `signature` set to `""` (Go
`encoding/json` string escaping, including HTML and U+2028/U+2029 escaping).
The received hex signature has exactly 64 bytes. SHA-256 of the raw key's DER
SubjectPublicKeyInfo must equal `pin`; the `(device,pin)` must already be a reviewed
routing identity. This key proves possession rather than granting initial trust.
Duplicate/unknown JSON keys and trailing documents are rejected. Independent
Python-generated [canonical bytes](../schemas/fixtures/lan-v1/canonical.hex) and
[announcement](../schemas/fixtures/lan-v1/announcement.json) are tested in Go.

Expiry is strictly future and at most ten minutes away. Each receiving interface
has its own bounded generation/expiry lease. A lower generation or altered record
at the same generation is refused. Source and candidate private prefixes must
match the actual permitted receiving interface, obtained from socket metadata;
remote interface names are not compared to local OS names. Reordered/address-change
records cannot renew another interface's lease. Unknown identities are not cached
or dialed; owner invitation/approval and per-request folder authority are unchanged.

W08 public TCP uses unchanged signed `NetworkAnnouncement` lookup bindings. Only
actual eligible global interface addresses and bound listener ports are published
for peer-data purpose, with `direct_https_v1` when available; enrollment remains
HTTPS/WSS on its separate handlers. A lookup lease, including an empty candidate
list, remains routing metadata only. Reusing it for relay allocation does not
replace fresh live offer/accept/reservation proofs or extend enrollment expiry.

## W09 HTTP3 transport contract

`quic_http3_v1` identifies implemented native direct HTTP3 over ordinary UDP;
`quic_ice_v1` remains reserved for W10's composed traversal. Public announcements
accept up to five distinct defined capabilities; UDP direct selection requires
`quic_http3_v1`. LAN records retain `direct_https_v1` first and may append
`quic_http3_v1`; UDP LAN candidates require that second capability. Candidate
identity is the transport/address tuple, so TCP and UDP may use the same numeric
port. Existing TCP-only canonical/signature fixtures remain byte-identical;
[UDP fixture](../schemas/fixtures/lan-quic-v1/announcement.json) is independently
signed. The existing 1,200-byte announcement and four-candidate LAN bounds remain.

HTTP3 serves unchanged `/peer/v1/*` POST encodings with TLS 1.3, h3, mandatory
client certificates and per-request folder/membership checks. Unknown certificates
are refused by existing authorization even when the TLS handshake succeeds.
Initial enrollment stays HTTPS/WSS. Early data is disabled on both roles.
The [owning limits](orbit-wan-architecture.md#w09-native-quic-http3-integration)
map header/body/request/idle deadlines and resource caps explicitly. A routing
observation or successful HTTP3 connection never establishes content readiness or
creates a stored receipt. Partial streams retain only independently verified chunks.

## W10 ICE offer extension

`NetworkOffer` accepts exactly one optional non-null `ice` object. Every historical
required key remains required; duplicate, unknown, case-alias and null keys remain
invalid. Offers without the extension retain their exact canonical/signature bytes.
ICE-capable peer-data announcements use the previously reserved `quic_ice_v1`.
Older peers without that capability retain TCP/native QUIC/relay routing.

The object has required `mode`, `ufrag`, `password`, `candidates` keys. Modes are
`request`, `offer`, `accept`. A request has empty credentials and an empty array;
offer/accept have 4–256-character username fragments, 22–256-character passwords
from the ICE alphabet, and 1–8 unique canonical SDP candidates of at most 512 bytes.
Pion's maintained parser validates numeric UDP component-one host/srflx candidates;
Orbit excludes prohibited/private/DNS/mDNS/TURN/TCP targets. Related private
addresses/ports are omitted. Request/offer use the existing initiator proof role;
accept uses responder. Only the smaller identity/pin pair may send an ICE offer;
only the larger may send request/accept. Enrollment offers cannot carry ICE.

The inner canonical domain is `ice-v1`, with mode, ufrag, password, candidate count
and each exact candidate. The outer `ice-offer-v1` contains the historical sender
and target generations, zero ordinary candidates, and the length-prefixed inner
canonical bytes as its final field. The existing proof signs its payload digest,
both identities/pins, purpose, session, role, generation, operation, challenge and
expiry. The service binds acceptance to the live opposite-leg session and exact
generation tuple and refuses relay reservations on ICE coordination sessions.
[Independent synthetic fixture](../schemas/fixtures/ice-v1/offer.json) and its
Python generator preserve a cross-implementation canonical/signature oracle.

ICE credentials and STUN observations authorize connectivity checks only. Both
QUIC TLS identities and request-level folder/membership authorization remain
mandatory. Credentials, offers, pairs, sockets and failure observations remain
transient. Closing retires the agent and clears Orbit's retained local credential
record; this is not a guarantee of cryptographic erasure of every Go/library heap
copy. Existing service records expire under their bounded session/replay policy.

## W12 diagnostic observations and probes

Terminal network status is a read-only projection. `NetworkStatus` carries
generated time, desired and active `NetworkPolicy`, profile state/expiry,
service readiness, a bounded action string and dated `NetworkObservation` values.
Each observation identifies a reviewed device/purpose and records route, typed
code, freshness, LAN/public/expired candidate counts and the last UDP result.
These observations never acknowledge file content, stored/applied state or
membership; a connected route cannot produce an `Everything synced` claim.

`network_doctor` is the only operation that performs probes. Its result contains
typed `ProbeResult` values with one of the stable kinds `service_dns_tcp`,
`service_tls`, `directory`, `direct_tls`, `relay_inner_tls` and `udp_stun`.
Codes include `VERIFIED`, `UNAVAILABLE`, `NOT_TESTED`, `DISABLED_BY_POLICY`,
`TIMEOUT`, `CANCELLED`, `QUOTA_EXCEEDED`, `IDENTITY_MISMATCH`,
`TLS_IDENTITY_FAILED` and `PEER_OFFLINE`; unknown transport errors are reduced
to `UNAVAILABLE`. Probe calls use bounded contexts and are not cached as route
observations. Direct and relay checks validate the existing peer/service pin and
stop after TLS; they do not send a peer HTTP request. UDP checks send a bounded
STUN binding request to a reviewed server and report `VERIFIED` only for a valid
transaction-matched response. No response is turned into a NAT/firewall label.

The doctor does not fan out: a peer route is tested only for an explicitly
selected device ID. Passive status, support export and TUI refresh use no
external network call. Disabling internet policy invalidates cached public
candidates and stops discovery/STUN/relay before WAN pools drain, while identity,
keys, history, membership and files remain. Profile/mode changes use the normal
reviewed mutation ledger and retain the existing generation/restart semantics.

## W13 profile rotation

A service may serve two adjacent epochs from one authority, environment and
origin. Challenge results echo the requested profile digest; reservations sign
relay attachments with the requesting epoch's service key and bind its digest;
attachments verify against that epoch. An unknown digest returns
`PROFILE_UNTRUSTED`; a known but expired epoch returns `PROFILE_EXPIRED`. Peer
announcement proofs and offer events from the same origin are accepted under
either digest, so the proof's `profile` field identifies the sender's reviewed
epoch rather than requiring equality with the receiver's. Enrollment routes still
require equal profiles. Devices never accept an older epoch; operators must keep
serving every epoch devices hold until it expires.

## W14 invitation codes and profile compatibility

Code prefixes are unchanged (`orbit-invitation:v2:` and `orbit-invitation:v3:`,
unpadded base64url JSON). A v3 code may omit `profile` when its
`route.profile` equals the inviter's packaged release digest; the receiver fills
it from its own packaged selection only when the digests match and then runs the
unchanged v3 validation, so the signature, digest binding and pin checks are the
same as for a full invitation. Invitation files and stored join mutations always
hold the full form. Prefixes or `version` values above 3 are rejected as
`UNSUPPORTED_INVITATION_VERSION` before any network use. Peer, relay, rendezvous
and enrollment wire messages are unchanged in W14.

## Peer LAN exchange (post-W17, 2026-10-07)

Multicast discovery fails when a host firewall drops inbound UDP 22027 and the
random direct ports, which sent the W14 same-LAN laptop/Pi pair through the
relay. Approved peers that already share a pinned session (direct or relayed)
now also exchange LAN records over it with `POST /peer/v1/lan`. Request and
reply are both `{"version":"1","records":[...]}`: at most eight `orbit-lan-v1`
records (one per discovery interface), each signed and verified exactly like a
multicast announcement, with a 16 KiB body limit. A relay forwards only the
peers' TLS ciphertext, so the service still never receives private addresses.

The responder requires the mutual-TLS client pin to belong to a known
peer-data target (`403 UNAUTHORIZED` otherwise). Records must carry that
session's pin. A candidate is installed only when it falls inside a selected
local interface prefix and is not one of the receiver's own addresses, keyed
to that local interface, with at most four peer-sent candidates per target.
Because a matching private prefix does not prove the peers share a network,
peer-sent leases never suppress the public directory lookup and never replace a
live lease heard on the link; a wrong-network candidate fails the pinned TLS
handshake like any other. Record generations follow the sender's clock, so
multicast and exchanged records stay ordered.

The exchange runs only while LAN advertising is enabled. A daemon exchanges
with each peer that its last ordinary request reached, at most every four
minutes (one minute after a failure). A `404` means an older peer or one
without LAN advertising, and is retried after 30 minutes. One request informs
both sides, so a direct path appears when either device accepts inbound
connections.


## E06 short-code invitation mailbox (2026-10-08)

The owner selected CPace. Code format is eight random Crockford base32 digits,
shown `XXXX-XXXX`; case, ASCII whitespace and dashes are ignored and O→0,
I/L→1. The public mailbox name is 20 bits and the password is 20 bits; the code
has 40 random bits, but its online password-guess bound is **one in 2^20 per
claimed code**, not 2^40. A guessed mailbox can be claimed to deny service;
this is diagnosed by expiry/failure and requesting a new random code.

CPaceRistretto255/SHA-512 uses the initiator/responder variant of
[draft21](https://www.ietf.org/archive/id/draft-irtf-cfrg-cpace-21.txt).
The inviter creates a random 32-byte session ID and first CPace element.
Context binds the suite, canonical service origin and mailbox name; ordered
associated data binds inviter and joiner device IDs/SPKI pins. Invalid points
and identity elements abort; the private scalar is consumed once. The joiner
sends its element plus HMAC-SHA256 key confirmation. Only after verification
does the inviter encrypt the ordinary v3 invitation with AES-256-GCM (fresh
96-bit nonce, suite-associated data). Separate HMAC labels derive confirmation
and encryption keys from CPace's ISK. AEAD authenticates the inviter's response.
The decrypted invitation must match the CPace inviter identity/pin and the
selected profile. Ordinary pinned enrollment, root review and approval follow.

`POST /network/v1/pairing` uses existing strict JSON, TLS and challenge-bound
Ed25519 device proofs (`kind=pairing`, `purpose=enrollment`). Its payload
canonicalization binds action, mailbox, session, expiry and hex data. Actions:
create → claim → respond → deliver → consuming poll. Owner poll reads the
joiner's proof; owner burn removes failed attempts. Poll never changes a claim
back to open, and respond/deliver are single-use. Service bounds: 128 mailboxes,
4 per owner key, 5 create/claim requests per source burst with 1/10 s refill,
128 source buckets, plus existing authenticated/global limits. Nonexistent
claims also spend source quota. All state is in memory and expires after at
most ten minutes. Successful consumption, burn and shutdown remove it; a
restart reports unavailable and requires a fresh code. Clients poll every
three seconds through the existing signed-operation pacing.

The service sees mailbox name, device IDs/pins, timing, public PAKE elements,
confirmation tag and invitation ciphertext. It receives no password, readable
invitation, folder name or contents. The existing profile privacy statement
remains accurate; no epoch bump is required. A malicious service can deny
service or make an online guess; CPace prevents passive offline enumeration,
and the inviter's single-use exchange enforces one attempt even if the service
replays messages. This adds no availability guarantee.

E10 diagnostic amendment: relay TLS probes allow ten seconds for paced
coordination plus the handshake, within the existing twenty-second total
doctor deadline. Direct TLS probes retain three seconds. Probes still send no
peer HTTP request and do not alter observed routes.
