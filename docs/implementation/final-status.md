# Final-scope status

Updated: 2026-10-11. **All packets R00–R06 complete; all gates closed. Final scope finished.** The [plan](../orbit-final-implementation-plan.md),
[packets](final-packets.md) and [gates](final-gates.md) form the handoff.

Current deployment for reference: hosted `orbit-net` 1.1.0 serves any device,
relaying under a 2 TiB monthly budget (2026-10-11 read-only snapshot: 2 directory
records, 2 controls, about 6.3 MB relayed this month). Orbit 2.3.0 is installed
as `orbit-trial` on PC, laptop and Pi. Hosted changes need owner confirmation.

## Packet tracker

| Packet | State | Dependencies | Acceptance owner |
| --- | --- | --- | --- |
| R00 Baseline/interim cap | complete (cap not applied, owner's choice) | repository | [R00](final-packets.md#r00--baseline-interim-protection-and-gate-scoping) |
| R01 Route accounting | complete | R00 | [R01](final-packets.md#r01--route-accounting-rg2) |
| R02 Service allowlist | complete | R00 | [R02](final-packets.md#r02--service-admission-allowlist-rg1) |
| R03 One-command self-hosting | complete | R02 | [R03](final-packets.md#r03--one-command-self-hosting) |
| R04 No default service | complete | R02 | [R04](final-packets.md#r04--release-builds-without-a-default-service-rg3) |
| R05 Measurement | complete | R01, R04 | [R05](final-packets.md#r05--measurement-campaign-rg4) |
| R06 Write-up/release | complete | R03, R05 | [R06](final-packets.md#r06--write-up-and-release) |

| Gate | State | Owning packet |
| --- | --- | --- |
| RG1 Service admission | closed (R02) | R02 |
| RG2 Route accounting | closed (R01) | R01 |
| RG3 Release default | closed (R04) | R04 |
| RG4 Measurement method | closed (R05) | R05 |

## Handoff log (planning)

- 2026-10-11: plan written after the owner chose to finish Orbit as a
  distributed systems project with zero operating cost (superseding the drafted
  bring-your-own-relay plan, which was never committed). No code changed.

## R00 — Baseline, interim protection and gate scoping

Complete 2026-10-11 at `b2172e2`. Evidence: `docs/evidence/final-r00-20261011/`.

- Commands: `ssh vps` read-only snapshot (version, unit, redacted `serve.json`,
  relay month file, `/metrics`); `scripts/trial/trial.sh status`;
  `ORBIT_FINAL_R00_WINDOW=300s go test ./internal/rendezvous/ -run TestFinalR00 -count=1 -v`
  (pass, 330 s); `go test -list '^TestFinal' ./internal/... ./cmd/... ./tests/...`
  (2 tests); `make quick` (pass, 66 s).
- VPS 05:48 UTC: 1.1.0, 2 records, 2 controls (one online device), 6.3 MB
  relayed in October, 57 STUN answers, no budget refusals. Unchanged.
- Interim cap **not applied**: the owner kept 2 TiB (within free egress, $0
  either way). No backup or rollback was needed.
- Fixture: an idle online device costs ≈ 367 KB/hour at the service (renewals
  dominate). 64 online devices fill the fixed 128 records/128 controls. A new
  device in a full directory stays not ready, retries every ~10 s, and recovers
  5 s after a slot frees.
- Gates RG1–RG4 scoped in `final-gates.md`. Most consequential: allowlist in
  `admitOrigin`; the challenge table needs an unlisted share; 2.3 clients retry
  an unknown refusal code ~25 times/min indefinitely.
- Test infrastructure: the rendezvous fixture's `countingListener` now also
  counts inbound bytes (`wireIn`).
- Limitations: the idle figure excludes STUN, lookups, dials and TCP/IP header
  overhead; the "one device is the owner's PC" attribution is inferred, since
  metrics have no IDs; `make check` was not run (milestone M1 is reached at R02).

## R01 — Route accounting

Complete 2026-10-11 (base `4de12fd`). Evidence: `docs/evidence/final-r01-20261011/`.

- Commands: the `TestFinalR01` set across network, config, control, contract,
  replication, terminal and cmd/orbit (all pass); `BenchmarkFinalR01*` ×3; the
  opt-in 1-GiB pull (pass, 17.3 s); W08/W09 production-binary daemon tests with
  the new route-usage assertions (pass, 95 s); `make quick` (pass, 110 s).
- Result: per-peer `lan`/`wan_tcp`/`quic`/`relay` request and byte counts, a
  durable 90-day rollup, `orbit network status --routes`, `route_usage` in JSON,
  and qualified share lines in status, doctor and TUI Connection details.
  Counting overhead is under 0.01% on a 1-GiB pull.
- Limitations: in-process fixtures (not namespaces), a faked public address for
  `wan_tcp`, microbenchmark-based overhead, own requests only; `make check` not
  run (M1 at R02).

## R02 — Service admission allowlist

Complete 2026-10-11 (base `86678f2`). Evidence: `docs/evidence/final-r02-20261011/`.

- Commands: `TestFinalR02*` in rendezvous (4), cmd/orbit-net (4 plus the
  opt-in old-client fixture) and tests/terminal (1), all pass; the 2.3 client
  (built from `7be6a77` in a temporary worktree, since removed) against a real
  allowlisted `orbit-net`; `make check` exit 0 (tests/terminal 2,100 s,
  packages built) at milestone M1.
- Result: `admission: allowlist` with `orbit-net devices list|add|remove` and
  SIGHUP reload. Unlisted devices are limited to pairing toward listed devices
  within their own small share; `NOT_ADMITTED` is typed for signed requests
  and controls. New clients pause 10 minutes; a 2.3 client retries
  8.5 times/min, all refused before allocation.
- Milestone M1: the allowlist refuses an unlisted device on a local fixture
  while listed devices sync. The interim cap was not applied (owner's choice,
  R00).
- Limitations: 2.3 retry cost until epoch 2 expires (2027-10-07); new-device
  bootstrap through the service waits for the operator's `devices add`; the
  VPS still runs open 1.1.0 (R06).

## R03 — One-command self-hosting

Complete 2026-10-11 (base `618a886`). Evidence: `docs/evidence/final-r03-20261011/`.

- Commands: `TestFinalR03*` (pass); `scripts/validation/selfhost_netns.py`
  rootless namespace runs with and without the A↔B drop (both pass);
  `make quick` (pass).
- Result: `orbit-net init` creates a complete owner-only self-hosted service
  and prints the device and allowlist steps. Two fresh devices with direct
  paths blocked joined (38 s) and synced both ways through it, 100% relay.
- Limitations: one host in namespaces; with private addressing the directory
  publishes no direct candidates, so the drop rules were not exercised;
  certificate renewal and unit install are manual.

## R04 — Public builds without a default service

Complete 2026-10-11 (base `dbfacd4`). Evidence: `docs/evidence/final-r04-20261011/`.

- Commands: build both variants; `TestFinalR04*`; tagged and untagged release
  tests; 2.3 → public → 2.3 upgrade/rollback in an empty namespace; the owner
  hosted-profile adoption from file; W06/W07/W14 terminal tests (W07 PTY
  failed first on its old expectation, fixed and passed); `PTY=all make
  quick`; hosted-tag short tests; PC `orbit-trial` public-build rehearsal
  (public build, then owner build restored).
- Result: public builds start on Local network and say "No Orbit service
  configured" with the self-hosting step; owner devices keep the VPS through
  their stored selection.
- Limitations: 2.x installs in the wild keep the embedded profile until
  2027-10-07; laptop/Pi trials not reinstalled.

## R05 — Measurement campaign

Complete 2026-10-11 (base `8ef117d`). Evidence: `docs/evidence/final-r05-20261011/`.

- Commands: `measure_netns.py`, five connectivity cells × 10 repetitions,
  plus three bulk runs (throughput, chunks, faults), all success;
  `summarize_r05.py`. Two discarded attempts (driver bugs; tmpfs quota) are
  recorded.
- Result: direct upgrade 10/10 on LAN (15 s), port-preserving NAT (73 s) and
  public TCP (68 s); 0/10 on symmetric NAT and UDP-blocked (relay). Throughput
  12.6 / 4.2 / 1.6 MiB/s (LAN / NAT QUIC / relay). Chunk tail append 4.6%,
  prefix insertion 101.6%. SIGKILL resume refetched about 1.2 MB. Partition
  converged to identical heads in 22 s. Roaming completed.
- Limitations: emulation on one host with concurrent runs; physical cells
  cited from W16 (single runs); hotspot/CGNAT, IPv6-only and fresh Pi samples
  unexecuted; small-file sync slows as the folder grows (finding).

## R06 — Write-up and release

Complete 2026-10-11. Evidence: `docs/evidence/final-r06-20261011/`.

- Commands: case study, README and bullets rewritten; version bump
  (`11b17da`); VPS switch (backup → install 1.2.0 → allowlist → `--check` →
  one restart → verify listed and unlisted); public 2.4.0 installs on
  PC/laptop/Pi; `make trial-install`; uncached `make check` (pass after one
  test update), `make test-race-core` (pass after a test-harness race fix),
  `make test-terminal-release` (pass), `make demo` (pass).
- Milestone M4: case study and README updated, VPS on allowlist, packaged
  build installed on all three hosts.
- Limitations: a full-terminal race run was not executed; laptop/Pi trial
  identities must be allowlisted once they exist; nothing pushed.

## Handoff log

- 2026-10-11: R06 complete. The final scope (R00–R06) is finished; RG1–RG4 are
  closed. Remaining items are future work only (plan "Scope boundaries"):
  hotspot/CGNAT measurement with the owner, a full-terminal race run, and
  publishing the 2.4.0 release.

- 2026-10-11: R05 complete; RG4 closed. Owner approved the R06 VPS switch
  (allowlist with the PC `orbit-trial` device, keep 2 TiB). Next: R06.

- 2026-10-11: R04 complete; RG3 closed. PC `orbit-trial` runs the R04 owner
  build (`dbfacd4-dirty`, hosted). Next: R05 measurement (RG4). The rootless
  namespace fixture needs public-like addresses for direct-path cells, since
  the directory never publishes private candidates.

- 2026-10-11: R03 complete. Next: R04 (no default service, RG3). Plan: split
  the embedded profile behind a build tag (`orbit_hosted` for owner builds),
  report `NO_SERVICE_CONFIGURED` with the self-hosting step, default fresh
  setup to Local network, and give short pairing codes a clear error.

- 2026-10-11: R02 complete; RG1 closed; `make check` passed. Next: R03
  (`orbit-net init` drafted: `cmd/orbit-net/init.go`, unit tests, rootless
  namespace driver `scripts/validation/selfhost_netns.py`), then R04.
- 2026-10-11: R01 complete; RG2 closed. Next: R02 (service allowlist, RG1), using
  the R00 RG1 scoping.
- 2026-10-11: R00 complete. Next: R01 (route accounting), then R02. Both are
  eligible and touch separate modules.
