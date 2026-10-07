# Security, resource limits and operator interface

## Native WAN operations amendment — 2026-10-05

Status: W00–W08 local implementation/evidence recorded in [W status](implementation/wan-status.md); ordinary reviewed WAN setup is implemented and hosted/native acceptance remains pending.
[WAN UX](orbit-wan-ux.md), [architecture](orbit-wan-architecture.md) and
[network protocol](orbit-wan-protocol.md) own automatic routes, profiles,
infrastructure admission and finite engineering limits. Earlier sections below
retain their direct/manual-network baseline and dated evidence.

Fresh setup reviews Automatic mode and service metadata visibility before global
announcements. Existing installations retain reviewed manual settings until opt-in.
The default profile must identify an actual operated rendezvous/relay/STUN deployment;
W13/WG6 supplies origins, certificate/profile authority, finite admission/egress
budgets, monitoring, privacy/retention and lifecycle procedures. The relay is an
opaque intermediary, separate from the optional trusted full replica.

Public HTTPS/WSS origins use normal TLS/DNS verification from an approved profile;
device sessions keep existing pins/mTLS and request authorization. Profile selection
does not change local control endpoint rules. Peer/enrollment route handlers remain
separate and never expose loopback owner control. Router/firewall/VPN policy and
privileged host settings remain operator-managed. Opening unprivileged optional
direct listeners follows the reviewed mode; failures may use relay without claiming
direct reachability. Network doctor uses explicit bounded probes and typed reasons.

Local-only disables internet announcements/STUN/relay; manual/private mode preserves
Tailscale/WireGuard paths; self-hosted mode reviews a private profile/trust using the
same guarantees. Changing mode/profile neither retires peers nor rekeys/deletes
data. Redact network session/invitation secrets and sensitive endpoint/file details
in diagnostics. Support claims name tested topologies: raw port 443/WSS availability
does not guarantee passage through every school/corporate proxy or firewall.

Status: implemented baseline. The limits below are configuration/admission bounds; release evidence describes the workloads actually exercised.

## Active terminal interface contract

The owner selected the [terminal UX](orbit-terminal-ux.md) and
[T00–T13 plan](orbit-terminal-implementation-plan.md) on 2026-10-03.
T00–T12 provide terminal controls and entry/packages; the terminal tracker records
actual evidence. Earlier browser/P/O notes retain their dated scope. The contract is:

- Bare `orbit` opens a small TUI in a TTY, with concise status for non-TTY use.
  Independent named CLI commands invoke the same authenticated local controls.
  Normal file commands can infer their registered folder from the current
  directory; ambiguity requires a picker or explicit selection.
- Use one validated finite-limit initializer and expose budgets/capacity during
  setup. Legacy missing-limit state is reported honestly, with reviewed migration.
- Keep peer/enrollment networking separate from loopback owner control. Prepare
  intended listeners consistently from CLI and service. A usable LAN or existing
  Tailscale path remains a prerequisite; do not change host firewall/VPN settings.
- Inviter identity is authenticated before sending invitation secrets. Private
  prompts/stdin/files replace secret argv. Explicit transfer output is deliberate;
  automatic logs, JSON progress and support exports remain redacted.
- Exiting a terminal client leaves the daemon running. Running, login startup,
  unattended prerequisites, root health and actual capture are distinct facts.
  Unattended lingering changes stay explicit owner steps.
- Errors preserve user inputs and return stable codes, retryability and a safe
  next action. Noninteractive mutations require necessary reviewed inputs.
  Keep legacy commands/state/services compatible through documented entry changes.

T01 freezes exact CLI/JSON/script contracts. T02–T12 provide production evidence;
native release and owner-use evidence remain T13.

## Orbit extension: local launch, bootstrap security & product settings

Status: frozen by gate outcomes [G01 and G05](orbit-design-gates.md). Implemented by packets O01/O02/O03.

1. **Singleton Daemon Locking and Ownership**:
   - The launcher and daemon enforce exclusive ownership of the state directory via `.agent.lock` using `state.Acquire`.
   - If a daemon is already active on the state directory, the launcher reuses the existing process and control interface rather than launching a duplicate.
   - Initialized vs uninitialized state is detected at startup.

2. **One-Use Browser Bootstrap Handoff (Invariant I21)**:
   - The launcher initiates browser sessions using a short-lived (60s TTL) high-entropy bootstrap token passed as a URL fragment (`/#bootstrap=<token>`).
   - The frontend immediately clears the fragment (`history.replaceState`) and exchanges the token via loopback `POST /api/v1/auth/bootstrap`.
   - The exchange enforces strict loopback `Host` (`127.0.0.1`, `localhost`, `[::1]`) and `Origin` headers, rejecting external host names to prevent DNS rebinding attacks.
   - Tokens are consumed immediately on first use; replays and expired requests are rejected.
   - Upon successful exchange, the server issues an `HttpOnly`, `SameSite=Strict` session cookie and a CSRF token.
   - **Session vs Daemon Synchronization**: Closing browser tabs or logging out terminates the UI session, but does **not** stop the background daemon or peer synchronization.

3. **Product Settings Separation**:
   - Product display preferences (device label, workspace display names, default workspace, UI theme) are stored in a separate `settings.json` or SQLite table.
   - `config.json` remains strictly validated at `format_version: 1` (`format_version`, `device_id`, `created_at`).
   - Orbit adopts existing `.filesync-internal` directories cleanly, and rejects unsupported newer database schemas (a `user_version` above `repository.CurrentSchema`) to preserve recoverable state (Invariant I20).

## Trust and authentication

Bare `orbit` and `orbit tui` require both input/output TTYs for the keyboard
interface, discover legacy/default state without bypassing ambiguity, and reuse
or start the selected daemon. Pipes and --json use ordinary status without
starting a daemon. q/Ctrl-C/SIGTERM cancel client queries/tools and restore the
terminal; committed work keeps its independent lifetime. Reviewed content sessions
remain controller-owned. Trusted external tools use direct argv and prlimit.
The terminal desktop entry has Terminal=true. `orbit legacy-browser` (alias
`orbit launch`) freezes explicit browser compatibility; embedded assets remain.
Ordinary builds/packages add no browser or GUI dependency. Packages distribute
completions, runbooks, licenses and the filesync/orbit service alias. Standalone
upgrade preserves customized service units. See [operator guide](runbooks/terminal-operator.md)
and [install/adoption](runbooks/install.md). Native logout/boot/LAN/Tailscale
and actual owner use remain release evidence obligations.

