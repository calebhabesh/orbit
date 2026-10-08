# Operator Runbook: Binary Rollback, Database Rollback, and Identity Safety

Pass `--state /absolute/selected/state` to every maintenance/engine command below
when using a custom state directory. These retained engine commands keep their default
state convention; selecting the TUI state does not change an engine command's
flags. Stop that exact daemon and inspect recovery markers before replacing data.

This runbook explains how to roll back binary versions in Orbit, how to recover from an earlier SQLite database backup, and why restoring older metadata requires an immediate identity reset under the Orbit causal consistency protocol.

---

## 1. Scenario A: Compatible Binary Rollback (Schema Unchanged)

A binary rollback keeps the **current** metadata and author counters. Schema
compatibility is necessary but insufficient: the older binary must understand
terminal runtime settings and durable operation ledgers. Reconcile pending
operations/editor/onboarding recovery first and migrate reviewed finite budgets
to the older supported configuration explicitly. Do not run an older browser-era
binary over new terminal records merely because both use schema 13. Once those
compatibility conditions have actual evidence:

1. Stop the running service:
   ```bash
   orbit service stop
   # Or via systemctl:
   systemctl --user stop orbit.service
   ```
2. Reinstall the previous compatible package or binary:
   ```bash
   sudo dpkg -i orbit_<previous-version>_<arch>.deb
   # or replace ~/.local/bin/orbit and ~/.local/bin/orbit
   ```
3. Run the schema check to confirm compatibility:
   ```bash
   orbit maintenance check
   # Must report: status=up_to_date
   ```
4. Restart the service:
   ```bash
   orbit service start
   orbit doctor
   ```

---

## 2. Scenario B: Incompatible Binary Downgrade Attempt (Invariant I20)

If a new release applied a database migration (e.g. bumping `user_version` from 13 to 14), and an operator attempts to launch an older binary that only supports schema 13:

- **Behavior**: The older binary immediately exits with an explicit error:
  ```text
  orbit: metadata schema is newer than this binary: database=14 binary=13
  ```
- **Guarantee (Invariant I20)**: The older binary refuses to execute queries or corrupt table structures. It does not delete or overwrite newer tables.
- **Resolution**: Reinstall the binary matching schema 14, or restore the database from a pre-migration backup following **Scenario C** below.

---

## 3. Scenario C: Restoring Older Database Backup & Safe Identity Reset (Invariant I08)

### Why Identity Rollback is a Critical Protocol Concern

In Orbit's decentralized causal consistency model:
1. Every version edit authored by device $D$ is stamped with a monotonically incrementing counter ($c = \text{next\_counter}++$), creating a permanent causal envelope $(D, c)$.
2. Peer replicas observe and durably record these envelopes alongside vector clocks and author signatures.
3. **The Rollback Hazard**: Suppose device $D$ authored versions up to counter 15. If the database on device $D$ is restored from a backup taken when its counter was at 5, and device $D$ continues using identity $D$:
   - It will author a new, different file modification with counter 6.
   - When peers receive this new envelope $(D, 6)$, they already hold an existing $(D, 6)$ with different parent versions, different content digests, and different vector clocks.
   - This creates an irreversible **counter collision**, causing version shadowing, silent sync failure, or cyclic causality divergence.

**Core Rule**: *Restoring older metadata cannot knowingly reuse causal identity/counters (Invariant I08).*

---

### Safe Restoration Workflow

Orbit provides an integrated, atomic restoration command that automatically enforces identity reset:

#### Step 1: Stop the Running Daemon
```bash
orbit service stop
# Or via systemctl:
systemctl --user stop orbit.service 2>/dev/null || true
```

#### Step 2: Restore from Consistent Backup
Execute `orbit maintenance restore-backup`:
```bash
orbit maintenance restore-backup --backup ~/.local/state/orbit/pre-upgrade-backup.sqlite
```
*(Compatibility note: legacy syntax `orbit maintenance restore-backup` is also supported).*

What this command does:
1. Validates backup file integrity (`PRAGMA quick_check`) and schema compatibility.
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
Because the node has assumed a fresh cryptographic identity, you must approve the new device ID on participating peer devices:
```bash
# Display new identity and certificate pin
orbit engine identity --certificate

# On peer device(s), approve the new device ID:
orbit engine pair-approve --folder <folder-id> --peer-device <new-device-id> --peer-key-pin <new-pin>
# Or via enrollment request workflow:
# orbit requests approve --request <request-id>
```

#### Step 4: Restart Service
```bash
orbit service start
orbit doctor
```
All existing workspace files and object store chunks remain intact; the sync engine reconciles divergent edits cleanly without ever violating counter monotonicity.
