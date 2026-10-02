# Security, resource limits and operator interface

Status: implemented baseline. The limits below are configuration/admission bounds; release evidence describes the workloads actually exercised.

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
Malformed endpoints or unapproved certificate pins refuse startup. Restart
after changing endpoints. Startup and the configured reconciliation interval
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
cannot authenticate a data read. Current schema is 12; newer schemas are
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
