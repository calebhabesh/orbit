# Orbit 2.3.0 publication checks — 2026-10-10 UTC

The owner authorized publishing all pending work to GitHub and checking it.
The candidate includes the previously unpushed `b962168` commit, E11/E12
onboarding polish, E13 real Leave and reviewed Remove Device, the Files-tab
arrow correction, and the centered README SVG using the actual TUI ASCII art
and color roles. Earlier dated evidence remains historical; references to
uncommitted work and unexecuted checks describe those earlier snapshots.

Review compared the staged candidate against remote main
`e75c1f6a878a9bba5d08d103d2b549fec155c952`, using
`git diff --cached e75c1f6a878a9bba5d08d103d2b549fec155c952`. The standards and
specification reviews ran independently through the code-review skill.

## Standards

All previous Standards findings are resolved. Both pending-maintenance guards
now recognize lowercase terminal phases, with regression coverage for removal
and Leave. Review-file creation uses the existing private, exclusive,
directory-synced writer and resolves relative paths before sending them to a
running controller.

The late-refusal guard checks peer retirement under the same repository lock
and SQLite transaction as the local removal marker. The drain/replay regression
covers the reported race. No remaining concrete defect was found in these
corrections.

## Spec

All four original Spec findings and the subsequent late-refusal race are
resolved. The readiness toast requires observed readiness. The pending-request
badge uses a global unexpired count across all views and idle forms. Leave
preserves a resumable removal operation; an uncommitted removal proposal is
invalidated when enrollment changes its reviewed membership. Manual sync
persists learned device removal through the same operation as scheduled sync.

The reporting-peer retirement check shares the marker transaction and repository
lock with retirement commit, closing the race. Case-insensitive terminal-phase
guards preserve compatibility with legacy completed/aborted maintenance. No
remaining concrete defect was found in the final corrections.

Remaining findings: Standards 0 (none); Spec 0 (none).

Both axes also reviewed the last polling correction without concrete findings:
badge-only queries start on refresh ticks, preserving idle review behavior and
drafts. Canceling a badge query hands the serialized lane to the exact confirmed
Retry or Rename after the canceled reply arrives.

## Validation

Commands use `TMPDIR=/home/ethioking/.cache/orbit-work-tmp` and `GOFLAGS=-p=2`.
The focused E11–E13 race checks passed for terminal (1.144 s), control
(5.980 s), replication (3.398 s), and CLI (2.419 s); see
[the log](logs/review-fixes-final.log). Regressions first reproduced the
readiness, review-file and pending-removal defects before their corrections.

The first final core run exposed three terminal unit failures after badge polling
was added: frozen review, Retry and Rename. Badge-only polls now start on refresh
ticks instead of when a workflow opens. The original regressions and new
in-flight cancellation tests pass in the full terminal unit race run:
`go test -race -count=1 ./internal/terminal` (12.679 s),
[log](logs/terminal-race-final.log).

An initial full terminal run was stopped when the source changed during review.
An initial broad race run had mixed-source compilation failures during those
edits. Neither is passing evidence for this candidate. Final release checks run
after the source is frozen; their results are recorded below and in GitHub
Actions for the published source commit.

| Final command | Actual result |
| --- | --- |
| `make check-core test-terminal-packages` | Pass: gofmt/vet, full core units and integration, model, design gates/faults, 36 Python harness tests, amd64/arm64 builds and packages, extracted-package/install/PTY checks. [Log](logs/release-final.log) |
| `go test -race -count=1 ./internal/terminal` | Pass, 12.679 s; full terminal units including frozen review, Retry, Rename, global badge, and cancellation handoff. [Log](logs/terminal-race-final.log) |
| `make quick` | Pass, 120 s: all touched short unit suites and five real PTY suites (basic, onboarding, everyday, keys, participation). [Log](logs/quick-final.log) |

Before publication, all newly staged evidence files were scanned for private
key material and common credential/token formats; no matches were found. JSON
evidence parses. Raw terminal captures retain their original CRLF and spacing
through scoped `.gitattributes` rules. `git diff --cached --check` passes.

The README SVG was XML-parsed, rendered with `rsvg-convert`, and visually checked
in light, dark and 375 px Markdown previews. Physical WAN fault campaigns,
maximum-size/scale experiments, package container transaction emulation, and an
owner Leave/removal walkthrough remain unexecuted. Existing E13 recovery and
offline-notification limitations still apply.
