# W14 closeout commands

Commands ran from the repository root with `ORBIT_DISABLE_PACKAGED_PROFILE=1`.
The focused W14 race command also sets `GOFLAGS=-race` so binaries built by its
PTY/mixed-version tests carry race instrumentation. W14's explicit packaged
profile/manual-mode checks opt back in without activating Automatic mode.

Each JSON record contains exact argv, start/end time, exit code, duration and
source fingerprints before/after the command. Use the accompanying log for test
names, subprocess oracles and skip boundaries. Commands can be recorded again
with `python3 docs/evidence/wan-w14-closeout-20261006/record_command.py NAME ARGV...`.

| Command | Result | Record / log |
| --- | --- | --- |
| `go test -list '^TestWANW14' ./internal/network ./internal/config ./internal/control ./internal/control/terminalcontract ./tests/terminal` | Passed; 12 named tests discovered | [record](discovery.json), [log](logs/discovery.log) |
| `GOFLAGS=-race go test -race -count=1 -v -timeout=8m -run '^TestWANW14' ./internal/network ./internal/config ./internal/control ./internal/control/terminalcontract ./tests/terminal` | Passed | [record](focused-w14-race.json), [log](logs/focused-w14-race.log) |
| `go test -count=1 -v -timeout=8m -run '^TestWANW07RealPTYRelayOnboarding$' ./tests/terminal` | Passed | [record](w07-regression.json), [log](logs/w07-regression.log) |
| `go test -race -count=1 -timeout=8m ./cmd/filesync/... ./internal/app/... ./internal/config/... ./internal/control/... ./internal/controlclient/... ./internal/network/... ./internal/terminal/...` | Passed; app/controlclient have no direct tests | [record](affected-race.json), [log](logs/affected-race.log) |
| `make GOFLAGS=-v check` | Passed, exit 0; terminal suite 1294.436 s | [record](aggregate-final.json), [log](logs/aggregate-final.log) |
| `bin/orbit version` | Reports the signed packaged release profile, epoch 1 | [record](packaged-version.json), [log](logs/packaged-version.log) |
| `python3 docs/evidence/wan-w14-closeout-20261006/check_links.py` | Passed | [record](docs-links-final.json), [log](logs/docs-links-final.log) |
| `git diff --check` | Passed after removing four trailing document blank lines | [record](whitespace-final.json), [log](logs/whitespace-final.log) |
| `python3 docs/evidence/wan-w14-closeout-20261006/verify_preservation.py` | Passed; code and earlier evidence preserved | [record](preservation-final.json), [log](logs/preservation-final.log) |

The inherited `make check` failed (exit 2) after 1,156.361 seconds in the
terminal package; its production binary predates the final rendering edit.
Its [original log](../wan-w14-20261006/logs/make-check.log) is preserved.
Native commands in the prior worker's [script](../wan-w14-20261006/w14-native.sh)
and [transcript](../wan-w14-20261006/logs/native-journey.log) are inherited evidence,
not commands rerun in this closeout.

Unexecuted here: full uncached race, demo, new physical WAN/Pi runs, native
expired bundled-profile run and WG6 owner backup/alert operations. W15/W17 own
the broader release campaigns; existing historical evidence remains scoped to
its original hosts, source and failure models.

The aggregate [raw stdout/stderr](logs/aggregate-final.raw.gz) is preserved
losslessly. Its readable `.log` normalizes line endings and trailing whitespace
to satisfy the repository whitespace check after another session committed the
in-progress capture. Exit codes and test results are unchanged.
