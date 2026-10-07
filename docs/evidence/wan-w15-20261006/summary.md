# W15 integrated failures, security and resources

Status: **complete for the recorded local/native/emulator/Pi acceptance**. Final
`make GOFLAGS=-v check` and uncached full race pass on the frozen source.
The owner-authorized scope uses disposable local/Pi
namespaces and in-process emulators; no deployed VPS faults or physical WAN
acceptance are claimed.

The [scenario matrix](scenario-matrix.md), [actual command index](commands.md),
[provenance manifest](manifest.json) and [quantitative measurements](resources.json)
provide the case-level evidence. Reproduction commands and privilege/root/child
rules are in the [runner guide](../../../scripts/validation/WAN_FAILURES.md).
Failed and interrupted campaigns stay recorded without acceptance credit.

## Implementation and diagnosed repairs

- A minimized red test showed the shared destructive-target validator accepted a
  symlinked root. It now refuses root aliases before any fault action. The Python
  worker independently validates the exact marker, canonical private owned root,
  remapped namespace ownership and direct child. Its transcript check refuses
  zero-match, skipped or missing required suites; safety checks survive Python `-O`.
- Actual process fixtures combine verified-chunk and committed-receipt SIGKILL,
  blocked TCP/UDP, QUIC/relay recovery, interface/default-route changes and service
  shutdown/restart. Oracles compare immutable heads, authors, manifests, working
  bytes, identity/key pins, readiness and the exact large-version endpoint receipt.
  Namespace/firewall mutations unwind between subcases, including failure paths.
- An independent symbolic transfer model predicts readiness and receipts at five
  production boundaries. QUIC→TCP recovery reuses verified chunks after protected
  aggressive GC. Separate fixtures combine post-rename publication recovery,
  offline conflicts and cached-transport refusal after retirement.
- Impaired native runs exposed repeated stale generations and quota starvation.
  Saved service-code traces show sender offers using a generation the directory
  never accepted. A small independent HTTP service oracle reproduces this with a
  quota-refused renewal. The runtime now publishes its offer generation only after
  acceptance, and suppresses new lookup/relay/ICE service work while its purpose
  announcement is unready. Existing authenticated transports can continue. Ten
  race repetitions pass. The owning network protocol documents this behavior.
- The earlier full race exposed Local-only fixture startup queries taking the
  stopped-adapter state lock before the child daemon. Readiness polling now uses
  live HTTP only. The existing two actual-binary tests are repeated with race.
- The native ICE fixture assumed quota refill before its bounded failed-gather
  probe. It now waits only after an explicit typed quota refusal, then keeps the
  original sub-two-second failed-gather oracle. Temporary diagnostic plumbing was
  removed; its compiled fixtures and sanitized traces remain evidence.

## Retained failed experiments

`safety-red` and `announcement-red` are minimized production-defect reproductions.
The first `full-race` found the live/stopped startup polling race; the first
`aggregate` ended at quota-stalled self-host readiness. The repaired final runs
are tracked separately. Early `recovery` selected an older small-file receipt;
its arm now requires the large transfer's verified chunks. Early `mtu` and
`impaired` left the first subcase's address/firewall mutations; cleanup now
restores the owned namespace even after a failure. `impaired-fixed` and the first
Pi impaired repetitions exposed the production announcement/quota defect.
`model-boundaries` refreshed the generation but retained a stale fixture client;
the corrected fixture constructs a fresh authenticated transport. `native-ice`
assumed quota refill too early. `runtime-focused` expected service-unavailable
where the existing shared quiet period correctly returns quota; the assertion
now accepts either local refusal and still forbids service traffic. Interrupted
recorder sessions and the mistyped snapshot invocation receive no acceptance
credit. No failed or interrupted record has been overwritten by a passing retry.

## Results and limitations

Recorded successes include 19 optimized Python safety tests; repeated independent
model/GC/publication/conflict/retirement race checks; native public IPv4/IPv6 and
ICE/STUN IPv6/fallback; four real enrollment SIGKILL boundaries; native MTU 1280;
Pi whole-daemon recovery and repaired laptop/Pi impairment; laptop/Pi three-route fairness;
four 30-second fuzz campaigns; local demo and operator package builds. Final
aggregate passes in 1,480.961 seconds and uncached full race passes in
1,391.455 seconds, each with identical before/after source digest
`7b5d56630478df744cf379927ed2773add98a5df295170065bad6973741a6a23`. Repaired
laptop impairment passed both boundaries in 245.4 seconds; Pi impairment passed
both in 361.4 seconds.

Final native ICE/fallback passes after the runtime repair. Its sampled lifecycle
closes from 31–35 FDs / 71–85 goroutines to 12 FDs / 7–8 goroutines while the
fixture repositories and service remain open.

The 16 MiB fairness version completed on relay/TCP/QUIC while continuous small
versions progressed under the shared 2 MiB/s cap. The laptop race fixture's large
completion times were 7.60/7.33/7.57 seconds; Pi non-race times were
15.57/16.10/13.51 seconds. These are workload-specific completion times, including
initial token bursts, and do not establish steady-state or physical WAN throughput.
The fairness process includes three peers and a local service. Separate sampled
production daemons in repaired impairment peaked at 141.7 MB RSS / 11.6 MB heap /
25 FDs / 58 goroutines on the race laptop and 35.2 MB RSS / 7.7 MB heap / 28 FDs /
59 goroutines on Pi. Their roughly 4.75-second sampled idle intervals used
0.38–0.51 CPU seconds on laptop and 0.37–0.44 CPU seconds on Pi; these include
metrics, networking and the reviewed frequent poll setting. Separate process
sampling records daemon CPU, idle intervals,
RSS, heap, FDs and goroutines, with relay ciphertext byte totals and route waits.

Binary hashes for 21 native campaign invocations are in
[campaign-binaries.json](campaign-binaries.json). Pi invocation/native records
retain exact commands, topology and elapsed duration; their original recorder
did not capture UTC start/source digests. The
[Pi runtime compilation](compile-pi-runtime.json) identifies the repaired
production source with the temporary test trace; final source removes that trace
and fixes the separate Local-only startup test. Other Pi compilation records
identify earlier recovery/fairness fixtures.

Host/kernel/filesystem, Go and pinned library versions are in the manifest and
host records. Laptop uses race instrumentation; Pi uses non-race cross-built
fixtures. Daemon samples are 250 ms; combined fairness samples are 25 ms. Short
peaks may be missed. Daemon journeys use explicitly reviewed 500 ms poll / 2 s quiet
/ 10 s reprobe timing and 1 MiB/s payload budget; default timing and universal NAT
success are not inferred. Native sockets use synthetic addresses in namespaces
with no external route. NAT shapes use Pion's virtual-network composition.

Process SIGKILL proves process recovery, not hardware power-loss durability.
Static bundled-profile native expiry, physically separate native WAN/N10 and
ordinary hosted-default setup remain unexecuted here. W16 owns real-network
acceptance. WG6 stays open for the second offline authority copy and received
alerts; wider distribution, W16 hosted-default acceptance and W17 release stay
gated. Unfinished T13 technical lifecycle checks and deferred P17 owner use and
explanation remain incomplete.

Final acceptance preserves all 2,277 inherited evidence files and unrelated
starting changes. Marked native campaign roots were removed after artifacts were
copied and live-process absence validated. No further W15 work remains within
this recorded scope. Next packet is W16; hosted-default acceptance still requires
closing WG6. W17 also retains the relevant T13 technical obligations.
