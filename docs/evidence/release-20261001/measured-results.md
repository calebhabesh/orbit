# Measured synthetic workloads — 2026-10-01

Engine and harness: `e13e53ac3b8cfd616a20691fe70d5bb689a2e56c`. All accepted
runs use the [checked package binary](packaged-binaries.json). These are
synthetic workloads, separate from [owner personal use](personal-pilot/handoff.md).

## Method and interpretation

The [generator and full-file baseline](../../../scripts/validation/benchmark.py)
use seed 20261001 for the same contents on both alternatives. The workstation
is an AMD Ryzen 7 9700X running Arch Linux, kernel 7.2.7-arch1-1, on ext4.
[Manifest](manifest.json) records inventories, binary hashes, finite budgets
and exact workload dimensions; [commands](commands.md) records reproduction.

Both alternatives use TLS 1.3 mutual authentication, whole-file SHA-256
verification, file flush and durable publication. The baseline hashes both
trees and skips unchanged files; it retains only working files. File Sync
also retains immutable content, explicit directory metadata, causal history
and publication recovery copies. Tree comparisons check the generated regular
files; these workloads do not compare empty-directory or executable changes.
Four transfer workers are allowed in each alternative. TCP_NODELAY is enabled.

Bytes below are the measured sum of both TLS-bearing TCP stream directions,
including HTTP, TLS records and handshakes. TCP/IP, SSH and VPN headers are
excluded. Positive savings means fewer bytes than the baseline; negative
savings means more. Savings = `100 * (1 - filesync_bytes / baseline_bytes)`.
MiB means 1,048,576 bytes. Times include sender scan/hash and transfer through
durable receipt/publication for File Sync, and source/destination hash plus
durable publication for the baseline. Server/tunnel startup and final tree
comparison are excluded. Times compare different storage/history work.

Each repetition starts with fresh application stores; workloads within a
repetition reuse those stores and accumulate history and recovery copies.
Filesystem caches are warm, without privileged cache drops. Shared workstation
load varies. During the final campaign, workstation space pressure required
[inactive fixture relocation and owner-requested cleanup](validation-fixture-cleanup.json).
This activity overlapped later workloads; their timings are exploratory
shared-host observations, not isolated throughput measurements.
Only prefix insertion uses an 8-MiB/s cap per response TCP stream
and 2-ms delay per read in each direction; these are neither an aggregate
link cap nor calibrated RTT. Other rows are unrestricted.

## 10,000 files and 1-GiB archive

[Raw report](benchmark-release/benchmarks.json): 9/9 successful measurements; 1 sample per completed workload.

Dimensions: 10,000 small files (4–64 KiB), 1,024-MiB archive; the mixed workload adds a 20-MiB object.

The original parent stopped after the successful initial copy.
[Original report](benchmark-release/benchmarks-interrupted-initial.json)
and [original log](benchmark-release.log) are preserved. The
[continuation log](benchmark-release-resume-20261001.log) records
the separate resume command. Disposable markers, package hashes and
source/receiver/baseline digests were checked before continuation.
The pause is excluded from each workload timing; this is one continued
campaign, not a second independent repetition.

| Workload | File Sync stream bytes | Baseline stream bytes | Savings | File Sync seconds | Baseline seconds | Fetched / reused chunks |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Initial small files | 386,343,284 | 351,004,322 | -10.07% | 1202.399 | 37.837 | 10000 / 0 |
| Unchanged tree | 15,924,220 | 1,135,011 | -1303.00% | 207.101 | 1.490 | 0 / 0 |
| Initial archive | 1,093,236,578 | 1,076,324,361 | -1.57% | 253.247 | 2.221 | 1024 / 0 |
| 4-KiB tail overwrite | 18,165,796 | 1,076,324,361 | 98.31% | 194.570 | 2.713 | 1 / 1023 |
| 4-KiB append | 17,120,577 | 1,076,328,479 | 98.41% | 197.620 | 2.701 | 1 / 1024 |
| Mixed objects | 36,987,423 | 22,140,529 | -67.06% | 181.998 | 2.315 | 20 / 0 |
| 1-byte prefix insertion | 37,166,327 | 22,140,552 | -67.87% | 179.722 | 5.823 | 21 / 0 |
| Archive rename | 17,124,348 | 1,076,328,602 | 98.41% | 195.515 | 4.662 | 0 / 1025 |
| Delete prefix file | 17,025,710 | 1,135,129 | -1399.89% | 173.723 | 2.112 | 0 / 0 |

