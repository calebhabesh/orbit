# W14 evidence summary

**Complete for recorded acceptance, 2026-10-06; final-source validation passed. Publication of packages beyond
the owner stays gated on WG6** (offline authority-key copy, alert destination).
Worker: Claude Code session, no delegation. Starting tree: the dirty W00–W13 tree
on `ee461a7` (W00–W13 source uncommitted), preserved. No pre-edit hash snapshot
was taken. Codex's takeover source snapshot and command records are in
the [closeout evidence](../wan-w14-closeout-20261006/manifest.json).

Status entry: [W14 tracker](../../implementation/wan-status.md#w14--packaged-defaults-migration-and-mixed-versions).
Packet: [release plan](../../implementation/wan-release.md#w14--migration-mixed-versions-and-packaged-defaults).
Commands and results: the [original aggregate log](logs/make-check.log) records
an exit-2 W07 PTY failure from a run started before the final rendering edit.
Fresh final-source checks pass, including the complete aggregate, affected
package race tests and all 12 W14 tests under race detection. See the
[closeout commands](../wan-w14-closeout-20261006/commands.md) and
[results](../wan-w14-closeout-20261006/results.json).

## What changed

| Area | Change |
| --- | --- |
| Packaged default | `internal/network/release-profile.json` (live epoch 1, digest `356f0ced…ec165`) embedded; `network.ReleaseAuthority` frozen; `ORBIT_DISABLE_PACKAGED_PROFILE=1` removes it for hermetic tests |
| Setup | CLI, guided CLI and TUI fresh setup select Automatic with the packaged digest; review shows operator, expiry and privacy |
| Upgrade/migration | `config.AdoptPackagedProfile` at daemon start (Automatic only); manual installs get a one-time offer; `network-offer.json`; `network-profile-floors.json` |
| Operator switch | `ReviewProfileChange` + `replace_operator`; precise `PROFILE_OPERATOR_CHANGE` |
| One-step CLI | `orbit network automatic | update | set` with one `Apply? [Y/n]` (`--yes` for scripts; `--decline`, `--replace-operator`) |
| Invitations | `devices invite --code`; shared code codec; compact v3 codes when both builds package the profile; `UNSUPPORTED_INVITATION_VERSION` |
| Mixed versions | `packaged_profile_v1` capability; `PROFILE_EPOCH_MISMATCH` / `PROFILE_OPERATOR_MISMATCH` / `NETWORK_REVIEW_REQUIRED` with device-specific actions |
| Join pacing fix | Joiner retries in 3 s (not 25 s) when its own relay is not ready yet |
| TUI | Overview fetches passive network status; offer/update line |
| Packaging/provenance | `orbit version` and `release-manifest.json` record the packaged profile; build refuses a profile expiring within 30 days |
| Hermetic tests | `TestMain` in daemon-launching Go suites and env default in PTY/package scripts |
| Specs | UX, architecture, protocol, persistence, operations, gates, schema, operator runbook |

## Acceptance

| Criterion | Evidence | Result |
| --- | --- | --- |
| Legacy state reopens with stable IDs/counters/hashes | `TestWANW14MixedVersionUpgradeAndRollback`: pre-WAN `ef462f2` state upgraded in place, identity + exact head JSON unchanged | Passed |
| Interrupted migrations/settings writes | Adoption matrix "interrupted update completes"; floors-before-selection ordering | Passed |
| New/old invitation and capability combinations | Pre-WAN inviter → new joiner (v2), both directions; rollback to pre-WAN binary syncs; compact/full v3 round trip; capability test | Passed (pre-WAN receiver of routed v3: unsupported by design, documented) |
| Unsupported future version | `TestWANW14FutureInvitationVersionIsPrecise` | Passed |
| Native packaged CLI/service with real profile | [native journey log](logs/native-journey.log): amd64 laptop + arm64 Pi packages, live hosted service, no profile file/addresses, pasted code, relay transfer both ways, matching SHA-256 | Passed |
| Native TUI with real profile | `TestWANW14OneStepNetworkAndInvitationCode` real PTY TUI overview shows the packaged offer | Passed |
| No-profile behavior | Native step 1 and hermetic suites: `PROFILE_MISSING_OR_EXPIRED`, local capture | Passed |
| Expired-profile behavior | Unit: adoption skips expired packaged profile; status `expired`; package build gate | Passed (no native run: static Go binaries cannot be clock-shifted) |
| Deliberate self-host override | `TestWANW14PackagedOfferReviewAndOperatorReplacement` (refuse, replace, switch back); W13 production-client self-host rehearsal | Passed |
| Manual/Local-only privacy and config | Adoption matrix; real-profile manual daemon makes no service contact and offers review | Passed |
| Automatic removes IP/port/Tailscale steps | Native journey | Passed |
| Runbooks and package provenance | [operator runbook](../../orbit-net-operator.md#rotation); manifest/version output | Done |

## Native journey timings (laptop Bell fiber LAN ↔ Pi 4B, same LAN, hosted relay)

| Step | Before fix | After fix |
| --- | --- | --- |
| Awaiting-profile install ready after restart with packaged build | 0.3 s | 0.3 s |
| Pi join submitted → request visible on laptop | 26.8 s | 5.3 s |
| Approval → first laptop file on Pi | ≈ 28 s | ≈ 29 s |
| Pi edit → laptop | 6.8 s | 5.7 s |

The approval gap is the joiner's 25 s status poll, which keeps within the inviter's
per-source enrollment budget (5 requests/minute). Route observed: `relay
CONNECTED`. Both devices were on one LAN, so this is not a cross-network (W16) claim.

## Limitations

- WG6 open items gate publishing packages beyond the owner.
- The TUI shows the offer/update as an overview line pointing at the CLI command;
  there is no in-TUI confirm screen yet.
- Codes stay ~1.7 KB (the inviter certificate dominates); a short typeable code
  needs a service mailbox. No QR: no camera on the receiving devices.
- A pre-WAN receiver cannot use a routed invitation; issue v2 from Manual mode or
  upgrade it. The pre-WAN binary's own error for a v3 code was not recorded.
- Rolling an Automatic-only install back to a pre-WAN binary leaves it without
  manual addresses (local only until configured).
- Inherited T13 login/logout/boot checks and deferred P17 owner use/explanation are
  unchanged.
