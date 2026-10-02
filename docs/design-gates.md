# P01 design-gate outcomes

Status: closed by executable experiments on 2026-09-21. These are v1
implementation contracts, not general proofs. Sources are in
`tests/designgates` and recorded results are in
`docs/evidence/p01-20260921`.

Orbit revamp design gates (G01–G05) are formally closed in [Orbit design gates](orbit-design-gates.md).

## D1 — Linux capture and publication races

### Counterexamples and traces

`TestD1PreRenameStatCannotCloseSaveByRenameRace` stats an existing target,
lets an editor install a new inode by rename, and then performs the proposed
plain stage rename. The editor's inode loses its only name despite the earlier
stat having succeeded. This is the concrete reason a pre-rename stat cannot
eliminate the publication race.

`TestD1ExchangePreservesObservedOverwriteAndSaveByRename` repeats the schedule
with `renameat2(RENAME_EXCHANGE)`. The newly installed editor inode remains
named in scratch, as does an inode modified by ordinary in-place overwrite.
Its open-descriptor case then writes after exchange: the
visible remote file stays unchanged, while the write reaches the displaced
scratch inode. `TestD1RecoveryAtEveryExchangeGapPreservesVariants` stops after
staging, exchange, and recovery naming; old and new bytes remain named at
each stop. `TestD1DescriptorRootedOpenRejectsSymlinkSwap` replaces an
intermediate directory with a symlink and observes `openat2` reject it with
`ELOOP`.

### Implementation rule

- Capture through a descriptor rooted at a verified root. Compare
  device/inode, size, mtime, ctime, byte count and path identity before and
  after one streaming hash. Retry boundedly on change; do not double-read
  every quiescent file.
- Stage and flush on the target root's filesystem. Publish a new path with
  `RENAME_NOREPLACE`. Publish over an existing regular file with
  `RENAME_EXCHANGE`, retain the actual displaced object under a unique
  recovery name, flush directories, then commit the working basis.
- Use `openat2` with
  `RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS|RESOLVE_NO_XDEV` after acquiring the
  root descriptor. A missing required kernel/filesystem operation blocks
  automatic publication; there is no plain-rename fallback.
- Recovery inspects actual types, identities and verified contents rather
  than trusting the last journal phase. Ambiguity preserves variants and
  blocks the path.

### Supported assumptions and rejected alternatives

The supported automatic cases are quiescent files, completed save-by-rename,
and in-place edits detected before exchange. A writer holding a descriptor
across replacement may continue changing the displaced inode indefinitely;
v1 preserves the observed candidate but does not promise to capture every
later write. Rejected alternatives are a pre-rename stat plus plain rename,
cross-filesystem staging, a timing delay as proof that writers closed, and
unconditional double hashing as a supposed concurrency proof.

The experiment ran on Linux 7.2/ext4. Its boundary stops are process-crash
state prototypes; P04 still owns journal implementation and SIGKILL recovery,
and P16 owns abrupt-reset evidence.

## D2 — Working basis and same-author lineage

### Counterexamples and traces

The three-author model creates concurrent A1/B1/C1, a reviewed A2 resolution
of A1/B1, and confirms C1 remains a head. Equal bytes in A1/B1 remain distinct
heads. An ordinary A edit whose working basis is A1 after B1 arrives creates
A2 from A1 only; adding B1 would falsely claim review. A second case has an A
working copy still based on A1 after A2 exists and demonstrates why minting
another A sibling is invalid.

### Implementation rule

Ordinary capture uses the persisted working basis plus the latest accepted
same-path event by the local author. Received heads are not implicit parents.
If the basis neither contains nor causally covers that latest local-author
event, preserve the candidate bytes and block event creation for review.
Resolution parents are exactly the reviewed head set protected by its stale
view token. Content equality never changes ancestry.

### Assumptions and rejected alternatives

The model assumes validated parents and focuses only on the gate seam; P02
will build the independent full DAG oracle. Rejected alternatives are using
all known heads as an ordinary edit's parents, allowing same-author siblings,
and coalescing causally distinct equal-byte versions.

## D3 — Membership retirement artifacts

### Counterexamples and traces

The model admits a retired author's exact `(counter, envelope digest)` entry,
rejects a later unknown old-epoch event, and rejects an active author's new
event when it depends on rejected ancestry. A mismatched membership digest
pauses admission. Canonicalization tests permute input order and reject
duplicates. Golden fixtures freeze the byte encodings and their semantic
digests:

