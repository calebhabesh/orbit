# Locked Portfolio Scope

Captured 2026-09-12 from the workspace v2 blueprint. This copy travels with the repository. Status: approved requirements, not evidence of completed implementation. Update this copy alongside future scope decisions.

# Project 3: File Sync - Local-First File Synchronization and Versioning

Proposed root: `<repo>` (new repository; product name TBD).
Decision locked: 2026-09-12. Language: **Go**. Initial platform: **Linux**.
Status: approved project scope, not an implemented or verified system.

## Purpose and User Workflow

Keep selected files available across trusted computers, support offline edits, and preserve work when versions conflict or transfers fail. The inspiration is the experience of Google Drive and the documented synchronization problems addressed by Syncthing. This is a focused personal sync product, not a full cloud-drive clone.

1. Run the background agent, register a folder, and explicitly pair a second trusted device.
2. Edit regular files using normal editors and file managers.
3. Synchronize changed versions and transfer only missing chunks.
4. Continue working while a device is offline; queue durable local changes.
5. Reconnect and either apply causally newer versions or preserve concurrent versions for review.
6. Inspect conflicts and restore earlier versions through a small interface.

Distribution is intrinsic: separate devices can independently modify their copies while disconnected. The central engineering problem is convergence and preservation of conflicting edits, not high traffic. A sync replica is not automatically a backup; version retention and restore behavior must be explicit.

## Locked Stack and Interface

| Component | Choice | Purpose |
| --- | --- | --- |
| Engine, background agent, CLI | Go | Networking, filesystem work, concurrency, synchronization protocol |
| Per-device metadata | SQLite | Durable manifests, versions, transfer journals and peer acknowledgements |
| Content | Managed filesystem directories | Immutable chunks, staging files and retained versions |
| Transport | Versioned HTTPS protocol | Authenticated known-peer communication and bounded transfer |
| User interface | React + TypeScript + Vite | Small locally served control interface |
| Packaging | Go binary; embed built web assets | Runtime users do not need Node.js |
| Initial deployment | Linux agents; systemd user service | All development, tests and demos can run on Linux |

CLI first: configure a root, pair a peer, scan/sync, inspect status/conflicts, and restore. The CLI and web UI use the same control API; neither implements a second sync engine. Background synchronization continues when either interface closes. Bind the control interface to loopback and protect state-changing requests; remote peer access is a separate authenticated boundary.

The web UI has three views: folders/devices and pending transfers; files/version history; conflicts with preserve/select actions. A TUI, Electron/Tauri shell, native tray integration and mobile app are not required. Go is selected for implementation fit, not a claim that language popularity determines employability; Java/Spring remains established portfolio evidence.

## V1 Scope and Explicit Non-Goals

- One owner, two trusted Linux peers, explicit addresses and pairing, selected roots.
- Direct synchronization on a LAN or existing private network. Peers must overlap online to exchange changes.
- Regular files and directories; explicitly reject unsupported symlinks, special files and path conflicts.
- Manual scan/sync vertical slice first, then filesystem notifications plus periodic reconciliation scans. Watch notifications are hints, not a complete change journal.
- Fixed-size hashed chunks first; bounded parallelism, resumability, retries and content verification.
- Offline updates, version history, explicit concurrent conflicts and delete-versus-edit semantics.
- Restore to a new current version without erasing the history being restored from.
- A deterministic failure harness and a two-peer product demo.

Exclude v1: custom consensus/Raft, Kafka, Kubernetes, distributed transactions, global discovery/NAT traversal, multi-user sharing, S3 compatibility, erasure coding, rich document editing, automatic semantic merges, Windows support, and real-time syncing of live databases or actively written game saves. Treat rename as delete plus create initially and document that behavior. Cross-file application-consistent snapshots are not promised.

An optional later always-on storage peer enables synchronization when user devices are online at different times and moves the product toward a personal cloud drive. It is not part of the direct-peer v1 guarantee. Content-defined chunking and bandwidth optimization come only after correctness measurements identify a need.

## Architecture and Metadata

```text
CLI / local browser                         CLI / local browser
         |                                           |
Go agent A <------ authenticated HTTPS ------> Go agent B
  | scanner / reconciliation / transfer workers       |
  + SQLite version and transfer metadata              + SQLite
  + immutable chunks / staging / retained versions    + local storage
  + selected user folders                             + selected folders
```

Implement the protocol and reconciliation logic yourself; reuse database, cryptography, TLS and HTTP libraries. Syncthing is a design reference, not a codebase to rename or a hidden engine behind a new UI.

Records: persistent device identity; registered root; normalized relative path; file-version identity; version vector; content manifest (size, chunk sizes/hashes); deletion tombstone; peer knowledge; transfer journal; conflict set; local publication journal. Protocol messages cover manifest exchange, missing-chunk requests, verified chunk receipt, version acknowledgement and reconciliation status.

