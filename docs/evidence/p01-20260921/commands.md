# P01 commands and actual results

Run from `<repo>` on 2026-09-21.

## Design-gate suite on ext4

```sh
mkdir -p <repo>/.tmp/p01
TMPDIR=<repo>/.tmp/p01 \
  go test -count=1 -v ./tests/designgates
rmdir <repo>/.tmp/p01 \
  <repo>/.tmp
```

Result: PASS, 0.002 seconds reported package time. All D1–D5 tests and
subtests passed. D1 reported `ELOOP` for the swapped symlink. The D3 semantic
digests and D4 schedules match `results.json`. Go's test cleanup removed the
marked disposable children before the two empty parent directories were
removed.

```sh
findmnt -T . -o TARGET,SOURCE,FSTYPE,OPTIONS -n
```

Result: `/ /dev/nvme1n1p2 ext4 rw,relatime`.

## Full repository checks

```sh
make check
make test-race
make test-faults
```

Results: all three commands passed. `make check` ran formatting validation,
`go vet ./...`, internal and integration tests, and amd64/arm64 builds.
`make test-race` passed every package including `tests/designgates`.
`make test-faults` reran every D1–D5 experiment successfully.
