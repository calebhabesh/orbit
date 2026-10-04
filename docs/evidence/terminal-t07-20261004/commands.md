# T07 Execution Commands and Intermediate Dispositions

Executed: 2026-10-04.
All tests executed in disposable roots on local Linux system; no destructive actions against personal files or live services.

## Validation Commands Executed

```sh
# 1. Detail observation baseline verification
ORBIT_TERMINAL_BASELINE=1 go test -v ./tests/terminal -run '^TestTerminalT00DetailObservation$'
# Output: PASS (0.018s)

# 2. Targeted T07 test run (consecutive count=2)
go test -count=2 -v ./tests/terminal -run '^TestTerminalT07' > docs/evidence/terminal-t07-20261004/packet.log 2>&1
# Output: PASS (0.935s)

# 3. Uncached T07 race detection
go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT07' > docs/evidence/terminal-t07-20261004/packet-race.log 2>&1
# Output: PASS (2.294s)

# 4. Control unit tests
go test -v ./internal/control/... > docs/evidence/terminal-t07-20261004/control-unit.log 2>&1
# Output: PASS (0.426s)

# 5. CLI commands tests
go test -v ./cmd/filesync/... > docs/evidence/terminal-t07-20261004/cmd-filesync.log 2>&1
# Output: PASS (0.044s)

# 6. Scoped code vet
go vet ./internal/control/... ./cmd/filesync/... ./tests/terminal/... > docs/evidence/terminal-t07-20261004/scoped-vet.log 2>&1
# Output: PASS (exit 0)

# 7. Full terminal test package
go test -count=1 ./tests/terminal
# Output: PASS (132.308s)

# 8. Full repository check
make check > docs/evidence/terminal-t07-20261004/make-check.log 2>&1
# Output: PASS (exit 0; fmt-check, vet, test, test-integration, test-model, test-faults, test-harness, build, build-arm64, package)

# 9. Full repository race detection
make test-race > docs/evidence/terminal-t07-20261004/make-test-race.log 2>&1
# Output: PASS (exit 0; race detection across all packages including tests/terminal in 178.420s)
```

## Intermediate Failures and Resolutions

1. **Flag Parsing Positional Value Consumption**:
   - *Failure*: In `handleOrbitStatus`, flag arguments starting with `-` were appended to `flagArgs`, but trailing arguments without `-` (such as `/path/to/state` following `--state`) were captured as `positionalFolder`, causing `--state` to consume `--json` as the directory path.
   - *Resolution*: Implemented `parsePositionalFolder` helper across `handleOrbitStatus`, `handleOrbitFoldersPause`, `handleOrbitFoldersResume`, and `handleOrbitFoldersRevalidate` that checks known value flags (`-state`, `--state`, `-folder`, `--folder`, `-limit`, `--limit`, `-cursor`, `--cursor`, `-reason`, `--reason`) and advances index `i` to attach the flag value directly to `flagArgs`.
2. **Pending Enrollment Requests Dual-Store Query**:
   - *Failure*: `AWAITING_APPROVAL` attention items were initially queried exclusively from terminal v2 enrollment metadata (`installation_metadata` under `enrollment/v2/request/%`), missing requests recorded directly in the `enrollment_requests` table via `db.RecordEnrollmentRequest`.
   - *Resolution*: Augmented `terminalAttention` and `checkFolderApprovalAndRevisions` to query both `terminalRequests` and `db.ListEnrollmentRequests(ctx, folder, "pending")`, mapping both store representations into unified `AWAITING_APPROVAL` attention items.
3. **Doctor Lifecycle State in Freshly Initialized Repositories**:
   - *Failure*: `checkDaemonLocalControl` reported `StatusWarn` when `control.addr` was absent, causing `p13_control_test.go` (`TestP13CLIDiagnosticsAndDoctor`) to fail because a newly initialized repository without a running daemon expected an overall status of `StatusOk`.
   - *Resolution*: Configured `checkDaemonLocalControl` so that an inactive daemon without `control.addr` reports `StatusOk` ("background daemon inactive"), while active daemons verify address and token permissions (reporting `StatusFail` if permissions are insecure). Updated `cmd/filesync/main.go`'s `handleDoctor` to pass `StoppedAdapter: true`.
4. **Doctor Root Revalidation Inode Sensitivity**:
   - *Failure*: In `TestTerminalT07_Doctor_DiagnosticsAndRemediations`, removing a root directory with `os.RemoveAll` and re-creating it with `os.MkdirAll` resulted in a new inode, correctly triggering `STALE_ROOT` on validation and failing the subsequent check.
   - *Resolution*: Isolated doctor test assertions into independent `t.Run` subtests (`StoppedAdapter`, `LiveCleanState`, `InsecureToken`, `MissingRoot`, `ExhaustedTask`) each operating on distinct clean disposable fixtures.

## Remaining Limitations

- Physical cross-host networking (multi-machine LAN, real Tailscale mesh) and native OS service integration (systemd login/logout/linger boots) remain deferred to T12/T13.
- Bounded interactive conflict review, editor sessions, and restore workflows are assigned to T08.
- P17 actual owner production use and unaided explanation work remains outstanding.
