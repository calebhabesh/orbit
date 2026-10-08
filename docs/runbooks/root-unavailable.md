# Operator Runbook: Root Unavailable and Unmounted Storage

## Trigger and Symptoms
- Agent logs error code `ROOT_UNAVAILABLE`.
- Background scheduler pauses synchronization for the affected folder.
- `orbit doctor` reports `FAIL` under category `roots`:
  `root directory <path> is unavailable: not mounted, device/inode mismatch, or registration marker missing`

## Guarantees
- Invariant **I11**: Unmounting an external drive or losing access to a workspace root **never** causes the agent to treat absent files as deletions. The folder operations are isolated and paused; **zero** deletion tombstones are authored or replicated to peers.

## Root Causes
1. **External Drive Detached or Sleeping**: USB drive or external SSD disconnected or unmounted by OS power management.
2. **Network Mount Disconnected**: NFS or SMB share dropped connection.
3. **Filesystem Remounted with New Inode/Device**: Re-partitioning, formatting, or mounting to an alternative mount point.
4. **Permissions Mismatch**: Ownership or permissions on root directory changed so orbit agent cannot access `.orbit-scratch`.

## Diagnostic Steps

1. **Verify Mount Status**:
   ```bash
   df -h
   mount | grep <root-path>
   ```

2. **Inspect Folder Status via CLI**:
   ```bash
   orbit folders list --json
   orbit doctor
   ```

## Remediation Workflow

### Step 1: Remount the Storage Volume
Remount the missing volume to its registered path:
```bash
mount /dev/sdX1 /media/user/sync-drive
```

### Step 2: Validate Private Scratch Directory
Ensure `.orbit-scratch/registration.json` is readable and matches the recorded folder registration:
```bash
ls -la /media/user/sync-drive/.orbit-scratch/
```

### Step 3: Revalidate Root Directory
Trigger explicit descriptor-based root revalidation:
```bash
orbit engine safety root-revalidate --folder <folder-id>
```
If successful, output reports:
`revalidated root for folder <folder-id>: valid`

### Step 4: Resume Folder Synchronization
```bash
orbit folders resume --folder <folder-id>
```

### Step 5: Verify Reconciliation
Trigger a quick reconciliation scan to ensure all paths are verified:
```bash
orbit engine work scan --folder <folder-id>
orbit engine work status --folder <folder-id>
```
