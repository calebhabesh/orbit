# Terminal control v1

Frozen T01 baseline, 2026-10-03. T02 now serves lifecycle/settings queries and
mutations plus operation inspection; other families remain T03–T12 work. Existing `/api/v1` routes,
base peer protocol 1, database schema 13 and causal encodings are unchanged.
Authoritative typed fields are in
[`terminalcontract`](../internal/control/terminalcontract/types.go); validation
and fingerprints are in [validate.go](../internal/control/terminalcontract/validate.go).
The owning semantic contracts remain protocol, persistence and operations.

E12 refinement (2026-10-09): successful query/mutation results include optional
`join_request_count`, a canonical decimal string summarizing unexpired local
approval requests across both enrollment formats. It is independent of the
current page and folder. The TUI retains the latest observed count across tabs,
refreshes it within its single query lane, and leaves form drafts untouched.

## Version, codec and transport

The terminal control namespace is `/control/terminal/v1` on authenticated
loopback owner control: `POST /query`, `POST /mutate`, `POST /read` and
`POST /upload`. This separates strict new objects from historical APIs rather
than changing old clients' response shapes. Query and mutation return `Result`;
read returns verified raw bytes; upload streams staged bytes and returns a
result with the durable upload operation and `UploadResult` ID, size,
digest and session expiry. Staging does not create a captured version. Upload takes session ID, expected size and SHA-256 in authenticated
headers, never filesystem paths or an arbitrary object-hash capability.

Clients first query `capabilities`. Require version `"1"` and
`terminal_control_v1`; unavailable capability means `UNSUPPORTED_CAPABILITY`,
without trying an older unsafe mutation. Base-only peers retain base sync;
network enrollment requires `enrollment_v2`. There is no implicit v1 invitation
upgrade or identity rotation. Unknown versions return `INCOMPATIBLE_VERSION`.
Endpoint implementation must bound the body before calling the codec.

All uint64 sizes, counts, revisions, counters, offsets, bandwidth and duration
values are canonical decimal JSON **strings**, including zero. No signs,
leading zeroes, fractions, exponents, overflow or JSON numeric alternative.
IDs/digests/tokens are 64 lowercase hex characters; operation identities cannot
be zero. Times use RFC3339Nano UTC; network challenge expiry uses Unix seconds
as a decimal string. Optional fields are omitted; collections are `[]`, not
`null`. Objects reject unknown/duplicate keys and trailing values via the
existing protocol strict codec. Metadata is at most 1 MiB; pages 1–200
(default 50, limit zero means default). Enrollment bodies remain 16 KiB.
Strings are bounded by their owning path/name/endpoint policy. Source paths
use existing exact UTF-8 validation, never Unicode/case normalization.

## Intent and result families

`Mutation` is a closed tagged union: `version`, `operation_id`, `kind`, and
exactly one named typed intent. `Validate` supplies structural checking;
controllers additionally check credentials, current membership, review expiry,
root identity and capacity under the actual commit boundary. A valid codec
object alone grants no authority. Sets of heads are sorted by author then
numeric counter, with no duplicates and at most 64. Review comprises opaque
`token`, `generation` and `expires_at`; all are server-issued and context-bound.

| Mutation kind | Intent | Reviewed authority / result |
| --- | --- | --- |
| setup / adopt | SetupIntent | Root preview, names, settings and service/network selection; operation + readiness |
| invite / share | InviteIntent | Exact folder/current membership; share additionally binds known device; explicit invitation result |
| join | JoinIntent | Transferred pinned invitation, persistent attempt, local root review and finite settings; operation + private join record |
| approval | ApprovalIntent | Request/key/folder/transcript/prior membership; pending or sequential approved rollout |
| folder | FolderIntent | Exact folder generation; pause/resume or relocation with old and new root |
| session | SessionIntent | Exact source versions and current heads; private editor session |
| content | ContentIntent | Select, keep_copies, merge, restore or separate_copy with exact sources, current heads and destination reviews |
| settings | SettingsIntent | Current settings generation and complete replacement finite settings |
| service | ServiceIntent | Reviewed start/stop/restart/enable/disable and mode, actual service observations |
| cancel | CancelIntent | Exact durable target operation; cancellation request with retained effects |

