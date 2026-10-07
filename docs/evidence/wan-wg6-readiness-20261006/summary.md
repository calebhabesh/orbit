# WG6 operator-readiness follow-up — 2026-10-06

State: **partial; WG6 remains open and W15 has not started**. This follow-up
preserves W14's completed validation and the historical P/O/T/W records. It does
not publish packages or inject faults into the shared VPS.

## Actual findings

- Read-only SSH checks find the deployed `orbit-net` active and loopback
  `/healthz` returning `ok`. Metrics report release epoch 1 valid, roughly
  90 days of profile/certificate validity, zero rejected certificate reloads,
  and a 4 MiB/s aggregate relay budget. See [live health](logs/live-health.log).
- No `prometheus` or `alertmanager` executable was found on the VPS PATH, and
  no corresponding unit appeared in the inspected unit list. No `sendmail`,
  `msmtp` or `mail` executable or Postfix/Exim unit was found. These checks do
  not exclude monitoring managed elsewhere. An alert recipient, delivery
  transport and successful actual delivery are still unverified.
- The original deployment record names the owner's laptop as key custodian.
  The recorded directory is absent under the SSH `laptop` account
  `/home/caleb2002`; root inspection there requires interactive sudo.
  The key is actually present on this development workstation at
  `/home/owner/.config/orbit-operator/authority.key`, mode `0600` in a
  `0700` directory. Packaged `orbit-net key verify` proves it can sign for
  frozen public authority
  `9af3cf8a979f1b635a56831259d7645a62fb7c19db2de8be51e6afb0ce423b36`.
  See [public verification result](logs/authority-verification.log).
  Private key contents were not printed or added to evidence.
- The owner delegated backup-storage selection, then confirmed no USB is
  currently available. Encrypted removable media was the initial selected
  approach; an existing encrypted password-manager attachment/secure note is
  the alternative already allowed by the W13 deployment record. Neither a
  second copy nor a restore from it has been performed.

## Changes and verification

`orbit-net key verify --file FILE --authority HEX` reads through the existing
bounded private-file reader and checks both the expected public half and a fresh
local signing challenge. It writes no profile or key and prints only the public
authority. This catches an unusable backup with a corrupted private seed even
when its stored public half still matches.

The restore test covers exact restored bytes, uppercase public input, another
authority, invalid authority encoding, corrupted seed, unsafe permissions and
symlink refusal. All four operator package tests pass uncached under the race
detector ([log](logs/operator-race.log)); `go vet ./cmd/orbit-net` and
`git diff --check` pass. `make package-orbit-net` builds amd64 and arm64 operator
archives ([log](logs/operator-packages.log)); the actual amd64 packaged binary
verifies the existing authority. The arm64 archive was built but not executed
in this follow-up. Full `make check`, full race and native transfer campaigns
were not repeated for this bounded operator-command addition; W14's earlier
closeout remains dated evidence for its recorded source.

The operator runbook now documents safe restoration and custody evidence.
A scratch wizard at `/tmp/orbit-wg6-offline-backup.sh` was prepared using the
wizard skill's unchanged library. `bash -n` passes; ShellCheck is unavailable.
It was not run, creates no backup until driven interactively and requires an
actual mounted USB before encryption. The owner has no USB, so it is currently
inapplicable. Its syntax validation does not establish encryption, restore or
offline custody. Passphrases would be entered directly into GnuPG, not chat,
repository evidence or environment files.

## Resumable handoff

1. Select an available encrypted vault/backup medium with the owner. Store the
   existing authority key and signed profile there; verify a restored copy
   against the frozen public authority and record custody without secrets.
   A second file on the same workstation alone does not close this condition.
2. Select an alert destination the owner receives and provision its actual
   delivery transport. Configure health, profile/certificate expiry, reload,
   quota/stranded-device and egress alerts. Exercise firing and recovery using
   synthetic monitoring inputs, without interrupting or flooding the shared
   production service. Record received delivery and on-call contact. A local
   log or unreceived webhook alone is not working alerting.
3. Close W13/WG6 only after both records exist; then start W15's first bounded
   disposable harness slice from the current tracker. Wider distribution
   remains gated. W16 physical WAN, inherited T13 technical checks and deferred
   P17 owner use/explanation retain their existing states.
