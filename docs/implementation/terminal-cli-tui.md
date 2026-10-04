# Terminal packets T06–T11: everyday CLI and small TUI

Use the [plan](../orbit-terminal-implementation-plan.md),
[architecture](../orbit-terminal-architecture.md),
[UX](../orbit-terminal-ux.md), and [tracker](terminal-status.md).
All validation here is planned. Proposed `TestTerminalTXX` groups must exist
and match nonzero tests before a run can count as evidence. Either implementation
worker owns the production behavior, integration and validation of its assigned
packet. Architectural responsibilities belong to modules, not worker models.

## T06 — Commands and context

Dependencies: T02, T04, T05. Owner: assigned worker for commands, context,
help, output and completions. Read: T01 schemas, UX command map,
protocol path rules, operations CLI/replay and compatibility. Change: modular
command adapters and shared name/context resolution; avoid enlarging main.go
with a second copy of every controller. Invariants: I09, I16, I19–I20, I27.

Required outcomes:

- Implement the agreed named command catalog with coherent grouped help,
  guided create/join/share, status, device/folder management, context inspection,
  diagnostics and discoverable advanced controls. Preserve deliberate aliases.
  History/conflict/restore semantics are completed in T08.
- Resolve human names and current-directory context to existing identities.
  Infer a relative file command from its registered root; outside a root use
  an explicit folder or picker. Duplicate names, no match and a changed root
  registration produce clear disambiguation/stale-context results.
- Convert shell-relative input into a validated root-relative target, while
  keeping protocol path checks and descriptor-rooted IO in their owning module.
  Symlink/state/overlap/traversal input cannot escape authorization; a name
  lookup or lexical clean is never a filesystem safety check.
- Human output describes actual phases and safe next commands; JSON output
  has frozen machine fields and no control sequences/prompts. Freeze stdout,
  stderr, exit categories and any wait/check behavior in T01's contract.
- Non-TTY mutations use explicit reviewed inputs. Secrets enter through prompts,
  stdin or private files; help/completions/logs do not repeat them. Escape control
  characters in user-controlled names and filenames. Support `--` for literal
  leading-dash paths and arguments with spaces.
- Cover live and supported stopped adapters through the same operations. A
  successful CLI mutation must have the same idempotency and reviewed-state
  semantics as its TUI counterpart.
- Retire duplicated command-local live/stopped transport helpers after all
  affected callers use the shared client and compatibility tests pass. Retained
  aliases dispatch into the same operations rather than preserving a second
  engine-facing implementation.

Checks: planned `TestTerminalT06` actual command tests for names/cwd/duplicate
names/relocated roots, Unicode/control characters and literal paths; pipes,
JSON/errors, secret handling and live/stopped equivalence. Explicitly run
`go test ./cmd/filesync/...`; run relevant control/integration/race checks.
Completion includes an executable help/example/completion walkthrough. Explain
how a convenient context lookup differs from authorization and path safety.

## T07 — Status, attention and diagnostics

Dependencies: T02, T06. Owner: assigned worker for aggregate queries/errors
and human status/attention renderers. Read: protocol receipts/status, persistence
capture/content readiness, operations observability/storage and UX labels.
Change: bounded aggregate/attention control queries, persisted observations and
CLI status/doctor adapters. Invariants: I05, I11, I13–I14, I18–I20, I22, I27–I28.

Required outcomes:

- `orbit status` reports named folders, local capture/pending work, attention,
  observed device-copy state/freshness and daemon/startup facts. Preserve working
  bytes versus durable capture, direct versus indirect reports, stored versus
  applied, and historical receipt versus current availability.
- Offline, stale, unknown, membership-behind and content-pending observations
  remain visible. A VPS receipt cannot become a final-device receipt. Report
  successful unrelated work alongside blocked paths; percentages use real totals.
- Persist/query actionable conflicts, root/unsupported-path issues, failed work,
  integrity/storage blocks, incomplete setup and membership problems across client
  close/restart. Safe lifecycle pruning cannot erase pending attention.
- Provide bounded pages/detail queries and cancellation; large folders do not
  become complete in-memory UI inventories. Status inspection does not scan,
  perform GC or alter membership as a hidden read side effect.
- Doctor distinguishes daemon/local-control trouble, network reachability,
  TLS identity, folder approval/revision, root availability and finite-limit
  state. It proposes safe corrective commands and reports unavailable network/
  systemd tooling without silently changing host settings.

