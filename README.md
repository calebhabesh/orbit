# File Sync

A planned Go application that synchronizes selected folders across trusted Linux devices, preserves concurrent offline versions, resumes interrupted transfers, and restores retained history.

**Status:** P00 foundation implementation is in progress: the Go CLI/agent
skeleton, private state/config handling, exclusive locking, SQLite migration
runner, test harness guardrails, and local build checks exist. Synchronization,
protocol behavior, benchmark results, and release binaries do not exist yet.

The completed release targets a Linux laptop, Raspberry Pi, and Oracle Cloud VPS as equal replicas. The VPS can forward stored versions between devices online at different times. Development starts with a two-peer CLI slice; three-host correctness and failure evidence are release requirements.

## Start here

- [Implementation plan](docs/implementation-plan.md): reading order, build sequence, gates, and builder workflow.
- [Approved scope](docs/portfolio-scope.md): product requirements and exclusions.
- [Domain glossary](CONTEXT.md): precise project vocabulary.
- [Build status](docs/implementation/status.md): packet progress and outstanding design experiments.

Go agent and CLI; SQLite metadata; immutable filesystem content; authenticated HTTPS; a small embedded React/TypeScript interface after CLI correctness. This project implements its own reconciliation and transfer logic and reuses established database, transport, and cryptographic libraries.

The engineering story is causal reconciliation and recovery under failure. Evidence must distinguish deterministic simulations, process-crash tests, abrupt-reset experiments, and actual multi-host use. Synchronization and retained history do not constitute an independent backup guarantee.
