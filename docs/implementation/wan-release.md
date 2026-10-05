# WAN blocks D–E: operations and release evidence

Read [plan](../orbit-wan-implementation-plan.md), [architecture](../orbit-wan-architecture.md),
[UX](../orbit-wan-ux.md), [network protocol](../orbit-wan-protocol.md),
[verification](../verification.md#native-wan-verification) and [tracker](wan-status.md).
All work/checks below are planned and unexecuted. Preserve historical P/O/T evidence
and deferred personal-use/unaided-explanation records.

## W12 — Observations, diagnostics and controls

Dependencies: W06, W07, W11. Change: typed network queries/policy mutations,
doctor/support export, CLI/TUI status/details/help. Invariants: I13, I19–I23,
I27–I28, N01, N05–N10.

Required work:

- Expose qualified mode/profile/route/candidate/freshness and typed failure/next
  action observations. Keep daemon/capture/peer-stored/applied/membership status
  separate. Relay operation is normal useful status, not automatic attention.
- Implement bounded explicit doctor probes for service DNS/TLS, directory, relay,
  direct and UDP. Passive status never activates external diagnostics. Distinguish
  unavailable, untested, expired, quota and verified identity errors.
- Add reviewed privacy/mode/profile settings and support export redaction. Internet
  disablement stops global discovery/STUN/relay, rejects cached WAN candidates and
  drains WAN direct pools while preserving keys/history/files;
  self-host profile validation retains TLS and per-peer identity checks.
- Keep fresh consent under first-setup review and existing installs under manual
  defaults until reviewed opt-in. Verify CLI/TUI results/context/error parity.

Acceptance evidence:

- Service healthy + peer offline, relay connected + file conflicted, direct healthy
  + service down, stale peer receipt, membership fork and blocked capture display
  distinct truthful states in real CLI/PTY flows.
- Privacy-mode packet capture contains no prohibited internet-service traffic;
  settings replay/stale reviews and daemon restart preserve intended policy.
- Doctor cancellation/timeout stays finite; redacted exports/logs contain no
  invitation/ICE/relay credentials or private filenames. No inferred NAT diagnosis
  is presented as measured fact.

## W13 — Operated defaults and self-hosting

Dependencies: W03, W04, W10; WG6 owner. Change: `orbit-net` packaging/config,
signed profiles, service runbooks, operator monitoring/admission controls and
isolated deployment rehearsal. Invariants: I13, I20, N01–N03, N06–N07, N09–N10.

Required work:

- Build separately configurable rendezvous/relay/STUN listeners and packages for
  declared server architectures. Run unprivileged with explicit directories,
  bounds, certificate paths and service supervision; omit owner/sync control.
- Produce a concrete default profile with actual operator, controlled DNS/TLS,
  profile signing key process, origins, transports, expiry, privacy/retention,
  contact/incident procedure and self-host override. Example origins are test-only.
- Define finite per-device/global admission and egress budgets, metrics/alerting,
  quotas under shared NAT, key/profile/certificate rotation, restart/rollback,
  patching and safe shutdown. Distinguish transient leases from configuration that
  requires backup. No unlimited availability/bandwidth commitment is implied.
- Rehearse self-hosting with the packaged executable/profile and custom trust;
  prepare deployment artifacts before external activation. Use only deployment
  authority actually available; record missing operator credentials/resources.
  Never repurpose personal folders or existing VPS services for fault tests.

Acceptance evidence:

- WG6 closed with actual operator/profile readiness or packet left incomplete with
  precise missing condition. A mocked operator or placeholder URL cannot pass.
- Package/server launch, certificate/profile expiration and rotation, overload,
  STUN abuse limits, service restart, revoke/disable and rollback are exercised.
- TLS-inspecting outer relay cannot read synthetic end-device content; tests
  confirm endpoint isolation and server memory/FD/bandwidth bounds at declared load.
- Self-host profile works with production clients without disabling verification;
  runbook distinguishes user installation from operator provisioning work.

## W14 — Migration, mixed versions and packaged defaults

Dependencies: W05–W07, W12, W13. Change: deliberate migration/configuration,
package builds/profile inclusion, compatibility tests/completions and runbooks.
Invariants: I07–I09, I20–I24, I28, N01, N04–N05, N09–N10.

Required work:

- Ship valid signed release profile in ordinary amd64/arm64 builds; freeze trust
  and update/expiry behavior. Development overrides are never production defaults.
- Preserve old state/keys/roots, version/membership encoding and v2 invitations.
  Import existing manual endpoints with existing explicit policy; automatic mode
  needs reviewed opt-in for previous users. No automatic identity rekeying.
- Negotiate additive network/enrollment/control capabilities. New↔old peers use
  supported explicit paths; unsupported automatic routing yields a precise error
  and manual choice, without silently weakening enrollment or wire validation.
- Deliver CLI/help/completions, startup/service adoption and self-host packages.
  Upgrade/uninstall preserve files/keys/history; rollback refuses incompatible
  state safely or restores a documented compatible backup.

Acceptance evidence:

- Actual legacy-state fixtures reopen with stable IDs/counters/hashes; interrupted
  migrations/settings writes recover or refuse safely. Test new/old invitation and
  capability combinations, plus unsupported future version.
- Native packaged TUI/CLI/service run with real included profile; explicitly test
  no-profile/expired-profile behavior and deliberate self-host override.
- Manual/local-only modes preserve privacy and existing configuration; confirmed
  Automatic mode removes normal IP/port/Tailscale steps. M4 includes operator
  runbooks and package provenance, without claiming unexecuted WAN behavior.

## W15 — Integrated failures, security and resources

Dependencies: W11–W14. Change: reproducible disposable network/process harness,
adversarial/fuzz/model campaigns, minimized regressions, measurements and fixes.
Read existing verification/fault-safety contract. Invariants: I01–I28, N01–N10.

Required work:

- Deliver documented network harness commands with explicit marker/root/process/
  namespace validation and privilege prerequisites. Synthetic emulator topologies
  include LAN, reachable public route, independent/dependent mappings, double NAT,
  blocked UDP/TCP, service outage, loss/reorder/delay, bandwidth cap and MTU effects.
- Test malicious directory/relay/unknown endpoint, pin substitution, replay/expiry,
  malformed signed metadata/STUN, candidate SSRF, shared-NAT floods, oversized
  frames and quota exhaustion. Extend independent state models and save traces.
- Combine route switches with chunk/receipt loss, crashes during enrollment and
  existing safe publication/GC/conflict/retirement scenarios. Diagnose and fix
  reproduced defects in their owning modules before broad retesting.
- Measure idle/active CPU, memory, FDs, goroutines and service egress on laptop/Pi
  profiles, connection/reconnection latency and direct/relay throughput. Include
  small/mixed/large fixtures, slow receiver and adversarial admission; record bounds.

Acceptance evidence:

- Complete scenario/invariant matrix with exact head/hash/identity/receipt/authority
  oracles. Unsupported or unavailable cases are unexecuted, not passing rows.
- Harness refuses missing marker, escaped/symlinked/personal paths and unrelated
  live processes. Privileged destructive checks are never hidden in ordinary tests.
- Relevant fuzz/race/model/CLI/PTY/process campaigns, `make check`, uncached full
  race, demo and packages pass with provenance. No leaks or unbounded allocation
  under declared limits; protected captured bytes and fair large work survive.
- Quantitative results name network/host/topology/library versions and limitations;
  no universal NAT success percentage is derived from a small simulated matrix.

## W16 — Real WAN and ordinary setup campaign

Dependencies: W07, W11, W13–W15. Change: safe native runners, real network fixtures,
sanitized transcripts and recorded physical deployment measurements.
Invariants: I01–I28, N01–N10.

Required work:

- Use packaged default builds on fresh dedicated state/roots across at least two
  physically separate networks; no Tailscale, SSH port forwarding or private VPN
  may carry the route for the native-WAN acceptance cases. SSH may administer hosts
  separately. Record the actual route and prohibit harness tunnel fallback.
- Ordinary create/invite/join/approve must work with no shared LAN history or
  manual connection parameters, in CLI and TUI. Use the bundled hosted profile
  without editing endpoint files. Confirm actual file transfer both directions.
- Exercise a supported direct WAN case, forced relay with UDP blocked in the
  dedicated environment, an address/interface change, relay/rendezvous restart,
  second folder and laptop/Pi/VPS forwarding/conflict/restore scenarios.
- Record observed topology, verified access constraints, actual hosts/versions,
  direct/relay classification, timing, throughput/resource and profile/service
  provenance. School/corporate/CGNAT/IPv6 cases require actual access/evidence before
  claims; simulations remain in W15. No fault injection on existing VPS workloads.

Acceptance evidence:

- At least one real direct cross-network sync and one real relay cross-network
  sync, plus ordinary default-profile first-time WAN onboarding and reconnect.
  Lack of a testable direct network leaves that acceptance item unexecuted.
- Qualified per-device stored/applied/conflict state and protected hashes/heads
  agree; relayed endpoint receipts identify the device, not the broker.
- Actual laptop/Pi/VPS and packaged CLI/TUI observations supplement existing engine
  evidence; old manual/SSH-relay campaigns do not establish this new WAN guarantee.
- No claim of success on an inaccessible school/corporate network. Keep exact
  missing native conditions in status; unavailable hardware is not fabricated.

## W17 — Combined release and handoff

Dependencies: W00–W16 and relevant remaining T13 technical gates. Change: owning
spec reconciliation, release/runbooks/demo/case study/portfolio evidence and tracker.
Invariants: I01–I28, N01–N10, approved engine scope and WAN requirements.

Required work:

- Reconcile actual implementation with scope, UX, architecture, network/causal
  protocol, persistence, limits and dependencies; resolve every contradiction in
  its owner. Link the actual gate outcomes and tests, with honest limitations.
- Finish baseline T13 technical networking/lifecycle/native checks or record their
  still-open acceptance; W status cannot overwrite historical T/P/O evidence.
  Personal-use/unaided-explanation remain deferred, with records preserved.
- Reproduce clean/snapshot-provenance build, packages and necessary regression
  campaigns after final material changes; demonstrate ordinary default setup,
  direct/relay operation, conflict/restore and failure recovery from documented steps.
- Publish evidence-backed supported networks, operator ownership/budgets, privacy,
  profile expiry/update, self-host/manual alternatives and troubleshooting.
  Case study explains architecture, tradeoffs and measured behavior without a
  Syncthing parity claim or invented numbers.

Acceptance evidence:

- Every W acceptance criterion, gate, WAN requirement and inherited release check
  has a supporting artifact. Required unavailable checks keep release incomplete.
- Fresh developer reproduction, packaged default profile and native campaign
  provenance agree. Secrets/personal data stay outside published evidence.
- Resume/portfolio statements link measured results; explain sync/traversal versus
  relay/replica roles. Final handoff names supported conditions, operator actions,
  future extensions and deferred owner observations distinctly.
