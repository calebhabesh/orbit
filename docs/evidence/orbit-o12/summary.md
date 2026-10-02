# Packet O12 Evidence Summary: Orbit Packages, Legacy Adoption, and Executable Documentation

**Packet:** O12  
**Status:** `complete`  
**Requirements Satisfied:** U01 (single binary / zero Node), U08 (desktop launcher & icon), U12 (service management & migration), U13 (backup & export), U14 (maintenance & diagnostics), U16 (retention & data preservation on uninstall), S20 (backup integrity), S21 (uninstall data preservation), S22 (reproducible packaging & checksums).  
**Invariants Verified:** I08 (causal counter monotonicity across identity reset), I19 (CLI & UI parity), I20 (crash consistency, schema rollback refusal), I21 (bootstrap security & daemon session independence), I28 (safe bounded record pruning).

---

## 1. Overview & Objectives

Packet O12 establishes the release packaging pipeline, desktop integration, single user-service adoption, and documentation for the Orbit personal file manager, fulfilling Gate G05 compatibility guarantees:
- Packages Orbit entry/desktop launcher, embedded frontend, scalable SVG icon, and user-service integration for Linux `amd64` and `arm64`.
- Freezes existing CLI, service unit, and storage compatibility under Gate G05: both `orbit` and `filesync` binary symlinks, service unit names, and commands function interchangeably.
- Guarantees zero Node.js / npm runtime requirement for end users. The static React/TypeScript Vite application is compiled directly into the binary via Go embed.
- Emits reproducible release artifacts with `release-manifest.json` and cryptographic `SHA256SUMS`.
- Rewrites operator runbooks and `README.md` around first-class `orbit` commands, marking legacy `filesync` names as compatibility details.
- Validates legacy database state adoption (schema 5 → 13), schema rollback refusal (`user_version = 14` rejected), stopped metadata restore with fresh identity generation, and user data preservation across uninstallation.

---

## 2. Release Packaging Pipeline & Reproducible Artifacts

