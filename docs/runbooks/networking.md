# Connecting devices across networks

Orbit's default **Automatic** connection mode lets two devices on different
networks pair and sync with no VPN, router change, IP address or account. This
page covers what Automatic does, which networks it has been tested on, who
runs the connection service and what it sees, and the alternatives.
For pairing steps, see [keyboard onboarding](terminal-onboarding.md).

## What Automatic does

Each device keeps its own identity key and its own copy of each folder. In
Automatic mode a device:

1. registers a short-lived signed entry with the Orbit connection service;
2. tries direct paths first: the local network, a reachable TCP or IPv6
   address, then a UDP path found with STUN/ICE (QUIC). Approved devices also
   share their local-network addresses with each other over their encrypted
   link, so two devices at home connect directly even when one of them has a
   firewall;
3. falls back to the service's encrypted relay when no direct path works, and
   keeps checking for a direct path while relayed.

Every path carries the same pinned mutual TLS between your two devices, so the
relay forwards ciphertext it cannot read. The route a device uses is shown as
**Direct connection**, **Connected via relay**, **Finding a connection**,
**Waiting for device** or **Connection blocked**. Being relayed is normal and
not an error. Sync, conflicts and restore behave the same on every route.

Automatic is a preference, not a promise that every network will connect.

## Tested networks

