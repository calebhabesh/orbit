# Actual commands and outcomes

All Go commands run from the repository root. Logs retain compile/fixture failures
and interrupted commands. Normal namespace-test skips do not receive coverage.
No live host namespace or personal root was faulted.

| Command | Outcome / record |
| --- | --- |
| `git status --short`, starting file SHA-256 snapshot, `git rev-parse HEAD` | Dirty inherited tree captured in manifest |
| `go test ./internal/protocol ./internal/config ./internal/control/terminalcontract ./internal/control ./internal/network ./internal/scheduler ./cmd/filesync -run 'TestWANW11\|TestBandwidthLimiter' -count=1` | Passed initial timing/queue slice; no matched cases in several packages explicitly logged |
| `go test ./internal/network ./internal/scheduler -run '^TestWANW11' -count=1` | First test compilation failed; repaired callback/type then passed (`runtime-focused-fixed.log`) |
| `go test -race ./internal/protocol ./internal/config ./internal/control/terminalcontract ./internal/control ./internal/network ./internal/scheduler ./internal/replication ./cmd/filesync -run 'TestWANW11\|TestBandwidthLimiter' -count=1 -v` | Passed core focused checks (`focused-race.log`) |
| `go test -race ./internal/network ./internal/scheduler ./internal/replication ./internal/control ./internal/config ./internal/protocol ./internal/control/terminalcontract ./cmd/filesync -count=1` | Passed complete compatibility packages (`compatibility-race.log`) |
| Same focused packages with `-run 'TestWANW11\|TestBandwidthLimiter' -count=2 -v` | Passed twice (`final-focused-race.log`); later serializer has separate final checks |
| `go test -race ./internal/network -run '^TestWANW11Competing' -count=2 -v` | Passed 32-peer slow DNS/relay cancel/join twice (`competition-race.log`) |
| `go test -race ./internal/replication ./tests/terminal -run '^TestWANW11(BandwidthReservation\|BinaryReviewed)' -count=1 -v` | Initial test type/review-file errors; repaired and passed (`reservation-cli-fixed.log`) |
| `go test -race ./tests/terminal -run '^TestWANW11ActualLargeSmallPeer' -count=1 -v` | Initial relay setup quota failed; bounded warmup quiet-refill retries then passed (`bandwidth-race-fixed.log`, `bandwidth-current.log`) |
| `go test -race ./tests/terminal -run '^TestWANW11(BinaryReviewedTimingAndRestart\|ActualLargeSmallPeerBandwidthAcrossRoutes)$' -count=1 -v` | Passed final measured bandwidth/burst and CLI journey (`final-terminal-w11.log`) |
| `go test -race ./internal/control/terminalcontract ./internal/control ./internal/config ./cmd/filesync -count=1` | Passed final zero-timing legacy serializer (`legacy-timing-race.log`) |
| `go test -race ./tests/terminal -run '^TestWANW11BinaryReviewedTimingAndRestart$' -count=1 -v` | Passed actual binary with final serializer (`final-serializer-binary.log`) |
| `go test -race ./internal/control ./internal/control/terminalcontract ./cmd/filesync -run 'TestWANW11\|Test.*Help' -count=1 -v` | Passed exact review/active-restart/legacy policy checks (`final-timing-review.log`); no CLI unit name matched |
| `go test -list '^TestWANW11' ./internal/... ./tests/terminal ./cmd/filesync` | 29 named cases discovered (`final-discovery.log`), including namespace cases that normally skip |
| `go test -c -race -o /tmp/orbit-w11-terminal*.test ./tests/terminal` | Initial format/type failures retained; current campaign binaries compiled; final checks build their own current binary |
| `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -c -o /tmp/orbit-w11-{network,replication,scheduler,terminal}-arm64*.test ./internal/{network,replication,scheduler}` or `./tests/terminal` | Separate cross-builds passed; exact destinations are recorded in manifests and compile logs |
| `ssh -o BatchMode=yes -o ConnectTimeout=10 rpi ...` | Read-only host/tool/user-namespace/disk/thermal inspection; Pi 4B aarch64, no Go install needed |
| `scp` cross-built binaries/namespace runner into new private marked `/tmp/orbit-w11*` directories on `rpi` | Passed; exact hosts/paths/files in `pi*-invocation.json` |
| Pi component binaries `-test.run='^TestWANW11' -test.v`; scheduler additionally `^TestBandwidthLimiter` | Passed (`pi-campaign.log`); guarded native test skip is not acceptance |
| `unshare --user --map-root-user --net python3 scripts/wan_daemon_roaming_namespace_test.py --test-binary ... --root ... --parent-namespace ...` | First firewall reply-direction failure; corrected actual daemon journey passed locally and on Pi (`daemon-campaign-fixed.log`, `pi-campaign.log`) |
| Same runner with `--latency-ms 25 --loss-percent 1` | Initial local untyped receipt failure retained; typed EOF receipt retry passed locally and on Pi (`daemon-netem-current.log`, `pi-netem-current.log`) |
| Same runner after excluding loopback control from netem | First shared IPv4/IPv6 filter priority refused before journey; distinct priorities passed local race (`daemon-scoped-fixed.log`, 138.83 s) |
| Pi guarded runner with `--latency-ms 25 --loss-percent 1 --default-timing` | First whole-loopback impairment failed owner-control dial; corrected scoped fixture passed (`pi-default-final.log`, 203.80 s) |
| Pi terminal binary `-test.run='^TestWANW11ActualLargeSmallPeerBandwidthAcrossRoutes$' -test.v -test.timeout=8m` | Current three-route fairness passed (`pi-bandwidth-current.log`, 135.91 s); earlier interrupted log gets no acceptance |
| `scp` daemon-metrics JSONL and namespace topology; Python summary | Captured actual measured samples (`metrics/`, `resource-summary.json`) |
| Guarded remote Python cleanup of only invocation-recorded canonical/private/owned/marked roots, after checking no process uses them | Passed (`pi-cleanup.log`); existing Pi services/data/network/sysctl untouched |
| Original W00 doc checker | Failed its frozen W01 tracker assertion (`docs.log`); original artifact preserved |
| Exploratory all-history relative-link check | Older O05 anchor mismatch (`docs-current.log`); outside W11, no historical edit |
| `python3 docs/evidence/wan-w11-followup-20261006/check_links.py` | Scoped owning-doc/follow-up links passed (`docs-final.log`) |
| `python3 -m py_compile scripts/wan_daemon_roaming_namespace_test.py` | Passed |
| `git diff --check` | Passed (`diff-check.log`) |
| `make check` | Earlier interrupted run unexecuted (`make-check.log`); resumed aggregate exited 2 at pairing after terminal/unit stages passed (`make-check-resumed.log`); repaired final targets pass separately below |

