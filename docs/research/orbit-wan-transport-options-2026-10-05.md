# Orbit WAN transport composition options

Research inspected on 2026-10-05. This note supplies implementation options for
the requested native WAN plan. It changes no existing specification, status,
dependency or runtime code. Every integration experiment below is **unexecuted**.
Library API availability is source evidence, not proof that the combination works
in Orbit. No particular dependency release is recommended without a pinned spike.

## Preferred composition

Engineering recommendation: preserve the existing peer HTTP API and enrolled
device TLS identities; introduce a network-path layer below/alongside that API.
Use three paths, delivered in increasing complexity:

| Path | Proposed composition | Authentication boundary |
| --- | --- | --- |
| Reachable LAN/WAN | Existing HTTPS over direct TCP, including reachable IPv6 | Existing mutually authenticated, pinned device TLS |
| Relay fallback | Outbound HTTP/1.1 WSS via `coder/websocket`; binary stream adapter; existing inner device TLS and HTTP | Public service TLS authenticates the relay endpoint; inner device TLS authenticates peers independently |
| Direct NAT-assisted UDP | Pion ICE-owned sockets; per-peer packet-preserving `net.PacketConn` adapter; quic-go and native HTTP/3 | Existing device certificate/pin policy adapted to QUIC TLS and checked again by existing HTTP authorization |

The third row is a **preferred spike candidate**, gated on the tests below. It
is not an assertion of a supported plug-in integration. Keep relay functionality
independent so failure of that spike does not remove VPN-free connectivity where
both devices can reach the relay. Do not introduce TURN simply because ICE
supports it: Orbit's proposed WSS relay is a separate fallback mechanism. Any
decision to deploy TURN should have a separate demonstrated need.

For the requested plan, use the approved default hosted-service profile so normal
owners install Orbit and pair devices without entering endpoints. Preserve an
optional self-hosted service profile whose operator configures public endpoints,
service certificates and capacity. Service authentication must bind Orbit's
existing random device identifier to its enrolled SPKI pin through proof of key
possession; do not redefine that identifier as a hash-derived identity.

Current Orbit code constructs its own HTTP transport in
[the peer client](../../internal/replication/client.go) and accepts an explicit
`net.Listener` before wrapping it in TLS in
[the peer server](../../internal/replication/server.go). Injecting path dialing
and choosing a round-tripper therefore requires refactoring. Changing transport
must preserve authorization, finite request/body limits, wire validation, retries,
causal history and durable receipts. Network addresses are routing information,
not identities or enrollment approval.

## Relay-first WSS integration

