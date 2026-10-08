# E02 commands — 2026-10-08

Base revision `82116cf`, Go `go1.27.1-X:nodwarf5 linux/amd64`, dev PC. All
state in marked disposable roots; daemons in the process tests are detached
(no user manager in their environment). The owner's `orbit.service` kept
`MainPID` 1666559 throughout.

## Focused tests

```sh
go test -list '^TestOnboardingE' ./internal/... ./cmd/orbit/... ./tests/...   # logs/test-list.txt: 24 names
go test ./internal/scheduler ./internal/control ./internal/terminal ./tests/terminal \
  -run '^TestOnboardingE02' -count=1 -v        # logs/e02-tests.log: 8 PASS
ORBIT_ONBOARDING_BASELINE=1 go test ./internal/terminal ./internal/replication ./tests/terminal \
  -run '^TestOnboardingE00' -count=1            # logs/e00-remaining-baseline.log: 8 expected FAILs (E03–E07)
```

Negative check: `TestOnboardingE02F03ControlRetryReachesRunningScheduler`,
run once with `WorkChanged` removed, failed (`task … state "queued", want
"completed"` after 5 s). The reload hook is required; the test was restored.

Real-PTY check: `TestOnboardingE02F09RealPTYEnterOpensRetry` (140×40, VT
emulator `scripts/terminal_vt.py`). Its first version sent Enter as soon as
`ROOT_UNAVAILABLE` was visible, which the Overview also shows, so Enter reached
an Attention list still loading and was lost. The script now waits for the
loaded list (`1 of 1`); three consecutive runs passed.

## Repository checks

```sh
make check            # FAIL at test-terminal: two T07 tests asserted the replaced
                      # advice text ("configured peer address", "orbit engine work retry");
                      # expectations updated, `go test ./tests/terminal -run TestTerminalT07` PASS
make test-terminal-packages                                       # exit 0
make fmt-check vet test test-integration test-model test-faults test-harness build build-arm64 package
                      # first try: fmt-check flagged the new scheduler test (gofmt -w); then exit 0
make test-terminal-pty test-terminal-onboarding-pty test-terminal-everyday-pty   # all exit 0
```

The remainder of the `tests/terminal` suite passed in that `make check` run.
