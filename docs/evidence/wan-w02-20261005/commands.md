# W02 validation commands

Run from repository root. Local temporary roots/listeners and synthetic data only.
`manifest.json` records the initial revision/dirty tree and file hashes. Existing
W00/W01 source/evidence was present and is preserved outside declared owning edits.
`results.json` and logs record exit status; cached M0 tests are distinguished from
the explicit uncached checks below. No privileged or native-host fault target.

Initial integration command `go test ./internal/network ./internal/replication
./internal/control ./internal/app` passed; [log](logs/initial.log).
The first policy fixture failed because its temporary state lacked required private
permissions; fixed the fixture to 0700, preserving production validation.
[First manager run](logs/manager-first.log). A new authorization test initially
used the wrong Hello fields; corrected it to a valid Inventory authorization
request and required an actual UNAUTHORIZED wire error.
[First focused compilation](logs/focused-first.log). Neither failure supports
an acceptance claim. [Uncached focused run](logs/focused.log) then passed.

Final commands (exact exits and logs in results.json):

```sh
go test -list '^TestWANW02' ./internal/... ./model/... ./tests/... ./cmd/filesync/...
go test -race -count=1 -timeout 90s -run '^TestWANW02' -v ./internal/network ./internal/config ./internal/replication ./internal/scheduler
go test -count=1 -timeout 120s ./cmd/filesync/... ./internal/control/... ./internal/replication/... ./internal/scheduler/... ./internal/config/... ./internal/network/... ./model/...
go test -count=1 -timeout 180s -v ./tests/terminal -run '^(TestTerminalT04TwoDeviceCLIInterruptedJoinAndEdits|TestTerminalT05ThreeProcessSharingOfflineRolloutAndEndpointRefresh)$'
go test -count=1 -timeout 120s -v ./tests/integration -run '^TestBackgroundSyncFromPersistedPeerEndpoints$'
make check
python3 docs/evidence/wan-w02-20261005/check_docs.py
git diff --check
```

Focused race includes every discovered W02 test (12 tests in network/config/
replication/scheduler). The production journeys use real CLI/daemon controls and
single-host nonloopback manual sockets. They verify reviewed create/join/approval,
restart, original-author forwarding, changed-address recovery, heads and hashes.
Background progress verifies a 16-MiB archive during continuing ordinary edits.
The final M0 run includes terminal/extracted-package checks, formatting, vet,
CLI/internal/model, integration, design-gate/fault checks, Python safety harness,
CGO-free amd64/arm64 binaries and packages; Makefile cached tests are labeled by Go.
Full uncached release race, native WAN/NAT/QUIC/ICE/roaming and T13 lifecycle checks
are unexecuted here and remain owned by their later packets.

The first M0 and a review M0 passed while final lifecycle/admission review was
still active. They are retained as `make-check.log` and `review-make-check.log`;
they do not claim frozen final-source provenance. After the per-borrow verifier
and independent pending-dial/owned-socket refinements, all 12 focused race tests
and the full uncached compatibility command passed again. `manifest.json` freezes
every Go source, go.mod/go.sum and Makefile for the subsequent final M0 command;
check_docs.py also verifies those hashes have not changed. `final-make-check.log`
is that frozen-source run. Repeated successful focused command results are kept
in supplemental-results.jsonl; the final log contains the last execution.

Final outcomes: discovery exit 0 (12 tests); focused race exit 0;
uncached compatibility exit 0; T04/T05 process journeys exit 0; background progress
exit 0; frozen-source `make check` exit 0. The final terminal suite executed
uncached in M0 (302.291 s); individual cached packages are labeled in the log.
Local links/anchors, preservation/frozen-source checks and `git diff --check`
exit 0. No zero-match runner or unexecuted native check supports completion.
