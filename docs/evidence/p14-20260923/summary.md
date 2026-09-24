# Packet P14 Summary: Focused Embedded Web Interface

**Packet:** P14
**Date:** 2026-09-23
**Status:** `complete`
**Prerequisites:** P13 complete. Read [scope](../../portfolio-scope.md), [operations](../../operations.md), [verification](../../verification.md), and [04-delivery](../04-delivery.md).
**Requirements Satisfied:** S08 (reviewed conflict actions), S16 (qualified status), S18 (automatic agent/shared UI).
**Invariants Verified:** I16 (control authentication, loopback defense, CSRF, stale-view rejection), I19 (UI and CLI use the same operations, display qualified progress, safe recovery).

---

## 1. System Overview

Packet P14 delivers a focused, accessible, and self-contained web user interface for File Sync operators, built entirely with React 19, TypeScript, and Vite 8, and embedded directly inside the compiled Go binary using `//go:embed all:dist`.

### Core Operational Principles
1. **Zero Runtime Node.js Dependency:** Production binaries embed all compiled HTML, CSS, JavaScript, and SVG assets. At runtime, the agent serves the full SPA directly over loopback HTTP with zero external process or Node dependencies.
2. **Decoupled Operator Session:** Closing the browser window or terminating a browser session does not interrupt file watching, continuous replication, background reconciliation, or scheduled garbage collection.
3. **Exact Parity with CLI Engine:** The web UI does not invent separate reconciliation logic or execute business logic client-side; every user interaction delegates to the exact same `/api/v1/...` control endpoints exercised by the `filesync` CLI commands.
4. **Honest Qualified Progress:** The web UI explicitly reflects the decentralized protocol realities—network progress is displayed only from direct peer receipts (never assuming transitivity), content states clearly differentiate `ready`, `pending`, `unavailable`, and `expired`, and storage warnings flag 512 MiB reserves and 256 MiB WAL caps.
5. **No Independent Conflict Winners in UI:** The frontend never decides a conflict winner; it presents the reviewed heads and cryptographic `head_token` to the operator, requiring the backend engine to atomically validate the token and commit a resolution envelope extending the causal DAG.

---

## 2. Views and Capabilities

The interface provides three focused functional views, an operator bootstrap authentication flow, and diagnostic modals:

### A. Bootstrap Authentication View
- Accessible when an unauthenticated browser navigates to the loopback control listener (`GET /`).
- Prompts for a 64-character one-time bootstrap token generated via `filesync control bootstrap-token`.
- Upon submission, exchanges the bootstrap token for an `HttpOnly`, `SameSite=Strict` session cookie and extracts the per-session CSRF token.
- Burning the one-time token prevents replay attacks; unauthenticated or cross-origin requests are rejected with `401 Unauthorized` or `403 Forbidden`.

### B. Folders & Work View
- **Registered Folders Table:** Displays folder ID, local workspace root path, operational status (`ACTIVE` vs `PAUSED`), scan generation, membership revision, and device/inode persistence numbers.
- **Folder Lifecycle Actions:**
  - `Pause`: Pauses automatic background sync with operator reason.
  - `Resume`: Resumes background reconciliation.
  - `Revalidate`: Re-checks root mount status and private scratch marker (`registration.json`).
  - `Run GC`: Manually triggers historical chunk garbage collection.
  - `Remove`: Safely removes the database registration while leaving all working copy files completely intact on disk (**I09**, authoring zero deletion tombstones).
- **Devices & Qualified Progress:** Summarizes cluster peers, membership revision, and cryptographic membership digest, clearly stating that progress is based strictly on direct receipts.
- **Durable Work Queue:** Shows pending, running, retry, and exhausted background tasks (scans, syncs, repairs, GCs), with attempt counters, retry times, and cancellation triggers.
- **Storage Reserve & Utilization:** Visual cards showing object store bytes, staging bytes, quarantine bytes, WAL bytes, and metadata database bytes.

### C. Files & History View
- **Active Workspace Projections:** Lists all active files tracked in the folder, displaying clean file basenames, formatted sizes, kind badges (`FILE` vs `DIR`), executable status (`+x`), and projection health (`OK` vs `BLOCKED`).
- **Live Search Filter:** Instant client-side path filtering.
- **Version History Drawer:** Opening history on any file reveals its complete causal lineage:
  - Cryptographic version ID (`author:counter`).
  - `HEAD` and `APPLIED` badges.
  - Timestamp of authored change.
  - File size and executable flag.
  - **Content Availability Badges:** `READY` (chunks locally verified and stored), `PENDING` (download scheduled), `UNAVAILABLE` (missing from local objects), and `EXPIRED` (reclaimed under retention policy).
- **Historical Restore Modal:**
  - Allows previewing historical restoration before committing.
  - Shows target file path, source version, size, and current reviewed heads to supersede.
  - Disables restore action if source content is `EXPIRED` or `UNAVAILABLE`.
  - Explains forward-causal semantics: restoration creates a new causal version in the present referencing current heads as parents, adopting historical content without rolling back causal counters.

### D. Conflicts View
- **Causal Multi-Head Conflicts:** Lists paths having multiple concurrent heads in the DAG with conflict kind (e.g. `edit-edit`, `edit-delete`), current head token, and publication suppression status.
- **Structural Conflicts:** Lists collisions between directory deletions and descendant creations (e.g., parent deleted while child created or edited).
- **Interactive Conflict Resolution Modal:** Offers three distinct resolution strategies:
  1. *Select Winner:* Radio selection to pick one reviewed head as the winner, marking other heads superseded.
  2. *Keep Separate Copies:* Dry-run preview of generated side-by-side branch paths (e.g., `file (conflict <author>-<counter>).ext`) preserving all concurrent data without overwriting.
  3. *Manual Merge:* Integrated text editor allowing the operator to author unified merged content with executable bit toggle, creating a forward resolution version extending the DAG.
