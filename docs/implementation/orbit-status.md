# Orbit implementation status

Active work moved to the [terminal tracker](terminal-status.md) on 2026-10-03.
Start at T00 using the [terminal plan](../orbit-terminal-implementation-plan.md).
The O entries below retain their dated scoped evidence, including O14 relocation;
their completion labels do not establish the newly required terminal journeys.

Updated: 2026-10-03. O00–O13 and owner-selected O14 are recorded complete with scoped evidence. Earlier engine
evidence and remaining P17 owner-use/explanation work are in [legacy status](status.md).

Plan: [Orbit implementation plan](../orbit-implementation-plan.md).
Scope/journeys: [scope](../portfolio-scope.md), [product](../orbit-product.md).
Contracts/gates: [architecture](../orbit-architecture.md).

## Packet tracker

| Packet | State | Dependencies | Evidence / next condition |
| --- | --- | --- | --- |
| O00 Recovery regressions | complete | Existing engine | TestOrbitRecovery (6/6), TestP13, TestP15, make test-race pass |
| O01 Design gates | complete | O00 | TestOrbitG01-G05 (19/19), model/membership (3/3), make test-race pass |
| O02 Records/contracts | complete | O01 | Schema 11 migration, typed settings/endpoints/leases/setup contracts, TestOrbitSettings/Migration pass |
| O03 Launcher/setup | complete | O02 | Local launcher, setup operations, systemd user service verified (19/19 tests) |
| O04 Shell/first-use UI | complete | O02, O03 | Browser runner (10/10), 9 screenshots, UI unit/integration tests pass |
| O05 Enrollment/rollout | complete | O01, O02 | TestOrbitEnrollment (4/4), TestOrbitMembership (5/5), TestOrbitEndpoints (5/5), make check, make test-race pass |
| O06 Pairing/device UI | complete | O04, O05 | TestOrbitPairing (5/5), 7 screenshots, headless runbook, make test-race pass |
| O07 Browse/content | complete | O02, G03 | [O07 evidence](../evidence/orbit-o07/summary.md); browse/search/history, pinned HTTP/ranges, GC/cancellation, CLI parity, make check/race pass |
| O08 Browser/details UI | complete | O04, O07 | [O08 evidence](../evidence/orbit-o08/summary.md); deep hierarchy, search 10,000 files, grid/list, I27 status badges, side panel, previews, exact download, history, 8 screenshots (17-24), make check/race pass |
| O09 File mutations | complete | O01, O02, O07 | [O09 evidence](../evidence/orbit-o09/summary.md); Schema 13, import/mkdir/move/delete, I26 source race, journal recovery, stopped vs live daemon CLI parity, make check/race pass |
| O10 Actions/attention UI | complete | O08, O09 | [O10 evidence](../evidence/orbit-o10/summary.md); file actions/collision modals, history timeline/conditional restore, deleted files view, Needs attention triage, 3 conflict resolution strategies, screenshots 25–33, make check/race pass |
| O11 Settings/sustained operation | complete | O02, O03, O06, O09, O10 | [O11 evidence](../evidence/orbit-o11/summary.md); storage accounting, retention preview/GC, Invariant I20 unregister, Invariant I28 bounded pruning, lost device guide, screenshots 34–38, make check/race pass |
| O12 Packages/migration | complete | O04, O06, O08, O10, O11 | [O12 evidence](../evidence/orbit-o12/summary.md); 8 packages, desktop/icon, service adoption, schema 5->13 adoption, rollback refusal, stopped restore, install/uninstall lifecycle, make check/race/demo pass |
| O13 Final validation/pilot | complete | O00–O12 | [O13 evidence](../evidence/orbit-o13/summary.md); 10 minimum product scenarios pass, scaling 10k files (0.59 MB heap), 38 UI screenshots, make check/race/demo pass |
| O14 Local location changes | complete | O03, O09, O11, O12 | [Relocation evidence](../evidence/orbit-relocation/summary.md); same/cross-filesystem moves, recovery, CLI parity and Chromium flow pass |

## Next action

Orbit revamp and local relocation complete across O00–O14. P17 personal owner pilot and unaided explanation ready for Caleb's execution.


## Planning record

Changed only documentation: product vision, architecture/gates, packet plan,
tracker, glossary and repository entry/owning-spec pointers.

Actual work: inspected git status and existing specs/control/identity/config/
Makefile/browser tooling; consulted writing-for-agents, domain-modeling,
codebase-design and UI/UX guidance. Found source-level recovery candidates;
did not execute identity reset, backup restore, cleanup or fault injection.
The generic design-system search returned marketing-page structure and was
not adopted; a narrower verified minimalism style informed the monochrome
baseline. No generated design-system output was persisted.

Validation actually executed:

- `git diff --check`: passed, no tracked-diff whitespace errors.
- Inline `python3` Markdown/plan validator: 19 files and 200 local links/
  anchors checked; code fences balanced; 0 errors.
- The same validator checked all 14 packet definitions/tracker rows,
  acyclic dependencies, coverage of all 16 U requirements and definitions
  of I01–I28; 0 errors.

Go/frontend/model/native/VM/pilot checks are **unexecuted for this planning
change**. Historical test results remain dated historical results.

Remaining limitations: all new behavior is pending; G01–G05 are proposals;
recovery findings need executable reproduction; the existing P17 owner pilot/
explanation still needs actual evidence.

## Packet completion entry

For each packet append:

```text
Packet / state:
Prerequisites and owning specs checked:
Gate outcomes and affected contract versions:
Changed files:
Requirements / invariants:
Actual commands and results (including skipped/unexecuted):
Evidence revision, packages and paths:
Remaining limitations:
Next eligible work:
```

Complete only when every packet acceptance criterion has evidence. Record
failures and safe partial results, not just successful compilation.

### O00 — Verify and repair recovery regressions

```text
Packet / state: O00 Recovery regressions / complete
Prerequisites and owning specs checked:
- Existing engine, build tools, and disposable test fixtures.
- docs/persistence.md: scanning, publication, SQLite metadata, restore, cleanup semantics.
- docs/operations.md: configuration, identity, maintenance, and upgrade runbooks.
- docs/verification.md: invariants I07, I08, I19, I20, crash consistency boundaries.
Gate outcomes and affected contract versions:
- Fenced live identity mutation by requiring exclusive stopped lock (state.Acquire) in ResetIdentity and RestoreBackup.
- Gate G04 interrupted-transition scope remains assigned to O01/O02; no breaking wire schema change; format_version: 1 and SQLite user_version: 5 preserved.
Changed files:
- internal/config/config.go: added Save function ensuring atomic write of typed config (format_version, device_id, created_at with mode 0600) and simplified Initialize.
- internal/config/config_test.go: added TestSaveAndLoadRoundtrip.
- internal/replication/identity.go: added RotateIdentity to explicitly generate fresh ed25519 keypair and certificate, keeping normal loading separate in LoadOrCreateIdentity.
- internal/replication/identity_test.go: added tests for LoadOrCreateIdentity key reuse and RotateIdentity key rotation / pin update.
- internal/workspace/workspace.go: added InspectRecoveryCopies read-only method scanning for unreferenced/committed recovery files without unlinking or releasing reservations.
- internal/workspace/workspace_test.go: updated TestReclaimRecoveryCopies to verify InspectRecoveryCopies leaves files and reservations intact before explicit reclaim.
- internal/control/maintenance.go: updated RecoveryInspection to call read-only InspectRecoveryCopies; updated ResetIdentity and RestoreBackup to acquire exclusive lock, rotate identity via replication.RotateIdentity, write typed config via config.Save, and execute transactional folders author/counter alignment (local_author = newID, next_counter = 0) with checked SQL error handling.
- cmd/filesync/main.go: updated handleMaintenanceResetIdentity to invoke control.ResetIdentity directly with exclusive stopped-daemon locking.
- internal/state/state.go: updated EnsureDirectory to secure permissions with os.Chmod(path, 0700).
- docs/runbooks/database-recovery.md and docs/runbooks/full-disk.md: updated stale database file references (repo.sqlite -> metadata.sqlite), documented automated restore-backup command, and corrected CLI flags (--file, --approve, folders revalidate).
- tests/integration/orbit_recovery_test.go: added 6 integration tests verifying candidate reproductions, repairs, and live mutation fencing.
Requirements / invariants:
- S11 (crash recovery / WAL replay)
- S20 (backup and restore integrity)
- U14 (recovery and diagnostics honesty)
- Invariants I07 (membership verification), I08 (causal counter monotonicity across identity resets), I19 (safe recovery preserves user data without silent peer loss), I20 (crash consistency and transactional operations).
Actual commands and results:
- make build: passed, compiled bin/filesync.
- make fmt-check: passed, zero formatting diffs.
- make vet: passed, clean.
- go test -count=1 -v ./internal/config ./internal/replication ./internal/control ./tests/integration -run 'TestOrbitRecovery|TestP13|TestP15': passed (24 test groups passed, 0 failed).
- make test-race: passed (go test -race ./... across all packages, 0 race conditions).
- Unexecuted: VM fault injection harness (scripts/validation/abrupt_reset.py) and VPS workload tests remain unexecuted as disposable marked local fixtures were used per test safety rules.
Evidence revision, packages and paths:
- tests/integration/orbit_recovery_test.go (TestOrbitRecoveryCandidate1ConfigWriteAndRestart, TestOrbitRecoveryCandidate2KeyRotation, TestOrbitRecoveryCandidate3TransactionalAuthorAlignment, TestOrbitRecoveryCandidate4InspectionDoesNotMutateState, TestOrbitRecoveryCandidate5RunbookWorkflow, TestOrbitRecoveryLiveResetFenced)
- internal/config/config_test.go (TestSaveAndLoadRoundtrip)
- internal/replication/identity_test.go (TestLoadOrCreateIdentityReusesExistingKey, TestRotateIdentityGeneratesFreshKeyAndChangesPin)
- internal/workspace/workspace_test.go (TestReclaimRecoveryCopies)
- internal/control/maintenance_test.go (TestMaintenanceOperations)
Remaining limitations:
- G01–G05 design gates are formally closed in O01; full product implementation spans packets O02–O12.
- Interrupted-transition fencing currently refuses live identity mutation with an explicit stopped error; full crash recovery state machine across partial transfers is addressed in G04/O02.
- P17 owner pilot/explanation remains open for the broader release.
Next eligible work:
- Packet O01: Close the revamp's design gates (G01–G05) in docs/design-gates.md, tests/designgates, and owning specifications.
```

