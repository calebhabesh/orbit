# E03 commands — 2026-10-08

Base revision `de2230e`, Go `go1.27.1-X:nodwarf5 linux/amd64`, dev PC. All
state lives in marked disposable roots; the owner's `orbit.service` was untouched.

## Focused tests

```sh
go test -list '^TestOnboardingE' ./internal/... ./cmd/orbit/... ./tests/...   # logs/test-list.txt: 26 names
go test ./internal/terminal ./tests/terminal -run '^TestOnboardingE03' -count=1 -v   # logs/e03-tests.log: 5 PASS
make test-terminal-keys-pty      # logs/keys-pty.log: new real-PTY campaign, passed:
                                 # setup form via arrows / Tab / Enter only; bracketed and
                                 # unbracketed invitation paste after a failed attempt;
                                 # approval with arrow keys only; selector ignores typing
ORBIT_ONBOARDING_BASELINE=1 go test ./internal/terminal ./internal/replication ./tests/terminal \
  -run '^TestOnboardingE00' -count=1  # logs/e00-remaining-baseline.log: 5 expected FAILs (E04, E05, E07)
```

Negative check: a build with replace-on-type disabled failed the keys
campaign at the unbracketed retry (`'1,419 characters received' absent`), so
the campaign detects appending. The source was restored before every later run.

## Repository checks

```sh
make check            # run 1: stopped by the host's memory-pressure reaper (not a test
                      #   failure; /tmp tmpfs held ~20 GB of unrelated data)
GOFLAGS=-p=4 make check   # run 2: tests/terminal FAIL in 4 PTY tests that drove the
                      #   old keys: Tab-to-search (T09, T12), Enter-submits-from-any-field
                      #   (W07, W16). Harnesses updated (/ for search; submit helper). The
                      #   TUI now focuses the root field when preview rejects the root.
                      #   The four tests then PASS.
GOFLAGS=-p=4 make test-terminal-packages fmt-check vet test test-integration test-model \
  test-faults test-harness build build-arm64 package          # exit 0
make test-terminal-pty test-terminal-onboarding-pty test-terminal-everyday-pty test-terminal-keys-pty
                                                              # all exit 0
```
