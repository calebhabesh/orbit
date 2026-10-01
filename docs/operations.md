# Security, resource limits and operator interface

Status: implemented baseline. The limits below are configuration/admission bounds; release evidence describes the workloads actually exercised.

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