### O01 — Close the revamp's design gates

```text
Packet / state: O01 Design gates / complete
Prerequisites and owning specs checked:
- docs/orbit-architecture.md: Design gates G01–G05 requirements and boundaries.
- docs/protocol.md: Enrollment capability, joining proof, linear membership rollout, competing administration fork detection.
- docs/persistence.md: File mutation journals, move overwrite/source races, read leases, crash consistency.
- docs/operations.md: Singleton state locking, one-use bootstrap handoff, product settings separation.
- docs/verification.md: Invariants I21, I23, I24, I25, I26, I20 oracles and test surfaces.
Gate outcomes and affected contract versions:
- G01 Local launch: Singleton state locking via state.Acquire (.agent.lock); 60s TTL one-use bootstrap token passed via URL fragment and consumed via loopback POST /api/v1/auth/bootstrap; strict Host/Origin checking against DNS rebinding; UI logout invalidates session cookie without stopping background daemon sync (Invariant I21).
- G02 Enrollment: Short-lived workspace invitations stored as SHA-256 verifier digest; token grants only capability to submit JoinRequest to bounded rate-limited endpoint, zero content/chunk access (Invariant I23); joining device proves Ed25519 key possession; explicit owner approval required to mint Revision(N+1); sequential linear rollout with explicit fork detection (ErrMembershipFork) on competing partitioned approvals (Invariant I24); retired device IDs cannot be reactivated.
- G03 File actions: Durable SQLite journals (Planned -> Staged -> Installed -> SourceVerified -> Completed); move overwrite displaces target to .filesync-internal/recovery/<opID>; concurrent source modification race retains both files and avoids data loss (Invariant I26); directory subtree token invalidation upon new children; active ReadLease protects version chunk digests from GC unlinking (Invariant I25).
- G04 Recovery: Stopped exclusive lock requirement (live mutation fenced); atomic identity rotation, SQLite transactional folders update, and config.Save; startup consistency check detects crashed partial transitions (ErrIncompleteRecovery, Invariant I20); missing CAS chunks after backup restore diagnose ContentUnavailable without synthetic substitution (Invariant I18).
- G05 Compatibility: Product settings (device label, workspace display names, theme) stored in separate settings.json / DB, preserving strict config.json format_version 1; protocol capability negotiation for mixed Orbit/legacy nodes; legacy state directory adoption without data loss and refusal of newer unsupported schemas (user_version > 5).
Changed files:
- docs/orbit-design-gates.md: Created comprehensive outcome document freezing G01–G05 decisions, rules, rejected alternatives, and invariants.
- docs/design-gates.md: Linked Orbit design gate outcomes.
- docs/protocol.md: Updated with frozen G02 enrollment capability, joining key proof, linear membership rollout, competing administration fork detection, and G05 capability negotiation.
- docs/persistence.md: Updated with frozen G03 file mutation journals, move overwrite/source races, read leases, and G04 crash consistency.
- docs/operations.md: Updated with frozen G01 singleton locking, one-use bootstrap handoff, and G05 product settings separation.
- docs/verification.md: Updated Orbit revamp verification table and oracles for Invariants I21–I28.
- model/membership.go: Independent test oracle for linear membership chains, canonical digests, signatures, and fork detection under competing administration.
- model/membership_test.go: Test suite verifying sequential rollout, competing forks, and retirement immutability.
- tests/designgates/orbit_g01_launch_test.go: 3 executable tests for G01 singleton locking, one-use bootstrap handoff, and UI logout vs background sync.
- tests/designgates/orbit_g02_enrollment_test.go: 5 executable tests for G02 invitation digest storage, capability gating, DoS limits, key possession proof, owner approval, sequential rollout, competing forks, and retirement immutability.
- tests/designgates/orbit_g03_fileactions_test.go: 4 executable tests for G03 move overwrite recovery, concurrent source modification retention, subtree invalidation, and read leases.
- tests/designgates/orbit_g04_recovery_test.go: 4 executable tests for G04 stopped recovery, atomic transition, interrupted transition detection, and missing CAS payloads.
- tests/designgates/orbit_g05_compat_test.go: 3 executable tests for G05 product settings separation, protocol capability negotiation, and schema rollback limits.
- docs/implementation/orbit-status.md: Updated tracker, marked O01 complete, set O02 eligible.
Requirements / invariants:
- U05, U08, U10, U13, U14.
- Invariants I08, I18, I19, I20, I21, I23, I24, I25, I26.
Actual commands and results:
- make build: passed, compiled bin/filesync.
- make fmt-check: passed, zero formatting diffs.
- make vet: passed, clean.
- make test-model: passed (14 test groups passed, 0.111s).
- make test-faults: passed (all fault and invariant suites passed, 0.810s).
- go test -count=1 -v ./tests/designgates -run TestOrbit: passed (19 tests executed across G01–G05, 19 passed, 0 failed, 0.508s).
- make test-race: passed (go test -race ./... across all packages, 0 race conditions, 86.4s).
- python3 link validator: 86 markdown files checked, 0 broken links.
Evidence revision, packages and paths:
- model/membership.go, model/membership_test.go (TestOrbitMembershipSequentialRollout, TestOrbitMembershipCompetingAdministrationForks, TestOrbitMembershipUnauthorizedOrForgedSignatures)
- tests/designgates/orbit_g01_launch_test.go (TestOrbitG01SingletonExclusiveOwnership, TestOrbitG01OneUseBootstrapHandoff, TestOrbitG01SessionAndLogoutKeepsSyncActive)
- tests/designgates/orbit_g02_enrollment_test.go (TestOrbitG02InvitationCreationAndDigestStorage, TestOrbitG02CapabilityGatingAndDosLimits, TestOrbitG02KeyPossessionAndOwnerApproval, TestOrbitG02OfflineRolloutAndCompetingFork, TestOrbitG02RetiredDeviceCannotRejoin)
- tests/designgates/orbit_g03_fileactions_test.go (TestOrbitG03MoveOverwriteAndRecoveryPreservation, TestOrbitG03MoveSourceConcurrentModificationRetainsBoth, TestOrbitG03DirectorySubtreeInvalidation, TestOrbitG03ReadLeaseProtectsChunksFromGC)
- tests/designgates/orbit_g04_recovery_test.go (TestOrbitG04LiveIdentityResetFenced, TestOrbitG04AtomicTransitionAndRollbackSafety, TestOrbitG04InterruptedTransitionDetection, TestOrbitG04MissingPayloadsAfterMetadataRestore)
- tests/designgates/orbit_g05_compat_test.go (TestOrbitG05ProductSettingsSeparation, TestOrbitG05ProtocolCapabilityNegotiation, TestOrbitG05LegacyStateAdoptionAndRollbackLimits)
- docs/orbit-design-gates.md
Remaining limitations:
- Implementation of product records and control operations is pending in O02.
- Local launcher CLI and setup operations are pending in O03.
- Full real-browser integration tests with puppeteer-core are scheduled for O04.
Next eligible work:
- Packet O02: Product records and shared control contracts (docs/implementation/orbit-foundations.md#o02--product-records-and-shared-control-contracts).
```

### O02 — Product records and shared control contracts

