# Orbit Packet O10 Verification Commands and Diagnoses

This log records the commands, environment, diagnoses, and exact terminal outcomes for Packet O10 verification.

## 1. Environment

- **OS:** Linux (x86_64)
- **Go Version:** `go version go1.24.4 linux/amd64`
- **Compiler Flags:** `-trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01'`
- **Target Architecture:** `linux/amd64` (native), `linux/arm64` (cross-build)
- **Browser Automation:** Headless Chromium via Puppeteer (`scripts/orbit_ui_test.mjs`)

---

## 2. Planned Checks & Diagnostic Traces

### Command 1: File Actions Browser Scenario
```sh
node scripts/orbit_ui_test.mjs --scenario file-actions
```
**Output:**
```
[INFO] Test environment initialized: /tmp/orbit-o04-ui-E8uSgn
[INFO] Using Chromium: /usr/bin/chromium
[INFO] Preexisting files prepared in: /tmp/orbit-o04-ui-E8uSgn/sync-root

[SCENARIO: FILE-ACTIONS] Starting file operations test (Packet O10)...
  ✓ Files view loaded
[STEP 1] Testing Create New Folder dialog...
  ✓ Saved screenshot-25-create-folder-modal.png
  ✓ Directory "projects" created and listed
[STEP 2] Testing File Upload & Overwrite Collision Confirmation...
  ✓ First upload succeeded and listed
  ✓ Saved screenshot-26-upload-overwrite-modal.png
  ✓ Overwrite confirmed and safely installed
[STEP 3] Testing Move / Rename File modal...
  ✓ Saved screenshot-27-move-file-modal.png
  ✓ File moved from root directory
  ✓ sample_moved.txt verified inside projects/ directory
[STEP 4] Testing Delete Confirmation Modal & Recursive Option...
  ✓ Saved screenshot-28-delete-file-modal.png
  ✓ Directory "projects" and contents recursively deleted
[STEP 5] Testing Mutation Error Context Retention...
  ✓ Error retained user input and displayed actionable guidance
  ✓ Saved screenshot-29-stale-view-context-retention.png

========================================================
  ✓ ALL O10 FILE-ACTIONS ACCEPTANCE CRITERIA PASSED
========================================================
```
**Status:** PASS. Captured screenshots 25–29.

---

### Command 2: History Timeline & Restore Scenario
```sh
node scripts/orbit_ui_test.mjs --scenario history
```
**Output:**
```
[INFO] Test environment initialized: /tmp/orbit-o04-ui-97x6ZS
[INFO] Using Chromium: /usr/bin/chromium
[INFO] Preexisting files prepared in: /tmp/orbit-o04-ui-97x6ZS/sync-root

[SCENARIO: HISTORY] Starting history timeline and deleted restore test (Packet O10)...
[STEP 1] Testing Historical Versions Timeline in drawer...
  ✓ Saved screenshot-30-file-history-drawer.png
[STEP 2] Testing Deleted Files view & restore flow...
  ✓ Saved screenshot-31-deleted-files-view.png
  ✓ Restore confirmed and new causal version authored
  ✓ Restored file "obsolete.txt" verified active in workspace

========================================================
  ✓ ALL O10 HISTORY & RESTORE ACCEPTANCE CRITERIA PASSED
========================================================
```
**Status:** PASS. Captured screenshots 30–31.

---

### Command 3: Needs Attention & Conflict Resolution Scenario
```sh
node scripts/orbit_ui_test.mjs --scenario attention
```
**Output:**
```
[INFO] Test environment initialized: /tmp/orbit-o04-ui-iBgI2p
[INFO] Using Chromium: /usr/bin/chromium
[INFO] Preexisting files prepared in: /tmp/orbit-o04-ui-iBgI2p/sync-root

[SCENARIO: ATTENTION] Starting Needs Attention & conflict resolution test (Packet O10)...
  ✓ Saved screenshot-32-needs-attention-view.png
[STEP 1] Testing Conflict Resolution Modal & Workflows...
  ✓ Verified human review context notice (timestamps/devices are context, not winner rules)
  ✓ Keep Separate Copies preview verified
  ✓ Manual Merge text editor verified
  ✓ Saved screenshot-33-conflict-resolve-modal.png
  ✓ Conflict resolved and modal dismissed
[STEP 2] Testing Storage Maintenance GC Trigger...
  ✓ Garbage collection triggered and completed successfully

========================================================
  ✓ ALL O10 ATTENTION ACCEPTANCE CRITERIA PASSED
========================================================
```
**Status:** PASS. Captured screenshots 32–33.

---

