# Final-scope packets

Read the [plan](../orbit-final-implementation-plan.md), the
[tracker](final-status.md) and the [gates](final-gates.md) first. Source
locations are dated 2026-10-11 at `5e61ac2` and must be re-checked before editing.

| Area | Location |
| --- | --- |
| Signed service operations, directory, offers, pairing mailbox | `internal/rendezvous/service.go`, `control.go`, `pairing.go` |
| Relay admission and monthly budget | `internal/rendezvous/relay.go` (`attach`), `budget.go` |
| Operator binary and config | `cmd/orbit-net/` (`serve.go` `serveConfig`, metrics, alert) |
| Fixed service limits | `internal/network/limits.go` (`MaxServiceControls`), `internal/rendezvous/service.go` (`MaxRecords`) |
| Embedded release profile | `internal/network/release-profile.json`, `network.BuiltinProfile`, `ORBIT_DISABLE_PACKAGED_PROFILE`, `config.AdoptPackagedProfile`, `scripts/build_packages.go` |
| Route provenance after authenticated HTTP | `internal/network/manager.go` and W08/W11 observations |
| Doctor mapping | `internal/control/network_doctor.go` |

## R00 — Baseline, interim protection and gate scoping

Dependencies: repository. Change: tests, fixtures, tracker, gates; one
owner-confirmed configuration change on the VPS.

Required work:

- Record revision, Go version and deployed versions from memory/operator records.
- Read-only VPS snapshot of `orbit-net` metrics (directory records, controls,
  relay month bytes, STUN counts). Snapshot on 2026-10-11: 2 records, 2 controls,
  about 6.3 MB relayed this month, 56 STUN answers.
- **Interim protection, owner-confirmed:** back up `serve.json`, set
  `relay_month_bytes` to a small value (proposed 1 GiB), `orbit-net serve --check`,
  restart, verify metrics. Data relaying then stops for everyone once spent;
  pairing relays stay available. Record the rollback command.
- On a local fixture, measure an idle online device's control cost (bytes/hour)
  and confirm fixed capacity limits and what a full directory does to a new device.
- Scope RG1–RG4: candidates and the evidence each needs.

Acceptance evidence: transcripts in `docs/evidence/final-r00-<run>/`; interim
change recorded with backup and rollback, or marked "not applied" if the owner
declines.

## R01 — Route accounting (RG2)

Dependencies: R00. Change: `internal/network` observations, `internal/control`
network status, `cmd/orbit`, `internal/terminal`.

Required work:

- Count per peer and purpose completed peer HTTP requests and body bytes by
  actual socket provenance: `lan`, `wan_tcp`, `quic` (native or ICE), `relay`.
- In-memory counters with a durable daily rollup (owner-only, 90 days, no
  addresses or filenames).
- `orbit network status --routes`, TUI connection details and doctor show
  per-peer shares for 24 hours and 30 days, labeled "observed by this device".
- A machine-readable export (`--json`) used by R05.

Acceptance evidence: classification tests per route class on a two-device
namespace fixture; rollup survives restart; rendering tests; overhead on a
1-GiB transfer recorded.

## R02 — Service admission allowlist (RG1)

Dependencies: R00. Change: `cmd/orbit-net`, `internal/rendezvous`,
`internal/protocol` (refusal code only), operator guide.

Required work:

- `serve.json` gains `admission` (`open` default, or `allowlist`) and
  `allowed_devices` (device ID and key pin pairs, bounded, e.g. 64), or an
  owner-only `allowed_devices_file`. Reloaded on SIGHUP like the certificate.
- In `allowlist`, signed operations from unlisted devices are refused before
  allocation with the typed code chosen by RG1. Scope per RG1: directory
  announce/lookup, offers, relay reservation and attachment, pairing mailbox.
  Unauthenticated STUN stays rate-limited as today unless RG1 decides otherwise.
- New-device pairing per RG1 (proposed: an unlisted joiner may use only the
  enrollment path toward a listed inviter, under existing enrollment bounds; the
  operator then adds it with `orbit-net devices add`).
- `orbit-net devices list|add|remove` editing the allowlist file atomically,
  accepting the identity line printed by `orbit devices` or `orbit status`.
