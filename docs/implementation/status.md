# Implementation status

Updated: 2026-09-23. P00 through P06 are complete. P07 is the first eligible
packet.

| Packet | State | Dependencies | Evidence |
| --- | --- | --- | --- |
| P00 Skeleton/toolchain | complete | none | [evidence](../evidence/p00-20260920/summary.md); [hosted CI](https://github.com/calebhabesh/file-sync/actions/runs/35559982219) |
| P01 Design experiments | complete | P00 | [evidence](../evidence/p01-20260921/summary.md); [decisions](../design-gates.md) |
| P02 Model/history | complete | P01 relevant gates | [evidence](../evidence/p02-20260921/summary.md) |
| P03 Durable repository | complete | P00, P02 | [evidence](../evidence/p03-20260921/summary.md) |
| P04 Workspace/publication | complete | P01 D1/D5, P03 | [evidence](../evidence/p04-20260921/summary.md) |
| P05 Peer/wire layer | complete | P02, P03 | [evidence](../evidence/p05-20260921/summary.md) |
| P06 Two-peer transfer | complete | P04, P05 | [evidence](../evidence/p06-20260923/summary.md) |
| P07 Reconciliation | pending | P06 | none |
| P08 Resolution/restore | pending | P07 | none |
| P09 Membership/forwarding | pending | P08, D3 | none |
| P10 Retention/GC | pending | P09, D4 | none |
| P11 Integrity/repair | pending | P10 | none |
| P12 Continuous operation | pending | P11 | none |
| P13 Operator controls | pending | P12 | none |
| P14 Web interface | pending | P13 | none |
| P15 Packaging/lifecycle | pending | P14 | none |
| P16 Fault campaign | pending | P15 | none |
| P17 Pilot/release evidence | pending | P16 | none |

Packet definitions: [foundations](01-foundations.md), [replication](02-replication.md), [operations](03-operations.md), [delivery](04-delivery.md).

## Engineering gate outcomes

D1 publication races; D2 working basis/same-author lineage; D3 membership
retirement; D4 safe reference cleanup; and D5 directory/bootstrap projection
were closed in P01. See [outcomes](../design-gates.md) and
[architecture](../architecture.md#design-gates). Later packets retain the
listed production implementation and fault-evidence obligations.

## Next action

Begin P07 bidirectional reconciliation and conflict projection using P06's
verified two-peer transfer primitives, P02's causal engine, and P04's publication
safeguards. P06 established single-head transfer, verified chunk resume,
durable receipts, and peer status exchange; P07 connects multi-head and
incomparable history reconciliation.

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
