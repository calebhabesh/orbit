# Persistence, filesystem safety and retention

Status: implementation contract; D1/D4/D5 outcomes are in [design gates](design-gates.md). Release evidence is tracked separately. This document owns durability and cleanup rules.

## Orbit extension: file mutation journals, read leases & crash-consistent recovery

Status: frozen by gate outcomes [G03 and G04](orbit-design-gates.md). Implemented by packets O01/O02/O03/O09.

1. **Durable File Mutation Journals**:
   - File actions (Import, CreateDir, Move, Delete) record phase transitions in SQLite: `Planned -> Staged -> Installed -> SourceVerified -> Completed`.
   - Operations carry a durable Operation ID, idempotency key, and reviewed basis token.

2. **Move Atomicity, Overwrite Safety & Source Race Protection**:
   - Move installs the destination first, re-verifies the source, and only then deletes the source.
   - If destination exists, an explicit reviewed overwrite token is required; the displaced file is atomically moved to `.filesync-internal/recovery/<opID>` before installation.
   - **Concurrent source modification (Invariant I26)**: If the source file is modified concurrently by an editor or writer while the move is staging/installing (stat/hash mismatch), **the source is NOT deleted**. Both the new destination and the modified source file are preserved on disk. The operation completes with `StatusCompletedWithSourceRetained` and flags a clear attention item.
   - **Subtree moves**: Directory moves review a snapshot of immediate children. If a new child is added before the move commits, the subtree token is invalidated (`ErrSubtreeInvalidated`), requiring re-review.

3. **Content Read Leases & GC Protection**:
   - Reading or previewing content via browser/CLI acquires a short-lived `ReadLease(versionID, chunkDigests, ttl)`.
   - **Invariant I25**: Active read leases prevent Garbage Collection (GC) sweeps from unlinking required chunk objects, even if the version is unreferenced in latest heads. Expired leases are safely reclaimed.

4. **Identity Recovery & Crash Consistency**:
   - Identity reset and backup restoration require stopped exclusive state directory ownership (`state.Acquire`). Live mutation is fenced.
   - The transition atomically rotates TLS identity (`peer-identity.pem`), updates SQLite `folders` (`local_author = newID`, `next_counter = 0`) while preserving historical DAGs, and writes `config.json`.
   - **Interrupted transition detection (Invariant I20)**: On restart, startup consistency verification compares `config.DeviceID`, certificate identity, and `folders.local_author`. Any inconsistency returns `ErrIncompleteRecovery` and fences sync.
   - **Missing chunk payloads (Invariant I18)**: Restoring a metadata database backup where local chunk files are missing marks versions `ContentUnavailable` and never serves synthetic or corrupt data.

## 1. Fault model

Required: process termination at named boundaries; dropped connections; duplicate requests; full storage; permission failures; supported concurrent editor patterns; detectably corrupt managed content; documented abrupt-reset experiments. Assume supported local Linux filesystems and storage that honor successful flush requests. SQLite/state on network filesystems, broken storage flush guarantees, arbitrary hardware bit rot recovery without another valid copy, and filesystem-wide atomic snapshots are outside the contract.

Durability claims name filesystem, mount options where relevant, kernel, storage device/environment, SQLite driver/configuration, and observed failure class. SIGKILL demonstrates process recovery, not power-loss durability. A VM reset is an abrupt-reset experiment under that hypervisor, not proof about the Pi's storage hardware.

## 2. Persistent layout

```text
state/
  identity/                 private identity material, owner-readable only
  metadata.sqlite           metadata and journals
  objects/sha256/            immutable verified chunk objects
  incoming/                 unverified partial chunks, never served as complete
  quarantine/               corrupt objects and recoverable candidates
  operations/               explicit backup/migration artifacts
root/
  .filesync-internal/        reserved same-filesystem staging/recovery area
  ... user paths ...
```

P05 stores one Ed25519 private key and self-signed peer certificate together in
`identity/peer-identity.pem` with mode `0600` beneath the `0700` identity
directory. Existing material is never silently regenerated when unreadable or
insecure. Reopening and reconnecting reuse the same key and device identity.

Use XDG defaults and explicit overrides; never place managed state inside a synchronized root. Root scratch names are reserved and excluded. Validate scratch ownership/type and root registration before use. Backups must be outside roots or explicitly excluded at registration. Treat scratch objects as budgeted storage even when on another filesystem.

One process holds an exclusive state-directory lock. Prevent a root from being registered twice under different folder IDs in that process. Document that running unrelated sync software over the same roots is outside tested operation.

## 3. Metadata and content commit

