# W17 combined release and handoff — 2026-10-07

State: **complete for the recorded release conditions.** Every W packet, gate,
WAN requirement and inherited T13 technical check has evidence in the
[traceability matrix](traceability.md). The open conditions below are recorded
as not executed; none of them is claimed.

## What W17 changed

- **Defect: packaged login startup could not be enabled.** In a disposable VM,
  `orbit service enable` refused the unit installed by `install.sh user`,
  because the unit names its state as `%h/.local/state/filesync` and Orbit
  compared the path literally. Fixed in `validateSelectedService`. The
  regression went red before the fix.
- **Defect: `service start` reported the wrong daemon as the service.** When
  setup had already launched a daemon, `start` reported success while the unit
  crash-looped on the state lock. `start`/`restart` now succeed only when the
  unit's `MainPID` owns the state. Otherwise they return `MANUAL_DAEMON_RUNNING`
  with the action `orbit stop`, then `orbit service start`. `orbit stop` is now
  the Orbit entry for the existing graceful stop (help and completions updated).
- Service failures no longer print their error code twice.
- The `make check` terminal-suite timeout was raised from 30 to 45 minutes. The
  suite takes about 25 minutes, so the old limit left little headroom.
- Added a [networking guide](../../runbooks/networking.md) covering tested
  networks, operator, privacy, budgets, profile expiry, other modes, startup
  and troubleshooting. Also added a case-study WAN section, WAN portfolio
  bullets and README updates, and reconciled stale status lines in the WAN
  specs, operator runbook and trackers.

## Evidence

| Check | Result |
| --- | --- |
| W16 field packages rebuilt from the committed source | Byte-identical: arm64 `596ceece…18fb`, amd64 `fa24501b…7546` ([log](provenance/w16-package-rebuild.log)) |
| Native login/logout/unattended boot, disposable KVM guest | Passed ([run 5](lifecycle-vm/run5/service-boot-vm.json)); runs 1–3 retain the failures |
| `make GOFLAGS=-v check`, clean snapshot | Passed, 1,719 s; terminal suite 1,506 s ([log](reproduction/make-check.log)) |
| Uncached full race | Passed, 1,615 s, 21 packages ([log](reproduction/race.log)) |
| Demo, packages, operator packages | Passed; repeated builds byte-identical ([sums](reproduction/SHA256SUMS.check)) |
| T13 release tests, twice under race | Passed, 146 s ([log](reproduction/t13-release.log)) |
| Harness unit tests | 36 passed |
| Orphaned test daemons after the campaign | None |
| Final-tree package test (docs only changed since snapshot) | Passed |
| Secret scan | No credential, private key or alert topic URL |

Commands: [commands.md](commands.md). Provenance: [manifest.json](manifest.json).
Results: [results.json](results.json).

**No new native WAN run.** The W17 runtime changes touch only user-service
management and the CLI entry point; the networking code is unchanged. The W16
runs therefore remain the native evidence, and their packages reproduce
byte-for-byte from the base revision. The local W16 runner rehearsal and all
WAN suites passed again inside the snapshot's `make check`.

Two orphaned daemons from interrupted October 6 runs were found and removed at
the start of this packet. The clean campaign left none.

## Supported conditions

These conditions are observed with the packaged build and the live hosted
service:

- home router NAT ↔ cloud 1:1 NAT, over direct UDP;
- the same pair with UDP blocked, over the relay;
- an address change and a service restart, with recovery;
- three devices sharing one home network, for forwarding, conflict and restore;
- same-LAN pairing (over the relay in that run);
- packaged per-user login and unattended startup (in a VM).

Details and timings are in the [networking guide](../../runbooks/networking.md#tested-networks).

## Open conditions (not executed or not claimed)

- School, corporate, CGNAT, IPv6-only and captive-portal networks; a third
  physically separate network.
- Native LAN-direct between physical devices: the W14 same-LAN run stayed on
  the relay, and the cause was not investigated. LAN direct paths have local
  fixture evidence only.
- Address-change recovery takes about 30 s, bounded by the service heartbeat.
  Shortening it needs a production `orbit-net` change, which requires owner
  approval.
- Native behaviour with an expired packaged profile (unit tests only).
- Restoring the authority-key backup and checking it with `orbit-net key verify`
  (waived by the owner).
- Physical-hardware login/logout/boot. The owner chose the VM.
- Physical power loss and Pi media resets (outside the tested fault model).
- P17 personal use and unaided explanation: removed as requirements by the owner on 2026-10-07 (not claimed).

## Operator actions

- Remove the laptop W16 namespace:
  `ssh -t laptop 'sudo ORBIT_W16_DISPOSABLE=1 ~/orbit-w16/wan_owner_netns.sh down caleb2002 wlp2s0 18'`,
  then `rm -rf ~/orbit-w16` on the laptop.
- Before **2027-01-04**: sign and ship an epoch-2 release profile, and renew TLS
  (automatic) ([rotation](../../orbit-net-operator.md#rotation)).
- Keep the relay budget within the Oracle free egress allowance, and keep the
  ntfy alert timer running.
- Optionally restore the Bitwarden authority-key copy and run
  `orbit-net key verify`.

## Future extensions (not in this release)

UPnP/NAT-PMP/PCP, TURN, WebRTC, proxy support, relay federation, mobile
clients, a short typed pairing code (needs a service mailbox), service-side
faster roaming, and Syncthing interoperability.
