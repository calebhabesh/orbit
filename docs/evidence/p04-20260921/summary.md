# P04 workspace and publication evidence

P04 adds a schema-v3 workspace projection/journal extension, root registration
with device/inode and private marker verification, descriptor-rooted Linux
scans and stable capture, explicit directory/scaffold handling, deletion
preview, verified staging, `RENAME_EXCHANGE`/`RENAME_NOREPLACE` publication,
and restart recovery. Minimal local CLI commands register, scan, inspect,
approve deletion proposals, and apply a supplied ready version. There is no
peer transport or automatic reconciliation in this packet.

## Process recovery matrix

The process harness killed a marked-disposable helper with SIGKILL at 30
production hooks. It reopened SQLite, ran journal recovery, verified protected
managed versions, retried unfinished operations, and checked that the next
scan did not author an application echo.

| Scenario | Named kill points | Restart result |
| --- | --- | --- |
| Existing-file exchange | prepared, stage flushed, staged, intent, before exchange, after exchange, recovery named, directory flushed, filesystem-published, SQL committed | Old or new visible bytes were valid; both captured local and ready remote objects rehashed; unfinished operations recovered or retried. |
| New file | prepared, stage flushed, staged, intent, before no-replace, after rename, directory flushed, filesystem-published, SQL committed | No partial file was reported applied; restart completed or safely retried. |
| Parent scaffold | after `mkdirat`, before parent flush/scaffold completion | Pending scaffold was reconciled before scan; no fabricated directory version. |
| Explicit directory / tombstone | prepared, intent, after filesystem action, filesystem-published, SQL committed for each kind | Empty-directory creation and empty-directory removal recovered or retried without recursive deletion. |

Unit tests also exercised in-place overwrite and save-by-rename during capture,
an editor save between preflight and exchange, writes through an old open
descriptor after exchange, parent-symlink swaps, unobserved local content,
root replacement/marker mismatch, hard links/FIFO/symlinks, incomplete
subtrees, executable-only edits, bootstrap absence, stale deletion tokens,
and small/large mass-deletion thresholds. An injected ENOSPC stage boundary
and a configured budget refusal left previously captured content intact.
The CLI integration test built a binary and exercised init → register → scan →
inspect across separate invocations.

## Filesystem and claim boundary

Executed locally on Linux 7.2.4, x86-64, ext4 (`rw,relatime`), Go 1.27.1,
modernc SQLite with WAL, foreign keys and `synchronous=FULL`. Fault tests
used disposable roots bearing the required marker. SIGKILL validates process
restart behavior, not abrupt-reset or physical power-loss durability. The
arm64 binary was cross-built, not executed for P04. No hosted CI, native arm64,
alternate filesystem, nested-mount injection, real-filesystem ENOSPC or VM
reset was run.

The supported automatic writer cases remain quiescent files, completed
save-by-rename, and in-place changes detected by the stable-read checks.
`RENAME_EXCHANGE` names the displaced inode, and an unexpected variant is
retained as `LATE_EDITOR_CANDIDATE`. A writer holding a descriptor across
replacement can keep changing that inode indefinitely; this is visible
recovery material, not a promise to capture every later write. A moved parent
descriptor can leave a retained variant under the moved directory; the
root-relative post-publication check blocks a false applied-basis report.

## Owner explanation

Equivalent histories mean replicas know the same causal versions and heads.
Their visible trees can still differ while a path is conflicted, structurally
blocked, or awaiting review. Forcing identical trees at that point would
silently select a winner or overwrite a local candidate.

See [commands](commands.md), [raw assertions](results.json), the
[persistence contract](../../persistence.md), and tests in
`internal/workspace`, `tests/faults`, and `tests/integration`.