Checks: planned `TestTerminalT07` mixed-state process/integration fixtures;
offline/stale/indirect peer reports, uncaptured edit, failed scan, storage and
integrity blocks, membership lag, restarted pending tasks and large paginated
results. Verify displayed fields against repository/control observations, with
no false Ready/global-synchronized claim. Run relevant scheduler/GC/race checks.
Explain what an offline device's last stored receipt actually establishes.

## T08 — Conflicts, history and restore

Dependencies: T06, T07. Owner: assigned worker. Presentation uses frozen results
after the operation contract is ready. Read: TG4, protocol reviewed resolution/restore,
persistence publication/read leases/GC, operations content limits and UX recovery.
Change: streamed live control adapters, durable reviewed editor sessions,
history/deleted/export/restore commands and exact-version copy operation.
Invariants: I03–I07, I10–I13, I16, I18–I20, I25–I28.

Required outcomes:

- List/show competing versions and availability; implement choose one, keep
  copies and manual merge through exact reviewed head-set tokens and idempotent
  operations. New versions invalidate review; never resolve all currently known
  heads merely because explicit reviewed inputs were omitted.
- Export exact verified versions with bounded buffers and content protection.
  Implement editor/diff argv parsing, subprocess invocation and session recovery
  in the owning adapter.
  Invoke tools directly, retain review/result paths privately, and bind any
  edited output to the original review before a commit preview.
- Stream merge input through budgeted admission rather than reading a complete
  file into CLI memory or unbounded JSON text. Interruption keeps protected
  captured bytes and reports durable partial work accurately.
- History and Deleted files expose actual candidates and content states.
  Missing/expired/corrupt bytes disable unavailable actions; a peer recovery
  option appears only if its fetch/verification path is implemented and tested.
- Restore defaults to the reviewed original path with replacement preview and
  protection of current working bytes. Recovering a separate copy has an explicit
  destination/collision plan; writing within a synced root uses owning mutations,
  while external recovery uses the exact-version export operation.
- Preserve original path-based history and reviewed ancestry. No timestamp
  winner, semantic auto-merge, fixed post-deletion timer, or cross-path atomic
  promise is added. Read/editor/session cleanup obeys pins and replay policy.

Checks: planned `TestTerminalT08` live-daemon CLI operations: concurrent file/
delete and structural conflicts, all resolution choices, replay/collision,
new arrival during editor review, configured tool exit/crash, large streamed
merge, GC racing reads, corruption and available/pending/expired restore.
Assert exact head sets, content hashes and preserved current bytes. Use a
deterministic editor adapter plus real tool/PTY smoke coverage later in T11/T13.
Run relevant model/workspace/control/race tests. Explain restored provenance
versus current-head ancestry and why a file timestamp cannot choose a winner.

## T09 — TUI shell and terminal lifetime

Dependencies: T01, T02, T06. Owner: assigned worker for library/client/PTY
integration and render/navigation modules. Read: TG5, UX keyboard/layout,
typed fixtures and shared client cancellation/error rules. Change: terminal
client module, event/render loop, PTY runner and opt-in TUI entry during development.
Invariants: I13, I19–I21, I27.

Required outcomes:

