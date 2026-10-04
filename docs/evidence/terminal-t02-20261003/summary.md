# T02 shared client and daemon lifecycle

T02 implements the shared authenticated live/stopped client and the scoped
lifecycle/settings production boundary. Existing relocation changes, P/O
history and P17 owner-use/explanation obligations were preserved. Enrollment
serving remains T03; TUI entry, native startup and broader family parity remain
T06/T09/T12/T13. No personal services, folders or network policies were changed.

Implemented behavior:

- Numeric loopback endpoint selection, private owner credential files, redirect/
  proxy refusal, device response binding, deadlines and bounded metadata/errors.
  Failed live calls cannot acquire stopped-state database ownership. Existing
  CLI helpers and browser launcher use this transport; both typed adapters
  invoke the owning controller under its required lock.
- One finite initializer for init and auto-initializing daemon paths. Limits
  precede identity creation; an identity config missing beside existing history/
  keys is refused. Legacy missing limits remain visible and can be repaired by
  an explicit reviewed settings operation without rekeying.
- Coordinated concurrent launch/reuse, selected-state readiness, independent
  daemon lifetime, validated persisted peer/network settings and peer listener
  reuse after restart. Stop pins the selected process with pidfd and verifies
  its descriptor to the state lock before signalling.
- Durable operation IDs/fingerprints, current settings/service reviews, replay,
  stale review refusal, expired replay guards, lost-response recovery and
  waiting cancellation that preserves accepted work. External service actions
  claim dispatch once durably and release stopped database ownership; uncertain outcomes require observation
  instead of automatically repeating system commands.
- Distinct running/enablement/mode/root/capture/unattended observations. Service
  actions preserve existing units for other states, report command failures,
  check unattended prerequisites and leave lingering as an owner step.
- `orbit settings runtime` reviews/applies private mutation files and inspects
  operations. Service review/apply supplies the same durable script workflow;
  compatible service verbs remain available.

Evidence: [commands](commands.md), [results](results.json),
[source manifest](manifest.json), [test discovery](discovery-complete.txt),
[twice-run targeted suite](acceptance-targeted.txt),
[final broad check](acceptance-check.txt) and
[broad race gate](acceptance-race.txt).

The first race run caught an existing unsynchronized `Server.httpServer`
publication during concurrent Serve/Shutdown. A minimal real server regression
failed under race before the fix and passed afterwards. The server is now
initialized before publication; [red reproduction](race-minimal-before.txt) and
[green reproduction](race-minimal-after.txt) retain that evidence. The initial
packet race failure only showed the child exit; diagnostic child output exposed
the race. No secret was needed to diagnose it.

An early compile failed for an unsupported ID method and a now-unused import;
these were corrected. Credential hardening exposed test fixture directories
with public modes. An initial broad check also exposed a service failure fixture
that invoked the real user service environment. Fixtures now use private state
and a disposable home/PATH for that negative case; no existing user unit was
overwritten. A final review added durable service dispatch claiming and capability
negotiation. Its overlapping stopped-start regression initially treated expected
connection-not-ready errors during ownership handoff as task failures. The final
assertion drains callers, replays their original ID after readiness, and verifies
exactly one external start plus a completed durable operation. It does not allow
HTTP errors to trigger database fallback. That earlier failed check is retained
in `targeted-dispatch-final.txt`.

Intermediate passing checks predate the final CLI/dispatch additions and
are retained separately; the final named checks establish the current result.
The first `race-targeted-final.txt` was interrupted by session steering during
its service case and is incomplete, not a pass. The resumed focused suite
passed; the final broad race gate covers subsequent edits.

Limitations: local Linux amd64 tests and cross-build/package checks do not prove
native boot/logout, Tailscale/LAN enrollment, abrupt-reset durability or Pi/VPS
behavior. Process service checks run real daemon binaries with a disposable
`systemctl` stand-in. Partial initialization/lost response are explicit boundary
experiments, not a SIGKILL/power-loss campaign. No claim of unsupported retention,
read/upload, setup or TUI behavior follows from the lifecycle subset capability.
Capture health and unattended verification are conservative false in the new
terminal result until their owning checks exist. Runtime tuning/listener changes
are saved with an explicit restart-required effect. Enrollment settings are
stored but the isolated listener is T03. Existing legacy browser/service aliases
and rollback adoption still require native T12 validation.

Schema remains 13 and peer protocol remains 1. New terminal records use the
existing typed metadata namespace. Older binaries cannot interpret the ledger
or runtime budgets; rollback requires reconciled operations and deliberate
budget migration, without database/counter rollback. No ledger pruning runs;
new admission obeys metadata budgets and expired replay guards are retained.

Worker explanation: enabled means login startup is configured; running is an
observation of ownership of the selected state. Neither establishes root/capture
health or unattended boot/logout. A failed authenticated live call says nothing
about release of the daemon's exclusive lock. Switching to SQLite after that
error could create a second writer, so the failure is returned and recovery
continues through the same durable identity. This is worker explanation, not
P17's outstanding unaided owner evidence.

Final cleanup inspection found an existing pairing integration fixture invoked
`stop` through the Orbit alias and ignored failure, leaving disposable daemons
behind. Its cleanup now validates the disposable marker and calls the owning
`app.StopAgent` operation. Seven exact known test processes from this session
were gracefully signalled after pidfd/command/deleted-lock verification; no
personal process was selected. [Cleanup record](process-cleanup.json) and
[focused race regression](cleanup-regression.txt) establish successful teardown
and no remaining session test daemon. Broad runtime gates passed before this
final harness-only repair; that final fixture change passed its focused race
regression. No runtime source changed after the broad gates.

Final validation: 15 ordinary T02 tests passed twice; the helper ran in daemon
children. Final serial `make check` and `make test-race` both exited 0. The
initial interrupted race log is not included in that claim.

Next eligible packet: **T03 authenticated network enrollment**.
