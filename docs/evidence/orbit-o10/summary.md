# Orbit Packet O10 Summary: File Actions, History, Deleted Files and Needs Attention

**Packet:** O10  
**Date:** 2026-10-02  
**Status:** `complete`  
**Prerequisites:** O08, O09 complete. Read [vision](../../orbit-product.md), [architecture](../../orbit-architecture.md), [protocol](../../protocol.md), [persistence](../../persistence.md), [operations](../../operations.md), [verification](../../verification.md), and [orbit devices & files](../../implementation/orbit-devices-files.md#o10--file-actions-history-deleted-files-and-needs-attention).  
**Requirements Satisfied:** U07 (conflicts & status), U10 (workspace mutations: upload/import, create dir, move/rename, delete), U11 (device status), U15 (health/recovery).  
**Invariants Verified:**
- **I03 (Causal Consistency & History DAG):** Concurrent heads survive on the DAG until explicit resolution. Restore operations never fabricate artificial histories; they author a new causal child envelope referencing current heads and restored version content.
- **I16 (Idempotency Key Tracking & Replay):** All mutating actions generate durable idempotency keys cached in `control_operations` for 24 hours. Replays return the cached result; stale view conflicts reject modified parameters.
- **I19 (CLI & UI Parity):** Every file mutation (create directory, import/upload, move/rename, delete), conflict resolution, and restore operation is accessible with equivalent semantics via the Web UI and the CLI (`orbit restore`, `orbit conflicts`, `orbit import`, `orbit mkdir`, `orbit move`, `orbit delete`).
- **I26 (Concurrent Modification Race Safety):** Source file race verification is reflected in action dialogs. When concurrent modifications occur, source bytes are retained and clear user-facing diagnostics report the outcome.
- **I27 (Explicit Non-Binary State Badges):** Availability states, tombstones, and integrity statuses are displayed explicitly with distinctive badges (`Available`, `Expired`, `Deleted (Tombstone)`, `Syncing`, `Paused`), never masked by misleading green checkmarks.

---

## 1. System Overview & User Workflows

Packet O10 delivers the user-facing interface, interaction modals, history inspection, conditional restore, and conflict resolution flows for the Orbit product experience.

### Core Interface Capabilities & Design Principles

1. **Toolbar & Selection File Actions:**
   - **Upload:** Single file or drag-and-drop file upload with collision pre-flight checks. Overwriting an existing file displays the concrete affected item, current file size, and requires explicit user confirmation before proceeding with atomic replacement and displacement.
   - **New Folder:** Accessible modal with automatic folder path normalization, parent directory scaffolding, and structural collision detection.
   - **Move / Rename:** Modal supporting within-workspace relocation. Confirms destination path, warns on collisions, and requires operator confirmation.
   - **Delete:** Single and multi-item delete confirmation dialog. Non-empty directories require checking an explicit "Recursively delete folder and all contents" checkbox, preventing accidental mass data loss.
   - **Error Context Retention:** Failed mutations (e.g. `DESTINATION_EXISTS`, `STALE_VIEW`) retain the user's input in the dialog alongside actionable diagnostic messages instead of discarding user entries.

2. **File History & Conditional Restore:**
   - The file details drawer displays a chronological timeline of all causal versions for the selected path.
   - Each history entry shows author device name, commit timestamp, human-readable file size, version hash prefix, and exact CAS availability badges (`Available` vs `Unavailable / Expired`).
   - Unavailable chunks cannot be restored as empty or corrupt dummy files.
   - Confirming a restore invokes `/api/v1/control/restore` (or CLI `orbit restore`), authoring a brand new causal child version that references current heads as parents and binds to the selected historical version's chunk content.

3. **Deleted Files View:**
   - Dedicated route (`/deleted`) displaying all tombstoned files in the workspace.
   - Struck-through paths clearly signal deletion state. Displays deletion author device, timestamp, and last known file size.
   - Integrated restore action triggers the preview and confirmation modal, bringing the file back into active sync without physically scouring disk trash bins.

4. **Needs Attention & Actionable Health Center:**
   - Grouped into distinct triage cards:
     - **Content Conflicts:** Lists paths with multiple concurrent leaf heads.
     - **Structural Conflicts:** Lists sibling namespace or directory collisions.
     - **Paused & Inaccessible Folders:** Root descriptor failures with direct revalidation action (`#btn-revalidate-root-...`).
     - **Failed Durable Tasks:** In-flight operations or journal tasks with `Retry` and `Cancel` actions.
     - **Health & Diagnostic Checks:** Root status, disk budget warnings, quarantine notices.
     - **Storage Maintenance:** Explicit button to trigger vacuum and garbage collection (`#btn-run-gc`).

5. **Human-Review Conflict Resolution Workflows:**
   - **Operator Review Context:** The conflict modal clearly displays: *"Timestamps and device names are shown for human review context only; they are never used as automatic winner resolution rules."*
   - **Three Causal Resolution Strategies:**
     1. **Select Winner:** Choose one head as the definitive version; creates a new version referencing all heads as parents with the chosen head's content.
     2. **Keep Separate Copies:** Preserves both versions by allocating disambiguated side-by-side filenames (e.g. `doc.txt` and `doc (peer-device).txt`), cleanly retiring the conflict.
     3. **Manual Merge:** In-browser text editor allows the operator to review differing contents side-by-side or synthesized, edit the final merged text, and author a resolved version.
   - **Delete vs. Edit Conflicts:** Tombstoned heads are explicitly labeled `Deleted (Tombstone)`. Operators can consciously choose whether the deletion or the edited content wins.

---

## 2. Deliverables & Files Created/Updated

1. **`web/src/components/FileActionModals.tsx`:** Modal components for Folder Creation (`CreateFolderModal`), Move/Rename (`MoveFileModal`), Delete Confirmation (`DeleteFileModal`), and Overwrite Collision Warning (`OverwriteUploadModal`).
2. **`web/src/views/FilesView.tsx`:** Updated with toolbar action buttons (`#btn-upload-file`, `#btn-new-folder`), row selection toolbar (`#btn-action-move`, `#btn-action-delete`, `#btn-action-download`), drag-and-drop dropzone overlay, and mutation progress notification banners.
3. **`web/src/components/FileDetailsDrawer.tsx`:** Integrated Move and Delete buttons into the drawer header; wired historical versions list with CAS availability badges.
4. **`web/src/components/ConflictResolveModal.tsx`:** Comprehensive conflict resolution modal featuring human review context guidance, peer origin attribution, tombstone badges, 3 resolution tabs (Select Winner, Keep Copies, Manual Merge), and stale view re-review handling.
5. **`web/src/views/NeedsAttentionView.tsx`:** Grouped attention triage view covering content conflicts, structural conflicts, paused folders, failed durable tasks, diagnostic doctor warnings, and manual GC triggers.
6. **`web/src/views/DeletedFilesView.tsx` & `web/src/components/RestoreModal.tsx`:** Struck-through deleted files browser with CAS availability badges, preview metadata, and conditional restore confirmation.
7. **`internal/repository/conflicts.go`:** Added `DisplayTime` to `ConflictHead` populated from version envelopes to support human review context.
8. **`cmd/filesync/orbit_files.go` & `cmd/filesync/main.go`:** Added `orbit restore` and `orbit conflicts` CLI subcommands; added automatic folder auto-discovery when `--folder` is omitted; updated client connection to read `control.addr` and `control.token`.
9. **`scripts/orbit_ui_test.mjs`:** Added end-to-end browser scenarios for `file-actions`, `history`, and `attention` using Puppeteer, capturing screenshots 25–33.

---

## 3. Verification & Evidence

### Planned Checks Execution

1. **UI Scenario: File Actions (`node scripts/orbit_ui_test.mjs --scenario file-actions`):**
   - *Result: PASS.*
   - Verified folder creation (`projects/`).
   - Verified file upload, collision detection, and overwrite confirmation modal.
   - Verified move/rename into subdirectory.
   - Verified recursive deletion modal and safe removal.
   - Verified error context retention on collision with stale view rejection.
   - *Screenshots:* `screenshot-25-create-folder-modal.png`, `screenshot-26-upload-overwrite-modal.png`, `screenshot-27-move-file-modal.png`, `screenshot-28-delete-file-modal.png`, `screenshot-29-stale-view-context-retention.png`.

2. **UI Scenario: History & Restore (`node scripts/orbit_ui_test.mjs --scenario history`):**
   - *Result: PASS.*
   - Verified file history timeline in drawer with CAS availability badges.
   - Verified deleted files index with strike-through path and metadata.
   - Verified conditional restore authoring a fresh causal version and restoring file to active sync.
   - *Screenshots:* `screenshot-30-file-history-drawer.png`, `screenshot-31-deleted-files-view.png`.

3. **UI Scenario: Needs Attention & Conflicts (`node scripts/orbit_ui_test.mjs --scenario attention`):**
   - *Result: PASS.*
   - Verified Needs Attention triage grouping (conflicts, roots, tasks, health, storage).
   - Verified human review notice banner.
   - Verified candidate conflict heads with peer device attribution and timestamps.
   - Verified tombstone badge on edit-delete conflict.
   - Verified preview and switching across Select Winner, Keep Copies, and Manual Merge tabs.
   - Verified conflict resolution submission and removal from attention list.
   - Verified storage GC trigger button execution.
   - *Screenshots:* `screenshot-32-needs-attention-view.png`, `screenshot-33-conflict-resolve-modal.png`.

4. **Model Consistency Suite (`make test-model`):**
   - *Result: PASS (0.258s).*
   - All reference set oracles, membership rollout, competing administration forks, bounded exhaustive schedules, and golden history fixtures passed.

5. **Fault Injection Suite (`make test-faults`):**
   - *Result: PASS (1.042s).*
   - All recovery journal boundaries, transfer kill/restart boundaries, storage barriers, checkpoints, GC boundaries, control resolution boundaries, quarantine repair, and Invariants I01–I20 passed.

6. **Concurrency & Race Detection (`make test-race`):**
   - *Result: PASS.*
   - All Go packages built and tested under `-race` with 0 race detector warnings.

7. **Full Verification Gate (`make check`):**
   - *Result: PASS (Exit code 0).*
   - Full test suite, python documentation link checks, cross-compilation (`linux/amd64`, `linux/arm64`), and release packaging (`.tar.gz`, `.deb`, `.rpm`) passed.

---

## 4. Acceptance Criteria & Requirements Traceability

| Requirement / Criterion | Status | Evidence / Verification |
| :--- | :--- | :--- |
| **U07: Conflicts & Attention** | Satisfied | Needs Attention grouped view; conflict resolution with select winner, keep copies, and manual merge; tombstone badges on edit-delete conflicts; human review context notice. Screenshots 32 & 33. |
| **U10: Workspace Mutations** | Satisfied | Toolbar & selection actions: upload, overwrite warning, new folder, move/rename, delete with recursive checkbox. Screenshots 25–28. |
| **U11: Device & Version Attribution** | Satisfied | History timeline and conflict resolution cards display peer device names and authored timestamps for human context. Screenshot 30 & 33. |
| **U15: Recovery & Maintenance** | Satisfied | Deleted files restore flow; paused root revalidation; failed task retry/cancel; manual GC trigger. Screenshots 31 & 32. |
| **I03: Causal History Preservation** | Verified | Restores author a new causal version referencing current heads; conflicts preserve all branch heads until explicit resolution. Model and fault suites pass. |
| **I16: Idempotency Tracking** | Verified | Mutating operations use durable idempotency keys; stale view rejections retain operator context. Screenshot 29. |
| **I19: CLI & UI Parity** | Verified | CLI subcommands `orbit restore`, `orbit conflicts`, `orbit import`, `orbit mkdir`, `orbit move`, `orbit delete` match UI operations and share backend control handlers. |
| **I26: Concurrent Modification Race** | Verified | Retains source files on concurrent edit race; displays clear diagnostics. |
| **I27: Explicit State Badges** | Verified | Distinct badges for Available, Expired, Syncing, Paused, and Tombstone. No false green checks. Screenshots 20, 30, 31, 33. |

---

## 5. Outstanding Limitations & Next Steps

1. **Packet O11:** Proceed to [Packet O11: Unified search, sorting, filtering and responsive layout](../../implementation/orbit-devices-files.md#o11--unified-search-sorting-filtering-and-responsive-layout).
2. **P17 Owner Pilot:** P17 owner pilot, use-testing, and unaided explanation work remains outstanding as scheduled.
