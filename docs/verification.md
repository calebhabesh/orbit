# Verification and evidence contract

Status: local design/model, unit, integration and process-fault suites exist
through P17. Required release experiments and their remaining gaps are in
[packet status](implementation/status.md) and the
[2026-10-01 evidence](evidence/release-20261001/summary.md). Passing a named
check verifies its stated oracle; it does not prove every failure model.

## Invariants

| ID | Observable requirement | Primary test surface |
| --- | --- | --- |
| I01 | Immutable version ID has exactly one envelope; duplicate delivery creates no logical duplicate | History/repository |
| I02 | Same valid causal history yields equivalent heads regardless of delivery order | Independent model vs history |
| I03 | Concurrent content and edit/delete heads survive until explicitly covered by a reviewed resolution | History/control |
| I04 | Ordinary capture does not implicitly resolve received but unreviewed heads | Working-basis model/workspace |
| I05 | No stored receipt without durable metadata and required verified content | Repository/process harness |
| I06 | Partial or corrupt content is never published as complete | Transfer/workspace |
| I07 | Recovery preserves previously durable protected versions and reports ambiguity | Journal/fault harness |
| I08 | Counter and event creation are atomic; identity rollback is never knowingly reused | Repository/lifecycle |
| I09 | Peer input cannot escape its authorized folder or request unauthorized objects | Peer/path interfaces |
| I10 | GC never removes protected content; expiry never erases causal knowledge | Independent reference-set model/repository |
| I11 | Unavailable roots/incomplete scans/bootstrap absence do not create deletions | Workspace |
| I12 | Concurrent structural operations preserve incompatible histories without recursive destructive replacement | Directory model/workspace |
| I13 | Work and resource use are bounded; large-file work eventually progresses when resources are available | Scheduler/integration |
| I14 | Third-party forwarding preserves author/ancestry; hub availability is not final-device receipt | Replication/three-peer harness |
| I15 | Membership disagreement/retirement cannot silently admit excluded old histories or resurrect deletion | Membership model/integration |
| I16 | Restore/resolution replay is idempotent and stale reviewed state is rejected | Control/repository |
| I17 | Scan after apply/restart does not fabricate local edits | Workspace/integration |
| I18 | Corruption yields unavailable state or verified repair, never substitute contents | Repository/repair |
| I19 | UI and CLI issue the same operations and display qualified progress | Control/UI end-to-end |
| I20 | Limits, schema/protocol incompatibility and failed migrations preserve recoverable state | Operations/integration |

## Independent causal model

Implement a small test-only event DAG oracle with explicit parent reachability rather than copying production vector-comparison code. It models accepted versions, reviewed resolutions, retirement acceptance, heads, and content availability as separate state. Include a separate simple reference-set oracle for GC. Production and model may share fixture encodings but not reconciliation or reachability algorithms.

Generate local capture, ordinary edit, delete, resolve, restore, duplicate delivery, message reorder, disconnect, forwarding, restart, membership changes and content expiry. Use deterministic seeds and explicit schedules. Transport delivery is an action, not a wall-clock sleep. Drain valid work at the end and compare normalized head/conflict sets, accepted identities, protected content requirements and reported availability. Do not compare transient working bytes as though conflict state had already been resolved.

Exhaustively enumerate bounded small histories for 2 and 3 actors; run seeded longer histories for 3–4 actors. Record the enumeration bounds and seed set. A failing run saves its complete trace and a minimized reproducer. Deliberately mutate a production causal rule to demonstrate that the independent oracle catches at least one defect; revert the mutation before acceptance.

## Scenario matrix

