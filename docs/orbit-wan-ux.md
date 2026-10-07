# Orbit automatic-network UX

Status: owner-selected direction, 2026-10-05; W06 implements reviewed CLI setup/pairing and network policy controls; W07 implements the keyboard TUI integration; W12 implements qualified status/doctor and privacy controls; W13/W14 ship the operated hosted default, and W16 records native WAN journeys. User-facing guidance: [connecting across networks](runbooks/networking.md). This amends network-related choices in [terminal UX](orbit-terminal-ux.md)
and preserves its keyboard, file-management, conflict and recovery contracts.
[Plan](orbit-wan-implementation-plan.md) and [status](implementation/wan-status.md)
own delivery. Use the existing Go Bubble Tea/Bubbles/Lip Gloss stack.

## Product decisions

| Decision | Target behavior |
| --- | --- |
| Connection default | Automatic: LAN/reachable direct paths, NAT traversal and encrypted relay fallback |
| Supporting services | Preconfigured Orbit profile, optional self-hosted profile; no separate account required |
| First device | Useful offline/local capture immediately; unavailable WAN services are a qualified networking block |
| Pairing | Exchange a short-lived invitation, request enrollment, compare verification code and explicitly approve |
| Folder access | Choose folders explicitly; a new device receives no current/future folder automatically |
| Local roots | Suggest `~/Orbit` and empty receiving root; existing-folder adoption is a normal reviewed option |
| Network details | Addresses, ports, transports, profiles and quotas under Advanced/details |
| Privacy | Setup review explains metadata visibility and offers Local network only before first announcement |
| Normal management | Existing shell/editor/file manager; TUI/CLI manage sync and recovery |

Automatic mode is a desired policy, not a connectivity guarantee. Default services
need an actual operator/profile. If that is missing or invalid, keep local use
available and explain the missing service configuration; never silently switch to
another public operator or print an internet-ready success message.

## First-device flow

1. Bare `orbit` offers **Create a synced folder** or **Join with an invitation**.
   Resume existing durable setup before proposing a fresh identity.
2. Create asks device name, folder name/location and startup preference. Keep the
   existing folder/capacity preview and finite-budget review; ordinary setup does
   not ask for an IP, port, transport or cryptographic ID.
3. Review editable defaults together. **Connection: Automatic** includes concise
   text: “Orbit services help your devices connect. They see device addresses and
   connection metadata; file contents stay encrypted in transit.” Link/open
   details in the terminal. **Local network only** and **Advanced** are accessible.
4. Confirm; persist policy and the existing setup operation. Create/reuse the
   identity, capture local contents, then activate the selected networking policy.
   A back/edit action before confirmation does not announce to global services.
5. Report local capture and service readiness independently. Offer **Add device**
   even when capture/network work is pending, with any concrete prerequisites.

Use a concise summary with Back/Edit and a focused field error. Preserve names,
paths and draft review when a probe fails. Perform network probes asynchronously
with a finite wait and cancel/retry; no spinning form with a hidden indefinite
dependency. “Saved locally; waiting for a connection” is useful honest progress.

## First-time WAN pairing

1. On device A, **Add device** selects the initial folder and produces an invitation.
   Show **Copy invitation**, **Save private file**, expiry and **Revoke**; neither
   logs nor normal JSON progress contain its secret.
2. On B, **Join with an invitation** accepts paste, stdin or a private file. Show
   inviter/folder and operator/profile context; suggest editable device name and
   empty local root. Review existing contents/capacity before committing adoption.
3. Verify the inviter's transferred certificate/pin over a direct or relay route
   before sending the enrollment secret. B can contact the relay without already
   being a folder member and without any LAN history.
4. Submit the signed request; A shows the exact device name, selected folder and
   verification code. **Approve** binds the requesting key and current reviewed
   membership; **Decline** changes no file access. Names alone do not authenticate.
