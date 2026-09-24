# Operator Runbook: Stuck Conflict Resolution

## Trigger and Symptoms
- Agent reports content or structural conflicts on synchronized paths.
- Filesystem shows conflicting versions, or files with unresolved concurrent mutations.
- Mutating operations fail with `STALE_VIEW` if conflicting heads change during manual resolution review.

## Guarantees
- Invariant **I03**: Concurrent edits create explicit conflict heads; the sync engine never silently overwrites concurrent user data.
- Invariant **I08**: Working directory basis is never advanced by unreviewed or partially transferred heads.
- Conflict resolution commands enforce stale-preview protection: if heads change between operator preview and resolution submission, the submission is rejected with `STALE_VIEW`.

## Diagnostic Steps

1. **List All Active Conflicts in Folder**:
   ```bash
   filesync conflicts list --folder <folder-id> --json
   ```
   Inspect:
   - `content_conflicts`: Paths where multiple active versions exist concurrently.
   - `structural_conflicts`: Collisions between directory vs file, or rename collisions.
   - `head_token`: The cryptographic digest over the reviewed concurrent heads.

2. **Inspect Specific Conflict Details**:
   ```bash
   filesync conflicts show --folder <folder-id> --path "path/to/conflict.txt"
   ```

## Resolution Workflows

### Option A: Select Winning Version (LWW or Manual Pick)
To designate one specific version as the authoritative successor:
```bash
# Obtain preview and head token
filesync conflicts list --folder <folder-id> --json

# Resolve by picking the winning version ID
filesync resolve select --folder <folder-id> \
  --path "path/to/conflict.txt" \
  --winner <version-id> \
  --head-token <head-token>
```

### Option B: Keep Both Copies with Divergent Names
To preserve both files side-by-side with device-disambiguated names:
```bash
filesync resolve keep-copies --folder <folder-id> \
  --path "path/to/conflict.txt" \
  --head-token <head-token>
```
The sync engine:
1. Retains the primary file at its original path.
2. Creates sibling copies formatted as `<name>.conflict-<short-device>-<counter>.<ext>`.
3. Authors a merged resolution version subsuming all prior concurrent heads.

### Option C: Manual Three-Way Merge
1. Inspect each version payload:
   ```bash
   filesync export --folder <folder-id> --version <version-id-1> --out /tmp/v1.txt
   filesync export --folder <folder-id> --version <version-id-2> --out /tmp/v2.txt
   ```
2. Manually merge the files and overwrite the local working copy.
3. Submit merged content with the reviewed head token:
   ```bash
   filesync resolve merge --folder <folder-id> \
     --path "path/to/conflict.txt" \
     --head-token <head-token>
   ```

### Handling STALE_VIEW Errors
If `filesync resolve` returns error `STALE_VIEW`:
1. Re-query conflicts to observe newly arrived concurrent heads:
   ```bash
   filesync conflicts show --folder <folder-id> --path "path/to/conflict.txt"
   ```
2. Review the new set of heads and re-execute resolution with the updated `head-token`.
