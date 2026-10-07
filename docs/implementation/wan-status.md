# Orbit native WAN implementation status

Updated: 2026-10-07. **W00–W17 complete for their recorded acceptance; the WAN plan is delivered.**
The [combined W17 release record](../evidence/wan-w17-20261007/summary.md) and its
[traceability matrix](../evidence/wan-w17-20261007/traceability.md) link every
packet, gate and requirement to evidence and list the open conditions. WG6 closed
2026-10-07 (live-received ntfy alerting; owner-attested Bitwarden authority-key
copy, restore check waived and unexecuted). W16 covers the reachable networks;
school/corporate/CGNAT/IPv6 remain unexecuted for lack of access. The owner
selected native WAN and confirmed preconfigured services with optional self-hosting.
The entries below are dated history; earlier "pending"/"open" statements in them
are superseded by later entries, not rewritten.
The [master plan](../orbit-wan-implementation-plan.md), [UX](../orbit-wan-ux.md),
[architecture](../orbit-wan-architecture.md), [network protocol](../orbit-wan-protocol.md)
and [design gates](../orbit-wan-design-gates.md) form the implementation handoff.

W12 now supplies **qualified network status, diagnostics and controls**. W01 contracts/adapters and local WG1–WG3 evidence
and W02 manual manager acceptance are recorded below. Manual manager, authenticated local service/profile and encrypted relay integration are implemented; ordinary TUI onboarding and local relay acceptance are complete; scoped LAN/direct TCP and isolated public IPv4/IPv6 fixtures pass; deployed release
profiles and packaged owner laptop/Pi transfers are recorded in W13/W14; physically separate-network WAN measurements remain unexecuted. Native HTTP3/QUIC and full authenticated ICE/STUN traversal now have W09/W10 local native/emulator evidence.
Source research is supporting planning evidence, not packet acceptance.

The initial W00 tree was clean; substantial existing P/O/T work is committed in
the recorded baseline revision. Preserve that source and evidence. [T13](terminal-status.md) retains
its incomplete technical conditions, and [P status](status.md) retains historical
P17 evidence and deferred owner use/explanation. W00–W11 are complete for recorded acceptance; combined
release W17 requires the inherited relevant technical checks.

## Packet tracker

