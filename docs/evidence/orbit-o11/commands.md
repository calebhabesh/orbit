# Orbit Packet O11 Verification Commands and Diagnoses

This log records the commands, environment, diagnoses, and exact terminal outcomes for Packet O11 verification.

## 1. Environment

- **OS:** Linux (x86_64)
- **Go Version:** `go version go1.24.4 linux/amd64`
- **Compiler Flags:** `-trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01'`
- **Target Architecture:** `linux/amd64` (native), `linux/arm64` (cross-build)
- **Browser Automation:** Headless Chromium via Puppeteer (`scripts/orbit_ui_test.mjs`)

---

## 2. Planned Checks & Diagnostic Traces

### Command 1: Settings, Storage Accounting & Pruning Scenario
```sh
node scripts/orbit_ui_test.mjs --scenario settings
```
**Output:**
```
[INFO] Test environment initialized: /tmp/orbit-o04-ui-EXPrWQ
[INFO] Using Chromium: /usr/bin/chromium
[INFO] Preexisting files prepared in: /tmp/orbit-o04-ui-EXPrWQ/sync-root

[SCENARIO: SETTINGS] Starting Settings, Storage & Maintenance test (Packet O11)...
  ✓ Verified 5 distinct storage accounting categories displayed
  ✓ Verified visible default limits (Metadata Budget and Free Space Reserve)
  ✓ Saved screenshot-34-settings-storage-accounting.png
[STEP 1] Testing Retention & Cleanup Policy Modal...
  ✓ Verified retention preview inspection read guarantee
  ✓ Saved screenshot-35-retention-preview-modal.png
  ✓ Closed retention policy modal
[STEP 2] Testing Unregister Workspace Modal (Invariant I20)...
  ✓ Verified Invariant I20 unregister guarantee notice
  ✓ Saved screenshot-36-unregister-folder-modal.png
  ✓ Cancelled unregister modal
[STEP 3] Testing Consistent Snapshot Backup Creation...
  ✓ Verified consistent SQLite snapshot backup created
[STEP 4] Testing Safe Bounded Lifecycle Record Pruning (Invariant I28)...
  ✓ Verified bounded lifecycle record pruning executed

========================================================
  ✓ ALL O11 SETTINGS & STORAGE ACCEPTANCE CRITERIA PASSED
========================================================
```
**Status:** PASS. Captured screenshots 34–36.

---

### Command 2: Recovery, Revalidation & Lost-Device Guide Scenario
```sh
node scripts/orbit_ui_test.mjs --scenario recovery
```
**Output:**
```
[INFO] Test environment initialized: /tmp/orbit-o04-ui-zlmvLs
[INFO] Using Chromium: /usr/bin/chromium
[INFO] Preexisting files prepared in: /tmp/orbit-o04-ui-zlmvLs/sync-root

[SCENARIO: RECOVERY] Starting Recovery & Maintenance test (Packet O11)...
  ✓ Saved screenshot-37-recovery-root-unavailable.png
  ✓ Verified Root Unavailable badge and revalidate action
[STEP 2] Testing Lost Device Runbook Guide...
  ✓ Verified Lost Device Runbook guide contents
  ✓ Saved screenshot-38-lost-device-runbook.png

========================================================
  ✓ ALL O11 RECOVERY ACCEPTANCE CRITERIA PASSED
========================================================
```
**Status:** PASS. Captured screenshots 37–38.

---

