# P13 Execution Commands and Test Outputs

## 1. Static Verification and Compilation

```bash
make check
```
Output:
```
go vet ./...
go test ./internal/... ./model/...
?   	github.com/calebhabesh/file-sync/internal/app	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/config	0.004s
ok  	github.com/calebhabesh/file-sync/internal/control	0.334s
ok  	github.com/calebhabesh/file-sync/internal/history	0.001s
ok  	github.com/calebhabesh/file-sync/internal/protocol	0.001s
ok  	github.com/calebhabesh/file-sync/internal/replication	1.330s
ok  	github.com/calebhabesh/file-sync/internal/repository	0.515s
ok  	github.com/calebhabesh/file-sync/internal/scheduler	1.415s
ok  	github.com/calebhabesh/file-sync/internal/state	0.006s
ok  	github.com/calebhabesh/file-sync/internal/testkit	0.003s
ok  	github.com/calebhabesh/file-sync/internal/workspace	0.826s
ok  	github.com/calebhabesh/file-sync/model	0.200s
go test ./tests/integration/...
ok  	github.com/calebhabesh/file-sync/tests/integration	13.087s
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=dev' -o bin/filesync ./cmd/filesync
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build  -trimpath -ldflags '-X main.version=dev' -o bin/filesync-linux-arm64 ./cmd/filesync
```

## 2. Race Condition Detection

```bash
make test-race
```
Output:
```
go test -race ./...
ok  	github.com/calebhabesh/file-sync/internal/config	(cached)
ok  	github.com/calebhabesh/file-sync/internal/control	16.061s
ok  	github.com/calebhabesh/file-sync/internal/history	1.009s
ok  	github.com/calebhabesh/file-sync/internal/protocol	1.011s
ok  	github.com/calebhabesh/file-sync/internal/replication	31.179s
ok  	github.com/calebhabesh/file-sync/internal/repository	15.848s
ok  	github.com/calebhabesh/file-sync/internal/scheduler	3.503s
ok  	github.com/calebhabesh/file-sync/internal/state	(cached)
ok  	github.com/calebhabesh/file-sync/internal/testkit	(cached)
ok  	github.com/calebhabesh/file-sync/internal/workspace	23.633s
ok  	github.com/calebhabesh/file-sync/model	1.653s
ok  	github.com/calebhabesh/file-sync/tests/designgates	(cached)
ok  	github.com/calebhabesh/file-sync/tests/faults	(cached)
ok  	github.com/calebhabesh/file-sync/tests/integration	18.438s
```

## 3. Fault Injection and Crash Recovery

```bash
make test-faults
```
Output:
```
=== RUN   TestD1ConcurrentEditConflictDetection
--- PASS: TestD1ConcurrentEditConflictDetection (0.00s)
=== RUN   TestD2TombstoneDominanceAndConcurrentRecreation
--- PASS: TestD2TombstoneDominanceAndConcurrentRecreation (0.00s)
=== RUN   TestD3ContentAddressedChunkDeduplication
--- PASS: TestD3ContentAddressedChunkDeduplication (0.00s)
=== RUN   TestD4TwoPhaseAtomicPublicationRollback
--- PASS: TestD4TwoPhaseAtomicPublicationRollback (0.00s)
=== RUN   TestD5StructuralConflictDetection
--- PASS: TestD5StructuralConflictDetection (0.00s)
=== RUN   TestP03KillRestartBoundaries
--- PASS: TestP03KillRestartBoundaries (0.07s)
=== RUN   TestP04PublicationKillRestartBoundaries
--- PASS: TestP04PublicationKillRestartBoundaries (0.13s)
=== RUN   TestP06TransferKillRestartBoundaries
--- PASS: TestP06TransferKillRestartBoundaries (0.23s)
PASS
```

## 4. Integration Test Suite for P13

```bash
go test -v ./tests/integration -run TestP13
```
Output:
```
=== RUN   TestP13CLIDiagnosticsAndDoctor
--- PASS: TestP13CLIDiagnosticsAndDoctor (0.45s)
=== RUN   TestP13SupportExportSanitizationAndRedaction
--- PASS: TestP13SupportExportSanitizationAndRedaction (0.35s)
=== RUN   TestP13FolderLifecycleAndZeroReplicatedDeletes
--- PASS: TestP13FolderLifecycleAndZeroReplicatedDeletes (0.32s)
=== RUN   TestP13MaintenanceBackupCheckRecoveryAndReset
--- PASS: TestP13MaintenanceBackupCheckRecoveryAndReset (0.31s)
=== RUN   TestP13LoopbackControlSecurityAndBootstrap
--- PASS: TestP13LoopbackControlSecurityAndBootstrap (0.33s)
=== RUN   TestP13ErrorCategoriesHaveSafeNextActions
--- PASS: TestP13ErrorCategoriesHaveSafeNextActions (0.00s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/integration	1.766s
```

## 5. CLI Verification Commands

### Doctor
```bash
./bin/filesync doctor
```
Output:
```
doctor report: overall=OK
  [OK] identity_key: identity key has valid permissions (0600)
  [OK] state_directory: state directory permissions are secure (0700)
  [OK] storage_capacity: free space satisfies reserve target (512 MiB)
  [OK] database_wal: WAL size is within soft cap (256 MiB)
  [OK] schema_version: database schema matches binary version
```

### Metrics
```bash
./bin/filesync metrics --json
```

### Support Export
```bash
./bin/filesync support-export --out /tmp/support.tar.gz
```
Output:
```
support bundle written to /tmp/support.tar.gz (size=1420 bytes, redacted=true)
```
