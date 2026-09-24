# Packet P13 Verification Summary: Complete Operator Control and Diagnostics

## Summary of Implementation

Packet P13 finishes operator controls, observability instrumentation, diagnostic tooling, and security enforcement across both the local CLI and the loopback HTTP control server:

1. **Authenticated Loopback Control Server (`internal/control/server.go`)**:
   - Loopback HTTP listener (`127.0.0.1:<port>`) with strict Host header verification against DNS rebinding attacks.
   - Strict Origin header checks ensuring cross-origin web pages cannot execute ambient-credential mutations (Invariant **I16**).
   - Local CLI Bearer authentication via private file `control.token` (mode `0600`).
   - Browser authentication flow: short-lived, one-use bootstrap tokens exchanged via `POST /api/v1/auth/bootstrap` for `HttpOnly`/`SameSite=Strict` session cookies plus unique per-session CSRF tokens required on all state-mutating HTTP methods.
   - Replay protection: burned bootstrap tokens are invalidated immediately upon first use.

2. **Operator Diagnostics & Doctor Engine (`internal/control/doctor.go`)**:
   - `Doctor(ctx)` inspects:
     - Identity key permissions (`peer-identity.pem`, mode `0600`).
     - State directory permissions (mode `0700`).
     - Root directory availability, mount status, and private scratch markers (`registration.json`).
     - Storage capacity: 512 MiB free space reserve and 256 MiB WAL soft admission cap.
     - Cluster membership bounds (16 active members maximum) and tombstone snapshot consistency.
     - Schema and wire protocol version compatibility.
     - Pending recovery items (in-flight tasks, uncommitted proposals, orphaned chunks).

3. **Sanitized & Redacted Support Export (`internal/control/support.go`)**:
   - Generates a local compressed tar archive (`tar.gz`) containing comprehensive diagnostic state: `manifest.json`, `doctor.json`, `system.json`, `config.json`, `membership.json`, `storage.json`, `work.json`, `events.json`, `metrics.json`.
   - Never uploads data automatically; stays local for owner review.
   - Invariant **I15** strictly enforced: all private keys, session tokens, passwords, and file contents are completely excluded.
   - Deterministic path pseudonymization (`path_<hash>.ext`) active by default.

4. **Safe Folder Lifecycle & Deletion Invariant (`internal/control/maintenance.go`, `internal/repository/workspace.go`)**:
   - Folders can be paused, resumed, revalidated, and unregistered via CLI and API.
   - Invariant **I09** / Requirement: Unregistering a folder clears database tracking records while leaving all files intact on disk, and authors **0** deletion tombstones into the replication graph.

5. **Maintenance & Safe Identity Reset (`internal/control/maintenance.go`)**:
   - `backup`: Consistent SQLite database backup using WAL snapshotting.
   - `check`: Validates SQLite schema version against binary `CurrentSchema`.
   - `recovery`: Inspects interrupted work tasks, quarantined chunks, and uncommitted publication staging.
   - `reset-identity`: Safely clears prior local author vector counters and assigns a new device ID and keypair when restoring an old metadata backup, guaranteeing that rolled-back counters can never cause silent remote data loss (Invariant **I19**).

6. **8 Comprehensive Operator Runbooks (`docs/runbooks/`)**:
   - `full-disk.md`, `root-unavailable.md`, `retired-lost-device.md`, `corrupt-content.md`, `stuck-conflict.md`, `expired-history.md`, `incompatible-versions.md`, `database-recovery.md`.

---

## Owner Explanation Note

### How Stored, Applied, Offline, Unknown, and Conflicted States Differ for the Same Path

In an eventually-consistent content-addressed distributed filesystem, a single path cannot be represented by a simple scalar state. The sync engine distinguishes five orthogonal state dimensions for any given path:

```
+---------------------------------------------------------------------------------------------------+
|                                      LIFECYCLE OF A PATH VERSION                                  |
|                                                                                                   |
|  [ Remote Peer ]                                                                                  |
|        |                                                                                          |
|        | (transfer chunks)                                                                        |
|        v                                                                                          |
|  +---------------+  (atomic two-phase rename)   +---------------+                                 |
|  |    STORED     | ---------------------------> |    APPLIED    | <--- Authoritative Working Basis|
|  | Object Store  |                              | Filesystem    |                                 |
|  +---------------+                              +---------------+                                 |
|        |                                                |                                         |
|        | (concurrent branch: v1 || v2)                  | (local user edits)                      |
|        v                                                v                                         |
|  +---------------+                              +---------------+                                 |
|  |  CONFLICTED   |                              |  NEW VERSION  |                                 |
|  | Needs Review  |                              | (Scanned)     |                                 |
|  +---------------+                              +---------------+                                 |
+---------------------------------------------------------------------------------------------------+
```

