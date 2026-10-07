# Implementation status

Active expansion: the owner-selected [native WAN plan](../orbit-wan-implementation-plan.md)
and [W tracker](wan-status.md), 2026-10-05. W00–W17 are recorded there; the
[combined W17 release record](../evidence/wan-w17-20261007/summary.md) links all
WAN and inherited T13 evidence. Existing entries below retain their dated
P/O/T evidence and limitations, including deferred P17 owner observations.

## Owner-directed delivery amendment — 2026-10-04

The owner deferred personal-use observations and the unaided learning/explanation
review until after project delivery. They are follow-up activities, not engineering
completion gates. Finish automated validation, product polish, measured resource
behavior and evidence-backed portfolio artifacts now. Retain historical P17 pilot
records and data; do not claim automated campaigns establish personal adoption or
owner understanding. Required technical checks and declared engine guarantees
remain in force; report unavailable network/host conditions explicitly.


The interface baseline is the owner-approved terminal redesign. Retain the
[terminal plan](../orbit-terminal-implementation-plan.md) and
[terminal tracker](terminal-status.md), including incomplete T13 technical checks.
The W plan above owns the active expansion. P/O evidence below remains historical;
P17 personal use/explanation is deferred until after delivery.

Updated: 2026-10-04. P00–P16 have implementation and scoped validation evidence.
Packaged laptop/Pi/VPS demonstrations pass; current measurements passed 45/45.
P17's technical release record is reconciled with T13 and complete under the
owner's delivery amendment. Terminal-specific native release conditions remain
in T13. Personal use and
explanation are deferred follow-up evidence, under the owner-directed amendment.
Earlier blanket completion and estimated benchmark claims are withdrawn.
Earlier packet entries retain the limitations of their original dated checks;
the release report records subsequent validation.

Active product work was the owner-selected **Orbit revamp**. Its
[plan](../orbit-implementation-plan.md) and [tracker](orbit-status.md) are
complete across packets O00–O13 with full scoped evidence in `docs/evidence/orbit-o13/`.
All 10 minimum product scenarios, UI flows, and resource bounds are verified.

