# W11 completion follow-up

**Complete for production integration, local native/emulator and actual Pi acceptance. WG5 closes; W12 is next.** This follow-up preserves the original
[W11 record](../wan-w11-20261006/summary.md) and all P/O/T/W00–W10 evidence.
The production tree was already dirty; [manifest](manifest.json) records its
starting hashes and revision. No commit or deployment is part of this work.

Delivered reviewed Advanced route timing through the shared network-policy
preview/apply control and CLI, with exact durable operation replay and restart
activation. Missing settings retain finite defaults. Invalid ranges, fractional
milliseconds, changed reviews and generation rollback are refused. Cached status
shows desired/active timing, and TUI network details identify the same controls.
No timing expands identity, folder approval or invitation/proof authorization.

Bandwidth admission now precedes every primary and fallback chunk attempt.
Retries spend their own budget, including uncertain delivery. At most 128 waiting
reservations proceed in FIFO order, with cancel removal and typed retryable
backpressure. Existing durable task aging remains in force. These changes repair
token stealing by a stream of tiny reservations and previously unpaced fallback
attempts; they preserve verified-chunk/publication/receipt semantics.

## Executed campaigns

The whole-daemon runner starts the production `app.ServeWithOptions` lifecycle in
separate child processes, including actual scheduler/control/TLS/QUIC/ICE/WSS,
scoped LAN discovery and network observation. Explicit invitation, pending denial,
exact approval and bootstrap precede networking. The runner verifies remapped
user/network-namespace ownership plus a private regular disposable marker before
any firewall/address/default-route/netem action. All changes occur in that new
namespace, including on the owner's Raspberry Pi 4B accessed as `ssh rpi`.
Existing Pi services, folders, firewall and sysctl values are untouched.

The campaign transfers a real 16 MiB version at a configured 1 MiB/s payload
budget, switches relay→QUIC, changes an address/default route during transfer,
switches back to relay, then returns to QUIC. Both daemons inject one failure after
a committed receipt send; the existing task retries a typed unexpected EOF.
Exact heads/authors/manifests/working bytes, persistent device keys, target pin and
durable receipts remain verified. This injection models loss of caller success
at the receipt boundary; it does not physically drop a particular HTTP response.
The earlier native TCP↔QUIC/chunk/receipt campaign remains separate evidence.

The marked namespace latency fixture applies real Linux netem 25 ms packet delay
and 1% packet loss. These are synthetic topology/loss conditions on native sockets;
they do not establish physical Wi-Fi/CGNAT/internet success or operated hosting.
Whole-daemon runs have process RSS/heap/FD/goroutine/CPU samples every 250 ms.

A separate three-peer campaign uses the actual sync engine, immutable chunks,
folder membership and pinned WSS/TCP/HTTP3 transports. One peer transfers 16 MiB
while another continuously creates and transfers small versions. Both contend
for one 2 MiB/s global payload limiter, including its one-second initial burst.
Each transport has actual route, byte, head, author and manifest oracles; sampling
every 25 ms measures the combined local test process (three replicas plus service)
heap/FD/goroutines and process CPU. FD totals bound socket FDs as well, but are not
an exact socket-only count. Measurements are fixture capacity, not production
scaling curves or machine-independent latency guarantees.

Local race run: relay/TCP/QUIC complete the large version in 7.63/7.30/7.57 s,
while 17/11/8 new small versions finish. First small completion is
0.108/0.698/0.777 s; first verified large chunk is 0.261/0.064/0.078 s. Combined
peak sampled heap is 43.7/65.5/42.6 MB, FD 59/53/48 and goroutines 119/99/106.
Process CPU during measured portions is 5.37/5.02/5.70 s. Race instrumentation
is included in local resources. Pi non-race results and full daemon measurements
are recorded in [logs](logs/pi-bandwidth-current.log) and
[resource summary](resource-summary.json); the Pi's existing load/storage/kernel
socket buffers are retained, so timings are not comparable as isolated CPU scores.

A 32-peer slow DNS/relay competition exercises production ServiceClient lookup,
its bounded resolver and manager route admission. Two race runs retain at most
four DNS workers and 31 observed relay attempts against the 32 admission limit;
cancel/close joins in 1–9 ms with zero retained pools/requests/dials/resolvers/relay
workers. The resolver and attachment completion are deliberately blocked, while
netem separately provides actual latency/loss on TLS/WSS service connections.

## Failures and limitations

Initial compile errors in test callback/type and log formatting, private review-file
reuse, and a firewall rule missing the service reply direction remain retained.
The first fairness relay warmup hit the unchanged metadata quota; bounded typed
quiet-refill retry establishes each tunnel before timing data competition. Quotas
are never disabled or enlarged. An initial local loss run with an untyped injected
receipt failure failed; the fixture now emits retryable unexpected EOF, matching
uncertain stream delivery. A passing retry does not turn that failure into evidence.

Interrupted aggregate/Pi fairness commands during turn continuation are unexecuted
for completion. Final commands/results distinguish accepted, failed,
interrupted and skipped outcomes. Pi default timing validation and final check-target evidence are complete.

Hosted defaults/W13 operator readiness, physical multi-host WAN/W16, combined
release/W17, inherited T13 login/logout/boot checks and deferred P17 owner
use/explanation retain their separate obligations. No packet beyond W11 has been
started.

The final serializer omits zero timing entirely, preserving exact legacy policy
bytes embedded in durable review/operation fingerprints. An independent old-policy
JSON oracle verifies this; nonzero reviewed timing remains included. Broad
control/contract/config/CLI race checks cover the final serializer. Packet transport
and resource campaign binaries retain their recorded source stage; this final
serialization repair does not change routing, pacing or socket behavior.

