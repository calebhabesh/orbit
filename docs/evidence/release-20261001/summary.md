# Release validation correction — 2026-10-01

Implementation is working; full release acceptance is still in progress.
The original estimated savings/power-cut claims are withdrawn. This campaign
records real resets, encrypted traffic counters, actual interrupted transfers,
and native service behavior. Scope and guarantees remain unchanged.

## Executed evidence

- Local unit/model/integration/design-gate/process-fault suites passed.
- `make check` passed after the scheduler test waits for its own durable result;
  [first failure](make-check-before-scheduler-wait.log),
  [30-repeat check](scheduler-regression.log), [full result](make-check.log).
- `make test-race` passed; [result](test-race.log).
- `go mod verify` passed, `govulncheck` v1.8.0 reported no vulnerabilities,
  and npm audit reported zero findings;
  [Go integrity](go-mod-verify.log), [Go vulnerabilities](govulncheck.log),
  [npm findings](npm-audit.json). These are point-in-time tool outcomes.
- Frontend `npm ci` and `npm run build` passed; embedded output is unchanged.
- [Final QEMU/KVM reset campaign](reset-final/abrupt-reset.json): 16/16
  cases passed, comprising the dirty-cache negative control and 15 actual
  object/version/publication boundaries. [Command output](reset-final.log).
  This discards guest caches in new marked ext4 images; host storage stays
  running. Physical Pi/VPS power cuts are unexecuted.
- [Actual workstation/Pi/VPS demo](workstation-demo-final/three-host.json)
  passed. The laptop alias was unreachable, so this is explicitly a
  workstation campaign. It asserts three heads and matching tokens, reviewed
  resolution, late C conflict and stale rejection, forwarding while A's
  listener is stopped, actual mid-file receiver termination/restart, reuse
  of completed verified chunks, whole-file hashes, restore, and restart.
- [Native service lifecycle](lifecycle/service-lifecycle.json) passed on
  workstation, Pi and VPS: fresh isolated install, process-changing restart,
  embedded UI HTTP response, removal of only the unique service, preserved
  state/working bytes and SQLite integrity. Explicit path/port overrides use
  the shipped service template. No system-wide package manager or lingering
  settings were changed.
- [Measured smoke benchmark](benchmark-smoke/benchmarks.json) passed nine
  synthetic workloads with 20 files and a 40-MiB archive. The unchanged and
  deletion cases favored the baseline; prefix insertion defeated chunk reuse.
  It predates the path-scaling fix and is not the final release measurement.
- [Inventory regression](benchmark-default-before/benchmarks.json) exposed
  the 1,024-version rejection. A [larger pre-fix scan](scan-before-cancel.json)
  was stopped after observed slow progress; that failure is preserved in
  [raw output](benchmark-before-scaling/benchmarks.json).
- A 1,000-directory capture microbenchmark measured 14.402 seconds before
  and 0.104 seconds after narrowing path operations, one sample each;
  [before](history-profile-before.log), [after](history-profile-after.log),
  [CPU profile before](history-profile-before-top.log),
  [CPU profile after](history-profile-after-top.log). This is not a general
  synchronization speedup claim.

## Corrections in the source

Remote roots are newly created/private with exact markers. Signal targets
must match PID, process start time, executable and state path; name-wide kill
and pre-marker deletion are gone. Negative safety tests exercise the worker.

Inventory is paged into a quota-checked spool rather than rejected at 1,024
folder entries. Per-fetch ancestry stays bounded; explicit backpressure has
bounded cancellable retries. Completed sender snapshots release capacity.
Per-path history/status/capture reads use the existing path index; global
immutable IDs and rollback-counter refusal remain checked across all paths.
Structural and GC analysis continue to use the whole folder.

The default service now agrees with `init`'s state path and enables loopback
control/UI delivery. The standalone system installer points to its actual
installed binary. Package metadata records a source revision rather than the
literal word `release`.

## Outstanding release acceptance

The full 10,000-file/1-GiB synthetic campaign is running on the corrected
engine. Its final raw samples/results must be recorded before claiming the
full benchmark matrix is demonstrated. Timing runs share this development
host with validation work and do not isolate CPU/filesystem caches.

The configured laptop address `192.168.88.83` returned “No route to host” on
two read-only SSH checks. A reachable laptop SSH address is required for the
prescribed laptop/Pi/VPS campaign. The owner has not provided evidence of real
personal use; automated scripts cannot establish adoption. Record the actual
start/end dates, normal edits, offline/reconnect cycle and restart before P17
completion. Owner explanation without agent assistance also remains unexecuted.

All generated roots are dedicated synthetic fixtures. Remote roots remain for
inspection; only run-owned daemons/services are stopped. Existing pilot folders
and unrelated VPS workloads were preserved. See [commands](commands.md) and
[packet status](../../implementation/status.md) for the resumable handoff.
