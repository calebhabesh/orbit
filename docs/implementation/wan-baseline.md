# W00 baseline and source ownership

Recorded 2026-10-05. This inventories the existing production implementation,
not automatic WAN capability. Executed results and revision/file provenance live
in [W00 evidence](../evidence/wan-w00-20261005/summary.md).

## Current production interface and state

- CLI: `orbit setup`, `orbit join`, `orbit devices add`, `orbit devices requests`,
  `orbit devices endpoint`, `orbit folders`, `orbit status`, `orbit doctor`;
  legacy `filesync init/serve/identity/pair-approve/sync/membership` remain.
  Invitation/review files are private; ordinary progress must not print secrets.
  No production `orbit network` command or Automatic policy exists yet.
- `config.json` format 1 retains a random persistent 32-byte DeviceID;
  `identity/peer-identity.pem` stores the existing Ed25519 key/certificate in
  private state. SPKI SHA-256 is the pin. An address change never rotates either.
  Fresh initialization creates finite limits before identity/database admission.
- SQLite `CurrentSchema=13`; canonical membership and version envelopes remain
  v1. Terminal controls are v1; invitation/enrollment is v2. Root setup/reviews,
  pending signed requests, exact approval and replay guards use private metadata
  namespaces and existing journals. No W00 migration or schema change.
- `runtime.json` holds finite settings, startup mode and numeric peer/enrollment
  listen/advertise addresses. Default listeners/advertised addresses are empty;
  startup is manual. Invitation creation requires both configured listeners and
  advertised addresses; advertised addresses must be numeric nonloopback origins.
- `peers.json` format 1 holds up to 64 folder/device/HTTPS-origin/certificate
  entries, private regular file capped at 64 KiB. These locate approved members;
  membership and pins still authorize each request. Scheduler work reloads this
  file and reconstructs pinned clients. Missing entries yield “peer endpoint is
  not configured”. CLI address-only refresh reuses the saved certificate anchor.
- Three distinct listeners: loopback authenticated owner control, isolated
  enrollment TLS `/enrollment/v2/{challenge,request,status}`, and certificate-
  required peer TLS `/peer/v1/*`. The daemon writes private actual listener records
  (`control.addr`, `peer.addr`, `enrollment.addr`) and owns the state lock.
  Enrollment authenticates the transferred inviter certificate/pin before sending
  the capability; approval binds exact folder/requester/pin/transcript/revision.
- Existing direct client is a bounded `http.Transport`/30-second HTTP client.
  Enrollment explicitly disables proxy/redirects and compression, uses a 3-second
  dial and 10-second client bound. Peer client does not explicitly reject redirects
  in its constructor: WG1/W02 must audit and freeze this at the new seam; this
  source observation alone is not evidence of an authorization bypass.
- `internal/app` owns identity, database, scheduler, listeners and joined shutdown.
  CLI/TUI use the shared live/stopped controller. Existing login/unattended user
  service packages and launch reuse keep the daemon independent of client lifetime.
  Service enablement does not prove native boot/logout capture.

## Source ownership for W packets

Paths marked proposed do not exist yet. Implement sequentially; shared wiring,
canonical types and migrations have one owner in each active packet.

