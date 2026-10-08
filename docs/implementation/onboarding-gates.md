# Onboarding design gates — E00 scoping

Scoped 2026-10-08 by E00. Each gate stays **open** until its owning packet
records the selected design, executable evidence, the updated owning
specification and remaining limitations ([plan](../orbit-onboarding-implementation-plan.md#design-gates)).
Facts below were read from source at `22f4bee` or observed read-only on the
owner's hosts; candidates are options, not decisions.

## EG3 — Host-class startup default (E01)

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
