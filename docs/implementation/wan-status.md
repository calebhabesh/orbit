# Orbit native WAN implementation status

Updated: 2026-10-05. **Planning complete; implementation unstarted.** The owner
selected native WAN and confirmed preconfigured services with optional self-hosting.
The [master plan](../orbit-wan-implementation-plan.md), [UX](../orbit-wan-ux.md),
[architecture](../orbit-wan-architecture.md), [network protocol](../orbit-wan-protocol.md)
and [design gates](../orbit-wan-design-gates.md) form the implementation handoff.

First eligible packet: **W00**. No W runtime tests, gate experiments, service
deployments, dependency integrations or native WAN measurements have been executed.
Source research is supporting planning evidence, not packet acceptance.

The working tree contains substantial existing P/O/T changes. Preserve them;
record current provenance before implementation. [T13](terminal-status.md) retains
its incomplete technical conditions, and [P status](status.md) retains historical
P17 evidence and deferred owner use/explanation. W00/W01 can proceed; combined
release W17 requires the inherited relevant technical checks.

## Packet tracker

| Packet | State | Dependencies | Acceptance owner |
| --- | --- | --- | --- |
| W00 Baseline | pending | existing repository | [Foundations](wan-foundations.md#w00--baseline-and-reproductions) |
| W01 Contracts/gates | pending | W00 | [Foundations](wan-foundations.md#w01--contracts-and-initial-design-gates) |
| W02 Manager/HTTPS | pending | W01 | [Foundations](wan-foundations.md#w02--connection-manager-and-preserved-https) |
| W03 Rendezvous/profile | pending | W01, W02 | [Relay](wan-relay.md#w03--authenticated-rendezvous-and-profiles) |
| W04 Encrypted relay | pending | W01, W02, W03 | [Relay](wan-relay.md#w04--encrypted-wss-relay) |
| W05 Routed enrollment | pending | W02, W03, W04 | [Relay](wan-relay.md#w05--routed-enrollment-v3-and-peer-data) |
| W06 CLI setup | pending | W05 | [Relay](wan-relay.md#w06--automatic-cli-setup-and-scripting) |
| W07 TUI setup | pending | W06 | [Relay](wan-relay.md#w07--tui-onboarding-and-relay-milestone) |
| W08 LAN/public direct | pending | W02, W03, W05 | [Direct](wan-direct.md#w08--lan-discovery-and-reachable-direct-candidates) |
| W09 QUIC integration | pending | W01, W02, W08 | [Direct](wan-direct.md#w09--quic-http3-transport-and-integration-subgate) |
| W10 ICE traversal | pending | W03, W04, W09 | [Direct](wan-direct.md#w10--icestun-coordination-and-traversal) |
| W11 Roaming/policy | pending | W08, W09, W10 | [Direct](wan-direct.md#w11--route-policy-roaming-and-fair-progress) |
| W12 Diagnostics/control | pending | W06, W07, W11 | [Release](wan-release.md#w12--observations-diagnostics-and-controls) |
| W13 Operated services | pending | W03, W04, W10 | [Release](wan-release.md#w13--operated-defaults-and-self-hosting) |
| W14 Compatibility/packages | pending | W05, W06, W07, W12, W13 | [Release](wan-release.md#w14--migration-mixed-versions-and-packaged-defaults) |
| W15 Failure/resource campaign | pending | W11, W12, W13, W14 | [Release](wan-release.md#w15--integrated-failures-security-and-resources) |
| W16 Native WAN | pending | W07, W11, W13, W14, W15 | [Release](wan-release.md#w16--real-wan-and-ordinary-setup-campaign) |
| W17 Combined release | pending | W00–W16, relevant T13 technical checks | [Release](wan-release.md#w17--combined-release-and-handoff) |

## Gate tracker

| Gate | State | Owning packet |
| --- | --- | --- |
| WG1 Inner encryption/transport | pending | W01 |
| WG2 Registration/profile/privacy | pending | W01 |
| WG3 Routed enrollment | pending | W01 |
| WG4 QUIC/ICE composition | pending | W09 transport subgate, W10 full integration |
| WG5 Connection/roaming policy | pending | W02 model, W11 production evidence |
| WG6 Operated defaults | pending | W13 |

## Requirement coverage

| Requirement | Packets | Required evidence |
| --- | --- | --- |
| U06/S23 Automatic address discovery | W02–W03, W08, W11 | Authenticated candidate expiry and direct/relay routing |
| S24 Relay confidentiality/isolation | W01, W04–W05, W15 | Inner TLS, wrong pins, opaque broker streams, per-folder/control isolation |
| S25 Direct traversal/fallback | W08–W11, W15–W16 | Supported direct success, blocked UDP relay and network-change recovery |
| U17/S26 Hosted/self-host modes | W03, W12–W14, W16 | Actual signed profile/operator plus self-host/privacy behavior |
| U18/S27 Simple CLI/TUI onboarding | W05–W07, W14, W16 | Real first-time WAN approval/capture/transfer without manual addresses |
| S28 Bounded operable networking | W02–W04, W10–W17 | Security/resource/failure/real-network measurements and diagnostics |
| Existing S01–S22 / I01–I28 | All changed paths, W15–W17 | Retained version/identity/history/authority/recovery and baseline release checks |

## Planning record

Commands executed in this planning session: repository status/file inventory,
targeted reads of owning docs and transport/enrollment implementation, Python UX
guideline queries, official source/documentation browsing and documentation
consistency checks recorded below after execution. No source code or dependencies
were modified. New notes: [WAN assessment](../research/orbit-native-wan-assessment-2026-10-05.md)
and [transport options](../research/orbit-wan-transport-options-2026-10-05.md).

Planning validation: **passed** `python3 /tmp/orbit-wan-plan-check.py` (local links,
anchors, whitespace, packet/dependency agreement, acyclic ordering and coverage)
and `git diff --check`. The exact validator and output are retained in
[planning evidence](../evidence/wan-planning-20261005/commands.md).
The source audit also checked TLS/HTTP3/WSS/ICE composition; integration experiments
remain pending. Runtime checks and all W acceptance commands: **unexecuted**.
Hosted operator origins, credentials,
profile signer and egress policy will be concretized in WG6/W13; no production
domain or service availability is invented by this plan.

## Per-packet completion record

Append an entry when work begins: state and assigned worker; prerequisite evidence;
changed files/modules; invariants and exact discovered tests; commands/results;
revision/snapshot/dirty provenance; evidence paths; remaining acceptance items;
limitations and next eligible packet. Retain failed reproductions and later fixes
without rewriting historical outcomes. A dependency marked complete with missing
relevant evidence remains a concrete gap to resolve before dependent acceptance.
