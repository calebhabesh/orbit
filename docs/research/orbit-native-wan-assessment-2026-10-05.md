# Assessment of native WAN connectivity for Orbit

Checked on 2026-10-05 against primary documentation and specifications. This is
an advisory proposal, not approval to change product scope or a connectivity
guarantee. No Orbit code, approved specification, packet status or existing
evidence was changed. The earlier [Syncthing verification](syncthing-wan-verification-2026-10-05.md)
contains the pinned upstream source inspection.

## Recommendation

Engineering judgment: automatic WAN connectivity could make Orbit easier to
adopt and add a strong networking dimension to its distributed-systems story.
It is a substantial subsystem, rather than a small CLI improvement. Pursue it
as a bounded follow-on milestone after preserving the existing sync and recovery
baseline. A useful initial goal is authenticated setup and encrypted transfer
across ordinary home networks without requiring a VPN application. Do not promise
every school/corporate network, no supporting servers, or Syncthing feature parity.

The [approved scope](../portfolio-scope.md) currently selects reachable LAN/private
networks, explicit addresses, and documented Tailscale for cross-network use;
U06 and the exclusions omit Orbit-owned discovery/NAT/relay infrastructure. This
assessment explores an amendment; it does not enact one. The existing project
already has distributed-systems depth in causal reconciliation, durable capture,
conflict review, forwarding and crash recovery. Additional technologies only
strengthen the portfolio when their behavior and limitations are demonstrated.

## What the comparison means