## Three smaller repetitions

[Raw report](benchmark-repeated/benchmarks.json): 27/27 successful measurements; 3 samples per completed workload.

Dimensions: 30 small files (4–64 KiB), 100-MiB archive; the mixed workload adds a 20-MiB object and additional objects of 10 MiB, 100 MiB.

Cells show medians of three independent repetitions. Timing cells also
show the observed min–max range, which is not a statistical tail estimate.

| Workload | File Sync stream bytes | Baseline stream bytes | Savings | File Sync seconds | Baseline seconds | Fetched / reused chunks |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Initial small files | 1,429,780 | 1,243,113 | -15.02% | 1.384 (1.369–7.858) | 0.041 (0.034–0.152) | 30 / 0 |
| Unchanged tree | 105,343 | 8,945 | -1077.67% | 0.310 (0.294–1.661) | 0.006 (0.005–0.006) | 0 / 0 |
| Initial archive | 105,334,462 | 105,013,070 | -0.31% | 1.924 (1.885–7.386) | 0.192 (0.171–0.201) | 100 / 0 |
| 4-KiB tail overwrite | 1,170,226 | 105,013,070 | 98.89% | 2.877 (1.293–3.258) | 0.233 (0.223–0.247) | 1 / 99 |
| 4-KiB append | 125,859 | 105,017,188 | 99.88% | 2.798 (1.308–3.326) | 0.237 (0.226–0.242) | 1 / 100 |
| Mixed objects | 136,945,165 | 136,524,143 | -0.31% | 7.623 (2.689–9.126) | 0.321 (0.292–0.448) | 130 / 0 |
| 1-byte prefix insertion | 21,152,344 | 21,014,695 | -0.66% | 4.007 (2.144–4.150) | 3.458 (3.455–3.469) | 21 / 0 |
| Archive rename | 130,596 | 105,017,528 | 99.88% | 3.583 (1.458–3.610) | 0.379 (0.341–0.487) | 0 / 101 |
| Delete prefix file | 120,802 | 9,279 | -1201.89% | 2.199 (0.590–2.222) | 0.200 (0.197–0.204) | 0 / 0 |

## Actual VPS route

[Raw report](benchmark-vps-route/benchmarks.json): 9/9 successful measurements; 1 sample per completed workload.

Dimensions: 10 small files (4–64 KiB), 16-MiB archive; the mixed workload adds a 20-MiB object and additional objects of 10 MiB, 30 MiB.

Both TLS streams travel through two SSH forwarding channels via the
actual Oracle VPS and back. Sender and receiver storage stay local.
These measurements do not measure remote disk performance; the
[native host campaign](laptop-release-packaged-final/three-host.json)
separately verifies VPS storage and forwarding. One sample per row.

| Workload | File Sync stream bytes | Baseline stream bytes | Savings | File Sync seconds | Baseline seconds | Fetched / reused chunks |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Initial small files | 512,860 | 448,428 | -14.37% | 1.483 | 0.374 | 10 / 0 |
| Unchanged tree | 48,869 | 6,704 | -628.95% | 0.658 | 0.151 | 0 / 0 |
| Initial archive | 16,878,546 | 16,812,173 | -0.39% | 1.301 | 0.648 | 16 / 0 |
| 4-KiB tail overwrite | 1,105,558 | 16,812,173 | 93.42% | 0.890 | 0.631 | 1 / 15 |
| 4-KiB append | 61,190 | 16,816,291 | 99.64% | 0.868 | 0.609 | 1 / 16 |
| Mixed objects | 63,144,399 | 63,023,049 | -0.19% | 3.125 | 1.705 | 60 / 0 |
| 1-byte prefix insertion | 21,091,655 | 21,012,478 | -0.38% | 2.383 | 3.615 | 21 / 0 |
| Archive rename | 65,995 | 16,816,654 | 99.61% | 1.449 | 0.724 | 0 / 17 |
| Delete prefix file | 64,440 | 7,060 | -812.75% | 1.635 | 0.206 | 0 / 0 |

