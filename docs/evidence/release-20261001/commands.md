# Commands and observed outcomes

All commands ran from the repository unless a working directory is named.
Source was based on commit `51b33be` plus the reviewed task changes; source and
binary hashes distinguish tested snapshots from that base revision. No private
keys, control credentials or file contents from personal folders are published.

```sh
go test -count=1 ./internal/... ./model/... ./tests/integration/... ./tests/designgates/... ./tests/faults/...
# PASS before changes; relevant suites and full checks ran again afterwards.
go test -count=1 ./internal/replication -run '^TestSyncInventoryLargerThanMemoryQueue$'
# FAIL: total inventory >1024. First repair then exposed explicit rate-limit failure.
go test -count=1 ./internal/repository ./internal/replication ./internal/workspace ./model/...
# PASS after spool/retry/path-history corrections.
go test -count=30 ./internal/scheduler -run '^TestSchedulerRootUnavailablePausesFolder$'
# PASS after waiting for the submitted task's durable outcome.
make check
# First run failed a scheduler test timing race; corrected run passed.
make test-race
# PASS.
make demo
# PASS, disposable loopback peers.
python3 -m unittest discover -s scripts/validation -p 'test_*.py'
# PASS, 3 safety tests.
go mod verify
# all modules verified.
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
# No vulnerabilities found.
```

From `web/`: `npm ci`, `npm run build`, `npm audit --json` passed with zero
findings. Frontend output remained identical.

```sh
python3 scripts/validation/abrupt_reset.py --kernel /boot/vmlinuz-linux --output docs/evidence/release-20261001/reset-final
# PASS 16 cases. Kernel and guest-init hashes plus QEMU version are recorded.
python3 scripts/validation/three_host.py --laptop local --pi rpi --vps vps --output docs/evidence/release-20261001/workstation-demo-final
# PASS 64.7 seconds on workstation/Pi/VPS; laptop unexecuted.
python3 scripts/validation/service_lifecycle.py --output docs/evidence/release-20261001/lifecycle
# PASS all three native hosts, unique user units, state/root preservation.
python3 scripts/validation/benchmark.py --output docs/evidence/release-20261001/benchmark-smoke --small-files 20 --large-mib 40 --repetitions 1
# PASS nine workloads; raw byte counters, not estimates.
python3 scripts/validation/benchmark.py --output docs/evidence/release-20261001/benchmark-default-before --small-files 1000 --large-mib 40 --repetitions 1
# FAIL inventory admission. Preserved as negative evidence.
python3 scripts/validation/benchmark.py --output docs/evidence/release-20261001/benchmark-full --small-files 10000 --large-mib 1024 --repetitions 1
# Pre-scaling run cancelled only its validated fixture scan after slow progress;
# preserved as benchmark-before-scaling. Corrected full campaign still running.
go test -run '^$' -bench '^BenchmarkCaptureDistinctPaths$' -benchtime=1x -cpuprofile=/tmp/filesync-history-before.pprof ./internal/repository
go tool pprof -top /tmp/filesync-history-before.pprof
# 14.402 seconds; profile identified repeated folder-wide history/SQL work.
go test -run '^$' -bench '^BenchmarkCaptureDistinctPaths$' -benchtime=1x -cpuprofile=/tmp/filesync-history-after.pprof ./internal/repository
go tool pprof -top /tmp/filesync-history-after.pprof
# 0.104 seconds, one sample. No generic throughput claim.
```

Two `ssh -o BatchMode=yes -o ConnectTimeout=8 laptop 'uname -m -sr'`
checks returned `No route to host` for the alias's address 192.168.88.83.
Read-only Pi/VPS inventory succeeded; their real OS/architecture/filesystem/
route output is in the reports.

Fault schedule: the VM helper reaches a named production hook, writes an
unflushed negative-control overwrite, emits readiness and waits. The host
validates its disposable image/marker, kills only its child QEMU instance,
then boots a new guest on the same image to verify protected hashes and run
publication recovery. There is no guest shutdown or unmount before the fault.
An unused `publication.flushed` alias was rejected by the first harness attempt;
`publication.directory.flushed` is the actual included flush boundary.

Remote interruption schedule: after independent file creation/capture, the
receiver begins a throttled 12-MiB transfer. The harness observes durable
`transfer_chunks.verified` growth, validates exact process identity, kills only
that sync process, restarts synchronization against the same sender, asserts
remaining fetch count and verified reuse, then compares full SHA-256 values.

Reproduction and cleanup: build both binaries first; wrappers create a new
output directory per run. Remote actions require marker/root checks and track
exact worker processes. Existing pilot directories are never reset. Unique
systemd units are unlinked after ownership checks; roots/state are retained.
Inspect a root's marker and stop its owning agent before removing a synthetic
fixture. Never apply these fault scripts to personal data or existing workloads.
