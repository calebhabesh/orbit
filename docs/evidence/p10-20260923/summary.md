# Summary of Packet P10: Finite Storage, Retention, and Safe Content Cleanup

## Overview

Packet **P10** completes the storage management, retention, garbage collection (GC), and fallback transfer subsystem of `file-sync`. It provides deterministic protection for active and historical data, enforces hard storage limits across both state and root filesystems, ensures crash-safe state transitions during object deletion, safely purges unreferenced scratch/recovery material, and supports multi-peer fallback transfers when primary chunk sources are expired or missing.

---

## Key Implementations & Architectural Enhancements

### 1. Independent Reference-Set Oracle ([`model/gc.go`](file://<repo>/model/gc.go))
- Implemented `ReferenceSetOracle` modeling:
  - Version DAG, heads, publication journal, path projections, explicit pins, and serve leases.
  - Retention policy with both protection arms: (1) time since durable acquisition (`RetentionDays`), and (2) minimum locally retained superseded versions per path (`MinSuperseded`).
  - Resistance to forward and backward clock jumps: backward clock jumps protect versions against premature expiry; forward clock jumps maintain the top `MinSuperseded` versions regardless of timestamp distance.
  - Explicit GC lifecycle states: `ObjectPresent -> ObjectIntent -> ObjectUnlinked -> ObjectFinalized`.
  - Crash recovery modeling extra bytes retained safely upon intent crash and finalization upon restart after physical unlink.
