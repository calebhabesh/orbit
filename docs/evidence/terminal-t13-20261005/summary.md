# T13 follow-up — 2026-10-05

T13 remains **in progress**. T00–T12 are complete; this extends the
[previous release evidence](../terminal-t13-20261004/summary.md).
Personal use and comprehensive owner review remain deferred and do not block
delivery. Original workspace/index, pilot data and existing services are preserved.

## Network prerequisites and deployment recommendation

Initial read-only inspection, before the owner installed Tailscale, found it
absent on laptop, Pi and VPS. At that inspection all user managers had
`Linger=no`; the three preserved pilot services
are running. Laptop/Pi route the VPS private address through their ordinary home
gateway, while the VPS has no private route back to their LAN addresses.
The [preflight](tailscale-preflight/terminal-private.json) records routes and
unauthenticated status for all three; it creates no test roots or services.
It fails the Tailscale prerequisite explicitly, rather than claiming a journey.

The owner asked which deployment best serves the distributed-systems scope.
Recommendation: one personal Tailscale network with a client on each Orbit host,
ordinary writable copies on all three, login startup on the laptop and deliberate
unattended startup on Pi/VPS. Tailnet admission and Orbit folder approval remain
separate. The agent made no VPN installation, authentication, firewall/DNS policy,
lingering, logout or reboot changes. The owner subsequently installed and
authenticated Tailscale on all four devices; the native lifecycle environment
remains needed. The
[network runbook](../../runbooks/private-network.md#recommended-personal-deployment)
records the concrete setup and official sources. Existing WireGuard entry to the
home network is preserved.

## Retirement finding and correction

The ordinary journey reached matching retirement previews, CLI-generated export/
approved survivor import, and successful retirement replay. A retired identity's
new request remained pending; request submission grants no membership. Attempting
its exact approval correctly failed canonical membership validation, but returned
`INTERNAL_ERROR` with a generic retry action. The
[minimal native-controller probe](local-private-01/retired-approval-probe.json)
and [red HTTP regression](retired-approval-before.log) retain the finding.

The owning enrollment-v2 controller now checks the reviewed requester against
retired membership entries before constructing an approved revision. It returns
`RETIRED_MEMBER_REVIVAL` and the existing fresh-identity recovery action. It does
not change accepted histories, membership semantics, keys, counters or schemas.
Declining the request remains possible. The
[ordinary regression](retired-approval-after-03.log) passed in 13.040 s; the full
[initial seven-case T13 race run](t13-race.log) passed in 49.126 s, including instrumented
child binaries. The test verifies repeated refusal, unchanged membership, signed
pending status without an admitted membership and successful request dismissal.
Two intermediate post-fix tests exhausted real per-IP status tokens; the final
fixture waits for replenishment and queries the owning controller for dismissal.
No safety assertion was removed.

## Validation executed

- [Control/contract/CLI tests](control-cli.log): uncached pass.
- [Initial package build](package.log): amd64/arm64 packages produced after the
  error-reporting fix, before the later replacement-bootstrap correction.
- [Package-extracted scenarios](package-scenarios.json): all selected native
  payload, standalone installation/removal and bare PTY scenarios passed.
  Container transactions/arm64 emulation were not selected in this follow-up.
- [Python safety/VT tests](python-safety-final.log): all 12 passed, including
  refusal before commands for invalid/public/loopback preflight addresses and
  marker fencing before network probes. Python compilation and diff checks pass.
- [Test discovery](test-discovery.log): eight `TestTerminalT13` cases exist.

The new [ordinary private-network runner](../../../scripts/validation/terminal_private.py)
uses package-verified binaries, exact reviewed setup/approval, direct network
sockets and authenticated history queries. Forwarding stops the receiver during
source-to-forwarder transfer, then stops the source before restarting the receiver;
the receiver must receive identical original-author version IDs and bytes.
Retirement requires identical exact survivor previews and uses an unmodified
CLI-exported membership bundle, rather than synthesizing certificates/membership.
Replacement uses a fresh installation/key, preserves the retired root/state,
rejects retired-key approval, and compares active/retired membership and histories.

Initial runner attempts remain archived, with their unsuccessful outcomes and
empty cleanup-error lists. `local-private-01` inspected the pending request list
too early, before testing approval. `native-lan-01` could not reach the workstation
enrollment socket. `local-private-02` and `native-lan-02` reached exact retired-key
refusal but hit normal per-IP throttling during replacement. The runner now
observes the same durable throttled job through its daemon retry, preserving the
operation/attempt/request identities; it never generates replacement retries.
Final journey results are recorded below.

The subsequent replacement attempts exposed a genuine bootstrap gap:
`local-private-04` reached the correct approved membership and preserved the
new root, but rejected known retired-author history because its canonical
snapshot entries had not accompanied enrollment. The
[minimal red regression](replacement-before.log) reproduces it over real
authenticated enrollment/peer transport. The controller now obtains the exact
admitted revision's hash-bound retirement artifacts through the existing peer
interface before importing history. Repository replay can atomically hydrate
missing artifacts for an unchanged approved revision, so older blocked jobs
can resume. No new endpoint, schema, membership authority, acceptance bypass
or relaxed rate limit was introduced.

The [ordinary fresh-bootstrap regression](replacement-after-02.log) passed in
25.064 s. The enhanced regression also seeds the previously approved membership
without artifacts and reopens the joining owner, checking actual recovery of
that interrupted/older state. All **eight** T13 cases passed with instrumented
children in [73.545 s](t13-hydration-race.log). Repository/control/CLI race checks
also passed in [this run](hydration-repository-control-race.log), including wrong
snapshot rejection, unchanged membership after hydration, idempotent replay,
known-envelope admission and continued unknown retired-history rejection.

The final runner schedules a 60-second quiet period before generating the
replacement's signed attempt, then uses the other active survivor for approval.
This respects the unchanged per-IP admission budget. Earlier attempts preserve
the real throttling/expiry outcomes: an unsent throttled transcript can expire
and requires a deliberately fresh review. The runner never mints such retries.
The earlier native attempt's stored `context canceled` outcome was captured
after scoped cleanup; the local minimized reproduction identifies the actual
missing-artifact cause. Sanitized attempts and the earlier runner source remain
archived.

## Current packaged ordinary journeys

After both production fixes, [packages were rebuilt](package-hydration.log).
The [local journey](local-private-05/terminal-private.json) passed in 153.065 s
using three installations on one workstation's nonloopback private address.
The [native LAN journey](native-lan-05/terminal-private.json) passed in 291.666 s
using one amd64 laptop installation and two arm64 Pi installations, followed by
a fresh Pi replacement. Both runs observed ordinary approval, matching membership,
all preexisting files, original-author forwarding while source and receiver never
overlapped, exact reviewed retirement/export/import/replay, explicit retired-key
approval refusal, preserved retired data and fresh replacement histories.
Both ended with no owned-worker cleanup errors. They do not prove the missing
three-physical-host or Tailscale conditions.

`native-lan-04` found a harness observation deadline: an 8-second controller read
can wait behind the owner's 10-second bootstrap transaction. The final runner
retries that exact timeout once; other failures still abort. Completion requires
observed membership, original version IDs and file bytes. The local run uses the
snapshot runner hash; the final native run uses the later observation-retry hash.
Each report records its runner and native payload hashes. No product timeout,
rate limit or completion criterion was relaxed.

## Isolated current-source release reproduction

An isolated source snapshot was committed as
`00992f6d8b6afcde7dfaffe2c8aad8c27c847602`, then cloned into a clean marked checkout.
The [snapshot manifest](reproduction-hydration/snapshot-manifest.json) preserves
every input; the original branch and index remain untouched. Current production
source matches this snapshot, as recorded in the [source audit](source-audit.json).
Only the later harness observation retry and status/network documentation differ
outside evidence files. Native development packages carry the original `61da64f` build
label; clean snapshot packages carry the isolated candidate label and therefore
have separate hashes. Neither label substitutes for the source manifest.

The [clean reproduction](reproduction-hydration/clean-release/reproduction.json)
passed every stage: `make check` (427.656 s), uncached full race (382.094 s), demo,
all 16 [VM reset cases](reproduction-hydration/clean-release/reset/abrupt-reset.json),
all five [storage exhaustion cases](reproduction-hydration/clean-release/disk-full/abrupt-reset.json),
checksums and identical repeated packages. No tracked candidate source changed.
The VM outcomes retain their virtual ext4/durability limitations; they do not
establish physical Pi media power-loss behavior. Exact commands, outcomes and
provenance are indexed in [commands](commands.md), [results](results.json) and
[manifest](manifest.json).
Final read-only service observations confirm that the preserved laptop, Pi and
VPS pilot services are still running; their logs are linked in the results index.

## Remaining acceptance and provenance limits

### Authenticated Tailscale and actual three-host journey

After the owner installed/authenticated Tailscale, the
[read-only authenticated preflight](tailscale-authenticated-preflight-01/terminal-private.json)
passed on laptop `100.101.0.12`, Pi `100.101.0.14` and VPS `100.101.0.11`.
Every host reports `Running`, authenticated/online self state, the expected local
Tailscale address and routes to both peers through `tailscale0`. Preflight alone
creates no installations and does not claim application transfer.

The [actual three-host journey](tailscale-three-host-01/terminal-private.json)
then **passed in 277.636 s**, using the clean candidate's checksum-verified
amd64/arm64 packages on three distinct physical hosts. It establishes ordinary
create/invite/join/exact approval with restart/delayed approval, TCP reachability
to every peer/enrollment listener, preservation of existing files, ordinary edits
on every host, original-author forwarding through the VPS while laptop and Pi
never overlapped online, exact reviewed Pi retirement/export/import/replay,
explicit refusal to approve the retired key, and a fresh Pi replacement with
matching membership/history/bytes. The retired installation's data remains
preserved. No owned-worker cleanup errors occurred; marked roots are retained.
Package hashes match the isolated passing release candidate exactly. Existing
WireGuard configuration and unrelated services were untouched by the runner.

Authenticated Tailscale and ordinary **laptop/Pi/VPS** private-network onboarding/
forwarding/retirement/replacement are now satisfied. Actual native
login/logout/unattended boot remains **unexecuted**, requiring a designated
disposable native environment. The existing hosts' unrelated workloads are not
reboot targets. Manual-start test workers, service restarts and connected
Tailscale clients do not substitute for Orbit lifecycle evidence.

The previous clean-release candidate `e37e2600cd236776d5161f3a293718098bd7500e`
does not contain these later corrections. Current package/source hashes
are in [manifest](manifest.json); earlier results retain their original candidate
scope. Complete T13 only after the missing native lifecycle
conditions have genuine evidence, with current candidate validation recorded.
