# T00 baseline and reproductions

Executed 2026-10-03 on the development Linux amd64 host, against revision
`86ae552` plus the preserved dirty working tree. Full revision, initial diff
digest, file hashes, dependencies, filesystem and binary digest are in
[manifest](manifest.json). Historical P/O evidence is unchanged. T00 is complete
as an inventory/disposition packet; none of the failures below is repaired or
accepted as correct terminal behavior.

The harness discovered **10 top-level tests and 13 leaf cases**. The final opt-in
run executed each leaf twice: **26 deliberate failures**, no unexpected fixture
failures. The separate opt-in race run produced the same 13 failures without a
race-detector report. These are failures of the approved behavior, not passing
security or terminal acceptance checks. Ordinary runs skip the opt-in group.

## Current executable and control inventory

- `make check` passed, including amd64/arm64 builds and packaging. The packaged
  binary identifies Orbit 1.0.0, protocol 1, config format 1 and **schema 13**.
  Fresh `init` confirmed SQLite `user_version=13`, WAL and positive limits:
  10 GiB data, 256 MiB metadata, 512 MiB reserve. Current schema supersedes the
  older schema-12 prose in the browser read documentation.
- Bare `orbit` still dispatches to the browser launcher. `orbit help` documents
  `launch/setup/join/invite/requests/devices`, browser-era file commands, legacy
  settings/storage/maintenance/service and relocation. Legacy `filesync` exposes
  history/export/doctor/control and engine commands. Terminal history/deleted,
  reviewed editor sessions and the small TUI are not delivered.
- `serve` defaults both listener flags to empty; `--peer-listen` is explicit.
  The default launcher invokes `serve --control-listen=... --allow-init` without
  peer networking. The installed service's ExecStart also supplies only the
  loopback control listener. Source inspection corroborates this configuration;
  the process fixture executes the same launcher arguments, rather than altering
  a personal systemd service or opening a browser.
- [API inventory](transcripts/inventory-api.json) lists 101 literal registrations
  with source/line ownership. Loopback control requires owner credentials for
  private routes; enrollment request/status are public exceptions under its
  Host/Origin middleware. The peer listener is TLS 1.3 with client certificates
  and handles `/peer/v1/*`, including membership retrieval. It has no enrollment
  request route. T03 must implement network enrollment while retaining data gates.
- CLI adapter selection is inconsistent: browsing/file actions/invitation/device
  controls include live HTTP calls, while setup/join/conflict/restore enter
  `app.WithWorkspace`, which requires the stopped-state lock. `orbit status`
  silently discards its stopped-adapter failure while the daemon is running.
  `callOrbitDaemonAPI` remains a command-local helper, not the shared T02 client.
- [Schema observations](transcripts/inventory-schema.json) show singleton setup
  state containing phase/root/folder/completed/time, without request or endpoint;
  enrollment `request_id` is a global primary key. `init` initializes finite
  limits; `serve --allow-init` follows a different initialization path.

## Candidate dispositions and repair ownership

Use the common command from [commands](commands.md), optionally selecting the
exact test name below. All reproduced cases use current production interfaces.
No disposition claims unauthorized file bytes were obtained or data was lost.