| Scenario | Required oracle/result | Invariants |
| --- | --- | --- |
| Three offline edits, every reconnect order | Same three heads and verified bytes retained | I01–I04, I14 |
| Resolve A/B, later receive C | Resolution and C remain concurrent | I02–I04, I16 |
| Equal-byte independent writes | Distinct ancestry remains until explicit resolution | I01–I04 |
| Same-author edit from stale basis | Safe block/review or proven ordered capture; no false dominance | I04, I08 |
| Delete vs edit; repeat delete; restore deleted file | No silent loss/resurrection; restore has new ancestry | I02, I03, I16 |
| Existing divergent enrollment folders | Preview conflicts; no bootstrap tombstones | I03, I11 |
| Parent delete vs new child; file/directory collision | Structural conflict and preserved child bytes | I12 |
| Editor rename/overwrite during transfer/apply | Supported observed edits preserved; unsupported race limits demonstrated honestly | I04, I07, I17 |
| Root unmount/replacement; unreadable subtree | Pause/partial-scan warning; no inferred mass deletion | I11 |
| Crash before/after every durable boundary | Recorded recovery outcome and protected hashes intact | I05–I08 |
| Transfer drop mid-chunk/mid-file; lost receipt | Verified chunks reused, whole-file digest correct, no false stored state | I01, I05, I06 |
| ENOSPC during write/fsync/SQLite/checkpoint/staging | Good captured data remains; bounded visible block | I05, I07, I13 |
| Bit flip in current/history/shared chunk | All affected versions diagnosed; authorized repair verified | I06, I18 |
| GC races receive/serve/restore/publication/restart | No protected object removed; leftovers may be reclaimed | I07, I10 |
| Long-offline enrolled peer and expired history | Current causal reconciliation correct; expired payloads explicit | I02, I10, I15 |
| Retirement/config mismatch/stale rejoin | No unknown retired history admitted; reviewed new identity enrollment | I08, I15 |
| A→VPS→B with no A/B link or online overlap | Original authors preserved; B eventually receives; A status is qualified | I14 |
| Unauthorized folder/hash, traversal, symlink race, malformed manifest | Rejected before unsafe IO/allocation | I09, I20 |
| Continuous small edits plus large archive | Bounded memory/FDs; large archive advances | I13 |
| Migration interrupted and newer schema opened | Safe recovery/refusal; no counter rollback reuse | I08, I20 |

## Failure harness design

Run actual agents with separate state directories, roots, ports and identities. Mark each environment disposable; validate all destructive targets remain beneath its canonical root before injection. Refuse symlinked targets, personal paths and remote production services. Reserve ports dynamically. Save generated configurations with secrets redacted in published artifacts.

Named hooks are emitted at object flush/install, metadata transaction boundaries, publication intent/stage/rename/directory flush/final commit, GC intent/unlink/finalization, receipt send and transfer progress. Harness waits for explicit hooks, then kills/disconnects/injects errors. Hooks are disabled or safely inaccessible in release operation; never expose a public kill endpoint. Use an injected filesystem adapter for deterministic IO faults and actual-process tests to verify production integration.

Use namespaces/containers or a protocol proxy for partitions/delay/drop behavior. The model handles message reordering; TLS transport experiments must not claim arbitrary tampering with encrypted application frames. Abrupt-reset experiments use disposable VMs/storage images and documented commands. Real Pi/VPS experiments are non-destructive unless explicitly conducted in a separate disposable environment.

## Test tiers and CI

- Every change: format, vet/static checks appropriate to pinned tools, compilation, unit/model fixtures, relevant packet tests.
- Pull request: integration scenarios, Go race detector on supported runner architecture, fixed deterministic seeds, protocol/schema fixtures, frontend checks once present.
- Scheduled/manual campaign: broader seeds, fuzzing, real filesystem fault matrices, long-running resource/fairness checks.
- Release: packaged binaries, local demo from clean checkout, actual three-host workflow, documented abrupt-reset experiments and personal pilot.

The Makefile provides these validation targets: `make check`, `make test`, `make test-race`, `make test-integration`, `make test-model`, `make test-faults`, `make demo`, `make build`, `make evidence`. The explicit QEMU abrupt-reset target is separate from ordinary checks. Publish what each target executes and any privilege requirements. Destructive tests are never a hidden dependency of ordinary checks.

## Benchmark plan

Label fixtures as synthetic and personal pilot inputs as real. Use reproducible generators and content hashes; exclude private contents from evidence. Initial workloads:

- Small files: 10,000 files around 4–64 KiB, varied directory depth.
- Mixed: documents/images plus several 10–100 MiB objects.
- Large: one 1 GiB file, plus a larger file only where declared budgets allow.
- Change patterns: unchanged scan, small in-place overwrite, append, prefix insertion, rename, deletion and concurrent edits.
- Network conditions: unrestricted local link, throttled link, latency/interruptions, and actual VPS path.

Compare full-file transfer with missing-chunk transfer using identical contents, transport settings, verification, cache state and concurrency. Report metadata/TLS overhead separately where measured and distinguish payload bytes from on-wire bytes. Warm/cold object caches and filesystem caches must be identified. Repetitions and median/tail summaries require enough samples; no p99 claim from a tiny sample. Record raw runs, failures and outliers.

