# Protocol and causal-state specification

Status: P01/P02 froze membership and causal fixtures; P05 implemented
authenticated v1 peer wrappers, bounded parsing, snapshot inventory, envelope
fetch and manifest-scoped chunk serving; P06 completed resumable two-peer
transfers, durable receipts and status reporting; P07 completed bidirectional
reconciliation, multi-head conflict projection and structural conflict isolation;
P08 completed reviewed resolution, historical restore, export and safe control replay;
P09 completed three-peer forwarding, membership distribution, canonical retirement
snapshots, and access termination. P10–P15 implement storage policy and operator interfaces. Release validation remains tracked in [status](implementation/status.md).
Protocol name is provisional. No Syncthing wire compatibility is claimed.

## 1. Identities and membership

Use cryptographically random persistent device identities bound to pinned peer public keys using established TLS libraries. Device identity is distinct from display name, network address, local database path, and shared-folder identity. A database reset, restored old database, or lost causal counter requires a new identity and explicit reenrollment; reusing a certificate with rolled-back counters is unsupported. Detect known mismatches and refuse; do not claim all offline rollback is automatically detectable.

A shared folder has a random stable 32-byte ID independent of its local root
path. Device IDs and SHA-256 public-key pins are also 32-byte values. Each
device explicitly approves its membership configuration: revision number,
prior revision digest, active device IDs/key pins, and retirement references.
The P01 canonical membership encoding is the ASCII domain separator
`filesync-membership-v1` followed by NUL, folder ID, big-endian `uint64`
revision, 32-byte prior digest (all zero for the first revision), big-endian
`uint16` active count, active `(device ID, key pin)` pairs sorted by device ID,
big-endian `uint16` retirement count, then retirement `(device ID,
retired-at revision, snapshot digest)` tuples sorted by device ID. Duplicate
or active-and-retired identities are invalid. The SHA-256 of those bytes is
the membership digest. Golden bytes are in
`tests/designgates/testdata/membership-revision-v1.hex`.

A single owner distributes the identical revision through the CLI; any device
can be the owner's administration endpoint, but revisions are a linear
sequence, not concurrently merged configuration edits.

Peers exchange data for a folder only when their locally approved membership revision/digest agrees. Mismatch pauses that folder's exchange and cleanup while local captures may continue. Enrollment and retirement are explicit maintenance operations; this sacrifices configuration-change availability to avoid silently inconsistent membership. Configuration updates never modify file causal ancestry.

Use mTLS on direct connections. The transport sender may forward another author's immutable version. Origin signatures and Byzantine protection are outside v1: authenticated authorized replicas are trusted to describe history honestly, while every message is still structurally/resource validated. A sender cannot obtain another folder's content just by knowing a chunk hash.

## 2. Version envelope

Fields:

| Field | Rule |
| --- | --- |
| `folder_id`, `path` | Folder identity and exact canonical relative path |
| `author_id`, `counter` | Origin event identity; counter allocated monotonically within author/folder |
| `parents` | Sorted unique IDs of same-path versions actually used as causal basis |
| `vector` | Per-path causal vector derived from parents and this author event |
| `kind` | `file`, `directory`, or `tombstone` |
| `manifest` | File size, full SHA-256, ordered fixed-size chunk hashes/lengths, executable boolean; absent for other kinds |
| `authored_revision` | Membership revision author was admitted under; does not establish acceptance by itself |
| `display_time` | Optional informational timestamp; never decides causality or conflict winners |

Version ID is `(folder_id, author_id, counter)`. Repeating the ID with identical immutable fields is idempotent. Repeating it with different fields is a protocol integrity error that blocks the offending exchange. A content hash identifies bytes, not a version.

Parents must be present and validated before an envelope becomes causally accepted. Fetch missing ancestry or mark the candidate pending; out-of-order delivery does not make unknown ancestry authoritative. V1 retains accepted version envelopes/ancestry metadata; content expiry is separate. This intentionally trades metadata space for simpler safe reasoning. Metadata-budget exhaustion pauses admission rather than pruning ancestry without proof.

## 3. Causality

Missing vector entries mean zero. `a` dominates `b` when every entry of `a` is at least `b` and at least one is larger. Equal vectors for different same-path version identities are invalid under the creation rules. Incomparable versions are concurrent. Compare only within the same folder/path.

To create a version, join the vectors of its explicit parents, then set its author's component to a newly allocated counter strictly above its previous folder counter and any own component in those parents. Allocation and version insertion are one metadata transaction. Gaps between events for a particular path are allowed because counters are folder-wide; they do not mean the receiver possesses intervening versions on other paths.

