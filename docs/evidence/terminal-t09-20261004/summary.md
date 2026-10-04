# T09 — TUI shell and terminal lifetime

Complete for the explicit development shell and its terminal/client/tool
adapters. Run `bin/orbit tui --state /absolute/state`; keyboard navigation,
search, help and context inspection use the shared authenticated controls.
Bare entry remains T12. T10/T11 own workflow screens.

Prerequisites T01/T02/T06 are complete; T08 content/session safety stays in its
owning controls. Scope/glossary, terminal UX/architecture/plan, TG5, typed/shared
client contracts, protocol, persistence, operations and verification were read.
Existing P/O/T00–T08 and relocation work are preserved in this dirty tree.

## Implemented and verified

- Stable Bubble Tea 2.0.10, Bubbles 2.2.1 and Lip Gloss 2.0.6 pins, official
  release/API verification, module sums and distributed license notices.
  `make check` builds/packages static Linux amd64 and arm64 binaries.
- One input/render owner, injected typed queries, cancellable single query lane,
  correlated request/view generations, bounded pages, retained selection/focus/
  drafts, ordinary focused input before global j/k/q/? shortcuts, Unicode/control
  escaping, narrow layout and terminal-default styling/colorless state.
- Actual PTY keyboard, search/paste, resize, 80x24/40x16, arrows/Tab/Enter/Esc/help,
  colorless rendering, pipes and JSON without TUI escapes/prompts, direct argv
  interactive tools through `ExecProcess` and the shared result-size adapter.
- Actual successful scratch edits, failed-tool candidate preservation, canonical
  input while yielded, raw input after return, active-tool SIGTERM, q/Ctrl-C/
  SIGTERM exit and exact termios/alternate-screen/paste-mode restoration.
- Live owner unavailable while its state lock remains held, visible failure and
  retry/reconnection; no direct-state fallback bypass. Every signal validates
  the private disposable marker, canonical state, executable, argv and birth time.
- Protected original bytes, retained device identity and the same live daemon's
  actual new captured file digest after all TUI clients exit.

Seven ordinary `TestTerminalT09*` tests were discovered across the terminal module
and integration suite. The final run passed all seven **twice**, without skips
(21.942s). Scoped race passed with `GOFLAGS=-race` so built CLI/daemon children
were instrumented (35.527s). Three extra full PTY campaigns passed (32.022s),
as did final sanitized transcript capture (10.324s) and the documented
`make test-terminal-pty` command (10.308s).

Explicit CLI/control/client/repository regressions passed. `go mod verify`,
formatting, Python safety/VT tests and `git diff --check` passed.
**`make check` passed** (117.793s), including packaging on both architectures;
**full `make test-race` passed** (202.749s), without race warnings. Some unchanged
packages use Go's ordinary cache; the packet run and relevant explicit regressions
are uncached through `-count`. I13, I19–I21 and I27 have scoped interface/process
evidence, not a new universal resource or engine-safety proof.

[Commands](commands.md), [results](results.json), [manifest](manifest.json),
[environment](environment.json) and [final transcripts](transcripts/run-3/results.json)
retain exact argv, outcomes, provenance and assertions. The individual transcript
text files are sanitized synthetic sessions; differential render instructions
are interpreted by the independent VT cell oracle during assertions. Actual
files/history/process outcomes remain required alongside visible labels.

## Findings and corrections

The initial narrow layout hid selected rows; shorter navigation and visible
selection corrected it. Draft fixture mistakes included membership JSON key
spelling, shell-relative history paths and inspecting legacy identity while its
daemon still owned state. The fixture now uses correct typed fields/root context
and checks identity after graceful stop. Folder/device pagination was restricted
to its exact query kind to preserve aggregate-status compatibility.

A byte-substring PTY oracle failed when the renderer emitted only the changed
characters of a label. The VT cell oracle reconstructs the visible label and has
independent regression tests for character deletion, split UTF-8 and wide/
combining cells.

The initial repeated packet run recorded a SIGTERM timeout. The CLI and Bubble
Tea both handled signals; Bubble Tea's unbuffered quit send can race context
shutdown. The CLI now exclusively owns process signals, including active-tool
cancellation. An instrumented active-tool shutdown also timed out while the
harness stopped draining terminal output. Shutdown waits now drain the PTY as a
terminal emulator would and retain a validated-child stack diagnostic on timeout.
Subsequent repeated and instrumented campaigns pass. Both failed recorded runs
remain failures in `commands.jsonl` and their logs; final results do not erase them.

## Limits and handoff

Search is page-local. Application collections retain at most 20 rows each;
folder/device explicit-limit queries use live SQL keyset pages bound to selectors,
not mutation review snapshots. Attention retains T07's existing computation and
bounded response; this evidence does not establish constant-cost repository-wide
inspection. Terminal-library decoding occurs before application paste admission.
The application caps paste conversion at 4096 bytes and draft input at 256
characters, with one query lane and a five-second query deadline.

Tools are trusted owner programs. T09's explicit scratch-tool handoff never
commits a conflict resolution; T11 must use T08 admitted session paths, verified
exports, staging and fresh reviewed commits. T10 owns setup/join/approval/sharing
screens, and T11 owns qualified copy detail/conflict/history/restore/recovery.

Native laptop/Pi/VPS, physical LAN/Tailscale, boot/login/logout/unattended,
bare entry/compatible package adoption and physical reset/power-loss checks are
unexecuted in T09 and remain T12/T13. Cross-compilation does not establish native
arm64 runtime behavior. **P17 actual owner use and unaided explanation remain
outstanding.** No personal roots or existing VPS workload were fault targets.

Worker explanation: interface cancellation releases queries/tools and terminal
ownership; daemon work remains durably owned elsewhere. Canceling a committed
operation requires its explicit controller action. Request identity and view
generation gate every reply, so a late former-folder response cannot populate a
new selected context. This is not unaided owner evidence.

Next: **T10 — TUI onboarding and device management**; T11 is also eligible.
Reuse `terminal.Run`, the frozen typed query seam and bounded direct-argv tool
adapter with real T04/T05/T07 controls; keep review/session/commit safety in the
owning modules. [Shell runbook](../../runbooks/terminal-shell.md) describes entry,
keyboard behavior and reproducible PTY validation.
