# E02 summary — self-healing attention and runnable actions (2026-10-08)

**Complete.** F02, F03, F09 and F13 are fixed. [Commands](commands.md), [results](results.json).

- **F02:** a completed full scan supersedes earlier exhausted scans of the
  folder. They are recorded as `completed`/`SUPERSEDED` and keep the original
  error, and the attention item clears without the owner doing anything.
- **F03:** `orbit retry --task ID | --all` (and `orbit engine work retry`) go
  through the running daemon's authenticated retry route, and a new
  `WorkChanged` hook wakes the running scheduler. Without it, the retried task
  stayed queued (negative check). Attention advice names `orbit retry` and
  what Enter does.
- **F09:** Enter routes by attention code through one table covering all 14
  codes the daemon emits. `EXHAUSTED_WORK` opens a retry review whose Enter
  calls control. A source-scanning test keeps the code list complete and
  rejects engine commands or nonexistent subcommands in actions. That check
  found `orbit conflicts resolve`, which does not exist; it is now `conflicts show`.
- **F13:** per-code error advice. Unknown codes show the daemon's action or
  where the details are.

Limitations: the real-PTY check covers `EXHAUSTED_WORK`; the other codes are
model-tested. Some doctor remediations still name engine commands. Outside the
Overview tab, the TUI header shows the daemon as `unknown` (noted, not in scope).
