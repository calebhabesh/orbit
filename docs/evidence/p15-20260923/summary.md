# Packet P15 Summary: Linux Packaging and Lifecycle

**Packet:** P15
**Date:** 2026-09-23
**Status:** `complete`
**Prerequisites:** P14 complete. Read [scope](../../portfolio-scope.md), [operations](../../operations.md), [verification](../../verification.md), and [04-delivery](../04-delivery.md).
**Requirements Satisfied:** S20 (versioned protocol/database, migrations, target device binaries), S21 (systemd user service, upgrade/uninstall preserving user data).
**Invariants Verified:** I08 (counter monotonicity; identity rollback never knowingly reused), I20 (limits, schema/protocol incompatibility and failed migrations preserve recoverable state).

---

## 1. System Overview

Packet P15 delivers release packaging, systemd user-service integration, reproducible builds, and lifecycle management workflows (preflight, consistent backup, transactional migration, safe rollback, identity reset, and data-preserving uninstallation) for File Sync on Linux `amd64` and `arm64` systems.

### Core Lifecycle Principles
1. **Multi-Architecture Static Packaging ([`dist/`](../../../dist/))**: File Sync compiles to standalone, statically linked binaries on both `linux/amd64` and `linux/arm64` (aarch64) with `CGO_ENABLED=0` using translated pure-Go SQLite (`modernc.org/sqlite`). Three package formats are distributed with cryptographic SHA-256 checksums:
   - **Debian (`.deb`)**: Standard format 2.0 packages with control files, postinst (user daemon reload), prerm (graceful service shutdown), and postrm (strictly preserving user data).
   - **RPM (`.rpm`)**: Standard v3.0 RPM binary packages containing CPIO payloads and post-uninstallation data preservation guards.
   - **Tarball (`.tar.gz`)**: Standalone archives containing binary, systemd service unit, license notices, and user/system install and uninstall scripts (`install.sh`, `uninstall.sh`).