Baseline SQLite: WAL mode, foreign keys enabled, explicit durability configuration suitable for the promised crash model, short transactions and controlled checkpoints. P00/P03 must verify the selected driver's connection/PRAGMA behavior; `synchronous=FULL` is the intended durable baseline. Do not assume one connection's setting configures an entire pool. Use a serialized writer and bounded readers. Never copy only the live main SQLite file as a backup while ignoring its WAL; use a supported consistent backup procedure.

Local capture ordering:

1. Safely open the path and record observation identity/stat data.
2. Stream bytes to chunk temporaries with bounded buffers; compute chunk and whole-file hashes.
3. Verify the supported stable-read condition. If unstable, keep prior captured state intact and retry within budget.
4. Flush chunk files; install immutable objects atomically; flush affected object directories as required.
5. Commit version envelope, counter allocation, manifest, content availability, references and working-basis update atomically in SQLite.
6. Only now report `saved locally` and advertise a durable local version.

A crash before step 5 may leave orphan objects; later orphan cleanup may reclaim them after proving no reference or active pin. A crash after step 5 must leave every referenced object durably present under the declared assumptions. If recovery detects missing/corrupt content, surface integrity failure and repair; do not invent a successful receipt.

Remote content readiness uses the same object durability discipline. Verify assembled whole-file digest before marking file content ready. Metadata may be accepted first as pending, but cannot authorize publication, a durable-content receipt, or GC of its required fallback content.

## 4. Scanning and working basis

Root registration stores the configured absolute path, the opened root's
device/inode pair, and a random registration ID. The same registration ID is
stored in the database and in an owner-only regular marker inside
`.filesync-internal`; the scratch directory and marker must not be symlinks or
hard-linked files. Registration walks the absolute path without following
symlinks, permits the root itself to be a mount point, and rejects overlap
with state or another root. Startup and every scan/deletion application reopen
the configured path, compare device/inode and marker, and retain the root
descriptor for the operation. A mismatch, inaccessible component, nested
mount encountered beneath the descriptor, or incomplete enumeration pauses
deletion inference. Bind mounts or a privileged replacement that deliberately
reproduces both identity and marker are outside this check; a marker alone is
not presented as mount authenticity.

A scan records success/failure per subtree. Only successfully and completely enumerated supported regions are eligible for absence-based tombstones. First enrollment has a separate bootstrap mode where absence never means deletion. Mass-deletion candidates remain local pending proposals until approved; do not publish tombstones first and ask later.

Notifications are hints. Periodic scans reconcile missed notifications and restart gaps. Stat metadata may optimize scheduling; it is not a cryptographic content identity. Define periodic full-content verification so same-size edits with restored timestamps are eventually found under the supported writer model.

P04's initial scanner hashes every supported regular file on every explicit
scan. This is deliberately more conservative than the later notification and
daily full-content schedule in P12. It suppresses an unchanged observation by
comparing the whole-file digest, executable bit, and persisted working basis;
mtime alone never suppresses capture.

P04 deletion review defaults to an absolute threshold of 100 missing tracked
paths **or** a ratio of at least 25% when at least 10 paths are missing.
Both thresholds are configurable through the workspace options; a zero
threshold disables that arm. The preview records a random token and scan
generation. A subsequent scan invalidates it, and approval revalidates root
identity and every candidate's absence before authoring tombstones. Bootstrap
and incomplete subtrees infer no deletions. P12/P13 will expose persistent
operator configuration and structured control responses.

Stable capture opens the path through the verified root descriptor, records
descriptor identity/size/mtime/ctime, streams and hashes that same descriptor,
then repeats `fstat` and verifies the path still names the opened inode. A
change or short/long read preserves the prior captured version and retries
within a bound. Save-by-rename therefore makes the attempted observation stale
rather than silently capturing the displaced inode. P01 rejected an
unconditional second matching read/hash: it doubles quiescent-file IO yet
still cannot prove safety against an arbitrary cooperating writer. P04 must
exercise in-place overwrite, truncate/write and save-by-rename patterns
against the chosen metadata checks. Continuously changing files are blocked
with `UNSTABLE_FILE`. No application-consistent or every-write capture
guarantee is made.

Track working basis separately from heads. Suppress scan feedback for application-owned writes through persisted basis/content comparison, not a timing delay. A crash followed by a scan must not create a new authored version of a just-applied remote file. Unexpected bytes become a local candidate and pass the causal creation rules in [protocol](protocol.md).

## 5. Publication journal

The implementation must recover each row below. State names are illustrative; recoverability is required even if the crash occurs between a filesystem action and recording the next state.

