# Orbit terminal implementation status

Native WAN amendment, 2026-10-05: follow the [W plan](../orbit-wan-implementation-plan.md)
and [W tracker](wan-status.md) for automatic networking and onboarding changes.
This tracker retains terminal baseline evidence and remaining T13 technical checks;
the WAN plan does not mark them complete or replace earlier network test provenance.

## Owner-directed delivery amendment — 2026-10-04

The owner deferred personal-use observations and the unaided learning/explanation
review until after project delivery. They are follow-up activities, not engineering
completion gates. Finish automated validation, product polish, measured resource
behavior and evidence-backed portfolio artifacts now. Retain historical P17 pilot
records and data; do not claim automated campaigns establish personal adoption or
owner understanding. Required technical checks and declared engine guarantees
remain in force; report unavailable network/host conditions explicitly.


Updated: 2026-10-04. **T00 complete** as a baseline/disposition packet; its
historical opt-in assertions remain retained. **T01 complete** for contracts and
design experiments; **T02 complete** for shared client/initialization/lifecycle;
**T03 complete** for authenticated enrollment transport/admission/approval;
**T04 complete** for reviewed setup and resumable joining (M1);
**T05 complete** for additional-folder sharing and local offline/third-peer rollout;
**T06 complete** for commands, context, output escaping, and adapter parity;
**T07 complete** for qualified copy/status, persistent attention, and actionable diagnostics;
**T08 complete** for reviewed conflict/editor/history/restore workflows and stream parity;
**T09 complete** for the keyboard shell, pinned v2 adapters and actual PTY/terminal lifetime;
**T10 complete** for reviewed keyboard onboarding, scoped sharing/approval and device/folder management;
**T11 complete** for everyday TUI management, external editor and recovery workflows;
**T12 complete** for terminal entry/packages, compatible adoption and operator runbooks.
TG1 transport/rollout, TG2 onboarding, and TG3 command adapter parity are recorded;
TG3 exact stream/editor adapters and TG4 scoped production proof are recorded;
TG5 terminal lifetime and packaged entry/adoption are verified; native modes remain T13.
Reviewed editor-screen and everyday management integration is verified; native/cross-host validation remains T13.
**T13 complete for technical acceptance (2026-10-07).** Clean release/failure, ordinary LAN,
actual-host engine, Tailscale three-host and (in a disposable KVM guest, W17)
login/logout/boot checks pass; personal use and review remain deferred.
Existing [P status](status.md) and
[O status](orbit-status.md) retain their historical evidence and P17 limitations.

Read the [terminal plan](../orbit-terminal-implementation-plan.md),
[terminal architecture](../orbit-terminal-architecture.md), and
[agreed UX](../orbit-terminal-ux.md). Changes to the approved direction are owned
by the specifications; this tracker records implementation evidence.

## Packet tracker

| Packet | State | Dependencies | Next condition |
| --- | --- | --- | --- |
| T00 | complete | none | [Baseline evidence](../evidence/terminal-t00-20261003/summary.md); production repairs remain assigned to later packets |
| T01 | complete | T00 | [Contract/design evidence](../evidence/terminal-t01-20261003/summary.md); production proof remains open |
| T02 | complete | T01 | [Client/lifecycle evidence](../evidence/terminal-t02-20261003/summary.md); native boot/logout remains T12/T13 |
| T03 | complete | T01, T02 | [Enrollment evidence](../evidence/terminal-t03-20261004/summary.md); T04 joining and T05 rollout remain |
| T04 | complete | T02, T03 | [Reviewed onboarding evidence](../evidence/terminal-t04-20261004/summary.md); cross-host/native/owner release campaign remains |
| T05 | complete | T03, T04 | [Sharing/rollout evidence](../evidence/terminal-t05-20261004/summary.md); cross-host release remains T13 |
| T06 | complete | T02, T04, T05 | [Commands/context evidence](../evidence/terminal-t06-20261004/summary.md); T07 attention and T08 recovery remain |
| T07 | complete | T02, T06 | [Status/diagnostics evidence](../evidence/terminal-t07-20261004/summary.md); T08 recovery/editor workflows remain |
| T08 | complete | T06, T07 | [Content/recovery evidence](../evidence/terminal-t08-20261004/summary.md); interactive PTY editor integration remains T11/T13 |
| T09 | complete | T01, T02, T06 | [Shell/PTY evidence](../evidence/terminal-t09-20261004/summary.md); reviewed editor screens T11, native/package/owner release T12/T13 |
| T10 | complete | T04, T05, T07, T09 | [Onboarding/device evidence](../evidence/terminal-t10-20261004/summary.md); native/cross-host/owner campaign remains T12/T13 |
| T11 | complete | T07, T08, T09 | [Everyday TUI evidence](../evidence/terminal-t11-20261004/summary.md); packages/adoption remains T12, cross-host/owner release T13 |
| T12 | complete | T10, T11 | [Entry/package/adoption evidence](../evidence/terminal-t12-20261004/summary.md); native/cross-host/owner release remains T13 |
| T13 | complete for technical acceptance (2026-10-07) | T00, T01, T02, T03, T04, T05, T06, T07, T08, T09, T10, T11, T12 | [Release evidence](../evidence/terminal-t13-20261004/summary.md); login/logout/boot passed in an owner-designated disposable KVM guest ([W17](../evidence/wan-w17-20261007/summary.md)); physical-hardware boot unexecuted; owner use/explanation deferred |

## Design gate evidence

| Gate | Design/experiment | Production evidence |
| --- | --- | --- |
| TG1 Enrollment transport | [T01 resolved](../terminal-design-gates.md) | T03 transport and [T05 local rollout](../evidence/terminal-t05-20261004/summary.md) verified; cross-host T13 remains |
| TG2 Durable onboarding | [T01 resolved](../terminal-design-gates.md) | [T04 production proof](../evidence/terminal-t04-20261004/summary.md); native release campaign pending |
| TG3 Shared adapters | [T01 resolved](../terminal-design-gates.md) | [T02 client/lifecycle subset](../evidence/terminal-t02-20261003/summary.md) and [T06 adapter parity](../evidence/terminal-t06-20261004/summary.md); [T08 exact stream/editor adapters](../evidence/terminal-t08-20261004/summary.md) verified |
| TG4 Reviewed content workflow | [T01 resolved](../terminal-design-gates.md) | [T08 scoped production proof](../evidence/terminal-t08-20261004/summary.md); interactive PTY integration T11/T13 |
| TG5 Terminal/legacy lifecycle | [T01 resolved](../terminal-design-gates.md) | [T09 shell/PTY lifetime](../evidence/terminal-t09-20261004/summary.md) and [T12 packages/adoption](../evidence/terminal-t12-20261004/summary.md) verified; native modes T13 |

The earlier G01–G05 outcomes remain historical. New source findings do not
constitute executed failure evidence, and an earlier complete label does not
establish the planned ordinary terminal journey.

## TUI library selection — 2026-10-03

