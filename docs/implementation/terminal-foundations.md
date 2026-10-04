# Terminal packets T00–T05: contracts, daemon and onboarding

Use the [plan](../orbit-terminal-implementation-plan.md),
[architecture](../orbit-terminal-architecture.md),
[UX contract](../orbit-terminal-ux.md), and [tracker](terminal-status.md).
Checks below define acceptance; actual execution is recorded in the tracker.
Discover nonzero matching tests
before using a packet's proposed `TestTerminalTXX` group. Evidence and completion
follow the plan's common rules; preserve existing uncommitted relocation work.

## T00 — Baseline and reproductions

Dependencies: none beyond the existing repository. Owner: assigned worker.
Read: scope/glossary, terminal documents, current P/O status, protocol,
persistence, operations and verification. Change: disposable reproduction
fixtures/harness, evidence and status; production repair belongs to later packets.
Invariants: identify affected I08–I09, I11, I13, I16, I19–I24 and I27–I28.

Required outcomes:

- Inventory the actual current binary/help, schema, API, launch/service options,
  limits initialization and live/stopped adapters. Record dirty-tree provenance
  and baseline commands separately from historical results.
- Reproduce the UX document's hardening findings using fresh private roots and
  production entry points: missing enrollment reachability, inviter verification,
  wrong-folder capability handling, same-key second-folder request collision,
  incomplete join persistence, root-preview/readiness limits, live-daemon CLI
  locks, and missing finite initialization/status fields.
- Mark each candidate reproduced, source-only, or disproved with the precise
  command/result. Update the finding when evidence differs; do not invent a
  failure because a prior source summary said it existed.
- Identify the smallest production-interface regression for each real failure,
  plus the owning T packet. Keep deliberately failing baseline cases opt-in
  until the repair packet promotes them into passing behavior regressions.
  Their observed failures remain failures in evidence.
- Record existing check failures without fixing unrelated work opportunistically.
  Publish ready work and concrete prerequisites, including native-host access.

Checks: version/help and current baseline `make check`/`make test-race` where
available; targeted disposable process/network reproductions. T00 is complete
when the inventory and every candidate disposition have actual evidence, not
when the new behaviors pass. Missing hardware is labeled, not inferred from
local tests. Explain which finding is a transport, identity, scope or UI adapter
problem and why changing the screen alone cannot repair it.

## T01 — Contracts and design gates

Dependencies: T00. Owner: assigned worker.
Read: terminal architecture TG1–TG5, UX command map and owning contracts.
Change: typed control/result/fixture definitions, owning specifications,
design experiments and exact source-ownership manifest. Invariants: I09,
I13, I16, I19–I28.

Required outcomes:

- Freeze additive operation/result schemas for setup/adoption, invitation/join,
  per-folder sharing, operation inspection, status/attention, named context,
  exact-version reads, reviewed conflict/restore and settings/service modes.
  Define large integers, error codes, retryability and version/capability rules.
- Give every mutation a durable operation identity, request fingerprint and
  explicit reviewed inputs. Define late-response/cancel/retry semantics and
  expired replay behavior. Freeze stdout/stderr/JSON and script exit categories.
- Resolve TG1–TG5 design choices with bounded executable experiments/models;
  record remaining production proof assigned to T02–T12. Define invitation
  identity verification and replay-bound request transcripts before endpoint code.
- Define root-preview generations, incomplete scan/unsupported-item results,
  available-capacity assumptions, resumable joining phases and exact readiness
  observations. Preserve bootstrap absence and uncaptured working-byte distinctions.
- Define editor/read sessions, stale reviewed heads, retention/expiry, and the
  separate-copy recovery result without changing causal ancestry or safety pins.
- Publish fixtures for success/empty/loading/awaiting approval/offline/stale/
  storage-blocked/root-unavailable/partial/fork cases. Assign files by task;
  fixture-backed view work can start but cannot establish integration completion.

Checks: planned `TestTerminalT01` codec/validation/contract tests and relevant
model cases; Markdown/schema fixtures and gate experiments. Completion requires
written outcomes, executed experiments, consistent owning contracts and fixtures.
Production gate evidence remains open until its named implementation packet.
Explain why operation identity and reviewed generations survive a screen restart.

## T02 — Shared client and daemon lifecycle

Dependencies: T01. Owner: assigned worker.
Read: TG3/TG5, operations authentication/lifecycle/limits, persistence ownership
and identity recovery. Change: shared control client, launcher/config/state,
service integration, control adapters and CLI lifecycle paths. Invariants:
I08, I13, I19–I22, I28.

Required outcomes:

- Extract/reuse typed live-daemon calls from existing helpers. Authenticate from
  owner-only local credentials, verify local endpoint selection, bound streams
  and errors, and enforce the exclusive lock for stopped adapters. An HTTP
  failure cannot trigger concurrent direct database access.
- Use one initializer for fresh CLI/launcher/service paths, with finite validated
  limits. Existing missing-limit states get an honest review/migration path;
  existing keys, roots and version history remain intact.
- Make ordinary terminal startup reuse or start exactly one selected daemon.
  Persist/validate peer-listener/network settings independently of loopback
  owner control. Start the intended listeners consistently from service and CLI.
- Separate running, enabled at login, unattended prerequisites, root health and
  capture health. Start/enable/stop/restart report actual results. Document owner
  steps for lingering; preserve existing personal services and firewall/VPN policy.
- Client exit and query cancellation leave the daemon running. Explicit daemon
  stop is separate. Recovery/journals run before ordinary scans and networking.

Checks: planned `TestTerminalT02` process/control/initialization cases; concurrent
starts, live and stopped adapters, stale endpoint/credentials, restart after
partial initialization, missing systemd and explicit service errors. Run relevant
CLI/package tests and race checks. Boot/logout guarantees need later native
T12/T13 evidence; local process tests are not that evidence. Explain enabled
versus running and why a failed live call cannot use the stopped adapter.

