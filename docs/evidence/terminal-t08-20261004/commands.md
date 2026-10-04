# Commands and results

All test roots come from `testkit.NewDisposable` and contain `.filesync-disposable`.
Destructive object corruption, tool-crash fixture and child daemon stop validate
marked canonical paths. Tests use only child processes and synthetic fixture bytes.
No personal roots, existing VPS workload, firewall, VPN or host service policy was changed.

Final commands (run serially; exact argv/exits/timings in `validation.json`):

```sh
go test -list ^TestTerminalT08 ./tests/terminal
```

Exit 0; 0.529s; [discovery.log](discovery.log).

```sh
go test -count=2 -v ./tests/terminal -run ^TestTerminalT08
```

Exit 0; 5.933s; [packet.log](packet.log).

```sh
go test -race -count=1 -v ./tests/terminal -run ^TestTerminalT08
```

Exit 0; 15.195s; [packet-race.log](packet-race.log).

```sh
go test -count=1 ./cmd/filesync/... ./internal/control/... ./internal/controlclient/... ./internal/workspace/... ./internal/repository/... ./model/...
```

Exit 0; 2.165s; [targeted.log](targeted.log).

```sh
make check
```

Exit 0; 76.951s; [make-check.log](make-check.log).

```sh
make test-race
```

Exit 0; 192.508s; [make-test-race.log](make-test-race.log).

```sh
git diff --check
```

Exit 0; 0.013s; [diff-check.log](diff-check.log).

The live CLI test builds a fresh `orbit` in its marked root, initializes/captures
synthetic data, seeds approved peer histories, closes fixture ownership, starts
`serve --state <private-state> --control-listen 127.0.0.1:0 --no-watch
--sync-interval 1s`, waits for authenticated capabilities, and executes actual
select/keep-copies/restore/export/editor/merge commands. It gracefully stops only
that child. Other production-control fixtures retain exclusive marked state and
use the authenticated HTTP server or supported stopped adapter.

Intermediate commands used the same `go test -count=1 -v ./tests/terminal
-run '^TestTerminalT08'` group as scenarios grew, plus scoped compilation/regression
runs. Retained logs record initial fixture/build failures, a new-object pin foreign-key
failure, and incorrect expectations around GC/stream protection. A repeated final
run caught stopped read-error return before state ownership release; the failure
is retained in `packet-adapter-intermediate.log`. Read success/error/cancellation
now wait for stopped ownership teardown. Early T06/T07 regression checks passed
before final full repository validation. Capability-scope and unused-import
compilation errors were fixed before the final discovered checks; they are not
passing evidence. Earlier broad passing logs and their results are retained as
`before-final-*` and `validation-before-final.json`, rather than replacing final
checks. Final replay review added completed-replay and pending-publication stale
head regressions; their first passing run is `replay-regression.log`. All final
checks were rerun afterward, preserving earlier results as `before-replay-*` and
`validation-before-replay.json`. Synthetic RSS values include fork/exec high-water effects.

Test cleanup removes only generated temporary roots through the Go harness;
explicit session discard tests validate their private marked target and remove
only known result/export/upload names. No manual destructive shell cleanup ran.
