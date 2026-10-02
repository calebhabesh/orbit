# Orbit packets O05–O10: devices and file management

Read [plan](../orbit-implementation-plan.md), [product](../orbit-product.md),
[architecture](../orbit-architecture.md), and each packet's owning specifications.
Commands/test groups below are planned deliverables, not evidence of execution.

## O05 — Enrollment, endpoints and durable membership propagation

**Depends on:** O01, O02. **Requirements:** U03, U05, U06, U09, U16.
**Invariants:** I09, I13, I15, I16, I20, I23, I24.
**Owner:** lead. **Owning specs:** protocol, operations, persistence.

Implement the G02 contract behind shared control/replication operations:
invitation create/revoke, join request, owner approve/decline, membership
rollout and progress. Bound invitations/attempts, require key possession,
pin the inviting identity, bind approval to exact workspace/key/prior revision,
and persist outcomes before reporting approval. Secrets never enter logs,
support exports, long-lived URLs or ordinary CLI arguments.

Only explicit invitation-scoped enrollment is exposed to an unknown key.
All existing data operations retain key/folder/revision authorization.
Propagate recorded owner-approved sequential updates through the gated control
exchange, including an offline active peer's catch-up. Detect concurrent
revision conflicts and expose the tested recovery procedure. Retired devices
cannot use that route to rejoin or fetch excluded content.

Install/validate peer endpoints and certificate material for the actual pull
directions. Exercise two-peer and A→hub→B forwarding rather than assuming one
saved address establishes both directions. Use the existing reachable LAN/
private network; return connection/pin/version/membership errors separately.

Existing contents on the joining root pass bootstrap preview/capture before
remote publication; absence is never deletion. Additional workspaces require
separate authorization. Retiring a lost device remains the conservative
survivor procedure and never silently retires other offline devices.

**Acceptance:** approved invitation yields authenticated folder-limited sync;
expired/revoked/replayed/wrong-key requests fail; unapproved request has no
inventory/chunk access; repeated approval is idempotent. Offline peers catch
up or show an explicit actionable configuration block. Restart during every
approval/rollout step preserves consent and versions. Concurrent administration,
stale rollback, retired-key attempts and mixed versions have recorded outcomes.

**Planned checks:**

```sh
make build
go test -count=1 -v ./internal/replication ./internal/control ./internal/repository ./tests/integration -run 'TestOrbitEnrollment|TestOrbitMembership|TestOrbitEndpoints'
make test-model
make test-race
```

## O06 — Pairing and device-management UI

**Depends on:** O04, O05. **Requirements:** U03, U05, U06, U09, U14, U16.
**Invariants:** I19, I23, I24, I27.
**Owner:** Flash lane; lead reviews authentication/status behavior.
**Owning specs:** product, operations/control contracts.

Build Add device and Join flows, invitation copy/paste, expiry/revocation,
pending joining request, explicit approve/decline, root preview and first-sync
progress. Device cards/list rows use names, last contact, reachable endpoint
diagnostics and qualified copy/membership states. Key pins/IDs remain in
approval/details where they inform a trust decision.

Explain existing network prerequisites before failing a join. Preserve entered
values, support cancellation/retry and distinguish approved from updating,
downloading and ready. Configure a headless Pi/VPS through equivalent CLI
operations and local control over SSH; update the runbook with an executed
marked-fixture flow.

Provide rename-alias, endpoint edit, workspace membership and retire preview.
Retirement explains known-history/uncaptured-work limits and cannot promise
remote disk erasure. A stalled membership update is visible rather than a
generic perpetual spinner.

**Acceptance:** two fresh devices pair through the real UI without certificate
file copying or manual membership JSON. An offline third device catches up
through the documented flow; restart and rejected request states remain usable.
Headless commands perform the same operation. Each device's status has correct
freshness and no false global-success claim.

**Planned checks:**

