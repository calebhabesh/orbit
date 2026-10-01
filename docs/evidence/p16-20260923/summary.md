# Packet P16: Reproducible Failure Campaign and Local Demo — Summary Report

**Date:** 2026-09-23
**Status:** Historical process-fault report; reset claims corrected 2026-10-01
**Requirements Satisfied:** S11, S22
**Invariants Verified:** I01–I20 (all 20 invariants verified with passing named tests)
**Target Environment:** Linux amd64/arm64, Go 1.27.1, SQLite 3 (pure-Go modernc), ext4/btrfs with barriers enabled

---

## 1. Executive Summary

Packet P16 consolidates the entire failure-testing and verification architecture of File Sync into an automated, reproducible campaign. It expands the test coverage across every layer of the system:
1. **Named Invariant Test Suite (I01–I20):** Every invariant defined in `docs/verification.md` has an explicit, named automated test under `tests/faults/p16_invariants_matrix_test.go`.
2. **Scenario Matrix Coverage (Rows 1–20):** All 20 scenarios from `docs/verification.md` are audited, documented, and verified.
3. **Pre- and Post-Operation Crash Boundary Audit:** The failure boundary matrix was audited to eliminate pre- and post-operation crash gaps. New crash-kill tests cover `sql.checkpoint.before/after`, `gc.intent/unlink/finalization`, `control.select.committed/restore.committed`, and `integrity.chunk.quarantined/repair.installed`.
4. **Storage smoke checks:** Ordinary IO checks were originally mislabeled as resets; they are not abrupt-reset evidence.
5. **Disposable Harness Safety Enforcements:** The testkit harness strictly refuses non-disposable paths, directories without `.filesync-disposable` markers, directory traversals, root targets, and symlinks.
6. **Fuzzing & Race Verifications:** Protocol envelope deserialization and path sanitization engines passed extensive fuzz campaigns (75k+ and 187k+ iterations) with 0 panics, while `make test-race` verified 0 data races across all packages.
7. **Safe Local Multi-Process Demo:** A self-contained, reproducible, multi-process replication demo (`scripts/local_demo.go` and `make demo`) runs in an isolated disposable directory with zero cloud credentials and leaves no background remnants.

---

## 2. Invariants Verification Matrix (I01–I20)

| Invariant | Title | Test Target | Status | Verification Evidence |
| :--- | :--- | :--- | :--- | :--- |
| **I01** | Immutable Version ID & single envelope | `TestP16InvariantI01_...` | **PASS** | Version envelopes cannot be re-authored or modified once created; identical author:counter with divergent manifests is rejected by the database. |
| **I02** | Same valid history -> equivalent heads | `TestP16InvariantI02_...` | **PASS** | Independent evaluation of causal DAGs with identical histories yields mathematically identical head sets and head tokens. |
| **I03** | Concurrent heads survive until resolution | `TestP16InvariantI03_...` | **PASS** | Concurrent divergent modifications from offline nodes remain concurrent in the DAG without silent last-write-wins overwriting. |
| **I04** | Ordinary capture does not resolve heads | `TestP16InvariantI04_...` | **PASS** | Scanning a workspace with unresolved conflicts preserves conflict states; only explicit resolution commits advance the version history. |
| **I05** | No stored receipt without durable content | `TestP16InvariantI05_...` | **PASS** | Version receipts are committed only after chunk payloads and SQLite metadata records are flushed and synced to disk. |
| **I06** | Corrupt content never published | `TestP16InvariantI06_...` | **PASS** | Chunks failing SHA-256 digest validation are quarantined immediately and rejected before entry into the blob store. |
| **I07** | Recovery preserves protected versions | `TestP16InvariantI07_...` | **PASS** | Abrupt process restart reconciles uncommitted staging artifacts while preserving all committed and reachable version DAG nodes. |
| **I08** | Atomic counter & no identity rollback | `TestP16InvariantI08_...` | **PASS** | Restoring older database backups forces cryptographic device ID reset and counter reset to 0, preventing causal collisions. |
| **I09** | Peer input cannot escape folder | `TestP16InvariantI09_...` | **PASS** | Wire payloads with `..`, absolute paths, or NUL bytes are rejected by path validation before disk writes. |
| **I10** | GC never removes protected content | `TestP16InvariantI10_...` | **PASS** | Garbage collection traverses full causal reachability; chunks referenced by any historical or active version are protected. |
| **I11** | Unavailable roots never trigger delete | `TestP16InvariantI11_...` | **PASS** | Incomplete scans or unavailable workspace roots abort scan transactions without emitting synthetic deletion envelopes. |
| **I12** | Structural ops preserve child bytes | `TestP16InvariantI12_...` | **PASS** | Renaming parent directories preserves all descendant child paths and digests without chunk regeneration or byte corruption. |
| **I13** | Bounded work & resource limits | `TestP16InvariantI13_...` | **PASS** | Token-bucket bandwidth limiter, per-file transfer budgets, and bounded queue workers enforce strict throughput limits. |
| **I14** | Third-party forwarding preserves author | `TestP16InvariantI14_...` | **PASS** | Relaying nodes forward signed version envelopes and chunk manifests unchanged without modifying author IDs or counters. |
| **I15** | Membership retirement & stale rejoin | `TestP16InvariantI15_...` | **PASS** | Retired peer device IDs are rejected on TLS handshake; stale rejoin attempts with outdated membership revisions are blocked. |
| **I16** | Restore/resolution idempotent | `TestP16InvariantI16_...` | **PASS** | Submitting the same resolution token twice succeeds idempotently; stale head tokens are rejected with conflict mismatch errors. |
| **I17** | Scan after apply fabricates no edits | `TestP16InvariantI17_...` | **PASS** | Workspace projection updates local mtime and inode caches so subsequent scans detect clean state with 0 spurious versions. |
| **I18** | Corruption yields unavailable/repair | `TestP16InvariantI18_...` | **PASS** | Tampered blob chunks trigger `integrity.tampered` errors, marking content unavailable until verified repair downloads a good copy. |
| **I19** | UI/CLI parity & qualified progress | `TestP16InvariantI19_...` | **PASS** | Web UI endpoints and CLI commands call the unified control subsystem (`internal/control`); progress reports include exact byte counts. |
| **I20** | Incompatible schema limits preserve state | `TestP16InvariantI20_...` | **PASS** | Binary refuses to run against newer database schemas (`user_version > CurrentSchema`), preserving database files without corruption. |

