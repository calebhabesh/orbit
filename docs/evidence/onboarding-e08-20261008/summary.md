# E08 summary — read-only Files view and default landing (2026-10-08)

**Complete; EG2 closed.** Code at `0da2e7d`. [Commands](commands.md),
[results](results.json), logs: [control](logs/control.log),
[model](logs/terminal-model.log), [PTY](logs/pty.log).

- **Queries:** `files` (directory page, folder-wide search, cursor, limit 1–200)
  and `file_details` (versions, last local scan, per-device reports with their
  time). TUI and `orbit files [path] [--search] [--json]` both use them.
- **States (EG2):** Saved here, Arriving, Downloading, Content missing,
  Conflict, Blocked, Deleted. Each is derived from this device's own records
  with the folder-readiness rules, at most one per entry. There is no global
  "synced everywhere" mark: other devices appear only in details, with the age
  of their last report.
- **View:** `5`/Tab. Enter/→ opens, ←/Backspace/Esc go up, `o` xdg-open,
  `e` `$EDITOR` with the interface suspended, `h` history, `c` conflicts,
  `D` deleted, `y` copies the path, `/` search, `]`/`[` pages. Nothing renames,
  moves or deletes. `.orbit-internal` is never listed, and names pass through
  `safe()`.
- **Landing:** Files when setup is complete and nothing needs attention, otherwise
  Overview. A screen the user has already chosen is never switched.

Evidence: each state is built through production capture/sync paths. An aged
report renders with its age. 10,000 entries page in 50 pages (about 0.2 s per
page). A real PTY run shows the TUI and `orbit files` listing the same entries,
walks the tree, round-trips `$EDITOR` and survives a resize.

Follow-ups found in E10: older PTY campaigns assumed an Overview landing and
now select Overview explicitly. Offline-peer sync failures no longer count as
attention, so a sleeping device does not keep the TUI off Files ([E10](../onboarding-e10-20261008/summary.md)).
