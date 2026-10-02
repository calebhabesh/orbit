# Operator Runbook: Orbit Linux Installation and Lifecycle Setup

This runbook guides operators through clean installation, desktop integration, systemd user-service configuration, lingering setup, and initial device initialization on target Linux architectures (`amd64` / `x86_64` and `arm64` / `aarch64`).

## Prerequisites and Requirements

- **Supported Architectures**: Linux `amd64` (x86_64) or `arm64` (aarch64).
- **Runtime Dependencies**: Zero external runtime dependencies. Built statically with pure-Go SQLite (`modernc.org/sqlite`); requires no external C compiler, dynamic shared libraries, or libc coupling.
- **Embedded Web Console**: Self-contained React/TypeScript/Vite operator console is embedded directly inside the binary via Go embed; **zero Node.js or npm runtime is required**.
- **Desktop Integration**: Installs FreeDesktop-compliant desktop entry (`orbit.desktop`) and scalable application icon (`orbit.svg`) for system application menus and desktop application launchers.
- **Systemd Session**: Standard systemd user session (`systemd --user`).
- **Explicit Root Permissions**: User workspace folders can reside anywhere under the user's home directory (`~`).

---

## 1. Choosing a Package Format

Orbit distributes three reproducible packaging formats in `dist/` with cryptographic `SHA256SUMS` (retaining `filesync` package compatibility per Gate G05):

| Format | Target Systems | Package File | Installed Executables & Service |
| --- | --- | --- | --- |
| **Debian (`.deb`)** | Debian, Ubuntu, Raspberry Pi OS | `filesync_1.0.0_amd64.deb` (`arm64`) | `/usr/bin/orbit`, `/usr/bin/filesync`, `/usr/lib/systemd/user/orbit.service` |
| **RPM (`.rpm`)** | Fedora, RHEL, Rocky Linux, CentOS | `filesync-1.0.0-1.x86_64.rpm` (`aarch64`) | `/usr/bin/orbit`, `/usr/bin/filesync`, `/usr/lib/systemd/user/orbit.service` |
| **Tarball (`.tar.gz`)** | Any Linux (Arch, Alpine, headless/non-root) | `orbit-v1.0.0-linux-amd64.tar.gz` (`arm64`) | User-local (`~/.local/bin/orbit`) or system (`/usr/local/bin/orbit`) |

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
*(Installs `/usr/bin/orbit` and `/usr/bin/filesync` symlink, desktop entry `/usr/share/applications/orbit.desktop`, icon `/usr/share/icons/hicolor/scalable/apps/orbit.svg`, and systemd unit `/usr/lib/systemd/user/orbit.service` with `filesync.service` alias).*

### Option B: Fedora / RHEL / Rocky Linux (`.rpm`)

For `x86_64`:
```bash
sudo rpm -Uvh filesync-1.0.0-1.x86_64.rpm
```
For `aarch64`:
```bash
sudo rpm -Uvh filesync-1.0.0-1.aarch64.rpm
```
*(Installs `/usr/bin/orbit`, `/usr/bin/filesync`, desktop entry, icon, and systemd user service).*


### Option C: Standalone Tarball Installation (`.tar.gz`)

Extract the archive and run the included installer:
```bash
tar -xzf orbit-v1.0.0-linux-amd64.tar.gz
cd orbit-v1.0.0-linux-amd64

# Install for current user (no root required -> installs to ~/.local/bin and ~/.local/share)
./install.sh

# Or install system-wide (requires sudo -> installs to /usr/local/bin and /usr/share)
sudo ./install.sh system
```

The installer verifies prerequisites, installs binaries, registers the systemd user service, installs the desktop launcher and icon, and reloads the systemd user daemon.

---

## 3. Initial Device Setup and Launch

### Desktop Launch
Launch Orbit directly from your desktop application menu by clicking the **Orbit** icon, or from the terminal:
```bash
orbit launch
```
`orbit launch` starts the background daemon (if not already running) and opens the web management console in your default web browser (`http://127.0.0.1:8080`).

### Headless or Terminal Setup
Initialize your node identity and validate the local database:
```bash
# Default state directory is ~/.local/state/filesync
orbit init

# Validate configuration and file permissions
orbit config validate
```

Expected output:
```text
initialized device <64-char-hex-device-id> in ~/.local/state/filesync
configuration is valid: state_dir=~/.local/state/filesync device_id=<id> key_pin=<pin>
```

Verify service and system health:
```bash
orbit doctor
```

---

## 4. Enabling Background Service and Lingering

Orbit operates as a standard systemd user service (`orbit.service`, aliased as `filesync.service`).

### Step 1: Enable User Session Lingering (CRITICAL for Headless / VPS / Pi)
By default, systemd terminates user processes when an SSH session disconnects or logs out. To allow the background sync daemon to start on system boot and continue running in the background across terminal disconnects:
```bash
loginctl enable-linger $USER
```
*(Documented owner step: Orbit avoids privileged system modifications and does not enable lingering automatically.)*

### Step 2: Enable and Start the User Service
Using the `orbit service` command:
```bash
orbit service enable
orbit service start
```
Or directly via `systemctl`:
```bash
systemctl --user daemon-reload
systemctl --user enable --now orbit.service
```

### Step 3: Verify Service Health
```bash
orbit service status
orbit doctor
```

---

## 5. Network and Firewall Guidance

- **Control Interface**: Binds strictly to loopback (`127.0.0.1:8080`). The control port and web UI are never exposed to public or non-loopback interfaces.
- **Peer Listener**: Disabled by default. If participating in multi-host replication, configure an explicit address (e.g. `--peer-listen 0.0.0.0:8443` or a private WireGuard/Tailscale VPN address).
- **Private Network Runbook**: For comprehensive LAN/VPN topology guidance, firewall rules, and reachability diagnostics without automated host mutations, consult [`private-network.md`](private-network.md).
