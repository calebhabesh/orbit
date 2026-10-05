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

Optional measured result: a 1-GiB file tail edit fetched one 1-MiB chunk and
reused 1,023; one full-size sample used 98.31% fewer TLS/TCP stream bytes than
a verified durable full-file baseline. [Measurement details](evidence/release-20261001/measured-results.md)
also record cases where the baseline performed better. Use that figure only
with its workload and sample count; it is not a general speedup.

Personal use and the owner's comprehensive project review follow delivery.
Technical release status and remaining native checks are recorded in the
[terminal release report](evidence/terminal-t13-20261004/summary.md).
