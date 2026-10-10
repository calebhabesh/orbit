# E13 commands — 2026-10-09

Workspace: `/home/ethioking/dev/orbit`, base `b962168`, uncommitted prior
2.3.0 onboarding work retained. All Go/PTY temporary roots use
`TMPDIR=/home/ethioking/.cache/orbit-work-tmp`; `/tmp` had a user quota issue.
No unrelated temporary folders were removed. Destructive fault harnesses use
marked disposable roots; trial hosts are only backed up, installed and inspected.

## Focused and regression checks

```sh
go test -race -count=1 ./cmd/orbit ./internal/control ./internal/replication \
  ./internal/repository ./internal/scheduler ./internal/terminal ./internal/workspace \
  -run '^TestOnboardingE(11|12|13)'
go test -race -count=1 ./internal/control ./internal/replication \
  ./internal/repository ./internal/scheduler \
  -run '^(TestOnboardingE13.*|TestCoalescingCannotRedispatchRunningPeerSync|TestGCSuspendedDuringMaintenance)$'
go test -race -count=1 ./internal/repository ./internal/scheduler \
  ./internal/workspace ./internal/terminal ./cmd/orbit
make quick
GOFLAGS=-p=2 make test-race-core
go test -race -count=1 -timeout=15m ./tests/integration/...
GOFLAGS=-p=2 make quick test-integration test-model test-faults test-harness test-terminal-packages
GOFLAGS=-p=2 make test-terminal-packages
GOFLAGS=-p=2 make fmt-check vet
make check
go test -count=1 -v -timeout=30m ./tests/terminal \
  -run "$(cat docs/evidence/onboarding-e13-20261009/terminal-rerun-pattern.txt)"
go test -count=1 -v -timeout=15m ./tests/terminal \
  -run '^TestWANW16NativeRunnerRehearsal/tui-three-host$'
go test -count=1 ./internal/control \
  -run '^TestOnboardingE13PendingRemovalRequiresFreshReviewBeforeContactingSurvivors$'
go test -race -count=1 ./internal/control ./internal/replication ./cmd/orbit \
  -run '^TestOnboardingE13'
GOFLAGS=-p=2 make quick test-terminal-packages
```

The uninterrupted `make check` run reached full `tests/terminal` and exited 1
(1505 s). E08 had a contiguous-key-chip text expectation, E10's fake peer was
not enrolled, and fourteen subsequent checks tried to build during a short
in-progress source edit. The sixteen failing tests are named in
`terminal-rerun-pattern.txt` and rerun against the stable tree. This is combined
acceptance evidence, not a claim of one uninterrupted successful `make check`.
The rest of its targets run separately after the fixes.

`release-resumed.log` passed: quick (117 s, all five PTYs), integration
(114.077 s), model, design gates, faults, 36 harness tests, both architecture
builds and extracted-package/install/PTY checks. Final artifact inspection found
the manifest still hardcoded schema 13 and the previous build date. The packager
now uses `repository.CurrentSchema` and the 2026-10-09 date; the package harness
compares manifest metadata with every executed payload. Rebuilt package checks
passed in `packages-metadata-final.log`. Final gofmt/vet results are in
`fmt-vet-packaging-final.log`.

`terminal-rerun-resumed.log` (489.022 s): fifteen of the sixteen formerly
failing tests passed, including W07/W13/W14. W16's CLI subtest passed, but the
third-host TUI helper failed before onboarding: its nested Unix socket path
exceeded Linux's pathname limit with the longer cache-based TMPDIR. The socket
now lives directly in the marked private fixture root; startup also explicitly
checks that it exists. `native-tui-resumed.log` reruns the remaining subtest.
The remaining TUI subtest passed (343.200 s). This harness fix does not change
the trial executable. Combined with the original full run, every terminal
regression has passing evidence; no uninterrupted full-run pass is claimed.

Final code review found that a pending removal with new local retiree history
could keep waiting on an unavailable survivor before checking that its saved
review was stale. `pending-review-red.log` reproduces the wrong pending result.
PROPOSED retries now revalidate locally before contacting survivors, retain the
atomic pre-commit recheck, and mark stale intent ABORTED/needs_review. The new
test also proves unchanged membership, cleanup release and acceptance of a
fresh review. `pending-review-race.log` passes all E13 control/wire/CLI tests
with race detection. Quick/PTYs and packages rerun in
`retry-review-release-final.log` passed: quick (125 s, all five PTYs) plus
rebuilt package checks. `fmt-vet-final-tree.log` passes final formatting/vet.