```sh
node scripts/orbit_ui_test.mjs --scenario pairing
node scripts/orbit_ui_test.mjs --scenario devices
```

Build frontend and fixture binaries first. Save the join/approval/connection
error/offline-device/retirement screenshots and filesystem/protocol assertions.

## O07 — Hierarchical browse, search and authenticated content

**Depends on:** O02; G03 read-lease outcome from O01.
**Requirements:** U07, U10, U11.
**Invariants:** I06, I09, I10, I13, I18, I19, I25.
**Owner:** lead. **Owning specs:** persistence, operations, protocol/control fixtures.

Add repository-backed immediate-child directory queries, bounded workspace
filename/path search, sort/pagination and file details. Include known pending
heads, working-copy state, conflicts and safe navigable implicit ancestor rows.
Do not author directory versions from a browse request. Results are local
knowledge, not a live authoritative census of the ring.

Use indexed queries with defined cursor/snapshot generation and byte/item
limits. Reject stale cursors clearly or return a documented refreshed view.
Avoid loading every workspace path into the browser for filtering. Add Deleted
files/history read models without pretending metadata retention means byte
availability.

Implement authenticated pinned content reads for exact versions, bounded
plain-text/raster previews and large-file download streaming. Baseline preview
limits: 1 MiB of text and 10 MiB encoded raster content, plus an explicit decoded
pixel budget; builders validate practical decoder/browser limits. Other files
download. Active formats remain escaped text/attachments rather than executing
under the app session.

Verify content and preserve GC leases through response/cancellation. Missing
or corrupt requested bytes produce a diagnosed failure; no fallback substitution.
Handle safe filenames in headers and safe MIME/cache/CSP policy. HTTP reads
cannot obtain another workspace's chunks or arbitrary absolute paths.

**Acceptance:** nested/empty/implicit directories, unusual names, tombstones,
pending content and structural conflicts are represented correctly. Invalid
paths/unauthorized folders and active-content injection are rejected. Search/
first-page queries stay bounded on a generated 10,000-file workspace. A large
download uses bounded memory, survives supported transfer behavior, and retains
pins against concurrent GC; disconnect releases them.

**Planned checks:**

```sh
make build
go test -count=1 -v ./internal/repository ./internal/control ./tests/integration -run 'TestOrbitBrowse|TestOrbitSearch|TestOrbitContent|TestOrbitReadLease'
```

Record query timings, response sizes and memory for the declared fixture.
Timing observations do not establish a universal latency guarantee.

## O08 — File browser, previews and details UI

**Depends on:** O04, O07. **Requirements:** U07, U10, U11, U15.
**Invariants:** I19, I25, I27.
**Owner:** Flash lane; lead reviews real-state integration.
**Owning specs:** product/control contracts.

Implement workspace selection, breadcrumbs, navigation history, backend search,
sort/paging, selection and list/grid presentation. Put history, exact preview
version, copy state and operations in a context-preserving side panel.
Display directory/file icons and human names; show technical details on demand.

Provide Open local folder through O03's verified desktop helper, with a clear
unavailable state on headless hosts.

Use stable keys and a coherent request-state model. Ignore obsolete responses
after navigation/search changes; preserve focus/selection/scroll through polling.
Distinguish loading, empty, stale, blocked and unavailable. Downloads must use
the O07 large-file mechanism instead of assembling arbitrary files in memory.

Provide keyboard navigation/actions, visible focus, full-name access for
truncated filenames, appropriate selection semantics and non-drag alternatives.
Respect reduced motion and browser zoom. Avoid adding a navigation tree that
recursively downloads the entire workspace.

**Acceptance:** owner browses deep directories, searches and sorts 10,000 files,
previews supported content, downloads exact bytes and inspects device copies.
Pending/conflicting/unavailable rows do not appear as normal complete files.
Fast navigation/polling cannot replace a newer view with an old response.
Keyboard and narrow-window scenarios pass against real fixtures.

**Planned checks:**