| Durable phase | Filesystem possibilities | Recovery obligation |
| --- | --- | --- |
| `PREPARED` | Old path exists; stage may be absent/incomplete | Preserve old captured content; finish/rebuild stage or abort safely |
| `STAGED` | Verified flushed stage present; old path may have changed | Revalidate observation; preserve new local candidate and defer if stale |
| `REPLACEMENT_INTENT` | Old, new, recovery sibling or combinations may exist | Inspect verified bytes/identities; never assume recorded phase proves which rename occurred |
| `FILESYSTEM_PUBLISHED` | New path may be installed but DB basis old | Complete required flushes and metadata only if observed output still matches; otherwise capture competing state |
| `COMMITTED` | Applied basis durable; leftover recovery objects possible | Clean only unreferenced scratch; retain policy-protected historical content |

Before replacing an existing regular file, preserve the supported observed
local contents durably or defer. Stage verified output beneath the root's
`.filesync-internal` directory, on the same `st_dev` as the target, and flush
it before publication. Revalidate through already opened root/parent
descriptors. For an existing regular target, v1 uses Linux
`renameat2(RENAME_EXCHANGE)` so the actual displaced namespace object remains
named in scratch even if an editor saved after the earlier observation. Move
that displaced object to a unique recovery name, flush the affected
directories, and only then complete the applied-basis commit. A new target
uses `RENAME_NOREPLACE`; structural changes have a separate subtree plan.
Lack of required `renameat2` or descriptor-safe resolution support blocks
automatic publication on that host with a diagnostic rather than falling
back to a lossy plain rename. Parent creation, rename, unlink and directory
removal have explicit flush/recovery steps. Fsyncing a file alone does not
establish durability of its directory entry.

The D1 experiment demonstrates that a pre-rename stat cannot close the race:
an editor can save-by-rename after the stat and a later plain rename removes
the editor's only name. Exchange preserves that inode for recovery. It also
demonstrates that a writer holding the old descriptor continues writing the
displaced scratch inode after exchange. Automatic publication supports
quiescent files, completed save-by-rename, and in-place writes detected before
exchange; an observation change preserves a candidate and defers. Arbitrary
writes through a descriptor held across replacement are outside the v1
capture guarantee. Recovery keeps and classifies the displaced object as a
candidate, but no finite retention delay is claimed to prove the writer has
closed. A path whose type changes or whose result is ambiguous is blocked for
review.

Journal recovery runs before ordinary scans/sync. On ambiguity, preserve all available variants and block the affected path for review. Good captured content wins over cleanup convenience. A blocked working path must not prevent serving already verified immutable versions elsewhere.

P04 reserves workspace stage plus the observed regular target's byte count
against the repository's configured data budget before staging. A committed
recovery copy retains its conservative reservation until an explicit later
review/cleanup operation; P10 owns reclaiming those copies safely. The actual
root scratch bytes are on the root filesystem, so this accounting is a budget
admission bound, not an OS-level free-space reservation. ENOSPC and fsync
errors can still occur after admission and must leave the journal recoverable.
An unexpected displaced file is retained under a unique recovery name and
reports `LATE_EDITOR_CANDIDATE`; it is not silently folded into the applied
basis. This classification does not claim that a long-lived writer has closed.
Projection-created parent directories use a durable pending-scaffold record
before `mkdirat`; recovery checks that record before any ordinary scan. A
crash between mkdir and the scaffold's durable completion retains the
directory but marks it `AMBIGUOUS_SCAFFOLD` instead of authoring an empty
directory version. Missing scaffold paths are removed from the scaffold set,
so a later explicit recreation can be captured as a directory version.

## 6. Directory and path safety

Use Linux `openat2` relative to a verified root/parent descriptor with
`RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS|RESOLVE_NO_XDEV`, plus no-follow
`fstatat` checks and descriptor-relative `renameat2`/unlink operations, for
every sensitive read, creation, replacement and deletion. Registration walks
the absolute root path without symlinks but does not use `NO_XDEV` until the
root descriptor is reached. The P01 experiment on the pinned Go/x/sys
versions and Linux 7.2 rejected a swapped intermediate symlink with `ELOOP`.
Lexical path cleaning followed by ordinary path opening is insufficient.

Reject symlinks and special files before reading them. Treat regular files with multiple links as unsupported for automatic mutation until the hard-link policy is proven; diagnostics must distinguish this from a symlink. Never follow a device node or FIFO as though it were a file. Unsupported objects block affected operations without deleting them.

Apply file modes as safe local permissions plus synchronized executable status; do not copy ownership, setuid/setgid, or arbitrary mode bits from peers. A file-to-directory or directory-to-file change is a structural operation with conflict checks, not recursive deletion. Remove a directory only when empty and its intended deletion remains valid. Concurrent child creation must make removal fail/block safely.

