# P07 bidirectional reconciliation and conflict projection evidence summary

P07 is complete locally under the recorded loopback-process test environment.
Bidirectional reconciliation, multi-head conflict detection, scaffold lifecycle,
structural conflict isolation, and the `filesync conflicts` inspection command
are fully implemented and verified against the independent causal model.

## Acceptance criteria verified

- **Offline edit/edit concurrency**: independent concurrent edits produce
  symmetric 2-head conflict sets under both delivery orders (A-then-B and
  B-then-A), matching the independent causal model DAG. Neither peer's working
  copy is overwritten.
- **Tombstone propagation without timestamp winners**: edit-vs-delete produces
  an explicit conflict with both the active file version and tombstone head
  preserved. Repeated deletes converge idempotently without error or spurious
  events.
- **Equal-byte conflicts**: identical contents written independently retain
  distinct version identities and causal vectors. Equal byte hashes do not erase
  ancestry or manufacture silent merges.
- **Executable-only changes**: changing only the executable bit (e.g. `0644` to
  `0755`) is detected during scanning, captures a new version envelope with the
  updated manifest, and reconciles across peers.
- **Working basis isolation (I04)**: receiving remote versions while a local
  editor is active never incorporates those remote heads into the local working
  basis. A subsequent local capture only extends the persisted working basis and
  the latest local-author event, ensuring concurrent remote branches remain
  unreviewed heads until resolved in P08.
- **Structural conflict isolation (I12)**: parent directory deletion concurrent
  with child file creation is detected as a structural conflict. The child file
  is preserved, parent directory removal is prevented, and automatic publication
  safely skips structurally conflicted paths.
- **Directory projection and scaffold pruning**: parent directories created to
  stage nested files are tracked in `workspace_scaffolds` and suppressed during
  scans. When an explicit directory envelope is published, the scaffold record is
  upgraded. When the last child under a scaffold is deleted by a tombstone, empty
  scaffolds are pruned from disk and the database, while preserving directories
  containing untracked files or other tracked children.
- **Protected fallback content**: when a remote head is accepted but its content
  is pending (`content_state = 'pending'`), local working content and object
  references remain protected and unmolested.
- **Scan and restart stability (I17)**: restarting processes and scanning after
  bidirectional reconciliation creates zero extra versions.
- **CLI inspection**: `filesync conflicts` displays conflict sets categorized as
  `edit-edit`, `edit-delete`, `delete-delete`, or `equal-content` in both human-readable
  tabular format and structured JSON.

## Owner explanation: why joining every known vector into a local edit can hide a conflict

If an authoring node were to join all received remote heads into the causal
parent set of an ordinary local edit, the newly created version would causally
dominate those remote heads ($V_{new} > p$ for all $p \in \text{Parents}$).

When other peers receive $V_{new}$, their dominance checks would observe that
$V_{new}$ is strictly newer than the remote heads. Under standard causal replication
rules, superseded versions are treated as obsolete history rather than active
conflicts. Consequently, the remote peer's changes would be silently discarded and
overwritten without:
1. Alerting either user to the existence of concurrent modifications;
2. Preserving the divergent working copy;
3. Recording an explicit resolution event with human-reviewed intent.

In reality, the local user never reviewed, reconciled, or merged the remote peer's
edits—the local user was simply editing their own file based on their local working
basis. To prevent silent data loss and uphold invariant **I04** ("Ordinary capture
does not implicitly resolve received but unreviewed heads"), local capture must
only advance from the persisted local working basis and the latest local-author
event (D2, I04). Received remote heads must remain separate concurrent heads until
an explicit, reviewed resolution operation is performed (P08).

## Limitations

All tests were executed on a single Linux ext4 host over loopback networking.
The arm64 binary was cross-compiled but not natively executed. No hosted CI,
multi-host physical network test, high-connection soak, or WAN network partition
campaign was performed. Conflict resolution operations (`select`, `keep-copies`,
manual merge, and restore) remain P08.
