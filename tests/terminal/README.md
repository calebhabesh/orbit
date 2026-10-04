# Terminal baseline reproductions

T00 exercises the current production HTTP handlers, real repository/workspace
operations and a real CLI/daemon child in fresh marked temporary roots. Generated
keys, synthetic capabilities and local credentials stay out of transcripts.
No personal roots, user services or host network policies are changed.

Discover the tests before running them:

```sh
go test ./... -list '^TestTerminalT00'
ORBIT_TERMINAL_BASELINE=1 go test -count=2 -v ./tests/terminal -run '^TestTerminalT00'
```

The opt-in command deliberately exits nonzero on the current implementation.
Assertions describe approved behavior, not an expectation that the bug persists.
Ordinary runs skip these cases. T02–T08 must promote the relevant cases into
passing, normally enabled regressions when repairing their owning behavior.
The T01 contracts may change provisional field names used for absent observations.

The CLI case builds a binary named `orbit` so it exercises the actual executable
dispatch. It starts the daemon with the current default launcher's arguments,
retains the direct child handle, and validates its marked state root and `/proc`
arguments before graceful SIGTERM and waiting. It does not signal a discovered
daemon. Database reopen models persisted state resumption; it is not crash or
power-loss evidence. The TLS identity case uses an unrelated self-signed fixture
server to detect capability disclosure; it does not claim a deployed exploit.

Evidence, dispositions, packet ownership and the source-only cases are recorded
in [the T00 report](../../docs/evidence/terminal-t00-20261003/summary.md).
Run broad gates sequentially: packaging tests and `make check` write the same
`dist` artifacts, so overlapping them can invalidate packaging observations.
