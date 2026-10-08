# Reviewed membership fork recovery

A `MEMBERSHIP_FORK` means two approved chains disagree. Data remains blocked;
retrying a stale approval or importing a chosen branch cannot overwrite an
immutable revision. Review each branch's devices, key pins, current digest and
retirement records through authenticated local control on each reachable machine.
A mere offline member is not a fork and must not be retired to finish rollout.

For an actual fork, preserve every affected state directory and root. Stop each
selected daemon with `orbit stop --state /private/state` before using the
compatible stopped inspection/export commands below (T06 expands live parity).
Pause the old group on each reachable participant:

```sh
orbit folders pause --state /private/state --folder OLD_FOLDER_ID --reason MEMBERSHIP_FORK
orbit engine membership export --state /private/state --folder OLD_FOLDER_ID --file /private/branch.json
```

Inspect retained path history and conflicts. Copy the working bytes selected by
the owner into a new, nonoverlapping recovery root; export additional retained
versions into distinct new paths when they differ from the working copy:

```sh
orbit export --state /private/state --folder OLD_FOLDER_ID \
  --version AUTHOR_ID:COUNTER --out /new/recovery/unique-reviewed-name
```

Choose absent destination paths: the compatible export command replaces a named
output file. Preserve unresolved alternatives under separate names, and leave
old state/roots/history intact. An offline participant can contain uncaptured or
unexchanged bytes; inspect it when available before making completeness claims.
This procedure cannot recover unavailable/expired/corrupt payloads by assumption.

Preview and adopt the recovery root through the ordinary reviewed workflow:

```sh
orbit setup --state /private/state --root /new/recovery --label Laptop --name Recovered \
  --preview --review-file /private/recovery.json --json
orbit setup --state /private/state --request-file /private/recovery.json --json
```

The result has a new folder identity. Share it with the selected existing trusted
devices, each using a separate nonoverlapping reviewed root and fresh scoped
attempt. Persistent device identities may remain; replacing a lost/retired device
still needs fresh identity under the existing recovery rules. Inspect exact files,
stored hashes and new membership before relying on it. Keep the old groups paused;
do not remove them or imply that their historical DAGs were merged. Further owner
review decides what retained alternatives to keep, with old causal metadata intact.

`TestTerminalT05ForkRecoveryPreservesOriginalGroup` executes fork refusal, old-group
pause, separate-root adoption, identical captured byte digest and retained old
heads. The model/repository suites separately cover competing administration and
retirement/revival. This is a conservative recovery procedure, not automatic fork
resolution or a general guarantee that every offline edit was captured.
