# File Sync implementation roadmap

Decision date: 2026-09-12. Status: all implementation milestones pending.

## Objective

Build a useful Linux file synchronization product whose distributed behavior is central to the user promise: edit selected files independently, reconnect, preserve conflicting changes, and recover from interrupted transfer. The initial topology is two known peers, one owner, direct authenticated connections. Neither peer must remain online continuously, but they must overlap online to exchange data. An always-on storage peer is deferred.

This is a fresh repository. RiftTrace is preserved separately as historical work; its collector, Kafka services and domain model are not dependencies of this project. See [the complete scope](portfolio-scope.md).

## Implementation structure

Proposed layout, to create when code is introduced:

```text
cmd/filesync/         agent and CLI entry point
internal/version/    causal history and reconciliation, pure logic
internal/manifest/   relative paths, file versions and chunk manifests
internal/store/      SQLite metadata and recovery journal
internal/scanner/    stable file reads, scans and notification hints
internal/transfer/   bounded chunk sender/receiver and staging
internal/protocol/   peer messages, authentication and validation
internal/control/    shared CLI/web control API
web/                 React + TypeScript + Vite
tests/              integration/model/failure scenarios
```

Go is locked for the agent/engine/CLI. Choose and pin a maintained toolchain and dependencies when implementing. Prefer a SQLite driver compatible with the packaging target. The browser UI is a static build embedded in the Go binary. Do not add a separate Node service, Rails service, Rust component, Kafka or custom consensus layer.

## Milestone 0 — Freeze the protocol before expanding the UI

Write a versioned protocol specification and an executable reference model. Decide the following explicitly:

- Persistent peer identity and monotonic counters; reinstall/database-loss behavior must not reuse old causal identities.
- Normalized root-relative paths, regular-file scope, directory behavior and excluded symlinks/special files.
- Fixed-size SHA-256 chunks, file manifests, chunk boundaries and complete-file verification.
- Version vectors, equal-content/concurrent-history handling, and conflict-set equivalence.
- Deletion tombstones, rename-as-delete/create, and restore as a new version.
- Exact acknowledgement boundaries: receiving a message, verifying a chunk, and publishing a version are different states.
- Retry/error classification, payload limits, disk budget and backpressure.

Acceptance: model scenarios for offline edits, edit/delete, duplicate messages and conflict resolution produce the expected histories without transport or filesystem code.

## Milestone 1 — One verified transfer through the CLI

Implement two agents with separate local stores and a paired-peer HTTPS connection. Add CLI configuration, explicit scan/sync and status. Transfer a file via manifest/missing-chunk exchange to staging, verify it, then publish it. All product actions go through one control layer.

Acceptance: real file contents arrive exactly; changed bytes are detected; invalid paths and unpaired senders are rejected. A partially received file is never shown as fully synchronized. Start with disposable folders.

## Milestone 2 — Durable history and bidirectional reconciliation

Persist local changes and their causal contexts in SQLite transactions. Detect incomparable versions and preserve each version's content. Expose conflicts, resolve against the versions actually inspected, and let both peers converge after exchanging the same history. A later unseen edit can create another conflict.

Acceptance: both devices independently edit the same path offline; either reconnect order retains both contents and yields equivalent conflict state. Duplicate transmission creates no duplicate logical version. Equal byte hashes alone do not erase causal distinctions. Edit-versus-delete and stale-peer reconnect are covered.

## Milestone 3 — Recoverable publication and bounded transfers

Bridge metadata and filesystem operations with a recovery journal; do not assume SQLite and filesystem rename form one atomic transaction. Specify flush/rename/fsync behavior and test the promised crash model. Recheck local state before replacing a path, because users may edit it during a transfer. File scans need a stable-read/retry rule.

Persist transfer progress and reuse verified chunks after interruption. Bound concurrency, queues, buffers, retries and storage. Keep old versions until publication succeeds. Automatic garbage collection remains disabled until reference tracking and peer/tombstone retention rules are verified.

Acceptance: crashes at each publication boundary recover to a defined state; incomplete/corrupt chunks never become accepted files; disk-full preserves good content; local edits during download survive. Resume avoids retransferring already verified chunks.

## Milestone 4 — Automatic operation and focused interface

Add filesystem notifications with periodic reconciliation scans. Provide a systemd user service and controlled shutdown. Build only three web views: folders/devices; files/history; conflicts. Provide status and limits in language meaningful to the user. Bind the control API locally, protect mutations, and keep remote peer authentication separate.

Acceptance: sync continues without the CLI/browser open; the UI and CLI produce the same state changes; users can restore a version and resolve a conflict. The UI must not imply cloud access while the other peer is offline.

## Milestone 5 — Evidence and portfolio release

Run deterministic reference-model comparisons under reordered/duplicated messages, disconnects and restarts. Add process-level fault injection at stable test hooks rather than timing-only sleeps. Include path traversal, symlink races, conflicting manifests and storage exhaustion in integrity tests. Keep fault workloads isolated from personal originals.

Record real personal use plus a demo of offline concurrent edits, interrupted large transfer, resumed verified output and version restore. Measure bytes transferred against a full-file baseline, CPU/RAM, scan/hash time, sync latency and recovery under a fixed workload. Publish commands, commit, hardware, file distribution and failure schedule.

Single-workstation processes/containers support development and simulated network tests. Demonstrate separate hosts before claiming host-failure independence. No Windows or Riot dependency exists.

## Ownership and stopping rules

Core contribution: version/reconciliation protocol, transfer scheduling, durable state and recovery, with independent tests. Reuse TLS, hashing, database and HTTP implementations. Attribute design references and reused code. Do not claim a protocol was implemented if an external sync engine did the work.

Do not expand to multi-user sharing, S3, multi-platform support, NAT traversal, semantic file merging or custom Raft before the above milestones pass. A runnable CLI, small UI, documented invariants and reproducible failure report define the first release. No throughput, safety or completion claims are earned by this scaffold.
