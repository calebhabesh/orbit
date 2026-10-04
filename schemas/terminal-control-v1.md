# Terminal control v1

Frozen T01 baseline, 2026-10-03. T02 now serves lifecycle/settings queries and
mutations plus operation inspection; other families remain T03–T12 work. Existing `/api/v1` routes,
base peer protocol 1, database schema 13 and causal encodings are unchanged.
Authoritative typed fields are in
[`terminalcontract`](../internal/control/terminalcontract/types.go); validation
and fingerprints are in [validate.go](../internal/control/terminalcontract/validate.go).
The owning semantic contracts remain protocol, persistence and operations.

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
