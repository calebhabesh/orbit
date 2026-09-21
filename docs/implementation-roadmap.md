# Implementation roadmap

Updated 2026-09-20. The authoritative build sequence is the [comprehensive implementation plan](implementation-plan.md); progress is tracked in [packet status](implementation/status.md).

1. **P00–P04 — Foundations:** toolchain, architecture experiments, independent reference model, durable state, scanner and publication recovery.
2. **P05–P09 — Replication:** explicit pairing, verified transfer, causal reconciliation, resolution/restore, three-peer forwarding, enrollment and retirement.
3. **P10–P13 — Operations:** safe retention, corruption repair, bounded background scheduling, shared CLI/control operations and diagnostics.
4. **P14–P17 — Delivery:** focused web UI, Linux packaging, reproducible fault campaign, real three-host pilot and measured case study.

Two peers are the first transfer milestone, not the final release topology. Safe content cleanup, an always-on ordinary replica, membership lifecycle, and three-host evidence are required by the revised [scope](portfolio-scope.md). No implementation milestone has passed yet.
