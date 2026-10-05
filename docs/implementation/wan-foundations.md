# WAN block A: foundations

Read the [master plan](../orbit-wan-implementation-plan.md),
[architecture](../orbit-wan-architecture.md), [network protocol](../orbit-wan-protocol.md),
[UX](../orbit-wan-ux.md), [gates](../orbit-wan-design-gates.md) and
[tracker](wan-status.md). Outcomes/checks below are planned and unexecuted.
Either implementation worker owns its assigned packet and integration.

## W00 — Baseline and reproductions

Dependencies: existing repository. Read scope/glossary, current T13/P17 and owning
protocol/persistence/operations/verification contracts. Change: baseline fixtures,
safe harness inventory, source-ownership map, evidence and tracker; preserve all
unrelated changes. Invariants: I01–I28; identify new N01–N10 test surfaces.

Required work:

- Record revision plus actual dirty/untracked file provenance; if using a snapshot,
  record its creation and file manifest. Inventory current commands, schemas,
  listeners, key/pin handling, peers.json, invitations, durable setup and service
  behavior. Existing T13 networking/lifecycle gaps stay labeled outstanding.
- Discover and run `make check`, relevant uncached CLI/control/replication tests,
  and a disposable ordinary LAN/manual create/invite/join/approve/sync flow. Read
  Makefile before claiming its targets include CLI tests. Record known failures
  and assign repairs to their owning T/W packet.
- Reproduce current dependence on explicit reachable addresses and address-change
  failure using disposable processes/network fixtures; this is a missing feature,
  not a claim that the existing private-network contract is broken.
- Identify acceptance entry points, existing fault safety checks and real-host
  resources. Specify the planned network harness's disposable-root, namespace,
  process and port validation; privilege-dependent checks remain explicit.
- Create a source-ownership map for network module, handlers, app wiring, schemas,
  CLI/TUI, service packages and evidence. Propose repair ownership for any relevant
  baseline fault rather than overwriting ongoing unrelated work.

Acceptance evidence:

- Exact baseline commands/results, working-tree manifest and current release gaps.
- Existing identity, protected versions and local bytes preserved in reproduction.
- Nonzero test discovery and one real production-interface LAN/manual sync fixture.
- Safe network/fault harness requirements and precise next eligible packet W01.

## W01 — Contracts and initial design gates

Dependencies: W00. Read WG1–WG3/WG5, current enrollment canonical types and TLS/
HTTP handlers. Change: network/terminal schema additions, canonical types/golden
fixtures, independent admission/connection model, disposable integration spikes,
dependency evidence and owning specs. Invariants: I08–I09, I13, I19–I24, N01–N07.

Required work:

- Execute WG1 direct/WSS inner-TLS spike and bounded stream cleanup/backpressure.
  Pin a supported WebSocket dependency after official API/license and amd64/arm64
  compilation checks; record actual dependencies, not guessed versions.
- Close WG2 registration/profile/relay-token encoding with independent model and
  replay/expiry/identity-substitution/URL scope fixtures. Deliver strict
  `schemas/network-v1.md`; freeze service origin, certificate/key and signature
  representations, including every integer and maximum-valid message.
- Close WG3 with v3 invitation/transcript/status/approval fixtures and v2 coexistence.
  Preserve current DeviceIDs and v1 membership/version bytes. Use test listeners
  and a fixture broker; production rendezvous/relay acceptance belongs to W03/W04.
- Freeze additive control capabilities, network query/policy preview/apply types,
  stable codes, migration intent and private durable setup fields. Specify trust
  injection at the transport seam and enrollment/data separation.
- Model WG5 route/cancellation/network-generation state. Declare finite defaults
  and discover maximum-message/resource tests before dependent implementation.

Acceptance evidence:

- WG1–WG3 closed with executed model/TLS experiments and linked owning-spec updates.
- Strict schema/golden fixtures, including maximum valid and one-over-limit cases.
- Wrong inviter pin blocks secret disclosure; discovery/relay possession grants no
  data access; competing pins cannot overwrite known routing identity.
- Direct and relay transport interface demonstrated; all untested integration
  combinations explicitly recorded. WG4/WG6 remain pending with their assigned packets.

## W02 — Connection manager and preserved HTTPS

Dependencies: W01. Read architecture, WG5 and existing scheduler/client factory.
Change: `internal/network`, transport injection in replication/app, config/peer
route migration scaffold and interface tests. Invariants: I01–I09, I13–I15, N01–N07.

Required work:

- Implement the frozen logical target/transport/observation seam and lifecycle.
  Supply TLS trust from existing replication. Route by immutable device/pin and
  purpose; membership stays validated by the existing request handlers.
- Adapt current direct HTTPS endpoint configuration through the manager. Preserve
  v1 peers.json/manual CLI/service behavior and all current deadlines/body limits.
  No global network behavior is enabled merely by upgrading a legacy install.
- Bound reusable pools, attempts and observations. Wire one manager to the daemon;
  shut it down and join its work before SQLite closes. Scheduler jobs continue to
  use existing stable operation/chunk identities.
- Implement policy generation/candidate validation, injected clocks/adapters and
  the initial WG5 state model. Keep stopped/live controller selection unchanged;
  network operations require the running daemon rather than direct DB fallback.

Acceptance evidence:

- Production LAN/manual two-way sync plus three-peer forwarding and interrupted
  chunk reuse through the new seam; normalized heads, versions and hashes match.
- No identity/counter reset, new mutation ID on retry, unpinned redirect/proxy or
  authorization bypass. Unknown/retired members still fail existing checks.
- Cancellation/restart releases listeners, pools and goroutines; a slow/failed
  peer leaves local capture and other peers progressing within configured bounds.
- Focused race tests and M0 `make check`; record new baseline limitations honestly.
