# Member sponsorship on the hosted service (2026-10-11)

Owner direction, 2026-10-11: adding devices to the hosted relay "should just
work out of the box … for all devices in my orbits", without SSH to the VPS.
Code: `8c3dbcf` (feature), `a0837a3` (Orbit 2.5.0, orbit-net 1.3.0).

## Checks

| Check | Result |
| --- | --- |
| `go test -count=1 ./cmd/orbit-net ./internal/rendezvous ./internal/network ./internal/protocol` | pass (rendezvous 300 s, network 118 s) |
| `make quick` | pass (83 s) |
| `TestSponsorshipAdmitsOrbitPeers` | listed sponsors unlisted, transitive sponsorship, repeats write nothing; revoked → `NOT_ADMITTED`, re-pinned operator device → `IDENTITY_MISMATCH`, self → `INVALID_REQUEST`, failed save admits nobody |
| `TestSponsorshipRuntimeJoinNeedsNoOperator` | production runtimes: refused joiner served 5 s after both sides register each other |
| `TestSponsorshipConfigAndDevicesCommand`, `TestSponsorshipReloadKeepsAndPrunes` | config validation, `devices list/remove/add` with revocation, reload keeps and prunes the list, metrics carry no identities |

## VPS deployment (UTC)

| Step | Result |
| --- | --- |
| Pre-check 18:53:05 | 1.2.0 active; 2 controls; allowlist 1; 6,335,236 relay bytes this month |
| Copy | arm64 `orbit-net 1.3.0 (a0837a3, 2026-10-11)`, sha256 `ba6b08ae…5a36aeb` |
| Backups | `/usr/local/bin/orbit-net.bak-1.2.0`, `/usr/local/lib/orbit-net-alert/orbit-net.bak-1.2.0`, `/etc/orbit-net/serve.json.bak-1.2.0` |
| Configure | `serve.json` adds `"sponsorship": true`, `"sponsored_devices_file": "/var/lib/orbit-net/sponsored-devices.json"` |
| `serve --check` | valid; allowlist (1 devices, 0 by member sponsorship) |
| Restart 18:53:06 | one `systemctl restart orbit-net`; active; sponsorship 1; month count preserved |

Not touched: Docker, Caddy, Cloudflare tunnel, linewatchto, iptables, TLS,
profiles, the allowlist file, the alert configuration.

## Owner devices joining with no operator step

`orbit-trial` 2.5.0 (owner build) on PC, laptop and Pi. Only the PC trial is
on the static allowlist. The laptop and Pi each joined the PC's `Synced` Orbit
with a short code, and the PC approved.

| Device | Refused | Approved and sponsored | `SERVICE_READY` |
| --- | --- | --- | --- |
| Laptop | `NOT_ADMITTED` before approval | 18:54:05 (sponsor = PC) | by 18:55:39 (coarse polling) |
| Pi | 18:56:06 | 18:56:09 (sponsor = PC) | 18:56:48 (1 s polling) |

The Pi then reached the PC through the relay (`route=relay; CONNECTED`). A file
written on the PC at 18:57:09 was on the laptop and the Pi by 18:57:16. No SSH
to the VPS and no reload were needed. `orbit-net devices list` on the VPS shows
both devices as sponsored by the PC.

Limitations: on real devices the joiner became ready 40–90 s after
sponsorship, not the 5 s of the unit test. That bounds when the joiner
registers its first peer-data peer during a join, not the service. A shared
Orbit with another person admits their devices too (documented trust scope).

## Rollback

```sh
sudo install -m 0755 /usr/local/bin/orbit-net.bak-1.2.0 /usr/local/bin/orbit-net
sudo install -m 0755 /usr/local/lib/orbit-net-alert/orbit-net.bak-1.2.0 /usr/local/lib/orbit-net-alert/orbit-net
sudo -u orbit-net cp /etc/orbit-net/serve.json.bak-1.2.0 /etc/orbit-net/serve.json
sudo systemctl restart orbit-net
```

The laptop and Pi would then be refused again until added with `devices add`.
