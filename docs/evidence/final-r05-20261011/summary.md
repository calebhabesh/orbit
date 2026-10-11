# R05 summary: measurement campaign (RG4)

Date: 2026-10-11, base `8ef117d`. RG4 is closed. Tables: `tables.md`; raw
results: `raw/`; drivers: `scripts/validation/measure_netns.py`,
`selfhost_netns.py`, `summarize_r05.py`.

## Method

Production binaries; an `orbit-net init` service; two devices paired once per
run through the service (invite → join → approve). Each cell is one rootless
namespace "internet" (synthetic 11.77.0.0/16, W10/W15 precedent), with homes
behind nftables masquerade, `tc netem delay 20ms` on each home uplink and LAN
advertising on. **Medians with [min–max] and n**. Every repetition and failure
is in `raw/`. The five connectivity cells ran concurrently on one 16-thread
host, as did the three bulk runs, so CPU contention is part of the timing.

## Connectivity (10 repetitions per cell)

Each repetition restarts B's daemon, times the first 256 KiB sync, then keeps
2 MiB transfers flowing for up to 180 s and records whether the route upgrades
to direct.

| Cell | First sync after restart | Direct upgrade | Upgrade after restart | Settled route |
| --- | --- | --- | --- | --- |
| Same LAN, one router | 4.3 s [3.1–11.1] | 10/10 QUIC | 15.1 s [4.7–19.8] | QUIC |
| Two homes, port-preserving NAT | 11.1 s [4.6–19.6] | 10/10 QUIC (hole punch) | 72.9 s [71.1–80.1] | QUIC (relay in 1 rep's settle window as well) |
| Public A, B behind NAT with UDP dropped | 11.8 s [3.6–19.1] | 10/10 direct TCP | 67.9 s [66.6–84.7] | `wan_tcp` |
| Two homes, symmetric NAT (`masquerade random`) | 12.6 s [3.6–22.1] | 0/10 | — | relay |
| Two homes, UDP dropped at B | 14.3 s [3.6–25.6] | 0/10 | — | relay |

Findings:

- The first sync after a restart always went through the relay. Direct paths
  arrived later: about 15 s on a LAN, and about 70 s across NATs, i.e. at the
  next 60 s direct probe plus jitter.
- Symmetric NAT and UDP-blocked cells never went direct, as designed (no
  TURN or port mapping).
- An ICE attempt rides on a live request and is cancelled when that request
  ends. On loopback-speed links with no delay, relayed requests always ended
  first, so ICE never completed until 20 ms of WAN delay was added. This
  dependence is a design limitation now recorded.

## Throughput, chunk reuse, faults (3 repetitions where repeated)

| Measurement | Result |
| --- | --- |
| 64 MiB file, LAN QUIC | 12.6 MiB/s [12.6–14.0] |
| 64 MiB file, NAT QUIC (20 ms each uplink) | 4.24 MiB/s [4.24–4.39] |
| 64 MiB file, relay (`init` budgets: 2 MiB/s per device) | 1.62 MiB/s [1.60–1.64] |
| 300 × 4 KiB files, LAN | 7.6 files/s [3.7–16.2], **slower with each batch** (18.5, 39.6, 81.6 s) as the folder grew |
| 300 × 4 KiB files, NAT / relay | 2.0 [1.5–3.0] / 1.8 [1.6–2.9] files/s, also slowing per batch |
| 1 MiB tail append to a 32 MiB file | B received 1.59 MB (4.6% of the file) |
| One-byte prefix insertion | B received 101.6% of the file (fixed chunks shift; known negative case) |
| Receiver SIGKILL at 20.7 MB of 64 MiB | Restarted, resumed and verified in 73.8 s; refetched 47.6 MB for 46.4 MB remaining (~1.2 MB re-sent) |
| Partition of A, concurrent edits on both sides, heal | Identical two-head conflict on both devices 22.4 s after heal |
| A's WAN address changes mid 32 MiB transfer | Completed 84.1 s after the change, mostly over a re-established QUIC path |
| Resources (B daemon, x86_64) | 34–36 MiB RSS idle, 37–46 MiB after transfers, 19–23 descriptors, 21–24 threads |

## Reused physical-network evidence (cited, not re-run)

From the W16 native runs (packaged builds, home fibre Pi ↔ Oracle VPS, hosted
service; [summary](../wan-w16-20261006/native-hosted/summary.md)):

| Observation | Result (single runs) |
| --- | --- |
| Home NAT → cloud 1:1 NAT, 4 MiB version | direct UDP, 6.1–8.4 s |
| UDP blocked on one side, 1 MB each way | relay, 5.0–7.8 s, zero direct bytes |
| Address change mid-session | first transfer after it 21.5–40.0 s |
| Service restart | relay back in 9.5 s |
| Pi / VPS / laptop daemon RSS | 30–36 MiB; `orbit-net` about 15 MiB |
| Relay framing overhead (E09) | 0.22% over payload |

Chunk reuse on 100 MiB archives (release 2026-10-01): a 4 KiB tail overwrite
fetched 1 of 100 chunks.

## Not measured

- **Phone hotspot / CGNAT:** needs the owner to move the laptop. Not run.
- **School or corporate networks, IPv6-only:** not available.
- **Fresh Pi resource samples:** W16 Pi figures are cited instead.
- **Direct-success rates on real networks:** physical results are single
  runs; the 10-repetition rates are emulated cells only.

## Safety

No flood, fault or rate-limit test touched the VPS; this packet made no VPS
contact at all.
