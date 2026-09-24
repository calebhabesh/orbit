# Packet P14 Verification Commands and Execution Evidence

This document records the exact commands executed to build, run, test, and capture evidence for **Packet P14 (Focused embedded web interface)**.

---

## 1. Frontend Asset Build and Embedded Compilation

```bash
# 1. Build frontend bundle from TypeScript and Vite into web/dist/
npm --prefix web run build

# Output:
# > file-sync-web@1.0.0 build
# > tsc && vite build
# vite v8.3.0 building client environment for production...
# ✓ 26 modules transformed.
# rendering chunks (1)...computing gzip size...
# dist/index.html                   0.82 kB │ gzip:  0.50 kB
# dist/assets/index-Blg8vsdD.css    7.51 kB │ gzip:  2.03 kB
# dist/assets/index-BUpGrZiR.js   274.21 kB │ gzip: 80.11 kB
# ✓ built in 97ms

# 2. Build single self-contained Go binary with embedded assets
make build
# Output:
# CGO_ENABLED=0 go build -trimpath -ldflags '-X main.version=dev' -o bin/filesync ./cmd/filesync
```

---

## 2. Automated Test Execution

### A. Full Verification Suite (`make check`)
```bash
make check
```
**Result:** `0` (Exit Success)
- `go vet ./...`: PASS
- Unit & Model tests: PASS
- Integration tests (`tests/integration/...` P00–P14): PASS (12.291s)
- AMD64 production build: PASS
- ARM64 cross-compilation: PASS

### B. Race Detector Verification (`make test-race`)
```bash
make test-race
```
**Result:** `0` (Exit Success)
- All packages and integration test suites run under `go test -race ./...` with 0 data race warnings.

### C. Fault Boundary & Crash Tests (`make test-faults`)
```bash
make test-faults
```
**Result:** `0` (Exit Success)
- Design gates D1–D5 passed.
- P03, P04, P06 SIGKILL crash boundary restarts passed.

### D. P14 Embedded Web Test Suite
```bash
go test -v -count=1 ./tests/integration/... -run TestP14
```
**Result:** `0` (Exit Success)
- `TestP14EmbeddedWebInterfaceAndSPARouting`: PASS
  - Verifies embedded assets served from binary without Node.js runtime.
  - Verifies unauthenticated GET `/` returns HTML SPA shell with `root` element and script tags.
  - Verifies client-side route fallback (`/folders`, `/files`, `/conflicts`) to `index.html`.
  - Verifies correct MIME types (`text/html`, `application/javascript`, `text/css`).
  - Verifies 404 on missing assets under `/assets/missing.js`.
  - Verifies Host header and Origin header security enforcement.
- `TestP14EndToEndConflictWorkflows`: PASS
  - Verifies Select Winner workflow creating causal resolution envelope superseding concurrent heads.
  - Verifies Keep Separate Copies workflow branching files side-by-side with author fingerprints.
  - Verifies Manual Merge workflow with custom operator-supplied content.
  - Verifies Stale-View rejection (409 Conflict / `STALE_VIEW`) when conflict heads change before resolution.
- `TestP14FilesInspectionAndRestoreWorkflow`: PASS
  - Verifies `GET /api/v1/files` active workspace file projection listing.
  - Verifies executable bit detection (`+x`).
  - Verifies file history inspection with content availability flags (`ready`, `pending`, `unavailable`, `expired`).
  - Verifies `POST /api/v1/restore/preview` and `POST /api/v1/restore` creating forward resolution version.
- `TestP14FilenameMarkupEscaping`: PASS
  - Verifies workspace filenames containing `<script>alert('xss').txt`, double quotes, and emoji characters are safely recorded, projected, and served without HTML markup injection vulnerabilities.

---

## 3. UI Screenshot Capture Workflow

Screenshots were captured using headless Chromium driven by Puppeteer against a running `filesync serve` instance:

```bash
node scripts/capture_screenshots.js
```

**Captured Artifacts:**
1. `screenshot-01-bootstrap-login.png`: One-time bootstrap token authentication modal with CLI instruction helper.
2. `screenshot-02-folders-work.png`: Folders table, lifecycle actions (Pause, Revalidate, Run GC, Remove), qualified peer progress, durable work queue, and storage usage cards.
3. `screenshot-03-doctor-modal.png`: Comprehensive doctor diagnostic health inspection report modal with categorized checks and OK statuses.
4. `screenshot-04-files-history.png`: Files listing, search filter, executable indicators, and detailed version history drawer with causal heads and content availability.
5. `screenshot-05-restore-preview.png`: Historical restore preview modal showing superseded heads, content readiness, head token, and forward-causal explanation note.
6. `screenshot-06-conflicts-view.png`: Active multi-head causal conflicts and structural conflict table with head tokens and suppression indicators.
7. `screenshot-07-conflict-resolve-modal.png`: Conflict resolution modal with three workflows: Select Winner (with radio buttons), Keep Separate Copies (dry run targets), and Manual Merge.
8. `screenshot-08-mobile-responsive.png`: Responsive layout on 390px mobile viewport with accessible touch targets, flex wrapping, and responsive tables.