The Syncthing daemon performs synchronization. Its built-in `syncthing cli`
controls a running daemon through its REST API. Therefore the networking
comparison is with Syncthing's daemon and supporting services; copying the CLI
alone does not reproduce automatic WAN connectivity. Source:
[command-line operation](https://docs.syncthing.net/users/syncthing.html#subcommands).

“No mandatory Tailscale” and “no external infrastructure” are different goals.
Syncthing's easy setup uses global discovery and community relay services when
direct connectivity is unavailable. Discovery authenticates announcements with
client certificates and returns possible connection addresses. Sources:
[global discovery](https://docs.syncthing.net/specs/globaldisco-v3.html) and
[relay-server operation](https://docs.syncthing.net/users/strelaysrv.html).
Engineering inference: Orbit can bundle clients and choose defaults so an owner
only installs Orbit, while its operator still supplies reachable supporting
services. Self-hosting those services moves setup work back to the owner.

## Work and complexity

These are engineering judgments about subsystem scope, not schedule estimates
or numerical difficulty scores.

| Capability | What it establishes | Implementation/validation burden |
| --- | --- | --- |
| Explicit reachable address | Connect to an already reachable peer | Smallest extension; secure exposure and diagnostics still matter |
| LAN/global discovery | Locate current candidate addresses by enrolled device identity | Moderate: authenticated registration, expiry, privacy, caching, rate limits and outage behavior |
| Outbound encrypted relay path | Connect devices that can both reach a relay | Substantial but bounded: session pairing, inner peer authentication, reconnects, quotas, bandwidth and outage handling |
| Automatic direct traversal | Try NAT mappings and/or coordinated candidate probing | High: socket ownership, mapping lifetime, candidate exchange, timeouts, NAT diversity, address changes and direct/relay selection |
| Reliable automatic WAN product | Make setup/status/recovery understandable across supported networks | Highest: security review, operations, realistic network validation and regression coverage |

Address discovery does not establish an inbound NAT mapping or permission to
reach a separate listener. Syncthing documents port forwarding and relay
fallback. One reachable device can accept connections from another behind NAT.
Source: [firewall setup](https://docs.syncthing.net/users/firewall.html).

QUIC supplies encrypted, reliable transport over UDP; it does not itself select
a reachable path through arbitrary NATs. `quic-go` can serve and dial on one UDP
socket and expose non-QUIC packets for other protocols. This helps integration
but leaves path establishment to the application. Source:
[quic-go transport](https://quic-go.net/docs/quic/transport/).

STUN can discover a NAT-assigned address, check connectivity and maintain
bindings, but is explicitly a tool within a larger traversal solution. ICE
gathers and checks candidate paths, including host, NAT-observed and TURN-relayed
addresses; it assumes a separate signaling connection already exists. Sources:
[STUN RFC 8489](https://www.rfc-editor.org/rfc/rfc8489) and
[ICE RFC 8445](https://www.rfc-editor.org/rfc/rfc8445).
Engineering inference: an Orbit ICE implementation still needs authenticated
rendezvous/candidate exchange and supporting reachable servers. ICE/TURN is one
possible design, not a claim that Syncthing implements ICE or TURN.

No design can promise connectivity if the network blocks every supported direct
and relay route. Putting a relay on TCP port 443 is not proof it will pass a
network that only allows an authenticated HTTP proxy or filters protocols.
Syncthing documents outgoing proxy support separately. Source:
[proxy operation](https://docs.syncthing.net/users/syncthing.html#proxies).
This is an engineering limit, not an experiment on the owner's school network.

## A bounded development sequence

1. **Keep identity and authorization foundational.** Bind rendezvous registrations
   and peer sessions to persistent cryptographic identities with proof of key
   possession. A new device still needs a route to request enrollment before it
   becomes a folder member. Discovery or possession of a relay session must not
   approve enrollment or folder membership. Syncthing's discovery
   certificate authentication and relay-contained peer TLS illustrate the
   separation. Sources: [discovery authentication](https://docs.syncthing.net/specs/globaldisco-v3.html#authentication)
   and [relay security](https://docs.syncthing.net/specs/relay-v1.html#how-syncthing-uses-relays-and-general-security).
2. **Prove a relay path first.** Proposed Orbit design: an owner-operated private
   rendezvous/relay with both devices making outbound connections, end-to-end
   authenticated encryption, finite session/bandwidth limits, and visible relay
   status. This answers the main setup question without relying on successful
   hole punching. It only works where those outbound connections are permitted.
3. **Add direct connectivity as an optimization.** Retain explicit/LAN routes;
   prototype direct UDP traversal and compare it with relay fallback. Evaluate
   whether NAT port mapping is useful for the declared network set. Do not add
   QUIC, ICE, WebRTC and every mapping protocol automatically: choose a coherent
   transport composition with evidence.
4. **Validate failure and reconnection before expanding defaults.** Exercise
   relay loss/restart, stale registrations, address/network changes, blocked UDP,
   both peers behind NAT, wrong identity, and transfer interruption. Report which
   scenarios were simulated and which used real networks. Measure connection
   success for the tested matrix, time to connect/reconnect, direct/relay transfer
   throughput and relay resource use. No universal success percentage follows
   from a small test matrix.

The suggested relay is an opaque transport service. Orbit's current optional VPS
is a trusted replica that retains authorized plaintext content/history. Those
roles can share an operator or host while remaining separate services. Sources:
[Syncthing relay privacy](https://docs.syncthing.net/users/relaying.html#security)
and [Orbit scope U03/S19](../portfolio-scope.md).

## Reuse and operational obligations

Use maintained primitives rather than writing cryptography or transport protocols.
Candidates to investigate include Go's [TLS package](https://pkg.go.dev/crypto/tls),
[quic-go](https://quic-go.net/docs/), [Pion ICE](https://github.com/pion/ice),
[Pion STUN](https://github.com/pion/stun) and
[Pion TURN](https://github.com/pion/turn). These are available components, not a
verified compatible Orbit stack. Prototype framing, reliable-stream semantics,
socket sharing, identity checks and library limits before selecting dependencies.
Orbit should own connection policy, service integration, bounded retries and
user diagnostics while retaining sync/recovery semantics in their current modules.

Current repository inspection shows a useful integration boundary: the peer
[server](../../internal/replication/server.go) accepts a `net.Listener` and wraps
it in peer TLS, while the [client](../../internal/replication/client.go) constructs
its own `http.Transport` with pinned peer TLS. Engineering judgment: introducing
relay-aware dialing/listening may preserve the current authenticated peer API,
but requires implementation and verification; it is not a configuration toggle.
The WAN milestone must preserve enrollment, folder authorization, causal history
and recovery behavior independently of the selected network path.

A relay consumes network capacity for every relayed transfer. Discovery/relay
operators need access controls or abuse limits, session/time limits, monitoring,
patching, certificate/service configuration, outage behavior and an explicit
metadata/privacy policy. These are engineering obligations implied by operating
internet services. Syncthing's relay server exposes global/per-session rate limits,
timeouts, statistics and private access tokens; its public relay pool is
community-operated. Source:
[relay-server options](https://docs.syncthing.net/users/strelaysrv.html).

## Evidence limits

This note records repository-document review and official web-source inspection.
No dependency was installed, no network traversal prototype was executed, and no
Orbit tests or school/home connectivity checks were performed. No WAN capability
was deployed and no delivery schedule was established. Existing upstream line
counts do not estimate this build's duration. A defensible implementation estimate
requires the selected architecture and a small connectivity experiment.
