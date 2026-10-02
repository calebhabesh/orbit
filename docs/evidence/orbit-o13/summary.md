# Packet O13 Evidence Summary: Failure Campaign, Usability Pilot, and Final Handoff

**Packet:** O13  
**Status:** `complete`  
**Requirements Satisfied:** U01–U16, S01–S22.  
**Invariants Verified:** I01–I28 (full causal DAG monotonicity, crash consistency, authorization containment, CAS integrity, safe GC read leases, bounded lifecycle pruning).  
**Commit:** `0a16d84c8e2acf8d5d0f712f42663aa292c42e29`  
**Target Release Version:** `1.0.0` (pure-Go SQLite, zero Node.js runtime, schema version 13, config format version 1).

---

## 1. Executive Summary & Verification Overview

Packet O13 concludes the Orbit personal file manager revamp, validating the system across all 10 minimum product scenarios, comprehensive resource scaling bounds, end-to-end browser workflows, and regression gates.

The verification campaign confirmed that:
1. **Zero Node.js / Python Runtime Requirement (U01, S22):** The desktop and headless packages embed all frontend web assets into statically-linked Go binaries. Pure-Go SQLite (`modernc.org/sqlite`) is utilized throughout without CGO requirements.
2. **Deterministic Causal Synchronization (U02, U03, S01–S11):** Concurrent offline mutations survive in the causal DAG as concurrent heads without silent last-write-wins data loss (Invariant I03). Linear membership transitions are cryptographically authenticated (Invariant I15, I24).
3. **Workspace & Navigation UX (U04–U10, S12–S17):** Deep directory browsing, breadcrumb stacks, substring search, status distinctions (pending, conflict, blocked, saved), and atomic file operations (mkdir, upload, move, delete) operate with bounded pagination and robust stale-subtree invalidation.
4. **Resilience, Recovery, and Storage (U11–U16, S18–S21):** Active read leases protect CAS chunks from concurrent garbage collection (Invariant I25). Missing chunks are diagnosed honestly without synthetic substitution (Invariant I18). Stopped backup restores enforce identity rotation and counter resets to prevent causal counter collisions (Invariant I08).
5. **CLI & Web Parity (I19):** All operations exposed via the web console share identical underlying control engine operations and validations with the CLI.

---

## 2. Minimum Product Scenarios Verification (Scenarios 1–10)

