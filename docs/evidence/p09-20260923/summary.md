# P09 Three-Peer Forwarding and Membership Lifecycle Evidence Summary

Packet P09 is complete locally under the recorded loopback-process test environment.
Third-party history propagation across an ordinary intermediary replica ($A \leftrightarrow B \leftrightarrow C$),
direct vs. indirect progress labeling, three-way offline concurrent edit reconciliation and late-arrival resolution,
canonical retirement snapshots matching Gate D3 golden fixtures, operator-approved membership distribution
(`export`, `import`, `preview`), active access termination upon peer retirement (`AuthorizePeer`),
divergent existing-folder enrollment (`enroll preview`, `enroll bootstrap`), and metadata-loss reinstall recovery
under a new identity are fully implemented and verified.

## Acceptance Criteria Verified

- **Three-peer forwarding ($A \leftrightarrow B \leftrightarrow C$)**: Replicas without direct connectivity or mutual pairing
  forward third-party authored histories and file contents transparently through an intermediate node.
  Original author identities, counters, and causal vectors are preserved byte-for-byte (**I08**).
- **Direct vs. indirect progress labeling**: Replicas record and expose `direct` flags in `peer_progress`
  and status representations, explicitly differentiating direct TCP/TLS peer contacts from transitive reachability.
- **Three-way concurrent edits & resolution late-arrival**: Three offline concurrent edits on the same path
  across $A, B, C$ reconcile via $B$. Resolving $A$ and $B$ creates a resolution version extending their heads;
  subsequently arriving concurrent edits from $C$ remain unreviewed concurrent heads (**I03**). All replicas
  converge upon topological exchange and agree with the independent DAG model oracle (**I02**).
- **Retirement snapshot canonical encoding (Gate D3)**: Implemented `EncodeRetirementSnapshot` matching the exact
  canonical wire format and golden hex fixture `tests/designgates/testdata/retirement-snapshot-v1.hex`.
  Retired author versions are sealed by SHA-256 digest into subsequent membership revisions.
- **Rejection of unknown retired-origin events**: Surviving replicas admit historical ancestry belonging to the
  retired author only if present in the approved retirement snapshot entries. Unseen versions authored by the retired
  identity after retirement are rejected with `ErrRetiredAuthorVersionRejected`. Unknown author envelopes are
  rejected with `ErrUnauthorized`.
- **Active access termination on retirement**: Retiring a peer updates local membership and terminates active
  synchronization access. Inbound requests from the retired identity fail with `ErrUnauthorized` (**I14**).
  Attempts to synchronize with stale membership revisions fail with `ErrMembershipMismatch`.
- **Membership revision distribution**: Implemented `filesync membership export`, `preview`, and `import --approve`,
  allowing operators to distribute identical sequential membership revisions and retirement bundles across survivors.
- **Divergent existing-folder enrollment (preview & bootstrap)**: New folders with pre-existing local disk files
  can be previewed and enrolled via `filesync enroll bootstrap`. Local files are captured without creating spurious
  tombstones for remote-only files, and divergent paths remain intact on disk without corruption (**I15**).
- **Metadata-loss reinstall recovery**: A replica whose local SQLite state directory is wiped reinitializes with a fresh
  device identity and re-enrolls existing directory contents safely via bootstrap.

---

## Owner Explanation Note

### 1. Why peer retirement changes cleanup assumptions

In ordinary active operation, distributed garbage collection (GC) and historical tombstone pruning are strictly constrained:
any active peer might still be partitioned, holding old working trees or causal references that require historical ancestors,
tombstones, or chunks to prove dominance and complete synchronization. As long as a peer is an active member, counterparts cannot
distinguish between a delayed legitimate sync and an obsolete branch, forcing replicas to retain speculative tombstones and
maintain open transfer availability.

When a peer is **retired**, this open-ended assumption changes fundamentally:
1. **Bounded historical universe**: The accepted causal history authored by the retiree is permanently frozen and sealed
   into a canonical retirement snapshot approved by the surviving members. No *new* versions will ever be admitted from that
   author identity (any subsequent event from that identity is rejected/quarantined as unauthorized). Replicas now possess an
   exhaustive, immutable list of every valid version that author will ever contribute.
2. **Payload retention decoupling**: While the causal metadata (envelope ID, counter, vector entries) must be retained
   indefinitely to preserve vector shapes and prevent identity renumbering, the heavy content payloads (chunks/objects) of
   superseded historical versions can be safely garbage collected once they are dominated and no longer needed for active heads
   or pinned restores. Replicas no longer risk receiving unexpected sync requests from the retired peer that demand pruned payloads.

### 2. Why this maintenance protocol is not consensus

Distributed consensus algorithms (e.g. Paxos, Raft, Viewstamped Replication) provide automatic, partition-tolerant, leader-elected
agreement among an online quorum of nodes without human intervention. The file-sync membership and retirement protocol is explicitly
**not** a consensus protocol for three design reasons:

1. **Linear operator approval**: Membership revisions form a strict linear sequence ($R_0 \to R_1 \to R_2 \to \dots$) where each
   revision is explicitly authorized and approved by an operator or administrative control action. Every revision cryptographically
   commits to the exact hash of its prior revision (`PriorDigest`) and canonical retirement snapshots.
2. **No quorum voting or automatic transitions**: There is no dynamic majority voting, no split-brain quorum calculation, and no
   automated transition behind network partitions. If a surviving node is temporarily partitioned or offline during a retirement
   operation, the cluster does not "elect a new configuration" without it; rather, the operator simply waits for connectivity or
   explicitly exports and imports the approved revision bundle onto each survivor.
3. **Safety over liveness**: Any configuration divergence immediately halts folder synchronization (`ErrMembershipMismatch`) rather
   than allowing diverging partitions to make independent progress. This design intentionally trades away partition-tolerant
   unattended reconfiguration in exchange for absolute safety: zero silent divergence, explicit audit trails, and strict owner authorization.
