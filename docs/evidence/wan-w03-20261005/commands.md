# W03 commands and actual results

Working directory: `<repo>`. All sockets/state/processes are local
synthetic fixtures; process roots are marked disposable. Final Go source and
initial dirty-tree hashes are recorded in [manifest](manifest.json).

| Command | Result / transcript |
| --- | --- |
| `go test -list '^TestWANW03' ./internal/... ./tests/... ./cmd/filesync/... ./cmd/orbit-net` | 24 nonzero matches in network/rendezvous/replication; [final discovery](logs/final-discovery-complete.log) |
| `go test -race -count=1 -timeout=90s -v ./internal/network ./internal/rendezvous ./internal/replication -run '^TestWANW03'` | All 24 pass on final frozen source; [final race](logs/header-final-race.log) |
| `go test -race -count=20 -timeout=60s -v ./internal/rendezvous -run '^TestWANW03ServiceBinaryRestart$'` | 20 passes, actual separate service process each run; [repeat](logs/binary-restart-repeat.log) |
| `go test -count=1 ./cmd/filesync/... ./cmd/orbit-net ./internal/config ./internal/control/... ./internal/network ./internal/replication ./internal/scheduler ./model` | Passed, uncached; [compatibility](logs/final-compatibility.log) |
| `go test -count=1 -v ./internal/protocol ./model` | Passed, unchanged independent golden/strict codec and model cases; [contracts](logs/strict-contracts-model.log) |
| `go test -count=1 -v ./tests/terminal ./tests/integration -run '^(TestTerminalT04TwoDeviceCLIInterruptedJoinAndEdits|TestTerminalT05ThreeProcessSharingOfflineRolloutAndEndpointRefresh|TestBackgroundSyncFromPersistedPeerEndpoints)$'` | Passed all three real manual CLI/forwarding/background journeys; [production](logs/manual-production.log) |
| `make check` | Final frozen-source run passed; ordinary Go cache where labeled; [final integrated check](logs/header-final-make-check.log) |
| `CGO_ENABLED=0 go build -trimpath -o /tmp/orbit-net-w03-amd64 ./cmd/orbit-net` | Passed, static native service binary |
| `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o /tmp/orbit-net-w03-arm64 ./cmd/orbit-net` | Passed, static arm64 cross-build; native arm64 execution unexecuted |
| `file /tmp/orbit-net-w03-amd64 /tmp/orbit-net-w03-arm64` | Expected architectures/static ELF; [builds](logs/service-builds.log) |
| `/tmp/orbit-net-w03-amd64 --help` | Passed; explicit operator configuration flags, no fabricated defaults; [help](logs/service-help.log) |
| `python3 docs/evidence/wan-w03-20261005/check_docs.py` | Passed local links/anchors, all discovered execution, frozen source and initial-file preservation; [validation](logs/final-docs-check.log) |
| `go test -race -count=1 -timeout=10s -v ./internal/rendezvous -run '^TestWANW03TLSAndHeaderOrigins$'` | Passed final valid-challenge duplicate-Origin regression; [header regression](logs/header-specific-final-race.log) |
| `git diff --check` | Passed; [whitespace](logs/diff-check.log) |

Earlier execution records remain in `logs/`: initial compile/focused/race runs,
uncached candidate compatibility and discovery, directory-loss compile correction,
WSS admission/binary smoke, and the first `make check`. The initial focused cache
fixture failed because it seeded 1024 entries on top of prior operations; its
repair retained service limits. `focused-2.log` retained the socket-close deadlock
and exact disposable test-process SIGQUIT diagnostics; production ownership/close
ordering was repaired and all final lifecycle checks pass. `directory-loss-1.log`
contains the incorrect test field compile failure; `directory-loss-2.log` passes.
`make-check.log` contains the binary fixture's stale fake-clock failure;
`final-make-check.log` and twenty real-clock restarts pass. Pre-correction final-named
candidate logs are historical only; authoritative production acceptance logs are
`header-final-race.log`, `header-specific-final-race.log`, `final-compatibility.log`
and `header-final-make-check.log` above. The final review tightened all-value Origin
checks and refused any Forwarded header; the strengthened valid-challenge header
fixture then passed separately. Production code was frozen before these final runs;
only that test fixture was refined after the full race run started.