2. **Reproducible Release Tooling ([`scripts/build_packages.go`](../../../scripts/build_packages.go))**: Builds use `-trimpath` and deterministic timestamps (`SOURCE_DATE_EPOCH` / fixed release timestamp `1790208000`), stamping git commit, date, architecture, and version metadata into binary linker symbols without relying on external host packaging tools.
3. **Embedded UI with Zero Runtime Node Dependency ([`S18`](../../../docs/implementation-plan.md#L86))**: Production packages embed all React 19/TypeScript web console assets directly inside the binary. The daemon operates entirely without Node.js, npm, or external web servers.
4. **Systemd User Service & Hardening ([`packaging/systemd/filesync.service`](../../../packaging/systemd/filesync.service))**: Service unit integrates into systemd user sessions with graceful shutdown (`TimeoutStopSec=30s`), auto-restart on failure, `NoNewPrivileges=yes`, `ProtectSystem=strict`, and explicit documentation for user session lingering (`loginctl enable-linger $USER`). User workspace folders are permitted anywhere under `%h` without artificial sandbox traps.
5. **Data Preservation Guarantee ([`S21`](../../../docs/portfolio-scope.md#L35))**: Ordinary package removal (`dpkg -r`, `rpm -e`, or `uninstall.sh`) removes only executables and service definitions. Local state directories (`~/.local/share/filesync`, `~/.filesync`) and all synchronized workspace roots are strictly preserved on disk.
6. **Destructive Test Hooks Excluded**: Production binaries accept no CLI flags or configuration parameters for test hooks. The engine's internal `FaultHook` is `nil` in all production execution paths.

---

## 2. Operator Lifecycle Commands

Packet P15 introduces four operator-facing lifecycle commands:

1. **Upgrade Preflight Check (`filesync maintenance preflight`)**:
   - Inspects state directory ownership and mode `0700`.
   - Checks whether the background agent is actively running (detects lock contention).
   - Reads database `user_version` against binary `CurrentSchema` (warns of pending migrations, blocks on newer schemas).
   - Runs `PRAGMA quick_check` to ensure no database corruption exists prior to binary replacement.
   - Verifies filesystem free space against the 512 MiB reserve threshold.
2. **Atomic Backup Restoration & Identity Reset (`filesync maintenance restore-backup --backup <path>`)**:
   - Atomically replaces `metadata.sqlite` and wipes desynchronized WAL/SHM files.
   - **Enforces Invariant I08**: Restoring an old database backup rolls back author counters. To prevent counter collisions and vector clock corruption with peer replicas, `restore-backup` automatically generates a brand-new cryptographic device ID, issues new TLS certificates, updates `config.json`, resets `local_author` in `folders`, and sets `next_counter = 0`.
3. **Configuration Validation (`filesync config validate`)**:
   - Inspects `config.json`, cryptographic TLS certificate, and private key permissions (`0600`), confirming device ID matches key pin.
4. **Graceful Daemon Stop (`filesync stop`)**:
   - Sends `SIGTERM` to the active daemon recorded in `.agent.pid` and polls until the process cleanly exits and releases the state lock.

---

## 3. Four Complete Operator Runbooks

Packet P15 delivers four comprehensive operational runbooks in [`docs/runbooks/`](../../../docs/runbooks/):
- [`docs/runbooks/install.md`](../../../docs/runbooks/install.md): Package installation (`.deb`, `.rpm`, `.tar.gz`), systemd user service setup, session lingering configuration (`loginctl enable-linger`), and firewall guidance.
- [`docs/runbooks/upgrade.md`](../../../docs/runbooks/upgrade.md): Preflight inspection, graceful service shutdown, consistent backup (`VACUUM INTO`), package upgrade, schema check, and post-upgrade health validation.
- [`docs/runbooks/rollback.md`](../../../docs/runbooks/rollback.md): Compatible binary rollback (schema unchanged), incompatible downgrade refusal (Invariant I20), and database restoration with mandatory identity reset (Invariant I08).
- [`docs/runbooks/uninstall.md`](../../../docs/runbooks/uninstall.md): Package removal, user data preservation guarantees, verification steps, and optional manual purge instructions.

---

## 4. Verification and Test Results

All packaging and lifecycle requirements were verified using the dedicated P15 integration test suite ([`tests/integration/p15_packaging_lifecycle_test.go`](../../../tests/integration/p15_packaging_lifecycle_test.go)) and full project verification:

| Test Case | Description | Result |
| --- | --- | --- |
| `TestP15VersionAndBuildMetadata` | Verifies binary version string contains version, commit, build date, architecture, and Go version | **PASS** |
| `TestP15ReleaseBinaryExcludesDestructiveTestHooks` | Confirms release binary rejects `--fault-hook` CLI flags and runs with nil hooks | **PASS** |
| `TestP15EmbeddedUIWithoutNode` | Verifies loopback web server returns React SPA HTML and routes with Node stripped from PATH | **PASS** |
| `TestP15ConfigValidation` | Verifies `filesync config validate` passes on clean init and detects malformed/uninitialized state | **PASS** |
| `TestP15AgentStop` | Starts background agent and confirms `filesync stop` shuts down the daemon cleanly and cleans `.agent.pid` | **PASS** |
| `TestP15UpgradePreflight` | Verifies preflight reports `status=ready` on clean state, warns when agent runs, and blocks on newer schemas (I20) | **PASS** |
| `TestP15InterruptedMigrationRollback` | Verifies failed/interrupted migration transactions roll back cleanly without bumping `user_version` (I20) | **PASS** |
| `TestP15ConsistentBackupAndRestoreSafety` | Verifies `maintenance backup` creates valid SQLite DB; `restore-backup` resets device ID/counters (I08) | **PASS** |
| `TestP15PackagingOutputsAndChecksums` | Confirms all 6 package archives (.tar.gz, .deb, .rpm) generate with valid checksums in `SHA256SUMS` | **PASS** |
| `TestP15UninstallPreservesUserData` | Verifies uninstallation removes binaries/services while strictly preserving user state and workspace files (S21) | **PASS** |

### Complete Verification Suite
- `make check`: **PASS** (`vet`, unit/model tests, integration tests P00–P15, amd64 static build, arm64 static cross-build, package generation).
- `make test-race`: **PASS** (0 data races across all packages).
- `make test-faults`: **PASS** (D1–D5 design gates and P03/P04/P06 SIGKILL crash boundary restarts).
- `sha256sum -c dist/SHA256SUMS`: **PASS** (all 6 package archives verified).
- `git diff --check`: **PASS** (clean diff).

---

## 5. Owner Explanation Notes

### A. Why copying a live SQLite main file may not create a consistent backup
In SQLite Write-Ahead Logging (WAL) mode:
1. **Committed Transactions Reside in the WAL File**: When transactions commit, their updated database pages are appended to `metadata.sqlite-wal`. They are not immediately copied back (checkpointed) into the main `metadata.sqlite` database file. A regular file copy (`cp metadata.sqlite backup.sqlite`) completely omits all committed WAL frames, resulting in silent data loss of recent sync events, version envelopes, and peer receipts.
2. **Torn Pages and Read Races**: The operating system file copy utility operates page-by-page non-atomically. If SQLite or the operating system flushes a 4096-byte database page while `cp` is reading that sector, `cp` can read half of an old page and half of a new page. The resulting copy will be physically corrupt and will fail `PRAGMA integrity_check`.
3. **Mismatched WAL Salt**: Even if an operator attempts to copy both `metadata.sqlite` and `metadata.sqlite-wal`, the WAL header contains random salt pairs and commit frame counters that must match the database file header exactly. Copying them sequentially risks copying a WAL that corresponds to a checkpointed state that changed mid-copy, rendering the snapshot unopenable.

**File Sync Solution**: File Sync executes `VACUUM INTO 'backup.sqlite'` through its serialized repository database connection (`filesync maintenance backup`). SQLite's `VACUUM INTO` acquires an exclusive database lock, merges all committed WAL frames into a clean, standalone, fully-checkpointed SQLite database file atomically, guaranteeing 100% transactional consistency without torn pages or WAL dependencies.

### B. Why identity rollback is a protocol concern
In a decentralized, eventually-consistent file synchronization engine:
1. **Monotonic Author Counters**: Causality is tracked using vector clocks and monotonically incrementing author counters: every modification authored by device $D$ is stamped with version envelope $V = (D, c)$, where $c = \text{next\_counter}++$. Version envelopes are immutable; once published, their digest and ancestry are permanent.
2. **The Counter Collision Danger**: If device $D$'s database is rolled back to an earlier backup taken when its author counter was at 5 (even though it subsequently authored versions 6 through 15 and gossiped them to peers), and device $D$ resumes synchronization under its original device identity $D$:
   - It will author a new, different local file edit with counter 6.
   - When remote peers receive $(D, 6)$, they already store the original $(D, 6)$ with different file contents, a different parent history, and different vector clocks.
   - Because version envelopes are immutable and content-addressed, peers will reject the new edit as a forged or conflicting duplicate, causing silent synchronization halts, shadowed updates, or cyclic causal graphs.
3. **Why Identity Reset is Mandatory (Invariant I08)**: Restoring an old database is not an isolated storage recovery; it is a critical protocol event. When an old database backup is restored, the device MUST retire its previous identity and generate a completely new Device ID ($D'$) and keypin. By re-enrolling as $D'$, all subsequent edits are authored under $(D', 1)$, cleanly extending the global DAG as forward-causal changes without ever reusing or colliding with rolled-back counters authored by $D$.
