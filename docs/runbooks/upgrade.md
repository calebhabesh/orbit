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
orbit maintenance preflight
```
*(Compatibility note: legacy syntax `filesync maintenance preflight` is also supported).*

The preflight verifies:
1. State directory ownership and mode `0700`.
2. Free disk space against the 512 MiB reserve limit.
3. Database integrity (`PRAGMA quick_check`).
4. Schema compatibility against target binary versions.

If the check passes with `status=ready`:
```text
upgrade preflight: status=ready database_schema=13 binary_schema=13 agent_running=true integrity_clean=true free_space_mb=29296
next steps:
  - environment is ready for upgrade; create consistent backup ('orbit maintenance backup') and proceed
```

*(If preflight reports `status=blocked`, resolve reported issues before proceeding).*

---

### Step 2: Stop the Background Service
Ensure all in-flight sync transfers cleanly flush their verified chunks:
```bash
orbit service stop
# Or via systemctl:
systemctl --user stop orbit.service filesync.service 2>/dev/null || true
```
Verify the process has terminated:
```bash
orbit maintenance preflight
# agent_running should now report false
```

---

### Step 3: Create a Consistent SQLite Backup
Take a transactionally consistent backup using SQLite `VACUUM INTO`:
```bash
orbit maintenance backup --out ~/.local/state/filesync/pre-upgrade-backup.sqlite
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
# Tarball update:
tar -xzf orbit-v<new-version>-linux-<arch>.tar.gz
cd orbit-v<new-version>-linux-<arch>
./install.sh

# Or direct binary replacement:
cp /path/to/new/filesync ~/.local/bin/filesync
ln -sf ~/.local/bin/filesync ~/.local/bin/orbit
```

---

### Step 5: Verify Schema Migration Status
Inspect migration requirements before restarting the daemon:
```bash
orbit maintenance check
```
- If `status=up_to_date`: No schema changes required.
- If `status=migration_needed`: Pending transactional migrations will execute automatically upon daemon start.
- If `status=incompatible`: The database was authored by a newer binary than the installed version. Upgrade the binary or see [`rollback.md`](rollback.md).

---

### Step 6: Start Service and Verify Health
Restart the daemon and allow pending migrations to execute:
```bash
orbit service start
# Or via systemctl:
systemctl --user start orbit.service
```

Verify that the service is running cleanly:
```bash
orbit service status
orbit doctor
```

All categories in `orbit doctor` should report `OK`. Check logs if any warnings are emitted:
```bash
journalctl --user -u orbit.service -n 50
```
