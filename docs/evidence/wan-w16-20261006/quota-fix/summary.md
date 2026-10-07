# W16 native defect: service budget exhaustion and roaming control — 2026-10-07

State: **fixed in the client, unit/integration validated; native re-validation
recorded in [native-hosted](../native-hosted/summary.md).** The operated service,
its limits and the wire protocol are unchanged; no service redeployment was made.

## Symptom

Hosted-default runs between the Pi (home) and VPS namespaces (runs 3 and 4):
with UDP blocked in the VPS namespace, Pi → VPS synced over relay but VPS → Pi
never reached the Pi in 180 s. Neither side had a connected peer route at
timeout, the VPS logged repeated inner-TLS handshake timeouts on relay streams,
and the service's quota refusals rose (run 5 timeline: the VPS daemon reported
`QUOTA_EXCEEDED` for ~80 s after an address change; 49 quota refusals).

## Causes found (with reproducing tests)

| Cause | Reproduction (fails before the fix) |
| --- | --- |
| Client allowed 4 concurrent signed operations; the service allows 2 outstanding challenges per device | `TestWANW16ConcurrentSignedOperationsStayWithinChallengeBound` |
| A caller cancelled after its challenge was issued strands it for 60 s; two strand all metadata, including renewal | `TestWANW16CancelledCallersLeaveNoOutstandingChallenge` (service delays the challenge response so the deadline always lands in the window; fails with the detach disabled) |
| No client pacing: the 10th fast operation is refused | `TestWANW16SignedOperationsArePacedToTheServiceBudget` |
| Relay setups refused midway waste both devices' budget | `TestWANW16RelaySetupIsAdmittedWholeOrNotAtAll` |
| Any quota refusal of a renewal withdrew readiness although the accepted record was live | `TestWANW15RejectedAnnouncementKeepsAcceptedOfferGeneration` (updated: transient keeps readiness, semantic refusal withdraws) |
| After an address change the control websocket fails silently; offers and accepts use it until the 75 s stale timeout | `TestWANW16AddressChangeRebuildsControlWithBackoff` |
| Pair limit of two relay tunnels is shared by both directions; one initiator holding two warm tunnels locks the peer out of its direction (run 12: 115 s; runs 3–4: >180 s) | `TestWANW16OneRelayTunnelPerTargetDirection` (fails with the slot widened to two) |

Native diagnosis used two temporary, uncommitted instrumented builds (signed
operation kinds/results); they were never committed or left in `dist/`.

## Fix

See [architecture](../../../orbit-wan-architecture.md#w16-native-corrections-service-budget-and-roaming-control)
and [protocol](../../../orbit-wan-protocol.md#rendezvous-and-coordination). In short:
two challenge slots with typed local backpressure; a six-token client bucket at the
service rate with one token reserved for renewal and local `QUOTA_EXCEEDED` after
≤1.5 s; cancellation honoured until the service connection is ready, then the
signed operation completes; whole-setup admission for relay offer/accept → reserve
→ attach; readiness kept on transient renewal failure; endpoint/control rebuild on
actual network change with 2/4/8 s backoff; one initiator relay tunnel per target
(explicit diagnostic relay probes bypass the slot). The W16 runner's restarts now use the
product's default reconciliation interval instead of the inherited forced 1 s.

## Validation commands

| Command | Result |
| --- | --- |
| `go test -race -count=1 ./internal/network ./internal/rendezvous ./internal/replication ./internal/scheduler ./internal/app` | pass ([log](race-suites-4.log), with the tunnel slot) |
| `make check` (final source) | pass in 25.9 min ([log](make-check-3.log)); earlier runs failed on W12 doctor relay probe ([log](make-check.log)) and W10 NAT matrix ([log](make-check-2.log)), both fixed |
| `go test -count=1 -timeout 30m ./tests/terminal -run 'TestWANW11\|TestWANW15WholeDaemon\|TestWANW16\|TestWANW13\|TestWANW04\|TestWANW09'` | pass in 368 s ([log](terminal-wan-2.log)) |
| `TestWANW03ControlFloodAndShutdown` | 0.02 s (a first, blocking-admission version took 195 s and was replaced) |

Earlier logs ([first](race-network-rendezvous-replication.log), [second](race-suites-2.log))
are retained as intermediate results.
