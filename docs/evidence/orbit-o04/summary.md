# Orbit Packet O04 Summary: Orbit Shell and Complete First-Device UI

**Packet:** O04  
**Date:** 2026-10-02  
**Status:** `complete`  
**Prerequisites:** O00, O01, O02, O03 complete. Read [vision](../../orbit-product.md), [architecture](../../orbit-architecture.md), [operations](../../operations.md), [verification](../../verification.md), and [orbit foundations](../../implementation/orbit-foundations.md#o04--orbit-shell-and-complete-first-device-ui).  
**Requirements Satisfied:** U01 (single-device onboarding and status), U04 (monochrome baseline design system and fluid layout), U07 (responsive shell with 5 primary destinations), U08 (preexisting file adoption review), U15 (honest qualified progress and storage states), U16 (accessible interactive controls and keyboard escape).  
**Invariants Verified:**
- **I19 (CLI & UI Parity):** UI and CLI use identical control operations; no separate reconciliation logic client-side.
- **I21 (Token & Session Lifecycle):** Bootstrap token handoff via URL fragment `#bootstrap=<token>` stripped immediately with `history.replaceState` before rendering; zero secret exposure in browser history, logs, or exports; session logout leaves sync engine running.
- **I22 (Safe Root Selection & File Preservation):** Safe root selection rejects empty paths, state directory, and system directories (`/etc`, `/var`, `/usr`, `/proc`, `/dev`, etc.); displays adoption review for preexisting files; scans files without fabricating deletion tombstones.
- **I27 (Honest Recovery & Service States):** Complete and honest recovery, service status indicators, and retention state reporting.

---

## 1. System Overview

Orbit Packet O04 replaces the legacy operator console with a modern, production-ready, monochrome file manager interface designed for everyday users while preserving low-level diagnostic depth for advanced operators. Built entirely with React 19, TypeScript, and Vite 8, the web frontend compiles into static assets embedded directly into the Go executable via `//go:embed all:dist`.

### Core Operational Principles
1. **Zero Runtime Node.js Dependency:** Production binaries embed all compiled HTML, CSS, JavaScript, and SVG assets. At runtime, the daemon serves the full Single Page Application (SPA) directly over loopback HTTP with zero external process or Node dependencies.
2. **Decoupled Operator Session:** Closing the browser tab or clicking "Sign out of web session" invalidates the web cookie while background file watching, replication, and reconciliation continue uninterrupted.
3. **Exact Parity with CLI Engine:** Every UI action delegates to the exact same `/api/v1/...` control endpoints exercised by the `filesync` and `orbit` CLI commands.
4. **URL Fragment Security (I21):** Single-use bootstrap tokens passed via URL fragment (`http://127.0.0.1:38081/#bootstrap=<token>`) are exchanged immediately for a session cookie and stripped from `window.location` via `history.replaceState` before any view is rendered.
5. **Preexisting File Safety (I22):** Adopting an existing folder inspects directory contents and presents an explicit adoption notice without authoring deletion tombstones.

---

## 2. Views and Capabilities

The interface provides an onboarding wizard, 5 primary navigation destinations, a local directory picker, and version history drawers:

### A. Setup & Onboarding Wizard (`web/src/views/SetupWizard.tsx`)
- **Choice Entry:** Displays "Create a workspace" and "Join existing workspace" cards. The Join entry provides clear guidance on upcoming cross-device pairing (O05/O06) without displaying fake success screens.
- **Live Root Preview & Validation:** As users type a local directory path, the UI debounces and calls `POST /api/v1/orbit/setup/preview/create`. Rejects `/etc`, `/var`, `/usr`, `/proc`, `/dev`, state directory, and nested paths with clear error messages and disables submission.
- **Preexisting File Adoption Review:** Nonempty directories display an informative notice indicating that files will be adopted into immutable version history.
- **Observable Setup Progress:** When setup begins, the wizard transitions into an observable 4-step progress tracker displaying live phase execution:
  1. Creating workspace and folder registration
  2. Indexing and adopting existing files
  3. Configuring background service
  4. Finalizing workspace setup

### B. Orbit Navigation Shell (`web/src/components/OrbitSidebar.tsx`, `OrbitTopbar.tsx`)
- **Five Primary Destinations:**
  1. **Files:** Workspace file manager table with real-time sync status badges (`synced`, `syncing`, `paused`, `error`, `attention`), directory drill-down, and history drawer.
  2. **Needs Attention:** Centralized inbox grouping sync conflicts, paused roots, and doctor diagnostics.
  3. **Devices:** Node replica index displaying member names, current node indicators, and key fingerprints.
  4. **Deleted Files:** Historical tombstone index with one-click restoration as a fresh causal version.
  5. **Settings:** Device label editing, sync roots management, 7-point systemd user service indicators, storage garbage collection, diagnostic support export, and session logout.
- **Responsive Layout:** Responsive layout with desktop sidebar, fluid header breadcrumbs, search input, "Open Local Folder" desktop integration, and narrow-viewport mobile drawer navigation.

### C. Directory Picker Modal (`web/src/components/DirectoryPickerModal.tsx`)
- Interactive directory browser powered by `POST /api/v1/orbit/picker/browse`.
- Supports Linux path traversal, pagination (50 items/page), breadcrumb navigation, and keyboard `Escape` dismissal with visible focus return.

---

## 3. Verification and Evidence

### Planned Checks Execution

```sh
# 1. Compile frontend assets
cd web
npm ci
npm run build
cd ..

# 2. Build Go binary embedding new frontend
make build

# 3. Execute deterministic browser test runner
node scripts/orbit_ui_test.mjs --scenario setup
```

### Browser Test Runner Results (`scripts/orbit_ui_test.mjs`)

```text
[INFO] Test environment initialized: /tmp/orbit-o04-ui-zuCnFw
[INFO] Using Chromium: /usr/bin/chromium
[INFO] Preexisting files prepared in: /tmp/orbit-o04-ui-zuCnFw/sync-root
[SCENARIO: SETUP] Starting uninitialized daemon with --allow-init...
[INFO] Daemon control listener active at: http://127.0.0.1:38081
[INFO] Generated 1-use bootstrap token: 238593e0...
[INFO] Launching Chromium via puppeteer-core...
[STEP 1] Testing bootstrap token exchange via URL fragment...
  ✓ Bootstrap token was stripped from URL immediately without leaking
  ✓ Onboarding wizard entry screen rendered successfully
  ✓ Saved screenshot-01-setup-choice.png
[STEP 2] Testing Join entry availability notice...
  ✓ Join screen properly identifies pairing availability in upcoming release
  ✓ Returned to Create Workspace form
[STEP 3] Testing root path validation on disallowed directory (/etc)...
  ✓ Root validation correctly rejected disallowed path /etc
  ✓ Create button disabled for disallowed directory
  ✓ Saved screenshot-02-validation-error.png
[STEP 4] Entering valid sync root with preexisting files...
  ✓ Preexisting files detected and adoption notice displayed
  ✓ Saved screenshot-03-root-preview-adoption.png
[STEP 5] Testing Directory Picker modal and keyboard escape...
  ✓ Directory Picker modal opened
  ✓ Escape key successfully closed Directory Picker modal
[STEP 6] Executing setup creation...
  ✓ Progress step tracking displayed
  ✓ Saved screenshot-04-setup-progress.png
  ✓ Setup completed and Open Files button appeared
[STEP 7] Verifying main Orbit Files shell...
  ✓ Sidebar navigation loaded
  ✓ All 5 primary destinations present in sidebar (Files, Needs attention, Devices, Deleted files, Settings)
  ✓ Preexisting files (welcome.txt, notes.md) are visible and preserved
  ✓ Saved screenshot-05-files-shell.png
[STEP 8] Testing navigation to Needs Attention, Devices, and Settings...
  ✓ Needs attention view verified and screenshot saved
  ✓ Devices view verified and screenshot saved
  ✓ Settings view with 7 service status indicators verified
[STEP 9] Testing narrow browser window layout (375x667 mobile viewport)...
  ✓ Mobile navigation toggle active on narrow viewport
  ✓ Saved screenshot-09-narrow-window-mobile.png
[STEP 10] Verifying database records and captured version history...
  ✓ Database inspection: folders=1, versions=4, projections=4
  ✓ Product settings verified: device_label="linux-workstation"

========================================================
  ✓ ALL O04 BROWSER & SHELL ACCEPTANCE CRITERIA PASSED
========================================================
```

---

## 4. Screenshot Artifacts

All screenshots captured during automated browser verification are saved in `docs/evidence/orbit-o04/screenshots/`:

| Artifact | Resolution | Description |
| :--- | :--- | :--- |
| `screenshot-01-setup-choice.png` | 1280x800 | Onboarding choice screen: "Create a workspace" vs "Join existing workspace" |
| `screenshot-02-validation-error.png` | 1280x800 | Disallowed path validation (`/etc` rejected, button disabled) |
| `screenshot-03-root-preview-adoption.png` | 1280x800 | Sync root preview displaying adoption review for preexisting files |
| `screenshot-04-setup-progress.png` | 1280x800 | Observable 4-step setup creation progress tracker |
| `screenshot-05-files-shell.png` | 1280x800 | Main Orbit Files shell displaying adopted workspace items and 5 sidebar destinations |
| `screenshot-06-needs-attention.png` | 1280x800 | Needs Attention inbox view displaying zero active conflicts and system status |
| `screenshot-07-devices-view.png` | 1280x800 | Devices view listing active replica node and pairing guidance |
| `screenshot-08-settings-view.png` | 1280x800 | Settings view displaying 7-point service status, storage reserve, and support bundle tools |
| `screenshot-09-narrow-window-mobile.png` | 375x667 | Narrow mobile viewport verifying responsive layout and mobile drawer navigation |

---

## 5. Backend & Integration Tests

```sh
go test -count=1 -v ./internal/control ./tests/integration -run 'TestOrbit'
```
*Result:* 20/20 test groups passed.

---

## 6. Limitations & Follow-Up Work

1. **Cross-Device Pairing (O05/O06):** Onboarding "Join workspace" displays an informative notice explaining pairing availability in Packet O05. The linear membership rollout and QR/string enrollment flow will be implemented in O05 and O06.
2. **Native Notification Integration (O07):** Desktop notification dispatch via DBus/libnotify is scheduled for O07.
