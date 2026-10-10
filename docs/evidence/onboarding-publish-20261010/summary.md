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

## Publication and trial verification

Source `f93575a041075df5611e1291f92ef36a9a608a83` and the previously unpushed
commit were pushed to `main`. Follow-up
`dae0ea884c45d022a24cf8aa991bc05d81468afd` changes only the trial shell helper:
the two-line version display now consumes the entire output instead of closing
the pipe early with `head`. `bash -n` and status calls on all three hosts pass.
The initial Pi status call failed once; ten direct retries passed before this
robustness correction. No root cause is inferred from that one failed call.

The GitHub README and SVG were fetched through the authenticated read API;
the SVG is byte-identical to the local file. The rendered README API response
contains the centered image, its 560 px width, and descriptive alternative text.

Private consistent metadata and prior-binary backups were taken immediately
before `make trial-install`. Installation passed (6 s), followed by all three
read-only verification helpers and `make trial-status`. The PC and laptop run
`dae0ea8`, 2.3.0/schema 15, preserving identity, root, membership, recorded history
and four ordinary files per host. Both report observed Ready with zero missing
content, pending publication, conflicts or attention items. The Pi executes the
native arm64 binary and remains unconfigured/stopped. Private backup manifests
stay on their hosts. Public verification summaries and binary checksums are in
[results](results.json); install and status logs are retained here.

`gh workflow run full.yml --ref main` dispatched the
[full native amd64/arm64 suite](https://github.com/calebhabesh/orbit/actions/runs/38011309012)
for `f93575a`. The shell-only follow-up started
[fast CI](https://github.com/calebhabesh/orbit/actions/runs/38011492958).
The first full run's failure and the corrected runs are recorded below.

## WAN cancellation correction found by full CI

The first native arm64 race job failed in the existing
`TestWANW01TransportCancellationRedirectAndBinding/direct/cancel`: a canceled
request returned a successful response ([filtered job log](logs/github-arm64-race-failure-excerpt.log)).
The other arm64 core, integration, repository, workspace, terminal and fault
race packages passed. Thirty focused direct-cancellation repetitions passed
locally (23.427 s), so that initial loop did not reliably reproduce the timing.

The deterministic `TestWANW01TransportCancellationWinsCompletedResponse`
exercises the real Orbit transport and HTTP client with an adapter delivering
response headers after canceling the caller. Before correction:
`go test -race -count=1 ./internal/network -run '^TestWANW01TransportCancellationWinsCompletedResponse$'`
fails in 0.004 s with `response=true, error=<nil>`
([red log](logs/wan-cancellation-deterministic-red.log)). The transport now
checks the caller context at return, closes the discarded body, and returns
the context error. This check covers direct, relay and QUIC responses.

The same regression with `-count=100` passes in 1.015 s
([green log](logs/wan-cancellation-deterministic-green.log)). Both standards and
spec reviewers found no concrete issue in the correction; the WAN architecture
and protocol now record cancellation taking precedence over concurrent response
delivery. The original direct/relay cancellation, redirect and pin-binding
fixture, full network race suite, and core/package gates are rerun on the fix.

`go test -race -count=30 ./internal/replication -run '^TestWANW01TransportCancellationRedirectAndBinding$'`
passes all direct/relay cancellation, redirect and target-pin cases (181.557 s,
[log](logs/wan-cancellation-fix-full-fixture.log)).
`go test -race -count=1 ./internal/network` passes the complete network race
suite (122.478 s, [log](logs/network-race-cancellation-final.log)). The core and
package gates pass again on the corrected source with
`make check-core test-terminal-packages`
([final log](logs/release-cancellation-final.log)).
That local command began with the correction already present, before its commit
was created; its early build metadata therefore names `dae0ea8`. The application
source remained fixed throughout the run. The subsequent trial install and
GitHub jobs build the committed `0b4ac78` source.

The correction is published as
`0b4ac78f35392706a3d3b7446c41cf87f92548e9`. Fresh private backups preceded
another successful `make trial-install`, followed by all three verification
helpers and `make trial-status`
([install log](logs/trial-install-cancellation-final.log),
[status log](logs/trial-after-cancellation-final.log)). All three installed
binaries report `0b4ac78`; the PC and laptop remain Ready with the same identity,
root, membership, recorded history and four ordinary files each. Their status
reports no missing content, pending publication, conflicts or attention items.
The Pi remains unconfigured/stopped. Final host checksums are in
[results](results.json).

The corrected source starts
[fast CI](https://github.com/calebhabesh/orbit/actions/runs/38013224728), which
passes, and a fresh
[full native amd64/arm64 matrix](https://github.com/calebhabesh/orbit/actions/runs/38013228435).
The earlier full run ended canceled after its arm64 cancellation failure; five
jobs passed and the two unfinished terminal jobs were canceled by the new full
dispatch. It is retained as failure evidence, not credited as a full pass.
The fresh full run completes successfully with all eight jobs passing.

The corrected native arm64 race job passes, including complete network
(121.685 s), replication (276.867 s), control (1016.847 s), repository,
workspace, terminal, integration, model and fault packages
([package summaries](logs/github-arm64-race-corrected-excerpt.log)).

## Final GitHub verification

Both final workflows pass for
`0b4ac78f35392706a3d3b7446c41cf87f92548e9`:
[fast CI](https://github.com/calebhabesh/orbit/actions/runs/38013224728) and
[the full suite](https://github.com/calebhabesh/orbit/actions/runs/38013228435).
The full suite runs `make check-core`, `make test-terminal`,
`make test-terminal-packages`, and `make test-race-core` on each native
architecture. All eight jobs pass, including both full terminal suites and both
core race suites. Exact job URLs, start/end times and conclusions are in
[results](results.json).

The final follow-up commit contains only documentation and evidence. The
application source remains identical to the tested and installed `0b4ac78`
source. Historical failed or canceled runs remain labeled separately; the
unexecuted physical campaigns and E13 recovery limitations above remain.
