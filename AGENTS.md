# Implementation guidance

For the selected **Orbit native WAN expansion**, start with
[WAN plan](docs/orbit-wan-implementation-plan.md) and
[WAN status](docs/implementation/wan-status.md). Read its
[UX](docs/orbit-wan-ux.md), [architecture](docs/orbit-wan-architecture.md),
[network protocol](docs/orbit-wan-protocol.md), design gates and selected packet.
Implement its first eligible packet (initially W00) when assigned a build task.
Planning requests produce the handoff rather than starting implementation.
Either 6.1 Sol Medium or 3.8 Flash High can implement any eligible W packet.
Preserve existing P/O/T changes and evidence, unfinished T13 technical checks,
and deferred P17 owner use/explanation. Preconfigured Orbit services with optional
self-hosting are the approved direction; runtime capabilities require evidence.

This repository has a working implementation with release validation in progress. Start at [the implementation plan](docs/implementation-plan.md) and [packet status](docs/implementation/status.md). Work on the first eligible packet unless the user selects another task.

For terminal baseline work, read
[UX](docs/orbit-terminal-ux.md), [architecture](docs/orbit-terminal-architecture.md),
[plan](docs/orbit-terminal-implementation-plan.md) and
[terminal status](docs/implementation/terminal-status.md). Start with its first
eligible packet (initially T00). Either 6.1 Sol Medium or 3.8 Flash High can
implement any assigned eligible packet under the master architect/designer's
plan; retain historical P/O evidence and outstanding P17 pilot/explanation work.

- Read [scope](docs/portfolio-scope.md) and [glossary](CONTEXT.md) before changing behavior.
- For causal state, messages, membership, or acknowledgements, read [protocol](docs/protocol.md).
- For scanning, publication, SQLite, restore, or cleanup, read [persistence](docs/persistence.md).
- For authentication, resource limits, configuration, packaging, or diagnostics, read [operations](docs/operations.md).
- For tests, failure claims, benchmarks, or release evidence, read [verification](docs/verification.md).
- Read the selected packet and its prerequisites before implementation. Treat unresolved design gates as work to finish, not permission to invent a guarantee.

Keep protocol and recovery semantics in their owning modules. CLI and UI use the same control operations. Reuse TLS, hashing, HTTP, and database implementations; implement the sync engine here.

For each packet, record actual commands/results and remaining limitations in its status entry. Mark complete only when every acceptance criterion has evidence. Keep unexecuted checks labeled unexecuted. Update the owning specification when an experiment changes a design; seek user input only for changes to approved product scope or guarantees.

Preserve unrelated changes. Use disposable roots for fault tests; require an explicit disposable-environment marker before destructive harness actions. Never run fault injection against personal folders or an existing VPS workload.

Routine dependency selection, implementation details, and reversible fixes are authorized by the build task. No per-packet approval ritual is required. Complete eligible work and maintain a resumable handoff.