| Surface | Owning paths | Packet integration |
| --- | --- | --- |
| Connection manager, route observations, adapters/profile/cache | proposed `internal/network/` | W01 types/spikes; W02 manager; W03/W04 service clients; W08–W11 routes/policy |
| TLS/pins, peer handlers, receipts/chunks and isolated enrollment | `internal/replication/{identity,client,server,enrollment,transfer,wire}.go` | W01 seam; W02 injection; W05 v3; W09/W10 equivalent HTTP3 authorization |
| Canonical network/enrollment bytes | `internal/protocol/terminal_enrollment*.go`, proposed network/v3 types; `schemas/{peer-v1,terminal-control-v1}.md`, proposed `schemas/network-v1.md` and golden fixtures | W01 freeze; W05 integration; W14 coexistence |
| Durable intent, peer routes and setups | `internal/repository/{repository,enrollment,onboarding,terminal_operations}.go`; `internal/config/{config,runtime,peers,storage}.go` | W01 record contract; W02/W05 additive persistence; W14 migration |
| Daemon transport/listeners/shutdown | `internal/app/app.go`; `internal/scheduler/{scheduler,retry}.go`; `internal/launcher/{daemon,discovery,launcher}.go` | W02 manager lifecycle and target factory; W11 generations |
| Shared controls, status and durable review/apply | `internal/control/{server,terminal_enrollment,terminal_setup,terminal_lifecycle,terminal_status,doctor}.go`; `internal/control/terminalcontract/`; `internal/controlclient/` | W01 additive types; W06/W12 controls; W14 compatibility |
| CLI/TUI | `cmd/filesync/{main,terminal_setup,terminal_enrollment,terminal_help,terminal_status}.go`; `internal/terminal/` | W06 CLI; W07 onboarding; W12 observations |
| Service implementation | proposed `cmd/orbit-net/` and isolated server modules | W03 rendezvous; W04 relay; W10 STUN; W13 operations |
| Packages and profiles | `scripts/build_packages.go`, `packaging/systemd/`, proposed service profile/package assets | W13 operator/self-host package; W14 default distribution |
| Campaigns/evidence | `tests/terminal/wan_baseline_test.go`, proposed W packet tests, `scripts/validation/`, `Makefile`, `docs/evidence/wan-wXX-*/`, `docs/implementation/wan-status.md` | Each packet owns discovered tests and actual evidence; W15–W17 integration |

Histories, working-basis, publication and cleanup stay in history/repository/
workspace. Network code does not take ownership of them. Retain all P/O/T evidence.

## Safe harness inventory and requirements

Existing executable entry points:

- `internal/testkit.NewDisposable` creates `.filesync-disposable` in a fresh temp
  root; `ValidateDestructiveTarget` rejects missing marker, symlink components,
  root itself and escaping targets. Its refusal tests run in the baseline suite.
- `scripts/validation/host_agent.py` verifies canonical absolute private owner-
  controlled roots, marker type/link count/owner/token and contained paths.
  Signals require saved PID start ticks, executable and exact state path; pilot
  sync interruptions are refused. `test_safety.py` exercises actual refusals.
- Existing T04/T05 process tests build child binaries in disposable roots, bind
  kernel-assigned nonloopback ports and assert control/identity/head/hash results.
  T04 kills only its marked owned receiving child; T05 gracefully stops its
  tracked child handles. W00's failed-route fixture holds its assigned socket
  throughout, avoiding reserve-close port reuse.
- `terminal_private.py` verifies package checksums and ordinary three-host direct
  traffic; SSH is harness control, not an Orbit data tunnel. Abrupt-reset/storage
  campaigns create fresh VM images. Existing pilot roots/services are excluded.

The planned W15 network harness must add the following before fault execution:

1. Require a fresh canonical 0700 owner-controlled root, single-link regular 0600
   `.filesync-disposable` marker with run token, and all config/state/image/log
   targets strictly beneath it. Refuse symlinks, hard-linked files, root itself,
   personal/pilot paths and existing workloads before issuing commands.
2. Create uniquely named per-run namespaces/containers only with explicit
   privilege availability. Record namespace inode/container ID, label/token,
   veth ownership and creation; refuse an existing namespace/container. Keep
   firewall, routes and shaping confined to the owned environment. Cleanup
   revalidates ownership before deleting only those resources.
3. Record child executable, exact state/config path, PID and Linux start ticks
   (or pidfd). Revalidate immediately before signals; join children and report
   cleanup failures. Never discover arbitrary processes by name/port.
4. Bind kernel-assigned ports and hold listeners through handoff where possible.
   Validate requested interface/address and namespace; refuse unspecified/public
   host targets for local faults. Where reserve-close is unavoidable, retry bind
   collisions and verify actual listeners; a free-port probe is no reservation.
5. Bound attempts, delays, loss rules, test lifetime, output and cleanup. Sanitize
   capabilities, control/ICE/relay credentials, private paths and sensitive
   addresses before publication. Record synthetic NAT topology separately from
   actual physical WAN and retain failed/minimized reproductions.