- Metrics: allowlist size and refusals by reason.

Acceptance evidence: fixture tests for listed/unlisted pairs on every covered
operation; enrollment path for a new device; SIGHUP reload; malformed list
fails `--check`; 2.3 client against an allowlisted service behaves as RG1 records.

## R03 — One-command self-hosting

Dependencies: R02. Change: `cmd/orbit-net`, packaging examples, operator guide, README.

Required work:

- `orbit-net init --origin https://<host-or-ip>:<port> [--stun <ip:port>] [--tls-cert … --tls-key …]`
  creates an owner-only directory with authority and service keys, a signed
  `self_hosted` profile, a self-signed certificate when none is supplied (IP
  origins allowed, as in the W13 Pi smoke), finite personal budgets, admission
  `allowlist` and `serve.json`. It prints the files to copy and the exact
  `orbit network set --mode self_hosted --profile-file … --service-roots …`
  command, and the `orbit-net devices add` step.
- Operator guide: "Run your own Orbit service" as a short path ahead of the
  full reference; firewall ports and egress guidance.

Acceptance evidence: `init` → `serve --check` → serve on a disposable netns host;
two disposable devices with direct routes blocked sync through it; tamper and
permission checks on generated files.

## R04 — Release builds without a default service (RG3)

Dependencies: R02. Change: `internal/network` embedded profile handling,
`scripts/build_packages.go`, setup/onboarding copy, `internal/terminal`, WAN UX.

Required work:

- Release packages ship without `release-profile.json` (or with an empty
  embedded profile) per RG3. Fresh setup offers **Local network** (default) or
  **Self-hosted service**; Automatic without a profile shows "No Orbit service
  configured" and the self-hosting step instead of waiting silently.
- Existing installs keep their reviewed stored selection. The owner's devices
  keep using the VPS service through that stored selection or by switching to
  `self_hosted` with the same profile, as RG3 decides, without losing identity,
  approvals, routes or history.
- Pairing-code and routed-invitation copy covers devices without a service.

Acceptance evidence: fresh-build setup tests (CLI, TUI, PTY) for both choices;
upgrade test from 2.3 with a stored release selection; owner-path rehearsal on
`orbit-trial` instances.

## R05 — Measurement campaign (RG4)

Dependencies: R01, R04. Change: scripts under `scripts/` or `tests/`, evidence.

Required work, per the RG4 matrix:

- **Connectivity:** direct-connection success and time to first authenticated
  request for LAN, home↔VPS, home↔phone hotspot (CGNAT), UDP blocked, and
  emulated symmetric NAT in namespaces; relay fallback time when direct fails.
- **Throughput:** large-file and many-small-file transfers over LAN, direct WAN
  and relay; chunk reuse for tail edits and the prefix-insertion negative case.
- **Correctness under faults:** partition and heal with concurrent edits (head
  and conflict agreement), receiver kill mid-file and resume, roaming between
  networks during a transfer.
- **Resources:** Pi RSS, CPU and descriptors at idle and during transfer.
- Reuse earlier evidence where conditions match and cite it instead of re-running.

Acceptance evidence: scripts, raw results and a summary table per category with
repetitions, conditions and failures; nothing run against the VPS beyond
ordinary owner-device sync through the allowlisted service.

## R06 — Write-up and release

Dependencies: R03, R05. Change: `docs/case-study.md`, README, portfolio bullets,
packaging, hosted deployment (owner-confirmed).

Required work:

- Case study: architecture, the main design decisions and their trade-offs,
  the R05 results with methods and limits, and what Orbit deliberately does not
  guarantee. Remove or date any claim the evidence no longer supports.
- README: what Orbit is, how to try it on a LAN and how to self-host.
- Hosted VPS, owner-confirmed: back up, deploy the R02 build, add the owner's
  device identities, set `admission: allowlist`, restore the normal relay budget
  for those devices, verify, record rollback.
- Package and install on PC/laptop/Pi; full uncached race, packaging and demo checks.

Acceptance evidence: doc review notes; deployment transcript; final check
results; tracker complete.
