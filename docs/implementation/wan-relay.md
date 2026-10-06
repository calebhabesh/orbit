# WAN block B: relay and simple setup

Read [plan](../orbit-wan-implementation-plan.md), [architecture](../orbit-wan-architecture.md),
[network protocol](../orbit-wan-protocol.md), [UX](../orbit-wan-ux.md) and
[tracker](wan-status.md). W03–W05 production outcomes are recorded below; W06 CLI controls/local integration and aggregate validation are complete; W07 keyboard TUI/local relay acceptance and full aggregate validation are complete.
Use the same production control/replication operations from CLI and TUI.

## W03 — Authenticated rendezvous and profiles

Dependencies: W01, W02; WG2 closed. Change: network directory/control client,
server implementation, profile validation/configuration, local fixtures and schemas.
Invariants: I08–I09, I13, I20, I23, N01–N03, N06–N07, N09.

Required work:

- Implement signed-profile validation, ordinary service TLS verification and
  explicit development/self-host trust. Default production profiles need actual
  operator values from W13; local profiles remain clearly development-only.
- Implement authenticated announce/lookup/control sessions, lease expiry, generation,
  reannouncement with jitter and bounded ephemeral records. Key directory entries
  by existing ID/pin and verify proof/signatures before mutation/lookup.
- Exchange purpose/peer-bound offers and reservation intent. Unknown enrollment
  routing gets separate finite admission; known data peers still need local pins.
- Enforce candidate scope, body/header/rate/resource bounds, safe DNS/origin
  handling, no enumeration and no arbitrary URL dialing. Do not store invitations,
  folders or inventories in the service. Specify real operator metadata retention.

Acceptance evidence:

- Fake ID/key registration cannot replace a known pinned route; unauthenticated,
  invalid-signature, wrong-origin/purpose, replayed and expired exchanges fail.
- Lease expiry/service restart/stale response/source-IP observations never invent
  reachability or trust. Private/loopback/control-target candidates are rejected.
- Maximum-valid and oversized messages/caches tested; shared-NAT admission works
  with per-key/global limits; no unbounded control goroutines under floods.
- Local capture/manual direct connections survive directory loss; privacy mode
  causes no public announcements. Profiles distinguish test versus release endpoints.

### W03 production outcomes — 2026-10-05

Implemented `internal/rendezvous`, the separately runnable `cmd/orbit-net`,
`network.ServiceClient` and private reviewed profile persistence. Exact signed
ID/pin/purpose records, single-use challenges, semantic idempotency, lease/offer
expiry, authenticated addressed WSS events, partner acceptance and signed
reservation intent use the W01 canonical bytes without changing golden fixtures.
Public candidate scope, all-answer DNS checks, normal outer TLS and independent
request/dial/socket/control/cache limits enforce the packet's trust/resource seam.