```text
Packet / state: O02 Records/contracts / complete
Prerequisites and owning specs checked:
- docs/orbit-architecture.md: Product records, shared control contracts, decimal-string counters, generation tokens.
- docs/operations.md: Product display preferences (settings.json) separate from config.json, validated peer endpoint configuration (peers.json), startup consistency checks.
- docs/persistence.md: Schema migration to version 11 (folders.display_name, invitations, enrollment_requests, setup_state, operation_progress, read_leases, read_lease_chunks), legacy adoption, downgrade protection (PRAGMA user_version > 11), read lease pins in content_pins.
- docs/verification.md: Invariants I08, I13, I16, I19, I20, I22, I25, I26.
Gate outcomes and affected contract versions:
- config.json strictly preserved at format_version: 1 (format_version, device_id, created_at with DisallowUnknownFields).
- Product display preferences (device_label, default_workspace, workspace_names, theme, dark_theme) persisted in separate settings.json with atomic writes, fallback on corrupt data, and owner-local aliasing.
- Peer endpoints persisted in peers.json with strict validation (HTTPS origin, canonical 32-byte hex key, no duplicates, max 64).
- SQLite metadata upgraded from schema version 10 to version 11; legacy schemas 1–10 adopted cleanly; future schemas (> 11) rejected with ErrIncompatibleSchema.
- Read leases (read_leases, read_lease_chunks) integrate with content_pins (owner_kind = 'read_lease') protecting unlinked CAS chunks from GC during downloads and staged actions (G03 / Invariant I25).
- Interrupted identity recovery detection implemented via VerifyRecoveryConsistency / CheckRecoveryConsistency and wired into daemon startup (ErrIncompleteRecovery, Invariant I20).
- Live mutating identity reset / restore endpoints explicitly fenced on running daemon with code OPERATION_BLOCKED while exposing read-only recovery status.
- Control API contracts typed with decimal-string counters, generation tokens, and shared request/response models.
Changed files:
- internal/config/settings.go: ProductSettings model and atomic storage.
- internal/config/settings_test.go: 5 unit tests for settings roundtrip, validation, fallback, identity independence, and duplicate labels.
- internal/config/peers.go: Peer endpoint validation, atomic storage, and mutating helpers.
- internal/config/peers_test.go: Unit tests for peer endpoint operations.
- internal/repository/repository.go: Upgraded CurrentSchema = 11 and added migration 11 creating Orbit tables.
- internal/repository/product_records.go: Repository methods for folders display name, device display name, invitations, enrollment requests, setup state, operation progress, and read leases.
- internal/repository/product_records_test.go: Unit tests for schema 11 migration, record CRUD, and legacy schema rollback limits.
- internal/control/consistency.go: VerifyRecoveryConsistency and CheckRecoveryConsistency checking config, cert CommonName, and author alignment.
- internal/app/app.go: Wired consistency check into ServeWithOptions startup.
- internal/control/types.go: Typed requests/responses for Settings, Endpoints, Setup, Invitations, Enrollment, Read Leases, Progress, and DecimalString helper.
- internal/control/orbit_control.go: Controller methods for product records, leases, invitations, enrollment, and recovery status.
- internal/control/server.go: HTTP endpoints mounted under /api/v1/... with live mutation fencing (OPERATION_BLOCKED).
- internal/control/orbit_control_test.go: 6 unit/contract test suites for control operations and recovery fencing.
- tests/integration/orbit_settings_test.go: Integration tests for settings persistence across restart, endpoints editing, and folder authorization isolation.
- tests/integration/orbit_migration_test.go: Integration tests for legacy adoption, schema rollback refusal, and interrupted recovery detection.
- docs/implementation/orbit-status.md: Updated tracker, completion record for O02, advanced next action to O03.
Requirements / invariants:
- U01 (product settings and preferences), U04 (setup state tracking), U09 (peer endpoint management), U13 (consistent error reporting), U14 (recovery and diagnostics honesty).
- Invariants I08 (author/counter consistency), I13 (bounded resources), I16 (idempotent operations), I19 (UI/CLI parity), I20 (crash consistency / schema compatibility), I22 (secure identity storage), I25 (read leases protect CAS chunks), I26 (safe mutation tracking).
Actual commands and results:
- make build: passed, compiled bin/filesync.
- make fmt-check: passed, zero formatting diffs.
- make vet: passed, clean.
- make test: passed, all internal unit tests passed.
- go test -count=1 -v ./internal/config ./internal/repository ./internal/control ./tests/integration -run 'TestOrbitSettings|TestOrbitMigration|TestOrbitRecovery': passed (16/16 test groups passed, 1.650s).
- make test-model: passed (14/14 test groups passed, 0.111s).
- make test-faults: passed (all P16 fault boundaries and invariant tests passed, 0.870s).
- make test-race: passed (go test -race ./... across all packages, 0 race conditions detected, 62.3s).
- make check: passed (fmt-check, vet, test, test-integration, test-model, test-faults, test-harness, build, build-arm64, package all passed cleanly).
- python3 link validator: 89 markdown files checked, 362 local links validated, 0 broken links.
Evidence revision, packages and paths:
- internal/config/settings_test.go (TestOrbitSettings_LoadAndSaveRoundtrip, TestOrbitSettings_InvalidRejectedWithoutPartialConfig, TestOrbitSettings_CorruptFallback, TestOrbitSettings_LabelChangeDoesNotRotateIdentity, TestOrbitSettings_DuplicateLabelsAllowed)
- internal/config/peers_test.go (TestSetPeerEndpoint_Validation, TestRemovePeerEndpoint)
- internal/repository/product_records_test.go (TestOrbitMigration_Schema11MigrationAndOperations, TestOrbitMigration_LegacyAdoptionAndRollbackLimit)
- internal/control/orbit_control_test.go (TestOrbitSettings_Control_SettingsOperations, TestOrbitSettings_Control_PeerEndpointsOperations, TestOrbitSettings_Control_SetupOperations, TestOrbitSettings_Control_InvitationsAndEnrollment, TestOrbitSettings_Control_ReadLeasesAndGCProtection, TestOrbitSettings_Control_HTTP_EndpointsAndRecoverySafety)
- tests/integration/orbit_settings_test.go (TestOrbitSettings_EndToEnd_SettingsAndRestart, TestOrbitSettings_EndToEnd_EndpointsEditing, TestOrbitSettings_EndToEnd_FolderAuthorizationIsolated)
- tests/integration/orbit_migration_test.go (TestOrbitMigration_EndToEnd_LegacyAdoption, TestOrbitMigration_EndToEnd_NewerSchemaRefused, TestOrbitMigration_EndToEnd_InterruptedRecoveryDetected)
- tests/integration/orbit_recovery_test.go (TestOrbitRecoveryCandidate1ConfigWriteAndRestart, TestOrbitRecoveryCandidate2KeyRotation, TestOrbitRecoveryCandidate3TransactionalAuthorAlignment, TestOrbitRecoveryCandidate4InspectionDoesNotMutateState, TestOrbitRecoveryCandidate5RunbookWorkflow, TestOrbitRecoveryLiveResetFenced)
Remaining limitations:
- Launcher CLI commands (filesync orbit, filesync setup, filesync service) and browser launcher handoff are scheduled for O03.
- Frontend web UI shell and first-use flows are scheduled for O04.
- P17 owner pilot/explanation remains open for the broader release.
Next eligible work:
- Packet O03: Local launcher, service and setup operations (docs/implementation/orbit-foundations.md#o03--local-launcher-service-and-setup-operations).
```

### O03 — Local launcher, service and setup operations

```text
Packet / state: O03 Launcher/setup / complete
Prerequisites and owning specs checked:
- docs/orbit-architecture.md: Local launcher, single-use bootstrap handoff, systemd user service, setup phase state machine.
- docs/operations.md: State discovery, singleton daemon locking, systemd user service management, lingering configuration.
- docs/persistence.md: Setup operations, root directory inspection, directory picker, Invariant I22 preexisting file capture without deletion.
- docs/verification.md: Invariants I08, I11, I13, I19, I20, I21, I22.
Gate outcomes and affected contract versions:
- Added orbit binary alias and filesync orbit CLI command while fully preserving the legacy CLI.
- Safe state discovery: inspects explicit --state, default XDG state directory, and legacy ~/.filesync. When multiple distinct initialized states exist, refuses silent choice with ErrMultipleStatesFound.
- Invalid state protection: validates directory mode 0700 and owner UID, loads config.json, executes VerifyRecoveryConsistency, and checks SQLite PRAGMA user_version <= CurrentSchema. Rejects corrupted or incompatible future states without deleting or overwriting data (Invariant I20).
- Auto-initialization on launch: added AllowInitialize flag to app.ServeOptions / --allow-init; initializes clean state with 0700 permissions, TLS keys, and schema 11 database on fresh launch.
- Singleton daemon reuse and bootstrap handoff: launcher detects existing daemon via .agent.lock, requests 1-use bootstrap token from POST /api/v1/auth/bootstrap-token, and opens browser to http://<addr>/#bootstrap=<token> (or outputs URL if --no-browser/headless).
- UI session separation: user logout via POST /api/v1/auth/logout invalidates session cookie while background sync daemon remains actively running and holding lock (Invariant I21).
- Setup operations: implemented InspectSetup, PreviewCreateRoot, PreviewJoinRoot, StartSetup, ResumeSetup, and GetSetupStatus. Enforces root validation: rejects empty, state directory, system paths (/etc, /var, /usr, /proc, /dev, etc.), and overlapping/nested sync roots.
- Preexisting file preservation (Invariant I22): StartSetup and ResumeSetup adopt preexisting files into immutable version history via ws.Scan without generating deletion tombstones. Interrupted setup records phase in setup_state and operation_progress and supports safe resumption.
- Directory picker & desktop helper: BrowseDirectories provides authenticated paginated directory exploration explaining denied directories (/proc, /sys, /dev, state dir). OpenLocalFolder safely executes desktop file manager via direct argument exec (no shell injection) within registered sync roots, cleanly returning DESKTOP_HELPER_UNAVAILABLE in headless environments.
- Systemd user service lifecycle: CheckServiceStatus reports 7 distinct boolean indicators: SystemdAvailable, UnitInstalled, EnabledOnLogin, CurrentlyRunning, RootVerified, CaptureSuccessful, LingeringEnabled, along with LingeringInstruction and ManualCommand fallback. Service actions (enable, start, stop) fail gracefully with SYSTEMD_UNAVAILABLE and manual command when user session D-Bus is unreachable. Host lingering is documented as an optional administrator command (loginctl enable-linger <user>), never executed silently.
Changed files:
- cmd/filesync/main.go: added handleOrbit dispatcher (launch, status, setup, service, open, picker, help); added --allow-init to serve.
- Makefile: added creation of bin/orbit symlink on make build and clean removal.
- internal/app/app.go: added AllowInitialize option to ServeOptions to safely initialize clean uninitialized state directories.
- internal/control/service.go: CheckServiceStatus, EnableService, StartService, StopService, InstallUserUnit.
- internal/control/types.go: typed models for Setup (PreviewJoinRoot, ResumeSetup), DirectoryPicker, OpenFolder, ServiceStatus, and ServiceAction.
- internal/control/orbit_control.go: InspectSetup, PreviewCreateRoot, PreviewJoinRoot, StartSetup, ResumeSetup, GetSetupStatus, BrowseDirectories, OpenLocalFolder, ServiceStatus, ServiceAction.
- internal/control/server.go: registered HTTP routes for setup preview-join-root, resume, directories, open-folder, service/status, and service/action.
- internal/repository/product_records.go: added HasAnyCapturedVersions query method.
- internal/launcher/discovery.go: DiscoverState and ValidateExistingState with read-only SQLite schema inspection.
- internal/launcher/launcher.go: Launch workflow, daemon lock detection, background starter, bootstrap handoff, and browser opening.
- internal/control/orbit_setup_test.go: unit and contract tests for InspectSetup, PreviewCreate/JoinRoot, Start/ResumeSetup, Preexisting file preservation, DirectoryPicker, OpenLocalFolder, and InterruptedSetupResume.
- tests/integration/orbit_launch_test.go: integration tests for first launch uninitialized, repeated launch reuse, multi-state refusal, invalid state rejection, token exchange and logout session separation, support export sanitization, and CLI binary alias.
- tests/integration/orbit_service_test.go: integration tests for service status reporting, service action and fallback, absent systemd handling, lingering documentation, user unit generation, and CLI service status.
- docs/implementation/orbit-status.md: updated status table, next action, and completion record for O03.
Requirements / invariants:
- U04 (workspace setup, default ~/Orbit, preview preexisting content, safe resume)
- U08 (embedded browser interface, desktop launcher, headless parity)
- U12 (startup on login via systemd user service, distinct status indicators, graceful fallback, documented lingering)
- U16 (desktop launcher/UI parity with CLI control operations)
- Invariants I08 (atomic transitions, identity preservation), I11 (unavailable roots/incomplete scans never delete files), I13 (bounded resources), I19 (CLI & UI use identical control operations), I20 (incompatible schema preserves recoverable state), I21 (launch/bootstrap session security, UI logout keeps sync daemon running), I22 (setup/restart preserves preexisting files, no deletions fabricated on adoption).
Actual commands and results:
- make build: passed, compiled bin/filesync and created bin/orbit symlink.
- make fmt-check: passed, zero formatting violations across all Go files.
- make vet: passed, clean.
- go test -count=1 -v ./internal/control ./tests/integration -run 'TestOrbitLaunch|TestOrbitSetup|TestOrbitService': passed (19/19 test groups passed, 0.566s).
- make test-race: passed (go test -race ./... across all packages, 0 race conditions detected, 63.8s).
- make check: passed (fmt-check, vet, test, test-integration, test-model, test-faults, test-harness, build, build-arm64, package all passed cleanly, release archives and packages generated).
- python3 link validator: 89 markdown files checked, 362 local links validated, 0 broken links.
Evidence revision, packages and paths:
- internal/control/orbit_setup_test.go (TestOrbitSetup_InspectSetup, TestOrbitSetup_PreviewCreateAndJoinRoot, TestOrbitSetup_StartAndResumeSetup_PreservesPreexistingFiles, TestOrbitSetup_DirectoryPicker, TestOrbitSetup_OpenLocalFolder, TestOrbitSetup_InterruptedSetupResume)
- tests/integration/orbit_launch_test.go (TestOrbitLaunch_FirstLaunchUninitialized, TestOrbitLaunch_RepeatedLaunchReusesDaemon, TestOrbitLaunch_MultiStateDetectionRefusesSilentChoice, TestOrbitLaunch_InvalidOldStateRefusesOverwrite, TestOrbitLaunch_TokenExchangeAndSessionSeparation, TestOrbitLaunch_BootstrapSecretNotLeakedInSupportExport, TestOrbitLaunch_CLI_BinaryAlias)
- tests/integration/orbit_service_test.go (TestOrbitService_StatusReporting, TestOrbitService_ActionAndFallback_HTTP, TestOrbitService_AbsentSystemdGracefulFallback, TestOrbitService_LingeringDocumentedNotSilent, TestOrbitService_InstallUserUnit, TestOrbitService_CLI_ServiceStatus)
Remaining limitations:
- Frontend HTML/CSS/JS shell, onboarding wizard, setup creation and root adoption UI are scheduled for O04.
- Peer enrollment and invite link exchange flows are scheduled for O05/O06.
- P17 owner pilot/explanation remains open for the broader release.
Next eligible work:
- Packet O04: Orbit shell and complete first-device UI (docs/implementation/orbit-foundations.md#o04--orbit-shell-and-complete-first-device-ui).
```

