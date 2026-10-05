# Verification of Syncthing WAN discovery claims

Checked on 2026-10-05 against official documentation and upstream source revision
[`4461a1ce354d3990660966ad70176e00246adb61`](https://github.com/syncthing/syncthing/tree/4461a1ce354d3990660966ad70176e00246adb61).
This is documentation/source inspection, not a school-to-home connectivity test
or a proposal to amend Orbit's approved scope.

## Verdict

The explanation of Syncthing's identity, discovery, direct connections and relay
fallback is substantially correct. Devices can be paired over the internet without
ever sharing a LAN. The implementation-size figures are reproducible nonblank-line
counts. The complexity ratings are subjective, the connection diagram simplifies
the implementation, and discovery does not establish that an endpoint is reachable.
The proposed Orbit roadmap differs from this repository's approved scope.

## Verified behavior and qualifications

- **No LAN bootstrap requirement.** The ordinary setup configures each device with
  the other's Device ID and shares the intended folders. `dynamic` addresses enable
  local/global discovery. Pairing authorizes the other device; it does not require
  physical proximity or a shared subnet. These mechanisms support initial WAN
  pairing, subject to available network connectivity. Sources:
  [getting started](https://docs.syncthing.net/intro/getting-started.html) and
  [device addresses](https://docs.syncthing.net/users/config.html#device-element).
- **Discovery locates candidate connection addresses.** Global discovery uses
  HTTPS POST announcements and GET lookups. Addresses can include direct or relay
  endpoints. Announcements identify the sender through its client certificate,
  rather than accepting a `device_id` field as proof of identity. Unspecified IPs
  are replaced with the request's source IP; explicitly supplied IPs are not all
  replaced. Sources: [global discovery specification](https://docs.syncthing.net/specs/globaldisco-v3.html)
  and [server address handling](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/cmd/stdiscosrv/apisrv.go#L439-L508).
- **Observed IP is insufficient for direct reachability.** Engineering inference:
  the source IP of an HTTPS announcement does not establish an inbound mapping for
  the separate sync listener, its external port, or firewall permission. A simple
  IP-plus-port registry can locate an already reachable listener, but does not by
  itself solve NAT. The protocol returns addresses, not a connectivity guarantee;
  Syncthing documents port forwarding and relay alternatives. Sources:
  [global discovery specification](https://docs.syncthing.net/specs/globaldisco-v3.html)
  and [firewall setup](https://docs.syncthing.net/users/firewall.html).
- **Direct TCP and QUIC, plus NAT assistance, are real.** Syncthing supports TCP
  and QUIC listeners, UPnP/NAT-PMP mappings and hole punching. QUIC's listener
  shares its UDP transport with STUN and the dialing registry. STUN supplies
  external-address/NAT information; it does not guarantee successful traversal.
  The inspected STUN service explicitly skips NAT types it considers unpunchable,
  including `NATSymmetric`. Sources:
  [configuration](https://docs.syncthing.net/users/config.html#listen-addresses),
  [NAT tuning](https://docs.syncthing.net/users/tuning.html#tuning-for-lan-only),
  [QUIC listener](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/connections/quic_listen.go#L104-L113),
  and [STUN classification](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/stun/stun.go#L176-L184),
  [punchable types](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/stun/stun.go#L271-L272).
- **Relay fallback preserves end-to-end encryption.** Relaying defaults to enabled,
  direct connections are preferred, and Syncthing retries direct connectivity while
  relayed. Relay operators can observe device IDs, IP addresses and traffic volume,
  but cannot inspect synchronized file contents. A network that blocks all usable
  direct/relay routes can still prevent synchronization. Sources:
  [relaying](https://docs.syncthing.net/users/relaying.html) and
  [firewall setup](https://docs.syncthing.net/users/firewall.html).
- **The LAN-first lookup diagram is conceptual.** The discovery manager aggregates
  results from enabled finders and their caches; it does not stop after a successful
  local result. Outgoing dialing groups candidates by priority and tries multiple
  targets concurrently within each group. Default priorities prefer TCP LAN,
  QUIC LAN, TCP WAN, QUIC WAN, then relay. Thus local/direct preference is accurate;
  a strict local-lookup-fails-then-global-lookup rule is not. Sources:
  [discovery manager](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/discover/manager.go#L113-L165),
  [dialing](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/connections/service.go#L1100-L1164),
  and [default priorities](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/config/optionsconfiguration.go#L77-L81).
- **Announcement intervals need qualification.** Local discovery normally broadcasts
  on IPv4 and multicasts on IPv6 every 30 seconds. Global discovery's client fallback
  is 30 minutes, but it honors `Reannounce-After`; the inspected bundled server
  returns a randomized interval from 2,400 through 4,199 seconds (40 to just under
  70 minutes). This establishes source behavior, not the running configuration of
  every public server. Sources:
  [security documentation](https://docs.syncthing.net/users/security.html),
  [client fallback](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/discover/global.go#L52-L57),
  [header handling](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/discover/global.go#L311-L321),
  [server constants](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/cmd/stdiscosrv/main.go#L33-L41),
  and [server header generation](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/cmd/stdiscosrv/apisrv.go#L538-L539).

The getting-started page's example logs identify Syncthing v1.7.1 from July 2020.
They illustrate the mechanism but should not be described as current runtime
evidence. Current source independently confirms QUIC/STUN support.

## Reproduced source-size figures

At the pinned upstream revision, all five quoted figures match **nonblank lines**,
including comments and imports. They are not physical file lengths or the complete
size of the subsystems: helpers, dependencies and tests add implementation work.

| File | Quoted / reproduced nonblank lines | Physical lines |
| --- | ---: | ---: |
| [Local discovery client](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/discover/local.go) | 301 | 358 |
| [Global discovery client](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/discover/global.go) | 412 | 487 |
| [Discovery API server](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/cmd/stdiscosrv/apisrv.go) | 497 | 590 |
| [Connection service](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/connections/service.go) | 1,258 | 1,438 |
| [NAT service](https://github.com/syncthing/syncthing/blob/4461a1ce354d3990660966ad70176e00246adb61/lib/nat/service.go) | 357 | 416 |

Actual verification used the downloaded files in a disposable temporary source
directory and this Python calculation for each path:

```python
lines = (source_root / name).read_text().splitlines()
physical = len(lines)
nonblank = sum(bool(line.strip()) for line in lines)
```

The result was respectively `358/301`, `487/412`, `590/497`, `1438/1258` and
`416/357` (physical/nonblank). No Syncthing execution or NAT success-rate experiment
was performed. The proposed 2/10, 3–4/10, 5/10 and 7–9/10 difficulty ratings are
author judgments; line counts do not validate them. A small discovery prototype
is plausibly easier than reliable traversal across diverse networks, but this
inspection does not establish an implementation schedule.

## Implications for Orbit

The proposed discovery/NAT/relay roadmap is a possible different product scope,
not this repository's current plan. Approved U06 uses an existing reachable LAN or
private network, documents Tailscale for cross-network use, and excludes Orbit's
own discovery/NAT/relay infrastructure. S03 uses explicit addresses and pairing;
S19 requires authenticated peers. Sources: [approved scope](../portfolio-scope.md)
and [network runbook](../runbooks/private-network.md).

Engineering judgment: identity, authenticated registration and authenticated,
encrypted peer transfer should be foundations of any future internet-facing
experiment. Deferring them until V5 is a poor development sequence. An unverified
`device_id` supplied to the example registrar would allow registration spoofing;
Syncthing instead binds announcements and sync sessions to certificate identity.
Sources: [global discovery authentication](https://docs.syncthing.net/specs/globaldisco-v3.html#authentication)
and [peer security](https://docs.syncthing.net/users/security.html).

A Syncthing transport relay forwards opaque encrypted traffic. Orbit's optional
VPS is a trusted full replica that stores and forwards authorized content/history;
it can read that folder's plaintext. These are different roles. Sources:
[Syncthing relaying](https://docs.syncthing.net/users/relaying.html#security),
[Orbit scope U02/U03/S19](../portfolio-scope.md) and
[Orbit network runbook](../runbooks/private-network.md).

No implementation, approved specification or packet status was changed during this
verification. No Orbit runtime checks were necessary for this research note.
