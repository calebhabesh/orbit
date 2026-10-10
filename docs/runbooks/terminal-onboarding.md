# Keyboard onboarding and device management

Run these journeys through `orbit` or `orbit tui --state /absolute/private/state`.
The CLI ensures the selected daemon is available before handing the terminal to
its client. Native startup acceptance remains T13 work. Closing
the interface leaves admitted daemon work running.

First use offers **c / Enter: Create** and **j: Join**. An unfinished setup opens
its actual durable operation, including blocked work. On the overview:

| Key | Action |
| --- | --- |
| c | Create/adopt another synced folder |
| J | Join a folder from a private invitation |
| a | Select a folder and create an Add device invitation |
| s | Select a folder and an existing device for separate sharing consent |
| w | List enrollment requests and inspect the exact request |
| u | Page through unfinished setups and reopen their actual operation |
| Enter | Inspect the selected folder, attention item or device |
| L | Review Leave for one Orbit; confirm to stop sync here and keep files |
| X | Review Remove Device for one Orbit; type the exact device name |

Forms retain entered values through validation and root/control errors. Tab and
Shift-Tab move between fields; Enter requests a measured root preview. Ctrl-A
shows Advanced metadata/reserve/concurrency/bandwidth inputs. The concise review
includes names, root, startup, finite budgets, numeric network listeners and
advertised addresses. Supported existing local contents will become shared;
unsupported/unreadable objects or incomplete enumeration block adoption. Esc
from the review returns to editing; Enter submits that exact controller review.

Use reachable LAN or existing Tailscale IP:port addresses for advertisements.
Loopback advertisements are refused. Orbit does not provision the network.
Manual startup is usable without systemd. Login requires the selected user
service; unattended also requires the documented lingering prerequisite.
An enabled login unit is not evidence of unattended operation. A startup error
keeps the operation visible and supplies a corrective action; it never creates
a Ready claim or changes privileged host configuration automatically.

Add device creates a one-hour folder invitation. The capability stays hidden
until **v** deliberately reveals it. Arrow/j/k scrolling exposes a long code.
**s** saves an exclusive owner-only transfer file in an existing private parent;
an existing file is preserved. Transfer the invitation deliberately, then paste
it into the receiver's hidden field or enter its absolute private file path.
Wrapped CR/LF code text is accepted in a single paste. Oversized pastes are
refused. Invitation structure, certificate pin and expiry are checked before
root review; the owning transport still authenticates the inviter before sending
the capability. Do not put invitations into diagnostics or shared transcripts.

The receiver reviews its local root and submits a possession-bound request.
On the inviter, **w**, select, Enter displays its device label, exact request,
folder, key pin and transcript verification code. Compare the code displayed
on the receiving device. **a** approves that exact request; **x** declines it.
Polling does not replace the reviewed request. A changed membership or request
requires a fresh review; retry never silently changes the approval scope.
Sharing a second folder selects the known device key and keeps a distinct
invitation/attempt/request and local-root consent.

Progress uses actual operation phases and readiness. Waiting, approval,
membership, local scanning, download/publication and observed local readiness
remain distinct. Completed setup views refresh current folder readiness rather
than treating a historical completion as current availability. Offline devices
can still have pending membership/copy work. No global synchronized claim or
invented percentage is used. Closing/reopening and restarting the selected daemon
retain the actual request and attempt.

Folder detail shows local root/pause state, membership participation and available
last-contact/stored/applied/direct observations; an absent report is unknown.
**p** previews local pause/resume; Enter confirms. **l** reviews the source and
explicit destination and calls the existing resumable relocation control.
Keep original/staging roots until recovery finishes. **x** explains local
unregistration and points to the existing removal procedure, preserving working
files. In 2.3.0, **L** provides real Leave, **X** provides exact-name-reviewed
Remove Device, and legacy **t** opens the same device removal flow. The review
counts recorded changes received here; unseen edits may remain only on the
departing device. All survivors must agree; pending operations resume from
Attention. Leave/removal do not erase remote bytes. See
[participation operations](../operations.md#leaving-an-orbit-and-removing-a-device--e13-2026-10-09). A known membership fork stays paused and links to the
[membership-fork runbook](membership-fork.md).

While a text field is focused, ordinary j/k/q/? are text. Ctrl-C closes the
interface; Esc returns from a form/review, and q closes from navigation. Explicit
operation cancellation and daemon stop are separate controls.

Run actual binary evidence with:

```sh
make test-terminal-onboarding-pty
python3 scripts/terminal_onboarding_pty_test.py --binary bin/orbit \
  --output /new/empty/evidence-directory
```

The runner needs Linux `/proc`, Python 3, PTYs and a nonloopback IPv4 interface.
It creates two new private marked roots and copies the binary into each. Signals
and cleanup validate canonical roots, markers, process birth/executable and exact
state arguments. Startup failure uses daemons with an empty fixture PATH, so
systemd/user-unit/lingering configuration cannot be changed. It asserts actual
identity/request/code, file bytes/digests, restart, second-folder scope,
pause/resume, relocation, masked invitation and terminal restoration. Native
hosts, physical LAN/Tailscale, boot/logout and actual owner use/explanation remain
separate release evidence.
