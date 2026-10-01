# Operator Runbook: Linux Installation and Lifecycle Setup

This runbook guides operators through clean installation, systemd user-service configuration, lingering setup, and initial device initialization on target Linux architectures (`amd64` / `x86_64` and `arm64` / `aarch64`).

## Prerequisites and Requirements

- **Supported Architectures**: Linux `amd64` (x86_64) or `arm64` (aarch64).
- **Runtime Dependencies**: Zero external runtime dependencies. Built statically with pure-Go SQLite (`modernc.org/sqlite`); requires no external C compiler or libc coupling.
- **Embedded Web Console**: Self-contained React/TypeScript/Vite operator console is embedded inside the binary via Go embed; **zero Node.js or npm runtime is required**.
- **Systemd Session**: Standard systemd user session (`systemd --user`).
- **Explicit Root Permissions**: User workspace folders can reside anywhere under the user's home directory (`~`).

---

## 1. Choosing a Package Format

File Sync distributes three reproducible packaging formats in `dist/` with cryptographic `SHA256SUMS`:

| Format | Target Systems | Installation Target |
| --- | --- | --- |
| **Debian (`.deb`)** | Debian, Ubuntu, Raspberry Pi OS | `/usr/bin/filesync`, `/usr/lib/systemd/user/filesync.service` |
| **RPM (`.rpm`)** | Fedora, RHEL, Rocky Linux, CentOS | `/usr/bin/filesync`, `/usr/lib/systemd/user/filesync.service` |
| **Tarball (`.tar.gz`)** | Any Linux (Arch, Alpine, non-root) | User-local (`~/.local/bin`) or system (`/usr/local/bin`) |

Verify package integrity before installation:
```bash
sha256sum -c SHA256SUMS
```

---

## 2. Installation Procedures

### Option A: Debian / Ubuntu / Raspberry Pi OS (`.deb`)

For `amd64`:
```bash
sudo dpkg -i filesync_1.0.0_amd64.deb
```
For `arm64` (Raspberry Pi 4 / 5):
```bash
sudo dpkg -i filesync_1.0.0_arm64.deb
```

### Option B: Fedora / RHEL / Rocky Linux (`.rpm`)

For `x86_64`:
```bash
sudo rpm -Uvh filesync-1.0.0-1.x86_64.rpm
```
For `aarch64`:
```bash
sudo rpm -Uvh filesync-1.0.0-1.aarch64.rpm
```

### Option C: Standalone Tarball Installation (`.tar.gz`)

Extract the archive and run the included installer:
```bash
tar -xzf filesync-v1.0.0-linux-amd64.tar.gz
cd filesync-v1.0.0-linux-amd64

# Install for current user (no root required -> installs to ~/.local/bin)
./install.sh

# Or install system-wide (requires sudo -> installs to /usr/local/bin)
sudo ./install.sh system
```

---

## 3. Initial Device Setup

Initialize your node identity and local database:
```bash
# Default state directory is ~/.local/state/filesync
filesync init

# Validate configuration and file permissions
filesync config validate
```

Expected output:
```text
initialized device <64-char-hex-device-id> in ~/.local/state/filesync
configuration is valid: state_dir=~/.local/state/filesync device_id=<id> key_pin=<pin>
```

---

## 4. Enabling Background Service and Lingering

File Sync operates as a systemd user service.

### Step 1: Enable User Session Lingering (CRITICAL for Headless / VPS / Pi)
By default, systemd terminates user processes when an SSH session logs out. To allow the background sync daemon to start on system boot and continue running in the background across terminal disconnects:
```bash
loginctl enable-linger $USER
```
*(Documented owner step: File Sync avoids privileged system modifications and does not enable lingering automatically.)*

### Step 2: Enable and Start the User Service
```bash
systemctl --user daemon-reload
systemctl --user enable --now filesync.service
```

### Step 3: Verify Service Health
```bash
systemctl --user status filesync.service
filesync doctor
```

---

## 5. Network and Firewall Guidance

- **Control Interface**: Binds strictly to loopback (`127.0.0.1:8080`). Never expose the control port to public interfaces.
- **Peer Listener**: Disabled by default. If participating in multi-host replication, configure an explicit address (e.g. `--peer-listen 0.0.0.0:8443` or private Wireguard/Tailscale IP).
- **Firewall Notice**: File Sync does not make automatic privileged modifications to iptables, nftables, or UFW. If peer listening is enabled, ensure the chosen TCP port is open in your local firewall.
