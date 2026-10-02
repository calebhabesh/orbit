# Orbit Packet O09 Verification Commands and Diagnoses

This log records the commands, environment, diagnoses, and exact terminal outcomes for Packet O09 verification.

## 1. Environment

- **OS:** Linux (x86_64)
- **Go Version:** `go version go1.24.4 linux/amd64`
- **Compiler Flags:** `-trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01'`
- **Target Architecture:** `linux/amd64` (native), `linux/arm64` (cross-build)

---

## 2. Planned Checks & Diagnostic Traces

### Command 1: Planned Test Suite Across Packages
```sh
go test -count=1 -v ./internal/workspace ./internal/repository ./internal/control ./tests/integration -run 'TestOrbitImport|TestOrbitMkdir|TestOrbitMove|TestOrbitDelete|TestOrbitMutation'
```
**Output:**
```
=== RUN   TestOrbitMutationWorkspace
--- PASS: TestOrbitMutationWorkspace (0.02s)
PASS
ok  	github.com/calebhabesh/file-sync/internal/workspace	0.022s
=== RUN   TestOrbitMutationJournal
--- PASS: TestOrbitMutationJournal (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/internal/repository	0.015s
=== RUN   TestOrbitMutationControl
--- PASS: TestOrbitMutationControl (0.02s)
PASS
ok  	github.com/calebhabesh/file-sync/internal/control	0.019s
=== RUN   TestOrbitImport
--- PASS: TestOrbitImport (0.02s)
=== RUN   TestOrbitMkdir
--- PASS: TestOrbitMkdir (0.01s)
=== RUN   TestOrbitMove
--- PASS: TestOrbitMove (0.03s)
=== RUN   TestOrbitDelete
--- PASS: TestOrbitDelete (0.02s)
=== RUN   TestOrbitMutation
=== RUN   TestOrbitMutation/FaultHooksAndJournalRecovery
=== RUN   TestOrbitMutation/CLIPartityStoppedAndLiveDaemon
--- PASS: TestOrbitMutation (0.06s)
    --- PASS: TestOrbitMutation/FaultHooksAndJournalRecovery (0.01s)
    --- PASS: TestOrbitMutation/CLIPartityStoppedAndLiveDaemon (0.04s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/integration	0.131s
```
**Status:** PASS (0.187s total).

### Command 2: Model Consistency Suite
```sh
make test-model
```
**Output:**
```
go test -count=1 -v ./model/...
=== RUN   TestReferenceSetOracleInterleavings
=== RUN   TestReferenceSetOracleInterleavings/[begin_reference_unlink]
=== RUN   TestReferenceSetOracleInterleavings/[begin_unlink_reference]
=== RUN   TestReferenceSetOracleInterleavings/[reference_begin_unlink]
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
--- PASS: TestDeterministicGeneratedSchedulesAgree (0.29s)
=== RUN   TestOracleCatchesIntentionalDominanceMutation
--- PASS: TestOracleCatchesIntentionalDominanceMutation (0.00s)
=== RUN   TestOracleKeepsAvailabilityAndRetirementSeparateFromHeads
--- PASS: TestOracleKeepsAvailabilityAndRetirementSeparateFromHeads (0.00s)
=== RUN   TestGoldenHistoryFixture
--- PASS: TestGoldenHistoryFixture (0.00s)
=== RUN   TestGoldenWireEnvelopeMapsToDomain
--- PASS: TestGoldenWireEnvelopeMapsToDomain (0.00s)
PASS
ok  	github.com/calebhabesh/file-sync/model	0.290s
```
**Status:** PASS.

### Command 3: Fault Injection Suite
```sh
make test-faults
```
**Output:**
```
PASS: TestP04StructuralHelper (0.00s)
PASS: TestP06TransferKillRestartBoundaries (0.22s)
PASS: TestP16StorageBarrierSmoke (0.02s)
PASS: TestP16CheckpointBoundaries (0.02s)
PASS: TestP16GCBoundaries (0.03s)
PASS: TestP16ControlResolutionBoundaries (0.03s)
PASS: TestP16IntegrityQuarantineRepairBoundaries (0.03s)
PASS: TestP16InvariantI01_ImmutableVersionIDOneEnvelope (0.01s)
PASS: TestP16InvariantI02_SameValidHistoryEquivalentHeads (0.00s)
PASS: TestP16InvariantI03_ConcurrentHeadsSurviveUntilResolution (0.01s)
PASS: TestP16InvariantI04_OrdinaryCaptureDoesNotResolveReceivedHeads (0.00s)
PASS: TestP16InvariantI05_NoStoredReceiptWithoutDurableMetadataAndContent (0.01s)
PASS: TestP16InvariantI06_PartialOrCorruptContentNeverPublished (0.01s)
PASS: TestP16InvariantI07_RecoveryPreservesProtectedVersionsAndReportsAmbiguity (0.01s)
PASS: TestP16InvariantI08_AtomicCounterAndNoIdentityRollbackReuse (0.01s)
PASS: TestP16InvariantI09_PeerInputCannotEscapeAuthorizedFolder (0.00s)
PASS: TestP16InvariantI10_GCNeverRemovesProtectedContent (0.01s)
PASS: TestP16InvariantI11_UnavailableRootsIncompleteScansNeverDelete (0.01s)
PASS: TestP16InvariantI12_StructuralOperationsPreserveChildBytes (0.00s)
PASS: TestP16InvariantI13_BoundedWorkAndResourceLimits (0.07s)
PASS: TestP16InvariantI14_ThirdPartyForwardingPreservesAuthorAncestry (0.00s)
PASS: TestP16InvariantI15_MembershipRetirementRejectionAndStaleRejoin (0.00s)
PASS: TestP16InvariantI16_RestoreResolutionIdempotentAndStaleRejected (0.01s)
PASS: TestP16InvariantI17_ScanAfterApplyDoesNotFabricateEdits (0.01s)
PASS: TestP16InvariantI18_CorruptionYieldsUnavailableOrVerifiedRepair (0.01s)
PASS: TestP16InvariantI19_UICLIParityAndQualifiedProgress (0.01s)
PASS: TestP16InvariantI20_IncompatibleSchemaLimitsPreserveRecoverableState (0.01s)
PASS: FuzzProtocolEnvelopeDecode (0.00s)
PASS: FuzzPathSanitization (0.00s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	1.039s
```
**Status:** PASS.

### Command 4: Concurrency Race Detector
```sh
make test-race
```
**Output:**
```
ok  	github.com/calebhabesh/file-sync/cmd/filesync	1.630s
ok  	github.com/calebhabesh/file-sync/internal/control	21.895s
ok  	github.com/calebhabesh/file-sync/internal/replication	76.218s
ok  	github.com/calebhabesh/file-sync/internal/repository	71.823s
ok  	github.com/calebhabesh/file-sync/internal/scheduler	13.132s
ok  	github.com/calebhabesh/file-sync/internal/workspace	17.685s
ok  	github.com/calebhabesh/file-sync/tests/designgates	1.560s
ok  	github.com/calebhabesh/file-sync/tests/faults	39.257s
ok  	github.com/calebhabesh/file-sync/tests/integration	52.804s
```
**Status:** PASS (0 data races detected).

### Command 5: Full Project Verification Gate
```sh
make check
```
**Output:**
- All tests in all packages passed.
- Python validation scripts passed.
- Release packages built: `.tar.gz`, `.deb`, `.rpm` for `linux/amd64` and `linux/arm64`.
- SHA256SUMS generated.
**Status:** PASS (Exit code 0).
