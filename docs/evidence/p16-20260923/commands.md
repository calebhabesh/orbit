# Packet P16: Executed Commands and Verifications

**Date:** 2026-09-23
**Environment:** Linux (Arch rolling, x86_64, Kernel 6.16), Go 1.27.1

---

## 1. Full Project Verification Pipeline (`make check`)

### Command:
```bash
make check
```

### Actual Output:
```text
test -z "$(gofmt -l $(find . -name '*.go' -not -path './.git/*'))" || (gofmt -l $(find . -name '*.go' -not -path './.git/*'); exit 1)
go vet ./...
go test ./internal/... ./model/...
?   	github.com/calebhabesh/file-sync/internal/app	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/config	(cached)
ok  	github.com/calebhabesh/file-sync/internal/control	(cached)
ok  	github.com/calebhabesh/file-sync/internal/history	(cached)
ok  	github.com/calebhabesh/file-sync/internal/protocol	(cached)
ok  	github.com/calebhabesh/file-sync/internal/replication	(cached)
ok  	github.com/calebhabesh/file-sync/internal/repository	(cached)
ok  	github.com/calebhabesh/file-sync/internal/scheduler	(cached)
ok  	github.com/calebhabesh/file-sync/internal/state	(cached)
ok  	github.com/calebhabesh/file-sync/internal/testkit	(cached)
ok  	github.com/calebhabesh/file-sync/internal/workspace	(cached)
ok  	github.com/calebhabesh/file-sync/model	(cached)
go test ./tests/integration/...
ok  	github.com/calebhabesh/file-sync/tests/integration	13.774s
go test -count=1 -v ./model/...
=== RUN   TestModel_DeterministicConvergence
--- PASS: TestModel_DeterministicConvergence (0.01s)
=== RUN   TestModel_GarbageCollectionPreservesReachability
--- PASS: TestModel_GarbageCollectionPreservesReachability (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/model	0.021s
go test -count=1 -v ./tests/designgates/... ./tests/faults/...
=== RUN   TestD1_FolderSeparation
--- PASS: TestD1_FolderSeparation (0.05s)
=== RUN   TestD2_StalePeerRejection
--- PASS: TestD2_StalePeerRejection (0.04s)
=== RUN   TestD3_AtomicCounterAndIdentityReset
--- PASS: TestD3_AtomicCounterAndIdentityReset (0.06s)
=== RUN   TestD4_DeterministicHeadsEvaluation
--- PASS: TestD4_DeterministicHeadsEvaluation (0.01s)
=== RUN   TestD5_TwoPhasePublicationSafety
--- PASS: TestD5_TwoPhasePublicationSafety (0.08s)
=== RUN   TestP03CrashDuringChunkWrite
--- PASS: TestP03CrashDuringChunkWrite (0.05s)
=== RUN   TestP04PublicationCrashRecoveryBoundaries
--- PASS: TestP04PublicationCrashRecoveryBoundaries (0.06s)
=== RUN   TestP06TransferKillRestartBoundaries
--- PASS: TestP06TransferKillRestartBoundaries (0.21s)
=== RUN   TestP16AbruptResetStorageAssumptions
=== RUN   TestP16AbruptResetStorageAssumptions/FsyncFlushDurabilityVsUnflushedDiscard
=== RUN   TestP16AbruptResetStorageAssumptions/SQLiteWALAbruptRecovery
=== RUN   TestP16AbruptResetStorageAssumptions/TwoPhasePublicationCrashScenarios
--- PASS: TestP16AbruptResetStorageAssumptions (0.01s)
=== RUN   TestP16CheckpointBoundaries
=== RUN   TestP16CheckpointBoundaries/sql.checkpoint.before
=== RUN   TestP16CheckpointBoundaries/sql.checkpoint.after
--- PASS: TestP16CheckpointBoundaries (0.02s)
=== RUN   TestP16GCBoundaries
=== RUN   TestP16GCBoundaries/gc.intent
=== RUN   TestP16GCBoundaries/gc.unlink
=== RUN   TestP16GCBoundaries/gc.finalization
--- PASS: TestP16GCBoundaries (0.03s)
=== RUN   TestP16ControlResolutionBoundaries
=== RUN   TestP16ControlResolutionBoundaries/control.select.committed
=== RUN   TestP16ControlResolutionBoundaries/control.restore.committed
--- PASS: TestP16ControlResolutionBoundaries (0.02s)
=== RUN   TestP16IntegrityQuarantineRepairBoundaries
=== RUN   TestP16IntegrityQuarantineRepairBoundaries/integrity.chunk.quarantined
=== RUN   TestP16IntegrityQuarantineRepairBoundaries/repair.installed
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
=== RUN   TestP16HarnessRefusesUnsafePaths
--- PASS: TestP16HarnessRefusesUnsafePaths (0.00s)
=== RUN   TestP16ScenarioMatrix_AllRowsCovered
--- PASS: TestP16ScenarioMatrix_AllRowsCovered (0.00s)
=== RUN   FuzzProtocolEnvelopeDecode
--- PASS: FuzzProtocolEnvelopeDecode (0.00s)
=== RUN   FuzzPathSanitization
--- PASS: FuzzPathSanitization (0.00s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	0.910s
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=7f0cb81 -X main.date=2026-09-23' -o bin/filesync ./cmd/filesync
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=7f0cb81 -X main.date=2026-09-23' -o bin/filesync-linux-arm64 ./cmd/filesync
go run ./scripts/build_packages.go
Release packaging complete! Generated artifacts:
  - filesync-v1.0.0-linux-amd64.tar.gz (12295125 bytes)
  - filesync_1.0.0_amd64.deb (12293596 bytes)
  - filesync-1.0.0-1.x86_64.rpm (12289753 bytes)
  - filesync-v1.0.0-linux-arm64.tar.gz (11556706 bytes)
  - filesync_1.0.0_arm64.deb (11555196 bytes)
  - filesync-1.0.0-1.aarch64.rpm (11554385 bytes)
  - SHA256SUMS
```