1. **Stored State**
   - **Definition**: The complete cryptographic manifest and all constituent 1 MiB chunk payloads for a version at this path have been transferred, cryptographically verified against SHA-256 digests, and durably written into the local content-addressed object store (`repo.sqlite` objects and `.filesync-scratch/staging/`).
   - **Distinction**: The bytes are verified and durable on local disk, but have **not** been placed into the user's mutable working tree directory. A version is "stored" when a background download finishes, but publication is awaiting scheduling, or is blocked by an unresolved conflict.

2. **Applied State**
   - **Definition**: A specific stored version has been published into the workspace directory via two-phase crash-safe atomic rename (`.filesync-scratch/stage -> path`), the directory parent has been fsynced, and the local `path_projections` table records this version as the authoritative working basis.
   - **Distinction**: The user can open and edit the actual plaintext file on disk. Observed file metadata (device, inode, size, mtime, and mode) match the projection record. Only one version can be the applied basis for a path at any given moment.

3. **Offline State**
   - **Definition**: A peer participating in the folder's canonical membership cannot be reached over the network (e.g. laptop lid suspended, VPS unreachable, network partition).
   - **Distinction**: The offline peer's stored/applied receipts are frozen at the timestamp of last contact. The local node knows which versions were delivered up to the disconnect, but cannot push new versions or query the peer's current disk state until connectivity is re-established.

4. **Unknown State**
   - **Definition**: The local node lacks direct knowledge of whether a peer has stored or applied a particular version.
   - **Distinction**: Distinct from "offline": the peer may be actively connected and exchanging other folders or versions, but has not yet transmitted an inventory cursor or receipt for this path. Crucially, in our decentralized model, **receipts are non-transitive**: a VPS having stored a version does not imply that a home Raspberry Pi has stored it. Until a direct receipt is received from the Pi, the Pi's state for that path is strictly "unknown."

5. **Conflicted State**
   - **Definition**: Two or more concurrent versions exist for the same path in the version history graph whose vector clocks are mutually incomparable ($v_1 \parallel v_2$), OR a structural conflict exists (e.g., file vs directory collision).
   - **Distinction**: Both versions may be fully *stored* in the object store, but neither can be safely *applied* as the sole working copy. The sync engine flags the path as a conflict head, preserving both versions in metadata, suppressing automatic publication, and presenting the conflicting heads to the operator for deterministic resolution (`filesync resolve select`, `keep-copies`, or `merge`).

---

## Acceptance Criteria and Evidence Matrix

| Criterion | Requirement / Invariant | Status | Evidence |
| :--- | :--- | :--- | :--- |
| Browser cross-origin mutation rejected | S13, I16 | PASS | `TestP13LoopbackControlSecurityAndBootstrap`: Origin header from external site returns 403 Forbidden |
| Unauthenticated sensitive reads fail | S13, I16 | PASS | `TestP13LoopbackControlSecurityAndBootstrap`: GET `/api/v1/doctor` without auth returns 401 Unauthorized |
| CLI mutations match browser operations | S13 | PASS | `TestP13CLIDiagnosticsAndDoctor`: CLI and HTTP API share identical `control.Controller` operations |
| Support export sanitization & redaction | S16, I15 | PASS | `TestP13SupportExportSanitizationAndRedaction`: Archive contains 0 keys, 0 tokens, 0 plaintext data; paths pseudonymized |
| Folder unregister authors 0 tombstones | S19, I09 | PASS | `TestP13FolderLifecycleAndZeroReplicatedDeletes`: Disk files preserved; 0 deletion tombstones authored |
| 15 stable error categories with next actions | S21 | PASS | `TestP13ErrorCategoriesHaveSafeNextActions`: All 15 error codes verified with actionable remediations |
| 8 Operator Runbooks Created | S13, S20 | PASS | `docs/runbooks/` contains all 8 complete markdown operational guides |
