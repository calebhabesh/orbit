# Onboarding and everyday-use packets

Read the [plan](../orbit-onboarding-implementation-plan.md) and the
[tracker](onboarding-status.md) first. Finding IDs (F01–F16) refer to the
[trial findings](onboarding-status.md#trial-findings). Source locations are dated
2026-10-08 at `58af25e` and must be re-checked before editing.

## E00 — Baseline and reproductions

Dependencies: existing repository. Change: tests, fixtures and the tracker only.

Required work:

- Record revision, Go version, host facts and the installed 2.0.0 package checksum.
- Reproduce each finding F01–F16 through production interfaces on disposable state
  (isolated `HOME`/`XDG_*`, a test user service manager or the existing PTY harness).
  A finding that cannot be reproduced is recorded as such with the attempt, not fixed blind.
- For F10, determine the actual cause before E07: which client loop hits which
  `orbit-net` limit (the operator guide lists 1 metadata op/s per device, burst 10).
  Use a local `orbit-net` fixture, never the deployed VPS.
- Scope EG1–EG4: list candidate designs and the evidence each gate needs.

Acceptance evidence: one failing (or documented non-reproducible) test or transcript
per finding, committed and referenced from the tracker; gate scoping recorded.

## E01 — Daemon lifecycle and service defaults

Dependencies: E00. Change: `packaging/systemd/orbit.service`, deb/tarball packaging,
`internal/launcher`, service control operations, status, setup startup choice.
Gate: EG3.

Required work:

- **F01:** ship the user unit with `--control-listen=127.0.0.1:0`. The CLI/TUI
  already read `control.addr`; verify every client path does. Packaging upgrades
  must replace the old unit; a local drop-in that sets the same value stays harmless.
- **One daemon owner (F04, F14):** when a user systemd manager is available, the
  TUI/CLI start Orbit through `orbit.service` instead of spawning a detached
  `serve`, so logs go to the journal and `orbit service stop/start` always work.
  Without systemd, keep the detached daemon and say so. If a detached daemon is
  already running, `orbit service start` offers to hand over (stop it, start the
  unit) rather than failing with `MANUAL_DAEMON_RUNNING`.
- **Truthful status (F04):** the header reports who runs the daemon (`service`,
  `terminal`, `manual`) separately from the configured startup mode, and shows a
  failing/restarting unit as such.
- **Startup default (F12, EG3):** default `login` on machines with a graphical or
  interactive login seat and `unattended` on headless hosts. Before offering
  `unattended`, check lingering; when it is off, show the exact
  `sudo loginctl enable-linger <user>` command and a re-check action. Orbit never
  runs sudo itself. If lingering stays off, fall back to `login` and say so.
- Update [operations](../operations.md) and the terminal architecture for the
  ownership rule.

Acceptance evidence:

- With another process holding 127.0.0.1:8080, a fresh install's service starts,
  `orbit status` connects, and the journal holds daemon output.
- Start from TUI, then `orbit service stop`, `start`, `restart`: each succeeds and
  status names the owner correctly at each step; a pre-existing detached daemon is
  handed over without losing committed work (existing restart invariants hold).
- Headless and desktop fixtures pick the documented defaults; linger-off shows the
  command and never escalates privileges.

## E02 — Self-healing attention and runnable actions

Dependencies: E00. Change: durable work scheduling, attention projection,
control operations, `internal/terminal/app.go` attention routing, CLI help text.

Required work:

- **F02:** a scan task exhausted with `ROOT_UNAVAILABLE` (or any root/capture
  precondition) is superseded when a later scan of the same folder completes; its
  attention item clears automatically and the history records why.
- **F03:** add a control operation to retry an exhausted task while the daemon
  runs; the CLI command and the TUI key both call it. Attention actions only name
  commands that work in the daemon's current state; `orbit engine …` advice is
  replaced by the control-backed command.
- **F09:** route Enter by attention code first (`AWAITING_APPROVAL` → requests
  review, `EXHAUSTED_WORK` → retry review, conflicts → review) and only then fall
  back to the generic operation screen. Add a table-driven test that covers every
  attention code the daemon can emit, so a new code cannot reach the wrong screen.
- **F13:** replace the generic "Retry; use orbit doctor…" action with a specific
  next step per error code; unknown errors say what failed and where its details
  are (journal or `orbit doctor`).

Acceptance evidence: root removed and restored during a scan clears its warning
without user action; retry works with the daemon running from CLI and TUI; Enter
on each attention code opens its screen in model and PTY tests; no displayed
action fails with "state directory is already owned by another agent".

## E03 — Keyboard, form and paste conventions

Dependencies: E00. Change: `internal/terminal` key handling, fields, help, footer.

Required work:

- **Convention** (record in the terminal UX): ↑/↓ and `j`/`k` move rows **and**
  form fields; Tab/Shift+Tab move between fields or panes (not into search);
  ←/→ change a selector and move through the Files tree; Enter selects, advances
  to the next field and confirms on the last; Esc goes back; `/` searches;
  `?` opens help; `q` quits outside text fields; Ctrl+C quits anywhere. Number keys
  `1`–`5` switch views in tab-bar order in addition to existing letters.
- Replace typed enumerations (startup, connection, any yes/no) with selectors
  prefilled with E01/E04 defaults; show the choices inline (`‹ login ›`).
- **F06:** a paste into a secret field replaces its content; the field shows a
  length/status line (`1,666 characters received`) without revealing the secret;
  Ctrl+U clears it. A pasted line break submits nothing by itself.
- **F15:** review-file and invitation-file errors name the file's directory and
  the fix (`create it with mkdir -m 700`, or use the suggested private default),
  instead of calling it a "state directory".
- Footers list only keys valid on the current screen.

Acceptance evidence: PTY tests drive each setup, join and approval form with
arrows only, Tab only, and Enter only; bracketed and unbracketed pastes into the
invitation field produce exactly the pasted value on repeat attempts.

## E04 — Join and setup defaults; actionable onboarding errors

Dependencies: E01, E03. Change: join/setup workflows, control enrollment,
network policy review, device listing.

Required work:

- **F08:** a fresh join offers Automatic connection by default, with the same
  operator/privacy review as first-device setup, before submitting. A routed
  (v3) invitation preselects the inviter's operator; Local-only remains a choice
  and explains that it needs a long invitation over LAN/Tailscale.
- Prefill device name (hostname), folder (`~/Orbit`, empty), startup (E01 default)
  and storage budget; one review screen; Enter confirms.
- **F07:** invitation errors are specific: incomplete/damaged code ("copy the
  whole line again"), expired, wrong operator/profile, newer version. No
  invitation failure maps to `INVALID_REQUEST` with generic advice.
- **F11:** the inviter's device list and approvals show the joining device's
  chosen name after approval, not `Device <id>`.
- **F16:** document that received files use safe local permissions (owner-only;
  executable bit synchronized) as specified in [persistence](../persistence.md);
  show it once in setup help. No behavior change.
- Join progress names what it is waiting for (approval on *device name*) and the
  verification code to compare.

Acceptance evidence: fresh disposable device joins a fresh inviter over a local
service fixture by accepting every default; each invitation failure class shows
its specific message; device names propagate in CLI, JSON and TUI.

## E05 — Long invitation display, copy and file transfer

Dependencies: E03. Change: `internal/terminal` invitation screens, CLI invite.

Required work:

- **F05:** never render an invitation inside a truncating panel. Show it as an
  unboxed, wrapped block that copies as one line (no border characters, no
  ellipsis), with its character count.
- `c` copies it with OSC 52 when the terminal supports it and reports success or
  "copy not supported here"; `s` saves it to a private default file and shows the
  `scp`/join path to use.
- The join prompt states it accepts a code **or** a file path; a file path is the
  documented fallback for SSH sessions.

Acceptance evidence: PTY capture at 80 and 200 columns reconstructs the exact code;
OSC 52 sequence emitted and bounded; saved file is 0600 in a 0700 directory and
accepted by join.

## E06 — Short pairing code through the Orbit service

Dependencies: E04, E05; EG1 closed. Change: `internal/rendezvous` and `cmd/orbit-net`
(mailbox), network protocol, enrollment client/control, TUI/CLI invite and join,
schemas, operator guide, privacy text if EG1 requires it.

Proposed design for EG1 to confirm or revise:

- Code: eight Crockford base32 characters shown `XXXX-XXXX` (40 bits). The first
  group is the mailbox name; the second is the PAKE password. Input ignores case,
  spaces and dashes and maps `O→0`, `I/L→1`.
- Inviter creates the ordinary routed v3 invitation, opens a mailbox on its
  operator's service authenticated by its device key, expiring with the code
  (10 minutes, renewable from the TUI).
- Joiner claims the mailbox once; both run a PAKE (CPace or SPAKE2 from a
  maintained Go implementation with test vectors) and key confirmation. A wrong
  password burns the mailbox: one guess per code, and the inviter is told a wrong
  code was tried. On success the invitation crosses encrypted under the PAKE key,
  then enrollment proceeds exactly as v3 (pins, request, verification code, owner
  approval).
- Service holds only mailbox name, two device keys, timing and ciphertext, in
  memory, deleted on claim, failure or expiry; per-key and per-source limits bound
  mailbox creation and claims. Privacy text and profile epoch change only if EG1
  finds the current text inaccurate; if so, plan the `orbit network update` review
  each device will see.
- Capability negotiation: older builds or Local-only devices get the long code.

Acceptance evidence: pairing by typed code with local service fixture; wrong code,
expired code, reused code, squatted/flooded mailboxes and service restart behave
as specified; synthetic service capture shows no readable invitation; mixed
versions fall back; [WAN protocol](../orbit-wan-protocol.md), WAN UX and operator
guide updated.

## E07 — Approval waiting within service limits

Dependencies: E00 diagnosis, E04. Change: join progress client loop, possibly
service event subscription, `orbit-net` limits only if E00 shows they are wrong.

Required work:

- **F10:** fix the cause E00 found: the inviter's enrollment per-source bucket
  (5/min, burst 5, `internal/replication/enrollment.go`) refuses the joiner's
  15 s status polls, which cost two POSTs each (8/min). `orbit-net` is not
  involved. Promote `TestOnboardingE00F10…` and the relay wait test. Expected direction: the joining device waits
  on an event or backs off (with jitter) instead of polling per second; a
  `RATE_LIMITED` response pauses the loop and is shown as "waiting; the service
  asked us to slow down", never as a blocking error.

Acceptance evidence: a joining device waits 30 minutes for approval against a local
fixture with production limits without a refusal; approval is noticed within the
documented latency.

## E08 — Read-only Files view and default landing

Dependencies: E02, E03; EG2 closed. Change: control query kinds over
`internal/repository/browse.go` (directory, search, file details, history),
`internal/terminal` view, CLI `orbit files [path]` with `--json`.

Required work:

- Control queries for directory pages, search and file details, used by both TUI
  and a new `orbit files` command (CLI and UI share operations). Bounded pages,
  cursors and the existing path validation.
- Tree view: folders at the top level when more than one is synced; columns name,
  size, modified, sync state. States limited to what EG2 proves: captured here,
  waiting to publish, downloading/missing content, conflict, blocked/unsupported,
  deleted. Never a global "synced everywhere" mark; other devices appear as
  qualified observations with freshness in the detail pane.
- Detail pane: last author device, versions, per-device observations.
- Actions: Enter/→ open directory; ← or Backspace up; `o` open with the desktop
  default (`xdg-open`); `e` open in `$EDITOR` with the TUI suspended; `h` history
  and restore; `c` conflict review; `y` copy path; `/` search. No rename, move or
  delete. `.orbit-internal` is never listed. Names pass through `safe()`.
- Default landing: Files when setup is complete and no attention item needs the
  owner; Overview otherwise, with a status line on Files showing the attention
  count, connection and devices.

Acceptance evidence: model and PTY tests for navigation, paging (10,000 entries),
search, unsafe names, each state, `$EDITOR` round-trip and landing rule; CLI and
TUI listings match for the same folder.

## E09 — Monthly relay egress budget and busy-relay UX

Dependencies: E00; EG4 closed. Change: `internal/rendezvous` relay accounting,
`cmd/orbit-net` config/state/alerts, operator guide, client route status.

Required work:

- Close EG4: establish whether `relay_bps` and `orbit_net_relay_bytes_total`
  count both directions, and what that means for VPS egress.
- Add `relay_month_bytes` (finite default; owner sets the deployed value against
  the 10 TB/month free egress) persisted to a private state file, reset at the
  UTC month boundary, surviving restarts. At the budget, new relay sessions are
  refused with reason `budget`; existing sessions end at the next chunk boundary.
- Metrics and `orbit-net alert`: month-to-date bytes, 80% and 100% transitions.
- Devices show "relay unavailable until <date>; direct connections still work"
  and keep trying direct routes. `orbit network status` reports relay versus direct
  share from local observations.
- Deployment to the VPS follows the operator runbook (backup binary/config, surgical
  changes, no Docker/Caddy/tunnel changes) and needs the owner's confirmation at
  E10 time.

Acceptance evidence: local fixture reaches budget, refuses, alerts, survives
restart and resets at a simulated month boundary; client wording verified.

## E10 — Integration, packaging, host migration and trial readiness

Dependencies: E00–E09. Change: version, packages, docs, host installs.

Required work:

- Bump the minor version, build the tarball and arm64 deb, run full uncached race,
  PTY, packaging and `make demo` checks.
- With owner confirmation, deploy E06/E09 service changes to the VPS per the runbook.
- Install the new package on PC, laptop and Pi. **Remove the 2026-10-08 workaround
  drop-ins** (`~/.config/systemd/user/orbit.service.d/control-port.conf` on PC and
  Pi) once the packaged unit carries the fix, then `systemctl --user daemon-reload`.
  Leave the Pi pilot daemon and the hosts' other services untouched.
- Rewrite [demo](../demo.md) as the TUI trial walkthrough for the new flow
  (create, short code, join with defaults, approve, Files view, conflict, history,
  WAN check), keeping the scripted campaign section.
- Scripted rehearsal on disposable state of the whole trial path, including a
  headless joiner with unattended startup.

Acceptance evidence: packages verified by checksum; rehearsal transcript; host
install record; workaround removal recorded; trial guide published in `docs/demo.md`.
