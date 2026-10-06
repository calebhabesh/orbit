# W04 commands and actual results

Working directory: `<repo>`. Development service sockets use
normal TLS on a private nonloopback interface. Engine/process data is synthetic
and disposable; no personal folder or VPS workload is a fault target. Initial
revision/dirty-tree and final source hashes are in [manifest](manifest.json).

| Command | Result / transcript |
| --- | --- |
| `go test -list '^TestWANW04' ./internal/network ./internal/rendezvous ./internal/replication ./internal/scheduler` | Nonzero discovery; [complete discovery](logs/complete-discovery.log) |
| `go test -race ./internal/network ./internal/rendezvous ./internal/replication ./internal/scheduler -run '^TestWANW04' -count=1 -v -timeout=120s` | [Complete focused race](logs/complete-race.log); final result in results.json |
| `go test -race ./internal/replication -run '^TestWANW04ProductionRelayRestartResumesVerifiedChunks$' -count=20 -timeout=120s` | Passed 20 real HTTP-service epoch/reannouncement/resume repetitions; [repeat](logs/restart-20-final.log) |
| `go test -count=1 ./cmd/filesync/... ./internal/control/... ./internal/config/... ./internal/network/... ./internal/rendezvous/... ./internal/replication/... ./internal/scheduler/... ./model/...` | Passed uncached; [compatibility](logs/compatibility-final.log) |
| `go test -count=1 ./tests/terminal -run '^(TestTerminalT04TwoDeviceCLIInterruptedJoinAndEdits|TestTerminalT05ThreeProcessSharingOfflineRolloutAndEndpointRefresh|TestDaemonBackgroundSyncWhileCaptureContinues)$'` | The two existing T04/T05 tests passed uncached; [manual CLI/forwarding](logs/manual-journeys.log). The final pattern's third name does not exist and supplies no coverage |
| `go test -count=1 -v ./tests/integration -run '^TestBackgroundSyncFromPersistedPeerEndpoints$'` | Passed the separately discovered actual background-sync test; 16-MiB archive plus eight ordinary edits; [background](logs/manual-background.log) |
| `make check` | [Integrated check](logs/make-check.log); final result in results.json. Ordinary cache where Go labels it; focused acceptance is uncached |
| `CGO_ENABLED=0 go build -trimpath -o /tmp/orbit-net-w04-amd64 ./cmd/orbit-net` | Passed static native service build |
| `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o /tmp/orbit-net-w04-arm64 ./cmd/orbit-net` | Passed static arm64 cross-build; native execution unexecuted |
| `file /tmp/orbit-net-w04-amd64 /tmp/orbit-net-w04-arm64` | [Architecture/static binary check](logs/service-builds.log) |
| `/tmp/orbit-net-w04-amd64 --help` | [Operator flags](logs/service-help.log); no invented hosted endpoints |
| `python3 docs/evidence/wan-w04-20261005/check_docs.py` | [Links, discovery, frozen source and preservation](logs/docs-check-final.log) |
| `go vet ./...` | Passed after final source freeze; [vet](logs/final-vet.log) |
| `git diff --check` | [Whitespace](logs/diff-check.log) |

Retained initial failures and repairs:

- `production-first.log`: actual inner-peer `httptrace.GotConn` verification
  accidentally ran on outer service TLS. Service contexts now retain cancellation
  explicitly without forwarding peer tracing values; production transfer passes.
- `resources-first.log`, `resources-second.log`, `resources-third.log`: metadata
  keepalives first exhausted the independent 256 pre-TLS socket limit, then the
  shared-source challenge bucket saturated while its fake clock was frozen.
  The capacity fixture releases metadata keepalives, paces supplementary source
  admission and advances its clock. No production quota was relaxed.
- `resources-fourth.log`: incomplete synthetic TLS test imports; corrected with
  real DNS SAN/root/pin verification. `resources-fifth.log` passes capacity and
  broker-boundary ciphertext inspection.
- `isolation-first.log`: unknown enrollment acceptance incorrectly required an
  initiator announcement. Acceptance now binds the exact stored offer and live
  authenticated control without requiring public registration; unchanged folder
  handlers still require explicit membership. `isolation-second.log` passes.
- `final-focused-race.log`, `restart-20.log`, `compatibility.log`: a five-counter
  admission refinement had four initial values. Corrected compilation precedes
  the passing final runs; failed runs count for no acceptance.

Other candidate runs are retained under `logs/`. Authoritative complete acceptance
is `complete-race.log`; final repeat/compatibility/manual/integrated logs are linked
above. Unexecuted physical WAN/Pi/operated-host/native-lifecycle checks stay explicit.