All new mutations, including staging/upload and lease/session lifecycle writes,
obey the operation ledger. Upload's operation ID is supplied explicitly with
its session and content fingerprint, and is not inferred from a screen request.
Session close/release is expressed as cancel targeting the session's owning
operation; completed reads release their transient stream resources internally.
Ordinary status/context queries create no durable mutation identity.

`Query` freezes `capabilities`, `context`, `root_preview`, `operation`, `status`,
`attention`, `devices`, `folders`, `requests`, `history`, `deleted`,
`content_review`, `session`, `settings`, `service` and `doctor`. Folder/path,
name/cwd, exact source/destination, ID and bounded cursor/limit are typed fields.
`context` resolves an explicit stable folder first, otherwise an exact unique
name, otherwise descriptor-validated cwd registration. Conflicting selectors
or multiple candidates return `AMBIGUOUS_CONTEXT` and candidate names/roots;
mutations consume the resulting generation and explicit folder/path, not cwd
re-inference. Empty path is permitted only for folder/query scope, not a
replicated root deletion. An outside cwd requires explicit selection.

`Result` shares the observation vocabulary across CLI/TUI: contextual names,
operation and effects, root review, join readiness, editor session, settings,
service observations, named items, enrollment requests, exact version summaries,
content review, copy observations, persistent attention, bounded cursor and typed
error. `ContentReview` lists exact current heads, source availability, affected
replacement paths, working capture observation and reviewed destination.
`VersionSummary.display_time` is informational, never a winner rule.

