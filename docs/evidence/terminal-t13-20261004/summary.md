# T13 release campaign — 2026-10-04

T13 remains **in progress** for three named native acceptance conditions below.
Personal use and the owner's comprehensive review are deferred until after
delivery by the owner's instruction. They do not block engineering delivery.
Historical P17 pilot roots and services are preserved.

## Candidate and provenance

The passing candidate is an isolated committed copy of the dirty development
source, `e37e2600cd236776d5161f3a293718098bd7500e`. The original branch and index
were untouched. The [snapshot manifest](reproduction-final-candidate/snapshot-manifest.json)
records input hashes and original HEAD/status. A separate clean clone ran the
[release reproduction](reproduction-final-candidate/clean-release/reproduction.json).
Its tracked source remained clean and repeated packages were byte-identical.
This snapshot commit belongs to a disposable repository; it is not a commit
on the original branch.

Production source matched that candidate when this report was assembled.
The [2026-10-05 follow-up](../terminal-t13-20261005/summary.md) records a later
enrollment-v2 retirement error correction and additional validation; the older
candidate does not include that correction. Two subsequent PTY
fixture corrections wait for actual selectable rows and verified content
instead of assuming that a heading or metadata implies readiness. The
[source audit](current-source-audit.json) records their exact hashes and scope.
Current local and native PTYs exercise those corrections. Documentation and
this assembled evidence report were updated after the snapshot.
The workspace's ignored `bin/` and `dist/` artifacts now match the verified
candidate payloads and package hashes, with the `orbit` launcher retained.

## Executed checks

| Check | Result and evidence |
| --- | --- |
| Clean `make check` | Passed, 392.981 s; formatting, vet, tests, static builds and packaging in [reproduction](reproduction-final-candidate/clean-release/reproduction.json) |
| Uncached full race suite | `go test -race -count=1 ./...` passed, 347.162 s; [log](reproduction-final-candidate/clean-release/race.log) |
| Demo | Passed, 0.605 s; [log](reproduction-final-candidate/clean-release/demo.log) |
| Abrupt-reset campaign | All 16 cases passed, including dirty-cache negative control; [results](reproduction-final-candidate/clean-release/reset/abrupt-reset.json) |
| Storage failure campaign | All five cases passed: objects, SQLite, checkpoint, staging and separately injected fsync/fdatasync ENOSPC; [results](reproduction-final-candidate/clean-release/disk-full/abrupt-reset.json) |
| Checksums and repeat packages | Passed; every package hash identical on repeat; [manifest](reproduction-final-candidate/clean-release/reproduction.json) |
| Six T13 tests, twice | Ordinary 4.687 s and instrumented 69.535 s; [normal](final-candidate-packet-normal.log), [race](final-candidate-packet-race.log); race also instruments spawned CLI/daemon binaries |
| Direct laptop/Pi LAN | Passed ordinary packaged create/invite/join/exact approval, delayed approval after both daemon restarts, existing files, bidirectional edits, same-device second-folder sharing and offline/reconnect; [results](lan-final-candidate/terminal-native.json) |
| Actual laptop/Pi/VPS engine | Passed, 129.045 s; normal sync, independent three-head conflicts, late arrival/stale rejection, original-author forwarding, interrupted/resumed transfer, historical restore and integrity after restart; [results](native-engine-final-candidate/three-host.json) |
| Native packaged keyboard/service | Pi, VPS and laptop all passed bare/shell, onboarding and everyday PTYs, plus unique user-unit enable/start/capture/restart/removal with preserved identity, integrity and bytes; [results](native-hosts-final-candidate-03/terminal-hosts.json), [33 sanitized transcripts](native-hosts-final-candidate-03/transcript-manifest.json) |
| Package transactions | All 11 scenarios passed: amd64/native and arm64/emulated tar/deb/rpm payloads, standalone repeat install/removal, extracted bare PTY, Debian/Fedora install/reinstall/removal; [results](final-candidate-package-transactions.json) |
| Current local keyboard fixtures | Shell and everyday campaigns passed; [shell](final-shell-pty/results.json), [everyday](final-everyday-pty/results.json) |
| Dependencies | `go mod verify` passed; pinned `govulncheck@v1.8.0 ./...` found no vulnerabilities at execution time; [verify](go-mod-verify.log), [scan](govulncheck.log) |

The [I01–I28 map](invariant-map.md) names executable assertions and their limits.
The [command index](commands.md) provides reproduction entry points and scope;
the [result index](results.json) reconciles successful reports and package hashes.
Actual UTC times cross into October 5; the campaign date is October 4 in Toronto.

The ordinary LAN campaign uses laptop `192.168.88.83` and Pi `192.168.88.63`.
SSH orchestrates it; Orbit packets travel directly over LAN. It does not manually
write membership/certificates. Native binaries are checksum-verified package
contents. In contrast, the three-host engine campaign deliberately uses manual
membership fixtures and measured SSH relays. It establishes actual-host engine
behavior, **not ordinary native three-host onboarding or Tailscale**.

## Defects found and fixed