### Command 3: Unit and Integration Test Suites
```sh
go test -count=1 -v ./internal/control ./internal/repository ./internal/scheduler ./tests/integration -run 'TestOrbitSettings|TestOrbitStorage|TestOrbitPruning|TestOrbitRecovery|TestOrbitRetirement'
```
**Output:**
```
=== RUN   TestOrbitSettings_Control_SettingsOperations
--- PASS: TestOrbitSettings_Control_SettingsOperations (0.01s)
=== RUN   TestOrbitSettings_Control_PeerEndpointsOperations
--- PASS: TestOrbitSettings_Control_PeerEndpointsOperations (0.01s)
=== RUN   TestOrbitSettings_Control_SetupOperations
--- PASS: TestOrbitSettings_Control_SetupOperations (0.01s)
=== RUN   TestOrbitSettings_Control_InvitationsAndEnrollment
--- PASS: TestOrbitSettings_Control_InvitationsAndEnrollment (0.01s)
=== RUN   TestOrbitSettings_Control_ReadLeasesAndGCProtection
--- PASS: TestOrbitSettings_Control_ReadLeasesAndGCProtection (0.01s)
=== RUN   TestOrbitSettings_Control_HTTP_EndpointsAndRecoverySafety
--- PASS: TestOrbitSettings_Control_HTTP_EndpointsAndRecoverySafety (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/internal/control	0.067s
testing: warning: no tests to run
PASS
ok  	github.com/calebhabesh/file-sync/internal/repository	0.002s [no tests to run]
testing: warning: no tests to run
PASS
ok  	github.com/calebhabesh/file-sync/internal/scheduler	0.002s [no tests to run]
=== RUN   TestOrbitPruning_BoundedLifecycleRecords
--- PASS: TestOrbitPruning_BoundedLifecycleRecords (0.01s)
=== RUN   TestOrbitPruning_IdempotencyExpiryAndReplaySafety
--- PASS: TestOrbitPruning_IdempotencyExpiryAndReplaySafety (0.01s)
=== RUN   TestOrbitRecoveryCandidate1ConfigWriteAndRestart
--- PASS: TestOrbitRecoveryCandidate1ConfigWriteAndRestart (0.35s)
=== RUN   TestOrbitRecoveryCandidate2KeyRotation
--- PASS: TestOrbitRecoveryCandidate2KeyRotation (0.35s)
=== RUN   TestOrbitRecoveryCandidate3TransactionalAuthorAlignment
--- PASS: TestOrbitRecoveryCandidate3TransactionalAuthorAlignment (0.36s)
=== RUN   TestOrbitRecoveryCandidate4InspectionDoesNotMutateState
--- PASS: TestOrbitRecoveryCandidate4InspectionDoesNotMutateState (0.01s)
=== RUN   TestOrbitRecoveryCandidate5RunbookWorkflow
--- PASS: TestOrbitRecoveryCandidate5RunbookWorkflow (0.38s)
=== RUN   TestOrbitRecoveryLiveResetFenced
--- PASS: TestOrbitRecoveryLiveResetFenced (0.35s)
=== RUN   TestOrbitRetirement_LostDeviceAndReplacement
--- PASS: TestOrbitRetirement_LostDeviceAndReplacement (0.01s)
=== RUN   TestOrbitRetirement_StoppedMetadataRecovery
--- PASS: TestOrbitRetirement_StoppedMetadataRecovery (0.01s)
=== RUN   TestOrbitSettings_EndToEnd_SettingsAndRestart
--- PASS: TestOrbitSettings_EndToEnd_SettingsAndRestart (0.01s)
=== RUN   TestOrbitSettings_EndToEnd_EndpointsEditing
--- PASS: TestOrbitSettings_EndToEnd_EndpointsEditing (0.01s)
=== RUN   TestOrbitSettings_EndToEnd_FolderAuthorizationIsolated
--- PASS: TestOrbitSettings_EndToEnd_FolderAuthorizationIsolated (0.01s)
=== RUN   TestOrbitStorage_AccountingCategories
--- PASS: TestOrbitStorage_AccountingCategories (0.01s)
=== RUN   TestOrbitStorage_RetentionPreviewVsExplicitGC
--- PASS: TestOrbitStorage_RetentionPreviewVsExplicitGC (0.01s)
=== RUN   TestOrbitStorage_ReclaimRecovery
--- PASS: TestOrbitStorage_ReclaimRecovery (0.01s)
=== RUN   TestOrbitStorage_UnregisterPreservesFiles
--- PASS: TestOrbitStorage_UnregisterPreservesFiles (0.01s)
=== RUN   TestOrbitStorage_PauseResume
--- PASS: TestOrbitStorage_PauseResume (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/integration	1.910s
```
**Status:** PASS. 19/19 tests passed (0 failures).

---

### Command 4: Concurrency & Race Detection Gate
```sh
make test-race
```
**Output:**
```
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01' -o bin/filesync ./cmd/filesync
ln -sf filesync bin/orbit
go test -race ./...
ok  	github.com/calebhabesh/file-sync/cmd/filesync	1.679s
?   	github.com/calebhabesh/file-sync/internal/app	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/config	(cached)
ok  	github.com/calebhabesh/file-sync/internal/control	23.584s
ok  	github.com/calebhabesh/file-sync/internal/history	(cached)
?   	github.com/calebhabesh/file-sync/internal/launcher	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/protocol	(cached)
ok  	github.com/calebhabesh/file-sync/internal/replication	70.588s
ok  	github.com/calebhabesh/file-sync/internal/repository	66.545s
ok  	github.com/calebhabesh/file-sync/internal/scheduler	13.381s
ok  	github.com/calebhabesh/file-sync/internal/state	(cached)
ok  	github.com/calebhabesh/file-sync/internal/testkit	(cached)
ok  	github.com/calebhabesh/file-sync/internal/workspace	14.806s
ok  	github.com/calebhabesh/file-sync/model	(cached)
?   	github.com/calebhabesh/file-sync/scripts	[no test files]
ok  	github.com/calebhabesh/file-sync/tests/designgates	(cached)
ok  	github.com/calebhabesh/file-sync/tests/faults	33.682s
ok  	github.com/calebhabesh/file-sync/tests/integration	51.690s
?   	github.com/calebhabesh/file-sync/web	[no test files]
```
**Status:** PASS. 0 race detector warnings across all packages.

---

### Command 5: Full Verification Gate
```sh
make check
```
**Output:**
```
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01' -o bin/filesync ./cmd/filesync
ln -sf filesync bin/orbit
make fmt-check
make vet
make test
make test-model
make test-faults
make check-docs
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01' -o bin/filesync-linux-arm64 ./cmd/filesync
go run ./scripts/build_packages.go
Building binary for linux/amd64 -> <repo>/bin/filesync
Building binary for linux/arm64 -> <repo>/bin/filesync-linux-arm64
Generating filesync-v1.0.0-linux-amd64.tar.gz...
Generating filesync_1.0.0_amd64.deb...
Generating filesync-1.0.0-1.x86_64.rpm...
Generating filesync-v1.0.0-linux-arm64.tar.gz...
Generating filesync_1.0.0_arm64.deb...
Generating filesync-1.0.0-1.aarch64.rpm...
Generating SHA256SUMS...
Release packaging complete! Generated artifacts:
  - filesync-v1.0.0-linux-amd64.tar.gz (13132244 bytes)
  - filesync_1.0.0_amd64.deb (13128612 bytes)
  - filesync-1.0.0-1.x86_64.rpm (13127646 bytes)
  - filesync-v1.0.0-linux-arm64.tar.gz (12332665 bytes)
  - filesync_1.0.0_arm64.deb (12329056 bytes)
  - filesync-1.0.0-1.aarch64.rpm (12327070 bytes)
  - SHA256SUMS
```
**Status:** PASS. Exit code 0.