Architect selected Bubble Tea v2, Bubbles v2 and Lip Gloss v2 after reviewing
official library documentation, v2 module paths and the interactive ExecProcess
operation. Rationale and module ownership are in the
[stack decision](../orbit-terminal-architecture.md#tui-stack-decision--2026-10-03).
T09 now verifies/pins that stack; T02 and its prerequisites are unchanged.

Documentation checks: `python3` inline Markdown validator passed for the six
updated documents (local targets/anchors, fences and trailing whitespace);
scoped `git diff --check` exited 0. T09 acceptance is **unexecuted**: dependency
pins, cross-builds, license inventory and actual PTY/editor/daemon lifetime proof
remain T09 work. A documented selection does not complete that packet.

## Planning handoff record

Owner request: a comprehensive block/packet plan for either 6.1 Sol Medium or 3.8 Flash High
after agreement on terminal UX recommendations Q1–Q14. Planning work includes
the packet dependency graph, scope/entry pointers, module responsibilities,
design gates, checks, release evidence and kickoff prompts.

Runtime source and existing relocation changes were preserved. Native host
availability and a usable Tailscale network have not been verified in this
planning session. Source findings are carried forward for T00 reproduction.
Documentation validation actually executed:

- `git diff --check`: passed across the tracked workspace diff.
- `python3` standard-library inline Markdown/plan validator: passed for 21
  documents and 289 local links/anchors; code fences balanced and no trailing
  whitespace.
- The same validator confirmed T00–T13 definitions exactly once, identical
  plan/tracker dependencies, an acyclic graph with T00 first, all runtime states
  pending, Q1–Q14/U01–U16/S01–S22 coverage, I01–I28 definitions, and the retained
  O14 follow-up section.

Changed only planning/specification documentation: new terminal plan,
architecture, packet details and tracker; agreed UX, scope/glossary, owning
contract/verification additions and repository/legacy entry pointers. Runtime
Go/CLI/TUI/network/service/fault/native/pilot checks remain **unexecuted** for
this planning change. Passing documentation checks completes the handoff,
not any T implementation packet.

Assignment clarification: the planning agent remains master architect/designer.
Either worker model implements any assigned eligible packet, including engine,
CLI/TUI, integration and checks, under the same acceptance criteria. Model choice
does not create a lead/presentation split or a supervision/delegation requirement.

Assignment correction validation actually executed:

- `rg` audited current terminal documents and entry pointers for model-specific
  ownership; packet/module assignments now refer to the assigned worker.
- `git diff --check`: passed.
- Inline `python3` documentation validator: 21 documents and 289 local links/
  anchors passed; all 14 packet/detail assignments are model-neutral, plan/tracker
  dependencies match and remain acyclic, generic worker prompts are present,
  and every runtime packet remains pending.

The correction changes implementation assignments and prompts only. The six
blocks, packet dependencies, product behavior and acceptance criteria remain.
Application code and runtime tests remain untouched/unexecuted for this correction.

Next action: the chosen worker reads the required documents and starts T00.
Use a resumable packet handoff when switching workers.
No per-packet approval ritual is needed. Retain P17 owner use/explanation until
its actual remaining acceptance evidence is recorded.

## Completion entry template

Append one entry per packet as work occurs:

```text
Packet/state:
Dependencies and owning contracts read:
Gate design and production evidence:
Changed files and task ownership:
Invariants/scenarios exercised:
Discovered test names/counts:
Exact commands/results, including failures and unexecuted checks:
Revision/dirty-tree provenance and evidence paths:
Known limitations and remaining acceptance criteria:
Owner learning/explanation record:
Next eligible work and concrete handoff:
```

Use pending/in_progress/blocked/complete as evidence warrants. Complete only
when every criterion has actual evidence. An unavailable owner task or hardware
check remains named and unexecuted; simulated usage cannot fill that record.

## T00 — Baseline and reproductions

Packet/state: **T00 / complete** as a baseline/disposition packet, 2026-10-03.
Production terminal behavior remains incomplete; opt-in baseline assertions
deliberately fail until the owning later packets repair them.

Dependencies and owning contracts read: none; AGENTS, main and terminal plans,
P/O/terminal status, UX/architecture, scope/glossary, protocol, persistence,
operations and verification. Existing relocation/planning changes were preserved.
No TG1–TG5 gate is closed by this packet; T01 owns those design experiments.

Changed files/task ownership: `tests/terminal/{baseline_test.go,README.md}`,
`docs/evidence/terminal-t00-20261003/*`, this tracker and the UX finding-disposition
pointer. No production repair, schema migration or dependency change was made.
Full dirty-tree provenance and task-file hashes are in the
[manifest](../evidence/terminal-t00-20261003/manifest.json).

Affected invariants/scenarios: I08/I09/I11/I13/I16/I19–I24/I27–I28 identified;
scope/authentication, finite startup, persistent joining/readiness, live/stopped
adapters, status provenance and missing commands exercised. These are observed
gaps, not new causal-counter, retirement, power-loss or no-loss proofs.

Discovered tests: ten `TestTerminalT00*` top-level cases, thirteen leaf cases.
`go test ./... -list '^TestTerminalT00'` found nonzero matches. Final opt-in
`ORBIT_TERMINAL_BASELINE=1 go test -count=2 -v ./tests/terminal -run
'^TestTerminalT00'` exited 1 with the same thirteen deliberate leaf failures
twice. The opt-in `-race -count=1` run exited 1 with those failures and no race
report. Ordinary `go test -count=1 -v ./tests/terminal` exited 0 with all ten
opt-in tests skipped; skips do not establish repaired behavior.

Actual broad checks: `make check` exited 0. The initial overlapping
`make test-race` exited 2 with tar/checksum packaging discrepancies; both gates
write `dist`. After both finished, **serial `make test-race` exited 0**. The
contaminated initial log is retained. `go vet ./tests/terminal`, explicit
`go test ./cmd/filesync/...` and `git diff --check` exited 0. Full commands,
fixtures, draft corrections and cleanup are in
[commands](../evidence/terminal-t00-20261003/commands.md), and dispositions in
[summary](../evidence/terminal-t00-20261003/summary.md).

Observed refinements: existing-root pending join resumes as completed while
the inviter remains pending. Join readiness ignores actual unsupported-object
scan diagnostics; fatal scan-error suppression remains source-only. The wrong
folder request is pending; this experiment does not claim unauthorized file
access. Repeated-key request collision also consumes the second invitation.
Whole-file merge allocation and missing editor sessions remain source-only;
measured RSS/editor execution is unexecuted. Current fresh schema is 13.

Prerequisites/limitations: read-only existing SSH aliases laptop/rpi/vps work
(x86_64/aarch64/aarch64). Local Tailscale is unavailable in the tested PATH;
remote status commands exit 127. LAN/Tailscale application reachability, native
terminal packages, boot/logout, PTY, fatal scan faults, root-swap execution and
VM-reset scenarios remain unexecuted. Personal roots/services and firewall/VPN
policy were not changed. P17 actual owner use and unaided explanation remain
outstanding; no owner learning exercise was claimed.

Worker explanation: listener/routing is a transport problem; authenticating the
inviter before capability disclosure is an identity problem; checking token
folder and distinguishing attempts are scope/persistence problems. Selecting
live authenticated operations instead of an exclusive database adapter is a
control ownership problem. Rendering alone cannot fix these engine boundaries.

Next eligible work: **T01 contracts and TG1–TG5 design experiments**. Use the
test/report map to assign later repairs and promote each opt-in case into a
passing ordinary regression. Do not weaken assertions or treat historical O
completion as evidence for the terminal journey. No per-packet approval is needed.

## T01 — Contracts and design gates

Packet/state: **T01 / complete**, 2026-10-03, for the additive contract and
bounded design-experiment acceptance bar. No production terminal endpoint or
schema migration was added; gate production proof remains open.

Dependencies/owning contracts read: T00 complete; AGENTS, main/terminal plans,
P/O/terminal status, UX/architecture, scope/glossary, protocol, persistence,
operations, verification, T00 evidence and current codec/control/state sources.

Gate outcomes: TG1 isolated enrollment TLS listener, pinned inviter and canonical
scope/attempt request and status possession proofs; TG2 reviewed root generation,
restartable phases and observed readiness; TG3 shared ownership/authentication,
operation replay/cancel and finite initialization; TG4 exact pinned streams,
editor/staging lifecycle, stale review and copy/restore provenance; TG5 terminal/
legacy/service lifecycle. [Decisions](../terminal-design-gates.md) name each
experiment, limitation and responsible production packet. TG1 T03/T05, TG2 T04,
TG3 T02/T06, TG4 T08/T11, TG5 T09/T12 production acceptance remains open.

Changed files/task ownership: additive `internal/control/terminalcontract/*`;
`internal/protocol/terminal_enrollment{,_wire,_test}.go`; typed schema and eleven
`schemas/fixtures/terminal-v1/*.json`; T01 gate tests and canonical golden;
terminal architecture, protocol/persistence/operations/verification additions;
[source manifest](terminal-source-ownership.md); gate decisions, evidence and
this tracker. Exact assignments/hashes are recorded in the manifest. No existing
runtime function, dependency, T00 assertion or relocation source was changed.

Invariants/scenarios: I09/I13/I16/I19–I28 at contract/model seams only. Tests
cover strict large integers/duplicates/unknowns, all intent families and upload,
review fingerprints, context/query bounds and script exits; TLS pin-before-body
and isolated peer admission; scope/forgery/expiry/concurrent capability use and
retry; serialized join phases/readiness; real existing lock exclusivity and
late-response decision model; replay/partial cancel; reference-set session versus
stream pins, stale heads, restore provenance and bounded stream; terminal/service
state transitions. No new causal, fsync, production auth/GC or keyboard proof.

Discovered tests: **17 top-level TestTerminalT01 tests** (8 contract, 2 protocol,
7 gates). `go test ./... -list '^TestTerminalT01'` discovered all; `go test
-count=2 -v ./... -run '^TestTerminalT01'` passed 34 executions. Targeted `-race`
run passed all 17, no race report. Final serial `make check` and `make test-race`
both exited 0 after Go changes, including CLI tests in the latter. The preserved
Python doc validator passed 11 documents/74 local links/anchors; final counts
are in its transcript. `git diff --check` passed. A draft documentation run
failed only because the results file had not yet been written; it passed after
the evidence record was created. Exact commands/results are in
[commands](../evidence/terminal-t01-20261003/commands.md).

Provenance/evidence: revision `86ae55280b22a5839258d3cca40210c6e2613025` plus
preserved dirty tree, Go 1.27.1, Linux amd64/ext4, unchanged schema 13/protocol 1.
[Summary](../evidence/terminal-t01-20261003/summary.md),
[manifest](../evidence/terminal-t01-20261003/manifest.json),
[results](../evidence/terminal-t01-20261003/results.json) and sanitized transcripts.
T00's deliberate baseline failures remain opt-in; ordinary skips prove no repair.

Limitations/unexecuted: actual terminal adapters/listeners and requester status/
approval, durable join fsync/SIGKILL/root traversal/swap, real editor/GC races,
PTY/lifetime, LAN/Tailscale enrollment, native login/logout/package adoption and
rollback. Models and fixtures are not integration proof. No personal folders,
existing services or host network/privilege policies were changed. P17 actual
owner use and unaided explanation remain outstanding.

Worker learning: durable identity/fingerprint survives the lost-response window
between engine commit and screen refresh; generations prevent a restart/retry
from approving newly changed roots/heads. Interface cancellation changes waiting,
not committed work. This explanation does not substitute for owner evidence.

Next eligible work: **T02 — shared client and daemon lifecycle**. Implement the
frozen seam, finite initializer, selected daemon singleton/recovery and network
settings, including durable replay for lifecycle/settings operations. T04 extends
that ledger to setup jobs; T02/T03 must satisfy their own mutation replay criteria.
Use the T00 regression map; keep its failing cases until their production repairs.


## T02 — Shared client and daemon lifecycle

Packet/state: **T02 / complete**, 2026-10-03, scoped to its client, finite
initializer and local process/service lifecycle acceptance. Native boot/logout,
enrollment serving and later operation families are not claimed.

Dependencies/owning contracts read: T01 complete; implementation/terminal plans,
P/O/terminal status, UX/architecture, scope/glossary, operations authentication/
lifecycle/limits, persistence ownership/recovery, protocol and verification,
TG3/TG5 and the frozen terminal schema/source ownership. Existing relocation
and planning work was preserved. TG3 now has production evidence for the shared
live/stopped lifecycle/settings subset; T06/T08 retain full-family parity/streams.
TG1 enrollment and TG5 terminal/native entry remain open.

Changed files/task ownership: new `internal/controlclient/client.go`,
`internal/launcher/daemon.go`, `internal/config/runtime.go`,
`internal/state/private.go`, `internal/control/terminal_lifecycle.go`,
`internal/repository/terminal_operations.go`, CLI lifecycle adapter/tests and
`tests/terminal/lifecycle_test.go`; app/config/control/launcher/CLI wiring,
service safety and five existing test-fixture files. Owning operations,
persistence/schema/gate docs, evidence and this tracker were updated.
No dependency, schema version, causal/wire identity or relocation behavior
change was introduced. [Manifest](../evidence/terminal-t02-20261003/manifest.json)
records exact task files/hashes and the preserved dirty-tree provenance.

Invariants/scenarios: I08/I13/I19–I22/I28 at local lifecycle/control seams.
Owner-only credentials and strict loopback selection; bounded/authenticated
calls; no HTTP-error fallback; stable identity/finite budgets; missing-limit
review; eight concurrent clients selecting one process; client cancellation/
exit leaves daemon alive; persisted peer listener across restart; stale PID
refusal; reviewed settings/service replay, changed fingerprint/stale review
rejection and retained expired guards; accepted settings survive canceled waiting;
lost response recovery; one durable external dispatch across service retries;
real process start/stop/restart using disposable service
command stand-in; distinct running/enabled/unattended and explicit service errors.
No new no-loss, membership, enrollment or native unattended guarantee is claimed.

Discovered tests: **16 TestTerminalT02 top-level definitions**, including one
subprocess entry helper skipped in parent runs. Fifteen ordinary tests passed
**twice (30 executions)** across CLI and terminal packages; subprocess children
execute the helper. The no-fallback case also has four leaf subtests.
The focused resumed race suite and final full `make test-race` exited 0.
Final serial `make check` exited 0, including integration/model/fault/harness
checks and amd64/arm64 package builds. `git diff --check` exited 0; final
Markdown local-link validation is recorded in the evidence results.

Failures retained: first compile iterations failed on an ID method and unused
imports. Stricter credential validation exposed public-mode fixtures; initial
`make check` exited 2 on those fixtures and a real-user service environment
fixture. Those fixtures now use private/disposable inputs. A targeted race run
failed in the child; a minimal concurrent real Serve/Shutdown reproduction
reported a race on the HTTP server pointer. Initialization before publication
fixed it; both minimal and original process race regressions pass. An overlapping startup test also exposed expected not-ready connection errors
during lock handoff; callers are drained and replayed with their original ID,
and exactly one external dispatch is verified. Final cleanup inspection repaired the existing pairing fixture's ignored Orbit
alias stop failure; seven exact known leaked disposable test daemons were
stopped gracefully. Its marker-validated owning stop cleanup passed a
focused race regression after the broad runtime gates. No runtime source changed
following those broad gates. One focused
race log interrupted by session steering remains incomplete and is not a pass.
See [commands](../evidence/terminal-t02-20261003/commands.md) for final versus
intermediate checks and [summary](../evidence/terminal-t02-20261003/summary.md).

Provenance: revision `86ae55280b22a5839258d3cca40210c6e2613025` plus preserved
dirty tree; Go 1.27.1-X:nodwarf5, Linux 7.2.8-arch1-2 amd64; schema 13 and
peer protocol 1. Terminal ledger uses the existing metadata namespace.
Actual results/transcripts and limitations are in the evidence directory.

Limitations/unexecuted: native boot/logout/login/Pi/VPS, Tailscale/LAN enrollment,
SIGKILL and power-loss boundaries, TUI/PTY, full operation-family parity,
exact streams/upload, explicit operation cancellation and native legacy
adoption/rollback. Enrollment settings are stored, but T03 supplies the listener.
Runtime changes report restart required. Terminal capture/unattended health is
conservative until later evidence; old capture success means historical capture.
Service experiments use actual daemon processes with a disposable `systemctl`
stand-in, not a native user manager. Older binaries require reconciled ledger
and deliberate budget migration before rollback. Personal folders/services and
host privilege/firewall/VPN policies were not changed. **P17 actual owner use
and unaided explanation remain outstanding.**

Worker explanation: enabled is a startup configuration fact; running is selected
state ownership now. Neither establishes healthy roots/capture or persistence
after logout. A failed live HTTP call does not prove ownership was released;
opening SQLite afterwards would violate the singleton boundary. Replay the same
durable identity to inspect a lost response, keeping committed effects intact.
This is worker explanation, not owner evidence.

Next eligible packet: **T03 — authenticated network enrollment**. Reuse the
shared typed client/initializer and persisted runtime addresses; implement the
isolated enrollment listener and capability/identity/scope controls. T04 extends
the ledger to durable setup work. Preserve T00/T01 evidence and P17 obligations.


## T03 — Authenticated network enrollment

Packet/state: **T03 / complete**, 2026-10-04, for TG1 transport/admission/
reviewed approval. Physical cross-host LAN/Tailscale and ordinary T04 joining
readiness are not claimed.

Dependencies/owning contracts read: T01/T02 complete; AGENTS, implementation/
terminal plans and status, UX/architecture, scope/glossary, protocol membership/
data authorization, persistence, operations isolation/limits, verification,
TG1 and frozen terminal/enrollment codecs. Relocation and unrelated dirty-tree
work were preserved. No agents, dependency change or schema increment were used.

Changed source paths: isolated `internal/replication/enrollment.go` and limiter
checks; private repository enrollment transaction records in `enrollment.go`,
shared `peers.go` membership transaction, scoped legacy consumption in
`product_records.go`; controller `terminal_enrollment.go` and
`enrollment_compatibility.go`, terminal query/mutation, legacy API/type integration;
app dual-listener lifecycle; protocol canonical membership decoder/tests;
CLI private invitation/review adapters and production terminal/pairing/enrollment
tests. Owning contracts, source ownership, schema subset and evidence were updated.
Exact task hashes/dirty-tree provenance are in the evidence manifest.

Gate/invariants/scenarios: I09/I13/I15/I20/I23–I24 at real transport/control/
persistence seams. Pinned inviter before capability disclosure; frozen Ed25519
scope/attempt/nonce/key/endpoint transcript; atomic single use and identical
replay; changed signed fields/certificate encoding conflict; exact reviewed key/
folder/prior membership, expiry/revocation and current local inviter rechecks;
atomic existing linear membership plus approval/replay result; fresh signed status,
forgery/arbitrary-ID/replayed nonce refusal; isolated control/data routes and
certificate-required peer admission; payload, global/IP/concurrency admission
and combined outstanding nonce/pending caps; bounded IP accounting, SQL pending
counts and paged queries. Existing model/fork/retirement regressions passed;
T05's multi-folder/offline production scenarios remain pending.

Discovered tests: **16 TestTerminalT03 definitions**, including one parent-skipped
subprocess entry helper. **15 ordinary tests passed twice (30 executions)**.
The receiving daemon child actually signs/sends its request/status, membership
is imported under receiving owner control and exact-revision peer hello succeeds.
A graceful real owner restart preserves invitation/approval replay and signed
artifacts. The frozen P01 membership golden and malformed bytes are checked.
Seeded cap records plus actual HTTP refusal do not claim 128 device enrollments.
Five migrated pairing tests passed; the private-file running-daemon CLI journey
passed twice with original file/membership/endpoint/alias assertions retained.

Final actual checks: serial **make check** and **make test-race** both exited 0
on the final runtime code. Packet tests passed twice after the final SQL and
wire-replay tightening. The focused race suite passed before that tightening;
full final race validation covers it. CLI tests, model/fault/harness checks and
amd64/arm64 packages are included in those broad gates. `git diff --check` and
local Markdown validation results are in the evidence results. Initial broad
passes are retained separately from final passes.

Failures retained: compile/type/pagination draft mistakes; process readiness
could incorrectly use a stopped query during startup; independently released
ports could select the same port. The helper now waits for live authentication
and reserves ports together; ten process repetitions passed, followed by two
final requester/restart runs. Legacy fixtures needed credentials, deliberate
network opt-in with actual pins, v2 invitations and exact review input. A CLI
fixture's public temporary parent correctly failed private-input validation;
its marked root is now 0700. Intermediate failures are preserved in transcripts
and are not counted as passes. No broad gates overlapped their packaging writes.

Provenance/evidence: revision `86ae55280b22a5839258d3cca40210c6e2613025` plus
preserved dirty tree; Go 1.27.1-X:nodwarf5, Linux 7.2.8-arch1-2 amd64;
unchanged schema 13/peer protocol 1; additive enrollment v2 transport.
[Summary](../evidence/terminal-t03-20261004/summary.md),
[commands](../evidence/terminal-t03-20261004/commands.md),
[results](../evidence/terminal-t03-20261004/results.json) and manifest.

Limitations/unexecuted: physical two-host LAN/Tailscale, native laptop/Pi/VPS,
boot/logout/login, SIGKILL/power loss, TUI/PTY, full operation-family parity,
T04 root generations/durable joining/scan and content readiness, T05 additional
folders/offline/third-peer rollout/runtime endpoint adoption. Legacy network join
requires a transferred v2 certificate-bound invitation; anonymous old owner-control
enrollment routes and bare TLS bypass are removed. Compatibility join preparation
is private and status is pinned, but its earlier scan/resume readiness claims
remain T04 work. No personal services/roots or host policies were changed.
**P17 actual owner use and unaided explanation remain outstanding.**

Worker explanation: invitation possession grants a bounded request; TLS pinning
identifies the inviter before disclosure; a signed transcript proves the existing
requester key and exact scope. Owner review authors folder authorization. A
status possession proof only protects artifact retrieval. Neither a token,
label, verification code nor a signature alone grants file access. This is worker
explanation, not owner evidence.

Next eligible packet: **T04 — reviewed setup and resumable joining**. Reuse the
isolated client and exact persisted signed request. Replace compatibility setup
phases with root-preview generations and durable join jobs; preserve live owner
selection, exact approval replay and bootstrap bytes, and prove honest capture/
content/publication readiness through actual two-device CLI transfer.

## T04 implementation record — 2026-10-04

Implemented reviewed create/adopt/join through shared live control and durable
private jobs. Recursive descriptor-rooted preview bounds entries, hash bytes,
time, depth and issue pages; measured counts and persisted seek/hash continuations
survive reopening. Review binds root identity, tree content/stat generation,
settings and operation family, expires and is consumed atomically with admission.
Overlaps, unsupported/unreadable paths, changed roots and insufficient conservative
capacity block admission. Existing bytes are captured before remote file history
or publication; bootstrap absence authors no tombstone. Independent approval,
membership, capture, content, publication and conflict observations qualify Ready.

Exact operation/fingerprint, signed request, inviter certificate/endpoint and
attempt persist across client exit, delayed approval and daemon restart. Expired
preparation checks prior acceptance by requester-possession status; it recovers an
accepted request or requires an explicit new attempt without silently renewing it.
The CLI retains correctable names/settings through a reviewed form, supports
private scripted request files and uses the same typed operations. Approved
requester endpoints refresh ordinary scheduler pulls without requiring restart.
Login/unattended setup uses a durable child service operation; missing systemd
blocks explicitly, with no privileged linger changes.

Production seams: workspace adoption/capture, repository capacity/readiness/job
transactions, control setup/enrollment/lifecycle adapters, app worker/peer targets,
scheduler bootstrap gating, CLI setup adapter and typed contract. No dependency,
schema 13, peer protocol 1 or enrollment protocol 2 change. Existing relocation
and P/O evidence were preserved; historical status entries remain historical.

**19 ordinary TestTerminalT04 tests passed twice (38 executions)**, with no
skipped helper. Tests cover empty/nonempty roots, stale content/stat/root/plan,
unsupported/unreadable/overlapping roots, bounded preview continuation and issue
pages, capacity/expiry, consumed reviews, initial scan/capture failure, no bootstrap
tombstones, divergent histories, expired accepted/unsent requests and persisted
phase recovery. An actual two-installation CLI scenario kills only its marked
acknowledged pending receiver, restarts the same operation/request/key, preserves
local bytes and verifies ordinary edits in both directions plus exact heads and
managed-content digests after stopping both owners. A real Linux CLI PTY corrects
an invalid finite input, edits the review, retains names, preserves existing bytes
and proves client exit leaves the daemon running. Other phase faults use production
hooks and close/reopen, not SIGKILL at every phase.

Final serial **make check** and **make test-race** exited 0 on the final runtime
source, including amd64/arm64 packaging and relevant setup/workspace regressions.
`git diff --check` and local Markdown validation passed.
Final gate dispositions are in [results](../evidence/terminal-t04-20261004/results.json).
[Evidence summary](../evidence/terminal-t04-20261004/summary.md),
[commands](../evidence/terminal-t04-20261004/commands.md) and source/environment
manifest retain intermediate failures, fixes and dirty-tree provenance. Draft
failures exposed polling throttling, mutable caller input, journal phase spelling,
reverse endpoint persistence, compatibility review fixtures and an arm64 type
conversion. These were corrected before final validation. Two known disposable
draft CLI children were gracefully stopped only after executable/argv/open-lock
validation; no personal service or root was touched.

Limitations/unexecuted: physical cross-host LAN/Tailscale, native laptop/Pi/VPS,
boot/logout/login/unattended guarantees, abrupt reset/power loss, SIGKILL at every
boundary, TUI rendering/keyboard acceptance, additional-folder/offline/third-peer
rollout and full operation-family parity. Completed readiness is historical;
current qualified status remains T07. Capacity observation is conservative
admission, not a reservation against unrelated writers. **P17 actual owner use
and unaided explanation remain outstanding.**

Worker explanation: enrollment admits an identity into membership; local capture
saves working bytes; verified content/publication/conflicts independently qualify
Ready. Restart retains the authenticated attempt and reviewed root; membership
alone transfers no files. This explanation is not owner evidence.

Next eligible packet: **T05 — additional-folder sharing and rollout**.


## T05 — Additional-folder sharing and rollout

Packet/state: **T05 / complete**, 2026-10-04, for scoped additional sharing,
sequential additive membership and local nonloopback offline/third-peer rollout.

Dependencies/owning contracts read: T03/T04 complete; implementation/terminal
plans and P/O/terminal status, UX/architecture, scope/glossary, protocol membership/
retirement/status and TG1/TG2, persistence private requests/replay/root review,
operations endpoints/authentication, verification and frozen control/peer schemas.
Existing relocation changes and historical P/O evidence remain preserved.

Changed source ownership: terminal enrollment/lifecycle and endpoint controller;
private enrollment transaction pin lookup; enrollment target binding;
peer membership handler/wire and reconciliation; durable retry classification;
CLI share/endpoint adapters; six ordinary sharing/process tests and endpoint
integration regression. Owning specifications, schemas, conservative recovery
runbook and source-ownership manifest were updated. No new dependency/schema,
identity/counter rewrite, pruning or agent delegation. Actual hashes and dirty-tree
provenance are in the [manifest](../evidence/terminal-t05-20261004/manifest.json).

Invariants/scenarios: I09, I14–I16, I19, I23–I24, I28 at authenticated live control,
real enrollment/peer TLS, SQLite and workspace/daemon seams. Same persistent key
joins a separately approved second folder with a reviewed existing root; retained
first attempts never collide. Wrong target ID/key and unshared data are rejected.
Exact predecessor/two-step chains install sequentially; stale approvals/forks
block data. Offline members remain enrolled through restart. With C offline, A's
captured file reaches B; after A stops, reopened C receives that original-author
version through B. Both B/C pull directions work with A offline. Live CLI address
refresh restores transfer with the same identity/membership and saved certificate.
Final stopped inspection verifies actual heads/content hashes and forwarded author.
Fork recovery preserves old paused heads/bytes and reviews a separate new root.

Discovered tests: **six ordinary TestTerminalT05 top-level cases**, no packet
skips; final `go test ./tests/terminal -run '^TestTerminalT05' -count=2 -v` exited
0, all six passed twice (197.981s). Final serial **make check** and
**make test-race** exited 0. Additional uncached
`GOFLAGS=-race go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT05'`
exited 0 (119.791s), instrumenting the built CLI and daemon children too.
Revocation/expiry/replay/admission regressions and independent membership/
retirement/revival checks passed; scoped vet, local Markdown validation and
`git diff --check` passed. [Commands](../evidence/terminal-t05-20261004/commands.md),
[results](../evidence/terminal-t05-20261004/results.json) and
[summary](../evidence/terminal-t05-20261004/summary.md) retain exact records.

Intermediate failures remain recorded: draft test method/type/field mistakes;
second enrollment hit the real per-IP quota; initial make check found the old
empty-certificate integration expectation. The corrected fixture reopens the
inviter between journeys, exercises durable records and resets its process-local
bucket. The endpoint regression now requires a certificate for an unknown pair
and verifies exact saved-anchor reuse for address-only refresh. No failure is
counted as a pass and no limiter is disabled.

Limitations/unexecuted: physical cross-host LAN/Tailscale, native boot/logout/
login/unattended and physical reset remain T12/T13. The five-request/minute/IP
admission bound remains: an unsent expired signed attempt needs explicit new
review, never silent renewal. Automatic rollout needs T05-capable peers and
admits only additive enrollment; retirement remains explicit survivor maintenance.
Fork recovery creates a new group, preserving old DAGs rather than merging them.
Named commands/full adapter parity, qualified persistent attention and actual TUI
sharing remain T06/T07/T10. **P17 actual owner use and unaided explanation remain
outstanding.** The worker explanation in the evidence does not fill owner evidence.

Next eligible packet: **T06 — commands and context**. Reuse the typed share/
setup/approval controls and runtime endpoint seam; implement ordinary named/contextual
CLI selection, explicit ambiguity, scripting/help/completions and live/stopped
operation-family equivalence. Preserve T00–T05/P/O evidence and relocation work.


## T06 — Commands and context

Packet/state: **T06 / complete**, 2026-10-04, for named commands, current-directory
and named context resolution, literal paths, output sanitization, shell completions,
and live/stopped operation-family adapter parity.

Dependencies/owning contracts read: T02, T04, T05 complete; implementation/terminal
plans, P/O/terminal status, UX/architecture, scope/glossary, protocol path rules,
persistence, operations, verification, and frozen control schemas. Existing relocation
changes and historical P/O evidence remain preserved.

Changed source ownership: modular terminal command adapters in `cmd/filesync/terminal_commands.go`;
name and context resolution in `internal/control/terminal_context.go` and
`cmd/filesync/terminal_context.go`; output escaping and rendering in
`cmd/filesync/terminal_output.go`; shell completions in `cmd/filesync/terminal_completion.go`;
help in `cmd/filesync/terminal_help.go`; command routing and live/stopped controller
dispatch in `cmd/filesync/main.go`; named browse queries in `internal/repository/browse.go`;
exit code mapping in `internal/control/terminalcontract/validate.go`; and integration
suite in `tests/terminal/commands_context_test.go`.

Invariants/scenarios: I09, I16, I19–I20, I27 at CLI invocation, controller, SQLite,
and filesystem boundary seams. Context infers active folder and relative path from
`cwd` inside registered roots and nested subdirectories. Relative target paths resolve
against `cwd`. Outside a registered root, explicit folder name or 64-character hex ID
resolves; duplicate display names or ambiguous targets return `AMBIGUOUS_CONTEXT`
(exit code 2) with candidate items. Unmatched folder names return `FOLDER_NOT_FOUND`
(exit code 2) with candidate folder items. Missing roots return `ROOT_UNAVAILABLE`
(exit code 5) and relocated/mismatched roots return `STALE_ROOT` (exit code 4).
Descriptor-rooted path safety blocks traversal escapes via `..` or reserved namespaces
(`.filesync`, `.orbit-*`) returning `INVALID_PATH` (exit code 2). Literal paths
starting with dashes or spaces are supported via `--`. Control characters and raw
ANSI escapes in names and paths are neutralized via `EscapeTerminal` in human output,
while structured machine output with `--json` preserves clean contract fields.
Command adapters route through `controlclient.Client.WithController`, maintaining
parity between live daemon and stopped states without SQLite database locks. Fast
shell completions for bash, zsh, and fish discover commands and folders without
disclosing secrets or tokens.

Discovered tests: **seven ordinary TestTerminalT06 top-level cases**, no skips;
final `go test -count=2 -v ./tests/terminal -run '^TestTerminalT06'` exited 0, all
seven passed twice (1.159s). Final serial **make check** and **make test-race**
exited 0. Packet race test `go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT06'`
exited 0 (21.252s). Control unit tests, `cmd/filesync` tests, scoped vet, local
Markdown validation, and `git diff --check` passed. [Commands](../evidence/terminal-t06-20261004/commands.md),
[results](../evidence/terminal-t06-20261004/results.json), and
[summary](../evidence/terminal-t06-20261004/summary.md) retain exact records.

Intermediate failures remain recorded: initial folder lookup omission of candidate
items for suggestion, help header launch test substring mismatch, and folder pause/resume
direct workspace database lock collisions when the daemon was active. All were corrected
and validated before final broad test gates.

Limitations/unexecuted: physical cross-host LAN/Tailscale, native boot/logout/login/unattended,
and physical reset remain T12/T13. Bounded reviewed editor sessions, streaming merge,
and deep conflict/restore semantics remain assigned to T08. Qualified persistent status
and diagnostics remain assigned to T07. **P17 actual owner use and unaided explanation
remain outstanding.**

Worker explanation: A convenient context lookup is a navigation helper, not an
authorization or safety check. Context maps `cwd` or a partial name to a candidate
folder descriptor and relative path. Authorization requires local authenticated
credentials and verified cryptographic membership in the folder's DAG. Path safety
requires descriptor-rooted containment, lexical validation, and traversal rejection
at the filesystem boundary. Finding a folder does not authorize access to its data;
resolving a path does not prove the target is safe to read or write.
This explanation is not owner evidence.

Next eligible packet: **T07 — status, attention, and diagnostics**. Reuse the shared
context and adapter seam; implement aggregate status queries, persistent attention
items, actionable doctor diagnostics, and live/stopped health inspection.

## T07 execution record — 2026-10-04

Implemented qualified copy status, persistent attention items across restart, actionable diagnostics (`orbit doctor`), and bounded pagination under the agreed UX and terminal architecture:
- Added `FolderReadiness(ctx, folder)` providing qualified, truthful readiness without hidden scans, GC, or membership mutations.
- Added `terminalAttention(ctx, q)` collecting actionable attention items (`CONFLICT`, `STRUCTURAL_CONFLICT`, `ROOT_UNAVAILABLE`, `STALE_ROOT`, `FOLDER_PAUSED`, `BLOCKED_PATH`, `EXHAUSTED_WORK`, `DISK_BUDGET`, `METADATA_BUDGET`, `AWAITING_APPROVAL`, `INCOMPLETE_SETUP`, `MEMBERSHIP_FORK`, `OFFLINE`) with deterministic sorting and bounded pagination (`limit`, `cursor`).
- Added `terminalStatus(ctx, q)` reporting named folders, aggregate readiness, service details, and device-copy observations (`saved`, `stored`, `applied`, `direct`, `online`, `availability`, `last_contact`).
- Resolved detail observation baseline gap by persisting and exposing `Direct bool` on `PeerProgressSummary` (`baseline_test.go: TestTerminalT00DetailObservation` passes).
- Enhanced `Doctor(ctx)` to evaluate daemon local-control, TLS certificates, state/db permissions, root availability/inodes, storage budgets/reserve, network reachability (peer freshness > 24h, Tailscale CLI reporting without mutating host), folder approval/revisions, membership forks, and service tooling.
- Maintained adapter and CLI parity via `controlclient.Client.WithController` across live running daemon and stopped adapter with `--json` and sanitized human output (`EscapeTerminal`).

Discovered tests: **five ordinary TestTerminalT07 top-level cases**, no skips;
final `go test -count=2 -v ./tests/terminal -run '^TestTerminalT07'` exited 0, all
five passed twice (0.935s). Final serial **make check** and **make test-race**
exited 0. Packet race test `go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT07'`
exited 0 (2.294s). Full repository `make test-race` passed across all packages (`tests/terminal` passed in 178.420s).
Control unit tests, `cmd/filesync` tests, scoped vet, local Markdown validation, and `git diff --check` passed.
[Commands](../evidence/terminal-t07-20261004/commands.md),
[results](../evidence/terminal-t07-20261004/results.json), and
[summary](../evidence/terminal-t07-20261004/summary.md) retain exact records.

Intermediate failures remain recorded: argument parser capturing state directory as positional folder when passed as `--state <dir>` (resolved via `parsePositionalFolder`), dual-store enrollment requests query gap (resolved by querying both `terminalRequests` and `db.ListEnrollmentRequests`), and doctor lifecycle check reporting WARN when daemon was inactive in fresh init (resolved by reporting OK for inactive daemon). All were corrected and verified.

Limitations/unexecuted: physical cross-host LAN/Tailscale, native boot/logout/login/unattended, and physical reset remain T12/T13. Bounded reviewed editor sessions, streaming merge, and deep conflict/restore semantics remain assigned to T08. **P17 actual owner use and unaided explanation remain outstanding.**

Worker explanation: An offline device's last stored receipt establishes only that *at the specific time the receipt was recorded*, that device durably committed the chunks and metadata to its local storage. It does not prove that the device is currently online, that the version was applied to its working directory (`Applied=false`), that the device hasn't edited or deleted it offline, or that other devices have received it. A receipt from a relaying intermediary or VPS forwarder confirms durable forwarding reception on that node alone; it cannot claim the final destination device received or applied the edit.
This explanation is not owner evidence.

Next eligible packet: **T08 — conflicts, history and restore** (and T09).



## T08 execution record — 2026-10-04

State: `complete`. Prerequisites T06/T07 checked; read TG4, terminal UX/architecture,
protocol reviewed ancestry, persistence read/publication/GC, finite operations and
verification. Historical P/O and unrelated/relocation changes are preserved.

Changed files: new `internal/control/terminal_{content,content_http,sessions}.go`,
`internal/controlclient/{content,tools}.go`, `internal/workspace/review.go`,
`cmd/filesync/terminal_content.go` and `tests/terminal/content_recovery_test.go`;
existing shared client/lifecycle/status routes, terminal contract validation,
repository browse/objects/resolution/GC/terminal records, controller merge limits,
CLI adapters/help/legacy explicit-input guards, owning specs/schema and
[recovery runbook](../runbooks/terminal-recovery.md).

Implemented authenticated bounded raw streams; exact reviewed select/keep-copies/
manual merge/restore/separate-copy; SQL conflict/history/Deleted pages; current-byte
review; atomic causal identity plus replay effects; privately admitted editor exports,
size/digest staging, direct argv/file-size limits, restart recovery attention,
renewal/expiry/cancel and fresh reviewed discard. Copies remain individually durable;
publication can stay pending. Restore provenance stays distinct from current parents.

Invariants exercised: I03–I07, I10–I13, I16, I18–I20, I25–I28 through exact heads/
parents/provenance, byte/hash and ownership oracles, quota admission, read/GC pins,
replay/partial effects, corruption refusal, path/collision guards and real CLI controls.
These are scoped interface checks, not new physical fault or universal safety claims.

Nine ordinary top-level `TestTerminalT08*` tests were discovered. Final
`go test -count=2 -v ./tests/terminal -run '^TestTerminalT08'` passed all nine twice
without skips (5.933s command elapsed). Final uncached packet race passed (15.195s).
Explicit CLI/control/client/workspace/repository/model tests passed (2.165s).
Final `make check` exited 0 (76.951s); full `make test-race` exited 0 (192.508s,
`tests/terminal` 190.639s, no race warnings). `git diff --check` passed.
[Commands](../evidence/terminal-t08-20261004/commands.md),
[results](../evidence/terminal-t08-20261004/results.json),
[summary](../evidence/terminal-t08-20261004/summary.md) and manifest retain actual
validation and dirty-tree provenance.

Intermediate failures are retained: unauthorized peer/CLI argument/private review
fixture issues; new-object pinning before installation causing a foreign-key error;
GC expectations that ignored protected response lifetime; and a repeated-run
stopped read-error/state-release race. Atomic install-and-pin, independent read pins,
private output handling, stable root reviews, and teardown-before-return corrected
those findings. Final replay regressions also ensure completed requests cannot
reapply old bytes after later capture and pending publication refuses newer or
competing heads. Source exports reopen sequentially, keeping one chunk buffer across
multiple reviewed versions. A real configured editor merges 6,000,000 bytes through
staging; Linux child peak RSS is logged with fork/exec high-water limitations.

Limitations/unexecuted: tools are trusted owner programs; result admission has a
largest-source/1 MiB-minimum limit and one upload per session. Unknown auxiliary
files block complete cleanup. Interactive PTY editor suspend/resume remains T11/T13;
physical LAN/Tailscale, native boot/logout/login/unattended and power-loss/reset
coverage remain T12/T13. **P17 actual owner use and unaided explanation remain
outstanding.** Worker explanations and synthetic process fixtures are not owner use.

Worker explanation: restored bytes come from the historical source, which is
provenance. The new restore parents are the reviewed current heads, retaining
current ancestry and allowing later arrivals to remain concurrent. File timestamps
may disagree, be reset or reflect delayed delivery; they cannot establish causal
dominance or select a safe conflict winner. This is not unaided owner evidence.

Next eligible packet: **T09 — TUI shell and terminal lifetime**. Pin/verify the
selected Charm v2 stack and build real PTY keyboard, resize, cancellation, tool
suspend/resume and daemon-independence evidence before T10/T11 screens.


## T09 execution record — 2026-10-04

State: `complete`. Prerequisites T01/T02/T06 checked; T08 controls retained. Read
terminal UX/architecture/plan, TG5, scope/glossary, operations, persistence,
protocol and verification, typed fixtures/shared-client contracts. Historical
P/O/T00–T08 evidence and unrelated/relocation changes remain preserved.

Implemented `orbit tui` as an explicit development entry with a single Charm
v2 input/render owner in `internal/terminal`. Stable pins are Bubble Tea
2.0.10, Bubbles 2.2.1, Lip Gloss 2.0.6, verified against official releases/APIs
and downloaded module licenses. New dependency license texts enter distributed
`NOTICE`; amd64/arm64 static builds and packaging passed.

Changed ownership: `internal/terminal/{library,app,keys,render,app_test}.go`;
`cmd/filesync/terminal_tui.go` and dispatch/help/completions; shared bounded
`controlclient.LimitedToolCommand`; repository SQL name pages/control routing;
`tests/terminal/tui_test.go`; `scripts/terminal_{pty_test,vt}.py` and independent
VT oracle tests; Makefile PTY target, dependencies/licenses and owning specs,
[shell runbook](../runbooks/terminal-shell.md), evidence and this tracker.
No schema/peer identity/counter change or agent delegation.

Invariants/scenarios: I13, I19–I21, I27 at typed control, actual CLI/daemon,
SQLite, terminal and scratch-file seams. Shell navigation covers j/k/arrows,
Tab/Enter/Esc/search/help; focused/pasted j/k/q/? remain text. One cancellable
query lane coalesces context changes and rejects late request/generation replies;
refresh preserves selected identity, focus and draft. Name pages use SQL keysets
and selector-bound cursors. Application pages cap each collection at 20 rows,
search at 256 characters; aggregate attention retains T07 computation.

Real marked-root PTYs cover 80x24, 40x16, resize/paste/Unicode, colorless output,
input/stdout pipes/JSON, owner direct argv and result-size adapter, canonical/raw
terminal handoff, successful actual scratch edits, failed result preservation,
active-tool SIGTERM and q/Ctrl-C/SIGTERM exit. Exact termios, alternate-screen
and paste-mode restoration pass. A paused marked daemon retains its lock; the
client reports unavailable live control and reconnects without direct-state
bypass. After every client exits, the same daemon captures a new actual file
digest; protected original bytes and device identity are checked.

Seven ordinary top-level `TestTerminalT09*` tests were discovered. Final
`go test -count=2 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT09'`
passed all seven twice without skips (21.942s). Packet race with `GOFLAGS=-race`
passed, instrumenting built CLI/daemon children too (35.527s). Three additional
complete PTY campaigns passed (32.022s); final recorded transcript run passed
(10.324s), as did `make test-terminal-pty` (10.308s). Explicit CLI/control/client/
repository regressions, `go mod verify`, formatting and `git diff --check` passed.
`make check` passed (117.793s), including static amd64/arm64 packaging and all
five Python safety/VT tests. Full `make test-race` passed (202.749s; terminal
suite 200.711s) without race warnings. [Commands](../evidence/terminal-t09-20261004/commands.md),
[results](../evidence/terminal-t09-20261004/results.json),
[summary](../evidence/terminal-t09-20261004/summary.md), sanitized transcripts
and source/environment manifest retain actual outcomes and dirty-tree provenance.

Intermediate failures remain recorded: narrow header obscured selected row;
fixture membership JSON key spelling/cwd/identity-lock mistakes; differential
renderer substring oracle; and repeated SIGTERM/tool-shutdown timeouts. The CLI
now exclusively owns signals, avoiding Bubble Tea's unbuffered quit send racing
context shutdown. PTY shutdown waits keep draining output; subsequent repeated
and instrumented campaigns pass. The independent VT cell oracle checks visible
labels while real byte/history/process assertions remain authoritative. A draft
paged folder branch was restricted to its exact query kind to preserve status.

Limitations/unexecuted: development shell queries/navigation are complete, while
T10 owns setup/join/device workflows and T11 owns full qualified copy detail,
conflict/history/restore and reviewed T08 editor-session integration. The optional
T09 tool acts on an explicit scratch file, never commits a merge. Tools remain
trusted owner programs; terminal-library event decoding precedes application
paste limits. Folder/device pages are live navigation, not review snapshots.
Native laptop/Pi/VPS, physical LAN/Tailscale, boot/login/logout/unattended, reset/
power loss, bare entry/package adoption remain T12/T13. **P17 actual owner use
and unaided explanation remain outstanding.**

Worker explanation: closing a client cancels its in-flight queries and trusted
active tool, then releases terminal ownership; durable daemon work has a separate
owner/lifetime. Canceling a committed operation requires its explicit control.
Every response carries a request ID and view generation; Update admits it only
for the current lane/context, so a former-folder reply cannot populate the new
selection. This explanation is not unaided owner evidence.

Next eligible packet: **T10 — TUI onboarding and device management**; T11 is
also eligible. Reuse the frozen shell/query/tool adapters and real T04/T05/T07
controls, keep reviewed state controller-owned, and add keyboard/PTY journeys.

## T10 — TUI onboarding and device management

Packet/state: **T10 / complete**, 2026-10-04, for the required local actual-control/
PTY acceptance bar. Development entry remains explicit; native release is separate.

Dependencies/owning contracts read: T04, T05, T07, T09 complete; main/terminal
plans/status, scope/glossary, UX/architecture, TG1/TG2/TG5, typed contracts and
protocol/persistence/operations/verification. Historical P/O/T evidence and the
existing dirty relocation/runtime work were preserved. No agent delegation.

Changed ownership: `internal/terminal/{setup,join,approval,share,setup_render,
setup_test}.go` plus shell app/keys/render; bounded `control/terminal_management`
and setup/lifecycle/status/contract integration; `controlclient/onboarding` and
CLI setup/TUI entry; repository operation pages; actual two-daemon PTY runner,
VT oracle/regressions, Makefile target, owning specs/runbook, evidence and tracker.
No dependency or schema migration. The shared setup adapter persists exact private
retry intent and listener-restart requirement; controller/engine modules retain
membership, capture, publication and recovery ownership.

Invariants/scenarios: I09, I11, I13, I19, I21–I24, I27 at keyboard, authenticated
control, SQLite, transport, identity and actual file seams. First-use Create/Join
and actual unfinished/blocked operations; bounded existing-root review and
Advanced finite/network/startup inputs; validation/back/edit and retained errors;
private/wrapped invitation paste, expired/wrong-pin refusal; exact request and
matching canonical transcript verification; delayed approval, client close/reopen
and real daemon restart; two-way verified files; second-folder separate consent
with unchanged keys and distinct attempt/request; local pause/resume, actual
relocation, unregister/retirement previews and unchanged working bytes/membership.
Fork tests reject a competing branch then follow the documented conservative pause;
T05 regressions prove sequential membership catch-up/stale-revision data refusal.
Current readiness refresh does not promote a stored completion or offline receipt.
Startup failure uses actual control under an empty fixture PATH, preserving host
services. Every actual PTY restores termios and terminal modes; daemon capture and
transfer continue after all clients exit.

Nine ordinary `TestTerminalT10*` cases were discovered (five view/adapter, four
production/process). Final `go test -count=2 -v ./internal/terminal ./tests/terminal
-run '^TestTerminalT10'` passed all nine twice without skips (127.750s command;
process suite 127.308s). `GOFLAGS=-race go test -race -count=1 -v` for the same
packages/group passed, instrumenting built children (74.940s). An additional final
instrumented actual-PTY confirmation passed (72.020s). Sanitized standalone PTY
artifacts passed (63.040s). Relevant T04/T05/T07/T09 regressions passed (42.413s),
explicit CLI tests passed, five independent VT tests passed, and Python compilation
and diff checks passed. `make check` passed (89.000s), including static amd64/arm64
builds and packaging. **Full `make test-race` passed (284.660s; terminal suite
281.501s)** without race warnings. Broad package-producing gates ran serially.
[Commands](../evidence/terminal-t10-20261004/commands.md),
[results](../evidence/terminal-t10-20261004/results.json),
[summary](../evidence/terminal-t10-20261004/summary.md), sanitized frames and source/
environment manifest retain exact evidence and dirty-tree provenance.

Intermediate outcomes retained: new detail title omitted Inspect; narrow identity
rows obscured readiness; the VT oracle lacked scrolling-margin/autowrap semantics;
harness selection assumed a reset after adding a folder; an instrumented key arrived
before folder detail loaded. Labels/layout and independent VT semantics were fixed;
fixture waits now observe actual loaded control state and honor retained selection.
A draft fork assertion was corrected to the documented rejection/pause procedure.
Verification uses the owning transcript/status and existing Result request fields,
not a fabricated request-ID abbreviation. Final repeated, instrumented and full
checks pass. An unavailable `/usr/bin/time` wrapper ran no tests; recorded shell
builtin timing replaced it.

Limitations/unexecuted: T11 owns full everyday status/content/editor/recovery
screens. Unregister/retire remain conservative previews linked to existing scoped
procedures. Native laptop/Pi/VPS, physical LAN/Tailscale, boot/login/logout/
unattended, packaged bare entry/adoption, VM reset/power loss remain T12/T13.
**P17 actual owner use and unaided explanation remain outstanding.** No personal
root, existing VPS service or network/lingering policy was changed.

Worker explanation: one private paste binds inviter identity, endpoint and folder
capability; the receiver's reviewed root and persistent key create a durable exact
request. One deliberate matching-code approval grants only that participation.
Waiting, membership, capture and download/publication remain separate authoritative
phases; local readiness cannot establish another offline device's current copy.
This explanation is not unaided owner evidence.

Next eligible packet: **T11 — TUI everyday management**. Reuse T09/T10's serialized
lane and keyboard/PTY adapters and the real T08 reviewed sessions/streams. Preserve
native/cross-host and P17 owner requirements for the later release campaign.

## T11 — TUI everyday management

Packet/state: **T11 / complete**, 2026-10-04, for the required local actual-control/
PTY acceptance bar. Development entry remains explicit; native release is separate.

Dependencies/owning contracts read: T07, T08, T09, T10 complete; main/terminal
plans/status, scope/glossary, UX/architecture, TG3/TG4/TG5, typed contracts and
protocol/persistence/operations/verification. Historical P/O/T evidence and the
existing runtime work were preserved. No agent delegation.

Changed ownership: `internal/terminal/{everyday,everyday_render,everyday_test}.go`,
`internal/control/terminal_everyday.go`, `internal/controlclient/session_result.go`,
`scripts/terminal_everyday_pty_test.py`, `tests/terminal/everyday_test.go`, updates to
`internal/terminal/{app,keys,render,setup,setup_render,share}.go`, `internal/app/app.go`,
`internal/control/{terminal_content,terminal_lifecycle,terminal_status}.go`,
`internal/control/terminalcontract/{types,validate}.go`, `internal/repository/work.go`,
`internal/scheduler/{retry,scheduler,watcher}.go`, and Makefile targets.
No dependency or schema migration.

Invariants/scenarios: I03–I07, I11, I13–I14, I16, I18–I20, I25–I28 at keyboard,
authenticated control, SQLite, transport, identity and actual file seams.
The seven core everyday management scenarios were verified under real two-daemon PTY execution:
1. Real offline/reconnect concurrent heads
2. Canonical diff and external editor invocation with non-zero exit code recovery
3. Stale editor session refusal with fresh review renewal
4. Exact select and keep copies conflict resolution
5. Delete history restore with separate copy creation
6. Storage preview with narrow root unavailable protection
7. Terminal restoration and continued daemon sync after client quit

All seven `TestTerminalT11*` cases passed twice without skips. `GOFLAGS=-race` packet
tests passed cleanly, instrumenting built child daemons and CLI invocations.
Sanitized standalone PTY transcripts were verified and archived. Relevant T08/T09/T10
regressions passed, explicit CLI tests passed, five independent VT tests passed,
`make check` passed with static builds, and **full `make test-race` passed** across the
entire repository.

Intermediate outcomes retained: scheduler watch revalidation bug with ROOT_UNAVAILABLE
was fixed to revalidate before clearing in-memory paused state.

Limitations/unexecuted: T12 owns packages and adoption; native laptop/Pi/VPS, physical
LAN/Tailscale, boot/login/logout/unattended validation, and VM reset/power loss remain T12/T13.
**P17 actual owner use and unaided explanation remain outstanding.**

Worker explanation: everyday status displays authoritative engine observations rather
than synthetic progress bars. Conflicts present concurrent heads frozen in time until
explicit resolution. Staging editor outputs validates content hashes and streams bytes
to quarantine before atomic commit confirmation. Temporary unavailable roots pause
activity without mass deletions and automatically resume when the root directory returns.

Next eligible packet: **T12 — Terminal packages/entry, compatible adoption and operator runbooks**.


## T12 — Terminal packages/entry, compatible adoption and operator runbooks

Packet/state: **T12 / complete**, 2026-10-04, for its local entry/package/adoption
acceptance bar. Native and owner release acceptance remains T13.
Prerequisites T10/T11 complete; read terminal UX/architecture/TG5/release packet,
main status, scope/glossary, operations, persistence/recovery/relocation and
verification. Historical P/O/T evidence and initial dirty runtime work preserved.
No agent delegation, dependency change, causal/wire/schema/identity rewrite.

Changed ownership: CLI main/TUI/help/completion, doctor startup advice, Makefile,
package generator/desktop/install/uninstall, bare PTY adapter, new package runner,
four T12 tests, README and operator/install/network/recovery/rollback/upgrade/
uninstall runbooks. Shared controllers remain owners of review/admission/replay;
repository/workspace retain counters, migrations, recovery and relocation.

Implemented bare TTY TUI/pipe/JSON entry, correct legacy discovery, explicit frozen
legacy-browser/launch compatibility, terminal desktop entry, one service with
aliases, all-format completions/runbooks/notices/checksums, custom-unit-preserving
standalone upgrade and scoped uninstall. Real package-manager failures closed
missing Debian directories and RPM tag/order/region/digest/ownership defects.
Normal checks now include CLI tests, terminal journeys and extracted-package PTY;
container transactions/QEMU remain an explicit optional target.

Four ordinary T12 tests passed twice; instrumented packet race passed with built
CLI/daemon children included. Compatibility migrations/newer refusal/interrupted
recovery/rekey/relocation and peer auth/protocol mismatch checks passed. Native
amd64 and QEMU 7.2 arm64 extracted tar/deb/rpm CLI execution passed. Standalone
repeat installation/removal retained custom unit, identity/state and files.
Actual Debian bookworm/Fedora 43 transactions with normal reviewed setup and
manual daemon startup/stop retained captured history after reinstallation and
files/config/database after removal. Bare actual PTY lifetime/capture passed.
`make demo` passed. Final `make check` passed (356.253s) and full
`make test-race` passed (320.850s, no race warnings). Final `make package`
passed, followed by all 11 final native/emulated/distro/PTY package scenarios
(16.786s); checksums and sanitized PTY frames are archived. Four T12 tests
passed twice (22.700s) and instrumented packet race passed (42.155s). Explicit
retained browser compatibility, CLI, formatting and diff checks passed. Exact
source/commands are in
[T12 evidence](../evidence/terminal-t12-20261004/summary.md).

Invariants/scenarios: I07–I08, I19–I22, I27 at real CLI/PTY/SQLite/state lock,
capability/peer authorization, migration/recovery and package-manager boundaries.
Failure findings and corrected test fixture assumptions are retained in evidence.
A temporary test version-type compile error stopped the initial broad gate; final
stable broad validation is recorded separately. No failing oracle was weakened.

Limitations/unexecuted: native laptop/Pi/VPS, physical LAN/Tailscale, systemd
login/logout/boot/unattended persistence, clean-checkout/native terminal release,
VM reset/power loss and actual personal use remain T13. QEMU is emulation;
containers do not establish native systemd behavior. Reinstall tests do not prove
arbitrary older terminal binaries understand current operations/runtime settings.
**P17 actual owner use and unaided explanation remain outstanding.**

Worker explanation: binary rollback keeps the current database/counters and
requires schema plus runtime/operation compatibility. Restoring old metadata
loses knowledge of already authored counters, so recovery creates a fresh key/
device identity and reenrollment rather than reusing the old author. This is
worker explanation, not unaided owner evidence.

Next eligible packet: **T13 — integrated native/failure/owner release campaign**.
Use the packaged terminal entry and current evidence, preserve P17 pilot data,
and distinguish actual owner use/explanation from worker scripts.

## T13 — Release campaign and portfolio delivery

Packet/state: **T13 / in_progress**, 2026-10-04. Prerequisites T00–T12 complete.
Read main/terminal plans and status, scope/glossary, UX/architecture, protocol,
persistence, operations and verification. Historical P/O/T evidence, existing
pilot roots/services and the original dirty tree were preserved. No delegation,
new dependency, migration, causal/wire guarantee or owner-use claim.

The owner deferred personal use and comprehensive explanation/review until after
project delivery. The dated amendment in the owning scope/plan/packet supersedes
those learning gates. Independent technical acceptance continues. P17 engineering
and portfolio delivery is reconciled in the [main tracker](status.md#p17-delivery-reconciliation--2026-10-04).

Changed ownership: repository history/readiness/connection startup, control
status/operation publication observation/live identity inspection, pending TUI
operation timing and explicit retry, native/snapshot/fault safety runners,
six normally enabled T13 tests, and README/case-study/resume/operator/evidence
artifacts. Diagnosing-bugs skill applied to reproduced performance and storage
faults. Temporary executable diagnostic probes removed; logs/profiles retained.

Production findings: scoped prepared history statements and batched/reused
readiness resolve the 1,024-file status timeout while retaining unavailable and
concurrent heads. Single-owner exclusive WAL is configured before WAL access,
including reopened databases, to prevent mapped-index SIGBUS under real disk
exhaustion; FULL synchronization is retained. Live inspections use the owning
repository/control. Operation observations recognize actual publication without
reauthoring; pending UI uses ticks and explicit `r` replays the same operation.
Automatic unavailable-root pauses retain their cause. Regressions went red before
fixes and pass afterwards. Older raw-live-database/double-owner harness fixtures
now close or back up correctly; no acceptance assertion was removed.

Exact final production candidate: isolated snapshot
`e37e2600cd236776d5161f3a293718098bd7500e`. Original branch/index untouched.
A separate clean clone passed `make check` (392.981 s), uncached
`go test -race -count=1 ./...` (347.162 s), `make demo` (0.605 s), all 16
abrupt-reset cases, all five storage-failure cases, package checksums and
byte-identical repeated packages. Six `TestTerminalT13*` cases passed twice:
ordinary 4.687 s; `GOFLAGS=-race` plus test race 69.535 s, including child binaries.
Post-snapshot changes are two PTY timing fixtures and documentation; all current
production source matched the candidate at this entry's 2026-10-04 assembly and
the exact fixture hashes are recorded. The later 2026-10-05 follow-up below
records the subsequent production change separately.

Direct packaged laptop/Pi LAN normal create/invite/join/exact approval passed,
including both daemon restarts before approval, original bytes, bidirectional
edits, a second folder on the same device, heads/membership/identity agreement and
offline/reconnect. Actual packaged laptop/Pi/VPS engine campaign passed (129.045 s),
including offline concurrency, late arrival/stale refusal, forwarding, interruption
with six verified chunks reused, restore and restart integrity. Manual membership
and SSH relays in that engine campaign are explicitly distinct from ordinary LAN
onboarding. Final native Pi/VPS/laptop runs all passed the three keyboard campaigns
and unique user-service enable/start/capture/restart/removal; 33 sanitized
transcripts are archived. All 11 package transaction scenarios passed, including extracted PTY
and Debian/Fedora install/reinstall/removal. Containers and arm64 emulation are
labeled separately from actual native execution.

Thirty-two CLI resource observations cover two ordinary and two instrumented
repetitions of 1,024 files, 512 reviewed tombstones and exact 8/32-MiB streamed
merges. Ordinary sampled peak RSS 19,728–21,928 KiB; instrumented
52,180–57,816 KiB; 6–7 descriptors. Five-ms sampling can miss brief peaks;
inherited child high-water RSS is separate; daemon/editor memory is not included.
The full suite verifies background large-file progress under continuing small
edits. Historical 45-run full-size engine measurements remain dated.
Dependency verification and pinned vulnerability scan passed at execution time.
Python safety/VT tests and source/diff checks passed.

Invariants I01–I28 map to named executable assertions with evidence/limits in the
[T13 map](../evidence/terminal-t13-20261004/invariant-map.md).
VM reset/storage experiments establish tested virtual ext4 outcomes, not physical
Pi media power loss or arbitrary broken flush promises. All fault roots are newly
marked disposable environments; safety tests refuse pilot/unrelated actions.

Remaining technical acceptance: no existing authenticated Tailscale path exists
on the hosts; laptop/Pi have no private route to the VPS for ordinary native
three-host onboarding/forwarding and reviewed retirement/replacement; native
login/logout/boot has not run, with all user
managers `Linger=no`. Scoped service restart/removal cannot prove those conditions.
Do not reboot existing workloads or replace these checks with emulation. Next step:
use an existing authenticated private route and a designated disposable native
service environment for the three named checks. Owner review follows delivery.
[Commands/results/provenance and resumable handoff](../evidence/terminal-t13-20261004/summary.md).

### T13 follow-up — 2026-10-05

State remains **in_progress**, prerequisites T00–T12 complete. Initial read-only
host inspection, before the owner's installation, found no installed/authenticated
Tailscale and `Linger=no` on
laptop/Pi/VPS. The owner asked about deployment; the network runbook now
recommends one personal tailnet with a client on each host, separate Orbit
folder approval, laptop login startup and deliberately configured unattended
Pi/VPS startup. Existing WireGuard, pilot services and the original branch/index
are preserved. VPN policy, credentials, lingering and reboot remain untouched.

The additional ordinary CLI retirement/replacement campaign found two concrete
gaps. Retired-key approval failed safely but returned a generic internal error;
the enrollment owner now returns `RETIRED_MEMBER_REVIVAL` with fresh-identity
recovery. Fresh replacement enrollment omitted hash-bound retirement artifacts,
so admitted known retired histories remained blocked. The controller now fetches
the exact admitted revision's canonical snapshots through existing pinned mTLS
membership transport before history import. Repository same-revision replay can
hydrate those artifacts atomically without changing membership. Both regressions
went red before the fixes. Unknown retired histories, wrong artifacts, replay,
keys/counters, schemas and per-IP admission limits remain protected.

Eight instrumented T13 tests passed (73.545 s), including reopened incomplete
bootstrap recovery. Repository/control/CLI race checks passed. Package-extracted
checks and Python safety/VT checks passed. The current packaged ordinary journey
passed locally (153.065 s) and on laptop/Pi LAN installations (291.666 s), including
original-author forwarding, reviewed retirement, retired-key refusal, preserved
retired data and fresh replacement. The native rehearsal uses two physical hosts;
it does not establish the laptop/Pi/VPS acceptance. Full isolated current-candidate
reproduction passed `make check` (427.656 s), uncached full race (382.094 s), demo,
16 VM reset cases, five storage exhaustion cases, checksums and identical repeated
packages. The isolated candidate is `00992f6d8b6afcde7dfaffe2c8aad8c27c847602`;
current production source matches it. Results and exact provenance are recorded in the
[follow-up evidence](../evidence/terminal-t13-20261005/summary.md).
All experiments use newly marked disposable roots and stop only their own workers.
Historical failing attempts remain archived with their actual rate/expiry and
missing-artifact outcomes. The earlier candidate is historical; this follow-up
records current-source validation separately.

After the owner installed/authenticated Tailscale on all four devices, read-only
preflight confirmed authenticated/online clients and `tailscale0` peer routes on
laptop/Pi/VPS. The actual ordinary three-host journey passed in 277.636 s using
the exact clean candidate packages: create/join/delayed exact approval, preserved
existing bytes, edits, offline-source original-author VPS forwarding, reviewed
retirement/export/import/replay, explicit retired-key refusal and fresh Pi
replacement with preserved history/data. Three physical hosts were observed;
there were no owned-worker cleanup errors. Agent network policy, credentials,
lingering and reboot remained untouched. The earlier absent-Tailscale and LAN
rehearsal limitations retain their historical scope.

Remaining: actual native login/logout/unattended boot in a designated disposable
environment. Authenticated Tailscale and ordinary laptop/Pi/VPS private-route
onboarding/forwarding/retirement/replacement are satisfied. Personal use and unaided owner review remain
deferred follow-up work, not completion blockers. T13 is still the only open
packet; there is no T14.

### T13 native lifecycle closeout — 2026-10-07 (W17)

The owner chose a disposable KVM guest as the designated environment for the
last technical check. `scripts/validation/service_boot_vm.py` boots a Debian 13
cloud image (SHA-512 checked against Debian's list) on a throwaway overlay,
installs the packaged amd64 archive with `install.sh user`, and drives real
logind sessions over SSH. An administrator account observes without creating a
user session.

The first runs found two product defects. After the packaged install,
`orbit service enable` refused the packaged unit with `SERVICE_SELECTION_REQUIRED`,
because the unit's `--state=%h/...` was compared literally. Separately,
`service start` reported success while setup's manually launched daemon held
the state lock and the unit crash-looped. `validateSelectedService` now expands
`%h`. `start`/`restart` require the unit's `MainPID` to be the recorded owner,
and refuse with `MANUAL_DAEMON_RUNNING` (action: `orbit stop`, then
`orbit service start`). `orbit stop` now exists as the Orbit entry for the
graceful stop. Regressions `TestSelectedServiceAcceptsPackagedHomeSpecifier`
(red before the fix) and `TestStateOwnedByUnitRequiresUnitMainProcess`, plus a
manual-daemon case in `TestTerminalT02ServiceProcessActionsAndSelection`,
pass. The duplicated error-code text in service failures was also removed.

Final run ([run5](../evidence/wan-w17-20261007/lifecycle-vm/run5/service-boot-vm.json)):
unattended enable refused without lingering, naming `loginctl enable-linger orbit`;
start refused while the setup daemon ran; after `orbit stop` the unit owned the
state and captured an edit in 2.2 s; logout stopped it in 10.8 s; the next login
started it and captured an edit made while logged out; with lingering the unit
survived logout; after a reboot it was active and captured with no user
session, 13.8 s from guest start. Runs 1–4 retain the failures and harness
corrections. This is virtual-machine evidence; physical-hardware boot was not
executed, by the owner's choice. All T13 automatic criteria now have evidence;
personal use and unaided explanation remain deferred follow-up, and P17 stays
open on those items.