### O04 — Orbit shell and complete first-device UI

```text
Packet / state: O04 Shell/first-use UI / complete
Prerequisites and owning specs checked:
- docs/orbit-product.md: Product vision, monochrome baseline token set, primary destinations (Files, Needs attention, Devices, Deleted files, Settings), onboarding workflow.
- docs/orbit-architecture.md: Standalone web UI shell, single-use bootstrap URL fragment exchange and stripping, 4-step setup progress, 7-point service status, zero Node runtime dependency.
- docs/implementation/orbit-foundations.md#o04--orbit-shell-and-complete-first-device-ui: Acceptance criteria, planned checks, puppeteer-core test runner requirements, screenshot exports.
- docs/verification.md: Invariants I19, I21, I22, I27.
Gate outcomes and affected contract versions:
- Built modern monochrome web UI shell using React 19, TypeScript, and Vite 8, embedded directly into Go binary via //go:embed all:dist with zero external runtime Node process.
- Designed complete token system: --canvas (#101010), --surface (#181818), --surface-raised (#222222), --text-primary (#F2F2F2), --text-secondary (#B6B6B6), --border (#393939), with 2px high-contrast focus rings and accessible icon-text pairings.
- Single-use bootstrap token exchange: detects #bootstrap=<token> URL fragment, exchanges for session cookie via POST /api/v1/auth/bootstrap, and immediately strips fragment from browser history via window.history.replaceState without leaking in exports or address bar (Invariant I21).
- Onboarding wizard: provides Create workspace vs Join workspace cards. The Join entry identifies upcoming cross-device pairing without faking success. Live root preview debounces and calls POST /api/v1/orbit/setup/preview/create, rejecting system directories (/etc, /var, etc.) and explaining preexisting file adoption without data loss or fabricated tombstones (Invariant I22).
- Observable setup progress: 4-step visual tracker (Folder registration, Content indexing, Background service, Finalizing) with live status updates.
- Five primary destinations: Files (directory browsing, status badges, version drawer, restore), Needs attention (conflicts, paused roots, doctor diagnostics), Devices (peer replicas, keys, retirement), Deleted files (tombstone browser with fresh-version restore), Settings (device label, sync roots, 7-point systemd user service indicators, storage GC, support export, logout).
- Directory picker modal: paginated local Linux directory browser with keyboard Escape dismissal and focus restore.
- Responsive layout: fluid header, breadcrumbs, search filter, desktop sidebar, and collapsible drawer navigation for narrow/mobile viewports (375x667).
- Backend addition: mounted GET /api/v1/files/deleted endpoint exposing tombstoned projections for the Deleted files view.
- Deterministic browser test runner: scripts/orbit_ui_test.mjs using puppeteer-core against live marked daemon fixtures. Asserts all 10 setup and shell workflow steps and exports 9 full-resolution screenshots.
Changed files:
- web/src/types.ts: added TypeScript interfaces for Orbit setup, service, and settings contracts.
- web/src/api.ts: implemented API client methods for Orbit endpoints.
- web/src/styles.css: complete monochrome baseline design system, status tokens, focus rings, responsive mobile classes.
- web/src/views/BootstrapView.tsx: URL fragment token consumption and immediate history replacement.
- web/src/views/SetupWizard.tsx: Create vs Join flow, live directory preview, adoption review, systemd startup option, 4-step progress bar.
- web/src/components/DirectoryPickerModal.tsx: local directory picker modal with keyboard Escape support.
- web/src/components/OrbitSidebar.tsx: shell sidebar with 5 primary destinations and attention indicators.
- web/src/components/OrbitTopbar.tsx: shell topbar with breadcrumbs, search filter, and Open Local Folder trigger.
- web/src/views/FilesView.tsx: file manager table, status indicators, directory navigation, version drawer, restore flow.
- web/src/views/NeedsAttentionView.tsx: centralized view for conflicts, paused roots, and doctor issues.
- web/src/views/DevicesView.tsx: node replica list, key fingerprints, and pairing guidance.
- web/src/views/DeletedFilesView.tsx: historical tombstone browser and causal restore.
- web/src/views/SettingsView.tsx: device label, sync roots, 7-point systemd indicators, storage GC, diagnostic export, logout.
- web/src/App.tsx: top-level router and 4s visibility-aware polling state machine.
- internal/control/control.go: added DeletedFiles method querying tombstoned projections.
- internal/control/server.go: mounted GET /api/v1/files/deleted route.
- internal/control/orbit_control_test.go: added TestOrbitShell_DeletedFiles_HTTP test.
- scripts/orbit_ui_test.mjs: deterministic automated browser test runner.
- docs/evidence/orbit-o04/summary.md: comprehensive summary and screenshot index.
- docs/evidence/orbit-o04/screenshots/: 9 browser verification screenshots.
- docs/implementation/orbit-status.md: updated tracker and completed O04 record.
Requirements / invariants:
- U01 (single-device onboarding and status)
- U04 (monochrome baseline design system and fluid layout)
- U07 (responsive shell with 5 primary destinations)
- U08 (preexisting file adoption review)
- U15 (honest qualified progress and storage states)
- U16 (accessible interactive controls and keyboard escape)
- Invariants I19 (CLI & UI parity), I21 (token & session lifecycle), I22 (safe root selection & file preservation), I27 (honest recovery & service states).
Actual commands and results:
- cd web && npm ci && npm run build && cd ..: passed, Vite built static assets with zero errors.
- make build: passed, compiled bin/filesync with embedded web/dist assets and bin/orbit symlink.
- make fmt-check: passed, zero formatting violations across all Go files.
- make vet: passed, clean.
- node scripts/orbit_ui_test.mjs --scenario setup: passed, all 10 assertion steps verified in Chromium; 9 screenshots captured.
- go test -count=1 -v ./internal/control ./tests/integration -run 'TestOrbit': passed (20/20 test groups passed).
- make test-race: passed (go test -race ./... across all packages, 0 race conditions detected, 55.4s).
- make check: passed (fmt-check, vet, test, test-integration, test-model, test-faults, test-harness, build, build-arm64, package all passed cleanly, release archives and packages generated).
- python3 link validator: 90 markdown files checked, 364 local links validated, 0 broken links.
Evidence revision, packages and paths:
- web/dist/ (embedded production assets)
- scripts/orbit_ui_test.mjs (deterministic browser runner)
- docs/evidence/orbit-o04/summary.md (verification summary)
- docs/evidence/orbit-o04/screenshots/ (screenshot-01 through screenshot-09)
- internal/control/orbit_control_test.go (TestOrbitShell_DeletedFiles_HTTP)
Remaining limitations:
- Cross-device pairing enrollment flows (workspace invitations, QR/string tokens, linear membership chain rollout) are scheduled for O05/O06.
- Desktop system notification daemon integration is scheduled for O07.
- P17 owner pilot/explanation remains open for the broader release.
Next eligible work:
- Packet O05: Enrollment and linear membership rollout (docs/implementation/orbit-devices-files.md#o05--enrollment-and-linear-membership-rollout).
```

### O05 — Enrollment, endpoints and durable membership propagation

