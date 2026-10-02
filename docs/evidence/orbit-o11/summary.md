# Orbit Packet O11 Summary: Settings, Storage, Recovery and Long-running Operation

**Packet:** O11  
**Date:** 2026-10-02  
**Status:** `complete`  
**Prerequisites:** O02, O03, O06, O09, O10 complete. Read [vision](../../orbit-product.md), [architecture](../../orbit-architecture.md), [plan](../../orbit-implementation-plan.md), [protocol](../../protocol.md), [persistence](../../persistence.md), [operations](../../operations.md), [verification](../../verification.md), and [orbit release](../../implementation/orbit-release.md#o11--settings-storage-recovery-and-long-running-operation).  
**Requirements Satisfied:** U04 (workspace management), U09 (system startup & background service), U11 (device status), U12 (service management), U13 (backup & export), U14 (maintenance & repair), U16 (retention & cleanup), S14 (retention policies), S17 (storage accounting), S21 (recovery & replacement).  
**Invariants Verified:**
- **I07 (Protected Versions Preservation):** Retention policies and recovery routines preserve active heads and protected versions; ambiguity is reported honestly without data loss.
- **I08 (Atomic Identity & Monotonic Counters):** Identity resets and replacement recovery generate fresh cryptographic keys with monotonic counters; old key rollback/reuse is strictly rejected.
- **I10 (GC Never Removes Protected Content):** Active DAG heads, historical versions within retention, displaced recovery files, and chunks covered by active read leases are immune from garbage collection.
- **I13 (Bounded Work & Resource Limits):** Finite metadata budget (default 256 MiB) and free space reserve (default 512 MiB) are visibly enforced and checked during operation preflights.
- **I15 (Membership Retirement Rejection):** Retired devices cannot rejoin or author new operations; replacement devices must be provisioned with fresh identity keys and explicit owner enrollment.
- **I16 (Idempotency Key Tracking & Replay):** Pruning enforces a 24-hour replay lifetime. Expired idempotency keys are safely pruned; replaying an expired operation explicitly returns `EXPIRED_REPLAY` rather than executing double mutations (Invariant I28).
- **I19 (CLI & UI Parity):** All operations (settings update, workspace pause/resume/unregister, storage accounting, retention preview, manual GC, recovery copy reclaim, consistent backup, record pruning) have equivalent semantics and identical guarantees across the Web UI and CLI (`orbit settings`, `orbit folders`, `orbit storage`, `orbit maintenance`).
- **I20 (Crash Consistency & State Preservation):** Unregistering a workspace removes sync configuration but strictly preserves local files and historical chunks on disk without emitting deletion tombstones to the DAG. Interrupted recovery transitions are detected on startup with `ErrIncompleteRecovery`.
- **I26 (Concurrent Modification Race Safety):** Staging and recovery areas isolate in-flight mutations; displaced files are safely cataloged in `.filesync-internal/recovery/` with explicit reclaim actions.
- **I28 (Safe Bounded Lifecycle Pruning):** Finished durable tasks, expired invitations, terminal join requests, expired read leases, and expired idempotency keys are boundingly pruned; pending work, running tasks, exhausted diagnostic failures, causal DAG metadata, and recovery journals are strictly preserved.

---

## 1. System Overview & Core Capabilities

Packet O11 implements the sustained operation, storage accounting, retention, lifecycle pruning, and recovery architecture for Orbit.

### Core Capabilities & Architectural Invariants

1. **Storage Accounting (5 Distinct Categories):**
   - The storage inspector ([`internal/repository/gc.go`](file://<repo>/internal/repository/gc.go)) calculates concrete byte usage across 5 mutually exclusive storage areas:
     1. **Working Root:** Realized sync tree files.
     2. **Managed Objects / History:** CAS chunk storage (`.filesync-internal/cas/objects/`).
     3. **Staging & Recovery:** In-flight uploads and displaced overwrite copies (`.filesync-internal/recovery/`).
     4. **Database & Metadata:** SQLite database, WAL journals, and index files.
     5. **Filesystem Free Space:** Available volume capacity.
   - **Default Visible Limits:** Displays active limits including **Metadata Budget** (default 256 MiB) and **Free Space Reserve** (default 512 MiB). Pre-flight checks prevent sync operations when available space approaches the safety threshold.

2. **Retention Preview vs. Explicit GC:**
   - **Inspection Guarantee:** Inspecting retention or opening the Retention Policy Modal (`/settings` -> `#btn-open-retention-modal`) executes a read-only query. It computes eligible vs. protected bytes without mutating disk state or deleting chunks.
   - **Conditional Nature of Deleted Files:** The retention preview explicitly explains why Deleted files visibility is conditional: tombstones persist on the causal DAG, but their underlying CAS chunk payloads are subject to retention unlinking once expired.
   - **Explicit Cleanup Mutation:** Storage cleanup (GC) is triggered solely through explicit operator action (`#btn-run-gc` or `orbit maintenance gc`), strictly honoring active read leases and retention fences (Invariant I10).
   - **Recovery Copies Reclaim:** Explicit reclaim button (`#btn-reclaim-recovery`) cleans up displaced overwrite copies once verified by the operator.

3. **Workspace Registration & Unregister Safety (Invariant I20):**
   - **Pause / Resume:** Temporarily halts watcher and scheduler replication without modifying workspace configuration or causal heads.
   - **Unregister Guarantee:** Unregistering a folder clears `root_path` and `registration_id`. All local user files and history chunks remain intact on disk. Crucially, unregistering **never emits deletion tombstones** to the causal DAG, preventing peer data erasure.

4. **Settings & Restart Lifecycle:**
   - Configuration parameters that require daemon restart (e.g. listen address, TLS configuration, port bindings) explicitly state the restart requirement in the interface.
   - The UI never reports live adoption when only a configuration file was updated on disk.
   - Operators can trigger daemon restart directly from the UI or via standard systemd user commands (`systemctl --user restart filesync`).

5. **Safe Bounded Lifecycle Record Pruning (Invariant I28):**
   - Implemented `PruneLifecycleRecords(ctx, maxAge, now)` to bound SQLite growth during long-running operations:
     - Prunes completed/cancelled tasks older than `maxAge` (24h).
     - Prunes expired pairing invitations and terminal enrollment requests.
     - Prunes expired read leases and expired idempotency keys older than 24h.
     - **Strict Invariant Protections:** Never prunes queued, running, retry, or exhausted tasks (retaining failed tasks for operator diagnosis). Never prunes causal metadata, active versions, or recovery journals.
     - **Expired Replay Protection:** Replaying an expired idempotency key explicitly returns error code `EXPIRED_REPLAY`, preventing stale replays from causing inconsistent mutations.

6. **Lost-Device Runbook & Replacement Recovery (G04):**
   - **Runbook Guidance:** Documented in [`docs/runbooks/lost-device-replacement.md`](file://<repo>/docs/runbooks/lost-device-replacement.md) and integrated directly into the Settings view (`#btn-lost-device-guide` -> `#lost-device-guide-panel`).
   - **Retirement & Lockout:** Lost devices are permanently decommissioned from any active peer via `orbit devices retire <id>`, incrementing membership revision and locking out the old public key (Invariants I08, I15).
   - **Replacement Provisioning:** Replacement devices generate fresh keypairs and submit enrollment join requests with key possession proof (G02).
   - **Stopped / Exclusive Backup Restore (G04):** Metadata restores require the daemon to be stopped (`systemctl --user stop filesync`). Restoring re-keys the identity, preserving causal counters monotonically.
   - **Payload Honesty:** Missing CAS payloads after metadata restore are diagnosed honestly as `ContentUnavailable` without synthetic empty substitutions. Missing chunks are re-requested from peers.

---

## 2. Deliverables & Code Changes

1. **[`internal/repository/gc.go`](file://<repo>/internal/repository/gc.go):**
   - Enhanced `DetailedStorageUsage` to report 5 distinct categories: working-root, managed objects, staging, recovery, and metadata bytes, along with filesystem free space, metadata budget (256 MiB), and free space reserve (512 MiB).
   - Implemented `PruneLifecycleRecords` and `LifecyclePruneReport` for safe bounded record pruning (Invariant I28).
2. **[`internal/repository/versions.go`](file://<repo>/internal/repository/versions.go):**
   - Added `SetAcquiredTime(ctx, id, time)` for testing version aging and retention policy boundaries.
3. **[`internal/repository/repository.go`](file://<repo>/internal/repository/repository.go):**
   - Added thread-safe `ExecRaw` and `QueryRowRaw` to enable test fixtures to inspect and manipulate durable records safely.
4. **[`tests/integration/orbit_storage_test.go`](file://<repo>/tests/integration/orbit_storage_test.go):**
   - Comprehensive test suite verifying storage accounting categories, retention preview vs. explicit GC, recovery copy reclaiming, unregister safety (Invariant I20), and pause/resume lifecycle.
5. **[`tests/integration/orbit_pruning_test.go`](file://<repo>/tests/integration/orbit_pruning_test.go):**
   - Test suite verifying Invariant I28: bounded lifecycle record pruning, preservation of pending/exhausted tasks, and `EXPIRED_REPLAY` idempotency safety.
6. **[`tests/integration/orbit_retirement_test.go`](file://<repo>/tests/integration/orbit_retirement_test.go):**
   - Test suite verifying lost device decommissioning, fresh identity replacement enrollment (Invariants I08, I15, I24), stopped metadata backup restore, and payload honesty (G04).
7. **[`docs/runbooks/lost-device-replacement.md`](file://<repo>/docs/runbooks/lost-device-replacement.md):**
   - Detailed operator runbook covering lost device retirement, replacement provisioning, stopped backup restoration, missing payload handling, and systemd user service lingering.
8. **[`web/src/views/SettingsView.tsx`](file://<repo>/web/src/views/SettingsView.tsx):**
   - Rendered 5 storage categories with visual progress bar, limits, Retention & Cleanup Modal trigger (`#btn-open-retention-modal`), GC trigger (`#btn-run-gc-settings`), Recovery copy reclaim (`#btn-reclaim-recovery`), Consistent SQLite Backup creation (`#btn-create-backup`), Bounded Lifecycle Pruning (`#btn-prune-records`), and collapsible Lost Device & Replacement Guide (`#btn-lost-device-guide` / `#lost-device-guide-panel`).
9. **[`web/src/components/RetentionModal.tsx`](file://<repo>/web/src/components/RetentionModal.tsx):**
   - Added retention policy preview with inspection read guarantee explanation and explicit GC trigger (`#btn-run-gc-modal`).
10. **[`web/src/components/UnregisterModal.tsx`](file://<repo>/web/src/components/UnregisterModal.tsx):**
    - Modal explaining Invariant I20 unregister guarantee (local files preserved, no DAG deletion tombstones emitted).
11. **[`scripts/orbit_ui_test.mjs`](file://<repo>/scripts/orbit_ui_test.mjs):**
    - End-to-end browser scenarios for `settings` (screenshots 34–36) and `recovery` (screenshots 37–38).

---

## 3. Verification & Evidence

### Planned Checks Execution

1. **Browser Scenario: Settings, Storage & Pruning (`node scripts/orbit_ui_test.mjs --scenario settings`):**
   - *Result: PASS.*
   - Verified 5 distinct storage accounting categories displayed.
   - Verified visible default limits: Metadata Budget (256.0 MB) and Free Space Reserve (512.0 MB).
   - Captured [`screenshot-34-settings-storage-accounting.png`](screenshots/screenshot-34-settings-storage-accounting.png).
   - Verified retention policy modal: inspection read guarantee and explicit cleanup action.
   - Captured [`screenshot-35-retention-preview-modal.png`](screenshots/screenshot-35-retention-preview-modal.png).
   - Verified unregister workspace modal: Invariant I20 notice (local files preserved, zero deletion tombstones).
   - Captured [`screenshot-36-unregister-folder-modal.png`](screenshots/screenshot-36-unregister-folder-modal.png).
   - Verified consistent SQLite snapshot backup creation (`backup.sqlite`).
   - Verified bounded lifecycle record pruning execution.

2. **Browser Scenario: Recovery & Maintenance (`node scripts/orbit_ui_test.mjs --scenario recovery`):**
   - *Result: PASS.*
   - Verified Root Unavailable badge and revalidation action (`#btn-revalidate-root-...`).
   - Captured [`screenshot-37-recovery-root-unavailable.png`](screenshots/screenshot-37-recovery-root-unavailable.png).
   - Verified interactive Lost Device & Replacement Runbook panel contents.
   - Captured [`screenshot-38-lost-device-runbook.png`](screenshots/screenshot-38-lost-device-runbook.png).

3. **Unit & Integration Test Suites (`go test -run 'TestOrbitSettings|TestOrbitStorage|TestOrbitPruning|TestOrbitRecovery|TestOrbitRetirement'`):**
   - *Result: PASS (19/19 tests passed).*
   - `internal/control`: 6 tests passed (0.067s).
   - `tests/integration`: 13 tests passed (1.910s).

4. **Concurrency & Race Detection (`make test-race`):**
   - *Result: PASS.*
   - Ran `go test -race ./...` across all packages with 0 race detector warnings.

5. **Full Verification Gate (`make check`):**
   - *Result: PASS.*
   - Code formatting (`make fmt-check`), linter (`make vet`), unit tests (`make test`), model verification (`make test-model`), fault injection suite (`make test-faults`), documentation links (`make check-docs`), cross-compilation (`linux/arm64`), Debian/RPM package generation, and SHA256 checksums generation completed with exit code 0.

---

## 4. Screenshot Evidence Index

| ID | File | Description |
|---|---|---|
| 34 | [`screenshot-34-settings-storage-accounting.png`](screenshots/screenshot-34-settings-storage-accounting.png) | Settings storage card displaying 5 accounting categories (Working Root, Managed Objects, Staging, Recovery, Metadata) and visible default limits (Metadata Budget: 256 MB, Free Space Reserve: 512 MB). |
| 35 | [`screenshot-35-retention-preview-modal.png`](screenshots/screenshot-35-retention-preview-modal.png) | Retention & Cleanup Policy modal explaining eligible vs. protected bytes, conditional nature of Deleted files, and explicit GC action. |
| 36 | [`screenshot-36-unregister-folder-modal.png`](screenshots/screenshot-36-unregister-folder-modal.png) | Unregister Workspace confirmation dialog displaying Invariant I20 guarantee (local files preserved, no deletion tombstones emitted). |
| 37 | [`screenshot-37-recovery-root-unavailable.png`](screenshots/screenshot-37-recovery-root-unavailable.png) | Needs Attention triage view showing Root Unavailable badge with direct `#btn-revalidate-root-...` recovery action. |
| 38 | [`screenshot-38-lost-device-runbook.png`](screenshots/screenshot-38-lost-device-runbook.png) | Settings view with expanded Lost Device & Replacement Guide panel detailing retirement, replacement enrollment, and stopped backup restore. |

---

## 5. Limitations & Next Work

- **Outstanding Pilot:** P17 owner-use testing and unaided explanation remain outstanding.
- **Next Eligible Packet:** **[Packet O12: Orbit packages, legacy adoption and executable documentation](file://<repo>/docs/implementation/orbit-release.md#o12--orbit-packages-legacy-adoption-and-executable-documentation)**.