The packaging tool ([`scripts/build_packages.go`](file://<repo>/scripts/build_packages.go)) builds deterministic artifacts for target Linux architectures (`amd64` and `arm64`):

1. **Tarball Distributions (`.tar.gz`):**
   - `filesync-v1.0.0-linux-amd64.tar.gz` / `orbit-v1.0.0-linux-amd64.tar.gz`
   - `filesync-v1.0.0-linux-arm64.tar.gz` / `orbit-v1.0.0-linux-arm64.tar.gz`
   - Archive layout contains `bin/filesync`, `bin/orbit` symlink, `desktop/orbit.desktop`, `icons/orbit.svg`, `systemd/orbit.service`, `systemd/filesync.service`, `install.sh`, `uninstall.sh`, `release-manifest.json`, `README.md`, `LICENSE`, `NOTICE`, and `LICENSES.md`.
2. **Debian Packages (`.deb`):**
   - `filesync_1.0.0_amd64.deb` / `filesync_1.0.0_arm64.deb`
   - Installs `/usr/bin/orbit`, `/usr/bin/filesync` symlink, `/usr/share/applications/orbit.desktop`, `/usr/share/icons/hicolor/scalable/apps/orbit.svg`, `/usr/lib/systemd/user/orbit.service`, `/usr/share/doc/filesync/release-manifest.json`.
   - `postinst` triggers desktop database update and reloads systemd user units; `prerm` stops services cleanly.
3. **RPM Packages (`.rpm`):**
   - `filesync-1.0.0-1.x86_64.rpm` / `filesync-1.0.0-1.aarch64.rpm`
   - Packages binaries, desktop launchers, scalable icons, and systemd user services.
4. **Reproducibility & Verification:**
   - Deterministic archive timestamps (`1790208000` / 2026-09-23T00:00:00Z).
   - `dist/SHA256SUMS` records cryptographic digests for all artifacts.

---

## 3. Desktop Integration, Launcher, and User-Service Adoption

1. **Desktop Entry (`packaging/desktop/orbit.desktop`):**
   - FreeDesktop compliant:
     ```ini
     [Desktop Entry]
     Version=1.0
     Type=Application
     Name=Orbit
     Comment=Personal Peer-to-Peer File Synchronizer
     Exec=orbit launch
     Icon=orbit
     Terminal=false
     Categories=Network;FileTransfer;Utility;
     Keywords=sync;files;storage;p2p;cloud;backup;orbit;
     ```
2. **Scalable Vector Icon (`packaging/icons/orbit.svg`):**
   - Clean, dark-mode SVG badge with central device nucleus, dual orbital trajectory rings, and synchronized peer satellites.
3. **Desktop Launch Command (`orbit launch`):**
   - Dispatches browser opening (`xdg-open` / browser detection) to `http://127.0.0.1:8080`.
   - Automatically probes if the background daemon is active; if stopped, starts the daemon before launching the browser.
4. **Single Service Unit Adoption (Gate G05):**
   - `orbit.service` provides `Alias=filesync.service`, and `filesync.service` provides `Alias=orbit.service`.
   - Operating system tools (`systemctl --user {start|stop|restart|status} orbit.service` or `filesync.service`) control the single background daemon process, preventing accidental duplicate daemon instances.

---

## 4. Embedded Frontend Freshness & Metadata

1. **Deterministic SHA-256 Digest Calculation ([`web/embed.go`](file://<repo>/web/embed.go)):**
   - Embedded assets are walked lexically and hashed at runtime/build-time to ensure frontend code reflects exact committed sources rather than stale committed bundles.
2. **Version Metadata ([`internal/control/server.go`](file://<repo>/internal/control/server.go), `GET /api/v1/version`):**
   - Reports product name ("Orbit"), version ("1.0.0"), commit hash, build date, `schema_version: 13`, `config_format_version: 1`, `pure_go_sqlite: true`, and `node_runtime_required: false`.
3. **CLI Version Subcommand:**
   - `orbit version` outputs human-readable runtime, architecture, schema, and asset hash.
   - `orbit version --json` outputs machine-readable JSON matching `/api/v1/version`.

---

## 5. Standalone Installer, Uninstaller, and Data Preservation (Invariant S21 / I20)

1. **`install.sh` ([`packaging/scripts/install.sh`](file://<repo>/packaging/scripts/install.sh)):**
   - Supports user-local (`~/.local/bin`) and system-wide (`/usr/local/bin`) installation modes without requiring root for single-user desktops.
   - Installs binaries, symlinks, desktop entry, icon, and systemd user unit.
   - Performs daemon reload and gentle service adoption (`try-restart`).
2. **`uninstall.sh` ([`packaging/scripts/uninstall.sh`](file://<repo>/packaging/scripts/uninstall.sh)):**
   - Cleanly stops and unregisters services. Removes executables, desktop entries, and icons.
   - **Data Preservation Guarantee (Invariant S21 / I20):** Explicitly leaves `~/.local/state/filesync`, `~/.filesync`, and all user workspace folders intact. Never authors deletion tombstones to the causal DAG.
3. **Integration Test ([`tests/integration/orbit_package_test.go`](file://<repo>/tests/integration/orbit_package_test.go#TestOrbitInstallAndUninstallScriptLifecycle)):**
   - Evaluated `install.sh` and `uninstall.sh` in isolated temporary `$HOME`. Verified files installed, services registered, executables verified, uninstall performed, and user workspace data intact.

---

## 6. Legacy State Adoption and Rollback Refusal

1. **Authentic Schema 5 Migration Adoption ([`tests/integration/orbit_package_test.go`](file://<repo>/tests/integration/orbit_package_test.go#TestOrbitLegacyStateAdoption)):**
   - Bootstrapped database at authentic schema 5 with legacy records.
   - Opened with modern repository: executed migrations 6 through 13 transactionally.
   - Verified schema updated to `user_version = 13` with all legacy devices, folders, and membership revisions preserved.
2. **Schema Rollback Refusal ([`tests/integration/orbit_package_test.go`](file://<repo>/tests/integration/orbit_package_test.go#TestOrbitSchemaRollbackRefusal)):**
   - Created database with future `user_version = 14`.
   - Attempted repository initialization: immediately failed with `ErrIncompatibleSchema` (`metadata schema is newer than this binary: database=14 binary=13`), preventing silent database corruption.
3. **Stopped Metadata Restore & Fresh Identity Reset (Invariant I08) ([`tests/integration/orbit_package_test.go`](file://<repo>/tests/integration/orbit_package_test.go#TestOrbitStoppedMetadataRestoreFreshIdentity)):**
   - Stopped service, backed up database with `orbit maintenance backup`.
   - Restored backup with `orbit maintenance restore-backup`.
   - Verified causal identity safely rotated: generated brand-new cryptographic Device ID and fresh TLS keypin, resetting `next_counter = 0` to prevent counter collisions.

---

## 7. Rewritten Documentation & Runbooks

All operator documentation was rewritten around Orbit commands and lifecycle operations:
- [`README.md`](file://<repo>/README.md): Product vision, package installation, desktop launcher, command quick reference, and runbook links.
- [`docs/runbooks/install.md`](file://<repo>/docs/runbooks/install.md): Package choices, desktop launch (`orbit launch`), service lingering, and initial device setup (`orbit init`).
- [`docs/runbooks/uninstall.md`](file://<repo>/docs/runbooks/uninstall.md): Safe package removal and Invariant S21 data preservation.
- [`docs/runbooks/database-recovery.md`](file://<repo>/docs/runbooks/database-recovery.md): Diagnostic steps, crash consistency replay, and stopped backup restore with identity rotation.
- [`docs/runbooks/rollback.md`](file://<repo>/docs/runbooks/rollback.md): Compatible rollback vs. incompatible schema refusal and safe backup recovery.
- [`docs/runbooks/upgrade.md`](file://<repo>/docs/runbooks/upgrade.md): Preflight check (`orbit maintenance preflight`), consistent backup, package update, and service health check.
- [`docs/runbooks/private-network.md`](file://<repo>/docs/runbooks/private-network.md): LAN, WireGuard, and Tailscale VPN topology, firewall verification without automatic host mutations.
- [`docs/runbooks/headless-pairing.md`](file://<repo>/docs/runbooks/headless-pairing.md): Interactive and non-interactive pairing over SSH with full CLI parity.
- [`docs/runbooks/lost-device-replacement.md`](file://<repo>/docs/runbooks/lost-device-replacement.md): Device retirement, key revocation, and replacement provisioning.

---

## 8. Verification Results

| Check / Test | Command | Result |
| --- | --- | --- |
| **Frontend Build** | `cd web && npm ci && npm run build && cd ..` | **PASS** (zero errors) |
| **Package Creation** | `make package` | **PASS** (8 packages + manifest + SHA256SUMS) |
| **Packaging & Migration Suite** | `go test -count=1 -v ./tests/integration -run 'TestOrbitPackaging\|...'` | **PASS** (7/7 tests passed) |
| **Service Lifecycle Validation** | `python3 scripts/validation/service_lifecycle.py --hosts local` | **PASS** (ordinary edit, service restart, uninstall) |
| **Verification Gate** | `make check` | **PASS** (fmt, vet, unit, integration, model, faults, arm64 cross-compile) |
| **Race Detector** | `make test-race` | **PASS** (0 race warnings across all packages) |
| **Multi-Process Demo** | `make demo` | **PASS** (TLS, sync, offline partition, conflict resolution) |

---

## 9. Remaining Limitations & Handoff to O13

- **P17 Personal Owner Pilot:** The unaided personal owner pilot on native hardware remains unexecuted and is tracked as prerequisite work for final release signoff in O13.
- **Next Eligible Packet:** **Packet O13: Failure campaign, usability pilot and final handoff** (`docs/implementation/orbit-release.md#o13--failure-campaign-usability-pilot-and-final-handoff`).