```sh
node scripts/orbit_ui_test.mjs --scenario browse
node scripts/orbit_ui_test.mjs --scenario previews
```

## O09 — Recoverable import, create, rename, move and delete

**Depends on:** O01, O02, O07. **Requirements:** U10, S06–S14.
**Invariants:** I01, I03–I13, I16, I17, I20, I26.
**Owner:** lead. **Owning specs:** persistence, protocol, operations.

Implement G03 file operations through workspace/repository/control, including
upload/import streaming, create directory, within-workspace move/rename and
delete. Every operation uses verified root descriptors, a reviewed plan,
destination collision policy, generation token, durable ID and checked
resource admission. Frontend handlers never directly unlink/rename user paths.

Capture/protect source or overwritten working bytes before destructive
publication. Move destinations are durably created before source deletion;
revalidate the source/subtree and preserve both safe copies when ambiguous.
Directory operations persist enumerated progress, invalidate on new children
and expose partial completion. Cross-workspace moves remain outside this packet.

Resume journals after process stops at staging, capture, destination creation,
source tombstone/application and operation completion. Cancellation stops
future safe steps without pretending committed changes were rolled back.
Concurrent local editor changes, remote arrivals and GC cannot remove a
protected captured version or implicitly resolve conflicts.

Delete obeys existing bulk-delete/root safeguards. Failed/inaccessible
enumeration is not permission to recurse. History remains path-based and
retention unchanged; no physical trash directory or deletion-time timer.
Complete/retry results have explicit idempotency expiry behavior.

**Acceptance:** real CLI/control actions produce correct ordinary files and
replicated histories. Replay creates no duplicate logical operation; changed
parameters/stale preview fail. Fault hooks across every new durable phase,
ENOSPC, destination collision, edit/delete conflict, source editor race, new
child and unavailable root preserve protected bytes and yield recoverable
operation state. Scan-after-action does not fabricate feedback edits.

**Planned checks:**

```sh
make build
go test -count=1 -v ./internal/workspace ./internal/repository ./internal/control ./tests/integration -run 'TestOrbitImport|TestOrbitMkdir|TestOrbitMove|TestOrbitDelete|TestOrbitMutation'
make test-model
make test-faults
make test-race
```

## O10 — File actions, history, Deleted files and Needs attention

**Depends on:** O08, O09; device details reuse O06 when present.
**Requirements:** U07, U10, U11, U15.
**Invariants:** I03, I16, I19, I26, I27.
**Owner:** Flash lane; lead reviews destructive/reviewed workflows.
**Owning specs:** product, operations/control contracts.

Expose upload/create folder/download/rename/move/delete in toolbar/context
actions and keyboard-accessible dialogs. Drag/drop is a convenience with a
button alternative. Destination overwrite and nonempty/bulk deletion show
concrete affected items, scope and reviewed generation. A failed/stale mutation
retains context, refreshes changed facts and asks for a new review instead of
auto-resubmitting a different operation.

Render actual operation progress and partial/canceled results. UI may update
an item as pending; completed success requires the engine result. Closing the
browser cannot cancel durable work implicitly.

History/Deleted files expose conditional restore and unavailable bytes clearly.
Needs attention groups actionable conflicts, integrity, root, storage and
operation errors. Reuse choose/keep-copies/manual-merge and stale-token handling.
Show reviewed versions with device names/times as context, never as winner rules.

**Acceptance:** all common actions work through real shared operations and
match CLI outcomes. Mid-action restart/refresh and stale review remain safe.
Delete vs edit retains both causal choices; restore creates a new version.
Expired content cannot restore an empty/substitute file. Keyboard-only file
actions, conflict resolution and dialogs are usable.

**Planned checks:**

```sh
node scripts/orbit_ui_test.mjs --scenario file-actions
node scripts/orbit_ui_test.mjs --scenario history
node scripts/orbit_ui_test.mjs --scenario attention
```
