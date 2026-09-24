# P07 commands and observed results

Run from `<repo>` on 2026-09-23.

```text
make test-model
```

Passed all independent causal model cases (150 schedules + seeded runs + intentional dominance mutation check).

```text
make check
```

Passed `fmt-check`, `go vet`, internal packages, model package, integration tests (including `TestP06CLITwoPeerTransfer` and `TestP07CLIBidirectionalReconciliation`), and built both `bin/filesync` (amd64) and `bin/filesync-linux-arm64` (arm64 cross-build).

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
go test -v -count=1 ./internal/replication -run TestReconciliation
```

Observed output:
```text
=== RUN   TestReconciliationOfflineEditEdit
=== RUN   TestReconciliationOfflineEditEdit/A-then-B
=== RUN   TestReconciliationOfflineEditEdit/B-then-A
--- PASS: TestReconciliationOfflineEditEdit (0.05s)
    --- PASS: TestReconciliationOfflineEditEdit/A-then-B (0.03s)
    --- PASS: TestReconciliationOfflineEditEdit/B-then-A (0.02s)
=== RUN   TestReconciliationOfflineEditDelete
--- PASS: TestReconciliationOfflineEditDelete (0.03s)
=== RUN   TestReconciliationRepeatedDelete
--- PASS: TestReconciliationRepeatedDelete (0.02s)
=== RUN   TestReconciliationEqualByteConflict
--- PASS: TestReconciliationEqualByteConflict (0.02s)
=== RUN   TestReconciliationExecutableOnlyChange
--- PASS: TestReconciliationExecutableOnlyChange (0.02s)
=== RUN   TestReconciliationProtectedFallbackWhileRemotePending
--- PASS: TestReconciliationProtectedFallbackWhileRemotePending (0.01s)
=== RUN   TestReconciliationWorkingBasisNotAdvancedByUnreviewedHeads
--- PASS: TestReconciliationWorkingBasisNotAdvancedByUnreviewedHeads (0.02s)
=== RUN   TestReconciliationStructuralConflictPreservesChildren
--- PASS: TestReconciliationStructuralConflictPreservesChildren (0.02s)
=== RUN   TestReconciliationRestartPreservesConflictAndZeroExtraVersions
--- PASS: TestReconciliationRestartPreservesConflictAndZeroExtraVersions (0.03s)
PASS
ok  	github.com/calebhabesh/file-sync/internal/replication	0.226s
```

```text
go test -v -count=1 ./tests/integration -run TestP07CLIBidirectionalReconciliation
```

Observed output:
```text
=== RUN   TestP07CLIBidirectionalReconciliation
--- PASS: TestP07CLIBidirectionalReconciliation (0.39s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/integration	0.390s
```

```text
go test -v -count=1 ./internal/workspace -run TestScaffold
```

Observed output:
```text
=== RUN   TestScaffoldPrunedWhenChildDeleted
--- PASS: TestScaffoldPrunedWhenChildDeleted (0.01s)
=== RUN   TestScaffoldNotPrunedIfHasOtherChildren
--- PASS: TestScaffoldNotPrunedIfHasOtherChildren (0.01s)
=== RUN   TestScaffoldNotPrunedIfUntrackedFilePresent
--- PASS: TestScaffoldNotPrunedIfUntrackedFilePresent (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/internal/workspace	0.030s
```

```text
go test -v -count=1 ./internal/repository -run TestConflictsAndStructuralConflicts
```

Observed output:
```text
=== RUN   TestConflictsAndStructuralConflicts
--- PASS: TestConflictsAndStructuralConflicts (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/internal/repository	0.013s
```

```text
go test -count=1 ./...
git diff --check
```

Both passed cleanly with no warnings or errors.

```text
python -m json.tool docs/evidence/p07-20260923/manifest.json
python -m json.tool docs/evidence/p07-20260923/results.json
```

Both evidence documents parsed valid JSON.

Environment inspection:

```text
git rev-parse HEAD
go version
uname -srmo
findmnt -no FSTYPE,OPTIONS --target .
```

Observed base revision `7f0cb81281a03a943455eb2853dc702b6bd49617`, Go `go1.27.1-X:nodwarf5 linux/amd64`, Linux `7.2.6-arch2-1 x86_64`, and ext4 filesystem mounted `rw,relatime`.
