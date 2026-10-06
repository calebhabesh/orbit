# W09 — QUIC HTTP3 transport and integration subgate

**Complete for production transport integration and single-host acceptance.**
WG4's transport subgate is closed; full ICE/traversal remains W10.
[Manifest](manifest.json), [commands](commands.md), [results](results.json),
[dependency audit](dependency.md) and [preservation](preservation.json) retain
provenance, actual executions, initial failures and limitations.

Delivered native direct UDP HTTP3 through unchanged peer handlers and
replication-supplied pinned TLS; one transport listening and dialing both pull
directions; optional daemon UDP listeners and actual-port LAN/public candidates;
strict additive `quic_http3_v1` capability/independent signature fixture; bounded
packet-preserving pair adapter with numeric immutable addresses, MTU/truncation
refusal, deadlines and joined closure. Local-only UDP binds a selected concrete
address and filters private same-prefix senders; no public service traffic.
Enrollment stays isolated HTTPS/WSS. TCP and relay remain available.
[Architecture](../../orbit-wan-architecture.md#w09-native-quic-http3-integration)
freezes versions, ownership, exact bounds and temporary selection policy.

Executed acceptance:

- **15 discovered W09 tests across five packages**. Focused network/protocol/config
  race checks pass twice; final replication checks pass twice with both native UDP
  and synthetic established-pair transport, one actual dropped/duplicated encrypted
  packet in each direction, exact two-way heads/hashes/authors and verified chunk reuse.
- Real TLS/HTTP3 checks pass for mandatory client certificate, wrong pin/ALPN,
  unknown member/unshared folder, incompatible peer version, enrollment/control
  isolation, 32-KiB request/16-KiB response header limits, 8-MiB body bound,
  actual five-second pre-parser header and 15-second body deadlines, 16-KiB
  response-header refusal and actual 15-second response-header / 30-second write
  expiration (additional race test executed once), cancellation
  and current borrower pin checks on reused pools. Early data is explicitly disabled;
  the authenticated QUIC route rejects any Used0RTT state before HTTP submission.
- Partial native chunk responses are rejected: no verified chunk, content-ready
  publication or premature stored receipt. An unavailable UDP socket preserves
  pinned TCP transfer. No failed HTTP3 request is replayed into another transport.
- Actual admission proves 32 accepted QUIC sessions and refusal of the 33rd; close
  releases slots/workers. Packet tests prove complete boundaries, 1,200-byte MTU,
  oversize/short-buffer refusal, frozen addresses, selected-pair change refusal,
  reversible read/write deadlines and close unblocking reads.
- The slow reader offers 32 MiB with a fixed 32-KiB server buffer; only about
  177–192 KiB sends before cancellation. Whole-test heap before/during samples
  are 920,712/1,503,776 and 1,233,800/1,985,408 bytes in the repeated final-focused
  run. These are samples, not peak RSS or Pi capacity. Cancellation joins endpoint
  workers; session-cap samples report two goroutines before and three/four after
  within the finite library-quiescence allowance.
- Race-instrumented **production binaries pass the Local-only QUIC journey twice**
  (81.669s): explicit invitation/pending denial/exact approval before signed LAN;
  both dated routes are QUIC; files/heads/hashes/identities match in both directions;
  zero service requests in Local-only; capture survives actual service shutdown.
  A separate real-binary UDP/TCP occupied-listener fixture passes fresh reviewed
  relay onboarding (45.24s). W08 TCP-only discovery and explicit manual collision
  also pass with valid configuration (combined 132.528s).
- W07 real PTY compatibility passes with child binaries instrumented (156.522s).
  Final uncached network/protocol/replication/config/control/TUI/CLI race suites pass.
  Focused Make aggregate passes format/vet/internal/CLI/integration/models/faults,
  all twelve Python harness checks and amd64/arm64 packages. Extracted package/PTY
  checks pass against final packages. Pion's pinned packet interface and QUIC test
  binary compile CGO-free for amd64/arm64; standalone service builds also pass.
- Independent Python Ed25519 UDP fixture, legacy canonical fixtures, relative docs
  links, source/evidence preservation and git diff whitespace checks pass.

Initial hello-limit/status-oracle failures were corrected against the existing
protocol. The first two real-binary runs exposed INVALID_ENCODING from using an
all-required signed-message decoder for additive local settings. The repaired
loader keeps W08 required fields, allows optional UDP fields and still refuses
unknown/duplicate/null/malformed data. The legacy regression and final binary
repetitions pass. The older listen-only collision fixture was likewise invalid;
it now supplies valid legacy settings and disables UDP to isolate TCP collision.
W09 additionally occupies UDP. Historical W08 evidence/failures remain untouched;
these final runs supply the corrected collision reproduction.

**Limits:** one Linux development host, actual local UDP/native private IPv4 and
synthetic established-pair sockets, signed development services and marked private
roots. Pion is pinned/compiled as a contract test; no actual ICE establishment,
ICE role conflict, STUN/NAT matrix, physical WAN/Pi capacity or hosted/default
operator/profile acceptance is claimed. The generic adapter cannot tune the raw
UDP receive buffer; W10 owns Pion socket buffers and selected-pair callbacks.
Local-only QUIC currently binds one selected address; TCP retains its existing
family/interface support. Mixed legacy discovery capability handling remains W14.
W11 retains mixed transport/family races, cooldown/reprobe/generation/roaming/fairness
and W08's failed restart evidence. Full make check is not rerun at this intermediate
packet; W07's full aggregate is preserved. T13 native login/logout/unattended boot
and deferred P17 owner use/explanation remain outstanding.

Next: **W10 — ICE/STUN coordination and traversal**. Establish bounded authenticated
Pion pairs, bind both pins/purpose/session/role/generations/expiry, attach the W09
packet-preserving transport per pair, close/rebuild on selected-pair changes and
prove both pull directions under actual ICE. Execute marked disposable NAT/loss/
blocked-UDP cases and relay fallback; close full WG4 before W11. Preserve all
prior evidence and keep hosted defaults/lifecycle release conditions explicit.
