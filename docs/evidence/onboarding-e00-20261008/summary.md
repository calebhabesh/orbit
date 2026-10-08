# E00 baseline and reproductions — 2026-10-08

State: **complete**. Every trial finding F01–F16 has a committed reproduction
through production interfaces on disposable state. All four gates are scoped in
[onboarding-gates.md](../../implementation/onboarding-gates.md). No production
code changed. [Commands](commands.md), [results](results.json), [logs](logs/).

The tests follow the T00 convention: they assert the **approved** behavior, so
they fail on the current build, and ordinary runs skip them. Opt in with
`ORBIT_ONBOARDING_BASELINE=1`. The packet that fixes each finding promotes its
case into an ordinary passing regression. It may rename provisional fields the
tests look for, such as `owner` in service status.

| Finding | Reproduction | Observed on `22f4bee` |
| --- | --- | --- |
| F01 | `tests/terminal` `…F01PackagedUnitStartsWithPort8080Taken` | packaged arguments exit: `bind: address already in use`; `service status` manual command also names `:8080` |
| F02 | `internal/scheduler` `…F02RootUnavailableScanHealsAfterLaterScan` | exhausted `ROOT_UNAVAILABLE` scan survives a completed later scan; attention keeps `EXHAUSTED_WORK` |
| F03 | `tests/terminal` `…F03ExhaustedWorkAdviceRunsWithDaemon` | the advised `orbit engine work retry` refuses: `state directory is already owned by another agent` |
| F04 | `tests/terminal` `…F04F14TerminalDaemonAndFailingUnit` | running + login-enabled; no owner, failing unit not shown |
| F05 | `internal/terminal` `…F05RevealedInvitationCopiesWhole` | code hard-wrapped into terminal-width rows past one screen; no single row copies; no character count. The reported `…` truncation did **not** reproduce in the model |
| F06 | `internal/terminal` `…F06SecretPasteReplacesAndReportsLength` | retry paste appends (800 + 1,863 = 2,663 characters); no length line |
| F07 | `internal/terminal` `…F07TruncatedInvitationIsSpecific` | truncated codes show `CONTROL_UNAVAILABLE: Retry; use orbit doctor…` |
| F08 | `internal/terminal` `…F08JoinDefaultsToAutomatic`; `tests/terminal` `…RelayJoinWaitNamesAndModes` | TUI join defaults to `manual` once the daemon has written config; the CLI's fresh routed join plans `automatic` with no profile instead of the invitation's operator, then blocks with `SETUP_BLOCKED: no such file or directory` |
| F09 | `internal/terminal` `…F09AwaitingApprovalOpensRequests` | Enter opens `day_operation` |
| F10 | `internal/replication` `…F10StatusPollingFitsInviterSourceBucket`; relay test | 23 `RATE_LIMITED` in a 150 s unapproved wait, first after 48 s |
| F11 | relay test | inviter lists the joiner as `Device b179a9ecd0b3d4af` (text and JSON) |
| F12 | `internal/terminal` `…F12StartupIsSelector` | typed `Startup (manual/login/unattended)` field, default `manual`, accepts `manualz` |
| F13 | `internal/terminal` `…F13NoGenericErrorAdvice` | 10 of 11 sampled failures show the shared generic advice |
| F14 | `tests/terminal` `…F04F14TerminalDaemonAndFailingUnit` | `service stop` → `IO_ERROR: requested service state was not observed`; `start` → `MANUAL_DAEMON_RUNNING`; detached daemon stdout/stderr are `/dev/null` |
| F15 | `tests/terminal` `…F15ReviewFileInGroupReadableHome` | `state directory "<home>" must not be accessible by group or other users` |
| F16 | relay test (passing check) | 0644 source received as 0600, as specified in persistence |

## F10 cause (input to E07)

The trial assumption was wrong: this is not an `orbit-net` limit. `orbit-net`
refuses with `QUOTA_EXCEEDED`, never `RATE_LIMITED`. The code comes from the
**inviter's** enrollment server (`internal/replication/enrollment.go` `allow`),
which keeps a per-source bucket of 5 requests a minute with a burst of 5. While
waiting, the joiner polls every 15 s (`internal/control/terminal_setup.go`).
Each `StatusV3` makes two POSTs (challenge, then signed status), so the joiner
sends 8 requests a minute. The source comment there assumes four. The routed
submit spends two more tokens up front. The deterministic bucket model refuses
75 s into the wait; the real two-daemon run saw the first refusal after 48 s.
Each refusal then shows in the operation and pushes the next contact out 60 s.

## Limitations

- F05/F06 are model-level, not a real-PTY capture. E03/E05 acceptance requires PTY.
- F04/F14 use a `systemctl`/`loginctl` stand-in on PATH under a disposable HOME,
  modeling the trial PC's enabled-but-crash-looping unit. No real user manager
  was used, and no host unit was touched.
- F01 ran with 127.0.0.1:8080 already held by an unrelated app on the dev PC.
  The test binds the port itself only when it is free.
- The relay test uses the local development profile (`w05Service`) with
  production admission limits, not the packaged release profile or the deployed VPS.
- `make check` was not run. Only opt-in tests and docs changed, and ordinary
  runs of every touched package skip the new cases.

Next eligible: **E01** (and E02, E03, E09 are also unblocked by E00).
