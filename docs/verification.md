# Verification and evidence contract

Status: local design/model, unit, integration and process-fault suites exist
through P17. Required release experiments and their remaining gaps are in
[packet status](implementation/status.md) and the
[2026-10-01 evidence](evidence/release-20261001/summary.md). Passing a named
check verifies its stated oracle; it does not prove every failure model.

## Invariants

| ID | Observable requirement | Primary test surface |
| --- | --- | --- |
| I01 | Immutable version ID has exactly one envelope; duplicate delivery creates no logical duplicate | History/repository |
| I02 | Same valid causal history yields equivalent heads regardless of delivery order | Independent model vs history |
| I03 | Concurrent content and edit/delete heads survive until explicitly covered by a reviewed resolution | History/control |
| I04 | Ordinary capture does not implicitly resolve received but unreviewed heads | Working-basis model/workspace |
| I05 | No stored receipt without durable metadata and required verified content | Repository/process harness |
| I06 | Partial or corrupt content is never published as complete | Transfer/workspace |
| I07 | Recovery preserves previously durable protected versions and reports ambiguity | Journal/fault harness |
| I08 | Counter and event creation are atomic; identity rollback is never knowingly reused | Repository/lifecycle |
| I09 | Peer input cannot escape its authorized folder or request unauthorized objects | Peer/path interfaces |
| I10 | GC never removes protected content; expiry never erases causal knowledge | Independent reference-set model/repository |
| I11 | Unavailable roots/incomplete scans/bootstrap absence do not create deletions | Workspace |
| I12 | Concurrent structural operations preserve incompatible histories without recursive destructive replacement | Directory model/workspace |
| I13 | Work and resource use are bounded; large-file work eventually progresses when resources are available | Scheduler/integration |
| I14 | Third-party forwarding preserves author/ancestry; hub availability is not final-device receipt | Replication/three-peer harness |
| I15 | Membership disagreement/retirement cannot silently admit excluded old histories or resurrect deletion | Membership model/integration |
| I16 | Restore/resolution replay is idempotent and stale reviewed state is rejected | Control/repository |
| I17 | Scan after apply/restart does not fabricate local edits | Workspace/integration |
| I18 | Corruption yields unavailable state or verified repair, never substitute contents | Repository/repair |
| I19 | UI and CLI issue the same operations and display qualified progress | Control/UI end-to-end |
| I20 | Limits, schema/protocol incompatibility and failed migrations preserve recoverable state | Operations/integration |

## Orbit revamp verification (planned)

I01–I20 remain required. These additional oracles apply to the
[Orbit packets](orbit-implementation-plan.md). Design gates G01–G05 formally closed
foundational oracles for I21, I23, I24, I25, I26, and I20 in [Orbit design gates](orbit-design-gates.md),
implemented in `tests/designgates/orbit_g*.go` and `model/membership.go`.

| ID | Observable requirement | Primary test surface |
| --- | --- | --- |
| I21 | Local launch/bootstrap preserves session security and exclusive daemon ownership; closing the UI does not stop sync | Launcher/control/real client lifetime |
| I22 | Setup/retry/restart preserves preexisting files and identity; bootstrap absence never creates deletion; completion reflects actual capture/service state | Setup/workspace/process |
| I23 | Invitation possession cannot grant data access; enrollment requires key possession and explicit approval of exact workspace/key; expired/revoked/replayed requests fail safely | Enrollment/control/adversarial peer |
| I24 | Approved membership rollout validates authority and revision ancestry; mismatches/forks are explicit; data remains gated and retired devices cannot rejoin | Independent membership model/three-peer process |
| I25 | Browse/content reports authorized locally known state accurately; queries/streams are bounded, requested bytes verified and reads protected against GC | Repository/control/browser/GC races |
| I26 | File-operation replay/stale review/interruption preserves protected versions; partial multi-path work is explicit and cancellation cannot pretend committed effects were undone | Journal/model/process/browser |
| I27 | Core workflows are keyboard accessible with visible focus and usable errors; UI never reports pending/failed engine work as complete | Real PTY/client assertions and owner walkthrough; historical browser evidence |
| I28 | Lifecycle-record pruning preserves pending work, journals/pins and causal metadata; expired idempotent replays cannot execute silently | Repository/scheduler/restart/resource |

The real browser runner is an O04 deliverable using existing puppeteer-core.
Source findings for identity reset/backup restore and inspection cleanup require
executable reproductions in O00; source inspection alone is not fault evidence.
Final Orbit evidence includes approval/offline membership rollout, file-action
crashes/races, read/GC leases, fresh-key recovery, legacy migration, native
packaged flows and actual owner use. Existing pilot data is never a fault target.

## Terminal redesign verification

The owner approved terminal-first use on 2026-10-03. Follow the
[T00–T13 plan](orbit-terminal-implementation-plan.md) and
[terminal tracker](implementation/terminal-status.md). These are planned
acceptance scenarios; T00–T12 runtime evidence is recorded in the tracker and
T13 adds native and release campaigns. I01–I28
remain authoritative. TG1–TG5 design and production evidence are separate from
historical browser G01–G05 outcomes.

