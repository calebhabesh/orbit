# Operator Runbook: Corrupt Content and Bitrot Remediation

## Trigger and Symptoms
- Agent reports error code `CONTENT_UNAVAILABLE` or logs checksum mismatch on chunk access.
- `filesync storage check` detects corrupted chunks or missing blobs.
- Filesystem bitrot or storage media degradation.

## Guarantees
- Invariant **I04**: Every stored and transferred chunk is cryptographically addressed and verified by SHA-256 digest and length.
- Corrupted chunks are immediately quarantined into `.filesync-scratch/quarantine/` and never served to peers or published into the workspace.
- The sync engine supports fallback chunk retrieval: if a primary peer provides a corrupted chunk, the syncer requests the chunk from alternative authorized cluster members.

## Diagnostic Steps

1. **Run Storage Integrity Check**:
   ```bash
   filesync storage check --folder <folder-id> --json
   ```
   Check:
   - `corrupted_chunks`: Count of chunks failing SHA-256 digest validation.
   - `missing_chunks`: Chunks referenced in manifests but absent from local storage.

2. **Inspect Specific File History and Availability**:
   ```bash
   filesync history --folder <folder-id> --path "relative/path/to/file.ext" --json
   ```
   Inspect `content_state`: `available`, `pending`, `unavailable`, or `expired`.

## Remediation Workflow

### Step 1: Execute Storage Repair
Instruct the agent to repair corrupt versions by querying surviving peers:
```bash
filesync storage repair --folder <folder-id> --json
```
The repair pipeline:
1. Identifies corrupted or missing chunks in local object store.
2. Queries registered peers that have advertised stored receipts for those versions.
3. Downloads, verifies, and installs valid chunks.
4. Quarantines or discards invalid bytes.

### Step 2: Rescan Workspace Root
Force a cryptographic verification scan to confirm workspace files match authored manifests:
```bash
filesync work scan --folder <folder-id> --full
```

### Step 3: Reclaim Quarantined Artifacts
Once repaired, purge quarantined bytes:
```bash
filesync storage recovery reclaim
```
