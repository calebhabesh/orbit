# Operator Runbook: Incompatible Versions and Protocol Upgrades

## Trigger and Symptoms
- Agent fails to connect or replicate with a peer, reporting error `INCOMPATIBLE_VERSION`.
- Agent fails to start with error: `database schema version <db_ver> is newer than binary supported schema <bin_ver>`.
- `orbit maintenance check` reports migration incompatibility.

## Guarantees
- The replication protocol and SQLite database enforce strict version gates.
- Newer database files cannot be opened by older binaries, preventing corrupt write operations or accidental rollback.
- Database schema upgrades are transactional; failure during migration rolls back cleanly to the prior schema version.

## Diagnostic Steps

1. **Check Local Binary and Schema Version**:
   ```bash
   orbit version
   orbit maintenance check --json
   ```
   Inspect:
   - `current_database_schema`: The user_version stored in SQLite.
   - `binary_schema`: The highest schema version supported by this executable.
   - `status`: `up_to_date`, `migration_available`, or `incompatible_newer_database`.

2. **Inspect Peer Protocol Compatibility via Doctor**:
   ```bash
   orbit doctor
   ```
   Check category `compatibility`.

## Remediation Workflow

### Case 1: Binary is Older than Database
If a binary was downgraded or rolled back, the existing database cannot be safely operated by the older executable:
1. Re-install the current or newer `orbit` binary that supports `binary_schema >= current_database_schema`.
2. Do not attempt to force or edit `PRAGMA user_version`.

### Case 2: Database Needs Migration
If a newer binary was installed:
1. Stop the active service:
   ```bash
   systemctl --user stop orbit
   ```
2. Take a consistent pre-upgrade backup:
   ```bash
   orbit maintenance backup --out ~/.local/state/orbit/pre-upgrade-backup.sqlite
   ```
3. Run maintenance check:
   ```bash
   orbit maintenance check
   ```
4. Start the new binary; schema migrations apply automatically inside an isolated SQLite transaction.
5. Verify health:
   ```bash
   orbit doctor
   ```

### Case 3: Peer Protocol Version Incompatible
If connecting to a peer with an unsupported protocol version:
1. Check the peer version and release notes.
2. Upgrade the lagging peer to matching binary release.
3. Verify peer connectivity:
   ```bash
   orbit engine peers list --folder <folder-id>
   ```