| Terminal scenario | Required oracle | Packets / invariants |
| --- | --- | --- |
| Fresh initialization and concurrent launch | One selected daemon, stable identity, finite limits and loopback-authenticated control | T02 / I08, I13, I20–I22 |
| Existing-folder adoption and incomplete/stale preview | Correct measured review, preserved bytes, no bootstrap/inaccessible-root tombstones | T04 / I03, I07, I11, I22 |
| Ordinary two-device create/invite/join/approve | Verified inviter/requester, folder approval, actual capture/transfer and accurate phases | T03–T04 / I05–I06, I19, I23–I24 |
| Wrong identity/scope, replay, revocation and unknown data requester | Reject before secret disclosure or unauthorized access; bounded admission | T03 / I09, I13, I23 |
| Delayed approval and restart during join | Resume exact request/root/endpoint; retain identity/files; no premature Ready | T04, T10 / I08, I22, I27–I28 |
| Second folder shared to same device; another remains private | Reuse key without request collision; separate approval/root review; deny unshared access | T05 / I09, I16, I23–I24 |
| Offline/competing membership and endpoint refresh | Valid sequential rollout; explicit fork/recovery; data gates; actual reconnection | T05 / I14–I15, I24 |
| Cwd/duplicate-name/context and non-TTY commands | Exact validated target, ambiguity/stale review handled, stable JSON and no hidden prompt | T06 / I09, I16, I19–I20 |
| Saved/stored/applied, offline/indirect/stale peer status | Qualified observed state/freshness; no global synchronized or perpetual receipt claim | T07 / I05, I14, I19, I27 |
| New version during external-editor conflict review | Re-review changed heads; preserve competing/captured bytes; bounded merge and replay | T08, T11 / I03–I07, I13, I16, I25–I26 |
| Restore/copy with available/expired/missing/corrupt history | Exact verified bytes and reviewed destination; disabled unavailable actions; no invented retention window | T08, T11 / I06–I07, I10, I16, I18, I25 |
| TUI keys/resize/paste/editor return/quit and pipe | Visible focus, bounded work, terminal restored, no escapes in JSON, daemon keeps capturing | T09–T11 / I13, I19, I21, I27 |
| Package adoption/login/unattended/relocation/recovery | Native package execution, one daemon, preserved history/keys/roots, explicit lifecycle and safe rollback | T12–T13 / I07–I08, I17, I20–I22 |
| LAN/Tailscale, three hosts and faults | Recorded network/fault assumptions, head/hash oracles; owner review deferred until after delivery on 2026-10-04 | T13 / I01–I28, S22 |

Use production CLI/control/network interfaces and real PTYs for integration.
Mocks/snapshots can test renderers but cannot establish syncing, authorization,
service persistence or owner usability. Planned test groups require discovered
nonzero matches. Capture sanitized transcripts together with actual repository/
file/service assertions and dirty-tree/package provenance. Mark unavailable
hardware, required owner actions and unsupported failure models explicitly.
Existing validated disposable-root/process checks remain required for faults.

## Native WAN verification

Status: W00/W01 local evidence, W02 manual-manager, W03 directory/profile and W04 encrypted relay checks are recorded in
[the tracker](implementation/wan-status.md); native WAN acceptance remains pending. [WAN plan](orbit-wan-implementation-plan.md)
and [W tracker](implementation/wan-status.md) own packet ordering/evidence. I01–I28
remain required on changed paths; network routing adds these observable invariants.

| ID | Observable requirement | Primary test surface |
| --- | --- | --- |
| N01 | Every usable route authenticates the expected persistent device/pin; route/profile changes never approve membership or change identity | Direct/relay/QUIC TLS and existing authorization handlers |
| N02 | Discovery/candidate/coordination input is authenticated, scoped, expiring and bounded; stale or substituted addresses cannot become identity authority | Strict fixtures, independent admission/connection model, adversarial service |
| N03 | Relays forward opaque endpoint-encrypted traffic with isolated enrollment/data; owner control and folder authority remain inaccessible | Broker stream inspection, wrong-pin/purpose tests, real handlers |
| N04 | First-time routed enrollment verifies inviter before secret disclosure, requires exact owner approval and preserves expiry/replay/folder/retirement gates | Admission model, production enrollment, CLI/PTY process |
| N05 | Network failure/restart/switch preserves captured versions, stable operation IDs and truthful receipts; resume never renews authorization implicitly | Network/process faults plus head/hash/identity/receipt oracles |
| N06 | Dialing, buffers, leases, sessions, resolver work, retry and service admission have finite bounds with fair work progress | Slow receiver, overload, shared NAT, race/resource campaign |
| N07 | UDP/QUIC/ICE composition preserves datagrams, TLS/request semantics and socket ownership; failures cannot bypass verification or leak resources | Packet adapter, full ICE/HTTP3 integration, malformed/loss/close fixtures |
| N08 | UI/CLI report observed route/freshness separately from capture/stored/applied/conflict/membership and provide finite diagnostics | Typed controls, real binaries/PTys, passive-status network capture |
| N09 | Reviewed mode/profile/privacy choices control service traffic without changing keys/folders/history; signed profile expiry/rotation preserves trust | Packet capture, profile/migration/replay fixtures, packaged config |
| N10 | Packaged default setup works on declared real WAN networks without manual addresses/Tailscale or harness tunnels; self-host/manual compatibility remains | Native first-time CLI/TUI, actual direct and relay paths, package provenance |

| Required scenario | Oracle and evidence | Packets |
| --- | --- | --- |
| Preserved direct/manual baseline | Same IDs, accepted heads, protected hashes and per-request authority | W00–W02 |
| Directory ID/pin impersonation, stale lease, replay and candidate SSRF | Safe rejection; no overwrite of known routing identity; bounded allocations | W01, W03, W15 |
| Both devices behind NAT, no prior LAN, first enrollment through relay | Inviter pin before secret, exact approval, actual two-way captured transfer | W04–W07, W16 |
| Unknown requester/relay partner, unshared folder and control URL | No metadata/chunks/owner control; purpose isolation | W01, W04–W05, W15 |
| Relay inspected/tampered/restarted or attachment expired | No readable content/secrets; pin failure; safe chunk/operation retry | W04, W11, W15 |
| Existing contents, delayed/revoked/expired approval, second folder, retired identity | Existing root/proof/membership oracles preserved; exact durable resume | W05–W07, W15–W16 |
| LAN discovery, public TCP/IPv6 and unsupported interface/occupied optional listener | Verified direct candidate or qualified relay; no false reachability | W08, W15–W16 |
| ICE mapping/filter combinations, double NAT and blocked UDP | Direct where supported; encrypted relay otherwise; topology/rules recorded | W09–W11, W15–W16 |
| UDP packet truncation, MTU/loss/reorder, roles and wrong pins | Datagram and HTTP/TLS invariants; bounded failure/cleanup | W09–W11, W15 |
| Direct↔relay, TCP↔QUIC, interface changes, flapping and lost receipt | Preserved hashes/heads/operation IDs, no premature receipt; measured recovery | W11, W15–W16 |
| Large/small concurrent work, slow reader and shared-NAT admission floods | Declared CPU/memory/FD/goroutine/session/egress bounds and fair progress | W04, W10–W11, W13, W15 |
| Service down, quota, wrong pin, offline peer, conflict and membership fork | Distinct qualified CLI/TUI status; diagnosis timeout/cancel; no hidden probe | W06–W07, W12, W15–W16 |
| Local-only/manual/self-host profiles, expiration and rotation | Packet-captured privacy, normal TLS/pins, explicit incompatibility | W03, W12–W14, W15 |
| Legacy state/v2/manual peers and interrupted upgrade | Stable keys/counters/history, no proof downgrade, compatible refusal/rollback | W14–W15 |
| Fresh packaged hosted-default WAN setup | Actual profile/operator, no endpoint editing or forwarding fallback, real direct+relay evidence | W13–W17 |

