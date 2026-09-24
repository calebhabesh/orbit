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
   filesync maintenance check --json
   filesync maintenance recovery --json
   ```
   Inspect:
   - `in_flight_tasks`: Tasks interrupted mid-execution.
   - `pending_proposals`: Uncommitted deletion proposals.
   - `recovery_needed`: True if database needs repair.

2. **Run Doctor Diagnostic**:
   ```bash
   filesync doctor
   ```

## Remediation Workflow

### Scenario A: Clean Crash or Power Interruption
If the machine abruptly shut down during a sync or scan:
1. Simply restart the agent:
   ```bash
   filesync serve
   ```
2. The agent automatically executes SQLite WAL replay and invokes `RecoverInFlightDurableTasks`, safely resetting uncommitted running tasks back to queued state without data loss.

### Scenario B: Database File Corrupted, Restoring from Backup
If the primary SQLite file was damaged and restored from an older backup:

#### Step 1: Replace Database File
Stop filesync service and replace `repo.sqlite`:
```bash
systemctl --user stop filesync
cp /path/to/backup.sqlite ~/.local/state/filesync/repo.sqlite
rm -f ~/.local/state/filesync/repo.sqlite-wal ~/.local/state/filesync/repo.sqlite-shm
```

#### Step 2: Safe Identity Reset (CRITICAL)
**Never reuse the previous device ID with a rolled-back database counter.**
Execute identity reset:
```bash
filesync maintenance reset-identity --json
```
This operation:
1. Generates a brand-new cryptographic device ID and keypair.
2. Clears stale local author memberships to prevent vector clock regressions.
3. Leaves all user files and local object chunks completely intact.

#### Step 3: Re-enroll Folders with Cluster
1. For each shared folder, re-approve membership from an active surviving peer:
   ```bash
   # On surviving peer:
   filesync peers add --folder <folder-id> --device <new-device-id>
   filesync membership export --folder <folder-id> --out updated-membership.json

   # On recovered device:
   filesync membership import --folder <folder-id> --file updated-membership.json
   filesync folders add --folder <folder-id> --root /path/to/workspace
   ```

#### Step 4: Reconcile Workspace Root
Trigger a full scan to adopt existing local files under the new identity:
```bash
filesync work scan --folder <folder-id> --full
filesync work sync --folder <folder-id>
```
Verify convergence:
```bash
filesync doctor
filesync work status --folder <folder-id>
```