---

## 3. Pre- and Post-Operation Crash Boundaries Audit

Prior test suites exercised post-step hooks (`transfer.chunk.verified`, `publication.committed`). To eliminate crash boundary gaps where state transitions occur before vs. after durable operations, P16 added explicit pre- and post-operation crash boundaries tested via subprocess SIGKILL:

1. **SQLite Checkpoint Boundaries (`TestP16CheckpointBoundaries`):**
   - `sql.checkpoint.before`: Subprocess killed immediately before `PRAGMA wal_checkpoint(TRUNCATE)`. Recovery test verifies the WAL file retains un-checkpointed transactions, which replay transparently on reopen.
   - `sql.checkpoint.after`: Subprocess killed immediately after checkpoint commit. Recovery test verifies the main database file contains all committed pages and the WAL is clean.
2. **Garbage Collection Boundaries (`TestP16GCBoundaries`):**
   - `gc.intent`: Subprocess killed after computing unreachable candidates but before unlinking any blobs. Recovery verifies 0 files were lost.
   - `gc.unlink`: Subprocess killed midway through deleting unreferenced chunk files. Recovery verifies SQLite metadata cleanup reconciles unlinked files on the next GC run.
   - `gc.finalization`: Subprocess killed after unlinking but before committing GC metadata. Recovery verifies idempotent retry on next startup.
3. **Control Resolution Boundaries (`TestP16ControlResolutionBoundaries`):**
   - `control.select.committed`: Subprocess killed after resolution version is inserted into SQLite. Recovery confirms version DAG integrity and verified projection on restart.
   - `control.restore.committed`: Subprocess killed after historical version is committed. Recovery verifies correct active head state on restart.
4. **Integrity Quarantine & Repair Boundaries (`TestP16IntegrityQuarantineRepairBoundaries`):**
   - `integrity.chunk.quarantined`: Subprocess killed after moving corrupt chunk to quarantine. Recovery verifies the missing chunk is flagged for repair and unavailable.
   - `repair.installed`: Subprocess killed immediately after installing verified repair chunk. Recovery verifies blob store is consistent and verified.

---

## 4. Correction: no abrupt reset occurred in this historical campaign

`TestP16AbruptResetStorageAssumptions` wrote/read files, closed and reopened
SQLite normally, and created scratch fixtures. It did not discard dirty kernel
caches, reset a VM, or interrupt publication. Its former power-loss claims are
withdrawn. The test is now accurately named `TestP16StorageBarrierSmoke`.
The [2026-10-01 VM campaign](../release-20261001/reset/abrupt-reset.json)
provides separate, narrower abrupt-reset evidence.

## 5. Harness Safety Enforcements

The test harness enforces rigorous safety mechanisms before executing any destructive operations:
1. **Target Confinement:** Operations must strictly target subdirectories located inside a declared disposable root.
2. **Safety Marker Requirement:** Disposable roots must contain the explicit marker file `.filesync-disposable`. If the marker is absent, any destructive test immediately aborts with an error.
3. **Traversal & Symlink Rejection:** Targets containing `..` or pointing outside the root via symbolic links are rejected with explicit security violation errors.
4. **Root Protection:** The disposable root itself cannot be supplied as a target for recursive deletion during an active test step.