Gate spikes/model fixtures close design only. Production tests complete packets;
native packaged first-time setup completes the real-network requirement. Discover
nonzero matching tests and record exact paths/commands. Network harness commands
are delivered in W15 and use existing explicit disposable markers plus canonical
root/process/namespace ownership validation. Save minimized failures and label
privilege/unavailable checks. Never inject faults into personal roots or an existing
VPS workload. Native administration via SSH is separate from measured sync traffic;
SSH forwarding and existing private VPNs cannot establish no-Tailscale WAN evidence.

Measure connection/reconnection timing, selected route, throughput, verified chunk
reuse, idle/active CPU, peak memory, FDs/goroutines, relay ingress/egress and admission
backpressure with reproducible small/mixed/large fixtures. State hosts, seeds,
sample counts, dependency/profile provenance and topology. No fabricated throughput,
universal NAT success percentage or guarantee for an inaccessible school network.
Existing abrupt-reset/publication/storage checks retain their fault models.

WAN release requires at least two physical networks, real direct and relay cases,
ordinary bundled-profile CLI/TUI enrollment, and actual laptop/Pi/VPS engine
workflows. If a required direct/native/operator condition is unavailable, keep
that item unexecuted and the relevant packet incomplete. Prior three-host SSH/
manual evidence remains valid for its scope, independently of WAN acceptance.

## Independent causal model

Implement a small test-only event DAG oracle with explicit parent reachability rather than copying production vector-comparison code. It models accepted versions, reviewed resolutions, retirement acceptance, heads, and content availability as separate state. Include a separate simple reference-set oracle for GC. Production and model may share fixture encodings but not reconciliation or reachability algorithms.

Generate local capture, ordinary edit, delete, resolve, restore, duplicate delivery, message reorder, disconnect, forwarding, restart, membership changes and content expiry. Use deterministic seeds and explicit schedules. Transport delivery is an action, not a wall-clock sleep. Drain valid work at the end and compare normalized head/conflict sets, accepted identities, protected content requirements and reported availability. Do not compare transient working bytes as though conflict state had already been resolved.

Exhaustively enumerate bounded small histories for 2 and 3 actors; run seeded longer histories for 3–4 actors. Record the enumeration bounds and seed set. A failing run saves its complete trace and a minimized reproducer. Deliberately mutate a production causal rule to demonstrate that the independent oracle catches at least one defect; revert the mutation before acceptance.

## Scenario matrix

| Scenario | Required oracle/result | Invariants |
| --- | --- | --- |
| Three offline edits, every reconnect order | Same three heads and verified bytes retained | I01–I04, I14 |
| Resolve A/B, later receive C | Resolution and C remain concurrent | I02–I04, I16 |
| Equal-byte independent writes | Distinct ancestry remains until explicit resolution | I01–I04 |
| Same-author edit from stale basis | Safe block/review or proven ordered capture; no false dominance | I04, I08 |
| Delete vs edit; repeat delete; restore deleted file | No silent loss/resurrection; restore has new ancestry | I02, I03, I16 |
| Existing divergent enrollment folders | Preview conflicts; no bootstrap tombstones | I03, I11 |
| Parent delete vs new child; file/directory collision | Structural conflict and preserved child bytes | I12 |
| Editor rename/overwrite during transfer/apply | Supported observed edits preserved; unsupported race limits demonstrated honestly | I04, I07, I17 |
| Root unmount/replacement; unreadable subtree | Pause/partial-scan warning; no inferred mass deletion | I11 |
| Crash before/after every durable boundary | Recorded recovery outcome and protected hashes intact | I05–I08 |
| Transfer drop mid-chunk/mid-file; lost receipt | Verified chunks reused, whole-file digest correct, no false stored state | I01, I05, I06 |
| ENOSPC during write/fsync/SQLite/checkpoint/staging | Good captured data remains; bounded visible block | I05, I07, I13 |
| Bit flip in current/history/shared chunk | All affected versions diagnosed; authorized repair verified | I06, I18 |
| GC races receive/serve/restore/publication/restart | No protected object removed; leftovers may be reclaimed | I07, I10 |
| Long-offline enrolled peer and expired history | Current causal reconciliation correct; expired payloads explicit | I02, I10, I15 |
| Retirement/config mismatch/stale rejoin | No unknown retired history admitted; reviewed new identity enrollment | I08, I15 |
| A→VPS→B with no A/B link or online overlap | Original authors preserved; B eventually receives; A status is qualified | I14 |
| Unauthorized folder/hash, traversal, symlink race, malformed manifest | Rejected before unsafe IO/allocation | I09, I20 |
| Continuous small edits plus large archive | Bounded memory/FDs; large archive advances | I13 |
| Migration interrupted and newer schema opened | Safe recovery/refusal; no counter rollback reuse | I08, I20 |

## Failure harness design

Run actual agents with separate state directories, roots, ports and identities. Mark each environment disposable; validate all destructive targets remain beneath its canonical root before injection. Refuse symlinked targets, personal paths and remote production services. Reserve ports dynamically. Save generated configurations with secrets redacted in published artifacts.

Named hooks are emitted at object flush/install, metadata transaction boundaries, publication intent/stage/rename/directory flush/final commit, GC intent/unlink/finalization, receipt send and transfer progress. Harness waits for explicit hooks, then kills/disconnects/injects errors. Hooks are disabled or safely inaccessible in release operation; never expose a public kill endpoint. Use an injected filesystem adapter for deterministic IO faults and actual-process tests to verify production integration.

Use namespaces/containers or a protocol proxy for partitions/delay/drop behavior. The model handles message reordering; TLS transport experiments must not claim arbitrary tampering with encrypted application frames. Abrupt-reset experiments use disposable VMs/storage images and documented commands. Real Pi/VPS experiments are non-destructive unless explicitly conducted in a separate disposable environment.

## Test tiers and CI

