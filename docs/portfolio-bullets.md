# Evidence-backed portfolio drafts

These drafts describe the implemented project and recorded validation.

- Built Orbit, a Go/SQLite file sync daemon with a keyboard TUI and CLI,
  causal conflict review, historical restore and VPS forwarding;
  [packaged laptop/Pi/VPS checks](evidence/terminal-t13-20261004/native-engine-final-candidate/three-host.json)
  verify matching contents, late-arrival conflicts and qualified receipts;
  [native keyboard campaigns](evidence/terminal-t13-20261004/native-hosts-final-candidate-03/terminal-hosts.json)
  exercise onboarding, conflict editing, restore and service capture.
- Built verified 1-MiB chunk transfer with resume after real receiver
  interruption, and recovery using immutable storage and publication journals;
  [native interruption](evidence/terminal-t13-20261004/native-engine-final-candidate/three-host.json),
  [16 scoped VM reset cases](evidence/terminal-t13-20261004/reproduction-final-candidate/clean-release/reset/abrupt-reset.json)
  and [five storage-failure cases](evidence/terminal-t13-20261004/reproduction-final-candidate/clean-release/disk-full/abrupt-reset.json)
  record the tested boundaries and protected hashes.
- Produced reproducible amd64/arm64 packages and instrumented mutual-TLS
  benchmarks against a durable full-file baseline, preserving positive and
  negative results; [repeat package hashes](evidence/terminal-t13-20261004/reproduction-final-candidate/clean-release/reproduction.json)
  and [raw measurements](evidence/release-20261001/benchmark-repeated/benchmarks.json)
  support workload-specific claims.
- Added native cross-network connectivity with no VPN: signed rendezvous
  leases, QUIC over ICE/STUN direct paths, and an encrypted WebSocket relay
  fallback that carries the same pinned device TLS. Deployed the operated
  service with a signed release profile and alerting (since 2026-10-11 it
  serves only the author's devices). Between a home network
  and an Oracle VPS, a 4 MiB version arrived over direct UDP in 6–9 s, and
  with UDP blocked, 1 MB went over the relay in 5–8 s each way with zero
  direct bytes ([native runs](evidence/wan-w16-20261006/native-hosted/summary.md)).
- Turned the service into a private, zero-cost deployment and measured the
  system: a device allowlist enforced after proof verification but before any
  allocation, with a pairing-only path for unlisted devices; per-route
  accounting of every peer byte (LAN, WAN TCP, QUIC, relay); one-command
  self-hosting (`orbit-net init`); and a 50-repetition network matrix in
  rootless namespaces. Direct upgrade was 10/10 through LAN, port-preserving
  NAT and public TCP, and 0/10 through symmetric NAT or blocked UDP
  ([R05 record](evidence/final-r05-20261011/summary.md),
  [allowlist](evidence/final-r02-20261011/summary.md)).
- Stress-tested the system at scale with a new N-device namespace harness: a
  30-minute 10-device churn soak (53 daemon SIGKILLs, 35 home outages, 38
  concurrent edits) converged to identical files and identical preserved
  conflict versions on every device. The same campaign located the scaling
  limit (replication follows the invitation graph, so a single inviter becomes
  a rate-limited relay hub; fanout p50 6.5 s at 5 devices, 39 s at 10, stalled
  at 16) and a sync-round cost that grew with folder size; batching durable
  receipts made an idle round 26× cheaper and stopped 300-file batches slowing
  down (41 → 73 → 83 s before, 41 → 49 → 48 s after)
  ([scale record](evidence/scale-20261013/summary.md)).
- Found and fixed defects that only showed up natively: relay recovery after
  losing UDP mid-session, which previously never recovered in 180 s and now
  takes about 5.5 s
  ([fix evidence](evidence/wan-w16-20261006/quota-fix/summary.md)); profile
  rotation stranding pairings; and a packaged user service that could not be
  enabled. A disposable-VM login/logout/reboot drill confirmed the last fix
  ([W17 record](evidence/wan-w17-20261007/summary.md)).

The relay forwards ciphertext between online devices. The VPS *replica* is a
separate Orbit device that stores and forwards versions. Describe them as two
roles. Single-run timings come from one home network and one cloud region;
school, corporate, CGNAT and IPv6-only networks were not tested.

Optional measured result: a 1-GiB file tail edit fetched one 1-MiB chunk and
reused 1,023; one full-size sample used 98.31% fewer TLS/TCP stream bytes than
a verified durable full-file baseline. [Measurement details](evidence/release-20261001/measured-results.md)
also record cases where the baseline performed better. Use that figure only
with its workload and sample count; it is not a general speedup.

These drafts claim technical results only, not personal adoption.
Technical release status and remaining native checks are recorded in the
[terminal release report](evidence/terminal-t13-20261004/summary.md) and the
[combined W17 release record](evidence/wan-w17-20261007/summary.md).
