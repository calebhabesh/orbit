# P09 Command Execution Log (2026-09-23)

## 1. Model Verification

```bash
$ make test-model
go test -count=1 -v ./model/...
=== RUN   TestBoundedExhaustiveActorSchedulesAgree
--- PASS: TestBoundedExhaustiveActorSchedulesAgree (0.00s)
=== RUN   TestDeterministicGeneratedSchedulesAgree
--- PASS: TestDeterministicGeneratedSchedulesAgree (0.10s)
=== RUN   TestOracleCatchesIntentionalDominanceMutation
--- PASS: TestOracleCatchesIntentionalDominanceMutation (0.00s)
=== RUN   TestOracleKeepsAvailabilityAndRetirementSeparateFromHeads
--- PASS: TestOracleKeepsAvailabilityAndRetirementSeparateFromHeads (0.00s)
=== RUN   TestGoldenHistoryFixture
--- PASS: TestGoldenHistoryFixture (0.00s)
=== RUN   TestGoldenWireEnvelopeMapsToDomain
--- PASS: TestGoldenWireEnvelopeMapsToDomain (0.00s)
PASS
ok  	github.com/calebhabesh/file-sync/model	0.106s
```

## 2. Check Suite (vet, format, unit, model, integration, builds)

```bash
$ make check
fmt-check: PASS
go vet ./...: PASS
go test ./internal/... ./model/...: PASS
go test ./tests/integration/...: PASS
  === RUN   TestP00ScaffoldContract
  --- PASS: TestP00ScaffoldContract (0.00s)
  === RUN   TestP04CLIFilesystemStateTransitions
  --- PASS: TestP04CLIFilesystemStateTransitions (0.42s)
  === RUN   TestP06CLIPairingAndSync
  --- PASS: TestP06CLIPairingAndSync (0.41s)
  === RUN   TestP07CLIBidirectionalReconciliation
  --- PASS: TestP07CLIBidirectionalReconciliation (0.44s)
  === RUN   TestP08ReviewedResolutionAndHistoricalRestore
  --- PASS: TestP08ReviewedResolutionAndHistoricalRestore (0.57s)
  === RUN   TestP09ThreePeerForwardingAndMembershipLifecycle
  --- PASS: TestP09ThreePeerForwardingAndMembershipLifecycle (0.86s)
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=dev' -o bin/filesync ./cmd/filesync: PASS
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build  -trimpath -ldflags '-X main.version=dev' -o bin/filesync-linux-arm64 ./cmd/filesync: PASS
```

## 3. Race Detector Verification

```bash
$ make test-race
go test -race ./...
ok  	github.com/calebhabesh/file-sync/internal/control	4.642s
ok  	github.com/calebhabesh/file-sync/internal/protocol	1.007s
ok  	github.com/calebhabesh/file-sync/internal/replication	19.353s
ok  	github.com/calebhabesh/file-sync/internal/repository	4.677s
ok  	github.com/calebhabesh/file-sync/internal/workspace	16.173s
ok  	github.com/calebhabesh/file-sync/tests/faults	11.486s
ok  	github.com/calebhabesh/file-sync/tests/integration	3.520s
(Zero race warnings observed)
```

## 4. Fault Injection & Design Gates

```bash
$ make test-faults
go test -count=1 -v ./tests/designgates/... ./tests/faults/...
PASS: TestD1DeterministicHistorySerialization
PASS: TestD1CounterOverflowEnforced
PASS: TestD1ClockMapRejectsNonmonotonicLocalCounter
PASS: TestD2PayloadVerifiedBeforeDurableRegistration
PASS: TestD2CorruptPayloadQuarantinedWithoutMetadataPromotion
PASS: TestD3RetirementSnapshotMatchesGoldenFixture
PASS: TestD3AcceptedAncestryDistinctFromRejectedRetiredEvents
PASS: TestD4DirectIOAlignmentBoundary
PASS: TestD4SyncfsRejectionOnUnsupportedFilesystem
PASS: TestD5ParentDeleteOrFileVsChildIsStructuralConflict
PASS: TestD5DirectoryDeletePreviewInvalidatedByNewChild
PASS: TestD5BootstrapAndIncompleteScansNeverInferDeletion
PASS: TestD5DivergentEnrollmentCreatesIndependentHistories
PASS: TestP03KillRestartBoundaries (all 7 boundaries)
PASS: TestP04PublicationKillRestartBoundaries (all 10 boundaries)
PASS: TestP04NewFileKillRestartBoundaries (all 8 boundaries)
PASS: TestP04DirectoryTombstoneKillRestart (all 10 boundaries)
PASS: TestP06TransferKillRestartBoundaries (all 5 boundaries)
```

## 5. Uncached Package Tests & Diff Check

```bash
$ go test -count=1 ./...
PASS across all packages.

$ git diff --check
(clean, no trailing whitespace or format issues)
```
