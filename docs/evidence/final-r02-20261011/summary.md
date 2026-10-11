# R02 summary: service admission allowlist (RG1)

Date: 2026-10-11, base `86678f2`. RG1 is closed with the design recorded in
`docs/implementation/final-gates.md`.

## What shipped

- `internal/rendezvous/admission.go`: `Admission` (open or a list of at most 64
  (device, pin) pairs), `SetAdmission` for reload (it withdraws a removed
  device's records, controls and sessions at once), and per-actor accounting
  for the unlisted share.
- Check point: `admitOrigin`, after proof verification and challenge
  consumption, before rate buckets or any allocation. Unlisted devices may use
  only the enrollment purpose toward a listed device: announce, authenticate,
  lookup, offer, accept, reserve, release and attach, plus mailbox claims.
  Mailbox creation is listed-only. Unlisted shares: 32/128 challenges, 16
  controls, 16 records.
- `NOT_ADMITTED` (HTTP 403, not retryable). A refused control gets one typed
  failure frame.
- Client: `ServiceClient.Control` maps that frame. `RelayRuntime` pauses only
  the refused purpose for 10 minutes. Network status reports `NOT_ADMITTED`
  with an action.
- `orbit-net`: `admission`, `allowed_devices`, `allowed_devices_file` in
  `serve.json`, validated by `--check`. SIGHUP re-reads them, keeping the old
  policy on failure. `orbit-net devices list|add|remove` edits the file
  atomically (0600) from the `device=… pin=…` identity line. New metrics:
  `orbit_net_admission_allowlist`, `_allowed_devices`, `_reloads_total`,
  `_reload_failures_total`, `refusals_total{reason="not_admitted"}`.
- Example `packaging/orbit-net/allowed-devices.example.json`, shipped in the
  operator archive; operator guide "Admission" section; protocol and UX notes.

## Evidence

| Acceptance item | Result |
| --- | --- |
| Listed/unlisted on every covered operation | `TestFinalR02AllowlistCoversEveryOperation`: data announce, lookup, control, offer and reserve refused for unlisted; enrollment announce, lookup and control, offer → accept → reserve toward listed admitted; lookup toward unlisted refused; mailbox create refused for unlisted, claim admitted; refused work leaves no rate bucket or record; 8 `not_admitted` refusals counted |
| Unlisted share | `TestFinalR02UnlistedShareCannotExhaustListed`: 33rd unlisted challenge and 17th unlisted record refused `QUOTA_EXCEEDED` while the listed device is still served |
| Enrollment path for a new device | `TestFinalR02UnlistedJoinerEnrollsThroughAllowlist` (production control and routed v3 enrollment through the relay): an unlisted joiner requests, is approved and waits resumably (`SERVICE_UNAVAILABLE`, retryable; status `NOT_ADMITTED`). After listing and a restart it bootstraps, and bytes, heads and hashes match both ways |
| Runtime back-off | `TestFinalR02UnlistedRuntimeBacksOffAndKeepsPairingPath`: 1 refusal, then 0 in 30 s; enrollment record and control stay live |
| SIGHUP reload | `TestFinalR02ReloadWithdrawsRemovedDevice` (in-process) and `TestFinalR02BinarySIGHUPReloadsAllowlist` (real binary: 1 → 2 devices; a malformed reload keeps 2 and counts 1 failure) |
| Malformed list fails `--check` | `TestFinalR02ServeCheckValidatesAdmission`: 12 malformed or ambiguous configurations refused |
| `devices` command | `TestFinalR02DevicesCommandEditsAllowlist`: identity line and bare pair accepted, duplicates and malformed lines refused, 0600 file, last device not removable under allowlist |
| 2.3 client behaviour | `TestFinalR02OldClientAgainstAllowlist` with a client built from `7be6a77` (2.3) and from the R02 tree against a real allowlisted `orbit-net`, 2 minutes each: **2.3: 17 refusals (8.5/min)**, **current: 1 (then quiet for 10 min)**; neither became ready; no other refusal reasons (`logs/old-client.log`) |

## Limitations

- 2.3 clients holding the hosted profile keep retrying their data control about
  every 8 s until epoch 2 expires (2027-10-07). Each attempt is a TLS handshake
  plus one signed request, refused before allocation.
- A new device cannot bootstrap through the service until the operator lists
  it. Pairing and approval work before that; LAN works regardless.
- Removing a device revokes its service access, not its folder membership; use
  Orbit's device removal for that.
- The hosted VPS still runs 1.1.0 with open admission; switching it is R06,
  with owner confirmation.
