# P01 evidence summary

All five design gates have executable passing outcomes and matching updates in
their owning specifications. See [design-gate decisions](../../design-gates.md),
[commands](commands.md), [raw assertions](results.json), and the experiment
sources under `tests/designgates`.

The most important negative result is D1: checking target metadata before
rename does not prevent an editor's later save-by-rename from being discarded.
The selected existing-file transition is exchange-based and keeps the actual
displaced object named for recovery.

Limitations: these are focused architecture experiments, not production
modules. D1 used boundary stops rather than SIGKILL or abrupt power reset.
Only this host's Linux 7.2/ext4 combination was exercised. D2 is not the
independent P02 causal oracle. D4 is not a durable SQL/object implementation.
P03, P04, P09 and P10 retain their packet-specific fault and integration
obligations.