```text
Packet / state: O05 Enrollment/rollout / complete
Prerequisites and owning specs checked:
- O01, O02 complete.
- docs/orbit-architecture.md: Gate G02 contracts, capability-gated enrollment, linear membership chain, competing fork detection.
- docs/protocol.md: membership revision structure, wire format, mutual TLS client authentication.
- docs/persistence.md: invitations table with SHA-256 verifiers, membership_revisions, membership_entries, devices, peers.json.
- docs/operations.md: peer endpoints, rate limits, payload bounding.
- docs/verification.md: invariants I09, I13, I15, I16, I20, I23, I24.
Gate outcomes and affected contract versions:
- Gate G02 contract implemented: workspace invitations with SHA-256 verifiers, cryptographic join proofs with Ed25519 signatures, sequential linear membership rollout (Revision N+1), competing fork detection (ErrMembershipFork), permanent retirement immutability (ErrRetiredMemberRevival), and directional peer endpoint configuration.
- Wire protocol /peer/v1/membership/get exposed for gated offline peer catch-up.
- Bounded join request payloads to 16 KiB (HTTP 413) and rate-limited join attempts to 5 req/min per IP (HTTP 429).
Changed files:
- internal/config/peers.go: added SetPeerEndpoint, RemovePeerEndpoint, and peer endpoint validation.
- internal/control/errors.go: added ErrMembershipFork, ErrRetiredMemberRevival, ErrInvalidSignature, ErrRateLimitExceeded, ErrPayloadTooLarge and corresponding ControlError constructors.
- internal/control/types.go: added Certificate field to ApproveEnrollmentRequest; added DetectForkRequest/Result and ReconcileMembershipRequest/Result.
- internal/control/orbit_control.go: implemented SubmitEnrollmentRequest, ApproveEnrollmentRequest (with Rev 1 initialization and Rev N+1 minting), DetectMembershipFork, ReconcileMembership, CatchUpMembership.
- internal/control/server.go: added 16 KiB request body bounding (MaxBytesReader -> HTTP 413), IP rate limiting (HTTP 429), and mounted membership control endpoints (/api/v1/membership/fork/detect, /api/v1/membership/reconcile).
- internal/replication/wire.go: added MembershipGetRequest and MembershipGetResponse wire types.
- internal/replication/transfer.go: added MembershipGet to PeerClient interface.
- internal/replication/client.go: implemented MembershipGet on *Client.
- internal/replication/server.go: added route and handler for POST /peer/v1/membership/get, enforcing client TLS authentication, membership verification, and retired device blocking.
- internal/repository/peers.go: implemented DetectMembershipFork, IsDeviceRetired, IsActiveOrHistoricalMember, DeviceKeyPin, and strict retirement revival fencing in ApproveMembership.
- internal/repository/membership_test.go: added TestOrbitMembership_RepositoryForksAndRevivals and TestOrbitEnrollment_RepositoryInvitations.
- internal/replication/server_test.go: added TestOrbitMembership_ReplicationGet.
- internal/control/orbit_control_test.go: added TestOrbitEndpoints_ControlUnit.
- tests/integration/orbit_enrollment_test.go: added TestOrbitEnrollment test suite (invitation lifecycle, verifiers, exclusions, 16 KiB bounds, rate limits, key proofs, approval, retirement rejection).
- tests/integration/orbit_membership_test.go: added TestOrbitMembership test suite (sequential rollout, competing administration fork, offline catch-up, restart persistence).
- tests/integration/orbit_endpoints_test.go: added TestOrbitEndpoints test suite (persistence/validation, two-peer direct pull sync, A-Hub-B forwarding sync, error categorization).
- docs/evidence/orbit-o05/summary.md: comprehensive summary and verification report.
- docs/implementation/orbit-status.md: updated tracker and completed O05 record.
Requirements / invariants:
- U03 (multi-device membership & linear progression)
- U05 (invitation lifecycle, verifiers & expiration)
- U06 (joining proofs & explicit owner approval)
- U09 (offline peer catch-up & partition fork reconciliation)
- U16 (accessible interactive controls & clear error taxonomy)
- Invariants I09 (folder authorization), I13 (resource limits: 16 KiB payloads, 5 req/min IP rate limits), I15 (membership retirement rejection), I16 (resolution idempotence), I20 (recoverable state), I23 (single-use & capability scoping), I24 (strict linear membership & competing fork detection).
Actual commands and results:
- go test -count=1 -v ./internal/replication ./internal/control ./internal/repository ./tests/integration -run 'TestOrbitEnrollment|TestOrbitMembership|TestOrbitEndpoints': passed (14/14 tests passed in 0.19s).
- make test-model: passed (all model oracles and formal membership chain tests passed).
- make test-race: passed (go test -race ./... across all packages, 0 race conditions detected).
- make check: passed (fmt-check, vet, test, test-integration, test-model, test-faults, test-harness, build, build-arm64, package all passed cleanly, release archives and packages generated).
- python3 link validator: 90 markdown files checked, 367 local links validated, 0 broken links.
Evidence revision, packages and paths:
- tests/integration/orbit_enrollment_test.go
- tests/integration/orbit_membership_test.go
- tests/integration/orbit_endpoints_test.go
- docs/evidence/orbit-o05/summary.md
Remaining limitations:
- Pairing and device management UI (web screens for Add device, Join flow, pending request inbox, explicit approve/decline) is scheduled for O06.
- Hierarchical browse, search, and pinned content downloads are scheduled for O07/O08.
- P17 owner pilot/explanation remains open for the broader release.
Next eligible work:
- Packet O06: Pairing and device-management UI (docs/implementation/orbit-devices-files.md#o06--pairing-and-device-management-ui).
```

### O06 — Pairing and device-management UI

```text
Packet / state: O06 Pairing/device UI / complete
Prerequisites and owning specs checked:
- O04, O05 complete.
- docs/orbit-product.md: user requirements U03, U05, U06, U09, U14, U16.
- docs/orbit-architecture.md: pairing contracts, reachability probe, retirement disclaimers.
- docs/implementation/orbit-devices-files.md#o06--pairing-and-device-management-ui.
- docs/runbooks/headless-pairing.md.
Gate outcomes and affected contract versions:
- Built full web UI for Add Device, Join Workspace, pending requests inbox, device detail inspection, and safe member retirement preview.
- Preserved preexisting content in joining roots without deletion (Invariant I22).
- Pure SVG zero-dependency QR code generator for invitation links.
- Implemented headless CLI parity via orbit (setup, join, invite, requests, devices) with automatic daemon lock fallback to loopback control API.
- Executed headless pairing runbook fixture and captured all 7 required screenshots.
Changed files:
- internal/protocol/membership.go: added explicit json tags to ActiveMember, RetiredMember, and Membership.
- internal/repository/product_records.go: added explicit json tags to EnrollmentRequestRecord.
- internal/control/orbit_control.go: implemented CheckJoinStatus daemon proxy method, ApproveEnrollmentRequest request ID lookup, and InsecureSkipVerify for loopback test client reachability probes.
- internal/control/server.go: mounted GET /api/v1/orbit/setup/join/status proxy route; supported 64-hex folder decoding in ApproveEnrollmentRequest.
- cmd/filesync/main.go: implemented orbit join, invite, requests, and devices subcommands with automatic daemon fallback (callOrbitDaemonAPI) when the background service is running.
- web/src/types.ts: added PeerEndpointsListResult and O06 UI contracts.
- web/src/api.ts: added getEnrollmentStatus proxy routing and getPeerEndpoints response unpacking.
- web/src/components/AddDeviceModal.tsx: created Add Device modal with TTL/uses settings, SVG QR code, and active invitation list with revocation.
- web/src/components/QRCodeDisplay.tsx: created zero-dependency pure SVG QR code generator component.
- web/src/components/DeviceDetailModal.tsx: created Device Detail modal with endpoint reachability probe and alias renaming.
- web/src/components/RetireDeviceModal.tsx: created Retire Device preview modal with known-history limits and remote erasure disclaimers.
- web/src/views/SetupWizard.tsx: created Join Workspace tab with link auto-parsing, reachability probe, and live approval status polling.
- web/src/views/DevicesView.tsx: integrated pending join requests inbox, alias editing, endpoint assignment, and approval/decline actions.
- tests/integration/orbit_pairing_test.go: created TestOrbitPairing test suite (5/5 PASS) covering join lifecycle, decline flow, device rename, and CLI offline/daemon parity.
- scripts/orbit_ui_test.mjs: added pairing and devices test scenarios with isolated browser contexts to prevent multi-daemon session cookie collisions.
- docs/runbooks/headless-pairing.md: documented headless pairing flow over SSH with executed fixture transcript.
- docs/evidence/orbit-o06/summary.md: comprehensive summary, acceptance criteria evidence, invariants verified, and screenshot catalog.
- docs/evidence/orbit-o06/screenshots/: captured screenshot-10 through screenshot-16.
- docs/implementation/orbit-status.md: updated packet tracker and O06 completion entry.
Requirements / invariants:
- U03 (multi-device membership & linear progression)
- U05 (invitation lifecycle, verifiers & expiration)
- U06 (joining proofs & explicit owner approval)
- U09 (offline peer catch-up & partition fork reconciliation)
- U14 (device discovery, management & retirement UI)
- U16 (accessible interactive controls & clear error taxonomy)
- Invariants I19 (headless parity), I22 (working-copy preservation), I23 (single-use & capability scoping), I24 (strict linear rollout & permanent retirement), I27 (failure visibility).
Actual commands and results:
- go test -count=1 -v ./tests/integration -run 'TestOrbitPairing': passed (5/5 tests passed in 1.17s).
- node scripts/orbit_ui_test.mjs --scenario pairing: passed (7/7 steps passed, screenshots 10-12 captured).
- node scripts/orbit_ui_test.mjs --scenario devices: passed (12/12 steps passed, screenshots 13-16 captured).
- make test-model: passed (all model oracles and formal membership chain tests passed).
- make test-race: passed (go test -race ./... across all packages, 0 race conditions detected).
- make check: passed (fmt-check, vet, test, test-integration, test-model, test-faults, test-harness, build, build-arm64, package all passed cleanly, release archives and packages generated).
- python3 link validator: 92 markdown files checked, 395 local links validated, 0 broken links.
Evidence revision, packages and paths:
- tests/integration/orbit_pairing_test.go
- scripts/orbit_ui_test.mjs
- docs/runbooks/headless-pairing.md
- docs/evidence/orbit-o06/summary.md
- docs/evidence/orbit-o06/screenshots/ (screenshot-10 through screenshot-16)
Remaining limitations:
- Hierarchical browse, search, and pinned content downloads are scheduled for O07/O08.
- Desktop system notification daemon integration is scheduled for O07.
- P17 owner pilot/explanation remains open for the broader release.
Next eligible work:
- Packet O07: Hierarchical browse, search and authenticated content (docs/implementation/orbit-devices-files.md#o07--hierarchical-browse-search-and-authenticated-content).
```






