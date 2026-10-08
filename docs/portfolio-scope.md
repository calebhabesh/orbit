# Approved portfolio scope

## Owner-directed native WAN amendment — 2026-10-05

The owner selected a comprehensive native-networking build and simple Syncthing-like
TUI/CLI setup, then confirmed preconfigured Orbit connection services with optional
self-hosting. [WAN UX](orbit-wan-ux.md), [architecture](orbit-wan-architecture.md),
[network protocol](orbit-wan-protocol.md), [plan](orbit-wan-implementation-plan.md)
and [W status](implementation/wan-status.md) own this expansion. This supersedes
the earlier exclusion of Orbit discovery/traversal/relay infrastructure and the
mandatory existing-network prerequisite for ordinary cross-network setup.

Approved direction is not an implementation or availability claim. The first
release targets authenticated discovery/rendezvous, encrypted relay fallback,
direct TCP/IPv6 and QUIC with ICE/STUN, without requiring Tailscale or manual
addresses in normal onboarding. Supporting services remain necessary on networks
where direct routes fail; their operator, profile, budgets and readiness are
required delivery evidence. Preserve single-owner trusted replicas, Linux support,
all causal/durability guarantees and explicit folder approval. P/O/T evidence and
unfinished technical acceptance remain; personal use/explanation were removed as requirements on 2026-10-07.

## Owner-directed requirement removal — 2026-10-07

The owner removed the P17 personal-use and unaided learning/explanation
requirements. They are no longer completion criteria or follow-up work for any
plan. Historical P17 pilot records and data stay as dated evidence. No
statement may claim personal adoption or owner understanding, because none was
measured.

## Owner-directed delivery amendment — 2026-10-04

The owner deferred personal-use observations and the unaided learning/explanation
review until after project delivery. They are follow-up activities, not engineering
completion gates. Finish automated validation, product polish, measured resource
behavior and evidence-backed portfolio artifacts now. Retain historical P17 pilot
records and data; do not claim automated campaigns establish personal adoption or
owner understanding. Required technical checks and declared engine guarantees
remain in force; report unavailable network/host conditions explicitly.


Approved through the planning interview on 2026-09-20, with Orbit amendments
on 2026-10-01, the terminal redesign on 2026-10-03 and native WAN on 2026-10-05. Supersedes the
2026-09-12 two-peer-only release scope. These are requirements, not implementation
claims. The repository owns Orbit's detailed scope; the parent portfolio
blueprint summarizes it.

## Orbit product direction — 2026-10-01

The owner selected **Orbit** and then approved terminal UX recommendations
Q1–Q14 on 2026-10-03. The current direction is a background sync daemon with
a small TUI and independent CLI; ordinary file management uses existing tools.
The table reflects the terminal and native WAN amendments. Engine guarantees
S01–S22 remain with S18's terminal presentation; S23–S28 and the revised exclusions
define the new networking target. [WAN UX](orbit-wan-ux.md) owns amended setup/
network journeys; [terminal UX](orbit-terminal-ux.md) owns the remaining baseline.
The [WAN plan](orbit-wan-implementation-plan.md) sequences W00–W17 and retains
the [terminal plan](orbit-terminal-implementation-plan.md)'s T13 technical checks.
The [earlier product brief](orbit-product.md) retains browser-era context.
Approval defines the target, not completed behavior or a proven guarantee.

| ID | Requirement / planning default | Maturity |
| --- | --- | --- |
| U01 | User-facing product name Orbit; existing identity/history remain intact through rebranding | Approved name; compatibility baseline |
| U02 | One owner, owner-operated storage, Linux only, complete local copies of joined folders | Approved |
| U03 | Equal writable replicas; an always-on Pi/NAS/VPS is optional and has no conflict authority | Approved |
| U04 | Suggest `~/Orbit`; existing local folders and additional named synced folders are normal options; review preexisting contents and preserve Change location | Approved terminal UX |
| U05 | Short-lived invitations from an enrolled device, explicit owner approval, persistent per-device authentication; defer Google/account login | Approved |
| U06 | Automatic LAN/WAN connectivity through Orbit discovery/rendezvous, direct paths and encrypted relay fallback; existing manual/private networks remain supported | Approved WAN target; unimplemented |
| U07 | Sync-manager journeys with names, paths and qualified plain-language status; ordinary browsing/editing stays in existing file tools | Approved terminal UX |
| U08 | Small Go TUI as interactive front door and independent CLI over the same local control operations; OS/SSH administration remains | Approved terminal UX |
| U09 | Human device/folder names distinct from cryptographic identities; directory-aware file commands and explicit ambiguity handling | Approved terminal UX |
| U10 | Setup, invitation/approval, explicit folder sharing, status, history, reviewed external-tool conflict workflows and restore; existing file mutation operations remain compatible | Approved terminal UX |
| U11 | Persistent attention, history and Deleted files retain reviewed conflicts and conditional restore, with replacement preview and separate-copy recovery; no fixed trash window | Approved terminal UX |
| U12 | Offer startup at OS login and documented explicit unattended user-service configuration; client exit leaves the daemon syncing | Approved terminal UX |
| U13 | Preserve existing state paths, roots, reserved scratch names, wire identities and legacy CLI/service compatibility during migration | Planning default |
| U14 | Recovery through a surviving trusted device with retained OS/SSH access; replacement gets fresh identity; no remote account-recovery promise | Planning default preserving existing guarantees |
| U15 | Light Vim navigation plus arrows/Tab, visible focus/context actions, narrow/colorless fallback and clear feedback; no status conveyed by color alone | Approved terminal UX |
| U16 | Guided terminal create/join/approval with reviewed existing files, editable finite settings and restartable progress; independent scriptable CLI/control operations | Approved terminal UX |
| U17 | Preconfigured Orbit connection services with optional self-hosted profile, visible metadata/privacy choices and local-only/manual modes | Approved WAN target; unimplemented |
| U18 | Ordinary create/invite/join/approve requires no separate service account, Tailscale setup, IP/port entry or prior shared LAN; technical settings remain under Advanced | Approved WAN target; unimplemented |