A 1,024-file fixture made status exceed its deadline under race instrumentation.
CPU profiling identified repeated SQL preparation/history loads and duplicate
readiness computation. Prepared statements now live within one history load;
availability observations batch metadata and preserve validated heads and
quarantine semantics; status reuses per-folder readiness within the request.
The temporary diagnostic probe was removed from executable test source.
[Before](status-probe-before.log), [after](status-probe-after.log), and
[profile](status-profile-before.txt) are retained. One probe sample each measured
readiness at 4.121 s before and 0.437 s after; this is a workload-specific
diagnostic, not a general speedup claim.

Actual VM storage exhaustion exposed a SQLite SIGBUS while growing its
memory-mapped WAL index. Orbit's single database owner now sets SQLite
`locking_mode=EXCLUSIVE` before accessing WAL, keeping the index in private
memory. The driver sorts DSN PRAGMAs, so WAL activation happens explicitly
after connection settings. Database format and `synchronous=FULL` remain the
same. SQLite documents this connection ordering in its
[WAL without shared memory contract](https://sqlite.org/wal.html#use_of_wal_without_shared_memory).
The final clean reset/storage campaign passed. Earlier
[disk-full regression](sqlite-enospc-fixed-02/abrupt-reset.json) and
[VM-child fsync error](fsync-fixed/abrupt-reset.json) also passed.
[Persistence](../../persistence.md) owns the revised connection contract.

Live identity diagnostics now query the owned repository and report unavailable
inspection accurately. Native service checks use authenticated history digests;
transfer interruption watches hash-verified immutable objects, stops its own
worker, then checks durable progress rows. Older tests close their owner before
stopped CLI/reopen inspection, use consistent backups for live raw-token checks,
and use actual state locking for live CLI parity. The fsync helper closes the
parent repository before its child opens the database. No acceptance assertion
was removed.

The Pi keyboard campaign caught publication completing while its operation
screen stayed pending, with immediate repolling. Operation queries now observe
actual working projections without reauthoring; pending screens use normal ticks,
and explicit `r` resumes the same retained operation. The VPS campaign found an
automatically paused missing root displaying only `FOLDER_PAUSED`; it now retains
`ROOT_UNAVAILABLE`. Both new assertions were red before the owning fix and green
afterwards: [operation before](pending-publication-before.log),
[after](pending-publication-after.log), [root before](root-cause-before.log),
[after](root-cause-after.log), [repeated race regressions](native-fixes-regressions.log).

Native PTYs also exposed fixture timing mistakes: broad welcome text matched
overview help before the chooser loaded; headings appeared before selectable
rows; conflict metadata arrived before verified payloads. The harness now waits
for the exact loaded control state and actual `ready` availability. Production
correctly refused unavailable content; that refusal was retained.

## Measurements and failure limits

The resource runner captures 1,024 real files, explicitly reviews 512 deletions,
checks cursor pages, and performs actual 8/32-MiB editor-result uploads/merges
with exact digests. There are two repetitions in each mode and 32 command
observations total. Sampled CLI peak RSS ranges were **19,728–21,928 KiB** for
ordinary binaries and **52,180–57,816 KiB** under race instrumentation, with
**6–7 descriptors**. [Raw measurements](resources.json) retain sample counts,
command durations and outputs.

Sampling every 5 ms can miss brief peaks. Inherited `ru_maxrss` is logged
separately and is not the isolated CLI memory estimate. Daemon and editor
resources are separate. Continuing large-file progress under small edits is
covered by `TestBackgroundSyncFromPersistedPeerEndpoints` in the full suite.
Existing full-size engine measurements and positive/negative tradeoffs remain
[historical evidence](../release-20261001/measured-results.md); they were not
rerun or relabeled as new measurements here.

Faults use newly created marked roots or QEMU ext4 images. VM resets discard guest
caches while host storage stays running. They establish the recorded virtual
device outcomes, not physical Pi power-loss or broken-flush guarantees. The
fsync case injects ENOSPC into actual syscalls in one VM child and is separately
labeled from natural disk exhaustion. Failed candidates remain in `reproduction/`
and `reproduction-final/`; neither is a successful final release. Intermediate
native failures remain archived; final outcomes supersede them only within the
stated scenario and source scope.

## Remaining technical acceptance

- Existing Tailscale path: unavailable on these hosts; no Tailscale installation
  or authenticated account is present. LAN and existing WireGuard/SSH routes
  cannot prove this criterion.
- Native login/logout/boot persistence: user managers are running with
  `Linger=no`. Scoped enable/start/capture/restart/removal proves service lifecycle,
  not unattended boot. Existing personal/VPS workloads were not rebooted.
- Ordinary native three-host joining/forwarding over a reachable private path:
  laptop/Pi have no existing route to the VPS private address. Local ordinary
  three-process rollout and actual three-host engine fixtures pass independently,
  but do not establish this native ordinary journey, including its reviewed
  retirement/replacement steps.

The resumable next step is to run the same packaged ordinary journey on an
existing authenticated Tailscale path, extend it through the third host, and
exercise login/logout/boot in a designated disposable native service environment.
Do not modify the preserved pilot or treat an unexecuted condition as a pass.
Owner use and comprehensive review follow delivery at the owner's chosen time.
