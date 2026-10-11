# Final-scope status

Updated: 2026-10-11. **R00 complete; R01 and R02 eligible** (default order: R01,
then R02). The [plan](../orbit-final-implementation-plan.md),
[packets](final-packets.md) and [gates](final-gates.md) form the handoff.

Current deployment for reference: hosted `orbit-net` 1.1.0 serves any device,
relaying under a 2 TiB monthly budget (2026-10-11 read-only snapshot: 2 directory
records, 2 controls, about 6.3 MB relayed this month). Orbit 2.3.0 is installed
as `orbit-trial` on PC, laptop and Pi. Hosted changes need owner confirmation.

## Packet tracker

| Packet | State | Dependencies | Acceptance owner |
| --- | --- | --- | --- |
| R00 Baseline/interim cap | complete (cap not applied, owner's choice) | repository | [R00](final-packets.md#r00--baseline-interim-protection-and-gate-scoping) |
| R01 Route accounting | eligible | R00 | [R01](final-packets.md#r01--route-accounting-rg2) |
| R02 Service allowlist | eligible | R00 | [R02](final-packets.md#r02--service-admission-allowlist-rg1) |
| R03 One-command self-hosting | blocked on R02 | R02 | [R03](final-packets.md#r03--one-command-self-hosting) |
| R04 No default service | blocked on R02 | R02 | [R04](final-packets.md#r04--release-builds-without-a-default-service-rg3) |
| R05 Measurement | blocked on R01, R04 | R01, R04 | [R05](final-packets.md#r05--measurement-campaign-rg4) |
| R06 Write-up/release | blocked on R03, R05 | R03, R05 | [R06](final-packets.md#r06--write-up-and-release) |

| Gate | State | Owning packet |
| --- | --- | --- |
| RG1 Service admission | open (R00 scoped) | R02 |
| RG2 Route accounting | open (R00 scoped) | R01 |
| RG3 Release default | open (R00 scoped) | R04 |
| RG4 Measurement method | open (R00 scoped) | R05 |

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

## Handoff log

- 2026-10-11: R00 complete. Next: R01 (route accounting), then R02. Both are
  eligible and touch separate modules.
