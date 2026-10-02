# Orbit packets O00–O04: correctness, contracts and first use

Read [plan](../orbit-implementation-plan.md), [scope](../portfolio-scope.md),
[product](../orbit-product.md), [architecture](../orbit-architecture.md) and the
owning specifications linked by the selected packet. Commands below are planned
checks, not executed evidence. New `TestOrbit…` names are proposed test groups;
builders create meaningful cases and verify that filtered runs actually execute them.

## O00 — Verify and repair recovery regressions

**Depends on:** existing engine/toolchain. **Requirements:** S11, S20, U14.
**Invariants:** I07, I08, I19, I20.
**Owner:** 6.1 Sol medium lead. **Owning specs:** persistence, operations, verification.

Source inspection identified these candidates; reproduce before asserting a
failure or marking a fix complete:

| Candidate | Starting source | Required regression oracle |
| --- | --- | --- |
| Reset/restore write `version` instead of required `format_version` and omit `created_at` | `internal/control/maintenance.go`; `internal/config/config.go` | Real config loader and restarted daemon accept the recovered state |
| Reset/restore call `LoadOrCreateIdentity`, which can reuse the old key | maintenance; `internal/replication/identity.go` | Device ID and key pin change; old credentials cannot act as the new device |
| Reset does not align database authors; restore ignores author-update errors | maintenance; repository folder records | Checked transactional author transition; old history preserved; no rolled-back old-author event |
| RecoveryInspection calls the reclaim operation | maintenance; `internal/workspace/workspace.go` | Repeated inspection leaves roots, protected recovery files, operation state and membership unchanged |
| Recovery runbook uses stale names/commands | `docs/runbooks/database-recovery.md` | Documented commands execute against a new marked fixture |

Build fresh marked disposable fixtures containing captured versions, a metadata
backup and pending recovery work. Add restart/key/author assertions through real
CLI/control/module interfaces. Preserve the failing reproductions. Repair the
typed configuration write and separate normal load from explicit identity
rotation; use a safe stopped/exclusive transition rather than changing identity
under a live writer. If interrupted-transition design needs G04, fence unsafe
recovery with an explicit pending/error state until G04/O02 completes it.

Audit rollback ordering, checked SQL errors, permissions, file/directory flushes,
WAL handling and unavailable content. Do not equate a returned new ID with
successful recovery. Separate inspection from an explicit cleanup mutation.
Update affected runbooks and historical release limitations without erasing
earlier test results.

**Acceptance:** each candidate is reproduced or explicitly disproved with
evidence; config/schema/key/author restart tests pass; reads perform no cleanup;
unsafe partial recovery is fenced; existing protected versions remain readable.
Unresolved candidates stay recorded and block relevant release claims.

**Planned checks:**

```sh
make build
go test -count=1 -v ./internal/config ./internal/replication ./internal/control ./tests/integration -run 'TestOrbitRecovery|TestP13|TestP15'
make test-race
```

Use existing `internal/testkit` marker validation for fault actions. No pilot
service, personal folder or existing VPS workload is a recovery fixture.

## O01 — Close the revamp's design gates

**Depends on:** O00. **Requirements:** U05, U08, U10, U13, U14.
**Invariants:** I01–I20 as applicable; planned I21–I28.
**Owner:** lead. **Owning specs:** protocol, persistence, operations, verification.