One owner; all enrolled replicas may read authorized folder contents in plaintext. Use established TLS with explicitly pinned identities and mutual authentication. Pairing exchanges identity fingerprints out of band and requires explicit local approval. Initial v1 needs no unauthenticated public pairing endpoint. Protect private keys/configuration with owner-only permissions and redact them from logs/support bundles.

Peer authorization is per folder and per operation. Every chunk request names an authorized version in that folder and a manifest chunk; knowing a digest is not sufficient. Rate-limit malformed and oversized requests before expensive allocations. Separate the peer listener from the loopback control listener and apply independent authentication policies.

Local control: loopback binding, strict Host/Origin validation, no permissive CORS, authenticated mutations and reads containing sensitive metadata. Use an owner-readable local CLI credential; establish a browser session with a short-lived one-use bootstrap secret, then HttpOnly/SameSite cookies and CSRF protection. Avoid long-lived secrets in URLs, browser storage, logs or shell command arguments. Freeze the exact bootstrap flow with tests in P13. Local malicious processes under the same OS account are outside isolation guarantees.

Use request timeouts, TLS verification, body limits, bounded decompression (or disable it initially), validated integer ranges, escaped UI filenames, and path-safe filesystem operations. Bind peer listener to explicitly configured interfaces. Existing private-network reachability is an operational prerequisite; the application still authenticates peers. Document needed ports without changing the owner's firewall or VPS services automatically.

P05 leaves the peer listener disabled unless `serve --peer-listen <address>` is
provided. `identity --certificate` exports only the public certificate plus the
device/key-pin display; `pair-approve` requires the peer device ID and key pin
received out of band. There is no network enrollment endpoint. Initial pairing
installs the same canonical folder membership revision on both devices.

Background outbound synchronization reads owner-only `peers.json` in the
state directory at startup. It contains `format_version: 1` and at most 64
`peers` entries, each with canonical hex `folder`/`device`, an HTTPS origin
`url`, and a public `certificate` path (relative to state or absolute).
Endpoints locate previously approved members; they do not authorize membership.
Malformed endpoints refuse startup; request construction and handlers reject
unapproved certificate pins. The scheduler reloads endpoint intent each work
cycle; T05 address refresh retains the reviewed certificate. W02 invalidates
new requests from superseded pools and lets finite in-flight requests drain. Startup and the configured reconciliation interval
queue bounded, coalesced pulls from these peers; each participant must configure
the peers it pulls from. The daemon's identity authenticates those requests.
Scheduled scans and publications serialize within each folder. Other folders
can progress concurrently. Connection clients and idle sockets are reused and
closed at shutdown; an absent bandwidth cap passes no limiter.

`init` writes owner-only `limits.json` with an initial finite 10-GiB data
budget, 256-MiB metadata admission budget and 512-MiB free-space reserve.
Its version-1 fields are `data_budget_bytes`, `metadata_budget_bytes` and
`free_space_reserve_bytes`; all must be positive. Select smaller/larger limits
before starting the daemon and restart after changes. Repository open reads
these limits for daemon and CLI operations alike. Legacy states without this
file retain their prior limits until `init` is rerun; diagnostics must not
describe their data budget as finite. The prepared personal pilot uses 1 GiB.

The user service retains `NoNewPrivileges` and memory/descriptor limits without
filesystem namespace sandboxing. On the tested Ubuntu laptop/VPS, namespace
creation selected AppArmor's `unprivileged_userns` profile and descriptor-rooted
folder access failed despite matching permissions/registration. Native file
capture must pass before declaring service readiness. Unix ownership already
prevents this unprivileged account from modifying system-owned directories.
No host AppArmor, firewall or VPN policy is changed by installation.

## Initial engineering limits

| Resource | Baseline | Behavior at limit |
| --- | --- | --- |
| Active members per folder | 16 configured maximum; release evidence uses 3, optionally 4 | Reject enrollment beyond configured supported cap |
| Historical identities per folder | 64 | Require explicit future migration/design, never truncate vectors |
| File size / chunk size | 64 GiB / 1 MiB | Block oversize file; preserve prior state |
| Metadata response/request | 8 MiB maximum, bounded parser | Reject before unbounded allocation |
| Inventory page | At most 128 summaries and byte cap | Continue through snapshot-bound cursor |
| Path | 4096 UTF-8 bytes, 255 per segment, 128 segments; actual filesystem may be stricter | Explicit unsupported-path status |
| Concurrent heads / parents | 64 each | Backpressure/review; never discard conflicts to fit |
| Transfers / hashing workers | 4 chunk transfers, 2 hash workers globally; Pi profile starts at 2/1 | Fair queue, bounded buffers |
| Queued in-memory tasks | 1024 lightweight IDs | Coalesce durable work; apply backpressure |
| Retry attempts | 5 transient attempts per work cycle with capped exponential jitter | Visible retry-exhausted state; later reconciliation/manual retry may restart |
| Full reconciliation scan | 5 minutes, configurable; debounce notifications | No overlapping unbounded scans |
| Full-content rescan | Daily, configurable and spread over work budget | Same-size/timestamp changes eventually inspected |
| Metadata budget | 256 MiB starting soft admission cap, including WAL accounting | Pause admission before hard disk exhaustion; preserve recovery reserve |
| Data budget | Explicitly selected at folder/device setup; preview usable capacity | Refuse unbounded implicit allocation |
| Free-space reserve | 512 MiB initial target per storage filesystem, configurable | Pause growth; preserve diagnostics/recovery capacity |
| Bandwidth | Optional global and per-peer cap; unlimited means no configured network cap, not unbounded memory | Token-budgeted streaming |

Restart loading selects active durable tasks before applying the 1024-task
limit, so completed history cannot hide pending work. Equivalent scans and
peer pulls coalesce without resetting a running task. New task rows obey the
metadata admission budget; stored retry errors are limited to 2048 bytes.
Completed task records currently remain in SQLite. The soft metadata cap can
therefore eventually require operator maintenance; automatic terminal-task
pruning is not implemented, and state updates needed for recovery may exceed
the admission threshold.

P00/P12 must reconcile these values with actual serialization sizes, open-file limits, Pi memory, file size/staging needs and workload. Body/page/vector limits must be compatible; test maximum valid envelopes and reject over-limit envelopes explicitly. A configured cap does not establish tested performance at that cap. Metadata limits include non-version tables and operation logs; define separate bounded pruning for diagnostic/idempotency records where safe.

Fairness baseline: round-robin among folders/peers, then small-file preference with aging so large files make progress. Reuse completed chunks. No speculative download of unlimited historical content. Admission accounts for the full pending storage plan; suspended tasks release memory while keeping required durable pins.

## Required CLI/control operations

Names are provisional; all return structured output with `--json` for automation and useful human messages by default.

