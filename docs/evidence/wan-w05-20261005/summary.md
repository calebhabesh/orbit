# W05 — Routed enrollment v3 and peer data

Run date: 2026-10-05. This packet records local production-service and daemon
acceptance on one Linux development host. All destructive process tests used
fresh roots carrying the explicit `.filesync-disposable` marker.

The packet adds v3 routed enrollment as an additive protocol. V2 enrollment and
manual endpoints remain unchanged. Invitations carry the reviewed profile and
logical inviter route, while mutable candidates and relay credentials stay out
of the signed folder authority. The requester proves its key before capability
disclosure; the owner approves the exact request and canonical membership digest.
Accepted records retain only token digests and the exact signed v3 request.

The durable route file is private, bounded and separate from legacy `peers.json`.
Both pull directions register the same reviewed device pin/profile. Restart
reloads route intent, rebuilds transient relay sessions and resumes the same
operation, attempt and root. A route or pin change cannot overwrite a reviewed
identity.

The focused security suite covers replay, changed signed bytes, invitation
revocation and expiry, wrong folder/route/profile, v2 downgrade, status nonce and
possession checks, wrong inviter TLS before HTTP disclosure, retired identity,
approval artifact binding, pending data/control isolation and quota recovery.
The integration suite covers two folders to one device, third-device/offline
rollout, fork distinction, retirement snapshot bootstrap and two-way verified
content. The daemon campaign kills marked child processes at four durable
boundaries and verifies exact request/attempt/root/device identity plus reverse
data after restart.

Unexecuted or still outside this packet: hosted operator/profile custody,
physical WAN/NAT/Pi resource measurements, QUIC/ICE/STUN, automatic roaming,
ordinary CLI/TUI onboarding, and native T13 lifecycle checks. The full terminal
W05 aggregate command was interrupted during the long combined fixture run; the
focused packet campaigns listed above completed separately and are the evidence
counted here.
