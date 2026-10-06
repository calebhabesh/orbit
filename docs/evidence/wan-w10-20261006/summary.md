# W10 — ICE/STUN coordination and traversal

**Complete for production composition and local native/emulator acceptance.**

Signed ICE coordination, full Pion/HTTP3 integration and a bounded optional STUN
listener are implemented. Final acceptance results are recorded in
[results](results.json) and [commands](commands.md), with the initial dirty-tree
[manifest](manifest.json), retained failed runs and [preservation](preservation.json).
[Architecture](../../orbit-wan-architecture.md#w10-authenticated-ice-integration)
and [protocol](../../orbit-wan-protocol.md#w10-ice-offer-extension) own the design.

Delivered deterministic controlling role and coalesced simultaneous pull directions;
strict additive independently signed wire fixture with unchanged legacy bytes;
identity/purpose/session/generation/expiry validation; maintained host/srflx gathering
without publishing private topology; selected-pair packet ownership; pinned peer TLS
and unchanged replication authority; typed failure plus separate WSS fallback;
joined pair retirement and fresh credentials; runtime-owned pairs that survive a
directory outage. Automatic/self-hosted settings enable it; UDP-disabled, manual,
Local-only and enrollment paths retain their documented behavior.

Acceptance evidence:

- Focused race tests execute twice. Pion emulator paths cover no NAT,
  endpoint-independent mappings and address-dependent/address-port-dependent
  filtering with actual HTTP3. Address/port-dependent incompatible mappings,
  explicit two-layer NAT and blocked peer UDP use actual WSS fallback. Exact rules
  and selected routes are logged. Production replication in both directions proves
  heads, bytes, hashes, original authors and interrupted-transfer chunk reuse.
- Actual consent loss closes the old pair; restored connectivity establishes fresh
  endpoints in approximately 6.5 seconds in the fixture. Service shutdown leaves a
  live pair usable: subsequent capture and transfer of a new file remain verified.
  These are local deterministic fixtures, not native roaming-policy acceptance.
- Signed wrong pin/role/session/generation, expiry, replay, mutation, prohibited
  addresses, duplicate/oversized candidates and ICE-to-relay reservation confusion
  fail closed. HTTP3 full-path tests retain unshared-folder denial, control/enrollment
  handler isolation, the 8-MiB body bound and borrower pin refusal. Broader W09 race
  tests retain mandatory TLS certificate, ALPN and authorization checks.
- A fresh marked native namespace executes real UDP4 ICE/HTTP3 and failed-ICE/WSS
  replication twice, plus actual UDP6 STUN (44-byte reply to 20-byte request).
  Fresh reverse relay may meet the unchanged metadata quota; the fixture retries
  that typed refusal after a five-second quiet refill within twelve seconds.
  One-second retries consumed the refill and are retained as failed evidence;
  production cooldown/fairness remains W11. IPv4 STUN produces 32/20 bytes; malformed/nonbinding/CHANGE-REQUEST/oversized
  packets are silent, and a 100-request burst yields at most ten replies per prefix.
- Slow receiver offers 32 MiB; only 65,536 bytes are sent before cancellation with
  a fixed 32-KiB producer buffer. Actual encrypted loss is tolerated. Shared QUIC
  admission accepts 32 connections across endpoints and rejects the 33rd; close
  releases slots. Session admission proves two concurrent attempts/eight residents;
  source flooding retains at most 32 tuples per socket. An empty local gather fails
  promptly in both controlling roles before coordination, preserving relay fallback.
  Native lifecycle samples
  show FDs/goroutines falling after joined runtime shutdown while repository and
  service fixtures remain open. They are samples, not peak RSS or Pi measurements.

Bounds are eight resident pairs, two establishments, eight advertised candidates,
two gathered addresses/four reviewed STUN servers, up to ten raw sockets per agent,
64-KiB requested socket buffers, 1,200-byte datagrams, 100 STUN packets/second/socket,
3-second STUN gather/4-second outer gather/5-second checks/12-second total attempt,
seven binding attempts per candidate pair and shared 32 incoming QUIC sessions.
STUN has one worker, 128-byte request/response limits, amplification at most three,
10 requests/second per /24 or /64, 200 globally and 1,024 retained prefixes.
Credentials remain ephemeral and clear on close; heap erasure is not guaranteed.

Real binary/PTY compatibility, aggregate verification, amd64/arm64 builds and
package extraction are recorded in the final command logs. Existing P/O/T and
W00–W09 evidence is preserved. No fault injection touched personal folders or a
live service workload; no changes were committed or published.

Next: **W11 — route policy, roaming and fair progress**. Full WG4 composition is
locally validated; W11 owns interface detection, timed reprobes/cooldowns, transport
races, combined resource budget and fairness. Physical WAN/CGNAT/Pi campaigns,
operated hosted defaults/self-hosting deployment, WG5 native timing, WG6, inherited
T13 login/logout/boot technical checks and deferred P17 owner use/explanation remain
outstanding. No TURN, WebRTC, router mapping or seamless migration is claimed.
