# Packet P15: Executed Commands and Verifications

**Date:** 2026-09-23
**Environment:** Linux (Arch rolling, x86_64), Go 1.27.1

---

## 1. Static Verification and Compilation

### Command:
```bash
make check
```

### Actual Output:
```text
go vet ./...
go test ./internal/... ./model/...
?   	github.com/calebhabesh/file-sync/internal/app	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/config	(cached)
ok  	github.com/calebhabesh/file-sync/internal/control	0.247s
ok  	github.com/calebhabesh/file-sync/internal/history	(cached)
ok  	github.com/calebhabesh/file-sync/internal/protocol	(cached)
ok  	github.com/calebhabesh/file-sync/internal/replication	(cached)
ok  	github.com/calebhabesh/file-sync/internal/repository	(cached)
ok  	github.com/calebhabesh/file-sync/internal/scheduler	(cached)
ok  	github.com/calebhabesh/file-sync/internal/state	(cached)
ok  	github.com/calebhabesh/file-sync/internal/testkit	(cached)
ok  	github.com/calebhabesh/file-sync/internal/workspace	(cached)
ok  	github.com/calebhabesh/file-sync/model	(cached)
go test ./tests/integration/...
ok  	github.com/calebhabesh/file-sync/tests/integration	13.774s
CGO_ENABLED=0 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=7f0cb81 -X main.date=2026-09-23' -o bin/filesync ./cmd/filesync
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build  -trimpath -ldflags '-X main.version=1.0.0 -X main.commit=7f0cb81 -X main.date=2026-09-23' -o bin/filesync-linux-arm64 ./cmd/filesync
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
  - filesync-v1.0.0-linux-amd64.tar.gz (12295125 bytes)
  - filesync_1.0.0_amd64.deb (12293596 bytes)
  - filesync-1.0.0-1.x86_64.rpm (12289753 bytes)
  - filesync-v1.0.0-linux-arm64.tar.gz (11556706 bytes)
  - filesync_1.0.0_arm64.deb (11555196 bytes)
  - filesync-1.0.0-1.aarch64.rpm (11554385 bytes)
  - SHA256SUMS
```

---

## 2. Race Detection Verification

### Command:
```bash
make test-race
```

### Actual Output:
```text
go test -race ./...
?   	github.com/calebhabesh/file-sync/cmd/filesync	[no test files]
?   	github.com/calebhabesh/file-sync/internal/app	[no test files]
ok  	github.com/calebhabesh/file-sync/internal/config	(cached)
ok  	github.com/calebhabesh/file-sync/internal/control	14.071s
ok  	github.com/calebhabesh/file-sync/internal/history	(cached)
ok  	github.com/calebhabesh/file-sync/internal/protocol	(cached)
ok  	github.com/calebhabesh/file-sync/internal/replication	(cached)
ok  	github.com/calebhabesh/file-sync/internal/repository	(cached)
ok  	github.com/calebhabesh/file-sync/internal/scheduler	(cached)
ok  	github.com/calebhabesh/file-sync/internal/state	(cached)
ok  	github.com/calebhabesh/file-sync/internal/testkit	(cached)
ok  	github.com/calebhabesh/file-sync/internal/workspace	(cached)
ok  	github.com/calebhabesh/file-sync/model	(cached)
?   	github.com/calebhabesh/file-sync/scripts	[no test files]
ok  	github.com/calebhabesh/file-sync/tests/designgates	(cached)
ok  	github.com/calebhabesh/file-sync/tests/faults	(cached)
ok  	github.com/calebhabesh/file-sync/tests/integration	22.787s
?   	github.com/calebhabesh/file-sync/web	[no test files]
```

---

## 3. Dedicated P15 Integration Suite

### Command:
```bash
go test -v ./tests/integration -run "TestP15"
```

