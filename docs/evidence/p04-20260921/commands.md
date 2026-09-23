# P04 commands and observed results

Run from `<repo>` on 2026-09-21. Base revision:
`eff887b159d80d3cbec4ced301a429c71355a222`; P01–P04 changes were
uncommitted.

```text
$ go version
go version go1.27.1-X:nodwarf5 linux/amd64

$ uname -srmo
Linux 7.2.4-arch1-2 x86_64 GNU/Linux

$ findmnt -no SOURCE,FSTYPE,OPTIONS --target .
/dev/nvme1n1p2 ext4 rw,relatime

$ make test-model
PASS; all model tests passed, including bounded exhaustive schedules and the
intentional comparison mutant check.

$ make check
go vet, internal/model tests, integration tests, amd64 build and arm64
cross-build passed.

$ make test-race
All packages passed under the Go race detector.

$ make test-faults
All D1–D5 gate tests, seven P03 boundaries, and 30 P04 SIGKILL/restart
boundaries passed.

$ go test -count=1 ./...
All packages passed without cached test results.

$ git diff --check
(no output; exit 0)
```

P04's existing-file hooks were `publication.prepared`,
`publication.stage.flushed`, `publication.staged`, `publication.intent`,
`publication.exchange.before`, `publication.filesystem.transition`,
`publication.recovery.named`, `publication.directory.flushed`,
`publication.renamed`, and `publication.committed`. New-file publication used
the same sequence except recovery naming. Directory and tombstone cases used
the five relevant prepared/intent/transition/renamed/committed points each.
The separate parent-scaffold case killed after `mkdirat` and before flush.
