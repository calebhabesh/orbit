# Operator Runbook: Safe Uninstallation and Data Preservation

This runbook documents the package uninstallation procedure for Orbit, explaining the data preservation guarantees that protect operator workspace folders and state directories during package removal.

---

## 1. Data Preservation Guarantees (Requirements S21, U16; Invariant I20)

Per Orbit's Product Vision (`docs/orbit-product.md`) and Operations Specification (`docs/operations.md`):
> *"Uninstall removes executables, desktop entries, icons, and service registration while preserving roots and state unless explicitly requested otherwise."*

When you uninstall the `orbit` (or `filesync`) package:
1. **Workspace Files are NEVER Deleted**: All files, directories, documents, and code stored inside your synchronized workspace roots remain completely untouched.
2. **Metadata & History are Preserved**: The SQLite database (`metadata.sqlite`), cryptographic keys (`tls.key`), content chunks (`objects/`), and configuration (`config.json`) stored in `~/.local/state/filesync` or `~/.filesync` are preserved.
3. **No Deletion Tombstones are Emitted (Invariant I20)**: Uninstallation or folder unregistration does not generate deletion tombstones across peer devices.
4. **Reinstallation is Seamless**: If you reinstall Orbit in the future, your device identity, key pins, and folder registrations immediately resume without needing re-pairing or re-downloading existing chunks.

---

## 2. Uninstallation Procedures

### Step 1: Stop and Disable the User Service
Before removing package binaries, cleanly shut down the daemon:
```bash
orbit service stop
orbit service disable
# Or via systemctl directly:
systemctl --user stop orbit.service filesync.service 2>/dev/null || true
systemctl --user disable orbit.service filesync.service 2>/dev/null || true
```

---

### Step 2: Remove Package

#### Debian / Ubuntu (`dpkg` / `apt`):
```bash
sudo dpkg -r orbit
# or:
sudo apt remove orbit
```
*(If installed as legacy package: `sudo apt remove filesync`). The Debian `postrm` script automatically reloads systemd user units and explicitly skips state and workspace directories.*

#### Fedora / RHEL (`rpm` / `dnf`):
```bash
sudo rpm -e orbit
# or:
sudo dnf remove orbit
```

#### Tarball Installation:
If installed via standalone tarball:
```bash
cd /path/to/extracted/orbit-tarball
./uninstall.sh
```
The uninstaller removes executables (`orbit`, `filesync`), desktop entries (`orbit.desktop`), icons (`orbit.svg`), and user service unit definitions, while explicitly leaving all state and workspace files intact.

---

## 3. Post-Uninstall Verification

Verify that package binaries, desktop entries, and services have been removed:
```bash
which orbit || echo "orbit executable removed"
which filesync || echo "filesync executable removed"
systemctl --user status orbit.service || echo "service successfully unregistered"
```

Verify that your user data and state remain intact:
```bash
ls -la ~/.local/state/filesync/metadata.sqlite
ls -la <path-to-your-synced-folders>
```

---

## 4. Optional: Complete Manual Purge

Only if you explicitly intend to permanently delete all local synchronization state and cryptographic keys from this machine:

```bash
# 1. Ensure service is stopped
systemctl --user stop orbit.service filesync.service 2>/dev/null || true

# 2. Delete state directory (removes local SQLite DB, keys, and chunk cache)
rm -rf ~/.local/state/filesync ~/.filesync

# NOTE: Your workspace folders containing your actual user documents
# still remain intact. If you wish to delete workspace files as well,
# you must manually remove those specific folders.
```
