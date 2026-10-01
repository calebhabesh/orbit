# Prepared personal-use pilot — owner evidence pending

Ready at 2026-10-01 15:18:08 UTC. [Setup and native checks](pilot-setup.json)
record two automated setup edits, not personal use.

All three services now run the verified `e13e53a` package binaries. The
[graceful upgrade](package-upgrade-e13e53a.json) stopped only each owned pilot
unit, made a consistent SQLite backup, preserved its root/state and verified
restart plus embedded UI. A [new automatic background check](post-upgrade-background.json)
uses a clearly named setup note; it also does not count as owner use.

Use `/home/caleb2002/FileSyncPilot-20261001/data` on the laptop,
`/home/owner/FileSyncPilot-20261001/data` on the Pi, and
`/home/ubuntu/FileSyncPilot-20261001/data` on the VPS. The folder is new and
contains only explicit automated setup notes. Edit a few real, non-sensitive
notes/documents normally. File Sync services watch and pull from approved peers;
there is no manual scan/sync requirement. The pilot selects a finite 1-GiB data
budget and 512-MiB free-space reserve.

The workstation runs three project-specific SSH forwarding units because the
laptop/Pi lack a direct route to the existing VPS private address and the
workstation blocks new incoming LAN ports. All forwarding listeners are now
loopback; File Sync still authenticates each peer with pinned mutual TLS.
The workstation gateway must stay on. This is an explicit pilot deployment
dependency, not discovery/NAT traversal or independent laptop VPN access.
Existing firewall, VPN, AppArmor and unrelated services were preserved.

User services start on login; lingering was not enabled. Use the ordinary
laptop sleep/disconnect/reconnect cycle for offline work. A normal reboot/login
restarts its enabled service. To restart only this pilot service manually:

```sh
ssh laptop systemctl --user restart filesync-pilot-1cb948085d964149a6c7923d2b01996d.service
```

The embedded UI listens on laptop loopback. Its current address is recorded
in `state/control.addr`; it can change on service restart. Authentication uses
the normal one-use bootstrap flow. Service and root identifiers are in the
setup report. Do not remove or recreate a populated pilot folder.

Fault scripts require `.filesync-disposable`. These folders instead have
`.filesync-pilot`; fault-worker interruption is explicitly refused. No fault
injection, reset or disk exhaustion may target this pilot.

## Owner record — unexecuted

- Actual start/end dates and elapsed duration:
- Normal editing activity (descriptions, no private contents):
- Device disconnected/offline; edits made; reconnect time and observed result:
- Normal service/laptop restart and observed result:
- Any conflict, resolution, failed sync or other limitation:
- Owner explanation of causal resolution, restore, publication recovery,
  retention/GC tradeoff and the measured bottleneck, without agent assistance:

Do not count setup notes, scripted edits, or a short synthetic campaign as
personal adoption. There is no invented minimum duration; record enough real
use to observe the required cycle and report the actual duration.