- Every change: format, vet/static checks appropriate to pinned tools, compilation, unit/model fixtures, relevant packet tests.
- Pull request: integration scenarios, Go race detector on supported runner architecture, fixed deterministic seeds, protocol/schema fixtures, frontend checks once present.
- Scheduled/manual campaign: broader seeds, fuzzing, real filesystem fault matrices, long-running resource/fairness checks.
- Release: packaged binaries, local demo from clean checkout, actual three-host workflow, documented abrupt-reset experiments and personal pilot.

The Makefile provides these validation targets: `make check`, `make test`, `make test-race`, `make test-integration`, `make test-model`, `make test-faults`, `make test-harness`, `make demo`, `make build`, and `make package`. Integration and race targets build the required binary first. Explicit QEMU abrupt-reset commands are separate from ordinary checks. Publish what each target executes and any privilege requirements. Destructive tests are never a hidden dependency of ordinary checks.

## Benchmark plan

Label fixtures as synthetic and personal pilot inputs as real. Use reproducible generators and content hashes; exclude private contents from evidence. Initial workloads:

- Small files: 10,000 files around 4–64 KiB, varied directory depth.
- Mixed: documents/images plus several 10–100 MiB objects.
- Large: one 1 GiB file, plus a larger file only where declared budgets allow.
- Change patterns: unchanged scan, small in-place overwrite, append, prefix insertion, rename, deletion and concurrent edits.
- Network conditions: unrestricted local link, throttled link, latency/interruptions, and actual VPS path.

Compare full-file transfer with missing-chunk transfer using identical contents, transport settings, verification, cache state and concurrency. Report metadata/TLS overhead separately where measured and distinguish payload bytes from on-wire bytes. Warm/cold object caches and filesystem caches must be identified. Repetitions and median/tail summaries require enough samples; no p99 claim from a tiny sample. Record raw runs, failures and outliers.

Metrics: bytes transferred/reused, wall/CPU time, scan/hash time, time to durable receipt and application, peak RSS, open descriptors, storage amplification, conflict resolution propagation, crash recovery time, interruption retransmission overhead, and large-file progress under small-file load. Show negative results such as prefix insertion defeating fixed-size chunk reuse. Do not choose a speedup target before measuring.

## Release evidence layout

Create as work occurs, not empty claims:

```text
docs/evidence/<run-id>/
  manifest.json       commit, build/protocol/schema versions, hosts, OS, filesystem, workload and seed
  commands.md         setup/run/cleanup and exact failure schedule
  results.json        raw counters and pass/fail assertions
  summary.md          conclusions, limitations and links to logs
  logs/               bounded sanitized event logs and minimized failure traces
```

P17 produces a 3–5 minute demo outline: enrollment → normal edit → three offline edits → reconnect/conflict → reviewed resolution → interrupted large transfer/resume → restore → qualified per-device status. Include a separate failure/recovery clip if needed. The case study explains one difficult bug and one rejected design, personal contribution, architecture, measurements, and limitations.

No claim of consensus, exactly-once network delivery, unlimited scale, universal no-loss behavior, or independent backup. Idempotent logical effects are not exactly-once delivery. Report three physical/cloud hosts as tested environments without pretending they establish arbitrary datacenter fault independence.

## Primary design references

Consulted for architecture planning on 2026-09-20; recheck version-sensitive APIs during implementation. Sources inform the design; this project does not inherit their correctness or claim compatibility.