| Group | Operations and contract |
| --- | --- |
| Agent | init, serve, stop/status, configuration validation |
| Peers | identity show/export, pair approve, list, revoke/retire, membership export/import/preview |
| Folders | add with root validation, initial preview/adopt, list, pause/resume, remove registration preserving files |
| Work | scan, sync, retry, cancel; durable operation ID and queryable progress |
| Files | list, history, inspect availability, export retained version |
| Conflicts | list/show, select, keep-copies, submit-merge with reviewed-head token |
| Restore | preview and execute against reviewed heads |
| Storage | usage, retention preview/change, GC preview/run, integrity check, repair |
| Safety | root revalidate, mass-deletion preview/approve with generation token |
| Diagnostics | doctor, logs, support export, protocol/build/status output |
| Maintenance | backup, migrate/check, recovery inspection and safe identity reset workflow |

Distinguish canceling a transfer from deleting a file, removing a local folder registration from propagating deletion, and retiring a device from erasing its disk. Confirmations required by the product should describe concrete affected paths/versions and use stale-preview protection. CLI scripting can provide explicit reviewed tokens; a generic `--force` must not bypass invariants.

Every mutation accepts an idempotency key or equivalent stable operation identity. Persist request fingerprint and result; replay with different parameters fails. For finite idempotency retention, explicitly reject expired replay or require a fresh reviewed operation; do not silently execute an old destructive command again. Persist important multi-step operations through restart.

Stable error categories: `UNAUTHORIZED`, `MEMBERSHIP_MISMATCH`, `INCOMPATIBLE_VERSION`, `INVALID_MANIFEST`, `INVALID_PATH`, `STALE_VIEW`, `ROOT_UNAVAILABLE`, `STRUCTURAL_CONFLICT`, `CONTENT_PENDING`, `CONTENT_EXPIRED`, `CONTENT_UNAVAILABLE`, `DISK_BUDGET`, `IO_ERROR`, `UNSTABLE_FILE`, `RETRY_EXHAUSTED`. Include retryability and safe next action; do not encode decisions by parsing human text.

## Status and UI

Three views only: folders/devices/pending work; files/history; conflicts. Show last contact and directly known per-peer stored/applied state; distinguish unavailable, stale and unknown. A VPS receipt is not a Pi receipt. A blocked path can coexist with successful unrelated work. History identifies expired bytes without pretending restore is available.

Conflict views show all reviewed heads and content availability. UI refresh after stale mutation responses; never resubmit with a new token without showing changed versions. Escape filenames and log messages. Directory resolution and bulk deletion preview actual affected paths and new arrivals.

## Observability

Structured events: operation ID, folder pseudonym, version ID, peer ID, phase, error code and durations; no file contents, keys or tokens. Filenames/paths are sensitive and omitted/redacted by default. Support export previews categories and writes locally for owner review; it never uploads automatically.

Metrics: captured files/bytes, hash and scan duration, metadata/content bytes sent, verified chunk reuse, queue depth/age, retry causes, conflicts, staging/quarantine/object/metadata bytes, publication recovery time, and resource high-water marks. Bound logs and diagnostic records. Doctor checks identity/state permissions, root availability, free capacity, membership mismatch, protocol compatibility and pending recovery.

## Packaging and lifecycle

Pin a supported Go release and dependencies in P00, verify SQLite driver packaging on Linux amd64/arm64, and record licenses. Use standard HTTP/TLS/hash packages where suitable. Choose a maintained SQLite driver through a build/runtime spike; do not force a static binary claim without checking linkage. UI build is embedded; users need no Node runtime.

systemd user service: graceful cancellation, restart behavior, writable paths, configured roots and limits documented. Hardening must allow arbitrary explicitly configured user roots without pretending a fixed sandbox covers every setup. Logout/boot persistence may require user-service lingering; document owner steps instead of enabling privileged settings silently.

Upgrade stops the agent, obtains a consistent backup, checks migration compatibility, applies migration transactionally where supported, and runs health checks before reopening replication. Recovery from an old metadata backup creates a fresh identity and safe reenrollment; it cannot resume rolled-back author counters. Distinguish binary rollback with unchanged DB from schema/data rollback. Newer DB/protocol versions fail clearly. Uninstall removes executables/service registration while preserving roots/state unless explicitly requested otherwise.

## O07 workspace reads and preview policy

The local control API adds authenticated `GET /api/v1/browse`, `/search`,
`/browse/details`, `/browse/history`, `/browse/deleted`, and `/content`.
All require a selected, registered workspace and the local identity's active
membership when a membership is configured. They retain bearer/browser-cookie,
loopback Host/Origin and browser-session enforcement. Invitation possession
cannot authenticate a data read. Current schema is 13 (T00 inventory); newer schemas are
refused through `repository.CurrentSchema`.

Directory queries accept `folder`, relative `path` (empty only for root),
`sort=name|size|mtime|kind`, `direction=asc|desc`, `limit=1..200` and `cursor`.
Directories sort first, then the selected value, then the exact path ascending
for deterministic ties. Cursor binding includes workspace, query/directory,
generation and page parameters; malformed/mismatched cursors return 400 and
stale generations return 409. Paths are validated before normalization, so
absolute, traversal, reserved and empty-segment paths cannot alias valid ones.

Search accepts `q` (maximum 256 UTF-8 bytes, trimmed) and a page limit/cursor.
Matching is a literal substring of the full relative path, with SQLite's ASCII
case folding; `%` and `_` are ordinary characters. Implicit directories can
match. This is local knowledge, not remote live state or full-text search.
History and Deleted files also paginate; history ordering is counter descending
with author ties for presentation, never a cross-device winner rule.

Content URLs contain only `folder`, exact `author`, decimal `counter`, and an
optional `preview=text|raster`. They require authentication on every request;
there is no bearer capability or filesystem path in the URL. Native attachment
responses stream without constructing a whole-file browser Blob and support
HTTP byte ranges and 416 for unsatisfiable ranges. CLI export uses the same
pinned read. Content responses replace the ordinary absolute write timeout
with a 30-second per-write idle timeout, allowing progressing large responses.
A disconnected response releases pins; a new authenticated range read can
resume the exact version if its content is still available.

Text preview is UTF-8 without NUL, at most 1 MiB, served as `text/plain`.
Raster preview is PNG/JPEG, at most 10 MiB encoded and 16,000,000 decoded pixels;
GIF/APNG animation and active/vector formats use attachment download. Decoder
header validation precedes browser preview; a malformed full image may still
fail to render. All responses are `no-store`; content additionally uses
`nosniff`, `sandbox; default-src 'none'`, and `no-referrer`. Attachment filenames
are encoded with `mime.FormatMediaType` after control-character removal.
Eight simultaneous HTTP content requests are admitted; excess requests return
429. Manual read leases are capped at 128 records and 300 seconds, derive
chunks from the requested manifest, and reject supplied digest mismatches.

