# Terminal design gate decisions

T01 design baseline, 2026-10-03. Executable experiments are in
[terminal_t01_test.go](../tests/designgates/terminal_t01_test.go), codec tests in
[terminalcontract](../internal/control/terminalcontract/contracts_test.go), and
network canonical tests in
[terminal_enrollment_test.go](../internal/protocol/terminal_enrollment_test.go).
See [T01 evidence](evidence/terminal-t01-20261003/summary.md) for actual runs.
TG1–TG5 are **design resolved**. T03 establishes TG1 transport/admission
production proof; [T05 evidence](evidence/terminal-t05-20261004/summary.md)
establishes local multi-folder/offline rollout. Other production proofs remain open. Historical G/P outcomes
and outstanding P17 owner use/explanation are unchanged.

## TG1 — Enrollment transport and identity

Choose an **isolated enrollment TLS listener**, alongside the existing mTLS
peer listener and loopback owner control. Use the same persistent device TLS
identity. This preserves the peer listener's mandatory certificate admission
instead of weakening its handshake policy for pending requesters. Setup stores
and reviews the advertised enrollment and peer HTTPS addresses separately;
ordinary startup starts both configured network listeners. No automatic relay,
discovery, firewall changes or Tailscale provisioning. A third connection/port
is the explicit operational cost of isolating public enrollment admission.

Invitations carry version 1, exact folder, enrolled inviter DeviceID, exact
certificate DER and SHA-256 SPKI pin, both endpoints, random 32-byte capability
and expiry. Deliberate owner transfer is the trust bootstrap. The certificate
is the exact trust anchor with existing server name `peer.filesync.invalid`;
normal chain/time/EKU/name verification and explicit SPKI comparison occur
**before HTTP**. No bare TLS verification bypass. Existing randomly allocated
DeviceIDs are preserved; they are not re-derived from new key hashes. The
inviter's membership must bind that DeviceID to that pin. Requester proof binds
its existing DeviceID/key; a lost identity requires existing fresh-key recovery.

Enrollment routes are `/enrollment/v2/challenge`, `/request` and `/status` on
that listener only. No owner controls or peer data handler is mounted there.
Client requests are at most 16 KiB, response also 16 KiB; body/header bounds
and established deadlines apply. Limit admission to 8 concurrent handlers,
5 requests/minute/IP (burst 5), 64/minute globally (burst 16), 128 outstanding
challenges, 128 pending requests; exceeding caps returns retryable RATE_LIMITED
without consuming capability. Expire challenges after 60 seconds. IP counts are
bounded/evicted, not an indefinitely growing map. A token grants a request,
not approval, data access or unrestricted status inspection.

Inviter stores SHA-256 of **decoded 32-byte capability**, folder, expiry,
revocation and use count (default one). Challenge binds token digest, exact
folder, persistent requester ID and random persistent attempt, nonce, prior
membership and min(invitation expiry, now+60s). Wrong scope never consumes a
use. Request holds requester certificate DER, raw Ed25519 public key, its SPKI
pin and exact fields in the frozen canonical transcript. Verify certificate
key/pin equals signing key; labels/endpoints confer no authority. Use existing
Ed25519/TLS libraries; never sign JSON text.

Canonical request bytes are domain `orbit-enrollment-request-v2` + NUL;
32-byte fields in order: folder, inviter, inviter SPKI pin, capability digest,
attempt, challenge, requester, requester SPKI pin, raw Ed25519 key, prior
membership digest; uint64 big-endian expiry; then uint32 byte-length-prefixed
UTF-8 enrollment endpoint, peer endpoint, requester endpoint (may be empty),
and label. The exact golden bytes and typed wire fields are in the protocol
module. All strings are used exactly as transferred; changed addresses require
fresh review/proof. Signature is Ed25519 over those bytes. Request ID is
SHA-256(domain `orbit-enrollment-attempt-v2` + NUL + folder + inviter + requester
+ attempt). Second folder and new attempt differ without rekeying.

Atomically validate scope, current inviter membership, proof, nonce/expiry,
revocation, use limit, pending caps and duplicate fingerprint, then consume the
challenge/capability and insert pending request. Invalid input commits nothing.
An identical signed lost-response retry returns its existing pending record
without another use; changed fields under that request ID conflict. Once a
request is recorded, inspection does not extend its approval lifetime (24h,
bounded by invitation expiry). Expired/revoked pending requests cannot be
approved. A fresh challenge before submission can reuse the attempt; once a
request exists its transcript is immutable. Expired replay guards follow the
operation ledger retention rule.

