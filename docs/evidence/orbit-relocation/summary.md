# Local workspace relocation — 2026-10-03

Owner-selected follow-up to U04 and O11. Settings now offers **Change location**;
`orbit folders relocate --folder ID --from CURRENT --to DESTINATION` uses the
same authenticated control operation, including fallback to a running daemon.
The destination must be unused and its parent must exist without symlinks.

Same-filesystem relocation moves the complete directory with no replacement.
Across filesystems, Orbit copies and verifies the complete supported tree,
including private scratch and recovery files, before switching registration.
The original remains as an explicitly reported safety copy. It is not synced
at its old location after the switch. Close editors before relocation; review
that copy for late edits before manually removing it.

A workspace IO gate waits for active local operations and blocks scanning,
publication, UI mutations and cleanup during relocation. CAS transfers can
continue. The durable intent is an owner-only state-directory JSON journal.
Recovery checks device/inode and the private registration marker, restores
prior pause state, and updates root registration and setup location atomically
without changing workspace identity, membership, counters, versions or working
basis. Inotify is refreshed and a reconciliation scan is queued at the new root.

Raw command output: [commands](commands.md); structured [results](results.json).

## Executed evidence

- `cd web && npm ci`: passed; 49 packages installed, audit reported zero vulnerabilities.
- `cd web && npm run build`: passed; TypeScript and Vite, 39 modules, embedded assets rebuilt.
- `go test -count=1 -v ./internal/workspace -run TestRelocation`: passed all five test families. Includes same-filesystem interruption before rename, after rename and after database commit; cross-filesystem copy/recovery on the host filesystem and `/dev/shm`; unchanged history/no fabricated edits; executable files/empty directories; stale/occupied/overlapping/symlink-parent destinations; existing pause preservation; waiting for active capture; editor mutation during copy and unsupported symlink refusal.
- `go test -count=1 ./cmd/filesync -run TestRelocation`: passed stopped and live HTTP daemon CLI relocation with the actual state lock.
- `go test -race -count=1 ./internal/workspace ./internal/scheduler ./internal/control ./cmd/filesync`: passed all four packages, no race detector reports.
- `make check`: passed formatting, vet, unit/integration/model/fault/harness checks, amd64/arm64 builds and packaging.
- `node scripts/orbit_ui_test.mjs --scenario relocation`: passed in Chromium. Verified initial input focus, preserved form values after server rejection, move and success feedback, and capture of an edit at the new root with notifications enabled.

The first browser run timed out because the new harness replaced a controlled
input's DOM value directly and also used the existing helper's no-watch default.
The final runner uses keyboard replacement and explicitly enables notifications;
the final run above passed. An initial frontend build found dependencies absent;
`npm ci` installed the existing lockfile before the successful build.

## Limits and unexecuted checks

- Fault hooks simulate interruption and reopen a new Workspace from persisted
  records. Actual process SIGKILL, physical power loss and cross-drive ENOSPC
  campaigns for this feature are unexecuted.
- Interrupted/refused cross-drive copies can leave private
  `.orbit-relocation-*` staging directories at the destination parent. They are
  retained for explicit inspection/cleanup; retries start a fresh copy.
- Relocation temporarily gates all roots in this daemon. Copy work streams file
  bytes but retains per-path verification inventories in memory. No new scale
  or cross-file snapshot guarantee is made.
- Across-drive source removal is manual; automatic destructive cleanup is not
  part of this operation. File changes after final verification remain in the
  original safety copy. Ordinary files/directories are supported; links, nested
  mounts and special files cause a safe copy refusal.
- P17 personal owner-use and unaided explanation evidence remain outstanding.

## Desktop notification regression discovered during validation

The owner's KDE dialog named
`/tmp/TestOrbitSetup_OpenLocalFolder.../RegisteredFolder`. That test inherited
the developer's display environment and could start real `xdg-open`, while test
cleanup removed its target. A safe executable spy in a disposable PATH, with
simulated DISPLAY/WAYLAND_DISPLAY, observed the helper invocation before the fix.
After making the test explicitly headless and requiring
`DESKTOP_HELPER_UNAVAILABLE`, the same probe passed without invoking the spy.
The production desktop opener is unchanged. This fixes the unintended desktop
side effect during development validation.