5. Both interfaces can close. Their daemons retain the attempt, approved root,
   request, profile/pins and actual phase. Relaunch resumes without a new identity
   or invitation, unless the existing authorization has expired/revoked.
6. After approval, show **Updating devices**, **Scanning**, **Downloading**, local
   capture/readiness and qualified peer copies. Route selection is automatic.

An offline inviter produces **Waiting for [device] to come online**, not lost
approval or a claim that the relay accepted enrollment. Initial relay buffers
are not a store-and-forward mailbox. Expired authorization offers **Request a new
invitation** with new review; Retry does not extend its lifetime. Incorrect pins
stop before secret disclosure and retain the user's entered root/name.

Additional folders reuse the persistent approved device identity and network
routes with separate per-folder invitation/approval and receiving-root choice.
A nearby-device list is an explicit convenience view; discovering a machine never
grants access or silently starts sharing. No typing of DeviceIDs is required in
the ordinary flow, but details expose exact verification information.

## CLI and scripting

Retain current command vocabulary and aliases. Add network controls through the
same authenticated local controller and typed shared client as the TUI. Target
human workflows:

```sh
orbit setup                         # guided create/adopt with automatic connection
orbit devices invite                # select folder; deliberately transfer invitation
orbit join                          # private prompt/paste; no invitation in argv
orbit devices requests              # show pending reviewed approval choices
orbit devices approve               # select request and review exact device/folder
orbit status
orbit devices show Pi
orbit network status
orbit network doctor
```

Existing invite/approval verbs must remain compatible if their names differ;
W06 maps these examples to the current dispatch and supplies aliases rather than
deleting working commands. The first device needs no separate `init`/`serve` step.
Scripted setup uses the existing preview/private review-file/apply operation-ID
pattern, with additive network policy fields. Explicit `--json`, `--state`, named
folder/device selection and stdin/private invitation files remain available.

Network queries report mode/profile, current observed route and freshness,
candidate/probe summaries, reason codes and next action. Policy/profile changes
use preview/apply with an operation ID. A plain query never probes remote services
unless the command explicitly requests a bounded check. Non-TTY mutations require
explicit inputs/review; they never open a picker or wait for an invisible prompt.
Human help shows the three ordinary steps and Advanced examples separately.

## Status and diagnostics

```text
Orbit                 Daemon: running       Startup: at login

Documents             Saved here; sending 3 known files
Pi                    Connected via relay; last contact 14:20
Laptop                Offline; last contact 2 hours ago

[a] Add device  [f] Folders  [n] Attention  [q] Quit
```

The route is observed transport state; it is separate from locally saved,
peer-stored, applied, conflicted and pending membership observations. Global
“Everything synced” is never inferred from connected devices or service health.
Default details use **Direct connection**, **Connected via relay**, **Finding a
connection**, **Waiting for device**, or **Connection blocked**. A normal relay
connection is useful status, not a persistent error banner. Technical details
can show TCP/QUIC/ICE, addresses, RTT, profile and last failure with freshness.

| Condition | Message and next action |
| --- | --- |
| Peer unavailable | Waiting for Pi; your captured changes remain saved here |
| Relay active | Connected via relay; direct connection checks continue |
| Service outage, valid direct connection | Direct connection working; Orbit connection services unavailable |
| All routes unavailable | Cannot reach Pi; check internet/device availability or open Connection details |
| UDP traversal blocked, relay works | Connected via relay; direct UDP connection unavailable in details |
| Profile expired/missing | Connection service configuration needs update; keep local capture and offer profile review |
| Wrong peer pin | Device identity could not be verified; stop enrollment and inspect invitation/details |
| Service quota reached | Connection service is busy or at its limit; retry later or select a reviewed alternative profile |
| Membership fork | Preserve existing membership recovery action; connection retries cannot resolve it |

Avoid claims of a definitive NAT type or precise firewall cause from a single
timeout. Network doctor tests DNS/service TLS, rendezvous, relay and direct/UDP
paths separately with finite deadlines, and labels **Not tested** versus
**Unavailable** versus a verified failure. Its suggested actions never silently
change firewall/router/VPN policy. Sensitive endpoint export is deliberate.

