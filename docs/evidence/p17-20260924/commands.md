# Packet P17: Execution Commands and Logs

- **Date:** 2026-09-24
- **Commit:** `51b33be`
- **Go Version:** `go1.27.1-X:nodwarf5 linux/amd64`
- **Hosts:**
  - `owner-desktop`: Workstation, x86_64, Arch Linux (kernel 7.2.6-arch2-1), AMD Ryzen 7 9700X (16 threads), 32 GB RAM, NVMe
  - `owner-pi`: Raspberry Pi 4 Model B, aarch64, Debian GNU/Linux 12 (bookworm), Linux 6.12.93+rpt-rpi-v8, ARM Cortex-A72 (4 cores), 4 GB RAM, ext4
  - `main-instance-vnic`: Oracle Cloud VPS, aarch64, Ubuntu 24.04.4 LTS, Linux 6.17.0-1018-oracle, ARM Neoverse-N1 (4 cores), 24 GB RAM, ext4

---

## 1. Three-Host Pilot Execution (`scripts/three_host_pilot.go`)

### Command
```bash
go run scripts/three_host_pilot.go
```

### Execution Log
```text
[STEP 1] Performing read-only hardware and OS inventory across all 3 hosts...
  -> owner-desktop: arch=x86_64 os=Arch Linux kernel=Linux 7.2.6-arch2-1 cpu=AMD Ryzen 7 9700X 8-Core Processor cores=16 disk_avail=49G
  -> Pi: arch=aarch64 os=Debian GNU/Linux 12 (bookworm) kernel=Linux 6.12.93+rpt-rpi-v8 cpu=ARM Cortex-A72 cores=4 disk_avail=40G
  -> VPS: arch=aarch64 os=Ubuntu 24.04.4 LTS kernel=Linux 6.17.0-1018-oracle cpu=ARM Neoverse-N1 cores=4 disk_avail=38G
[STEP 1 PASS] 3 physical/cloud hosts inventoried with distinct architectures and operating systems.

[STEP 2] Resetting dedicated disposable pilot directories on all 3 hosts...
  -> Directories: ~/filesync-pilot/{state,data} with .filesync-disposable markers.
[STEP 2 PASS] Dedicated disposable environments prepared.

[STEP 3] Establishing loopback SSH port-forwarding tunnels...
  -> Tunnels active: owner-desktop (:17001) <-> Pi (:17002) <-> VPS (:17003)
[STEP 3 PASS] Encrypted loopback transport bridge verified.

[STEP 4] Enrolling nodes and establishing canonical 3-node membership Revision 1...
  -> Device IDs:
     owner-desktop: 1ac98d04087c4a90b6682c8f581a45bbe92b463a7507c44a5e531f82d765ffcd
     Pi:      ba1bbcb66000bbceb0ab76d0a53bc7dd9b9490820f67398aeb327e091442b232
     VPS:     fd5af69b2f7be70814bae0881cfb40fc9d33b484b436dfd81d60f567d28f0a3e
  -> Key Pins:
     owner-desktop: 0795e8c1ce775cb14723e114f874b77f3225c530a59563c9eec37938334b9c0d
     Pi:      46b39269186d06f288d52ab568084ae14eec4f76fa2d2db303c30c92e6bf05ca
     VPS:     982188702af18c937c66b9955af732676587aa98308844b19e4b05d87e5302b9
[STEP 4 PASS] Revision 1 canonical membership approved on all 3 hosts with pinned SPKI TLS 1.3 certificates.

[STEP 5] Scenario 1: Normal Multi-Host Sync...
  -> owner-desktop authors README.md and notes.txt
  -> Pushed to VPS relay; Pi pulled from VPS relay.
  -> Verifying file integrity on Pi...
  -> File README.md matches expected content byte-for-byte on Pi!
  -> File notes.txt matches expected content byte-for-byte on Pi!
[STEP 5 PASS] Scenario 1 Normal Sync succeeded across all 3 hosts.

[STEP 6] Scenario 2: Three Independent Offline Edits...
  -> owner-desktop (offline) authored: owner-desktop offline edit at 2026-09-24T08:18:18Z
  -> Pi (offline) authored: Pi offline edit at 2026-09-24T08:18:19Z
  -> VPS (offline) authored: VPS offline edit at 2026-09-24T08:18:20Z
[STEP 6 PASS] Scenario 2 captured 3 concurrent offline modifications.

[STEP 7] Scenario 3: Reconnection, Gossip, and Concurrent Head Detection...
  -> Gossiping version envelopes across owner-desktop, VPS, and Pi...
  -> Conflict status on owner-desktop: 3 active branch heads!
     Head Token: 087f3db651cb1d0a5bcefa7a8e2da69ff558e6538f9bbaef3958da1f9b3cf286
  -> Conflict status on Pi: 3 active branch heads!
     Head Token: 087f3db651cb1d0a5bcefa7a8e2da69ff558e6538f9bbaef3958da1f9b3cf286
  -> Conflict status on VPS: 3 active branch heads!
     Head Token: 087f3db651cb1d0a5bcefa7a8e2da69ff558e6538f9bbaef3958da1f9b3cf286
[STEP 7 PASS] Scenario 3 Invariant I02 and I03 verified: all 3 hosts computed identical 3-head conflict token 087f3db651cb1d0a... without data loss.

[STEP 8] Scenario 4: Reviewed Conflict Resolution...
  -> Committing resolution on owner-desktop selecting owner-desktop's head (token: 087f3db651cb1d0a5bcefa7a8e2da69ff558e6538f9bbaef3958da1f9b3cf286)...
  -> Propagating resolution envelope to VPS and Pi...
  -> Conflict status on owner-desktop: 0 heads (converged!)
  -> Conflict status on VPS: 0 heads (converged!)
  -> Conflict status on Pi: 0 heads (converged!)
[STEP 8 PASS] Scenario 4 Invariant I16 verified: reviewed resolution merged DAG heads and converged all 3 replicas to 0 conflicts.

[STEP 9] Scenario 5: Third-Party Forwarding (A -> VPS -> B without A/B Online Overlap)...
  -> owner-desktop authors forwarding_proof.txt: Proof of forwarding through relay without direct A-B connection
  -> Pushed from owner-desktop to VPS.
  -> Shutting down owner-desktop sync daemon completely (offline)...
  -> Pi pulls from VPS relay while owner-desktop is completely unreachable...
  -> Verifying forwarding_proof.txt on Pi...
  -> Received byte-for-byte identical content!
[STEP 9 PASS] Scenario 5 Invariant I14 verified: third-party forwarding succeeded with zero online overlap between author and consumer.

[STEP 10] Scenario 6: Resumption, Large Transfer, and Historical Restore...
  -> owner-desktop authors 3 MiB multi-chunk file (dataset.bin) with deterministic pattern...
  -> Syncing 3 MiB file through VPS to Pi...
  -> Verifying dataset.bin size (3145728 bytes) and SHA-256 on Pi...
  -> Hash matches expected: 8251e06d9da26d4002c918f6f5817c1bf20ff68593a2eb681ef9482faee0f16f!
  -> Testing historical restore of README.md to version 1 on owner-desktop...
  -> Historical restore authored forward version without rolling back DAG!
[STEP 10 PASS] Scenario 6 Invariant I07 and I08 verified: multi-chunk transfer and forward historical restore succeeded.

[STEP 11] Scenario 7: Personal Pilot Verification and Database Integrity Audit...
  -> Running SQLite PRAGMA integrity_check on owner-desktop...
  -> Result: ok
  -> Running SQLite PRAGMA integrity_check on Pi...
  -> Result: ok
  -> Running SQLite PRAGMA integrity_check on VPS...
  -> Result: ok
[STEP 11 PASS] Scenario 7 passed: all 3 SQLite databases report 100% integrity (PRAGMA integrity_check = ok).

[STEP 12] Tearing down SSH tunnels and temporary processes...
[STEP 12 PASS] Teardown complete.

======================================================================
THREE-HOST PILOT COMPLETED SUCCESSFULLY! ALL SCENARIOS AND INVARIANTS VERIFIED.
Execution Time: 37.53s
Report saved to: docs/evidence/p17-20260924/pilot_results.json
======================================================================
```

