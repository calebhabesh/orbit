# E09 commands — 2026-10-08

Base revision `4f8ea36`, Go `go1.27.1-X:nodwarf5 linux/amd64`, dev PC. All
services are local fixtures in disposable roots; the deployed VPS was not
contacted or changed.

```sh
go test ./internal/rendezvous ./cmd/orbit-net ./internal/control ./cmd/orbit \
  -run '^TestOnboardingE09' -count=1 -v     # logs/e09-tests.log: 7 PASS
#   RelayBudgetPersistsAndResets: 0600 state file; count survives reopen; a new
#     UTC month resets to 0 (simulated clock, 2026-10-31 -> 2026-11-01); a
#     non-private file and a zero budget are refused
#   RelayRefusesAndEndsAtBudget: live data relay ended at the chunk boundary
#     after exactly 65,536 bytes (64 KiB budget); a new data relay was refused
#     with RELAY_BUDGET (refusals{reason="budget"} = 1); a pairing relay still worked
#   RelayFramingOverhead (EG4): 4,194,304 payload bytes counted, 4,203,352 bytes
#     of service TLS wire egress = 0.22% framing overhead
#   RelayBudgetAlerts: FIRING relay_budget_80 at 81%; FIRING relay_budget_spent
#     and RESOLVED relay_budget_80 at 100%; RESOLVED relay_budget_spent when the
#     month resets
#   ServeBudgetConfigAndRestart: 2 TiB finite default; a negative budget refused;
#     $STATE_DIRECTORY/relay-month.json count restored on restart
#   RelayBudgetWording: "Relay unavailable until <date> ...; direct connections still work"
#   RouteShare: "Routes: 2 direct, 1 relay, 1 not connected"
make test-orbit-net-rehearsal               # logs/orbit-net-rehearsal.log: PASS
GOFLAGS=-p=4 make check                     # tests/terminal: 1 failure, TestWANW12BinaryDoctorPrivacyAndPTY
                                            #   (relay_inner_tls TIMEOUT); see below
GOFLAGS=-p=4 make test-terminal-packages fmt-check vet test test-integration test-model \
  test-faults test-harness build build-arm64 package          # exit 0
make test-terminal-pty test-terminal-onboarding-pty test-terminal-everyday-pty test-terminal-keys-pty   # all exit 0
```

## TestWANW12BinaryDoctorPrivacyAndPTY

Its explicit doctor's `relay_inner_tls` probe (a 3 s limit for relay
attachment plus the inner TLS handshake) returned `TIMEOUT`. The test accepts
only `VERIFIED` or `QUOTA_EXCEEDED` (which it retries). The same failure
reproduced on the unmodified E07 commit `4f8ea36`, with all E09 work stashed:
3 of 4 runs failed; it also passed one run there and in the E05/E07 full
checks. Load was low (load average 0.7). It is a pre-existing intermittent
failure, not an E09 regression. Its root cause is open and is listed for E10.
While bisecting, a single pass with one file reverted was briefly
misattributed to `relay_runtime.go`; reverting that file reproduced the
failure too.