| Candidate | Disposition and actual observation | Smallest regression / owner |
| --- | --- | --- |
| Missing enrollment reachability | **Reproduced.** Default invitation endpoint equals loopback control address; pinned mutual TLS against the peer handler returns HTTP 404 on enrollment. Default launch/service listener settings are separately inventoried. Actual multi-host LAN enrollment remains unexecuted. | `TestTerminalT00InvitationEndpoint`, `TestTerminalT00PeerEnrollmentRoute`; T02 listener startup, T03 transport |
| Missing inviter binding / verification | **Reproduced.** Invitation has no inviting key/certificate binding. Live join submit sends the synthetic capability to an unrelated self-signed HTTPS server and accepts HTTP 200. | `TestTerminalT00InviterVerification`; T03 |
| Wrong-folder capability | **Reproduced.** Folder A's invitation is consumed for a folder B request, which is recorded as pending. Approval/data transfer were not attempted in this case. | `TestTerminalT00WrongFolderCapability`; T03 |
| Same-key second-folder request | **Reproduced.** First request succeeds; the same key and another folder's invitation return HTTP 500 with `enrollment_requests.request_id` UNIQUE collision. The second invitation has already been consumed. | `TestTerminalT00SameKeySecondFolder`; T03 attempt/scope contracts and T05 multiple folders |
| Incomplete join persistence | **Reproduced.** Root/phase and identity survive DB reopen; resuming an existing-root pending join returns completed while inviter still records pending. Schema omits request/endpoint. | `TestTerminalT00JoinPersistence`; T04 |
| Initial join scan readiness | **Reproduced, narrowed.** Join completion returns ready/completed with an existing unsupported symlink; actual scan reports `UNSUPPORTED_OBJECT`, and the link survives. A fatal scan error being swallowed remains **source-only** (`_, _ = c.ws.Scan`); no fatal scan-error or filesystem-fault experiment was run. | `TestTerminalT00JoinReadiness`; T04 must cover both partial result and fatal error |
| Root adoption preview | **Reproduced.** Recursive fixture has one 4096-byte file and an unsupported link. Preview returns only two immediate rows/samples; no size, unsupported-item, incomplete, capacity or reviewed-generation observation. Tree-swap execution is **unexecuted**, not a data-loss claim. | `TestTerminalT00RootPreview`; T04 |
| Live-daemon CLI locks | **Reproduced.** Real `orbit setup --preview`, `join`, `conflicts`, `restore --preview` and `conflicts merge` return `state directory is already owned by another agent`. Live authenticated HTTP setup succeeds on the same daemon. | `TestTerminalT00CLI/LiveAdapters`; T02 shared adapter, T04 onboarding, T08 content controls |
| Finite initialization bypass | **Reproduced.** Actual default-launch `serve --allow-init` args create state without `limits.json`; loader reports zero budgets/reserve. Separate fresh `init` writes positive finite limits. | `TestTerminalT00CLI/FiniteInitialization`; T02 |
| Status omissions | **Reproduced.** Running-daemon status JSON reports running but omits setup/folders/attention despite successful live setup. Detail API retains receipt/time but drops stored `direct=false`. | `TestTerminalT00CLI/Status`, `TestTerminalT00DetailObservation`; T07 |
| CLI whole-file merge | **Source-only.** `handleMerge` calls `os.ReadFile` before the exclusive adapter. No large-file RSS/budget experiment was executed; the live merge-lock symptom is independently reproduced. | T08: exact-head reviewed stream/editor integration with a file exceeding the bounded review buffer; measure RSS and verified resulting bytes |
| Missing editor/deleted/history journeys | **Reproduced** for `orbit deleted` (unknown command). **Source-only** for editor-session absence and `orbit history` dispatch (legacy `filesync history` exists); current help/dispatch are recorded, but no editor/PTY journey was run. | `TestTerminalT00CLI/DeletedCommand`; T06 vocabulary, T08 content workflow, T11 TUI |

Draft runs are retained for honesty: the first pending-join fixture chose a
nonexistent root, so resume failed instead of producing the completed symptom.
The final minimized case supplies an existing joining root. An earlier restore
invocation omitted its required `--source`; the final case supplies it and reaches
the lock failure. Neither draft outcome is counted as a successful reproduction.

## Check results and environment prerequisites

`make check` exited 0. Initial overlapping `make test-race` exited 2 because
packaging tests saw tar EOF/checksum discrepancies while another gate wrote
`dist`; no data-race report appeared. **Serial `make test-race` exited 0.** Both
logs remain recorded; gates sharing packaging outputs must run sequentially.
No unrelated production fix was made for these scheduling-contaminated results.

Read-only SSH `uname -m` succeeded through existing aliases: laptop x86_64,
rpi aarch64, vps aarch64. Tailscale is absent from this host's tested PATH;
remote `tailscale status --json` exited 127 on all three aliases. These facts
establish SSH prerequisites only. Tailscale availability outside the tested PATH,
usable LAN routes, listener/firewall reachability, native packages, systemd
logout/boot behavior and actual cross-host terminal journeys remain unexecuted.
No network/host policy or existing workload was changed.

This packet identifies affected I08/I09/I11/I13/I16/I19–I24/I27–I28. Its failures
exercise scope/identity (I09/I23), lifecycle/finite initialization (I13/I20–I21),
persisted onboarding/readiness (I22/I27–I28), and qualified adapters/status
(I19). No new causal-counter, retirement, reviewed-head, fault-durability or
power-loss proof is claimed. P17 actual owner use and unaided explanation remain
outstanding.

## Handoff and explanation

**Next eligible packet: T01.** Freeze contracts and execute TG1–TG5 design
experiments before dependent repairs. Use these cases as production regressions;
do not weaken them to accept present failures. T01 should specify scoped attempts
and atomic capability admission, persisted exact join observations, finite
initialization, streamed editor sessions, and qualified status provenance.

The transport gap is listener/routing/reachability. The identity gap discloses a
capability before authenticating an inviter. The scope gap consumes a capability
for another folder; the repeated-key gap confuses separate workflows with one
device identity. The adapter gap bypasses live control and collides with daemon
ownership. Screens alone cannot supply authentication, atomic scope checks,
durable phases or lock-safe engine access. This is an implementation-worker
explanation; it does not fulfill the owner's unaided explanation requirement.
