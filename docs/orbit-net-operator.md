# Operating orbit-net

`orbit-net` is Orbit's connection service: an authenticated rendezvous directory,
an opaque encrypted relay and an optional STUN responder. It never stores folders,
file history, device keys or invitation secrets. File contents cross it only as
the devices' inner pinned TLS; the service cannot read them.

This runbook is for **operators**: whoever runs the hosted default service, or
someone self-hosting for their own devices. Device owners do not need any of it;
their side is one reviewed command (see [Self-hosted devices](#self-hosted-devices)).

| Who | Work |
| --- | --- |
| Operator (provisioning) | Host, DNS, TLS, firewall, keys, signed profile, monitoring, rotation, incidents |
| Device owner (installation) | Install Orbit; for self-hosting, review the operator's profile and CA once per device |

## What you need

- A Linux host (amd64 or arm64) with systemd, reachable from the devices. For a
  public service that means a public IPv4 and/or IPv6 address. Do not reuse a host
  that runs unrelated production workloads unless they are isolated with separate
  users, ports and resource limits.
- One TCP port for HTTPS/WSS (normally 443) and, optionally, one UDP port for STUN
  (normally 3478) open in the firewall.
- A DNS name you control, and a TLS certificate for it. Release (hosted default)
  profiles need a publicly trusted certificate and public addresses. Self-hosted
  profiles may use private addresses and a private CA that devices review.
- An offline machine (or at least an offline directory) for the **profile
  authority key**. It is the root of device trust and never belongs on the server.
- An egress budget. Relay traffic is counted in both directions; see
  [Budgets](#budgets-and-capacity).

## Install

From the operator archive `orbit-net-v<version>-linux-<arch>.tar.gz` (built with
`make package-orbit-net`, checksums in `dist/orbit-net-SHA256SUMS`):

```sh
sha256sum -c orbit-net-SHA256SUMS --ignore-missing
tar -xzf orbit-net-v1.0.0-linux-amd64.tar.gz
cd orbit-net-v1.0.0-linux-amd64
sudo install -m 0755 bin/orbit-net /usr/bin/orbit-net
sudo install -m 0644 lib/systemd/system/orbit-net.service /etc/systemd/system/
sudo install -m 0644 lib/sysusers.d/orbit-net.conf /etc/sysusers.d/
sudo systemd-sysusers
sudo install -d -o orbit-net -g orbit-net -m 0700 /etc/orbit-net /etc/orbit-net/tls
```

Every file under `/etc/orbit-net` must be owned by `orbit-net` and mode `0600`;
the service refuses group/world-readable keys, profiles and configuration.

## Keys and the signed profile

There are two Ed25519 keys:

| Key | Lives | Signs | Backup |
| --- | --- | --- | --- |
| Profile authority | Offline, never on the server | Each profile epoch | **Required**, offline, two copies |
| Service key | `/etc/orbit-net/service.key` | Relay attachment credentials | Optional; replace via a new epoch |

```sh
# Offline machine, owner-only directory:
orbit-net keygen --out authority.key        # prints the authority public key
# Server (or offline, then copy):
orbit-net keygen --out service.key          # prints the service public key
```

Write a profile template (see `profile-template.example.json`). Numbers such as
`epoch` are JSON strings. `origins` lists exactly the `https://` and `wss://`
form of the service host (omit `:443`); `stun` lists numeric `IP:port` entries
(or `[]`). `privacy` is shown to every device owner before they approve the
service, so state plainly what the service sees, how long it is kept, which host
logs exist and how to contact you.

```sh
orbit-net profile sign --authority-key authority.key --template template.json \
  --environment release --valid-for 2160h --out profile-1.json
orbit-net profile verify --profile profile-1.json --authority <authority-public-key>
```

`--environment` is `release` (hosted default: public origins and addresses,
system TLS roots), `self_hosted` (may use private addresses and a reviewed CA) or
`development` (tests only). Signing refuses a release profile with private
origins. Keep each signed selection file; later epochs are signed with
`--previous` so the tool enforces the same authority, environment and a higher
epoch, exactly as devices do. Recommended validity is 90 days with a new epoch
signed 30 days before expiry.

## Configure and start

Copy `serve.example.json` to `/etc/orbit-net/serve.json` and set the real origin,
paths, STUN address and budgets. Unknown fields are rejected. On clouds that
reach the VM through 1:1 NAT (the VM only sees a private address), set `listen`
to the private address and add `"stun_bind": "<private-ip>:3478"`; `stun_listen`
stays the public address from the signed profile. Then:

```sh
sudo -u orbit-net orbit-net serve --config /etc/orbit-net/serve.json --check
sudo systemctl enable --now orbit-net
journalctl -u orbit-net
```

`--check` validates the profile, service key, origin, TLS pair (current validity
and host coverage), STUN address, loopback-only metrics address and budgets, and
warns when the profile or certificate expires within 14 days. The unit runs the
same check before every start. TLS must terminate at `orbit-net`: requests with
forwarding headers are refused, so do not place a TLS-terminating proxy or CDN in
front of it. A TCP/UDP passthrough load balancer is fine.

## Budgets and capacity

Every value is a finite ceiling, not an availability or bandwidth promise.

| Setting | Default | Meaning |
| --- | --- | --- |
| `relay_bps` | 20 MiB/s | Aggregate relay ciphertext, both directions |
| `relay_device_bps` | 5 MiB/s | Per device, retained 60 s across reconnects |
| `relay_session_bytes` | 16 GiB | Per tunnel lifetime |
| Fixed | 256 sockets, 128 controls, 128 sessions, 128 leases | Overflow is refused, never queued |
| Fixed | 128 pre-auth req/s; 2/s per source (burst 128) | Shared NAT: up to 128 devices can register together, then 2/s |
| Fixed | 1 metadata op/s per device (burst 10) | Per device key, independent of source address |
| STUN | 10/s per IPv4 /24 or IPv6 /64, 200/s total | Responses ≤ 3× request, ≤ 128 bytes |

Worst-case relay egress per month is about `relay_bps × 2.6 million seconds`
(20 MiB/s is about 54 TiB). Set `relay_bps` to what your hosting plan can pay
for; devices fall back to direct paths and simply see the service as busy when
it is reached. The unit caps the process at 4096 file descriptors and 512 MiB of
memory; the local rehearsal measured about 265 descriptors and 25 MiB RSS under
a 320-socket flood (see W13 evidence).

## Monitoring

Set `metrics_listen` to a loopback address (for example `127.0.0.1:9464`). It
serves `/healthz` (200 while any served epoch is valid) and `/metrics` in
Prometheus text format. No metric carries an address, device ID, pin or session.

| Alert | Condition |
| --- | --- |
| Profile expiring | `orbit_net_profile_expiry_seconds` < 30 days on the newest epoch |
| Certificate expiring | `orbit_net_certificate_expiry_seconds` < 14 days |
| Reload rejected | `orbit_net_certificate_reload_failures_total` increased |
| Saturation | `orbit_net_connections_refused_total` or `orbit_net_refusals_total{reason="quota"}` rising for 10 min |
| Stranded devices | `orbit_net_refusals_total{reason="untrusted"}` or `{reason="expired"}` rising after a change |
| Egress | `rate(orbit_net_relay_bytes_total)` near `orbit_net_relay_limit_bytes_per_second` |
| Down | `/healthz` not 200, or the unit not active |

Without a Prometheus stack, `orbit-net alert` implements this table. Each run
takes one sample and posts to an ntfy-compatible topic only on a firing or
resolved transition. Down needs two consecutive failed checks; the rising and
egress (≥ 80% of the aggregate limit) rows need ten minutes of consecutive
one-minute samples, and a gap over five minutes restarts that streak. A rejected
reload stays firing until a later successful reload or a restart. If delivery
fails, the transition is kept pending and retried on the next run, and the unit
fails visibly in the journal.

```sh
install -d -m 0700 -o orbit-net -g orbit-net /etc/orbit-net-alert
# alert.json (see alert.example.json), owned by orbit-net, mode 0600. The topic
# URL is a secret: anyone who knows it can read and post to the topic.
sudo -u orbit-net orbit-net alert --config /etc/orbit-net-alert/alert.json --test
systemctl enable --now orbit-net-alert.timer
```

To drill firing and recovery without disturbing the service, run the check as
`orbit-net` with a separate state file and `--metrics-url` pointing at an
unused loopback port (twice, which fires Down), then once without it (resolved).

The service writes only lifecycle lines to the journal (start, reload, stop,
configuration errors). Your host, firewall and hosting provider may keep their
own connection logs; disclose them in the profile's privacy text.

## Rotation

**TLS certificate.** Renew as usual (for example with an ACME client). Install the
new chain and key into `/etc/orbit-net/tls/` as `orbit-net`-owned `0600` files from
a deploy hook, then `systemctl reload orbit-net`. A chain that does not cover the
origin host or is not currently valid is refused, and the old certificate stays
in service. Check the journal for `certificate reloaded`.

**Profile epoch or service key.** Devices accept a new epoch only after their
owner reviews it, so serve old and new side by side:

1. Offline: `orbit-net profile sign ... --previous profile-N.json --out profile-N+1.json`
   (with a new `service_key` if rotating it).
2. Set `profile`/`service_key` to epoch N+1 and `overlap_profile`/`overlap_service_key`
   to epoch N in `serve.json`; run `--check`; `systemctl restart orbit-net`.
3. Distribute `profile-N+1.json`. Release: copy it to
   `internal/network/release-profile.json` (the only packaged-profile file; the
   authority is frozen in `network.ReleaseAuthority`) and publish the next package
   version. `make package` refuses a packaged profile that expires within 30 days
   and records its digest in `release-manifest.json`. Updated devices apply it at
   daemon start when operator and privacy text are unchanged; otherwise their
   owners confirm once with `orbit network update`. Self-hosted: send it to device
   owners. Devices on either epoch keep syncing with each other.
4. When `orbit_net_refusals_total{reason="expired"}` stays flat after epoch N
   expires, remove the overlap settings and restart.

A restart drops ephemeral leases; devices reconnect and reannounce within about
two minutes and resume synchronization. New invitations require both devices to
be on the same epoch; a mismatch reports `PROFILE_EPOCH_MISMATCH` naming the
device to update. Start the overlap and package the new epoch well before the
current one expires (epoch 1 expires 2027-01-04).

**Authority key.** There is no in-band authority change. If the authority key is
lost or compromised, sign a new profile under a new authority and ask every device
owner to review it; treat it as a new operator.

## Restart, patching, rollback and shutdown

- **Patch:** install the new binary, run `--check`, `systemctl restart orbit-net`.
  Graceful stop closes controls and relays, then drains HTTP for five seconds.
- **Roll back** the binary or configuration by restoring the previous files, but
  never drop an epoch that devices have already reviewed: devices refuse older
  epochs, and a service that no longer serves theirs strands them (visible as
  `untrusted` refusals). Fix forward with a higher epoch instead.
- **Disable for a device:** the owner switches to Local-only or Manual; keys,
  history and folders stay.
- **Decommission:** announce through the profile contact channel, stop signing new
  epochs, keep the service running until the last epoch expires, then
  `systemctl disable --now orbit-net`. Devices report the service as unavailable
  and keep syncing over LAN and manual routes.

## What to back up

| Item | Back up? |
| --- | --- |
| Authority key | Yes, offline, two copies; losing it means a new operator identity |
| Signed profiles (all epochs) | Yes; needed for overlap and `--previous` |
| `serve.json`, unit overrides | Yes |
| Service key | Optional; replacing it needs a new epoch |
| TLS key and chain | Per your certificate process |
| Directory leases, sessions, rate buckets, relay state | No; memory only, rebuilt by devices after restart |

### Verify an offline authority backup

Keep the second copy on separate offline media or in the owner's selected
encrypted vault. A second file on the same laptop does not establish this custody
condition. Restore the copy into a private directory on the offline machine
(directory mode `0700`, key mode `0600`), then run:

```sh
orbit-net key verify --file /private/restore/authority.key \
  --authority 9af3cf8a979f1b635a56831259d7645a62fb7c19db2de8be51e6afb0ce423b36
```

This checks both the public identity and actual signing capability without
creating a profile, modifying the key or printing private material. For
self-hosting, substitute that operator's public authority. Record the medium or
vault name, custody owner, restoration date and public verification result;
never put the private key, passphrase or decrypted backup in repository evidence.
Remove the temporary restored copy after verification and return the backup to
offline custody. Successful verification alone does not establish that the
media was disconnected or that the owner can unlock the vault.

## Incidents

- **Service key or TLS key exposed:** sign a new epoch with a new service key,
  serve it with overlap only as long as needed, revoke and reissue the certificate,
  then remove the old epoch. Device keys and file contents are not exposed by
  either key.
- **Authority key exposed:** see Rotation; device owners must review a new
  authority.
- **Abuse or overload:** lower budgets in `serve.json` and restart; quotas refuse
  rather than queue. Contact affected device owners through the published contact.

## Self-hosted devices

Give each device owner `profile.json` and, for a private CA, the CA certificate.
On each device (both files owner-only):

```sh
orbit network set --mode self_hosted --profile-file profile.json \
  --service-roots service-ca.pem
# shows the operator and privacy text, asks to replace the packaged operator,
# then "Apply? [Y/n]"; scripts use preview/apply with --review-file
orbit network status    # Profile: verified; service trust custom:<fingerprint>
```

Devices that switch keep their identity, approvals and files. A device switching
back to the hosted operator runs `orbit network automatic`. Per-operator rollback
floors stay recorded, so switching operators never re-admits an older epoch.

The CA is trusted only for that profile's service, never for anything else, and
normal hostname verification and per-device pins still apply. Release profiles
always use the system roots.

## Hosted default readiness (WG6)

Since W14, ordinary builds carry the signed release profile, so every package is
a hosted-default distribution. Publish packages to anyone other than the operator
only once all of the following exist and are recorded in the W13 status entry:
the operator's legal name and contact; the service DNS name and who controls it;
the host, its addresses, egress plan and budget values; the TLS issuance/renewal
process; the authority key's custody with a second offline copy; the signed
release profile and its privacy text; and the monitoring destination with an
on-call contact. Example origins and private test profiles are development-only.
As of 2026-10-07 both are recorded: `orbit-net alert` posts to the owner's ntfy
topic (live receipt confirmed) and the owner holds a Bitwarden copy of the
authority key (restore check waived). Earlier, both were open. The owner deferred them to allow W15 isolated development under
[the sequencing amendment](orbit-wan-implementation-plan.md#owner-directed-sequencing-amendment--2026-10-06).
Both remain required before wider package distribution, W16 hosted-default
acceptance and combined W17 release. Deferral does not establish backup custody
or working notification delivery.
