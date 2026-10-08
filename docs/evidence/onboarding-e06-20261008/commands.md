# E06 commands

```sh
go test -count=1 -v -run OnboardingE06 ./internal/pairing/ ./internal/rendezvous/ ./internal/terminal/ ./tests/terminal/
# Real PTY: the TUI short-code join is part of the WAN PTY campaign
go test -count=1 -run TestWANW07RealPTYRelayOnboarding ./tests/terminal/
make check   # see E10 logs for the uncached run
```
