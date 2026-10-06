# W13 evidence summary

**Partial, 2026-10-06.** Everything W13 can prove without a real operator passes:
packaged operator binary, key/profile tooling, rotation overlap, certificate
reload, sanitized monitoring, finite budgets, overload and STUN limits,
restart/disable/rollback behavior and self-hosting with production clients and
reviewed custom trust. **WG6 stays open**: no actual operator, DNS name, public
host, publicly trusted certificate, authority custody, signed release profile or
monitoring destination exists, and a mocked operator cannot pass.

Commands and logs: [commands](commands.md). Status entry:
[W13 tracker](../../implementation/wan-status.md#w13--operated-defaults-and-self-hosting).
Operator procedures: [runbook](../../orbit-net-operator.md).

## Acceptance

| Criterion | Evidence | Result |
| --- | --- | --- |
| WG6 actual operator/profile readiness | Missing conditions listed in the [runbook](../../orbit-net-operator.md#hosted-default-readiness-wg6) | **Open** |
| Package/server launch | Packaged amd64 archive in the real-binary rehearsal; arm64 archive on Pi 4B; sandbox under `systemd-run --user`; reproducible archives | Passed (root-only unit properties unexecuted) |
| Certificate expiration/rotation | SIGHUP renewal served new serial; wrong-host chain refused, valid one kept; expired chain refused by `--check`; reload counters | Passed |
| Profile expiration/rotation | Epoch 1→2 with new service key and overlap; devices on 1, mixed 1/2 and 2 all transfer; older epoch refused by device; short epoch 3 expires while 2 serves; expired profile refused by service and device | Passed |
| Overload | 320-socket flood: 256 active, rest refused before TLS; FDs 265, RSS ≈ 22–25 MiB; pre-auth burst refused (untrusted/quota); sync resumes after | Passed |
| STUN abuse limits | 100 requests from one source: 10 answers, 32-byte responses; malformed/oversized dropped; counters | Passed (Pi: same) |
| Restart, revoke/disable, rollback | Graceful SIGTERM; devices reconnect after restart; Local-only closes the device's control; outage reported; rollback to an epoch-1-only config strands epoch-2 devices visibly (`untrusted` refusals), so the runbook requires forward fixes | Passed |
| TLS-inspecting relay cannot read content | `TestWANW04BrokerSeesOnlyInnerTLSCiphertext`, `TestWANW01PinnedTransport`; rehearsal metrics/journal free of IDs, pins, capability, filenames, content | Passed |
| Self-host with production clients, verification on | Without reviewed trust: unready, no control admitted; with `--service-roots`: `custom:<sha256>`, pairing and two-way transfer, no system trust for the CA | Passed |
| Runbook separates user and operator work | [Runbook](../../orbit-net-operator.md) | Done |

## Defect found

Profile rotation stranded every existing pairing: durable routes kept the old
digest (the daemon refused to register them) and peers rejected each other's
proofs across epochs. Routes now follow the active reviewed digest, and clients
accept same-origin adjacent-epoch proofs; a strict-digest reproduction fails the
mixed-epoch transfer.

## Limitations

- Local rehearsals use private addresses and a private CA. They are not hosted
  capacity, egress or availability measurements.
- Clients accept peer proofs under any digest the same-origin service admits;
  the service bounds that to two epochs under one authority.
- Routed enrollment requires both devices on the same epoch.
- Switching a device to a different operator (authority) is still refused by
  profile review; W13 did not change that.
- Inherited T13 technical checks and deferred P17 owner use/explanation are
  unchanged.