Metrics: bytes transferred/reused, wall/CPU time, scan/hash time, time to durable receipt and application, peak RSS, open descriptors, storage amplification, conflict resolution propagation, crash recovery time, interruption retransmission overhead, and large-file progress under small-file load. Show negative results such as prefix insertion defeating fixed-size chunk reuse. Do not choose a speedup target before measuring.

## Release evidence layout

Create as work occurs, not empty claims:

```text
docs/evidence/<run-id>/
  manifest.json       commit, build/protocol/schema versions, hosts, OS, filesystem, workload and seed
  commands.md         setup/run/cleanup and exact failure schedule
  results.json        raw counters and pass/fail assertions
  summary.md          conclusions, limitations and links to logs
  logs/               bounded sanitized event logs and minimized failure traces
```

P17 produces a 3–5 minute demo outline: enrollment → normal edit → three offline edits → reconnect/conflict → reviewed resolution → interrupted large transfer/resume → restore → qualified per-device status. Include a separate failure/recovery clip if needed. The case study explains one difficult bug and one rejected design, personal contribution, architecture, measurements, and limitations.

No claim of consensus, exactly-once network delivery, unlimited scale, universal no-loss behavior, or independent backup. Idempotent logical effects are not exactly-once delivery. Report three physical/cloud hosts as tested environments without pretending they establish arbitrary datacenter fault independence.

## Primary design references

Consulted for architecture planning on 2026-09-20; recheck version-sensitive APIs during implementation. Sources inform the design; this project does not inherit their correctness or claim compatibility.

- [Syncthing BEP](https://docs.syncthing.net/specs/bep-v1.html): example separation of file vectors, transfer blocks, and inventory sequencing.
- [Syncthing synchronization](https://docs.syncthing.net/users/syncing): comparison point for conflict handling; this product chooses explicit reviewed resolution.
- [Syncthing versioning](https://docs.syncthing.net/users/versioning.html): useful comparison for retention scope; our captured-version contract must be tested independently.
- [SQLite atomic commit](https://www.sqlite.org/atomiccommit.html): database durability assumptions and failure boundaries.
- [SQLite WAL](https://www.sqlite.org/wal.html): local WAL operation, checkpointing and deployment constraints.
- [Linux rename](https://man7.org/linux/man-pages/man2/rename.2.html): namespace replacement does not invalidate existing open descriptors.
- [Linux fsync](https://man7.org/linux/man-pages/man2/fsync.2.html): directory-entry durability requires separate consideration.
- [Go os documentation](https://pkg.go.dev/os): check descriptor-rooted APIs against the selected toolchain; name validation alone is not race-safe containment.

## Current executable release campaigns

- `python3 -m unittest discover -s scripts/validation -p 'test_*.py'` exercises
  the actual host worker's marker, path/link and process-identity refusals.
- `make demo` runs the local multi-process demonstration without cloud access.
- `python3 scripts/validation/abrupt_reset.py --kernel /path/to/vmlinuz --output /path/to/evidence`
  boots newly created ext4 images, stops only the child QEMU process at actual
  production hooks, then boots a fresh guest to verify protected hashes and
  recovery. Its dirty-cache negative control must lose an unflushed overwrite.
- `go run scripts/three_host_pilot.go --laptop laptop --pi rpi --vps vps`
  performs the scripted actual-host scenarios in fresh private roots. It stops
  the receiving sync process after observing durable verified chunk progress,
  restarts it, checks exact remaining fetch counts, and verifies whole-file hashes.
- `go run scripts/benchmark_suite.go --small-files 10000 --large-mib 1024 --repetitions 1`
  generates deterministic synthetic fixtures and counts actual encrypted TCP
  bytes. Both alternatives use TLS 1.3 mutual authentication and whole-file
  verification; the full-file baseline hashes existing files and skips unchanged
  contents, flushes received files, and publishes through rename/directory flush.
  Raw sample counts accompany every summary. These are warm filesystem-cache
  runs, with fresh application stores per repetition.

Traffic counters include TLS record/handshake and HTTP overhead in both
stream directions; they exclude TCP/IP and SSH encapsulation. Call them
TLS/TCP stream bytes, rather than physical-link wire bytes. The algorithms
perform different history/storage work, so their times do not establish a
universal speedup. Proxy delay is stated per read, not claimed as calibrated RTT.
Automated sessions remain distinct from owner-reported personal use.
