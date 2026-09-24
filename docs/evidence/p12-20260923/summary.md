# P12 — Bounded Continuous Operation Summary

## Status: COMPLETE

All acceptance criteria across bounded work queues, fair scheduling, dual-scan cadences, watcher hints, bandwidth limiting, hardware profiles, retry classification, crash recovery, and root failure isolation have been implemented and verified.

---

### Implementation Overview

1. **Database Schema Migration 9 (`internal/repository/repository.go`, `internal/repository/work.go`)**
   - Bumped `CurrentSchema = 9`.
   - Updated `path_projections` with stat tracking fields: `observed_size`, `observed_mtime_ns`, `observed_ctime_ns`, `observed_inode`, and `last_scanned_ns`.
   - Added `durable_work_tasks` table in SQLite with STRICT mode, foreign keys, indexes, and full lifecycle states (`queued`, `running`, `retry`, `exhausted`, `completed`, `canceled`).
   - Wired `RecoverInFlightDurableTasks` into `OpenWithOptions` so any tasks left running across process restarts safely reset to `queued` (satisfying Invariant I20).

2. **Dual-Scan Cadence & Stat Optimization (`internal/workspace/workspace.go`)**
   - Added `ScanWithOptions(ctx, folder, ScanOptions{FullContent, Paths})`.
   - **Quick scan (`FullContent: false`)**: Fast stat comparison skipping hashing when size, mtime, inode, and mode match projection metadata.
   - **Full-content verification scan (`FullContent: true`)**: Forces cryptographic re-hashing of all file bytes to catch timestamp-preserving same-size edits.
   - **Watcher feedback echo suppression (Invariant I17)**: Verifies file digests before authoring; matching digests update observed stats without authoring duplicate versions.
   - **Root failure isolation (Invariant I11)**: Detects unavailable or unmounted workspace roots and fails without deleting tracked files or emitting spurious tombstone versions.

3. **Scheduler & Resource Management (`internal/scheduler/`)**
   - **Resource Profiles (`profile.go`)**: Measured defaults for `laptop` (4 transfer workers, 2 hash workers, 5m sync, 24h full scan) and `pi` (2 transfer workers, 1 hash worker, 5m sync, 24h full scan).
   - **Token-Bucket Bandwidth Limiter (`limiter.go`)**: Global and per-peer rate limiting with burst allowance. Integrated into `Syncer` chunk transfers.
   - **Inotify Watcher (`watcher.go`)**: Linux inotify event consumer with non-blocking epoll, debouncing window, and automatic full-scan escalation on `IN_Q_OVERFLOW`.
   - **Retry Classifier (`retry.go`)**: Distinguishes transient errors (`UNSTABLE_FILE`, network timeouts) with exponential backoff and capped jitter up to 5 attempts, from permanent errors (`ROOT_UNAVAILABLE`, structural conflicts) which immediately transition to `exhausted`.
   - **Fair Work Queue (`queue.go`)**: Round-robin iteration across folders and small-file preference with aging (`AgeBonusBytes = 64 KiB`). Every round a task is bypassed, its effective score drops by 64 KiB, guaranteeing large files advance and prevent starvation (Invariant I13). Bounded capacity at 1024 tasks.
   - **Scheduler Coordinator (`scheduler.go`)**: Coordinates dispatcher loop, worker semaphores, dual-scan tickers, watcher hints, root pause/resume, and prompt graceful shutdown.

4. **Continuous Agent Operation & Control Layer (`internal/app/app.go`, `internal/control/`, `cmd/filesync/`)**
   - Extended `app.ServeWithOptions` to start scheduler alongside peer listener with graceful shutdown on `SIGINT`/`SIGTERM`.
   - Extended CLI `serve` flags: `--profile` (`laptop`|`pi`), `--bandwidth-limit`, `--sync-interval`, `--full-scan-interval`, `--no-watch`.
   - Implemented `filesync work` command family:
     - `filesync work scan [--folder <id>] [--full] [--json]`
     - `filesync work sync --folder <id> [--peer-device <id>] [--json]`
     - `filesync work status [--folder <id>] [--json]`
     - `filesync work retry [--folder <id>] [--task <id> | --all] [--json]`
     - `filesync work cancel --task <id>] [--json]`
     - `filesync work list [--folder <id>] [--state <state>] [--json]`

---

### Invariant Verification

- **I11 (Unavailable roots create no deletions)**: Verified in `TestP12RootUnavailablePauseFolderNoDeletions`. Renaming/unmounting the workspace root pauses folder work with `ROOT_UNAVAILABLE` and emits zero deletion proposals.
- **I13 (Bounded resources; large-file progress amid small edits via aging)**: Verified in `TestP12BoundedQueueAndFairScheduling`. Queue enforces 1024 task limit; large files advance against continuous small file streams via the 64 KiB per-round aging bonus.
- **I17 (Scan after apply/restart or repeated watcher feedback creates no authored versions)**: Verified in `TestP12WatcherFeedbackSuppression`. Multiple repeated quick and full scans on unchanged files generate zero new authored versions.
- **I20 (Limits preserve recoverable state)**: Verified in `TestP12CrashRecoveryOfInFlightTasks`. In-flight tasks interrupted mid-execution are automatically restored to `queued` on restart.

---

### Owner Explanation Note

> **Why watchers improve latency but cannot replace reconciliation scans**
>
> Modern operating system notification systems (such as Linux `inotify`, macOS `FSEvents`, or Windows `ReadDirectoryChangesW`) are invaluable optimizations for low-latency continuous synchronization, but they cannot act as the sole mechanism for change detection in an eventually consistent distributed filesystem:
>
> 1. **Best-Effort Delivery & Queue Overflow**
>    Operating system event notification queues are bounded by kernel memory constraints (e.g., `fs.inotify.max_queued_events`). During intense I/O bursts, compiler runs, or mass archive extractions, the kernel drops events and emits `IN_Q_OVERFLOW`. When this occurs, granular file paths are lost, necessitating a directory scan.
>
> 2. **Offline and Unobserved Mutations**
>    Files modified while the sync agent is offline, paused, restarting, or asleep (such as laptop lid-close suspend, OS reboots, or external drive detached and edited on another host) generate no kernel notifications for the sync process.
>
> 3. **Timestamp-Preserving and Same-Size Edits**
>    Tools such as `tar -x`, `rsync -a`, backup restorers, and hex editors frequently rewrite file contents while explicitly retaining the previous modification timestamp (`mtime`) and file size. Because these mutations leave stat fingerprints unchanged, quick stat checks miss them; only a periodic full-content cryptographic scan detects the modified content.
>
> 4. **Filesystem and Virtual Mount Limitations**
>    Network-attached storage (NFS, SMB), FUSE mounts, container volume bindings, and overlay filesystems often do not emit local inotify notifications when modified by remote peers or out-of-process drivers.
>
> **Architectural Conclusion:** Filesystem watchers serve exclusively as a **latency optimization** to initiate immediate dispatch when mutations occur in real time. **Periodic bounded reconciliation scans** and **full-content verification scans** provide the **authoritative correctness guarantee**, ensuring eventual convergence regardless of notification drops, kernel buffer overflows, or offline modifications.
