## Connectivity (reconnect repetitions)

| Cell | Netem | Runs ok | First sync after restart, s | Direct upgrade within settle | Upgrade after restart, s | Direct share of bytes per rep | Route of settled 256 KiB |
| --- | --- | --- | --- | --- | --- | --- | --- |
| lan | delay 20ms | yes | 4.32 [3.08–11.08] n=10 | 10/10 (quic) | 15.07 [4.69–19.84] n=10 | 0.50 [0.35–1.00] n=10 | quic×10 |
| eim | delay 20ms | yes | 11.07 [4.57–19.58] n=10 | 10/10 (quic) | 72.91 [71.06–80.13] n=10 | 0.16 [0.03–0.27] n=10 | quic×9, relay×2 |
| public_tcp | delay 20ms | yes | 11.83 [3.58–19.09] n=10 | 10/10 (direct) | 67.85 [66.60–84.69] n=10 | 0.18 [0.14–0.27] n=10 | wan_tcp×10 |
| symmetric | delay 20ms | yes | 12.58 [3.57–22.08] n=10 | 0/10 (none) | n=0 | 0.00 [0.00–0.00] n=10 | relay×10 |
| udp_blocked | delay 20ms | yes | 14.33 [3.57–25.58] n=10 | 0/10 (none) | n=0 | 0.00 [0.00–0.00] n=10 | relay×10 |

## Join, resources and doctor

| Cell | Join, s | B idle RSS KiB / fds | B after reps RSS KiB / CPU s / fds | Doctor UDP / direct |
| --- | --- | --- | --- | --- |
| lan | 50.6 | 34720 / 22 | 40584 / 0.44 / 21 | VERIFIED / UNAVAILABLE |
| eim | 40.1 | 34564 / 23 | 40252 / 1.38 / 23 | VERIFIED / UNAVAILABLE |
| public_tcp | 41.4 | 36316 / 23 | 37428 / 1.57 / 23 | UNAVAILABLE / VERIFIED |
| symmetric | 49.7 | 34812 / 22 | 41428 / 5.14 / 22 | VERIFIED / UNAVAILABLE |
| udp_blocked | 43.7 | 34272 / 22 | 41612 / 4.66 / 23 | UNAVAILABLE / UNAVAILABLE |
| lan | 43.7 | 35200 / 22 | 46384 / 27.82 / 19 | None / None |
| eim | 39.1 | None / None | None / None / None | None / None |
| udp_blocked | 41.6 | None / None | None / None / None | None / None |

## Throughput (A → B)

| Cell | Large file MiB/s | Large route | Small files/s | Small route |
| --- | --- | --- | --- | --- |
| lan | 12.61 [12.60–13.97] n=3 | quic | 7.57 [3.68–16.20] n=3 | quic |
| eim | 4.24 [4.24–4.39] n=3 | quic | 1.96 [1.50–2.98] n=3 | quic, relay |
| udp_blocked | 1.62 [1.60–1.64] n=3 | relay | 1.84 [1.55–2.92] n=3 | relay |

## Chunk reuse (lan)

| Change | File bytes | Bytes B received | Ratio |
| --- | --- | --- | --- |
| base | 33554432 | 34066396 | 1.015 |
| tail_append | 34603008 | 1587139 | 0.046 |
| prefix_insert | 34603009 | 35165999 | 1.016 |

## Faults (eim)

- Receiver SIGKILL at 20718395 of 67108864 bytes: resumed and verified in 73.8 s; received after restart 47556503 bytes (remaining at kill 46390469).
- Partition of A with concurrent edits on both sides: heal to identical two-head conflict on both devices in 22.4 s.
- A's WAN address changed mid-transfer (33554432 bytes): completed 84.1 s after the change; routes ['quic', 'relay'].
