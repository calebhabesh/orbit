# W01 — 2026-10-05

State: **complete**. [Contracts/gate outcomes](../../implementation/wan-contracts.md)
and [strict schema](../../../schemas/network-v1.md) freeze the local design;
[commands](commands.md), [results](results.json), [dependency audit](dependency.md)
and [manifest](manifest.json) record actual provenance and validation.

Delivered direct/WSS pinned HTTP adapters with a TLS-only RoundTripper, immutable
Target pin, cloned replication trust, HTTP/1.1 ALPN, purpose-specific listeners,
bounded frame/read/copy buffers, request deadlines, backpressure and joined close.
Pinned coder/websocket v1.8.15/ISC after official API/source/license review and
amd64/arm64 compilation. No daemon wiring or production network capability is
advertised yet.

WG1–WG3 local gate evidence passes: verified production file/chunk transfer via
both adapters; wrong inviter/target pin before HTTP secret disclosure; missing
mTLS, unknown signed requester and enrollment/peer/control isolation; encrypted
synthetic broker captures; plaintext and redirects refused; request/stream cancel,
slow WSS reader, maximum/over-limit frames and both-leg cleanup. The v3 fixture
verifies signed logical enrollment over direct/relay TLS, uses an independent
exact-approval oracle and existing repository v1 membership/retirement checks.
Actual v2 relay challenge/request reaches pending approval without folder access.

Independent Python Ed25519/canonical goldens and Go tests cover profiles, proof,
announcement, relay attachment, v3 invitation/request/status/approval, maximum
profile/announcement, one-over fixtures, 64 KiB bodies, 16 KiB invitations,
4096-byte DER certificates, exact uint64 strings, scoped URLs/candidates and signed
substitution. Models cover replay/expiry/restart, competing pins, lease replacement,
capacity protection, separate same-key folders, revocation/retirement, lost-response
resume, token-versus-tunnel expiry and 10,000 seeded connection-generation events.

The initial relay composition failed because NetConn's irreversible read deadline
cancellation conflicts with net/http background-read cancellation. Retained
[failed run](transport-first-failure.log); fixed-buffer reversible read adapter
passed uncached and race transfer/cancellation checks. A later cancellation test
fixture initially left its request body unread; after consuming it as production
handlers do, HTTP detects client closure and the direct/relay cancellation checks
pass. No production authorization/causal semantics were weakened.

Passed nonzero W01 discovery, focused uncached and race checks, existing uncached
CLI/control/internal/model suites, T03/T04/T05 compatibility journeys, vet and
CGO-free amd64/arm64 builds. Supplementary final race/build/docs results are recorded
in commands.md/results.json. W01 does not require re-running M0 `make check`;
that integrated milestone is W02. Prior W00 `make check` evidence stays unchanged.

Preserved the initial W00 dirty/untracked source/evidence outside owning specification
updates. No database/config/identity/history migration or live workload action.
All network fixtures are local disposable listeners and synthetic data; no personal
folder, VPS service, privileged namespace or physical LAN/WAN fault target.

Unexecuted: W02 daemon manager/trust/policy integration; W03/W04 production admission,
DNS/rebinding, quotas and services; W05 durable v3 challenge/status/approval/resume
and retirement-artifact bootstrap; HTTP2 WSS/HTTP3/QUIC/ICE/STUN; native NAT/WAN,
roaming, Pi load, aggregate memory/FD/egress measurements, operated hosted/default
profile and signer custody. Explicit test CAs/brokers are development fixtures,
not hosted-default acceptance. N07 and WG4/WG6 remain pending. WG5 retains its
W02/W11 closure. T13 native login/logout/unattended boot remains outstanding;
P17 personal use/explanation stays deferred.

Next eligible packet: **W02 — Connection manager and preserved HTTPS**.
