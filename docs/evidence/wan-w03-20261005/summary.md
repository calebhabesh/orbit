# W03 — 2026-10-05

State: **complete** for W03 local production-service acceptance. All 24 discovered
focused tests passed under race on the final production source. `make check` passed;
uncached manual CLI/forwarding/background journeys passed. [Manifest](manifest.json),
[commands](commands.md) and [results](results.json) record source provenance,
executions and limitations. Existing P/O/T and W00–W02 changes/evidence are preserved.

Delivered the separately runnable `cmd/orbit-net`/`internal/rendezvous` service,
`network.ServiceClient`, signed independently reviewed profile selection and
private atomic profile persistence. Exact device ID/pin/purpose records, one-use
challenges, semantic operation replay, lease/generation expiry, addressed WSS
control offers/acceptance and service-signed reservation intent use frozen W01
canonical bytes. A real process validates private operator files, normal service
TLS, authenticated announce/lookup, SIGTERM shutdown and restart/reannouncement.
The 20-repetition race run passed after correcting the binary fixture's clock.

Every service reconnection resolves/checks all DNS answers before numeric dialing
with the original TLS hostname; private origins require explicit self-host or
development trust, and loopback is always refused. Proxies, redirects, browser
Origins, forwarded-header exemptions and arbitrary supplied dialing URLs are
absent. Signed profiles reject expiry, unrelated authority, rollback and changed
same-epoch digests. A final focused header regression refines the test fixture after
the full final race run began; production sources stayed frozen. Saving a profile alone leaves manual policy unchanged.

| Acceptance | Actual evidence |
| --- | --- |
| ID/key impersonation and authentication | Same-ID competing key gets a separate record, cannot replace the known pin; invalid key/signature/profile/origin/purpose, missing proof, changed payload/path, nonce reuse and expired exchanges fail before route mutation |
| Replay and leases | Same operation/payload with fresh proof returns the previous result; changed intent conflicts; stale generation, service restart, purpose substitution and exclusive lease expiry fail; historical proof expiry does not extend a lease |
| Routing trust | Client lookup verifies exact expected ID/pin/profile/purpose/signature/payload/current lease/minimum generation; local pins filter data offers; private/loopback/control-target/mapped/link-local/CGNAT candidates and mixed unsafe DNS answers fail; source IP is never used as a candidate |
| Profiles and outer TLS | Normal TLS with explicit development CA works, missing CA fails; private 0600 selection round-trips; signed rollback/same-epoch fork/authority/environment replacement fail; a valid higher epoch succeeds; expired profiles fail |
| Control and intent | Production WSS offer/accept binds both peers, roles, purpose/session/generations; reservation requires partner acceptance and returns exact role/pin/purpose/epoch credential; wrong-purpose release and released reservations fail; unknown enrollment has two-session admission separate from data |
| Wire/cache bounds | Exact 64-KiB HTTP body and 16-candidate message pass, one-over messages/candidates fail; exact 16-KiB WSS authentication passes, one-over closes; later nonempty duplicate Origin is rejected on an otherwise valid challenge; actual oversized TLS HTTP header rejected; 128 live route/challenge caches and seeded 1024-entry replay saturation reject overflow and expire safely |
| Shared NAT and floods | Twenty independent certified keys behind one synthetic source IP register; a key cannot consume more than two live challenges; 200 concurrent lookups obey client/table bounds; 1,000 canceled callers measure resolver peak four and zero remaining workers/sockets; 272 raw sockets measure a 256-goroutine pre-TLS ceiling and bounded join |
| Lifetimes and retention | Real 25-second WSS ping/application heartbeat preserves idle authenticated control; jitter lies in 100–140 seconds, synchronous renewal advances generations without changing pins; one-second sweeper removes idle expired metadata; cancellation/shutdown join sockets/readers/expiry work |
| Directory independence/privacy | After a real local directory shutdown, production manual capture and pinned sync transfer exact verified bytes with the original causal author; manual/local-only reannouncement performs zero DNS/service/candidate work; development profile cannot become Automatic release default |
| Compatibility | Uncached CLI/control/config/network/replication/scheduler/model suites and strict protocol/model fixtures pass; actual CLI interrupted join/restart, three-peer forwarding/address refresh and background 16-MiB transfer while eight edits are produced pass |
| Integration | Final `make check` passes terminal/package checks, formatting, vet, CLI/internal/model, integration, fault/design-gate suites, 12 Python safety checks, amd64/arm64 daemon binaries/packages; standalone service builds for both architectures; docs/discovery/preservation checks and `git diff --check` pass |

The [architecture](../../orbit-wan-architecture.md#w03-directory-and-profile-integration)
owns concrete quotas, lifetime/retention and TLS/DNS/socket policy. The
[schema clarification](../../../schemas/network-v1.md#w03-implemented-service-selection-and-wss-control-behavior)
records authenticated HTTPS-origin challenge binding on the approved same-host
WSS control transport and heartbeat/event envelopes. W01 golden bytes remain
unchanged. The independent admission model remains separate from wire/service code.
Maximum replay-cache saturation uses explicitly seeded bounded ephemeral records;
route and challenge limits use verified service calls. This is not a 1024-device
capacity measurement.

All fixtures run on one Linux development host using private nonloopback service
sockets, synthetic public candidate addresses, explicit test roots/CA/key material
and disposable marked process binaries. Directory loss and binary restart affect
only these fixtures. No personal folder, existing VPS workload, privileged network
namespace or real WAN/NAT was a fault target. Metadata is kept in bounded memory
only: routes 600 seconds, challenges 60, replay 300, sessions 30, rate buckets 60
seconds of inactivity, connected controls until closure; idle expiry runs every
second. Application/HTTP/TLS error logs retain no addresses or identity timing.
W13 must document external infrastructure retention and actual operator values.

Initial failures retained: cache-saturation fixture accidentally included earlier
records (fixture repaired); client socket ownership exposed a real close-under-lock
deadlock (production close order repaired, failed process stopped by exact test PID);
the directory-loss fixture initially used a nonexistent Envelope.Author field
(corrected to ID.Author); the first integrated check exposed frozen test-client versus
real service-clock skew in the binary fixture (both now use the system clock,
20 subsequent restart repetitions pass). No failed run counts as acceptance.

Unexecuted/later packets: relay attachment/forwarding and broker quotas (W04),
v3 durable enrollment/routed sync and reviewed ordinary CLI/TUI activation
(W05–W07), LAN/public candidate dialing/QUIC/ICE/roaming, physical WAN/NAT cases,
Pi peak RSS/FD/egress campaign, production hosted profile/authority custody,
operator monitoring/infrastructure logs and packaged service distribution. No
bundled release endpoint/operator is invented. Existing installs remain manual
and no new runtime capability is advertised. Strict signed Unix expiry windows
assume compatible clocks; skew fails safely rather than extending authorization.
T13 native login/logout/unattended boot remains outstanding; P17 owner personal
use/explanation remains deferred. No native-WAN or combined-release completion claim.

Next eligible: **W04 — Encrypted WSS relay**. Use the existing BinaryStream/Forward
inner-TLS seam and W03 admitted partner/session/purpose/epoch intent. Before forwarding,
verify live reservation plus exact requesting leg and fresh attachment proof, bound
unknown enrollment separately, enforce deadlines/byte/egress quotas, and close/join
both legs without producing file receipts. W03 token issuance alone is not attachment
admission; the local service currently exposes no relay-forwarding handler.