## Advanced and privacy modes

Modes: **Automatic**, **Local network only**, **Manual/private network** and
**Self-hosted automatic**. Local-only stops global announcement, rendezvous,
STUN and relays, rejects cached public WAN candidates and drains active WAN direct
connections; permitted interface-scoped local discovery/direct paths continue.
Manual mode retains
explicit endpoints and Tailscale/WireGuard configurations. Self-hosted mode
reviews a private service profile and its operator/trust, then uses the same
protocol and guarantees. Treat internet-service consent and LAN advertising as
visible policy choices, not an accidental side effect of a status screen.

Existing installations keep their current explicit/manual policy until the owner
reviews automatic mode. Fresh installations select Automatic in their first
review. Changing a profile/mode never rotates a device identity, retires a peer,
changes folder membership or deletes local files. Preserve restartable operations
and indicate any required daemon restart honestly.

## Interaction acceptance

Use arrows/Tab and existing Vim navigation, visible focus, Back/Edit, complete
error text, colorless/narrow fallback and terminal-safe filenames. A paste must
not trigger navigation keys. Polling preserves selected folder, drafts and request
context; late responses from old selections cannot apply to a new one. Quitting
restores the terminal and leaves the daemon's accepted work running.

W06/W07 require real CLI and PTY journeys on production controls and actual file/
identity/approval assertions, including incomplete capture, wrong pin, delayed
approval, restart, expired invite and service outage. Renderer snapshots are
supporting tests. Any later usability review remains separate from automated
journey evidence; deferred P17 owner use/explanation is not claimed as complete.


## W07 keyboard controls

Create/join forms show device/folder/root, startup, data budget and connection
mode. Tab/Shift-Tab traverse visible fields, Ctrl-N cycles Automatic / Local
network only / Manual / Self-hosted and Ctrl-A opens Advanced numeric settings.
Confirm adoption includes metadata disclosure and the exact measured root; Esc
returns to editable inputs. Existing installations retain their reviewed mode.
Self-hosted operator trust is independently reviewed with `orbit network
preview/apply`; transferred invitation profile details are informational.

Join accepts masked bracketed v2/v3 paste or a private absolute file. Add device
provides `s` private save, `v` deliberate reveal/hide and `x` exact revocation
review. A revealed code can be copied from terminal output; no implicit clipboard
process runs. Exact request review compares key/folder/verification code before
`a` approval or `x` decline. Pending progress resumes after client/daemon restart.
`N` opens cached Connection details from overview, folder or setup progress;
`r` refreshes, while retrying an interrupted submission retains the reviewed
operation/attempt. Relay observations have dates and stay separate from local
capture, stored/applied copies, conflicts and membership. Missing profile/service
availability blocks networking while local capture remains usable.


## W08 LAN advertising and direct paths

Fresh setup review now shows whether signed device identity/pin and direct listener
addresses are broadcast on the LAN. Automatic and Local-only first setup select
advertising; an existing installation keeps its reviewed value. Advanced shared
network preview/apply supports `--lan-advertising=true|false` and reports restart
requirements. Mode/profile changes retain keys, approvals, roots and histories.

Known approved peers can sync on permitted LAN interfaces in Local-only without
public service traffic. Discovery does not approve unknown devices. Local-only
first pairing currently uses explicitly configured isolated legacy local enrollment;
ordinary WAN onboarding remains v3 invitation/approval through reviewed services.
Optional direct listener collision is a route limitation while capture and relay
remain available. Connection details report dated actual direct/relay observations
separately from service readiness and stored/applied copies. W11 owns native
roaming and W12 owns expanded diagnostics/privacy; W13/W14/W16 later delivered
the hosted defaults and physical WAN evidence.

## W12 qualified status and explicit doctor