Status first posts the typed status request with only version/request set,
nonce/signature empty and expires_unix `"0"`. It receives TerminalChallengeResult
with challenge/expiry and empty prior_membership (no private artifact). It then
posts the completed proof to the same route. Status uses a one-use 60s nonce,
then signs domain `orbit-enrollment-status-v2` + NUL + request ID (32 bytes) +
nonce (32) + expiry (uint64 BE) using the recorded requesting key. No capability
or arbitrary ID retrieves approval artifacts. Status uses the same admission
caps and checks exact key/nonce/expiry transactionally. Approved result includes
existing canonical membership bytes and authenticated endpoints; it cannot
invent a new membership encoding. Approval through authenticated owner control
binds request, folder, requester ID/pin, transcript digest and prior membership.
Recheck every binding before authoring the next existing linear revision.
Verification code = first 80 bits of transcript SHA-256, lowercase hex grouped
5-5-5-5; compare deliberately between requester and owner. It supplements exact
key review; it neither replaces the signature nor grants approval. Existing
fork/retirement rules and exact revision data gates remain unchanged.

Experiment: TLS pin mismatch fails before enrollment handler sees the body;
unknown clients fail mandatory peer certificate admission; peer paths return
404 on enrollment. Every transcript field mutation invalidates the signature;
independent golden bytes match. Atomic admission model checks wrong folder,
forgery/expiry, concurrent single use and identical replay; separate folder/
attempt IDs differ. This uses httptest/in-memory records, not the actual
replication handler/database. T03 now supplies real listener/control/negative proof and requester status
authentication in [T03 evidence](evidence/terminal-t03-20261004/summary.md).
Fresh nonloopback local connections and actual child processes establish this
transport seam; physical cross-host LAN/Tailscale and native release checks are
unexecuted. T05 now records local multi-folder/offline rollout in its
[evidence](evidence/terminal-t05-20261004/summary.md). Cross-host release checks
remain T13.

## TG2 — Durable onboarding and reviewed roots

Root preview is descriptor-rooted, recursively bounded (at most 10,000 entries
or 5s per work slice), paged at 200 issues, with durable continuation owned by
the controller. Larger trees resume enumeration; no invented total or percentage.
Review expires after 300s idle (refresh through explicit new preview); the
commit revalidates root registration/dev/inode and tree generation. Generation
binds canonical path observations, type/stat and stable-read content digests,
plus registration identity. Modification or unsupported/unreadable/incomplete
regions invalidates/blocks adoption readiness. Enforce existing root overlap,
symlink/state/nested mount rules. Tokens bind exact root, settings, names and
operation family. Revalidate before registration/capture; no filesystem-wide
snapshot guarantee and changes during capture still use stable-read checks.

Report files/directories/measured bytes, unsupported/unreadable issues,
complete enumeration and capacity-known/observed time. Capacity is per relevant
filesystem (objects, state/WAL and root scratch), observed available-to-owner
space minus configured reserve and current reservations. Cross-filesystem
plans account separately, include duplicate staging/captured content and
metadata estimates; deduplicated space is credited only when verified. Capacity
is an admission observation, not a reservation against unrelated writers.
Unknown space or over-budget plans block growth, preserve input and prior files.
Structural preview types carry a summary; per-filesystem admission detail is
owned by the existing storage planner, not a guessed CLI total.

Persist reviewed -> request_prepared -> awaiting_approval -> membership_received
-> bootstrap_capture -> content_pending -> publishing -> ready. First-device
setup skips request/approval but retains registration/capture/publication phases.
Identity and finite limits initialize once under exclusive ownership before
request preparation. Persist attempt and transcript before sending; persist
request/certificate/pin/endpoints/folder/root/review/phase before acknowledging.
Never regenerate identity or attempt on screen restart. Journal includes secure
local capability material only until admission; inviter stores only verifier.
Private local state is 0700/0600, never diagnostic output. `JoinRecord` is the
safe inspection vocabulary and omits the raw capability.