| `go test -race ./tests/terminal -run '^TestWANW11ActualLargeSmallPeerBandwidthAcrossRoutes$' -count=1 -v` | Final socket/RSS sampler passed (`final-socket-bandwidth.log`) |
| Pi terminal binary, same bandwidth test | Passed all three routes with socket/RSS metrics (`pi-sockets-bandwidth.log`, 128.04 s); guarded root cleanup passed (`pi-sockets-cleanup.log`) |
| Minimized setup operation timeout-poll tests, then repair and `-race -count=2` | Red before repair (`setup-wait-red.log`), green after repair (`setup-wait-green.log`); original operation/deadline retained |
| `go test ./tests/integration -run '^TestOrbitPairing_CLI_RunningDaemon_Parity$' -count=3 -v` | Real pairing passes three times (`pairing-repair.log`, 77.888 s) |
| `GOFLAGS=-race` affected W06/W11 CLI journeys | Relay/local capture/timing passed; guided harness premature EOF failure retained (`setup-wait-cli-regressions.log`) |
| Guided setup and timing/restart CLI journeys under race, `-count=2` | Passed after waiting for process reaping within original deadline (`guided-pty-reap-fixed.log`, 34.297 s) |
| `make test-integration` | Complete integration passes (`integration-final.log`, 96.858 s, exit 0) |
| `make fmt-check vet test test-model test-faults test-harness build-arm64 package` | All remaining targets pass (`check-remaining-targets.log`, exit 0); composed coverage, no claim of a second full aggregate rerun |

The invocation JSON files contain the exact namespace argv and disposable roots.
Pi binaries are non-race; local campaign binaries are race-instrumented. Candidate
addresses/NAT topology are simulated inside an actual owned Linux namespace.
The namespace scripts and sampler add no production service installation.