**Working basis is not all known heads.** Receiving B's edit while A still
edits its own working copy does not mean A's next save has reviewed B's
contents. Ordinary local capture extends only the persisted working basis and
the latest accepted same-path event by the local author; it never adds other
known heads merely because they were received. Same-path events by one author
must remain causally ordered. If the working basis neither contains nor
causally covers the latest local-author event, preserve the observed candidate
bytes and block event creation for explicit review. Do not mint two
same-author sibling branches and compare them using ordinary version vectors.
The D2 model fixes these outcomes: an edit based on A1 after receiving B1
creates A2 with parent A1 and leaves B1 concurrent; an edit still based on A1
after A2 exists is blocked.

A normal unchanged scan creates no event. Mode-only executable changes do create events. Identical concurrent contents retain separate causal identities; UI may show an equal-content conflict, but may not silently merge it. Repeated observation of an already recorded deletion creates no event.

Heads are the maximal accepted versions. Importing a new head whose content is still missing marks it pending; existing applied/recoverable content remains protected. Only content-ready states may be applied or receive a durable-content receipt.

### Required causal fixtures

Notation omits folder/path when identical:

- `A1:{A:1}` and `B1:{B:1}` remain two heads after either delivery order.
- A resolution `A2:{A:2,B:1}` with parents A1/B1 supersedes both.
- An unseen `C1:{C:1}` remains concurrent with A2.
- A further ordinary A edit based only on A1 remains concurrent with B1.
- A tombstone and concurrent file edit remain conflicting heads.
- Receiving C's version via B preserves C as author and never increments B's author counter.
- Matching file hashes with A1/B1 ancestry remain distinct versions.
- A folder inventory cursor advancing to 100 does not prove receipt of every origin event numbered ≤100.

## 4. Path and structure rules

Baseline: exact UTF-8 relative paths with `/` separators. Reject absolute paths, empty segments, `.`/`..`, NUL, invalid UTF-8, and reserved agent-internal names. Do not silently normalize Unicode or case; reject locally unrepresentable paths. JSON encoding and URL routing must preserve names exactly, with display escaping for control characters. Paths go in validated request bodies, not concatenated filesystem URLs. Pin byte/segment/depth limits in [operations](operations.md).

Roots cannot overlap, contain the state directory, be nested inside another
registered root, or contain another registered root. Registration rejects
symlinks in the configured absolute path. Internal publication scratch
directories are reserved, excluded from scans, protected, and validated on
startup. The root itself is a container, not a deletable replicated path.
After registration, operations are relative to a verified root descriptor and
do not cross symlinks or nested mount points.

Explicit directory versions represent directory existence, including empty
directories. A local scan may author an observed directory, but parent
directories created while applying a child are persisted as projection
scaffolds. A later scan suppresses those scaffolds rather than fabricating
local directory versions; applying an explicit remote directory changes the
basis from scaffold to explicit. Empty unused scaffolds are removed as part of
the operation that removed their last projected child when safe. Child
existence imposes a structural requirement on all ancestors. A file or
tombstone at an ancestor may block materialization; preserve both histories
and report a structural conflict rather than recursively deleting children.

Directory deletion is a local batch over observed descendants plus the
directory. It is not a cross-file atomic transaction. The batch carries a
preview/generation token covering the subtree; any local observation or
accepted arrival in that subtree invalidates the token and requires
reevaluation. Tombstones describe paths individually. A new concurrent child
survives as a structural conflict. Bootstrap scans and scans with an
unverified root or incomplete subtree never infer absence-based tombstones.

## 5. Resolution and restore

A resolution request names folder/path, exact reviewed head IDs, expected current head-set token, action, and idempotency key. The server rechecks these in a transaction. If the current reviewed set has changed before commit, return `STALE_VIEW` with the new heads. If a previously unseen version arrives after commit, normal reconciliation may produce another conflict.

Select uses chosen verified bytes; manual merge captures and verifies supplied content; both produce a new version whose parents are the reviewed heads. Keeping multiple copies creates new destination-path versions before resolving the original. Persist a recoverable multi-step operation; destination collisions fail visibly, partial completion is reported, and replay never creates duplicate logical copies. Do not claim cross-path atomic visibility.

Restore names a historical source version and a reviewed current head set. Source content must be available and verified. The new version's parents are the reviewed current heads; the source version is provenance, not a replacement for current ancestry. Restoration of a deleted path follows the same rule. An expired historical payload returns `CONTENT_EXPIRED`, not an empty file.

## 6. Transfer format and exchange