- membership:
  `84560dcb0304c7a7b4d3aa5eb0ed4420577d46b68ef9c40692410e7f30f7987d`
- retirement:
  `09629ab3321e761ac3844c7120784a63e380d417c6d6f9b924bd5ee22a87638d`

### Implementation rule

Membership revisions and retirement snapshots use the domain-separated,
sorted, fixed-width encoding specified in `protocol.md`. Survivors first
converge metadata, approve the exact retirement-stream digest, import the
same next revision, and only then resume exchange and membership-dependent
cleanup. Known retired records remain forwardable. Unknown or digest-mismatched
retired records and successors with rejected ancestry are quarantined. A lost
device is waited for or explicitly retired; recovered filesystem content
returns through bootstrap under a new identity.

### Assumptions and rejected alternatives

This is an owner-coordinated linear configuration, not consensus. The flat
canonical stream can be transported in entry-offset pages but approval hashes
the complete stream. P05 will freeze transport framing. Rejected alternatives
are accepting any version signed by an old configuration, truncating retired
vector components, silently changing configuration during a partition, and
reusing a lost identity.

## D4 — Content references and interrupted GC

### Counterexamples and traces

The reference model enumerates every ordering of GC intent, unlink and a new
publication reference that respects intent-before-unlink:

1. reference first: intent and unlink are blocked;
2. intent then reference: the reference cancels intent and unlink is blocked;
3. intent, unlink, reference: verified content is reinstalled before the
   reference commits.

An active serve lease excludes intent. Crash-state cases after intent retain
extra bytes; after unlink, absence is allowed only with no protected root.
Expiry tests confirm current heads, pending publication and active restore
remain protected, while a superseded payload is not pinned forever merely
because an enrolled peer is offline.

### Implementation rule

Compute candidates at a metadata generation and acquire durable deletion
intents through the repository's serialized mutation boundary. Reference and
lease admission consult the same boundary. New durable references cancel the
intent; if unlink already happened, they reinstall verified bytes before
commit. Startup reconciles intents before new references or GC. Protected
roots are the list in `persistence.md`; age and "might be useful" are not
proof of safety.

### Assumptions and rejected alternatives

The executable model is an interleaving oracle, not the P10 database and
filesystem implementation. P03 must implement durable pins and P10 must rerun
the schedules at SQL/object fault boundaries. Rejected alternatives are
age-only orphan cleanup, an unlocked mark/unlink pass, leases that begin
after intent, and permanent payload pins for offline peers.

## D5 — Root, directory and bootstrap projection

### Counterexamples and traces

The model applies a remote nested file and marks its created parents as
scaffolds; the next scan produces no local directory events. An explicit
empty directory remains observable. File/child and tombstone/child schedules
both yield structural conflicts while retaining the child. A new child
invalidates an earlier directory-delete generation. Bootstrap, unverified-root
and incomplete-subtree scans infer no tombstones. Divergent enrollment bytes
become independent heads.

Path fixtures accept exact UTF-8 relative slash paths and reject absolute
paths, empty/dot segments, backslashes, invalid UTF-8, NUL, reserved scratch
segments and configured byte/depth limits. A root-identity test replaces the
registered directory while copying its marker, then changes only the marker;
the device/inode/marker tuple detects both mismatches.

### Implementation rule

Root registration and revalidation follow `persistence.md`: no symlinked path
components, no root/state overlap, device/inode plus a random marker, and
descriptor-rooted operations that reject nested mounts. Projection-created
parents are durable scaffold basis records, not authored versions. Explicit
directories, including empty ones, are versions. Child existence requires
directory-shaped ancestors; incompatible file/tombstone ancestry blocks
materialization without recursive deletion. Absence creates tombstones only
under a verified root after complete enumeration outside bootstrap mode.

### Assumptions and rejected alternatives

Device/inode plus a marker detects ordinary unmount/replacement mistakes, not
a privileged actor deliberately reproducing both. Unicode and case are not
normalized. Rejected alternatives are treating bootstrap absence as deletion,
turning every scaffold into a local edit, recursively replacing structural
conflicts, trusting a marker without filesystem identity, and inferring
deletion from a partial scan.
