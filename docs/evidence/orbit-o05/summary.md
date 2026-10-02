# Orbit Packet O05 Summary: Enrollment, Endpoints and Durable Membership Propagation

**Packet:** O05  
**Date:** 2026-10-02  
**Status:** `complete`  
**Prerequisites:** O01, O02 complete. Read [vision](../../orbit-product.md), [architecture](../../orbit-architecture.md), [plan](../../orbit-implementation-plan.md), [protocol](../../protocol.md), [persistence](../../persistence.md), [operations](../../operations.md), [verification](../../verification.md), and [orbit devices and files](../../implementation/orbit-devices-files.md#o05--enrollment-and-linear-membership-rollout).  
**Requirements Satisfied:** U03 (multi-device membership & linear progression), U05 (invitation lifecycle, verifiers & expiration), U06 (joining proofs & explicit owner approval), U09 (offline peer catch-up & partition fork reconciliation), U16 (accessible interactive controls & clear error taxonomy).  
**Invariants Verified:**
- **I09 (Folder Authorization):** Peer input cannot escape authorized workspace folders.
- **I13 (Bounded Work & Resource Limits):** Join request payload bounded to 16 KiB (HTTP 413 `PAYLOAD_TOO_LARGE`); IP-based rate limiting enforces maximum 5 requests/minute (HTTP 429 `RATE_LIMITED`).
- **I15 (Retirement Rejection):** Retired devices cannot re-enter, rejoin, or fetch updates under the same cryptographic identity.
- **I16 (Resolution Idempotence):** Repeated owner approval of enrollment requests is idempotent and returns replay status without minting redundant revisions.
- **I20 (Schema & Recoverable State):** Daemon restart preserves all configuration, membership revisions, and peer endpoints across restarts.
- **I23 (Single-Use & Capability Scoping):** Raw invitation secrets are never persisted to SQLite or logged; only SHA-256 verifier digests are retained. Possession of an invitation token grants zero access to chunk data, inventory, or files.
- **I24 (Strict Linear Membership & Competing Fork Detection):** Owner approvals append to a linear chain: `Revision(N+1)` with `PriorDigest = Hash(Revision N)`. Partitioned concurrent approvals produce competing revisions with the same `PriorDigest`, flagged as an explicit `ErrMembershipFork`. Competing branches are never silently auto-merged and require explicit owner reconciliation.

---

## 1. Architecture and Design

Packet O05 implements the Gate G02 contract behind shared control and peer replication operations, enabling decentralized multi-device pairing without centralized coordination or certificate file juggling.

```
       [Owner / Device A]
             │
             │ 1. CreateInvitation()
             ▼
     [Expiring Invitation]
      (SHA-256 Verifier)
             │
             │ 2. Token handoff out-of-band
             ▼
      [Joining Device B]
             │
             │ 3. SubmitEnrollmentRequest()
             │    - Ed25519 signature over challenge nonce
             │    - Bounded payload <= 16 KiB (HTTP 413)
             │    - Rate limited <= 5 req/min (HTTP 429)
             ▼
     [Pending Join Request]
             │
             │ 4. ApproveEnrollmentRequest()
             │    - Mint Revision N+1
             │    - PriorDigest = Hash(Rev N)
             ▼
 [Sequential Linear Membership Rollout]
             │
   ┌─────────┴─────────┐
   ▼                   ▼
[Direct Pull]     [Forwarding Sync]
 (A <──> B)        (A <──> H <──> B)
```

### Core Implementations

| Component | Architecture & Responsibilities | Files |
| :--- | :--- | :--- |
| **Workspace Invitations (I23)** | Expiring invitations with configurable TTL and single/multi-use limits. SQLite stores only SHA-256 verifier digests (`invitations` table); raw tokens never touch disk, logs, or exports. Possession of token permits only bounded join request submission and yields 401/403 on inventory, chunks, or files. | [`internal/repository/product_records.go`](file://<repo>/internal/repository/product_records.go), [`internal/control/orbit_control.go`](file://<repo>/internal/control/orbit_control.go) |
| **Joining Key Proofs & Rate Limiting (I13)** | Joining devices present Ed25519 public key and cryptographic signature over a challenge nonce. `POST /api/v1/enrollment/request` enforces strict 16 KiB body bounds (`http.MaxBytesReader` -> HTTP 413) and 5 req/min IP rate limiting (HTTP 429). Forged or mismatched signatures are rejected (`INVALID_SIGNATURE`). | [`internal/control/orbit_control.go`](file://<repo>/internal/control/orbit_control.go), [`internal/control/server.go`](file://<repo>/internal/control/server.go) |
| **Sequential Linear Rollout & Idempotency (I16, I24)** | Owner approval (`POST /api/v1/enrollment/approve`) mints next linear revision (`Revision N+1`) bound to `PriorDigest = Hash(Revision N)`. Durably updates `folders`, `membership_revisions`, and `membership_entries`. Idempotent replay returns existing approved state (`replay: true`) without creating spurious revisions. Optionally installs display name alias and peer endpoint. | [`internal/repository/peers.go`](file://<repo>/internal/repository/peers.go), [`internal/control/orbit_control.go`](file://<repo>/internal/control/orbit_control.go) |
| **Competing Fork Detection & Reconciliation (I24)** | Detects competing revisions at the same revision level or sharing the same prior digest (`ErrMembershipFork`). Rejecting automatic silent merges, partitioned branches pause sync until the owner explicitly reconciles via `POST /api/v1/membership/reconcile`, minting a unifying sequential revision. | [`internal/repository/peers.go`](file://<repo>/internal/repository/peers.go), [`internal/control/orbit_control.go`](file://<repo>/internal/control/orbit_control.go) |
| **Gated Peer Control Exchange & Catch-Up** | Mounted `POST /peer/v1/membership/get` on the mutual TLS replication server. Authenticates client certificates, verifies caller is an active or historical member of the folder, and serves latest approved revisions and retirement snapshots to offline peers. Strictly blocks retired devices (`RETIRED_MEMBER`). | [`internal/replication/server.go`](file://<repo>/internal/replication/server.go), [`internal/replication/client.go`](file://<repo>/internal/replication/client.go) |
| **Directional Endpoints & Sync Forwarding** | Configured via `peers.json` and `/api/v1/settings/peers`. Validates HTTPS origins, 32-byte hex folder/device IDs, and local certificate paths. Verified across two-peer direct pull sync and three-node A→Hub→B forwarding sync without direct A-B connectivity. Categorizes connection, pin, version, and membership errors distinctly. | [`internal/config/peers.go`](file://<repo>/internal/config/peers.go), [`internal/replication/transfer.go`](file://<repo>/internal/replication/transfer.go) |

---

## 2. Test Suites and Evidence

### A. Orbit Enrollment Test Suite (`tests/integration/orbit_enrollment_test.go`)
- `TestOrbitEnrollment_InvitationLifecycleAndExclusions`: Verifies invitation generation, SHA-256 verifier storage (confirms raw secret is not in SQLite), token exclusion from chunk/inventory APIs (Invariant I23), TTL expiration, revocation, and single-use vs multi-use limits.
- `TestOrbitEnrollment_RequestBoundingAndRateLimits`: Verifies 16 KiB payload ceiling (HTTP 413) and 5 req/min rate limit (HTTP 429 on 6th request).
- `TestOrbitEnrollment_KeyPossessionAndApproval`: Verifies Ed25519 key possession proof, rejection of forged signatures, zero data access before approval, Revision N+1 minting, display name configuration, peer endpoint storage, and approval idempotency (`replay: true`).
- `TestOrbitEnrollment_RetiredDeviceRejection`: Verifies Invariant I24 that a retired device identity cannot submit join requests or be approved (`ErrRetiredMemberRevival`).

### B. Orbit Membership Test Suite (`tests/integration/orbit_membership_test.go`)
- `TestOrbitMembership_SequentialRollout`: Verifies sequential linear rollout across multiple revisions (Rev 1 -> Rev 2 -> Rev 3) with `PriorDigest` integrity.
- `TestOrbitMembership_CompetingAdministrationFork`: Verifies Invariant I24 competing fork detection when partitioned nodes approve from the same base revision, verifies `DetectMembershipFork`, and exercises explicit owner reconciliation into Revision N+2.
- `TestOrbitMembership_OfflineCatchUp`: Verifies that an offline peer reconnects, calls `POST /peer/v1/membership/get`, receives newer revisions, and catches up. Verifies that retired devices calling the control endpoint receive HTTP 403 `RETIRED_MEMBER`.
- `TestOrbitMembership_RestartPreservesConsentAndRevisions`: Verifies that full daemon shutdown and restart preserves all approved revisions, enrollment requests, display names, and endpoints.

### C. Orbit Endpoints Test Suite (`tests/integration/orbit_endpoints_test.go`)
- `TestOrbitEndpoints_PersistenceAndValidation`: Tests `peers.json` CRUD, HTTPS URL parsing, 32-byte hex validation, and certificate path checks.
- `TestOrbitEndpoints_TwoPeerDirectPullSync`: Tests mutual TLS synchronization between Node A and Node B using configured peer endpoints.
- `TestOrbitEndpoints_ForwardingSync`: Tests A→Hub→B forwarding sync where Node A and Node B synchronize without direct network connectivity.
- `TestOrbitEndpoints_ErrorCategorization`: Tests separate categorization for connection refused (network), pin mismatch (certificate), version mismatch (wire protocol), and unauthorized/membership mismatch (permissions).

---

## 3. Verification Execution Results

1. **O05 Planned Test Suite**:
   ```bash
   go test -count=1 -v ./internal/replication ./internal/control ./internal/repository ./tests/integration -run 'TestOrbitEnrollment|TestOrbitMembership|TestOrbitEndpoints'
   ```
   *Result:* **14/14 tests passed** across all packages in 0.19s.

2. **Formal Membership & Reference Set Models**:
   ```bash
   make test-model
   ```
   *Result:* All independent formal test oracles passed (including `TestOrbitMembershipSequentialRollout`, `TestOrbitMembershipCompetingAdministrationForks`, `TestOrbitMembershipUnauthorizedOrForgedSignatures`).

3. **Data Race Detection**:
   ```bash
   make test-race
   ```
   *Result:* Ran `go test -race ./...` across all packages (`cmd`, `internal`, `model`, `tests`). **0 data races detected**.

4. **Full Project Pipeline**:
   ```bash
   make check
   ```
   *Result:* `fmt-check`, `vet`, `test`, `test-integration`, `test-model`, `test-faults`, `test-harness`, `build`, `build-arm64`, and `package` all **passed cleanly**. Packages (`.tar.gz`, `.deb`, `.rpm`) generated.

5. **Documentation Link Integrity**:
   *Result:* 90 markdown files checked; **367 local links validated; 0 broken links**.
