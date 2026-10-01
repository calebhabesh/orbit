# Commands and actual outcomes

The release engine is commit `e13e53ac3b8cfd616a20691fe70d5bb689a2e56c`.
[Manifest](manifest.json) records the harness revision, tested hashes,
environments, generator seeds and remaining gates. Commands ran from the
repository unless stated. New evidence directories must be empty; do not
reuse a previous run's output. No personal contents or authentication secrets
are included in published results.

## Clean source and packages

```sh
python3 scripts/validation/reproduce_release.py --source e13e53ac3b8cfd616a20691fe70d5bb689a2e56c --kernel /boot/vmlinuz-linux --output docs/evidence/release-20261001/release-candidate
```

PASS. The script made a new marked checkout, ran `make check`,
`go test -race -count=1 ./...`, `make demo`, both VM campaigns below,
`sha256sum -c SHA256SUMS` in its `dist/`, and `make package` again.
All six packages and the checksum file matched the first build byte-for-byte;
tracked source remained unchanged. Each executed command, directory, duration
and exit code is in [reproduction.json](release-candidate/reproduction.json).
The earlier clean-checkout missing-binary failure is retained; Make targets
now build before integration/race execution.

Binaries used in final native/benchmark runs were extracted from that
checkout's checked tar archives, not rebuilt from the working tree. The
archive member is the top-level `filesync`. Check the archive checksum first,
then extract with `tar -xOf ARCHIVE filesync` into the appropriate `bin/`
file and make it executable. [packaged-binaries.json](packaged-binaries.json)
records both archive and binary hashes. The initially incorrect member
selection and canceled attempt are explicitly invalidated.

## Native prescribed hosts and owner pilot

```sh
python3 scripts/validation/three_host.py --laptop laptop --pi rpi --vps vps --output docs/evidence/release-20261001/laptop-release-packaged-final
python3 scripts/validation/service_lifecycle.py --hosts laptop rpi vps --output docs/evidence/release-20261001/lifecycle-release-packaged-final
python3 scripts/validation/pilot_setup.py --output docs/evidence/release-20261001/personal-pilot
python3 scripts/validation/upgrade_pilot.py --setup docs/evidence/release-20261001/personal-pilot/pilot-setup.json --packages docs/evidence/release-20261001/packaged-binaries.json --output docs/evidence/release-20261001/personal-pilot/package-upgrade-e13e53a.json
```

Final native scenarios/lifecycle: PASS on actual laptop amd64, Pi arm64 and
Oracle VPS arm64. Public content hashes and tested binary hashes are in their
JSON reports. Laptop access was repaired after earlier dated unreachable
checks. The first resume assertion was too strict: an object can become durable
before its progress row. The corrected oracle requires every recorded chunk
reused, newly fetched count no greater than the remainder, and complete whole
hash equality. Final native run recorded 5 reused + 7 fetched = 12 chunks.

Pilot setup's first attempt failed native capture/network readiness. The
portable unit's filesystem namespace triggered Ubuntu's AppArmor
`unprivileged_userns` profile; filesystem namespace directives were removed
from the unit while unprivileged ownership/resource limits/NoNewPrivileges
remain. No host security policy was disabled. The workstation's existing
VPS route is bridged with three owned loopback SSH units; their exact settings
are committed in `pilot_setup.py` and recorded in the setup report.
The repaired prepared pilot is PASS, not owner adoption.

Upgrade's first backup attempt correctly refused while an agent owned state.
The corrected helper gracefully stops only the owned unit before `VACUUM INTO`
backup, installs the checked binary and verifies restart/UI/SQLite integrity.
Final upgrade PASS on all three. A uniquely named automated setup note was
written on the laptop and observed by SHA256 on all three without any scan/sync
command; [post-upgrade report](personal-pilot/post-upgrade-background.json).
Actual owner use and explanation remain unexecuted/unconfirmed.

## VM failure schedules