Close G01–G05 from [architecture](../orbit-architecture.md#design-gates). Add
experiments under `tests/designgates` and independent membership schedules under
`model` where appropriate. Freeze:

1. Local launch/bootstrap for initialized and uninitialized state; singleton
   behavior; one-use handoff and session/logout behavior.
2. Invitation transport and capability scope, key proof, owner-approved artifact,
   expiration/revocation/replay, opt-in policy and bounded unauthenticated work.
3. Offline sequential membership rollout and competing administration. Define
   a tested fork/error recovery path and compatibility with manual approval.
4. File mutation ordering and durable journals, overwrite/subtree tokens,
   partial/canceled moves, source edits and content-read pin lifetimes.
5. Identity/config/metadata transition across crashes, legacy product preferences,
   schema/protocol capabilities and rollback limits.

Preserve exact membership agreement for content requests, conservative retirement,
explicit conflicts, root safeguards and captured-version protection. If an
experiment cannot maintain a guarantee, record the counterexample and seek
owner input only for the actual scope/guarantee change. A blocked mechanism is
an engineering problem; do not invent a stronger guarantee to bypass it.

Write gate outcomes in `docs/design-gates.md` (or a linked Orbit-specific outcome
file when the material warrants it), and update the owning specifications and
schema fixtures. Distinguish adopted contracts from planned mechanisms.

**Acceptance:** every gate has a written decision, executable model/reproducer,
negative/adversarial cases, matching specs and stated limitations. Enrollment,
file mutations and stopped recovery cannot begin with their gates unresolved.

**Planned checks:**

```sh
make test-model
make test-faults
go test -count=1 -v ./tests/designgates -run TestOrbit
```

## O02 — Product records and shared control contracts

**Depends on:** O01. **Requirements:** U01, U04, U09, U13, U14.
**Invariants:** I08, I13, I16, I19, I20, I22, I26.
**Owner:** lead. **Owning specs:** operations, persistence, protocol/schema fixtures.

Extend typed repository/configuration interfaces for product preferences,
device/workspace display names, default workspace, endpoints, setup/enrollment
state and operation progress. Preserve old config/state/keys; product-label
changes never rotate identity. Freeze exact control request/response/error
contracts for later frontend packets, including decimal-string counters,
generation tokens, durable IDs, cancellation and partial results.

Implement schema migration/backfill and safe legacy import. Label policy is
owner-local aliases unless G05 adopts a different documented contract.
Conflicting/duplicate labels remain usable. Endpoint edits use validated
durable configuration and safe runtime refresh, with explicit failure states.
Enforce folder authorization separately from endpoint/name knowledge.

Complete G04's stopped recovery implementation if O00 fenced an unsafe path.
Expose recovery status without a live reset endpoint that bypasses exclusive
state ownership. Publish fixtures/types so presentation work can proceed without
inventing control logic.

**Acceptance:** old states open with identities, roots, heads and keys intact;
preferences survive restart; invalid settings are rejected without partial
configuration; endpoint changes actually affect the intended pull directions;
CLI and browser types describe the same operations. Interrupted migrations/
identity transitions recover or refuse safely with actionable status.

**Planned checks:**

```sh
make build
go test -count=1 -v ./internal/config ./internal/repository ./internal/control ./tests/integration -run 'TestOrbitSettings|TestOrbitMigration|TestOrbitRecovery'
```

## O03 — Local launcher, service and setup operations

**Depends on:** O02. **Requirements:** U04, U08, U12, U16.
**Invariants:** I08, I11, I13, I19, I20, I21, I22.
**Owner:** lead. **Owning specs:** operations, persistence.

Add an Orbit entry/launcher while preserving the legacy CLI. Discover selected
state and running daemon safely; reuse the authenticated local interface; open
the browser using the tested G01 handoff. Do not silently choose between
multiple states or initialize over invalid existing state.

Implement InspectSetup, PreviewCreate/JoinRoot, Start/ResumeSetup and progress
queries behind control. Generate folder/device identities internally. Default
to `~/Orbit`, permit location change, reject unsafe/overlapping roots, and
review nonempty contents before adoption. State remains outside synchronized
roots. Setup preserves files through cancellation and restart.

Provide an authenticated, paginated directory picker or validated location
entry, and a safe Open local folder desktop helper. Handle headless sessions
without assuming a browser upload input reveals the daemon's filesystem path.

Integrate user-service enable/start as a real operation with separate reported
results: enabled on login, currently running, root verified, capture successful.
Handle absent systemd/session tooling gracefully; manual/headless alternatives
remain available. Storage-host lingering is a documented optional administrator
step, never a silent privileged host change.

**Acceptance:** first launch, repeated launch, already-running service, partial
setup and invalid old state are exercised. No duplicate daemon owns the state.
Existing content survives. Bootstrap secrets do not leak via logs/argv/browser
storage/history/support export. Closing/logout of UI does not stop background
work. Setup failure identifies the step and supports safe resume.

**Planned checks:**

```sh
make build
go test -count=1 -v ./internal/control ./tests/integration -run 'TestOrbitLaunch|TestOrbitSetup|TestOrbitService'
```

## O04 — Orbit shell and complete first-device UI

**Depends on:** O02, O03. Contract-bound layout work may start after O02.
**Requirements:** U01, U04, U07, U08, U15, U16.
**Invariants:** I19, I21, I22, I27.
**Owner:** 3.8 Flash implementation lane; lead reviews integration.
**Owning specs:** product, operations/control contracts.

Replace the operator-console shell with Files, Needs attention, Devices,
Deleted files and Settings navigation. Introduce semantic CSS tokens,
consistent inline/vector icons, responsive sidebar/topbar and reusable dialogs/
feedback. Use the existing React/TypeScript/Vite stack and local CSS unless an
additional dependency demonstrably simplifies the agreed interactions.
Bundle necessary assets; daily use must not depend on remote font/image services.

Build Create/Join entry and the complete create workflow over real O03
operations. Show device name, default/changeable root, root-content preview,
storage configuration and actual service/capture progress. Technical IDs move
to details. The Join entry can identify the next packet's availability;
do not ship a success-looking fake join.

Add a deterministic browser runner using the existing puppeteer-core tooling
and a configurable installed Chromium path. It launches real marked fixture
daemons and asserts outcomes, not just screenshots. A missing browser is a
recorded unexecuted check, not a pass.

**Acceptance:** a fresh user creates a workspace through the launcher/UI;
nonempty/invalid roots and retries are understandable; keyboard-only completion,
focus return, loading/error states and narrow-window layout work. No runtime
Node service. Real filesystem/capture state matches the displayed completion.

**Planned checks:**

```sh
cd web
npm ci
npm run build
cd ..
make build
node scripts/orbit_ui_test.mjs --scenario setup
```

`scripts/orbit_ui_test.mjs` is a deliverable of this packet, not an existing
command at planning time. Export screenshots for setup, validation, progress,
Files shell and a narrow browser window alongside assertion results.
