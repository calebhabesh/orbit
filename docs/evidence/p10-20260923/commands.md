# P10 Verification Commands and Evidence

**Environment:** Linux 7.2.6-arch2-1 x86_64, ext4 filesystem, real TLS 1.3 loopback mTLS.
**Execution Date:** 2026-09-23.

---

### 1. Reference-Set Oracle & Interleaving Verification

```bash
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
=== RUN   TestReferenceSetOracleCrashRecovery/crash-after-intent
=== RUN   TestReferenceSetOracleCrashRecovery/crash-after-unlink
--- PASS: TestReferenceSetOracleCrashRecovery (0.00s)
=== RUN   TestBoundedExhaustiveActorSchedulesAgree
--- PASS: TestBoundedExhaustiveActorSchedulesAgree (0.00s)
=== RUN   TestDeterministicGeneratedSchedulesAgree
--- PASS: TestDeterministicGeneratedSchedulesAgree (0.12s)
=== RUN   TestOracleCatchesIntentionalDominanceMutation
--- PASS: TestOracleCatchesIntentionalDominanceMutation (0.00s)
=== RUN   TestOracleKeepsAvailabilityAndRetirementSeparateFromHeads
--- PASS: TestOracleKeepsAvailabilityAndRetirementSeparateFromHeads (0.00s)
=== RUN   TestGoldenHistoryFixture
--- PASS: TestGoldenHistoryFixture (0.00s)
=== RUN   TestGoldenWireEnvelopeMapsToDomain
--- PASS: TestGoldenWireEnvelopeMapsToDomain (0.00s)
PASS
ok  	github.com/calebhabesh/file-sync/model	0.119s
```

---

### 2. Comprehensive Quality & Integration Suite

```bash
make check
```
**Output:**
```
go vet ./...
go test ./internal/... ./model/...
ok  	github.com/calebhabesh/file-sync/internal/control	0.130s
ok  	github.com/calebhabesh/file-sync/internal/replication	0.723s
ok  	github.com/calebhabesh/file-sync/internal/repository	0.188s
ok  	github.com/calebhabesh/file-sync/internal/workspace	0.493s
ok  	github.com/calebhabesh/file-sync/model	0.140s
go test ./tests/integration/...
ok  	github.com/calebhabesh/file-sync/tests/integration	3.673s
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=dev' -o bin/filesync ./cmd/filesync
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build  -trimpath -ldflags '-X main.version=dev' -o bin/filesync-linux-arm64 ./cmd/filesync
```

---

### 3. Data Race Analysis

```bash
make test-race
```
**Output:**
```
go test -race ./...
ok  	github.com/calebhabesh/file-sync/internal/control	5.823s
ok  	github.com/calebhabesh/file-sync/internal/replication	22.621s
ok  	github.com/calebhabesh/file-sync/internal/repository	7.519s
ok  	github.com/calebhabesh/file-sync/internal/workspace	18.993s
ok  	github.com/calebhabesh/file-sync/model	1.668s
ok  	github.com/calebhabesh/file-sync/tests/faults	13.687s
ok  	github.com/calebhabesh/file-sync/tests/integration	6.627s
```
*(0 race warnings detected across all packages)*

---

### 4. Design Gates and Crash Boundary Faults

```bash
make test-faults
```
**Output:**
```
PASS
ok  	github.com/calebhabesh/file-sync/tests/designgates	0.002s
=== RUN   TestP03KillRestartBoundaries
--- PASS: TestP03KillRestartBoundaries (0.05s)
=== RUN   TestP04PublicationKillRestartBoundaries
--- PASS: TestP04PublicationKillRestartBoundaries (0.10s)
=== RUN   TestP04NewFileKillRestartBoundaries
--- PASS: TestP04NewFileKillRestartBoundaries (0.09s)
=== RUN   TestP04DirectoryTombstoneKillRestart
--- PASS: TestP04DirectoryTombstoneKillRestart (0.09s)
=== RUN   TestP06TransferKillRestartBoundaries
--- PASS: TestP06TransferKillRestartBoundaries (0.19s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	0.539s
```

---

### 5. P10 CLI Integration Verification

```bash
go test -v -count=1 ./tests/integration -run TestP10
```
**Output:**
```
=== RUN   TestP10StorageAccountingAndCLI
--- PASS: TestP10StorageAccountingAndCLI (0.33s)
=== RUN   TestP10RetentionExpiryAndCrashSafeGC
--- PASS: TestP10RetentionExpiryAndCrashSafeGC (0.38s)
=== RUN   TestP10LongOfflinePeerNoResurrectedDeletions
--- PASS: TestP10LongOfflinePeerNoResurrectedDeletions (0.41s)
=== RUN   TestP10InterruptedGCFaultRecovery
--- PASS: TestP10InterruptedGCFaultRecovery (0.01s)
=== RUN   TestP10MultiPeerFallbackChunkTransfer
--- PASS: TestP10MultiPeerFallbackChunkTransfer (0.02s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/integration	1.154s
```