Recovery resumes exact work; a stale root, expired approval or changed heads
requires explicit new review/attempt rather than silent completion. Register
root and complete bootstrap capture before destructive remote projection.
Bootstrap absence and incomplete enumeration never generate tombstones. Keep
uncaptured working bytes separate from durable saved versions. Ready requires
approved/current membership, verified available root, complete successful scan,
zero unsupported/unreadable/uncaptured items, no missing content/pending
publication/conflicts and no storage block. Blocked areas still allow unrelated
capture/serving. Service running/enabled is not readiness. Readiness describes
this observation, not perpetual global synchronization.

Experiment: serialized join records survive each planned phase with exact
identity/review/endpoints; independent readiness perturbations block Ready;
root identity/generation changes require review, and bootstrap/incomplete
absence cannot delete. No fsync/SIGKILL/root-swap proof is claimed. T04 owes
real durable migrations, root traversal/race/fault tests and two-device CLI sync.

## TG3 — Shared control adapters and operation lifecycle

Freeze the Client seam in terminalcontract; implement it once in
`internal/controlclient`, invoking existing deep control methods. Live mode
reads owner-only verified local endpoint/credentials. If a daemon owns state,
use live authenticated calls; any timeout/auth/endpoint error is returned,
never followed by direct database access. Stopped mode first acquires the
existing exclusive state lock, then invokes the same owning operation. A
network-required operation reports DAEMON_REQUIRED or explicitly starts the
selected daemon. Startup attempts coordinate with that lock and select one
identity/state; no HTTP failure proves absence. Recovery runs before scans/
networking. One finite initializer serves init/launcher/service and legacy
missing limits require honest reviewed repair, without overwriting keys/roots.

Ledger/replay/cancel follow the schema document. Streaming cancellation owns
only its read/upload resources; accepted durable mutations survive client loss.
Credentials bind owner endpoint selection, not arbitrary URLs from an invitation.
Family errors retain stable codes; bounded streams, pages and deadlines share
existing admission. Queries carry view generation and sequence; mutations carry
durable IDs and reviewed generations. Late query results cannot change a new
selection; inspect/replay recovers a lost mutation response.

Experiment: exhaustive live/lock/HTTP-success decision model refuses unsafe
fallback; persisted fingerprint/effects reject different retries and expired
identities, retaining partial effects after cancel. The existing state lock also rejects a second owner and permits acquisition
after release in a marked disposable root. Late selection/sequence responses
are rejected by a correlation model. This is not the real shared client
implementation. T02 owes singleton, credentials,
initializer and live/stopped production proof; those scoped checks are now in
[T02 evidence](evidence/terminal-t02-20261003/summary.md). T06 now supplies
command-family adapter parity, context resolution, and output escaping in
[T06 evidence](evidence/terminal-t06-20261004/summary.md); T08 owns streams/editor
lifecycle.

## TG4 — Exact reads, editor sessions and recovery

Use existing OpenVersionRead, per-stream pins and content verification, not
user-supplied chunk pins. ReadIntent names exact folder/author/counter and
bounded offset/length; zero length means remainder. Every reopen rechecks
membership, exact manifest and availability. No unavailable source substitution.
The response holds stream pins until Close/error/cancel, even beyond session
TTL; restart reclaims abandoned streams under exclusive ownership after recovery.

Session operation retains exact source IDs, reviewed head set/generation,
private admitted export/result paths and content pins. Session inactivity TTL
is 300s; explicit renewal revalidates membership and current review before
extending. Configured tool uses direct argv with exact files, never shell
interpolation of filenames. Content streams with at most existing 1 MiB chunk
buffer; whole-file merge allocation is forbidden. Disk spool admission includes
exports, editor result and upload; exceeding budget preserves prior versions.
Stage upload verifies expected size/digest and records an immutable upload ID.
Preview the staged result; mutation binds session/upload/digest/size/current
heads, then existing history authors only the reviewed parents. New heads or
working-byte changes force review/capture; no automatic refreshed resubmit.

Session close/expiry releases session-only pins, stops further commits and
preserves uncommitted editor result as a recovery candidate/attention item.
Interrupted cleanup preserves ambiguous candidates under existing quarantine
budget/policy. Only explicitly reviewed discard may remove them. Pending
journals and active read pins outrank cleanup. Completed/session metadata
pruning follows replay guard rules; a expired session ID cannot become new.