- Exhaustive interleaving schedules in [`model/gc_test.go`](file://<repo>/model/gc_test.go) verify Gate D4 ordering properties.

### 2. Schema v7 and Durable Storage Accounting ([`internal/repository/`](file://<repo>/internal/repository/))
- **Schema Migration 7 ([`internal/repository/repository.go`](file://<repo>/internal/repository/repository.go#L379-L388))**:
  - Added `folder_retention` table storing `folder_id`, `retention_days` (default 30), and `min_superseded` (default 20).
  - Added SQLite fault hooks: `HookGCIntent`, `HookGCUnlink`, `HookGCFinalization`.
  - Enforced admission boundaries: `ErrMetadataBudgetExceeded`, `ErrStorageExhausted`, `ErrGCIntentActive`, and `ErrCleanupSuspended`.
- **Protected Chunks Engine ([`internal/repository/gc.go`](file://<repo>/internal/repository/gc.go))**:
  - `ComputeProtectedChunks`: Protects (1) all causal heads and concurrent conflict versions, (2) fallback content for projected paths while remote heads are pending or journal operations are active, (3) versions protected by dual-arm retention policy, (4) explicit pins and active serve leases.
  - `IsCleanupSuspended`: Automatically halts cleanup if an unapproved membership revision is pending or active resumable maintenance is ongoing.
  - `DetailedStorageUsage`: Aggregates state filesystem stats (total, free, available), SQLite database bytes and budget, object chunk bytes and data budget, quarantine bytes, reservations, and folder root staging/recovery statistics.

### 3. Crash-Safe GC State Machine & Intent Reconciliation
- **Three-Phase GC Execution ([`internal/repository/gc.go`](file://<repo>/internal/repository/gc.go#L650-L840))**:
  1. *Intent Phase*: Computes candidate chunks, verifies absence of active pins/leases, and inserts durable intents into `gc_intents` table.
  2. *Unlink Phase*: Verifies intent validity, transitions state to `unlinked`, removes file from disk, and syncs parent directory.
  3. *Finalize Phase*: In a single atomic SQLite transaction, removes rows from `objects`, `object_references`, and `gc_intents`, marks affected versions `content_state='unavailable'`, and records expired retention records.
- **Startup Reconciliation ([`internal/repository/repository.go`](file://<repo>/internal/repository/repository.go#L390-L435))**:
  - Automatically executed on `OpenWithOptions`.
  - If object file still exists on disk (intent crash), the intent is safely cancelled, retaining bytes without loss.
  - If object file was already unlinked and has no active references or pins, metadata is finalized cleanly.

### 4. Workspace Recovery Copies and Reservation Reclaim ([`internal/workspace/`](file://<repo>/internal/workspace/))
- Implemented `ReclaimRecoveryCopies`:
  - Safely scans `.filesync-internal/` using file-descriptor-rooted operations.
  - Discovers orphaned or committed `recovery-*` files not referenced by active publication journal entries.
  - Unlinks files, releases associated storage reservations, deletes committed journal records, and flushes directory metadata.

### 5. Multi-Peer Fallback Replication Transfer ([`internal/replication/`](file://<repo>/internal/replication/))
- Added `Fallbacks []PeerClient` to `TransferOptions` and `Syncer`.
- When fetching chunk payloads, if the primary peer fails or returns `CONTENT_EXPIRED` / `CONTENT_UNAVAILABLE` / non-retryable error, the syncer iterates over fallback peers to retrieve and verify the chunk before failing.

### 6. Control Operations and CLI Interface ([`internal/control/`](file://<repo>/internal/control/), [`cmd/filesync/`](file://<repo>/cmd/filesync/))
- Added subcommands under `filesync storage`:
  - `filesync storage usage [--json]`
  - `filesync storage retention preview --folder <id> [--retention-days <N>] [--min-superseded <N>] [--json]`
  - `filesync storage retention change --folder <id> --retention-days <N> --min-superseded <N> [--json]`
  - `filesync storage gc preview [--folder <id>] [--retention-days <N>] [--min-superseded <N>] [--json]`
  - `filesync storage gc run [--folder <id>] [--idempotency-key <key>] [--json]`
  - `filesync storage recovery reclaim --folder <id> [--json]`

---

## Verification Evidence Matrix

| Assertion / Requirement | Verification Mechanism | Status |
| :--- | :--- | :--- |
| **Interrupted GC loses no protected object** | Injected SIGKILL/crash hooks `HookGCIntent` and `HookGCUnlink` in `gc_test.go` and `p10_cli_test.go`; startup reconciliation validates byte retention and object cleanup | **PASS** |
| **Expired history is visibly unavailable while current reconciliation succeeds** | `TestP10RetentionExpiryAndCrashSafeGC`: superseded v1 unlinked, restore returns `CONTENT_EXPIRED`, v2 head remains intact and active reconciliation succeeds | **PASS** |
| **Long-offline peers do not resurrect deletions (I10, I15)** | `TestP10LongOfflinePeerNoResurrectedDeletions`: C offline when file deleted; post-GC reconnection applies tombstone; neither B nor C resurrects file | **PASS** |
| **Storage exhaustion pauses safely with actionable diagnostics** | `TestMetadataBudgetExceeded`: enforces hard budget, returns `ErrMetadataBudgetExceeded` without corrupting state | **PASS** |
| **Full-file staging budgeted despite chunk reuse** | Workspace reservations calculate whole file staging requirements before admission | **PASS** |
| **Clock jumps do not accelerate protected expiry** | Model oracle clock jump tests (+50y, -50y) verify both retention arms; repo retention protects acquired timestamps | **PASS** |
| **Multi-peer fallback chunk transfer** | `TestSyncerChunkFallbackTransfer` and `TestP10MultiPeerFallbackChunkTransfer`: retrieves chunk from fallback peer when primary returns `CONTENT_EXPIRED` | **PASS** |
| **Reference-set model oracle agreement** | `TestReferenceSetOracleInterleavings`, permutations, and crash recovery pass cleanly in `model/gc_test.go` | **PASS** |

---

## Owner Explanation Note

### *Why historical content can expire while its version/tombstone metadata remains necessary*

In a distributed, partition-tolerant file synchronization system based on a causal Directed Acyclic Graph (DAG), there is a fundamental separation between **causal metadata** (version envelopes, vector timestamps, parent dependencies, and tombstone markers) and **content payloads** (the binary chunks stored on disk).

1. **Causal Dominance and Convergence Require Metadata Graph Continuity**
   - In distributed synchronization, when two replicas reconcile, they compare their causal vector clocks and DAG ancestors to determine whether one edit happened before another ($v_1 \prec v_2$) or whether they represent concurrent conflicting edits ($v_1 \parallel v_2$).
   - A **deletion tombstone** $t$ explicitly references its superseded predecessors $\{v_1\}$ as its causal parents. If replica $B$ were to delete $v_1$'s metadata envelope when $v_1$'s file was deleted, the causal link connecting the initial creation to the tombstone would be broken.
   - When a long-offline replica $C$ eventually reconnects offering version $v_1$, replica $B$ must be able to prove that $t$ causally dominates $v_1$. If $v_1$'s metadata were forgotten, $B$ could not determine that $t$ was intended to delete $v_1$; instead, $B$ might mistake $v_1$ for a brand new, concurrent version created independently, inadvertently **resurrecting** the deleted file (violating invariants **I10** and **I15**).
   - Therefore, causal metadata, vector components, and tombstone envelopes must remain part of the immutable historical record to preserve the algebraic properties of the join-semilattice and guarantee eventual convergence across arbitrary network partitions.

2. **Content Payloads Consume Finite Physical Storage**
   - While metadata is compact (hundreds of bytes per version), payload chunks can be gigabytes or terabytes. A node with bounded disk space cannot retain every historical byte of every superseded version forever.
   - Once a version $v_1$ is no longer a current head, no longer a concurrent unresolved conflict, and no longer needed as fallback projection during pending remote transfer, its physical chunks become superseded history.
   - Under the configured retention policy (e.g. 30 days or top 20 superseded versions per path), those heavy payloads can be safely unlinked.
   - When an operator or peer requests historical content for an unlinked version, the system cleanly and explicitly surfaces `CONTENT_EXPIRED` rather than fabricating empty bytes or corrupt data. The metadata proves that the version genuinely existed and what its exact cryptographic hash was, even though local storage has reclaimed the underlying payload.
