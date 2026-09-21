# Persistence, filesystem safety and retention

Status: design baseline. D1/D4/D5 experiments must establish the exact implementation contract. This document owns durability and cleanup rules.

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

Root identity includes a registration marker plus observed filesystem identity; verify it on startup and before scans/deletion application. Missing marker, changed mount/root identity, permission failures or incomplete enumeration pause deletion inference. A marker alone is not proof against every mount/replacement scenario; D5 defines accepted root checks and failure modes.

A scan records success/failure per subtree. Only successfully and completely enumerated supported regions are eligible for absence-based tombstones. First enrollment has a separate bootstrap mode where absence never means deletion. Mass-deletion candidates remain local pending proposals until approved; do not publish tombstones first and ask later.

Notifications are hints. Periodic scans reconcile missed notifications and restart gaps. Stat metadata may optimize scheduling; it is not a cryptographic content identity. Define periodic full-content verification so same-size edits with restored timestamps are eventually found under the supported writer model.

Stable-read baseline uses a safely opened descriptor, pre/post metadata checks, and bounded retries. Arbitrary concurrent writes can evade stat-only checks or produce mixed reads; D1 must decide whether a second matching read/hash is warranted for the supported workload and document its limits. Quiescent file capture must succeed; continuously changing files may be explicitly blocked. No application-consistent or every-write capture guarantee.

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

Before replacing an existing path, preserve the supported observed local contents durably or defer. If the path changes during preparation, keep the candidate and reconcile instead of silently treating it as the old version. Stage output on the root filesystem; a cross-filesystem rename is not the publication mechanism. Parent creation, rename, unlink and directory removal have explicit flush/recovery steps. Fsyncing a file alone does not establish durability of its directory entry.

A replacement/rename does not prevent another process from writing through an already open descriptor. The implementation must not promise preservation of every uncaptured write. D1 must test save-by-rename, ordinary overwrite, and long-lived descriptor writers, then specify when automatic publication is supported, deferred or blocked. Preserve displaced objects long enough for the chosen supported protocol; do not claim a finite delay proves no writer remains. If a stronger guarantee requires cooperative writers or a managed filesystem, report that as a scope tradeoff rather than sneaking it into v1.

Journal recovery runs before ordinary scans/sync. On ambiguity, preserve all available variants and block the affected path for review. Good captured content wins over cleanup convenience. A blocked working path must not prevent serving already verified immutable versions elsewhere.

## 6. Directory and path safety

Use descriptor-rooted operations and appropriate no-follow/type checks for every sensitive read, creation, replacement and deletion. Lexical path cleaning followed by ordinary path opening is insufficient for symlink races. Select current supported Go/OS facilities in D1 and test their actual behavior, including intermediate components and mount boundaries. API availability must be checked against the pinned Go toolchain.

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

GC protocol: compute candidates under a consistent metadata generation → acquire durable deletion intents that exclude new references → unlink unreferenced objects under repository coordination → flush required directories → finalize metadata. New captures/fetches must pin/recreate content rather than race an in-flight deletion. A crash at any step yields either retained extra bytes or a recoverable missing unreferenced object, never a missing protected object. D4 validates interleavings with serving, restore, publication and enrollment.

Orphan cleanup uses separate proven-unreferenced classification; age alone is not proof. Startup reconciles interrupted GC before admitting reference changes. Membership mismatch suspends membership-dependent cleanup. Retired author entries remain valid history; retirement does not delete their content automatically.

## 8. Space accounting and integrity

Budget metadata/WAL, immutable chunks, transfer temporaries, root staging, quarantine, and journal recovery copies. Reserve enough space before admitting work; staging may need a full file even if network transfer reuses most chunks. Reservations are durable/recoverable or recomputed on restart. A file that cannot fit is blocked before replacing good data. Other unrelated folders continue when safe.

Handle ENOSPC from writes, fsync, rename metadata, SQLite commit and checkpoint, not just a preflight free-space check. Successful preflight does not reserve OS capacity against other processes. Leave a configured emergency reserve for metadata recovery; if even recovery cannot write, remain read-only/blocked with a diagnostic.

Verify hashes on content reads for transfer/restore. Corruption invalidates current availability, quarantines the object, and blocks affected versions. Fetch matching bytes from an authorized peer with current availability, verify them, then restore readiness. If no verified copy exists, report unavailable versions explicitly. Repeatedly bad peers are throttled and diagnosed. Integrity checks never rewrite history to point at a different payload.
