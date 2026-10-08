# E01 commands — 2026-10-08

Base revision `a49c3d2`, Go `go1.27.1-X:nodwarf5 linux/amd64`, dev PC (Arch
Linux, kernel 7.2.8). All state lives in marked disposable roots. Service
managers are stand-ins on `PATH` except in the opt-in transient-unit check.
The owner's real `orbit.service` was never started, stopped or rewritten: its
`MainPID` (1666559) and unit/drop-in mtimes (2026-10-07 23:00, 2026-10-08
00:08) were the same before and after.

## Focused tests

```sh
go test -list '^TestOnboardingE' ./internal/... ./cmd/orbit/... ./tests/...   # logs/test-list.txt: 20 names
go test ./tests/terminal ./internal/terminal -run '^TestOnboardingE01' -count=1 -v
                       # logs/e01-tests.log: 7 PASS, 1 SKIP (opt-in real manager)
ORBIT_E01_REAL_USER_MANAGER=1 go test ./tests/terminal \
  -run '^TestOnboardingE01F01RealUserManagerTransientUnit$' -count=1 -v
                       # logs/real-user-manager.log: PASS; transient unit orbit-e01-<rand>,
                       # 8080 held by java, control.addr 127.0.0.1:<ephemeral>, journal has
                       # "agent ready"; afterwards `systemctl --user list-units 'orbit-e01-*'` = 0
ORBIT_ONBOARDING_BASELINE=1 go test ./internal/terminal ./internal/scheduler \
  ./internal/replication ./tests/terminal -run '^TestOnboardingE00' -count=1
                       # logs/e00-remaining-baseline.log: 12 expected FAILs for later packets
```

## No privilege escalation (EG3)

```sh
grep -rn --include='*.go' -E 'exec\.Command[^(]*\([^)]*"(sudo|pkexec|doas)"|"enable-linger"' cmd internal | grep -v _test
                       # no matches (exit 1)
grep -rn --include='*.go' 'enable-linger' cmd internal | grep -v _test
                       # only display strings: service.go LingeringInstruction,
                       # host_startup.go LingerCommand, doctor.go remediation
```

The test stand-ins `sudo` and `loginctl` record every call and fail the test
on any `sudo` use or `enable-linger` (`e01Stubs`); no run recorded either.

## Repository checks

```sh
make check            # run 1: FAIL in tests/terminal (10 tests): disposable setups on this
                      #   desktop were proposed login startup and blocked at enable with
                      #   SERVICE_SELECTION_REQUIRED (the owner's unit serves another state).
                      #   Fixed: HostStartupFor scopes the proposal to states that can use the unit.
make check            # run 2: tests/terminal PASS (whole suite); then test-integration
                      #   FAIL: 2 tests asserted the old 8080 unit text and the old linger
                      #   instruction without sudo; expectations updated
make fmt-check vet test-integration test-model test-faults test-harness build build-arm64 package
                      # the targets run 2 never reached: exit 0
make test-terminal-pty test-terminal-onboarding-pty test-terminal-everyday-pty
                      # real-PTY harnesses: all exit 0
```

Earlier steps in run 2 (test-terminal, test-terminal-packages, fmt-check, vet,
test) passed before the integration failure. After the two integration
expectations were corrected, those targets were not rerun as a single uninterrupted
`make check`.