The first `make quick` found a nullable unconfigured membership digest;
`COALESCE` fixed it. Subsequent PTYs found a stale Unregister Folder title
expectation, a history-list loading race in the harness (wait for an actual
version row), missing form paste handling, and an absent plain pairing-code
separator for a long label. Focused tests plus reruns cover these fixes. An
older broad race run was intentionally terminated after compiling a transient
source snapshot. The next broad race run passed all packages except the legacy
single-member retirement preview; the fixed integration suite ran with race
again. No data race report was observed. Logs retain the original failures.

Two ongoing shell sessions ended when the conversation resumed; `release-final`
and `terminal-rerun-final` are interrupted checks. `release-resumed` and
`terminal-rerun-resumed` are their replacements. No interrupted check counts
as passed.

## Real PTYs

```sh
make test-terminal-participation-pty
python3 scripts/terminal_participation_pty_test.py --binary bin/orbit \
  --output docs/evidence/onboarding-e13-20261009/pty
ORBIT_W07_PTY_EVIDENCE=/home/ethioking/dev/orbit/docs/evidence/onboarding-e13-20261009/wan-pty \
  go test -count=1 -v -timeout=12m ./tests/terminal \
  -run '^TestWANW(07RealPTYRelayOnboarding|12BinaryDoctorPrivacyAndPTY)$'
```

W12 passed in `wan-network-pty.log`. W07 failed former review-copy assertions
and plain pairing-code parsing, then passed in `wan-pty-final.log` (175.84 s).
`quick-complete.log` passed all five PTY suites (112 s), and final resumed
checks repeat them after the attribution/proposal fixes. Fixtures use a local
signed development service; these are not physical-WAN or hosted-service claims.
`wan_tui_phase.py` Details assertions were updated and its harness tests run;
the physical hosted three-machine campaign is unexecuted for E13.

## Host install and preservation

```sh
make trial-status
python3 /home/ethioking/.cache/orbit-work-tmp/e13-trial-backup.py
ssh -o BatchMode=yes -o ConnectTimeout=8 laptop python3 - < /home/ethioking/.cache/orbit-work-tmp/e13-trial-backup.py
ssh -o BatchMode=yes -o ConnectTimeout=8 rpi python3 - < /home/ethioking/.cache/orbit-work-tmp/e13-trial-backup.py
make trial-install
make trial-status
python3 /home/ethioking/.cache/orbit-work-tmp/e13-trial-verify.py
ssh -o BatchMode=yes -o ConnectTimeout=8 laptop python3 - < /home/ethioking/.cache/orbit-work-tmp/e13-trial-verify.py
ssh -o BatchMode=yes -o ConnectTimeout=8 rpi python3 - < /home/ethioking/.cache/orbit-work-tmp/e13-trial-verify.py
```

The temporary helper backs up the previous binary and transactionally consistent
metadata through authenticated local owner control; tokens remain private and
are not printed. Read-only verification compares versions/checksums, schema,
roots, membership, identity, history counts, file manifests and service state.
The Pi remains unconfigured. An initial backup helper omitted the HTTP scheme
from `control.addr`; corrected before backups succeeded. No reset/fresh command
or owner default-service modification was used. Consult results.json for which
install/verification commands have actually completed.

`trial-install.log`: exit 0, 10 s, linux/amd64 and linux/arm64 built and installed
on local, laptop and rpi; only the two previously running trial units restarted.
All three verification helpers exited 0 (`trial-verify-{pc,laptop,pi}.log`). PC
and laptop migrated 14 → 15, including both retirement tables, retaining their
identity, roots, membership and history. Their preexisting ordinary-file
manifests were empty; byte preservation has separate real-PTY/fault evidence.
The Pi remains unconfigured/stopped and executes the arm64 build natively.
`trial-after.log` confirms the final state. Previous binaries and owner-control
database backups remain privately under each account's
`~/.local/lib/orbit-trial/backups/e13-*/`.

The stale-pending-review fix required a final refresh. All three
`trial-backup-final-*.log` helpers passed, retaining the previous installed build.
`trial-install-final.log`: exit 0, 9 s, all three hosts updated, PC/laptop
trial units restarted. All `trial-verify-final-*.log` helpers passed and
`trial-after-final.log` confirms the same roots/membership/identities and active
PC/laptop trials, with the Pi unconfigured/stopped. Final binary checksums and
rebuilt package metadata are in results.json and package-SHA256SUMS.
