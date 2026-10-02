# Orbit product brief

Planning baseline: 2026-10-01. Product requirements U01–U16 live in
[scope](portfolio-scope.md#orbit-product-direction--2026-10-01).
This is the vision and user-experience contract for the revamp; the
[architecture](orbit-architecture.md) and [implementation plan](orbit-implementation-plan.md)
describe how to build it. Nothing here marks a feature implemented.

## Vision

Orbit keeps your files on your own Linux devices and makes them understandable
through a familiar file manager. Install it, create or join a workspace, choose
a local folder, and edit with ordinary applications. The daemon continues
capturing and synchronizing files after the browser closes. An optional
always-on replica forwards files between devices that are online at different times.

The default experience is one workspace called Orbit, rooted at `~/Orbit`.
A workspace corresponds to the existing shared-folder identity. A device can
join additional workspaces through advanced setup; each joined workspace is
fully replicated. A directory inside a workspace is ordinary file organization,
not a separate authorization group.

Availability follows verified copies and connectivity. Orbit displays locally
known histories and device reports, with their freshness. An unknown offline
change cannot appear until learned. A local working copy, durable captured
version, stored peer copy and applied peer copy remain distinct.

## Settled choices and planning defaults

Settled: Orbit name; one owner; user-operated storage; Linux; full local copies;
optional equal always-on replica; one default folder with an alternate location;
approved invitations; existing reachable networks; no Google login this release.

The remaining interview was replaced by the owner's request for a comprehensive
architectural handoff. The plan therefore chooses these defaults:

- Local browser interface opened by an Orbit desktop launcher.
- Full basic file operations within a workspace, plus bounded text/image previews.
- Existing conditional retention presented through History and Deleted files.
- Recovery administered from a surviving device or its OS/SSH session.
- A dark monochrome visual language based on the existing TODO preference.
- Desktop startup on login; host logout/boot persistence documented separately.

These defaults can be adjusted without reopening settled scope. None adds a
fixed trash period, universal availability, atomic directory moves, a backup
service, or recovery after losing every accessible copy/credential.

## First-device journey

1. Install a package and launch Orbit. Reuse an existing healthy daemon or
   start exactly one daemon for the selected state.
2. Show **Create your Orbit** and **Join an existing Orbit**. No account screen.
3. Create: accept/change a suggested device name; accept/change `~/Orbit`;
   show finite storage settings and actual service/startup status.
4. Validate the root and state independently. Existing content gets a count,
   size/unsupported-items preview and explicit adoption review. A missing or
   inaccessible root gets a useful correction, rather than deletion inference.
5. Generate workspace/device identities internally. Persist a restartable setup
   operation; never ask the owner to type a folder ID.
6. Show capture/initial-sync progress, then open Files. Pair another device
   remains a visible action; one-device use is useful before pairing.

A create failure preserves preexisting files and identifies the unfinished
step. Opening the app again resumes or safely retries the same setup operation.
Service-enabled, running, root-accessible and capturing are separate observations.

## Join-device journey

1. On an enrolled device, choose **Add device**, select the workspace and create
   an expiring invitation. Display the reachable endpoint and connection prerequisites.
2. On the new device, choose Join, paste the invitation, name the device and
   confirm its local folder and reachable peer address where required.
3. Verify the inviting identity from the invitation and establish a bounded
   enrollment request proving possession of the joining device key.
4. The inviting UI shows the joining name, key identity and workspace scope.
   The owner approves or declines that exact request.
5. Review any existing contents on the joining root. Empty bootstrap paths
   do not mean deletions; differing existing contents remain reviewable histories.
6. Show **Approved**, **Updating devices**, **Downloading files**, and **Ready
   here** separately. Older membership on an offline peer may need a queued update.

An invitation is a request capability, not permission to read files.
Approval of an additional workspace is separate. Expired/revoked/replayed
requests do not create a second enrollment or silently renew authorization.
Headless CLI uses the same operations, with secrets supplied through stdin or
private files rather than ordinary command arguments.

## Everyday file manager

Files is the landing view. Navigation uses a workspace selector, breadcrumbs,
back/forward and a searchable list, with optional grid view for images.
The list shows human names, type, size and relevant state. Device names,
conflicts and pending content are understandable without knowing vectors.
Large directories/search results are paginated by the backend.

The baseline actions are Open local folder, create folder, upload, download,
rename, move within the current workspace, delete, history and restore. Open
local folder connects the browser workflow to the ordinary Linux file manager;
it is unavailable with a clear reason in a headless session. Multi-selection/bulk actions
use a preview when required. Destination collisions receive explicit choices;
an overwrite cannot be inferred from a drop gesture. Folder moves are recoverable
batches and may report partial completion. Existing rename semantics remain
delete plus create, with path-based history.

Preview initially covers bounded plain text and common raster images. Other
types offer download. Users edit through normal Linux applications. Active
HTML/SVG content is not executed in the app's authenticated origin.

Details is a side panel: available content, versions, device copies and operation
progress. A conflict shows the local working copy and competing captured
versions without pretending that one is the global winner.

## Information architecture

| Destination | Primary content |
| --- | --- |
| Files | Current workspace, breadcrumbs, search, file actions, details |
| Needs attention | Conflicts, root problems, failed operations, storage/integrity issues |
| Devices | Human device names, last contact, copy status, add/retire flow |
| Deleted files | Known deletions, retained candidates and conditional restore |
| Settings | Local roots, storage/retention, startup, advanced workspaces, diagnostics |

History is contextual to a path. Ordinary files lead the experience; diagnostics
remain available in Settings and relevant attention items. Labels and IDs can
be copied from advanced details without becoming primary navigation.

## Status and copy

| Observation | User-facing wording / behavior |
| --- | --- |
| Working bytes not yet captured | Saving locally; no durable-save claim |
| Captured durable version | Saved on this device |
| Transfer or metadata pending | Syncing / Waiting for files |
| Current direct receipt | Stored on Laptop; report contact/freshness separately |
| Applied peer report | Updated on Laptop, with observation time |
| Offline peer | Laptop offline; show last contact, not global sync success |
| Membership behind | Device update required / Updating device membership |
| Multiple heads | Needs review; show all reviewed choices |
| Root inaccessible/replaced | Folder unavailable; syncing paused for this folder |
| Required history bytes expired | Version unavailable; restore disabled with reason |
| Space admission blocked | Storage full for Orbit; show safe next actions |

Do not collapse these into a single green check. Other paths/workspaces can
progress while one item is blocked. Progress percentages appear only with a
defined measured denominator; discovery/metadata phases can be indeterminate.

## Deletion, conflicts and recovery

Deletion propagates as existing tombstones. Deleted files is an index into known
history, not a second physical trash folder. Restore creates a new reviewed
version using bytes actually available or recoverable from an authorized peer.
Existing retention uses acquisition age/version count; deletion starts no
new guaranteed timer. UI explains availability without inventing an expiry date.

Needs attention offers choose a version, keep copies, or manual merge through
the existing reviewed operations. Stale review requires showing the changed
versions again. There is no timestamp winner or automatic semantic merge.

A lost device can be replaced from a surviving administration endpoint.
Retirement preserves its known history and follows the conservative survivor
procedure. Reinstallation and rolled-back metadata require a fresh identity/key
and reviewed reenrollment. Losing all usable administration access has no
account-based remote recovery path; independently preserved plaintext files
can be imported into a new workspace if accessible.

## Visual and interaction direction

Use graphite surfaces, off-white text, restrained borders, clear hierarchy and
consistent vector icons. A sans-serif UI font carries names/actions; monospace
is limited to technical details. Avoid decorative dashboard cards consuming
the file list. Motion is subtle and respects reduced-motion settings.

The desktop layout has a stable sidebar, top search/actions and a wide content
area. A details drawer preserves browsing context. Smaller browser windows
collapse navigation and keep essential actions accessible.

Define semantic CSS tokens for surfaces, text, borders, focus, spacing, type
and status. Preserve readable contrast, visible keyboard focus, native control
semantics, non-drag alternatives and dialogs that return focus. Status includes
text/icons as well as color. Errors retain entered values and offer a concrete
next action. Polling must preserve selection, scroll and focused controls.

Initial visual tokens are a baseline to implement and measure, not a substitute
for rendered contrast/focus checks:

| Token / region | Starting value |
| --- | --- |
| Canvas / surface / raised surface | `#101010` / `#181818` / `#222222` |
| Primary / secondary text | `#F2F2F2` / `#B6B6B6` |
| Border / selected surface | `#393939` / `#303030` |
| Interactive control boundary | Start near `#707070` where a boundary is required; measure contrast |
| Primary action | Off-white background with near-black text |
| Focus | Visible 2px off-white outline with separation from the control |
| Spacing | 4, 8, 12, 16, 24, 32px |
| Type | System sans; 14px list/body, 12px supplementary labels, 20px view title |
| Desktop list / sidebar / details | Approximately 44px rows / 232px / 360px |
| Motion | Subtle 120–180ms control/drawer transitions; reduced-motion alternative |

Semantic warning/error/success colors may supplement text/icons. Refine sizes
for browser zoom and smaller windows. Use system fonts or bundled licensed
assets, preserving an entirely offline everyday interface.

## Completion experience

The owner can install, create a workspace, pair a second Linux device, browse
and manipulate nested files, work offline, reconnect, resolve a conflict and
restore retained content without using the terminal for ordinary desktop tasks.
The equivalent headless workflow remains documented and executable.

Release includes the existing three-host distributed scenarios, the new
enrollment/mutation failure scenarios, a keyboard walkthrough and actual owner
use. Recorded timings describe observations; no cloud-scale or universal
performance claim is introduced.