```sh
python3 scripts/validation/abrupt_reset.py --kernel /boot/vmlinuz-linux --output NEW_EMPTY_RESET_DIRECTORY
python3 scripts/validation/abrupt_reset.py --kernel /boot/vmlinuz-linux --output NEW_EMPTY_DISK_DIRECTORY --hook enospc.object --hook enospc.sqlite --hook enospc.checkpoint --hook enospc.staging --hook enospc.fsync
```

PASS 16 reset cases and 5 storage-error cases from the clean source. Kernel,
guest-init and QEMU versions are recorded. The helper reaches an actual named
production boundary, emits readiness and waits. The host verifies the new
marked image, kills only its child QEMU, then boots a fresh guest to check
protected hashes and publication recovery. No shutdown/unmount precedes the
reset; a dirty-cache negative control must lose its unflushed overwrite.

Disk exhaustion fills each new guest image and invokes the named operation.
Staging uses a 2-MiB file to fail during writes rather than a later metadata
commit. Fsync uses a child seccomp filter returning ENOSPC from real flush
syscalls; the shard is precreated so the included case reaches incoming-file
flush. Earlier small-stage/directory-flush failures remain separate records.
These are VM experiments, not physical Pi/VPS power cuts.

Native transfer interruption waits for durable `transfer_chunks.verified`
growth, then checks PID/start time/executable/state before stopping only that
sync process. Resume preserves that state and hashes all completed contents.
Four parallel in-flight chunks can account for extra interrupted traffic.

## Instrumented measurements

```sh
python3 scripts/validation/benchmark.py --output docs/evidence/release-20261001/benchmark-release --small-files 10000 --large-mib 1024 --repetitions 1
python3 scripts/validation/benchmark.py --output docs/evidence/release-20261001/benchmark-repeated --small-files 30 --large-mib 100 --mixed-extra-mib 10 100 --repetitions 3
python3 scripts/validation/benchmark.py --output docs/evidence/release-20261001/benchmark-vps-route --small-files 10 --large-mib 16 --mixed-extra-mib 10 30 --repetitions 1 --relay vps
```

The original full-size parent was absent after its successful initial workload.
Before resuming, both retained roots had valid `.filesync-disposable` markers
and the checked package binary hash. The original JSON was copied with
exclusive creation to
[benchmarks-interrupted-initial.json](benchmark-release/benchmarks-interrupted-initial.json).
The continuation used a new exclusively created
[log](benchmark-release-resume-20261001.log), preserving `benchmark-release.log`:

```sh
python3 scripts/validation/benchmark.py --output docs/evidence/release-20261001/benchmark-release --small-files 10000 --large-mib 1024 --repetitions 1 --resume-after-initial
```

The helper checked saved source, receiver and baseline tree digests and both
binary hashes before continuing. It checked marker and process identity before
reaping only workers recorded by this run. Roots stayed available until the
completed report was preserved, then were removed in the owner-requested cleanup.

Full-size campaign PASS 9/9 and the continuation exited 0; smaller campaign
PASS 27/27, VPS-route
campaign PASS 9/9. Fresh states have finite 10-GiB/256-MiB/512-MiB limits.
Each workload compares actual resulting tree hashes and TLS/TCP stream counts
against the same verified/durable full-file TLS baseline. OS caches are warm;
the workstation also ran validation jobs. Both proxy directions and baseline
sockets use TCP_NODELAY. Earlier Nagle-proxy timings are historical, not current
speedup evidence. Proxy delay is per read and its rate cap is per response
TCP stream. VPS-route storage stays local; TLS travels through the actual VPS
and back via two SSH forwarding channels. Neither root directories nor
existing VPS workloads are reset for these runs.

[Measured results](measured-results.md) report stream bytes, timings, chunk
reuse, receiver CPU/RSS/FD samples, scan times and accumulated storage. Exact
workload dimensions and sample counts accompany positive and negative results.

## Regressions, tools and limits

