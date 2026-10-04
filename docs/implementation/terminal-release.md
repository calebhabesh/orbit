# Terminal packets T12–T13: delivery and release evidence

Use the [plan](../orbit-terminal-implementation-plan.md),
[architecture](../orbit-terminal-architecture.md),
[UX](../orbit-terminal-ux.md), and [tracker](terminal-status.md).
These are future acceptance requirements. Earlier browser screenshots and
scripted engine campaigns do not demonstrate ordinary terminal onboarding.

## T12 — Packaging and compatible entry

Dependencies: T10, T11. Owner: assigned worker for binaries/service/migration/
integration, runbooks, help and completions. Read: TG5, operations lifecycle/
upgrade/uninstall, persistence identity recovery and legacy adoption, current
package scripts and O14 relocation behavior. Invariants: I07–I08, I19–I22, I27.

Required outcomes:

- Make bare `orbit` launch the TUI in a TTY and concise status in non-TTY use.
  Keep explicit CLI commands independent, with a clear direct daemon/service
  path. A desktop entry, if retained, opens a terminal rather than requiring
  a new browser session for ordinary Orbit use.
- Preserve `filesync` engine commands, existing services, roots, state locations,
  identity/wire/version fixtures and scratch names. Freeze any explicit legacy
  browser launch path and document its status; existing data never depends on
  removing browser assets. Ordinary terminal packages/builds need no new GUI
  runtime or browser installation.
- Adopt legacy state/services explicitly and exactly once. Missing limits,
  unsupported schemas, older peers and partially completed migration return
  accurate next actions. A restart cannot reuse rolled-back author counters.
  Preserve O14 relocation and cross-filesystem retained-source behavior.
- Deliver amd64/arm64 archives and the repository's existing distribution package
  formats with checksums, dependency licenses, service/entry files and completions.
  Exercise binaries extracted from those packages; cross-compilation alone
  cannot claim native execution.
- Replace ordinary-use documentation with CLI/TUI create/join/share/status/
  conflict/restore instructions. Document LAN and Tailscale prerequisites,
  reachable addresses/ports, separate login/unattended modes, safe upgrades,
  backups/rekey recovery, and uninstall preserving files/state. Secrets appear
  through private input, not example argv or automatically exported transcripts.
- Deliver/integrate the terminal/PTY runner in standard relevant checks, including
  `cmd/filesync` tests currently omitted by `make test`. Keep any retained browser
  compatibility checks explicit; terminal release completion uses terminal journeys.

Checks: planned `TestTerminalT12` binary/entry/migration/legacy cases; package
install/upgrade/uninstall in new test environments; existing and newer schema
refusal, mixed capability negotiation, duplicate-daemon avoidance and real
service startup. Run `make check`, `make test-race`, `make demo`, native/cross
builds and `make package`. Record package-extracted CLI/TUI operation and checksums.
Native laptop/Pi/VPS systemd/logout/boot tests belong to T13 if not run here;
keep them explicitly unexecuted. Explain binary rollback versus old metadata
restore and how the latter changes identity rather than resetting counters.

## T13 — Release campaign and owner use

Dependencies: all T00–T12 complete. Owner: assigned worker for campaign/evidence/integration;
the owner performs actual personal use and unaided explanation. Read:
verification's full invariant/scenario matrix, persistence fault model,
terminal acceptance and existing safety-validated release harnesses.
Change: production-interface terminal/native campaign, sanitized evidence,
final tracker/runbooks/case study and bounded fixes through owning modules.
Invariants: reconcile every applicable I01–I28, with explicit evidence or limitations.

Required outcomes:

- Execute clean-checkout/package-extracted local and native journeys. At least
  two ordinary installations create/join/approve through the planned interface;
  the harness cannot substitute manual certificates, membership files, IDs,
  or endpoint edits for the product's normal workflow.
- Validate three-host laptop/Pi/VPS normal edits, offline/reconnect, third-peer
  forwarding, same-device second-folder sharing, qualified receipts/status,
  retirement/replacement and existing-folder preservation. Validate a reachable
  LAN path and an existing Tailscale path, with recorded network assumptions.
- Exercise invitation identity/scope/replay/authorization negatives, delayed
  approval, process restarts at onboarding boundaries, fork/offline rollout,
  live-daemon CLI use and root/limits initialization. Verify actual keys,
  memberships, head sets and bytes rather than only screen transcripts.
- Run applicable process/IO faults and release regressions: interrupted verified
  transfer, stale conflict/editor review, GC/read/restore races, expired/missing/
  corrupt history, root changes/incomplete scan, full storage, replay/pruning,
  relocation, and migration/recovery fencing. Add abrupt-reset experiments
  when changed durability mechanisms require them; retain declared failure limits.
- Run keyboard-only real PTY paths for setup and everyday management, narrow/
  colorless terminals, resizing, external editor return, piped/JSON CLI and
  daemon continuation after quitting. Native service tests separately establish
  start-at-login and explicitly configured unattended logout/boot persistence.
- Measure bounded terminal/query/merge memory and descriptors on large-directory
  and large-file fixtures, plus continued large-file progress under small edits.
  Report measured costs and sample counts; no speedup target or unlimited-scale
  claim substitutes for results.
- Record actual owner use on an intentionally selected personal sync root:
  ordinary edits, offline/reconnect, restart, conflict/restore and observed
  usability with actual times/results. Preserve existing P17 pilot data and
  distinguish new terminal use from historical scripted or browser sessions.
- Record the owner's unaided explanation of causal conflicts, durable capture,
  membership/retirement, conditional restore, recovery and evidence limits.
  Finalize the resumable release report and case study with supported claims.

Checks: discover/run planned `TestTerminalT13` and the delivered CLI/PTY/native
journey runners; `make check`, `make test-race`, `make demo`, package/checksum
and targeted campaign commands. Reuse safety validation before each destructive
worker. Existing three-host scripts can check engine invariants, but adapt or
add ordinary terminal onboarding rather than claiming their manual setup proves it.

Completion requires all automatic criteria and actual owner evidence. If owner
use/explanation or a required native scenario is unavailable, keep T13 in
progress with the remaining named step and recorded limitation. Finish every
independent authorized check; do not call the entire redesign complete because
the automatic suite passes. P17 closes only when its remaining criteria have
their own genuine evidence, linked from both trackers.

## Release evidence table

Populate as checks actually run; this table defines the required observations.

| Campaign | Required observations |
| --- | --- |
| Ordinary two-device onboarding | Inputs, approval, verified identities, file hashes, phased readiness and restart/resume |
| LAN/Tailscale and three hosts | Actual addresses/network route, contact/freshness, forwarding author/head equivalence and denied unshared folder |
| Enrollment/rollout negatives | Identity/scope/replay rejection, bounded resource behavior, fork/mismatch data gates |
| Conflict/history/restore | Reviewed tokens, stale review, exact exported/restored bytes and unavailable payload reasons |
| Failure/recovery | Named injection boundaries, protected hashes, journal outcome, cleanup and declared fault model |
| TUI/CLI and service lifecycle | Real PTY/JSON output, terminal restoration, continued daemon edits, startup/logout/boot results |
| Legacy/package adoption | Package provenance/checksums, one daemon, preserved IDs/history/roots and deliberate migration/refusal |
| Personal use/explanation | Owner's actual start/end observations and unaided answers, separate from agent-run campaigns |
