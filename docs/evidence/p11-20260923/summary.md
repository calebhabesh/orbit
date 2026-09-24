# P11 Summary — Integrity Diagnosis and Peer-Assisted Repair

**Packet:** P11
**Execution Date:** 2026-09-23
**Status:** Completed
**Requirements:** S15
**Invariants:** I06, I09, I10, I18

---

## 1. Acceptance Criteria Verification

| Criterion | Result | Evidence |
|:---|:---:|:---|
| **Shared Chunk Corruption Diagnoses All Affected Versions** | **PASS** | `TestP11IntegrityScanAndQuarantineAffectedVersions` verifies that bit rot injected into a chunk shared by multiple files (`fileA.txt` and `fileB.txt`) identifies all referencing versions and workspace paths in `CorruptChunkDetail`. |
| **Peer-Assisted Authorized Repair** | **PASS** | `TestP11PeerAssistedRepairAndInvariantPreservation` verifies that an authorized replica holding valid bytes over TLS serves the chunk, receiver verifies content hash and length, installs it, unquarantines it, and restores availability to `ready` while leaving causal version identity unchanged (Invariants I06, I10). Shared chunk repair restores all dependent versions. |
| **Explicit Unavailable State When No Copy Remains** | **PASS** | `TestP11NoRemainingCopyYieldsExplicitUnavailable` verifies that when no peer has valid bytes, repair terminates deterministically with `status="unavailable"` and `ErrRepairFailed` without crash or busy-loops. |
| **Unauthorized Folder Repair Rejected** | **PASS** | `TestP11UnauthorizedFolderRepairRejected` verifies that repair requests targeting unapproved or unregistered folders fail immediately with `ErrUnauthorized`. |
| **Scan/Repair Pins Prevent GC Races** | **PASS** | `TestP11IntegrityScanAndRepairPinningPreservesGCInterleavings` verifies that pinning chunks with `Pin(...)` during checking and repair prevents concurrent GC from collecting superseded chunks even under 0-day retention policies. |
| **No Durable Receipts for Corrupt Content** | **PASS** | `CanIssueDurableReceipt` checks quarantine status and returns `false` for versions referencing unverified or quarantined chunks (Invariants I09, I18). Verified in unit and CLI integration tests. |
| **Verified Reads Invalidate and Quarantine** | **PASS** | `ReadAuthorizedChunk` and `StageFileContent` compute sha256 and length on read; any mismatch automatically triggers atomic quarantine and updates referencing versions to unavailable. |
| **Five Availability States Distinguished** | **PASS** | `TestP11AvailabilityStateClassification` verifies correct classification across `ready`, `pending`, `expired`, `missing_protected`, and `corrupt`, alongside transient `peer_unavailable`. |
| **Unavailable Content Fixture** | **PASS** | Generated canonical fixture `schemas/fixtures/unavailable-content-v1.json`. |

---

## 2. Key Technical Implementations

1. **Independent Reference-Set Oracle (`model/gc.go`, `model/gc_test.go`)**
   - Modeled object quarantine and corruption states (`ObjectCorrupt`, `ObjectQuarantined`).
   - Implemented `CorruptChunk`, `QuarantineChunk`, `AffectedVersions`, and `RepairChunk`.
   - Verified that chunk repair leaves DAG version identities completely invariant.

2. **Schema Migration 8 & Durable Repository Integrity Engine (`internal/repository/`)**
   - Added tables: `quarantined_chunks` and `peer_integrity_incidents`.
   - Atomic `QuarantineChunk`: renames corrupted chunk to `quarantine/<digest>_<timestamp>`, updates `objects.verified=0`, sets `versions.content_state='unavailable'`, and flushes directories.
   - Atomic `UnquarantineChunk`: marks `quarantined_chunks.repaired=1`, `objects.verified=1`, and restores versions to `content_state='ready'` when all member chunks are verified.
   - Cancelable and bounded `CheckIntegrity`: pins chunks during stream hashing to prevent GC race conditions.

3. **Replication Repair Layer (`internal/replication/repair.go`)**
   - `Repairer` queries approved active membership for candidate peers.
   - Requests authorized chunks via existing peer protocol (`POST /peer/v1/chunks/get`).
   - Verifies incoming chunk payloads byte-for-byte; records `peer_integrity_incidents` on corrupt peer responses to prevent infinite retries without logging sensitive payload bytes.

4. **Controller Operations & CLI Subcommands (`internal/control/`, `cmd/filesync/`)**
   - Added `StorageIntegrityCheck` and `StorageRepair` control operations with idempotency tracking.
   - CLI subcommands:
     - `filesync storage check [--folder <id>] [--path <path>] [--quarantine] [--limit <N>] [--json]` (and `filesync check` alias).
     - `filesync storage repair --folder <id> --version <author:counter> [--peer-url <url> --peer-device <id> --peer-certificate <cert>] [--idempotency-key <key>] [--json]` (and `filesync repair` alias).

---

## 3. Owner Explanation Note

> **Why a historical receipt cannot prove a peer still has good bytes today**
>
> In a causal distributed storage system, a **durable receipt** is an immutable cryptographic statement signed by a peer at a specific point in physical time:
>
> $$\text{Receipt}_B(V_A) = \text{Sign}_B(\text{"Peer } B \text{ durably stored version } V_A \text{ and verified its payload at timestamp } T_0\text{"})$$
>
> While a valid historical receipt proves that peer $B$ successfully received, verified, and committed the bytes of version $V_A$ into its physical storage at time $T_0$, it **cannot prove** that peer $B$ possesses those exact valid bytes at any subsequent query time $T > T_0$:
>
> 1. **Local Media Degradation and Silent Bit Rot**
>    Between time $T_0$ and $T$, physical storage hardware is subject to unrecoverable read errors, silent flash cell degradation, filesystem corruption, or operating system driver faults. A bit flip inside the object file renders the chunk unverifiable despite the historical receipt being 100% genuine.
>
> 2. **Finite Storage Retention and Garbage Collection (P10)**
>    Under approved finite storage retention policies, peer $B$ is permitted and expected to reclaim physical storage occupied by superseded versions after their local retention window expires. A node that correctly issued a receipt for $V_1$ at $T_0$ may legitimately unlink and delete $V_1$'s chunks at $T_1$ after $V_1$ is superseded by $V_2$.
>
> 3. **Operator Actions and Administrative Reset**
>    Local storage maintenance, accidental operator deletion of the cache/objects directory, or database recovery can invalidate or discard stored chunks on $B$.
>
> **System Invariant & Protocol Design Impact:**
> A receipt only proves that a peer once satisfied its replication obligation. Content availability is fundamentally **ephemeral and time-varying**. Therefore:
> - Local serving and export must perform **verified reads** at the moment bytes are read from disk.
> - Nodes must not assume a peer has valid bytes merely because a receipt was recorded. Chunks requested during repair must be independently re-hashed and re-verified before acceptance.
> - Causal version dominance (DAG ancestry) is preserved by metadata tombstones, completely decoupled from physical chunk payload survival.