Headless `orbit browse`, `orbit search` and `orbit details` output JSON and use
the same controller operations, falling back to the running daemon's control
API. For example: `orbit browse --state STATE --folder HEX --path docs --limit 50`
and `orbit search --state STATE --folder HEX --query notes`.

## O09 file mutations: import, mkdir, move, delete

The local control API adds authenticated mutation endpoints:
- `POST /api/v1/files/import`: stream or upload file into workspace. Requires `folder`,
  `path`, optional `overwrite=true`, and optional `reviewed_token`. Returns `ImportFileResult`.
- `POST /api/v1/files/mkdir`: creates a directory with durable versioning. Requires `folder` and `path`.
- `POST /api/v1/files/move`: renames or relocates a file or directory within workspace.
  Requires `folder`, `source_path`, `dest_path`, optional `overwrite=true`, and
  optional `reviewed_token`.
- `POST /api/v1/files/delete`: deletes a file or directory. Requires `folder`, `path`,
  and optional `recursive=true`.

All mutation requests accept an `Idempotency-Key` header (or JSON field) cached for
24 hours. Replays with matching parameters return the cached result; replays with
changed parameters return 409 `IDEMPOTENCY_CONFLICT`.

Headless CLI commands provide parity with the control API (Invariant I19):
- `orbit import --state STATE --folder HEX --path PATH --file LOCAL_FILE [--overwrite]`
- `orbit mkdir --state STATE --folder HEX --path PATH`
- `orbit move --state STATE --folder HEX --source SRC --dest DST [--overwrite]` (alias: `orbit rename`)
- `orbit delete --state STATE --folder HEX --path PATH [--recursive]`

Commands connect to the live daemon via `.agent.lock` control URL, or execute directly
against repository/workspace if the daemon is stopped.

## Change local folder location

Settings → Workspaces & Sync Folders → **Change location** invokes
`POST /api/v1/folders/relocate` with `folder`, `expected_path` and `path`.
The equivalent stopped/live CLI is:

```sh
orbit folders relocate --folder FOLDER_ID --from /home/you/Orbit --to /home/you/Documents/Orbit
```

Choose an unused destination directory name with an existing parent. Stale
current-location reviews, overlapping roots/state, symlinked parents and
occupied destinations are refused. Close editors and other folder users first.
The operation waits for local IO, preserves the prior paused/active setting,
and refreshes file watching at the new location. On the same filesystem it
moves the directory. Across filesystems it verifies a copy and retains the
original; `source_retained` and `source_path` identify that safety copy in the
result. Review it for late edits before removing it manually. Interrupted or
refused copies can leave private `.orbit-relocation-*` staging directories.
Retry/revalidation recovers durable intent before scanning the folder again.

## T01 terminal operation and lifecycle freeze

Use the [typed terminal schema](../schemas/terminal-control-v1.md) for operation
families, strict decimal-string integers, capability/version rules, human/JSON
stdout/stderr and stable script exits. Use [TG3/TG5](terminal-design-gates.md)
for exclusive adapter selection, finite initialization, entry dispatch and
manual/login/unattended observations. TG1 adds an isolated bounded enrollment
listener plus explicit peer/enrollment addresses; local owner control remains
loopback-authenticated and mandatory client certificates remain on peer data.
No TLS HTTP failure authorizes direct database fallback or certificate bypass.
The older base-only manual-pairing description remains legacy behavior; new
terminal enrollment is the approved extension with production acceptance still
open. Never infer unattended operation from an enabled service. T01 creates no
listener, service policy, privilege change or schema migration.

## T02 shared client and lifecycle

Terminal lifecycle/settings controls now use `internal/controlclient`, with
strict numeric loopback endpoint selection, owner-only regular credential files,
no symlinks/proxy/redirects, a 10-second client deadline, 16-KiB response headers,
and 1-MiB metadata/error bodies. Typed live calls first negotiate the terminal version/capability. Strict terminal
results reject unknown/duplicate fields. Successful responses bind the selected device
through `X-Orbit-Device`. A held state lock selects authenticated HTTP; an HTTP
failure never selects direct SQLite. Stopped operations acquire the exclusive
state lock before recovery or controller access. Legacy daemon helpers and the
browser launcher reuse this transport. Exact read/upload support remains T08.

`launcher.EnsureDaemon` coordinates launch attempts with `.launch.lock`, then
checks `.agent.lock`; the latter remains the daemon's exclusive ownership.
Authenticated readiness must succeed before reuse/start reports success. Client
context cancellation or exit leaves the child running. Explicit `filesync stop`
checks a private PID, a pidfd and an open descriptor to the selected lock before
signalling, refusing a stale PID. This requires Linux pidfd and `/proc` access.

`init` and auto-initializing `serve` use the same locked initializer. Finite
limits are durable before creation of a new device identity. A missing config
beside existing database/certificate state is a recovery error, not permission
to rekey. Existing missing-limit states are preserved and return
`LIMITS_REVIEW_REQUIRED` through runtime settings inspection. Reviewed repair
writes `runtime.json` atomically; it becomes authoritative over `limits.json`.
Existing per-folder retention remains authoritative; nonzero terminal
`retention_seconds` is rejected until a corresponding reviewed control exists.

To review or inspect the additive runtime controls:

```sh
orbit settings runtime --state /absolute/private/state
orbit settings runtime --state /absolute/private/state --request-file /private/settings-request.json
orbit settings runtime --state /absolute/private/state --operation OPERATION_ID
orbit service review --state /absolute/private/state --json
orbit service apply --state /absolute/private/state --request-file /private/service-request.json --json
```

A request file contains a complete typed `Mutation` with a fresh random
32-byte hex operation ID and the exact returned review. Change only the intended
settings/action, retain the review, and repeat the same file to recover a lost
response. Changed inputs under the same ID fail. Runtime settings are desired
configuration: budgets/concurrency/bandwidth and the peer listener take effect
on restart, and the operation effect explicitly says restart is required.
Listener and advertised addresses are independently validated; advertising
loopback/unspecified addresses is rejected. CLI and service `serve` read the same
persisted peer listener. Enrollment addresses are saved but T03 must provide
the isolated enrollment listener; T02 does not claim network enrollment works.

