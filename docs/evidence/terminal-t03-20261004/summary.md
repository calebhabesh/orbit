# T03 — Authenticated network enrollment

T03 implements TG1's isolated pinned TLS transport, bounded challenge/request/
status exchange and authenticated reviewed owner approval. **T03 is complete** at this transport/authorization acceptance bar. Fifteen
ordinary packet tests passed twice; final serial `make check` and
`make test-race` both passed. Actual commands, failures and limitations are in
[commands](commands.md) and [results](results.json). Historical P/O and T00–T02 evidence is preserved.

The receiving device verifies the deliberately transferred inviter certificate
and SPKI before sending a capability. Ed25519 possession binds exact folder,
inviter, requester key, attempt, nonce, prior membership, endpoints, label and
expiry. SQLite commits capability use, request admission and nonce consumption
atomically; exact request retries consume one use, changed transcript/certificate
encodings conflict. Pending admission and nonce storage are capped. Status uses
a fresh requester signature, so an arbitrary ID or token cannot obtain approval
artifacts. Reviewed approval and its canonical existing membership revision share
one commit/replay boundary. Safe operation inspection excludes raw capabilities.

Fifteen ordinary packet tests passed twice. Actual nonloopback TLS/HTTP checks
exercise rejection of wrong scope/identity/proof, expired/revoked capabilities,
stale membership review, data/control isolation, unsigned/forged/replayed status,
oversized bodies and admission limits. The limiter has bounded accounting;
pending counts and owner pagination are performed in SQLite. Caps are seeded
record experiments followed by real HTTP refusal, not 128 real device enrollments.
The frozen P01 membership golden is also decoded with malformed-input checks.

Two real daemon child processes exchange request, approval and membership; the
receiving process signs and transmits its own proofs. Authenticated receiving
owner control imports the approved membership and a peer hello verifies the
exact revision/key authorization. A graceful owner restart preserves invitation/
approval replay and signed artifact retrieval. This is neither SIGKILL nor
power-loss evidence. The migrated pairing/CLI regressions retain preexisting
file preservation, exact membership, persisted authenticated endpoints and aliases.

Legacy loopback enrollment request/status routes now require owner authentication.
Legacy remote helpers reject an unpinned invitation before HTTP and use the new
TLS/status transport. Private invitation/review-file CLI adapters avoid capability
argv in the tested journey. Invitations without configured network settings stay
local administrative capabilities and cannot bootstrap remote join. The owning
protocol documents the HMAC capability derivation that permits explicit replay
without storing a raw capability at the inviter.

The compatibility join record supplies pinned transport and private preparation;
T04 still owns complete root-preview generations, durable joining/recovery and
truthful scan/content readiness. T05 owns same-key additional-folder/offline/third
peer rollout and runtime endpoint adoption. Physical two-host LAN, Tailscale,
native boot/logout and P17 actual owner use/unaided explanation are unexecuted.
No personal roots/services or host policies were changed; relocation sources and
unrelated changes were preserved. Next eligible packet after acceptance: T04.

Worker explanation: a capability permits a bounded enrollment request; the TLS
pin authenticates the inviting device and the signed transcript proves the
requester's existing key. Only explicit reviewed membership approval authorizes
folder data. A status nonce proves possession for retrieval, not owner approval.
These are separate checks with separate failure and replay boundaries. This is
worker explanation, not the outstanding owner's unaided evidence.
