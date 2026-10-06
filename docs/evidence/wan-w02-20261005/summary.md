# W02 — 2026-10-05

State: **complete**. All W02 acceptance items have local production/manual
evidence; final frozen-source M0, focused race and compatibility passed. [Manifest](manifest.json) records
initial revision/dirty source and frozen final Go source hashes; [commands](commands.md)
and [results](results.json) record executions. [Architecture](../../orbit-wan-architecture.md#w02-manual-manager-integration)
owns the implemented bounds and lifecycle.

Delivered one daemon-owned manual connection manager, shared by scheduler and
live enrollment/bootstrap/membership-artifact clients. The frozen Target/pinned
replication TLS/RoundTripper seam retains immutable device/pin/purpose; route
changes do not grant folder authority. v1 peers.json and existing stopped/live
controller selection are preserved. Missing private policy intent loads manual/
generation 1 without writing state. Future automatic/local-discovery policy
activation fails explicitly; no global service or network capability is enabled.

Pools, requests, imported routes and passive observations have finite bounds.
Idle LRU eviction retains all imported targets for existing fair scheduling.
Generation changes reject new requests from old pools, allow bounded old requests
to drain and suppress stale completion observations. Busy/stale errors use existing
retry budgets/identities. Independent dial/socket admission prevents detached
net/http work from escaping limits during flapping. Shutdown cancels/joins work,
closes active bodies and sockets and releases pools before SQLite closes.
Enrollment retains its shorter deadlines; peer metadata/chunk/body limits remain.
Plaintext, redirects, proxy inheritance, URL-chosen destinations and competing pins
are refused. Cached pools reject changed roots/client certificates; each borrower's
verifier also runs on the authenticated socket before HTTP writes.

Executed acceptance evidence:

| Acceptance | Evidence |
| --- | --- |
| Manual two-way sync and interrupted chunk reuse | W02 production-handler transfer verifies one reused/one fetched chunk, exact normalized heads and protected hashes; retry after generation change creates no additional version; reverse-direction bytes/heads match |
| Real CLI restart and three-peer forwarding | Uncached T04/T05 process journeys use production create/invite/review/join/approve/sync, receiver restart, original-author forwarding through B, endpoint refresh and verified head/hash oracles |
| Continued progress | Uncached background daemon test transfers a 16-MiB archive while ordinary edits continue; T05 verifies progress with another peer offline; slow-manager fixture preserves other-peer progress with bounded queued callers |
| Identity and authority | Wrong pin, competing pin and unknown requester fail; existing uncached replication/control suites retain member/retirement checks; T04/T05 retain identities/authors/history after restart and address refresh |
| Bounded lifecycle/policy | 10,000 generations agree with independent WG5 model; purpose/draining pools, 128-target admission, 32-pool LRU across all targets, request/dial/socket caps, canceled pending dials and abandoned response bodies are verified |
| Race and compatibility | All 12 discovered W02 tests pass under race; uncached CLI/control/replication/network/config/scheduler/model suites pass |
| M0 | Final frozen-source `make check` passed: terminal/extracted-package checks, formatting, vet, CLI/internal/model, integration, fault/design-gate suites, 12 Python harness checks, amd64/arm64 binaries and packages |

All network fixtures are single-development-host disposable sockets, synthetic
files and private test roots. Production-interface checks use manual nonloopback
addresses; they are not physical LAN/native WAN evidence. The non-destructive
chunk interruption fixture explicitly marks its synthetic state disposable.
No personal folder, existing VPS service, privileged namespace or real network
was a fault target. Existing P/O/T and W00/W01 evidence/dependencies/fixtures remain
preserved outside declared owning code/specification edits.

Documentation/anchor/discovery and preservation checks passed: all 12 W02 tests
executed with race, 2148 initial files retained outside declared owning edits,
and every frozen Go source/go.mod/go.sum/Makefile hash remained unchanged.
`git diff --check` passed. M0 uses Go's ordinary cache where labeled; the focused
race and compatibility commands explicitly use `-count=1`.

Initial test failures: the new policy fixture lacked private directory permissions,
and the new authorization fixture used incorrect Hello fields. Corrected test
fixtures retained production validation; failed logs remain linked in commands.md.
Final review added per-borrow verifier checks and independent detached dial/socket
admission; the final focused race and M0 executions cover those refinements.

Unexecuted: production rendezvous/profile/lease/DNS-rebinding, WSS relay selection/
quotas, v3 durable routing/enrollment, direct/relay races, QUIC/ICE/STUN, actual
network-change detection/roaming, physical WAN/NAT/Pi memory/FD/egress campaigns
and operated hosted services/signing custody. WG4/WG6 remain pending; WG5's manual
model integration is complete but native timing/roaming closure remains W11.
T13 native login/logout/unattended boot checks remain outstanding. P17 owner
use/explanation stays deferred. No release-wide or native-WAN completion claim.

Next eligible: **W03 — Authenticated rendezvous and profiles**.
Read its packet in [relay/setup](../../implementation/wan-relay.md#w03--authenticated-rendezvous-and-profiles),
W01 strict signed fixtures/admission model and W02 manager evidence. Extend the
same target/trust/generation/lifetime seam; service lookup cannot establish pins
or authorize folder access. Preserve legacy manual policy until reviewed migration.
