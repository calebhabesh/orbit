# P06 two-peer transfer evidence summary

P06 is complete locally under the recorded loopback-process test environment.
Capture → inventory → fetch → durable readiness → durable receipt → publication
is fully connected across two actual agents communicating over mutual TLS 1.3
with pinned key certificates.

## Acceptance criteria verified

- **Content fidelity across sizes**: empty files (0 bytes), small single-chunk
  files, and multi-chunk files arrive byte-for-byte in the receiver workspace.
- **Deduplication and repeated chunks**: identical chunks within a file or across
  transfers are reused; duplicate positions are not retransmitted across the
  network.
- **Corruption rejection**: corrupted chunk payloads (mismatched length, header,
  or SHA-256 digest) are rejected on arrival and after retry exhaustion. Corrupted
  data is never installed or referenced in the repository, and no receipt is issued.
- **Resumable transfer without retransmission**: interrupted transfers survive
  process kills and network crashes. Completed verified chunks remain pinned in
  the object store; subsequent sync sessions inspect `transfer_chunks` and
  `VerifiedChunk`, reusing already verified chunks and fetching only missing parts.
- **Idempotent durable receipts**: receipts are issued only after durable
  readiness is committed in SQLite. Replays of receipts are safe and idempotent.
- **Stored versus applied differentiation**: versions stored in the repository
  remain clearly labeled as `stored=true, applied=false` when awaiting publication,
  held in structural conflict, or synced with a headless receiver. Once applied,
  they transition to `stored=true, applied=true`.
- **Two-agent local demo**: Node A and Node B run independent CLI processes
  with isolated state and root directories, exchange public certificates, approve
  pairing, serve/sync via TLS, and verify transferred files and status end-to-end.

## Owner explanation: sender disappearance before workspace application

If the sender disappears after the receiver has stored all chunks but before
the receiver has applied the file to its working workspace:

1. **Self-sufficient durability**: The receiver's database transaction has already
   committed `content_state = 'ready'`, object references are bound, and all
   chunks are verified and permanently stored in the repository chunk store.
   The transfer is marked complete and content pins/reservations are cleared.
2. **Independent local application**: Because all required chunks and metadata are
   already durably present locally, the receiver does not need the sender to be
   online to publish or project the file. A subsequent publication pass or
   workspace apply will read the verified chunks directly from local storage and
   safely project the file into the working tree.
3. **No data loss or blocking**: If the sender never reconnects, the file content
   and version history are fully preserved on the receiver.
4. **Replay-safe receipt recovery**: If the sender disappeared before receiving
   the durable receipt acknowledgment, the receiver retains the ready version
   in its repository and will replay the receipt during any future sync session
   with that peer.

## Limitations

All tests were executed on a single Linux ext4 host over loopback networking.
The arm64 binary was cross-compiled but not natively executed. No hosted CI,
multi-host physical network test, high-connection soak, or real-link WAN latency
campaign was performed. P06 automatic publication is intentionally conservative
(single unapplied heads only); bidirectional conflict reconciliation and
multiple concurrent heads remain P07.