| Packet | State | Dependencies | Acceptance owner |
| --- | --- | --- | --- |
| W00 Baseline | complete | existing repository | [Foundations](wan-foundations.md#w00--baseline-and-reproductions) |
| W01 Contracts/gates | complete | W00 | [Foundations](wan-foundations.md#w01--contracts-and-initial-design-gates) |
| W02 Manager/HTTPS | complete | W01 | [Foundations](wan-foundations.md#w02--connection-manager-and-preserved-https) |
| W03 Rendezvous/profile | complete | W01, W02 | [Relay](wan-relay.md#w03--authenticated-rendezvous-and-profiles) |
| W04 Encrypted relay | complete | W01, W02, W03 | [Relay](wan-relay.md#w04--encrypted-wss-relay) |
| W05 Routed enrollment | complete | W02, W03, W04 | [Relay](wan-relay.md#w05--routed-enrollment-v3-and-peer-data) |
| W06 CLI setup | complete | W05 | [Relay](wan-relay.md#w06--automatic-cli-setup-and-scripting) |
| W07 TUI setup | complete | W06 | [Relay](wan-relay.md#w07--tui-onboarding-and-relay-milestone) |
| W08 LAN/public direct | complete | W02, W03, W05 | [Direct](wan-direct.md#w08--lan-discovery-and-reachable-direct-candidates) |
| W09 QUIC integration | complete | W01, W02, W08 | [Direct](wan-direct.md#w09--quic-http3-transport-and-integration-subgate) |
| W10 ICE traversal | complete | W03, W04, W09 | [Direct](wan-direct.md#w10--icestun-coordination-and-traversal) |
| W11 Roaming/policy | complete | W08, W09, W10 | [Direct](wan-direct.md#w11--route-policy-roaming-and-fair-progress) |
| W12 Diagnostics/control | complete for CLI/TUI and local privacy/diagnostic acceptance | W06, W07, W11 | [Release](wan-release.md#w12--observations-diagnostics-and-controls) |
| W13 Operated services | complete: hosted service live; alerting received; owner-attested key copy (restore check waived) | W03, W04, W10 | [Release](wan-release.md#w13--operated-defaults-and-self-hosting) |
| W14 Compatibility/packages | complete for recorded acceptance, final-source validation passed (WG6 publication gate closed 2026-10-07) | W05, W06, W07, W12, W13 | [Release](wan-release.md#w14--migration-mixed-versions-and-packaged-defaults) |
| W15 Failure/resource campaign | complete for recorded acceptance | W11, W12, W13 technical evidence, W14 | [Release](wan-release.md#w15--integrated-failures-security-and-resources) |
| W16 Native WAN | complete for reachable networks; school/corporate/CGNAT/IPv6 unexecuted | W07, W11, W13, W14, W15 | [Release](wan-release.md#w16--real-wan-and-ordinary-setup-campaign) |
| W17 Combined release | complete for recorded release conditions (2026-10-07) | W00–W16, relevant T13 technical checks | [Release](wan-release.md#w17--combined-release-and-handoff) |

## Gate tracker

| Gate | State | Owning packet |
| --- | --- | --- |
| WG1 Inner encryption/transport | closed (local gate) | W01 |
| WG2 Registration/profile/privacy | closed (local gate) | W01 |
| WG3 Routed enrollment | closed (local gate) | W01 |
| WG4 QUIC/ICE composition | closed for local native/emulator composition | W09 transport and W10 full integration evidence |
| WG5 Connection/roaming policy | closed for production composition, local native/emulator and Pi acceptance | W11 production evidence |
| WG6 Operated defaults | closed 2026-10-07: operator, origin, TLS, release profile live; alerting received; owner-attested key copy (restore check waived) | W13 |

## Requirement coverage

| Requirement | Packets | Required evidence |
| --- | --- | --- |
| U06/S23 Automatic address discovery | W02–W03, W08, W11 | Authenticated candidate expiry and direct/relay routing |
| S24 Relay confidentiality/isolation | W01, W04–W05, W15 | Inner TLS, wrong pins, opaque broker streams, per-folder/control isolation |
| S25 Direct traversal/fallback | W08–W11, W15–W16 | Supported direct success, blocked UDP relay and network-change recovery |
| U17/S26 Hosted/self-host modes | W03, W12–W14, W16 | Actual signed profile/operator plus self-host/privacy behavior (W14: packaged release profile, reviewed operator switch) |
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

## W00 — Baseline and reproductions

Packet/state: **W00 / complete**, 2026-10-05. Worker: active Codex session;
no agent delegation. Existing repository prerequisite, required WAN and owning
specifications, current T13/P17 evidence and Makefile inspected. Initial tree
clean; revision and every tracked file hash recorded before edits in the
[manifest](../evidence/wan-w00-20261005/manifest.json). No snapshot or remote
workload changes.

Changes: [baseline inventory/source ownership](wan-baseline.md), focused
`tests/terminal/wan_baseline_test.go`, packet evidence and this tracker. Production
code, dependencies, schemas, identities and P/O/T records remain preserved.
Existing T04 CLI and T05 process fixtures supply real manual-network sync and
address-change reproduction rather than duplicating their production harness.

Executed: nonzero CLI/control/replication/terminal discovery; uncached
CLI/control/contract/replication checks; T04 two-device CLI interrupted join and
bidirectional edits; T05 three-process sharing/forwarding/address-change failure
and manual anchor-preserving refresh; new W00 missing/unusable address test,
also under race. All passed. `make check` passed, including terminal/package tests, format/vet,
CLI, integration/model/fault suites, 12 Python safety/VT checks, amd64/arm64 builds
and packaging. Local-link/anchor, dependency, discovered-test and original-file
preservation checks and `git diff --check` passed. Exact commands/results live in
[W00 evidence](../evidence/wan-w00-20261005/summary.md).

Invariant scope: existing I01–I28 suites retained; focused fixture checks
I07/I08/I22 preservation and N01/N05's manual baseline, without claiming WAN
routes. Inventory maps new N01–N10 acceptance surfaces and every dependent W
packet's baseline prerequisites/repair ownership.

Current inherited gap: actual native login/logout/unattended boot remains T13.
Its 2026-10-05 follow-up satisfies ordinary three-host Tailscale networking;
earlier limitations are historical. P17 owner use/explanation remains deferred.
W00 local nonloopback processes do not establish physical LAN/WAN, NAT traversal,
operated services, privileged harness or power-loss behavior. Safe network-harness
requirements and real-host resource boundaries are recorded in the inventory.

Next eligible: **W01 — Contracts and initial design gates**.
WG1–WG3 experiments, schemas/goldens and WG5 model freeze remain required before
dependent transport production changes; WG4/WG6 retain W09/W10/W13 ownership.

## W01 — Contracts and initial design gates

Packet/state: **W01 / complete**, 2026-10-05. Active Codex worker, no delegation.
Prerequisite W00 baseline manifest/summary/source map and required owning docs read;
initial dirty/untracked W00 tree recorded and preserved in the
[manifest](../evidence/wan-w01-20261005/manifest.json). No snapshot, live host workload,
identity/history/config/schema migration or public service activation.

Changed: `internal/network` bounded direct/WSS adapters, transport seam and finite
limits; additive `internal/protocol` network/v3 canonical contracts; additive
terminal network policy/query/review/private setup types; independent `model`
admission/enrollment/connection oracles; focused replication fixtures;
[strict schema](../../schemas/network-v1.md), independent goldens, one pinned
WebSocket dependency/license notices and [owning gate outcomes](wan-contracts.md).
No WAN capabilities advertised merely from defining their types.

WG1–WG3 **closed for local design composition**, with executed independent models
and real pinned TLS/production peer and v2 enrollment handlers, plus an explicit v3
gate fixture. First relay spike failed at HTTP background-read deadline reset;
retained reproduction and bounded read-pump repair are documented. Wrong inviter
and target pins, unknown requester data denial, missing mTLS, opaque broker bytes,
control/purpose isolation, redirect/plaintext refusal, slow receiver/cancellation,
frame limits and cleanup pass. Independent signed canonical fixtures cover strict
JSON, every integer representation, profile/service-token bindings, maximum-valid
and one-over cases. Exact existing v1 membership artifacts and retirement checks
remain authoritative. V2/manual coexistence and terminal T03/T04/T05 journeys pass.

Validation: nonzero W01 discovery in network/protocol/replication/terminalcontract/
model; focused uncached checks and race; uncached existing CLI/control/internal/model
and terminal enrollment/sharing compatibility; vet; CGO-free linux amd64/arm64
compilation; fixture reproducibility, local links/anchors, preservation and
`git diff --check`. Exact commands/results in
[W01 evidence](../evidence/wan-w01-20261005/summary.md). Original filtered compatibility
command matched no CLI/control tests; the separately recorded full uncached command
runs those packages without a filter. No zero-match invocation establishes coverage.

Invariant scope: I08–I09/I13/I19–I24 and local N01–N06 contract/adapter checks.
N07 QUIC/ICE remains unexecuted; defining its eventual capability does not support it.
WG5's initial bounded generation/cancellation/flapping model and finite defaults
are frozen; W02 integration and W11 production roaming/timing remain required.
WG4 stays W09/W10, WG6 W13. Production directory/relay/v3 durable resume,
DNS/rebinding implementation, native WAN/NAT/Pi resource campaigns and hosted
operator/signing custody remain unexecuted. Local spikes are not those claims.
T13 native lifecycle checks and deferred P17 owner activities remain unchanged.

Next eligible: **W02 — Connection manager and preserved HTTPS**. Integrate the
frozen Target/replication-TLS/RoundTripper seam into one daemon-owned manager;
retain manual peers.json policy, membership checks, retry identities and deadline
bounds. Join manager work before SQLite shutdown and demonstrate M0 production
manual sync/fair progress through that seam with focused race and `make check`.

## W02 — Connection manager and preserved HTTPS

State: **complete**. All W02 acceptance items have linked local production/manual
and focused lifecycle evidence; native WAN acceptance remains with later packets.
Dependencies: W01/WG1–WG3 local gates and W00 baseline prerequisites inspected.
Invariants: retained I01–I09/I13–I15 request/history paths, manual N01/N02/N05/N06
checks; full native N01–N10 and final WG5 closure remain later packets.

Implemented one daemon-owned manager shared by scheduler, live enrollment,
bootstrap and membership-artifact clients. Replication supplies pinned TLS trust;
existing handlers retain folder/revision/retirement authority. Manual v1 endpoints
remain explicit and are reloaded without rewriting peers.json or approved pins.
The private policy scaffold defaults to manual/generation 1 without a state write;
unimplemented automatic/local-discovery modes fail explicitly. No new runtime
network capability is advertised and stopped/live controller selection is intact.

Pool/request/route/observation bounds and generation draining are specified in
[architecture](../orbit-wan-architecture.md#w02-manual-manager-integration).
Requests cannot select another destination; redirects/proxies/plaintext fail.
Cached roots/client identities cannot change and each borrowed verifier runs
before HTTP writes. Idle LRU eviction retains imported targets; eight bounded HTTP
callers per target support existing four-worker chunks, while two TCP sockets per
pool provide backpressure. Typed busy/stale errors use existing retry budgets.
Independent 32-global/two-target pending-dial and 64-global/four-target owned-socket
bounds survive generation flapping and detached net/http work. Shutdown
cancels/joins requests and dials, closes bodies/sockets/pools before SQLite.

Executed evidence: nonzero W02 discovery; focused race; manual two-way transfer,
interrupted verified-chunk reuse, exact head/hash replay, wrong pin/unknown member,
10,000 generation events against the independent model, purpose/draining pool caps,
slow-peer isolation and pending-dial/body shutdown. Production T04 CLI restart and
T05 three-process forwarding/address refresh passed; background 16-MiB transfer
progressed alongside ordinary edits. Full uncached CLI/control/replication/config/
network/scheduler/model compatibility passed. Final production-source `make check` passed: terminal/extracted packages, formatting,
vet, CLI/internal/model, integration, fault/design-gate checks, 12 Python harness
checks and CGO-free amd64/arm64 binaries/packages. Documentation/link/discovery
and preservation checks passed for all 12 W02 race tests and 2148 initial files
outside declared owning changes; frozen Go source hashes and `git diff --check`
passed. Exact commands/results are recorded in [W02 evidence](../evidence/wan-w02-20261005/summary.md).

Unexecuted: discovery/service/profile/lease integration, relay selection,
QUIC/ICE/STUN, direct/relay races and native interface-change roaming, physical LAN/
WAN/NAT/Pi/resource/operator campaigns. Those remain W03–W17; WG4/WG6 stay pending,
WG5 remains open for W11 native policy/timing evidence. T13 native lifecycle checks
remain outstanding; P17 owner use/explanation stays deferred. All new checks use
synthetic disposable roots/listeners; no personal folder or existing VPS workload.

Next eligible: **W03 — Authenticated rendezvous and profiles**.


## W03 — Authenticated rendezvous and profiles

State: **complete** for the selected local production-service acceptance.
Prerequisites W01/W02 and WG2 checked; scope, glossary, WAN UX/architecture/protocol,
security/operations/persistence/verification and prerequisite evidence read.
One worker preserved the existing dirty P/O/T/W00–W02 source/evidence.

Delivered `cmd/orbit-net`, `internal/rendezvous`, signed `network.ProfileSelection`,
bounded `network.ServiceClient` and private `config/network_profile.go` persistence,
focused service/client/replication tests and owning architecture/protocol/schema/
operations/persistence/verification updates. Invariants: I08–I09, I13, I20, I23,
N01–N03, N06–N07 and N09 on the W03 paths.

Actual results: **24 discovered W03 tests pass under race**; 20 repeated actual
service-binary restart tests pass; uncached compatibility and strict protocol/model
suites pass; real manual CLI interrupted join/restart, three-peer forwarding/address
refresh and background capture/sync journeys pass. Final production-source `make check`
passes terminal/package/format/vet/unit/integration/model/fault/harness/build checks.
Standalone service static amd64/arm64 builds, local-link/discovery/preservation
validation and `git diff --check` pass. Twenty shared-source keys register; the
1,000-canceled-call fixture measures resolver peak four/zero remaining sockets;
272 raw connections measure the pre-TLS 256-goroutine admission ceiling.

[Summary](../evidence/wan-w03-20261005/summary.md),
[commands](../evidence/wan-w03-20261005/commands.md),
[results](../evidence/wan-w03-20261005/results.json),
[manifest](../evidence/wan-w03-20261005/manifest.json).
Owning [architecture](../orbit-wan-architecture.md#w03-directory-and-profile-integration)
records exact bounds/retention; signed canonical W01 fixture bytes are unchanged.
Initial fixture/cache/compile/clock failures and the repaired close-under-lock bug
are retained with their final passing reproductions; no failed run counts as passing.

Limitations: all service/TLS/DNS/process and synthetic-source NAT fixtures are on
one development host. Hosted operator/profile/signing custody, infrastructure
retention/monitoring, native WAN/NAT/Pi resource measurements remain W13/W15/W16.
W03 delivers accepted reservation **intent**, not a broker forwarding endpoint.
Relay attachment/streaming is W04; v3 enrollment/routed data and reviewed setup
activation are W05–W07; direct/QUIC/ICE/roaming are later. The existing daemon still
activates manual/no-advertising policy only, and saving a profile never activates
public networking or a reserved capability. Strict expiry windows do not extend
authorization under clock skew. T13 native login/logout/unattended boot remains
outstanding; P17 owner use/explanation stays deferred.

Next eligible: **W04 — Encrypted WSS relay**. Integrate the existing W01 BinaryStream/
Forward adapters and W03 accepted partner/epoch/session/purpose intent into bounded
live attachment/forwarding admission. Require fresh proofs, live exact leg credentials,
separate unknown enrollment/data limits, measured quotas, and immediate close/join
on expiry/error/cancel; routing acknowledgements are never file receipts.

## W04 — Encrypted WSS relay

State: **complete** for the packet's local production-service/engine acceptance.
One Codex worker, no delegation; W01/W02/W03, WG1/WG2, scope/glossary and owning
UX/architecture/protocol/security/persistence/verification evidence inspected.
Original dirty source/evidence is retained; W04 manifest records provenance.

Delivered live authenticated broker attachment/forwarding, bounded service client
and purpose-specific offer/accept coordinator, manager logical routes and typed
observations/retry classification, standalone operator quota flags and isolated
daemon virtual TLS data/enrollment listeners. No content/repository/owner-control
dependency enters the broker. Manual startup remains no-advertising; saving a
profile does not enable WAN use. V2 handlers/signed bytes and folder authority stay
in their owning modules; v3 durable setup remains W05.

Executed: **23 discovered W04 tests pass under race**, with full authenticated
production-service two-way file transfer, exact heads/hashes/authors, interrupted
verified-chunk reuse, unknown pending enrollment and folder/control/pin isolation.
Twenty actual local service epoch/reannouncement/chunk-resume repetitions pass.
Slow reader/idle/lifetime/byte/rate/cancel/leg loss, frame bounds, role replay,
restart tokens and bounded capacity pass. Synthetic broker-boundary captures contain
no readable filename/content/invitation token. End-device receipts remain separate
from routing acknowledgements; partial content is never published ready.

Capacity: accepted reservation state is explicitly seeded for 64 data plus two
enrollment tunnels, then actual fresh proof/token attachment and forwarding run.
Complete offer/accept/control journeys are separately exercised by production sync
fixtures. Actual heap/FD/goroutine/8-MiB throughput samples and joined baseline are
retained in [evidence](../evidence/wan-w04-20261005/summary.md). This is whole local
test-process measurement, not isolated broker/Pi/native-host capacity.

Uncached compatibility and manual T04/T05 CLI/three-process journeys pass; the
background daemon transfers a 16-MiB archive while eight edits are produced.
`make check` passes terminal/package/format/vet/unit/integration/model/fault/harness
and amd64/arm64 package/build checks; standalone service cross-builds pass.
Documentation/discovery/frozen-source/preservation checks and `git diff --check`
pass. Exact commands, final logs and initial failures/repairs are in
[commands](../evidence/wan-w04-20261005/commands.md),
[results](../evidence/wan-w04-20261005/results.json) and
[manifest](../evidence/wan-w04-20261005/manifest.json).

[Architecture](../orbit-wan-architecture.md#w04-encrypted-relay-integration)
records finite quotas, token-versus-tunnel lifetimes, opaque hard drain ceiling,
shutdown and context-trace ownership. The production sync experiment exposed and
repaired peer HTTP tracing on outer TLS; unknown acceptance now requires an exact
live offered session/control rather than an unnecessary public announcement.
Both changes have passing focused reproductions; failed runs count for no acceptance.

Limitations: one Linux development host, explicit development profiles/CA, disposable
synthetic roots/listeners; restart repetitions replace real local HTTP-service state,
not OS processes or physical interfaces. No hosted/default operator/profile, native
WAN/NAT/Pi peak RSS/CPU, monitoring or packaged service deployment evidence is claimed.
Daemon virtual handlers are integrated; ordinary policy activation/durable logical
peer routes remain W05–W07. The coordinator uses caller-owned announcements/generations;
automatic reconnect/roaming remains W11. WG4/WG6 and native WG5 evidence remain open.
T13 native login/logout/unattended boot stays outstanding, P17 owner use/explanation
deferred. Existing P/O/T/W00–W03 evidence is preserved.

Next eligible: **W05 — Routed enrollment v3 and peer data**. Implement exact signed
v3 admission/approval, durable same-attempt/root/profile resume and both logical
pull routes through this transport, with canonical membership/retirement bootstrap.
Preserve v2/manual contracts, verify inviter before capability disclosure and retain
unknown-data denial. W06/W07 provide ordinary reviewed CLI/TUI WAN activation.

## W05 — Routed enrollment v3 and peer data

State: **complete for local production-service and daemon acceptance**. W02–W04,
WG3, the existing membership/retirement transaction, and the v2 enrollment
transcript were preserved. V3 uses separate `/enrollment/v3/*` envelopes and
canonical bytes; v2 paths and signed bytes remain unchanged.

Delivered exact routed invitation/request/status/approval types, private durable
peer routes, v3 challenge/request/status handlers, signed approval artifacts,
profile-bound relay enrollment, route registration for both pull directions and
daemon recovery from durable setup intent. Capability bytes are not retained in
ordinary inspection or durable accepted records. A new route cannot replace a
known device pin/profile; repeated route registration is idempotent.

Executed focused evidence:

- `go test ./internal/replication -run '^TestWANW05' -count=1`: exact replay,
  revocation/expiry, wrong folder/route/signature, v2 downgrade, status possession
  and nonce reuse, wrong inviter TLS before disclosure, unsent expiry, single-use
  capability, retired identity, approval artifact binding and pending data/control
  isolation pass.
- `go test ./internal/rendezvous -run '^TestWANW05' -count=1`: quota refusals
  consume their proof challenge and do not strand admission slots.
- `go test ./internal/config -run '^TestWANW05' -count=1`: private bounded route
  persistence, legacy `peers.json` preservation, idempotent replay and pin/profile
  overwrite refusal pass.
- The real terminal routed journey passes reviewed setup, two folders, restart
  and two-way bytes/heads/hashes. The marked-root daemon campaign passes SIGKILL
  at request-prepared, request-accepted, owner-approval and membership-received
  boundaries, preserving device key, attempt, root, operation and reverse data.
  The third-device/offline rollout, fork distinction and retired-history bootstrap
  fixture passes with canonical membership/retirement snapshots.
- Compatibility packages (`internal/config`, `replication`, `rendezvous`,
  `network`, `control`, `app`) and `git diff --check` pass. The full W05 terminal
  command and integrated `make check` were interrupted after the long terminal
  fixture phase; their individually
  completed routed and daemon campaigns are recorded, and no interrupted command
  is counted as acceptance.

Evidence is in [wan-w05-20261005](../evidence/wan-w05-20261005/summary.md).

Limitations: fixtures use one Linux development host, disposable marked roots,
development profile/CA and local service endpoints. This closes routed enrollment
and peer-data behavior through the existing relay; it does not claim hosted/default
operator readiness, physical WAN/NAT/Pi capacity, QUIC/ICE, roaming, or T13 native
login/logout/unattended boot. P17 owner use/explanation remains deferred. W06/W07
still own ordinary automatic CLI/TUI activation.

Next eligible: **W06 — Automatic CLI setup and scripting**.

## W06 — Automatic CLI setup and scripting

Status: **complete** on 2026-10-05. Implementation, focused acceptance and the
clean final `make check` aggregate pass. [Evidence summary](../evidence/wan-w06-20261005/summary.md),
[commands](../evidence/wan-w06-20261005/commands.md) and
[manifest](../evidence/wan-w06-20261005/manifest.json) record the dirty W00–W05/P/O/T
provenance and one-host disposable service assumptions.

Implemented fresh Automatic setup without separate init/serve, reviewed local-only
selection before announcements, retained explicit/manual installations, private
v2/v3 invitation/stdin input, named folder invitations and exact device/request
approval reviews. The retained `invite create` verb follows the reviewed routed
policy while keeping manual v2 transfer labels and bytes. Secret argv is rejected;
normal named invitation JSON omits its capability. Existing review-file/apply and
operation-ID retry paths retain root/name/request/identity under waiting and quota.

Network status/policy/profile controls are additive to the shared authenticated
terminal client/controller and durable ledger. Preview binds current state and
exact intended trust/policy; replay and monotone writes preserve newer reviewed
intent through interruption. Desired/active policy and restart requirements are
separate from cached service readiness, actual dated routes and existing copy
receipts. Missing/invalid profiles leave local capture/control running. Routed
modes refresh reconciliation every five seconds by default; manual stays at five
minutes, and explicit interval overrides remain. W11 owns native timing/fairness.

Executed focused acceptance:

- `go test -race ./internal/config ./internal/control/... ./cmd/filesync -count=1`:
  review, replay, stale/changed intent, monotone generation, accepted-before-effect
  recovery and unchanged identities pass. Network/shared CLI race checks also pass.
- `go test -race ./tests/terminal -run '^TestWANW06' -count=1 -v`: production-binary
  scripted create/invite/join/relaunch/approve and verified files in both directions;
  matching version author and inviter certificate/pin; second folder, private stdin,
  delayed and quota-blocked approval, real PTY guided setup/approval, wrong pin,
  expired fresh invite, private permissions/redaction, non-TTY input rejection,
  incomplete root review, missing profile and service outage/local capture pass.
- `go test ./tests/integration -run '^TestOrbitPairing_CLI' -count=1 -v`: stopped and
  running manual compatibility pass after their reviews explicitly select Manual.
- Canonical protocol/control fixtures, `go vet ./...`, relative documentation file
  links and `git diff --check` pass. The first full aggregate timed out at ten
  minutes and is uncredited. Its next run passed all terminal checks (784.945s),
  package checks, formatting, vet and internal tests before finding the manual
  fixture assumption. The thirty-minute terminal target retains all test cases;
  the final clean `make check` run passes the entire terminal campaign, packages,
  formatting, vet, CLI/internal tests, integration, models, design-gate/fault tests
  and all twelve Python validation-harness tests. Final log:
  [make-check-complete.log](../evidence/wan-w06-20261005/logs/make-check-complete.log).

Limits: these are signed local-development services with separately trusted TLS
on one Linux host, not deployed defaults or physical WAN/NAT evidence. Local-only
currently blocks daemon/manual network work and reports LAN discovery unimplemented;
W08 still owns supported local discovery/direct behavior. QUIC/ICE/STUN, roaming,
expanded doctor/status, operated profiles, distribution and real networks remain
later packets. T13 login/logout/unattended checks remain incomplete, and P17 owner
use/explanation remains deferred. No release or universal reachability claim.

Next sequential packet: **W07 — TUI onboarding and relay
milestone**, through the same reviewed controls. W08 is independently eligible
under the dependency table; default assignment remains sequential.

## W07 — TUI onboarding and relay milestone

Complete: 2026-10-05. [Evidence and handoff](../evidence/wan-w07-20261005/summary.md).
Fresh setup reviews Automatic/privacy or Local-only before confirmation;
existing policy is preserved. Ordinary keyboard forms omit address/port prompts,
with legacy settings under Advanced. Private v2/v3 input, measured root review,
exact request/code approval, retained retry identities, durable revocation and
progress use the same authenticated controls as CLI. Connection details separate
cached service/dated routes from local readiness and stored/applied copies.

Actual validation:

- `go test -race ./internal/terminal ./internal/controlclient ./cmd/filesync -count=1`
  passes; five focused W07 tests are discovered, plus one real PTY acceptance test.
- `ORBIT_W07_PTY_EVIDENCE=... go test ./tests/terminal -run '^TestWANW07' -count=1 -v`
  passes (142.840s). Production binaries verify both transfer directions, existing
  file edits, exact version/hash/author and saved inviter identity; second-folder
  consent/approval; delayed exit and daemon restart retaining request/attempt/root;
  revocation, wrong pin, expiry, root correction, paste, narrow/colorless resize,
  terminal restoration and local capture after interface exit/service outage.
- Canonical protocol/control-contract/model checks pass. Full `make check` passes
  all targets, including terminal (925.129s), package/format/vet/internal tests,
  integration, models, design gates/faults, twelve Python harness tests and release
  build/package targets. [Final log](../evidence/wan-w07-20261005/logs/make-check-final.log).
- Documentation links, initial source preservation and `git diff --check` pass.
  Initial fixture failures and the first aggregate's hex-digest oracle failure
  remain recorded without acceptance credit.

Limits: signed local-development services on one Linux host; hosted defaults,
physical WAN/NAT/Pi and native lifecycle are unexecuted. The successful extra-folder
journey keeps both daemons alive and allows 65 seconds for the existing enrollment
bucket to refill. Earlier restart-only setup/transfer experiments are uncredited;
W11 retains generation/route-change recovery evidence. W08–W17, WG4/WG6,
T13 native login/logout/boot checks and deferred P17 owner use/explanation remain.
Existing P/O/T/W00–W06 source and evidence are preserved.

Next sequential packet: **W08 — LAN discovery and reachable direct candidates**.


## W08 — LAN discovery and reachable direct candidates

Complete: 2026-10-06 for production integration and local/isolated fixture acceptance.
[Evidence and handoff](../evidence/wan-w08-20261006/summary.md) retain all initial source
hashes, actual commands and failed runs. Signed scoped LAN discovery, actual optional
TCP ports and public IPv4/IPv6 gathering, independent expiring interface leases,
known-pin TLS family races and Local-only known-peer sync are integrated in the
production daemon. Reviewed advertising choices and existing manual intent persist;
folder approval, chunk/receipt semantics and owner-control isolation are unchanged.

Actual validation: 15 W08 tests discovered; focused race/resource checks pass, with
actual multicast two-way heads/hashes and verified-chunk reuse, native local ULA
IPv6 transfer and separately executed marked public-scope IPv4/IPv6 namespace fixture.
The two race-instrumented production-binary journeys each pass twice (173.308s):
explicit invitation/pending denial/approval before LAN, zero Local-only service requests,
matching versions/keys, outage capture and fresh occupied optional-port relay sync;
mandatory manual collision fails startup. Bounds exercise 32 raw dials, 64 incoming
sockets and 1,000 datagrams with FD 6/8/6 and goroutines 2/2 before/after. Broader
network/protocol/replication/config/control/TUI/CLI race checks, W06/W07 real binary/PTY
compatibility and focused Make aggregate plus amd64/arm64 packages pass. Full
`make check` is not rerun for this intermediate packet; W07's complete log is preserved.

Limits: one development host, signed development services, native local ULA IPv6 and
simulated public addresses in a new isolated namespace. No public internet/multi-host/
Pi/operated-default claim. IPv6-only LAN multicast is absent; fresh Local-only pairing
uses configured isolated legacy local enrollment. Failed restart-to-relay/quota runs
remain uncredited and owned by W11; fresh startup with occupied ports is W08's
collision oracle. WG4/WG6, native WG5 timing/roaming/fairness, T13 login/logout/boot
and deferred P17 owner use/explanation stay outstanding. Existing evidence is preserved.

Next sequential packet: **W09 — QUIC HTTP3 transport and integration subgate**.
Inspect actual supported APIs/licenses, prove native pinned UDP HTTP3 and both pull
directions with explicit request/body/header/deadline limits, and close WG4's transport
subgate before full ICE composition. Preserve W08 TCP/LAN and W11 failed-run evidence.

## W09 — QUIC HTTP3 transport and integration subgate

Complete: 2026-10-06 for production native transport and single-host acceptance.
[Evidence and handoff](../evidence/wan-w09-20261006/summary.md),
[commands](../evidence/wan-w09-20261006/commands.md),
[dependency audit](../evidence/wan-w09-20261006/dependency.md) and manifest retain
original dirty source, actual failures and all previous evidence. WG4 transport
subgate is closed with quic-go v0.63.0 / qpack v0.6.0 and pinned Pion ICE v4.4.6
API compatibility; full ICE remains W10.

Delivered optional actual-port UDP HTTP3, pinned replication TLS and unchanged
peer/membership handlers, one packet transport serving/dialing both pull directions,
strict additive capability/signature fixture, numeric frozen packet adapter and
explicit HTTP3 header/body/request deadlines, no early data and finite resources.
Local-only UDP uses a selected concrete address and same-prefix filtering; old
advanced direct settings remain valid. Enrollment/control stay isolated.

Actual validation: 15 focused tests discovered in five packages; network/protocol/
config checks and final native/pair-adapter replication checks pass twice under race.
Both pull directions verify heads/hashes/authors and interrupted chunk reuse under
packet loss/duplication; partial chunks yield no verified content/publication/receipt.
Missing/wrong TLS identity, ALPN, unknown membership/folder/version, header/body
bounds, actual preparse/body timeouts, 32-session admission, cancel/close and sampled
slow-stream heap/backpressure pass. Unavailable UDP preserves pinned TCP transfer.
Production-binary Local-only QUIC passes twice (81.669s), with exact approval,
dated QUIC routes, matching two-way files/identities, zero service requests and
capture after actual service outage. Valid UDP/TCP occupied listeners preserve
fresh relay onboarding; W08 TCP/manual collision passes (132.528s combined).
W07 real PTY compatibility passes under race (156.522s). Final broader race suites,
focused Make aggregate, amd64/arm64 packages/extracted-package PTY checks and pinned
adapter/service cross-builds pass; source/evidence/link/whitespace checks pass.

First binary runs exposed the all-required signed decoder's incompatibility with
additive local settings. The repaired loader/legacy regression and repeated binaries
pass. The older listen-only collision fixture now supplies valid settings; its
new passing reproduction preserves all historical W08 records. Full make check is
not rerun for this intermediate packet; W07's complete aggregate is retained.

Limits: actual UDP on one Linux host and synthetic established-pair sockets;
Pion's API is pinned/compiled, not a runtime ICE/NAT acceptance. One selected UDP
address in Local-only, mixed older discovery capabilities, Pi/physical WAN,
operated hosted profiles, W11 races/cooldowns/roaming/fairness and W08 failed restart
cases remain later work. WG4 full integration/WG6, inherited T13 login/logout/boot
and deferred P17 owner use/explanation remain outstanding.

Next sequential packet: **W10 — ICE/STUN coordination and traversal**. Establish
bounded authenticated Pion pairs, bind both pins/purpose/session/role/generations/
expiry, attach this packet transport per pair, close/rebuild on selected-pair changes
and execute marked disposable NAT/loss/blocked-UDP/relay cases before closing WG4.


## W10 — ICE/STUN coordination and traversal

**Complete for production composition and local native/emulator acceptance.**
[Evidence and W11 handoff](../evidence/wan-w10-20261006/summary.md),
[actual commands](../evidence/wan-w10-20261006/commands.md),
[results](../evidence/wan-w10-20261006/results.json) and
[preservation](../evidence/wan-w10-20261006/preservation.json) retain the dirty
W00–W09 baseline, failed runs and limitations. Full WG4 composition closes with
Pion v4.4.6 and existing quic-go/qpack pins; no raw-stream alternate was selected.

Signed ICE requests/offers/accepts bind identity, pin, purpose, session,
deterministic controlling role, both generations and expiry; the independent
signature fixture preserves legacy bytes. Automatic/self-hosted peer traffic
uses bounded host/srflx gather/checks and the selected-pair adapter. Private topology
is omitted, targets are scope-checked and enrollment remains isolated. Optional
separate STUN exposure requires an exact reviewed profile endpoint and enforces
fixed response/amplification/source/global bounds. ICE sessions cannot allocate
relay attachments; traversal failure uses a separate WSS session and typed reason.

Fourteen discovered W10 tests across four packages execute with focused race
checks twice; the two native guarded tests execute separately twice inside a
marked disposable user/network namespace. Actual production sync over simulated
no-NAT, endpoint-independent and dependent-filtering paths selects HTTP3; exact
incompatible/double-NAT and blocked-peer-UDP rules select WSS. Both directions
verify heads, bytes, hashes, original authors and interrupted chunk reuse. Actual
consent loss retires and rebuilds pairs; directory shutdown still permits new
capture/transfer over a live authenticated pair. Signed abuse, malformed STUN,
HTTP authority/body/handler/pin checks, encrypted loss, slow stream/cancellation,
shared 32-session admission and joined close pass. Native UDP6 STUN returns 44/20
bytes; native lifecycle FD/goroutine samples fall after runtime closure.

The 32-MiB slow stream sends 65,536 bytes before cancellation with a fixed 32-KiB
producer buffer. Component bounds are eight resident agents, two establishments,
eight advertised candidates, two selected addresses/four STUN endpoints, at most
ten gathering sockets per agent, 32 admitted public sources/socket, 100 STUN
packets/second/socket, 1,200-byte packets and 12-second complete attempts. Shared
incoming QUIC admission is 32; STUN prefix/global rates are 10/200 per second with
1,024 retained prefixes and amplification at most three. These are finite bounds
and local samples, not the combined W11 resource/Pi budget.

Final compatibility race, aggregate, real binary/PTY, amd64/arm64 service/client
build and package checks are recorded in the evidence command logs. Retained
failures explain fixes for canonical srflx serialization, pair remote addresses,
capability announcement, repeated fallback, borrowing retired endpoints,
control-owned pair cancellation, case-alias acceptance and waiting on a remote
offer despite an empty local gather. Local gathering now precedes coordination
in both roles, with an explicit prompt-fallback regression. Fresh reverse relay
can reach the unchanged metadata quota; the native fixture retries the typed
refusal after a bounded five-second quiet refill. One-second retries failed to
accumulate setup tokens; production cooldown/fairness remains W11. No failed run receives
acceptance credit; no live workloads or personal roots underwent fault injection.

Next sequential packet: **W11 — route policy, roaming and fair progress**. Timed
reprobes/cooldowns, interface detection/generations, transport races, combined
resource bounds and native roaming/fairness remain W11. Physical WAN/CGNAT/Pi,
operated hosted defaults, WG5 native timing, WG6, inherited T13 login/logout/boot
technical checks and deferred P17 owner use/explanation remain outstanding.


## W11 — route policy, roaming and fair progress

State: **complete for production integration, local native/emulator and actual Pi acceptance**. W08–W10 and full local WG4 evidence were read/preserved.
[Evidence and resumable handoff](../evidence/wan-w11-20261006/summary.md) own actual
commands/results, starting dirty-tree provenance and retained failures.

Implemented pinned TCP/QUIC races before HTTP submission, shared outgoing
handshake admission, demand-driven idle relay reprobes, bounded exponential direct
cooldowns, shared quota quiet refill plus bounded responder retries, joined Linux
interface/default-route observation, scoped discovery/announcement refresh,
Local-only UDP rebuilding, stale-generation response rejection and durable queue
aging with forced progress. Existing policy/pins, author/version/operation IDs,
membership, chunk verification and receipt semantics remain unchanged.

Native QUIC→TCP→QUIC chunk/receipt recovery, actual isolated address/default-route
detection, real TLS/HTTP reprobes, busy-generation/flapping bounds, queue retry/
reload identity retention and prerequisite NAT/authorization/race checks have
local evidence. Initial compile errors, a cooldown mutex leak and remote relay
attachment-quota regressions are retained uncredited. Final command outcomes and
limits are in the evidence rather than inferred from source review.

The replacement-join throttle/expiry regression has a focused repair pass: retain
the original prepared proof and reuse authenticated submit state until the next
status poll. Final `make check` passes (terminal suite 953.851 s), with focused race, native
namespace, binary/PTY and package evidence retained.

Follow-up work now delivers reviewed Advanced timing through shared policy controls,
FIFO bounded bandwidth admission before every primary/fallback chunk attempt and
exact legacy policy serialization. [Completion follow-up](../evidence/wan-w11-followup-20261006/summary.md)
records actual whole-daemon relay→QUIC→address/default-route change→relay→QUIC
journeys during a 16 MiB transfer and receipt-boundary loss, locally under race and
on the owner's Raspberry Pi 4B. Native isolated netem adds 25 ms delay/1% loss;
Pi production-default timing passes. Actual three-peer large/small progress over
WSS/TCP/HTTP3 and combined process resources are measured on both hosts. Slow
DNS/relay competition stays within four resolver/32 dial limits and joins cleanly.

Validation: complete compatibility race packages pass; 27 initially discovered W11
cases plus two timeout-wait regressions have named coverage. Final CLI/control and
legacy canonical-policy checks pass. Full `make check` passes terminal (1,038.603 s),
package extraction, formatting/vet and internal/model unit suites, then retains an
integration control-poll timeout failure. A minimized failing regression repairs
read-only polling within the original deadline and same durable operation. Real
pairing passes three times, full integration passes (96.858 s), affected W06/W11
race CLI journeys pass (with a separately repaired PTY EOF/reap fixture), and all
remaining Make check targets pass. The original failed/interrupted commands are
not relabeled as a successful full rerun. Every target has recorded passing
coverage; final changed CLI behavior has focused production-binary evidence.

WG5 closes and M3 automatic direct/recovery acceptance is satisfied for these
recorded native isolated/local/Pi conditions. Actual socket/RSS/CPU/FD/goroutine
samples and 16 MiB plus continuous small-version progress cover WSS/TCP/HTTP3 on
both hosts. Pi production defaults pass 25 ms/1% loss roaming with unchanged host
settings. All 2,078 starting historical evidence files remain unchanged; marked
Pi roots were removed after capture. Existing Pi services/data/network were untouched.

## W12 — qualified network status, diagnostics and controls

State: **complete for the recorded CLI/TUI, local privacy and bounded diagnostic
acceptance**. W06/W07/W11 prerequisites and the [W11 follow-up handoff](../evidence/wan-w11-followup-20261006/summary.md)
were read and preserved. This packet does not claim operated hosted defaults,
physical WAN/CGNAT or T13 lifecycle acceptance.

The shared control path now reports desired and active mode/profile state,
profile expiry, service readiness, dated route observations, candidate counts,
freshness, typed probe outcomes and a next action while keeping daemon state,
capture, peer-stored/applied state and membership separate. `network status` is
passive. `network doctor` performs one explicitly requested, bounded set of
service DNS/TLS, authenticated directory, pinned direct/relay TLS and actual
UDP STUN checks; it never infers a NAT or firewall type. Cancellation, resolver,
dial and probe admission remain finite and cleanup joins before return.

Reviewed mode/profile controls now enforce the privacy boundary: disabling
internet discovery closes service/relay clients, invalidates WAN leases and
drains WAN pools while retaining identity, keys, history and files. Existing
installs remain manual until reviewed; first setup carries explicit consent.
CLI and TUI share the same status/doctor operations and keyboard-visible next
actions. Support archives whitelist configuration and redact nested diagnostics,
credentials, private paths and filenames; existing archives are never replaced.

Validation: focused W12/control/network/terminal checks pass, including the
actual pinned-probe HTTP-suppression test and support-export privacy test; the
owning packages pass under the race detector. The real binary journey exercises
service healthy/peer offline, relay quota/healthy distinctions, PTY keyboard
cancel/refresh, local-only zero-service traffic, reviewed apply/replay/restart,
identity/file retention and offline cleanup. Exact commands, retained failures,
dirty-tree provenance and logs are in the [W12 evidence directory](../evidence/wan-w12-20261006/summary.md).

Remaining acceptance: W13 operated hosted defaults/self-host deployment and WG6,
W14 migration/packages, W15 integrated failure/resource campaign, W16 physical
WAN/CGNAT and ordinary setup, plus inherited T13 login/logout/boot technical
checks and deferred P17 owner use/explanation. Next: **W13 — operated defaults
and self-hosting**.

## W13 — operated defaults and self-hosting

State: **partial**. Worker: Claude Code session, no delegation. Prerequisites W03,
W04 and W10 evidence and the W12 handoff were read; the dirty W00–W12 tree was
preserved. A pre-edit hash snapshot was not captured for this packet; the final
changed-file hashes are in [`final-source.json`](../evidence/wan-w13-20261006/final-source.json).

Changes: `cmd/orbit-net` (`serve --config/--check`, SIGHUP certificate reload,
loopback metrics/health, `keygen`, `profile sign/verify`, `version`);
`internal/rendezvous` two-epoch rotation overlap and sanitized counters;
`internal/network` STUN counters and same-origin adjacent-epoch peer proofs;
`internal/config` reviewed service trust and route rebinding; control/CLI
`--service-roots` with `service_trust` status; daemon trust loading and
rebinding; system unit, sysusers entry, examples, reproducible operator archives
and the [operator runbook](../orbit-net-operator.md). Owning specifications were
updated (architecture, protocol, operations, persistence, gates, schema).

Defect found and fixed: rotating to a new epoch stranded every pairing (routes
kept the old digest and peers rejected each other's proofs). A strict-digest
reproduction fails the mixed-epoch transfer; the fix passes it.

Validation: format, vet and all `cmd`/`internal`/`model` packages pass; changed
packages pass under the race detector; the real-binary rehearsal passes with
both a fresh build and the packaged amd64 archive. The arm64 archive passes a
smoke on the owner's Pi 4B in a marked temporary directory, and the unit's
sandbox properties pass under `systemd-run --user`. Exact commands, results and
logs: [W13 evidence](../evidence/wan-w13-20261006/summary.md).

Hosted deployment (owner-authorized, same day): `https://connect.calebhabesh.com:8443`
on the owner's Oracle VPS under the hardened system unit, Let's Encrypt with
automatic renewal, signed release profile epoch 1, and a live laptop↔Pi relay
pairing and two-way transfer in Automatic mode. Root-only unit properties now ran
on the real host. Deployment exposed the 1:1-NAT STUN bind gap, fixed by
`stun_bind`. Record: [deployment](../evidence/wan-w13-20261006/deployment.md).

Remaining acceptance: WG6 needs a second offline copy of the authority key and a
configured alert/on-call destination; profile distribution as the packaged default
is W14. W14 default distribution and W16 hosted tests depend on WG6. Inherited
T13 technical checks and deferred P17 owner use/explanation are unchanged.
Next: obtain operator resources to close WG6, or the owner selects other work.

2026-10-06 readiness follow-up: the live service remains healthy. Added
`orbit-net key verify` and a documented local restore check; all four operator
package tests pass uncached under race, vet passes and amd64/arm64 operator
archives build. The existing authority verifies with the packaged amd64 binary.
Custody correction: its actual observed location is this development
workstation's `/home/owner/.config/orbit-operator`, not the SSH laptop
account's home named in the earlier record. The owner has no USB currently;
an encrypted password-manager backup remains an available approach once a
vault is identified. No second copy or received alert has been established.
WG6 stays open. W15 was pending at this follow-up, before the owner-directed
deferral below. Exact checks and next steps:
[readiness follow-up](../evidence/wan-wg6-readiness-20261006/summary.md).

Owner-directed deferral, 2026-10-06: the owner requested recording the backup
and alerting for later and moving to the next major packet. W13 remains partial;
WG6 remains open. W15 may now use the completed W13 technical work for isolated
development/validation, without waiting for these operator actions. This changes
sequencing only; distribution and hosted-default release gates remain in force.

Deferred operator checklist (W13 owner Caleb Habesh; due before wider package
distribution and W16 hosted-default acceptance):

- Store a second independent authority-key copy in an encrypted vault or offline
  medium, verify a restored copy against the frozen public authority with
  `orbit-net key verify`, and record custody and restoration evidence without
  secrets. Current source key is on the development workstation; no second
  copy has been verified. The owner currently has no USB.
- Configure monitoring rules and an alert destination the owner receives;
  demonstrate actual firing and recovery delivery using synthetic monitoring
  inputs, and record on-call contact. Live metrics/health alone do not complete
  alerting. No working destination has been verified.

Next development packet: **W15**. Both tasks remain required to close WG6/W13
and to complete W17; deferral grants no acceptance credit.

## W14 — packaged defaults, migration and mixed versions

State: **complete for implementation and recorded acceptance; final-source validation passed**. Opus's
implementation and owner laptop/Pi same-LAN packaged relay journey are retained in
the [W14 evidence](../evidence/wan-w14-20261006/summary.md); the original worker's
[command handoff](../evidence/wan-w14-20261006/commands.md) remains recorded. Codex took over final
validation without delegation and without changing the inherited runtime code.

The original aggregate exited 2 at `TestWANW07RealPTYRelayOnboarding`: its binary
displayed an inviter/operator label that did not match the PTY expectation. The
source already contained the compatible `Inviter operator:` label by takeover;
that edit occurred after the original aggregate started. This original failure
is retained and does not validate the final source.

Fresh commands, source fingerprints, dependency/host assumptions and remaining
conditions are in the [closeout evidence](../evidence/wan-w14-closeout-20261006/manifest.json).
Twelve discovered W14 tests pass uncached under race detection, including the
native PTY confirmation/code journey and real pre-WAN upgrade/rollback. The
affected CLI/config/control/network/TUI packages pass uncached race tests; the
standalone W07 keyboard relay regression passes. Fresh `make GOFLAGS=-v check`
exits 0 (terminal suite 1294.436 s) with packages, format/vet, CLI/unit,
integration, model, fault and harness checks. Runtime/test/build source hashes
remain identical before and after every command; 2,222 historical P/O/T/W
evidence files and inherited W14 raw transcripts are unchanged. The closeout
corrects missing evidence links/status and four trailing document blank lines;
it makes no additional runtime change.

W13/WG6 authority-key backup and alerting still gate hosted-default publication;
W15 depends on W13 and W14. Native expired bundled-profile execution remains
unexecuted (unit boundary coverage exists). W16 physically separate-network
direct/relay acceptance, inherited T13 technical checks and deferred P17 owner
use/explanation are unchanged.

The W14 handoff selected W15 under the owner's sequencing amendment, using
completed W13 technical prerequisites while retaining deferred WG6 operator
conditions. The full W15 campaign is now recorded below.

## W15 — integrated failures, security and resources

State: **complete for recorded local/native/emulator/Pi acceptance**. The owner
sequencing amendment authorizes W15 using
W13's completed technical prerequisites; WG6 authority backup and received
alert delivery remain deferred. This campaign uses marked disposable local/Pi
user/network namespaces and in-process emulators. Existing deployed services,
personal roots, P/O/T evidence, unfinished T13 technical checks and deferred
P17 owner use/explanation are preserved.

Delivered the guarded [network/process runner](../../scripts/validation/WAN_FAILURES.md),
independent transfer-boundary model, QUIC→TCP boundary/receipt recovery with GC,
post-rename publication recovery, conflict preservation and cached transport
refusal after retirement. A minimized regression exposed a symlinked disposable
root accepted by the shared destructive-target guard; its repair passes.
Whole-daemon fixtures combine chunk/receipt SIGKILL, relay recovery, roaming,
service outage/restart and exact head/hash/identity/pin/receipt oracles.

[Scenario matrix](../evidence/wan-w15-20261006/scenario-matrix.md) identifies the
named production coverage and explicitly unexecuted external conditions.
Final native impairment/MTU, Pi recovery/impairment/resources, independent models,
fuzz, enrollment, focused security and repeated Local-only startup checks pass.
The campaign also diagnosed and repaired refused announcement generations that
caused stale offers and quota starvation. [Evidence summary](../evidence/wan-w15-20261006/summary.md)
records commands, retained failures, resource samples and provenance. Final-source
`make GOFLAGS=-v check` passes (1,480.961 s), as does uncached full race
`go test -race -count=1 -timeout=35m -v ./...` (1,391.455 s); both retain source
digest `7b5d56630478df744cf379927ed2773add98a5df295170065bad6973741a6a23`.
Local repaired impairment/MTU and native ICE, Pi repaired impairment, actual
CLI/PTY/enrollment, four fuzz targets, demo and client/operator packages pass.
Sampled Pi impaired daemon peaks are 35.2 MB RSS / 28 FDs / 59 goroutines;
three-route fairness completes the 16 MiB version while small versions progress.
All 2,277 historical evidence files and unrelated starting changes are preserved;
marked native campaign roots are cleaned after copying artifacts. Physical WAN/N10,
static native bundled-profile expiry and hardware power/lifecycle conditions are
explicitly unexecuted. Next packet is **W16**. W16
hosted-default acceptance and wider distribution still require closing WG6.

## W16 — real WAN and ordinary setup campaign

State: **complete for reachable networks (closeout 2026-10-07, below)**; this entry began as safe runner preparation and read-only host inventory.
W07/W11/W13–W15 technical evidence was read and preserved. WG6 remains open
for verified independent authority backup and received firing/recovery alerts;
W16 hosted-default tests and wider distribution remain gated. T13 technical
checks and deferred P17 owner use/explanation retain their states.

The owner confirmed that only the VPS is remotely located. The laptop/Pi behind
the home router plus the Oracle VPS can supply the two physical networks;
replicas do not all need public listener addresses. Read-only SSH inventory
observed the laptop's route to the VPS public address through `wlp2s0` and the
home gateway, and the VPS through `enp0s6`. Existing Tailscale/WireGuard links
remain active. No host VPN, firewall, route, shared service or workload was changed.

Delivered the [native runner guide](../../scripts/validation/WAN_NATIVE.md),
read-only route/interface/policy inventory, and a fresh checksummed-archive CLI
journey with ordinary create/invite/join, exact approval, two-way small bytes,
4 MiB transfer, captured offline version/reconnect and second-folder enrollment.
The runner checks protected hash/version/author, endpoint stored receipt,
independent receiver working bytes/readiness, stable identities/SPKI pins and
operation/attempt continuity. It refuses open WG6 before hosted host actions and
active/ambiguous tunnel routes before setup. Invitations stay private; no SSH
forwarding or manual replica endpoint configuration is available. Successful
cleanup removes only freshly allocated marked roots after owned process checks.

Validation and retained runner experiments are in the
[W16 evidence](../evidence/wan-w16-20261006/summary.md). The local fixture uses
production binaries, a disposable self-host service/private CA and a synthetic
archive. It establishes runner development coverage, not physical WAN, real
release packaging or bundled hosted-default acceptance. Full W16 completion is
not claimed.

Actual commands: `GOFLAGS=-race go test -race -count=1 -v -timeout=12m
./tests/terminal -run '^TestWANW16NativeRunnerRehearsal$'` passes in 119.935 s,
with an identical before/after source digest. `python3 -O -m unittest discover
-s scripts/validation -p 'test_*.py' -v` passes 29 tests; `go vet ./tests/terminal`
and `git diff --check` pass. Read-only laptop/Pi/VPS inventories pass; the hosted
runner refuses WG6 before any host action. All 2,630 inherited evidence files
and 553 preexisting non-document source files match their recorded hashes.
`make check`, full race/fuzz/demo/package campaigns were not repeated for this
runner-only slice; W15's historical validation remains preserved.

Remaining work: close WG6, prepare isolated replica environments that cannot
use existing VPN links, then execute actual bundled-profile CLI/TUI direct and
relay journeys across home/VPS. Record socket/packet route proof, full native
timing/resources, UDP-blocked relay, interface/address change, a separately
disposable service restart, and laptop/Pi/VPS forwarding/conflict/restore.
School/corporate/CGNAT/IPv6 claims require actual access and remain unexecuted.

Follow-up, 2026-10-06 (same day): owner chose ntfy for WG6 alerts and asked
for read-only inspection plus an exact change list before host changes. Added
`orbit-net alert` with packaged timer units, validated locally
([WG6 alert evidence](../evidence/wan-w16-20261006/wg6-alert/summary.md)); not
deployed and no delivery received. Added `scripts/validation/wan_netns.sh`, an
isolated replica namespace NATed only to the physical uplink, rehearsed rootless
(tunnel peer blocked, host ports rejected, cleanup complete). Inspection found
rootless namespaces blocked on laptop/VPS and no user-space uplink helper on any
host, so isolation needs sudo. The
[host-change proposal](../evidence/wan-w16-20261006/host-change-proposal.md)
awaits approval; WG6 remains open and no host was changed.

The owner approved the proposal. Alerting is deployed on the VPS as a separate
binary/timer (production service unchanged) and the test/firing/recovery drill
sent all three notifications; owner receipt is awaited. VPS and Pi isolated
namespaces are up and verified (only veth inside, tunnel blocked, service via
the physical uplink). The runner gained `netns` topology support with
host-rule verification and per-transfer route counters; 32 harness tests and
the local W16 rehearsal (107.9 s) pass. WG6 still needs confirmed receipt and
the owner's verified authority backup before hosted-default journeys run.
Relay restart and laptop participation remain unexecuted.

2026-10-07: WG6 closed (live-received alerts; owner-attested Bitwarden key copy,
restore check waived). Hosted-default runs between the Pi (home) and VPS
namespaces, with namespace path accounting
([evidence](../evidence/wan-w16-20261006/native-hosted/summary.md)): ordinary
bundled-profile CLI create/invite/join/approve, two-way transfer, 4 MiB,
reconnect and second folder pass. A real direct cross-network transfer (4 MiB
over direct UDP) and real relay transfers, including forced relay with
UDP blocked Pi → VPS (zero direct bytes), are observed. **Open defect:** with
UDP blocked mid-session, VPS → Pi does not recover within 180 s (relay inner
TLS handshake timeouts), reproduced twice. TUI, address change, service restart,
laptop/three-host cases and resources remain unexecuted.

Defect fixed, 2026-10-07 ([fix evidence](../evidence/wan-w16-20261006/quota-fix/summary.md)):
client-side signed-operation admission (two challenge slots, paced budget with
renewal reserve, no stranded challenges), whole relay-setup admission, readiness
kept on transient renewal failure, control rebuild on network change with
backoff, and one initiator relay tunnel per target (the per-pair limit of two was
shared by both directions). Protocol/architecture specs updated; the service is
unchanged. Unit/integration race suites, terminal WAN suites and final
`make check` pass. Native runs 14–18 (17–18 on the final source) pass the CLI hosted journey plus forced relay
(~5.5 s each direction, zero direct bytes, zero service quota refusals) and an
address change (~31 s recovery). Remaining W16: keyboard TUI journey, relay/
rendezvous restart (needs a disposable public service or owner approval),
laptop/three-host forwarding/conflict/restore and resources.

W16 closeout, 2026-10-07 ([evidence](../evidence/wan-w16-20261006/native-hosted/summary.md#tui-service-restart-three-hosts-and-resources-2026-10-07)):
same packages as runs 17–18. Runner gained `--journey tui` (keyboard phases on
real PTYs), `--service-restart`, `--three-host` (laptop through an owner-started
namespace and user-owned socket shell, no runner sudo) and per-stage `/proc`
resource samples. Run 19 (TUI + impairments + restart) and run 20 (three hosts)
pass: TUI create/invite/join/approve with packaged-profile review and matching
verification code (join → completed 31.2 s); the owner-approved single
`orbit-net` restart during forced relay recovered relay in 9.5 s with transfers
of 11.7/7.7 s and zero direct bytes; laptop forwarding via the Pi (7.8 s, author
preserved, laptop offline), a two-head conflict identical on all three (13.7 s)
resolved from the laptop, and a VPS restore as a new identity. Daemons used
30–36 MiB RSS, orbit-net about 15 MiB. Actual commands: local rehearsal
`GOFLAGS=-race go test -race -count=1 -v -timeout=20m ./tests/terminal -run
'^TestWANW16NativeRunnerRehearsal$'` passes (CLI 107 s, TUI + three hosts 196 s);
`python3 -O -m unittest discover -s scripts/validation -p 'test_*.py'` passes 36
tests; final `make check` passes ([log](../evidence/wan-w16-20261006/native-hosted/make-check-closeout.log); terminal suite 1501.935s of its 30 min limit).

Limitations: one home network and one VPS; the laptop shares the Pi's home
network (three-device engine behaviour, not a third network) and has only
interface totals for path accounting; address-change recovery stays bounded by
the service's ~30 s heartbeat drop (service-side replacement would need a
production redeploy); single-run timings are observations, not deadlines.
School/corporate/CGNAT/IPv6 remain unexecuted. Validation roots on all hosts and
the Pi/VPS namespaces were removed; pilot roots were untouched.

Next: **W17 — combined release and handoff**.

## W17 — combined release and handoff

State: **complete for the recorded release conditions** (2026-10-07). Worker:
Claude Code session, no delegation. The W00–W16 records, T13 status and P17
deferral were read and preserved. The owner chose a disposable KVM guest for
the last T13 technical check.

Changes: `internal/control/service.go` now expands `%h` when matching the
packaged unit's state, and requires the unit's `MainPID` to own the state for
`start`/`restart`; otherwise it returns `MANUAL_DAEMON_RUNNING`.
`terminal_lifecycle.go` no longer duplicates error codes in service failures.
The Orbit entry gains `orbit stop` (help and completions). The TUI maps the new
code. Also: regressions (`internal/control/service_test.go`, T02 lifecycle
mock), `scripts/validation/service_boot_vm.py`, and a 45-minute `make check`
terminal timeout. Docs: [networking guide](../runbooks/networking.md),
operations, install runbook, operator runbook, case study, portfolio bullets,
README and WAN spec status lines.

Validation ([evidence](../evidence/wan-w17-20261007/summary.md)): the
committed source rebuilds the W16 field packages byte-for-byte. The VM
login/logout/unattended-boot drill passes (run 5, final source). On the
isolated snapshot `0c48429`, all of the following pass: `make GOFLAGS=-v check`
(1,719 s; terminal 1,506 s); uncached full race (1,615 s, 21 packages); demo;
byte-identical repeated client and operator packages; T13 release tests twice
under race; and 36 harness tests. No orphaned test daemons remain. Only
documentation changed after the snapshot, and the final-tree package test
passes.

No new native WAN run: the runtime changes do not touch networking, and W16
remains the native evidence. Open conditions, operator actions and future
extensions are listed in the release record. P17 personal use and unaided
explanation were then removed as requirements by the owner (2026-10-07,
[scope](../portfolio-scope.md)). There is no further W packet.

## Post-W17 — same-LAN direct paths and profile epoch 2 (2026-10-07)

Owner-directed pre-trial fixes, Claude Code session. Evidence:
[summary](../evidence/wan-lan-exchange-20261007/summary.md).

**Same-LAN relay.** Cause: the laptop's `ufw` deny-incoming policy dropped LAN
multicast and the random direct ports, and LAN addresses never travel through
the service. Fix: approved peers exchange their signed `orbit-lan-v1` records
over the established pinned session (`POST /peer/v1/lan`,
[protocol](../orbit-wan-protocol.md#peer-lan-exchange-post-w17-2026-10-07)).
Code: `internal/network/lan_exchange.go` (new), `lan.go` (shared signing,
clock-based generations, `AcceptPeer` scoping), `direct.go` (peer-sent leases
never suppress the public lookup or replace a link-heard lease), `limits.go`,
`internal/protocol/lan.go`, `internal/replication/server.go` (route) and
`internal/app/app.go` (wiring, shared peer TLS). Tests:
`internal/network/lan_exchange_test.go` (scoping and own-address exclusion,
lease precedence, relay → direct after an exchange, endpoint authorization).
Docs: networking guide firewall section, operations, architecture,
`schemas/peer-v1.md`, `schemas/network-v1.md`.

**Profile epoch 2.** Signed offline 2026-10-07 with one-year validity (expires
2027-10-07, digest `9138a478…ab681`), same operator, privacy text and service
key, so updated devices apply it at start. The hosted service now serves epoch 2
with epoch 1 as overlap (`--check` passed; `/healthz` 200; metrics show both
epochs valid); `serve.json` was backed up on the host first. Packaged in
`internal/network/release-profile.json`. `orbit-net serve` now skips an expired
overlap epoch with a warning instead of refusing to start
(`TestOverlapEpochExpiryDoesNotStopService`); the deployed binary predates this,
so remove the overlap settings after 2027-01-04 or deploy a current build first.
The operator runbook now recommends one-year validity and explains what expiry
bounds.

Commands and results:

| Command | Result |
| --- | --- |
| `go test -race -count=1 ./internal/network/... ./internal/replication/... ./internal/protocol/... ./internal/app/...` | passed |
| `go test -count=1 ./cmd/... ./internal/... ./model/...` | passed |
| `make check` | passed, exit 0; terminal stage compiled before the LAN edits ([log](../evidence/wan-lan-exchange-20261007/logs/make-check.log)) |
| `make test-terminal` on the final source | passed, 1,511 s ([log](../evidence/wan-lan-exchange-20261007/logs/test-terminal.log)) |
| `lan-native.sh dist` (laptop + Pi, same LAN, hosted service) | passed: Pi relay → both `quic`, LAN candidates 4 each, 1.7 s each way |

Correction to the W14 record: its summary reports a `relay` route, but its log's
only route observation is `quic; code=CONNECTED`. The W14 files are unchanged.

Not done: no rerun on other routers or with firewalls on both devices (fixed
ports are documented, not tested natively); the hosted `orbit-net` binary was
not redeployed; nothing is committed or published.

**Follow-up the same day.** The hosted `orbit-net` was redeployed from original
commit `f65b623` (public `513a96d`; previous binary kept as a backup):
`--check` passed with epoch 2 plus epoch 1 overlap, `/healthz` 200. A laptop/Pi
rerun against it passed. CI had failed since 2026-09-24 on arm64 packaging
(completions executed the amd64 binary), amd64 `test-race` (10-minute default
timeout) and one co-located W16 rehearsal wait; all three were fixed. The
joiner's post-submission status poll moved from 25 s to 15 s: join to first file
went from 38.2 s to 22.7 s natively
([log](../evidence/wan-lan-exchange-20261007/logs/native-lan-poll15.log)).
The repository then became public after the checks in the
[publication record](../publication-2026-10-07.md).