Implement approved choices and planning defaults through the terminal packets. Refine
reversible implementation details with evidence. Changes to single-owner trust,
supported platforms, retention/availability guarantees or excluded capabilities
still require owner input. Network enrollment and automatic distribution of
owner-approved membership are planned extensions: their design gates must close
and their owning specifications must be updated before dependent implementation.

## Purpose and positioning

Build a useful Linux folder-sync product demonstrating causal reconciliation, distributed failure handling, durable local storage, and engineering ownership. One owner uses ordinary editors and file managers across independent writable replicas. The primary resume story is preservation of captured work and predictable recovery, supported by measurements rather than technology count.

No timeline is imposed. Completion is bounded by these guarantees, not by optional feature growth. The owner plans a comprehensive review of the invariants, failure traces, and tests after delivery.

## Release requirements

| ID | Approved requirement |
| --- | --- |
| S01 | Go background agent and CLI; SQLite metadata; managed immutable content; versioned authenticated HTTPS. |
| S02 | Linux first. Two-peer initial slice; three-host release validation using laptop, Pi, and Oracle VPS. Optional fourth workstation. No unlimited-scale claim. |
| S03 | Equal replicas with third-party version forwarding. An always-on VPS is supported without conflict authority. Explicit pairing over automatic LAN/WAN or configured manual/private paths. |
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
| S18 | Filesystem notifications plus periodic scans; continuous agent operation; independent CLI and small TUI over shared controls. A future GUI can reuse those controls. |
| S19 | Trusted plaintext replicas. Authenticate peers, restrict folder access, validate even authenticated input, and protect the loopback control interface. |
| S20 | Versioned protocol and database; clear incompatibility errors; deliberate migrations and recovery procedures; Linux binaries for tested device architectures. |
| S21 | systemd user service, structured diagnostics, sensitive-data-conscious support export, upgrade and uninstall preserving user data. |
| S22 | Independent reference model, reproducible failure harness, local disposable demo, actual three-host demo, and honest benchmark/case-study artifacts; personal pilot/review deferred until after delivery. |
| S23 | Authenticated local/global candidate discovery and rendezvous with finite leases, pinned identities and bounded candidate/coordination state. |
| S24 | Opaque encrypted relay fallback with endpoint authentication, no relay folder authority/storage receipts, and strict separation from local owner control. |
| S25 | Direct reachable TCP/IPv6 and QUIC/ICE/STUN paths, bounded direct/relay selection and network-change recovery; no universal NAT/firewall reachability guarantee. |
| S26 | Actual operated default service profile, optional self-hosting, documented metadata/retention/bandwidth limits and reviewed local-only/manual privacy modes. |
| S27 | Simple first-time WAN CLI/TUI onboarding without Tailscale/manual addresses/prior LAN, retaining exact device/folder approval, root review and restartable operations. |
| S28 | Bounded secure network implementation, truthful route diagnostics, compatible migration and reproducible simulated plus real WAN failure/resource evidence. |

## User workflow

1. Initialize an identity, register a verified root, and explicitly approve peers and folder membership.
2. Preview existing files before initial reconciliation; capture divergent contents without destructive replacement.
3. Edit normally; let bounded scans capture stable versions and synchronization transfer required chunks.
4. Work offline, reconnect directly or through another replica, and inspect causal or structural conflicts.
5. Resolve reviewed versions, restore retained contents, and inspect device-specific progress.
6. Manage retention, storage pressure, corruption, unavailable roots, and device replacement through visible operations.

The pilot uses a dedicated folder of notes/documents/images and static archives. User-specific contents can be chosen when the pilot begins; no product decision depends on that choice. Fault testing always uses separate disposable data.

## Exclusions

No custom consensus, replicated database product, distributed transactions, Kafka, Kubernetes, S3 compatibility, erasure coding, custom cryptographic/transport primitives, automatic semantic merge, multi-user folder sharing, encrypted storage on untrusted replicas, mobile app, Windows/macOS support, or live application-data synchronization. The first WAN release excludes Syncthing interoperability/parity, TURN/WebRTC, router port-mapping protocols, arbitrary proxy support and public relay federation. No replication of ownership, ACLs, extended attributes, symlinks, hard-link relationships, or special files. Live databases, VM disks, active game saves, and cross-file application-consistent snapshots are outside guarantees.

A trusted participant can intentionally author bad changes; Byzantine consistency and remote erasure after revocation are excluded. Disk loss, broken hardware durability promises, and arbitrary writes through long-lived descriptors are not covered by an unconditional no-loss claim. Evidence must name the supported failure model.

## Completion

Complete the active packets in [WAN status](implementation/wan-status.md),
remaining technical acceptance in [terminal status](implementation/terminal-status.md)
and retained obligations in [engine status](implementation/status.md).
Demonstrate ordinary terminal onboarding, real edits, three-way offline conflicts,
forwarding through the VPS, restore, restart recovery, finite storage and safe
retirement. Another developer can reproduce the local demo and failure experiments
without cloud credentials. No numeric resume claims are published before measurement.
