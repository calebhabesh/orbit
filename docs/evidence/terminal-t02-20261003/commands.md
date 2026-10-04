# Commands and actual results

Executed from `<repo>` on the preserved dirty tree rooted at
`86ae55280b22a5839258d3cca40210c6e2613025`. Go 1.27.1-X:nodwarf5, Linux
7.2.8-arch1-2 amd64. No concurrent packaging gates were run.

| Command | Actual result / transcript |
| --- | --- |
| `go test ./... -list '^TestTerminalT02'` | 16 top-level definitions: 15 ordinary tests plus subprocess helper; `discovery-complete.txt` |
| `go test -count=2 -v ./cmd/filesync ./tests/terminal -run '^TestTerminalT02'` | exit 0, 30 ordinary top-level executions passed; helper skipped in parent and executed in child processes; `acceptance-targeted.txt` |
| `go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT02'` | first exit 1 at child shutdown; `race-targeted.txt`; incomplete interrupted later run `race-targeted-final.txt`; resumed exit 0 in `race-targeted-resumed.txt` |
| `go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT02ControlServeShutdown$'` | exit 1, actual data race; `race-minimal-before.txt` |
| `go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT02(ControlServeShutdown|ConcurrentProcessLaunchAndLifetime)$'` | exit 0 after server initialization fix; `race-minimal-after.txt`  |
| `make check` | first exit 2 on private-state and real service environment fixtures; `make-check.txt`; corrected intermediate exit 0 `make-check-final.txt`; final exit 0 `acceptance-check.txt` |
| `make test-race` | final gate result recorded in `results.json`, transcript `acceptance-race.txt` |
| `git diff --check` | exit 0 after source and documentation changes; repeated final result recorded in `results.json` |

Initial compile/test iterations exposed an unsupported ID method, unused imports
and the stricter state fixture requirements; these errors were corrected before
the final named checks. Intermediate packet transcripts `targeted.txt`,
`targeted-final.txt` and discovery files predate the final suite and are retained
as work records, not inflated into extra acceptance repetitions.

Unexecuted: native login/logout/boot, native Pi/VPS package execution, enrollment
TLS/HTTP serving (T03), Tailscale reachability, SIGKILL/fsync boundaries, VM reset,
full CLI/TUI family parity, explicit operation cancellation, streaming reads/
uploads, native legacy adoption/rollback and owner use/explanation. Test data
and stand-in service actions used marked disposable roots; exact process IDs
limited stop actions. No unrelated personal workload was fault-tested.


Final dispatch/capability integration checks are `acceptance-targeted.txt`,
`acceptance-check.txt` and `acceptance-race.txt`. Earlier complete/final-named
transcripts are retained as intermediate snapshots. `targeted-dispatch-final.txt`
exited 1 because callers saw the expected not-yet-ready live endpoint during
concurrent ownership handoff; the corrected regression verifies safe errors,
replay after readiness and exactly one external command. No unsafe fallback
assertion was relaxed. `targeted-dispatch-complete.txt` then exited 0; the final
acceptance suite additionally covers typed capability negotiation.


The final harness-only cleanup repair was validated with
`go test -race -count=1 -v ./tests/integration -run
'^TestOrbitPairing_CLI_RunningDaemon_Parity$'`: exit 0,
`cleanup-regression.txt`. `process-cleanup.json` records graceful teardown of
seven exact known disposable daemons leaked by the earlier alias cleanup bug.
The corrected fixture validates its disposable marker before stopping, and no
session test daemon remained after verification. Broad runtime gates passed
before this fixture-only repair; no later runtime code change occurred.

A draft local-link check failed because `acceptance-race.txt` had not yet been
created. Once that actual gate ran, validation passed all seven owning/evidence
documents and their local paths; `docs-validation.txt` records the final count.