Use HTTPS with bounded JSON metadata and raw binary chunk responses. Decimal counters/sizes in JSON are canonical unsigned decimal strings to avoid JavaScript integer truncation; validate ranges. Freeze field encoding, unknown-field policy, stable error bodies, and golden fixtures in P02/P05. Reject duplicate JSON keys and unsupported protocol versions. Do not hash arbitrary JSON textual serialization as a version identity.

P02 freezes the version-envelope JSON field spellings in
`schemas/fixtures/version-envelope-v1.json`: fixed IDs and SHA-256 digests are
64 lowercase hexadecimal characters; counters, revisions, sizes and chunk
lengths are canonical unsigned decimal strings; parents and vector entries use
their domain-defined sorted order; absent optional values are omitted rather
than encoded as `null`. Envelope decoders reject unknown fields, duplicate
keys, noncanonical numbers and trailing JSON values. P05 still owns endpoint
wrappers, paging, complete request/response schemas and stable wire errors.

Initial endpoints:

| Operation | Purpose |
| --- | --- |
| `POST /peer/v1/hello` | Device, protocol, folder-membership digests and configured limits |
| `POST /peer/v1/inventory` | Paged snapshot of accepted envelope IDs and current availability |
| `POST /peer/v1/versions/get` | Bounded immutable envelopes and ancestry requests |
| `POST /peer/v1/chunks/get` | One raw chunk, authorized through a requested version in the shared folder |
| `POST /peer/v1/receipts` | Direct peer durable receipts for exact version IDs |
| `POST /peer/v1/status` | Current per-version availability/application/conflict status |

P05 froze the first four endpoint paths in this table. P06 implements and
freezes `POST /peer/v1/receipts` and `POST /peer/v1/status` with closed JSON
objects and bounded batches (1..128). Exact schemas and golden examples are
documented in [the peer v1 schema](../schemas/peer-v1.md).
Every request repeats the TLS-authenticated device ID plus folder membership
revision/digest; authorization is rechecked per request. Direct durable receipts
are recorded in `peer_progress` upon receipt. Status exchanges report metadata
knowledge, readiness, stored, applied, conflict, and blocked state.

The P05 listener uses TLS 1.3, requires a client certificate, caps headers at
32 KiB and metadata bodies/responses at 8 MiB, and applies 5-second header,
15-second read, 30-second write and 30-second idle deadlines. Self-signed
device certificates are authenticated by their explicitly approved SHA-256
SPKI pin. Clients perform ordinary certificate verification against the exact
out-of-band certificate and additionally compare the pin; they do not disable
certificate verification. Before parsing a body, the process admits at most 32
concurrent requests and applies a 128-request burst/64-request-per-second
token bucket; later scheduler work may refine fairness per peer.

Both sides can initiate reconciliation. First release uses full paged inventories; incremental indexes are optional after measurement. Snapshot token and cursor bind all pages to one consistent view. Expired snapshots restart safely. Concurrent changes are picked up by subsequent sessions. Start with a 30-second snapshot lifetime and a bounded number of open snapshots; avoid holding long SQLite read transactions that indefinitely prevent WAL checkpoints. Bound snapshot lifetime, memory, response size and request count. Inventory pages contain compact summaries; large manifests are fetched separately.

P05 materializes each authorized inventory into schema-v4 snapshot rows in one
short SQLite transaction. A peer may hold at most four snapshots per folder;
each expires after 30 seconds. Pages contain at most 128 entries. Completion or
expiry does not imply the receiver accepted any entry. `SNAPSHOT_EXPIRED`
requires a fresh token and cursor `0`; materialized rows prevent concurrent
versions from being skipped or inserted into an older page sequence.

Chunk baseline: fixed 1 MiB chunks, final chunk shortened, SHA-256 per chunk and full file. Zero-length file has no chunks and the standard empty-file digest. Validate ordered lengths sum exactly to file size, no overflow, chunk count/size caps, hash encoding, and kind/manifest consistency. Repeated chunk hashes are permitted; positions are defined by the manifest. Whole-file validation catches incorrect ordering.

Prioritize current heads and unresolved conflicts, then fetch advertised retained historical payloads that are eligible under the receiving replica's retention policy and remaining budget. A peer may already have expired a historical payload; expose that gap rather than promising identical historical byte availability. All accepted ancestry metadata remains part of reconciliation. Receiver downloads only locally missing verified chunks. Interrupted unverified chunks may be discarded/restarted; verified chunks survive restart. Do not claim mid-chunk byte-offset resume. A changed prefix may shift chunk boundaries and reduce savings; benchmarks must include this limitation.

## 7. Receipts and status