- [Syncthing BEP](https://docs.syncthing.net/specs/bep-v1.html): example separation of file vectors, transfer blocks, and inventory sequencing.
- [Syncthing synchronization](https://docs.syncthing.net/users/syncing): comparison point for conflict handling; this product chooses explicit reviewed resolution.
- [Syncthing versioning](https://docs.syncthing.net/users/versioning.html): useful comparison for retention scope; our captured-version contract must be tested independently.
- [SQLite atomic commit](https://www.sqlite.org/atomiccommit.html): database durability assumptions and failure boundaries.
- [SQLite WAL](https://www.sqlite.org/wal.html): local WAL operation, checkpointing and deployment constraints.
- [Linux rename](https://man7.org/linux/man-pages/man2/rename.2.html): namespace replacement does not invalidate existing open descriptors.
- [Linux fsync](https://man7.org/linux/man-pages/man2/fsync.2.html): directory-entry durability requires separate consideration.
- [Go os documentation](https://pkg.go.dev/os): check descriptor-rooted APIs against the selected toolchain; name validation alone is not race-safe containment.

## Current executable release campaigns

- `python3 -m unittest discover -s scripts/validation -p 'test_*.py'` exercises
  the actual host worker's marker, path/link and process-identity refusals.
- `make demo` runs the local multi-process demonstration without cloud access.
- `python3 scripts/validation/abrupt_reset.py --kernel /path/to/vmlinuz --output /path/to/evidence`
  boots newly created ext4 images, stops only the child QEMU process at actual
  production hooks, then boots a fresh guest to verify protected hashes and
  recovery. Its dirty-cache negative control must lose an unflushed overwrite.
- `go run scripts/three_host_pilot.go --laptop laptop --pi rpi --vps vps`
  performs the scripted actual-host scenarios in fresh private roots. It stops
  the receiving sync process after observing durable verified chunk progress,
  restarts it, checks reuse of every recorded verified chunk and bounds new
  fetches by the remaining count, then verifies whole-file hashes. A chunk may
  become durable before its progress row commits, allowing additional reuse.
- `go run scripts/benchmark_suite.go --small-files 10000 --large-mib 1024 --repetitions 1`
  generates deterministic synthetic fixtures and counts actual encrypted TCP
  bytes. Both alternatives use TLS 1.3 mutual authentication and whole-file
  verification; the full-file baseline hashes existing files and skips unchanged
  contents, flushes received files, and publishes through rename/directory flush.
  Raw sample counts accompany every summary. These are warm filesystem-cache
  runs, with fresh application stores per repetition.
- `python3 scripts/validation/reproduce_release.py --source COMMIT --kernel /path/to/vmlinuz --output /empty/evidence`
  checks an explicit committed revision in a new marked checkout, records all
  commands, runs local/race/demo/reset/disk-exhaustion checks and verifies
  checksums plus identical packages on a second build. Native host campaigns
  use binaries extracted from those checked archives.

Traffic counters include TLS record/handshake and HTTP overhead in both
stream directions; they exclude TCP/IP and SSH encapsulation. Call them
TLS/TCP stream bytes, rather than physical-link wire bytes. The algorithms
perform different history/storage work, so their times do not establish a
universal speedup. Proxy delay is stated per read, not claimed as calibrated RTT.
Automated sessions remain distinct from owner-reported personal use.

The current benchmark enables TCP_NODELAY on proxy and baseline sockets to
match Go's defaults. Earlier proxy runs retained default Nagle behavior;
their measured byte counts remain historical evidence, while their timings
include artificial proxy/delayed-ACK stalls. Current finite storage admission
costs are included in the release run. Primary API reference:
[Go TCPConn.SetNoDelay](https://pkg.go.dev/net#TCPConn.SetNoDelay).

Actual disk exhaustion runs in new guest ext4 images cover incoming writes,
SQLite WAL growth, checkpoint allocation and 2-MiB publication staging.
`enospc.fsync` is a separate VM-child syscall experiment: a seccomp filter
returns ENOSPC from fsync/fdatasync, then the parent checks protected content.
It is explicitly fault injection rather than a naturally full filesystem's
delayed-allocation flush behavior. See
[Linux seccomp filter semantics](https://www.kernel.org/doc/html/latest/userspace-api/seccomp_filter.html).

## T01 design evidence boundary

[T01 evidence](evidence/terminal-t01-20261003/summary.md) records nonzero codec/
fixture tests, canonical request/signature binding, isolated prototype TLS and
bounded admission/restart/adapter/session/lifecycle models. These exercise
I09/I13/I16/I19–I28 at the contract/model boundary; they do not prove production
networking, durability, GC races, editor behavior, service modes or keyboard
accessibility. TG1–TG5 production proof remains open in T02–T12. Keep T00
reproductions opt-in/failing until repaired. See
[source ownership](implementation/terminal-source-ownership.md) for integration
responsibilities and [gate decisions](terminal-design-gates.md) for exact oracles.

### T03 production checks

[Current T03 evidence](evidence/terminal-t03-20261004/summary.md) covers actual
nonloopback TLS/HTTP and owner control: pinned identity before disclosure,
canonical signed scope/attempt binding, transactional single use/replay, exact
review and membership approval, possession-based status and nonce replay refusal,
expiry/revocation/stale revision rejection, isolated data/control routes,
certificate-required peer admission, body/rate/pending/nonce limits and bounded
IP accounting. Caps use seeded durable records plus actual HTTP refusal; they do
not claim 128 separately enrolled production devices. Frozen canonical membership
bytes are decoded against the existing P01 golden, with malformed-input checks.

The two-process scenario has the receiving daemon process sign/send its own
request and status proof, exchanges canonical owner-approved membership, imports
it under authenticated receiving owner control and verifies the exact-revision
peer hello. A graceful owner restart preserves invitation and approval replay and
signed artifact retrieval. The retained legacy pairing test verifies preexisting
bytes, revision/key sets, authenticated certificate/endpoint persistence and
aliases; the real CLI test uses private invitation/review files against a running
owner daemon. These establish transport/authorization, not T04 initial scan
readiness, content-sync completion, SIGKILL durability, native boot/logout,
physical two-host LAN or Tailscale availability, or P17 owner learning/use.

### T04 production checks

The `TestTerminalT04` group exercises descriptor-rooted recursive measured review,
private continuation after reopening state, content/stat/root swap/symlink and
changed plan invalidation, unsupported/unreadable paths, finite/capacity and review
expiry rejection, one-use reviews, failed service enablement, capture/traversal
failure, bootstrap-complete gating and durable phase recovery. Existing histories
remain concurrent conflicts with both manifests verified. Actual CLI daemons create,
invite, review, join and approve through authenticated control/network paths; the
receiving child is killed only after acknowledged pending admission in a marked
root, restarted with the same request/key/root, and completes without the waiting
CLI. Ordinary edits transfer in both directions, followed by exact single-head and
managed-content hash assertions after stopping both owners. Other phase tests use
production fault hooks plus close/reopen and do not claim SIGKILL at every boundary,
power loss, native hosts, TUI keyboard/lifetime behavior or owner use. A separate
actual CLI PTY case corrects an invalid finite value, edits the review, preserves
entered names and existing bytes, observes plain Ready output and checks that the
daemon survives client exit. This is setup-form evidence, not T09 TUI acceptance.

### T05 production checks

Six ordinary `TestTerminalT05*` tests cover scoped same-device sharing/replay,
wrong ID and rekey rejection, exact predecessor and two-step production sync
rollout, wrong TLS member key, data denial while revisions differ, competing
reviewed approval refusal, conservative fork recovery, and a three-daemon process
journey. The latter checks independently reviewed existing second-root bytes,
retained first attempts, denied unshared inventory, offline member/inviter restarts,
explicit B/C endpoints, A-authored forwarding through B and both B/C pull
directions with A stopped, live CLI address refresh,
connection-error work observations, unchanged keys/membership and exact head/hash
agreement after graceful stop. Native cross-host LAN/Tailscale and boot/logout
are separate T12/T13 evidence; local nonloopback processes do not establish them.

See [T05 evidence](evidence/terminal-t05-20261004/summary.md) for commands/results
and limitations. The process fixture restarts the inviter between enrollment
journeys, exercising durable records and resetting the real process-local rate
bucket; it does not bypass the admission limiter or prove unlimited immediate
same-IP joins. All destructive fixture lifecycle actions validate the disposable
marker and target; personal roots/services are untouched.

### T08 production checks

Nine ordinary `TestTerminalT08*` cases exercise authenticated live/stopped exact
reads/ranges, an active large response racing GC, unavailable and expired reopening,
reviewed heads and uncaptured working changes, restore source/current-parent
separation, explicit copy collision plans, durable partial-copy/response-loss replay,
6 MiB streamed merges, new arrival during session review, corruption/pending/expired
restore refusal, Deleted candidates and bounded structural conflict pages, interrupted
upload/restart attention, real tool exit/crash and file-size enforcement, renewal,
expiry and reviewed discard. A real daemon-backed CLI journey invokes select,
keep-copies, original-path restore, exact external export, configured editor and
6,000,000-byte merge; tests compare committed heads/provenance and actual bytes/hashes.

Process RSS observations are recorded in packet logs as Linux child `ru_maxrss`
including fork/exec high-water effects, rather than an isolated steady-state CLI or
production capacity benchmark. One fixed synthetic large workload is evidence of
actual streaming, not a scaling curve or throughput guarantee. Other fault/restart
cases use authenticated production control with marked fixture state and named
production hooks; they do not establish SIGKILL at every boundary or power loss.
Interactive PTY editor suspend/resume is T11/T13; native hosts/network/lifecycle
and P17 actual owner use/unaided explanation remain outstanding.

### T09 production shell checks

Seven ordinary `TestTerminalT09*` cases exercise the selected stable Charm v2
stack, focused text before shortcuts, j/k/arrows/Tab/Enter/Esc/search/help,
bounded one-lane queries, cancellation, correlation, preserved selection/drafts,
Unicode/control escaping and 80x24/narrow layout. Folder/device pages operate
the real SQLite/control seam across 45 fixtures, bounding returned rows and
rejecting a selector-mismatched cursor. Attention retains T07's existing
computation and bounded response; these checks do not claim constant-cost
whole-repository inspection.

The ordinary process test runs `scripts/terminal_pty_test.py` against the real
binary in new private marked state. It asserts 80x24/40x16, pasted j/k/q/? and
Unicode text, arrows/Tab/Enter/Esc/help, resize, colorless output, stdout/input
pipes and JSON, canonical input for direct-argv tools, successful actual scratch
edits, failed tool preservation, active-tool SIGTERM, q/Ctrl-C/SIGTERM exit, exact
termios/alternate-screen/bracketed-paste restoration and the same daemon's actual
new captured digest after every client has exited. A validated child daemon is
paused while holding its lock, producing unavailable-control feedback; resumed
control clears that error without a stopped-adapter ownership bypass. Process
identity/marker/canonical-target checks run before signals and cleanup. The VT
cell oracle asserts visible content from differential rendering; snapshots do
not substitute for byte/history/process assertions.

Use `make test-terminal-pty` or the [shell runbook](runbooks/terminal-shell.md)
for recorded sanitized transcripts. The packet race command exports
`GOFLAGS=-race` so built child binaries are instrumented too. T09 adds no
mutation/causal engine semantics. Reviewed conflict/editor screen integration
remains T11; native boot/logout/login/unattended, physical networks/faults and
P17 owner use/unaided explanation remain T12/T13 release evidence.

### T10 production keyboard onboarding checks

Ordinary `TestTerminalT10*` tests discover bounded blocked setup pages, conservative
folder/fork/retirement observations, exact private setup retry and listener-restart
intent, focused keyboard forms, stale replies, reviewed approval/sharing,
truthful readiness, masked/wrapped invitation input and narrow/colorless layouts.
The actual two-daemon PTY runner drives reviewed nonempty roots, validation/back/
edit, expired and wrong-pin invitations, delayed exact-request approval with
matching transcript codes, client close/reopen and real daemon restart. It checks
bidirectional bytes, a distinct second-folder attempt with persistent identities,
local pause/resume, actual relocation, unregister preview and continued capture/
transfer after all clients exit. Startup failure is real control execution under
an empty fixture PATH, keeping host services untouched. The fork fixture rejects
a competing branch then follows the owning conservative pause procedure; existing
T05 tests exercise sequential membership catch-up and stale-revision data refusal.

`make test-terminal-onboarding-pty` runs the new marked-root campaign. The VT oracle
now covers scrolling margins, reverse index, disabled autowrap and repeated
characters with independent unit assertions. Screen cells support presentation
checks; identities, actual byte/digest and control observations remain the
acceptance oracles. Packet race uses `GOFLAGS=-race` to instrument built children.
Native hosts, physical LAN/Tailscale, boot/logout/reset and P17 owner evidence
remain T12/T13 and are not established by these local processes.


## W03 local acceptance evidence

The [W03 record](evidence/wan-w03-20261005/summary.md) covers 24 discovered focused
checks in `internal/network`, `internal/rendezvous` and `internal/replication`.
They use production TLS/service/control/client and peer interfaces; disposable
private roots and a real `orbit-net` process verify restart without retaining
routes. Separate finite table-saturation fixtures test quota/expiry, and an
independent admission model checks accepted registration semantics. A 272-socket
fixture measures the 256 pre-TLS goroutine ceiling; a 1,000-canceled-call fixture
measures at most four resolver workers with zero active workers/sockets after
close. These local bounds do not claim Pi peak RSS/FD/egress or native WAN capacity.

The shared-NAT source is synthetic/local, not a physical NAT campaign. Privacy
checks assert zero DNS/service work for manual/local-only reannouncement and no
policy activation from a saved profile. Manual capture/verified sync survives an
actual local directory shutdown. Hosted-default packet capture, full WAN/NAT,
WSS forwarding and v3 enrollment remain later evidence. A real 25-second WSS
heartbeat and repeated binary restart cover lifetimes beyond immediate handshakes.
Inherited T13 login/logout/unattended boot and deferred P17 owner review remain
separate from these checks.

## W04 local acceptance evidence

Discover with `go test -list '^TestWANW04' ./internal/network
./internal/rendezvous ./internal/replication ./internal/scheduler`, then run those packages with
`-race -count=1 -run '^TestWANW04'`. The
[evidence](evidence/wan-w04-20261005/summary.md) distinguishes full authenticated
coordination/production engine sync from seeded accepted-reservation capacity tests.
Actual checks cover both outbound legs, wrong/replayed/expired tokens, restart
epochs, binary frame bounds, ciphertext inspection, idle/lifetime/byte/rate limits,
slow readers/cancellation and joined shutdown. Production TLS/HTTP/file tests
force direct listeners unavailable, verify two-way heads/hashes, folder/pin/control
isolation, pending unknown enrollment, verified-chunk reuse across service restart
and lost-receipt replay without false completion. Memory/FD/goroutine samples name
this single Linux development host; native topology, Pi peak RSS and operated
hosting remain W13/W15/W16. T13 lifecycle and deferred P17 owner work remain open.

## W06 CLI integration checks

Discover `TestWANW06` in `internal/config`, `internal/control` and `tests/terminal`.
The controller tests check exact policy/profile reviews, changed-input and stale
operation rejection, identity preservation, monotone generation writes and recovery
after accepted-before-effect interruption. Production-binary tests use explicitly
marked disposable roots and a signed local development rendezvous/relay with
separately trusted TLS. They assert private invitation permissions/redaction,
wrong-pin and expired fresh invitation rejection, no non-TTY hidden prompt,
guided CLI in a real PTY, reviewed script create/join, delayed/relaunched approval,
second-folder root/authority separation, verified bytes in both directions,
version author identity, saved inviter certificate and service outage/local capture.
The local service fixture is not native WAN or an operated default.

`make check` retains the complete terminal campaign. Its package timeout is now
thirty minutes because the combined serial T/W fixtures exceeded Go's ten-minute
default before queued parallel checks ran. Individual waits/processes remain finite;
an aggregate timeout is failed/uncredited evidence, not permission to skip tests.
W06 evidence records the first timeout separately from the final aggregate result.
T13 login/logout/unattended evidence and deferred P17 owner use remain independent.


## W07 TUI integration checks

Discover `TestWANW07` in `internal/terminal` and `tests/terminal` before executing
focused race checks. The production-binary `terminal_wan_pty_test.py` runner uses
an independently signed local-development service/profile and trusted TLS CA,
private marked roots and no configured direct endpoints. TUI keyboard mutations
cover measured adoption/Back/Edit/privacy, private v3 invitation transfer,
expired/wrong-pin rejection, unsupported-root correction, exact cross-device
approval, second folder, restartable operation/attempt/root and daemon work after
client exit. CLI queries verify actual bytes, hashes, exact version authors/heads,
inviter certificate/pin and persistent identities; renderer snapshots alone give
no milestone credit. Resize/colorless/paste and terminal restoration are checked
in actual PTYs. The service-outage hook closes only the disposable local service
and requires the explicit marked test root. Fresh Automatic with missing profile
and Local-only review retain local capture. Native WAN, hosted profiles and T13
login/logout/unattended boot remain unexecuted by this local campaign.


## W08 direct networking checks

Discover `TestWANW08` in `internal/network`, `internal/protocol`,
`internal/replication` and `tests/terminal`. Boundary tests execute strict independent
LAN canonical/signature fixtures; spoofed/unknown pins, oversized/truncated or wrong
scope inputs; independent interface/generation leases; expiry and 16-candidate bounds;
complete pinned family races, blocked/wrong-certificate fallback; cached empty leases;
32 blocked raw dials and 64 pre-TLS incoming sockets; 1,000 spoof/oversized datagrams
with FD/goroutine/whole-process heap samples and joined close.

Production engine tests exercise actual nonloopback IPv4 multicast discovery,
known-peer two-way hashes/heads, interrupted verified-chunk reuse and native local
ULA IPv6 TLS/HTTP transfer. The public fixture normally skips deliberately: run
`scripts/wan_direct_namespace_test.py` inside a newly created user/network namespace
with a private `.filesync-disposable` root and the compiled race test binary. The
script verifies namespace ownership with Linux `NS_GET_USERNS` before creating its
isolated dummy interface/addresses. Actual signed directory/public-scope IPv4/IPv6
TCP transfer and directory-outage direct reuse run there; addresses are simulated,
with no external route. This is not native internet reachability evidence.

Production binaries use invitation/pending denial/exact approval before Local-only
LAN, assert zero service requests and exact two-way versions/hashes/keys, and keep
capture after the disposable service closes. A separate fresh ordinary CLI journey
occupies optional listener ports from startup and verifies relay bytes/approval;
explicit manual collision still fails startup. Run with `GOFLAGS=-race go test
-race ./tests/terminal -run '^TestWANW08' -count=2 -v` to instrument launched children.
Retain failed restart-to-relay experiments for W11 and do not count them as W08
collision acceptance. [Evidence](evidence/wan-w08-20261006/summary.md) records actual
commands, source provenance, resource samples and remaining physical/operator limits.

## W09 QUIC checks

Discover `TestWANW09` in network, protocol, replication, config and terminal.
Focused race checks run actual native UDP HTTP3 and the established-pair test
adapter through production peer handlers; both listen/dial pull directions verify
bytes/heads/hashes and interrupted verified-chunk reuse. Packet boundaries, MTU,
truncation, reversible deadlines, address-change refusal, loss/duplicates, wrong
ALPN/pins/missing client certificate, unauthorized folder/version/body/header
inputs, partial chunks/no premature receipt, current borrower pin verification,
32-session admission, slow-stream heap/backpressure/cancel/join and preserved TCP
fallback have separate oracles. Pion's actual pinned Conn compiles against the
packet interface for amd64/arm64; actual ICE/NAT remains W10.

Real race-instrumented binaries use explicit invitation/pending denial/approval
before Local-only signed LAN QUIC discovery, assert dated `quic` observations,
two-way versions/hash/identities and zero service requests, and keep capture after
the local service stops. Occupied optional UDP/TCP sockets separately verify fresh
relay onboarding. These single-host disposable fixtures do not establish physical
WAN/Pi/hosted profile or T13 login/logout/boot acceptance. Older direct settings
and TCP fixtures are explicitly retained. Failed runs stay uncredited in the
[W09 evidence](evidence/wan-w09-20261006/summary.md).

## W10 ICE verification

Discover `TestWANW10` in network/protocol/rendezvous/replication. Production signed
coordination, STUN codec, packet adapter, HTTP3/TLS, peer handlers and sync engine
are exercised through in-process Pion vnet backends with exact configured mapping/
filtering rules. This is a disposable emulator, not a physical WAN/NAT measurement.
The native counterpart uses `scripts/wan_ice_namespace_test.py` in a new marked
private user/network namespace; it checks namespace ownership before any `ip`
mutation. Running the Go fixture without its explicit environment skips it and
cannot establish native acceptance.

Oracles include both pull directions, exact heads/hashes/authors, interrupted
verified-chunk reuse, actual QUIC versus WSS observations, typed fallback failures,
simultaneous controlling/controlled requests, actual consent-loss retirement/new
pair establishment, source/check/session/shared-QUIC limits, signed replay/pin/
role/session/generation/expiry refusal, malformed/rate-bounded STUN, slow streaming
and cancellation. Legacy protocol goldens and W09 authorization/deadline/native
binary/PTY checks remain compatibility prerequisites. Full physical WAN/Pi/hosted
profiles and native T13 lifecycle remain later acceptance, not emulator claims.


## W11 partial policy verification

`TestWANW11` spans `internal/control`, `internal/network`, `internal/replication` and
`internal/scheduler`. Tests execute pinned TCP relay/direct reprobes with injected
address/time outcomes, late-generation rejection, timed failed ICE reprobes,
non-sliding quota cooldown, real cold-race quota selection, bounded responder
retry, exponential cooldown reset, joined/coalesced watcher callbacks and 200
busy generations across eight targets. Sixteen live responses drain without
stale observations and prune all old pools. The persistent queue test forces a
synthetic 1-TiB declared task within nine ready dispatches across retry/reload;
it does not transfer that payload or measure bandwidth fairness.

The native replication journey interrupts a two-chunk HTTP3 transfer, switches
to actual pinned TCP, reuses the verified chunk, loses a receipt response and
switches back to HTTP3. Exact original heads/authors/manifests, working bytes and
durable receipts remain verified. The namespace-only test requires a private
`.filesync-disposable` marker and a different parent network namespace.
`scripts/wan_roaming_namespace_test.py` additionally validates user-namespace
ownership before any interface/route mutation; it exercises actual Linux address
and default-route detection and the same native replication test. A simulation or
isolated dummy interface is not physical Wi-Fi/Pi/native WAN evidence.

[W11 evidence](evidence/wan-w11-20261006/summary.md) retains commands, discovered
coverage, failed quota/deadlock reproductions and accepted runs. WG5/M3 remain
incomplete until timing controls, Pi/latency/loss/slow DNS/relay measurements,
whole-daemon route/interface switch journeys and measured large/small bandwidth
and combined resource fairness have evidence. Hosted defaults and T13 lifecycle
checks remain independently outstanding.

The final W11 partial-implementation `make check` passes, including the full
terminal suite (953.851 s). The initial aggregate failures and their focused
repairs remain in that evidence; this does not close the pending WG5/M3 campaigns.

## W12 diagnostics and privacy verification

The W12 focused suite covers passive status versus explicit doctor, bounded
cancellation/admission, actual STUN response validation, pinned direct/relay
TLS without HTTP or route-observation mutation, typed identity/timeout/quota
codes, profile-state/next-action rendering and local-only policy behavior.
Support-export tests scan the complete archive for nested credentials, private
paths and filenames and verify existing archives are not overwritten.

The real binary journey runs the production CLI and PTY TUI. It exercises a
healthy service with an offline peer, relay quota/healthy distinctions, keyboard
doctor cancel/refresh, three local-only doctor calls with no additional service
requests, reviewed apply/replay/restart, retained device identity/file bytes and
dedicated-daemon cleanup. It records the service/relay/direct/UDP probe codes;
`QUOTA_EXCEEDED` is a truthful bounded service result and is not relabeled as a
successful relay connection. No test reports a NAT or firewall classification.

Commands and outputs are retained under
[`docs/evidence/wan-w12-20261006/`](evidence/wan-w12-20261006/), including
initial compile failures, cancellation/probe repairs, support redaction repairs,
focused pass logs, the owning race run and the final binary/PTY run. The
disposable fixtures do not establish W13 hosted operator readiness, W16 physical
WAN/CGNAT behavior or inherited T13 login/logout/boot acceptance.


## W11 follow-up production campaigns

[Follow-up evidence](evidence/wan-w11-followup-20261006/summary.md) implements
reviewed timing and pre-download FIFO bandwidth reservation. Exact legacy policy
bytes have an independent JSON oracle so adding zero timing cannot change existing
durable mutation fingerprints. Control/CLI checks prove reviewed ranges, exact
intent, changed-input refusal, durable replay, restart activation and restoration.

`TestWANW11ActualLargeSmallPeerBandwidthAcrossRoutes` uses three real trusted
replicas plus the signed service to measure simultaneous 16 MiB and continuous
small-version progress across encrypted WSS, pinned TCP and HTTP3. The limiter
budgets scheduled pull payload attempts before network IO, including retries and
fallbacks, with a one-second initial burst. Serving quotas and protocol overhead
remain separate. Report original versions/heads/authors/manifests/bytes, actual
route, first/complete progress, combined sampled heap/FD/goroutine peaks and CPU.
A 25 ms sampling interval can miss shorter peaks; FD totals include non-socket FDs.

`TestWANW11WholeDaemonRoamingAndMixedProgress` ordinarily skips. Run the documented
marked `wan_daemon_roaming_namespace_test.py` runner under a newly owned remapped
user/network namespace; it validates marker/root/namespace before mutations.
Actual production daemon lifecycle, scheduler, owner control, network watchers and
native transports run in child test binaries. The campaign blocks peer routes,
recovers WSS→HTTP3, changes an address/default route during a 16 MiB transfer,
recovers relay and reprobes direct. A typed unexpected EOF after committed receipt
send exercises loss of caller success; marker checks guard the hook. It is a
boundary injection, not physical removal of one HTTP receipt response. Exact
heads/hashes/authors/keys/pins and durable receipt oracles remain required.

The runner can apply real Linux netem packet delay/loss to peer/service traffic,
exempting loopback owner control, and can use production default timing. Execute
and record locally under race and on actual Pi hardware; physical topology and
operated-default service claims remain W13/W16. Daemon process samples every
250 ms are not guaranteed instantaneous maxima. Thirty-two competing delayed
DNS/relay peers separately test resolver/admission/cancel/join bounds. No failure
or skipped ordinary namespace test receives packet acceptance credit.


## W15 integrated WAN failure campaign

The [disposable runner instructions](../scripts/validation/WAN_FAILURES.md)
define exact commands, privilege prerequisites, root/marker/process/namespace
refusals and the distinction between emulator topologies and native sockets.
W15 adds transfer-boundary/model checks across QUIC→TCP with aggressive GC,
post-rename publication recovery, conflict preservation and cached-route denial
after retirement. Whole-daemon fixtures combine chunk/receipt SIGKILL with
relay recovery, address/default-route changes and service outage/restart.
Fault hooks are test-injected Go callbacks; release operation exposes no fault
endpoint or environment switch. The ordinary aggregate skips guarded namespace
cases and never mutates host networking.

The [W15 tracker](implementation/wan-status.md#w15--integrated-failures-security-and-resources)
owns executed campaign status and remaining limitations. WG6, physical WAN/N10,
inherited T13 technical checks and deferred P17 owner use/explanation retain
separate acceptance requirements.