Coder maintains the successor to `nhooyr/websocket`; its API provides `NetConn`
specifically for tunneling arbitrary protocols. Every write becomes a WebSocket
message and reads concatenate message contents into stream behavior. Its own
current README lists HTTP/2 as future work. Use the documented HTTP/1.1 WebSocket
upgrade path; do not assume HTTP/2 extended CONNECT support. Sources:
[project](https://github.com/coder/websocket) and
[NetConn source](https://raw.githubusercontent.com/coder/websocket/master/netconn.go).

Proposed flow: two independently authenticated devices each dial a relay's WSS
endpoint. The relay pairs authorized session endpoints and copies binary stream
bytes using fixed buffers. Once paired, one side acts as inner TLS client and
the other as inner TLS server according to an authenticated role assignment.
Outer HTTPS/WSS protects the device-to-relay hop; inner pinned peer TLS prevents
the relay from reading file contents or impersonating the destination. Neither
a valid public service certificate nor a relay session is proof of enrollment.
This composition is engineering inference from the stream adapter and Go TLS,
and must be demonstrated with Orbit's peer API. Source:
[Go TLS](https://pkg.go.dev/crypto/tls).

Adapter caveats: `NetConn` disables its read limit, an active read/write deadline
can close the entire WebSocket, and dialing returns placeholder addresses.
Session metadata must carry diagnostics separately from those addresses. Retain
or restore finite message limits deliberately and stream with bounded buffers;
do not use whole-message reads for bulk forwarding. The writer API is
context-bounded and serializes open writers, which supports blocking flow control
but does not establish a process-wide memory bound. Source:
[writer source](https://raw.githubusercontent.com/coder/websocket/master/write.go).
Continuous read processing is required for control frames/pongs; combine it with
application/session deadlines, byte quotas and finite connection admission.
Source: [connection documentation](https://pkg.go.dev/github.com/coder/websocket#Conn).

The spike must show inner TLS remains pinned even when the relay swaps endpoints,
and that slow consumers apply backpressure without unbounded queues. Exercise
proxy upgrade refusal and idle timeout as failure cases. WSS on TCP 443 is not a
promise that every HTTP proxy, captive portal or managed firewall allows it.

## QUIC streams compared with native HTTP/3

A QUIC connection is not a `net.Conn`: application bytes travel on streams. A
bidirectional stream supplies reads/writes and deadlines, but a usable
`net.Conn` adapter must define addresses, closing/cancellation and parent
connection ownership. Source:
[QUIC stream documentation](https://quic-go.net/docs/quic/streams/).

Native HTTP/3 is preferable for the first UDP spike because quic-go supplies an
`http.RoundTripper`, client TLS configuration and an injectable QUIC dialer.
Its server consumes normal `http.Handler`s and sets `http.Request.TLS` from
the QUIC connection's TLS state, preserving a useful route to Orbit's current
certificate-based authorization. Sources:
[HTTP/3 client](https://quic-go.net/docs/http3/client/),
[transport source](https://raw.githubusercontent.com/quic-go/quic-go/master/http3/transport.go),
[server source](https://raw.githubusercontent.com/quic-go/quic-go/master/http3/server.go)
and [request TLS assignment](https://raw.githubusercontent.com/quic-go/quic-go/master/http3/server_conn.go).

Engineering gates: clone the enrolled identity configuration, preserve client
certificate requirements and pin checks, select HTTP/3 ALPN, test TLS 1.3
compatibility and HTTP handler behavior, and translate existing timeout/admission
limits deliberately. Verify each cached connection remains bound to one expected
device identity and route generation. Disable 0-RTT for the initial integration;
replay semantics are unnecessary for this milestone. The official client docs
warn that early data can be replayed. Documentation examples and current source
show different QUIC dial signatures; implementation must use one selected module
release consistently, not paste signatures from mixed versions.

**Bounded alternate:** if native HTTP/3 fails an Orbit compatibility gate, spike
one raw QUIC bidirectional stream per HTTP connection with a `net.Conn` adapter,
then run the existing inner pinned TLS/HTTP above it. Authenticate outer QUIC
with the enrolled peer policy too; use a versioned ALPN and finite stream count.
This adds redundant encryption but minimizes assumptions about handler TLS state.
Measure its CPU/throughput cost, cancellation and close behavior. Do not invent a
new sync wire format or rely on an untrusted HTTP header to supply peer identity.
If the ICE/QUIC packet boundary itself fails, this alternate cannot fix it:
retain direct TCP plus WSS relay and close that design gate before promising
automatic direct UDP traversal.

## Pion ICE packet semantics and socket ownership

Despite having familiar read/write/address methods, Pion's established ICE
`Conn` carries **packets**, not a reliable ordered byte stream. The inspected
implementation reads a packet buffer, rejects STUN-shaped application writes,
writes through the selected candidate pair and ties connection lifetime to the
ICE agent. Feeding it straight into `tls.Client` would not create a reliable
transport. Source:
[ICE transport source](https://raw.githubusercontent.com/pion/ice/main/transport.go).

quic-go accepts custom `net.PacketConn` implementations. A per-peer adapter over
the established ICE application connection is therefore a plausible boundary:
ICE owns path establishment and sockets; QUIC receives/sends only application
datagrams through the adapter. The adapter must define a stable expected remote
route, reject misaddressed writes, preserve packet boundaries, honor deadlines and
close the owned agent exactly once. Source:
[quic-go transport](https://quic-go.net/docs/quic/transport/).
This is an inference about compatibility; no verified upstream recipe was found
or executed here. Prefer rebuilding the session when the ICE selected pair changes
until migration behavior is demonstrated. Do not label that reconnect as seamless
QUIC migration.

Pion's UDP mux returns packet connections by ICE ufrag and maintains its own
socket reader and address routing. quic-go also manages packet reading. Giving
both independent readers the same raw UDP socket would race consumers; neither
library's mux is automatically a combined ICE/QUIC mux. Sources:
[ICE UDP mux](https://raw.githubusercontent.com/pion/ice/main/udp_mux.go) and
[ICE mux API](https://pkg.go.dev/github.com/pion/ice/v4#UDPMux).
Start with per-peer ICE agents and strict finite peer/session counts. Shared
socket optimization is later work requiring explicit ownership and demultiplexing
evidence. A wrapped packet connection can lose quic-go UDP kernel optimizations;
measure rather than claim direct QUIC speed parity.

## IPv6, mapping and difficult NATs

Include reachable global IPv6 and IPv4 direct candidates; prefer candidates only
after successful peer-authenticated connection attempts. IPv6 avoids common IPv4
address translation but not firewall filtering or absence of IPv6 connectivity.
Residential IPv6 security recommendations explicitly include filtering of
unsolicited inbound traffic. Source:
[RFC 6092](https://www.rfc-editor.org/rfc/rfc6092.html).

Syncthing's `natEnabled` controls UPnP/NAT-PMP port mapping and its configuration
includes lease/renewal settings. Source:
[Syncthing NAT options](https://docs.syncthing.net/users/config.html#options-element).
Gateway mapping is an optional direct-route improvement, not a prerequisite for
relay-based WAN sync or a guarantee through CGNAT/nested NAT. One reachable peer
can accept an outbound connection from another behind NAT. Source:
[Syncthing firewall guidance](https://docs.syncthing.net/users/firewall.html).

Engineering recommendation: keep gateway mapping outside the first native WAN
release; any later mapping extension should be bounded and explicit in advanced
settings. Retain relay fallback when mapping is unsupported or expires. ICE
selects candidate paths and uses separate signaling; STUN does not by itself
overcome every NAT. Source: [ICE RFC 8445](https://www.rfc-editor.org/rfc/rfc8445).
Endpoint-dependent mappings, UDP filtering, same-router hairpin behavior and
double NAT belong in the declared test matrix. Use LAN candidates for same-network
peers and relay fallback where no tested direct path succeeds. Avoid mapping
implementation work until its expected benefit for the supported matrix is clear.

## Dependencies and licenses

These are verified candidate libraries, not installed selections. Confirm the
selected tagged release's API, Go compatibility, license and transitive notices
during the implementation spike. Inspected branch URLs can change.

| Component | Verified license | Owning source |
| --- | --- | --- |
| coder/websocket | ISC | [license](https://github.com/coder/websocket/blob/master/LICENSE.txt) |
| quic-go | MIT | [license](https://github.com/quic-go/quic-go/blob/master/LICENSE) |
| Pion ICE | MIT | [license](https://github.com/pion/ice/blob/main/LICENSE) |
| huin/goupnp, optional UPnP client | BSD 2-Clause | [project](https://github.com/huin/goupnp), [license](https://github.com/huin/goupnp/blob/main/LICENSE) |
| jackpal/go-nat-pmp, optional NAT-PMP client | Apache 2.0 | [project](https://github.com/jackpal/go-nat-pmp), [license](https://github.com/jackpal/go-nat-pmp/blob/master/LICENSE) |

## Required spike evidence — all unexecuted

1. **WSS/inner TLS:** two disposable peers use the real hello/version/chunk API
   through a relay; wrong pin, swapped destination, expired session and forbidden
   folder are rejected. Retain separate service and device certificate policy.
2. **Bounded relay behavior:** slow reader, oversized messages, disconnect during
   chunk transfer, full admission queue, cancellation, ping timeout and relay
   restart. Record active session count, RSS and copy-buffer/queue bounds.
3. **ICE/QUIC composition:** loss, reordering, MTU boundaries, tiny read buffers,
   deadlines and repeated teardown. Record packet truncation/error behavior and
   verify no leaked sockets, agents or goroutines within declared limits.
4. **HTTP/3 compatibility:** test the existing peer authorization using actual
   request TLS certificates, malformed/body-limited requests, concurrent chunk
   transfer, cancellation and route/identity cache separation. Replay-disabled
   handshakes and membership changes must retain existing authorization behavior.
5. **Routing and recovery:** IPv4-only, IPv6-capable, blocked UDP, independent NATs,
   nested/endpoint-dependent NAT, LAN without hairpin, network/address changes,
   service outage and direct-to-relay transition. Use disposable marked networks;
   distinguish simulated topology evidence from actual independent-host evidence.
6. **Decision record:** pin the exact dependency versions used, keep commands and
   results, select the preferred/alternate composition based on evidence, and
   record unsupported network conditions. No success-rate claim beyond the tested
   matrix and no promise of arbitrary school-network access.

Research used official web docs/source inspection plus a read-only GitHub source
lookup. No dependencies were installed and no integration experiment ran.
