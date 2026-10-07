# Orbit Revamp Design Gate Outcomes (G01–G05)

Status: closed by executable models, negative/adversarial tests, and formal specifications on 2026-10-01. Sources are in [`tests/designgates`](../tests/designgates) and [`model/membership.go`](../model/membership.go).

These decisions freeze the operational, protocol, and persistence contracts for packets O02–O12 without altering the core engine's existing guarantees (I01–I20).

---

## G01 — Local Launch, Bootstrap & Session Security

### Decisions and Rules
1. **Singleton and State Directory Ownership**:
   - The Orbit desktop launcher and daemon enforce exclusive ownership via `.agent.lock` in the state directory using [`state.Acquire`](../internal/state/state.go#L53).
   - If a daemon is already active, the launcher detects the existing process and opens the UI through its authenticated local control interface; it never spawns a duplicate daemon or silently creates parallel state directories.
   - Initialized state vs uninitialized state is detected at startup. Uninitialized state serves the setup flow; initialized state serves the file manager.
2. **One-Use Browser Bootstrap Handoff**:
   - The launcher initiates browser sessions via a short-lived (60s TTL), high-entropy (32-byte crypto-random) bootstrap token passed as a URL fragment (`/#bootstrap=<token>`).
   - URL fragments are never sent in HTTP request headers, preventing leakage into web server logs or `Referer` headers.
   - The embedded web client extracts the fragment, immediately strips it using `history.replaceState`, and executes a single-use exchange via `POST /api/v1/auth/bootstrap`.
   - The exchange verifies:
     - Strict loopback `Host` header (`127.0.0.1`, `localhost`, `[::1]`). Non-loopback Host headers are rejected (HTTP 403) to protect against DNS rebinding attacks.
     - Matching loopback `Origin` header.
     - Token has not been consumed and has not expired.
   - Upon successful exchange, the token is permanently invalidated (one-use; replay attempts fail immediately with `ErrBootstrapTokenUsed`).
   - The daemon issues an `HttpOnly`, `SameSite=Strict` session cookie and a CSRF token.
3. **Session vs Daemon Synchronization Lifetime**:
   - Closing the browser window or logging out revokes the UI session cookie and frees control sessions.
   - **Invariant I21**: UI session termination does **not** stop the background daemon or interrupt peer synchronization, folder watching, or scheduled transfers. Stopping the daemon requires an explicit, authenticated shutdown control operation.

### Executable Fixtures and Oracles
- [`TestOrbitG01SingletonExclusiveOwnership`](../tests/designgates/orbit_g01_launch_test.go#L100): Confirms dual acquisition of state lock is rejected with `state.ErrLocked`.
- [`TestOrbitG01OneUseBootstrapHandoff`](../tests/designgates/orbit_g01_launch_test.go#L134): Verifies DNS rebinding rejection, origin checks, one-use replay prevention, and TTL expiration.
- [`TestOrbitG01SessionAndLogoutKeepsSyncActive`](../tests/designgates/orbit_g01_launch_test.go#L185): Verifies session logout revokes UI access while preserving the daemon lock and engine state.

### Rejected Alternatives
- Passing bootstrap tokens in URL query strings (leaks into proxy/server access logs and browser history).
- Storing long-lived administrative API keys in `localStorage` or `sessionStorage` (vulnerable to XSS extraction).
- Auto-terminating the sync daemon when all browser tabs close (would break unattended background peer synchronization).

---

## G02 — Invitation Transport, Enrollment & Sequential Rollout

### Decisions and Rules
1. **Invitation Transport & Capability Scope**:
   - Invitations are issued by an owner-authenticated enrolled device for a specific workspace with bounded lifetime (e.g., 24h) and bounded uses (default 1).
   - The inviter stores only the SHA-256 verifier digest of the token; raw tokens are never persisted in SQLite or log output.
   - An invitation secret provides **only** the capability to submit an unauthenticated join request to `/api/v1/enrollment/request`.
   - **Invariant I23**: Invitation possession alone grants **no access** to workspace files, chunk payloads, directory trees, or control operations.
   - The enrollment endpoint enforces bounded request body sizes (max 16 KiB) and IP-based rate limiting to prevent denial-of-service.
2. **Key Proof of Possession & Explicit Owner Approval**:
   - The joining device generates its own fresh Ed25519 keypair and permanent DeviceID (`SHA256(public_key)`).
   - It submits a JoinRequest containing its public key, suggested display label, and a cryptographic signature over a fresh challenge nonce + token digest + timestamp, proving private key possession.
   - Requests enter a `pending_approval` state.
   - The owner must explicitly review the exact joining Device ID, key pin, suggested label, and target workspace on an enrolled device and approve the request. Invitations never grant automatic admission.
3. **Sequential Membership Rollout & Competing Administration**:
   - Owner approval appends a new device to the linear membership chain: `Revision(N+1)`, with `PriorDigest = Hash(Revision N)`.
   - Peers apply revisions sequentially, verifying monotonicity, prior digest, and authorized signature.
   - **Invariant I24**: If two partitioned administrator devices approve concurrent joins from the same base revision (Revision N), both emit competing revisions with the same `PriorDigest`. When reconnecting, this is flagged as an explicit **Membership Fork** (`ErrMembershipFork`).
   - Conflicting branches are **never silently auto-merged**. Content synchronization across the forked membership is paused until the owner issues an explicit reconciling revision (Revision N+2).
4. **Retirement Irrevocability**:
   - Once a device is retired in a membership revision, it cannot be readmitted or resurrected under the same Device ID. A replaced or wiped device must generate a fresh cryptographic identity.

### Executable Fixtures and Oracles
- [`TestOrbitG02InvitationCreationAndDigestStorage`](../tests/designgates/orbit_g02_enrollment_test.go#L119): Verifies token digest one-way hashing and revocation.
- [`TestOrbitG02CapabilityGatingAndDosLimits`](../tests/designgates/orbit_g02_enrollment_test.go#L155): Verifies that invitation tokens cannot access chunks/inventory, and verifies 16 KiB body bounds and rate limiting.
- [`TestOrbitG02KeyPossessionAndOwnerApproval`](../tests/designgates/orbit_g02_enrollment_test.go#L236): Verifies Ed25519 signature proof of possession, rejection of forged signatures, single-use enforcement, and pending approval quarantine.
- [`TestOrbitG02OfflineRolloutAndCompetingFork`](../tests/designgates/orbit_g02_enrollment_test.go#L295): Models concurrent partitioned approvals, demonstrates fork detection, and verifies explicit owner reconciliation.
- [`TestOrbitG02RetiredDeviceCannotRejoin`](../tests/designgates/orbit_g02_enrollment_test.go#L372): Verifies rejection of retired device revival.
- [`model.MembershipChain`](../model/membership.go#L76): Independent formal test oracle for linear revisions, signatures, and fork detection.

### Rejected Alternatives
- Unsigned gossip or distributed voting for membership (violates single-owner trust model and invites partition inconsistency).
- Automatic LWW (last-write-wins) resolution of competing membership revisions (risks admitting untrusted devices).
- Transitive device-to-device trust or shared group symmetric keys.

---

## G03 — File Actions, Durable Journals & Read Leases

### Decisions and Rules
1. **Durable File Mutation Journals**:
   - File actions (Import, CreateDir, Move, Delete) record phase transitions in a durable SQLite journal: `Planned -> Staged -> Installed -> SourceVerified -> Completed`.
   - Every file action request carries a durable Operation ID and reviewed generation/basis token.
2. **Move Atomicity, Overwrite Safety & Recovery Preservation**:
   - Move is executed as stage destination + install destination + verify source + delete source.
   - If destination path exists:
     - Without an explicit reviewed overwrite token: operation fails with `ErrDestinationExists`.
     - With overwrite token: the displaced destination file is moved into `.filesync-internal/recovery/<opID>` via `RENAME_EXCHANGE` or atomic displace before installing the new destination. Displaced bytes are never permanently unlinked.
   - **Invariant I26 (Concurrent Source Modification Race)**:
     - If the source file is modified concurrently by an editor or writer between planning and deletion (detected via stat/hash re-verification): **the source file is NOT deleted**.
     - Both the new destination file and the concurrently modified source file are preserved on disk. The operation completes with `StatusCompletedWithSourceRetained` and surfaces a clear attention item.
3. **Directory Subtree Moves**:
   - A directory move reviews a snapshot token of its immediate children.
   - If a new child file or subdirectory is created inside the source directory before the move commits, the subtree token is invalidated (`ErrSubtreeInvalidated`), requiring re-review to prevent silent orphan omission or partial moves.
4. **Read Leases & GC Protection**:
   - Reading or previewing content via the browser or CLI acquires a short-lived `ReadLease` binding the Version ID and its CAS chunk digests.
   - **Invariant I25**: Active read leases prevent concurrent Garbage Collection (GC) sweeps from unlinking or quarantining required chunk objects, even if the file was deleted in a newer head.
   - Expired or released idle leases permit GC. O07 live responses retain separate stream pins until completion/cancellation; wall-clock expiry cannot revoke an active read. There is no reusable read capability: every content/range request authenticates the exact version again.

### Executable Fixtures and Oracles
- [`TestOrbitG03MoveOverwriteAndRecoveryPreservation`](../tests/designgates/orbit_g03_fileactions_test.go#L58): Demonstrates destination displacement to recovery storage upon overwrite.
- [`TestOrbitG03MoveSourceConcurrentModificationRetainsBoth`](../tests/designgates/orbit_g03_fileactions_test.go#L116): Verifies that concurrent edits to source file during a move prevent source deletion, retaining both copies.
- [`TestOrbitG03DirectorySubtreeInvalidation`](../tests/designgates/orbit_g03_fileactions_test.go#L166): Verifies subtree token invalidation when new children appear during directory moves.
- [`TestOrbitG03ReadLeaseProtectsChunksFromGC`](../tests/designgates/orbit_g03_fileactions_test.go#L254): Verifies active read leases protect chunks from GC sweeps and release upon TTL expiry.

### Rejected Alternatives
- Plain rename overwrite that unlinks existing destination without recovery copy.
- Unconditional source deletion during move without re-verifying content stability.
- Whole-file in-memory Blob buffering for browser downloads.

---

## G04 — Identity Recovery & Crash Consistency

### Decisions and Rules
1. **Stopped Exclusive Recovery Requirement**:
   - Identity reset and backup restoration are stopped maintenance operations. They require acquiring an exclusive lock on the state directory (`state.Acquire`).
   - Any live attempt while the daemon is actively running fails immediately with `state.ErrLocked`.
2. **Atomic Transition Ordering & Rollback Prevention**:
   - The recovery sequence executes:
     1. Rotate TLS identity (`peer-identity.pem`) generating fresh Ed25519 keypair and certificate (`replication.RotateIdentity`).
     2. Transactionally update SQLite `folders` table (`UPDATE folders SET local_author = ?, next_counter = 0`) with checked error handling, while preserving the full DAG history in `versions`.
     3. Atomically write updated `config.json` (`config.Save`) with the new Device ID.
   - **Invariant I08**: Restart never emits rolled-back causal counters under an old key. Historical versions remain preserved under their original author IDs.
3. **Crash Consistency & Interrupted Transition Detection**:
   - If the system crashes midway through recovery (e.g. after key rotation but before DB transaction or config commit):
   - On restart, the daemon runs startup verification comparing `config.DeviceID`, the TLS certificate identity, and `folders.local_author`.
   - Any mismatch is detected as an incomplete recovery (`ErrIncompleteRecovery`). The daemon fences normal synchronization and returns an actionable maintenance error rather than operating with split identity.
4. **Metadata Restore with Missing CAS Payloads**:
   - Restoring a SQLite database backup restores metadata version DAGs. If local chunk payloads are missing from the content-addressed store (CAS):
   - **Invariant I18**: The system diagnoses the version as `ContentUnavailable` or `PendingTransfer`. It **never** serves substitute, empty, or corrupt data to users or peers.

### Executable Fixtures and Oracles
- [`TestOrbitG04LiveIdentityResetFenced`](../tests/designgates/orbit_g04_recovery_test.go#L60): Verifies live reset is blocked when daemon lock is held.
- [`TestOrbitG04AtomicTransitionAndRollbackSafety`](../tests/designgates/orbit_g04_recovery_test.go#L81): Verifies key rotation, checked SQLite transaction, atomic config write, and version history preservation.
- [`TestOrbitG04InterruptedTransitionDetection`](../tests/designgates/orbit_g04_recovery_test.go#L165): Verifies startup consistency check detects crashed partial transitions.
- [`TestOrbitG04MissingPayloadsAfterMetadataRestore`](../tests/designgates/orbit_g04_recovery_test.go#L208): Verifies missing CAS chunks yield `ErrContentUnavailable`.

### Rejected Alternatives
- Live identity reset while daemon is actively replicating.
- Deleting the `versions` history table on reset (destroys causal resolution records).
- Reusing an existing key pin across identity reset.

---

## G05 — Compatibility, Product Settings & Rollback Limits

### Decisions and Rules
1. **Product Settings Isolation**:
   - User display preferences (device display label, workspace display names, default workspace, UI theme) are stored in an independent file (`settings.json`) or dedicated SQLite table.
   - `config.json` schema remains strictly locked to `format_version: 1` (`format_version`, `device_id`, `created_at`). Product display settings must not be added to `config.json`.
   - Corruption or absence of `settings.json` falls back to defaults without corrupting `config.json` or preventing daemon startup.
2. **Protocol Capability Negotiation**:
   - Pinned TLS connections exchange an initial handshake declaring supported capabilities (e.g. `base_sync_v1`, `orbit_enrollment_v1`, `orbit_read_leases_v1`).
   - Legacy peers that only support `base_sync_v1` establish normal bidirectional synchronization; Orbit-specific endpoints are disabled for that connection without connection errors.
   - Mandatory capabilities that are unsupported cause a clean rejection (`ErrMandatoryCapUnsupported`).
3. **Legacy State Adoption and Schema Rollback Limits**:
   - Orbit adopts existing `.filesync-internal` state directories containing schema version 5 without data loss or re-initialization.
   - **Invariant I20**: If the database `user_version` is newer than the supported maximum (e.g. rolling back to an older binary after a future upgrade), the daemon refuses to run (`ErrUnsupportedSchemaVersion`) to preserve recoverable state.

### Executable Fixtures and Oracles
- [`TestOrbitG05ProductSettingsSeparation`](../tests/designgates/orbit_g05_compat_test.go#L33): Verifies that product settings do not alter or break strict `config.Load` validation.
- [`TestOrbitG05ProtocolCapabilityNegotiation`](../tests/designgates/orbit_g05_compat_test.go#L116): Verifies capability negotiation between legacy and Orbit nodes, and mandatory capability checks.
- [`TestOrbitG05LegacyStateAdoptionAndRollbackLimits`](../tests/designgates/orbit_g05_compat_test.go#L163): Verifies clean adoption of schema version 5 and refusal of newer unsupported schemas.

### Rejected Alternatives
- In-place modification of `config.json` format version 1 with unversioned UI fields.
- Hard failure when connecting to nodes that lack Orbit UI extensions.
- Unchecked downgrades against newer database schemas.
