# W01 contracts and local gate outcomes

W01 defines additive contracts and executes local design experiments. It does not
activate discovery/relay services or wire automatic networking into the daemon.
[Evidence](../evidence/wan-w01-20261005/summary.md) owns actual commands and results;
[strict schema](../../schemas/network-v1.md) owns field order/limits and fixtures.

## WG1 — Selected composition

Use `internal/network.NewTransport(Target, replicationTLS, StreamDialer)` for
pinned HTTPS on direct TCP or outbound WSS binary streams. Clone the supplied
TLS configuration and root pool, retain standard certificate verification and
replication's VerifyConnection, additionally enforce Target.Pin, select HTTP/1.1
ALPN, disable proxy/compression/redirects and retain finite handshake/header/body
request deadlines. Target ID/pin/purpose/profile is immutable per pool. A request
URL never chooses the StreamDialer's destination. Replication retains request
DeviceID, membership, chunk and receipt verification; routing cannot approve keys.
`Manager.Transport(ctx, Target, *tls.Config)`, `Observe(Target)` and `Close()` are
the frozen W02 seam. Replication already supplies the reviewed bootstrap cert/pin
in its TLS policy, so a second Bootstrap field is unnecessary at this seam.

`BinaryStream` uses coder/websocket **v1.8.15**, ISC license, compression off,
32,768-byte binary frames and bounded splitting. The first executable spike
failed: the library NetConn irreversibly cancels an active read on a deadline;
net/http deliberately expires then clears background-read deadlines between
requests. Direct NetConn use closed a reusable relay HTTP session after Hello.
The repaired adapter uses an independently owned read pump and one 32,769-byte
frame buffer (one byte detects overflow). It waits for the consumer before refill;
caller read deadlines are reversible without cancelling the underlying WebSocket
Reader. NetConn supplies serialization/write deadlines; invalid text/oversize
frames fail. Close/cancellation joins the read pump. Never restore an unlimited
read limit as a workaround. EOF remains terminal across subsequent reads.

`Forward` owns two 32 KiB copy buffers; socket and reader-pump backpressure
bounds pending bytes without file-size allocations. First error, idle expiry or
cancellation closes both legs and joins both pumps. W04 adds service reservations,
quotas/lifetime/accounting. Slow WSS receiver blocks the synthetic writer and
cancellation joins it; this is local adapter evidence, not peak-RSS/Pi measurement.

Separate `StreamListener`s feed **only** the production peer or enrollment HTTP
server through tls.NewListener. Relay RemoteAddr is logical `orbit-relay:0`, never
a forwarded/client IP; existing enrollment admission conservatively groups all
relayed unknown requesters into one bucket. W05 adds authenticated per-key limits
while preserving global/unknown limits. No owner-control handler is attached.
Fixture broker has two outward WSS legs, normal outer TLS with an explicit fixture
CA, rejects every browser Origin, and captures synthetic opaque inner TLS bytes.
It has no supplied-URL dialing facility.

Executed transport tests verify direct/relay production chunk transfer and HTTP
ALPN/TLS, wrong inviter/target pin, missing data mTLS, unknown signed requester
folder denial, owner/enrollment/data handler separation, redirect refusal,
request cancellation, frame maximum/one-over, reset deadlines, slow reader and
leg/pump cleanup. No production service/profile endpoints exist yet. HTTP2 WSS,
QUIC/HTTP3, ICE, real NAT/WAN/roaming and resource campaign remain untested.

## WG2 — Selected admission/trust contracts

Ed25519, SHA-256, Go crypto/x509 and TLS reuse current primitives. Independent
Python-generated canonical/signature fixtures exercise Go's closed codecs.
All signed bindings/paths, online service signing key, offline profile authority,
certificate DER/raw public key/signature representations and numeric bounds are
frozen in the schema. Highest reviewed profile epoch prevents rollback;
untrusted/self-supplied, expired and altered profiles fail. A service restart
invalidates transient routes/challenges/reservations with a fresh epoch.

`model.WANAdmission` independently exercises actor-bound one-use challenges,
exact `(DeviceID,pin)` records, changed-input idempotency conflicts, fresh-challenge
lost-response replay, stale generation/lease, expiry/restart and finite capacity.
Competing keys cannot overwrite a pinned record. Unknown/unauthenticated lookup,
wrong epoch/purpose and duplicate relay legs fail. Strict unknown-key fixtures
reject forwarded headers; no forwarded address is an identity/rate exemption.
Public origin/candidate fixtures reject loopback/control URLs, private/CGNAT,
mapped/unspecified/multicast/link-local/documentation addresses and malformed URL
classes. Private origins require self-host review; loopback remains prohibited.
DNS resolution/rebinding implementation and service quotas are W03/W04 acceptance,
not satisfied by syntax-only validation. No directory result supplies initial trust.

## WG3 — Selected routed enrollment contract

Add v3 logical references binding exact identities/pins/profile/purpose; mutable
addresses are outside folder authority. New signed request/status/approval and
invitation fixtures preserve v2/manual coexistence and v1 DeviceIDs/membership.
Independent `model.WANEnrollment` separates admission from folder access and
exercises exact approval, wrong folder/key/review, second same-key folder,
capability revocation/substitution, retirement, expiry, lost response/restart.

A v3 fixture handler composes frozen request verification with the oracle over
both direct and relayed pinned TLS for a fresh requester. Explicit approval then
uses existing repository membership validation; artifact SHA-256 matches the
unchanged v1 bytes and retired-identity revival fails. A separate relay spike runs
actual production v2 challenge/request handlers for an unknown key: it reaches
pending approval, its secret is opaque at the broker and data inventory is denied.
V3 production challenge/status/approval, durable daemon resume, bootstrap artifacts
and real WAN first-time journeys remain W05–W07. The local design gate neither
advertises enrollment_v3 nor claims a complete production enrollment flow.

## WG5 — Initial model and bounds

`model.WANConnection` independently models two direct attempts plus one relay,
verified direct preference, route failure, old attempt completion, generation
changes, cancellation and 10,000 seeded flapping events. It never outputs a file
receipt or changes mutation/device identity. A generation change discards new-route
authority; finite current requests can drain under existing replication deadlines.
W02 implements the manager and global fairness; WG5 stays open until its W02 model
integration and W11 production timing/roaming evidence.

`internal/network/limits.go` freezes initial finite defaults: 750 ms direct head
start, 10 s cycle, 60 s direct reprobe, 32 active slots, two pools per target,
8 global data tunnels, two per peer, two unknown enrollment tunnels; service 64
data tunnels/128 controls/128 reservations, 1024 replay entries, 60-second challenge,
300-second replay, 600-second lease, 120-second reannounce, 30-second attach,
10-second inner handshake, 3600-second active tunnel, 60-second idle, 25-second
heartbeat/75-second stale threshold, 60 metadata operations per minute/burst 10.
These are admission defaults, not measured capacity. WG4 stays with W09/W10;
WG6 and actual hosted operator/signing/rotation/bandwidth budgets stay with W13.