- Pin compatible stable Bubble Tea v2, Bubbles v2 and Lip Gloss v2 releases,
  following the [stack decision](../orbit-terminal-architecture.md#tui-stack-decision--2026-10-03).
  Verify official APIs/licensing and run a small amd64/arm64/PTY spike. Freeze
  the terminal/client/tool adapters before implementing dependent screens; keep
  library types within `internal/terminal` and use one input/render owner.
- Run control I/O through cancellable commands returning correlated messages;
  background work never mutates view state. Focused text inputs receive ordinary
  text before global shortcuts. Use direct argv and Bubble Tea's ExecProcess for
  interactive tools, with session/review safety owned by the shared controls.
- Render overview/navigation/focus and context actions with injected typed
  queries. Implement j/k and arrows, Tab, Enter, Esc, search and help; text
  entry stays ordinary. Plain/colorless terminals retain meaningful state.
- Bound refresh/event/page work and preserve selection, focus and draft inputs.
  Correlate asynchronous responses to their selected context and operation;
  stale responses cannot apply to another folder or an abandoned form.
- Handle TTY/non-TTY, resize, signal/exit, external-tool suspend/resume, Unicode,
  long names and display escaping. Quit restores the terminal and leaves the
  daemon running. Explicit cancellation/daemon stop remain different actions.
- Deliver a documented PTY test command that exercises the real binary, using
  new private roots/processes and sanitized transcripts. Snapshot tests cover
  view logic but cannot substitute for process/control assertions.

Checks: planned `TestTerminalT09` library adapter/view/key tests and real PTY
scenarios: 80x24, narrower terminal, resize/paste, lost/reconnected daemon,
colorless output, stdout pipe, Ctrl-C/quit, terminal restoration and continued
file capture after client exit. Build both supported architectures and run
race/cancellation checks. Ordinary bare entry switches in T12; development uses
an explicit TUI command until real journeys are ready. Explain client versus
daemon cancellation and how late responses are safely discarded.

## T10 — TUI onboarding and device management

Dependencies: T04, T05, T07, T09. Owner: assigned worker for forms/screens and
integration. Read: UX create/join/share/setup review, frozen fixtures and
operation phases, TG1/TG2/TG5. Change: exclusively assigned setup, invitation,
approval, root-review, device/folder/startup screens. Invariants: I09, I11,
I13, I19, I21–I24, I27.

Required outcomes:

- First use offers Create/Join; opening an unfinished setup resumes the actual
  operation. Keep name/root/network/startup/finite settings on a concise review
  with clear existing-content adoption and Advanced tuning.
- Add device creates an invitation for selected folder participation. Join accepts
  it privately, shows inviter/request verification information, and guides local
  root review. Approval is for the exact request; additional-folder sharing
  reuses identity while preserving separate consent.
- Render waiting/approved/membership/scanning/download/local-ready phases from
  authoritative results. Keep entered values through errors; provide next
  actions for unreachable/expired/wrong identity/storage/root/stale states.
- The UI can close during pending approval/work and reopen after daemon restart.
  Other offline devices remain pending; no success animation or progress formula
  overrides an incomplete controller observation.
- Device/folder detail exposes last contact, sharing, pause/resume and existing
  relocation controls. Retire/unregister previews explain the actual scope and
  preserve existing conservative procedures; they do not imply remote erasure.

Checks: planned `TestTerminalT10` view/key fixtures plus two-process PTY journeys
using actual operations, approved identities and verified files. Cover nonempty
root review, validation/back/edit, delayed approval, close/reopen/restart,
expired invitation, second folder, fork/membership lag and startup errors.
Record sanitized transcripts and relevant regression results. Completion needs
integrated acceptance evidence, not a fixture-only walkthrough. Explain the
one-paste/one-approval journey and how each phase relates to actual engine state.

## T11 — TUI everyday management

Dependencies: T07, T08, T09. Owner: assigned worker for screens and content/
editor/recovery integration. Read: status/conflict/history/restore UX and bounded typed results.
Change: assigned overview/attention/device-copy/history/settings/review screens.
Invariants: I03–I07, I11, I13–I14, I16, I18–I20, I25–I28.

Required outcomes:

- Land on persistent attention, named folders and compact devices. Show saved/
  stored/applied/freshness separately; paginated path search supports history and
  attention selection without implementing another general file manager.
- Inspect conflict versions, launch configured external diff/editor through
  the shared adapter, return to review, and commit only the reviewed result. Keep
  copies/select/manual merge and stale-review correction remain keyboard usable.
- Offer history/deleted candidates, explicit replacement preview and separate
  copy recovery. Explain unavailable content and retention conditions; leave
  unsupported peer-fetch options unavailable with a reason.
- Present storage usage/budgets, safe maintenance previews, root problems,
  recovery/retirement runbooks and operation progress from existing controls.
  Reads do not trigger cleanup; partial work remains visible after refresh/restart.
- Preserve focus/selection/drafts during polling, errors and external-tool return.
  Shortcut help and narrow/colorless fallback cover every essential action.

Checks: planned `TestTerminalT11` real PTY edit/offline/reconnect/conflict/restore
and storage/root-block workflows; external editor exit, stale arrival during
review, a missing payload, background work after quit and relocation regression.
Assert actual files/heads/service effects alongside rendered state. Verify full
CLI/TUI equivalence and race/resource results. Completion establishes M3;
actual owner usability remains a T13 requirement, not simulated owner evidence.
Explain how the overview communicates confidence without a global green check.
