# Packet O12: Executed Verification Commands

This document records the exact commands, execution outputs, and exit codes for Packet O12 verification.

---

## 1. Embedded Web Console Rebuild

```sh
cd web && npm ci && npm run build && cd ..
```

**Output:**
```text
added 49 packages, and audited 50 packages in 691ms

16 packages are looking for funding
  run `npm fund` for details

found 0 vulnerabilities

> file-sync-web@1.0.0 build
> tsc && vite build

vite v8.3.0 building client environment for production...
✓ 38 modules transformed.
rendering chunks (1)...computing gzip size...
dist/index.html                   0.82 kB │ gzip:   0.50 kB
dist/assets/index-D4clrXK4.css    3.06 kB │ gzip:   1.23 kB
dist/assets/index-DYl1kZ4C.js   491.31 kB │ gzip: 116.45 kB

✓ built in 99ms
```
**Exit Code:** 0

---

## 2. Packaging Pipeline & Reproducible Artifacts

```sh
make package
```

**Output:**
```text
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01' -o bin/filesync ./cmd/filesync
ln -sf filesync bin/orbit
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01' -o bin/filesync-linux-arm64 ./cmd/filesync
go run ./scripts/build_packages.go
Building binary for linux/amd64 -> <repo>/bin/filesync
Building binary for linux/arm64 -> <repo>/bin/filesync-linux-arm64
Generating filesync-v1.0.0-linux-amd64.tar.gz...
Generating filesync_1.0.0_amd64.deb...
Generating filesync-1.0.0-1.x86_64.rpm...
Generating filesync-v1.0.0-linux-arm64.tar.gz...
Generating filesync_1.0.0_arm64.deb...
Generating filesync-1.0.0-1.aarch64.rpm...
Generating SHA256SUMS...
Release packaging complete! Generated artifacts:
  - filesync-v1.0.0-linux-amd64.tar.gz (13141800 bytes)
  - orbit-v1.0.0-linux-amd64.tar.gz (13141800 bytes)
  - filesync_1.0.0_amd64.deb (13137494 bytes)
  - filesync-1.0.0-1.x86_64.rpm (26265443 bytes)
  - filesync-v1.0.0-linux-arm64.tar.gz (12344470 bytes)
  - orbit-v1.0.0-linux-arm64.tar.gz (12344470 bytes)
  - filesync_1.0.0_arm64.deb (12340170 bytes)
  - filesync-1.0.0-1.aarch64.rpm (24676832 bytes)
  - release-manifest.json (385 bytes)
  - SHA256SUMS
```
**Exit Code:** 0

---

## 3. Orbit Version and Release Manifest

```sh
./bin/orbit version
./bin/orbit version --json
```

**Output:**
```text
Orbit Personal File Manager v1.0.0 (filesync compat v1.0.0)
Commit: 0a16d84 (built 2026-10-01)
Runtime: linux/amd64 (go1.27.1-X:nodwarf5, pure-Go SQLite, zero Node runtime)
Schema: SQLite user_version 13, Config format 1
Embedded Assets: 3 files (SHA-256: 42ca7a842b9dce46c52461444f1bff40cfc8ac0940d438ab834c9db3a39dea92)

{"built":"2026-10-01","commit":"0a16d84","config_format_version":1,"embedded_assets":{"digest_sha256":"42ca7a842b9dce46c52461444f1bff40cfc8ac0940d438ab834c9db3a39dea92","node_runtime_required":false,"pure_go_sqlite":true,"total_files":3},"go_version":"go1.27.1-X:nodwarf5","goarch":"amd64","goos":"linux","product":"Orbit","schema_version":13,"version":"1.0.0"}
```
**Exit Code:** 0

---

## 4. O12 Integration Test Suite

```sh
go test -count=1 -v ./tests/integration -run 'TestOrbitPackaging|TestOrbitTarball|TestOrbitVersion|TestOrbitInstall|TestOrbitLegacy|TestOrbitSchema|TestOrbitStopped'
```

**Output:**
```text
=== RUN   TestOrbitPackagingArtifacts
--- PASS: TestOrbitPackagingArtifacts (1.73s)
=== RUN   TestOrbitTarballContentsAndSymlinks
--- PASS: TestOrbitTarballContentsAndSymlinks (0.07s)
=== RUN   TestOrbitVersionAndManifestMetadata
--- PASS: TestOrbitVersionAndManifestMetadata (0.02s)
=== RUN   TestOrbitInstallAndUninstallScriptLifecycle
--- PASS: TestOrbitInstallAndUninstallScriptLifecycle (0.04s)
=== RUN   TestOrbitLegacyStateAdoption
--- PASS: TestOrbitLegacyStateAdoption (0.01s)
=== RUN   TestOrbitSchemaRollbackRefusal
--- PASS: TestOrbitSchemaRollbackRefusal (0.00s)
=== RUN   TestOrbitStoppedMetadataRestoreFreshIdentity
--- PASS: TestOrbitStoppedMetadataRestoreFreshIdentity (0.03s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/integration	1.892s
```
**Exit Code:** 0

---

## 5. Native User-Service Lifecycle Validation

```sh
python3 scripts/validation/service_lifecycle.py --hosts local --output /tmp/service-lifecycle-test-o12
```

**Output:**
```text
PASS native user-service lifecycle: local
```
**Exit Code:** 0

---

## 6. Complete Verification Gate

```sh
make check
```

**Output:**
```text
go vet ./...
PASS
ok  	github.com/calebhabesh/file-sync/tests/faults	1.078s
python3 -m unittest discover -s scripts/validation -p 'test_*.py'
...
----------------------------------------------------------------------
Ran 3 tests in 0.001s

OK
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01' -o bin/filesync-linux-arm64 ./cmd/filesync
go run ./scripts/build_packages.go
...
Release packaging complete! Generated artifacts:
...
```
**Exit Code:** 0

---

## 7. Race Detector Verification

```sh
make test-race
```

**Output:**
```text
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=0a16d84 -X main.date=2026-10-01' -o bin/filesync ./cmd/filesync
ln -sf filesync bin/orbit
go test -race ./...
ok  	github.com/calebhabesh/file-sync/cmd/filesync	1.754s
ok  	github.com/calebhabesh/file-sync/internal/control	18.182s
ok  	github.com/calebhabesh/file-sync/internal/replication	70.520s
ok  	github.com/calebhabesh/file-sync/internal/repository	66.628s
ok  	github.com/calebhabesh/file-sync/internal/scheduler	13.531s
ok  	github.com/calebhabesh/file-sync/internal/workspace	15.316s
ok  	github.com/calebhabesh/file-sync/tests/designgates	1.563s
ok  	github.com/calebhabesh/file-sync/tests/faults	33.372s
ok  	github.com/calebhabesh/file-sync/tests/integration	55.414s
```
**Exit Code:** 0

---

## 8. Multi-Process Replication Demo

```sh
make demo
```

**Output:**
```text
[Step 1] Initializing Node A (Alice) and Node B (Bob) with mutual TLS
[Step 2] Demonstrating normal file creation and verified transfer
[Step 3] Simulating offline partition & concurrent edits on both nodes
[Step 4] Reconnecting nodes: bidirectional gossip and conflict detection
[Step 5] Operator performs reviewed conflict resolution via CLI
[Step 6] Demonstration complete: Clean process shutdown and disposable teardown
[PASS] Local multi-process demo completed successfully.
```
**Exit Code:** 0
