# Packet O13 Validation Commands

**Packet:** O13  
**Date:** 2026-10-02  
**Commit:** `0a16d84c8e2acf8d5d0f712f42663aa292c42e29`  

---

## 1. Automated 10-Scenario Campaign Suite

```bash
# Execute the full 10-scenario campaign integration suite
go test -count=1 -v ./tests/integration -run '^TestOrbitO13_'
```
**Output:**
```
=== RUN   TestOrbitO13_Scenario01_FreshInstall_LaunchReuse_Adoption_DisallowedRoot
--- PASS: TestOrbitO13_Scenario01_FreshInstall_LaunchReuse_Adoption_DisallowedRoot (0.02s)
=== RUN   TestOrbitO13_Scenario02_Invitation_Approval_Expiry_Rollout_CatchUp
--- PASS: TestOrbitO13_Scenario02_Invitation_Approval_Expiry_Rollout_CatchUp (0.04s)
=== RUN   TestOrbitO13_Scenario03_ExistingContentJoin_HubForwarding_ReplicaStatus
--- PASS: TestOrbitO13_Scenario03_ExistingContentJoin_HubForwarding_ReplicaStatus (0.03s)
=== RUN   TestOrbitO13_Scenario04_NestedBrowseSearchPreview_JournaledMutations_Races
--- PASS: TestOrbitO13_Scenario04_NestedBrowseSearchPreview_JournaledMutations_Races (0.02s)
=== RUN   TestOrbitO13_Scenario05_ThreeOfflineEdits_ReviewedResolution_LateArrival_Restore
--- PASS: TestOrbitO13_Scenario05_ThreeOfflineEdits_ReviewedResolution_LateArrival_Restore (0.02s)
=== RUN   TestOrbitO13_Scenario06_SessionSecurity_ServiceRestart_SingletonLock_HeadlessCLI
--- PASS: TestOrbitO13_Scenario06_SessionSecurity_ServiceRestart_SingletonLock_HeadlessCLI (0.02s)
=== RUN   TestOrbitO13_Scenario07_MissingChunkDiagnosed_ReadLease_SafePruning
--- PASS: TestOrbitO13_Scenario07_MissingChunkDiagnosed_ReadLease_SafePruning (0.01s)
=== RUN   TestOrbitO13_Scenario08_LostDeviceRetirement_ReplacementKey_StoppedBackupRestore
--- PASS: TestOrbitO13_Scenario08_LostDeviceRetirement_ReplacementKey_StoppedBackupRestore (0.02s)
=== RUN   TestOrbitO13_Scenario09_LegacySchemaAdoption_RollbackRefusal_UninstallDataPreservation
--- PASS: TestOrbitO13_Scenario09_LegacySchemaAdoption_RollbackRefusal_UninstallDataPreservation (0.01s)
=== RUN   TestOrbitO13_Scenario10_KeyboardAccessibility_UnusualFilenames_ResponsiveLayout
--- PASS: TestOrbitO13_Scenario10_KeyboardAccessibility_UnusualFilenames_ResponsiveLayout (0.02s)
PASS
ok  	github.com/calebhabesh/file-sync/tests/integration	0.198s
```

---

## 2. Resource Scaling & Performance Measurements

```bash
# Execute 10,000 files browse and search scaling test
go test -count=1 -v ./internal/repository -run TestOrbitBrowse_ScalingTenThousandFiles
```
**Output:**
```
=== RUN   TestOrbitBrowse_ScalingTenThousandFiles
    orbit_browse_test.go:458: Generating 10,000 files across 100 directories...
    orbit_browse_test.go:510: Generated 10,000 files in 439.546755ms
    orbit_browse_test.go:527: Root first-page browse (50 items) on 10,000 files took: 110.353922ms (items: 50, total: 100)
    orbit_browse_test.go:543: Deep subdir browse (50 items) took: 170.600367ms (items: 50, total: 100)
    orbit_browse_test.go:559: Workspace search for 'file_42' on 10,000 files took: 110.67206ms (matched: 50, total found: 100)
    orbit_browse_test.go:569: Root page JSON bytes: 8754
    orbit_browse_test.go:570: Memory allocated during scaling queries: 0.59 MB
--- PASS: TestOrbitBrowse_ScalingTenThousandFiles (0.84s)
PASS
ok  	github.com/calebhabesh/file-sync/internal/repository	0.845s
```

```bash
# Verify CAS corruption and intent handling
go test -count=1 -v ./internal/repository -run TestOrbitContent_CorruptionCancellationAndIntent
```
**Output:**
```
=== RUN   TestOrbitContent_CorruptionCancellationAndIntent
--- PASS: TestOrbitContent_CorruptionCancellationAndIntent (0.01s)
PASS
ok  	github.com/calebhabesh/file-sync/internal/repository	0.015s
```

---

## 3. End-to-End Browser UI Scenario Suite

```bash
# Run all 10 UI scenarios and 38 screenshot checkpoints via Puppeteer
node scripts/orbit_ui_test.mjs --scenario all
```
**Output:**
```
========================================================
  ✓ ALL O04 SETUP ACCEPTANCE CRITERIA PASSED
  ✓ ALL O05 PAIRING ACCEPTANCE CRITERIA PASSED
  ✓ ALL O06 DEVICES ACCEPTANCE CRITERIA PASSED
  ✓ ALL O08 BROWSE ACCEPTANCE CRITERIA PASSED
  ✓ ALL O08 PREVIEWS ACCEPTANCE CRITERIA PASSED
  ✓ ALL O10 FILE-ACTIONS ACCEPTANCE CRITERIA PASSED
  ✓ ALL O10 HISTORY & RESTORE ACCEPTANCE CRITERIA PASSED
  ✓ ALL O10 ATTENTION ACCEPTANCE CRITERIA PASSED
  ✓ ALL O11 SETTINGS & STORAGE ACCEPTANCE CRITERIA PASSED
  ✓ ALL O11 RECOVERY ACCEPTANCE CRITERIA PASSED
========================================================
```

---

## 4. Full Release Verification Gate

```bash
# Format, vet, unit, integration, model, faults, arm64 cross-compile, packages
make check
```
**Output:**
```
Exit Code: 0
All tests passed, arm64 binary compiled, 8 release packages generated with SHA256SUMS.
```

```bash
# Concurrency safety & race detection
make test-race
```
**Output:**
```
Exit Code: 0
0 race conditions detected across all packages.
```

```bash
# Multi-process replication demo
make demo
```
**Output:**
```
[PASS] Local multi-process demo completed successfully.
```

```bash
# Native user-service lifecycle script
python3 scripts/validation/service_lifecycle.py --hosts local --output /tmp/service-lifecycle-test-o13
```
**Output:**
```
PASS native user-service lifecycle: local
```