### O07 — Hierarchical browse, search and authenticated content

Packet/state: O07 / `complete` (2026-10-02).

Prerequisites/specifications checked: O02 complete; O01 G03 read-lifetime
outcome; product, architecture, scope/glossary, protocol, persistence,
operations and verification. No product scope/retention guarantee was changed.
The owning persistence/operations/architecture/G03 contracts now describe
stream pin lifetime, exact authenticated GETs, cursor policy and preview budgets.

Changed files: repository `browse.go`, `content_read.go`, schema-12 migration,
lease/GC coordination and scaffold mutation fencing; control `browse.go`,
`browse_http.go`, server routes, shared pinned export and scoped lease creation;
CLI `orbit_browse.go` and command dispatch/help; repository, CLI and HTTP
integration tests; owning specifications and O07 evidence/tracker.

Requirements/invariants: U07/U10/U11; I06/I09/I10/I13/I18/I19/I25.
Tests cover immediate/nested/empty/implicit directories, Unicode/unusual names,
tombstones, pending and structural conflicts; deterministic sort/cursor
rejection; history/Deleted files with conditional bytes; authorization,
active-format rejection, byte/pixel budgets; exact-version download/ranges,
corruption, GC intent/expiry/race/restart/cancellation and CLI parity.

Actual commands/results:
- `go test -count=1 -v ./cmd/filesync ./internal/repository ./internal/control ./tests/integration -run 'TestOrbitBrowse|TestOrbitSearch|TestOrbitContent|TestOrbitReadLease|TestOrbitSettings_Control_ReadLeases'`: 13 targeted tests passed.
- `make check`: passed, including format/vet/unit/integration/model/fault/harness,
  amd64/arm64 builds and tar/deb/rpm packages.
- `make test-race`: passed across all packages, no data-race report. Initial run
  failed the draft's fixed timing assertions; observations are retained, and
  measurements no longer assert a universal latency guarantee.
- Documentation links: 95 Markdown files, 433 local links, zero missing targets.
- Standalone compiled 10,000-file test: passed; root JSON 8,754 bytes, about
  0.59 MiB query allocations, whole-process maximum RSS 28,944 KiB. Timings are
  in the evidence. 64 MiB TLS HTTP download allocated about 1.7 MiB.

Evidence: [summary](../evidence/orbit-o07/summary.md),
[commands](../evidence/orbit-o07/commands.md),
[results](../evidence/orbit-o07/results.json),
[manifest](../evidence/orbit-o07/manifest.json), and logs linked there.

Limitations/unexecuted: O08 browser layout/keyboard/native preview workflows;
new native host O07 campaign and power-loss experiments. Single-frame PNG/JPEG
preview only; malformed image rendering can fail. Substring search scans local
folder paths; global generations conservatively stale other workspace pages.
Dirty-source/package hashes identify this worktree, not a clean release commit.
P17 owner pilot/use/explanation remains outstanding and personal roots were
untouched. Next work: O08 browser/details UI; O09 mutations also eligible.

### O08 — File browser, previews and details UI

Packet/state: O08 / `complete` (2026-10-02).

Prerequisites/specifications checked: O04, O07 complete; product, architecture,
scope/glossary, protocol, operations and verification.

Changed files: web components `FilesView.tsx`, `FileDetailsDrawer.tsx`,
`DeletedFilesView.tsx`, `OrbitTopbar.tsx`, `App.tsx`, `types.ts`, `api.ts`;
backend optimization `internal/history/history.go` (added `byPath` index reducing
causal operations across 10,000 files from O(N^2) to O(1)); test harness
`scripts/orbit_ui_test.mjs` (scenarios `browse` and `previews`); evidence files
and status tracker.

Requirements/invariants: U07/U10/U11/U15; I19/I25/I27.
Tests cover deep directory hierarchy navigation, navigable breadcrumbs trail,
history stack (`←`, `→`, `↑ Up`), backend search across 10,000 files with bounded
pagination (50/page) and Load More, column sorting (`name`, `size`, `mtime`),
list vs. grid presentation modes, distinct Invariant I27 status badges (pending,
conflict, blocked, saved), keyboard navigation and focus rings, narrow-window
responsive mobile layout (375x667), context-preserving side panel drawer, bounded
UTF-8 text preview, raster PNG preview, non-previewable fallback notice, exact
version native streaming download, historical version timeline with CAS availability,
and remote peer copy progress inspection.

Actual commands/results:
- `node scripts/orbit_ui_test.mjs --scenario browse`: passed (7/7 steps), captured screenshots 17–21.
- `node scripts/orbit_ui_test.mjs --scenario previews`: passed (6/6 steps), captured screenshots 22–24.
- `node scripts/orbit_ui_test.mjs --scenario all`: passed (5/5 scenarios: setup, pairing, devices, browse, previews).
- `make check`: passed all unit, integration, model, fault, package build, and validation checks.
- `make test-race`: passed all packages with zero race detector warnings.

Evidence: [summary](../evidence/orbit-o08/summary.md),
[commands](../evidence/orbit-o08/commands.md),
[results](../evidence/orbit-o08/results.json),
[manifest](../evidence/orbit-o08/manifest.json), and screenshots 17–24 linked there.

Limitations/unexecuted: O09 file operations (create, upload/import streaming, rename,
move, delete); O10 destructive mutation safeguards and in-drawer conflict resolution;
P17 owner pilot/use/explanation remains outstanding.
Next work: O09 recoverable import, create, rename, move, and delete.

### O09 — Recoverable import, create, rename, move and delete

Packet/state: O09 / `complete` (2026-10-02).

Prerequisites/specifications checked: O01, O02, O07 complete; product, architecture,
scope/glossary, protocol, persistence, operations and verification.

Changed files:
- `internal/repository/repository.go`: SQLite Schema 13 (`file_mutations`, `file_mutation_entries`), `CurrentSchema = 13`.
- `internal/repository/file_mutation.go`: durable mutation records, phase transitions, entry recording.
- `internal/repository/versions.go`: `SkipProjection` option and projection basis fallback in `CreateLocalVersion`.
- `internal/repository/file_mutation_test.go`: unit tests for journal recording, phase transitions, and foreign key cascades.
- `internal/workspace/workspace.go`: mutation fault hooks, error definitions, `ApplyWithOperationID`.
- `internal/workspace/file_mutations.go`: `ImportFile`, `CreateDirectory`, `Move`, `Delete`, `RecoverFileMutations`, and subtree tokens.
- `internal/workspace/file_mutation_test.go`: unit tests for workspace mutations.
- `internal/control/types.go`: file action request/response models.
- `internal/control/file_actions.go`: controller methods, 24h idempotency key caching, and error mapping.
- `internal/control/file_actions_http.go`: HTTP handlers for `/api/v1/files/*`.
- `internal/control/server.go`: mounted file action endpoints.
- `internal/control/file_actions_test.go`: unit tests for controller file actions.
- `cmd/filesync/orbit_files.go`: CLI subcommands (`mkdir`, `import`, `move`, `delete`).
- `cmd/filesync/main.go`: registered orbit file commands.
- `tests/integration/orbit_mutations_test.go`: integration test suite covering import, mkdir, move, delete, I26 source race, fault recovery, and stopped vs live daemon CLI parity.
- `docs/persistence.md`: documented Schema 13, journal recovery phases, and recovery scratch area.
- `docs/operations.md`: documented mutation HTTP endpoints, idempotency caching, and CLI commands.

Requirements/invariants: U10, S06–S14; I01, I03–I13, I16, I17, I19, I20, I26.
Tests cover fresh import, collision rejection (`DESTINATION_EXISTS`), overwrite displacement
to `.filesync-internal/recovery/<opID>`, Invariant I17 scan-after-action, 24h idempotency replay,
idempotency conflict rejection, nested directory creation with scaffolding, directory collision
rejection (`STRUCTURAL_CONFLICT`), single file move, Invariant I26 concurrent editor race
retaining both modified source and destination on disk (`SourceRetained: true`), directory subtree
move, directory subtree invalidation (`SUBTREE_INVALIDATED`), single file deletion, non-empty
directory rejection (`DIRECTORY_NOT_EMPTY`), recursive deletion, fault injection across `PLANNED`
and `INSTALLED` phases with automatic journal recovery, and full stopped CLI vs. live loopback
daemon CLI parity (Invariant I19).

Actual commands/results:
- `go test -count=1 -v ./internal/workspace ./internal/repository ./internal/control ./tests/integration -run 'TestOrbitImport|TestOrbitMkdir|TestOrbitMove|TestOrbitDelete|TestOrbitMutation'`: passed across all 4 packages (0.131s integration, 0.056s internal packages).
- `make test-model`: passed all model and reference set oracles (0.290s).
- `make test-faults`: passed all fault injection boundaries and invariants I01–I20 (0.972s).
- `make test-race`: passed all packages with 0 race detector warnings.
- `make check`: passed all unit, integration, validation, cross-compilation, and release packaging checks.

Evidence: [summary](../evidence/orbit-o09/summary.md),
[commands](../evidence/orbit-o09/commands.md),
[results](../evidence/orbit-o09/results.json),
[manifest](../evidence/orbit-o09/manifest.json).

