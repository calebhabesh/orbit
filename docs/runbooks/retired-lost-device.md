# Operator Runbook: Retired or Lost Device Decommissioning

## Trigger and Symptoms
- A participating device is permanently decommissioned, replaced, lost, or stolen.
- Operator needs to revoke the device's authorization to prevent it from authoring or receiving versions.
- Doctor reports a warning if membership approaches the 16 active members capacity limit.

## Guarantees
- Invariant **I09**: A retired or removed device cannot author accepted versions. Removal preserves folder contents and local history unless local deletion is explicitly selected.
- Retirement installs a canonical tombstone snapshot in folder membership, permanently freezing the retired member's vector counter.
- Surviving devices reject all subsequent replication or chunk requests from the retired device keypin.

## Procedure to Retire a Device

### Step 1: Preview Member Retirement
Inspect the effect of retirement on the remaining cluster:
```bash
filesync peers retire --folder <folder-id> --device <target-device-id> --preview --json
```
Verify:
- `surviving_members`: Lists all surviving devices.
- `next_revision`: Increment of membership epoch.
- `accepted_versions_count`: Number of accepted versions locked into the retirement snapshot.

### Step 2: Execute Retirement on an Active Device
```bash
filesync peers retire --folder <folder-id> --device <target-device-id>
```
The output confirms:
`retired device <device-id> on folder <folder-id>: new_revision=<rev> digest=<digest>`

### Step 3: Export and Distribute Membership Update
Export the canonical updated membership with retirement snapshot:
```bash
filesync membership export --folder <folder-id> --out membership-rev<rev>.json
```
On surviving devices, import the updated membership:
```bash
filesync membership import --folder <folder-id> --file membership-rev<rev>.json
```

### Step 4: Decommissioning the Physical Device (If Accessible)
If the retired machine is still physically accessible:
1. Stop the sync service:
   ```bash
   systemctl --user stop filesync
   ```
2. Unregister synchronized folders (this preserves user files while clearing agent metadata):
   ```bash
   filesync folders remove --folder <folder-id>
   ```
3. If complete wipe is desired, remove the state directory:
   ```bash
   rm -rf ~/.local/state/filesync/
   ```
