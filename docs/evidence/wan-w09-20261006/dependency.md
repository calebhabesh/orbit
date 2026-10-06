# W09 dependency and API audit

Selected and downloaded from the Go proxy on 2026-10-06:

- quic-go **v0.63.0**, commit `9d085cc690f7c96451e8ae5659eb0e64671da47a`, MIT,
  Go >=1.26; module sum `h1:LIFGHI4PFUhhw2dDD1ARHdCff143ffMHwZtbnbuJ78A=`.
- Pion ICE **v4.4.6**, commit `9272687bdf25a21e8e352dc559601eb3e284d374`, MIT,
  Go >=1.24; module sum `h1:iOjj09NIYeWcveyymIeCSo8PNeja1wFnsx2A85JdWjo=`.
- qpack v0.6.0 (MIT); exact full dependency graph retained in `modules.json`.
  Go.mod/go.sum pin the stack; NOTICE retains dependency licenses. Pion is a
  test import proving `*ice.Conn` implements the selected `PacketPair` contract.
  Production traversal and actual ICE pair establishment remain W10.

Inspected downloaded tagged source (not assumed signatures): `quic.Transport`
`Listen`, `Dial`, `ConnContext`, `VerifySourceAddress`, `Close`; `http3.Server`
`NewRawServerConn`, `RawServerConn.HandleRequestStream` and
`HandleUnidirectionalStream`; `http3.Transport.NewClientConn`,
`ClientConn.RoundTrip`; HTTP3 `responseWriter.SetReadDeadline/SetWriteDeadline`;
`Config.Allow0RTT`, receive windows and stream limits. `Listen` and `Dial`
wait for handshakes; Orbit never calls `ListenEarly`/`DialEarly`, disables server
session tickets and client session caches, and explicitly sets `Allow0RTT=false`.

Native server has no header-read timeout setting. Orbit accepts request streams,
sets their five-second read deadline **before** passing them to the library's
HTTP3 parser, then resets body read and response write deadlines through
`http.ResponseController`. The library supplies `req.TLS`/RemoteAddr and the
existing peer handler performs exact DeviceID/SPKI/folder/revision authorization.

Pion `transport.go` exposes `Conn.Read` (packet buffer), `Write`, `LocalAddr`,
`RemoteAddr`, `Close`, `SetDeadline`, `SetReadDeadline`, `SetWriteDeadline`.
Read delegates to packetio.Buffer; write deadline applies to the selected local
candidate. W10 must own pair-change callbacks/closure and must never let QUIC
read the raw socket that the ICE agent reads. W09 tests use a synthetic established
pair backed by real UDP, including one dropped and duplicated short-header packet.
No ICE-role/NAT/STUN runtime acceptance is claimed.

Official supporting sources: [quic-go transport](https://quic-go.net/docs/quic/transport/),
[HTTP3 server](https://quic-go.net/docs/http3/server/),
[HTTP3 client](https://quic-go.net/docs/http3/client/),
[tagged quic-go source](https://github.com/quic-go/quic-go/tree/v0.63.0),
[tagged Pion source](https://github.com/pion/ice/tree/v4.4.6).
Local source and executable checks are authoritative for this selected stack.