[Architecture](../orbit-wan-architecture.md#w03-directory-and-profile-integration)
records bounds and metadata retention; [schema clarification](../../schemas/network-v1.md#w03-implemented-service-selection-and-wss-control-behavior)
records HTTPS challenge binding on same-host WSS and heartbeat behavior.
[W03 evidence](../evidence/wan-w03-20261005/summary.md) records actual tests,
operator/binary fixtures and limitations. No default hosted operator, relay
forwarding, automatic daemon activation, v3 enrollment or new runtime capability
is claimed. Saving a profile leaves manual behavior intact. W04 and W05/W06
integrate routing and reviewed setup; W13 supplies production operator values.

## W04 — Encrypted WSS relay

Dependencies: W01, W02, W03; WG1/WG2 closed. Change: broker, WSS stream adapter,
relay reservations/attachments, incoming virtual listeners and daemon integration.
Invariants: I05–I09, I13–I14, I23, N01–N07.

Required work:

- Build both outward legs with role/peer/purpose-bound expiring credentials, partner
  acceptance and finite reservation/session admission. Separate enrollment/data
  listener queues. Reuse inner pinned TLS and existing HTTP handlers.
- Forward ciphertext with fixed buffers/backpressure, explicit library limits,
  service/per-device bandwidth ceilings and deadlines. Close/join both legs on
  error/expiry/cancellation; release reservations immediately on failure.
- Wire manager route observations and typed overload/offline errors. Broker has no
  database/content/owner-control dependency; allocation/forward acknowledgements
  cannot become stored receipts. Keep initial quotas visible and configurable.

Acceptance evidence:

- Force direct routes unavailable; two disposable agents sync verified files over
  outbound relay, including reverse pulls. Enforce folder authorization and pins.
- Inspect synthetic broker streams: no readable filenames/content/invitation tokens;
  inject wrong-leg/pin/purpose/expired attachments and prove safe rejection.
- Exercise slow reader, abrupt leg loss, relay restart, chunk interruption and
  request cancellation. Verified chunks survive; no false receipt or file completion.
- Bound memory/FD/goroutines under idle, overload and streaming tests; unknown
  enrollment cannot starve already-admitted data or local capture.

### W04 production outcomes — 2026-10-05

Implemented live WSS attachment/forwarding, fresh exact leg proof admission,
separate enrollment/data limits, finite bandwidth/byte/idle/lifetime ceilings,
purpose-specific control coordination and manager logical routes/observations.
Daemon-owned virtual listeners serve the existing isolated pinned TLS handlers;
manual policy still selects no public service. Operator flags configure positive
finite relay ceilings. No routing acknowledgement becomes a file receipt.

[Evidence](../evidence/wan-w04-20261005/summary.md) records 23 discovered race tests,
20 local service restart/chunk-resume repetitions, complete production-service
bidirectional verified sync with direct disabled, broker-boundary ciphertext
inspection, unknown v2 enrollment/folder/control isolation, bounded capacity and
manual compatibility plus make check. Capacity reservations are explicitly seeded;
full offer/accept/control journeys have separate production tests. Hosted/native
WAN/Pi and ordinary v3/CLI/TUI activation remain later packet requirements.
[Architecture](../orbit-wan-architecture.md#w04-encrypted-relay-integration) and
[wire clarification](../../schemas/network-v1.md#w04-implemented-wss-attachment-behavior)
record exact limits/lifetimes and the unchanged canonical contracts.

## W05 — Routed enrollment v3 and peer data

Dependencies: W02–W04; WG3 closed. Change: protocol/replication enrollment,
repository durable records/migration, typed controls, membership bootstrap and
logical peer routing references. Invariants: I08–I09, I15, I20, I22–I24, I28, N01–N05.

Required work:

- Implement explicit v3 capabilities, logical-route invitations and exact signed
  transcripts/approval artifacts. Persist necessary receiver fields privately with
  existing setup/replay lifetime rules; keep ordinary inspection secret-free.
- Support unknown requester relay routing before membership; authenticate inviter
  before capability disclosure. Bind approval to exact device/key/folder/prior
  membership and preserve current retirement/rollback/fork checks.
- Store routes for both pull directions and refresh them independently of folder
  authority. Bootstrap canonical membership/retirement artifacts through existing
  handlers. Additional folders reuse keys/routes with separate scoped attempts.
- Preserve v2/manual operations and reject unsupported automatic mode explicitly.
  On restart reconnect/reannounce from durable intent; transient network tokens do
  not renew an expired invitation or replace a request identity.

Acceptance evidence:

- First-time two-device join with no shared LAN/no usable direct route, explicit
  approval and actual content in both directions; wrong pin fails before disclosure.
- Unknown requester cannot fetch metadata/chunks, reach control, or access an
  unshared folder. Test expiry/revocation/replay/wrong-folder/retired identity.
- Kill/restart at submit, accepted-request, approval and bootstrap boundaries; reuse
  exact operation/identity/root and recover uncertain responses without duplicates.
- Two folders to the same device, third-device/offline rollout, fork and retired
  history fixtures pass. V2 fixtures and signed bytes remain unchanged.

### W05 production outcomes — 2026-10-05

Implemented additive routed enrollment v3 in `internal/protocol`,
`internal/replication` and the existing reviewed control/setup path. V3 carries
logical profile-bound routes, signs the exact requester transcript, verifies the
inviter certificate before capability disclosure, and binds approval to the
canonical membership digest. Accepted records retain the exact signed request
and token digest; raw capabilities stay private and are omitted from ordinary
inspection.

Private bounded `peer-routes.json` records preserve both pull directions and are
reloaded after daemon restart. Route registration is idempotent and refuses a
changed pin/profile for a known device. Existing v2/manual invitations and
`peers.json` remain unchanged. Approval recovery installs canonical membership
and retirement snapshots through the existing repository transaction.

Focused security, quota, route-persistence and compatibility checks pass. The
marked-root daemon campaign kills request-prepared, request-accepted, approval
and membership-received boundaries and recovers the same identity, attempt,
root, request and reverse data. The third-device/offline, fork and retired
history fixture passes. Evidence and limitations are recorded in
[`wan-w05-20261005`](../evidence/wan-w05-20261005/summary.md). Hosted/default
operator readiness, native WAN, QUIC/ICE, roaming and ordinary CLI/TUI activation
remain later packets.

## W06 — Automatic CLI setup and scripting

Dependencies: W05. Change: control/controlclient, existing CLI dispatch/help,
setup settings, profile review and commands; no CLI-owned networking engine.
Invariants: I19–I24, I27–I28, N04–N05, N08–N10.

Required work:

- Implement create/join defaults in WAN UX: names/root/startup/capacity review,
  Automatic connection, metadata disclosure and Local network only before global
  traffic. Existing manual installations require reviewed opt-in.
- Reuse current invite/request/approve commands; add compatible aliases where the
  UX vocabulary differs. Keep secret prompt/stdin/private-file paths and durable
  preview/apply operations for scripts; no secret argv or invisible prompt.
- Add typed network status/policy operations and stable capability/error results.
  Distinguish local saved state, networking readiness and actual peer copies.
  Preserve root/name drafts and operations under service errors and delayed approval.

Acceptance evidence:

- Real binaries complete create/invite/join/approve/transfer without manual address,
  Tailscale, separate init/serve or service account; repeat through scripted reviews.
- Service unavailable at setup, wrong pin, expired invite, incomplete root/capture
  preview, delayed approval, second folder and restarted CLI produce accurate states.
- Private files/permissions and stdout/stderr/JSON redaction/exit behavior verified;
  non-TTY commands do not request hidden input. Existing CLI compatibility passes.
- M1 checks actual files/heads/certificates and `make check`, not just successful
  rendering or returned operation IDs. Local service fixtures are labeled as such.

## W07 — TUI onboarding and relay milestone

Dependencies: W06. Change: `internal/terminal` screens/actions plus shared typed
client; retain the existing terminal stack and state ownership. Invariants:
I19, I21–I24, I27–I28, N04–N05, N08–N10.

Required work:

- Implement create/join review, invitation transfer, exact pending approval,
  restartable progress and initial network detail views through the same controls.
  Put technical settings under Advanced; keep Back/Edit and privacy choices visible.
- Preserve entered text/selection while asynchronous calls complete; reject stale
  response contexts. Support current keys/Tab, narrow/colorless layouts and pasted
  invitations. Exit restores terminal state and leaves daemon work running.
- Display relay as ordinary observed connection, with saved/stored/applied/conflict
  state separate. Retry network work without replacing reviewed enrollment identity.

Acceptance evidence:

- Production binary in a real PTY completes keyboard-only first-time relay pairing,
  edits/transfer, exact approval, additional folder and restart/resume with byte and
  identity oracles. Verify no address/port prompt in ordinary Automatic flow.
- Wrong pin, service outage, expired invite, existing-root review, delayed approval,
  resize/colorless mode, paste and quit while pending retain safe state/inputs.
- M2 includes CLI/TUI parity and daemon capture after interface exit. Snapshot-only
  screen tests do not establish the milestone; default hosted tests remain W16.

### W06 production CLI outcomes — 2026-10-05

Implementation and focused controls/CLI evidence are recorded in the
[tracker](wan-status.md#w06--automatic-cli-setup-and-scripting) and
[run summary](../evidence/wan-w06-20261005/summary.md); the clean aggregate
`make check` passes all of its targets. Names/root/capacity/startup and optional connection policy share the
existing reviewed root job. Fresh setup selects Automatic; missing operated
configuration reports a networking block without losing local capture. Existing
manual state and explicit legacy settings keep their mode until owner review.
Network policy/profile preview/apply uses the durable controller ledger and
monotone private writes; CLI presentation never owns routing or folder authority.

Named invitations save owner-only transfer artifacts and omit capabilities from
normal progress/JSON. Retained invite/approval vocabulary continues alongside
exact request/key/folder/transcript/membership reviews and real TTY confirmation.
Private v2/v3 files/codes and stdin support scripted root reviews and resumable
operations; secret argv is refused. The production-binary fixture checks both
verified transfer directions, version authorship, inviter trust, second-folder
approval, quota/delay/relaunch, wrong-pin/expiry, input permissions, blocked root
review and local capture through service outage. Guided CLI PTY evidence is
separate from W07's keyboard TUI work.

The experiment exposed a five-minute onboarding delay in the old CLI default;
routed policies now default to five-second reconciliation through the same bounded
scheduler. Explicit overrides/manual cadence remain. The aggregate terminal
campaign exceeded Go's default ten minutes, so its finite target budget is now
thirty minutes without removing cases. All fixtures remain local/disposable;
real operated profiles, LAN/direct traversal, roaming/Pi measurements, T13 native
lifecycle and P17 owner use/explanation are not completed by this packet.

### W07 production TUI outcomes — 2026-10-05

W07 is complete through the same reviewed controls used by the CLI. The
[run summary](../evidence/wan-w07-20261005/summary.md) and
[tracker](wan-status.md#w07--tui-onboarding-and-relay-milestone) record actual
commands, failures, passing oracles and limitations. Ordinary create/join review
shows connection/privacy choices without address/port prompts; Advanced retains
legacy configuration. Fresh Automatic and pre-confirmation Local-only preserve
local capture, and existing installations retain their policy. Private v2/v3
input, exact approval, safe retry/resume and durable revocation use shared controls.
Connection details show dated observed routes separately from file-copy readiness.

Five focused W07 tests pass with the terminal/shared-client/CLI race campaign.
The real production-binary PTY journey verifies bytes in both directions, an
existing-file edit, exact version/hash/author and inviter identity, second-folder
consent, pending exit/daemon restart with unchanged request/attempt, wrong pin,
expiry, root correction, masked paste, resize and terminal restoration. Marked
local service shutdown retains drafts and daemon capture. Full `make check`
passes, including the 925.129-second terminal campaign. Initial fixture failures
remain uncredited in the evidence; the additional-folder journey allows the
existing enrollment bucket 65 seconds to refill without restarting either daemon.

This is one-host local development-service acceptance. Operated defaults,
physical WAN/NAT, direct discovery/traversal, QUIC/ICE/STUN, roaming and Pi evidence
remain later packets. T13 native lifecycle remains incomplete; P17 owner
use/explanation remains deferred. Next sequential packet: **W08**.
