# Orbit onboarding and everyday-use status

Updated: 2026-10-08. **E00–E10 complete.** The
[plan](../orbit-onboarding-implementation-plan.md) and
[packet details](onboarding-packets.md) form the handoff. Orbit 2.1.0 is
installed on the PC, laptop and Pi, and orbit-net 1.1.0 (short codes, 2 TiB
monthly relay budget) runs on the hosted service. The owner's three-machine
trial follows [the trial guide](../demo.md).

## Packet tracker

| Packet | State | Dependencies | Acceptance owner |
| --- | --- | --- | --- |
| E00 Baseline/reproductions | complete ([evidence](../evidence/onboarding-e00-20261008/summary.md)) | repository | [E00](onboarding-packets.md#e00--baseline-and-reproductions) |
| E01 Lifecycle/service defaults | complete ([evidence](../evidence/onboarding-e01-20261008/summary.md)) | E00 | [E01](onboarding-packets.md#e01--daemon-lifecycle-and-service-defaults) |
| E02 Attention/actions | complete ([evidence](../evidence/onboarding-e02-20261008/summary.md)) | E00 | [E02](onboarding-packets.md#e02--self-healing-attention-and-runnable-actions) |
| E03 Keys/forms/paste | complete ([evidence](../evidence/onboarding-e03-20261008/summary.md)) | E00 | [E03](onboarding-packets.md#e03--keyboard-form-and-paste-conventions) |
| E04 Join/setup defaults | complete ([evidence](../evidence/onboarding-e04-20261008/summary.md)) | E01, E03 | [E04](onboarding-packets.md#e04--join-and-setup-defaults-actionable-onboarding-errors) |
| E05 Long invitation | complete ([evidence](../evidence/onboarding-e05-20261008/summary.md)) | E03 | [E05](onboarding-packets.md#e05--long-invitation-display-copy-and-file-transfer) |
| E06 Short code | complete ([evidence](../evidence/onboarding-e06-20261008/summary.md)) | E04, E05, EG1 | [E06](onboarding-packets.md#e06--short-pairing-code-through-the-orbit-service) |
| E07 Approval wait | complete ([evidence](../evidence/onboarding-e07-20261008/summary.md)) | E00, E04 | [E07](onboarding-packets.md#e07--approval-waiting-within-service-limits) |
| E08 Files view | complete ([evidence](../evidence/onboarding-e08-20261008/summary.md)) | E02, E03, EG2 | [E08](onboarding-packets.md#e08--read-only-files-view-and-default-landing) |
| E09 Relay budget | complete; deployed at E10 ([evidence](../evidence/onboarding-e09-20261008/summary.md)) | E00, EG4 | [E09](onboarding-packets.md#e09--monthly-relay-egress-budget-and-busy-relay-ux) |
| E10 Trial readiness | complete ([evidence](../evidence/onboarding-e10-20261008/summary.md)) | E00–E09 | [E10](onboarding-packets.md#e10--integration-packaging-host-migration-and-trial-readiness) |

| Gate | State | Owning packet |
| --- | --- | --- |
| EG1 Short-code security | **closed** ([E06](onboarding-gates.md#eg1--short-code-security-e06)) | E06 |
| EG2 Files-view truthfulness | **closed** ([E08](onboarding-gates.md#eg2--files-view-truthfulness-e08)) | E08 |
| EG3 Host-class startup default | **closed** ([E01](onboarding-gates.md#eg3--host-class-startup-default-e01)) | E01 |
| EG4 Relay egress accounting | **closed** ([E09](onboarding-gates.md#eg4--relay-egress-accounting-e09)) | E09 |

## Trial findings

Observed during the owner's Orbit 2.0.0 trial on 2026-10-07/08 (build `8abd497`,
docs `58af25e`): PC created the Orbit in Automatic mode, the laptop joined and
reached Ready over direct QUIC. E00 reproduced every finding on disposable state;
each test is named in the [E00 summary](../evidence/onboarding-e00-20261008/summary.md)
and runs with `ORBIT_ONBOARDING_BASELINE=1`. E00 corrections: F05 hard-wraps rather
than truncating with `…`; F08 also affects the CLI join; F10 comes from the inviter's
enrollment bucket, not `orbit-net`.

| ID | Finding | Observation / source | Packet |
| --- | --- | --- | --- |
| F01 | Packaged user unit hardcodes `--control-listen=127.0.0.1:8080`; the service crash-loops when another app holds the port | PC journal: `bind: address already in use` (port held by an unrelated Java app); Pi has a different Java app on 8080; `packaging/systemd/orbit.service:8` | E01 |
| F02 | A scan that failed because the root was briefly missing stays as `EXHAUSTED_WORK` after later scans succeed | Task `47eb67dc…`: `ROOT_UNAVAILABLE`, exhausted after 1 attempt, while 71 later scans completed | E02 |
| F03 | Attention advice `orbit engine work retry --task …` refuses while the daemon runs | `orbit: state directory is already owned by another agent` | E02 |
| F04 | Header showed `Startup: login` and `running` while the daemon was the TUI-spawned one and the unit was failing | `orbit status`/`systemctl --user status orbit` | E01 |
| F05 | TUI reveal truncates the ~1,666-character invitation code to panel width with `…` | `internal/terminal/theme.go` `panel` truncates rows; `setup_render.go` `invitation_out` | E05 |
| F06 | Hidden invitation field appends each paste to existing content; no visible length; retries cannot succeed | `internal/terminal/app.go` `PasteMsg` handler uses `Value() + text` | E03 |
| F07 | Invitation failures show `INVALID_REQUEST`/generic advice instead of "code incomplete" | Owner screenshot; `workflowError` in `internal/terminal/setup.go` | E04 |
| F08 | A fresh join stays in `manual` network mode; a routed invitation then blocks with `NETWORK_REVIEW_REQUIRED` | Laptop `orbit network status`: `Connection: manual`; owner fixed with `orbit network automatic` | E04 |
| F09 | Enter on an `AWAITING_APPROVAL` attention item opens a generic operation screen and fails with `INTERNAL_ERROR` | `internal/terminal/app.go:365` checks `OperationID` before the approval code | E02 |
| F10 | Joining device showed `RATE_LIMITED` while waiting for approval | Owner screenshot. **E00 cause:** inviter enrollment per-source bucket (5/min, burst 5) vs. 15 s status polls of two POSTs each (8/min); not an `orbit-net` limit (`orbit-net` returns `QUOTA_EXCEEDED`) | E07 |
| F11 | Inviter lists the joined laptop as `Device 41ae…` instead of the name the laptop chose | PC `orbit devices` after approval | E04 |
| F12 | Startup is a typed `manual/login/unattended` field; join defaulted to manual; unattended needs lingering with no guidance | `internal/terminal/setup.go:221`; `Linger=no` on PC, laptop and Pi | E01, E03 |
| F13 | Many failures share "Retry; use orbit doctor to inspect local control/network reachability." | `internal/terminal/setup.go:362` | E02 |
| F14 | `orbit service stop/start` failed (`IO_ERROR`, `MANUAL_DAEMON_RUNNING`) while a TUI-spawned daemon ran; that daemon logs nowhere | PC transcript 2026-10-08; empty journal for the laptop's TUI-spawned daemon | E01 |
| F15 | `--review-file ~/r.json` fails with "state directory … must not be accessible" when `~` is 0750 | `cmd/orbit/terminal_setup.go` `writeSetupRequest` | E03 |
| F16 | Received files are owner-only (`0600`) while the source was `0644` | Laptop `~/Orbit` listing; by design per [persistence](../persistence.md) (safe local permissions) | E04 (document only) |

Also observed and working: Automatic mode, operator review, routed enrollment
by invitation file, verification-code approval from the CLI, direct QUIC between
PC and laptop, and download of all seven files.

## Host workarounds removed in E10

Applied 2026-10-08 with the owner's go-ahead to unblock the trial (F01):
`~/.config/systemd/user/orbit.service.d/control-port.conf` on the PC and the
Pi. Both were removed at E10 after the 2.1.0 install rewrote the user units to
`--control-listen=127.0.0.1:0`. Copies are kept in the E10 session scratch space,
not in the repository. The laptop needed none. Owner trial state: PC and laptop
joined in folder `Orbit`; Pi not yet joined.

## Packet log

### E00 — complete (2026-10-08)

Revision `22f4bee`; installed 2.0.0 checksums (PC/laptop `d8607009…`, Pi deb
`0bc710de…`) and host-class probes recorded. 14 `TestOnboardingE00…` cases in
`internal/terminal`, `internal/scheduler`, `internal/replication` and
`tests/terminal`; opt-in run fails as intended on all 15 open findings; F16
passes as specified behavior. The ordinary run skips all 14 (exit 0). Gate
scoping is in [onboarding-gates.md](onboarding-gates.md). Unexecuted: real-PTY
capture of F05/F06 (left to E03/E05) and `make check` (no production change).
[Commands](../evidence/onboarding-e00-20261008/commands.md),
[results](../evidence/onboarding-e00-20261008/results.json).

### E01 — complete (2026-10-08)

Base `a49c3d2`. F01, F04, F14 and the F12 startup default are fixed, and EG3 is closed. The units
listen on `127.0.0.1:0`. The launcher starts `orbit.service` when it serves the selected
state, and otherwise runs a detached daemon logging to `<state>/daemon.log`. Status reports
`owner` and `unit_state` apart from the startup mode. `orbit service start/restart` hand
a terminal daemon over to the unit, and `stop` also stops it. Host-class startup
proposals are scoped to states the unit can serve. Unit selection trusts
`systemctl --user show -p ExecStart` over `$HOME` files. `TestOnboardingE00F01…` and
`…F04F14…` were promoted to 8 `TestOnboardingE01…` cases (7 ordinary plus 1 opt-in
real-manager check, run once). `make check` run 1 found
state-unscoped proposals blocking disposable setups (fixed). Run 2 found two stale
integration expectations (updated). The targets after them and all three
PTY harnesses pass. Unexecuted: one uninterrupted `make check` after that last
test-only fix, and real-host upgrades (E10).
[Commands](../evidence/onboarding-e01-20261008/commands.md),
[results](../evidence/onboarding-e01-20261008/results.json).

### E02 — complete (2026-10-08)

Base `82116cf`. F02: a completed full scan supersedes earlier exhausted scans
(`SUPERSEDED`, original error kept). F03: `orbit retry` and the engine command go
through the daemon's retry route, and `WorkChanged` wakes the scheduler (a
negative check proved it is needed). F09: Enter routes through a per-code
table covering all 14 emitted codes; a real-PTY Enter → retry → Enter passes.
F13: per-code advice. Also fixed: advice naming a nonexistent `conflicts resolve`
and a relocate command missing required flags. `make check` found two stale T07 text assertions (updated);
the remaining targets and the PTY harnesses pass. Unexecuted: one uninterrupted
`make check` after those test-text fixes, and PTY checks for codes other than
`EXHAUSTED_WORK`. Observed, not fixed: outside Overview the TUI header shows the
daemon as `unknown`.
[Commands](../evidence/onboarding-e02-20261008/commands.md),
[results](../evidence/onboarding-e02-20261008/results.json).

### E03 — complete (2026-10-08)

Base `de2230e`. Arrow keys and Tab move between form fields, and Enter advances
and confirms on the last field. Startup and Connection are `‹ ›` selectors.
Keys `1`–`4` and Tab switch views, and `/` searches. A failed preview focuses
the field at fault. F06: a paste replaces the hidden invitation and the field
shows a character count. Ctrl-U clears it, and after a failed attempt a typed
re-paste replaces the old value. F15: private-file errors name the folder and
the `mkdir -m 700` fix. The new `make test-terminal-keys-pty` campaign passes,
and a negative build shows it catches appending. Four older PTY harnesses
assumed the old keys and were updated. The first `make check` was killed by host
memory pressure; every target passed afterwards, but not in one uninterrupted
run. The line-mode CLI setup prompts still take typed words.
[Commands](../evidence/onboarding-e03-20261008/commands.md),
[results](../evidence/onboarding-e03-20261008/results.json).

### E04 — complete (2026-10-08)

Base `e292743`. F08: `tc.JoinPolicy` takes the joiner's connection from the
invitation (the inviter's operator; its profile is installed from the
invitation after review). A `network_restart` phase replaces the
`SERVICE_UNAVAILABLE`/"no such file" failures. F07: `INVITATION_INCOMPLETE`
covers cut-short or damaged codes. F11: on approval the inviter names the device
with the joiner's label, with non-printable characters removed. F16 is
documented. A fresh joiner accepting every default joined over a local
fixture. `make check` found five in-process WAN tests broken by the first
restart condition; that was fixed, and every target and the WAN/onboarding
subset pass. Limitations: private-CA self-hosted operators need trust roots
first; the wait screen cannot name the inviter.
[Commands](../evidence/onboarding-e04-20261008/commands.md),
[results](../evidence/onboarding-e04-20261008/results.json).

### E05 — complete (2026-10-08)

Base `176da46`. F05: the invitation panel shows a character count, never the
code. `c` sends a bounded OSC 52 copy; `v` prints one unbroken line outside
the panel, then clears the screen and scrollback; `s` saves a private default
file and shows the scp/join commands; the join prompt accepts a code or a
path. A real PTY run at 80 and 200 columns reconstructs the exact code. A full,
uninterrupted `make check` and all PTY suites pass.
[Commands](../evidence/onboarding-e05-20261008/commands.md),
[results](../evidence/onboarding-e05-20261008/results.json).

### E07 — complete (2026-10-08)

Base `8b5a088`. F10: approval status checks are spaced 30–36 s apart (two
requests each against the inviter's 5/min bucket), and `RATE_LIMITED` during a
join is a paused `slowed_down` state. A 30-minute two-daemon wait against
production limits had no refusal; approval was noticed after 8 s. Fixtures
that sleep a fixed 25 s set `control.Options.ApprovalPoll` to 15 s. Every target
and all PTY suites pass, but not in one uninterrupted run after the fixture
change.
[Commands](../evidence/onboarding-e07-20261008/commands.md),
[results](../evidence/onboarding-e07-20261008/results.json).

### E09 — complete (2026-10-08)

Base `4f8ea36`. EG4 closed: the relay counter is egress payload in both
directions, with 0.22% framing overhead. `relay_month_bytes` (2 TiB default)
lives in a private state file under a new `StateDirectory`; it survives
restarts and resets monthly. At the budget, data relays are refused with
`RELAY_BUDGET` and live ones end at the next chunk; pairing relays are exempt.
Metrics, 80%/100% alerts, device wording and the route share are added.
`make check`: `TestWANW12BinaryDoctorPrivacyAndPTY` fails intermittently
(relay probe `TIMEOUT`), and that reproduced on unmodified `4f8ea36`. All other
targets, PTY suites and the orbit-net rehearsal pass. The VPS deployment and its
`relay_month_bytes` value need the owner's confirmation at E10.
[Commands](../evidence/onboarding-e09-20261008/commands.md),
[results](../evidence/onboarding-e09-20261008/results.json).

### E06 — complete (2026-10-08)

Code `0da2e7d`. EG1 closed: CPaceRistretto255/SHA-512 (draft-21) on
`gtank/ristretto255`, published vectors pass. `XXXX-XXXX` codes allow one guess
per code (2^-20). Mailboxes live in memory: 128 total, 4 per key, 10 minutes,
burned by a wrong code, lost on restart. The service sees only public PAKE
messages and invitation ciphertext; the capture test finds neither the code
nor the invitation. `orbit devices invite --code` prefers the short code, and
`--long` keeps the long one. The TUI and `orbit join` accept either, and
approval is unchanged. The profile privacy text stays accurate. Fixed in
acceptance: a fresh headless host with lingering proposed manual startup
because systemctl prints an empty line for a missing unit. Unexecuted: a
short-code join between physical devices through the hosted service (owner trial).
[Evidence](../evidence/onboarding-e06-20261008/summary.md).

### E08 — complete (2026-10-08)

Code `0da2e7d`. EG2 closed (seven per-device states, other devices only as
aged reports). `files`/`file_details` queries back both the Files view (`5`)
and `orbit files`. Configured devices with no attention land on Files. Model
tests cover navigation, 10,000-entry paging, search, unsafe names and every
state, and a real PTY run checks CLI/TUI parity, `$EDITOR` and resize.
Unexecuted: `xdg-open` on a real desktop session.
[Evidence](../evidence/onboarding-e08-20261008/summary.md).

### E10 — complete (2026-10-08)

Orbit 2.1.0 and orbit-net 1.1.0. Uncached validation: `tests/terminal` (full,
2036 s), the rest of `make check` (packages, integration, model, faults,
harness, builds), `test-race-core`, all four PTY suites, `make demo` and the
orbit-net rehearsal pass. One race-mode timing failure
(`TestWANW15RejectedAnnouncementKeepsAcceptedOfferGeneration`, 15 s budget
under full parallel load) passed 6/6 alone and in a full `internal/network`
race rerun. Fixes made in E10: PTY/W16 harnesses for the Files landing,
`--code`/`--long` and the reworded join prompt. The packaging tests and
`terminal_package_test.py` were still pinned to 2.0.0 (the latter had passed
against stale 2.0.0 artifacts, which are now archived). Also fixed: the package
build date, and help for `--code`/`--long`. Trial finding fixed: a device that
is only switched off no longer produces one EXHAUSTED_WORK item per periodic
sync (the owner's PC showed 50). The W12 relay-probe timeout did not recur in
three logged full terminal runs after relay probes got the 10 s handshake
bound; a fourth run exited 1 with its log lost, cause unknown. Deployed with
the owner's confirmation: orbit-net 1.1.0 on the VPS with `relay_month_bytes`
2 TiB and a `StateDirectory` drop-in. Installed 2.1.0 on PC, laptop and Pi and
removed the port drop-ins. [Evidence](../evidence/onboarding-e10-20261008/summary.md).

## Handoff

**2.2.0 trial polish (2026-10-08, uncommitted to hosts).** Owner requests from
the 2.1.0 trial are implemented and committed but **not yet installed**: the
"Orbits" list with a live file preview (TUI wording only), live status on the
invitation page with Enter to review the waiting request, inviter and folder
names in invitations (2.1 cannot read them; update all devices together), the
proposed join folder name and `~/<name>` root, and the join "reconnecting"
wait. Validation run: build, vet, `internal/terminal`, `internal/control`,
`cmd/orbit`, and all four PTY suites pass. Not run: full `tests/terminal`,
integration/packaging tests, race. Version is 2.2.0; packages are not built.

Trial state: PC, laptop and Pi share `Trial` on 2.1.0 (Pi unattended with
lingering). The owner wants fast iteration: next, a quick check target and a
disposable-trial reset for the three hosts, so a full suite is not needed per
change.

Remove `overlap_profile`/`overlap_service_key` from the VPS `serve.json` after
2027-01-04.
