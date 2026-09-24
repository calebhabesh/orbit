# Operator Runbook: Binary Rollback, Database Rollback, and Identity Safety

This runbook explains how to roll back binary versions, how to recover from an earlier SQLite database backup, and why restoring older metadata requires an immediate identity reset under the File Sync protocol.

---

## 1. Scenario A: Compatible Binary Rollback (Schema Unchanged)

If an upgraded binary introduced an application regression, but the database schema version remains identical (`database user_version == binary CurrentSchema`):

1. Stop the running service:
   ```bash
   systemctl --user stop filesync.service
   ```
2. Reinstall the previous compatible package or binary:
   ```bash
   sudo dpkg -i filesync_<previous-version>_<arch>.deb
   # or replace ~/.local/bin/filesync
   ```
3. Run the schema check to confirm compatibility:
   ```bash
   filesync maintenance check
   # Must report: status=up_to_date
   ```
4. Restart the service:
   ```bash
   systemctl --user start filesync.service
   filesync doctor
   ```

---

## 2. Scenario B: Incompatible Binary Downgrade Attempt (Invariant I20)

If a new release applied a database migration (e.g. bumping `user_version` from 9 to 10), and an operator attempts to launch an older binary that only supports schema 9:

- **Behavior**: The older binary immediately exits with an explicit error:
  ```text
  filesync: metadata schema is newer than this binary: database=10 binary=9
  ```
- **Guarantee (Invariant I20)**: The older binary refuses to execute queries or corrupt table structures. It does not delete or overwrite newer tables.
- **Resolution**: Reinstall the binary matching schema 10, or restore the database from a pre-migration backup following **Scenario C** below.

---

## 3. Scenario C: Restoring Older Database Backup & Safe Identity Reset (Invariant I08)

### Why Identity Rollback is a Critical Protocol Concern

In File Sync's decentralized causal consistency model:
1. Every version edit authored by device $D$ is stamped with a monotonically incrementing counter ($c = \text{next\_counter}++$), creating a permanent causal envelope $(D, c)$.
2. Peer replicas observe and durably record these envelopes alongside vector clocks and author signatures.
3. **The Rollback Hazard**: Suppose device $D$ authored versions up to counter 15. If the database on device $D$ is restored from a backup taken when its counter was at 5, and device $D$ continues using identity $D$:
   - It will author a new, different file modification with counter 6.
   - When peers receive this new envelope $(D, 6)$, they already hold an existing $(D, 6)$ with different parent versions, different content digests, and different vector clocks.
   - This creates an irreversible **counter collision**, causing version shadowing, silent sync failure, or cyclic causality divergence.

**Core Rule**: *Restoring older metadata cannot knowingly reuse causal identity/counters (Invariant I08).*

---

### Safe Restoration Workflow

File Sync provides an integrated, atomic restoration command that automatically enforces identity reset:

#### Step 1: Stop the Running Daemon
```bash
systemctl --user stop filesync.service
# or: filesync stop
```

#### Step 2: Restore from Consistent Backup
Execute `filesync maintenance restore-backup`:
```bash
filesync maintenance restore-backup --backup ~/.local/share/filesync/pre-upgrade-backup.sqlite
```

What this command does:
1. Validates the backup file integrity (`PRAGMA quick_check`) and schema compatibility.
2. Acquires exclusive state directory lock to ensure no concurrent processes are active.
3. Cleans up stale WAL (`metadata.sqlite-wal`) and SHM (`metadata.sqlite-shm`) files.
4. Atomically replaces `metadata.sqlite`.
5. **Safely Resets Device Identity**:
   - Generates a brand-new cryptographic Device ID ($D'$).
   - Generates a fresh TLS certificate and private key.
   - Updates `config.json`.
   - Updates all folder records in the database, setting `local_author = D'` and resetting `next_counter = 0`.
   - Records an audit log event preserving Invariant I08.

Sample output:
```text
backup restored from /path/to/backup.sqlite
causal identity safely reset: old_device=756c9bc5... new_device=a809e552... key_pin=375710f4...
CRITICAL (Invariant I08): Rolled-back causal author counters have been retired.
Action required: re-enroll new device ID in folder memberships with peers (Invariant I08)
```

#### Step 3: Re-enroll New Device Identity with Peers
Because the node has assumed a fresh cryptographic identity, you must pair and approve the new device ID on participating peer devices:
```bash
# Display new identity and certificate pin
filesync identity --certificate

# On peer device(s), approve the new device ID:
filesync pair-approve --folder <folder-id> --peer-device <new-device-id> --peer-key-pin <new-pin>
```

#### Step 4: Restart Service
```bash
systemctl --user start filesync.service
filesync doctor
```
All existing workspace files and object store chunks remain intact; the sync engine reconciles divergent edits cleanly without ever violating counter monotonicity.
