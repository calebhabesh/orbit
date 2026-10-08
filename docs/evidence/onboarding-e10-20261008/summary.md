# E10 summary — integration, packaging, host migration and trial readiness (2026-10-08)

**Complete.** Orbit 2.1.0 (`037584b`) and orbit-net 1.1.0 (`0da2e7d`).
[Commands](commands.md), [results](results.json), step results in
[logs/summary.txt](logs/summary.txt).

## Validation

Uncached, on `0da2e7d` plus the harness fixes described below:

| Check | Result |
| --- | --- |
| `tests/terminal` (full) | pass, 2036 s ([log](logs/make-check-run2-terminal-pass.log)) |
| rest of `make check`: packages, fmt, vet, unit, integration, model, faults, harness, builds | pass ([log](logs/make-check-rest.log)) |
| `make test-race-core` | one failure: `TestWANW15RejectedAnnouncementKeepsAcceptedOfferGeneration` ran out of its 15 s budget under full parallel race load; 6/6 alone and a full `internal/network` race rerun pass ([log](logs/test-race-core.log), [rerun](logs/test-race-network-rerun.log)) |
| PTY suites (terminal, onboarding, keys, everyday) | pass |
| `make demo`, `make test-orbit-net-rehearsal` | pass |
| After the attention fix (`037584b`): `make test`, full `tests/terminal`, package test | pass. The first post-fix terminal run exited 1 and its log was lost (my log scrub replaced the file mid-run); the rerun passed ([log](logs/post-fix-terminal-2.log)) |

The first `make check` ([failures](logs/make-check-run1-failures.log)) found
harness drift from E06/E08, fixed here. The old Overview-first PTY campaigns
and the W16 runner now select Overview explicitly. W16 asks for `--code --long`
where it needs a long v3 invitation. Its `cli()` failures name the command in
rehearsal mode, and the keys PTY waits for the reworded join prompt. The
2.1.0 bump had missed the integration packaging tests and
`scripts/terminal_package_test.py`. The script had passed against stale 2.0.0
artifacts in `dist/`; old artifacts now live in `dist/archive/`. The package
build date was also still 2026-10-01.

The W12 relay-probe timeout carried from E09 did not recur in the three full
terminal runs with logs after relay probes got the 10 s handshake bound. The
cause of the fourth run's failure (log lost) is unknown, so W12 cannot be
excluded.

## Trial finding fixed

The owner's PC showed **50 EXHAUSTED_WORK attention items** while the laptop
was simply off. Each periodic sync ended with `ROUTE_UNAVAILABLE` (or a
busy/restarting service code), and because the TUI only lands on Files when
nothing needs attention, they also kept it off Files. Syncs that could not reach the device now
are not attention: the next successful sync resolves them, and a device unseen
for 24 h is still the OFFLINE item. Other sync failures show once per folder,
device and cause. `TestOnboardingE10OfflinePeerSyncsAreNotAttention` fails
without the change. After the install the PC listed no attention items.

## Deployment (owner-confirmed 2026-10-08, 2 TiB/month)

orbit-net 1.1.0 (`orbit-net-v1.1.0-linux-arm64.tar.gz`, binary
`d559069b…438b838e`) replaced 1.0.0 on the hosted service and on its alert job.
`serve.json` gained `relay_month_bytes: 2199023255552`, and a drop-in added
`StateDirectory=orbit-net` for `relay-month.json`. `--check` passed before the
swap. After restart, metrics show `orbit_net_relay_month_limit_bytes
2199023255552`, the alert timer runs cleanly, and `orbit network doctor` from
the PC verifies DNS/TCP, TLS, directory and STUN. Backups are
`*.bak-20261008`. No Docker, Caddy, tunnel or firewall change.

## Hosts

| Host | Install | Version | Package SHA-256 |
| --- | --- | --- | --- |
| PC | tarball `install.sh user` | 2.1.0 `037584b` | `12c39ec664ff02b7…d3556aa299` (amd64 tar) |
| Laptop | tarball `install.sh user` | 2.1.0 `037584b` | same |
| Pi | `dpkg -i` | 2.1.0 `037584b` | `138dd2ce8ac71cc0…40c116ed850` (arm64 deb) |

The installer rewrote the PC and laptop user units from the fixed 8080 port
to `127.0.0.1:0`. The F01 drop-ins on the PC and the Pi were removed,
followed by `daemon-reload`. The Pi pilot daemon kept its PID throughout. The PC
service is running (login startup). The laptop and Pi daemons stay stopped
until the trial, as before.

Short code over the hosted service: the PC issued a code and a disposable
laptop state resolved it in preview (no enrollment submitted, then deleted):
the decrypted invitation produced an automatic-mode join plan. Non-interactive
`orbit join` needs `--pairing-profile` on a fresh device; interactive and TUI
joins offer the packaged operator.

## Trial guide

[docs/demo.md](../../demo.md) is the TUI walkthrough: create, short code, join
with defaults, approve, Files, conflict, history and connection check, plus the
scripted campaign.

Not executed: the owner's trial itself, including the Pi join and a
cross-network relay transfer with physical devices.
