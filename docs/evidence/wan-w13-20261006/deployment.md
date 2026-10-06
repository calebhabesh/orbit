# W13 hosted deployment record — 2026-10-06

Owner-authorized deployment of the release connection service. No fault, flood
or STUN-abuse testing was run against this host; those stay local/Pi.

| Item | Value |
| --- | --- |
| Operator | Caleb Habesh, calebhabesh@gmail.com |
| Host | Oracle Cloud ca-toronto-1, Ubuntu 24.04 arm64 (shared with linewatchto and the portfolio; isolated user/unit) |
| Origin | `https://connect.calebhabesh.com:8443` (Cloudflare DNS-only A record → 132.145.111.200) |
| STUN | `132.145.111.200:3478`, bound as `10.0.0.187:3478` through OCI 1:1 NAT (`stun_bind`) |
| Profile | Release epoch 1, digest `356f0ced2c898e7e6bcdd4913713c797737809ffa27a54e40362168f577ec165`, expires 2027-01-04T19:37:37Z |
| Authority | Public key `9af3cf8a979f1b635a56831259d7645a62fb7c19db2de8be51e6afb0ce423b36`; private key only on the owner's laptop (`~/.config/orbit-operator`, 0600 in 0700) |
| Service key | Public `2f630e69a4a337df26e12d2cf30d2e3f7929f2a9228d8ed28e5d2e1119b5b097`; private in `/etc/orbit-net/service.key` (orbit-net, 0600) |
| TLS | Let's Encrypt (YE1) via lego v5.5.2 DNS-01, Cloudflare token scoped to calebhabesh.com with IP filter; expires 2027-01-04; daily `orbit-net-cert.timer`, deploy hook reloads the service |
| Budgets | relay 4 MiB/s aggregate, 2 MiB/s per device, 16 GiB per session; host egress before deployment ≈ 18 GB/month of the 10 TB OCI allowance |
| Firewall | OCI security list ingress TCP 8443, UDP 3478; host iptables two commented ACCEPT rules before REJECT in live chain and `/etc/iptables/rules.v4` (backup `rules.v4.bak-orbit-net-20261006`) |
| Unit | Hardened `orbit-net.service` as user `orbit-net` (uid 987), CapEff `0x400` (CAP_NET_BIND_SERVICE only), `--check` before start |

## Checks executed

| Check | Result |
| --- | --- |
| `orbit-net serve --check` (as orbit-net) | Valid, release epoch 1 |
| Service start under the real system unit | Active; 8 FDs, 8.6 MiB RSS; listeners 10.0.0.187:8443/tcp, :3478/udp, 127.0.0.1:9464 |
| Public TLS from the owner's laptop with system trust | TLS 1.3, verify 0, strict JSON `INVALID_REQUEST` for a bad body |
| Public STUN from the laptop | 32-byte response mapping the laptop's public address |
| `orbit-net-cert.service` | Renewal correctly skipped (not due) |
| Deploy hook | `certificate reloaded` without restart |
| Live pairing: laptop + Pi 4B, disposable states, release profile in Automatic mode | Both `SERVICE_READY` with system trust; invitation, verification code, approval; laptop→Pi in ≈23 s, Pi→laptop in ≈7 s; route `relay CONNECTED`; 58,393 relay bytes |
| Journal privacy | 7 lifecycle lines; no client address, device ID, folder or filename |
| Cleanup | Disposable states and daemons removed on laptop and Pi; service left running |

A first real-bind attempt was not made with the original binary: review found
that STUN could not bind the public address behind OCI NAT, so `stun_bind` was
added (with tests and runbook text) before the first start.

## Still open for WG6

- **Authority key backup:** the only copy is on the laptop. The runbook requires
  two offline copies (for example an encrypted USB drive and a password manager).
- **Alerting destination:** metrics and `/healthz` exist on loopback only; no
  scraper, alert rule or on-call notification is configured yet.
- **Profile distribution:** devices currently receive the profile file manually;
  bundling it as the packaged default is W14.
