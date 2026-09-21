# Approved portfolio scope

Approved through the planning interview on 2026-09-20. Supersedes the 2026-09-12 two-peer-only release scope. These are requirements, not implementation claims. The repository owns File Sync's detailed scope; the parent portfolio blueprint summarizes it.

## Purpose and positioning

Build a useful Linux folder-sync product demonstrating causal reconciliation, distributed failure handling, durable local storage, and engineering ownership. One owner uses ordinary editors and file managers across independent writable replicas. The primary resume story is preservation of captured work and predictable recovery, supported by measurements rather than technology count.

No timeline is imposed. Completion is bounded by these guarantees, not by optional feature growth. The owner must be able to explain the invariants, failure traces, and tests despite AI-assisted implementation.

## Release requirements

| ID | Approved requirement |
| --- | --- |
| S01 | Go background agent and CLI; SQLite metadata; managed immutable content; versioned authenticated HTTPS. |
| S02 | Linux first. Two-peer initial slice; three-host release validation using laptop, Pi, and Oracle VPS. Optional fourth workstation. No unlimited-scale claim. |
| S03 | Equal replicas with third-party version forwarding. An always-on VPS is supported without conflict authority. LAN or existing private network, explicit addresses and pairing. |
| S04 | Folder-level membership and authorization; full current folder replication, including unresolved conflicts, plus retained history under policy. No selective placeholders. |
| S05 | Explicit owner-approved enrollment and retirement. Preview existing content on enrollment; missing bootstrap paths are not deletions. Reinstalled devices use new identities. |
| S06 | Ordinary files, empty directories, and executable status. Initial rename is delete plus create. Unsupported objects and path structures receive explicit diagnostics. |
| S07 | Durable captured versions, causal version tracking, explicit concurrent conflicts, deletion tombstones, and restore as a new version. Equal bytes do not erase ancestry. |
| S08 | Resolve by selecting a version, keeping separate copies, or supplying a manual merge. Resolve only reviewed versions; unseen updates may conflict again. |
| S09 | Preserve current working copies where feasible during conflict. Equivalent replicated head sets are required; identical working trees are not required while blocked or conflicted. |
| S10 | Fixed-size hashed chunks, whole-file verification, missing-chunk transfer, bounded concurrency/retries, and resumability at verified chunk boundaries. |
| S11 | Recoverable publication spanning metadata and filesystem. Agent-crash recovery plus documented abrupt-reset experiments under declared local filesystem assumptions. |
| S12 | Protect successfully captured versions; detect supported concurrent editing patterns. No promise to record every intermediate editor write or provide cross-file snapshots. |
| S13 | Root verification, unavailable-root pause, bulk-delete preview and mass-deletion safeguard. Inaccessible or unreadable files are not inferred deletions. |
| S14 | Finite storage budgets and safe content cleanup in the completed release. Initially disabled cleanup; pause when no safe space can be reclaimed. Offline peers are never automatically retired. |
| S15 | Verify managed content on use; integrity-check command; quarantine and peer-assisted repair; explicit unrecoverable-content state. |
| S16 | Per-device saved/stored/applied/conflicted progress and last contact. Historical receipt is not perpetual availability. Peer offline is not global synchronization success. |
| S17 | Per-path error isolation where safe; bounded retries, fair scheduling, configurable bandwidth/concurrency/storage. |
| S18 | Filesystem notifications plus periodic scans; continuous agent operation; CLI then embedded React/TypeScript/Vite UI with three focused views. |
| S19 | Trusted plaintext replicas. Authenticate peers, restrict folder access, validate even authenticated input, and protect the loopback control interface. |
| S20 | Versioned protocol and database; clear incompatibility errors; deliberate migrations and recovery procedures; Linux binaries for tested device architectures. |
| S21 | systemd user service, structured diagnostics, sensitive-data-conscious support export, upgrade and uninstall preserving user data. |
| S22 | Independent reference model, reproducible failure harness, local disposable demo, actual three-host demo, personal pilot, and honest benchmark/case-study artifacts. |

## User workflow

1. Initialize an identity, register a verified root, and explicitly approve peers and folder membership.
2. Preview existing files before initial reconciliation; capture divergent contents without destructive replacement.
3. Edit normally; let bounded scans capture stable versions and synchronization transfer required chunks.
4. Work offline, reconnect directly or through another replica, and inspect causal or structural conflicts.
5. Resolve reviewed versions, restore retained contents, and inspect device-specific progress.
6. Manage retention, storage pressure, corruption, unavailable roots, and device replacement through visible operations.

The pilot uses a dedicated folder of notes/documents/images and static archives. User-specific contents can be chosen when the pilot begins; no product decision depends on that choice. Fault testing always uses separate disposable data.

## Exclusions

No custom consensus, replicated database product, distributed transactions, Kafka, Kubernetes, S3 compatibility, erasure coding, global discovery, custom NAT traversal, automatic semantic merge, multi-user sharing, untrusted-server encryption, mobile app, Windows/macOS support, or live application-data synchronization. No replication of ownership, ACLs, extended attributes, symlinks, hard-link relationships, or special files. Live databases, VM disks, active game saves, and cross-file application-consistent snapshots are outside guarantees.

A trusted participant can intentionally author bad changes; Byzantine consistency and remote erasure after revocation are excluded. Disk loss, broken hardware durability promises, and arbitrary writes through long-lived descriptors are not covered by an unconditional no-loss claim. Evidence must name the supported failure model.

## Completion

All release packets in [status](implementation/status.md) meet their acceptance criteria. Real edits, three-way offline conflicts, forwarding through the VPS, restore, restart recovery, finite-storage behavior, and safe retirement are demonstrated. Another developer can reproduce the local demo and failure experiments without cloud credentials. No numeric resume claims are published before measurement.