---

## 2. Benchmark Suite Execution (`scripts/benchmark_suite.go`)

### Command
```bash
go run scripts/benchmark_suite.go
```

### Execution Log
```text
[BENCH] Starting File Sync vs Full-File Baseline Benchmark Campaign

[BENCH] 1. Small Files Hierarchy (1,000 files, varied depth)
        Payload: 30.85 MB | Files: 1000
        File Sync: 23.015348006s (Wire: 31.18 MB) | Baseline: 15.49145ms (Wire: 30.85 MB)

[BENCH] 2. Unchanged Tree Rescan
        ★ File Sync Wire: 4 KB (100% reused) vs Baseline: 30.85 MB

[BENCH] 3. Large File: Tail Overwrite (40 MiB file, modify 4 KiB at tail)
        ★ File Sync Wire: 1.0 MB (39.0 MB reused) vs Baseline: 40.0 MB (Savings: 97.5%)

[BENCH] 4. Large File: Prefix Insertion (NEGATIVE RESULT: 1-byte shift defeating fixed-size chunking)
        ⚠ NEGATIVE RESULT: 1-byte prefix shift forced 100% chunk retransmission (Wire: 20MB, Reused: 0B)

[BENCH] 5. File Rename (Metadata-Only Transfer)
        ★ File Sync Wire: 1.0 KB (40.0 MB reused) vs Baseline: 40.0 MB (Savings: 99.99%)

[BENCH] Benchmark Campaign Complete
        Results exported to: docs/evidence/p17-20260924/benchmarks.json
```

---

## 3. Project Pipeline Verification (`make check`, `make test-race`)

### Commands
```bash
make check
make test-race
```

### Verification Outcomes
- `fmt-check`: PASS (All Go files formatted per `gofmt`).
- `vet`: PASS (0 issues found across all packages).
- `test`: PASS (All unit and model tests pass).
- `test-integration`: PASS (Integration test suites P00 through P15 pass).
- `test-faults`: PASS (Design gates D1–D5, crash boundaries, abrupt-reset tests, and invariants I01–I20 pass).
- `build`: PASS (`bin/filesync` ELF static binary generated for `linux/amd64`).
- `build-arm64`: PASS (`bin/filesync-linux-arm64` ELF static binary generated for `linux/arm64`).
- `package`: PASS (Debian, RPM, and tarball archives generated with verified SHA256 checksums).
- `test-race`: PASS (0 data races detected across all packages under `-race`).
