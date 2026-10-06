# W00 commands — 2026-10-05

Working directory: repository root. Commands ran on the live initial clean tree
whose revision and full tracked-file SHA-256 manifest are in `manifest.json`.
The provenance capture preceded source edits. No snapshot was used. All actual
process/network fixtures use fresh `.filesync-disposable` roots; their transient
keys/capabilities are not retained. Logs contain synthetic assertions only.

## Inventory and provenance

```sh
pwd
git status --short
rg --files -g AGENTS.md -g '*wan*' -g CONTEXT.md
cat AGENTS.md docs/orbit-wan-implementation-plan.md docs/implementation/wan-status.md docs/implementation/wan-foundations.md
cat Makefile
```

Required scope/glossary, WAN UX/architecture/protocol/gates and owning
protocol/persistence/operations/verification documents were inspected alongside
current terminal/P status and T13 follow-up evidence. `rg` source inspection
covered app factory/listeners/shutdown, identity/client/enrollment, runtime/peer
config, schema version, CLI, testkit and host worker safety. No remote inspection
or namespace/firewall/service policy changes were performed.

The capture command used Python `subprocess` with structured argument arrays for
`git rev-parse HEAD`, `git status --porcelain=v1 --untracked-files=all`,
`git ls-files -z`, `go version`, and `findmnt -T <repository> -n -o FSTYPE,OPTIONS`.
It hashed all tracked files' actual bytes and recorded platform/toolchain/network
and filesystem assumptions. The initial status was empty; new W00 evidence was
written after capture. Final task-file hashes are recorded separately without a
self-referential manifest hash.

## Runtime checks

```sh
make check > docs/evidence/wan-w00-20261005/logs/make-check.log 2>&1
go test -count=1 -v ./tests/terminal -run '^TestWANW00' > docs/evidence/wan-w00-20261005/logs/wan-w00.log 2>&1
go test -list '^(TestWANW00|TestTerminalT04|TestTerminalT05|Test)' ./cmd/filesync/... ./internal/control/... ./internal/replication/... ./tests/terminal/... > docs/evidence/wan-w00-20261005/logs/discovery.log 2>&1
go test -count=1 ./cmd/filesync/... ./internal/control/... ./internal/replication/... > docs/evidence/wan-w00-20261005/logs/focused-uncached.log 2>&1
go test -count=1 -v ./tests/terminal -run '^(TestTerminalT04.*CLI|TestTerminalT05ThreeProcessSharingOfflineRolloutAndEndpointRefresh)$' > docs/evidence/wan-w00-20261005/logs/manual-process.log 2>&1
go test -count=1 -v ./tests/terminal -run '^TestTerminalT04TwoDeviceCLIInterruptedJoinAndEdits$' > docs/evidence/wan-w00-20261005/logs/manual-cli.log 2>&1
go test -race -count=1 -v ./tests/terminal -run '^TestWANW00' > docs/evidence/wan-w00-20261005/logs/wan-w00-race.log 2>&1
```

The initial process selector matched only T05: the T04 name continues after
`CLI`. The explicit second command ran the discovered exact T04 test; no zero-
match coverage is claimed. Each runtime command's exit status/results are in
`results.json` and its retained log. Independent checks ran concurrently;
production process fixtures have distinct private roots and assigned ports.

`make check` started before the new W00 fixture was added; its initial terminal
compilation covered existing fixtures. The new test was compiled/executed by the
two explicit W00 commands. Ordinary check targets may use Go's test cache; the
focused commands above deliberately use `-count=1`. The Makefile includes CLI
via `test`; it excludes full race, demo, native hosts, privileged/VM campaigns and
container package transactions. Those exclusions are not reported as executions.

## Final documentation/provenance checks

```sh
python3 docs/evidence/wan-w00-20261005/check_docs.py
git diff --check
```

The retained validator checks local links/anchors for W00 documents, strict
packet dependency ordering, final manifest preservation of every original
tracked file outside the declared task edits, and nonzero selected test names
in discovery. It does not claim a new wire-schema fixture: W00 changes none.
