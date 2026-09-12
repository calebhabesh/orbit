# File Sync

A planned Go application for synchronizing selected folders between trusted Linux devices, preserving concurrent offline edits, resuming interrupted transfers, and restoring earlier versions.

**Status:** planning scaffold only. No sync engine, CLI, or UI has been implemented or tested yet. Product name is provisional.

## Scope

- Go background agent and CLI; SQLite metadata on each device.
- Versioned HTTPS protocol between two explicitly paired Linux peers.
- Chunk integrity, resumable transfer, causal version tracking and conflict preservation.
- Small React/TypeScript web interface served locally by the agent after CLI correctness is established.
- Direct peer synchronization first; an always-on storage peer is a later extension.

## Implementation documents

- [Implementation roadmap](docs/implementation-roadmap.md): milestones, protocol decisions and acceptance checks.
- [Locked portfolio scope](docs/portfolio-scope.md): complete agreed product boundaries and evidence requirements.

The implementation will study established synchronization designs, including [Syncthing's protocol](https://docs.syncthing.net/specs/bep-v1.html), while identifying the project's own code and acknowledging reused components.