The 10 end-to-end minimum product scenarios specified in [`docs/implementation/orbit-release.md`](file://<repo>/docs/implementation/orbit-release.md#o13--failure-campaign-usability-pilot-and-final-handoff) were validated via the automated campaign suite ([`tests/integration/orbit_o13_campaign_test.go`](file://<repo>/tests/integration/orbit_o13_campaign_test.go)) and browser test runner ([`scripts/orbit_ui_test.mjs`](file://<repo>/scripts/orbit_ui_test.mjs)):

| Scenario | Objective & Target Boundaries | Automated Test Suite | Result |
| :--- | :--- | :--- | :--- |
| **01** | **Fresh Install & Workspace Creation:** Default folder init, singleton launch reuse, nonempty alternate-root preview, system root rejection (`/etc`), single-owner device labeling. | `TestOrbitO13_Scenario01_FreshInstall_LaunchReuse_Adoption_DisallowedRoot` | **PASS** |
| **02** | **Enrollment, Approval, and Catch-up:** UI invitation token generation, Ed25519 signature proof of possession, decline flow, expired token rejection, forged signature rejection, linear membership rev 2 minting, and sequential catch-up for offline peer. | `TestOrbitO13_Scenario02_Invitation_Approval_Expiry_Rollout_CatchUp` | **PASS** |
| **03** | **Existing-Content Join & Hub Forwarding:** Multi-device sync (A → Hub → B) without direct A-B connection, preexisting file preservation on joining node (Invariant I22), accurate peer replica status tracking. | `TestOrbitO13_Scenario03_ExistingContentJoin_HubForwarding_ReplicaStatus` | **PASS** |
| **04** | **Nested Browse, Search & Mutations:** Deep directory navigation, substring search across hierarchy, destination collision rejection, stale subtree invalidation (token mismatch), journaled move and delete. | `TestOrbitO13_Scenario04_NestedBrowseSearchPreview_JournaledMutations_Races` | **PASS** |
| **05** | **Offline Concurrency & Restore:** 3 concurrent offline edits, head preservation, human-reviewed selection resolution, late partitioned arrival remaining concurrent, conditional historical restore with monotonic author counter. | `TestOrbitO13_Scenario05_ThreeOfflineEdits_ReviewedResolution_LateArrival_Restore` | **PASS** |
| **06** | **Session Security & Lifecycle:** Local `control.token` bootstrap authorization, single-use bootstrap token burn, browser session cookies, CSRF protection on mutating POSTs, background daemon sync continuity across logout, singleton socket lock. | `TestOrbitO13_Scenario06_SessionSecurity_ServiceRestart_SingletonLock_HeadlessCLI` | **PASS** |
| **07** | **Safety, Read Leases & Pruning:** Honest `CONTENT_UNAVAILABLE` on missing chunk, active read lease pin protecting unlinked chunk from GC (Invariant I25), GC reclamation after lease release, safe bounded task pruning preserving exhausted diagnostic tasks (Invariant I28). | `TestOrbitO13_Scenario07_MissingChunkDiagnosed_ReadLease_SafePruning` | **PASS** |
| **08** | **Device Retirement & Stopped Restore:** Permanent device retirement in linear membership chain (cannot rejoin, Invariant I24), stopped SQLite backup restore (`VACUUM INTO`), device identity rotation and `next_counter = 0` reset (Invariant I08), interrupted recovery detection. | `TestOrbitO13_Scenario08_LostDeviceRetirement_ReplacementKey_StoppedBackupRestore` | **PASS** |
| **09** | **Legacy Adoption & Rollback Refusal:** Migration from authentic legacy schema 5 to schema 13, future schema 14 rollback refusal (`ErrIncompatibleSchema`), zero external runtime dependency, embedded asset digest freshness. | `TestOrbitO13_Scenario09_LegacySchemaAdoption_RollbackRefusal_UninstallDataPreservation` | **PASS** |
| **10** | **Accessibility & Unusual Filenames:** Unicode characters, emoji (`🚀`), spaces, brackets, dots in filenames, deep path search, keyboard accessibility (Enter/Escape/focus rings), responsive narrow-viewport layouts (375x667). | `TestOrbitO13_Scenario10_KeyboardAccessibility_UnusualFilenames_ResponsiveLayout` | **PASS** |

---

## 3. Resource Scaling & Performance Measurements

Resource scaling benchmarks were executed to verify memory bounds and query scaling on 10,000 files ([`internal/repository/orbit_browse_test.go`](file://<repo>/internal/repository/orbit_browse_test.go#TestOrbitBrowse_ScalingTenThousandFiles)):

- **Synthetic Fixture Creation:** 10,000 files across 100 directories populated in **439.5 ms** (22.7 files/ms).
- **Directory Browse Query (Root Page, 50 items):** Executed in **110.4 ms**; payload size: **8,754 bytes**.
- **Deep Subdirectory Browse Query (50 items):** Executed in **170.6 ms**.
- **Full Workspace Substring Search:** Scanned 10,000 items in **110.7 ms** with 50 items returned per page.
- **Memory Consumption:** Total heap allocated during scaling queries was **0.59 MB**.
  - Confirms the UX and control engine operate with $O(\text{page size})$ memory rather than $O(\text{total workspace})$ memory.
- **Large Content Streaming:** 64 MiB stream transfer evaluated via chunked CAS streaming with memory overhead bounded at **~1.7 MiB** (well below the 12 MiB limit).
- **Durable Task Pruning:** Evaluated bounded lifecycle pruning (`PruneLifecycleRecords`) across durable work tasks. Completed tasks are purged while exhausted diagnostic tasks are preserved.

---

## 4. End-to-End Browser UI Scenario Suite

The Puppeteer test suite ([`scripts/orbit_ui_test.mjs`](file://<repo>/scripts/orbit_ui_test.mjs)) executed all 10 browser scenarios and 38 screenshot checkpoints against an active test daemon:

1. `setup` (screenshots 1–7): Setup wizard, custom root path preview, adoption confirmation, device labeling.
2. `pairing` (screenshots 8–13): Invitation modal, QR code presentation, join request submission, manual approval.
3. `devices` (screenshots 14–16): Device details, reachable probe, unreachable endpoint error display, safe retirement preview.
4. `browse` (screenshots 17–21): 10,000 files fixture, deep breadcrumbs, search pagination, list/grid toggle, status distinctions (pending, conflict, blocked, saved), mobile responsive view (375x667).
5. `previews` (screenshots 22–24): Text preview, PNG raster preview, non-previewable fallback notice, exact download, historical version timeline.
6. `file-actions` (screenshots 25–29): Create folder, upload with overwrite collision modal, move/rename, delete confirmation, mutation error context retention.
7. `history` (screenshots 30–31): File version history drawer, deleted files view, restore file flow.
8. `attention` (screenshots 32–33): Needs attention queue, conflict resolution modal, side-by-side diff preview, manual merge.
9. `settings` (screenshots 34–36): Storage accounting breakdown (working files, CAS objects, metadata, overhead, free space), retention policy modal, unregister workspace warning.
10. `recovery` (screenshots 37–38): Unavailable root diagnostics, revalidation button, lost device runbook guide.

---

## 5. Verification Gate Summary

- **Packaging Pipeline (`make package`):** Generated 8 reproducible release packages (`.tar.gz`, `.deb`, `.rpm`) across `linux/amd64` and `linux/arm64` with `release-manifest.json` and `dist/SHA256SUMS`.
- **Full Verification Suite (`make check`):** PASS (Exit code 0; formatting, vet, unit, integration, model, faults, fuzz, arm64 cross-compile, packages).
- **Concurrency Race Detector (`make test-race`):** PASS (Exit code 0; 0 data races detected across all Go packages).
- **Multi-Process Local Replication Demo (`make demo`):** PASS (Authenticated mutual TLS, verified chunk transfer, partition simulation, concurrent edit preservation, reviewed resolution, and clean teardown).
- **Native User-Service Lifecycle (`service_lifecycle.py`):** PASS (Local test passed).

---

## 6. Personal Owner Pilot (P17) & System Explanation

### 6.1 Pilot Scope & Test Safety Boundaries
- **Disposable Roots Only:** Automated integration tests and failure campaigns run strictly against disposable directories (`/tmp/...`). The harness refuses any action against personal directories.
- **Dedicated Pilot Workspace:** The live personal owner pilot must run within a dedicated directory (e.g. `/home/caleb2002/FileSyncPilot-20261001/data`) with no active fault injection.
- **Execution Checklist for Owner:**
  1. Install binary via `install.sh --user` or unpack `orbit-v1.0.0-linux-amd64.tar.gz`.
  2. Run `orbit launch` to launch the background daemon and open the web console.
  3. Complete initial wizard selecting the pilot folder.
  4. Perform normal edits, offline edits across disconnected periods, and verify conflict surfacing in the "Needs Attention" tab.
  5. Test `orbit maintenance backup` and verify safe stopped restore.

### 6.2 Owner Distributed System Explanation Notes
- **Causal State Model:** Changes are recorded as immutable version envelopes forming a directed acyclic graph (DAG) per path. Concurrency is detected by comparing vector clocks; conflicting heads survive concurrently until human review.
- **Zero Silent Data Loss:** The engine never uses wall-clock timestamps or device heuristics to silently choose a winning version. All concurrent edits are surfaced explicitly.
- **Content-Addressable Storage (CAS):** File contents are divided into chunks and indexed by SHA-256 digests. Chunks are deduplicated across versions and files. Active read leases and content pins protect chunks from garbage collection while in flight or being inspected.
- **Linear Membership Revisions:** Workspace membership is organized into cryptographically signed, linearly incremented revisions. Retired devices are permanently tombstoned in the membership log and cannot rejoin under the same key identity.

---

## 7. Limitations & Unexecuted Checks

In accordance with release integrity rules, items not executed in this environment remain explicitly labeled:

1. **Physical Bare-Metal Hardware Cut:**
   - **Label:** `unexecuted` (in physical hardware lab).
   - **Coverage Provided:** Simulated via in-process SIGKILL, crash consistency boundary audits (`tests/faults/`), and SQLite transaction rollback tests.
2. **Physical Multi-Node Hardware (Raspberry Pi native execution):**
   - **Label:** `unexecuted` (on physical ARM board).
   - **Coverage Provided:** Cross-compiled binary `bin/filesync-linux-arm64` built with `GOARCH=arm64` and verified in Debian/RPM packaging pipeline.
3. **P17 Owner Live Data Directory Execution:**
   - **Label:** `unexecuted` (by assistant).
   - **Coverage Provided:** End-to-end integration and browser tests verified against disposable fixtures; live personal pilot reserved for the owner's personal evaluation.
