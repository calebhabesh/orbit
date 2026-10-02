# O07 executed commands

Run date: 2026-10-02 UTC. Commands ran in the existing dirty development
worktree; prior Orbit work was preserved. Fixtures use `testkit.NewDisposable`
with `.filesync-disposable` markers. No personal/remote workloads were touched.

```sh
go test ./internal/repository ./internal/control -run 'TestOrbitBrowse|TestOrbitSearch|TestOrbitReadLease|TestOrbitContent' -count=1
```

Initial inherited draft failed compilation (`checkPathConflictUnlocked` missing).
After repository fixes, inherited test fixtures failed same-author stale-basis
validation; they were repaired to pass the current heads as working basis.
Search expectations now include the navigable implicit `src/config` directory.

```sh
go test -count=1 -v ./cmd/filesync ./internal/repository ./internal/control ./tests/integration -run 'TestOrbitBrowse|TestOrbitSearch|TestOrbitContent|TestOrbitReadLease|TestOrbitSettings_Control_ReadLeases'
make check
make test-race
```

Final results are in [targeted log](logs/targeted.log),
[pipeline log](logs/make-check.log), and [race log](logs/test-race.log).
The control package has an actual matching lease test in the targeted command;
its earlier command without that name reported **no tests to run**, not a pass
of a control browse suite. CLI tests are in `cmd/filesync`; HTTP tests cross
the real controller/repository through `tests/integration`.

Intermediate failures are retained in [initial targeted log](logs/targeted-initial.log),
[initial pipeline log](logs/make-check-initial.log), and
[initial race log](logs/test-race-initial.log). The initial HTTP fixture used an
uncreated root and attempted to re-mark expired history ready through a
pending-only transfer primitive; tests now register actual disposable roots
and author a separate measurement version. The first race run failed fixed
latency assertions (about 9–13 seconds under instrumentation), with no reported
data races. Timing assertions were removed in favor of recorded measurements
and bounded result/memory assertions, as required by the packet.

Schedules: a paused HTTP writer signals its first response bytes; GC runs while
that exact response remains open; request cancellation then unblocks the writer
and releases pins; a second GC reclaims the superseded payload. Repository tests
also expire the read record while retaining the live stream, then explicitly
close it. Corruption, GC-intent and restart checks affect only marked fixtures.
A real HTTP server with a 1 ms absolute write timeout verifies the content
handler switches to its per-write idle deadline.

The standalone resource measurement excluded compiler RSS:

```sh
go test -c ./internal/repository -o /tmp/filesync-orbit-o07-repository.test
python3 - <<'PY'
import subprocess, resource
result = subprocess.run([
    '/tmp/filesync-orbit-o07-repository.test',
    '-test.run', '^TestOrbitBrowse_ScalingTenThousandFiles$',
    '-test.count=1', '-test.v',
])
print('maximum_resident_set_kib:',
      resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss)
raise SystemExit(result.returncode)
PY
```

Result: exit 0, 28,944 KiB maximum RSS for the entire fixture/test process.
`/usr/bin/time` was unavailable (exit 127); the Linux `resource` measurement
above was executed instead. Exact test output is in
[resource log](logs/scaling-resource.log).
