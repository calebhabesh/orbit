# Terminal operator guide

Run `orbit` or `orbit tui` with interactive stdin/stdout. Both discover the
selected private state and reuse or start its daemon. `--state /absolute/path`
selects explicitly; competing XDG/legacy installations require that selection.
Pipes use status and `--json` uses JSON, without starting a daemon or prompting.
The desktop entry opens a terminal. `filesync serve --state /absolute/path`
is the direct daemon entry; ordinary engine commands remain available.

Use j/k or arrows to select, Enter to inspect, Esc to return, ? for help and
q/Ctrl-C to close. Typed j/k/q/? remain text when an input is focused. Closing
cancels the client's requests/tools and restores the terminal; committed work
continues under the daemon. Explicit `orbit service stop` stops synchronization.

Create offers a measured root review before adoption. Inspect names, existing
contents, finite budgets/reserve, listener/advertised addresses and startup mode.
Unsupported objects or incomplete scans block readiness. Join uses a privately
transferred invitation, a separate root review and durable waiting operation.
Compare the receiving verification code with the inviter's exact request before
approval. On Overview, a adds a device, w reviews requests, s shares a selected
folder with an existing device, and u reopens unfinished operations. Each folder
requires separate consent. See [keyboard onboarding](terminal-onboarding.md).

CLI create/adopt uses the same durable review, and retries use the same request:

```sh
orbit setup --root /absolute/Notes --name Notes --label Laptop \
  --preview --review-file /private/setup.json
orbit setup --request-file /private/setup.json
orbit setup --resume --request-file /private/setup.json
orbit join --invitation-file /private/invitation --root /absolute/Notes \
  --preview --review-file /private/join.json
orbit join --request-file /private/join.json --timeout 0
```

For a previously reviewed typed share mutation, run `orbit folders share
--request-file /private/share.json > /private/second-folder-invitation` with
an owner-only output directory and umask 077. The file binds exact folder,
device/key and membership review; use the TUI s workflow to obtain an interactive
review when you do not already have one. Sharing does not authorize another
folder implicitly.

Use an existing owner-only parent for private files. Invitations must not appear
in example argv, diagnostics or shared evidence. CLI device/sharing help and
`docs/runbooks/terminal-recovery.md` specify the review files for approvals and
content operations. Lost responses do not authorize a new operation identity.
On a pending content-operation screen, `r` resumes the retained exact operation.
The screen observes actual publication on normal refresh ticks. Completed
operations keep their committed version IDs; a newer or competing head requires
fresh review rather than replaying old bytes.

Overview/Attention reports local root/capture/membership and dated peer copy
observations separately. f opens folders, C conflicts, n attention, s storage,
m maintenance, b Deleted and p selects a path. Folder inspection lists bounded
path pages; history shows exact versions and actual content availability.
Conflict selection/keep copies/manual editor merge require a current exact
review. Nonzero editor exit preserves its result; changed heads require a fresh
review/session before commit. Deleted offers original-path or separate-copy
restore. Storage and Maintenance screens are read-only previews. Local root
unavailability pauses affected work without manufacturing deletes.

```sh
orbit status --json
orbit conflicts --folder Notes
orbit history notes.txt --folder Notes
orbit deleted --folder Notes
orbit storage usage
orbit doctor
```

LAN or an already configured Tailscale network must route between the advertised
numeric nonloopback addresses. Choose distinct TCP ports for pinned peer transfer
and possession-authenticated enrollment, for example 8443 and 8444. Setup Advanced
records listener addresses and reachable advertisements. Keep owner control on
loopback; it is not the enrollment endpoint. Permit both selected ports on the
intended interface using your existing firewall/ACL policy. Orbit does not install
Tailscale, enable lingering or change firewall policy. [Network details](private-network.md)
and [install/legacy adoption](install.md) explain prerequisites and next actions.

Manual, login and unattended startup are different choices. A running daemon is
not evidence of login enablement; enablement is not evidence of logout/boot
persistence. For custom/legacy state paths, review the unit's ExecStart/ExecStop
before enabling it. [Rollback](rollback.md) distinguishes keeping the current
metadata from restoring old counters. [Recovery](database-recovery.md) uses the
stopped restore/rekey procedure. [Uninstall](uninstall.md) preserves state/files.

`orbit legacy-browser` and `orbit launch` retain the earlier explicit browser
bootstrap behavior for compatibility. Embedded assets are retained; they are not
required for terminal workflows. Keep bootstrap output private. Retained browser
regressions establish compatibility, while current terminal release acceptance
uses CLI/PTY journeys and separately recorded native service/network evidence.