`METADATA_KNOWN`, `CONTENT_PENDING`, `STORED`, `APPLIED`, `CONFLICT`, `BLOCKED`, and `CONTENT_EXPIRED/UNAVAILABLE` are distinct dimensions/views, not one universally monotonic enum. A version may be stored and conflicting. Applied status changes after local edits.

A direct durable receipt is issued only after verified required content and accepted metadata meet [persistence](persistence.md) durability boundaries. A tombstone/directory needs durable metadata, not file chunks. Lost receipts can be reissued idempotently. No receipt is sent merely after parsing a message or writing an unflushed temporary file.

Receipt of a successor does not imply possession of ancestor payloads. A receipt records historical storage, not permanent retention or present liveness. Poll authorized availability to choose repair/fetch sources. Do not forward a peer's alleged receipt as direct proof from that peer; forwarded claims, if displayed, must be labeled indirect. In a hub topology the client may know only that the hub stored its version until it obtains direct status from the other device. Causal convergence does not require global status omniscience.

## 8. Enrollment, retirement and old histories

Enrollment uses an approved membership revision and an existing-peer metadata snapshot. Before applying remote contents, scan/import the joining device's existing files as independent histories and preview divergence. Absence during bootstrap creates no tombstone. Initial content that differs from an existing remote tombstone is a conflict, not automatically an intentional restoration. Equal-byte independent imports retain causal distinctions; optimize only via an explicit reviewed adopt-existing operation.

Retirement baseline: pause membership-dependent exchange/GC; converge known metadata among all surviving members; produce an identical retirement record and next membership revision; explicitly approve/import it on each survivor; then resume. If a survivor is unavailable, wait or explicitly retire it too. This is an operator maintenance procedure, not a consensus protocol.

The retirement snapshot is a canonical stream that identifies the exact
accepted versions of the retired author included by the survivors. Its
encoding is the ASCII domain separator `filesync-retirement-v1` followed by
NUL, folder ID, big-endian `uint64` configuration revision being retired,
retired device ID, big-endian `uint64` entry count, then
`(author counter, 32-byte immutable-envelope digest)` entries sorted by
counter. Duplicate counters are invalid. Transport may page this stream by
entry offset, but approval is for the SHA-256 of the complete canonical
stream. The next membership revision carries that digest and retired-at
revision. Golden bytes are in
`tests/designgates/testdata/retirement-snapshot-v1.hex`.

Those exact records remain forwardable historical records. Previously unknown
versions authored by the retired identity, or a known ID with a different
envelope digest, are rejected/quarantined for owner recovery rather than
silently admitted on the strength of an old revision. Previously unseen
successor histories that depend on rejected ancestors are also blocked.
Retired components remain in vectors; do not renumber identities or truncate
causal ancestry.

The owner preview explicitly states that uncaptured/unexchanged changes on the retired device are not imported by retirement. Recover its filesystem contents by enrolling under a new identity with a preview. The D3 fixtures and model freeze these artifacts and admission outcomes for P09. The chosen conservatism is intentional: no automatic, partition-tolerant membership changes in v1.

## 9. Conditional convergence claim

For an agreed membership configuration, after local writes stop and valid required histories/content can flow through a temporally connected peer graph, fair retries and successful local processing should eventually yield equivalent accepted head/conflict state at participating replicas. Working-folder equivalence additionally requires resolved structural/content conflicts, available content, supported paths, and successful publication.

Convergence is not promised while disk pressure blocks ingestion, all copies of required bytes are lost, configurations disagree, or an authorized participant fabricates histories. Finite model/fault tests support the stated scenarios; they are not a general proof.

## Inventory admission after release validation

Inventories are fetched in pages of at most 128 summaries into a temporary,
budgeted spool under `state/incoming`, before slower content/storage work.
The 1,024-entry limit applies to an individual ancestry fetch, rather than the
size of a folder. Each pass streams the spool and releases per-fetch caches.
Storage admission writes a bounded page at once; checking the entire storage
tree per summary made an unchanged populated replica exceed snapshot expiry.
Snapshot expiry restarts the inventory with a truncated spool. A cursor that
fails to advance is rejected. Completed sender snapshots are released after
successful row iteration; a lost final response can restart inventory safely.
Explicit retryable HTTP 429/503 responses use at most five metadata attempts
with 50/100/200/400 ms cancellable backoff. Transport delivery remains
idempotent, with no exactly-once claim.

Chunk fetches also retain at most five attempts with the same initial backoff.
Their cooldown is shared by a sync session's chunk pool: successful workers
wait when another worker observes backpressure. This prevents fast workers
from consuming every refilled server token while another exhausts its retries.
Cancellation interrupts cooldown waits. Persistent pressure still produces a
visible exhausted state for the scheduler's bounded later work cycle.