"Observed" means real devices on real networks, with the packaged release
build and the hosted service. Times are single measurements on these hosts,
not limits. Records: [W16 runs](../evidence/wan-w16-20261006/native-hosted/summary.md),
[W14 packaged LAN journey](../evidence/wan-w14-20261006/summary.md),
[W10/W15 emulated matrix](../implementation/wan-status.md#w10--icestun-coordination-and-traversal).

| Network situation | Result | Evidence |
| --- | --- | --- |
| Two devices on the same home LAN | Observed: pairs and syncs. The laptop's firewall blocked Orbit's LAN discovery and direct LAN connections; the last recorded route was direct UDP through the router's public address. The peer LAN exchange (2026-10-07) handles a firewall on one side | W14 laptop/Pi packaged journey; [LAN exchange record](../implementation/wan-status.md#post-w17--same-lan-direct-paths-and-profile-epoch-2-2026-10-07) |
| LAN discovery and direct LAN paths between approved devices | Local fixtures and namespaces only | W08 |
| Home router NAT (Raspberry Pi) ↔ cloud VM behind 1:1 NAT (Oracle VPS) | Observed: **direct UDP**; a 4 MiB version in 6–9 s | W16 runs 14–19 |
| Same pair with UDP blocked on one side | Observed: **relay**, zero direct bytes; 1 MB in 5–8 s each way | W16 forced-relay drill |
| One device's address changes mid-session | Observed: recovers; first transfer after the change 21–40 s | W16 address-change drill |
| Connection service restarts | Observed: relay back 9.5 s after the restart | W16 run 19 |
| Third device on the same home network, forwarding while the author is offline | Observed: forwarded version keeps its author; conflicts and restore reach all three | W16 run 20 |
| Endpoint-independent, address-dependent filtering, double/incompatible NAT, peer UDP blocked | Emulated only (Linux namespaces): direct where compatible, relay otherwise | W10, W15 |
| School, corporate, carrier-grade NAT (CGNAT), IPv6-only, captive portals | **Not tested** | — |

The two physical networks are one home fibre connection and one Oracle Cloud
region. Untested networks may still work over the relay, provided outbound
HTTPS to the service is allowed, but no result is claimed for them.

## Who runs the service and what it sees

The packaged release profile names the operator. Orbit shows its operator,
expiry and privacy text during setup, before anything is announced:

> Operated by Caleb Habesh on an Oracle Cloud server in Canada. Contact:
> calebhabesh@gmail.com. The service sees device IDs, device key fingerprints,
> public IP addresses and connection timing. It keeps them in memory only:
> directory entries expire within 10 minutes and relay sessions end when
> devices disconnect. It never sees file names, folder names or contents;
> relayed data stays encrypted end to end between your devices. The service
> writes no request logs. Oracle Cloud may keep its own network records under
> its policies.

The hosted service is a personal project's best-effort service, with no uptime
or indefinite hosting commitment. Its relay has finite budgets: 4 MiB/s in
total, 2 MiB/s per device and 16 GiB per relay session. When a budget is used
up, devices see "Connection service is busy or at its limit" and keep using
direct paths. Operator procedures (keys, monitoring, rotation, incidents) are
in the [operator runbook](../orbit-net-operator.md).

## Profile expiry and updates

The release profile in current builds (epoch 2) expires on **2027-10-07**.
Builds that carry epoch 1 keep working until **2027-01-04**; the service accepts
both until then. Newer Orbit releases carry newer profiles:

- A newer profile from the same operator with the same privacy text is applied
  automatically when the daemon starts.
- If the operator or privacy text changed, `orbit status` shows "An updated
  service profile … review it with `orbit network update`". Nothing changes
  until you confirm.
- An expired profile stops the service connection, but local capture and any
  existing LAN, manual or self-hosted paths continue. Status reports that the
  service configuration needs an update. Install a newer Orbit to fix it.

Devices that pair through the service must use the same operator. A
`PROFILE_EPOCH_MISMATCH` pairing error names which device to update.

Native behaviour of an expired *packaged* profile has only been checked in unit
tests.

## Other connection modes

Changing mode never changes a device's identity, approvals, history or files.
Each command shows the change and asks `Apply? [Y/n]` once (`--yes` for
scripts).

| Mode | Use when | Command |
| --- | --- | --- |
| Automatic (default) | Ordinary use across networks | `orbit network automatic` |
| Local network only | No internet service: no announcement, STUN or relay; LAN discovery continues | `orbit network set --mode local_only` |
| Manual/private network | Fixed addresses, or Tailscale/WireGuard ([prerequisites](private-network.md)) | `orbit network set --mode manual` |
| Self-hosted | Your own `orbit-net` service | `orbit network set --mode self_hosted --profile-file profile.json` ([operator guide](../orbit-net-operator.md#self-hosted-devices)) |

Devices that are not upgraded can still pair with upgraded devices through
invitations created in Manual mode.

## Keeping sync running

Quitting `orbit` leaves the daemon running. To start it automatically:

```sh
orbit service enable --mode login     # start at login
orbit service status
```

The setup process may have started the daemon outside systemd. In that case
`orbit service start` replies `MANUAL_DAEMON_RUNNING`; run `orbit stop`, then
`orbit service start`, or wait for the next login. For a Pi or server that
should sync with nobody logged in, run the documented owner step once, then
enable unattended mode:

```sh
loginctl enable-linger "$USER"        # owner step; Orbit never runs it
orbit service enable --mode unattended
```

In a disposable Debian 13 KVM guest, with the packaged per-user install, login
mode started at login and stopped at logout. With lingering on, the unit
survived logout and, after a reboot, was capturing edits 14 s after the guest started booting, with nobody
logged in ([W17 record](../evidence/wan-w17-20261007/summary.md)). That was a
virtual machine; physical-hardware reboots have not been tested.

## Firewalls on your devices

Orbit never changes firewall settings. Outbound connections are all it needs
to work, using the relay if necessary. To get a direct connection between two
devices on the same network:

- **One device accepts inbound connections** (for example a Pi or a desktop
  with no firewall): nothing to do. Once the devices have reached each other
  once, they swap local addresses over their encrypted link and the
  firewalled device connects directly to the open one.
- **Both devices drop inbound connections** (for example `ufw` or
  `firewalld` with a deny-incoming policy on both): give Orbit fixed ports on
  one device and allow them from your local network only. Orbit otherwise
  picks random ports at each start. Stop the daemon (`orbit stop`), then
  create `~/.local/state/orbit/direct-network.json`, readable only by you:

  ```json
  {"interfaces": [], "listen": ":22028", "disabled": false, "udp_listen": ":22028"}
  ```

  ```sh
  chmod 600 ~/.local/state/orbit/direct-network.json
  # ufw example; replace 192.168.1.0/24 with your network
  sudo ufw allow from 192.168.1.0/24 to any port 22027 proto udp   # discovery
  sudo ufw allow from 192.168.1.0/24 to any port 22028             # direct TCP and UDP
  ```

  Then start Orbit again. Only one Orbit daemon per machine can use fixed
  ports.

`orbit status` on the firewalled device shows **Direct connection** once its
path to the open device works. The open device may keep showing **Connected
via relay**, because its own connections toward the firewalled device are
still blocked. Sync works either way.

## Troubleshooting

Start with `orbit network status`. Next, run `orbit network doctor`. It checks
DNS, service TLS, rendezvous, relay and direct/UDP paths separately, each
within a fixed time limit, and reports each as passed, unavailable or not
tested. It never changes firewall, router or VPN settings.

| Message | Meaning and next step |
| --- | --- |
| Connected via relay; direct UDP connection unavailable | UDP is filtered somewhere. Sync works; nothing to fix |
| Connected via relay while both devices are on the same network | A firewall drops inbound connections on both devices. See [Firewalls on your devices](#firewalls-on-your-devices) |
| Waiting for *device*; your captured changes remain saved here | The other device is off or offline. Changes send when it returns |
| Cannot reach *device* | No route works. Check both devices' internet access, then open Connection details |
| Direct connection working; Orbit connection services unavailable | The service is down; existing direct paths continue |
| Connection service is busy or at its limit | Relay budget reached. Retry later, or use a direct, manual or self-hosted path |
| Connection service configuration needs update | The profile expired or is missing. Install a newer Orbit (see above) |
| Device identity could not be verified | Wrong pin: stop and check the invitation came from your device |
| `PROFILE_EPOCH_MISMATCH` / `PROFILE_OPERATOR_MISMATCH` | The two builds use different service profiles. Update the named device, or use one operator |
| `NETWORK_REVIEW_REQUIRED` | A Manual-mode device was given a routed code. Run `orbit network automatic`, or invite from Manual mode |

After a network change (Wi-Fi to wired, new address), expect up to about 30–40
seconds before transfers resume. The service holds the old connection until
its heartbeat expires.

Orbit's sync engine is its own. Its setup flow resembles Syncthing's, but it
is not Syncthing-compatible and does not aim for Syncthing's feature set.
