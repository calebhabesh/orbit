# W10 executed commands

Commands run from the repository root on the recorded Linux host. Final logs are
in `logs/`; first and superseded attempts remain beside them. Host invocations
skip the two explicitly guarded namespace tests; their separate runner executes
real native UDP in a fresh marked user/network namespace with no external route.

| Command | Final log |
| --- | --- |
| `go test -race ./internal/network ./internal/protocol ./internal/rendezvous ./internal/replication -run '^TestWANW10' -count=2 -timeout=240s -v` | [focused-final-accepted.log](logs/focused-final-accepted.log) |
| `go test -race ./internal/network ./internal/protocol ./internal/rendezvous ./internal/replication ./internal/config ./internal/control/... ./internal/terminal ./cmd/filesync -count=1 -timeout=300s` | [compatibility-final-accepted.log](logs/compatibility-final-accepted.log) |
| `make fmt-check vet test test-integration test-model test-faults test-harness build build-arm64 package` | [aggregate-final-accepted.log](logs/aggregate-final-accepted.log) |
| `GOFLAGS=-race go test -race ./tests/terminal -run '^TestWANW07\|^TestWANW09' -count=1 -timeout=8m -v` | [binary-pty-gather-accepted.log](logs/binary-pty-gather-accepted.log) |
| `go test -race -c ./internal/replication -o /tmp/orbit-w10-replication-final.test` followed by two executions of [exact namespace command](namespace-final-command.json) | [native-quota-accepted.log](logs/native-quota-accepted.log) |
| Four commands with explicit `CGO_ENABLED=0 GOOS=linux GOARCH=amd64/arm64` in [cross-build-commands.json](cross-build-commands.json) | [cross-build-accepted.log](logs/cross-build-accepted.log) |
| `python3 scripts/terminal_package_test.py --dist dist` | [package-accepted.log](logs/package-accepted.log) |
| `python3 schemas/fixtures/ice-v1/generate.py` | [fixture-accepted.log](logs/fixture-accepted.log) |
| `python3 docs/evidence/wan-w10-20261006/check_evidence.py` | [evidence-check.log](logs/evidence-check.log) |
| `git diff --check` | [diff-check.log](logs/diff-check.log) |

The namespace runner verifies a private disposable root, explicit marker, changed
network namespace and its owning user namespace before any link/address mutation.
[Network rules](namespace-network.json) show only dummy local addresses, including
IPv6; production HTTPS/WSS still uses native sockets. Emulator mappings/filtering,
inner double-NAT topology and blocked peer UDP are logged per case in the focused
run. NAT labels describe configured rules, never a classification from timeout.

Diagnostic attempts include a blocked adapter read requiring termination of its
own test process (`ice-first.log`), srflx related-address serialization and frozen
pair-address corrections (`ice-second.log` through `ice-fourth.log`), the initial
missing announced ICE capability (`namespace-first.log`, `namespace-minimal.log`),
and successful but excessively repeated fallback attempts (`namespace-second.log`).
The latter motivated the 12-second complete attempt and one failure per pool.
`authenticated-nat-final.log` exposes borrowing a retired pair; cache retirement
now precedes endpoint closure, with `pair-retirement-regression.log` passing twice.
`directory-outage-before.log` exposes control-owned pair cancellation; a runtime
owner fixes it, verified by `directory-outage-after.log` and the final full-path
new-file transfer. `ice-alias-before.log` exposes case-insensitive optional-key
acceptance; `ice-alias-after.log` and the final protocol run reject the alias.
Failed runs are diagnostic evidence, not acceptance. All earlier successful logs
are superseded by final runs where affected by later changes.

The combined binary/PTY run in `binary-pty-accepted.log` timed out querying the
control endpoint in the UDP-collision journey. The isolated repeat is retained
in `binary-collision-recheck.log`. A deterministic regression in
`no-local-candidates-before.log` proves the controlled role spent twelve seconds
waiting for an offer despite an empty local gather. Gathering now precedes any
coordination request; both roles must report `ICE_NO_CANDIDATES` within two seconds
in the marked no-interface fixture. `native-quota-accepted.log` and the final
focused/compatibility/aggregate/binary logs validate that fix. The original control
timeout is retained as a failed run; it is not credited or labeled a CPU failure.

The final collision-only repeat uses
`GOFLAGS=-race go test -race ./tests/terminal -run '^TestWANW09BinaryOptionalUDPCollisionRelayOnboarding$' -count=2 -timeout=4m -v`:
[binary-collision-final-accepted.log](logs/binary-collision-final-accepted.log).
The passing full binary/PTY run precedes the removal of unnecessary service
release attempts on empty gathering; the final collision repeat and native/full-path
runs cover that change. No authority or terminal code changed.

Native follow-ups `native-gather-accepted.log` and `native-gather-final.log` expose
metadata quota after prompt gathering removed the old twelve-second accidental
refill. `native-final-accepted.log` proves one-second retries keep consuming refill
without leaving enough for a fresh relay's lookup/offer/reservation. The fixture
now retries only the typed `QUOTA_EXCEEDED` failure after a five-second quiet refill,
within a twelve-second retry deadline, retaining strict two-way heads/bytes/chunk
oracles; independent role checks then wait two seconds for their lookup budget.
`native-quota-accepted.log` passes both complete namespace executions. Limits were
not raised and production cooldown/fairness remains W11 work. Failed native runs
remain uncredited. The link audit was rerun after creation of its diff log; the
initial missing-log ordering failure is corrected.