## Receiver resources and accumulated storage

Resource samples cover only the File Sync receiver during sync. Sender
scan CPU/RSS, server/tunnel work and baseline CPU/RSS are unmeasured.
Descriptors and sampled RSS use a 20-ms interval and can miss brief peaks.
The separate process peak RSS comes from OS resource accounting.

| Campaign | Maximum sampled RSS (MiB) | Maximum process peak RSS (MiB) | Maximum sampled FDs | Receiver user + system CPU seconds, range | Sender scan seconds, range |
| --- | ---: | ---: | ---: | ---: | ---: |
| benchmark-release | 43.91 | 43.25 | 16 | 12.310–270.008 | 0.678–204.549 |
| benchmark-repeated | 32.53 | 33.84 | 16 | 0.031–1.138 | 0.035–2.998 |
| benchmark-vps-route | 27.55 | 28.38 | 16 | 0.023–0.492 | 0.036–0.516 |

Storage is apparent regular-file size, not allocated filesystem blocks.
Managed total adds receiver state and the working root, including internal
staging/recovery scratch. It excludes the separately measured baseline tree.
No GC is run between these workloads. The final deletion removes the visible
prefix file while policy-protected history/recovery contents remain.

| Campaign, last completed row | State (MiB) | Working root including scratch (MiB) | Scratch subset (MiB) | Managed total / source payload | Baseline tree (MiB) |
| --- | ---: | ---: | ---: | ---: | ---: |
| benchmark-release, delete | 1425.40 | 4466.60 | 3112.00 | 4.35× | 1354.60 |
| benchmark-repeated, delete | 253.01 | 551.16 | 340.00 | 3.81× | 211.15 |
| benchmark-vps-route, delete | 97.89 | 144.41 | 88.00 | 4.30× | 56.40 |

## Positive results, negative results and remaining limits

Tail overwrites, appends and rename reuse verified chunks. A tail overwrite
still fetches one full 1-MiB chunk for a 4-KiB edit; append fetches the new
4-KiB final chunk. Rename is delete plus create and reuses the archive.
Every successful row checks resulting file-tree hashes against the same
source; the counters alone are not the correctness oracle.

Prefix insertion shifts the fixed-size chunk boundaries: all 21 chunks of
the 20-MiB-plus-one-byte file are fetched again. Initial transfer also needs
all content. Metadata, receipts and TLS overhead can make File Sync send
more stream bytes than the full-file baseline. Unchanged trees and deletion
have especially unfavorable percentage comparisons because the baseline
sends a small manifest and no file payload. The raw byte totals keep that
denominator visible. Lower byte counts do not establish lower wall time.

The full-size run exposes per-object budget traversal, whole-tree hashing
and durable per-file metadata/publication costs. Inventory page admission
and per-path history queries fixed measured regressions; further throughput
improvements are not an accepted guarantee. The repeated smaller workload
has visible timing variation under shared workstation load.

The [native interruption](laptop-release-packaged-final/three-host.json)
recorded five verified chunks, then reused five and fetched seven of twelve.
Partial and resumed TLS/TCP stream counts are 8,711,620 and 7,391,748 bytes.
In-flight bytes can be discarded; this is verified-chunk resume, not
mid-chunk byte-offset resume. No uninterrupted counterpart was measured in
that run, so a retransmission-overhead percentage is not claimed.

Full-size and VPS-route workloads have one sample each; smaller workloads
have three. No p99, universal speedup, cold-cache result or maximum-scale
claim follows. Physical power loss and isolated recovery latency remain
unmeasured. Earlier runs on old binaries, unlimited budgets or Nagle-enabled
proxies remain historical diagnostic evidence and are excluded from these
tables. Owner edits, offline/reconnect, ordinary restart and the required
owner explanation remain unexecuted/unconfirmed P17 gates.
