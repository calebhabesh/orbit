# Hosted VPS switch to allowlist admission (R06, owner-approved 2026-10-11)

Owner confirmation: 2026-10-11, in session ("Yes, apply it") for exactly the
listed change. Logs: `logs/vps-0*.log`, `logs/pc-trial-after-switch.log`,
`logs/unlisted-device-against-vps.log`.

| Step (UTC 2026-10-11) | Result |
| --- | --- |
| Pre-check 16:43:13 | 1.1.0 active; 2 records, 2 controls; 6,335,236 relay bytes this month |
| Copy and verify | arm64 `orbit-net 1.2.0 (11b17da, 2026-10-11)`, sha256 `86ef652b…97ca4` |
| Backups | `/usr/local/bin/orbit-net.bak-20261011`, `/usr/local/lib/orbit-net-alert/orbit-net.bak-20261011`, `/etc/orbit-net/serve.json.bak-20261011` (hash-identical to the live files before the change) |
| Install | 1.2.0 at `/usr/local/bin/orbit-net` and the alert copy |
| Configure | `serve.json`: `"admission": "allowlist"`, `"allowed_devices_file": "/etc/orbit-net/allowed-devices.json"`; `relay_month_bytes` kept at 2 TiB. Allowlist: the PC `orbit-trial` device (`350b0105…`, label `CalebPC-orbit-trial`), 0600 orbit-net |
| `serve --check` | valid; admission allowlist (1 devices) |
| Restart 16:43:52 | one `systemctl restart orbit-net`; active; `healthz 200`; build 1.2.0; allowlist 1, 1 device; month count preserved |
| Listed device | PC `orbit-trial`: Automatic, `SERVICE_READY`, operator Caleb Habesh, profile verified |
| Unlisted device | disposable owner-build state: `NOT_ADMITTED` with the action text; service `not_admitted` = 1 |

Not touched: Docker, Caddy, Cloudflare tunnel, linewatchto, iptables, TLS,
profiles, the alert unit configuration.

## Adding a device later

```sh
sudo -u orbit-net orbit-net devices add --config /etc/orbit-net/serve.json --label NAME 'device=<64 hex> pin=<64 hex>'
sudo systemctl reload orbit-net
```

The laptop and Pi trials have no identity yet. After `make trial-fresh`, their
new identities must be added this way. They can pair before that.

## Rollback

```sh
sudo install -m 0755 /usr/local/bin/orbit-net.bak-20261011 /usr/local/bin/orbit-net
sudo install -m 0755 /usr/local/lib/orbit-net-alert/orbit-net.bak-20261011 /usr/local/lib/orbit-net-alert/orbit-net
sudo -u orbit-net cp /etc/orbit-net/serve.json.bak-20261011 /etc/orbit-net/serve.json
sudo systemctl restart orbit-net
```

This restores 1.1.0 with open admission. `allowed-devices.json` can stay; 1.1.0
ignores it.