Device counters and version creation must be persisted atomically. Specify database-loss/device-reinstallation behavior so a reset cannot silently reuse a prior identity/counter. A content hash identifies bytes, not causal history; equal content does not mean identical version ancestry.

## Correctness Contract

1. Track causality with version vectors for the fixed peer set. Dominating versions supersede older versions; incomparable versions remain concurrent. Wall-clock modification time never silently selects the winner.
2. Preserve competing content versions, including edit-versus-delete conflicts. Conflict resolution creates a new version whose causal context covers the versions actually resolved; a later unseen edit may create another conflict.
3. Use stable operation/version identities so repeated manifests, chunk requests and acknowledgements are safe. Peers receiving the same valid history eventually reach equivalent version/conflict state after communication and local processing succeed.
4. Never expose a partially transferred version as complete. Verify chunks and the completed file before publishing it. Use temporary files, documented filesystem durability steps and a recoverable journal bridging SQLite metadata and filesystem replacement; they are not one atomic transaction.
5. Recheck local state before replacing a path. If the user edits during download or publication, preserve the local version and reconcile again. Define stable-read/retry behavior for files that change during scanning.
6. Bound queues, open files, memory, transfer retries and disk use. Disk-full conditions must fail visibly without replacing good data or acknowledging unsupported durability.
7. Keep tombstones until the supported peers have acknowledged sufficient history. Never prune solely by elapsed time while an offline peer could resurrect a deletion. Version/chunk cleanup must preserve current versions, unresolved conflicts, active transfers and retained history. Automatic garbage collection can remain disabled in v1; expose storage use and explicit limits.
8. Authenticate peers and constrain all incoming paths to registered roots. Reject traversal and symlink escapes, malformed/oversized manifests, and hash-mismatched content. Pairing is explicit; do not design custom cryptography.

Precise deletion, persistence and acknowledgement rules must be frozen in a protocol specification before claiming convergence or data-loss guarantees. These are design requirements, not claims that the implementation already meets them.

## Verification and Measurements

Build a small reference model for version histories and conflicts. Compare the implementation with it across reproducible operation sequences, message duplication/reordering, disconnects and restarts. Passing finite tests supports claims under the tested fault model; it is not a universal proof.

Required cases:

- Same file edited on both offline peers, reconnecting in different orders: both versions preserved and equivalent conflict state.
- Edit versus delete, repeated delete, and restoration after deletion: no silent loss or resurrection from stale peers.
- Transfer interrupted mid-chunk and mid-file: resume safely and verify final bytes.
- Agent crash before/after filesystem publication or metadata updates: recover to a documented state.
- Local edit during remote transfer: do not overwrite unseen local changes.
- Duplicate/reordered messages and lost acknowledgements: no duplicated logical versions or incorrect completion.
- Corrupt chunk, exhausted disk and invalid paths: explicit errors and preserved good state.
- Long-offline peer reconnect and repeat scan: convergence once all required data is available.

Use disposable roots for destructive fault tests. Run Linux process/network-namespace or container experiments on the existing workstation. Show at least two actual hosts before claiming physical-host independence; single-host experiments establish process and simulated-network behavior only. No Windows or game data dependency.

Publish workload shape (file counts/sizes/change pattern), hardware, revision, protocol version, failures, bytes transferred, peak memory, scan/hash time, synchronization latency and resume overhead. Separate hashing, disk and network bottlenecks. Compare missing-chunk transfer with full-file transfer under identical conditions; do not invent speedups.

## Milestones and Definition of Done

1. Protocol/state model and explicit unsupported-file rules.
2. CLI with one-way verified transfer between two agents.
3. Durable version metadata, offline edits and conflict preservation.
4. Bidirectional reconciliation, deletion handling and journaled crash recovery.
5. Bounded automatic synchronization and reference-model/fault tests.
6. Small web UI, real personal-folder pilot, two-peer demo and measured case study.

Done means real file edits synchronize; offline conflicts survive; version restore works; interruptions/restarts are tested; limits are visible; the same protocol works through CLI/UI; and another developer can reproduce the demo and fault experiments. Naming and UI polish must not delay protocol validation.

Resume template, only after measurement:

- Built a Go file-synchronization system with causal version tracking, conflict preservation, resumable chunk transfers and durable local metadata across Linux devices.
- Validated convergence and recovery under [tested failures], measuring [bytes saved/recovery time/latency] over [documented workload].

References: [Syncthing protocol](https://docs.syncthing.net/specs/bep-v1.html), [synchronization behavior](https://docs.syncthing.net/users/syncing.html), [Go concurrency](https://go.dev/doc/effective_go#concurrency), [Go embedded assets](https://pkg.go.dev/embed).

---