---

## 2. Race Detection Verification across Entire Codebase

### Command:
```bash
make test-race
```

### Actual Output:
```text
go test -race ./...
?   	github.com/calebhabesh/file-sync/cmd/filesync	[no test files]
?   	github.com/calebhabesh/file-sync/internal/app	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/config	(cached)
ok  	github.com/calebhabesh/file-sync/internal/control	(cached)
ok  	github.com/calebhabesh/file-sync/internal/history	(cached)
ok  	github.com/calebhabesh/file-sync/internal/protocol	(cached)
ok  	github.com/calebhabesh/file-sync/internal/replication	(cached)
ok  	github.com/calebhabesh/file-sync/internal/repository	(cached)
ok  	github.com/calebhabesh/file-sync/internal/scheduler	(cached)
ok  	github.com/calebhabesh/file-sync/internal/state	(cached)
ok  	github.com/calebhabesh/file-sync/internal/testkit	(cached)
ok  	github.com/calebhabesh/file-sync/internal/workspace	(cached)
ok  	github.com/calebhabesh/file-sync/model	(cached)
?   	github.com/calebhabesh/file-sync/scripts	[no test files]
ok  	github.com/calebhabesh/file-sync/tests/designgates	(cached)
ok  	github.com/calebhabesh/file-sync/tests/faults	26.322s
ok  	github.com/calebhabesh/file-sync/tests/integration	23.644s
?   	github.com/calebhabesh/file-sync/web	[no test files]
```

---

## 3. Dedicated Invariant Verification Suite (I01–I20)

### Command:
```bash
go test -v -run 'TestP16Invariant' ./tests/faults/...
```

### Actual Output:
```text
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
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	0.190s
```

---

## 4. Controlled Abrupt Reset & Storage Assumptions

### Command:
```bash
go test -v -run 'TestP16Abrupt' ./tests/faults/...
```

### Actual Output:
```text
=== RUN   TestP16AbruptResetStorageAssumptions
=== RUN   TestP16AbruptResetStorageAssumptions/FsyncFlushDurabilityVsUnflushedDiscard
=== RUN   TestP16AbruptResetStorageAssumptions/SQLiteWALAbruptRecovery
=== RUN   TestP16AbruptResetStorageAssumptions/TwoPhasePublicationCrashScenarios
--- PASS: TestP16AbruptResetStorageAssumptions (0.02s)
    --- PASS: TestP16AbruptResetStorageAssumptions/FsyncFlushDurabilityVsUnflushedDiscard (0.00s)
    --- PASS: TestP16AbruptResetStorageAssumptions/SQLiteWALAbruptRecovery (0.01s)
    --- PASS: TestP16AbruptResetStorageAssumptions/TwoPhasePublicationCrashScenarios (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	0.027s
```

---

## 5. Pre- and Post-Operation Crash Boundaries Audit

### Command:
```bash
go test -v -run 'TestP16Checkpoint|TestP16GC|TestP16Control|TestP16Integrity' ./tests/faults/...
```

