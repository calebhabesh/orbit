# Operator Runbook: Database Recovery and Identity Reset

## Trigger and Symptoms
- Storage media corruption, unrecoverable SQLite corruption error, or accidental database deletion.
- Recovery from an older database backup or filesystem snapshot.
- Attempting to start the agent after restoring an old SQLite file.

## Guarantees
- Invariant **I19**: Safe recovery preserves user data. Recovering state cannot cause silent data loss on remote peers.
- Invariant **I20**: Interrupted operations restart safely. Crashing during an operation leaves the system in a consistent, recoverable state.
- Restoring an old metadata backup creates a fresh identity and safe re-enrollment workflow. It is mathematically impossible to resume rolled-back author counters safely because peers may have already witnessed higher counters.

## Diagnostic Steps

1. **Check Database Health and Recovery Needs**:
   ```bash
   orbit maintenance check --json
   orbit maintenance recovery --json
   ```
   Inspect:
   - `in_flight_tasks`: Tasks interrupted mid-execution.
   - `pending_proposals`: Uncommitted deletion proposals.
   - `recovery_needed`: True if database needs repair.

2. **Run Doctor Diagnostic**:
   ```bash
   orbit doctor
   ```

## Remediation Workflow

### Scenario A: Clean Crash or Power Interruption
If the machine abruptly shut down during a sync or scan:
1. Simply restart the agent:
   ```bash
   orbit service restart
   # Or directly:
   orbit serve
   ```
2. The agent automatically executes SQLite WAL replay and invokes `RecoverInFlightDurableTasks`, safely resetting uncommitted running tasks back to queued state without data loss.

### Scenario B: Database File Corrupted, Restoring from Backup
If the primary SQLite file was damaged and restored from an older backup:

#### Step 1: Restore Backup and Safe Identity Reset (CRITICAL)
**Never reuse the previous device ID with a rolled-back database counter (Invariants I08, I19).**

Stop the background service first, then restore the backup and reset causal identity:
```bash
systemctl --user stop orbit.service filesync.service 2>/dev/null || true
orbit maintenance restore-backup --backup /path/to/backup.sqlite --json
```
*(Compatibility note: legacy syntax `filesync maintenance restore-backup` is also supported).*

This automated command:
1. Verifies the backup database integrity.
2. Cleans up stale WAL and shared memory files (`metadata.sqlite-wal`, `metadata.sqlite-shm`).
3. Atomically replaces `metadata.sqlite`.
4. Rotates the cryptographic device ID and TLS keypair with fresh key pin.
5. Updates configuration (`config.json`) and database local author records in a checked transaction.

Alternatively, if manually replacing the database file:
```bash
systemctl --user stop orbit.service filesync.service 2>/dev/null || true
cp /path/to/backup.sqlite ~/.local/state/filesync/metadata.sqlite
rm -f ~/.local/state/filesync/metadata.sqlite-wal ~/.local/state/filesync/metadata.sqlite-shm
orbit maintenance reset-identity --json
```

#### Step 2: Re-enroll Folders with Cluster
1. For each shared folder, approve membership of the new device ID and key pin from an active surviving peer:
   ```bash
   # On surviving peer:
   orbit pair-approve --folder <folder-id> --peer-device <new-device-id> --peer-key-pin <new-key-pin>
   orbit membership export --folder <folder-id> --file updated-membership.json

   # On recovered device:
   orbit membership import --folder <folder-id> --file updated-membership.json --approve
   orbit folders revalidate --folder <folder-id>
   ```

#### Step 3: Reconcile Workspace Root
Trigger a full scan to adopt existing local files under the new identity:
```bash
orbit work scan --folder <folder-id> --full
orbit work sync --folder <folder-id>
```
Verify convergence:
```bash
orbit doctor
orbit work status --folder <folder-id>
```
