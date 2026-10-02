# Implementation roadmap

Updated 2026-10-01. Active work is the
[Orbit revamp plan](orbit-implementation-plan.md), tracked in
[Orbit status](implementation/orbit-status.md), starting at O00.
The original [engine plan](implementation-plan.md) and
[packet status](implementation/status.md) retain the completed scoped work
and outstanding P17 owner-use/explanation evidence.

1. **P00–P04 — Foundations:** toolchain, architecture experiments, independent reference model, durable state, scanner and publication recovery.
2. **P05–P09 — Replication:** explicit pairing, verified transfer, causal reconciliation, resolution/restore, three-peer forwarding, enrollment and retirement.
3. **P10–P13 — Operations:** safe retention, corruption repair, bounded background scheduling, shared CLI/control operations and diagnostics.
4. **P14–P17 — Delivery:** focused web UI, Linux packaging, reproducible fault campaign, real three-host pilot and measured case study.

Two peers are the first transfer milestone, not the final release topology.
P00–P16 have scoped evidence; P17 remains in progress. Orbit builds on these
modules through correctness/gates → onboarding/pairing → file management →
sustained operation/packaging → actual product validation. See its plan for
complete dependencies and acceptance criteria.