- **Stale-View Handling (I16):** If concurrent changes arrive from another peer while the modal is open, submitting a resolution returns `409 Conflict` (`STALE_VIEW`). The modal highlights the stale view, reloads current heads, and requires the operator to review the updated set before committing.

### E. Doctor Health Inspection Modal
- Triggered directly from the persistent header badge.
- Runs complete system health inspection:
  - Identity key permissions (`peer-identity.pem`, `0600`).
  - State directory permissions (`0700`).
  - Metadata database permissions (`0600`).
  - Root directory mount status and scratch registration marker.
  - Free disk space reserve target (512 MiB reserve).
  - SQLite WAL file size soft admission cap (256 MiB).
  - Canonical cluster membership bounds (16 active members max).
  - Uncommitted proposals and interrupted task recovery.

---

## 3. Screenshots Captured

All 8 requested screenshots are captured in `docs/evidence/p14-20260923/screenshots/`:
1. `screenshot-01-bootstrap-login.png`: Clean authentication screen with token input and terminal command guidance.
2. `screenshot-02-folders-work.png`: Folders dashboard with active folders, peer progress notice, work queue, and storage usage cards.
3. `screenshot-03-doctor-modal.png`: Doctor inspection modal displaying categorized checks with green OK badges and evaluations.
4. `screenshot-04-files-history.png`: Files view showing workspace files and history drawer with causal heads and content availability.
5. `screenshot-05-restore-preview.png`: Historical restore preview modal showing superseded heads, content readiness, head token, and forward-causal explanation note.
6. `screenshot-06-conflicts-view.png`: Active multi-head causal conflicts and structural conflict table with head tokens and suppression indicators.
7. `screenshot-07-conflict-resolve-modal.png`: Conflict resolution modal with three workflows: Select Winner (with radio buttons), Keep Separate Copies (dry run targets), and Manual Merge.
8. `screenshot-08-mobile-responsive.png`: Responsive layout on 390px mobile viewport with accessible touch targets, flex wrapping, and responsive tables.

---

## 4. Owner Explanation Note

### Which decisions live in the engine and why the UI cannot independently decide a conflict winner

In an eventually-consistent distributed filesystem, all safety, causality, and conflict guarantees are defined by the mathematical properties of the version DAG and the SQLite transactional state machine. Consequently, **every decision affecting causal state lives strictly in the sync engine, not in the user interface.**

#### 1. The UI is a Presentation and Review Tool, Not a Consensus Engine
The web interface (running in an operator's browser) has only a point-in-time, eventually-consistent view of local SQLite state. It does not participate in network gossip, chunk replication, or SQLite transaction serialization. If the UI were permitted to independently "decide" a conflict winner (e.g. by choosing the version with the newest clock timestamp, the largest file, or an arbitrary device heuristic), several fatal invariant violations would occur:
- **Clock Skew and Silent Data Loss:** Physical wall-clock timestamps are inherently unreliable across distributed nodes. An autonomous UI resolution based on "latest mtime" would silently discard legitimate concurrent edits authored on machines whose clocks were slightly behind.
- **Split-Brain Resolutions:** If two operators on different devices had web consoles open simultaneously, their local UIs might select different winners for the same conflict. If the UI authored the decision without causal validation, both nodes would diverge permanently or create recursive conflict storms.
- **Race Windows and Stale Views:** A user viewing a conflict in the browser may spend seconds or minutes inspecting the differences. During that time, a third peer may author an edit that causally supersedes one of the conflicting heads. If the UI could independently force a winner, it would overwrite that fresh, unreviewed edit.

#### 2. Why Resolution Semantics Belong to the Engine
The sync engine enforces deterministic resolution via strict cryptographic primitives:
- **Causal Parents and Clock Dominance:** Resolving a conflict does not mean "deleting" the losing version. It requires creating a **resolution envelope** in the DAG whose parent list explicitly references *all* reviewed concurrent heads (`Parents = [V_A, V_B]`) and whose vector clock merges and advances both lines of history (`Vector = merge(V_A.Vector, V_B.Vector) + local:counter`). Only the engine's database layer can allocate the monotonic author counter and commit this transactional change.
- **Cryptographic Head Token Validation (`STALE_VIEW` Rejection):** When the UI requests a resolution (`select`, `keep-copies`, or `merge`), it is required to send the `expected_head_token`—a cryptographic hash of the sorted IDs of the heads currently on screen. Inside a single SQLite transaction, the engine computes the current head token. If a remote peer delivered a new version while the operator was reviewing, the head token does not match. The engine **rejects** the request with `409 Conflict` (`STALE_VIEW`), preventing stale overwrites (**I16**).
- **Atomic Two-Phase Workspace Publication:** Resolving a conflict in metadata requires atomically publishing the winning content into the working tree. The engine coordinates two-phase staging (`stage -> rename`), validates inode/device boundaries, and handles file-versus-directory structural collisions. The UI has no direct filesystem access; it must rely on the engine's crash-safe publication pipeline.

In summary, the UI's role is strictly limited to **eliciting the operator's reviewed intent**. The engine translates that intent into a cryptographically verified, forward-causal DAG extension.
