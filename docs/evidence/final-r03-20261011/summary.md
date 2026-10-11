# R03 summary: one-command self-hosting

Date: 2026-10-11, base `618a886`.

## What shipped

- `orbit-net init --dir DIR --origin https://HOST[:PORT] [--stun IP:PORT]
  [--stun-bind] [--listen] [--operator] [--privacy] [--valid-for]
  [--tls-cert --tls-key] [--config-dir]` writes one owner-only directory
  (0700 dirs, 0600 files) and never overwrites:
  - `offline/authority.key` (to move off the host);
  - `service.key`;
  - a signed `self_hosted` `profile.json` (epoch 1, one year);
  - a self-signed P-256 TLS pair for the host (IP or DNS) unless one is
    supplied;
  - `service-ca.pem`;
  - `serve.json` with personal budgets (4 MiB/s, 2 MiB/s per device, 16 GiB
    per session, 200 GiB/month), loopback metrics and allowlist admission.

  It validates the result the way `serve --check` does and prints the device
  command (`orbit network set --mode self_hosted --profile-file profile.json
  --service-roots service-ca.pem`), the `orbit-net devices add` step and the
  start commands.
- Operator guide: "Run your own Orbit service" short path ahead of the
  reference. README: a self-hosting paragraph.
- `scripts/validation/selfhost_netns.py`: a rootless three-namespace fixture
  (service, A, B behind a forwarding router), with optional A↔B drop and
  `tc netem`. Reused by R05.

## Evidence

| Acceptance item | Result |
| --- | --- |
| `init` → `serve --check` → serve | Unit tests: modes 0600/0700; `--check` refuses until a device is listed, then passes; `profile verify` shows `self_hosted` and the STUN address; DNS origins drop `:443`; 7 bad-input cases refused (`logs/init-tests.log`). Namespace run: `init` in the service namespace, `--check` refused with no device, `devices add` ×2, `--check` passed, service started (`netns-run1.json`) |
| Two disposable devices, direct routes blocked, sync through it | Run 1 (`--block-direct`, A↔B forwarding dropped both ways): both devices `SERVICE_READY`; B joined A's Orbit by code through the service in 38.3 s; A→B small file, then an 8 MiB random file B→A in 7.1 s. Route accounting on both devices: 100% `relay`; service relayed 8.47 MB, `not_admitted` 0 |
| Tamper and permission checks | A changed profile byte and a 0644 service key each fail `--check`; restoring them passes; re-running `init` into the same directory refuses |

Control run 2 (no block): also 100% relay, join 42.7 s, 8 MiB in 7.6 s. With
private addresses the service publishes no direct candidates (the public
directory never exchanges private or LAN candidates), so the drop rules in run
1 counted 0 packets. They guarantee that no other path exists but were not
exercised. Direct-versus-relay behaviour with public-like addressing and NAT is
R05's matrix.

## Limitations

- The fixture runs on one host in namespaces; there is no physical WAN or
  cloud firewall.
- The generated certificate is self-signed for one year. Renewal is manual
  (`--tls-cert/--tls-key` or a new `init`), and a new profile epoch needs the
  offline authority key.
- `init` does not install the systemd unit or open firewall ports; the guide
  lists the ports.
