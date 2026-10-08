# Operator Runbook: Full Disk and Storage Budget Exhaustion

## Trigger and Symptoms
- Agent transitions into paused state with error `DISK_BUDGET`.
- Diagnostic output or doctor reports status `WARN` or `FAIL` on `storage_budget`:
  `free space on filesystem (<bytes>) is below reserve target (512 MiB)`
- Background scans and peer transfers pause to preserve diagnostic and recovery capacity.
- WAL soft-admission cap (256 MiB) is reached or approached.

## Guarantees
- Invariant **I12**: Hard disk exhaustion does not cause database or chunk corruption. The agent reserves a 512 MiB free space buffer per filesystem to permit administrative recovery, diagnostics, and WAL checkpoints.
- Existing synchronized files remain intact and accessible in plaintext on disk.

## Diagnostic Steps

1. **Inspect Storage Usage**:
   ```bash
   orbit storage usage --json
   ```
   Examine:
   - `state_filesystem.available_bytes`: Bytes remaining on the state drive.
   - `folders[].filesystem.available_bytes`: Bytes remaining on each workspace mount.
   - `wal_bytes`: Size of SQLite write-ahead log.
   - `staging_bytes` and `quarantine_bytes`: Transient chunk buffers.

2. **Run Doctor Diagnostic**:
   ```bash
   orbit doctor
   ```
   Check the `storage` category for specific filesystem remediation advice.

## Remediation Workflow

### Step 1: Run Garbage Collection on Retained Unreferenced Chunks
Reclaim unreferenced and expired chunk payloads:
```bash
# Preview reclaimable capacity
orbit storage gc preview

# Execute garbage collection
orbit storage gc run
```

### Step 2: Prune Staging and Quarantined Artifacts
If corrupted chunks or aborted in-flight transfers have occupied disk space:
```bash
# Preview recovery items
orbit maintenance recovery --json

# Reclaim recovery artifacts
orbit storage recovery reclaim
```

### Step 3: Trigger SQLite WAL Checkpoint
If SQLite write-ahead log (`metadata.sqlite-wal`) is holding excess disk space:
```bash
orbit maintenance backup --out /mnt/external/backup.sqlite
```
Taking a consistent backup checkpoints active WAL transactions into the main database.

### Step 4: Expand Storage or Move Workspace Root
If user data exceeds available drive capacity:
1. Pause replication:
   ```bash
   orbit folders pause --folder <folder-id> --reason "expanding disk capacity"
   ```
2. Move data or expand storage partition.
3. If mount point changed, update folder root:
   ```bash
   orbit folders add --folder <folder-id> --root /new/mount/path
   orbit engine safety root-revalidate --folder <folder-id>
   orbit folders resume --folder <folder-id>
   ```

4. Verify agent health:
   ```bash
   orbit doctor
   ```