### Command 4: Model Consistency Suite
```sh
make test-model
```
**Output:**
```
go test -count=1 -v ./model/...
=== RUN   TestReferenceSetOracleInterleavings
--- PASS: TestReferenceSetOracleInterleavings (0.00s)
=== RUN   TestReferenceSetOracleServeLeaseBlocksGCIntent
--- PASS: TestReferenceSetOracleServeLeaseBlocksGCIntent (0.00s)
=== RUN   TestReferenceSetOracleSharedChunks
--- PASS: TestReferenceSetOracleSharedChunks (0.00s)
=== RUN   TestReferenceSetOraclePendingFallbackPreservation
--- PASS: TestReferenceSetOraclePendingFallbackPreservation (0.00s)
=== RUN   TestReferenceSetOracleClockJumps
--- PASS: TestReferenceSetOracleClockJumps (0.00s)
=== RUN   TestReferenceSetOracleCrashRecovery
--- PASS: TestReferenceSetOracleCrashRecovery (0.00s)
=== RUN   TestReferenceSetOracleCorruptionAndRepair
--- PASS: TestReferenceSetOracleCorruptionAndRepair (0.00s)
=== RUN   TestOrbitMembershipSequentialRollout
--- PASS: TestOrbitMembershipSequentialRollout (0.00s)
=== RUN   TestOrbitMembershipCompetingAdministrationForks
--- PASS: TestOrbitMembershipCompetingAdministrationForks (0.00s)
=== RUN   TestOrbitMembershipUnauthorizedOrForgedSignatures
--- PASS: TestOrbitMembershipUnauthorizedOrForgedSignatures (0.00s)
=== RUN   TestBoundedExhaustiveActorSchedulesAgree
--- PASS: TestBoundedExhaustiveActorSchedulesAgree (0.00s)
=== RUN   TestDeterministicGeneratedSchedulesAgree
--- PASS: TestDeterministicGeneratedSchedulesAgree (0.25s)
=== RUN   TestOracleCatchesIntentionalDominanceMutation
--- PASS: TestOracleCatchesIntentionalDominanceMutation (0.00s)
=== RUN   TestOracleKeepsAvailabilityAndRetirementSeparateFromHeads
--- PASS: TestOracleKeepsAvailabilityAndRetirementSeparateFromHeads (0.00s)
=== RUN   TestGoldenHistoryFixture
--- PASS: TestGoldenHistoryFixture (0.00s)
=== RUN   TestGoldenWireEnvelopeMapsToDomain
--- PASS: TestGoldenWireEnvelopeMapsToDomain (0.00s)
PASS
ok  	github.com/calebhabesh/file-sync/model	0.258s
```
**Status:** PASS (0.258s).

---

### Command 5: Fault Injection Suite
```sh
make test-faults
```
**Output:**
```
go test -count=1 -v ./tests/faults/...
=== RUN   TestP04RecoveryJournalBoundaries
--- PASS: TestP04RecoveryJournalBoundaries (0.42s)
=== RUN   TestP06TransferKillRestartBoundaries
--- PASS: TestP06TransferKillRestartBoundaries (0.23s)
=== RUN   TestP16StorageBarrierSmoke
--- PASS: TestP16StorageBarrierSmoke (0.02s)
=== RUN   TestP16CheckpointBoundaries
--- PASS: TestP16CheckpointBoundaries (0.03s)
=== RUN   TestP16GCBoundaries
--- PASS: TestP16GCBoundaries (0.04s)
=== RUN   TestP16ControlResolutionBoundaries
--- PASS: TestP16ControlResolutionBoundaries (0.03s)
=== RUN   TestP16IntegrityQuarantineRepairBoundaries
--- PASS: TestP16IntegrityQuarantineRepairBoundaries (0.02s)
=== RUN   TestP16InvariantI01_ImmutableVersionIDOneEnvelope
--- PASS: TestP16InvariantI01_ImmutableVersionIDOneEnvelope (0.01s)
=== RUN   TestP16InvariantI02_SameValidHistoryEquivalentHeads
--- PASS: TestP16InvariantI02_SameValidHistoryEquivalentHeads (0.00s)
=== RUN   TestP16InvariantI03_ConcurrentHeadsSurviveUntilResolution
--- PASS: TestP16InvariantI03_ConcurrentHeadsSurviveUntilResolution (0.01s)
=== RUN   TestP16InvariantI04_OrdinaryCaptureDoesNotResolveReceivedHeads
--- PASS: TestP16InvariantI04_OrdinaryCaptureDoesNotResolveReceivedHeads (0.00s)
=== RUN   TestP16InvariantI05_NoStoredReceiptWithoutDurableMetadataAndContent
--- PASS: TestP16InvariantI05_NoStoredReceiptWithoutDurableMetadataAndContent (0.01s)
=== RUN   TestP16InvariantI06_PartialOrCorruptContentNeverPublished
--- PASS: TestP16InvariantI06_PartialOrCorruptContentNeverPublished (0.01s)
=== RUN   TestP16InvariantI07_RecoveryPreservesProtectedVersionsAndReportsAmbiguity
--- PASS: TestP16InvariantI07_RecoveryPreservesProtectedVersionsAndReportsAmbiguity (0.01s)
=== RUN   TestP16InvariantI08_AtomicCounterAndNoIdentityRollbackReuse
--- PASS: TestP16InvariantI08_AtomicCounterAndNoIdentityRollbackReuse (0.01s)
=== RUN   TestP16InvariantI09_PeerInputCannotEscapeAuthorizedFolder
--- PASS: TestP16InvariantI09_PeerInputCannotEscapeAuthorizedFolder (0.00s)
=== RUN   TestP16InvariantI10_GCNeverRemovesProtectedContent
--- PASS: TestP16InvariantI10_GCNeverRemovesProtectedContent (0.01s)
=== RUN   TestP16InvariantI11_UnavailableRootsIncompleteScansNeverDelete
--- PASS: TestP16InvariantI11_UnavailableRootsIncompleteScansNeverDelete (0.01s)
=== RUN   TestP16InvariantI12_StructuralOperationsPreserveChildBytes
--- PASS: TestP16InvariantI12_StructuralOperationsPreserveChildBytes (0.00s)
=== RUN   TestP16InvariantI13_BoundedWorkAndResourceLimits
--- PASS: TestP16InvariantI13_BoundedWorkAndResourceLimits (0.05s)
=== RUN   TestP16InvariantI14_ThirdPartyForwardingPreservesAuthorAncestry
--- PASS: TestP16InvariantI14_ThirdPartyForwardingPreservesAuthorAncestry (0.00s)
=== RUN   TestP16InvariantI15_MembershipRetirementRejectionAndStaleRejoin
--- PASS: TestP16InvariantI15_MembershipRetirementRejectionAndStaleRejoin (0.00s)
=== RUN   TestP16InvariantI16_RestoreResolutionIdempotentAndStaleRejected
--- PASS: TestP16InvariantI16_RestoreResolutionIdempotentAndStaleRejected (0.01s)
=== RUN   TestP16InvariantI17_ScanAfterApplyDoesNotFabricateEdits
--- PASS: TestP16InvariantI17_ScanAfterApplyDoesNotFabricateEdits (0.01s)
=== RUN   TestP16InvariantI18_CorruptionYieldsUnavailableOrVerifiedRepair
--- PASS: TestP16InvariantI18_CorruptionYieldsUnavailableOrVerifiedRepair (0.01s)
=== RUN   TestP16InvariantI19_UICLIParityAndQualifiedProgress
--- PASS: TestP16InvariantI19_UICLIParityAndQualifiedProgress (0.01s)
=== RUN   TestP16InvariantI20_IncompatibleSchemaLimitsPreserveRecoverableState
--- PASS: TestP16InvariantI20_IncompatibleSchemaLimitsPreserveRecoverableState (0.01s)
=== RUN   FuzzProtocolEnvelopeDecode
--- PASS: FuzzProtocolEnvelopeDecode (0.00s)
=== RUN   FuzzPathSanitization
--- PASS: FuzzPathSanitization (0.00s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	1.042s
```
**Status:** PASS (1.042s).

