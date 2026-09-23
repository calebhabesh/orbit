# P02 commands and observed results

Run from `<repo>` on 2026-09-21. The base revision was
`eff887b159d80d3cbec4ced301a429c71355a222`; P01 and P02 changes were
uncommitted.

```text
$ go version
go version go1.27.1-X:nodwarf5 linux/amd64

$ uname -srmo
Linux 7.2.4-arch1-2 x86_64 GNU/Linux

$ make test-model
go test -count=1 -v ./model/...
PASS
ok github.com/calebhabesh/file-sync/model

$ make check
go vet ./...
go test ./internal/... ./model/...
go test ./tests/integration/...
CGO_ENABLED=0 go build ... ./cmd/filesync
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ... ./cmd/filesync
All commands passed.

$ make test-race
go test -race ./...
All packages passed.

$ make test-faults
go test -v ./tests/designgates/...
All D1-D5 design-gate tests passed.

$ git diff --check
(no output; exit 0)
```

The model run reported these named checks: bounded exhaustive actor schedules,
deterministic generated schedules, intentional dominance mutation detection,
availability/retirement separation, and golden history/wire fixtures. Hosted
CI and native arm64 execution were not run.
