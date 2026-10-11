# Final-scope status

Updated: 2026-10-11. **Planned; no packet started.** The
[plan](../orbit-final-implementation-plan.md), [packets](final-packets.md) and
[gates](final-gates.md) form the handoff. Start with R00.

Current deployment for reference: hosted `orbit-net` 1.1.0 serves any device,
relaying under a 2 TiB monthly budget (2026-10-11 read-only snapshot: 2 directory
records, 2 controls, about 6.3 MB relayed this month). Orbit 2.3.0 is installed
as `orbit-trial` on PC, laptop and Pi. Hosted changes need owner confirmation.

## Packet tracker

| Packet | State | Dependencies | Acceptance owner |
| --- | --- | --- | --- |
| R00 Baseline/interim cap | eligible | repository | [R00](final-packets.md#r00--baseline-interim-protection-and-gate-scoping) |
| R01 Route accounting | blocked on R00 | R00 | [R01](final-packets.md#r01--route-accounting-rg2) |
| R02 Service allowlist | blocked on R00 | R00 | [R02](final-packets.md#r02--service-admission-allowlist-rg1) |
| R03 One-command self-hosting | blocked on R02 | R02 | [R03](final-packets.md#r03--one-command-self-hosting) |
| R04 No default service | blocked on R02 | R02 | [R04](final-packets.md#r04--release-builds-without-a-default-service-rg3) |
| R05 Measurement | blocked on R01, R04 | R01, R04 | [R05](final-packets.md#r05--measurement-campaign-rg4) |
| R06 Write-up/release | blocked on R03, R05 | R03, R05 | [R06](final-packets.md#r06--write-up-and-release) |

| Gate | State | Owning packet |
| --- | --- | --- |
| RG1 Service admission | open | R02 |
| RG2 Route accounting | open | R01 |
| RG3 Release default | open | R04 |
| RG4 Measurement method | open | R05 |

## Handoff log

- 2026-10-11: plan written after the owner chose to finish Orbit as a
  distributed systems project with zero operating cost (superseding the drafted
  bring-your-own-relay plan, which was never committed). No code changed.
