# Orbit Packet O08 Summary: File Browser, Previews, and Details UI

**Packet:** O08  
**Date:** 2026-10-02  
**Status:** `complete`  
**Prerequisites:** O04, O07 complete. Read [vision](../../orbit-product.md), [architecture](../../orbit-architecture.md), [protocol](../../protocol.md), [operations](../../operations.md), [verification](../../verification.md), and [orbit devices & files](../../implementation/orbit-devices-files.md#o08--file-browser-previews-and-details-ui).  
**Requirements Satisfied:** U07 (responsive shell with 5 primary destinations), U10 (browse, search, sorting and presentation), U11 (file details, preview, version timeline, download), U15 (honest qualified progress, replica progress, and distinct status badges).  
**Invariants Verified:**
- **I19 (CLI & UI Parity):** UI and CLI use identical control operations; frontend handlers query `/api/v1/browse`, `/api/v1/search`, `/api/v1/browse/file-details`, `/api/v1/browse/history`, `/api/v1/browse/deleted`, `/api/v1/content`; no separate reconciliation or synthetic state invented in UI.
- **I25 (Bounded Memory Streaming & Previews):** Inline previews enforce strict size budgets (128 KB text, bounded raster dimensions); file downloads stream directly from authenticated `/api/v1/content` as native attachments without assembling arbitrary whole-file blobs in browser memory.
- **I27 (Status Distinctions & Honest Recovery):** Pending content, active conflicts, blocked paths, and saved complete files are rendered with distinct, non-overlapping visual badges and explicit diagnostic explanations. Blocked, pending, and conflicting files never appear as normal complete files.

---

## 1. System Overview

Orbit Packet O08 delivers the user-facing file manager, deep hierarchical navigation, backend search across workspace files, list/grid presentation toggles, responsive narrow-window support, and the context-preserving `FileDetailsDrawer` side panel. It connects the authenticated HTTP endpoints and bounded streaming reader implemented in O07 directly into the embedded React 19 web interface.

### Core Architecture & Implementation Decisions
1. **Hierarchical Directory Browsing (`FilesView.tsx`):** Immediate directory children are browsed with parent path traversal, deep breadcrumbs trail (`Root / documents / work / projects / alpha`), navigation stack (`←`, `→`, `↑ Up`), and column sorting (`name`, `size`, `mtime`).
2. **Backend Search Integration (`api.search`):** Search input in the topbar triggers literal workspace path search via `/api/v1/search` with bounded pagination (50 items/page) and "Load More" cursor advancement across 10,000 files.
3. **Request Sequencing Protection (`reqSeq.current`):** Navigation, sorting, and search query changes increment a local request sequence counter. Obsolete asynchronous responses arriving out of order are discarded immediately, preventing newer views from being replaced by older responses.
4. **Context-Preserving Side Panel (`FileDetailsDrawer.tsx`):** Selecting any file opens a slide-over drawer with three tabs:
   - **Details:** Full relative path, human size, working copy status, conflicting head comparison (when multiple heads exist), and device replica copies (`peer_progress`).
   - **Preview:** Bounded UTF-8 plain text preview (`<pre>`), raster image rendering (PNG/JPEG `<img>` via authenticated bearer URL), and informative fallback notice with download options for non-previewable formats.
   - **History:** Causal version timeline showing every historical version, display timestamp, author name/ID, CAS local availability badges, direct download action, and conditional restoration for historical heads.
5. **Native Streaming Downloads (`api.downloadContent`):** File downloads use native browser downloads via authenticated `GET /api/v1/content?folder=...&version=...`, streaming directly to disk with Content-Disposition headers and per-write idle timeouts without assembling in-memory blobs.
6. **High-Performance Causal History (`internal/history/history.go`):** Added `pathKey` secondary indexing to `History` struct. This reduces `Heads`, `StructuralConflicts`, and `Accept` validations from O(N^2) to O(1) per path, ensuring workspaces with 10,000+ files evaluate conflicts and history in milliseconds without blocking the control server or browser polling loops.

---

## 2. Views and Components Created & Updated

1. **`web/src/views/FilesView.tsx`:** Complete file manager view supporting hierarchical navigation, breadcrumbs, search results, list vs. grid toggle, keyboard navigation (Arrow keys, Enter, Space, Escape, Backspace), visible focus rings, and Invariant I27 status badges.
2. **`web/src/components/FileDetailsDrawer.tsx`:** Multi-tab context-preserving side panel displaying technical metadata, exact version download, plain text / raster previews, remote peer copy progress, and historical version timeline.
3. **`web/src/views/DeletedFilesView.tsx`:** Paginated tombstone index backed by `/api/v1/browse/deleted` with CAS availability indicators and conditional restore.
4. **`web/src/types.ts`:** Strongly-typed contracts for `BrowseItem`, `BrowseResult`, `SearchResult`, `FileDetails`, `FileVersionDetail`, `FileHistoryItem`, `FileHistoryResult`, `DeletedFileItem`, `DeletedFilesResult`.
5. **`web/src/api.ts`:** Client API methods for `browse()`, `search()`, `getFileDetails()`, `getBrowseHistory()`, `getBrowseDeleted()`, `getContentURL()`, `fetchTextPreview()`, and `downloadContent()`.
6. **`web/src/App.tsx`:** Wired topbar search input to `FilesView` search query.
7. **`internal/history/history.go`:** Added `byPath map[pathKey][]VersionID` indexing to `History`, eliminating O(N^2) bottleneck across 10,000 files.

---

## 3. Verification and Evidence

### Planned Checks Execution

1. **Hierarchy, Search & Status Browser Scenario (`--scenario browse`):**
   ```sh
   node scripts/orbit_ui_test.mjs --scenario browse
   ```
   - **Step 1:** Deep directory navigation through `documents/work/projects/alpha`, verifying deep breadcrumbs trail, parent `Up` navigation, direct ancestor click, and navigation history back/forward stack. Captured `screenshot-17-deep-directory-breadcrumbs.png`.
   - **Step 2:** Search across 10,000 files for `item_42`. Verified initial 50 bounded results, clicked "Load More" to load all 100 matching items, cleared search to restore root directory view. Captured `screenshot-18-search-results.png`.
   - **Step 3:** Column sorting toggled by Size, then returned to Name (ascending).
   - **Step 4:** Grid view toggle presentation verified. Captured `screenshot-19-grid-view.png`, returned to List view.
   - **Step 5:** Distinct status badges verified (Invariant I27): "Syncing / Pending content" for pending files, "Needs review (Conflict)" for concurrent heads, "Blocked: unsupported path attribute" for blocked items, and "Saved on this device" for normal synced files. Captured `screenshot-20-status-distinctions.png`.
   - **Step 6:** Keyboard navigation verified (ArrowDown focus outline, Enter opens drawer, Escape dismisses drawer).
   - **Step 7:** Narrow viewport (375x667 mobile) verified with mobile toggle and responsive table overflow. Captured `screenshot-21-browse-narrow-window.png`.

2. **Previews, Exact Downloads & History Scenario (`--scenario previews`):**
   ```sh
   node scripts/orbit_ui_test.mjs --scenario previews
   ```
   - **Step 1:** Plain text preview of `readme.md` loaded in `<pre>` inside `FileDetailsDrawer`. Captured `screenshot-22-text-preview.png`.
   - **Step 2:** Raster PNG preview of `logo.png` rendered with complete image dimensions. Captured `screenshot-23-image-preview.png`.
   - **Step 3:** Non-previewable binary ELF format (`program.bin`) verified with inline fallback notice and download options.
   - **Step 4:** Exact version download action verified via authenticated native stream.
   - **Step 5:** Historical versions timeline verified with Head indicator and CAS local availability badges. Captured `screenshot-24-history-timeline.png`.
   - **Step 6:** Device replicas and copy progress verified for remote peer `Laptop-B` ("Updated on device").

3. **Full Regression and Verification Suite:**
   - `node scripts/orbit_ui_test.mjs --scenario all`: All 5 scenarios passed (O04 Setup, O06 Pairing, O06 Devices, O08 Browse, O08 Previews).
   - `make check`: All unit, integration, model, fault, package build, and validation tests passed cleanly.
   - `make test-race`: All Go packages passed under `-race` with zero data races.

---

## 4. Screenshot Evidence Catalog

| Screenshot | Description | Acceptance Mapping |
| --- | --- | --- |
| `screenshot-17-deep-directory-breadcrumbs.png` | Deep directory navigation into `documents/work/projects/alpha` showing full breadcrumbs trail and parent navigation | Deep directory browse, breadcrumbs, history stack |
| `screenshot-18-search-results.png` | Backend search across 10,000 files for `item_42` with 50-item bounded pagination and Load More | Search scaling, bounded pagination, clear query |
| `screenshot-19-grid-view.png` | Fluid responsive Grid view presentation with directory and file cards | List vs. Grid presentation toggle |
| `screenshot-20-status-distinctions.png` | Status badge distinctions for Pending, Conflict, Blocked, and Saved items | Invariant I27: honest status distinctions |
| `screenshot-21-browse-narrow-window.png` | Responsive mobile viewport (375x667) file browser layout | Responsive narrow-window support |
| `screenshot-22-text-preview.png` | Plain text preview rendered inside `FileDetailsDrawer` | Bounded plain text preview |
| `screenshot-23-image-preview.png` | Raster PNG image preview rendered inside `FileDetailsDrawer` | Raster image preview |
| `screenshot-24-history-timeline.png` | Historical version timeline with Head badge and CAS availability indicators | History timeline, exact download, copy state |

---

## 5. Limitations and Unexecuted Scope

1. **O09 File Operations:** File creation, upload/import streaming, rename, move, and deletion dialogs/actions are part of Packet O09 and remain **unexecuted** here.
2. **O10 Destructive Action Review:** Destructive action confirmations, bulk delete safeguards, and in-drawer conflict resolution actions belong to Packet O10.
3. **P17 Pilot & Explanation:** P17 owner-use testing and unaided explanation work remain open.
