# Orbit packets O11–O13: sustained operation and release

Read [plan](../orbit-implementation-plan.md), owning specifications and
[verification](../verification.md). Release evidence applies to the exact new
source/package, not screenshots or tests from the earlier operator console.

## O11 — Settings, storage, recovery and long-running operation

**Depends on:** O02, O03, O06, O09, O10.
**Requirements:** U04, U09, U11–U14, U16; S14, S17, S21.
**Invariants:** I07, I08, I10, I13, I15, I16, I19, I20, I26, I28.
**Owner:** lead for persistence/settings; Flash for contract-bound presentation.
**Owning specs:** operations, persistence, protocol.

Expose local root/workspace registration, pause/resume/unregister, storage
accounting and finite limits, retention preview/change, integrity/repair,
explicit cleanup, diagnostics and startup status through shared operations.
Advanced workspace creation/join uses the same O03/O05 flows. Every selected
workspace is fully replicated; folder selection never masquerades as placeholders.

Show working-root, managed objects/history, staging/recovery, metadata and
actual filesystem free space distinctly. Default limits are visible and
capacity checks cover state/root filesystems where separate. Retention
preview explains which bytes are eligible, which remain protected and why
Deleted files is conditional. Cleanup remains an explicit mutation.

Finish any missing endpoints/parity for these existing engine operations.
Settings changes that require restart say so and provide a safe operation;
UI must not report live adoption when only a file was written.
Unregister preserves local files/history and emits no deletion.
Root relocation/mass deletion remains a reviewed procedure; arbitrary
dragging/remounting a root is not silently adopted.

Audit finished scheduler/operation/invitation/read-lease records. Add bounded
pruning where safe with a defined replay lifetime and explicit expired replay
rejection; preserve pending work, causal metadata, recovery journals and
content pins. Demonstrate sustained ordinary use without unbounded terminal
task growth or hiding new work behind old records.

Implement the lost-device/replacement runbook and UI-guided maintenance status.
Actual identity/backup restore executes stopped/exclusive through G04. Report
missing payloads honestly; metadata backups are not presented as full backups.
Headless owner access and optional service lingering are documented.

**Acceptance:** setting changes survive/reconcile restart; accounting matches
declared categories; cleanup never occurs through an inspection read; stopped
recovery yields a valid fresh-key state and reviewed replacement enrollment.
Pruning preserves pending work/idempotency safety across restart and limits.
Root unavailable, full storage, corruption and retirement each yield a
concrete recoverable UI flow. Removal/uninstall retain user data.

**Planned checks:**

```sh
make build
go test -count=1 -v ./internal/control ./internal/repository ./internal/scheduler ./tests/integration -run 'TestOrbitSettings|TestOrbitStorage|TestOrbitPruning|TestOrbitRecovery|TestOrbitRetirement'
node scripts/orbit_ui_test.mjs --scenario settings
node scripts/orbit_ui_test.mjs --scenario recovery
```

## O12 — Orbit packages, legacy adoption and executable documentation

**Depends on:** O04, O06, O08, O10, O11.
**Requirements:** U01, U08, U12–U14, U16; S20–S22.
**Invariants:** I08, I19, I20, I21, I28.
**Owner:** lead for installation/migration; Flash for reviewed docs/assets.
**Owning specs:** operations, persistence, verification.

Package Orbit entry/desktop launcher, embedded frontend, icon and user-service
integration for Linux amd64/arm64. Keep the existing CLI/service/state migration
policy frozen in G05. For the first release, retain `filesync` compatibility
and adopt or explicitly migrate one existing user service rather than enabling
a parallel process. No automatic relocation of state/roots or identity reset.

Make frontend freshness checkable: release packages must contain the rebuilt
UI from the declared source revision, not stale committed `web/dist` assets.
Users require no Node runtime. Asset build, licensing/checksums, configuration
and schema versions are recorded.

Exercise fresh install, existing state/service adoption, startup/login,
ordinary restart, upgrade/preflight backup, interruption, supported binary
rollback and refusal of newer incompatible state. Metadata rollback follows
fresh-identity recovery rather than quietly resuming old counters.
Uninstall preserves roots/state by default.

