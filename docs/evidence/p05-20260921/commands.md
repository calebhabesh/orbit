# P05 commands and observed results

Run from `<repo>` on 2026-09-21.

```text
make test-model
```

Passed all independent-model cases, including exhaustive bounded schedules,
fixed seeds and the intentional comparison mutant.

```text
make check
```

Passed formatting, `go vet`, internal/model tests, integration tests, amd64
build and arm64 cross-build. The P05 CLI integration exported a public
certificate and replayed the same explicit pair approval idempotently.

```text
make test-race
```

Passed all packages. The real TLS replication suite completed under the race
detector in 12.532 seconds.

```text
make test-faults
```

Passed all P01 design-gate and P03/P04 process-fault regressions.

```text
go test -count=1 ./...
git diff --check
```

Both passed after the final implementation and specification changes.

```text
python -m json.tool docs/evidence/p05-20260921/manifest.json
python -m json.tool docs/evidence/p05-20260921/results.json
```

Both evidence documents parsed successfully.

Environment inspection:

```text
git rev-parse HEAD
go version
uname -srmo
findmnt -no FSTYPE,OPTIONS --target .
```

Observed base revision `eff887b159d80d3cbec4ced301a429c71355a222`, Go
`go1.27.1-X:nodwarf5 linux/amd64`, Linux `7.2.4-arch1-2` x86_64, and ext4
mounted `rw,relatime`. P01-P05 remained uncommitted during the run.
