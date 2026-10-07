# W15 scenario and invariant matrix

Scope: isolated Linux native sockets/processes and in-process Pion vnet emulation.
The command records and logs supply actual outcomes. A `SKIP` is unexecuted;
namespace-only cases have separate explicit runner records. Physical WAN/N10,
WG6 operator readiness, inherited T13 lifecycle checks and personal owner review
are outside these local acceptance claims. Original failures remain uncredited.

| Scenario | Exact oracle / invariant | Named production coverage / record |
| --- | --- | --- |
| LAN and reachable simulated public IPv4/IPv6 | Approved pin before HTTP; same immutable heads/authors/hashes/bytes; N01–N02, I09 | W08 multicast/local IPv6 regressions in full race; `native-public` guarded TCP/directory journey |
| Independent mapping and dependent filtering | Actual Pion/HTTP3 both directions; exact heads/hashes, chunk reuse; N01/N05/N07 | `TestWANW10AuthenticatedNATPeerMatrix`, `TestWANW10ICEHTTP3NATMatrix` in final security and full race |
| Dependent mappings, incompatible double NAT, blocked UDP | Direct check fails with typed reason; separate WSS fallback verifies two-way bytes; N03/N05/N07 | Same NAT matrix; `native-ice-final` executes native ICE and failed-gather relay cases |
| Blocked peer TCP and UDP | Service/control remain permitted; observed relay route delivers exact protected bytes; N01/N05 | W15 `crash` runner: initial block and post-crash block on both IP families |
| Delay/loss/reorder/bandwidth cap | Actual kernel netem parameters in topology JSON; finite wait, verified bytes/heads and endpoint receipts; N05–N07/I13 | `impaired-final` local and `pi-impaired-runtime-fixed` isolated runner records; supported outcomes and failed runs distinguished |
| MTU 1280 | Native packet paths and fallback preserve exact content; N05/N07 | `mtu-final` guarded whole-daemon record; packet adapter truncation/MTU regressions in full race |
| Malicious directory, substituted identity/pin, DNS rebinding and SSRF | Reject before route use/secret disclosure; all DNS answers scoped and bounded; N01–N02/I09/I23 | `TestWANW03LookupRejectsSubstitution`, `DNSAllAnswersAndRebinding`, W01 URL/proof binding fixtures, W03 candidate/body bounds |
| Unknown relay requester, purpose, folder and owner-control path | Endpoint mTLS and current per-request membership; no enrollment→data/control access; N01/N03/N04/I09/I23/I24 | W04 production unknown-enrollment isolation, wrong attachments, purpose isolation and W02 unknown-requester/pin checks |
| Relay inspects or disrupts traffic | Broker sees inner TLS ciphertext; oversized frames bounded; loss/cancel closes both legs; N03/N05/N06 | W04 broker inspection, post-auth frame bounds, abrupt loss, byte/idle/lifetime quotas and production relay receipt replay |
| Replay, expiry, malformed signed metadata | Fixed independent canonical fixtures and authority; no live replay guard eviction or authorization renewal; N02/N04/N09/I23/I28 | W01/W03/W04/W05 production fixtures and models; W15 signed-profile and seven-record codec fuzz targets |
| Malformed/oversized STUN and shared-NAT floods | Silent refusal, fixed amplification and prefix/global/source/socket bounds; N02/N06/N07/I13 | W10 STUN mapping/rate fixtures, `native-ice-final` UDP6; W03 shared-NAT/control/socket tests; W13 320-socket rehearsal in full race/aggregate |
| QUIC↔TCP transfer/receipt boundary loss plus GC | Pending content and receipt eligibility match independent symbolic oracle at five boundaries; exact version survives; verified chunk reuse; I01/I05–I07/I10/N05 | `TestWANW15DurableBoundariesMatchIndependentModel` (two race repetitions plus final security/full race), with complete named traces |
| Whole-daemon chunk and committed-receipt SIGKILL | Stop exact marked child; reopen correct durable readiness; original large head/author/pin/key/bytes and endpoint receipt survive relay recovery; I01/I05–I08/N01/N05 | W15 guarded `crash` fixtures, local/Pi; physical HTTP response packet drop is not claimed by this callback boundary |
| Interface/default-route changes and flapping | New generation retires stale attempt state without changing version/operation identity; bounded admission; N01/N05/N06 | W15 whole-daemon route journey; W15 refused-announcement accepted-generation regression; W11 actual generation/drain/race/slow-DNS competition and queue retry/reload fixtures |
| Directory/relay outage and restart | Existing authenticated direct path transfers a new captured file; restarted service accepts fresh leases/relay transfer; same immutable heads and identity; N05/N08/N09 | Final W15 `crash` journey and W10 actual consent/pair rebuild/outage matrix |
| Crashes during enrollment | Exact operation/attempt/request/root/key; no duplicate registration or renewed capability; exact owner approval; I08/I22–I24/I28/N04/N05 | `local-enrollment`: prepared, accepted, approval and membership-received SIGKILL; full race/aggregate W05/W07 CLI/PTY |
| Publication interruption plus route change and GC | Post-rename journal recovers exact protected bytes/head/hash; scan produces no extra version; I06/I07/I10/I17/I26/N05 | `TestWANW15RouteSwitchPublicationGCConflictAndRetirement/publication`; P04/P16 actual process boundary campaigns retained and rerun |
| Offline conflicts and retirement across routes | Two exact conflicting heads and both working candidates survive GC and QUIC/TCP convergence; cached authenticated TCP returns exact `UNAUTHORIZED` after retirement; I03/I04/I10/I15/I24/N01/N05 | W15 conflict/retirement subtest; causal/membership/GC reference-set models and production adversarial fixtures |
| Slow receiver and adversarial admission | Fixed streaming buffers; bounded heap/session/socket/resolver/dial work; cancellation and shutdown join; N06/N07/I13 | W09/W10 streaming tests, W11 32-peer slow-DNS/relay competition and flapping; W04/W13 service resource checks |
| Mixed small/large fair progress and throughput | Three peers; shared 2 MiB/s admission, verified 16 MiB version plus continuous small versions; CPU/RSS/heap/FD/socket/goroutine samples; I13/I14/N06 | `local-fairness`, `pi-fairness`, full race/aggregate W11 fairness fixture; raw measurements retained |
| Truthful status, diagnosis, CLI/TUI parity and privacy | Passive status performs no probe; bounded explicit doctor; route separate from stored/applied/conflict; mode disable stops service traffic; I19/I21/I27/N08/N09 | W12 real binary/PTY/doctor/privacy and control-support export fixtures; W06/W07 actual binary/PTY journeys |
| Profiles, expiration, rotation, migration and mixed versions | Explicit independently reviewed trust and opt-in; original identities/counters/history/keys retained; no silent downgrade; I07–I09/I20/I22/N09 | W03/W13 rotation/expiry/operator tests, W14 packaged adoption/code/legacy binary fixtures; full race/aggregate |
| Native bundled-profile physical WAN and ordinary setup | Requires real direct/relay on physically separate networks with no VPN/forwarding | **Unexecuted here — W16; WG6 still open.** |
| Native expired immutable bundled profile | Unit/config boundaries execute; no manipulated wall clock for static release binary | **Native case unexecuted, retained W14 limitation.** |
| Hardware power loss and login/logout/boot | Requires separately owned VM/storage/native lifecycle evidence | **Unexecuted in W15; retain historical P/T checks and unfinished T13 obligations.** |

I01–I20 have individually named `TestP16InvariantIXX_*` checks plus the owning
production suites. I21–I28 are covered by terminal launch, reviewed setup,
enrollment, membership, exact content leases, file operations, PTY and lifecycle
pruning/replay fixtures in the aggregate/full race campaigns. The independent
DAG model exhaustively enumerates bounded actor schedules, uses deterministic
longer seeds, compares production heads, and detects an intentionally mutated
dominance rule. The reference-set/membership/WAN models keep content availability,
route generation and durable approval separate. W15's new transfer model imports
no production SQL/hash/transport implementation; production boundary tests compare
its readiness and receipt outputs against actual repositories and authenticated
peer traffic.
