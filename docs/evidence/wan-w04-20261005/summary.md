# W04 — 2026-10-05

State: **complete** for local production-service/engine acceptance. **23 discovered focused tests pass under race**, 20 relay restart/resume repetitions pass, uncached compatibility and manual production journeys pass, and `make check` passes. The production encrypted WSS broker, client,
coordinator, manager logical routes and isolated daemon listener integration are
implemented. Final results are recorded in [commands](commands.md),
[results](results.json) and [manifest](manifest.json).

Both devices connect outward through the approved TLS/WSS service. Attachment
requires fresh proof, exact live issued role/peer/purpose credential, current
restart epoch and partner acceptance. The broker forwards bounded ciphertext;
replication keeps its pinned inner TLS, per-request folder/membership checks,
verified chunks, causal IDs and end-device receipts. Owner control has no virtual
listener or tunneled handler. Manual behavior and existing dirty P/O/T/W00–W03
source/evidence are preserved.

| Acceptance | Executed local evidence |
| --- | --- |
| Forced relay and two-way verified files | Production rendezvous offer/accept/reserve/attach, two engine identities/repositories, isolated peer TLS servers and manager routes with direct listener unavailable; exact file bytes, heads, authors and manifest verification in both directions |
| Confidentiality and authentication | 2,144 synthetic inner-TLS bytes captured at the decrypted outer-WSS stream boundary contain no synthetic private filename, content or invitation token; wrong role/pin/purpose/epoch/signature/expired/released/duplicate credentials and reused proof challenges fail; pooled retired sockets cannot write new HTTP bytes |
| Purpose/folder isolation | Actual unknown device, with authenticated control but no directory announcement, reaches v2 enrollment pending explicit approval; data/control paths return 404, and no membership is granted. Unshared data folder/pin substitution fails. V3 durable setup remains W05 |
| Failure and receipt boundaries | Idle/lifetime/byte limits, slow reader, abrupt loss, unpaired expiry, client/HTTP request cancellation and joined shutdown; service restart after the first verified chunk reuses one chunk/fetches one, preserves exact heads/hashes and does not publish partial content; lost receipt replay retains the same causal version |
| Finite admission and resources | 64 data plus two enrollment tunnels, actual signed-token/fresh-proof attachment and live forwarding, maximum/one-over/text frame tests, service and pair overload refusals, already admitted data progress under exhausted enrollment/data admission, canceled bandwidth waits and reconnect bucket retention |
| Compatibility and integration | Uncached CLI/control/config/network/rendezvous/replication/scheduler/model compatibility, manual T04/T05 process journeys and background 16-MiB archive alongside eight edits; integrated make check and static service builds recorded separately |

The capacity fixture seeds **accepted reservation state** with exact signed
credentials so it measures live attachment/forwarding limits without claiming
66 simultaneous complete offer/accept/control journeys. Separate production sync
and unknown-enrollment fixtures exercise the full authenticated coordination.
The test explicitly closes metadata keepalives and paces the existing shared-source
challenge bound; 256 accepted service sockets are an independent combined ceiling.
Heap, FD, goroutine and 8-MiB streaming samples are retained in the complete race
log. Those are whole test-process samples including both clients and the broker,
not isolated broker peak RSS or Pi measurements. No file-size buffer is added to
production forwarding. Quotas remain visible and configurable.

[Architecture](../../orbit-wan-architecture.md#w04-encrypted-relay-integration)
records exact ceilings: 128 combined intents/tunnels, 64 data/two enrollment,
eight data per device/two per pair, fixed 32-KiB copy/frame bounds, 20/5-MiB/s
aggregate/device rates with 32-KiB bursts, 16-GiB ciphertext per tunnel, bounded
reconnect-retained buckets, 30-second unused attachments, 60-minute active tunnels,
60-second idle and a finite 30-second request drain grace. Broker faults/profile
expiry close immediately. Allocation/forwarding acknowledgements cannot enter
receipt persistence. Service restart invalidates tokens without changing device
keys, reviewed folder authority or retained verified chunks.

Initial failures and their passing repairs are retained in commands.md, including
the real peer-trace/outer-TLS composition bug and unknown enrollment acceptance
repair. No failed or zero-matched invocation establishes acceptance.

Limitations: all service/TLS/WSS and engine/process fixtures run on one Linux
host, using development profiles/test roots and synthetic disposable state.
Twenty restart repetitions replace the actual local HTTP service/epoch and rebuild
coordination; they are not 20 OS-process kills or physical-network changes. W03's
existing standalone binary restart evidence is retained separately. Hosted
operator/default profiles/signing custody, physical WAN/NAT, Pi peak RSS/FD/CPU,
monitoring/egress spend and packaged service deployment remain W13/W15/W16.
The daemon owns and serves isolated virtual listeners, while default installs
continue manual/no advertising. Saving a profile does not opt into WAN traffic.
`RelayEndpoint` requires caller-owned announcement renewal/generation and rebuild
on service changes; durable routed enrollment/setup and activation remain W05–W07,
and automatic roaming/reconnection policy remains W11. QUIC/ICE and native WAN
acceptance are unexecuted. T13 login/logout/unattended boot remains outstanding;
P17 owner personal use/explanation remains deferred.

Next eligible: **W05 — Routed enrollment v3 and peer data**.
Implement its durable logical routes, exact signed enrollment transcripts and
restart/replay/approval/bootstrap behavior using this transport; preserve v2 manual
signed bytes and folder/retirement gates. Ordinary reviewed CLI/TUI activation
belongs to W06/W07; no default endpoint/operator is invented here.
