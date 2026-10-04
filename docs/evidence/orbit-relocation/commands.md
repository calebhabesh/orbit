# Executed relocation checks — 2026-10-03

Base commit: `86ae552`; validation used the modified working tree. All final
commands below exited 0. All filesystem fixtures were disposable.

## `go test -count=1 -v ./internal/workspace ./cmd/filesync -run TestRelocation`

```text
=== RUN   TestRelocationPreservesHistoryAndRecovery
=== RUN   TestRelocationPreservesHistoryAndRecovery/#00
=== RUN   TestRelocationPreservesHistoryAndRecovery/relocation.prepared
=== RUN   TestRelocationPreservesHistoryAndRecovery/relocation.moved
=== RUN   TestRelocationPreservesHistoryAndRecovery/relocation.committed
--- PASS: TestRelocationPreservesHistoryAndRecovery (0.05s)
    --- PASS: TestRelocationPreservesHistoryAndRecovery/#00 (0.01s)
    --- PASS: TestRelocationPreservesHistoryAndRecovery/relocation.prepared (0.01s)
    --- PASS: TestRelocationPreservesHistoryAndRecovery/relocation.moved (0.01s)
    --- PASS: TestRelocationPreservesHistoryAndRecovery/relocation.committed (0.01s)
=== RUN   TestRelocationDestinationRefusalsAndPausedState
--- PASS: TestRelocationDestinationRefusalsAndPausedState (0.01s)
=== RUN   TestRelocationWaitsForCapture
--- PASS: TestRelocationWaitsForCapture (0.04s)
=== RUN   TestRelocationAcrossFilesystems
--- PASS: TestRelocationAcrossFilesystems (0.01s)
=== RUN   TestRelocationCrossFilesystemRecoveryAndCopyRefusal
=== RUN   TestRelocationCrossFilesystemRecoveryAndCopyRefusal/relocation.prepared
=== RUN   TestRelocationCrossFilesystemRecoveryAndCopyRefusal/relocation.moved
=== RUN   TestRelocationCrossFilesystemRecoveryAndCopyRefusal/relocation.committed
=== RUN   TestRelocationCrossFilesystemRecoveryAndCopyRefusal/editor-change
=== RUN   TestRelocationCrossFilesystemRecoveryAndCopyRefusal/symlink
--- PASS: TestRelocationCrossFilesystemRecoveryAndCopyRefusal (0.05s)
    --- PASS: TestRelocationCrossFilesystemRecoveryAndCopyRefusal/relocation.prepared (0.01s)
    --- PASS: TestRelocationCrossFilesystemRecoveryAndCopyRefusal/relocation.moved (0.01s)
    --- PASS: TestRelocationCrossFilesystemRecoveryAndCopyRefusal/relocation.committed (0.01s)
    --- PASS: TestRelocationCrossFilesystemRecoveryAndCopyRefusal/editor-change (0.01s)
    --- PASS: TestRelocationCrossFilesystemRecoveryAndCopyRefusal/symlink (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/internal/workspace	0.174s
=== RUN   TestRelocationCLIStoppedAndLive
--- PASS: TestRelocationCLIStoppedAndLive (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/cmd/filesync	0.016s
```

## `go test -race -count=1 ./internal/workspace ./internal/scheduler ./internal/control ./cmd/filesync`

```text
ok  	github.com/calebhabesh/file-sync/internal/workspace	15.687s
ok  	github.com/calebhabesh/file-sync/internal/scheduler	12.460s
ok  	github.com/calebhabesh/file-sync/internal/control	13.911s
ok  	github.com/calebhabesh/file-sync/cmd/filesync	2.021s
```

## `node scripts/orbit_ui_test.mjs --scenario relocation`

```text
[INFO] Test environment initialized: /tmp/orbit-o04-ui-7AJMwn
[INFO] Using Chromium: /usr/bin/chromium
[INFO] Preexisting files prepared in: /tmp/orbit-o04-ui-7AJMwn/sync-root
[HTTP] 400 {"code":"RELOCATION_FAILED","message":"destination overlaps the current folder or private state directory","retryable":false,"action":"check the folder locations and retry; keep original and staging folders until recovery finishes"}

  ✓ Settings relocation: focus, retained errors, move, success feedback and new-location capture
```

## `make check`

Exit 0. Passed format/vet, unit/integration/model/fault/harness checks, builds
and packaging. Final output:

```text
Generating filesync-1.0.0-1.aarch64.rpm...
Generating SHA256SUMS...
Release packaging complete! Generated artifacts:
  - filesync-v1.0.0-linux-amd64.tar.gz (13192961 bytes)
  - orbit-v1.0.0-linux-amd64.tar.gz (13192961 bytes)
  - filesync_1.0.0_amd64.deb (13188404 bytes)
  - filesync-1.0.0-1.x86_64.rpm (26373565 bytes)
  - filesync-v1.0.0-linux-arm64.tar.gz (12372807 bytes)
  - orbit-v1.0.0-linux-arm64.tar.gz (12372807 bytes)
  - filesync_1.0.0_arm64.deb (12368260 bytes)
  - filesync-1.0.0-1.aarch64.rpm (24735696 bytes)
  - release-manifest.json (385 bytes)
  - SHA256SUMS
```

## Desktop helper probe

A temporary marked PATH containing a harmless xdg-open executable logged
arguments to a temporary file. Running only TestOrbitSetup_OpenLocalFolder
with simulated DISPLAY/WAYLAND_DISPLAY invoked that helper before the fix.
After the test cleared both variables, the same probe reported:

```text
ok github.com/calebhabesh/file-sync/internal/control
PASS: no desktop helper invoked even with inherited desktop environment
```
