# W16 host-change proposal — 2026-10-06

State: **approved by the owner and A/C executed 2026-10-06 23:51 UTC; B (owner
backup) and owner confirmation of alert receipt are pending.** See
[execution record](#execution-record). The owner asked for read-only inspection
and an exact change list before host changes.
Topic URLs and passphrases are never written to this repository.

## Read-only inspection results

| Host | OS / arch | Rootless netns | sudo | Uplink | Tunnels | Forwarding |
| --- | --- | --- | --- | --- | --- | --- |
| `vps` | Ubuntu 24.04, arm64 | blocked (AppArmor userns restriction) | passwordless | `enp0s6` via 10.0.0.1 | `wg0`, `tailscale0` | `ip_forward=1`, FORWARD policy DROP (ts/Docker chains) |
| `rpi` | Debian 12, arm64 | works, but no pasta/slirp4netns uplink | passwordless | `eth0` via home router | `tailscale0` | `ip_forward=1`, FORWARD policy DROP (ts/Docker chains) |
| `laptop` | Ubuntu 26.04, amd64 | blocked (AppArmor) | needs password | `wlp2s0` via home router | `tailscale0` | `ip_forward=0` |

No host uses a Tailscale exit node. Default routes use the physical uplinks, but
ICE gathers every selected interface, so a candidate pair over `tailscale0`/`wg0`
could carry sync traffic. Isolation must therefore hide the tunnel interfaces.
The VPS reaches its own public address `132.145.111.200` through the OCI gateway
(hairpin observed: HTTP 400 from `:8443` over `enp0s6`), so a VPS replica in a
namespace reaches the service across the physical uplink, not via loopback.
`ntfy.sh` is reachable from the VPS over HTTPS.

## A. WG6 alerting on the VPS (ntfy)

1. Copy the new arm64 `orbit-net` (sha256 `0c699f87…6fac9`, from
   `dist/orbit-net-v1.0.0-linux-arm64.tar.gz`) to
   `/usr/local/lib/orbit-net-alert/orbit-net` (root, 0755). Only its `alert`
   subcommand is used. **The running `/usr/local/bin/orbit-net` (d7f14028…) and its
   unit are not replaced or restarted.**
2. Create `/etc/orbit-net-alert/` (orbit-net, 0700) and `alert.json` (orbit-net,
   0600) with `metrics_url` `http://127.0.0.1:9464`, the private ntfy topic URL
   and name `connect.calebhabesh.com`.
3. Install `/etc/systemd/system/orbit-net-alert.{service,timer}` from
   `packaging/systemd/`, with `ExecStart` pointing at the path in step 1;
   `daemon-reload`; enable the timer (one loopback check per minute, outbound
   HTTPS only on transitions).
4. Drill: `--test`; then two runs with a separate state file and
   `--metrics-url` at an unused loopback port (FIRING down); then one normal run
   on that state (RESOLVED down). The production service is not touched. The
   owner confirms receipt of all three on their phone.

No firewall, Docker, Caddy, tunnel or orbit-net configuration changes.
Rollback: `systemctl disable --now orbit-net-alert.timer`, remove the two units,
`/etc/orbit-net-alert`, `/var/lib/orbit-net-alert` and `/usr/local/lib/orbit-net-alert`.

## B. WG6 authority backup (owner action)

Store `~/.config/orbit-operator/authority.key` and the signed release profile in
an encrypted password-manager item. Then restore the attachment into a fresh
0700 directory and run
`orbit-net key verify --file <restored> --authority 9af3cf8a979f1b635a56831259d7645a62fb7c19db2de8be51e6afb0ce423b36`,
then delete the restored copy. Record only the vault product, item name and
verification output. Claude cannot reach the vault and records no secret.

## C. Isolated replica networks (VPS and Pi)

Using `scripts/validation/wan_netns.sh` (rehearsed rootless; see
[netns rehearsal](netns-rehearsal.log)), each host gets one namespace whose only
link is a veth pair NATed to the physical uplink:

| Host | Command | Namespace address |
| --- | --- | --- |
| `vps` | `sudo ORBIT_W16_DISPOSABLE=1 wan_netns.sh up w16 enp0s6 16` | 10.231.16.2/29 |
| `rpi` | `sudo ORBIT_W16_DISPOSABLE=1 wan_netns.sh up w16 eth0 17` | 10.231.17.2/29 |

Each `up` adds, in runtime tables only (never saved to `/etc/iptables`):
- `ip netns orbit-w16`, veth `ow-w16-h`/`ow-w16-n`, `/etc/netns/orbit-w16/resolv.conf`
  (1.1.1.1, 9.9.9.9);
- FORWARD (inserted at the top, before ts/Docker chains): accept namespace →
  uplink, accept established replies, **reject namespace → any other interface**
  (so no Tailscale/WireGuard/Docker path);
- one MASQUERADE for 10.231.N.0/29 out the uplink;
- INPUT: reject namespace → host services (Postgres, Docker, Tailscale, metrics).

All rules carry comment `orbit-w16-w16`; `down` removes exactly those and the
namespace. `ip_forward` is already 1 on both hosts and is not changed. Rule
byte counters, read during each transfer, are the route proof.

Replicas then run as the ordinary SSH user inside the namespace
(`sudo -n ip netns exec orbit-w16 sudo -n -H -u <user> …`) from fresh marked
roots. Fault drills stay inside the namespace: a namespace-local nft UDP drop for
forced relay and a namespace address change for roaming. Laptop participation
(third host for forwarding/conflict/restore) needs the owner to run the same
script with their sudo password and to allow `ip_forward=1` temporarily.

## Still needing an owner decision

- **Relay/rendezvous restart**: the only public service is production. A
  disposable instance would need a temporary public port (none planned), so this
  case stays unexecuted unless the owner allows one.
- Whether hosted-default runs wait for WG6 (A and B) to close, as the gate
  requires, before C is used for the bundled-profile journeys.

## Execution record

- **A executed.** Installed binary sha256 `0c699f87…6fac9` at
  `/usr/local/lib/orbit-net-alert/orbit-net`; production `/usr/local/bin/orbit-net`
  stays `d7f14028…` and `active`. `systemd-analyze verify` passes for the new
  units (warnings only concern Oracle's own monitoring units). Drill
  ([log](wg6-alert/logs/vps-drill.log)): `--test` sent; two dead-port runs →
  `FIRING down`; normal run → `RESOLVED down`, all exit 0 at 23:51:09–29Z. The
  drill state directory was removed. Timer enabled; first production run exits
  0 with nothing firing ([log](wg6-alert/logs/vps-timer-enable.log)). **Owner
  receipt of the three notifications is not yet confirmed.**
- **C executed for VPS and Pi** ([VPS](netns-hosts/vps-up.log),
  [Pi](netns-hosts/rpi-up.log); script sha256 `32fb9115…3c970`). Inside each
  namespace only `lo` and the veth exist; the service answers over the uplink
  (`132.145.111.200`, OCI hairpin on the VPS) and the Tailscale address is
  blocked. The home public address is redacted from the Pi log. Namespaces stay
  up, idle, until the journeys run; `sudo ORBIT_W16_DISPOSABLE=1 bash
  /tmp/wan_netns.sh down w16 <uplink> <octet>` removes them.
- Runner namespace inventory ([report](netns-inventory/wan-native.json)): both
  hosts `eligible`, uplinks `eth0`/`enp0s6` verified from host rules.
- Laptop not configured (needs the owner's sudo). Relay restart stays unexecuted
  (no temporary public port approved).