### Actual Output:
```text
=== RUN   TestP15VersionAndBuildMetadata
--- PASS: TestP15VersionAndBuildMetadata (0.00s)
=== RUN   TestP15ReleaseBinaryExcludesDestructiveTestHooks
--- PASS: TestP15ReleaseBinaryExcludesDestructiveTestHooks (0.00s)
=== RUN   TestP15EmbeddedUIWithoutNode
--- PASS: TestP15EmbeddedUIWithoutNode (0.11s)
=== RUN   TestP15ConfigValidation
--- PASS: TestP15ConfigValidation (0.01s)
=== RUN   TestP15AgentStop
--- PASS: TestP15AgentStop (0.21s)
=== RUN   TestP15UpgradePreflight
--- PASS: TestP15UpgradePreflight (0.02s)
=== RUN   TestP15InterruptedMigrationRollback
--- PASS: TestP15InterruptedMigrationRollback (0.00s)
=== RUN   TestP15ConsistentBackupAndRestoreSafety
--- PASS: TestP15ConsistentBackupAndRestoreSafety (0.02s)
=== RUN   TestP15PackagingOutputsAndChecksums
--- PASS: TestP15PackagingOutputsAndChecksums (1.08s)
=== RUN   TestP15UninstallPreservesUserData
--- PASS: TestP15UninstallPreservesUserData (0.02s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/integration	1.482s
```

---

## 4. Package Artifacts Verification and Checksums

### Command:
```bash
file dist/*
```

### Actual Output:
```text
dist/filesync-1.0.0-1.aarch64.rpm:       RPM v3.0 bin AArch64
dist/filesync-1.0.0-1.x86_64.rpm:        RPM v3.0 bin i386/x86_64
dist/filesync_1.0.0_amd64.deb:           Debian binary package (format 2.0), with control.tar.gz , data compression gz
dist/filesync_1.0.0_arm64.deb:           Debian binary package (format 2.0), with control.tar.gz , data compression gz
dist/filesync-v1.0.0-linux-amd64.tar.gz: gzip compressed data, original size modulo 2^32 21165056
dist/filesync-v1.0.0-linux-arm64.tar.gz: gzip compressed data, original size modulo 2^32 20024832
dist/SHA256SUMS:                         ASCII text
```

### Command:
```bash
cd dist && sha256sum -c SHA256SUMS
```

### Actual Output:
```text
filesync-v1.0.0-linux-amd64.tar.gz: OK
filesync_1.0.0_amd64.deb: OK
filesync-1.0.0-1.x86_64.rpm: OK
filesync-v1.0.0-linux-arm64.tar.gz: OK
filesync_1.0.0_arm64.deb: OK
filesync-1.0.0-1.aarch64.rpm: OK
```

---

## 5. Live CLI Lifecycle Verification

### Version Metadata Command:
```bash
bin/filesync version
```
```text
filesync 1.0.0 (commit=7f0cb81, built=2026-09-23, linux/amd64, go=go1.27.1-X:nodwarf5)
```

### Configuration Validation Command:
```bash
bin/filesync config validate --state /tmp/test-state
```
```text
configuration is valid: state_dir=/tmp/test-state device_id=756c9bc523324ee3130d428853d57a4d0c81b1087426c9d282b1069cf26d0ace key_pin=956f65e3ba01497b3cdf7e959ea708bd932ad861be17504ba412f9108cd19c51
```

### Upgrade Preflight Command:
```bash
bin/filesync maintenance preflight --state /tmp/test-state
```
```text
upgrade preflight: status=ready database_schema=10 binary_schema=10 agent_running=false integrity_clean=true free_space_mb=29296
next steps:
  - environment is ready for upgrade; create consistent backup ('filesync maintenance backup') and proceed
```

### Consistent Backup and Safe Restoration Command:
```bash
bin/filesync maintenance backup --state /tmp/test-state --out /tmp/backup.sqlite
bin/filesync maintenance restore-backup --state /tmp/test-state --backup /tmp/backup.sqlite
```
```text
backup created at /tmp/backup.sqlite (size=335872 bytes)
backup restored from /tmp/backup.sqlite
causal identity safely reset: old_device=756c9bc5... new_device=a809e552... key_pin=375710f4...
CRITICAL (Invariant I08): Rolled-back causal author counters have been retired.
Action required: re-enroll new device ID in folder memberships with peers (Invariant I08)
```