---

### Command 6: Concurrency & Race Detection Suite
```sh
make test-race
```
**Output:**
```
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01' -o bin/filesync ./cmd/filesync
ln -sf filesync bin/orbit
go test -race ./...
ok  	github.com/calebhabesh/file-sync/cmd/filesync	1.697s
?   	github.com/calebhabesh/file-sync/internal/app	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/config	(cached)
ok  	github.com/calebhabesh/file-sync/internal/control	17.615s
ok  	github.com/calebhabesh/file-sync/internal/history	(cached)
?   	github.com/calebhabesh/file-sync/internal/launcher	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/protocol	(cached)
ok  	github.com/calebhabesh/file-sync/internal/replication	71.822s
ok  	github.com/calebhabesh/file-sync/internal/repository	68.571s
ok  	github.com/calebhabesh/file-sync/internal/scheduler	13.244s
ok  	github.com/calebhabesh/file-sync/internal/state	(cached)
ok  	github.com/calebhabesh/file-sync/internal/testkit	(cached)
ok  	github.com/calebhabesh/file-sync/internal/workspace	(cached)
ok  	github.com/calebhabesh/file-sync/model	(cached)
?   	github.com/calebhabesh/file-sync/scripts	[no test files]
ok  	github.com/calebhabesh/file-sync/tests/designgates	(cached)
ok  	github.com/calebhabesh/file-sync/tests/faults	34.114s
ok  	github.com/calebhabesh/file-sync/tests/integration	49.367s
?   	github.com/calebhabesh/file-sync/web	[no test files]
```
**Status:** PASS. 0 race warnings.

---

### Command 7: Full Verification Gate
```sh
make check
```
**Output:**
```
go test -count=1 ./...
...
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	1.177s
python3 -m unittest discover -s scripts/validation -p 'test_*.py'
...
Ran 3 tests in 0.001s
OK
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01' -o bin/filesync-linux-arm64 ./cmd/filesync
go run ./scripts/build_packages.go
Release packaging complete! Generated artifacts:
  - filesync-v1.0.0-linux-amd64.tar.gz (13111553 bytes)
  - filesync_1.0.0_amd64.deb (13107922 bytes)
  - filesync-1.0.0-1.x86_64.rpm (13107798 bytes)
  - filesync-v1.0.0-linux-arm64.tar.gz (12315324 bytes)
  - filesync_1.0.0_arm64.deb (12311706 bytes)
  - filesync-1.0.0-1.aarch64.rpm (12310617 bytes)
  - SHA256SUMS
```
**Status:** PASS (Exit code 0).
