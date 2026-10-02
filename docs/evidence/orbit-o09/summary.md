# Orbit Packet O09 Summary: Recoverable Import, Create, Rename, Move and Delete

**Packet:** O09  
**Date:** 2026-10-02  
**Status:** `complete`  
**Prerequisites:** O01, O02, O07 complete. Read [vision](../../orbit-product.md), [architecture](../../orbit-architecture.md), [protocol](../../protocol.md), [persistence](../../persistence.md), [operations](../../operations.md), [verification](../../verification.md), and [orbit devices & files](../../implementation/orbit-devices-files.md#o09--recoverable-import-create-rename-move-and-delete).  
**Requirements Satisfied:** U10 (within-workspace mutations, upload/import streaming, directory creation, rename/move, recursive delete), S06–S14 (filesystem safety, durable captured versions, crash-consistent journals, root descriptor validation).  
**Invariants Verified:**
- **I01 (Immutable Version ID):** Local versions authored by mutations generate monotonically advancing author counters and preserve strict one-envelope immutability.
- **I03–I13 (Core Sync Invariants):** Ordinary capture preserves received heads; no stored receipt without durable content; partial/corrupt content is never published; recovery preserves protected versions; atomic counter updates without rollback reuse; peer input isolation; GC never unlinks protected content; unavailable roots never cause deletion; structural operations preserve child bytes; bounded resource limits enforced.
- **I16 (Idempotent Resolution & Expiry):** All mutations carry an `Idempotency-Key` cached in `control_operations` for 24 hours. Replays with matching parameters return the cached result without duplicate execution; replays with altered parameters fail with `IDEMPOTENCY_CONFLICT`.
- **I17 (Scan-After-Action):** Mutations update path projections and publication tracking atomically; subsequent `Workspace.Scan` runs do not fabricate spurious feedback edits.
- **I19 (CLI & UI Parity):** `orbit import`, `orbit mkdir`, `orbit move`, and `orbit delete` subcommands execute identically against both a stopped CLI workspace and a live daemon loopback HTTP server.
- **I20 (Schema & Crash Safety):** Schema 13 adds `file_mutations` and `file_mutation_entries` tables; process termination across `PLANNED`, `STAGED`, `INSTALLED`, and `SOURCE_VERIFIED` phases safely recovers without losing protected bytes or corrupting state.
- **I26 (Concurrent Source Modification Race):** If a source file is modified concurrently by an editor or writer during a move, the source is NOT deleted. Both the installed destination copy and the modified source file remain preserved on disk, and the operation completes with `SourceRetained: true`.

---

## 1. System Overview

Orbit Packet O09 delivers durable, crash-consistent file mutations through workspace, repository, control, and CLI layers. Every mutation executes through verified root descriptors, disk budget reservations, destination collision policies, reviewed snapshot tokens, durable operation IDs, and structured crash journals.

### Core Architecture & Implementation Decisions

1. **Schema 13 & Durable Journaling (`internal/repository/`):**
   - Added SQLite Schema 13 (`file_mutations`, `file_mutation_entries`) in `internal/repository/repository.go` and `internal/repository/file_mutation.go`.
   - Journal phase progression: `PLANNED -> STAGED -> INSTALLED -> SOURCE_VERIFIED -> COMPLETED`.
   - Bounded child enumeration and progress tracking for multi-file directory moves.
2. **Atomic Publication & Projection Ordering (`internal/workspace/`):**
   - `CreateLocalVersion` supports `SkipProjection: true`, allowing file mutation actions to author history envelopes without pre-emptively updating `path_projections`.
   - `Workspace.ApplyWithOperationID` performs verified atomic disk mutations (`unix.RENAME_NOREPLACE` or `unix.RENAME_EXCHANGE`) and trailing `CommitPublication` atomically sets `path_projections` with observed disk inode and digest.
   - When overwriting existing files, displaced destination content is safely moved into `.filesync-internal/recovery/<opID>` rather than being unlinked.
3. **Concurrent Editor Modification Protection (Invariant I26):**
   - In `Workspace.Move`, `reviewedToken` is verified prior to source deletion. If the source file was modified concurrently by an external process or editor, the source file is retained (`SourceRetained: true`). Both the newly installed destination and the modified source are preserved on disk.
4. **Directory Subtree Invalidation:**
   - Directory moves evaluate a deterministic child token (`computeSubtreeToken`). If an external file or directory is introduced before the move installs, the operation is rejected with `ErrSubtreeInvalidated` (`SUBTREE_INVALIDATED`), requiring re-review.
5. **Controller & HTTP Endpoints (`internal/control/`):**
   - Added `POST /api/v1/files/import`, `/mkdir`, `/move`, and `/delete`.
   - 24-hour idempotency key tracking using `control_operations`.
   - Structured error mapping to client error codes: `DESTINATION_EXISTS`, `SUBTREE_INVALIDATED`, `STALE_VIEW`, `DIRECTORY_NOT_EMPTY`, `STRUCTURAL_CONFLICT`, `IDEMPOTENCY_CONFLICT`.
6. **CLI Commands (`cmd/filesync/`):**
   - Implemented `orbit import`, `orbit mkdir`, `orbit move` (alias `orbit rename`), and `orbit delete`.
   - Automatic daemon detection via `.agent.lock`; commands transparently forward to the live daemon or execute directly when the daemon is stopped.

---

## 2. Deliverables & Files Created/Updated

1. **`internal/repository/repository.go`:** Added Schema 13 migration creating `file_mutations` and `file_mutation_entries`; updated `CurrentSchema = 13`.
2. **`internal/repository/file_mutation.go`:** Added durable journal records, phase transitions, and mutation entry batch persistence.
3. **`internal/repository/versions.go`:** Added `SkipProjection` support and projection basis fallback in `CreateLocalVersion`.
4. **`internal/repository/file_mutation_test.go`:** Added unit tests verifying mutation journal records, phase transitions, and foreign key cascades.
5. **`internal/workspace/workspace.go`:** Added fault hooks (`HookFileMutationPlanned`, `HookFileMutationStaged`, `HookFileMutationInstalled`, `HookFileMutationSourceVerified`, `HookFileMutationCompleted`), error definitions, and `ApplyWithOperationID`.
6. **`internal/workspace/file_mutations.go`:** Implemented `ImportFile`, `CreateDirectory`, `Move`, `Delete`, `RecoverFileMutations`, and subtree tokens.
7. **`internal/workspace/file_mutation_test.go`:** Added unit tests verifying workspace-level mutation actions.
8. **`internal/control/types.go`:** Added request and response structs for file actions.
9. **`internal/control/file_actions.go`:** Added controller methods, idempotency caching, and error mapping.
10. **`internal/control/file_actions_http.go`:** Added HTTP route handlers and parameter decoders for `/api/v1/files/*`.
11. **`internal/control/server.go`:** Mounted file action routes onto the control HTTP router.
12. **`internal/control/file_actions_test.go`:** Added unit tests for controller file actions.
13. **`cmd/filesync/orbit_files.go`:** Implemented CLI subcommands (`mkdir`, `import`, `move`, `delete`).
14. **`cmd/filesync/main.go`:** Registered orbit file mutation subcommands.
15. **`tests/integration/orbit_mutations_test.go`:** Comprehensive integration suite covering import collision, recovery displacement, mkdir scaffolding, directory moves, subtree invalidation, recursive delete, Invariant I26 concurrent editor race, fault injection journal recovery, and stopped vs live daemon CLI parity.
16. **`docs/persistence.md`:** Documented Schema 13, journal recovery phases, and recovery scratch area.
17. **`docs/operations.md`:** Documented mutation HTTP endpoints, idempotency caching, and CLI commands.

---

## 3. Verification & Evidence

### Planned Checks Execution

1. **Targeted Package and Integration Suite:**
   ```sh
   go test -count=1 -v ./internal/workspace ./internal/repository ./internal/control ./tests/integration -run 'TestOrbitImport|TestOrbitMkdir|TestOrbitMove|TestOrbitDelete|TestOrbitMutation'
   ```
   *Result: PASS.* All packages executed and passed:
   - `github.com/calebhabesh/file-sync/internal/workspace`: `TestOrbitMutationWorkspace` (0.022s)
   - `github.com/calebhabesh/file-sync/internal/repository`: `TestOrbitMutationJournal` (0.015s)
   - `github.com/calebhabesh/file-sync/internal/control`: `TestOrbitMutationControl` (0.019s)
   - `github.com/calebhabesh/file-sync/tests/integration`:
     - `TestOrbitImport` (0.02s): import, collision rejection, overwrite recovery displacement to `.filesync-internal/recovery/<opID>`, Invariant I17 scan-after-action, idempotency replay, and parameter mismatch conflict.
     - `TestOrbitMkdir` (0.01s): nested directory creation, idempotent repeat, structural collision rejection.
     - `TestOrbitMove` (0.03s): single file move, Invariant I26 concurrent editor race retaining both copies, directory subtree move, subtree invalidation on new child.
     - `TestOrbitDelete` (0.02s): single file deletion, non-empty directory rejection without recursive flag, full recursive subtree deletion.
     - `TestOrbitMutation/FaultHooksAndJournalRecovery` (0.01s): injected crashes at `HookFileMutationPlanned` and `HookFileMutationInstalled`; verified automatic journal recovery.
     - `TestOrbitMutation/CLIPartityStoppedAndLiveDaemon` (0.04s): verified full parity of `orbit mkdir`, `orbit import`, `orbit move`, and `orbit delete` between stopped CLI direct execution and live loopback daemon execution (Invariant I19).

2. **Model Consistency Suite:**
   ```sh
   make test-model
   ```
   *Result: PASS (0.290s).* All reference set oracles, membership rollout, competing administration forks, bounded exhaustive schedules, and golden history fixtures passed.

3. **Fault Injection Suite:**
   ```sh
   make test-faults
   ```
   *Result: PASS (0.972s).* All storage barrier smoke, checkpoint boundaries, GC boundaries, control resolution boundaries, quarantine repair boundaries, fuzz tests, and Invariants I01–I20 passed.

4. **Concurrency and Race Detection:**
   ```sh
   make test-race
   ```
   *Result: PASS.* All Go packages passed under `-race` with 0 race detector warnings.

5. **Full Project Verification Gate:**
   ```sh
   make check
   ```
   *Result: PASS.* Code compilation, all unit/integration tests, fault suites, validation scripts, cross-architecture builds (`linux/amd64`, `linux/arm64`), and package generation (`.tar.gz`, `.deb`, `.rpm`) passed with code 0.

---

## 4. Limitations and Unexecuted Scope

1. **O10 Destructive Action UI:** Frontend UI dialogs, toolbar actions, bulk-deletion confirmations, and in-drawer conflict resolutions belong to Packet O10 and remain unexecuted here.
2. **Cross-Workspace Moves:** Moves across different workspaces/folders are intentionally out of scope for O09 and remain unexecuted.
3. **P17 Pilot & Explanation:** Personal pilot testing and unaided explanation work remain open under P17.
