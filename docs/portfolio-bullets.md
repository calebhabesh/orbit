# Evidence-backed portfolio drafts

These describe the project and its AI-assisted implementation/validation.
Adapt personal contribution claims to work you actually performed and can
explain. They do not certify the unfinished owner-use/learning gates.

- Implemented a Go/SQLite synchronization engine for trusted Linux replicas,
  with causal conflict review, historical restore and VPS forwarding;
  [packaged laptop/Pi/VPS checks](evidence/release-20261001/laptop-release-packaged-final/three-host.json)
  verify matching contents, late-arrival conflicts and qualified receipts.
- Built verified 1-MiB chunk transfer with resume after real receiver
  interruption, and recovery using immutable storage and publication journals;
  [native interruption](evidence/release-20261001/laptop-release-packaged-final/three-host.json)
  and [scoped VM resets](evidence/release-20261001/release-candidate/reset/abrupt-reset.json)
  record the tested boundaries and protected hashes.
- Produced reproducible amd64/arm64 packages and instrumented mutual-TLS
  benchmarks against a durable full-file baseline, preserving positive and
  negative results; [repeat package hashes](evidence/release-20261001/release-candidate/reproduction.json)
  and [raw measurements](evidence/release-20261001/benchmark-repeated/benchmarks.json)
  support workload-specific claims.

Any numeric bullet must name its workload, sample count and TLS/TCP stream
measurement boundary. Full release completion, universal power-loss safety,
generic speedups and owner adoption are not supported claims.
