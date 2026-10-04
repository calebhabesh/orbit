# T09 commands and actual results

Working revision is a dirty implementation tree at `86ae55280b22a5839258d3cca40210c6e2613025`;
[manifest](manifest.json) records selected source hashes, complete dirty status,
versions, environment and binaries. No clean-commit reproduction or native host
execution is claimed. Commands below executed from the repository root.

## Dependency/API spike and exploratory checks

These commands executed in the session before the automated log recorder; their
stdout was observed in tool output, not copied into a fictitious historical log:

```sh
go list -m -versions charm.land/bubbletea/v2 charm.land/bubbles/v2 charm.land/lipgloss/v2
go mod download -json charm.land/bubbletea/v2@v2.0.10 charm.land/bubbles/v2@v2.2.1 charm.land/lipgloss/v2@v2.0.6
go get charm.land/bubbletea/v2@v2.0.10 charm.land/bubbles/v2@v2.2.1 charm.land/lipgloss/v2@v2.0.6
go mod tidy
go test ./internal/terminal ./cmd/filesync ./internal/controlclient
go test -count=1 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT09' -skip RealPTY
go build -o bin/filesync ./cmd/filesync
python3 scripts/terminal_pty_test.py --binary bin/filesync --output docs/evidence/terminal-t09-20261004/transcripts/run-1
python3 scripts/terminal_pty_test.py --binary bin/filesync --output docs/evidence/terminal-t09-20261004/transcripts/run-2
```

Versions were discovered from the official module proxy and verified against
[official releases/APIs](../../dependencies.md#t09-terminal-dependency-decision--2026-10-04)
and downloaded `go.mod`/license/source APIs. Initial narrow-layout and PTY fixture
failures are described in [summary](summary.md). Successful preliminary run-1
and run-2 transcripts are retained separately; run-3 is final. An intermediate
missing `tc` import was corrected before production checks. The draft name-page
branch was restricted to `folders`/`devices`, preserving aggregate status.

## Logged checks

Exact argv, UTC start, observed duration and exit code are in [commands.jsonl](commands.jsonl).
Intermediate failed runs remain failures. Successful final runs do not rewrite them.

| Run | Command | Exit | Seconds | Log |
| --- | --- | --- | --- | --- |
| discovery | `go test -list '^TestTerminalT09' ./internal/terminal ./tests/terminal` | 0 | 0.232 | [output](logs/discovery.log) |
| packet-twice | `go test -count=2 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT09'` | 1 | 50.062 | [output](logs/packet-twice.log) |
| packet-twice-final | `go test -count=2 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT09'` | 0 | 21.919 | [output](logs/packet-twice-final.log) |
| packet-twice-final-2 | `go test -count=2 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT09'` | 0 | 22.03 | [output](logs/packet-twice-final-2.log) |
| packet-race | `env GOFLAGS=-race go test -race -count=1 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT09'` | 1 | 59.044 | [output](logs/packet-race.log) |
| go-mod-verify | `go mod verify` | 0 | 0.751 | [output](logs/go-mod-verify.log) |
| make-check | `make check` | 0 | 117.793 | [output](logs/make-check.log) |
| packet-race-debug | `env GOFLAGS=-race go test -race -count=1 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT09'` | 0 | 35.527 | [output](logs/packet-race-debug.log) |
| cli-regressions | `go test -count=1 ./cmd/filesync ./internal/control ./internal/controlclient ./internal/repository` | 0 | 6.69 | [output](logs/cli-regressions.log) |
| signal-repeat | `go test -count=3 -v ./tests/terminal -run '^TestTerminalT09RealPTYLifetime$'` | 0 | 32.022 | [output](logs/signal-repeat.log) |
| make-test-race | `make test-race` | 0 | 202.749 | [output](logs/make-test-race.log) |
| packet-twice-final-3 | `go test -count=2 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT09'` | 0 | 21.942 | [output](logs/packet-twice-final-3.log) |
| markdown-diff-check | `git diff --check` | 0 | 0.014 | [output](logs/markdown-diff-check.log) |
| pty-transcripts | `python3 scripts/terminal_pty_test.py --binary bin/filesync --output docs/evidence/terminal-t09-20261004/transcripts/run-3` | 0 | 10.324 | [output](logs/pty-transcripts.log) |
| documented-pty-target | `make test-terminal-pty` | 0 | 10.308 | [output](logs/documented-pty-target.log) |
| architectures | `file bin/filesync bin/filesync-linux-arm64` | 0 | 0.012 | [output](logs/architectures.log) |
| formatting-and-diff | `make fmt-check` | 0 | 0.043 | [output](logs/formatting-and-diff.log) |
| final-diff | `git diff --check` | 0 | 0.013 | [output](logs/final-diff.log) |

## Fixture safety and cleanup

The PTY runner creates a new private canonical temporary root, writes an explicit
`.filesync-disposable` token, and copies the executable before initialization.
All signals validate the owner/marker, canonical root and state path, process
start ticks, executable and argv/state binding. Its daemon suspension is SIGSTOP/
SIGCONT on that child only; ordinary cleanup uses SIGTERM. Timeout diagnosis may
send SIGQUIT then SIGKILL only to the same validated child. The final successful
campaigns used no diagnostic kills. Root removal occurs after child cleanup and
marker validation. No personal root, VPS workload or existing service was targeted.

`GOFLAGS=-race` in the scoped packet command instruments binaries built by the
tests; the broad `make test-race` remains its documented ordinary command. Both
static architectures were built by `make check`; local amd64 executes PTYs.
No native arm64, physical network, service-login/unattended, reset, reviewed T11
editor screen or actual P17 owner-use/explanation check was executed in T09.