### Actual Output:
```text
=== RUN   TestP16CheckpointBoundaries
=== RUN   TestP16CheckpointBoundaries/sql.checkpoint.before
=== RUN   TestP16CheckpointBoundaries/sql.checkpoint.after
--- PASS: TestP16CheckpointBoundaries (0.02s)
=== RUN   TestP16GCBoundaries
=== RUN   TestP16GCBoundaries/gc.intent
=== RUN   TestP16GCBoundaries/gc.unlink
=== RUN   TestP16GCBoundaries/gc.finalization
--- PASS: TestP16GCBoundaries (0.03s)
=== RUN   TestP16ControlResolutionBoundaries
=== RUN   TestP16ControlResolutionBoundaries/control.select.committed
=== RUN   TestP16ControlResolutionBoundaries/control.restore.committed
--- PASS: TestP16ControlResolutionBoundaries (0.02s)
=== RUN   TestP16IntegrityQuarantineRepairBoundaries
=== RUN   TestP16IntegrityQuarantineRepairBoundaries/integrity.chunk.quarantined
=== RUN   TestP16IntegrityQuarantineRepairBoundaries/repair.installed
--- PASS: TestP16IntegrityQuarantineRepairBoundaries (0.02s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	0.104s
```

---

## 6. Fuzzing Protocol Envelopes and Paths

### Commands:
```bash
go test -fuzz=FuzzProtocolEnvelopeDecode -fuzztime=3s ./tests/faults/...
go test -fuzz=FuzzPathSanitization -fuzztime=3s ./tests/faults/...
```

### Actual Output:
```text
fuzz: elapsed: 0s, gathering baseline coverage: 0/4 completed
fuzz: elapsed: 0s, gathering baseline coverage: 4/4 completed, now fuzzing with 16 workers
fuzz: elapsed: 3s, execs: 75680 (25216/sec), new interesting: 139 (total: 143)
fuzz: elapsed: 4s, execs: 75680 (0/sec), new interesting: 139 (total: 143)
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	6.440s

fuzz: elapsed: 0s, gathering baseline coverage: 0/10 completed
fuzz: elapsed: 0s, gathering baseline coverage: 10/10 completed, now fuzzing with 16 workers
fuzz: elapsed: 3s, execs: 187541 (62508/sec), new interesting: 40 (total: 50)
fuzz: elapsed: 4s, execs: 187541 (0/sec), new interesting: 40 (total: 50)
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	6.390s
```

---

## 7. Safe Local Multi-Process Demo Execution

### Command:
```bash
make demo
```

### Actual Output:
```text
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=7f0cb81 -X main.date=2026-09-23' -o bin/filesync ./cmd/filesync
go run ./scripts/local_demo.go --quick
===============================================================
       File Sync: Local Multi-Process Replication Demo
===============================================================

[Step 1] Initializing Node A (Alice) and Node B (Bob) with mutual TLS
       Node A Device: 1d2b2113c23e... (Key Pin: 4d5a9e1d65968801...)
       Node B Device: cb80e726e332... (Key Pin: 84492c12ef52afb3...)
       ✓ Mutual pairing approved for shared folder: dddddddddddd...

[Step 2] Demonstrating normal file creation and verified transfer
       Node A authors 'README.md'
       Node A serving TLS on https://127.0.0.1:38073
       ✓ Node B received 'README.md' and verified SHA-256 chunk byte-for-byte

[Step 3] Simulating offline partition & concurrent edits on both nodes
       Node A (offline) modifies 'architecture.md' (Proposal: P2P)
       Node B (offline) modifies 'architecture.md' (Proposal: VPS Hub)
       ✓ Both nodes captured independent local versions into causal DAG

[Step 4] Reconnecting nodes: bidirectional gossip and conflict detection
       Detected Conflict on 'architecture.md':
          Head 1: Author=1d2b2113, Counter=2
          Head 2: Author=cb80e726, Counter=1
       ✓ Invariant I03 Preserved: Concurrent edits retained without silent overwrite

[Step 5] Operator performs reviewed conflict resolution via CLI
       Resolving 'architecture.md' by selecting Head 1 with token 67896793485f...
       ✓ Resolution committed atomically to causal version DAG
       ✓ Both Node A and Node B converged with 0 remaining conflicts

[Step 6] Demonstration complete: Clean process shutdown and disposable teardown
       ✓ All invariants verified: authenticated TLS, verified chunk transfer,
       ✓ causal concurrency preservation, reviewed resolution, and convergence.

[PASS] Local multi-process demo completed successfully.

       Cleaning up disposable environment: /tmp/filesync-demo-2455623645
```
