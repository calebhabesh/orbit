# W16 safe runner preparation and physical-host inventory

State: **complete for reachable networks (2026-10-07)** — see the final section. The
historical preparation record below is kept as written.
The final local production-binary rehearsal and all 29 Python harness tests pass.
No runtime production code or existing deployed workload was changed in W16.
WG6 still gates hosted-default execution, wider distribution and W17 release.
T13 technical conditions and deferred P17 owner use/explanation are unchanged.

The owner confirmed that only the VPS is remote. The home laptop/Pi and Oracle
VPS can provide two physical replica networks: a device behind the home router
can connect outward. No additional remotely located personal device is required
for that topology. The read-only inventories record the laptop public route via
`wlp2s0`/home gateway, Pi via its home interface, and VPS via `enp0s6`/OCI gateway.
The laptop and Pi have active Tailscale; the VPS has active WireGuard/Tailscale.
Those installations were not disabled or altered. Public route lookup by itself
is not proof that all sync traffic avoids a VPN.

## Delivered work and actual validation

The [runner guide](../../../scripts/validation/WAN_NATIVE.md) documents dependencies,
commands, topology declarations, admission and cleanup. The new runner collects
read-only host/network facts without activating Orbit. Its hosted journey checks
WG6 before any host operation, checks tunnel/route prerequisites before setup,
extracts a checksummed archive into a fresh marked private root and runs the
ordinary reviewed CLI create/invite/join/approve operations. It supplies no manual
replica address, profile file or membership in hosted mode. SSH forwarding/shared
masters and proxy fallback are disabled. The hosted path additionally checks the
binary/manifest embedded-profile digest, Automatic policy and system TLS trust.

The local fixture exercises that runner with independently provisioned production
CLI/service binaries, a private CA and a synthetic checksummed archive. It is
explicitly self-hosted, runs on one workstation and contacts no hosted service.
Two-way small files, a logical 4 MiB version, an offline-captured version/reconnect
and distinct second-folder approval/transfer pass. Exact file hash, version,
author, receiver stored receipt, receiver working bytes/readiness, device identity
and public SPKI pin checks pass. A remote `applied=false` remains visible rather
than being manufactured into an applied acknowledgement. Retries retain the
reviewed operation and attempt. Successful roots were removed only after exact
owned-daemon stop and live-process absence checks; final cleanup has no errors
and no retained roots.

[Final race execution](runner-process-race.json) exits 0 in 119.935 seconds;
`GOFLAGS=-race` applies to the helper `go build` commands as well as the Go test
process. [Sanitized journey record](runner-process-race-report.json) retains
archive/executable hashes, versions, qualified status, receipt/version oracles
and the actual local observations (`relay`, `peer_data`, `recent` at both ends).
The before/after tested-source digest is identical:
`1e60df207d443eb80953b43f684811f23aab3e08457954a6b2544b4a27bd9366`.
[Test discovery](discovery.json) finds exactly one W16 production-binary fixture.
[Optimized harness execution](harness-final.json) passes 29 tests, including
10 W16 refusal/oracle tests. [Vet](vet-final.json) and
[whitespace](whitespace.json) pass. Exact commands and original failed attempts
remain in [the command index](commands.md).

Observed local fixture times: join-to-approved 27.93 s; small versions 9.04/4.89 s;
logical 4 MiB completion 5.69 s; reconnect verification 1.07 s; second-folder file
4.27 s. These are single-run development measurements, including polling and
scan latency. The initial journey uses ordinary setup settings; the inherited
restart helper runs with a 1 s reconciliation interval. The 4 MiB generator
repeats the same 1 MiB content and permits chunk deduplication. Logical completion
bytes/second are not network throughput. No physical WAN performance, default
reconnect deadline, native resource bound or universal NAT success is inferred.

## Retained experiments and limitations

The original `rehearsal` failed because the runner passed `--state` to `orbit
version`; the version command now runs separately. `rehearsal-version-fixed`
incorrectly waited for a remote applied report despite a valid stored receipt and
verified working bytes. `rehearsal-receipt-fixed` used the stopped-state legacy
identity command while a daemon held the state lock. The worker now extracts only
the public certificate and hashes SPKI locally with OpenSSL, without returning
private PEM or passing it to a subprocess.