The compatible `service enable/start/stop/restart/disable` verbs remain available;
review/apply is the explicit durable script workflow. Running, enabled, mode,
root health and capture health are separate observations. Terminal capture health
is conservatively false until T07 supplies current capture evidence; the legacy
`capture_successful` field still means some historical version was captured.
Unattended verification is false until native lifecycle validation. Unattended
enablement checks lingering first and returns the owner step
`loginctl enable-linger USER` when absent; Orbit never executes it.

Service dispatch first claims the admitted operation durably; concurrent retries
cannot repeat its external command. A caller arriving while another stopped
adapter or startup holds ownership may receive a connection-not-ready error;
it retries the same ID after the selected endpoint becomes ready, without direct
database fallback. Service commands refuse a user unit for another/unverified selected state and
never overwrite an existing unit. `daemon-reload` and action failures are reported;
a command exit alone cannot substitute for an observed running/enabled state.
The selected unit is matched by its `ExecStart` `--state=` value after expanding
systemd's `%h` home specifier, so the packaged per-user unit written by
`install.sh user` is accepted for `~/.local/state/filesync` and nothing else.
`start`/`restart` succeed only when the unit's `MainPID` is the recorded state
owner. If a daemon started outside the unit (for example by setup or the
terminal) owns the state, they return `MANUAL_DAEMON_RUNNING` with the action
`orbit stop`, then `orbit service start`, and dispatch nothing; enabling login
startup is unaffected and takes over at the next login. `orbit stop` is the
Orbit entry for the existing graceful `filesync stop`.

Native lifecycle (W17, 2026-10-07): `scripts/validation/service_boot_vm.py`
installs the packaged archive with `install.sh user` in a disposable KVM guest
and drives real logind sessions. Without lingering, login startup starts the
unit at login and logout stops it; with the owner step `loginctl enable-linger`,
the unit survives logout and starts and captures after reboot with no user
session. This is a virtual-machine result, not physical-hardware boot evidence;
see the [W17 evidence](evidence/wan-w17-20261007/summary.md).

## T03 authenticated enrollment operations

Configured `enrollment_listen` starts an isolated TLS listener using the persistent
peer identity, alongside `peer_listen` and authenticated loopback owner control.
`enrollment.addr` records the actual bound address privately for the daemon's
lifetime. Bind failure fails startup; cancellation shuts down and joins both
network servers before SQLite closes. No firewall, VPN, discovery or host policy
is changed. Advertised enrollment and peer addresses are separate reviewed
nonloopback numeric addresses; a port-zero bind needs actual advertised ports
before producing an invitation.

The terminal API's `invite` and `approval` mutations and `requests` query implement
TG1. Requests expose exact key/folder/prior digest and transcript verification
code; paged queries sort by request identity and return at most 200 items.
Revocation also accepts a v2 verifier on authenticated
`POST /api/v1/invitations/revoke`. Legacy invitation creation uses the secure
transport when the reachable runtime settings exist; unconfigured legacy
invitations are local administrative capabilities and cannot bootstrap the
pinned remote join helper.

An enrollment-v2 request from a retired identity may remain pending: request
possession grants no data access. Approval rejects that identity with
`RETIRED_MEMBER_REVIVAL` and a fresh-identity recovery action before attempting
membership construction. Rejected retries leave membership and request state
unchanged; the owner can still decline the pending request. Replacement joins
under a fresh identity through the ordinary reviewed onboarding workflow.

The compatibility CLI accepts `orbit join --invitation-file <private-file>` with
invitation JSON or an `orbit-invitation:v2:` code. The file and directory must be
owned by the current user and private (0600/0700); its contents never enter argv.
`orbit requests approve|decline --request <id> --review-file <private-file>
--operation <random-64-hex-id>` consumes an exact `ApprovalIntent` JSON previously
reviewed through the typed requests query. Its fields include requester, key pin,
folder, transcript digest, expected membership and decision. Persist/reuse the
operation ID to inspect a lost response; do not manufacture a new identity to
retry committed approval. A bare request ID cannot approve a v2 request. These
are low-level compatibility adapters; T04/T06 own the ordinary guided review,
named commands and resumable joining. Support bundles omit enrollment record
payloads and private preparation/capability files. Explicit invitation stdout
transfer remains deliberate.

## T04 terminal create/adopt/join

`orbit setup` in a terminal retains editable names/root, finite budgets,
concurrency, network addresses and startup mode through review corrections.
Existing local contents are the normal adoption path. A new review is necessary
if its root changes. No browser or stopped-state SQLite adapter is involved in
ordinary CLI onboarding: selected daemon initialization, preview and mutation use
the shared authenticated client. Closing the prompt leaves accepted work owned by
the daemon. UI rendering and full keyboard/PTY behavior remain T09/T10 work.

Scripts deliberately separate preview from approval. Use a private directory:

```sh
orbit setup --state /private/state --root /local/notes --label Laptop --name Notes \
  --preview --review-file /private/create.json --json
orbit setup --state /private/state --request-file /private/create.json --json
orbit join --state /private/receiver --root /local/notes --label Pi --name Notes \
  --invitation-file /private/invitation.json --preview \
  --review-file /private/join.json --json
orbit join --state /private/receiver --request-file /private/join.json --timeout 0 --json
orbit setup --state /private/receiver --operation OPERATION_ID --json
orbit setup --state /private/receiver --resume --request-file /private/join.json --json
```

The request file retains the reviewed inputs, random operation/attempt and
private invitation; keep it owner-only and reuse it for retry. Preview files
are created exclusively, flushed and parent-directory flushed; existing files
are preserved. `--settings-file` supplies a private JSON `terminalcontract.Settings`
plan for the same review. Interactive success records the request under private
state and prints its resume path. Noninteractive mutations require the request
file and never wait on an invisible prompt. `--timeout 0` returns current phase;
waiting has a finite deadline. `--operation` inspects accepted work without
renewing authorization. Expired attempts need a deliberately new invitation and
review; retries never generate a replacement identity/attempt.

Reviewed listener changes require restart; CLI application restarts the selected
daemon when listener/address settings differ. Peer target discovery reloads saved
endpoints at the scheduler cadence; clients authenticate each configured
certificate/pin against the current folder membership. Joining preserves the
inviter endpoint; approval preserves the requester's authenticated advertised
endpoint for the reverse pull direction. Additional-folder/offline/third-peer
rollout and endpoint changes across those scenarios remain T05 acceptance.

Manual startup leaves the daemon running. Login/unattended choices create a
child service-enable operation under the setup's durable identity, using existing
service selection, replay and recovery checks. Missing systemd or a conflicting
unit is an explicit setup block; it does not erase files or report enablement.
Unattended mode still requires the documented owner-managed lingering/prerequisites;
no privileged lingering or firewall/VPN policy is changed. Native login/logout/
boot acceptance remains T12/T13. Local Ready and startup enablement are separate
observations, and no unattended health is inferred from enablement.

