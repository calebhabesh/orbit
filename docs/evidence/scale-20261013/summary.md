# Scale, churn and demo — 2026-10-13

Owner-directed measurement after the final scope (R00–R06) closed: how Orbit
behaves with more than two or three devices, under sustained faults, plus a
recorded demo. Commands: [commands.md](commands.md). Raw results: `raw/`.
Charts: `charts/`. Drivers: `scripts/validation/scale_netns.py` (new),
`measure_netns.py` (R05), `plot_scale.py` (new) and
`scripts/demo/record_demo.py` (new).

## Method

Production binaries in one rootless user + network namespace per run (no
sudo, no external route), extending the R05 fixture from two devices to N:
an `orbit-net init` service on synthetic 11.77.0.2, two devices per home
behind nftables masquerade (port-preserving), `tc netem delay 20ms` on every
home uplink, LAN advertising on. d00 creates the Orbit and invites and
approves every other device one at a time. Arrival is measured on disk (the
expected SHA-256 at the expected path, polled every 0.25 s).

- **fanout:** 20 repetitions; a random device writes a 64 KiB file; time until
  every other device holds it.
- **concurrent:** every device writes its own 64 KiB file at the same moment;
  time until all N × (N−1) copies exist (3 repetitions).
- **soak (N=10):** 30 minutes, one random event every 1–4 s: new file (4 KiB,
  64 KiB or 1 MiB), edit or delete of the writer's own file, SIGKILL of a
  device with restart 5–30 s later, a home router's WAN link down for 10–45 s,
  or concurrent edits of one shared file on two devices. At most a third of the
  devices are impaired at once; d00 is never impaired. Then everything heals and
  the check requires, on **every** device, exactly the expected tree (every
  live file with its last written hash, no deleted or stray file) and identical
  heads for the shared file — the [conditional convergence claim](../../protocol.md#9-conditional-convergence-claim).

The host is a 16-thread desktop that was also running ordinary desktop
applications; all devices share it, so timings include CPU and disk
contention. Release 2.5.1 (`247216f`) unless marked otherwise.

## Results

### Scale-out (release 2.5.1)

| Devices | Edit reaches all devices, p50 / p95 / max | Per device p50 | Concurrent writers, all-to-all | Daemon RSS (median, max) |
| --- | --- | --- | --- | --- |
| 5 | 6.5 s / 9.8 s / 24.1 s | 4.3 s | 10, 17, 14 s (20 copies) | 41 / 42 MiB |
| 10 | 39.2 s / 223.6 s / 228.0 s | 15.3 s | 91, 157, 342 s (90 copies) | 45 / 56 MiB |
| 16 | stopped: rep 1 took 459 s; rep 3 reached **no** peer in 600 s | — | not reached | — |

`orbit-net` RSS: 21 MiB at N=5, 27 MiB at N=10.

**Membership cap confirmed.** A 20-device run was refused when approving the
17th member: `membership exceeds v1 limits` (`protocol.MaxActiveMembers = 16`).
The bound is a protocol v1 design limit and is enforced.

**Finding: replication follows the invitation graph, so one inviter becomes
a hub.** Membership is shared by all members, but a joined device pins only
its inviter's certificate (one `enrolled-*.crt` each) and syncs only with it:
at N=16, d03 and d09 had sync tasks for d00 alone, while d00 had tasks for all
15 peers ([raw](raw/n16-sync-peers.json)). With d00 inviting everyone, every
edit travels spoke → d00 → spokes, the documented hub forwarding
([architecture](../../orbit-architecture.md)), and d00 alone must reach N−1
peers. Most of those pairs were relayed (direct paths across NAT upgrade only
after about 70 s of live traffic, R05), and each relayed exchange needs signed
signalling from the hub's single per-device bucket on the service (burst 10,
1 signed operation/s, at most two outstanding challenges).

- N=5: an edit reaches everyone within one or two 5 s anti-entropy polls.
- N=10: median 6× higher, tail almost four minutes. The service refused **292
  requests as `quota`** (HTTP 429) and **2,611 as `other`** (unavailable
  target, stale generation or replay; not separated by the service) while
  relaying 1.3 GB, far below any byte budget.