Overview and `orbit status` show the selected connection mode, active-versus-
desired policy, profile state/expiry, service readiness and a dated observation
for each known peer. Each observation includes route, freshness, LAN/public/
expired candidate counts, the last UDP result and a plain next action. These
fields describe connectivity only; capture, peer-stored/applied copies,
membership and file conflict state remain separate. A relay route is useful
status and does not create an attention error.

`orbit network status` reads the cached view and never starts a network probe.
`orbit network doctor --device ID` is the deliberate action. It has a finite
budget and reports separate service DNS/TLS, authenticated directory, pinned
direct, pinned relay and actual UDP STUN results. `VERIFIED`, `UNAVAILABLE`,
`NOT_TESTED`, `DISABLED_BY_POLICY`, `QUOTA_EXCEEDED`, `IDENTITY_MISMATCH` and
`TIMEOUT` remain distinct; a timeout does not name a NAT or firewall type. `r`
refreshes cached details, `d` starts the explicit doctor, and `Esc` cancels it;
the same status and action text is available in CLI JSON, human output and TUI.

Mode/profile changes are reviewed mutations. Local-only stops global discovery,
STUN and relays, rejects cached public candidates and drains WAN direct pools;
identity, keys, history and files remain. Existing installations retain manual
behavior until the owner reviews automatic mode, while first setup records the
internet-service consent. Support exports contain only an allowlisted config
summary and redacted diagnostic strings, with no invitation, ICE/relay secret or
private filename.


## W14 packaged default, upgrades and one-step changes

Every ordinary build carries the hosted service's signed release profile.
First setup (CLI, guided CLI or TUI) selects Automatic with that profile and the
review shows its operator, expiry and privacy text next to the existing metadata
disclosure; there is no profile file to copy and no address to type. A build
without a usable packaged profile keeps the earlier "Automatic, awaiting profile"
state and local capture.

Upgrades never change a reviewed choice silently. An install that already chose
Automatic but was waiting for a profile starts using the packaged one at the next
daemon start. A manual install stays manual: status, `orbit status` overview and
the TUI overview show one offer line ("Automatic connection with … is
available"), which `orbit network automatic` reviews and
`orbit network automatic --decline` dismisses permanently. Local-only installs
are not prompted.

`orbit network automatic`, `orbit network update` and `orbit network set` replace
the preview/review-file/apply pair for people: they print the mode change,
operator, privacy text and LAN advertising, state that identity, approvals,
history and files are unchanged, and ask `Apply? [Y/n]` once. Without a terminal
they refuse unless `--yes` is given; `preview`/`apply` remain for scripts.
Switching to another operator (self-hosting, or back) explains that devices meet
through Orbit services only on one operator and asks "Replace the operator?
[y/N]" before the usual confirmation.

A newer packaged profile from the same operator with unchanged privacy text is
applied when the daemon starts; changed text appears as "An updated service
profile … review it with orbit network update" until confirmed.

`orbit devices invite --code` (or pressing Enter at the invite prompt) prints a
one-line invitation code for the other device's `orbit join` paste prompt or
`--invitation-stdin`; files remain available with `--out`. When both devices run
a build with the same packaged profile the routed code omits the profile (about
1.7 KB instead of about 3 KB). The TUI reveal shows the same code. A QR code is
not offered: Orbit has no phone client, and the receiving laptop or Pi has no
camera. A short human-typeable code needs a service-side mailbox and is a future
extension.

Pairing errors name the cause and the device to change:
`PROFILE_EPOCH_MISMATCH` ("update Orbit on this device" or "on the inviting
device"), `PROFILE_OPERATOR_MISMATCH`, `PROFILE_NOT_PACKAGED` (short code from a
different build; transfer the file instead), `UNSUPPORTED_INVITATION_VERSION`
(newer Orbit) and `NETWORK_REVIEW_REQUIRED` (manual device given a routed code).
Pre-WAN Orbit devices can still pair with and sync with upgraded devices through
v2 invitations issued from Manual mode; an Automatic device's invitation needs an
upgraded receiver.
