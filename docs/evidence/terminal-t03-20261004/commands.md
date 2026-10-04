# T03 commands and results

Executed in `<repo>`, revision
`86ae55280b22a5839258d3cca40210c6e2613025` plus the preserved dirty tree.
Go 1.27.1-X:nodwarf5, Linux 7.2.8-arch1-2 amd64. The task used fresh marked
test roots and numeric ports bound to the development host's nonloopback IPv4.
No personal roots, SSH hosts, existing services, firewall or VPN settings were
changed. Process helpers validate the disposable marker, signal exact children
or use the owning pidfd-based stop, and wait for graceful exit.

## Discovery and final packet execution

```sh
go test ./... -list '^TestTerminalT03'
go test -count=2 -v ./... -run '^TestTerminalT03'
go test -race -count=1 -v ./internal/protocol ./internal/replication ./tests/terminal -run '^TestTerminalT03'
```

Discovery: 16 top-level definitions, including one subprocess-only helper. The
15 ordinary tests passed twice (30 executions); the helper is skipped in parent
runs and actually executes in child processes. Nested cases cover signed-field
mutations, status forgery/replay, invalidated pending requests and seeded caps.
See `transcripts/discovery.txt`, `ordinary-final.txt`, `focused-race.txt`.
The focused race run passed before the final SQL pagination/count and exact
wire-retry tightening; the final full race gate below validates the final code.

## Integration and broad checks

```sh
make build
go test -count=1 -v ./tests/integration -run '^TestOrbitPairing_'
go test -count=2 -v ./tests/integration -run '^TestOrbitPairing_CLI_RunningDaemon'
go test -count=1 ./internal/control ./internal/repository ./cmd/filesync ./tests/integration -run 'TestOrbit|TestTerminalT02'
make check
make test-race
git diff --check
```

The pairing group passed all five tests after migration of the automatic network
fixtures; the private-file CLI journey passed twice. The intermediate scoped
regression failed on newly authenticated legacy HTTP fixtures, a campaign fixture
that unintentionally inherited secure networking while retaining synthetic pins,
and the obsolete v1 CLI invitation. These were migrated while retaining their
original file/membership/endpoint/alias assertions. The final broad gates include
these regressions, explicit CLI tests in the race gate, model/fault checks and
amd64/arm64 package builds. Gates run serially because they write shared `dist`.
Initial broad checks passed, then were rerun after final bounds/replay tightening;
see `make-check-initial-pass.txt` and `make-test-race-initial-pass.txt` separately
from the final `make-check.txt` and `make-test-race.txt`.

## Development failures and corrections

- Compile iterations caught wrong existing names (`HelloFolder`,
  `RequireDisposable`), the initially absent membership decoder, one incorrect
  Validate assignment and a misplaced pagination guard. Corrected before the
  final executed test groups.
- `go test -count=1 -v ./tests/terminal -run '^TestTerminalT03'` initially passed
  the network negatives but failed the process fixture: its query could select
  the stopped adapter before daemon startup. The helper now waits for successful
  authenticated live HTTP. Two listener ports remain reserved together before
  release to avoid selecting the same port. `process-retry.txt` retains the
  intermediate failure; `process-diagnosis.txt` and ten repetitions in
  `process-stability.txt` passed. Final `process-restart.txt` passed twice after
  adding actual requester-process signing and graceful owner restart.
- The first CLI migration failed because a private invitation was placed in a
  Go temporary directory with public directory mode. The fixture now makes its
  marked root 0700. `cli-diagnosis.txt` records the refusal and `cli-final.txt`
  records two passing journeys. No capability is printed in that transcript.
- The broad initial scoped regression failure is preserved in
  `regression-initial.txt`; the further obsolete CLI failure is in
  `regression-final.txt`. These are failures, not final acceptance passes.

## Remaining checks

Unexecuted: physical two-host LAN, Tailscale, native laptop/Pi/VPS enrollment,
boot/logout/login, SIGKILL/power loss, TUI/PTY and actual owner-use/explanation.
T04 retains complete root review, setup/join replay and honest scan/content
readiness. T05 retains same-device second-folder/offline/third-peer rollout.
No simulated owner activity is counted as P17 evidence.

Final documentation validation: 10 Markdown documents and 75 local links/anchors
passed; fences and trailing whitespace passed. `git diff --check` exited 0.
The final scoped `ps`/`rg` cleanup audit returned no matching disposable daemon
or T03 helper; `rg` exit 1 denotes no matches, not a test failure.
