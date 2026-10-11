# R00 summary: baseline, interim protection and gate scoping

Date: 2026-10-11. Revision `b2172e2`. Go 1.27.2. Raw data: `results.json`,
`logs/`.

## Baseline

| Item | Value |
| --- | --- |
| Repository | `b2172e2` (final-scope plan, pushed 2026-10-11) |
| Hosted `orbit-net` | 1.1.0 (`0da2e7d`), active since 2026-10-08 17:39 UTC, 0 restarts |
| Hosted relay budget | `relay_month_bytes` 2 TiB; 4 MiB/s aggregate, 2 MiB/s per device |
| Owner trial installs | `orbit-trial` 2.3.0 on PC (running, set up), laptop and Pi (stopped, not set up) |

## VPS snapshot (read-only, 05:48 UTC)

2 directory records, 2 controls, 0 relay sessions. 6,335,236 bytes relayed in
October (0.0003% of the budget), 57 STUN answers, no budget, expired or
untrusted refusals. The cumulative `invalid` (347) and `other` (375) refusal
counts since 2026-10-08 are not broken down by source. Two records plus two
controls is exactly one online device (two purposes). Only the PC's
`orbit-trial` daemon was running, which fits, but metrics carry no device IDs,
so this is not proof.

## Interim relay cap: not applied

On 2026-10-11 the owner asked whether the cap mattered if usage was only theirs.
The answer recorded: any 2.x release build embeds this service's profile, so the
budget is open to anyone until the R02 allowlist is deployed (R06). But 2 TiB
sits inside OCI's 10 TB/month free egress, so the cost is $0 either way; a lower
cap would only reduce exposure and could cut off the owner's own relayed
trials. The VPS was left unchanged, with no backup or rollback needed. Revisit
if relay month bytes approach the budget (the existing 80% alert covers this).

## Local fixture

Production `RelayRuntime` against the production `Service` on a local TLS
listener (`internal/rendezvous/final_r00_test.go`).

| Measurement | Result |
| --- | --- |
| Idle online device, 300 s window | 16,856 B in + 13,758 B out at the service socket: **≈ 367 KB/hour**, ≈ 8.4 MiB/day, ≈ 252 MiB per 30 days |
| Breakdown | 4 renewals (challenge + announce, 2 purposes × every 2 min) dominate. Control heartbeat (ping + ack per purpose every 25 s) is ≈ 52 KB/hour (30-s development run: 430 B, no renewal) |
| Fixed capacity | 128 records and 128 controls; each online device holds 2 of each, so **64 online devices** fill the service. Challenges: 128 global, 2 per actor |
| Full directory (127 pre-filled) | New device: one purpose admitted (128/128), the other refused `QUOTA_EXCEEDED` 3 times in 25 s (10 s retry + 5 s quiet period); device not ready; controls admitted independently (2). Ready 5.08 s after a slot freed |

Scope limits: the idle figure excludes STUN, peer lookups, dials and any relay
data, and counts a fresh TLS handshake per renewal as the client does today. It
is one device on loopback-class LAN; WAN packet overhead (TCP/IP headers) is
not included.

## Gate scoping

Recorded in `docs/implementation/final-gates.md` (each gate's "R00 scoping"
block). Main findings:

- **RG1:** the allowlist check belongs in `admitOrigin`, since the challenge
  request has no kind or purpose. The shared 128-slot challenge table needs an
  unlisted share or the pairing exception lets strangers lock out listed
  devices. 2.3 clients treat an unknown refusal code as non-transient and retry
  about 25 times/min, indefinitely. Every 2.x release keeps reaching the VPS until the
  allowlist ships or epoch 2 expires (2027-10-07).
- **RG2:** count at peer-request completion in `manager.go`, never service
  control traffic.
- **RG3:** stored release selections do not depend on the embedded file, as
  far as the code read shows (candidate 1). R04 must prove it with a build.
- **RG4:** existing namespace tooling and reusable W13/W16/E09 evidence are
  identified. Recreating namespaces on the Pi or laptop needs owner approval.

## Acceptance

| R00 item | Evidence |
| --- | --- |
| Revision, Go and deployed versions | Baseline table; `logs/vps-snapshot.log` |
| Read-only VPS snapshot | `logs/vps-snapshot.log`, `results.json` |
| Interim cap (owner-confirmed) or "not applied" | Not applied, owner's choice (above) |
| Idle control cost, fixed limits, full-directory behaviour | `logs/r00-fixture.log`, `results.json` |
| RG1–RG4 scoped | `final-gates.md` R00 scoping blocks |