Rewrite README/install/headless/private-network/recovery/uninstall runbooks
around actual Orbit commands. Mark legacy wire/storage names as compatibility
details, not half-finished user branding. Remove obsolete commands from
executable runbooks. Retain earlier release evidence under its original names.

**Acceptance:** extracted packages launch the matching UI and capture actual
files on native target architectures; desktop startup and headless operation
are exercised. Old identities/heads/content persist through ordinary upgrade.
Unsupported protocols/schemas fail clearly. Documented procedures execute in
marked fixtures. Repeated build/checksums and browser assertions use packaged
binaries. Cross-build success alone is not native-device evidence.

**Planned checks:**

```sh
cd web
npm ci
npm run build
cd ..
make package
make check
make test-race
make demo
```

Extend existing marked `scripts/validation/service_lifecycle.py` and
`reproduce_release.py` where applicable rather than creating an unsafe parallel
host harness. Native checks remain explicitly scheduled against fresh roots.

## O13 — Failure campaign, usability pilot and final handoff

**Depends on:** O00–O12 complete. **Requirements:** U01–U16; S01–S22.
**Invariants:** I01–I28.
**Owner:** lead integrates evidence; actual owner performs the personal pilot.
**Owning specs:** verification, all updated contracts.

Run the current full suites plus new enrollment/operation/browser scenarios
on the final source/packages. Use safe existing harnesses and new named hooks
for launch, approval, membership rollout, import/move/delete and identity
transition. Re-run abrupt-reset/disk-exhaustion experiments for affected durable
phases with declared virtual-storage assumptions.

Minimum product scenario set:

1. Fresh install/create default folder; repeat launch; nonempty alternate-root
   preview; invalid root; single-owner device names and advanced workspace.
2. UI invitation/approval of second device; invitation decline/expiry/replay;
   wrong key/unauthorized workspace; third peer offline during rollout.
3. Existing-content join; no bootstrap deletions; A→hub→B without direct
   author/receiver overlap; correct copy status and last contact.
4. Nested browse/search/preview; ordinary create/upload/rename/move/delete;
   destination collision; stale subtree; interrupted and partial operation.
5. Three offline edits, reconnect permutations, reviewed resolution, later
   unseen arrival, delete vs edit and conditional historical restore.
6. Browser close/logout while daemon syncs; service restart and OS-login
   behavior; no duplicate daemon; headless administration.
7. Corrupt/missing content, expired history, root unavailable/replaced,
   full storage, safe GC/read leases and bounded long-running records.
8. Lost device/retirement and fresh-key replacement; metadata recovery with
   interrupted transition; no old-author/counter reuse.
9. Existing File Sync state/package upgrade, capability mismatch and supported
   rollback; uninstall retains data; no remote asset/runtime dependency.
10. Keyboard-only create/join/browse/actions/conflict/restore, long/unusual
    filenames, narrow window, zoom, focus return and reduced motion.

Record exact pass/fail assertions, screenshot states, per-scenario commands,
source/package checksums and limitations. Measure browse/search costs on 10,000
synthetic files, a large stream/import and long-running task growth; report
actual samples/resources rather than invented speedups. New UX must not
introduce O(total workspace) memory per ordinary page request.

Perform an actual owner pilot with start/end times, normal edits, sleep/offline/
reconnect and restart. Use a separate dedicated folder; fault campaigns never
target that pilot. Preserve the still-outstanding P17 owner-use/explanation
requirements; credit this pilot toward them only when its actual evidence
matches their acceptance criteria.

**Acceptance:** every U requirement and applicable old/new invariant has
evidence on the final revision. Known material defects have a fix or an
explicit scope decision. No failing safety oracle is weakened for release.
Unexecuted native/VM/personal-use checks remain labeled unexecuted and block
their corresponding completion claim. Produce a reproducible demo, measured
limitations, final status and the owner's distributed-system explanation.

**Planned checks:** existing `make check`, `make test-race`, `make demo`;
the implemented `node scripts/orbit_ui_test.mjs --scenario all`; marked native
three-host/service campaigns; explicitly separate VM/reset/disk-full campaigns.
Consult `docs/verification.md` for existing commands and prerequisites.
