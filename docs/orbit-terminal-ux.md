# Orbit terminal UX and implementation handoff

Network amendment, 2026-10-05: [WAN UX](orbit-wan-ux.md) supersedes the LAN/Tailscale
prerequisite and address-entry portions of this handoff for new Automatic-mode
work. Its relay/default-service journeys are planned, not implemented. Existing
manual/private-network flows and all non-network terminal guarantees remain.

## Owner amendment 2026-10-08 (names and folders)

Approved by the owner during the 2.2.0 trial. Create and join forms label
their fields **Device name** ("How your other devices see this one"), **Orbit
name** ("What this Orbit is called in the app") and **Folder** ("Where its
files live on this device; created if missing"); the focused field shows its
hint. Folder defaults to `~/Orbit`, shows `~` as typed, and follows the Orbit
name (`~/<name>`) until the owner edits it. A folder whose parents are missing
is created with them on confirmation (the nearest existing ancestor is pinned
at review). The create form opens with "An Orbit is a folder that stays in
sync across your devices"; the join form names the inviter and the Orbit. The
review reads "Orbit <name> will sync <folder> on <device>." Invitation and
approval screens name the Orbit ("<device> wants to join Demo (~/Demo here)").
*Shared names (same day):* an Orbit's name and each device's name are shared
by every member ([protocol](protocol.md#shared-orbit-and-device-names--2026-10-08)).
The join form no longer asks for the Orbit name when the invitation carries
one; it shows "Join <name>" and asks for the device name and folder. `R` renames
the highlighted Orbit (Orbits view, or the Orbit's screen) or device (Devices
view); `orbit folders rename <name> <new>` and `orbit devices rename <name> <new>`
do the same. Other devices show a rename after their next sync.

## Owner amendment 2026-10-08 (trial polish)

Approved by the owner during the 2.1.0 trial; implemented in 2.2.0. These
supersede conflicting text below.

- **Orbits.** The terminal calls synced folders *Orbits*: view `2` is
  `Orbits` and the Overview list is grouped under `Orbits`. CLI commands,
  JSON and the other docs keep "folder". On wide terminals (100 columns or
  more) the right pane follows the cursor: the highlighted Orbit's root, any
  attention, and its top-level entries with their state here. Enter still
  opens the Orbit's management screen; `5` (or Tab into Files) opens the
  highlighted Orbit in Files. ←/→ keep switching views.
- **Inviting.** Choosing an Orbit under `a` creates the code straight away;
  sharing with a device that is already a member keeps its review. The
  invitation page shows one status line that follows the invited device:
  waiting for the code, code used, `Awaiting your approval: <device> wants to
  join. Verification code …`, then approved or declined. Enter on a waiting
  request opens its review. Expiry is local 12-hour time with the time left.
- **Joining.** Invitations carry the inviting device's name and the folder
  name (display text only; trust is still the key pin). The join form
  proposes that folder name and `~/<name>` as the root; typed values are
  kept. Local root accepts `~` and `~/…`. The waiting screen names the
  inviting device ("Awaiting approval: waiting for CalebPC…"). When the
  inviting device cannot be reached, the join keeps its step and says
  "Can't reach the inviting device yet (<cause>)", retrying every 15–25
  seconds, instead of stopping as blocked. Builds before 2.2.0 cannot read
  these invitations; update every device together.

## Owner amendment 2026-10-08

After the first 2.0.0 trial the owner approved these changes; they are planned in
the [onboarding plan](orbit-onboarding-implementation-plan.md) and not yet implemented.
They supersede the conflicting text below.

- **Files view.** The terminal gains a read-only Files view of synced folders with
  per-file sync state, details, and actions that open the file in the owner's own
  tools, its history or its conflict review. It is the default screen when setup
  is complete and nothing needs attention; Overview remains the default otherwise.
  Renaming, moving, deleting and editing still happen in the owner's tools. This
  replaces the decision that ordinary browsing is outside the terminal.
  *As implemented (E08, 2026-10-08):* Files is view `5` (tab `5 Files`). With
  several folders it first lists them; with one it opens that folder. Columns
  are name, size, modified and the state here: Saved here, Arriving,
  Downloading, Content missing, Conflict or Blocked ([EG2](implementation/onboarding-gates.md#eg2--files-view-truthfulness-e08)).
  Enter or → opens a directory, ← / Backspace / Esc go up; Enter on a file shows
  its versions and each other device's last report with its age. `o` opens with
  the desktop default (`xdg-open`; without a desktop session it says to use `e`),
  `e` opens the working copy in `$EDITOR` with the interface suspended, `h`
  history and restore, `c` conflict review, `D` deleted files, `y` copies the
  local path (OSC 52), `/` searches known paths in the folder, `]`/`[` next and
  previous page. The status panel shows attention count, devices and connection.
  The first screen is Files when a folder exists, no setup is unfinished and no
  attention item is open; a screen already in use is never switched. `orbit
  files [path] [--search text] [--json]` lists the same entries and states.
- **Keys.** ↑/↓ (and `j`/`k`) move rows and form fields; Tab/Shift+Tab move
  between fields or panes; ←/→ change selectors and move through the Files tree;
  Enter selects, advances and confirms; Esc goes back. Fixed choices are selectors
  with defaults, never typed words. Number keys switch views.
  *As implemented (E03, 2026-10-08):* in lists ↑/↓ or `j`/`k` move rows; in
  forms ↑/↓, Tab and Shift+Tab move between fields (`j`/`k` type text in text
  fields and move fields on selectors). On the main screen Tab/Shift+Tab move
  between views, never into search; `/` searches. ←/→ change a selector shown
  as `‹ value ›` (typing is ignored there) and otherwise move between views.
  Enter selects; in a form it advances to the next field and confirms on the
  last. Esc goes back, `?` opens help, `q` quits outside text fields and Ctrl+C
  quits anywhere. `1`–`5` pick Overview, Folders, Attention, Devices and Files.
  A hidden invitation field shows how many characters arrived. Each paste
  replaces its content, Ctrl+U clears it, Enter on an empty field submits nothing, and after a failed attempt the next typed character
  starts over, so an unbracketed re-paste replaces rather than appends. A failed
  preview focuses the field with the problem. Footers list only keys the screen
  handles.
- **Defaults.** Create and join default to Automatic connection after one privacy
  review; startup defaults to login on desktops and unattended on headless hosts
  (with the lingering step shown, never run implicitly); the service uses an
  ephemeral control port; the device name is the hostname.
- **Invitations.** A short pairing code is the default in service modes
  ([WAN UX](orbit-wan-ux.md#short-pairing-code-amendment-2026-10-08)); long
  invitations are shown unboxed, copied whole and transferable as a file.
  *As implemented (E05, 2026-10-08):* the invitation screen never draws the code
  inside a panel; it shows its character count and three actions. `c` sends it
  to the clipboard with OSC 52 (bounded at 16 KiB; "Copy not supported here"
  where `TERM` rules OSC 52 out; terminals do not acknowledge OSC 52, so success
  is stated as sent). `v` pauses the interface and prints the code on the
  ordinary screen as one unbroken line that the terminal wraps itself, so a
  selection copies one line; Enter returns and clears the screen and its
  scrollback. `s` saves it to a private file, by default
  `<state>/invitation-<folder>-<digest>.json` (0600 in the 0700 state folder),
  and shows the `scp` and `orbit join --invitation-file` commands. The join
  prompt says it takes a code or a file path, the fallback for SSH sessions.
- **Attention.** Items clear themselves once their cause is gone, Enter opens the
  matching review, and every suggested action works while the daemon runs.

Date: 2026-10-03. The owner agreed to recommendations Q1–Q14 and selected the
terminal redesign. This document is the agreed UX contract. The comprehensive
[implementation plan](orbit-terminal-implementation-plan.md),
[architecture](orbit-terminal-architecture.md) and
[terminal tracker](implementation/terminal-status.md) define the implementation
handoff. Commands and screens below are targets, not claims about today's
executable. Runtime implementation and acceptance checks are unexecuted in
this planning session.

Read this document for the terminal direction. Existing engine guarantees
remain in [scope](portfolio-scope.md), [protocol](protocol.md),
[persistence](persistence.md), [operations](operations.md), and
[verification](verification.md). Earlier browser work and evidence remain
historical; the [Orbit tracker](implementation/orbit-status.md) does not establish
completion of these terminal journeys. Outstanding P17 owner use and unaided
explanation remain outstanding.

## Product decisions

Orbit manages synchronization among one owner's trusted Linux devices. Each
joined folder has a complete local copy. Ordinary file browsing and editing
use the owner's shell, Vim, and other applications. (Amended 2026-10-08: a
read-only Files view is added; see [the amendment](#owner-amendment-2026-10-08).)

| Interview | Agreed behavior |
| --- | --- |
| Q1 | Focus on setup, devices, synced folders, progress, conflicts, history, and restore |
| Q2 | A small TUI is the interactive front door; independent CLI commands use the same controls |
| Q3 | Suggest `~/Orbit`; syncing an existing folder is an ordinary option |
| Q4 | Support reachable LANs directly; document Tailscale as the initial cross-network path |
| Q5 | Select folders explicitly when sharing with a device; future folders need a separate sharing decision |
| Q6 | Create invitation, paste on new device, approve on inviter; persist interrupted progress |
| Q7 | Suggest an empty joining root; review existing-folder contents before adopting them |
| Q8 | Offer startup at login for laptops and unattended operation for storage hosts |
| Q9 | Keep attention items visible on the next TUI launch and in `orbit status` |
| Q10 | Support light Vim navigation alongside arrows and Tab; show contextual actions |
| Q11 | Review editable setup defaults together; advanced retention/performance tuning is secondary |
| Q12 | Infer file-command context from the current registered directory; resolve ambiguity explicitly |
| Q13 | Integrate configured external diff/editor tools with reviewed conflict operations |
| Q14 | Preview restore to the original path; offer recovery to a separate copy |

The background daemon continues syncing after the terminal interface exits.
A future GUI can become another client of the same control operations. The
browser file manager stops being the target for new ordinary-use features.
Existing state, identities, roots, compatibility commands, and recovery
guarantees must survive the interface transition.

The approved scope reflects terminal-first ordinary use in U07, U08, U10,
U15, U16 and S18. U06 continues to require an existing
reachable network. Tailscale is a separate setup dependency, not an Orbit
account or an Orbit discovery/relay service. An optional Pi/NAS/VPS remains a
trusted equal replica that stores and forwards files.

## Concepts and ownership

Use **device** and **synced folder** in normal controls. A synced folder is the
existing shared folder/workspace replication group, with its own membership
and one local root on each participant. A subdirectory is ordinary file
organization. The existing [glossary](../CONTEXT.md) still owns the domain model.

Adding a device records and approves its identity for selected folder
participation. It does not grant access to every current or future folder.
Additional sharing reuses its persistent key and connection information, with
separate folder approval and local-root review. Names support selection but
cannot authorize access. Duplicate names require disambiguation; normal
commands never require cryptographic IDs.

The daemon owns identity, SQLite, immutable content, capture, publication,
replication, membership, and scheduling. TUI and CLI invoke authenticated local
control operations. Engine modules own invariants, reviewed generations,
idempotency, resource bounds, and recovery. An adapter must work while the
daemon runs; a stopped-state adapter requires exclusive ownership and invokes
the same owning operations.

## First-device setup

1. `orbit` opens **Create your Orbit** or **Join an existing Orbit** when setup
   is incomplete. An existing setup resumes from its actual persisted phase.
2. Create suggests the device's host name and `~/Orbit`. Both are editable;
   **Use an existing folder** is a normal choice.
3. Preview existing contents, their measured size, unsupported items, usable
   capacity, and the effect of adoption. Explain that supported local contents
   will become shared. Paginate large results and identify an incomplete scan.
4. Review device name, folder name/path, connection mode, startup choice, and
   a finite storage budget together. Display the capacity assumptions and
   reserve; put retention and performance controls under Advanced.
5. Confirm the review. Generate identities internally and persist setup work.
   Begin capture and show measured progress or an indeterminate phase when
   the total is unknown.
6. Show what is saved locally, what remains uncaptured, and any attention
   items. **Add device** is visible; one-device use is useful immediately.

Root errors retain entered values and identify a correction. A failed preview
or capture does not produce a Ready claim. Existing files survive failed or
interrupted setup. Service running, startup configured, root accessible, and
successful capture are separate observations.

Unattended setup explains and verifies the host requirements, including user
service lingering where needed. An enabled login service alone cannot be
reported as boot/logout persistence. Privileged host settings remain explicit
owner steps under the existing operations contract.

## Add-device and additional-folder journeys

1. On an enrolled device, **Add device** selects the initial folder to share.
   Generate a short-lived invitation with folder scope, inviting identity/key
   binding, reachable enrollment address, and request capability.
2. Choose LAN or an existing Tailscale network. Inspect available connection
   information, explain any missing prerequisite, and avoid advertising a
   loopback address to another machine. Orbit keeps local control separate
   from network enrollment and replication.
   Exchange and persist the connection information needed for each supported
   pull direction; test reconnection through the chosen network path.
3. On the new device, `orbit join` accepts the invitation through an interactive
   prompt, stdin, or a private file. Suggest a device name and empty local root;
   preview/review an existing root if selected. Authenticate the inviting
   identity before sending secrets or accepting enrollment artifacts.
4. Submit a request proving the new device's key possession. The inviter shows
   the exact request, device name, selected folder, and a verification code
   bound to the identities for comparison. The owner approves or declines.
5. Persist the request identity, endpoints, root, and phase. Approval can occur
   later; closing either interface leaves the recorded workflow resumable. The
   inviting daemon must be reachable for request submission and completion.
6. Show Waiting for approval, Approved, Updating devices, Scanning local files,
   Downloading files, and the resulting local readiness separately. Offline
   peers may have a pending membership update.

Additional folders are explicit sharing actions using the enrolled device
identity. Each receiving device chooses/reviews its local root before capture
and reconciliation. The backend retains per-folder membership approval even
when the interface groups several intended sharing actions together. The
first usable slice may share the initial folder, then offer additional folders.

Expired invitations have an explicit retry path. Revocation, replay, wrong
folder scope, failed identity checks, and membership forks return actionable
errors. Interface refresh or retry cannot silently renew authorization.

## Everyday overview and keyboard behavior

The overview puts attention first, followed by named folders and a compact
device summary. Keep file-copy observations distinct from daemon health.
Example layout; figures and states are illustrative:

```text
Orbit                         Daemon: running   Startup: at login

Needs attention (1)
  Documents/notes.md          Conflict: Laptop and Pi versions

Synced folders
  Documents   ~/Documents    Saved here; transferring 12 known files
  Photos      ~/Orbit/Photos Updated on Pi at 14:20

Devices
  Pi                         Connected; last contact 14:20
  Laptop                     Offline; last contact 2 hours ago

[a] Add device  [f] Folders  [n] Attention  [/] Search  [?] Help  [q] Quit
```

Names and paths are human labels. A peer observation includes its freshness;
offline or unknown state never becomes a global synchronized checkmark.
Details distinguish current working bytes, durable local capture, reported
stored copies, and reported applied copies. A blocked path can coexist with
successful work on other paths.

Support `j/k` and arrows, `/` search, Enter to inspect/select, Esc to return,
Tab between controls, and `?` help (the 2026-10-08 amendment extends arrows and
Enter to forms and replaces typed choices with selectors). Text fields use ordinary entry. Show focus
and status with text, preserve selection during refresh, and adapt to a narrow
terminal without truncating essential recovery actions. Plain output remains
usable without color or terminal-specific glyphs.

Quitting the interface leaves the daemon and committed background work
running. An explicit operation-cancel action describes its effect and any
already committed results. Stopping the daemon is a separate service action.
Attention persists in daemon state, not only in the current screen.

## Proposed command vocabulary

These are target commands; inspect current help before implementing aliases
and parser changes. Preserve compatible legacy commands deliberately.

| Command | Normal purpose |
| --- | --- |
| `orbit` | Interactive setup or overview |
| `orbit status` | Concise folder/copy progress, attention, daemon and startup status |
| `orbit setup` | Create or resume local setup |
| `orbit join` | Prompt for an invitation and guide enrollment |
| `orbit devices` | List named devices and last contact |
| `orbit devices add` | Guided invitation creation for a selected folder |
| `orbit devices requests` | Review/approve/decline exact pending requests |
| `orbit folders` | List named folders and local roots |
| `orbit folders add` | Preview and register a folder |
| `orbit folders share <name> --device <name>` | Explicit additional-folder sharing |
| `orbit folders pause/resume <name>` | Control local synchronization for that folder |
| `orbit folders relocate <name>` | Change the local path through existing reviewed relocation |
| `orbit conflicts` | List attention items requiring version/structure review |
| `orbit conflicts resolve <path>` | Guided exact-version review and resolution |
| `orbit history <path>` | List known versions with content availability |
| `orbit deleted` | Find known deleted paths and available restore candidates |
| `orbit restore <path>` | Select an available version and preview restoring it |
| `orbit storage` | Usage, finite budgets, retention and maintenance |
| `orbit service` | Running/startup state and explicit service operations |
| `orbit doctor` | Actionable diagnostics and safe next commands |
| `orbit help` | Grouped help, examples, and advanced operations |

Inside a registered root, relative file paths use the current directory's
synced folder. Outside it, select a named folder or use an interactive picker.
Ambiguous roots/names require disambiguation. Mutations display the resolved
folder and affected paths before approval. Explicit configuration options
allow scripting; a generic force flag cannot bypass reviewed-state checks.

Provide useful human output, shell completion, and stable `--json` output for
automation. Noninteractive mutations require explicit inputs and necessary
reviewed tokens; they never wait on an invisible prompt. If bare `orbit` has
no interactive terminal, return concise status rather than starting a TUI.
Errors carry a stable code, retryability, and a practical next action. Help
shows examples using names and paths, with secrets supplied separately.

## Conflicts, history, and restore

Inspect competing versions by device, informational timestamp, size, and
availability. Time is context, never a conflict winner. Offer inspect/diff,
keep separate copies, select one reviewed version, and manual merge.

Export exact reviewed versions to bounded local review storage and invoke
configured diff/editor tools with direct arguments. Preserve the review token
through the editor session. Preview the edited result before committing; a
changed head set requires another review. Stream larger files instead of
loading the whole merge into memory. Unavailable bytes produce a reason and
available recovery actions, not substitution of another version.

History describes locally known versions and actual content availability.
Deleted files is an index of known history. Restore creates a new reviewed
version at the original path after showing what will be replaced. **Recover
a separate copy** is an explicit alternative. Protect current working bytes
through existing capture/publication checks before replacement.

Deletion starts no guaranteed retention window. Receipt of historical bytes
does not promise perpetual availability. Show locally available, pending,
expired, unavailable, and verified peer recovery observations accurately.
An automatic peer fetch for restore requires implemented evidence before the
interface offers it.

## Hardening work found during the interview

These are dated source findings from the current workspace, not executed
failure or exploit evidence. Reproduce them through the production interfaces
before fixing and update owning contracts where necessary.

T00 subsequently recorded [current dispositions and executable reproductions](evidence/terminal-t00-20261003/summary.md).
The observed readiness failure is
partial scan diagnostics discarded while reporting Ready; fatal scan-error
suppression and whole-file merge allocation remain source-only. The dated table
below retains its original source context. T02–T08 own production repairs.

| Finding / source | Required outcome |
| --- | --- |
| Launcher/service starts control only; invitation defaults to control address; peer listener lacks enrollment routes (`internal/launcher/launcher.go:204`, `packaging/systemd/orbit.service:8`, `internal/control/orbit_control.go:826`, `internal/replication/server.go:99`) | Ordinary startup prepares intended peer/enrollment connectivity; local owner control remains loopback-authenticated |
| Invitation lacks inviting key pin; join skips TLS verification; token consumption does not check recorded folder against requested folder (`internal/control/orbit_control.go:835,920,1414`, `internal/repository/product_records.go:190`) | Authenticate the inviter, prove requester possession, bind capability to exact scope, and retain explicit owner approval |
| Enrollment request ID derives only from joining key and is a global primary key (`internal/control/orbit_control.go:935`, `internal/repository/repository.go:588`) | Additional-folder enrollment and safe retries work with the same persistent device identity |
| Join persistence omits request/endpoint; completion ignores initial scan failure (`internal/control/orbit_control.go:1450,1582`) | Restart resumes the exact request; actual capture/membership/content observations determine readiness |
| Setup/join and several conflict/restore CLI paths enter exclusive stopped-state adapters (`cmd/orbit/main.go:3406,672,1546,1625,1730`) | Normal terminal controls work against a running daemon through the shared authenticated operations |
| Launcher initialization bypasses finite storage initialization (`internal/launcher/launcher.go`, `internal/app/app.go:53,116`, `internal/config/storage.go:26`) | Every initialization path creates validated finite budgets and the setup review exposes them |
| Root preview counts immediate entries without full size/unsupported/race review (`internal/control/orbit_control.go:280,295`) | Bounded measured adoption review binds execution to the reviewed root and detects changes |
| `orbit status` is service/setup-only; detail summaries lose direct-observation information (`cmd/orbit/main.go:3325,3345`, `internal/repository/browse.go:394`) | Status includes attention and qualified saved/stored/applied/contact observations |
| CLI merge reads entire file; reviewed editor sessions and a terminal deleted-files command are missing (`cmd/orbit/main.go:1535,3213`) | Bounded exact-version review, stale-result protection, and human-facing history/deleted recovery work end to end |

## Implementation sequence and acceptance

The [T00–T13 packet plan](orbit-terminal-implementation-plan.md) owns the
complete implementation sequence, dependencies, roles and acceptance criteria.
The following summarizes its blocks; historical O00–O14 evidence remains scoped
to the earlier implementation.

1. **Contracts and failing reproductions:** record the terminal scope change,
   freeze additive controls, reproduce invitation/wiring/scope/resume/limits
   gaps, and define the owning protocol/persistence/operations changes.
2. **Ordinary CLI onboarding:** make live-daemon setup/join/invite/approval,
   finite budgets, existing-root review, and service modes work on two real
   installations with names, paths, and one invitation transfer.
3. **Everyday CLI:** qualified status/attention, folder sharing/context,
   reviewed conflicts with external tools, history/deleted/restore, diagnostics,
   human output and machine output over the same controls.
4. **Small TUI:** implement the setup and overview plus focused forms/detail
   screens over those working controls. Keep daemon lifetime independent.
5. **Packaging and owner validation:** switch the ordinary Orbit entry to the
   terminal experience, retain compatibility, validate LAN/Tailscale scenarios,
   and collect actual owner use and outstanding P17 explanation evidence.

Completion requires recorded commands, actual outcomes, and limitations for:

- Fresh first-device setup, reviewed adoption, and interrupted setup without
  changing identity or losing preexisting files.
- Two ordinary installs linking over LAN and an existing Tailscale network;
  no manual folder IDs, certificate files, membership files, or loopback
  endpoints in the normal journey. Network setup assumptions stay explicit.
- Expired/revoked/replayed/wrong-scope invitations, identity mismatch,
  unauthorized data access, competing approvals, and offline rollout.
- Closing/reopening the TUI, approval later, daemon restart during joining,
  device reboot, and separately tested login/unattended startup behavior.
- A second folder shared to the same device without rekeying or request-ID
  collision; a folder left unshared remains inaccessible.
- Ordinary edits, offline/reconnect, transfer interruption, one offline peer,
  root unavailability, and storage exhaustion with truthful status.
- Conflict diff/manual merge, keep-copies/select, a new version arriving
  during review, and available/expired-content restore with protected current
  bytes and explicit replacement previews.
- Current-directory inference, duplicate-name/root ambiguity, live-daemon
  CLI use, noninteractive scripts, and narrow/colorless keyboard operation.

Preserve I01–I28 as applicable. Use explicitly marked disposable roots for
failure tests, preserve unrelated workspace changes, and label every unrun
check unexecuted. Browser-era evidence is not terminal acceptance evidence.