The original W00 link-check script contains a frozen W01 tracker assertion and
cannot validate current status. An all-history exploratory link scan also finds
an older O05 anchor mismatch outside this task. The scoped current link check
validates owning W11 specifications and this follow-up, without rewriting either
historical artifact. Both exploratory failures remain in the logs.


Final scoped netem campaigns pass: local race whole-daemon journey 138.83 s and
Pi production-default journey 203.80 s. Local relay→QUIC is 11.35 s, address/default
route detection 0.859 s, direct→relay 7.20 s and final relay→QUIC 22.69 s. Pi defaults
measure 26.12/3.59/7.47/49.18 s for those stages. Route waits begin after fixture
state changes or prerequisite byte convergence and are not end-to-end outage SLAs.
Local sampled daemon peaks are 24/25 FD, 67/68 goroutines and 144/150 MB RSS including
race instrumentation. Pi default peaks are 26/27 FD, 58/61 goroutines and 33.3/34.1 MB
RSS; CPU totals during sampled lifetimes are 12.85/20.50 s. These successful Pi
measurements support retaining the initial finite defaults, while reviewed 500 ms
poll/2 s quiet/10 s reprobe settings demonstrate faster local recovery. No host
buffer, CPU governor, thermal or existing service setting was changed for tuning.

The first scoped netem setup reused the IPv4 filter priority for IPv6 and was
refused by the kernel before the daemon journey. Distinct filter priorities repair
that harness error. The earlier default Pi run impaired loopback control as well
and failed an owner-control dial; the final fixture exempts loopback owner control
and impairs actual peer/service traffic. Both failed runs remain uncredited.


The aggregate terminal suite and package extraction pass; the later integration
failure and repaired final check targets are recorded below. The early
aggregate invocation was interrupted during continuation, not passed or failed.
The resumed command and all subsequent final focused checks are the completion
record. Pi test directories were removed only after copying artifacts and
validating canonical/private/owned/marked roots plus absence of active fixture
processes; [cleanup](logs/pi-cleanup.log) and [final socket-fixture cleanup](logs/pi-sockets-cleanup.log) name those allocated roots.

Next eligible sequential packet after W11 completion is **W12 — qualified network
status, diagnostics and controls**. Read its packet/dependencies before work;
W13's operated services/default profiles, W16 physical topology and inherited T13
checks remain required in their owning packets. No W12 source is implemented here.


The resumed full `make check` passed the terminal suite (1,038.603 s), package
extraction, formatting/vet and complete internal/model unit suites, then failed
`TestOrbitPairing_CLI_RunningDaemon_Parity`: a read-only control operation poll
expired while the durable join remained in progress. The unchanged focused
reproduction passed in 25.96 s, so this is not reported as a deterministic routing
regression. A minimized timeout-poll oracle independently reproduces the CLI
behavior: it aborted on one transient timeout and could discard the last known
operation result. Both minimized tests failed before repair.

The CLI now retains that result and polls the same operation after read-only
network timeouts, bounded by the original wait deadline. Authentication/other
refusals remain errors; no mutation is resubmitted, no invitation/proof is replaced,
and expiration still reports the last known pending state rather than Ready.
Race regression tests pass twice. Repeated real pairing and affected W06/W11 CLI
journeys validate the repair; full integration and remaining check targets are
recorded separately. The failed/interrupted aggregate invocations retain their
actual states. Unrelated long terminal campaigns are not silently reclassified as
a successful rerun of the original aggregate command.

The final bandwidth sampler additionally counts unique process socket identities
and RSS. Local race relay/TCP/QUIC unique socket peaks are 36/32/28 and combined
process RSS peaks 317/374/419 MB. Pi non-race socket peaks are 36/32/34 and RSS
83/103/100 MB; the 16 MiB version completes in 16.89/13.23/16.17 s with 8/5/4
small versions. Pi QUIC's first small completion is 8.36 s; progress is bounded in
this fixture, not guaranteed subsecond for every competing peer. Sampling records
kernel socket identities once even if a descriptor is duplicated; short-lived
peaks between samples can still be missed. [Local](logs/final-socket-bandwidth.log)
and [Pi](logs/pi-sockets-bandwidth.log) retain full measured results.


Final closure: the real pairing journey passes three times (77.888 s), the complete
integration target passes (96.858 s), and formatting/vet/full unit/model/fault/harness/
amd64+arm64 build/package targets pass in `check-remaining-targets.log`. Affected
routed CLI relay transfer and local capture pass under race. Its guided PTY harness
initially mistook PTY EOF before process reaping for timeout after only 4.21 s;
waiting for child reaping within the existing 90 s budget fixes that fixture.
Guided setup and final timing/restart pass twice under race (34.297 s). Neither
product nor harness authorization/timeout was enlarged. Original failures remain.
The earlier full terminal suite (1,038.603 s) plus final focused changed-flow tests
and remaining targets form composed check coverage; **the original full `make
check` exit remains 2, and a second full aggregate was not claimed or rerun**.

Every W11 acceptance item has execution evidence across the original and follow-up
records. Final [results](results.json), [commands](commands.md),
[preservation](preservation.json) and [source snapshot](final-source.json) identify
the result. Hosted defaults/W13, physical multi-host WAN/W16, T13 technical checks
and deferred P17 owner use/explanation remain outstanding. No W12 work, commit,
publication or deployment occurred.
