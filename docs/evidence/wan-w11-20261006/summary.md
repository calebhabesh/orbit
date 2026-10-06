# W11 — route policy, roaming and fair progress

**In progress. Policy/roaming core implemented; packet acceptance and WG5 remain open.**

The daemon now races authenticated native QUIC/TCP handshakes before submitting
HTTP, starts relay after a bounded head start, reprobes idle relayed pools, backs
off failed direct cycles and rebuilds scoped networking after Linux interface/
default-route changes. Fresh signed candidates reset early empty-probe cooldowns;
a fresh lookup may revalidate an unchanged remote lease after local roaming.
Network changes never rewrite reviewed policy, pins, memberships, keys, histories,
version authors, operation IDs or invitation expiry. Old finite requests drain or
use existing verified-chunk/operation retry rules. HTTP is submitted to one route.

Implemented shared outgoing TCP/relay/QUIC handshake admission, joined/coalesced
network observation, LAN/service reannouncement, Local-only concrete UDP/socket
and listener-scope rebuilding, stale-generation rejection, demand-driven reprobes,
exponential direct cooldowns and shared quota quiet refill. Explicit responder
quota refusals may retry within the same session/role/generations; an attachment
quota refusal no longer leaves the peer silently waiting for inner TLS. Ready
queue work gains priority after eight skips and resets durable age on dispatch;
retry/reload retains its original task identity.

[Architecture](../../orbit-wan-architecture.md#w11-route-policy-and-roaming-integration-partial)
owns exact defaults and limitations. [Commands](commands.md), [results](results.json)
and [starting manifest](manifest.json) record provenance and actual checks.
[Preservation audit](preservation.json) confirms 381 prior WAN evidence files and
unrelated initial files are unchanged; [final source hashes](final-source.json)
identify this partial implementation.

Accepted local evidence:

- Native UDP HTTP3 and pinned TCP switch QUIC→TCP→QUIC across a two-chunk
  interruption and lost receipt response. Exactly one verified chunk is reused;
  original heads/authors/manifests, byte hashes, working files and durable receipt
  oracles agree. The mixed native TCP/QUIC candidate set chooses authenticated
  QUIC before TCP's bounded stagger. Isolated measurements are about 42 ms for
  TCP resume and 52 ms for QUIC receipt recovery; they exclude the watcher delay,
  production DNS/latency and physical network switching.
- A new private marked user/network namespace mutates only its own dummy interface
  and default route. Actual Linux address detection takes about four seconds;
  the subsequent default-route callback takes about six seconds. The same native
  replication journey passes there. This is native socket/kernel evidence in an
  isolated topology, not physical Wi-Fi/Pi/internet evidence.
- Race tests cover late lookup refusal, unchanged-lease revalidation, candidate
  arrival after an empty probe, timed failed ICE reprobes, quota quiet periods,
  cold-race error selection, joined shutdown during injected slow lookup/relay
  operations, bounded responder retries and exponential cooldown
  reset. Two hundred generation changes across eight busy targets retain sixteen
  pools/requests; draining all response bodies prunes them without stale status.
- Real Local-only binaries retain invitation/pending denial/exact approval,
  two-way native QUIC bytes/heads/hashes, zero service traffic and local capture
  after actual service shutdown. Optional UDP collision uses verified relay.
  The keyboard PTY setup/pairing/approval/quit journey passes.
- A synthetic one-TiB declared task is selected within nine ready dispatches
  twice, including durable queue reload and retry. This proves bounded dispatch
  and original task retention; it does not transfer that payload or establish
  large/small bandwidth fairness.
- Focused W08–W11 race/authorization/NAT/STUN regressions and the aggregate
  formatting/vet/unit/integration/model/fault/harness/build/package checks pass.
  amd64/arm64 builds and package extraction retain existing release constraints.

The first full `make check` attempt failed in the terminal suite: an early empty
candidate cache hid native QUIC, and the replacement join exhausted its signed
proof while waiting after throttling. The native binary/PTY repair passes. The
replacement join now passes in 83.245 s after preserving the original proof,
retrying a refused prepared submit within its deadline and using successful
submit state until the scheduled status poll. The final `make check` passes, including the complete terminal suite
(953.851 s), package extraction, formatting/vet, unit/integration/model/fault/harness
checks and amd64/arm64 builds; exact outcomes are in commands/results. Expiry and quotas were never enlarged. Initial compile errors,
the cooldown mutex leak, quota/error-order, early-candidate-cache and enrollment
admission failures remain retained.

Remaining acceptance:

1. Add reviewed Advanced timing controls and tune finite defaults on Pi and
   latency/loss fixtures. No Pi or physical-network measurements occurred here.
2. Exercise whole production daemons switching relay→direct/direct→relay and
   Wi-Fi-style addresses/default routes during chunks, receipt loss and membership
   polling, including slow DNS/relay and competing-peer cancellation.
3. Measure actual concurrent large/small bandwidth progress and combined peak
   sockets/FDs/goroutines/CPU/memory across routes; queue metadata is insufficient.

Continue **W11**. W12 remains dependent on W11 completion; WG5/M3 are incomplete.
Hosted profiles/services, physical WAN/Pi acceptance, T13 login/logout/boot checks
and deferred P17 owner use/explanation remain outstanding. Prior P/O/T and W00–W10
evidence is preserved. No personal folders or live VPS service workloads were
faulted; no commit, publication or deployment was performed.