## T05 additional folders and connection refresh

The local terminal API advertises `folder_sharing_v1`. Submit a private complete
`Mutation` with `kind=share`, random durable `operation_id`, and `invite` containing
exact `folder`, known `device`, current `expected_membership` digest and expiry.
The initial script adapter deliberately transfers the resulting invitation:

```sh
orbit folders share --state /private/inviter --request-file /private/share.json --json
```

Reuse that file for a lost response. Transfer its `invitation` to the known device
and perform the same reviewed `orbit join --preview`/`--request-file` workflow as
T04. Approval remains separate; no receiving directory is selected remotely.
Operation inspection omits the capability. T06 supplies ordinary named selection
and command rendering; this packet's adapter uses exact reviewed IDs.

To change the address of an already configured folder/device through live or
exclusive stopped control, keeping its existing trust anchor:

```sh
orbit devices endpoint --state /private/state --folder FOLDER_ID \
  --device DEVICE_ID --url https://NEW_NUMERIC_ADDRESS:PORT --json
```

Supplying `--certificate` explicitly remains the compatible endpoint preconfiguration
path; a configured address/certificate grants no membership. Every scheduled client
still validates the certificate pin against active folder membership. Changes take
effect at the next scheduler cycle, while listener configuration itself requires
the existing reviewed restart. Configure both required pull directions explicitly;
for B/C forwarding configure their pinned endpoints in that folder. Orbit does not
discover other machines or edit firewall/VPN policy.

Durable work observations retain membership wire codes, including retryable
`MEMBERSHIP_MISMATCH` and blocked `MEMBERSHIP_FORK`. An offline or changed network
path yields connection retries; it does not retire or rekey a device. Queue retries
are bounded; a later ordinary scheduler cycle can create fresh work. Full persistent
attention/plain-language status is T07. See [fork recovery](runbooks/membership-fork.md).
The enrollment limiter remains five requests/minute/IP with burst five. Multiple
journeys from one source must respect admission/backoff; an expired unsent signed
attempt needs explicit new review, not silent nonce/authorization renewal.

## T08 terminal conflict and recovery operations

[Terminal recovery walkthrough](runbooks/terminal-recovery.md) documents preview,
select/keep-copies/merge, history/Deleted, original-path restore and separate-copy
recovery. Named terminal mutations require a private explicit review file; omission
only previews. Legacy resolution/restore mutations also require explicit reviewed
heads and head tokens. Legacy merged files stream through a reader with a bound
content digest rather than being loaded wholly into CLI memory.

Exact-version external export refuses collisions and registered synced roots,
and publishes only after the complete verified stream and destination flush.
Review metadata outputs are private, refuse collisions and stay outside synced
roots. A relative file input is still shell-relative under T06's context rules.

Editor/diff commands parse quoted argv and execute directly. The Linux util-linux
`prlimit` adapter sets the admitted result file-size limit before tool exec; missing
tooling returns unsupported capability. This bounds the declared result file and
is not a sandbox for trusted owner tools. Tool output uses stderr in JSON mode.
Tool failure/crash retains the private result, without uploading or resolving it.
Sessions last 300 seconds from creation/explicit renewal; renew revalidates the
original review. Session inspection, cancel and freshly reviewed discard expose
recovery without silently deleting uncommitted results. Content operations have a
separate serialization gate, so streamed work does not own the status/service gate.

Uploaded bytes are staged content rather than a captured version. A returned
captured resolution can remain pending publication; operation effects distinguish
captured from applied. Restored source identity is provenance, and new ancestry
is always the reviewed current head set. Unavailable/pending/expired/corrupt bytes
refuse restore; this interface does not offer an unverified peer-recovery shortcut.

## W03 rendezvous operation

The separately runnable service is `go run ./cmd/orbit-net --listen <address>
--profile <private-selection.json> --origin <approved-https-origin>
--tls-cert <certificate.pem> --tls-key <key.pem> --service-key <private-hex-file>`.
Run from the repository (or build `go build ./cmd/orbit-net`). Profile selection
and signing-key files live in owner-only directories and are bounded private
regular files. The hex file contains the 64-byte Ed25519 private key matching the
signed profile's online `service_key`; TLS certificates are independently loaded.
No identity, invitation, account or endpoint is generated implicitly. SIGINT/SIGTERM
close/join controls and expiry work, then drain HTTP for five seconds; expired
profiles refuse new service admission. HTTP/1.1 and TLS 1.3 are required here.

