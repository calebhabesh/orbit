# Commands and actual outcomes

All commands ran from `<repo>`, unless the fixture says
otherwise. No personal state/root, service or remote workload was modified.

## Provenance and inventory

Initial `pwd`, `git status --short`, `git rev-parse HEAD`, `git diff --binary`,
`go version`, `go list -m all`, and `findmnt -T . -o FSTYPE,OPTIONS -n` were read
before new files were created. A Python standard-library collector stored the
revision, SHA-256 of the initial tracked diff and each initially changed regular
file, toolchain/dependencies, host/kernel/filesystem and the initial worktree
listing in `manifest.json` / `transcripts/initial-worktree.txt`. Full unrelated
diff contents were not published. Final task-file hashes are appended separately.

After the baseline build:

```sh
bin/orbit version --json
bin/orbit help
bin/filesync help
bin/orbit serve --help
bin/orbit setup --help
bin/orbit join --help
bin/orbit service --help
bin/orbit status --help
bin/orbit conflicts merge --help
```

Version/top-level help exited 0; subcommand flag help exited 1 with usage and
`flag: help requested`. Exact stdout/stderr are in `inventory-cli.txt`.

A Python `tempfile.TemporaryDirectory` fixture with mode 0700 and explicit
`.filesync-disposable` marker ran `bin/orbit init --state <TEMP>/state` (exit 0).
Python `sqlite3` queried `PRAGMA user_version`, `PRAGMA journal_mode`,
`PRAGMA table_info(setup_state)`, `PRAGMA table_info(enrollment_requests)` and the
enrollment table SQL from `sqlite_master`; it read the fixture's `limits.json`.
The resulting observations are in `inventory-schema.json`. The connection closed
before temporary-root cleanup. No keys/tokens or fixture identity were printed.

API inventory uses Python `pathlib` / regex over non-test
`internal/control/*.go`, extracting literal `mux.Handle` / `mux.HandleFunc`
route registrations with source file and line number (101 results). This is
source inventory, not proof that every listed route has been exercised.
Source readings also covered the default launcher starter, packaged service,
`app.Initialize` / `ServeWithOptions`, CLI dispatch/adapter helpers,
`handleMerge`, setup-state schema, join scan completion and detail-summary query.

## Baseline checks and corrected scheduling

```sh
make check > docs/evidence/terminal-t00-20261003/transcripts/make-check.txt 2>&1
make test-race > docs/evidence/terminal-t00-20261003/transcripts/make-test-race.txt 2>&1
```

These first two commands were launched concurrently. `make check` exited 0;
`make test-race` exited 2 with tar EOF / SHA256SUMS packaging failures and no
race report. They both write `dist`, so this overlap invalidates a clean
packaging-failure conclusion. After both finished, the serial command was:

```sh
make test-race > docs/evidence/terminal-t00-20261003/transcripts/make-test-race-serial.txt 2>&1
```

Exit 0. This includes CLI and integration packages. Existing tests were left
intact; no unrelated repair or oracle change was made.

## Reproduction development and final run

```sh
go test ./tests/terminal -list '^TestTerminalT00'
ORBIT_TERMINAL_BASELINE=1 go test -count=1 -v ./tests/terminal -run '^TestTerminalT00'
```

The first discovery listed nine then-existing top-level cases. The first run
(`baseline-first.txt`, exit 1) identified a missing-existing-root setup in the
pending-join fixture and a different spelling of the live lock error. The
second draft (`baseline-second.txt`, exit 1) reproduced pending-join completion
but found a restore invocation missing required `--source`. Both are preserved
as development observations, not acceptance counts.

The final harness supplies a real existing joining root and valid restore
arguments, and adds the persisted indirect-observation case:

```sh
go test ./... -list '^TestTerminalT00' > docs/evidence/terminal-t00-20261003/transcripts/test-discovery.txt 2>&1
ORBIT_TERMINAL_BASELINE=1 go test -count=2 -v ./tests/terminal -run '^TestTerminalT00' > docs/evidence/terminal-t00-20261003/transcripts/baseline-final.txt 2>&1
ORBIT_TERMINAL_BASELINE=1 go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT00' > docs/evidence/terminal-t00-20261003/transcripts/baseline-race.txt 2>&1
```

Discovery exited 0, ten top-level cases. Final behavior run exited 1 with
13 leaf failures on each of two repetitions, all for the documented gaps.
The opt-in race run exited 1 with the same 13 leaf failures and no race report.
These failures must remain reported as failures until the owning repair packets.

Each case can be selected individually, for example:

```sh
ORBIT_TERMINAL_BASELINE=1 go test -count=1 -v ./tests/terminal -run '^TestTerminalT00WrongFolderCapability$'
```

See `tests/terminal/baseline_test.go` for exact private-root setup and HTTP
payloads. The fixture calls real production handlers with generated identities,
then asserts persisted requests, file survival and scan observations. It closes
servers/DB/locks before Go cleans temporary roots. The process case runs `go
build -o <MARKED_TEMP>/orbit ./cmd/filesync` from the repository root, then:

```text
<MARKED_TEMP>/orbit serve --state=<MARKED_TEMP>/state --control-listen=127.0.0.1:0 --allow-init
```

It waits for control health, exercises actual command dispatch, validates the
disposable state path and direct child's `/proc/<pid>/cmdline`, sends SIGTERM to
that child handle, and waits. No SIGKILL, partition, firewall change, host reset
or destructive existing-directory action occurs. HTTP credentials and capability
payloads stay private; transcripts contain only sanitized assertions/statuses.

## Read-only host prerequisites

```sh
ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=5 laptop uname -m
ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=5 rpi uname -m
ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=5 vps uname -m
```

All exited 0: `x86_64`, `aarch64`, `aarch64` respectively. Existing known-host
validation stayed enabled. No native application/service test was executed.

Each alias was then queried with the same SSH options and `tailscale status
--json`; all exited 127. The collector publishes only exit/backend-state
observations in `native-prerequisites.json`, omitting node names and addresses.
Local `command -v tailscale` exited 1 (not in tested PATH). No Tailscale install
or host/network change was attempted.

## Harness validation

```sh
gofmt -w tests/terminal/baseline_test.go
go vet ./tests/terminal
go test ./cmd/filesync/...
go test -count=1 -v ./tests/terminal
git diff --check
```

Each exited 0. Normal terminal tests skip all ten opt-in cases; this establishes
that baseline failures are excluded from ordinary gates, not repaired behavior.
The opt-in race run exercises the new harness independently of those skips.
Local evidence/status/README links and final provenance hashes were checked
with Python's standard library. Logs from commands with no output are retained
as empty files, with actual exit results in `results.json`.

Whole-file merge RSS, fatal initial-scan error, tree-swap execution, PTY/TUI,
cross-host enrollment/transfer, LAN/Tailscale reachability, boot/logout,
power-loss and actual owner pilot/explanation remain unexecuted.