## T03 — Authenticated network enrollment

Dependencies: T01, T02. Owner: assigned worker.
Read: TG1, protocol enrollment/membership/data authorization and operations
peer/local-control isolation. Change: replication enrollment listener/handlers,
invitation/request controller and repository records, client TLS and peer fixtures.
Invariants: I09, I13, I15, I20, I23–I24.

Required outcomes:

- Serve bounded capability-gated enrollment over the configured reachable
  network transport; preserve full pinned member authorization on data routes
  and loopback owner authentication on control routes. Resolve same-listener
  versus isolated enrollment admission through TG1's recorded experiment.
- Encode an expiring invitation with exact folder, inviting key/certificate
  binding, capability and usable address. Verify that identity before token
  disclosure. A bare TLS-verification bypass cannot authenticate the inviter.
- Verify key possession over the frozen replay-bound request transcript; bind
  invitation consumption to recorded scope, attempt and limits. Signatures,
  display names and possession of a token cannot substitute for owner approval.
- Approval displays and binds the exact requester/key/folder/current revision.
  Freeze the human verification-code derivation through established crypto
  primitives. Approval artifacts/status retrieval cannot be read or forged using
  only an arbitrary request ID. Preserve linear membership and fork detection.
- Expired/revoked/replayed/wrong-scope invitations, invalid proof, mismatched
  inviter, unknown data client, oversized inputs and throttled requests fail
  before unauthorized data access or unbounded work. Keep secrets out of argv,
  automatic logs and support exports; explicit invitation transfer is deliberate.

Checks: planned `TestTerminalT03` tests cross actual TLS/HTTP/control interfaces;
negative identity/scope/replay/data-isolation cases and two-process request/
approval/membership exchange. Record explicit fresh nonloopback/network tests
where available. Close TG1 transport production proof only with those results;
T05 still owns multi-folder/offline rollout proof. Run relevant model/race tests.
Explain how invitation capability, device identity and folder authorization differ.

## T04 — Reviewed setup and resumable joining

Dependencies: T02, T03. Owner: assigned worker.
Read: TG2, persistence scanning/bootstrap/root safety, setup UX and operations
storage. Change: root preview/adopt controls, setup journal/jobs and interactive
CLI setup/join adapters. Invariants: I03–I08, I11–I13, I16, I19–I22, I27–I28.

Required outcomes:

- Implement bounded recursive adoption preview with measured file/byte counts,
  unsupported/unreadable items, capacity and incomplete-enumeration indicators.
  Bind approval to the root identity/generation; a changed tree requires review.
  Reject overlapping/state/symlink roots through existing owning validation.
- CLI create/join prompts expose name/root/network/startup/finite settings in
  one review, retain correctable inputs and offer existing-folder adoption normally.
  Use typed operations against a live daemon; keep TUI-specific rendering for T10.
- Persist request ID, authenticated inviter/endpoint, folder, root, review,
  operation/fingerprint and current phase needed for safe restart. Store sensitive
  data privately; retries use the same valid attempt and handle expired attempts
  through explicit new requests rather than silent authorization renewal.
- Scan/import existing local contents before destructive remote projection;
  bootstrap absence creates no deletion. Distinct existing histories remain
  reviewable conflicts. Preserve files through partial/failed setup.
- Track approval, membership update, local capture, missing content and
  publication separately. A scan error or pending content prevents the relevant
  Ready claim. Closing the prompt, delayed approval and daemon restart resume
  the workflow; percentages have measured denominators.

Checks: planned `TestTerminalT04` real CLI/process cases: empty/nonempty roots,
unsupported objects and root swaps, delayed approval, interruption at persisted
phases, expired attempt, disk admission and initial scan failure. Confirm actual
files/versions/keys and no bootstrap tombstones, with two-device normal edits and
verified transfer. Run relevant workspace/setup/race regressions. Completion
establishes M1. Explain local capture versus enrollment versus Ready here.

## T05 — Additional-folder sharing and rollout

Dependencies: T03, T04. Owner: assigned worker.
Read: protocol membership/retirement/status, TG1/TG2, per-folder sharing UX,
operations endpoints and persistence request/replay retention. Change: scoped
enrollment/request identity, endpoint persistence/runtime adoption, membership
rollout and additional-root acceptance. Invariants: I09, I14–I16, I19, I23–I24, I28.

Required outcomes:

- Share another folder with an already known device using its persistent key,
  separate folder approval, distinct scoped attempt identity, and safe retry.
  A retained first request cannot collide with the new request or bypass scope.
- Require receiving local-root choice/adoption review. Reuse authenticated
  identity and validated connection information; directory names or device aliases
  do not authorize a folder. New folders remain unshared until explicitly selected.
- Persist and apply approved endpoint changes at runtime; configure/test each
  required pull direction, including third-peer forwarding. Network path changes
  produce retryable connection observations rather than rekeying or new membership.
- Propagate owner-approved revision chains sequentially with observable offline
  work and expected-predecessor checks. Do not transfer data on mismatch; surface
  competing approvals/forks with an executable reviewed recovery procedure.
- Preserve conservative retirement, known history and fresh-identity replacement.
  An offline peer is never retired to make another operation appear complete.

Checks: planned `TestTerminalT05` two-folder/same-device and third-peer process
scenarios; denied unshared-folder reads, offline rollout/restart, competing
approvals, endpoint refresh, revocation/replay and retirement model regressions.
Verify metadata/head sets and exact files, not only labels or request status.
Run relevant model/race checks. Explain why one device identity can participate
in several independent folder memberships without global trust being invented.