The [W03 architecture](orbit-wan-architecture.md#w03-directory-and-profile-integration)
records implemented quotas and ephemeral retention. HTTP/TLS error logging is
suppressed to avoid default source-address logs; W13 must add bounded sanitized
operator counters/monitoring and record any front-proxy/infrastructure retention.
There is no production bundled profile yet. Local evidence selects explicit
`development` profiles and normal TLS with a fixture CA on private nonloopback
addresses; this is distinct from operated release endpoints. Merely saving a
profile does not enable announcements. CLI/TUI policy review and route activation
remain in their later packets.

## W04 relay operation

The same standalone `orbit-net` process now supplies the opaque WSS broker.
`--relay-bps` (20 MiB/s), `--relay-device-bps` (5 MiB/s) and
`--relay-session-bytes` (16 GiB) select positive finite ciphertext ceilings;
zero/negative CLI limits fail configuration. Each bandwidth bucket has a 32-KiB
burst, counts both directions and survives reconnects for 60 seconds of inactivity.
[Architecture](orbit-wan-architecture.md#w04-encrypted-relay-integration) records
session, socket, frame, timeout and lifetime/drain bounds. These are engineering
limits, not hosted capacity or an egress-spending commitment. Shutdown closes and
joins control/relay work before HTTP draining. No relay credentials, application
bytes, invitation capabilities, filenames or source-address timing are logged.

Daemon virtual peer/enrollment listeners use the existing isolated TLS handlers;
manual configuration and public-network opt-in remain unchanged. Explicit internal
relay routes use reviewed profile/device pins and typed overload/offline observations.
W05–W07 implement durable setup/activation, W11 service/network reconnection policy,
and W13 operated defaults/monitoring/distribution. Local W04 fixtures do not establish
native WAN/Pi capacity or production profile/signing custody.

## W06 reviewed CLI networking

`orbit setup` initializes and starts its daemon, reviews names/root/capacity,
finite limits and startup, and selects Automatic for a fresh installation.
`--connection local_only` is available before any internet announcement. Existing
manual installations and explicit legacy `--settings-file` setup retain manual
policy unless the owner chooses another mode. No hosted profile is bundled:
Automatic can retain explicit `awaiting_profile` intent, capture locally and report
`PROFILE_MISSING_OR_EXPIRED`; it does not announce or claim service readiness.
Local-only currently disables peer/enrollment listeners, outgoing sync and global
coordination. Its status says `LOCAL_DISCOVERY_NOT_IMPLEMENTED`; W08 still owns
LAN discovery, rather than claiming that it works today.

`orbit network status` reports cached desired/active policy, required restart,
service readiness and actual dated relay observations, independently of version
receipts. It never probes services. `network preview --mode self_hosted
--profile-file PRIVATE_SELECTION --review-file PRIVATE_REVIEW` reviews a signed
profile plus independently supplied authority/environment and operator privacy
text. `network apply --review-file PRIVATE_REVIEW` commits the exact idempotent
operation and restarts when active policy differs. Profile saving retains the
W03 authority/epoch rollback checks; neither policy nor profile changes rotate
identity or folder authority. TLS trust is separate from profile signing trust.
Examples using disposable development services do not qualify as hosted defaults.

`devices invite --folder NAME --out PRIVATE_FILE` deliberately transfers the
v2/v3 invitation with owner-only permissions; ordinary output and JSON omit its
capability. Optional `--preview --review-file` and `--request-file` preserve exact
invitation operation replay. `join --invitation-file` or `--invitation-stdin`
accepts a bounded private transfer, including v3 encoded codes. The deprecated
secret `--invitation` argument is rejected without echoing its value. Setup/join
scripts retain the root preview/apply/operation pattern. Fresh expired invites
require a new invitation; accepted durable attempts still use their original job.

`devices requests show --device NAME --review-file PRIVATE_REVIEW` saves the exact
request/key/folder/transcript/membership/decision. `devices approve --review-file`
applies that operation. `--decision decline` makes a decline review. A real TTY
can instead select a request, compare its verification code and explicitly approve
or decline; non-TTY approval requires the private review. Existing `invite`,
`requests` subcommands, manual endpoint workflows and `devices add` remain.
Names select only unambiguous records and never grant authority.

The daemon's default reconciliation interval is five seconds for reviewed
Automatic/self-hosted mode so new roots/routes and remote edits are discovered
without a five-minute onboarding delay. Manual retains five minutes; an explicit
`serve --sync-interval` overrides either. Both use the same bounded durable queue,
concurrency and retry engine. This is local integration evidence, not W11 timing,
Pi fairness or real-network capacity acceptance. Service errors leave local
capture and durable enrollment intent intact.

## W08 direct networking settings

Fresh reviewed Automatic and Local-only setup enables LAN advertising and describes
its signed device identity/pin and listener-address visibility. Existing policy
files retain their saved advertising choice; legacy missing policy remains Manual.
Use shared `orbit network preview --mode local_only --lan-advertising=true
--review-file /private/review.json` followed by `orbit network apply --review-file
/private/review.json`, then the reported daemon restart. The corresponding false
flag disables advertising without changing identities, folders or receipts.

Advanced private `direct-network.json` is optional, for example
`{"interfaces":["eth0"],"listen":":0","disabled":false}` (owner-only file and
private state directory). Empty interface selection means up multicast-capable
nonloopback interfaces, at most eight. `disabled:true` disables the optional peer
listener. Port collision/unavailable selected interface leaves capture/control and
reviewed relay available; an explicitly configured manual listener still fails
startup. Optional data listeners expose no owner control or enrollment handler.
Local-only filters incoming/outgoing interface/private scope and performs no service
lookup/announcement/STUN/relay. Configured isolated legacy local enrollment remains
available; fresh Local-only initial pairing needs those Advanced local settings.

LAN IPv4 multicast uses UDP 22027 / 239.255.79.66, TTL 1, bounded signed records and
no automatic approval of discovered machines. Permitted IPv4/ULA IPv6 TCP routes
use existing TLS and folder authority. IPv6-only LAN multicast is not yet supplied;
public IPv6 candidates can use the directory where an actual global address and
reachable TCP port exist. Host firewall/router changes are never automatic.
Native internet/Pi/operator capacity and roaming evidence remain later packets.

## W09 optional UDP direct settings

Automatic/self-hosted/local-only modes add an optional native HTTP3 peer socket on
an actual ephemeral unprivileged UDP port. Private `direct-network.json` retains
required `interfaces`, `listen`, `disabled` fields and adds optional `udp_listen`
(numeric bind address/port) and `udp_disabled`. Missing UDP fields select `:0` /
false; absent settings preserve automatic defaults. Existing manual mode opens no
new UDP listener. Bind failures remain route limitations while capture and other
routes continue. No firewall/router rules change. Local-only UDP binds one selected
concrete local address and filters private same-prefix sources. HTTP3 serves peer
data/membership only, with the existing pinned TLS/request authorization. Initial
enrollment and owner control remain isolated. W10 adds ICE/STUN, W11 network-switch
recovery and W12 expanded diagnostics; hosted operator profiles remain W13 work.

## W10 ICE and optional STUN service

Reviewed Automatic/self-hosted profiles supply numeric STUN addresses; enabled
UDP permits bounded ICE attempts for known pinned peer data. Manual, Local-only,
disabled UDP and initial enrollment retain their existing behavior. Missing
usable ICE candidates/check failures preserve pinned TCP or encrypted WSS; no NAT
class is inferred from a timeout. A failed pool does not continuously repeat ICE;
W11 owns reprobes and roaming policy. Runtime failure reasons are available through
`ConnectionManager.ICEFailure`; W12 owns user-facing diagnostic presentation.

`orbit-net --stun-listen IP:PORT` activates a separate optional UDP listener only
when the exact address is in the reviewed profile's STUN list. Its codec, fixed
response/rate/source/socket limits and close behavior are frozen in
[architecture](orbit-wan-architecture.md#w10-authenticated-ice-integration).
Listener failure is explicit service configuration failure; no firewall/router
changes occur. STUN observes the source address, supplies no file authorization
and never allocates a relay. W13 retains production endpoints/operator readiness.


## W11 roaming and retry defaults (partial)

Nonmanual daemons now observe interface/address/default-route changes using a
joined two-second watcher. Two stable changed samples and a five-second callback
quiet period bound flapping. Discovery and reviewed service announcements refresh;
Local-only UDP binds and listener scope rebuild. Explicit address binds can require
operator reconfiguration when that address disappears. No router/firewall changes
or service/policy/identity writes occur. Manual mode retains its explicit behavior.

Peer handshakes race TCP/QUIC with shared 32-global/two-target outgoing admission.
Relay starts after the 750-ms head start; initial race work has ten seconds plus
existing finite service cleanup. Demand-driven idle relay reprobes use 60-second
intervals; failed cycles back off 60/120/240 seconds plus 0–15-second peer jitter.
Changed candidates/networks clear negative state. Quota refusal uses a five-second
quiet refill, including one bounded responder retry of refused relay setup phases.
Submitted HTTP is never replayed by route policy. `ICE_PROBE_DEFERRED` means a
competing route ended the waiter, not proven UDP failure. Connection observations
remain separate from content receipts and service readiness.

These numeric values are finite local engineering defaults. Advanced timing
configuration, Pi/latency/loss tuning and actual mixed-transfer fairness remain
W11 work; no new throughput, seamless migration or full roaming acceptance claim
is made. See [W11 handoff](evidence/wan-w11-20261006/summary.md).


### W11 reviewed route timing and fair bandwidth reservations

The private `NetworkPolicy.timing` object adds optional decimal-string millisecond
fields. Missing/zero fields retain finite defaults. `head_start_ms` defaults to
750 (250–3,000), `cycle_ms` to 10,000 (5,000–30,000), `probe_ms` to 60,000
(10,000–300,000), `cooldown_ms` to 240,000 (probe interval–900,000), `poll_ms`
to 2,000 (500–10,000), and `quiet_ms` to 5,000 (2,000–30,000). Cycle must cover
two head starts; quiet period must cover polling. Cooldown includes the existing
0–15-second stable peer jitter after its configured cap. Service quota refill,
ICE establishment, invitation/proof expiry and authentication bounds remain fixed.

`orbit network preview --review-file PRIVATE_FILE` accepts Advanced duration flags
`--direct-head-start`, `--connection-cycle`, `--direct-probe`, `--direct-cooldown`,
`--network-poll`, `--network-quiet`. Omit `--mode` to retain current policy. Whole
nonnegative milliseconds are required; zero restores the corresponding default.
The existing preview/apply ledger binds exact timing, current policy and generation;
apply activates changes by daemon restart. Desired/active timing is exposed in
cached status. TUI network details point to this shared reviewed control flow.
Timing never supplies remote authority or expands an invitation deadline.

Replication reserves each manifest chunk's bytes before every primary/fallback
network attempt. Uncertain delivery and retries consume budget. The global/per-peer
limiter serves at most 128 waiting reservations in arrival order; cancellation
removes the waiter and wakes the next one. Overflow is retryable `NETWORK_BUSY`.
This prevents repeated tiny reservations taking every refill ahead of a large
waiting chunk; one slow peer can delay later reservations until its finite request
is admitted or canceled. A configured limit governs scheduled pull chunk payload attempts, with
a one-second initial burst; protocol/control overhead is additional. Serving a
remote peer remains subject to the existing request/stream quotas rather than
this local pull limiter. This is not
a strict interface-wide bandwidth cap. Queue aging and verified-chunk/receipt
semantics are unchanged. W11 evidence must separately record measured progress
and combined process resources on the declared host.


W11 CLI setup/join waiting retains the last known durable operation after a
read-only control status timeout and retries observation within the original wait
deadline. Each query shares that deadline and the existing RPC limits. Authentication
and non-timeout refusals remain errors. The waiter never resubmits a mutation or
replaces an operation/invitation/proof; expiration reports pending state without
claiming Ready. This handles a slow or reconnecting daemon whose background join
can outlive one control query. Exact wait-deadline and refusal regressions plus
real legacy/routed CLI journeys are recorded in W11 follow-up evidence.

## W12 diagnostics, privacy and support export

`orbit network status` is a passive read of the daemon's cached policy/profile,
service readiness and route observations. `orbit network doctor` is an explicit
bounded action; it tests service DNS/TLS, authenticated directory, pinned direct
and relay TLS, and reviewed UDP STUN targets. It does not probe on status refresh,
fan out to every peer, send peer HTTP, or infer a NAT/firewall type. `TIMEOUT`,
`UNAVAILABLE`, `NOT_TESTED`, `DISABLED_BY_POLICY`, `QUOTA_EXCEEDED` and identity
failures stay distinct and each has an actionable operator message.

Switching to Local-only or Manual closes WAN service/relay clients, invalidates
cached public candidates and drains WAN pools before the reviewed mutation
returns. The daemon remains available to report active policy/restart state.
Device identity, keys, membership, history, roots and files are retained.
Existing installations keep their explicit/manual policy until consent is
reviewed; first setup records the choice. Self-hosted mode requires the signed
profile and the same TLS/pin checks as hosted mode.

Support archives use an allowlist for configuration and stable support codes for
diagnostic/task/event failures. Path-like text is pseudonymized consistently;
invitation, ICE, relay credentials and private filenames are not exported.
Archive creation is exclusive and cannot overwrite an existing support archive.

## W13 connection service operation

The [operator runbook](orbit-net-operator.md) covers provisioning, key custody,
profile signing and rotation overlap, certificate reload, budgets, monitoring and
alerts, restart/patch/rollback/decommission, backups and incidents. Operators
install `orbit-net` from its separate archive (`make package-orbit-net`); it
runs as the unprivileged `orbit-net` user under a hardened unit that validates
the configuration (`serve --check`) before every start.

Device owners self-host with one reviewed step:
`orbit network preview --mode self_hosted --profile-file <profile> --service-roots <ca.pem>`
then `orbit network apply`. Custom trust is stored privately with the profile,
applies only to non-release profiles and is reported as
`service_trust custom:<sha256>`. A later profile review without
`--service-roots` removes it. Reviewing a newer epoch keeps existing pairings;
older epochs are refused.

No hosted default profile exists yet: WG6 needs an actual operator, origin,
certificate process, authority custody, signed release profile and monitoring
destination. Automatic mode keeps its `awaiting_profile` behavior until then.

## W14 packaged service profile and upgrades

Ordinary packages embed the hosted service's signed release profile; `orbit
version` and `release-manifest.json` report its operator, epoch, expiry and
digest, and packaging fails when it expires within 30 days. New installs select
Automatic with it in the reviewed first setup. Upgrading keeps every reviewed
choice: manual installs get a one-time offer (`orbit network automatic`,
dismiss with `--decline`), Automatic installs that were awaiting a profile adopt
it at the next daemon start, and a same-operator profile with unchanged privacy
text is adopted at start while changed text waits for `orbit network update`.
Operator switches need explicit replacement. `ORBIT_DISABLE_PACKAGED_PROFILE=1`
runs a process as if no profile were packaged (used by tests and for diagnosing
a manual-only host); it cannot add trust. Rolling back to a pre-WAN package
keeps identity, history and manual peers; see persistence for details.
