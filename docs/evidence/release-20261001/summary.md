# Release validation — 2026-10-01

The technical implementation has substantially stronger release evidence.
P16 is complete for the recorded failure models. All 45 current benchmark
measurements passed. P17 remains in progress while actual owner use/explanation
is unconfirmed. Scripted tests do not establish personal adoption.
The earlier estimated wire-savings and universal power-loss claims are withdrawn.

## Checked source and artifacts

The [clean checkout](release-candidate/reproduction.json) of engine commit
`e13e53ac3b8cfd616a20691fe70d5bb689a2e56c` passed `make check`, uncached race
checks, the local demo, VM reset/storage-failure campaigns and package checksum
verification. All six amd64/arm64 tar/deb/rpm artifacts were bit-identical on a
repeat build. The checkout remained clean. Earlier source snapshots and the
failed pre-backoff measurement remain separate historical evidence.

The [package/binary hashes](packaged-binaries.json) match every binary used in
[final native scenarios](laptop-release-packaged-final/three-host.json) and
[final service lifecycle checks](lifecycle-release-packaged-final/service-lifecycle.json).
Those checks used the actual laptop, Raspberry Pi and Oracle VPS. They verify
normal sync, three offline heads, reviewed resolution, late-arrival conflicts,
stale-token rejection, forwarding while the author's listener is stopped,
historical restore, restart and SQLite integrity. The interrupted 12-chunk
transfer recorded five verified chunks; restart reused five and fetched seven,
then matched the whole-file hash. This was an actual process interruption.

Native lifecycle checks require an ordinary edit to be captured while the
installed service runs, a changed process after restart, embedded UI HTTP 200,
and preserved data/state after uninstall of only the owned unit. They exercise
archive binaries and the shipped user-service template with explicit paths and
ports, not system-wide deb/rpm installation or lingering configuration.

## Failure evidence

The [named invariant/scenario map](invariant-map.md) links I01–I20 and all
scenario rows to executed checks. The catalog-description test is not used as
execution evidence. Local/model/process checks retain their stated limits.

The [clean VM campaign](release-candidate/reset/abrupt-reset.json) passed
16 cases: an unflushed overwrite is lost in the dirty-cache negative control,
while protected hashes and recovery pass at 15 production boundaries. Each
case stops only its dedicated QEMU/KVM child, then boots a fresh guest on a new
marked ext4 image with virtio-blk `cache=none`. This loses guest caches; the
host remains running. Physical Pi/VPS power cuts, alternate filesystems and
broken hardware flush promises are unexecuted. Recorded verify times include
guest boot, repository opening and recovery; they are not isolated recovery latency.

The [five storage-failure cases](release-candidate/disk-full/abrupt-reset.json)
passed: four actual guest disk-exhaustion cases cover object writes, SQLite
growth, checkpoint allocation and 2-MiB staging. The fifth injects ENOSPC into
real fsync/fdatasync syscalls in a VM child via seccomp. It is separately labeled,
not a claim of naturally delayed allocation or physical hardware behavior.
Earlier failed fixtures/hook aliases remain dated diagnostic evidence.

Host-worker [safety checks](harness-safety-release.log) verify marker/token,
path/link and process-identity refusals. [Evidence overwrite checks](evidence-directory-refusal.log)
refuse nonempty output directories before host setup. Two 15-second
[fuzz](fuzz-envelope.log) [campaigns](fuzz-path.log) passed without a failing
input. `go mod verify`, [govulncheck 1.8.0](govulncheck.log), frontend build and
[npm audit](npm-audit.json) passed; vulnerability outcomes are point-in-time,
not a permanent security guarantee.

## Measurements and corrections

[Three smaller repetitions](benchmark-repeated/benchmarks.json) passed 27/27
checks with 30 small files, a 100-MiB archive, and additional 10/20/100-MiB
objects. [Actual VPS-route measurements](benchmark-vps-route/benchmarks.json)
passed nine checks with both TLS streams routed through the actual VPS and
back. Storage stays local in that comparison; native VPS storage is verified
separately. The [final full-size campaign](benchmark-release/benchmarks.json)
passed 9/9 workloads with 10,000 files and a 1-GiB archive under finite budgets.
Its successful first workload was preserved; a [new continuation log](benchmark-release-resume-20261001.log)
records the remaining eight checks after marker, binary and tree-digest validation.
The [interrupted report](benchmark-release/benchmarks-interrupted-initial.json)
and original log remain unchanged. The failed pre-backoff run stays historical.

