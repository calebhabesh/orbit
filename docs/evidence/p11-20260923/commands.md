# P11 Verification Commands and Evidence

**Environment:** Linux 7.2.6-arch2-1 x86_64, ext4 filesystem, real TLS 1.3 loopback mTLS.
**Execution Date:** 2026-09-23.

---

### 1. Reference-Set Oracle Corruption & Repair Verification

```bash
go test -count=1 -v ./model/... -run TestReferenceSetOracle
```
**Output:**
```
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
=== RUN   TestReferenceSetOracleCorruptionAndRepair
=== RUN   TestReferenceSetOracleCorruptionAndRepair/shared-chunk-corruption-diagnoses-all-versions
=== RUN   TestReferenceSetOracleCorruptionAndRepair/pins-during-repair-protect-from-gc
=== RUN   TestReferenceSetOracleCorruptionAndRepair/distinguish-all-availability-states
--- PASS: TestReferenceSetOracleCorruptionAndRepair (0.00s)
PASS
ok  	github.com/calebhabesh/file-sync/model	0.001s
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
ok  	github.com/calebhabesh/file-sync/internal/control	0.139s
ok  	github.com/calebhabesh/file-sync/internal/replication	0.728s
ok  	github.com/calebhabesh/file-sync/internal/repository	0.206s
ok  	github.com/calebhabesh/file-sync/internal/workspace	0.446s
ok  	github.com/calebhabesh/file-sync/model	0.128s
go test ./tests/integration/...
ok  	github.com/calebhabesh/file-sync/tests/integration	3.990s
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
ok  	github.com/calebhabesh/file-sync/internal/control	5.544s
ok  	github.com/calebhabesh/file-sync/internal/replication	22.893s
ok  	github.com/calebhabesh/file-sync/internal/repository	7.901s
ok  	github.com/calebhabesh/file-sync/internal/workspace	17.381s
ok  	github.com/calebhabesh/file-sync/model	1.562s
ok  	github.com/calebhabesh/file-sync/tests/faults	12.510s
ok  	github.com/calebhabesh/file-sync/tests/integration	7.464s
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
--- PASS: TestP04PublicationKillRestartBoundaries (0.09s)
=== RUN   TestP04NewFileKillRestartBoundaries
--- PASS: TestP04NewFileKillRestartBoundaries (0.08s)
=== RUN   TestP04DirectoryTombstoneKillRestart
--- PASS: TestP04DirectoryTombstoneKillRestart (0.08s)
=== RUN   TestP06TransferKillRestartBoundaries
--- PASS: TestP06TransferKillRestartBoundaries (0.18s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	0.494s
```

---

### 5. P11 CLI Integration Verification

```bash
go test -v -count=1 ./tests/integration -run TestP11
```
**Output:**
```
=== RUN   TestP11IntegrityScanAndQuarantineAffectedVersions
--- PASS: TestP11IntegrityScanAndQuarantineAffectedVersions (0.31s)
=== RUN   TestP11PeerAssistedRepairAndInvariantPreservation
--- PASS: TestP11PeerAssistedRepairAndInvariantPreservation (0.33s)
=== RUN   TestP11NoRemainingCopyYieldsExplicitUnavailable
--- PASS: TestP11NoRemainingCopyYieldsExplicitUnavailable (0.01s)
=== RUN   TestP11UnauthorizedFolderRepairRejected
--- PASS: TestP11UnauthorizedFolderRepairRejected (0.01s)
=== RUN   TestP11IntegrityScanAndRepairPinningPreservesGCInterleavings
--- PASS: TestP11IntegrityScanAndRepairPinningPreservesGCInterleavings (0.01s)
=== RUN   TestP11AvailabilityStateClassification
--- PASS: TestP11AvailabilityStateClassification (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/integration	0.676s
```
