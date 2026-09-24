# Operator Runbook: Expired History and Pruned Retention

## Trigger and Symptoms
- Operator or client attempts to restore or export an older version of a file and receives error code `CONTENT_EXPIRED`.
- File history displays historical versions with `content_state: "expired"`.
- A peer that was offline longer than retention window reconnects and attempts to request expired chunks.

## Guarantees
- Invariant **I10**: Bounded retention prunes superseded version payloads while maintaining vector clock invariants. Pruned bytes can never be restored locally, but causality metadata is never truncated.
- The sync engine distinguishes between `available`, `pending`, `unavailable`, and `expired` content states.
- Expired history never causes silent data loss on active versions; only superseded past revisions are subject to pruning.

## Diagnostic Steps

1. **Inspect Path History**:
   ```bash
   filesync history --folder <folder-id> --path "relative/path.txt" --json
   ```
   Check the `content_state` field on historical entries:
   - `available`: Chunks are present locally in the content-addressed store.
   - `pending`: Chunks are queued or actively transferring from peers.
   - `unavailable`: Chunks are known in metadata but currently not held locally.
   - `expired`: Chunks have aged out beyond the retention window and were purged by GC.

2. **Inspect Folder Retention Policy**:
   ```bash
   filesync storage retention preview --folder <folder-id> --json
   ```

## Operator Actions

### Step 1: Clarify Expired Content vs Current Content
Explain to user/operator:
- The metadata entry (timestamp, author, and digest) is permanently retained in causal history.
- The raw chunk bytes for this specific historical version were deleted to conserve local storage according to retention rules (e.g., 30 days retention or minimal superseded versions).
- Current head versions are **never** expired.

### Step 2: Check Remote Peer Availability
Even if chunks have expired locally, another participating replica (such as an always-on VPS or NAS with larger retention settings) may still retain the chunks.
```bash
# Attempt to fetch or inspect availability across peers
filesync storage repair --folder <folder-id>
```

### Step 3: Adjust Retention Policy Going Forward
If longer history retention is required for this folder:
```bash
filesync storage retention change --folder <folder-id> \
  --retention-days 90 \
  --min-superseded 5
```
Preview the anticipated storage requirements:
```bash
filesync storage retention preview --folder <folder-id>
```