- N=16: d00's sync tasks toward its peers ended `exhausted` with
  `QUOTA_EXCEEDED`; the first edit needed 459 s to reach all devices and an
  edit written on d08 reached no device in 600 s, which stopped the run
  ([arrivals](raw/n16-fanout-arrivals.json)).

The rate limits are abuse protection for a personal service and were not
changed. Fixes worth considering, in order: distribute every member's
certificate and route with membership so peers form a mesh (or let devices
invite in a tree), keep relay sessions alive between polls instead of
re-signalling, and budget signalling per peer pair rather than per device.

**Joins** take 70–80 s each (median; up to 483 s at N=16) and grow with the group: 30–36 s is the
joiner's approval poll (`approvalPollInterval`) and about 25 s the service's
two-token admission refill for a second concurrent joiner. Both are deliberate
rate limits.

### Churn soak (N=10, 30 minutes, release 2.5.1)

408 writes, 115 edits, 26 deletes, **53 SIGKILLs**, **35 home internet
outages** and 38 concurrent edits of one shared file. After healing, **all 10
devices held exactly the expected 382 files with identical hashes**, no
deleted file reappeared, and the shared file showed the **same 10 preserved
concurrent heads on every device** (no silent last-writer-wins). Convergence
after healing took **1,128 s**; during churn a write reached every device in
349 s median (p95 1,293 s), for the throttling reason above. Safety held under
heavy churn; liveness at this size is slow.

### Batched receipts (fix found by this campaign)

Every sync round re-asserts a durable receipt for every ready version, which
repairs lost receipts. Release 2.5.1 sent **one request per version**, so an
idle round cost O(versions) round trips. R05 had recorded the symptom (300
small-file batches slowing 18.5 → 39.6 → 81.6 s). The fix sends receipts in
batches of 128 (the server already accepted 1..128; no wire change), while a
newly fetched version is still acknowledged immediately.

| Idle round, 20 ms per request (unit fixture) | 300 versions | 600 | 900 |
| --- | --- | --- | --- |
| 2.5.1 | 6.38 s, 307 requests | 12.70 s, 611 | 19.14 s, 917 |
| batched | 0.28 s, 10 requests | 0.48 s, 16 | 0.72 s, 25 |

End to end (R05 driver, NAT cell over QUIC, one run per build), three
successive 300 × 4 KiB batches: 2.5.1 **41.5 → 72.5 → 82.5 s**; batched
**41.5 → 49.0 → 47.5 s** (third batch 1.74× faster; no growth with folder
size). `TestSyncerBatchesReceiptReassertion` guards the request count.

### Demo

[`docs/assets/orbit-demo.mp4`](../../assets/orbit-demo.mp4) (2 min 17 s,
captions in `orbit-demo.vtt`): three devices in two NAT homes with a
self-hosted relay, recorded from the real terminal UI. A new file reaches
both peers in 4.0 s; the laptop's home loses internet while both sides edit
the same file; 28 s after the link returns all three devices show the
conflict; the desktop reviews and selects a version (3.4 s to reach every
device); the Pi daemon is SIGKILLed during an 8 MiB write and catches up 8.5 s
after restart with matching hashes. Built from the batched-receipt tree
(`2.5.1+receipts`). All links in the recording stayed on the relay. The desktop
and Pi share a home but were both invited by the laptop, so they sync only
through it (the hub finding above).

## Limits

- One host, emulated networks; single campaign per size (N=5, 10, 16) and one
  soak. The small-file comparison is one run per build. Every run used a single
  inviter (star); chained or tree invitations were not measured.
- Scale and soak runs used release 2.5.1, so they do not include the receipt
  batching; its effect at N=10 was not measured.
- The `other` refusal class is not broken down by the service.
- Physical networks, CGNAT, IPv6-only and more than 16 members were not tested
  (the last is unsupported by protocol v1).