```sh
go test -count=1 ./internal/... ./model/... ./tests/integration/... ./tests/designgates/... ./tests/faults/...
go test -count=30 ./internal/scheduler -run '^TestSchedulerRootUnavailablePausesFolder$'
go test -count=1 ./internal/replication ./internal/repository
go test -race -count=1 ./internal/scheduler ./internal/repository
go test ./tests/faults -run '^$' -fuzz '^FuzzProtocolEnvelopeDecode$' -fuzztime=15s -parallel=2
go test ./tests/faults -run '^$' -fuzz '^FuzzPathSanitization$' -fuzztime=15s -parallel=2
python3 -m unittest discover -s scripts/validation -p 'test_*.py'
go mod verify
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
```

Relevant/full Go and race checks PASS. Fuzz campaigns PASS, no failing input.
Host-worker safety PASS. Existing-output refusal was exercised for all five
entry points with an existing evidence sentinel whose hash remained unchanged.
From `web/`, `npm ci`, `npm run build`, `npm audit --json` PASS; embedded
output unchanged, audit zero findings. Go modules verified; govulncheck no
findings at execution time.

The inventory >1024 regression failed before spool/retry fixes. The later
finite-budget populated inventory expired before page admission; preserved
observations are partial after the orchestration overwrite. The retained
10,000-file fixture then passed unchanged sync with no new metadata/chunks.
Before/after path-capture profiles used
`go test -run '^$' -bench '^BenchmarkCaptureDistinctPaths$' -benchtime=1x -cpuprofile=PROFILE ./internal/repository`
and `go tool pprof -top PROFILE`: 14.402 s vs 0.104 s, one sample each.
Other retained failures include absent background endpoints, typed-nil limiter,
coalescing and large limiter request regressions; corrected checks pass.

Cleanup stops/unlinks only run-owned worker processes, SSH children or unique
user services after ownership checks. Harnesses retain synthetic roots by
default; the subsequent owner-requested fixture cleanup is recorded below. Persistent
pilot services/gateway intentionally remain enabled. Never inject faults or
remove a populated personal pilot. No broad process-name kill, pre-marker root
delete or privileged firewall/VPN/AppArmor change is used.

## Owner-requested fixture cleanup

Read-only inventories ran locally and through `ssh laptop`, `ssh rpi` and
`ssh vps`, using the archived [inventory script](cleanup-inventory.py):

```sh
python3 /tmp/filesync-validation-space-inventory.py
ssh -o BatchMode=yes -o ConnectTimeout=8 laptop python3 - < /tmp/filesync-validation-space-inventory.py
ssh -o BatchMode=yes -o ConnectTimeout=8 rpi python3 - < /tmp/filesync-validation-space-inventory.py
ssh -o BatchMode=yes -o ConnectTimeout=8 vps python3 - < /tmp/filesync-validation-space-inventory.py
```

The archived [cleanup worker](cleanup-worker.py) received the corresponding
inventory as `PLAN` through stdin to `python3 -`, locally or over the same SSH
aliases. Its first pass explicitly preserved both final benchmark roots; the
completion pass removed that preserve set only after report success and the
read-only inventory showed no active processes. This was one-off authorized
maintenance, separate from the measured engine/harness revision.

The cleanup checked canonical home-child paths, owner UID,
regular single-link `.filesync-disposable` markers, absence of `.filesync-pilot`,
same-filesystem directories, live process arguments/cwd/open descriptors and
registered user-service links before deletion. Removal used Python's descriptor
protected `shutil.rmtree`; marker and root identity were rechecked immediately
before removal. No broad home-directory glob was deleted without these checks.

The workstation hit zero available bytes during a documentation write while
the benchmark ran. Reports were copied to RAM-backed `/tmp`; an inactive
6.09-GiB failed-run fixture was copied there with every regular-file hash
verified before its original was removed. After the owner requested cleanup,
33 other inactive workstation roots, 14 laptop roots, 19 Pi roots and 19 VPS
roots were deleted. The two final benchmark roots were deleted only after all
nine rows passed and the completed report/log were copied and hash-checked.
The temporary old fixture was then discarded; release-report backups remain.

Exact paths, byte counts and per-host outcomes are in
[validation-fixture-cleanup.json](validation-fixture-cleanup.json) and
[space-relocation.json](space-relocation.json). All validation home roots were
removed; personal pilot roots/services and unrelated workloads were preserved.
The affected case-study draft was restored before the documentation audit.
