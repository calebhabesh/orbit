# Implementation status

Updated: 2026-09-21. P00 is complete with local, emulated arm64 and hosted
amd64/arm64 evidence. P01 is the first eligible packet.

| Packet | State | Dependencies | Evidence |
| --- | --- | --- | --- |
| P00 Skeleton/toolchain | complete | none | [evidence](../evidence/p00-20260920/summary.md); [hosted CI](https://github.com/calebhabesh/file-sync/actions/runs/35559786069) |
| P01 Design experiments | pending | P00 | none |
| P02 Model/history | pending | P01 relevant gates | none |
| P03 Durable repository | pending | P00, P02 | none |
| P04 Workspace/publication | pending | P01 D1/D5, P03 | none |
| P05 Peer/wire layer | pending | P02, P03 | none |
| P06 Two-peer transfer | pending | P04, P05 | none |
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

## Open engineering gates

D1 publication races; D2 working basis/same-author lineage; D3 membership retirement; D4 safe reference cleanup; D5 directory/bootstrap projection. See [architecture](../architecture.md#design-gates). P01 must produce executable outcomes and update owning specifications. Approved product decisions are already settled; these are engineering work, not questions deferred to the owner by default.

## Next action

Begin P01 architecture experiments and contract freeze. Execute D1–D5 in
disposable environments, record counterexamples and traces, and update each
owning specification before dependent production work.

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
[35559786069](https://github.com/calebhabesh/file-sync/actions/runs/35559786069)
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