## 7. Retention and garbage collection

Release requires safe content GC. V1 keeps accepted envelope/vector/tombstone metadata; causal metadata compaction is deferred. Expose its growth and enforce a metadata budget. This still provides meaningful reclamation of obsolete file payloads without pretending tombstone compaction is solved.

Baseline historical-content policy: protect versions newer than 30 days since local durable acquisition OR among the 20 newest locally retained superseded versions per path. These are tunable starting defaults, not safety rules or promised capacity. Use deterministic tie-breaks and local acquisition records, not remote authored timestamps. Expiration means eligibility, never permission to override a safety pin. Clock rollback must not accelerate deletion; test clock changes and record conservative behavior.

Protected content roots:

- Every accepted current head, including unresolved concurrent versions.
- Previous usable/applied contents while a successor lacks content or publication is pending.
- Retained historical payloads under policy or explicit pin.
- Active transfer/serve leases, publication journals and restore/resolve operations.
- Quarantined candidates requiring owner recovery, unless explicitly discarded through a reviewed operation.

Do not conflate a working-copy file with an immutable recovery copy. Working files may change independently. Do not require every peer to receive every superseded payload forever; peers can catch up to current history while expired historical payloads are visibly unavailable. Metadata and current/conflicting content requirements remain intact.

GC protocol: compute candidates under a consistent metadata generation →
acquire durable deletion intents that exclude new references → unlink
unreferenced objects under repository coordination → flush required
directories → finalize metadata. Intent creation, reference commits, and
lease acquisition use the serialized repository mutation boundary. A new
capture/fetch/publication/restore reference cancels an intent; if unlink
already occurred, verified bytes must be reinstalled before its reference can
commit. A serve lease starts only while the object exists and no intent is
active. Startup reconciles intents before admitting either references or GC.
A crash at any step yields either retained extra bytes or a recoverable
missing unreferenced object, never a missing protected object. Policy expiry
and an offline peer's possible historical interest are not safety pins:
current heads, pending publication/fallback, explicit retention, active
transfer/serve, restore/resolution, and recovery candidates are.

Orphan cleanup uses separate proven-unreferenced classification; age alone is not proof. Startup reconciles interrupted GC before admitting reference changes. Membership mismatch suspends membership-dependent cleanup. Retired author entries remain valid history; retirement does not delete their content automatically.

## 8. Space accounting and integrity

Budget metadata/WAL, immutable chunks, transfer temporaries, root staging, quarantine, and journal recovery copies. Reserve enough space before admitting work; staging may need a full file even if network transfer reuses most chunks. Reservations are durable/recoverable or recomputed on restart. A file that cannot fit is blocked before replacing good data. Other unrelated folders continue when safe.

Handle ENOSPC from writes, fsync, rename metadata, SQLite commit and checkpoint, not just a preflight free-space check. Successful preflight does not reserve OS capacity against other processes. Leave a configured emergency reserve for metadata recovery; if even recovery cannot write, remain read-only/blocked with a diagnostic.

Verify hashes on content reads for transfer/restore. Corruption invalidates current availability, quarantines the object, and blocks affected versions. Fetch matching bytes from an authorized peer with current availability, verify them, then restore readiness. If no verified copy exists, report unavailable versions explicitly. Repeatedly bad peers are throttled and diagnosed. Integrity checks never rewrite history to point at a different payload.

## 9. Metadata backup and identity recovery

The P03 repository backup primitive uses SQLite `VACUUM INTO` while the
repository owns its serialized database connection. This produces a
transactionally consistent destination containing committed WAL state; copying
only `metadata.sqlite` from a live WAL database is not a supported backup.
The destination must not already exist and belongs under an excluded
operations/backup location, never a synchronized root.

A backup is suitable for inspection and upgrade rollback only while it retains
the current causal counters. Restoring older metadata, resetting the database,
or otherwise losing an author counter invalidates that device identity for new
authorship. Recovery must create a new device identity and use reviewed
reenrollment; it must not reuse the old certificate/identity and continue from
a rolled-back counter. Binary rollback against the unchanged current database
is a distinct operation and still requires schema compatibility. P15 owns the
operator-facing backup, migration, restore, and identity-reset workflow.

Schema v4 adds approved peer inventory snapshots and their materialized
entries. They are bounded, short-lived transport state rather than causal
receipt or permanent history. Expired rows are deleted before new snapshot
admission; no long SQLite read transaction is held across network pagination.