| Packet | State | Dependencies | Evidence |
| --- | --- | --- | --- |
| P00 Skeleton/toolchain | complete | none | [evidence](../evidence/p00-20260920/summary.md); [hosted CI](https://github.com/calebhabesh/file-sync/actions/runs/35559982219) |
| P01 Design experiments | complete | P00 | [evidence](../evidence/p01-20260921/summary.md); [decisions](../design-gates.md) |
| P02 Model/history | complete | P01 relevant gates | [evidence](../evidence/p02-20260921/summary.md) |
| P03 Durable repository | complete | P00, P02 | [evidence](../evidence/p03-20260921/summary.md) |
| P04 Workspace/publication | complete | P01 D1/D5, P03 | [evidence](../evidence/p04-20260921/summary.md) |
| P05 Peer/wire layer | complete | P02, P03 | [evidence](../evidence/p05-20260921/summary.md) |
| P06 Two-peer transfer | complete | P04, P05 | [evidence](../evidence/p06-20260923/summary.md) |
| P07 Reconciliation | complete | P06 | [evidence](../evidence/p07-20260923/summary.md) |
| P08 Resolution/restore | complete | P07 | [evidence](../evidence/p08-20260923/summary.md) |
| P09 Membership/forwarding | complete | P08, D3 | [evidence](../evidence/p09-20260923/summary.md) |
| P10 Retention/GC | complete | P09, D4 | [evidence](../evidence/p10-20260923/summary.md) |
| P11 Integrity/repair | complete | P10 | [evidence](../evidence/p11-20260923/summary.md) |
| P12 Continuous operation | complete | P11 | [evidence](../evidence/p12-20260923/summary.md) |
| P13 Operator controls | complete | P12 | [evidence](../evidence/p13-20260923/summary.md) |
| P14 Web interface | complete | P13 | [evidence](../evidence/p14-20260923/summary.md) |
| P15 Packaging/lifecycle | complete | P14 | [evidence](../evidence/p15-20260923/summary.md) |
| P16 Fault campaign | complete | P15 | [clean reproduction](../evidence/release-20261001/release-candidate/reproduction.json); [invariant/scenario map](../evidence/release-20261001/invariant-map.md) |
| P17 Pilot/release evidence | complete | P16 | [2026-10-04 reconciliation](#p17-delivery-reconciliation--2026-10-04); [historical release](../evidence/release-20261001/summary.md); owner review deferred |

Packet definitions: [foundations](01-foundations.md), [replication](02-replication.md), [operations](03-operations.md), [delivery](04-delivery.md).

## Engineering gate outcomes

D1 publication races; D2 working basis/same-author lineage; D3 membership
retirement; D4 safe reference cleanup; and D5 directory/bootstrap projection
were closed in P01. See [outcomes](../design-gates.md) and
[architecture](../architecture.md#design-gates). Later packets retain the
listed production implementation and fault-evidence obligations.

## Orbit revamp status

Orbit personal file manager revamp is complete across packets O00 through O13.
See [Orbit status](orbit-status.md) and [O13 evidence](../evidence/orbit-o13/summary.md).

## Deferred P17 owner-use evidence

Deferred until after delivery under the owner-directed 2026-10-04 amendment.
Preserve the prepared pilot and historical records. Future personal use of
`/home/caleb2002/FileSyncPilot-20261001/data`: normal edits, offline/reconnect
and ordinary restart, with actual start/end times and observed results.
The native campaign now uses `ssh laptop`; hardware access is resolved.
The owner's comprehensive explanation/review follows delivery. The
[pilot handoff](../evidence/release-20261001/personal-pilot/handoff.md) records
the persistent services and workstation gateway dependency. Continue from the
[current release report](../evidence/release-20261001/summary.md).

## P17 delivery reconciliation — 2026-10-04

State: **complete under the owner's delivery amendment**. This closes engineering
delivery, not personal adoption or owner understanding. The original dated P17
entry below remains historical, including its then-in-progress state and withdrawn
estimated measurements. Personal use and comprehensive review are deferred.

All three actual hosts participate in the
[current packaged engine campaign](../evidence/terminal-t13-20261004/native-engine-final-candidate/three-host.json):
normal sync, independent offline edits, late-arrival reviewed resolution, stale
refusal, original-author forwarding with no A/B overlap, durable interrupted
transfer reuse, historical restore and restart integrity passed. This uses
manual membership fixtures and SSH relays; ordinary two-installation LAN
onboarding has [separate evidence](../evidence/terminal-t13-20261004/lan-final-candidate/terminal-native.json).
Neither is personal-use evidence. Existing pilot folders/services were preserved.

The [historical 45-run benchmark matrix](../evidence/release-20261001/measured-results.md)
retains raw fair-baseline positive and negative results. Current terminal resource
measurements add 32 CLI observations over repeated 1,024-file and 8/32-MiB merge
fixtures. The [clean reproduction](../evidence/terminal-t13-20261004/reproduction-final-candidate/clean-release/reproduction.json)
passed check, uncached full race, demo, 16 VM reset cases, five storage cases,
checksums and byte-identical repeated packages. Source provenance is an isolated
snapshot of the development tree, not a clean original branch. Current README,
demo, architecture/case study and resume drafts link their supporting evidence.

T13 remains in progress for its additional existing-Tailscale, ordinary native
three-host onboarding and native login/logout/boot acceptance conditions. They do
not reopen P17's separately evidenced engine, measurement and portfolio work.
The [T13 report](../evidence/terminal-t13-20261004/summary.md) names those remaining
technical conditions and the resumable next steps.

## P00 — Reproducible project skeleton

Packet: P00

State: `complete`.

Prerequisites and design gates checked: no dependencies or design gates. Read
scope, glossary, architecture, operations, persistence, and verification.

Changed files: `go.mod`, `go.sum`, `Makefile`, `.github/workflows/ci.yml`,
`cmd/filesync`, `internal/{app,config,repository,state,testkit}`,
`tests/{integration,faults}`, `docs/dependencies.md`, P00 evidence, README and
this tracker.

Invariant IDs: I20 (incompatible configuration/schema refusal and migration
safety surface). P00 also establishes harness prerequisites for later I07/I20
fault evidence; it does not claim those invariants are fully verified.

Commands and actual results: `go mod verify` passed; `make check` passed on the
development tree and a fresh clone of a temporary committed snapshot;
`make test-race` passed; `actionlint` v1.7.7 returned no diagnostics; both
amd64 and arm64 static binaries built. The arm64 binary ran `version` and the
SQLite-backed `init` command through explicit QEMU 7.2 user-mode emulation.
The resulting database reported `user_version=1`; state/config/database modes
were `0700/0600/0600`. Hosted CI run
[35559982219](https://github.com/calebhabesh/file-sync/actions/runs/35559982219)
passed `make check` and `make test-race` on native GitHub-hosted amd64 and
arm64 runners. Full commands and outputs are in
[commands](../evidence/p00-20260920/commands.md).

Evidence paths: [summary](../evidence/p00-20260920/summary.md),
[commands](../evidence/p00-20260920/commands.md), tests in `internal/*` and
`tests/integration`, dependency decision in
[dependencies](../dependencies.md).

Unexecuted checks / limitations: no Raspberry Pi execution has occurred;
GitHub's native arm64 runner and local QEMU do not establish Raspberry Pi
filesystem, storage or service behavior. P03 still owns full SQLite
connection, WAL/checkpoint and crash-boundary verification. P15 owns final
dependency notices and packaging. No fault injection was run.

Owner explanation notes: the SQLite driver choice determines whether target C
toolchains/libc coupling enter packaging. A cross-compiled binary alone proves
neither execution nor target filesystem/storage behavior; QEMU narrows only
the instruction/runtime gap. See the evidence summary for the full answer.

Next eligible work: P01 architecture experiments and contract freeze.

## P01 — Architecture experiments and contract freeze

Packet: P01

State: `complete`.

Prerequisites and design gates checked: P00 complete. Read scope, glossary,
architecture, protocol, persistence, operations, verification, and the P01
packet. D1–D5 now have executable counterexamples/outcomes in
`tests/designgates` and written decisions in
[design gates](../design-gates.md).

Changed files: `Makefile`; `tests/designgates/*` and golden fixtures;
`docs/design-gates.md`; protocol, persistence and architecture specifications;
P01 evidence; implementation plan and this tracker.

Invariant IDs: I04, I07–I12 and I15 were exercised at their P01 design seams.
D1 covers publication preservation/recovery and descriptor-rooted path safety
(I07/I09); D2 working basis (I04); D3 conservative retirement admission
(I08/I15); D4 reference/GC safety (I07/I10); D5 bootstrap, root and structural
rules (I09/I11/I12). These experiments do not claim the later production
modules fully verify those invariants.

Commands and actual results: the design-gate suite passed on the development
host's ext4 filesystem with
`TMPDIR=<repo>/.tmp/p01 go test -count=1 -v
./tests/designgates`. `make check`, `make test-race` and `make test-faults`
all passed after the final changes. Full commands and observed traces are in
[commands](../evidence/p01-20260921/commands.md).

Evidence paths: [summary](../evidence/p01-20260921/summary.md),
[results](../evidence/p01-20260921/results.json),
[decisions](../design-gates.md), executable tests in `tests/designgates`, and
canonical membership/retirement fixtures in `tests/designgates/testdata`.

Unexecuted checks / limitations: no hosted CI, arm64 design-gate execution,
SIGKILL publication harness, VM abrupt reset, or alternate Linux filesystem
was run for P01. D1 modeled stops at filesystem transition boundaries rather
than claiming power-loss durability. D2 is not the independent P02 oracle; D4
is not the durable P03/P10 implementation. Production packets retain those
acceptance criteria.

Owner explanation notes: a pre-rename stat cannot eliminate publication races
because an editor can install a new inode after the stat and before the
publisher's rename. A plain rename then removes the editor inode's only name.
`RENAME_EXCHANGE` instead leaves the actual displaced inode named for
recovery, although it still cannot promise capture of future writes through a
descriptor held across replacement.

Next eligible work: P02 independent causal model and production history
module.

## P02 — Independent causal model and production history module

Packet: P02

State: `complete`.

Prerequisites and design gates checked: P01 complete. Read scope, glossary,
architecture, protocol, verification, operations limits, the P02 packet and
the D2/D3/D5 outcomes. The production implementation is in
`internal/history`; the explicit-parent reachability oracle is separately
implemented in `model` and does not import production causal algorithms.

Changed files: `internal/history/*`, `internal/protocol/*`, `model/*`,
`schemas/fixtures/*`, `Makefile`, protocol specification, P02 evidence,
implementation plan and this tracker.

Invariant IDs: I01–I04, I12, I15 and I16. Tests cover immutable duplicate
identity handling, delivery-order-independent heads, concurrent file/delete
and equal-byte histories, working-basis planning and same-author stale-basis
blocking, structural conflicts, conservative retired-author admission, and
reviewed-head/token checks.

Commands and actual results: `make test-model`, `make check`,
`make test-race`, `make test-faults` and `git diff --check` passed. Model
coverage exhaustively enumerated every length 1–4 author schedule for two and
three actors (150 histories total) and ran 128-event schedules with seeds 2,
17, 101 and 20260921. A test-only scalar-winner comparison mutant disagreed
with the reachability oracle as required; the production rule was not left
mutated. Full commands and results are in
[commands](../evidence/p02-20260921/commands.md).

Evidence paths: [summary](../evidence/p02-20260921/summary.md),
[results](../evidence/p02-20260921/results.json), production tests in
`internal/history` and `internal/protocol`, independent model tests in
`model`, and golden domain/wire fixtures in `schemas/fixtures`.

Unexecuted checks / limitations: no hosted CI or native arm64 test execution
was run for P02. The ordinary checks cross-built arm64 but did not execute it.
The finite bounds and fixed seeds are evidence for those schedules, not a
general causal proof. P03 still owns SQL counter allocation, content readiness
and durable version admission; P05 owns complete endpoint schemas and wire
error fixtures; P07/P08 own reconciliation and persisted resolution/restore.

Owner explanation notes: receiving a version records knowledge, not review.
An ordinary edit can causally claim only the persisted working basis it was
actually produced from (plus required local same-author lineage). Treating all
received heads as parents would falsely resolve unseen contents and could
erase a real conflict.

Next eligible work: P03 durable repository and immutable content.

## P03 — Durable repository and immutable content

Packet: P03

State: `complete`.

Prerequisites and design gates checked: P00 and P02 complete. Read scope,
glossary, architecture, protocol, persistence, verification, the P03 packet,
and the P01 D4 reference/pin outcome.

Changed files: `internal/repository/{repository,objects,versions}.go`,
repository unit tests, `tests/faults/p03_boundaries_test.go`, `Makefile`,
persistence recovery documentation, P03 evidence, implementation plan and
this tracker.

Invariant IDs: I01, I05–I08 and I20. Tests cover immutable duplicate IDs and
objects, streamed fixed-size manifests, whole-file verification, atomic local
counter/version creation, pending remote metadata, readiness/receipt gating,
durable pins/reservations, budget rejection, consistent backup, orphan
classification, ENOSPC injection, and subprocess SIGKILL/restart at each P03
object/version/readiness boundary. A real competing SQLite writer and an
injected checkpoint failure both left remote readiness pending.

Commands and actual results: `make test-model`, `make check`, `make
test-race`, `make test-faults`, `go test -count=1 ./...`, and `git diff
--check` passed. The fault suite killed disposable helper processes at
`object.flushed`, `object.installed`, `object.recorded`, `sql.version.before_commit`,
`sql.version.after_commit`, `sql.readiness.before_commit`, and
`sql.readiness.after_commit`; every restart assertion passed. Full commands
and observed outcomes are in [commands](../evidence/p03-20260921/commands.md).

Evidence paths: [summary](../evidence/p03-20260921/summary.md),
[results](../evidence/p03-20260921/results.json), repository tests in
`internal/repository`, and process fault tests in `tests/faults`.

Unexecuted checks / limitations: no hosted CI, native arm64 execution,
abrupt VM reset, physical power-loss, forced real-filesystem ENOSPC, or
alternate filesystem was run. The arm64 binary was cross-built only. SIGKILL
supports process-recovery claims under the recorded ext4 environment, not
power-loss durability. P04 still owns working-folder publication journals;
P10 owns object deletion/GC; P11 owns quarantine and repair; P15 owns the full
operator backup/migration workflow.

Owner explanation notes: bytes in an incoming file are untrusted and may be
partial; a flushed and atomically named chunk proves one object, while
whole-file verification proves ordered manifest assembly. Only the final SQL
transaction binds verified objects, immutable ancestry, counter allocation,
references and readiness, so only that milestone permits `saved locally` or
a durable receipt.

Next eligible work: P04 workspace capture and journaled publication.

## P04 — Workspace capture and journaled publication

Packet: P04

State: `complete`.

Prerequisites and design gates checked: P01 D1/D5 and P03 complete. Read
scope, glossary, architecture, protocol, persistence, operations, verification
and the P04 packet. The implementation uses the D1 exchange/openat2 contract
and D5 bootstrap/scaffold rules.

Changed files: schema-v3 migration and workspace records in
`internal/repository`, `internal/workspace/*`, local CLI commands in
`cmd/filesync` and `internal/app`, P04 unit/integration/process-fault tests,
persistence contract, P04 evidence, implementation plan and this tracker.

Invariant IDs: I04, I06, I07, I09, I11, I12 and I17. Tests cover working-basis
capture without applying unseen heads; rehashed ready-version staging; all
named journal transitions; descriptor-rooted path handling; root replacement,
marker mismatch and incomplete scans; file/directory structural blocks;
deletion preview and stale generation; and no authored echo after apply/restart.

Commands and actual results: `make test-model`, `make check`, `make test-race`,
`make test-faults`, `go test -count=1 ./...` and `git diff --check` passed.
`make check` built amd64 and cross-built arm64 binaries. The P04 process
harness passed 30 SIGKILL/restart cases across existing-file exchange,
new-file no-replace, parent creation, directory creation and tombstone
removal. See [commands](../evidence/p04-20260921/commands.md).

Evidence paths: [summary](../evidence/p04-20260921/summary.md),
[results](../evidence/p04-20260921/results.json), unit tests in
`internal/workspace`, process fault tests in `tests/faults`, and CLI exercise
in `tests/integration`.

Unexecuted checks / limitations: no hosted CI, native arm64 run, abrupt VM
reset, physical power loss, real-filesystem ENOSPC, alternate filesystem or
nested-mount fault was run for P04. An injected ENOSPC boundary and budget
refusal were tested, but do not establish real-device behavior. Long-lived
descriptor writers can continue changing the retained recovery inode; no
finite close-detection or every-write claim is made. A parent-path swap after
descriptor acquisition can leave a preserved variant under a moved directory
and block application; it cannot escape the verified root descriptor. P10
owns reviewed recovery-copy cleanup and final space reclamation; P12/P13 own
persistent scheduling/configuration and structured control operations.

Owner explanation notes: equivalent causal head sets describe the same
recorded history, not necessarily the same visible working tree. One replica
may retain a local conflicting copy or block a structural publication while
another shows a different ready head. Making working trees identical before
review would require silently choosing or overwriting a candidate.

Next eligible work: P05 peer/wire layer.

## P05 — Peer identity, pairing and bounded wire layer

Packet: P05

State: `complete`.

Prerequisites and design gates checked: P02 and P03 complete. Read scope,
glossary, architecture, protocol, persistence, operations, verification and
the P05 packet. The production membership encoder was checked against the P01
canonical membership fixture.

Changed files: schema-v4 peer snapshots and membership operations in
`internal/repository`; strict endpoint encodings and canonical membership in
`internal/protocol`; identity, pinned TLS client/server, bounded handlers and
real-listener tests in `internal/replication`; identity/pair/listener CLI and
app wiring; peer schema/golden fixtures; protocol, persistence, operations and
verification specifications; P05 evidence and this tracker.

Invariant IDs: I01, I09 and I20. Tests cover immutable envelope fetch;
persistent key identity; ordinary certificate verification plus exact SPKI
pinning; active folder membership on every request; unpaired, wrong-key,
revoked and wrong-folder rejection; explicit protocol/membership errors;
manifest-scoped chunk authorization; closed/duplicate-free bounded JSON;
stable and expired snapshot behavior; maximum pages/batches and body limits.

Commands and actual results: `make test-model`, `make check`, `make test-race`,
`make test-faults`, `go test -count=1 ./...`, `git diff --check`, and P05
evidence JSON parsing all passed. `make check` built amd64 and cross-built
arm64 binaries. Real loopback TCP/TLS tests paged 129 versions across a stable
snapshot and ran the authentication/authorization matrix. See
[commands](../evidence/p05-20260921/commands.md).

Evidence paths: [summary](../evidence/p05-20260921/summary.md),
[results](../evidence/p05-20260921/results.json), wire tests in
`internal/replication`, canonical/golden tests in `internal/protocol`, CLI
exercise in `tests/integration`, and frozen examples under `schemas/fixtures`.

Unexecuted checks / limitations: no hosted CI, native arm64 execution,
physical multi-host networking, internet-exposed listener test, certificate
expiry/rotation workflow, large concurrent-connection soak or network
partition/proxy campaign was run. The arm64 binary was cross-built only. P05
does not implement transfer scheduling, receive progress, durable receipts,
status exchange, reconciliation or publication; P06/P07 own those behaviors.
The initial `pair-approve` command creates/replays revision 1 for a two-device
folder; full enrollment preview and membership lifecycle remain P09.

Owner explanation notes: mTLS authenticates a device key, not authority over
every folder or object held by that process. Membership differs by folder and
revision, and identical chunks can occur in different folders. Rechecking the
exact folder membership and requiring a manifest version plus chunk position
prevents a trusted connection from becoming a global digest/content oracle.

Next eligible work: P06 first two-peer verified CLI transfer.

## P06 — First two-peer verified CLI transfer

Packet: P06

State: `complete`.

Prerequisites and design gates checked: P04 and P05 complete. Read scope,
glossary, architecture, protocol, persistence, operations, verification and
the P06 packet.

Changed files: schema-v5 migration, transfer progress, content pins, and peer
status in `internal/repository`; transfer orchestration, chunk streaming,
receipts, and peer status in `internal/replication`; CLI `sync` and `status`
commands in `cmd/filesync`; `transfer_test.go` in `internal/replication`,
`p06_cli_test.go` in `tests/integration`, and `p06_boundaries_test.go` in
`tests/faults`; protocol and persistence specifications; P06 evidence and
this tracker.

Invariant IDs: I01, I05–I07, I09 and I17. Tests cover empty (0 bytes), small,
multi-chunk and repeated-chunk file transfers arriving byte-for-byte;
chunk-level hash and whole-file manifest verification; corruption rejection
and retry exhaustion; interrupted transfer resumption reusing already verified
chunks without retransmission; lost receipt recovery and idempotent replay;
peer progress and status distinguishing stored from applied; independent local
workspace application if the sender disappears after storing chunks; and two-agent
CLI transfer over real TLS with isolated roots.

Commands and actual results: `make test-model`, `make check`, `make test-race`,
`make test-faults`, `go test -count=1 ./...`, `git diff --check`, and P06
evidence JSON validation all passed. `make check` built amd64 and cross-built
arm64 binaries. The process fault suite killed disposable helper processes at
`transfer.chunk.verified`, `transfer.readiness.before`, `transfer.readiness.after`,
`transfer.receipt.before_send`, and `transfer.receipt.after_send`; every restart
assertion passed and resumed transfers completed without re-fetching verified
chunks. See [commands](../evidence/p06-20260923/commands.md).

Evidence paths: [summary](../evidence/p06-20260923/summary.md),
[results](../evidence/p06-20260923/results.json), transfer tests in
`internal/replication`, two-agent CLI test in `tests/integration`, and
process fault tests in `tests/faults`.

Unexecuted checks / limitations: no hosted CI, native arm64 execution,
physical multi-host networking, internet-exposed listener test, high-concurrency
transfer soak, or throttled network partition/proxy campaign was run for P06.
The arm64 binary was cross-built only. Automatic publication in P06 conservatively
applies only single unapplied heads; multi-head and conflicting history
reconciliation remains P07.

Owner explanation notes: if the sender disappears after the receiver has stored
all chunks but before the receiver applies the file, no content or history is
lost. The receiver's database transaction has already committed `content_state = 'ready'`,
all chunks are verified and permanently stored in the repository, and the transfer
is marked complete. Because all required chunks and metadata are already present
locally, the receiver does not require the sender to be online to apply the file.
A subsequent publication or workspace apply reads the verified chunks directly
from local storage. If the sender reconnects later, the receiver replays the
durable receipt during the next sync session.

Next eligible work: P07 bidirectional reconciliation and conflict projection.

## P07 — Bidirectional reconciliation and conflict projection

Packet: P07

State: `complete`.

Prerequisites and design gates checked: P06 complete. Read scope, glossary,
architecture, protocol, persistence, operations, verification and the P07
packet.

Changed files: conflict calculation and structural conflict queries in
`internal/repository/conflicts.go`; local author/counter tracking in
`internal/repository/versions.go`; scaffold pruning and descendant queries in
`internal/repository/workspace.go` and `internal/workspace/workspace.go`;
structural conflict isolation in `internal/replication/transfer.go`; CLI
`conflicts` command in `cmd/filesync/main.go`; unit and integration tests in
`internal/repository/conflicts_test.go`, `internal/workspace/scaffold_test.go`,
`internal/replication/reconciliation_test.go`, and `tests/integration/p07_cli_test.go`;
protocol and verification status updates; P07 evidence and this tracker.

Invariant IDs: I01–I04, I11, I12 and I17. Tests cover offline edit/edit
concurrency under both delivery orders matching the independent causal DAG;
offline edit/delete and repeated-delete tombstone propagation without timestamp
heuristics; equal-byte conflict preservation without erasing ancestry;
executable-only changes; protected fallback content while remote heads are
pending; unreviewed received heads never advancing an active editor's working
basis (I04); structural conflict isolation (parent delete vs child create and
file/directory collisions) without recursive deletion (I12); directory
projection scaffold pruning when child tombstones are applied while preserving
untracked files or siblings; zero extra versions created on rescan or restart
(I17); and `filesync conflicts` CLI command in text and JSON modes.

Commands and actual results: `make test-model`, `make check`, `make test-race`,
`make test-faults`, `go test -count=1 ./...`, `git diff --check`, and P07
evidence JSON validation all passed. `make check` built amd64 and cross-built
arm64 binaries. The reconciliation suite passed all offline concurrency and
delivery order scenarios. The P07 CLI integration test verified bidirectional
sync between two real mTLS processes, conflict reporting, working byte
preservation, and zero spurious versions on rescan. See
[commands](../evidence/p07-20260923/commands.md).

Evidence paths: [summary](../evidence/p07-20260923/summary.md),
[results](../evidence/p07-20260923/results.json), reconciliation tests in
`internal/replication`, repository conflict tests in `internal/repository`,
scaffold tests in `internal/workspace`, and two-agent CLI test in
`tests/integration`.

Unexecuted checks / limitations: no hosted CI, native arm64 execution,
physical multi-host networking, internet-exposed listener testing, or
network partition/proxy throttled-link campaigns were run for P07. The arm64
binary was cross-built only. Interactive resolution operations (`select`,
`keep-copies`, manual merge, and restore) remain P08.

Owner explanation notes: if an authoring node joins all received remote heads into
the parent vector of an ordinary local edit, the newly created version causally
dominates those remote heads ($V_{new} > p$). Other peers receiving this version
will see it as strictly newer, treating concurrent remote edits as superseded
history. Consequently, the remote peer's work is silently discarded and overwritten
without flagging a conflict, preserving divergent working bytes, or recording an
explicit reviewed resolution. In reality, the local user never reviewed the remote
changes. Upholding invariant I04 requires that local captures only advance from
the persisted local working basis and latest local-author event (D2, I04), leaving
received remote branches as concurrent heads until reviewed and resolved.

Next eligible work: P08 reviewed resolution, restore and safe control replay.

## P08 — Reviewed resolution, restore and safe control replay

Packet: P08

State: `complete`.

Prerequisites and design gates checked: P07 complete. Read scope, glossary,
protocol (section 5), persistence, verification, and the P08 packet definition.
Reviewed resolution, keep-copies, export, manual-merge, and historical restore
are implemented as shared control operations with explicit reviewed heads,
stale-view tokens, idempotency records, and safe partial multi-path resumption.

Changed files: `cmd/filesync/main.go`; `internal/control/*`;
`internal/history/history.go`; `internal/history/history_test.go`;
`internal/repository/conflicts.go`; `internal/repository/resolution.go`;
`internal/repository/versions.go`; `internal/workspace/workspace.go`;
`internal/replication/transfer.go`; `tests/integration/p08_cli_test.go`;
protocol and verification status updates; P08 evidence and this tracker.

Invariant IDs: I03, I04, I16, I19. Tests cover idempotent control replay returning
cached results with zero duplicate versions; unseen versions arriving after
resolution remaining concurrent (I03); pre-commit changed heads producing
`STALE_VIEW` responses; destination name collisions preserving existing files;
partial multi-path operations (`keep-copies`) resuming without duplicate copies;
restored executable status following the selected historical version; content
states (`ready`, `pending`, `unavailable`, `expired`) exposed accurately; and
unified CLI control commands (`filesync resolve select|merge|keep-copies`,
`filesync restore`, `filesync export`, `filesync history`).

Commands and actual results: `make test-model`, `make check`, `make test-race`,
`make test-faults`, `go test -count=1 ./...`, `git diff --check`, and P08
evidence JSON validation all passed. `make check` built amd64 and cross-built
arm64 binaries. The control unit tests passed all idempotency, replay, conflict,
stale-view, and collision scenarios. The P08 CLI integration test verified
bidirectional sync between two real mTLS processes, conflict resolution via
`select`, replay safety, rescan authoring zero extra versions, convergence across
peers, history inspection, historical restore with preview, and `keep-copies`
with replay. See [commands](../evidence/p08-20260923/commands.md).

Evidence paths: [summary](../evidence/p08-20260923/summary.md),
[results](../evidence/p08-20260923/results.json), control tests in
`internal/control/control_test.go`, and two-agent CLI test in
`tests/integration/p08_cli_test.go`.

Unexecuted checks / limitations: no hosted CI, native arm64 execution,
physical multi-host networking, internet-exposed listener testing, or
network partition/proxy throttled-link campaigns were run for P08. The arm64
binary was cross-built only. Three-peer forwarding and membership lifecycle
remain P09.

Owner explanation notes: in causal version tracking (version vectors / DAGs),
an event's causal vector defines its exact temporal position and ancestry relative
to all other versions in the distributed system ($A \le B \iff \forall k, V_A[k] \le V_B[k]$).
If restoring yesterday's file restored yesterday's causal vector $V_{\text{yesterday}}$,
any events authored after yesterday (e.g. today's edits, or a deletion tombstone
$V_{\text{today}} > V_{\text{yesterday}}$) would causally dominate yesterday's vector.
When replicas exchange history, standard dominance checks would observe
$V_{\text{today}} > V_{\text{yesterday}}$ and treat the restored version as an
already-superseded, obsolete historical event. The restoration would be instantly
and silently erased by the very tombstone or edit the user intended to revert.
Furthermore, other peers may have authored concurrent work in parallel with today's
state; restoring an old vector breaks the causal relationship with those concurrent
branches. Therefore, restoration is fundamentally an action in the present. While it
adopts yesterday's content bytes and metadata (provenance), it must author a new
causal version whose parents are the currently reviewed heads of the path, extending
current causal history rather than attempting to roll back time.

Next eligible work: P09 three-peer forwarding and membership lifecycle.

## P09 — Three-peer forwarding and membership lifecycle

Packet: P09

State: `complete`.

Prerequisites and design gates checked: P08 complete. Gate D3 verified against
canonical golden hex fixture `tests/designgates/testdata/retirement-snapshot-v1.hex`.
Read scope, protocol, persistence, operations, verification, and packet specifications.

Changed files: `internal/protocol/membership.go`, `internal/protocol/membership_test.go`,
`internal/repository/repository.go` (schema v6), `internal/repository/peers.go`,
`internal/repository/versions.go`, `internal/repository/transfers.go`,
`internal/repository/membership_test.go`, `internal/control/types.go`,
`internal/control/control.go`, `internal/control/control_test.go`,
`internal/workspace/workspace.go`, `cmd/filesync/main.go`,
`tests/integration/p09_cli_test.go`, evidence artifacts in `docs/evidence/p09-20260923/*`.

Invariant IDs: I02, I08, I14, I15. Tests cover three-peer forwarding ($A \leftrightarrow B \leftrightarrow C$)
preserving author identities, counters, and vectors byte-for-byte (I08); direct vs. indirect progress
labeling; offline three-way concurrent edits and late-arrival resolution converging to identical heads (I02);
canonical retirement snapshot artifact creation and verification (Gate D3); rejection of unknown retired-origin
events; access termination upon peer revocation or retirement (I14); stale revision sync rejection;
divergent existing-folder enrollment preview and bootstrap without spurious tombstones (I15); and
metadata-loss reinstall recovery under a fresh device identity.

Commands and actual results: `make test-model`, `make check`, `make test-race`, `make test-faults`,
`go test -count=1 ./...`, `git diff --check`, and P09 evidence JSON validation all passed.
`make check` passed fmt-check, vet, unit, model, integration (including P06, P07, P08, P09 CLI tests),
amd64 build, and arm64 cross-build. `make test-race` completed with 0 race warnings across all packages.
`make test-faults` passed all D1–D5 design gate assertions and P03/P04/P06 SIGKILL crash restarts. The three-agent CLI
integration test (`TestP09ThreePeerForwardingAndMembershipLifecycle`) verified end-to-end multi-hop
forwarding, three-way resolution late arrival, retirement execution and idempotent replay, survivor
membership export/import/preview, access termination, divergent existing-folder enrollment, and
reinstall recovery. See [commands](../evidence/p09-20260923/commands.md).

Evidence paths: [summary](../evidence/p09-20260923/summary.md),
[results](../evidence/p09-20260923/results.json), [manifest](../evidence/p09-20260923/manifest.json),
control tests in `internal/control/control_test.go`, repository tests in `internal/repository/membership_test.go`,
protocol tests in `internal/protocol/membership_test.go`, and three-agent CLI integration test in
`tests/integration/p09_cli_test.go`.

Unexecuted checks / limitations: no hosted CI, native arm64 execution, physical multi-host networking,
or internet-exposed listener testing was run for P09. Multi-process forwarding was verified across three
independent local processes communicating over loopback mTLS 1.3. Actual three-host deployment evidence
follows in P17. Retention and garbage collection remain P10.

Owner explanation notes:
1. *Why peer retirement changes cleanup assumptions:*
In ordinary active operation, distributed garbage collection (GC) and historical tombstone pruning are
strictly constrained: any active peer might still be partitioned, holding old working trees or causal
references that require historical ancestors, tombstones, or chunks to prove dominance and complete
synchronization. Counterparts cannot distinguish between a delayed legitimate sync and an obsolete branch,
forcing replicas to retain speculative tombstones and maintain open transfer availability.
When a peer is retired, this open-ended assumption changes fundamentally:
- Bounded historical universe: The accepted causal history authored by the retiree is permanently frozen
  and sealed into a canonical retirement snapshot approved by the surviving members. No new versions will
  ever be admitted from that author identity (any subsequent event from that identity is rejected/quarantined
  as unauthorized). Replicas now possess an exhaustive, immutable list of every valid version that author
  will ever contribute.
- Payload retention decoupling: While the causal metadata (envelope ID, counter, vector entries) must be
  retained indefinitely to preserve vector shapes and prevent identity renumbering, the heavy content payloads
  (chunks/objects) of superseded historical versions can be safely garbage collected once they are dominated
  and no longer needed for active heads or pinned restores. Replicas no longer risk receiving unexpected
  sync requests from the retired peer that demand pruned payloads.

2. *Why this maintenance protocol is not consensus:*
Distributed consensus algorithms (e.g. Paxos, Raft, Viewstamped Replication) provide automatic,
partition-tolerant, leader-elected agreement among an online quorum of nodes without human intervention.
The file-sync membership and retirement protocol is explicitly not a consensus protocol for three design reasons:
- Linear operator approval: Membership revisions form a strict linear sequence ($R_0 \to R_1 \to R_2 \to \dots$)
  where each revision is explicitly authorized and approved by an operator or administrative control action.
  Every revision cryptographically commits to the exact hash of its prior revision (`PriorDigest`) and
  canonical retirement snapshots.
- No quorum voting or automatic transitions: There is no dynamic majority voting, no split-brain quorum
  calculation, and no automated transition behind network partitions. If a surviving node is temporarily
  partitioned or offline during a retirement operation, the cluster does not "elect a new configuration"
  without it; rather, the operator simply waits for connectivity or explicitly exports and imports the approved
  revision bundle onto each survivor.
- Safety over liveness: Any configuration divergence immediately halts folder synchronization
  (`ErrMembershipMismatch`) rather than allowing diverging partitions to make independent progress. This design
  intentionally trades away partition-tolerant unattended reconfiguration in exchange for absolute safety:
  zero silent divergence, explicit audit trails, and strict owner authorization.

Next eligible work: P10 retention, garbage collection, and fallback transfer.

## P10 — Finite storage and safe content cleanup

Packet: P10

State: `complete`.

Prerequisites and design gates checked: P09, D4 reference cleanup rules and crash boundaries closed.
Read `operations.md`, `persistence.md`, and `verification.md`.

Changed files: `model/gc.go`, `model/gc_test.go`, `internal/repository/repository.go`,
`internal/repository/gc.go`, `internal/repository/gc_test.go`, `internal/repository/objects.go`,
`internal/repository/versions.go`, `internal/repository/resolution.go`, `internal/repository/peers.go`,
`internal/workspace/workspace.go`, `internal/workspace/workspace_test.go`,
`internal/replication/transfer.go`, `internal/replication/transfer_test.go`,
`internal/control/types.go`, `internal/control/control.go`, `internal/control/control_test.go`,
`cmd/filesync/main.go`, `tests/integration/p10_cli_test.go`, P10 evidence directory,
and this tracker.

Invariant IDs: I05 (durable boundary / crash-safe commit), I07 (budget and storage limits),
I10 (conflict and history preservation across GC), I13 (atomic state transitions),
I15 (divergent file safety and tombstone preservation without resurrection).

Commands and actual results:
`make test-model` passed 100% (ReferenceSetOracle interleavings, serve lease exclusion, shared chunks,
pending fallback, 50-year forward/backward clock jumps, crash-state transitions);
`make check` passed (fmt-check, vet, unit/model, integration P00-P10 CLI, amd64 build, arm64 cross-build);
`make test-race` passed (0 race warnings across all packages);
`make test-faults` passed (D1-D5 design gates and P03/P04/P06 SIGKILL boundary restarts);
`go test -count=1 ./...` passed across all packages;
`git diff --check` passed clean.

Evidence paths: [summary](../evidence/p10-20260923/summary.md), [commands](../evidence/p10-20260923/commands.md),
[results](../evidence/p10-20260923/results.json), [manifest](../evidence/p10-20260923/manifest.json),
model oracle in `model/gc.go` and `model/gc_test.go`, repository tests in `internal/repository/gc_test.go`,
workspace reclaim tests in `internal/workspace/workspace_test.go`, replication fallback tests in
`internal/replication/transfer_test.go`, control tests in `internal/control/control_test.go`, and
P10 CLI integration tests in `tests/integration/p10_cli_test.go`.

Unexecuted checks / limitations: no hosted CI, native arm64 execution, physical multi-host networking,
or internet-exposed listener testing was run for P10. Full continuous operation and periodic reconciliation
scans remain P12. Active integrity corruption diagnosis and peer-assisted repair remain P11.

Owner explanation notes:
*Why historical content can expire while its version/tombstone metadata remains necessary:*
In a distributed, partition-tolerant causal DAG system, there is a fundamental separation between causal
metadata (version envelopes, vector timestamps, parent dependencies, and tombstone markers) and content payloads
(the heavy binary chunks stored on disk).
1. Causal Dominance and Convergence Require Metadata Graph Continuity:
   When two replicas reconcile, they compare their causal vector clocks and DAG ancestors to determine whether one
   edit happened before another or whether they are concurrent conflicts. A deletion tombstone explicitly references
   its superseded predecessors as causal parents. If a replica deleted the metadata envelope of the deleted version,
   the causal chain would be severed. When a long-offline replica eventually reconnects offering that old version,
   the surviving replica could not prove that the tombstone dominates it, risking accidental resurrection of the
   deleted file (violating invariants I10 and I15). Retaining causal metadata indefinitely preserves join-semilattice
   continuity and guarantees eventual convergence.
2. Content Payloads Consume Finite Physical Storage:
   While metadata is compact (hundreds of bytes per version), payload chunks can be gigabytes or terabytes. A node with
   bounded disk space cannot retain every historical byte of every superseded version forever. Once a version is no
   longer a current head, no longer a concurrent unresolved conflict, and no longer needed as fallback projection during
   pending remote transfer, its physical chunks become superseded history and can be safely unlinked under the retention
   policy. When historical content is requested for an unlinked version, the system cleanly returns `CONTENT_EXPIRED`
   rather than corrupting data or fabricating empty files.

Next eligible work: P11 integrity diagnosis and peer-assisted repair.

## P11 — Integrity diagnosis and peer-assisted repair

Packet: P11

State: `complete`.

Prerequisites and design gates checked: P10 complete. Read operations, persistence, protocol, and verification specifications. Reviewed S15 and Invariants I06, I09, I10, I18.

Changed files: `model/gc.go`, `model/gc_test.go`, `internal/repository/repository.go`, `internal/repository/integrity.go`, `internal/repository/integrity_test.go`, `internal/repository/versions.go`, `internal/repository/resolution.go`, `internal/repository/peers.go`, `internal/repository/workspace.go`, `internal/replication/repair.go`, `internal/replication/repair_test.go`, `internal/control/types.go`, `internal/control/control.go`, `internal/control/control_test.go`, `cmd/filesync/main.go`, `schemas/fixtures/unavailable-content-v1.json`, `tests/integration/p11_cli_test.go`, P11 evidence directory, and this tracker.

Invariant IDs: I06 (immutable causal version identity preserved across chunk repair), I09 (verified reads and quarantine prevent serving corrupted content), I10 (DAG preservation and convergence during repair), I18 (no durable receipt issued for unverified or corrupt content).

Commands and actual results:
`make test-model` passed 100% (ReferenceSetOracle corruption and repair interleavings, multi-version affected accounting, quarantine transitions, availability statuses);
`make check` passed (fmt-check, vet, unit/model, integration P00-P11 CLI, amd64 build, arm64 cross-build);
`make test-race` passed (0 race warnings across all packages);
`make test-faults` passed (D1-D5 design gates and P03/P04/P06 SIGKILL boundary restarts);
`go test -count=1 ./...` passed across all packages;
`git diff --check` passed clean.

Evidence paths: [summary](../evidence/p11-20260923/summary.md), [commands](../evidence/p11-20260923/commands.md), [results](../evidence/p11-20260923/results.json), [manifest](../evidence/p11-20260923/manifest.json), canonical fixture in `schemas/fixtures/unavailable-content-v1.json`, model oracle in `model/gc.go` and `model/gc_test.go`, repository tests in `internal/repository/integrity_test.go`, replication repair tests in `internal/replication/repair_test.go`, control tests in `internal/control/control_test.go`, and P11 CLI integration tests in `tests/integration/p11_cli_test.go`.

Unexecuted checks / limitations: no hosted CI, native arm64 execution, physical multi-host networking (P17), or internet-exposed listener testing was run for P11. Watcher hints, periodic reconciliation, bounded scheduling, and bandwidth limits remain P12.

Owner explanation notes:
*Why a historical receipt cannot prove a peer still has good bytes today:*
A durable receipt is a signed attestation from a peer stating that it successfully verified and committed a specific version's payload at timestamp $T_0$. While cryptographically binding for that historical event, a receipt cannot guarantee that the peer possesses valid bytes at a later query time $T > T_0$:
1. Local Media Degradation and Silent Bit Rot:
   Physical storage hardware experiences unrecoverable read errors, silent flash cell wear, or filesystem corruption. A single bit flip makes a stored chunk fail its SHA-256 digest check even though the node previously held valid bytes and issued a genuine receipt.
2. Finite Storage Retention and Garbage Collection (P10):
   Under approved retention policies, a node is permitted and expected to reclaim storage occupied by superseded versions after their retention window expires. A node that durably stored $V_1$ and issued a receipt at $T_0$ may legitimately unlink and delete $V_1$'s chunks at $T_1$ after $V_1$ is superseded by $V_2$.
3. Operator Interventions:
   Administrative cleanup, accidental deletion of object cache directories, or disk recovery may cause valid bytes to be discarded.
Consequently, content availability is ephemeral and time-varying. In the file-sync architecture:
- Historical receipts record past replication obligations; they are never treated as proofs of current live availability.
- All reads during serving, export, and staging perform verified on-the-fly hashing; any discrepancy immediately triggers quarantine.
- Repair requests fetch chunks authorized by version context from active peer replicas, verifying incoming payloads byte-for-byte before unquarantining and restoring availability to `ready`.

Next eligible work: P12 bounded continuous operation.

## P12 — Bounded continuous operation

Packet: P12

State: `complete`.

Prerequisites and design gates checked: P11 complete. Read operations, persistence, protocol, and verification specifications. Reviewed S10, S13, S17, S18 and Invariants I11, I13, I17, I20.

Changed files: `internal/repository/repository.go`, `internal/repository/work.go`, `internal/repository/work_test.go`, `internal/repository/workspace.go`, `internal/workspace/workspace.go`, `internal/workspace/workspace_test.go`, `internal/scheduler/profile.go`, `internal/scheduler/limiter.go`, `internal/scheduler/limiter_test.go`, `internal/scheduler/watcher.go`, `internal/scheduler/watcher_test.go`, `internal/scheduler/retry.go`, `internal/scheduler/retry_test.go`, `internal/scheduler/queue.go`, `internal/scheduler/queue_test.go`, `internal/scheduler/scheduler.go`, `internal/scheduler/scheduler_test.go`, `internal/replication/transfer.go`, `internal/control/types.go`, `internal/control/control.go`, `internal/control/work_test.go`, `internal/app/app.go`, `cmd/filesync/main.go`, `tests/integration/p12_continuous_test.go`, P12 evidence directory, and this tracker.

Invariant IDs: I11 (unavailable roots create no deletions), I13 (bounded resources; large-file progress amid small edits via aging), I17 (scan after apply/restart or repeated watcher feedback creates no authored versions), I20 (limits preserve recoverable state).

Commands and actual results:
`make check` passed (fmt-check, vet, unit/model, integration P00-P12 CLI, amd64 build, arm64 cross-build);
`make test-race` passed (0 race warnings across all packages);
`make test-faults` passed (D1-D5 design gates and P03/P04/P06 SIGKILL boundary restarts);
`go test -count=1 ./...` passed across all packages;
`git diff --check` passed clean.

Evidence paths: [summary](../evidence/p12-20260923/summary.md), [commands](../evidence/p12-20260923/commands.md), [results](../evidence/p12-20260923/results.json), [manifest](../evidence/p12-20260923/manifest.json), scheduler tests in `internal/scheduler/`, workspace dual scan tests in `internal/workspace/workspace_test.go`, work task tests in `internal/repository/work_test.go`, control tests in `internal/control/work_test.go`, and P12 CLI integration tests in `tests/integration/p12_continuous_test.go`.

Unexecuted checks / limitations: no hosted CI, native arm64 execution, physical multi-host networking (P17), or internet-exposed listener testing was run for P12. Complete operator control and diagnostics remain P13.

Owner explanation notes:
*Why watchers improve latency but cannot replace reconciliation scans:*
Modern operating system notification systems (such as Linux inotify, macOS FSEvents, or Windows ReadDirectoryChangesW) are invaluable optimizations for low-latency continuous synchronization, but they cannot act as the sole mechanism for change detection in an eventually consistent distributed filesystem:
1. Best-Effort Delivery & Queue Overflow:
   Operating system event notification queues are bounded by kernel memory constraints (e.g., `fs.inotify.max_queued_events`). During intense I/O bursts, compiler runs, or mass archive extractions, the kernel drops events and emits `IN_Q_OVERFLOW`. When this occurs, granular file paths are lost, necessitating a directory scan.
2. Offline and Unobserved Mutations:
   Files modified while the sync agent is offline, paused, restarting, or asleep (such as laptop lid-close suspend, OS reboots, or external drive detached and edited on another host) generate no kernel notifications for the sync process.
3. Timestamp-Preserving and Same-Size Edits:
   Tools such as `tar -x`, `rsync -a`, backup restorers, and hex editors frequently rewrite file contents while explicitly retaining the previous modification timestamp (`mtime`) and file size. Because these mutations leave stat fingerprints unchanged, quick stat checks miss them; only a periodic full-content cryptographic scan detects the modified content.
4. Filesystem and Virtual Mount Limitations:
   Network-attached storage (NFS, SMB), FUSE mounts, container volume bindings, and overlay filesystems often do not emit local inotify notifications when modified by remote peers or out-of-process drivers.
Architectural Conclusion: Filesystem watchers serve exclusively as a latency optimization to initiate immediate dispatch when mutations occur in real time. Periodic bounded reconciliation scans and full-content verification scans provide the authoritative correctness guarantee, ensuring eventual convergence regardless of notification drops, kernel buffer overflows, or offline modifications.

Next eligible work: P13 complete operator control and diagnostics.

## P13 — Complete operator control and diagnostics

Packet: P13

State: `complete`.

Prerequisites and design gates checked: P12 complete. Read operations, verification, protocol, and persistence specifications. Reviewed S05, S13, S16, S19–S21 and Invariants I09, I15, I16, I19, I20.

Changed files: `internal/control/*`, `internal/repository/repository.go`, `internal/repository/events.go`, `internal/repository/resolution.go`, `internal/repository/workspace.go`, `internal/workspace/workspace.go`, `internal/history/types.go`, `internal/app/app.go`, `cmd/filesync/main.go`, `tests/integration/p13_control_test.go`, `tests/integration/p10_cli_test.go`, `docs/runbooks/*`, `schemas/fixtures/*`, P13 evidence directory, and this tracker.

Invariant IDs: I09 (unregistered folders preserve local user files and author zero tombstones), I15 (support export sanitizes keys/tokens/content, defaults to path redaction), I16 (strict loopback Host/Origin defense against DNS rebinding and cross-origin attacks; authenticated sessions with CSRF protection), I19 (safe recovery preserves user data; reset-identity prevents rolled-back counter reuse), I20 (crash consistency and transactional operations).

Commands and actual results:
`make check` passed (fmt-check, vet, unit/model, integration P00-P13 CLI, amd64 build, arm64 cross-build);
`make test-race` passed (0 race warnings across all packages);
`make test-faults` passed (D1-D5 design gates and P03/P04/P06 SIGKILL boundary restarts);
`go test -count=1 ./internal/... ./model/... ./tests/integration/...` passed clean;
`git diff --check` passed clean.

Evidence paths: [summary](../evidence/p13-20260923/summary.md), [commands](../evidence/p13-20260923/commands.md), [results](../evidence/p13-20260923/results.json), [manifest](../evidence/p13-20260923/manifest.json), contract fixtures in `schemas/fixtures/`, operator runbooks in `docs/runbooks/`, control unit tests in `internal/control/`, and P13 integration tests in `tests/integration/p13_control_test.go`.

Unexecuted checks / limitations: no hosted CI, native arm64 execution, physical multi-host networking (P17), or external reverse proxy testing was run for P13. Linux and macOS packaging remains P14.

Owner explanation notes:
*How stored, applied, offline, unknown, and conflicted states differ for the same path:*
In an eventually-consistent content-addressed distributed filesystem, a path cannot be represented by a simple scalar state. The sync engine distinguishes five orthogonal state dimensions for any given path:
1. Stored State:
   The complete cryptographic manifest and all constituent 1 MiB chunk payloads for a version at this path have been transferred, cryptographically verified against SHA-256 digests, and durably written into the local content-addressed object store (`repo.sqlite` objects and `.filesync-scratch/staging/`). The bytes are verified and durable on local disk, but have not been placed into the user's mutable working tree directory. A version is "stored" when a background download finishes, but publication is awaiting scheduling, or is blocked by an unresolved conflict.
2. Applied State:
   A specific stored version has been published into the workspace directory via two-phase crash-safe atomic rename (`.filesync-scratch/stage -> path`), the directory parent has been fsynced, and the local `path_projections` table records this version as the authoritative working basis. The user can open and edit the actual plaintext file on disk. Observed file metadata (device, inode, size, mtime, and mode) match the projection record. Only one version can be the applied basis for a path at any given moment.
3. Offline State:
   A peer participating in the folder's canonical membership cannot be reached over the network (e.g. laptop lid suspended, VPS unreachable, network partition). The offline peer's stored/applied receipts are frozen at the timestamp of last contact. The local node knows which versions were delivered up to the disconnect, but cannot push new versions or query the peer's current disk state until connectivity is re-established.
4. Unknown State:
   The local node lacks direct knowledge of whether a peer has stored or applied a particular version. Distinct from "offline": the peer may be actively connected and exchanging other folders or versions, but has not yet transmitted an inventory cursor or receipt for this path. Crucially, in our decentralized model, receipts are non-transitive: a VPS having stored a version does not imply that a home Raspberry Pi has stored it. Until a direct receipt is received from the Pi, the Pi's state for that path is strictly "unknown."
5. Conflicted State:
   Two or more concurrent versions exist for the same path in the version history graph whose vector clocks are mutually incomparable (v1 || v2), OR a structural conflict exists (e.g., file vs directory collision). Both versions may be fully stored in the object store, but neither can be safely applied as the sole working copy. The sync engine flags the path as a conflict head, preserving both versions in metadata, suppressing automatic publication, and presenting the conflicting heads to the operator for deterministic resolution (`filesync resolve select`, `keep-copies`, or `merge`).

Next eligible work: P14 focused embedded web interface.

---

## P14 — Focused embedded web interface

Packet: P14

State: `complete`.

Prerequisites and design gates checked: P13 complete. Read delivery, operations, verification, protocol, and persistence specifications. Reviewed S08, S16, S18 and Invariants I16, I19.

Changed files: `web/*`, `internal/control/types.go`, `internal/control/control.go`, `internal/control/server.go`, `tests/integration/p14_web_test.go`, `scripts/capture_screenshots.js`, `scripts/create_conflict.go`, P14 evidence directory, and this tracker.

Invariant IDs: I16 (control authentication, loopback host/origin validation, SameSite/CSRF token enforcement, stale-view 409 rejection), I19 (UI and CLI use the same control operations, qualified progress display, safe recovery semantics).

Commands and actual results:
`npm --prefix web run build` passed (Vite + TypeScript bundle generated to `web/dist/`);
`make build` passed (Go binary with embedded assets compiled cleanly);
`make check` passed (vet, unit/model, integration P00-P14, amd64 build, arm64 cross-build);
`make test-race` passed (0 race warnings across all packages);
`make test-faults` passed (D1-D5 design gates and P03/P04/P06 SIGKILL boundary restarts);
`node scripts/capture_screenshots.js` passed (8 UI screenshots captured via headless Chromium);
`git diff --check` passed clean.

Evidence paths: [summary](../evidence/p14-20260923/summary.md), [commands](../evidence/p14-20260923/commands.md), [results](../evidence/p14-20260923/results.json), [manifest](../evidence/p14-20260923/manifest.json), screenshots in `docs/evidence/p14-20260923/screenshots/`, web source and embedded handler in `web/`, and P14 integration tests in `tests/integration/p14_web_test.go`.

Unexecuted checks / limitations: no hosted CI, native arm64 execution, physical multi-host networking (P17), or external browser testing beyond headless Chromium was run for P14. Linux packaging and daemon lifecycle remains P15.

Owner explanation notes:
*Which decisions live in the engine and why the UI cannot independently decide a conflict winner:*
In an eventually-consistent distributed filesystem, all safety, causality, and conflict guarantees are defined by the mathematical properties of the version DAG and the SQLite transactional state machine. Every decision affecting causal state lives strictly in the sync engine, not in the user interface:
1. The UI is a Presentation and Review Tool, Not a Consensus Engine:
   The web interface (running in an operator's browser) has only a point-in-time, eventually-consistent view of local SQLite state. It does not participate in network gossip, chunk replication, or SQLite transaction serialization. If the UI were permitted to independently "decide" a conflict winner (e.g. by choosing the version with the newest clock timestamp, the largest file, or an arbitrary device heuristic), wall-clock skew would silently discard legitimate concurrent edits, concurrent web consoles would author conflicting decisions resulting in split-brain divergence, and unreviewed edits arriving over the wire during the operator's review session would be silently overwritten.
2. Why Resolution Semantics Belong to the Engine:
   Resolving a conflict requires creating a forward-causal resolution envelope in the DAG whose parent list explicitly references all reviewed concurrent heads (`Parents = [V_A, V_B]`) and whose vector clock merges and advances both lines of history (`Vector = merge(V_A.Vector, V_B.Vector) + local:counter`). Only the engine's database layer can allocate the monotonic author counter and commit this transactional change. Furthermore, the engine enforces cryptographic head token validation (`STALE_VIEW` rejection): the UI sends the hash of the sorted IDs of the heads reviewed on screen, and if any head changes before the commit transaction executes, the engine rejects the resolution with `409 Conflict` (I16). Finally, atomic two-phase publication into the workspace directory requires crash-safe staging (`stage -> rename`), inode/device validation, and directory parent fsyncing, which the browser cannot execute directly.

Next eligible work: P15 Linux packaging and lifecycle.

---

## P15 — Linux packaging and lifecycle

Packet: P15

State: `complete`.

Prerequisites and design gates checked: P14 complete. Read delivery, operations, verification, protocol, and persistence specifications. Reviewed requirements S20, S21 and Invariants I08, I20.

Changed files: `packaging/systemd/filesync.service`, `packaging/scripts/install.sh`, `packaging/scripts/uninstall.sh`, `packaging/LICENSES.md`, `NOTICE`, `scripts/build_packages.go`, `docs/runbooks/install.md`, `docs/runbooks/upgrade.md`, `docs/runbooks/rollback.md`, `docs/runbooks/uninstall.md`, `cmd/filesync/main.go`, `internal/app/app.go`, `internal/control/maintenance.go`, `internal/control/types.go`, `internal/control/server.go`, `tests/integration/p15_packaging_lifecycle_test.go`, `Makefile`, `docs/dependencies.md`, P15 evidence directory, and this tracker.

Invariant IDs: I08 (counter and event creation are atomic; identity rollback is never knowingly reused), I20 (limits, schema/protocol incompatibility and failed migrations preserve recoverable state). Requirements: S20, S21.

Commands and actual results:
`make check` passed (vet, unit/model, integration P00-P15, amd64 static build, arm64 cross-build, package generation);
`make test-race` passed (0 race warnings across all packages);
`make test-faults` passed (D1-D5 design gates and P03/P04/P06 SIGKILL boundary restarts);
`go test -v ./tests/integration -run "TestP15"` passed (all 10 packaging, preflight, rollback, and lifecycle tests);
`file dist/*` passed (valid RPM v3.0, Debian binary package format 2.0, and gzip tarball archives verified);
`cd dist && sha256sum -c SHA256SUMS` passed (all 6 package checksums verified);
`bin/filesync version` passed (emits version, commit, build date, architecture, Go toolchain);
`bin/filesync maintenance preflight` passed (status=ready, integrity clean, free disk space verified);
`bin/filesync maintenance restore-backup` passed (safely restores database and unconditionally resets device identity);
`git diff --check` passed clean.

Evidence paths: [summary](../evidence/p15-20260923/summary.md), [commands](../evidence/p15-20260923/commands.md), [results](../evidence/p15-20260923/results.json), [manifest](../evidence/p15-20260923/manifest.json), systemd service template in `packaging/systemd/filesync.service`, standalone scripts in `packaging/scripts/`, legal notices in `NOTICE` and `packaging/LICENSES.md`, package generator in `scripts/build_packages.go`, operator runbooks in `docs/runbooks/`, and integration suite in `tests/integration/p15_packaging_lifecycle_test.go`.

Unexecuted checks / limitations: no physical multi-host networking (P17) or Raspberry Pi native execution was run for P15 (arm64 static compilation and ELF validation executed). Destructive fault injection campaign remains P16.

Owner explanation notes:
*Why copying a live SQLite main file may not create a consistent backup and why identity rollback is a protocol concern:*
1. Copying a Live SQLite Main File May Not Create a Consistent Backup:
   In SQLite WAL (Write-Ahead Logging) mode, committed transactions are written sequentially into the `-wal` file (`metadata.sqlite-wal`) and are only periodically checkpointed into `metadata.sqlite`. A plain filesystem copy (`cp metadata.sqlite backup.sqlite`) misses all uncheckpointed WAL transactions, causing silent data loss of recent sync events and version receipts. Furthermore, operating system file copying is non-atomic across 4096-byte SQLite pages: if SQLite checkpoints or flushes pages concurrently with `cp`, the resulting copy suffers torn pages and fails `PRAGMA integrity_check`. Copying both files sequentially also risks WAL salt mismatch. File Sync uses SQLite's official `VACUUM INTO 'backup.sqlite'` via its serialized connection (`filesync maintenance backup`), which acquires an exclusive lock, checkpoints all committed transactions into a clean standalone B-tree, and guarantees 100% transactional consistency without WAL dependencies or torn pages.
2. Why Identity Rollback is a Protocol Concern:
   In an eventually-consistent causal sync protocol, causality is tracked via vector clocks and strictly monotonic author counters: each modification authored by device $D$ is stamped with version envelope $(D, c)$ where $c = \text{next_counter}++$. Version envelopes are immutable; once published, their digest and ancestry are permanent. If device $D$'s database is rolled back to an earlier backup taken when its author counter was at 5 (even though it had previously authored versions 6 through 15 and gossiped them to peers), and device $D$ continues authoring under identity $D$, it will author new, divergent file changes with counter 6. When remote peers receive $(D, 6)$, they already store the original $(D, 6)$ with different contents and ancestry, causing fatal counter collisions, version shadowing, or causality cycles. Therefore, restoring old metadata is an explicit protocol event: `filesync maintenance restore-backup` automatically generates a brand-new cryptographic Device ID ($D'$), issues fresh TLS certificates, updates `config.json`, resets `local_author` in `folders`, and sets `next_counter = 0`. By assuming a fresh identity ($D'$), all new edits advance the global DAG as $(D', 1)$, cleanly preserving Invariant I08 without ever colliding with historical counters authored by $D$.

Next eligible work: P16 reproducible failure campaign and local demo.

---

## P16 — Reproducible failure campaign and local demo

Packet: P16. State: `complete` for the recorded failure models and boundaries.

Prerequisites: P15 implementation and packaged native user-service lifecycle
executed on the laptop, Pi and VPS. Read scope, glossary, protocol,
persistence, operations, verification and delivery packet contracts.

The 2026-09-23 local model/invariant/process-fault results remain historical
scenario evidence. Its so-called abrupt-reset test performed ordinary readback
and orderly close/reopen; those durability claims were withdrawn and the test
renamed `TestP16StorageBarrierSmoke`.

Changed files in the release correction: `scripts/validation/{abrupt_reset.py,
reset_guest.go,host_agent.py,harness.py,test_safety.py}`, `scripts/local_demo.go`,
`tests/faults/p16_abrupt_reset_test.go`, Makefile and owning specifications.

Actual new results: local Go unit/model/integration/design-gate/fault command
passed; `make demo` passed; host-worker safety checks passed (marker/token,
traversal, symlink/hard-link and unrelated/reused PID refusal). QEMU/KVM
abrupt-stop/reboot passed the dirty-cache negative control plus 15 selected
object/version/publication boundaries. A second run uses the final repository
changes; see the [release report](../evidence/release-20261001/summary.md) for
exact commands, final results, environment and hashes.

The VM experiment loses guest dirty cache, unlike daemon SIGKILL. It uses a
new ext4 image and virtio-blk `cache=none`, verifies protected content and
journal recovery after a fresh guest boot, and never stops an existing host
workload. It does not establish physical Pi/VPS power-cut behavior or broken
storage flush promises. An unused hook alias was rejected by the first harness
attempt and removed from the executed matrix; the actual directory-flush hook
is included. No failed attempt is counted as passing.

Final source snapshot `e13e53a` passed `make check`, uncached race checks,
local demo, the 16-case VM matrix, all five storage-failure cases, package
checksums and identical repeat builds of all six amd64/arm64 archives/packages.
The checkout stayed clean. Every invariant and scenario has a named passing
check or scoped experiment in the [matrix](../evidence/release-20261001/invariant-map.md).
Two 15-second fuzz campaigns passed without a failing input. Physical resets,
alternate filesystems and broken hardware flush promises remain unexecuted
and outside the published VM claim.

## P17 — Three-host pilot, measurements and case study

Packet: P17. State: `in_progress`.

Requirements S02/S03/S22 remain unchanged. The 2026-09-24 37.53-second
workstation/Pi/VPS demo is not a personal-use pilot. Its uninterrupted transfer
is not interruption evidence; its estimated benchmark percentages are withdrawn.

Changed files: safe remote and measured benchmark entry points under `scripts`,
release evidence, README/case study, protocol/resource/storage specifications,
repository inventory spool and path-history queries, metadata retry handling,
regression tests, and packaging/default-state corrections. Existing unrelated
`TODO.md` is preserved.

Actual results on the prescribed hardware: verified packaged binaries passed normal sync,
three independent offline heads with matching tokens, reviewed resolution,
late C arrival preserving a conflict, stale-token rejection, A→VPS→B with A's
listener stopped, an actually interrupted 12-chunk file followed by verified
chunk reuse and whole-file hash equality, historical restore as a new version,
and restart/integrity checks. These were scripted checks on laptop/Pi/VPS.
The reports' binary hashes match the checked amd64/arm64 archives. Dedicated roots and exact PIDs
isolate each run; existing pilot folders and unrelated services remain intact.
Native user-service install/restart/embedded UI/uninstall checks passed on all
three reachable hosts, preserving state and workspace bytes.

Ordinary background capture and peer pulls now use persisted authenticated
endpoints; finite budgets survive reopening. Coalescing cannot redispatch a
running pull; completed history cannot hide active work after restart. Native
capture found and fixed the portable unit's Ubuntu AppArmor namespace issue.
The persistent personal pilot is prepared and gracefully upgraded with
consistent backups. Its setup edits are explicitly automated, not adoption.

Measurement work found and fixed a real 1,024-version inventory rejection,
explicit server-backpressure failures, and folder-wide history reconstruction
for per-path operations. The pre-fix regression failed; the repaired test and
relevant suites pass. A single local 1,000-directory capture sample improved
from 14.402 s to 0.104 s; this is a microbenchmark, not a general speedup.
A 20-file/40-MiB measured smoke campaign passed all nine workload checks. The
full 10,000-file/1-GiB campaign passed all nine workloads with verified package
binaries. Three smaller repetitions with 10/20/100-MiB mixed objects passed
27/27; the actual VPS-route campaign passed 9/9. Counters
measure both encrypted TCP directions, not estimated payload or physical wire.
The verified full-file HTTPS baseline skips unchanged files after hashing;
unchanged scans and deletion can therefore favor the baseline.

A later full-size mixed transfer exposed chunk retries that exhausted during
server backpressure. The controlled regression failed before the shared
50/100/200/400-ms chunk-pool cooldown and passed afterwards, including
cancellation and repeated race checks. Fresh final campaigns use `e13e53a`.

A finite-budget unchanged-tree run exposed per-summary storage traversal that
outlived inventory expiry. Bounded page admission fixed it; the same populated
10,000-file fixture passed. An orchestration mistake overwrote part of that
failed record; retained captured observations are labeled partial, not used
in final summaries. Evidence-directory guards now refuse accidental overwrite.

The interrupted full-size parent was absent after its successful initial
workload. The original report was preserved and continuation used a new log;
marker, binary and fixture digests were verified before remaining workloads.
[Measured results](../evidence/release-20261001/measured-results.md) now record
actual bytes, sample counts, timing ranges, receiver resources and storage costs.
Host space pressure and concurrent fixture cleanup limit timing interpretation.
On owner request, nonessential marked validation folders were removed on all
four hosts after process/service checks; active benchmark roots waited until
report completion. Raw evidence and personal pilot services remain intact.

Remaining acceptance evidence: real owner pilot with actual
duration/offline/reconnect/restart, and owner explanation
without agent assistance. Approval of product scope is
not required for these implementation/evidence tasks. Next eligible work is
continuing P17, not declaring every requirement complete.

## Completion entry template

```text
Packet:
State:
Prerequisites and design gates checked:
Changed files:
Invariant IDs:
Commands and actual results:
Evidence paths:
Unexecuted checks / limitations:
Owner explanation notes:
Next eligible work:
```