Limitations/unexecuted: O10 user interface actions, dialogs, and in-drawer conflict resolution;
cross-workspace moves are intentionally outside packet scope; P17 owner pilot/use/explanation
remains outstanding.
Next work: O10 file actions, history, Deleted files and Needs attention UI.

### O10 — File actions, history, Deleted files and Needs attention

Packet/state: O10 / `complete` (2026-10-02).

Prerequisites/specifications checked: O08, O09 complete; product, architecture,
scope/glossary, protocol, persistence, operations and verification.

Changed files:
- `web/src/components/FileActionModals.tsx`: modal dialogs for folder creation, move/rename, deletion with recursive confirmation, and upload collision overwrite warnings.
- `web/src/views/FilesView.tsx`: toolbar buttons (`#btn-upload-file`, `#btn-new-folder`), selection action bar (`#btn-action-move`, `#btn-action-delete`, `#btn-action-download`), drag-and-drop dropzone overlay, and operation progress notices.
- `web/src/components/FileDetailsDrawer.tsx`: wired Move and Delete actions; integrated historical version timeline with CAS availability badges.
- `web/src/components/ConflictResolveModal.tsx`: conflict resolution modal with human review context notice (timestamps/devices are review context only, not winner rules), peer origin device attribution, tombstone badges on edit-delete conflicts, 3 resolution workflows (Select Winner, Keep Copies, Manual Merge), and stale view re-review handling.
- `web/src/views/NeedsAttentionView.tsx`: grouped triage cards for content conflicts, structural conflicts, paused folders with revalidation actions, failed durable tasks with retry/cancel, health warnings, and manual GC trigger (`#btn-run-gc`).
- `web/src/views/DeletedFilesView.tsx` & `web/src/components/RestoreModal.tsx`: struck-through deleted files browser with metadata, CAS availability badges, and conditional restore confirmation.
- `internal/repository/conflicts.go`: added `DisplayTime` to `ConflictHead` for human review context.
- `cmd/filesync/orbit_files.go` & `cmd/filesync/main.go`: added `orbit restore` and `orbit conflicts` CLI subcommands; folder auto-discovery; authenticated client connectivity via `control.addr` and `control.token`.
- `scripts/orbit_ui_test.mjs`: added deterministic browser test scenarios for `file-actions`, `history`, and `attention` capturing screenshots 25–33.

Requirements/invariants: U07, U10, U11, U15; I03, I16, I19, I26, I27.
Tests cover directory creation, file upload and overwrite collision warning with atomic replacement/displacement, move/rename into subdirectory, recursive delete with confirmation checkbox, error context retention on collision/stale view, historical versions timeline in side drawer with CAS availability badges, deleted files index with strike-through path and metadata, conditional restore authoring a fresh causal child version, Needs Attention triage grouping, candidate conflict heads with peer attribution, tombstone badge display on edit-delete conflict, preview across Select Winner, Keep Copies, and Manual Merge tabs, conflict resolution submission, manual storage GC trigger, and CLI subcommands parity.

Actual commands/results:
- `node scripts/orbit_ui_test.mjs --scenario file-actions`: passed (5/5 steps), captured screenshots 25–29.
- `node scripts/orbit_ui_test.mjs --scenario history`: passed (2/2 steps), captured screenshots 30–31.
- `node scripts/orbit_ui_test.mjs --scenario attention`: passed (2/2 steps), captured screenshots 32–33.
- `make test-model`: passed (0.258s).
- `make test-faults`: passed (1.042s).
- `make test-race`: passed all packages with 0 race detector warnings.
- `make check`: passed all unit, integration, validation, cross-compilation, and release packaging checks.

Evidence: [summary](../evidence/orbit-o10/summary.md),
[commands](../evidence/orbit-o10/commands.md),
[results](../evidence/orbit-o10/results.json),
[manifest](../evidence/orbit-o10/manifest.json), and screenshots 25–33 linked there.

Limitations/unexecuted: P17 owner pilot/use/explanation remains outstanding.
Next work: O11 Settings, storage, recovery and long-running operation.

### O11 — Settings, storage, recovery and long-running operation

Packet/state: O11 / `complete` (2026-10-02).

Prerequisites/specifications checked: O02, O03, O06, O09, O10 complete; product, architecture,
scope/glossary, protocol, persistence, operations and verification.

Changed files:
- `internal/repository/gc.go`: enhanced `DetailedStorageUsage` to report 5 distinct categories (working-root, managed objects, staging, recovery, metadata) plus free space reserve (512 MiB) and metadata budget (256 MiB); implemented `PruneLifecycleRecords` and `LifecyclePruneReport` for safe bounded record pruning (Invariant I28).
- `internal/repository/versions.go`: added `SetAcquiredTime` for retention policy testing.
- `internal/repository/repository.go`: added thread-safe `ExecRaw` and `QueryRowRaw` to `*DB`.
- `tests/integration/orbit_storage_test.go`: test suite covering 5 storage categories, retention preview inspection read guarantee vs explicit GC, recovery copy reclaim, unregister safety (Invariant I20), and pause/resume lifecycle.
- `tests/integration/orbit_pruning_test.go`: test suite verifying Invariant I28: bounded lifecycle record pruning, preservation of pending/exhausted diagnostic tasks, and `EXPIRED_REPLAY` idempotency safety.
- `tests/integration/orbit_retirement_test.go`: test suite covering lost device decommissioning, fresh identity replacement enrollment (Invariants I08, I15, I24), stopped metadata backup restore, and payload honesty (G04).
- `docs/runbooks/lost-device-replacement.md`: operator runbook covering lost device retirement, replacement provisioning, stopped backup restoration, missing payload handling, and systemd user service lingering.
- `web/src/views/SettingsView.tsx`: rendered 5 storage categories with progress visualization, limits, Retention & Cleanup Modal trigger (`#btn-open-retention-modal`), GC trigger (`#btn-run-gc-settings`), Recovery copy reclaim (`#btn-reclaim-recovery`), Consistent SQLite Backup creation (`#btn-create-backup`), Bounded Lifecycle Pruning (`#btn-prune-records`), and collapsible Lost Device & Replacement Guide (`#btn-lost-device-guide` / `#lost-device-guide-panel`).
- `web/src/components/RetentionModal.tsx`: added retention preview with inspection read guarantee explanation and explicit GC trigger (`#btn-run-gc-modal`).
- `web/src/components/UnregisterModal.tsx`: modal explaining Invariant I20 unregister guarantee (local files preserved, zero deletion tombstones emitted).
- `scripts/orbit_ui_test.mjs`: added deterministic browser test scenarios for `settings` and `recovery` capturing screenshots 34–38.

Requirements/invariants: U04, U09, U11–U14, U16; S14, S17, S21; I07, I08, I10, I13, I15, I16, I19, I20, I26, I28.
Tests cover 5 distinct storage accounting categories, visible default limits (Metadata Budget 256 MiB, Free Space Reserve 512 MiB), retention preview inspection read guarantee (calculates eligible vs protected bytes without mutating disk state), explicit storage cleanup mutation (GC), recovery copy reclaim, unregister folder safety (clears folder registration while strictly preserving local files and history chunks without emitting deletion tombstones to the DAG), pause/resume sync lifecycle, safe bounded lifecycle record pruning (completed tasks, expired invitations, terminal join requests, expired read leases, expired idempotency keys) with preservation of queued/running/exhausted diagnostic tasks, expired idempotency replay safety (`EXPIRED_REPLAY`), lost device retirement and key lockout, replacement device enrollment with fresh identity keypair, stopped/exclusive metadata backup restore with monotonic counter reset, payload honesty for missing CAS chunks, and CLI vs UI parity across all operations.

Actual commands/results:
- `node scripts/orbit_ui_test.mjs --scenario settings`: passed (4/4 steps), captured screenshots 34–36.
- `node scripts/orbit_ui_test.mjs --scenario recovery`: passed (2/2 steps), captured screenshots 37–38.
- `go test -count=1 -v ./internal/control ./internal/repository ./internal/scheduler ./tests/integration -run 'TestOrbitSettings|TestOrbitStorage|TestOrbitPruning|TestOrbitRecovery|TestOrbitRetirement'`: passed (19/19 tests passed, 0 failures).
- `make test-race`: passed all packages with 0 race detector warnings.
- `make check`: passed all formatting, lint, unit, integration, model, fault, link validation, cross-compilation, and release packaging checks.

Evidence: [summary](../evidence/orbit-o11/summary.md),
[commands](../evidence/orbit-o11/commands.md),
[results](../evidence/orbit-o11/results.json),
[manifest](../evidence/orbit-o11/manifest.json), and screenshots 34–38 linked there.

Limitations/unexecuted: O12 release packaging, native installer testing, desktop launcher integration, and legacy adoption;
P17 owner pilot/use/explanation remains outstanding.
Next work: O12 Orbit packages, legacy adoption and executable documentation.

### O12 — Orbit packages, legacy adoption and executable documentation

Packet/state: O12 / `complete` (2026-10-02).

Prerequisites/specifications checked: O04, O06, O08, O10, O11 complete; product, architecture,
scope/glossary, protocol, persistence, operations and verification.

