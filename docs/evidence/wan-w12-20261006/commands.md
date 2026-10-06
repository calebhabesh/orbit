# W12 validation commands

The repository already contained the W00–W11 implementation and evidence. This
packet preserves that dirty-tree provenance; no unrelated changes were reset.
The source snapshot is [`initial-source.json`](initial-source.json).

## Focused implementation checks

```text
gofmt -w cmd/filesync/terminal_network.go internal/terminal/setup_render.go internal/network/diagnostics.go internal/control/network_doctor.go internal/control/terminal_network.go internal/control/terminalcontract/network.go internal/control/support.go internal/app/app.go internal/controlclient/client.go internal/control/terminal_status.go internal/terminal/render.go internal/terminal/setup.go
go test ./internal/network ./internal/control ./internal/terminal ./internal/controlclient ./cmd/filesync -count=1 -v
```

Result: **pass**. The output is in `logs/focused-final.log`; it includes the
actual STUN response test, pinned-probe HTTP-suppression test, passive/privacy
control tests, support-export redaction test and W06/W07 compatibility tests.

```text
go test -race ./internal/network ./internal/control ./internal/terminal ./internal/controlclient ./cmd/filesync -count=1
```

Result: **pass**. `logs/owning-race-final.log` records network 95.046 s,
control 16.928 s, terminal 1.162 s and CLI 3.478 s.

## Production binary and PTY acceptance

```text
GOFLAGS=-race go test -race ./tests/terminal -run '^TestWANW12' -count=1 -v
```

Result: **pass** in 102.710 s. `logs/binary-pty-final3.log` records the real
CLI/PTY doctor journey, service-healthy/peer-offline distinction, relay quota
handling, local-only zero-service requests, reviewed apply/replay/restart and
identity/file preservation. Earlier failed reproductions remain in the same
directory (`binary-pty.log`, `binary-pty-retry.log`, `binary-pty-third.log`,
`binary-pty-final.log` and `binary-pty-final2.log`) and are not acceptance credit.

## Full package check

```text
go test ./... -count=1
```

The command is retained in `logs/all-packages-final.log`. It passed every package
through `tests/integration`, then hit the repository's documented original
aggregate terminal failure: `TestWANW08BinaryOptionalCollisionFreshRelayOnboarding`
timed out at the Go test 10-minute alarm. This is retained as a failed aggregate
run and receives no W12 acceptance credit; the focused W12 packages, owning race
run, integration package and dedicated W12 binary/PTY run pass independently.

## Review/consistency checks

```text
git diff --check
```

Result: **pass** after the W12 documentation/source edits. The initial source
snapshot and all previous packet evidence remain untouched. No privileged,
physical-WAN, personal-folder or existing-VPS fault harness was run.

## Post-handoff re-verification (2026-10-06, Claude Code session)

The Codex session stopped during a final documentation consistency sweep. That
sweep was finished (stale W09 "later packets" wording in `docs/operations.md`),
then the current tree was re-checked:

```text
gofmt -l cmd internal tests
go vet ./internal/network ./internal/control/... ./internal/terminal ./internal/controlclient ./cmd/filesync
go test ./internal/network ./internal/control/... ./internal/terminal ./internal/controlclient ./internal/config ./cmd/filesync -count=1
go test -race ./tests/terminal -run '^TestWANW12' -count=1 -timeout 15m
git diff --check
```

Result: **pass** (no gofmt/vet output; network 92.527 s; W12 binary/PTY race run
78.302 s). The full `go test ./...` aggregate was not re-run; its retained
`TestWANW08BinaryOptionalCollisionFreshRelayOnboarding` timeout stands as recorded.