[Measured results](measured-results.md) report every workload's stream bytes,
times and chunk reuse, plus receiver resources and accumulated storage.
In the full-size single sample, the tail overwrite fetched one chunk and reused
1,023, using 98.31% fewer stream bytes than the baseline. Prefix insertion
fetched all 21 shifted chunks and used 67.86% more stream bytes. Full inventory
traffic dominates the latter comparison. Timing samples include shared-host
load, space pressure and fixture cleanup; they are not isolated throughput tests.

Both alternatives authenticate TLS 1.3 peers, verify whole files, flush received
bytes, and publish durable working files. The baseline hashes and skips
unchanged files. Counters measure both TLS/TCP stream directions including
HTTP/TLS overhead; TCP/IP, SSH and VPN headers are excluded. File Sync also
commits immutable objects/history/journals. CPU/RSS/FD samples cover its receiver;
baseline CPU/RSS are unmeasured. Warm filesystem caches, shared workstation
load and differing storage work prevent generic speedup claims. Proxy delay is
per read, not calibrated RTT; throttling here is per response TCP stream,
not a fixed aggregate link rate. Three samples support observed ranges,
not a p99 or statistical tail claim. Initial copies, unchanged trees, prefix
insertion and deletion can favor the baseline; localized edits/renames can
reuse chunks. Every numeric claim must name its workload and sample count.

Validation found real defects: a total-inventory rejection at 1,024 versions,
backpressure retry failures, folder-wide history reads for path operations,
per-summary quota scans that expired populated inventories, missing background
peer scheduling, a typed-nil limiter panic, large limiter requests that could
never progress, coalescing that reset running work, and restart loading hidden
behind completed history. Regression evidence is retained. The per-path capture
microbenchmark changed from 14.402 s to 0.104 s in one local sample each;
[before](history-profile-before.log) and [after](history-profile-after.log).
Per-object storage admission still traverses storage; durable per-file work
and metadata/receipt overhead remain measured limitations.

An archive-selection mistake started a development-binary run; it was stopped
and is [invalid release evidence](archive-selection-correction.json). Restart
also overwrote the original finite-budget expiry JSON/log. Only
[captured partial observations](benchmark-finite-budget-before-page/observed-failure.json)
remain; omitted values are unavailable, not estimated. The same retained
10,000-file fixture [passed after page admission was fixed](inventory-page-warm-regression.json).
Final summaries use separately executed, verified-package runs. Historical
unlimited-budget/Nagle-proxy runs stay separate from current measurements.

## Prepared personal pilot and remaining acceptance

The [pilot handoff](personal-pilot/handoff.md) gives the actual laptop/Pi/VPS
folders and service names. Verified packages were installed through a
[graceful upgrade](personal-pilot/package-upgrade-e13e53a.json), with consistent
backups and preserved roots/state. A new ordinary edit
[reached all three without manual scan/sync](personal-pilot/post-upgrade-background.json).
All setup notes are automated and excluded from owner-use evidence.

The workstation's three dedicated SSH forwarding services must remain on;
no existing firewall, VPN, AppArmor or unrelated VPS service was modified.
Pilot services start on login; lingering was not enabled. Fault harnesses
refuse these `.filesync-pilot` roots. Completed task diagnostics currently
remain in SQLite under soft metadata admission; automatic pruning is absent.
Legacy states without limits require an explicit `init` rerun to persist them.

On owner request, [nonessential generated fixtures were cleared](validation-fixture-cleanup.json)
on the workstation, laptop, Pi and VPS after disposable-marker, process,
filesystem and service checks. The active benchmark roots were preserved until
all nine checks completed, then removed. A temporarily relocated old fixture
was also discarded. Raw reports/logs and the personal pilots remain intact.
Recorded validation root paths are provenance, not currently retained datasets.

Remaining: record actual normal owner edits, offline/reconnect, ordinary restart
and elapsed duration; obtain
the owner's product/causal/recovery/GC/bottleneck explanation without agent
assistance. [Manifest](manifest.json), [result index](results.json),
[commands](commands.md), [demo](../../demo.md), [case study](../../case-study.md)
and [portfolio drafts](../../portfolio-bullets.md) provide the reproducible handoff.
