# P12 Verification Commands and Evidence

**Environment:** Linux 7.2.6-arch2-1 x86_64, ext4 filesystem, real TLS 1.3 loopback mTLS.
**Execution Date:** 2026-09-23.

---

### 1. P12 Bounded Continuous Operation Integration Suite

```bash
go test -v ./tests/integration/... -run TestP12
```
**Output:**
```
=== RUN   TestP12BoundedQueueAndFairScheduling
--- PASS: TestP12BoundedQueueAndFairScheduling (0.09s)
=== RUN   TestP12EqualSizeTimestampPreservingEdits
--- PASS: TestP12EqualSizeTimestampPreservingEdits (0.45s)
=== RUN   TestP12WatcherFeedbackSuppression
--- PASS: TestP12WatcherFeedbackSuppression (0.41s)
=== RUN   TestP12RootUnavailablePauseFolderNoDeletions
--- PASS: TestP12RootUnavailablePauseFolderNoDeletions (0.35s)
=== RUN   TestP12WorkStatusRetryCancelAndList
--- PASS: TestP12WorkStatusRetryCancelAndList (0.35s)
=== RUN   TestP12ContinuousServeProfilesAndShutdown
--- PASS: TestP12ContinuousServeProfilesAndShutdown (1.56s)
=== RUN   TestP12BandwidthLimiterDirect
--- PASS: TestP12BandwidthLimiterDirect (1.00s)
=== RUN   TestP12NotificationLossRecoveredByReconciliationScan
--- PASS: TestP12NotificationLossRecoveredByReconciliationScan (1.60s)
=== RUN   TestP12RetryClassificationAndExhaustion
--- PASS: TestP12RetryClassificationAndExhaustion (0.00s)
=== RUN   TestP12CrashRecoveryOfInFlightTasks
--- PASS: TestP12CrashRecoveryOfInFlightTasks (0.01s)
=== RUN   TestP12ResourceLimitsValidation
--- PASS: TestP12ResourceLimitsValidation (0.00s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/integration	5.831s
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
?   	github.com/calebhabesh/file-sync/internal/app	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/config	0.002s
ok  	github.com/calebhabesh/file-sync/internal/control	0.178s
ok  	github.com/calebhabesh/file-sync/internal/history	0.002s
ok  	github.com/calebhabesh/file-sync/internal/protocol	0.003s
ok  	github.com/calebhabesh/file-sync/internal/replication	0.910s
ok  	github.com/calebhabesh/file-sync/internal/repository	0.344s
ok  	github.com/calebhabesh/file-sync/internal/scheduler	1.389s
ok  	github.com/calebhabesh/file-sync/internal/state	0.005s
ok  	github.com/calebhabesh/file-sync/internal/testkit	0.003s
ok  	github.com/calebhabesh/file-sync/internal/workspace	0.723s
ok  	github.com/calebhabesh/file-sync/model	0.197s
go test ./tests/integration/...
ok  	github.com/calebhabesh/file-sync/tests/integration	10.187s
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=dev' -o bin/filesync ./cmd/filesync
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build  -trimpath -ldflags '-X main.version=dev' -o bin/filesync-linux-arm64 ./cmd/filesync
```

---

### 3. Thread Safety and Race Detection

```bash
make test-race
```
**Output:**
```
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
ok  	github.com/calebhabesh/file-sync/tests/designgates	(cached)
ok  	github.com/calebhabesh/file-sync/tests/faults	(cached)
ok  	github.com/calebhabesh/file-sync/tests/integration	16.905s
```

---

### 4. Fault Injection & SIGKILL Boundary Restarts

```bash
make test-faults
```
**Output:**
```
=== RUN   TestP03KillRestartBoundaries
--- PASS: TestP03KillRestartBoundaries (0.07s)
=== RUN   TestP04PublicationKillRestartBoundaries
--- PASS: TestP04PublicationKillRestartBoundaries (0.12s)
=== RUN   TestP04NewFileKillRestartBoundaries
--- PASS: TestP04NewFileKillRestartBoundaries (0.10s)
=== RUN   TestP04ParentCreationKillDoesNotAuthorScaffold
--- PASS: TestP04ParentCreationKillDoesNotAuthorScaffold (0.01s)
=== RUN   TestP04DirectoryTombstoneKillRestart
--- PASS: TestP04DirectoryTombstoneKillRestart (0.10s)
=== RUN   TestP06TransferKillRestartBoundaries
--- PASS: TestP06TransferKillRestartBoundaries (0.20s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	0.604s
```

---

### 5. Uncached Full Suite Run

```bash
go test -count=1 ./...
```
**Output:**
```
?   	github.com/calebhabesh/file-sync/cmd/filesync	[no test files]
?   	github.com/calebhabesh/file-sync/internal/app	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/config	0.002s
ok  	github.com/calebhabesh/file-sync/internal/control	0.251s
ok  	github.com/calebhabesh/file-sync/internal/history	0.002s
ok  	github.com/calebhabesh/file-sync/internal/protocol	0.003s
ok  	github.com/calebhabesh/file-sync/internal/replication	1.091s
ok  	github.com/calebhabesh/file-sync/internal/repository	0.443s
ok  	github.com/calebhabesh/file-sync/internal/scheduler	1.386s
ok  	github.com/calebhabesh/file-sync/internal/state	0.005s
ok  	github.com/calebhabesh/file-sync/internal/testkit	0.003s
ok  	github.com/calebhabesh/file-sync/internal/workspace	0.723s
ok  	github.com/calebhabesh/file-sync/model	0.197s
ok  	github.com/calebhabesh/file-sync/tests/designgates	0.003s
ok  	github.com/calebhabesh/file-sync/tests/faults	0.851s
ok  	github.com/calebhabesh/file-sync/tests/integration	9.862s
```
