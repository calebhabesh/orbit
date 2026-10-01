# Comprehensive implementation plan

Planning baseline: 2026-09-20. Implementation exists through P17; current release acceptance and remaining validation are recorded in [packet status](implementation/status.md).

## Read in this order

1. [Approved scope](portfolio-scope.md) and [glossary](../CONTEXT.md).
2. [Architecture](architecture.md) for module ownership and data flow.
3. [Protocol](protocol.md), [persistence](persistence.md), and [operations](operations.md) for the selected packet's contracts.
4. [Verification](verification.md) for invariant IDs and evidence requirements.
5. [Packet status](implementation/status.md), then the first eligible packet.

Scope owns product requirements. Protocol owns causal/wire/membership semantics. Persistence owns durability and cleanup. Operations owns limits, security, and operator behavior. Verification owns test oracles and evidence. Packets sequence work and link these sources; they do not override them. A contradiction is resolved in its owning document and recorded before dependent code is accepted.

## Decision maturity

- **Approved:** the scope and user-visible behavior agreed in the interview.
- **Design baseline:** concrete engineering choices in this handoff; builders may refine them with evidence while preserving approved behavior.
- **Design gate:** an identified hard question requiring a written outcome, executable experiment/model, and matching spec update before dependent implementation proceeds.
- **Verified:** a claim supported by committed tests or recorded experiments on the current implementation. Packet evidence records the scenarios actually verified.

Do not present baseline pseudocode as a completed safety proof. Design gates are implementation tasks, not unexplained placeholders.

## Sequence and release gates

| Stage | Packets | Outcome |
| --- | --- | --- |
| Foundations | P00–P04 | Reproducible toolchain, closed design experiments, independent model, durable local state and safe publication |
| Replication | P05–P09 | Paired transfer, reconciliation, reviewed resolution, three-peer forwarding and lifecycle |
| Operations | P10–P13 | Safe cleanup, integrity repair, bounded automation, complete operator controls |
| Delivery | P14–P17 | Embedded UI, packaging, fault campaign, three-host pilot and portfolio evidence |

Packet details: [foundations](implementation/01-foundations.md), [replication](implementation/02-replication.md), [operations](implementation/03-operations.md), [delivery](implementation/04-delivery.md).

Complete dependencies rather than implementing all layers simultaneously. The end of P06 is the first two-peer product slice. P09 establishes membership/forwarding behavior. P17 is release completion. Tests begin in P01 and remain part of each packet.

## Builder loop

1. Inspect working-tree changes and status. Identify the selected packet, its inputs, invariant IDs, and dependencies.
2. Read the owning specifications. For a design gate, execute the prescribed experiment and record a decision before writing dependent production logic.
3. Implement the smallest complete behavior through the owning module's interface. Add failure hooks as part of recovery work.
4. Run packet checks and relevant regression tests. Record commands, revision, actual outcomes, skipped checks, and limitations.
5. Update status and affected specifications. Leave a handoff listing next eligible work and any concrete blockers.

If tests contradict a design, fix the design or implementation and preserve the failing reproduction. Do not soften the oracle to match observed behavior. User approval is needed for changing agreed scope/guarantees, not for routine implementation choices or advancing completed packets.

## Completion record per packet

Record: status; prerequisites checked; changed files; invariants exercised; exact validation commands; results; evidence locations; known limitations; owner learning answers; next packet. Suggested states: `pending`, `in_progress`, `blocked`, `complete`. A passing compilation is not packet completion.

## Initial builder prompt

> Read AGENTS.md, docs/implementation-plan.md, and docs/implementation/status.md. Implement P00, then continue through eligible packets within the session. Read each packet's owning specifications first. Resolve design gates with executable evidence before dependent implementation. Keep approved product guarantees intact, use disposable data, and update status with actual validation results. Do not claim unrun checks passed. End with a resumable handoff if work remains.

## Ownership and learning

The owner's contribution is directing architecture, implementing/reviewing the sync engine, validating invariants, and defending tradeoffs. Each packet supplies a short explanation exercise. Keep a concise engineering journal of difficult failures and rejected alternatives; generated code volume and test counts are not substitutes for understanding.

## Sources and attribution

Primary references are collected in [verification](verification.md#primary-design-references). Study their mechanisms, cite reused ideas, and distinguish this protocol from Syncthing compatibility. Dependency/toolchain versions are selected and pinned in P00 after checking current official support and actual amd64/arm64 build results.

## Requirement coverage

Use this map when checking whether a packet change leaves a product requirement uncovered. Detailed acceptance criteria remain in the packet and invariant matrix.

| Requirements | Implementation packets |
| --- | --- |
| S01 stack and durable local engine | P00, P03 |
| S02 three-host release | P09, P17 |
| S03 equal-replica connectivity/forwarding | P05, P09, P17 |
| S04 folder membership/authorization | P05, P09 |
| S05 enrollment/retirement | P01, P09, P13 |
| S06 supported filesystem behavior | P01, P04, P07 |
| S07 causal versions/delete/restore | P02, P03, P07, P08 |
| S08 reviewed conflict actions | P08, P14 |
| S09 conflict/working-copy distinction | P02, P04, P07 |
| S10 verified resumable transfer | P03, P06, P12 |
| S11 crash/restart recovery | P01, P03, P04, P06, P16 |
| S12 captured-version guarantee | P01, P03, P04 |
| S13 root/deletion safeguards | P04, P07, P12, P13 |
| S14 bounded storage/cleanup | P10 |
| S15 corruption/repair | P11 |
| S16 qualified status | P06, P08, P13, P14 |
| S17 bounded fair operation | P10, P12 |
| S18 automatic agent/shared UI | P08, P12, P14 |
| S19 security | P01, P05, P13 |
| S20 compatibility/migrations | P00, P05, P15 |
| S21 packaging/diagnostics | P13, P15 |
| S22 evidence/ownership | P00, P16, P17 and each packet's explanation exercise |
