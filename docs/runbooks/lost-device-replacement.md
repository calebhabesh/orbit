# Operator Runbook: Lost Device Retirement & Replacement

## Trigger and Scope
- A synchronized device has been lost, stolen, damaged, or permanently replaced.
- Operator needs to:
  1. Revoke the lost device's authorization so it can never author or sync changes.
  2. Provision a replacement device with a fresh key pair (no key or counter reuse).
  3. Enroll the replacement device into the sync cluster.
  4. Perform stopped/exclusive metadata backup restore if recovering state.
  5. Configure headless operation and service lingering where applicable.

---

## Guarantees & Safety Invariants
- **Invariant I08 (No Counter Reuse after Restore):** Restoring a database snapshot resets local device identity to a fresh key pair before resuming sync.
- **Invariant I09 / I24 (Retiree Deauthorization & Fork Prevention):** Once a device is retired, its vector counter is pinned via a canonical retirement snapshot digest. Surviving nodes reject any future requests or versions authored by the retired device.
- **Invariant I20 (Unregister Safety):** Unregistering a workspace retains all local files on disk and emits zero deletion tombstones.
- **G04 (Stopped/Exclusive Recovery Gate):** State restoration and identity resets must execute while the background daemon is strictly stopped. Running daemons reject restore attempts.

---

## Step-by-Step Procedure

### 1. Retire the Lost Device from a Surviving Device
From any active device (via Orbit Web UI under **Devices** > **Retire Device**, or via CLI):

```bash
# 1. Preview retirement impact
orbit peers retire --folder <folder-id> --device <lost-device-id> --preview --json

# 2. Execute retirement
orbit peers retire --folder <folder-id> --device <lost-device-id>
```
The retirement creates a deterministic snapshot of all versions authored by the lost device up to the retirement revision. The lost device cannot fork the workspace.

### 2. Provision the Replacement Device (Fresh Identity)
On the new/replacement hardware:
1. Install File Sync / Orbit.
2. Initialize with a brand new, independent device identity:
   ```bash
   orbit init
   ```
   > [!IMPORTANT]
   > Never attempt to copy private keys or `.filesync-internal` states from an old machine to "re-use" the lost device's identity. Reusing device IDs or counter sequences violates causal DAG invariants and causes permanent branch rejection.

### 3. Enroll the Replacement Device
1. On the surviving device, generate an invitation code:
   - In Orbit Web UI: Click **Add Device** > Copy pairing code or show QR code.
   - In CLI: `orbit invite --folder <folder-id>`
2. On the replacement device:
   - In Orbit Web UI: Navigate to **Join Folder** > Enter folder ID and invitation token.
   - In CLI: `orbit join --token <token> --folder <folder-id> --root ~/Sync`
3. On the surviving device, review and approve the enrollment request.
   - Both devices now actively replicate the folder.

### 4. Stopped/Exclusive Metadata Backup Restore (When Recovering from Snapshot)
If restoring state from a consistent SQLite backup (`backup-*.sqlite`):

> [!CAUTION]
> Design Gate G04 requires the daemon to be stopped. Attempting to restore while the daemon is running will be rejected.

1. Stop the background service:
   ```bash
   systemctl --user stop filesync.service
   # or
   orbit service stop
   ```
2. Run exclusive restore with automatic identity reset:
   ```bash
   filesync maintenance restore-backup --state ~/.local/state/filesync --backup /path/to/backup.sqlite
   ```
   Output:
   ```json
   {
     "backup_path": "/path/to/backup.sqlite",
     "new_device_id": "9b12...4c",
     "new_key_pin": "e3b0...12",
     "message": "backup successfully restored and causal identity safely reset"
   }
   ```
3. Restart the background service:
   ```bash
   systemctl --user start filesync.service
   # or
   orbit service start
   ```

> [!NOTE]
> **Payload Honesty:** Metadata backup restore recovers folder configuration, causal heads, and DAG history. Chunks that were not locally cached are marked as `pending` or `unavailable` and will be fetched from online peers. Metadata backups are never misrepresented as containing full chunk data.

### 5. Headless Administration & Service Lingering
For servers, headless home labs, or VPS instances where the operator is not logged in interactively:
1. Enable systemd user lingering so the background sync daemon starts at boot and survives logout:
   ```bash
   loginctl enable-linger $USER
   ```
2. Verify service status:
   ```bash
   systemctl --user status filesync.service
   ```
3. Access Orbit Web UI remotely via secure SSH port forwarding:
   ```bash
   ssh -N -L 8080:127.0.0.1:8080 user@remote-host
   ```
   Then browse to `http://127.0.0.1:8080` in your local browser.
