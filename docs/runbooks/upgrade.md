# Safe upgrade

Use the exact state path and service unit of the installation being upgraded.
The [installation/adoption guide](install.md) owns the terminal upgrade procedure:
stop that daemon, create a consistent SQLite backup, protect configuration/keys,
settings and content objects, run preflight, install the compatible binary, then
inspect schema/recovery/doctor results before reopening replication.

Supported additive migrations run under exclusive ownership. Newer schemas are
refused. A partial recovery marker is a fenced operation requiring the documented
recovery procedure; never delete its marker to force startup. Preserve all roots
and private scratch/spool candidates. A cross-filesystem relocation retains its
original safety copy until explicitly inspected.

Standalone upgrades retain existing customized units and do not enable startup.
Debian/RPM managed unit aliases are one service. Package scripts can reload and
try-restart only an already active service in the invoking user's manager;
root installation cannot silently manage another user's session. If deliberately
stopped for backup, restart the selected service explicitly after health checks.
A direct manually started daemon also requires explicit supervised restart.

Review compatibility beyond schema: runtime settings and unfinished terminal
operation/enrollment/editor ledgers must be understood by the candidate binary.
[Binary rollback](rollback.md) preserves current counters; [old metadata recovery](database-recovery.md)
creates a fresh identity and reenrollment. Never restore old counters under the
old key/author. Native login/logout/boot and actual host/network behavior require
separate T13 observations; local package transactions do not establish them.