No namespace/NAT/privileged network harness is delivered by W00. Namespace,
netem/firewall, service outage/relay and physical WAN tests remain **unexecuted**.
No remote host is required for W00. Historical release evidence identifies laptop
amd64, Pi arm64 and Oracle VPS amd64; current availability/capacity/operator origins
must be inspected again in W13/W16. Their personal services are not test resources.

## Baseline prerequisites and release gaps

Current T13 follow-up records successful ordinary laptop/Pi LAN and three-host
Tailscale setup/forwarding/retirement/replacement. Earlier tracker introductions
retain stale limitations; use its dated follow-up and
[current evidence](../evidence/terminal-t13-20261005/summary.md) as authority.
Native login/logout/unattended boot remains outstanding in T13. P17 owner use
and unaided explanation remain deferred; automated evidence does not satisfy them.
W00 reruns local baseline checks, not the historical native or VM campaigns.

| Dependent packet | Relevant baseline acceptance to retain / repair owner |
| --- | --- |
| W01 | Nonzero contract/TLS/enrollment/control discovery, protected identity/content; WG1–WG3 experiments must close before production integration |
| W02 | Existing T04 create/join two-way sync, T05 address refresh/three-peer forwarding, replication retry/resume, scheduler/lifecycle tests; manager repairs owned W02 |
| W03 | W01 admission/profile/signature fixtures, existing isolated handler authority; registration/profile work owned W03 |
| W04 | W01 encrypted stream gate and existing TLS/body/deadline/pin tests; broker cleanup/backpressure owned W04 |
| W05 | T03/T04/T05 scoped admission, bootstrap, retirement artifacts, exact membership and durable retries; enrollment integration owned W05 |
| W06 | CLI/control/contract uncached tests, T04 real CLI review/join, secret-file handling; CLI adapters owned W06 |
| W07 | T09/T10 PTY/lifetime/readiness and T04 durable setup; WAN onboarding owned W07 |
| W08 | Existing pinned HTTPS/manual behavior and current-peer authorization; discovered candidate integration owned W08 |
| W09 | Existing handler auth/limits/chunk/retry tests plus WG4 transport subgate; HTTP3 parity owned W09 |
| W10 | W09 parity, W01 coordination/model bounds; packet/socket and ICE integration owned W10 |
| W11 | Retry/resume, original-author forwarding, capture during route failure, scheduler fairness/lifecycle; route policy owned W11 |
| W12 | Qualified T07 observations/control errors and T09 bounded passive queries; network diagnosis owned W12 |
| W13 | Existing package/service safety and T13 remaining native lifecycle; service origins, budgets and readiness owned W13/WG6 |
| W14 | Schema-13/manual peers compatibility, T12 package/adoption and T13 retirement/replacement; upgrade changes owned W14 |
| W15 | I01–I28 fault/model/safety campaigns, retained protected hashes and disposable fencing; combined network campaign owned W15 |
| W16 | Actual host/package provenance plus completed W15; native WAN evidence owned W16, inherited lifecycle stays T13 |
| W17 | All W acceptance plus relevant T13 native lifecycle, full release checks; W17 cannot close on W00 local checks alone |

## New invariant entry points

N01/N05 reuse pinned direct identity, protected-head and interrupted-sync oracles;
W00 does not establish new routes. N02 registration/candidate/replay models belong
to W01/W03; N03 relay opacity/isolation to W01/W04; N04 v3 admission to W01/W05–W07;
N06 resource/fairness to W02–W04/W10–W15; N07 packet/HTTP3 to W09/W10; N08 qualified
controls/PTY to W06/W07/W12; N09 privacy/profile/migration to W01/W13/W14; N10
packaged real WAN to W16/W17. None of N02–N10's new WAN capability is accepted
merely by the existing manual-network tests.

Next eligible packet after W00 acceptance: **W01**, contracts and initial gates.
Use the existing enrollment/TLS handlers and schema-13 baseline; freeze additive
interfaces with actual spikes/models before changing production routing.
