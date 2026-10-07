# WG6 alert checker — 2026-10-06

State: **deployed on the VPS and drilled 2026-10-06 23:51Z; owner receipt not yet confirmed.**
WG6 stays open until the owner approves deployment and confirms receipt.

`orbit-net alert --config FILE --state FILE` takes one loopback `/healthz` and
`/metrics` sample and posts to an ntfy-compatible topic only on firing/resolved
transitions for the runbook's alert table (down, profile/certificate expiry,
reload rejected, saturation, stranded devices, egress). Failed delivery keeps
the transition pending, exits nonzero and never echoes the topic URL. Config and
state must be owner-only files; the metrics origin must be numeric loopback and
the notify URL https without credentials. `--test` sends one test message and
`--metrics-url` supports the non-disruptive firing/recovery drill.
Packaged as `orbit-net-alert.{service,timer}` plus `alert.example.json`.

| Command | Result |
| --- | --- |
| `go test -race -count=1 -v ./cmd/orbit-net` | pass, 9 tests incl. 5 W16 alert tests ([log](logs/orbit-net-race.log)) |
| mutation: sustained 10→5 min, down samples 2→1 | the two corresponding tests fail; source restored |
| `go vet ./cmd/orbit-net` | pass |
| `make package-orbit-net` | amd64/arm64 archives include the units and example ([log](logs/package-orbit-net.log)) |
| `go test -count=1 -v -timeout 10m ./tests/terminal -run '^TestWANW13PackagedSelfHostRehearsal$'` | pass in 117.6 s with the extended archive listing ([log](logs/w13-packaged-rehearsal.log)) |
| `systemd-analyze verify` on the units | only reports `/usr/bin/orbit-net` absent on this workstation |

VPS installation and the test/firing/recovery drill ran ([drill](logs/vps-drill.log), [timer](logs/vps-timer-enable.log)); ntfy accepted all three posts. Owner receipt is not yet confirmed. Deployment steps are in the
[host-change proposal](../host-change-proposal.md#a-wg6-alerting-on-the-vps-ntfy).
