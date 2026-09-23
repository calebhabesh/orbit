# P06 commands and observed results

Run from `<repo>` on 2026-09-23.

```text
make test-model
```

Passed all independent causal model cases (150 schedules + seeded runs).

```text
make check
```

Passed `fmt-check`, `go vet`, internal packages, model package, integration tests (including `TestP06CLITwoPeerTransfer`), and built both `bin/filesync` (amd64) and `bin/filesync-linux-arm64` (arm64 cross-build).

```text
make test-race
```

Passed all packages with the Go race detector enabled. The full suite completed cleanly in ~17 seconds with zero race warnings.

```text
make test-faults
```

Passed all design gates (D1-D5) and all process-kill fault boundaries:
- P03 object, version, and readiness commit boundaries;
- P04 existing-file exchange, new-file creation, parent/directory creation, and tombstone removal;
- P06 transfer boundaries: `transfer.chunk.verified`, `transfer.readiness.before`, `transfer.readiness.after`, `transfer.receipt.before_send`, and `transfer.receipt.after_send`. Every restart assertion succeeded and resumed transfers cleanly reused verified chunks.

```text
go test -count=1 ./...
git diff --check
```

Both passed cleanly with no warnings or errors.

```text
python -m json.tool docs/evidence/p06-20260923/manifest.json
python -m json.tool docs/evidence/p06-20260923/results.json
```

Both evidence documents parsed valid JSON.

Environment inspection:

```text
git rev-parse HEAD
go version
uname -srmo
findmnt -no FSTYPE,OPTIONS --target .
```

Observed base revision `eff887b159d80d3cbec4ced301a429c71355a222`, Go `go1.27.1-X:nodwarf5 linux/amd64`, Linux `7.2.6-arch2-1 x86_64`, and ext4 filesystem mounted `rw,relatime`.
