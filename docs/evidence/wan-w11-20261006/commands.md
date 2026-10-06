# Commands and actual outcomes

Commands ran from `<repo>`. Fixtures use private temporary roots;
namespace mutations require the disposable marker and remapped namespace ownership.
`manifest.json` captures the dirty starting tree. Logs retain failures separately.

| Command | Outcome / log |
| --- | --- |
| `go test -list '^TestWANW11' ./internal/... ./tests/... ./cmd/filesync/...` | Sixteen named tests discovered across control, network, replication, scheduler; [discovery](logs/discovery-final.log). Namespace case skips outside its explicit runner. |
| `go test -race ./internal/network ./internal/replication ./internal/scheduler -run '^TestWANW11' -count=2 -v -timeout=3m` | Passing policy/chunk/receipt/queue checks; [slice race](logs/w11-complete-slice.log). Later fresh-candidate change has the final focused and real binary passes below. |
| `go test -race ./internal/network ./internal/replication ./internal/scheduler ./internal/rendezvous -run '^TestWANW(08\|09\|10\|11)' -count=1 -v -timeout=8m` | [Final focused acceptance](logs/acceptance-final.log), includes prerequisite native/emulator NAT matrix, authorization, bounded resources and W11 tests. |
| `go test -race -c -o /tmp/orbit-w11-network.test ./internal/network` and `go test -race -c -o /tmp/orbit-w11-replication.test ./internal/replication` | Built race-instrumented native test binaries; exact hashes and namespace command in [runner record](namespace-accepted-command.json). |
| `unshare --user --map-root-user --net python3 scripts/wan_roaming_namespace_test.py --test-binary /tmp/orbit-w11-network.test --replication-binary /tmp/orbit-w11-replication.test --root <private-marked-root> --parent-namespace <parent-netns>` | Passed [native kernel/socket checks](logs/native-namespace-accepted.log); [exact rules](namespace-accepted-network.json). Address/default-route detection ~4/~6 seconds, native TCP/QUIC recovery ~42/~52 ms. No external route/service. |
| `GOFLAGS=-race go test -race ./tests/terminal -run '^TestWANW09BinaryLocalOnlyQUICAfterApproval$' -count=1 -v -timeout=3m` | Initial direct-only observation failed in [binary-native-route](logs/binary-native-route.log); fresh-candidate revision/reset fixes it in [repair](logs/binary-native-cache-repair.log). |
| `GOFLAGS=-race go test -race ./tests/terminal -run '^TestWANW07\|^TestWANW09' -count=1 -timeout=8m -v` | Passed actual Local-only QUIC, UDP collision/relay and keyboard onboarding [binary/PTY journeys](logs/binary-pty-current.log). |
| `make fmt-check vet test test-integration test-model test-faults test-harness build build-arm64 package test-terminal-packages` | Passed [aggregate](logs/aggregate-current.log). This command excludes the full terminal suite and does not imply native T13 lifecycle acceptance. |
| `make check` | Failed full terminal validation after 1,044.505 s: earlier repaired QUIC route cache and replacement-join expiry; [full output](logs/check.log). Not credited as passing. |
| `go test ./tests/terminal -run '^TestWANW05ThirdDeviceOfflineRolloutForkAndRetiredBootstrap$' -count=1 -v -timeout=4m` | [Focused replacement-join recheck](logs/w05-recheck.log); final outcome in results. |

Retained failed experiments:

- `initial.log`: the first compile failed after a mutex field accidentally changed
  another positional struct literal; the owning type was corrected. Initial retry
  passed in `initial-retry.log`.
- `w11-race.log`: a non-sliding cooldown early return retained its mutex and timed
  out at 90 seconds. Deferred unlock fixes it; `w11-race-fixed.log` passes.
- `routes-regression.log`, `fallback-repair.log`, `fallback-error-selection.log`,
  `fallback-race-repeat.log`, `responder-quota-repair-fixed.log`: new racing exposed
  remote quota at relay attachment plus competing error-order failures. Typed
  service refusals now survive losing waiter cancellation; explicit quota-refused
  accept/reservation/attachment phases get bounded quiet retry.
- `fallback-trace.log`, `fallback-race-trace.log`: narrowly tagged synthetic target/
  result/context traces. Temporary instrumentation was removed. No credentials,
  capability or content were logged.
- `responder-quota-repair.log`: a missing import caused compilation failure;
  subsequent runs repaired it. `responder-attach-quota.log` passes the minimized
  blocked-UDP oracle; complete final NAT coverage is in accepted focused logs.
- `same-lease-before.log`: local invalidation mutated signed-lease fields and
  rejected a fresh unchanged remote generation. Invalidation now retains those
  fields separately; final revalidation tests pass.

Two guarded diagnostic attempts to signal the long-running full terminal test
were refused by command/canonical-root/marker assertions as the fixture progressed.
No signal was sent. The full suite completed and its failures remain recorded.
Unexecuted: Advanced timing controls, Pi/latency/loss tuning, full daemon Wi-Fi/
relay switches, slow-service campaign and actual mixed-transfer/peak-resource
fairness. W11/WG5 completion is not inferred from these commands.

Final repair validation:

- `w05-expiry-trace.log`, `w05-admission-trace.log` retain the first refusal and
  subsequent admission sequence. A 60s prepared-submit throttle consumed the
  original one-minute proof. The first 25s repair still hit admission by polling
  possession twice immediately after successful submission; its failure remains
  in `w05-prepared-retry-repair.log`. Instrumentation printed only synthetic
  endpoint paths, counts/booleans, phases and timestamps; it has been removed.
- The final repair keeps the original prepared proof, selects 25s only if it
  fits that proof's unchanged deadline, and uses the authenticated submit result
  until the next scheduled status poll. Expired/lost submissions retain status
  recovery; it neither enlarges quotas nor regenerates authorization.
- `go test -race ./internal/control/... -run 'TestWANW11|Test.*T04' -count=1 -timeout=3m`
  passed in [control retry](logs/control-retry-final.log).
- `go test ./tests/terminal -run '^TestWANW05ThirdDeviceOfflineRolloutForkAndRetiredBootstrap$' -count=1 -v -timeout=4m`
  passed in 83.245 s; output is in [admission repair](logs/w05-admission-repair.log).
- `go test -race ./internal/control/... ./internal/network ./internal/replication ./internal/scheduler ./internal/rendezvous -run '^TestWANW(08|09|10|11)|^TestTerminalT04' -count=1 -v -timeout=8m`
  passed; output is in [current acceptance](logs/acceptance-current.log).
- `make check` rerun with the final repair **passed (exit 0)** in [current full check](logs/check-final.log). The complete terminal suite passed in 953.851 s; package extraction, formatting/vet, unit/integration/model/fault/harness checks and amd64/arm64 builds passed. The earlier failed attempt remains uncredited.

- `go test -race ./internal/network -run '^TestWANW11SlowLookupAndRelayShutdownJoin$' -count=3 -v -timeout=30s`
  passed in [slow-operation shutdown](logs/slow-service-shutdown-fixed.log).
  Context-aware lookup and relay attachment are deliberately blocked; shutdown
  cancels and joins them with zero retained pools/requests/dials. This is an
  injected operation delay, not a real slow-DNS/relay latency campaign. The initial
  compile failure naming the private callback type is retained in
  `slow-service-shutdown.log`.
- `go test -race ./internal/control/... -run '^TestOrbitSetup_' -count=1 -timeout=3m`
  passed in [setup regressions](logs/setup-regression-final.log).
