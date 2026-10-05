# WAN block C: direct connectivity

Read [plan](../orbit-wan-implementation-plan.md), [architecture](../orbit-wan-architecture.md),
[network protocol](../orbit-wan-protocol.md), [gates](../orbit-wan-design-gates.md)
and [tracker](wan-status.md). All checks/outcomes below are planned and unexecuted.
The relay milestone remains usable while these packets are developed.

## W08 — LAN discovery and reachable direct candidates

Dependencies: W02, W03, W05. Change: local discovery, candidate gathering/validation,
optional direct listeners, connection selection, scoped fixtures and settings.
Invariants: I08–I09, I13, I15, N01–N03, N06–N09.

Required work:

- Implement bounded local announcements on selected interfaces with signed device/
  pin/capability/candidate data and no folders/secrets. Dial only known pinned peers;
  an explicit nearby-device view may show unknown candidates without authorizing them.
- Gather actual routable interfaces and eligible public IPv4/IPv6 addresses; keep
  LAN candidates scoped locally. Treat source-address observations as candidates
  rather than reachable ports. Support direct public HTTPS where it is reachable.
- In fresh Automatic mode, choose unprivileged optional direct listeners and
  advertise actual bound ports. Direct listener collision/unsupported interface is
  a route limitation while relay/local capture continue; manual listener failure
  retains existing explicit-config semantics. Freeze exact defaults in WG5/spec.
- Race candidate address families within global/per-peer limits, verify pin before
  declaring usable, expire stale cache entries and preserve service-outage behavior.
  No host firewall or router configuration is changed.

Acceptance evidence:

- Production LAN discovery/known-peer sync without manual IP, plus reachable public
  TCP/IPv6 fixtures; initial pairing still requires invitation and explicit approval.
- Fake/spoofed/oversized datagrams, unknown pins, loopback/multicast/private public
  announcements, interface-scope mismatch and address changes fail safely.
- IPv6 unavailable, blocked TCP and occupied optional listener preserve relay/capture;
  a directory outage does not break an existing valid direct session.
- Finite packets/timers/sockets and maximum candidate bounds pass race/resource
  checks. Real IPv6 availability is reported rather than inferred from loopback tests.

## W09 — QUIC HTTP3 transport and integration subgate

Dependencies: W01, W02, W08. Change: direct UDP transport adapter, HTTP3 server/
client wiring, compatible dependency pins, fixtures and WG4 transport evidence.
Invariants: I05–I09, I13, I15, N01–N07.

Required work:

- Inspect and pin actual supported quic-go/Pion APIs and licenses; compile amd64/
  arm64. Prove native HTTP3 over ordinary UDP before composing full traversal.
- Reuse peer HTTP request encoding/handlers and replication-supplied device TLS.
  Verify `req.TLS`, mandatory client certs/pins, ALPN, per-request membership and
  all header/body/deadline bounds. Disable 0-RTT/early data explicitly.
- Implement the intended packet adapter contract/test adapter with datagram, address,
deadline, MTU and closure behavior. Freeze adapter addresses per generation and
verify one transport can listen/dial both HTTP pull directions. Explicitly map
HTTP3 timeout/body/header limits. W10 supplies actual ICE establishment.
  Make UDP optional; initial enrollment stays on isolated HTTPS/WSS.
- If native HTTP3 fails WG4 transport acceptance, execute the bounded raw QUIC
  stream/inner-TLS alternate and update the owning design with measured costs.

Acceptance evidence:

- Real peer API and verified chunk transfer over direct QUIC, wrong/missing cert,
  unauthorized folder, incompatible version and oversized/header-body inputs.
- Datagram boundary/deadline/cancel/close tests; no raw-ICE-as-stream assumption;
  no simultaneous socket readers or data/control handler mixing.
- Interrupted transfer retains verified chunks, rejects partial bytes and never
  sends premature stored receipts. Streaming memory stays bounded on tested loads.
- WG4 transport subgate recorded complete with selected versions/composition;
  full ICE integration and NAT success remain explicitly unexecuted until W10.

## W10 — ICE/STUN coordination and traversal

Dependencies: W03, W04, W09. Change: ICE gather/check integration, authenticated
candidate exchange, actual packet adapter, STUN server/client configuration and
disposable NAT matrix. Invariants: I08–I09, I13, I15, N01–N07, N09.

Required work:

- Establish per-peer ICE sessions through signed rendezvous offers/accepts binding
  IDs/pins, purpose, session, role, network generation and expiry. Pion owns raw
  UDP; QUIC consumes only the selected-pair packet adapter. Coalesce simultaneous
  offers, cap candidate gathering/checks and destroy temporary credentials on close.
- Gather host/STUN-observed candidates with maintained libraries; reject prohibited
  scopes and stale generations. Service STUN must have anti-amplification/rate
  limits and separate UDP exposure; a STUN result never grants file access.
- Implement candidate-to-peer route establishment and typed fallback reasons.
  Use WSS relay where checks fail, UDP is blocked or mappings are incompatible.
  TURN, WebRTC and router mappings are outside this packet.
- Close WG4 full integration with actual ICE packet/HTTP3 or selected alternate
  composition under network changes, cancellation and loss.

Acceptance evidence:

- Disposable emulator cases: no NAT, endpoint-independent mappings, address/port
  dependent filtering, incompatible/double NAT and blocked UDP. Record exact rules;
  do not label CGNAT or NAT classes from timeout guesses.
- Demonstrate direct success on supported simulated paths and relay success on
  blocked/incompatible cases; measure actual selected route and hashes/heads.
- Wrong target pin/role/session, expired/replayed candidates, malicious address
  targets, malformed STUN and repeated offers cannot bypass identity or exceed bounds.
- Slow receiver/large stream, MTU loss, pair-change rebuild and shutdown release
  all resources. Repeat relevant race/HTTP authorization tests over the full path.

## W11 — Route policy, roaming and fair progress

Dependencies: W08, W09, W10; WG4 complete. Change: manager state machine, cooldowns,
network-change detection, pool draining, scheduler integration and timing settings.
Invariants: I01–I08, I13–I15, I28, N01–N08; closes WG5 production evidence.

Required work:

- Integrate bounded TCP/QUIC/direct races and relay head start/fallback, direct
  reprobes while relayed, authenticated route selection and sensible cache cooldown.
  Tune documented numeric defaults using latency/loss/Pi experiments.
- Rebuild generations on interface/default-route changes; reannounce and recreate
  ICE as needed. Drain in-flight requests or retry them with existing stable IDs
  and verified chunks. Keep membership mismatch/fork distinct from network failure.
- Cap sessions/attempts/goroutines across peers; apply bandwidth and fairness budgets
  to all routes. Continuous retries or small-file traffic cannot starve large work.
- Keep local capture independent of service failure. Persist desired policy, not
  stale socket/ICE secrets; restart from durable setup and routing intent.

Acceptance evidence:

- Relay→direct, direct→relay, TCP↔QUIC and Wi-Fi-style address changes during chunks,
  receipt loss and membership polling preserve versions/heads/hash oracles.
- Outages, reordered generation responses, repeated flapping, slow DNS/relay,
  competing dialers and cancel/shutdown stay bounded without downgrade or pin change.
- No new author/version/operation identity, false stored/applied state or extended
  invitation authorization after route switching. Working files stay protected.
- M3 reports tested reconnection times and fair large/small progress; source-only
  state-machine review does not replace executable WG5 evidence.
