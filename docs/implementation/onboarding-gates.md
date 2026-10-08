# Onboarding design gates — E00 scoping

Scoped 2026-10-08 by E00. Each gate stays **open** until its owning packet
records the selected design, executable evidence, the updated owning
specification and remaining limitations ([plan](../orbit-onboarding-implementation-plan.md#design-gates)).
Facts below were read from source at `22f4bee` or observed read-only on the
owner's hosts; candidates are options, not decisions.

## EG3 — Host-class startup default (E01)

**Closed 2026-10-08 by E01.** Selected design: candidate 3, scoped to the
selected state. A host is a desktop if `systemctl get-default` is
`graphical.target` or the user's `graphical-session.target` is active, and
proposes `login`. Otherwise it is headless: `unattended` with lingering on,
else `login` plus the shown `sudo loginctl enable-linger USER` and a re-check.
With no user manager the proposal is `manual`. The proposal applies only when
the effective unit serves the state (or no unit exists and it is the default
state); otherwise `manual` with a note. Saved settings keep the owner's choice. A
setup that chose `unattended` while lingering is off enables `login`, and status
reports `login`. Evidence: `TestOnboardingE01EG3HostClassFixtures` (six host
fixtures), `TestOnboardingE01EG3FreshSettingsUseHostDefault`,
`TestOnboardingE01F12HostNoteAndLingerRecheck`, the no-escalation grep, and the
recording `sudo`/`loginctl` stand-ins ([E01 evidence](../evidence/onboarding-e01-20261008/summary.md)).
Owning spec: [operations](../operations.md#daemon-ownership-and-service-defaults-e01-2026-10-08).
Limitations: classification is fixture-tested plus the PC's real manager; a
desktop booted to multi-user with no graphical session is classed headless
(the selector shows the alternative, E03).

Scoping record (E00):

Facts:

- Read-only host probes (2026-10-08): PC `graphical.target`, local Wayland
  session, user `graphical-session.target` active; laptop over SSH
  `graphical.target`, session `tty`/remote, `graphical-session.target` active
  (the owner was logged in graphically); Pi `multi-user.target`, session
  `tty`/remote, `graphical-session.target` inactive. All three have `seat0`
  and `Linger=no`.
- So seat presence does not separate desktop from headless, and the *current*
  session type is misleading over SSH: an SSH session to the laptop is `tty`.
- `CheckServiceStatus` already reads lingering from `/var/lib/systemd/linger/<user>`
  then `loginctl show-user -p Linger`, and prints `loginctl enable-linger <user>`
  without `sudo`.

Candidates:

1. `systemctl get-default` = `graphical.target` → desktop (login); otherwise
   headless (unattended). Stable across SSH; wrong for a desktop booted to
   multi-user by choice.
2. User manager's `graphical-session.target` ever active / currently active →
   desktop. Correct for the trial hosts now, but depends on someone being
   logged in graphically at the time of setup.
3. Combination: desktop if (1) or (2); headless otherwise; the selector always
   shows the alternative, so a wrong guess costs one arrow key.

Evidence needed: fixtures (stubbed `systemctl`/`loginctl` on PATH, as in
`TestOnboardingE00F04F14…`) for desktop local, desktop over SSH, headless with
linger off/on, and no user manager (container); proof that no code path runs
`sudo` or `loginctl enable-linger` (grep plus a stub that fails the test if
invoked with `enable-linger`); linger re-check after the owner runs the command.

## EG1 — Short-code security (E06)

Owner selected CPace on 2026-10-08. Implementation under validation uses
CPaceRistretto255/SHA-512, initiator/responder variant pinned to
[draft-irtf-cfrg-cpace-21](https://www.ietf.org/archive/id/draft-irtf-cfrg-cpace-21.txt),
with `github.com/gtank/ristretto255 v0.2.0` (BSD-3-Clause) and its
`filippo.io/edwards25519 v1.1.0` arithmetic dependency. This is a source-reviewed
library choice, not a claim of an independent audit of Orbit's integration.
Published Appendix B.3 generator, public-message and ISK vectors pass.
The gate remains open until the full production acceptance set is recorded.


Facts: the WAN service already authenticates devices by key with signed proofs,
one-use challenges (`MaxOutstandingChallenges` 2, `ChallengeLifetime` 60 s),
per-device metadata buckets (1/s, burst 10) and a 1,024-entry rate table; it
holds state in memory. No PAKE implementation is in `go.mod` today; Go's
standard library has no PAKE.

Candidates:

- Construction: CPace (draft-irtf-cfrg-cpace) over ristretto255 or SPAKE2
  (RFC 9382). Candidate implementations must be maintained, have published test
  vectors, and avoid cgo. Option of implementing CPace on a reviewed
  ristretto255 library with the RFC/draft vectors as tests, versus importing a
  full PAKE package; dependency review either way.
- Code: 8 Crockford base32 characters (40 bits) split mailbox/password as
  proposed, versus a longer password half if the one-guess bound is weakened by
  anything (e.g. the inviter renewing a code reuses the password).
- Mailbox: one claim, one PAKE attempt, burn on failure, 10-minute expiry;
  creation authenticated by device key; per-key and per-source caps.

Evidence needed: published-vector tests for the chosen PAKE; a test that a
wrong password burns the mailbox and tells the inviter; expiry, reuse, restart
(mailbox lost → clear "ask for a new code"), squatting (claims on random names
bounded per source) and flood bounds against a local fixture; a synthetic
service capture showing no readable invitation; a review of the profile privacy
text against what the mailbox holds (decides whether the profile epoch changes).

## EG2 — Files-view truthfulness (E08)

**Closed 2026-10-08 by E08.** Each listed entry gets at most one label, derived
from this replica's own records with the folder-readiness rules
(`internal/repository/file_state.go`), in this order:

| Label (`state`) | Shown as | Derived from |
| --- | --- | --- |
| `blocked` | Blocked | the path's projection has a block reason (capture failed, unstable file, publication refused) |
| `conflict` | Conflict | more than one head, or a structural conflict on the path or an ancestor |
| `content_missing` | Content missing | the newest file head's content is `unavailable` or has an unrepaired quarantined chunk |
| `downloading` | Downloading | the newest file head's content is `pending` |
| `waiting_publish` | Arriving | one ready head that the projection has not applied (`applied_author/counter` ≠ head) |
| `captured` | Saved here | one ready head applied to this working copy |
| `deleted` | Deleted | the head is a tombstone (file details and the deleted list; deleted paths are not listed in the tree) |

Implicit parent directories with no version get no label. Two candidates were
dropped: **waiting to capture** (an edit made since the last scan leaves no
record until the scanner sees it; details show "Last checked here" instead),
and **unsupported entries** (symlinks, hard links, nested mounts are scan
issues only, never stored; they stay in Attention/readiness). The listing now
includes never-captured entries that carry a block reason, which it previously
hid, and never lists `.orbit-internal`.

**On other devices** appears only in a file's details: one line per device from
`peer_progress` for the newest head, as `stored` (durable receipt or remote
`STORED`/`APPLIED`) and `in its folder` (remote `APPLIED`), each with the time
of that device's report ("reported 3 h ago"); no report reads "No report from
another device about this version yet". There is no "synced everywhere" mark.

Evidence: `TestOnboardingE08FileStatesFromProductionPaths` (each state from scan,
import, `MarkContentReady`, `QuarantineChunk`, a two-head conflict, a refused
publication over an uncaptured edit, a remote tombstone, then `Apply`),
`TestOnboardingE08ObservationKeepsItsAge`, `TestOnboardingE08PagesTenThousandEntries`
(control); model and PTY tests in `internal/terminal` and `tests/terminal`
([E08 evidence](../evidence/onboarding-e08-20261008/summary.md)). Owning
specs: [terminal UX amendment](../orbit-terminal-ux.md#owner-amendment-2026-10-08)
and the `files`/`file_details` queries in `internal/control/terminalcontract`.

Scoping record (E00):

Facts: repository already offers `BrowseWorkspaceDirectory`, `SearchWorkspace`,
`FileDetails`, `FilePathHistory`/`BrowsePathHistory`, `BlockedPaths`,
`BrowseConflictPaths` and deleted-file browsing. W12 observations
(`tc.Observation`) carry per-device `saved/stored/applied`, `observed_at`,
`last_contact`, `availability` and `online`.

Candidates for the per-file state (local, derived without new guarantees):
captured here (projection matches a captured version), waiting to publish
(local version not yet published), downloading/missing content (manifest known,
objects missing), conflict (concurrent heads), blocked/unsupported
(`BlockedPaths`), deleted (tombstone head). "On other devices" only as
per-device observations qualified saved/stored/applied with `observed_at`
freshness — never a global "synced everywhere".

Evidence needed: a fixture per state built through production capture/sync
paths, mapping each to exactly one label; proof that observation staleness is
shown (aged observation renders with its age, not as current); CLI/TUI parity
on the same folder; 10,000-entry paging.

## EG4 — Relay egress accounting (E09)

**Closed 2026-10-08 by E09.** `orbit_net_relay_bytes_total` counts relay
payload once, when written to the receiving device, in both forwarding
directions: it is relay **egress** payload. The monthly budget uses the same
count. It is kept in a private `relay-month.json` (written at most every 30 s
and on stop, so a crash forgets at most 30 s of counting) and reset at each UTC
month start. Measured TLS/WebSocket framing overhead was 0.22% for a 4 MiB
bulk transfer, so the deployed budget leaves a few percent of headroom. At the
budget, new data relays are refused with `RELAY_BUDGET` and live ones end at
the next chunk; pairing relays stay available. Devices say "relay unavailable
until <next month>; direct connections still work" and retry the relay hourly.
Evidence: `TestOnboardingE09…` in `internal/rendezvous`, `cmd/orbit-net` and
`internal/control` ([E09 evidence](../evidence/onboarding-e09-20261008/summary.md)).
Owning specs: [operator guide](../orbit-net-operator.md#budgets-and-capacity).

Scoping record (E00):

Facts: `orbit_net_relay_bytes_total` is incremented in `quotaConn.Write`
(`internal/rendezvous/relay.go`) on both legs, so each forwarded byte is counted
once, when it is written to the receiving device: it measures relay payload
**egress** in both forwarding directions. Ingress of the same bytes is not
counted separately. TLS/WebSocket framing overhead is not included, so the
counter undercounts wire egress by that overhead. `relay_bps`
(default 20 MiB/s) and `relay_device_bps` (5 MiB/s) are rate limits, not
volume budgets; `relay_session_bytes` (16 GiB) bounds one session. State is in
memory only; nothing survives restart.

Candidates: a monthly budget on the same counter with a private state file
(month key + bytes) written periodically and on shutdown; crash loses at most
one write interval (bounded undercount, documented) versus fsync per chunk
(cost). Reset at UTC month start. At the budget: refuse new relay sessions with
reason `budget`; end existing sessions at the next chunk boundary. Overhead
factor: measure TLS/WS overhead on a local fixture and either add it or set the
budget with headroom.

Evidence needed: fixture that reaches the budget, refuses, alerts at 80%/100%,
survives restart with the persisted count, and resets at a simulated month
boundary; measured overhead ratio; client wording for "relay unavailable until
<date>".