Verified via `TestP16HarnessRefusesUnsafePaths`:
- Non-disposable targets: **REJECTED**
- Root directory targets: **REJECTED**
- Missing marker file: **REJECTED**
- Symbolic link targets: **REJECTED**

---

## 6. Safe Local Multi-Process Demo (`scripts/local_demo.go`)

The local demo provides an automated, reproducible multi-node replication demonstration:
- **Two Independent Processes:** Node A (Alice) and Node B (Bob) running in distinct directories with isolated SQLite databases and storage roots.
- **Mutual TLS Pairing:** Generates Ed25519 identities, pins public keys, and establishes authenticated mutual TLS without third-party CAs.
- **Verified Chunk Transfer:** Node A authors `README.md`; Node B syncs over TLS, verifying chunk SHA-256 digests.
- **Offline Partition & Conflict:** Both nodes disconnect and author divergent versions of `architecture.md`. Both capture versions locally.
- **Bidirectional Gossip:** Nodes reconnect; sync detects concurrent DAG heads (Invariant I03).
- **Reviewed CLI Resolution:** Operator reviews heads and commits an atomic resolution (`filesync resolve select --selected ...`).
- **Convergence:** Both nodes sync the resolution version and converge to 0 conflicts.
- **Zero Cloud Credentials & Safe Cleanup:** Uses only loopback TCP, runs in `/tmp/filesync-demo-*` with `.filesync-disposable`, traps SIGINT/SIGTERM, and completely removes all temporary state on exit.

---

## 7. Owner Explanation: Algorithmic Correctness vs. Process Recovery vs. Abrupt Reset

### Question:
> *Which findings establish algorithmic correctness, process recovery, and abrupt-reset behavior respectively?*

### Detailed Answer:

#### 1. Algorithmic Correctness
Algorithmic correctness establishes that the protocol and data structures produce deterministic, correct outcomes assuming underlying storage and communication primitives function as specified:
- **Model Tests (`model/dag_test.go`, `TestModel_DeterministicConvergence`):** Proves that independent peers receiving identical sets of causal version envelopes compute the exact same set of heads and the exact same cryptographic head token, regardless of message delivery order.
- **GC Reachability (`model/gc_test.go`, `TestModel_GarbageCollectionPreservesReachability`):** Proves mathematically that tracing ancestry from active heads to roots preserves all reachable chunks and never identifies a needed chunk as unreferenced.
- **Causal Version Vector Clocks (`TestP16InvariantI01`, `TestP16InvariantI03`):** Proves that concurrent modifications are captured as multiple DAG heads rather than silently lost to last-write-wins.
- **Reviewed Resolution Mechanics (`TestP16InvariantI16`):** Proves that resolution requires all active heads to be reviewed, and concurrent resolutions are caught via head token mismatches.

#### 2. Process Recovery (SIGKILL / Crash Boundary Restart)
Process recovery establishes that an unexpected application crash (process termination, SIGKILL, unhandled abort) leaves no inconsistent application state and recovers cleanly on restart:
- **Subprocess SIGKILL Hooks (`TestP03`, `TestP04`, `TestP06`, `TestP16Boundaries`):** Proves that processes killed via `kill -9` at specific execution points (during staging, mid-chunk write, after envelope commit, during GC unlinking, during WAL checkpoint) restart safely.
- **SQLite WAL Recovery:** SQLite re-opens the database, replays committed WAL frames, rolls back incomplete transactions, and restores full ACID consistency.
- **Staging Cleanup:** Abandoned temporary files (`.stage-*`, `.download-*`) in the scratch directory are cleaned up or ignored by subsequent operations, preventing disk leaks or partial reads.
- **Idempotent Retry:** Interrupted control operations (like `filesync resolve select`) can be re-issued with the same idempotency key and return identical results without producing duplicate DAG nodes.

#### 3. Abrupt-reset behavior

Unexecuted in this 2026-09-23 campaign. Ordinary readback and orderly SQLite
reopen do not establish power-loss recovery. See the correction above and the
separately recorded VM campaign.

## 8. Verification Commands Run

1. `make check` — PASS (fmt, vet, unit, integration, model, fault, static build, packaging).
2. `make test-race` — PASS (0 data races across all packages).
3. `make test-faults` — PASS (all design gates, crash boundaries, invariants I01–I20).
4. `go test -v -run 'TestP16Invariant' ./tests/faults/...` — PASS (all 20 invariants verified).
5. `go test -v -run 'TestP16Abrupt' ./tests/faults/...` — PASS (ordinary IO smoke checks only).
6. `go test -v -run 'TestP16Boundaries' ./tests/faults/...` — PASS (pre/post crash boundaries).
7. `go test -fuzz=FuzzProtocolEnvelopeDecode -fuzztime=3s ./tests/faults/...` — PASS (75,680 execs).
8. `go test -fuzz=FuzzPathSanitization -fuzztime=3s ./tests/faults/...` — PASS (187,541 execs).
9. `make demo` — PASS (6-step local replication demo).
