# P08 commands and observed results

Run from `<repo>` on 2026-09-23.

```text
make test-model
```

Passed all independent causal model cases (150 schedules + seeded runs + intentional dominance mutation check).

```text
make check
```

Passed `fmt-check`, `go vet`, internal packages, model package, integration tests (including `TestP06CLITwoPeerTransfer`, `TestP07CLIBidirectionalReconciliation`, and `TestP08CLIResolutionRestoreControlReplay`), and built both `bin/filesync` (amd64) and `bin/filesync-linux-arm64` (arm64 cross-build).

```text
make test-race
```

Passed all packages with the Go race detector enabled. The full suite completed cleanly with zero race warnings.

```text
make test-faults
```

Passed all design gates (D1-D5) and process-kill fault boundaries:
- P01 design gates D1–D5 (publication preservation/exchange, working basis dominance, retirement admission, GC safety, structural conflicts, bootstrap safety);
- P03 object, version, and readiness commit boundaries;
- P04 existing-file exchange, new-file creation, parent/directory creation, and tombstone removal;
- P06 transfer boundaries: `transfer.chunk.verified`, `transfer.readiness.before`, `transfer.readiness.after`, `transfer.receipt.before_send`, and `transfer.receipt.after_send`. Every restart assertion succeeded.

```text
go test -v -count=1 ./internal/control
```

Observed output:
```text
=== RUN   TestResolveSelect
--- PASS: TestResolveSelect (0.01s)
=== RUN   TestResolveSelectResponseLossAndReplay
--- PASS: TestResolveSelectResponseLossAndReplay (0.01s)
=== RUN   TestResolveSelectReusedKeyWithChangedArguments
--- PASS: TestResolveSelectReusedKeyWithChangedArguments (0.01s)
=== RUN   TestResolveSelectStaleViewPreCommit
--- PASS: TestResolveSelectStaleViewPreCommit (0.01s)
=== RUN   TestResolveSelectUnseenVersionRemainsConcurrent
--- PASS: TestResolveSelectUnseenVersionRemainsConcurrent (0.01s)
=== RUN   TestResolveManualMerge
--- PASS: TestResolveManualMerge (0.01s)
=== RUN   TestRestoreHistoricalVersion
--- PASS: TestRestoreHistoricalVersion (0.01s)
=== RUN   TestRestoreExecutableStatusFollowsSelectedVersion
--- PASS: TestRestoreExecutableStatusFollowsSelectedVersion (0.01s)
=== RUN   TestRestoreExpiredPayloadReturnsContentExpired
--- PASS: TestRestoreExpiredPayloadReturnsContentExpired (0.01s)
=== RUN   TestKeepCopiesDestinationCollisionPreservesFiles
--- PASS: TestKeepCopiesDestinationCollisionPreservesFiles (0.01s)
=== RUN   TestKeepCopiesPartialResumptionNoDuplicateCopies
--- PASS: TestKeepCopiesPartialResumptionNoDuplicateCopies (0.01s)
=== RUN   TestExport
--- PASS: TestExport (0.01s)
=== RUN   TestHistoryAvailability
--- PASS: TestHistoryAvailability (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/internal/control	0.102s
```

```text
go test -v -count=1 ./tests/integration -run TestP08CLIResolutionRestoreControlReplay
```

Observed output:
```text
=== RUN   TestP08CLIResolutionRestoreControlReplay
--- PASS: TestP08CLIResolutionRestoreControlReplay (0.42s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/integration	0.425s
```

```text
go test -count=1 ./...
```

Observed output:
```text
?   	github.com/calebhabesh/file-sync/cmd/filesync	[no test files]
?   	github.com/calebhabesh/file-sync/internal/app	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/config	0.002s
ok  	github.com/calebhabesh/file-sync/internal/control	0.089s
ok  	github.com/calebhabesh/file-sync/internal/history	0.001s
ok  	github.com/calebhabesh/file-sync/internal/protocol	0.002s
ok  	github.com/calebhabesh/file-sync/internal/replication	0.612s
ok  	github.com/calebhabesh/file-sync/internal/repository	0.091s
ok  	github.com/calebhabesh/file-sync/internal/state	0.002s
ok  	github.com/calebhabesh/file-sync/internal/testkit	0.002s
ok  	github.com/calebhabesh/file-sync/internal/workspace	0.441s
ok  	github.com/calebhabesh/file-sync/model	0.119s
ok  	github.com/calebhabesh/file-sync/tests/designgates	0.002s
ok  	github.com/calebhabesh/file-sync/tests/faults	0.551s
ok  	github.com/calebhabesh/file-sync/tests/integration	1.792s
```

```text
git diff --check
```

Passed with clean output and no whitespace errors.