Restore source is provenance; parents are reviewed current heads. Original-path
replacement captures/protects supported current working bytes under existing
publication safety. Separate-copy recovery binds an absent/reviewed destination,
never silently overwrites it, authors a new destination version from reviewed
destination ancestry and returns source provenance plus installed effects.
Keep-copies installs recorded destination copies before original resolution;
partial outcomes/cancel retain those copies. No cross-path atomic visibility.
Expired/unavailable/corrupt payloads return explicit errors; peer recovery must
be verified before offering restore. There is no fixed deletion retention window.

Experiment: independent reference sets keep chunks through session expiry when
stream pins remain; closed streams release pins. New heads reject old editor
review; restore provenance is distinct from current parents; partial copy effects
survive serialization. An 8 MiB synthetic input streams with a 1 MiB transfer
buffer into an incremental digest. This is not production RSS, GC or editor
execution evidence. T08 now supplies scoped streamed upload/editor/restore, GC races, replay and
large-file evidence in [T08 results](evidence/terminal-t08-20261004/summary.md).
T11/T13 still owe the interactive terminal editor journey and release campaign.

## TG5 — Terminal entry, service and compatibility

Bare orbit requires interactive input/output terminals for TUI; a pipe uses
concise status/JSON with no hidden prompts or terminal escape sequences.
`filesync` preserves legacy dispatch and explicit Orbit commands stay available.
T09 pins/verifies the [subsequently selected Charm v2 stack](orbit-terminal-architecture.md#tui-stack-decision--2026-10-03);
it owns terminal modes and restore on
quit/signals/error/resize and external-tool exit. Client query cancel/resize/
editor transitions never stop the daemon. Direct editor argv runs with terminal
ownership yielded, reacquired on return, preserving selection/draft context.
Daemon stop is an explicit service operation, independent of UI quit.

Manual/login/unattended are explicit modes; running and enabled are distinct.
Unattended verification records actual host prerequisites (including linger),
not inferred from enablement. Do not change host privilege/firewall/VPN policy
silently. Use the same finite initializer/listener settings from all entry paths.
Legacy state paths, scratch, certificate/device/folder/version IDs and service
aliases remain. Additive migration is deliberate with backup and schema refusal;
binary rollback is allowed only when compatible with current schema. Never
roll back database/counters under the old identity. Uninstall preserves data.

Experiment: terminal-state decision model exercises TTY/setup/pipe, query
cancel, editor yield/return/quit and separate running/enabled/unattended flags.
This is no real terminal/PTY, process-lifetime, native service or package proof.
T09 owes actual library/PTY and live daemon lifetime; T12 owes packaged alias/
adoption/rollback/native modes; T13 owes release and actual owner observations.

T09's production shell now uses the stable pinned v2 family and shared query/
bounded tool adapters. [T09 evidence](evidence/terminal-t09-20261004/summary.md)
records real keyboard, resize/paste, plain/pipe/JSON, query cancellation and
correlation, canonical/raw tool handoff, failure and active-tool SIGTERM, exact
termios/alternate-screen restoration and continued daemon capture. The reconnect
fixture pauses only its validated marked daemon child while it retains its lock;
the client reports control unavailable and retries that owner. This scoped TG5
terminal-lifetime evidence leaves packaged entry/migration/native service modes
to T12/T13 and reviewed T08 editor-screen integration to T11.

### TG2 production implementation — T04

The reviewed job and recursive descriptor-rooted preview now have production
controls and ordinary CLI adapters. [T04 evidence](evidence/terminal-t04-20261004/summary.md)
records actual checks. Directory offsets and stream hash state are durable private
continuations; each slice bounds entries, bytes, time and issue page, and complete
commit re-enumeration rejects changed observations. Conservative capacity charges
and readiness checks are detailed in [persistence](persistence.md#t04-reviewed-onboarding-implementation).
Local capture precedes remote file import/publication. Approved membership is
installed before capture, so the author uses its admitted revision; membership
installation alone imports no file history. Signed-status polling respects the
existing enrollment limiter through persisted backoff. The packet's SIGKILL scope
is pending approval; other persisted-phase interruptions are hook/reopen evidence.
A real CLI PTY exercises correction, retained review inputs and daemon lifetime.
Native hosts, abrupt reset, TUI PTY and owner evidence remain later work.
