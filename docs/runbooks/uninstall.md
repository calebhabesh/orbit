# Operator Runbook: Safe Uninstallation and Data Preservation

This runbook documents the package uninstallation procedure, explaining the data preservation guarantees that protect operator workspace folders and state directories during package removal.

---

## 1. Data Preservation Guarantees (Requirement S21)

Per File Sync's Portfolio Scope (`docs/portfolio-scope.md`) and Operations Specification (`docs/operations.md`):
> *"Uninstall removes executables/service registration while preserving roots/state unless explicitly requested otherwise."*

When you uninstall the `filesync` package:
1. **Workspace Files are NEVER Deleted**: All files, directories, documents, and code stored inside your synchronized workspace roots remain completely untouched.
2. **Metadata & History are Preserved**: The SQLite database (`metadata.sqlite`), cryptographic keys (`tls.key`), content chunks (`objects/`), and configuration (`config.json`) stored in `~/.local/share/filesync` or `~/.filesync` are preserved.
3. **Reinstallation is Seamless**: If you reinstall `filesync` in the future, your node identity, key pins, and folder registrations immediately resume without needing re-pairing or re-downloading existing chunks.

---

## 2. Uninstallation Procedures

### Step 1: Stop and Disable the User Service
Before removing package binaries, cleanly shut down the daemon:
```bash
systemctl --user stop filesync.service
systemctl --user disable filesync.service
```

---

### Step 2: Remove Package

#### Debian / Ubuntu (`dpkg` / `apt`):
```bash
sudo dpkg -r filesync
# or:
sudo apt remove filesync
```
*(The Debian `postrm` script automatically reloads systemd user units and explicitly skips state/workspace directories).*

#### Fedora / RHEL (`rpm` / `dnf`):
```bash
sudo rpm -e filesync
# or:
sudo dnf remove filesync
```

#### Tarball Installation:
If installed via standalone tarball:
```bash
cd /path/to/extracted/filesync-tarball
./uninstall.sh
```

---

## 3. Post-Uninstall Verification

Verify that package binaries have been removed:
```bash
which filesync || echo "filesync successfully uninstalled"
systemctl --user status filesync.service || echo "service successfully unregistered"
```

Verify that your user data and state remain intact:
```bash
ls -la ~/.local/share/filesync/metadata.sqlite
ls -la <path-to-your-synced-folders>
```

---

## 4. Optional: Complete Manual Purge

Only if you explicitly intend to permanently delete all local synchronization state and cryptographic keys from this machine:

```bash
# 1. Ensure service is stopped
systemctl --user stop filesync.service 2>/dev/null || true

# 2. Delete state directory (removes local SQLite DB, keys, and chunk cache)
rm -rf ~/.local/share/filesync ~/.filesync

# NOTE: Your workspace folders containing your actual user documents
# still remain intact. If you wish to delete workspace files as well,
# you must manually remove those specific folders.
```
