# P03 commands and observed results

Run from `<repo>` on 2026-09-21. Base revision:
`eff887b159d80d3cbec4ced301a429c71355a222`; P01–P03 changes were uncommitted.

```text
$ go version
go version go1.27.1-X:nodwarf5 linux/amd64

$ uname -srmo
Linux 7.2.4-arch1-2 x86_64 GNU/Linux

$ findmnt -no SOURCE,FSTYPE,OPTIONS --target .
/dev/nvme1n1p2 ext4 rw,relatime

$ make test-model
PASS
ok github.com/calebhabesh/file-sync/model

$ make check
go vet, internal/model tests, integration tests, amd64 build and arm64
cross-build all passed.

$ make test-race
All packages passed under the Go race detector.

$ make test-faults
All D1–D5 design-gate tests passed. All seven P03 SIGKILL/restart boundary
cases passed.

$ go test -count=1 ./...
All packages passed without cached test results.

$ git diff --check
(no output; exit 0)
```

The P03 boundary cases were `object.flushed`, `object.installed`, `object.recorded`,
`sql.version.before_commit`, `sql.version.after_commit`,
`sql.readiness.before_commit`, and `sql.readiness.after_commit`.