`rehearsal-identity-fixed` and `rehearsal-final` reached reconnect but refused an
initial transient second-folder submit; the runner now waits for the same durable
operation/attempt to obtain its request. The initial owner stop observation also
hit the old 5 s worker ceiling; the W16 worker allows 15 s for graceful shutdown.
These are runner corrections; no production guarantee was changed. The initial
`runner-race` completed the journey/cleanup but the test's explicitly requested
report destination was relative to the wrong working directory. The final run
uses an absolute exclusive destination. Failed/refactoring attempts have no
acceptance credit, and source-before/after differences remain recorded.
Failed private synthetic roots were retained for diagnosis; no W16 daemon remains.

The [hosted-gate refusal](hosted-gate-refusal/wan-native.json) records exit 1,
zero host observations and no roots: WG6 cannot be bypassed with a CLI switch.
The [laptop/VPS inventory](native-inventory-final/wan-native.json) and
[Pi/VPS inventory](native-pi-inventory/wan-native.json) succeed as read-only
collection while explicitly reporting tunnel restrictions. Earlier inventory
is retained as an original observation. Hosted and physical acceptance fields
stay `unexecuted` in every artifact, including the successful local rehearsal.

Full `make check`, full race, fuzz, demo and release packages were **not repeated**
for this validation-runner slice. W15's final-source acceptance remains dated
historical evidence; it is not relabeled as W16 validation. No physical TUI,
public direct sync, public relay sync, forced UDP block, address/interface change,
disposable physical service restart, laptop/Pi/VPS forwarding/conflict/restore or
physical resource/throughput campaign ran here. School/corporate/CGNAT/IPv6 access
and static native bundled-profile expiry also remain unexecuted.

## Resumable handoff

1. Close WG6 with the W13-owned independently restored authority backup and
   actual received alert firing/recovery evidence. The owner's deferral remains;
   no backup or alert delivery is implied by W16 development.
2. Prepare dedicated replica environments on home laptop/Pi and the VPS that
   cannot use existing VPN links. Do not disable personal networking or restart
   the deployed shared service. Capture actual socket/packet routes during
   transfers; network labels and public route lookup are insufficient.
3. Run real release archives and bundled-profile first-time CLI **and keyboard
   TUI** create/invite/join/approve across home/VPS. Prove direct and relay
   separately, with protected heads/hashes and endpoint-qualified receipts.
4. Extend the native runner for disposable UDP-blocked relay, interface/address
   change and a separate restartable service instance. Add second folder and
   laptop/Pi/VPS forwarding/conflict/restore, timing/resources and service provenance.
   Retain inaccessible cases as unexecuted. Continue W16; W17 is not eligible.

The preservation check hashes all 2,630 inherited evidence files and compares
preexisting non-document source to W15's recorded final source. Results and link
checks are in [handoff verification](handoff-verification.json).

## Follow-up: WG6 alerting and isolation proposal

The owner selected ntfy alerts and inspect-then-propose host handling. The
[alert checker](wg6-alert/summary.md) is implemented and locally validated but
not deployed. The [host-change proposal](host-change-proposal.md) lists the
exact VPS/Pi changes, backed by a rootless [namespace rehearsal](netns-rehearsal.log).
Next: on approval, deploy alerting and run the drill (owner confirms receipt),
owner stores and verifies the authority backup, then bring up the namespaces and
extend the runner to execute inside them with rule-counter route proof.

Approved and executed the same day: alerting deployed and drilled on the VPS
(receipt awaited), VPS/Pi namespaces up, runner `netns` support with host-rule
route proof ([harness](harness-netns.log), [inventory](netns-inventory.log),
[local regression](rehearsal-netns-regression.log)). Next: once the owner
confirms receipt and verifies the authority backup, record WG6 closed in
`docs/orbit-wan-design-gates.md` and run the bundled-profile CLI journey with
`topology-netns.json`, then extend it with the TUI, UDP-block and roaming drills.

## Follow-up: native runs and defect fix (2026-10-07)

WG6 closed; hosted-default CLI journeys ran between Pi and VPS namespaces with
path accounting ([native runs](native-hosted/summary.md)). A relay-recovery
defect was diagnosed and fixed client-side ([fix](quota-fix/summary.md)); runs
14–16 pass forced relay and address change consistently. Next: keyboard TUI
journey, then the service-restart decision and laptop/three-host cases.

## Closeout (2026-10-07)

Keyboard TUI journey and the owner-approved service restart (run 19) plus the
laptop as a third host (run 20) pass, with resource samples
([details](native-hosted/summary.md#tui-service-restart-three-hosts-and-resources-2026-10-07)).
All disposable validation roots and the Pi/VPS namespaces were removed; the
owner removes the laptop namespace. School/corporate/CGNAT/IPv6 unexecuted.
Next packet: W17.
