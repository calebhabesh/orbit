# Replication packets

Read [protocol](../protocol.md), [persistence](../persistence.md), and [verification](../verification.md). Work through the shared control operations rather than adding independent CLI logic.

## P05 — Peer identity, pairing and bounded wire layer

**Depends on:** P02, P03. **Requirements:** S03–S05, S19, S20. **Invariants:** I01, I09, I20.

Implement key-backed identity, pinned mTLS, explicit out-of-band pairing, per-folder authorization and compatible-membership handshake. Freeze request/response schemas and golden fixtures, including decimal integer strings, duplicate-key rejection, byte limits, version errors and stable error codes. Implement snapshot-bound inventory pagination and immutable envelope fetch. Both sides can initiate sessions; reconnect does not generate new identities.

Keep chunk authorization tied to a manifest in the requested folder. Introduce listeners with bounded headers/bodies, deadlines and cancellation. No custom TLS validation bypass, unauthenticated enrollment endpoint or global digest oracle.

**Acceptance:** paired peers exchange bounded metadata; unpaired/wrong-key/revoked/wrong-folder requests fail; protocol/membership mismatch is explicit; snapshot expiration restarts without skipping permanent state; maximum valid and over-limit inputs behave predictably; malformed requests cannot trigger unbounded allocations. Real TLS integration tests exercise the listener, not only mocks.

**Evidence:** protocol fixtures and authentication/authorization matrix. **Explain:** why a trusted connection still requires per-request folder authorization.

## P06 — First two-peer verified CLI transfer

**Depends on:** P04, P05. **Requirements:** S10, S11, S16. **Invariants:** I01, I05–I07, I09, I17.

Implement missing-chunk discovery, authorized streaming fetch, content pins, bounded transfer workers and persisted verified progress. Connect capture → inventory → fetch → durable readiness → publication through two actual agents. Include direct durable receipt and per-device status. Add explicit scan/sync/status CLI commands; automatic background scans wait for P12.

Retry transient failures using stable version/chunk identities. After restart, reuse verified chunks and discard/retry incomplete chunks. Whole-file verification precedes publication. Receive metadata and content without implying the working path was applied.

**Acceptance:** empty, small, multi-chunk and repeated-chunk files arrive byte-for-byte; corruption rejected; interrupted transfers resume without retransmitting completed verified chunks; lost receipts are replay-safe; peer/status differentiates stored from applied. Two-agent local demo runs with isolated roots and real TLS.

**Evidence:** reproducible first vertical slice and transfer/restart results. **Explain:** what happens if the sender disappears after the receiver stored all chunks but before it applied the file.

## P07 — Bidirectional reconciliation and conflict projection

**Depends on:** P06. **Requirements:** S07–S09, S13. **Invariants:** I01–I04, I11, I12, I17.

Connect production history decisions to durable repository and workspace transitions. Exchange versions in either direction, preserve incomparable content, handle pending content/ancestry, detect executable-only changes, and propagate tombstones without timestamp winners. Implement directory projection and protected fallback content while remote heads are incomplete.

Expose conflict sets separately from displayed working bytes. Reconciliation itself does not manufacture authored events. Receipt of unreviewed versions never advances an ordinary editor's working basis to include them.

**Acceptance:** every offline edit/edit, edit/delete, repeated delete, equal-byte conflict and structural scenario matches the independent model across delivery orders and restarts. No repeated scans or duplicated messages create extra versions. Unsupported paths remain blocked with histories preserved.

**Evidence:** model-vs-process scenario traces and protected content hashes. **Explain:** why joining every known vector into a local edit can hide a conflict.

## P08 — Reviewed resolution, restore and safe control replay

**Depends on:** P07. **Requirements:** S08, S16, S18. **Invariants:** I03, I04, I16, I19.

Implement select, keep-copies, export/manual-merge and historical restore as shared control operations with explicit reviewed heads, stale-view tokens and idempotency records. Persist recoverable steps for keep-copies and bulk operations. Restore chooses old bytes but extends reviewed current causal state. Expose pending, unavailable and expired content accurately.

Test response loss after commit, repeated commands, changed arguments with reused keys, partial keep-copies completion and concurrent incoming versions. A browser/CLI should not silently refresh a stale token and retry a destructive choice.

**Acceptance:** replay creates one logical resolution/restore; unseen version after resolution remains concurrent; pre-commit changed heads produce stale response; destination name collisions preserve existing files; partial multi-path operations resume without duplicate copies; restored executable status follows the selected historical version.

**Evidence:** command examples and replay/concurrency tests. **Explain:** why restoring yesterday's bytes must not restore yesterday's causal vector.

## P09 — Three-peer forwarding and membership lifecycle

**Depends on:** P08, D3 outcome. **Requirements:** S02–S05. **Invariants:** I02, I08, I14, I15.

Implement propagation of third-party authored histories/content through an ordinary replica. Add enrollment preview/bootstrap, identical membership revision distribution, retirement snapshot artifact and explicit new-identity recovery. Keep old accepted ancestry distinct from newly rejected retired-origin events. Terminate active access when a new revocation configuration takes effect; configuration mismatch blocks folder exchange.

Enrollment captures existing local files independently before application; it never infers deletion from empty bootstrap state. Retirement maintenance requires all survivors' metadata as specified, or explicit retirement of additional unavailable members. Persist resumable maintenance state.

**Acceptance:** A→B→C works without A/C overlap or direct connectivity; all original authors preserved; three offline edits/resolution-late-arrival agree with model; direct versus indirect progress is labeled; stale old configuration, retired device reconnect, unknown retired ancestry and metadata-loss reinstall fail safely. Divergent existing-folder enrollment is non-destructive.

**Evidence:** three-process topology scripts plus membership/recovery runbook. Actual three-host evidence follows in P17. **Explain:** why peer retirement changes cleanup assumptions and why this maintenance protocol is not consensus.
