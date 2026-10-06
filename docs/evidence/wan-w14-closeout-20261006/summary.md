# W14 final-source validation closeout

State: **in progress**. Codex took over the dirty W00–W14 tree after Opus reached
quota. Runtime code is unchanged from the takeover snapshot. Hosted-default
publication remains gated on W13/WG6 authority-key backup and working alerting.

## Validation

Twelve discovered W14 tests pass uncached under race detection, including the
native CLI/PTy confirmation, invitation-code journey and actual pre-WAN
`ef462f2` upgrade/rollback. Affected runtime packages also pass uncached race
tests. The standalone W07 keyboard relay onboarding regression passes.
The full aggregate is still running; no passing aggregate result is claimed yet.

Exact argv, exit codes, durations and source fingerprints are in
[command records](commands.md) and individual JSON records. The
[manifest](manifest.json) records revision, dependencies, host/filesystem and
network assumptions; [initial-source.json](initial-source.json) captures the
inherited source before this closeout.

## Retained failure and correction

The [original Opus aggregate](../wan-w14-20261006/logs/make-check.log) exited 2
at `TestWANW07RealPTYRelayOnboarding`: the production binary displayed
`Inviter uses the same operator and profile:` while the keyboard fixture waited
for `Inviter operator:`. At takeover the source already contained the compatible
label with the same-profile explanation; that file was edited after the old
aggregate started. The standalone [final-source regression](logs/w07-regression.log)
passes without a further runtime change. The original failure remains intact.

The old W14 summary declared completion before final validation and linked
missing commands/hash artifacts and an absent tracker anchor. This closeout
supplies actual command/source records, adds the detailed W14 tracker entry and
corrects the owning plan/status/outcome text. It does not fabricate the earlier
unsaved command transcript.

## Evidence boundary and next work

The [inherited W14 native journey](../wan-w14-20261006/summary.md) used packaged
amd64 laptop and arm64 Pi builds, the real bundled hosted profile, a pasted code
and matching two-way SHA-256 hashes. Both devices were on the same LAN. That
journey was not rerun here and is not physically separate-network WAN evidence.
Native expired bundled-profile execution remains unexecuted; adoption/validation
expiry boundaries have unit coverage. The TUI offer/update still points at the
CLI, and old receivers still need Manual invitations or an upgrade.

W13/WG6 needs a second offline authority-key copy and a configured monitoring/
alert destination. Those operator conditions remain open. The next packet is
W15 after its W13/W14 prerequisites are satisfied; begin with the guarded
harness and transfer-recovery slice, then finish the full security/resource
matrix before W16 physical WAN acceptance. Inherited T13 technical checks and
deferred P17 owner use/explanation remain unchanged.