Schema v5 adds resumable peer transfer records (`transfers`, `transfer_chunks`)
with per-chunk verified progress, content pins (`content_pins` owner_kind='transfer'),
direct durable receipts and peer status (`peer_progress`, `peer_contacts`).
Active transfers reserve disk budget, pin verified chunks against premature collection,
track attempts and last errors, and survive restarts so already verified chunks are
reused without retransmitting across the network.

## Recorded abrupt-reset experiment

The [2026-10-01 campaign](evidence/release-20261001/release-candidate/reset/abrupt-reset.json)
uses QEMU/KVM, a new raw ext4 image with virtio-blk `cache=none`, and a static
Go guest init. The host stops the dedicated QEMU child without guest shutdown
or unmount; a newly booted guest reopens SQLite and runs journal recovery.
An existing unflushed overwrite disappears in the negative control, showing
that guest dirty pages are discarded. Protected captured content survives the
selected object, version-commit and publication boundaries.
This observes guest-reset behavior under the recorded kernel/hypervisor and
successful-flush assumptions. Host storage remains running; physical power
loss, host-controller cache loss and Pi media behavior are unexecuted.
See [QEMU's cache options](https://www.qemu.org/docs/master/system/invocation.html)
and [SQLite WAL synchronization](https://www.sqlite.org/wal.html).

Inventory transport spools count as incoming data and are capped by both the
metadata spool limit and configured data/free-space budgets. Closing a session
removes its spool. A killed process can leave a non-authoritative spool; it is
never content-ready or published, remains budgeted, and can be removed during
inspected maintenance after the owning agent has stopped.

## O07 browse generations and active reads

Schema 12 adds a durable database-wide `browse_generation` and a reverse
parent-reference index. Triggers on versions/parents, projections, scaffolds
and folders invalidate directory, search, Deleted files and history cursors.
An unrelated workspace change can conservatively invalidate a cursor. Pages
are refreshed by starting without a cursor; no long-lived SQLite snapshot is
held while the browser navigates. Reads never create directory versions.

Directory/search queries combine locally accepted heads, observed projections
and scaffolds in SQL, synthesize navigable ancestor rows, and materialize at
most 200 results in Go. SQLite uses the folder/path and reverse-parent indexes;
substring search still scans the selected folder's locally known paths.
Working state is `observed` or `unobserved`, with pending/conflict/block flags;
`observed` does not promise unchanged current disk bytes. Details/history keep
exact captured identities separate. Locally present chunk metadata is an
availability hint; exact content reads verify hashes again.

`OpenVersionRead` derives every chunk from the exact stored manifest, checks
readiness, object presence and deletion intents, and installs pins under the
same mutex/transaction boundary as GC. Repeated chunk digests acquire one pin.
It verifies the whole file before response headers and each chunk before
emission, retaining one chunk buffer (1 MiB plus a length-check byte), alongside
bounded manifest metadata. Corrupt chunks use the existing quarantine path.
No replacement version is selected when a requested version is unavailable.

A response owns `stream` pins until completion, error or cancellation. Its
five-minute read-record timestamp cannot expire those live pins. Ordinary
short leases are pruned at startup and during GC; active stream pins remain
protected even if that record is pruned. `Close` releases response pins;
startup removes abandoned stream pins after interrupted GC reconciliation.
Startup relies on existing exclusive state ownership. A prepared lease is not
a permission to consume bytes and does not bypass exact-version authorization.

## O09 file mutation journals and recoverable file actions

Schema 13 adds durable SQLite mutation tracking via `file_mutations` and
`file_mutation_entries`. File actions (Import, CreateDir, Move, Delete) record
atomic phase transitions: `PLANNED -> STAGED -> INSTALLED -> SOURCE_VERIFIED -> COMPLETED`.
Each record includes folder ID, action kind, source/dest paths, reviewed snapshot
tokens, overwrite flags, and operation IDs.

Multi-path operations (such as directory subtree move) record sequenced entries in
`file_mutation_entries` with position, source/destination paths, item kinds, and
per-entry completion status.

Recovery semantics (`Workspace.RecoverFileMutations`) executed at startup or
workspace initialization:
- `PLANNED` or `STAGED`: No changes committed to workspace filesystem; safely marks
  phase `ABORTED` and cleans up temporary staging files.
- `INSTALLED`: Destination is durably present on disk. For move operations, re-verifies
  source hash against the reviewed token; if unchanged, safely authors a tombstone and
  unlinks source, then marks `COMPLETED`. If concurrently modified, retains source
  (`source_retained = 1`) and completes without deleting source (Invariant I26).
- `SOURCE_VERIFIED`: Verification passed before interruption; ensures tombstone
  publication and marks `COMPLETED`.