A copy observation binds device/folder/**exact version**, saved/stored/applied,
direct versus forwarded information, observation time and last contact,
online and availability. No observation promises perpetual storage or current
unchanged working bytes. A receipt from a hub cannot claim final-device receipt.
Attention IDs persist across launches; refresh does not acknowledge or discard
an unresolved issue. Loading is presentation state, not a completed operation.

## Operation identity, replay and cancellation

Before any side effect, persist the random 32-byte operation identity, owning
state identity, normalized fingerprint, reviewed inputs and initial phase.
Fingerprint = SHA-256(`orbit-terminal-mutation-v1` + NUL + Go typed JSON encoding
of the validated Mutation with `operation_id` set to `""`). Upload uses the separate `orbit-terminal-upload-v1` + NUL domain and typed
UploadIntent with its operation identity cleared, binding session, size and
digest before streaming. Struct field order
is fixed by these types; no arbitrary client JSON text is hashed. Arrays with
set semantics must use the canonical order. Secrets may enter a digest, but
never a log/status result. Schema additions require a new version if they change
fingerprinting. Operation ID alone does not authenticate inspection.

The ledger transaction precedes effects. Matching ID/fingerprint returns the
same operation and committed effects, even when a response was lost. Different
inputs return `IDEMPOTENCY_CONFLICT`. Preserve full pending/running/blocked/
partial records until reconciled. Keep completed result payloads for at least
24h; afterwards retain an identity/fingerprint tombstone under metadata budget
and return `EXPIRED_REPLAY`. Do not prune that guard then treat the old ID as
new. Metadata pressure pauses new ledger admission; it never erases causal
history or unresolved recovery. A fresh reviewed operation is required after
expiry. Journal recovery precedes ledger pruning. No indefinite throughput
promise is made with finite metadata and retained replay guards.

States: pending, running, blocked, completed, partial, canceled, failed. Phases
are family-specific and persisted; partial records list per-path committed
effects and errors. Client deadline/disconnect/quit cancels only waiting/stream
ownership. An accepted mutation keeps running. Explicit cancel records its own
operation, marks the target cancel requested and stops at a safe recovery
boundary. It never undoes installed files, membership approval or captured
versions. Outcome is partial when effects remain. Lost mutation responses are
resolved by inspect/replay with the same ID, never automatic fresh IDs.

A TUI response has client selection-generation, query sequence and operation
identity. Apply a query response only to its originating selection/sequence.
A mutation response is retained under its operation, even if its former screen
has closed; it cannot replace the current folder selection. Resuming loads the
ledger and reviews, not a re-created screen draft.

## Errors and script rendering

`Error` has stable code, local human message, retryable boolean, practical
next action and retry_after_seconds (zero if absent). Retryable permits another
bounded attempt against the same operation, never fresh approval or automatic
review replacement. `STALE_VIEW`, `STALE_ROOT`, `MEMBERSHIP_FORK`,
`EXPIRED_REPLAY`, `IDEMPOTENCY_CONFLICT`, `DESTINATION_COLLISION`,
`INVITATION_EXPIRED`, `INVITATION_REVOKED`, `IDENTITY_MISMATCH`,
`INVALID_SIGNATURE`, `CONTENT_EXPIRED`, `DISK_BUDGET`, `METADATA_BUDGET`,
`INCOMPATIBLE_VERSION`, `UNSUPPORTED_CAPABILITY` and validation/auth failures
are not automatically retryable. `OFFLINE`, `CONTENT_PENDING`,
`CONTENT_UNAVAILABLE`, `ROOT_UNAVAILABLE`, `RATE_LIMITED`, `DAEMON_REQUIRED`,
`IO_ERROR`, `UNSTABLE_FILE` are retryable after their stated correction/backoff.
`MEMBERSHIP_MISMATCH` and `RETRY_EXHAUSTED` need explicit maintenance/retry.
Existing stable engine codes remain; unknown error codes use exit 1.

| Exit | Meaning |
| --- | --- |
| 0 | Query or requested operation completed successfully, including empty results |
| 1 | Execution/integrity failure or unknown error |
| 2 | Invalid input/path or ambiguous context |
| 3 | Authentication, identity/signature or invitation authorization failure |
| 4 | Review required, stale state, partial effects, fork, replay expiry/conflict, expired content |
| 5 | Pending approval/work, unreachable/offline/root unavailable or retry later |
| 6 | Storage/metadata admission blocked |
| 7 | Incompatible version or unsupported capability |
| 130 | Client interrupted/canceled; committed work may continue |

`--json` writes exactly one Result JSON value plus newline to stdout on success
or error, and exits with the category. Raw exact-version export writes only
binary bytes to its explicit destination/stdout; it cannot combine with JSON.
Human mode writes requested data to stdout, errors/progress/prompts to stderr.
Non-TTY mutation missing explicit reviews returns code 2, never waits for input.
No escape sequences/progress in JSON, no implicit invitation output, and no
secret in argv/log/support output. Deliberate invite transfer output may contain
the secret; join reads it from a prompt, stdin or owner-only file. Human text
escapes terminal controls. Stable codes, not translated text, drive scripts.

## Presentation fixtures

T09 adds no wire fields. Explicit `folders`/`devices` `Query.limit` requests use
bounded live SQL keyset pages; `Result.cursor` binds the kind and all folder/ID/
name selectors. Refresh without a cursor starts a new live page. Concurrent
label changes are not review snapshots or mutation authorization. Legacy
no-limit lists retain compatibility behavior. Aggregate status and attention
cursors belong to their own query families; a folder/device cursor cannot be
used for them. The terminal shell requests 20 rows per collection.

[Fixtures](fixtures/terminal-v1/success.json) include success, empty, loading,
awaiting approval, offline, stale, storage blocked, root unavailable, partial,
fork and conflict. They contain synthetic paths/identities and no capability.
They support view development and contract decoding only. Tests that show a
fixture do not establish authorization, daemon lifetime, file capture or syncing.


## T02 production subset

The authenticated namespace now serves capabilities, settings, service and
operation queries, settings/service mutations, and the owner-only internal
`POST /service/claim` and `POST /service/complete` handoffs used after releasing stopped-state ownership.
Other frozen families explicitly return `UNSUPPORTED_CAPABILITY`. The capability
`lifecycle_settings_v1` identifies this subset; `terminal_control_v1` identifies
the envelope/codec version, not evidence that every family is implemented.
Read/upload and explicit cancellation remain their owning later packets.

Service claim/completion are internal authenticated dispatch/reconciliation calls, not a new
mutation identity or public command intent. It references the already admitted
service operation. Lost stop/restart replies are inspected by that original ID.

E01 (2026-10-08) adds optional fields. `Service.owner` (`service`, `terminal`,
`manual`; omitted when stopped) and `Service.unit_state` (systemd ActiveState)
are observations. `Result.host` (`HostStartup`: `class`, `suggested`,
`systemd`, `lingering`, `linger_command`, `note`) accompanies the `settings`
query; it is advisory and never enables anything. Service `start` is now
client-dispatched like `stop`/`restart`, so a client can hand a daemon started
outside the unit over to it; claim/complete are unchanged.
Runtime retention remains per-folder; nonzero terminal retention changes are
rejected rather than silently claiming that storage policy changed.

## T03 production subset

`enrollment_v2` identifies the isolated transport and terminal `invite`/
`approval` mutations, `requests` query and their durable operation inspection.
It does not advertise T04 `join` jobs or T05 `share`/rollout completion.
Invitation result transfer is deliberate; stored/inspected invitation results
omit capabilities. Exact mutation replay can reconstruct the invitation using
the owning protocol's private-key HMAC derivation. Review/fingerprint changes
never refresh and execute implicitly. Request queries are deterministic pages
of at most 200 scoped observations, including exact requester/key, transcript
code/digest and prior membership. Invitation revocation is also available on
the existing authenticated compatibility route with the v2 verifier digest.

### T04 served setup subset

`reviewed_setup_v1` adds production `root_preview`, `setup`, `adopt`, `join` and
safe setup/join `operation` results. `Query.root_plan` is optional `SetupIntent`
without an operative `preview`; when present its root must equal `Query.path`.
`Query.name` chooses `setup`, `adopt` or `join` (default `setup`). The root review
binds that family and all plan fields. `Query.cursor` is a private controller
continuation identity, bound to the root; pages report measured totals and up to
200 issues. `Result.preview.complete=false` carries a continuation, not a guessed
total. Finite/network/startup settings and names must remain identical in mutation.

Safe `Result.join` describes both create/adopt and join jobs; create/adopt leaves
inviter/request/attempt fields empty. Operation phase is authoritative. A blocked
job retains committed effects and reviewed inputs. Reusing its original mutation
resumes the same work unless authorization/review requires an explicitly new
attempt. Completion uses `phase=ready` and verified readiness observations.
Legacy create responses retain their `completed` phase spelling through adapters.
T06 retains full script exit/command-family parity work.

## T05 production sharing subset

`folder_sharing_v1` adds the `share` mutation through the same live/stopped client.
Its `InviteIntent.device` is an exact known DeviceID (the inviter looks up and
retains its approved pin), with independent folder/current-membership review.
Result/replay uses the invitation/operation schema above; operation inspection
keeps capability empty. A receiver uses T04 reviewed join with a fresh scoped
attempt, existing persistent key, selected root and separate owner approval.
Endpoint refresh remains authenticated compatible local control, atomically
replacing the address for a folder/device; an omitted certificate reuses that
pair's saved trust anchor. Full named context/rendering remains T06.

## T08 production content capability — 2026-10-04

`reviewed_content_v1` advertises the implemented reviewed content/session families
and raw streaming routes. Clients require it before using those routes. The
additive `conflicts` query returns bounded `Attention` pages of content and
structural conflicts. History and Deleted use existing generation-bound SQL
pages; Deleted also returns an actual prior-file candidate in `versions`.

Read/Upload metadata uses `X-Orbit-Intent`, strict JSON capped at 8 KiB; request
and response content remains raw bytes. Reads reauthorize each exact version,
validate ranges and retain independent stream pins through close/error/cancel.
Upload verifies expected size and digest into one private immutable session
spool. Incomplete upload IDs cannot be reused or committed; inspect their
session recovery record. Streaming body size is bounded by declared bytes and
session admission, rather than the 1 MiB metadata limit.

Content review binds stable root registration and membership, exact current
heads, named working inode/stat/hash, optional historical source and absent
copy destination ancestry. Scan counters do not invalidate unchanged reviews.
Uncaptured working changes require capture and a new review. Causal creation and
its operation effects commit in one SQLite transaction. Replay resumes existing
publication or recorded copy steps, never another causal identity. Partial
copy effects remain observable without cross-path atomic visibility.

Session `create`/`renew` retain their T01 fields. `discard` is an additive action
under this capability, with an exact session ID, its original sources and a
fresh current content review. It closes commits before unlinking only known
private candidate paths; unexpected auxiliary files prevent complete cleanup.
Cancellation targets the session's owning ID and preserves results. Expiry
blocks commits and releases session pins on inspection or GC. Raw read pins
have an independent lifetime. Editor/export/upload spools remain budgeted until
explicit reviewed discard; replay guards are retained.

## T10 onboarding management capability — 2026-10-04

`onboarding_management_v1` advertises these additive query names. The live client
checks that capability before invoking them; existing query results omit the new
folder-management field. Existing nested join contracts stay unchanged.

- `setups`: unfiltered unfinished setup/adopt/join discovery, including pending,
  running, blocked and partial operations. Limit/cursor bound SQL decoding and
  response size. Cursor is the last operation ID; other context selectors are
  refused. Items contain operation identity, phase and root, with no invitation,
  capability, private mutation or success/readiness inference.
- `folder_management`: requires an exact folder. Optional `folder_management`
  result contains root, local pause, local membership digest/revision and its
  protocol-bounded named active members. Existing readiness, attention and copy
  observations retain the T07 semantics and response limits; reads do not scan,
  clean up, retire or authorize sharing.

Setup operation results can include their own possession-bound enrollment
observation in the existing `requests` collection. The verification code is
obtained from the canonical transcript or authenticated status and retained
across restart; it is not a hash abbreviation of the operation/request ID. This
uses the existing frozen fields and strict codecs. Invitation capabilities remain
explicit-transfer-only. CLI and TUI submit identical setup/adopt/join mutations;
the shared client persists an owner-only exact retry intent and listener-restart
record before submission. Folder pause/resume/relocation and retirement preview
reuse the existing authenticated compatibility operations and recovery ownership.

## Additive WAN contracts — W01

[Network-v1](network-v1.md#additive-terminal-contracts-and-migration) freezes
policy/query/preview/apply, observation and private setup types in
`internal/control/terminalcontract/network.go`. The capability names
`network_control_v1` and `enrollment_v3` are reserved; W01 does not advertise them
or expose production handlers. Existing terminal JSON, signed v2/manual enrollment
and v1 membership/version contracts are unchanged.

## W06 network controls and CLI activation

`network_control_v1` is now advertised by production terminal controls.
Queries `network_status` and `network_preview` use the same authenticated terminal
query route. Preview carries `network_plan` with `policy`, optional signed
`profile`, independently reviewed `authority` and `environment`; the returned
review binds both current state and the exact intent. Mutation `kind=network`
carries the same intent plus its `review` and the existing operation identity.
Acceptance, replay, expiry and changed-input behavior use the existing ledger.
Policy generation is assigned by preview; LAN advertising is explicitly refused
until implemented. `NetworkPolicy.awaiting_profile` is optional and may be true
only for Automatic with no profile. This is explicit incomplete desired intent,
not a trusted route. Missing this field retains the strict W01 policy validation.

W13 adds optional `network_plan.service_roots`: at most 16 KiB of PEM containing
one to four currently valid CA (or self-signed service) certificates. It is
accepted only together with a `profile` whose `environment` is `self_hosted` or
`development`; release profiles always verify against system roots. Preview and
the review generation bind the exact bytes; applying a profile replaces or removes
the stored trust. `network.service_trust` reports `system`, `custom:<sha256>` or
`invalid`. Trust never weakens hostname verification or per-peer pins.

Setup/join intents can add optional `network` policy, included in root review and
fingerprint. Legacy intents omit it. Results can add `network` with desired and
active policy, restart requirement, cached readiness/code, reviewed operator/privacy
text and bounded dated peer route observations. Service readiness and route
observations never substitute for stored/applied receipts. No capability bytes,
relay attachment credentials, candidate lease or directory lookup appears in these
results. Cached queries do not initiate probes; W12 still owns network doctor.


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

## W12 qualified diagnostics

Production terminal controls advertise additive `network_diagnostics_v1`.
`network_status` remains a passive query; `network_doctor` is an explicit query
with an optional validated device ID and the same authenticated control envelope.
`NetworkStatus` adds `generated_at`, `profile_expires`, `profile_state`, `action`
and bounded `probes` while retaining desired/active policy, restart/readiness,
operator/privacy and peer observations. `NetworkObservation` adds `freshness`,
`action`, `lan_candidates`, `public_candidates`, `expired_candidates` and
`udp_code`; it does not merge route state with capture, stored/applied copies or
membership. `ProbeResult` records only `kind`, typed `code` and `observed_at`.

Doctor probe kinds are `service_dns_tcp`, `service_tls`, `directory`,
`direct_tls`, `relay_inner_tls` and `udp_stun`. Stable codes distinguish
`VERIFIED`, `UNAVAILABLE`, `NOT_TESTED`, `DISABLED_BY_POLICY`, `TIMEOUT`,
`CANCELLED`, `QUOTA_EXCEEDED`, `IDENTITY_MISMATCH`, `TLS_IDENTITY_FAILED` and
`PEER_OFFLINE`; unrecognized transport text is sanitized to `UNAVAILABLE`.
The result never contains invitation, ICE, relay credential, candidate lease or
private filename data. Passive status and support export perform no probes.

## W14 packaged profile, migration and invitation codes

Production terminal controls advertise additive `packaged_profile_v1`.
`NetworkStatus` adds optional `builtin` (`digest`, `operator`, `privacy`, `epoch`,
`expires`, `expired`) describing the signed release profile compiled into the
daemon, `profile_update` (`"available"` when that profile is a newer epoch of the
selected release authority whose operator or privacy text changed) and
`automatic_offer` (a manual install that has not reviewed or declined Automatic).
A setup, join or network intent whose `policy.profile` equals `builtin.digest`
selects that profile; the daemon installs it on apply. `NetworkIntent` adds
`replace_operator` (confirms a reviewed switch to another authority or
environment; without it the preview fails `PROFILE_OPERATOR_CHANGE`) and
`decline_automatic_offer`. All fields are omitted when unused, so earlier
clients decode unchanged results; earlier daemons reject the new intent fields
under strict decoding, and clients check the capability first.

Invitation codes keep their prefixes. A routed `orbit-invitation:v3:` code whose
route digest equals the inviter's packaged profile may omit `profile`; the
receiver restores it from its own packaged profile (the route digest still binds
the exact bytes) or fails `PROFILE_NOT_PACKAGED`. Invitation files keep the full
form. A code prefix or invitation `version` above 3 fails
`UNSUPPORTED_INVITATION_VERSION`. Routed joins against a different profile fail
`PROFILE_OPERATOR_MISMATCH` or `PROFILE_EPOCH_MISMATCH` with an action naming the
device to change; a manual device given a routed code fails
`NETWORK_REVIEW_REQUIRED`.


## E08 read-only Files queries

`files` takes a locally authorized `folder`, directory `path` (empty for root),
optional folder-wide substring `name` search (mutually exclusive with `path`),
`limit` (1–200) and an opaque `cursor`. `files` results contain path/name,
directory flag, byte size, observed modification time, local state and block
reason. Generations invalidate cursors; clients restart pagination after
`STALE_VIEW`. `file_details` requires folder/path and adds current versions,
last local scan time and version-specific peer observations with report times.
No query reads arbitrary filesystem paths or changes content. The CLI and TUI
use these same queries. EG2 owns the labels and their limitations.

## E06 short-code pairing

`InviteIntent.short_code` requests an ephemeral code for a routed invitation.
The result's optional `pairing` contains code, state, expiry and a safe error.
This is deliberate private transfer, like the returned full invitation;
operation/status replay does not persist or reveal the short-code password.
A `pairing` query with the invitation operation ID returns state without code.
A `pairing` mutation takes a fresh operation ID and `{code, profile}`: submission
consents to one exchange through the exact displayed operator/profile. It
returns the decrypted ordinary v3 invitation; no membership or root is changed.
The eight-entry ephemeral result cache prevents retrying a claim in one daemon
lifetime. Restart requires a new code. A service without the pairing endpoint
returns `UNSUPPORTED_CAPABILITY`; Add device keeps the long invitation/file.
Older receivers use the explicitly available long invitation/file.

`short_pairing_v1` advertises short-code mutations. Invite adapters omit
`short_code` when that capability is absent. Unsupported services and local-only
invitations retain the explicit long-code/private-file transfer. The pairing
control request has a 45-second operation bound and a 50-second HTTP write
deadline; ordinary requests retain their existing deadline.

## 2.2.0 invitation names and join reconnect

`Invitation` gains optional `inviter_name` and `folder_name` (UTF-8, at most
128 bytes each): display text chosen on the inviting device, never identity.
Decoders before 2.2.0 reject invitations that carry them. A join whose
enrollment connection fails for a retryable reason keeps its phase, state
`running`, no error, and one effect `reconnecting:<CAUSE>` (a service code,
`TIMEOUT` or `UNREACHABLE`); an identity mismatch still blocks.


## E13 participation management — 2026-10-09

Additive capability: `participation_management_v1`. `FolderManagement` adds
`name`, `local_device` (64-hex), optional `removed_by` (display name), or
`removal_reporter` when the original removal actor is unknown.
`Result.retirement` is a review with `device_name`, numeric `received_changes`,
64-hex `membership_digest`/`snapshot_digest`, `warning` and `disclaimer`.
`Result.removal` has `device_id`, `state` (`completed`, `pending`, `needs_review`),
`operation_id`, numeric `received_changes`, `pending_devices` and `message`.
The integer counts match the compatibility result types; they are not uint64
revision/counter fields. There is no implicit mutation from a progress poll.

Shared authenticated compatibility operations (bounded by the control server):

- `POST /api/v1/orbits/leave`: `LeaveOrbitRequest` (`folder` native ID byte
  array, `expected_root`); success `{"state":"left"}`. Replay succeeds for an already left
  root; a changed registered root refuses the old request.
- `POST /api/v1/peers/remove`: `RemoveDeviceRequest` (`folder`, `device_id`,
  `operation_id`, `confirm_name`, `membership_digest`, `snapshot_digest`). ID and
  digest fields use the existing compatibility native byte arrays; operation ID
  is 64-hex. Returns `RemovalResult`. Exact request replay preserves intent.
- `POST /api/v1/peers/remove/resume`: `ResumeRemovalRequest` (`folder`,
  `operation_id`) resumes the saved owner-confirmed request, returning the same
  result vocabulary. The folder must match the saved scope.

CLI and TUI call these through `controlclient`; stopped-state execution invokes
the same controller methods. There is no erase/reset operation in these routes.
