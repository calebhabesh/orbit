# T11 actual commands and results

Run from `<repo>`, 2026-10-04, on the existing tree. No commit/reset/stash or delegation.
Commands below are actual executions; logs retain output.

## Discovery and final packet checks

```sh
go test ./internal/terminal ./tests/terminal -list '^TestTerminalT11'
go test -count=2 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT11'
GOFLAGS=-race go test -race -count=1 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT11'
GOFLAGS=-race go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT11RealPTYEveryday$'
```

[Discovery](logs/discovery.log): seven ordinary top-level tests (four view/adapter, three production/process).
[Final repeated run](logs/packet-final.log): all seven passed twice without skips.
[Packet race](logs/packet-race-final.log): all seven passed without race warnings under `GOFLAGS=-race`, which instruments the built CLI and daemon child processes.
[PTY race confirmation](logs/pty-race-confirmation.log): passed with all seven scenarios verified under real PTY execution.

## Relevant regression and broad checks

```sh
go test -count=1 -v ./cmd/filesync/... ./tests/terminal -run '^TestTerminalT09|^TestTerminalT10|^TestTerminalT08'
go test -count=1 ./cmd/filesync/...
python3 -m unittest discover -s scripts/validation -p 'test_terminal_vt.py'
python3 -m py_compile scripts/terminal_everyday_pty_test.py
make check
make test-race
git diff --check
```

[Relevant regressions](logs/regressions.log) passed, including T08 reviewed reads/atomic replay/content recovery, T09 PTY lifetime, and T10 onboarding assertions.
[Explicit CLI tests](logs/cli-final.log) passed.
[VT tests](logs/vt-final.log): five passed.
Python compilation and git diff check passed with exit 0.
[Final make check](logs/make-check-final.log) passed with all package tests, vetting, model/fault checks, and multi-architecture binary builds.
Full `make test-race` passed across all packages in the repository, retained in [its log](logs/make-test-race.log).

## Sanitized actual-binary PTY artifacts

```sh
python3 scripts/terminal_everyday_pty_test.py --binary bin/filesync \
  --output docs/evidence/terminal-t11-20261004/transcripts
```

[Transcript log](logs/pty-transcripts.log): passed, producing transcripts in `transcripts/`:
- `everyday-offline-conflict-editor.txt`
- `narrow-storage-root-recovery.txt`
- `paired-peer.txt`
