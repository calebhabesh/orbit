# Operator Runbook: Safe Upgrade and Preflight Procedure

Upgrade uses a graceful service stop, a consistent SQLite backup, compatibility
checks and health checks before restarting replication. Use the actual state
directory and service unit for your installation in the commands below.

---

## Upgrade Lifecycle Overview

Per the Operations Specification (`docs/operations.md`):
> *"Upgrade stops the agent, obtains a consistent backup, checks migration compatibility, applies migration transactionally where supported, and runs health checks before reopening replication."*

---

## Step-by-Step Upgrade Procedure

### Step 1: Execute Upgrade Preflight Check
Before modifying any binaries or stopping services, run the upgrade preflight check:
```bash
filesync maintenance preflight
```

The preflight verifies:
1. State directory ownership and mode `0700`.
2. Free disk space against the 512 MiB reserve limit.
3. Database integrity (`PRAGMA quick_check`).
4. Schema compatibility against target binary versions.

If the check passes with `status=ready`:
```text
upgrade preflight: status=ready database_schema=10 binary_schema=10 agent_running=true integrity_clean=true free_space_mb=29296
next steps:
  - environment is ready for upgrade; create consistent backup ('filesync maintenance backup') and proceed
```

*(If preflight reports `status=blocked`, resolve reported issues before proceeding).*

---

### Step 2: Stop the Background Service
Ensure all in-flight sync transfers cleanly flush their verified chunks:
```bash
systemctl --user stop filesync.service
# Or via CLI directly:
filesync stop
```
Verify the process has terminated:
```bash
filesync maintenance preflight
# agent_running should now report false
```

---

### Step 3: Create a Consistent SQLite Backup
Take a transactionally consistent backup using SQLite `VACUUM INTO`:
```bash
filesync maintenance backup --out ~/.local/state/filesync/pre-upgrade-backup.sqlite
```
The CLI requires exclusive state ownership, so stop the agent before backup.
SQLite `VACUUM INTO` creates a consistent snapshot including committed WAL
content. Copying only a live `metadata.sqlite` file can omit that content.
Keep the backup together with the documented identity-rollback recovery plan;
it is not an independent copy of all historical payloads.

---

### Step 4: Install Updated Binary or Package

#### Debian / Ubuntu:
```bash
sudo dpkg -i filesync_<new-version>_<arch>.deb
```

#### Fedora / RHEL:
```bash
sudo rpm -Uvh filesync-<new-version>-1.<arch>.rpm
```

#### Tarball / Direct Binary:
```bash
cp /path/to/new/filesync ~/.local/bin/filesync
chmod 0755 ~/.local/bin/filesync
```

---

### Step 5: Verify Schema Migration Status
Inspect migration requirements before restarting the daemon:
```bash
filesync maintenance check
```
- If `status=up_to_date`: No schema changes required.
- If `status=migration_needed`: Pending transactional migrations will execute automatically upon daemon start.
- If `status=incompatible`: The database was authored by a newer binary than the installed version. Upgrade the binary or see [`rollback.md`](rollback.md).

---

### Step 6: Start Service and Verify Health
Restart the daemon and allow pending migrations to execute:
```bash
systemctl --user start filesync.service
```

Verify that the service is running cleanly:
```bash
systemctl --user status filesync.service
filesync doctor
```

All categories in `filesync doctor` should report `OK`. Check logs if any warnings are emitted:
```bash
journalctl --user -u filesync.service -n 50
```
