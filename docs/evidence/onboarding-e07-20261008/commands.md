# E07 commands — 2026-10-08

Base revision `8b5a088`, Go `go1.27.1-X:nodwarf5 linux/amd64`, dev PC. The
service is a local `orbit-net` fixture with production admission limits; the
inviter's enrollment server uses its production per-source bucket (5/min,
burst 5). The deployed VPS was not contacted.

```sh
go test ./internal/replication ./internal/control -run 'TestOnboardingE07' -count=1 -v
    # logs/e07-unit.log: 2 PASS. 30 minutes at the fastest new spacing (30 s, two
    # requests per check) is never refused; the old 15 s spacing is refused
    # within two minutes; the interval stays within [30 s, 36 s)
ORBIT_E07_WAIT=30m go test ./tests/terminal -run TestOnboardingE07F10RelayJoinWait -count=1 -v
    # logs/relay-wait-30min.log: PASS (1,811 s). Two real daemons; the joiner
    # waited 30m1s unapproved with no error codes and no blocked state (900
    # local service requests); approval was noticed after 8 s. (The line
    # "first RATE_LIMITED after 0s" is the unset counter: none occurred.)
go test ./tests/terminal -run TestOnboardingE07 -count=1     # ordinary 100 s wait: PASS; approval noticed after 27 s
GOFLAGS=-p=4 make check
    # tests/terminal: 9 in-process tests failed. Their fixtures sleep a fixed 25 s
    # after approval, which assumed the old 15 s spacing. Added control.Options.ApprovalPoll
    # (zero = production 30–36 s); the sleeping fixtures set 15 s
    # (baseline/setup/W05 fixtures, integration pairing). The 9 then PASS
    # (412 s), and test-integration's pairing test likewise.
GOFLAGS=-p=4 make test-terminal-packages fmt-check vet test test-integration test-model \
  test-faults test-harness build build-arm64 package                          # exit 0
make test-terminal-pty test-terminal-onboarding-pty test-terminal-everyday-pty test-terminal-keys-pty   # all exit 0
```
