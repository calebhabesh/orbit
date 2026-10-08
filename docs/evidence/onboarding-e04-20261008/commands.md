# E04 commands — 2026-10-08

Base revision `e292743`, Go `go1.27.1-X:nodwarf5 linux/amd64`, dev PC. Every
service is a local `orbit-net` fixture (`w05Service`) in a marked disposable
root; the deployed VPS and the owner's devices were not contacted.

## Focused tests

```sh
go test -list '^TestOnboardingE' ./internal/... ./cmd/orbit/... ./tests/...   # logs/test-list.txt: 31 names
go test ./internal/terminal ./internal/control ./internal/control/terminalcontract \
  -run '^TestOnboardingE04' -count=1 -v       # logs/e04-model-unit.log: 6 PASS
go test ./tests/terminal -run '^TestOnboardingE04' -count=1 -v
                                              # logs/e04-process.log: PASS (21 s). A fresh joiner
                                              # takes every default; the plan is self_hosted with the
                                              # inviter's profile from the invitation; it joins, F16
                                              # 0644 -> 0600; the inviter lists "Pi" in text, JSON and
                                              # terminal control
ORBIT_ONBOARDING_BASELINE=1 go test ./internal/terminal ./internal/replication \
  -run '^TestOnboardingE00' -count=1          # logs/e00-remaining-baseline.log: F05 (E05), F10 (E07) fail as expected
```

`TestOnboardingE00F10RelayJoinWait` (tests/terminal, ~3.5 min, E07's
baseline) was not rerun here; its F08/F11/F16 parts moved to the E04 test.

## Repository checks

```sh
GOFLAGS=-p=4 make check      # tests/terminal: 5 WAN in-process tests failed (W05 x2, W08,
                             # W09, W11). E04's new network_restart phase treated a
                             # controller without a recorded starting policy as needing
                             # a restart. Fixed: a restart is needed only when the relay
                             # runtime is absent or the known starting policy differs.
go test ./tests/terminal -run '<the 5>|TestOnboardingE04' -count=1     # PASS
GOFLAGS=-p=4 make test-terminal-packages fmt-check vet test test-integration test-model \
  test-faults test-harness build build-arm64 package                    # exit 0
GOFLAGS=-p=4 go test ./tests/terminal -run 'TestWAN|TestOnboarding' -count=1   # PASS (1,137 s)
make test-terminal-pty test-terminal-onboarding-pty test-terminal-everyday-pty test-terminal-keys-pty
                             # all exit 0 (run with the first make check)
```