Changed files:
- `packaging/desktop/orbit.desktop`: FreeDesktop application launcher entry for desktop environments.
- `packaging/icons/orbit.svg`: scalable application vector icon for desktop menus and docks.
- `packaging/systemd/orbit.service` & `packaging/systemd/filesync.service`: single user-service unit adoption with mutual aliases (`Alias=filesync.service` / `Alias=orbit.service`).
- `packaging/scripts/install.sh`: standalone installer supporting user-local (`~/.local/bin`) and system-wide modes, installing binaries, desktop entries, icons, and systemd units with gentle service adoption.
- `packaging/scripts/uninstall.sh`: standalone uninstaller cleanly unregistering services while strictly preserving all workspace roots and state directories (`~/.local/state/filesync`, `~/.filesync`) per Invariant S21/I20.
- `packaging/LICENSES.md`: third-party licensing attribution including React, React-DOM, and Vite.
- `scripts/build_packages.go`: deterministic reproducible release packager building `.tar.gz`, `.deb`, and `.rpm` archives for `linux/amd64` and `linux/arm64`, generating `release-manifest.json` and `SHA256SUMS`.
- `scripts/validation/service_lifecycle.py`: extended native user-service lifecycle harness to assert and verify checksums for `desktop/orbit.desktop` and `icons/orbit.svg`.
- `scripts/validation/reproduce_release.py`: updated release reproduction script to record `orbit_version` alongside binary version.
- `web/embed.go` & `internal/control/server.go`: implemented `web.GetAssetInfo()` calculating deterministic SHA-256 digest across embedded `dist/` assets; exposed product ("Orbit"), version ("1.0.0"), commit, build date, `schema_version: 13`, `config_format_version: 1`, pure-Go SQLite indicator, and asset digest in `GET /api/v1/version`.
- `cmd/filesync/main.go`: added `orbit version`, `orbit version --json`, `orbit init`, and `orbit service restart` commands; bound build ldflags in `main()`.
- `tests/integration/orbit_package_test.go`: 7 integration tests covering release packages, tarball contents, version/manifest metadata, install/uninstall lifecycle, legacy schema 5 adoption, schema rollback refusal, and stopped backup restore with fresh identity.
- `docs/runbooks/install.md`: rewritten around Orbit package choices, desktop launch (`orbit launch`), service lingering, and initial device setup.
- `docs/runbooks/uninstall.md`: rewritten around Orbit safe uninstallation and Invariant S21 data preservation.
- `docs/runbooks/database-recovery.md`, `docs/runbooks/rollback.md`, `docs/runbooks/upgrade.md`: updated to use Orbit commands with `filesync` backward-compatible notes.
- `docs/runbooks/private-network.md`: runbook covering LAN/VPN topology, firewall setup, and reachability diagnostics.
- `README.md`: rewritten around Orbit personal file manager with package installation, desktop launch, command reference, and runbooks.

Requirements/invariants: U01, U08, U12–U14, U16; S20–S22; I08, I19, I20, I21, I28.
Tests cover reproducible package creation (8 archives across amd64 and arm64), tarball extraction and symlink structure, embedded frontend asset freshness and SHA-256 digest calculation, zero Node.js runtime requirement, desktop entry and scalable SVG icon installation, user-service integration and mutual alias adoption (preventing duplicate daemon processes), standalone installer and uninstaller lifecycle with Invariant S21/I20 data preservation (workspace files and SQLite state untouched, zero deletion tombstones emitted), authentic legacy schema 5 database adoption to schema 13 with all historical records preserved, schema rollback refusal (`user_version = 14` immediately refused with `ErrIncompatibleSchema`), stopped metadata backup restore with fresh causal identity rotation (preventing counter collisions per Invariant I08), native service lifecycle execution on local host, and complete CLI vs UI parity across all operations.

Actual commands/results:
- `cd web && npm ci && npm run build && cd ..`: passed (38 modules transformed, zero errors).
- `make package`: passed, generated 8 release packages, `release-manifest.json`, and updated `dist/SHA256SUMS`.
- `go test -count=1 -v ./tests/integration -run 'TestOrbitPackaging|TestOrbitTarball|TestOrbitVersion|TestOrbitInstall|TestOrbitLegacy|TestOrbitSchema|TestOrbitStopped'`: passed (7/7 tests passed, 0 failures).
- `python3 scripts/validation/service_lifecycle.py --hosts local --output /tmp/service-lifecycle-test-o12`: passed (ordinary edit, service restart, and uninstall data preservation verified).
- `make check`: passed all formatting, lint, unit, integration, model, fault, link validation, cross-compilation, and release packaging checks.
- `make test-race`: passed all packages with 0 race detector warnings.
- `make demo`: passed local multi-process replication demo.

Evidence: [summary](../evidence/orbit-o12/summary.md),
[commands](../evidence/orbit-o12/commands.md),
[results](../evidence/orbit-o12/results.json), and
[manifest](../evidence/orbit-o12/manifest.json).

Limitations/unexecuted: none remaining for O12.
Next work: O13 Failure campaign, usability pilot and final handoff.

### O13 — Failure campaign, usability pilot and final handoff

Packet/state: O13 / `complete` (2026-10-02).

Prerequisites/specifications checked: O00–O12 complete; product, architecture,
scope/glossary, protocol, persistence, operations and verification.

Changed files:
- `tests/integration/orbit_o13_campaign_test.go`: comprehensive automated integration test suite validating all 10 minimum product scenarios (Scenarios 1 through 10) covering fresh install, enrollment, hub forwarding, nested mutations/stale invalidation, 3-way offline concurrency/restore, session security/daemon lifecycle, CAS missing chunk/read lease GC protection/bounded pruning, device retirement/stopped backup restore, legacy schema 5 adoption/rollback refusal, and Unicode/accessibility.
- `docs/evidence/orbit-o13/summary.md`: comprehensive evidence summary of all 10 scenarios, resource scaling measurements, browser UI suite results, verification gate pass outcomes, P17 owner pilot handoff guidelines, and honest unexecuted labels.
- `docs/evidence/orbit-o13/commands.md`: exact commands, parameters, and outputs executed during verification.
- `docs/evidence/orbit-o13/results.json`: structured test matrix, scenario results, scaling metrics, and unexecuted labels.
- `docs/evidence/orbit-o13/manifest.json`: release manifest capturing commit hash, binary builds, embedded asset digests, and package checksums.

Requirements/invariants: U01–U16; S01–S22; I01–I28.
Tests cover all 10 minimum product scenarios defined in `docs/implementation/orbit-release.md`, resource scaling across 10,000 synthetic files (0.59 MB heap allocated during queries, 110.4 ms root browse, 170.6 ms deep subdir browse, 110.7 ms search, 8,754 bytes root page JSON), chunked CAS streaming bounded memory overhead (~1.7 MiB), active read lease protection against concurrent GC (Invariant I25) with collection upon release, safe bounded lifecycle task pruning preserving exhausted diagnostic tasks (Invariant I28), permanent device retirement (Invariant I24), stopped backup restore with identity rotation and `next_counter = 0` reset (Invariant I08), schema rollback refusal (Invariant I20), single-use bootstrap token security with daemon session independence (Invariant I21), and full UI and CLI parity (Invariant I19).

Actual commands/results:
- `go test -count=1 -v ./tests/integration -run '^TestOrbitO13_'`: passed all 10 minimum product scenarios (10/10 tests passed, 0 failures, 0.198s).
- `go test -count=1 -v ./internal/repository -run TestOrbitBrowse_ScalingTenThousandFiles`: passed (10,000 files in 439.5ms, root browse in 110.4ms, search in 110.7ms, heap allocated 0.59 MB).
- `go test -count=1 -v ./internal/repository -run TestOrbitContent_CorruptionCancellationAndIntent`: passed (CAS corruption and intent handling verified).
- `node scripts/orbit_ui_test.mjs --scenario all`: passed (10/10 scenarios, 38 screenshot checkpoints verified).
- `make check`: passed all formatting, lint, unit, integration, model, fault, fuzz, arm64 cross-compilation, and release packaging checks.
- `make test-race`: passed all packages with 0 race detector warnings.
- `make demo`: passed local multi-process replication demo with TLS, sync, partition, and reviewed conflict resolution.
- `python3 scripts/validation/service_lifecycle.py --hosts local --output /tmp/service-lifecycle-test-o13`: passed native service lifecycle.

Evidence: [summary](../evidence/orbit-o13/summary.md),
[commands](../evidence/orbit-o13/commands.md),
[results](../evidence/orbit-o13/results.json), and
[manifest](../evidence/orbit-o13/manifest.json).

Limitations/unexecuted:
- Bare-metal physical power loss / KVM abrupt-reset (covered via in-process process termination, SQLite crash consistency, write-ahead log recovery, and transaction rollback; physical hardware cut unexecuted).
- Multi-node physical hardware deployment on Raspberry Pi (cross-compilation for arm64 tested and verified; physical execution on physical Pi hardware unexecuted).
- P17 owner live personal pilot execution in user's personal live data directory (disposable end-to-end verification completed; live personal pilot reserved for Caleb's unaided evaluation).
Next work: Orbit revamp complete. Personal owner pilot and unaided distributed system explanation ready for Caleb.




### O14 — Owner-selected local folder relocation

Packet/state: O14 / `complete` (2026-10-03).
Prerequisites: O03/O09/O11/O12; read product, architecture, plan/status,
scope/glossary, persistence, protocol, operations and verification. Scope is
local relocation; peer identity/membership/history are unchanged.

Changed files: workspace relocation/gating and tests; repository transactional
root/setup update; shared control endpoint; stopped/live CLI adapter/test;
Settings form/API and rebuilt embedded assets; watcher refresh; Chromium runner;
README, product/plan, persistence/operations and relocation evidence.

Requirements/invariants: U04/U16; I07/I11/I17/I19/I20/I27. Actual commands/results:
`npm ci` and `npm run build` passed; `go test -count=1 -v ./internal/workspace -run
TestRelocation` passed five test families including all journal boundaries on
same/cross filesystems; `go test -count=1 ./cmd/filesync -run TestRelocation`
passed stopped/live API parity; `go test -race -count=1 ./internal/workspace
./internal/scheduler ./internal/control ./cmd/filesync` passed; `make check`
passed; `node scripts/orbit_ui_test.mjs --scenario relocation` passed keyboard
focus, retained errors, relocation success and new-location edit capture.
Detailed results/limits: [evidence](../evidence/orbit-relocation/summary.md).

Limitations: cross-drive originals and interrupted staging copies are retained
for manual review/cleanup. All roots' workspace IO is gated temporarily. Fault
hooks plus persisted-state reopening exercised recovery; actual SIGKILL,
physical reset and cross-drive ENOSPC remain unexecuted. P17 owner-use and
unaided explanation remain outstanding. Next action: owner pilot/explanation.

Additional validation fix: `TestOrbitSetup_OpenLocalFolder` now clears DISPLAY
and WAYLAND_DISPLAY and asserts a headless refusal. A disposable `xdg-open` spy
reproduced the unintended helper launch before the fix and verified no launch
afterward. This prevents the owner's KDE missing-temporary-folder dialogs from
this test while preserving production Open in File Manager behavior.
