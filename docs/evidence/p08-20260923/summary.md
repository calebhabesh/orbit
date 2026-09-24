# P08 reviewed resolution, restore, and safe control replay evidence summary

P08 is complete locally under the recorded loopback-process test environment.
Reviewed resolution (`select`, `merge`, `keep-copies`), historical `restore` with
preview, `export`, and `history` inspection are fully implemented as shared control
operations with explicit reviewed heads, stale-view tokens, idempotency records,
and safe multi-step replay.

## Acceptance criteria verified

- **Idempotent replay creates one logical resolution/restore**: repeated commands
  with identical arguments and idempotency keys return the cached result with
  `replay = true` without creating duplicate versions or reapplying mutations.
- **Unseen versions arriving after resolution remain concurrent (I03)**: resolving
  a reviewed subset of heads correctly sets the resolution version's parent vector
  to exactly those reviewed heads. Any unreviewed or concurrent versions arriving
  after the resolution event remain distinct concurrent heads.
- **Pre-commit changed heads produce stale response (STALE_VIEW)**: if concurrent
  versions arrive or local edits occur before the resolution transaction commits,
  the mismatch against `expected_head_token` triggers a `STALE_VIEW` error with the
  new heads, preventing destructive retries without explicit user review.
- **Destination name collisions preserve existing files**: `keep-copies` validates
  destination paths against both active repository records and existing workspace
  files before staging copies. Collisions fail visibly with `DESTINATION_COLLISION`
  without overwriting files or corrupting state.
- **Partial multi-path operations resume without duplicate copies**: `keep-copies`
  persists each completed copy step in an in-progress idempotency record. Resuming
  after a mid-operation interruption reuses previously completed steps and finishes
  the remaining copies without duplicate files.
- **Restored executable status follows selected historical version**: restoring an
  old version with executable permissions (or normal file permissions) applies the
  exact mode bit from the selected historical manifest.
- **Content states exposed accurately**: versions across `history` and `restore --preview`
  truthfully report `ready`, `pending`, `unavailable`, and `expired`. Expired
  payloads return `CONTENT_EXPIRED` rather than empty files or corrupt data.
- **Reused idempotency key with changed arguments fails**: reusing a key with different
  request parameters returns `IDEMPOTENCY_CONFLICT` without modifying state.
- **Response loss recovery**: interrupting control operations after commit allows
  immediate client recovery of the completed result upon replay.
- **CLI interface integration**: `filesync resolve select|merge|keep-copies`, `filesync restore`,
  `filesync export`, and `filesync history` provide both human-readable and structured
  JSON output, sharing identical semantics with the core engine.

## Owner explanation: why restoring yesterday's bytes must not restore yesterday's causal vector

In causal version tracking (version vectors / DAGs), an event's causal vector defines its
exact temporal position and ancestry relative to all other versions in the distributed
system: $A \le B \iff \forall k, V_A[k] \le V_B[k]$.

If restoring yesterday's file restored yesterday's causal vector $V_{\text{yesterday}}$:
1. **Silent erasure by subsequent history**: any events authored after yesterday
   (e.g., today's edits, or a deletion tombstone $V_{\text{today}} > V_{\text{yesterday}}$)
   causally dominate yesterday's vector. When replicas exchange history, standard dominance
   rules would see $V_{\text{today}} > V_{\text{yesterday}}$ and treat the restored version as
   an already-superseded, obsolete historical event. The restoration would be instantly and
   silently erased by the very tombstone or edit the user intended to revert!
2. **Loss of concurrent branch context**: other peers might have authored concurrent work
   $V_{\text{peer}}$ in parallel with today's state. Restoring an old vector places the event
   in the past, breaking the causal relationship with current concurrent branches.

Therefore, restoration is fundamentally an **action in the present**. While it adopts
**yesterday's content bytes and metadata** (provenance), it must author a **new causal version**
whose parents are the **currently reviewed heads** of the path:
$$\text{Parents}(V_{\text{restored}}) = \text{CurrentHeads}, \quad V_{\text{restored}} = \text{DeriveVector}(\text{CurrentHeads}, \text{localAuthor}, \text{nextCounter})$$

This ensures the restoration causally dominates the previous heads (including tombstones),
survives synchronization across all replicas, and properly reflects user intent in the distributed DAG.

## Limitations

All tests were executed on a single Linux ext4 host over loopback networking.
The arm64 binary was cross-compiled but not natively executed. No hosted CI,
multi-host physical network test, high-connection soak, or WAN network partition
campaign was performed. Three-peer forwarding and membership lifecycle remain P09.
