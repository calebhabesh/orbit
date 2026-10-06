# W14 commands and results (2026-10-06)

| Command | Result |
| --- | --- |
| `go test ./internal/network/ ./internal/config/ ./internal/control/... ./internal/terminal/ ./internal/protocol/ ./internal/rendezvous/` | pass |
| `go test ./cmd/...` | pass |
| `go test ./tests/terminal -run TestWANW14MixedVersionUpgradeAndRollback` (legacy `ef462f2`) | pass (32.95 s) |
| `go test ./tests/terminal -run TestWANW14OneStep` (real PTY CLI + TUI, real packaged profile, no service contact) | pass |
| `make package` | pass; manifest records packaged profile digest `356f0ced…ec165` |
| `docs/evidence/wan-w14-20261006/w14-native.sh dist` (laptop + Pi, live hosted service) | pass; [log](logs/native-journey.log) |
| `make check` ([log](logs/make-check.log)) | **failed** at test-terminal: `TestWANW07RealPTYRelayOnboarding` waited for the `Inviter operator:` label that W14 had reworded. Source was also edited during the run. Remaining make-check targets did not run. |
| Fix: restore label; `go test ./tests/terminal -run 'TestWANW07RealPTYRelayOnboarding\|TestWANW14' -count=1` | pass (181.7 s) |
| Full `make check` rerun on final source | **unexecuted** |
