# Uninstall preserving files and state

Stop the selected daemon and disable managed startup before removing binaries:

```sh
orbit service stop --state /absolute/selected/state
orbit service disable --state /absolute/selected/state
```

Use `sudo dpkg -r orbit` for Debian packages, `sudo rpm -e orbit` for RPM,
or `bash uninstall.sh user` from the extracted standalone archive. Standalone
system installation uses `bash uninstall.sh system` with appropriate privilege.
The package name remains orbit; orbit is its binary/service alias.

Removal deletes executables, distributed completion/runbook assets, desktop/icon
and service registration. It preserves selected state, identities, SQLite/history,
objects, private operation/editor spools, registered roots and working files.
It emits no file deletion tombstones. Customized units/drop-ins outside the
removed package remain operator-owned and require explicit review.

Verify the selected state and ordinary working bytes after removal. Reinstallation
with a compatible binary can adopt that unchanged state; interrupted recovery,
missing budgets, unsupported schema and old peer capabilities still require their
specified reviews. Uninstall is not permission to reset identity or restore old
counters. See [install/adoption](install.md), [rollback](rollback.md) and
[metadata recovery](database-recovery.md). Purging private state is a separate
explicit recovery/retirement decision, not part of this procedure.
