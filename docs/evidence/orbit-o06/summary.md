# Orbit Packet O06 Summary: Pairing and Device-Management UI

**Packet:** O06  
**Date:** 2026-10-02  
**Status:** `complete`  
**Prerequisites:** O04, O05 complete. Read [vision](../../orbit-product.md), [architecture](../../orbit-architecture.md), [plan](../../orbit-implementation-plan.md), [devices and files](../../implementation/orbit-devices-files.md#o06--pairing-and-device-management-ui), [operations](../../operations.md), and [runbook: headless pairing](../../runbooks/headless-pairing.md).  
**Requirements Satisfied:** U03 (multi-device membership & linear progression), U05 (invitation lifecycle, verifiers & expiration), U06 (joining proofs & explicit owner approval), U09 (offline peer catch-up & partition fork reconciliation), U14 (device discovery, management & retirement UI), U16 (accessible interactive controls & clear error taxonomy).  
**Invariants Verified:**
- **I19 (Headless Parity):** Every onboarding, pairing, invitation, request approval, and device management flow is available via the `orbit` CLI or local control over SSH.
- **I22 (Working-Copy Preservation):** Existing files in the joining device's sync root are indexed and preserved; join never triggers deletion of preexisting content.
- **I23 (Single-Use and Capability Scoping):** Raw invitation tokens are never stored on disk, logged, or exported (only SHA-256 verifier digests). Possession of an invitation token permits only bounded join request submission (<= 16 KiB, <= 5 req/min) and grants zero read/write access to folder inventory, chunks, or files.
- **I24 (Strict Linear Rollout & Permanent Retirement):** Device approval appends strictly to the linear membership chain (`Revision N+1` bound to `PriorDigest = Hash(Revision N)`). Device identity retirement is permanent and immutable; readmission is strictly forbidden.
- **I27 (Failure Visibility):** Stalled or unreachable connections, declined requests, rate limiting, and retirement constraints are explicitly diagnosed; no infinite spinners or misleading global success claims.

---

## 1. Architecture and UI Components

Packet O06 builds the interactive frontend and headless CLI workflows for pairing devices, managing cluster replicas, approving enrollment requests, and previewing member retirement.

```
       [Owner Node A (Web UI or CLI)]
                     │
         1. Create Invitation Modal / CLI
            (Single/multi-use, TTL, endpoint)
                     │
                     ▼
         [Expiring Invitation Token]
          (orbit-invitation:v1?token=...)
                     │
                     ├─────────────────────────────────────────┐
                     │ Out-of-band link / QR code               │
                     ▼                                         ▼
      [Joiner Node B (Web UI)]                   [Joiner Node B (Headless CLI)]
        (Setup Wizard: Join Flow)                  (orbit join --invitation ...)
                     │                                         │
         2. Submit Join Proof                       2. Submit Join Proof
            (Ed25519 signature nonce)                  (Ed25519 signature nonce)
                     │                                         │
                     └────────────────────┬────────────────────┘
                                          │
                                          ▼
                         [Owner Node A Pending Inbox]
                         (DevicesView / orbit requests)
                                          │
                             3. Explicit Owner Approval
                                (Mint Revision N+1,
                                 Assign Label & Endpoint)
                                          │
                                          ▼
                         [Sequential Rollout & Pull Sync]
                                  (Replicas Active)
```

### Core Implementations

| Component | Architecture & Responsibilities | Files |
| :--- | :--- | :--- |
| **"Add Device" Flow** | Modal supporting TTL selection, single vs multi-use limits, pure SVG QR code generation (`QRCodeDisplay.tsx`), formatted link copying (`orbit-invitation:v1?...`), and active invitation list with explicit revocation. | [`web/src/components/AddDeviceModal.tsx`](file://<repo>/web/src/components/AddDeviceModal.tsx), [`web/src/components/QRCodeDisplay.tsx`](file://<repo>/web/src/components/QRCodeDisplay.tsx) |
| **"Join Workspace" Flow** | Setup wizard join tab accepting formatted links or tokens, reachability probe with network prerequisite diagnostics, preexisting content safety guarantee (Invariant I22), Ed25519 key possession proof submission, and live approval status polling. | [`web/src/views/SetupWizard.tsx`](file://<repo>/web/src/views/SetupWizard.tsx), [`web/src/api.ts`](file://<repo>/web/src/api.ts) |
| **Pending Join Requests Inbox** | Device management inbox on owner node displaying incoming join requests with device IDs and key pins. Allows custom label editing, endpoint assignment, and explicit approve/decline actions. Approval appends Revision N+1 to linear membership chain. | [`web/src/views/DevicesView.tsx`](file://<repo>/web/src/views/DevicesView.tsx), [`internal/control/orbit_control.go`](file://<repo>/internal/control/orbit_control.go) |
| **Device Details & Probe** | Detailed device inspect modal displaying device identity, key pin, status badges, endpoint reachability probe with latency reporting and actionable connection error diagnosis. Supports alias renaming. | [`web/src/components/DeviceDetailModal.tsx`](file://<repo>/web/src/components/DeviceDetailModal.tsx) |
| **Safe Retirement Preview** | Explains known-history limits, uncaptured local work, permanent cryptographic revocation (Invariant I24), and disclaimer that retirement cannot promise remote disk erasure. | [`web/src/components/RetireDeviceModal.tsx`](file://<repo>/web/src/components/RetireDeviceModal.tsx) |
| **Headless Parity (I19)** | Complete CLI parity via `orbit` (`setup`, `join`, `invite`, `requests`, `devices`, `status`). Automatically handles daemon lock by falling back to loopback control HTTP API when daemon is active. Verified via SSH runbook. | [`cmd/filesync/main.go`](file://<repo>/cmd/filesync/main.go), [`docs/runbooks/headless-pairing.md`](file://<repo>/docs/runbooks/headless-pairing.md) |

---

## 2. Evidence: Screenshot Catalog

All 7 planned UI screenshots were captured using the automated Puppeteer test runner (`scripts/orbit_ui_test.mjs`) in isolated browser contexts:

| Screenshot | File | Size | Description |
| :--- | :--- | :--- | :--- |
| **Screenshot 10** | [`screenshot-10-add-device-modal.png`](screenshots/screenshot-10-add-device-modal.png) | 94,353 B | Add Device modal showing invitation parameters (TTL, uses), formatted link, SVG QR code, and active invitation list. |
| **Screenshot 11** | [`screenshot-11-join-workspace-form.png`](screenshots/screenshot-11-join-workspace-form.png) | 73,617 B | Join Workspace onboarding wizard showing invitation link parsing, sync root selection, and preexisting content preservation notice. |
| **Screenshot 12** | [`screenshot-12-waiting-approval.png`](screenshots/screenshot-12-waiting-approval.png) | 60,725 B | Waiting for approval screen on joining device with live polling indicator and cryptographic public key pin. |
| **Screenshot 13** | [`screenshot-13-devices-inbox-pending.png`](screenshots/screenshot-13-devices-inbox-pending.png) | 72,697 B | Devices view on owner node showing pending enrollment request inbox with suggested alias editing and Approve/Decline controls. |
| **Screenshot 14** | [`screenshot-14-device-details-probe.png`](screenshots/screenshot-14-device-details-probe.png) | 84,275 B | Device Detail modal showing device identity, key pin, replica status, and successful endpoint reachability probe. |
| **Screenshot 15** | [`screenshot-15-unreachable-endpoint-error.png`](screenshots/screenshot-15-unreachable-endpoint-error.png) | 89,947 B | Actionable connection error diagnosis showing network failure details and reachability checklist without silent hanging. |
| **Screenshot 16** | [`screenshot-16-retire-preview-modal.png`](screenshots/screenshot-16-retire-preview-modal.png) | 87,848 B | Retirement preview modal explaining known-history limits, uncaptured local work, permanent identity revocation, and remote erasure disclaimer. |

---

## 3. Automated Verification Execution Results

### A. Orbit Pairing Integration Test Suite (`tests/integration/orbit_pairing_test.go`)
```bash
go test -count=1 -v ./tests/integration -run 'TestOrbitPairing'
```
*Result:* **5/5 tests passed** cleanly:
- `TestOrbitPairing_FullJoinFlowLifecycle`: Verifies full end-to-end join flow across two nodes: invitation generation, join request submission, pending status verification, owner approval with custom label/endpoint, Revision 2 minting, Invariant I22 preexisting content preservation, and reciprocal endpoint installation.
- `TestOrbitPairing_DeclineJoinFlow`: Verifies that declining an enrollment request transitions status to `"declined"` and prevents unauthorized membership rollout.
- `TestOrbitPairing_DeviceDetailAndRename`: Verifies device display alias renaming, persistence in SQLite, reachability probe categorization, and retirement preview disclaimers.
- `TestOrbitPairing_CLI_Parity`: Verifies offline CLI parity (`orbit setup`, `orbit invite create/list/revoke`, `orbit devices list`).
- `TestOrbitPairing_CLI_RunningDaemon_Parity`: Verifies live CLI parity when the background daemon is active (`orbit launch`, daemon fallback routing for `invite`, `requests`, `devices`).

### B. Headless Pairing Runbook Execution
The complete CLI pairing workflow over SSH was executed and verified in [`docs/runbooks/headless-pairing.md`](file://<repo>/docs/runbooks/headless-pairing.md), demonstrating:
- Owner workspace setup and background daemon launch.
- Invitation creation with advertised endpoint.
- Headless join request submission with key possession proof and timeout polling.
- Owner pending request listing and approval.
- Automatic completion and verification of Invariant I22 (preexisting file preserved without deletion).

### C. Formal Membership & Reference Set Models
```bash
make test-model
```
*Result:* All independent formal test oracles passed (`model.MembershipChain`, sequential rollout, competing administration fork detection, unauthorized signature rejection).

### D. Data Race Detection
```bash
make test-race
```
*Result:* Ran `go test -race ./...` across all packages (`cmd`, `internal`, `model`, `tests`). **0 race conditions detected**.

### E. Full Project Pipeline
```bash
make check
```
*Result:* Full validation passed cleanly: formatting, static analysis (`go vet`), unit tests, integration tests, model tests, fault injection tests, build targets (`amd64` and `arm64`), and package generation (`.tar.gz`, `.deb`, `.rpm`).
