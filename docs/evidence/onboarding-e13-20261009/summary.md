# E13 — real Leave and reviewed Remove Device

Status: complete and installed in orbit-trial, 2026-10-09.

The owner selected real local Leave, with files preserved, and authorized trial
installation on PC, laptop and Pi. Existing E11/E12 2.3.0 polish is retained;
Leave/Remove now has its own E13 packet. Changes remain uncommitted.

Leave cancels/drains this Orbit's exchanges and workspace writers, persists a
left marker, unregisters its root and blocks new/retried sync across restart.
It keeps working files, history, journals and identity; other Orbits continue.
Pinned peers get one deduplicated PEER_LEFT attention item opening removal.

Removal requires the exact name and reviewed membership/retiree history digest.
The card counts recorded changes received here and says the remote total is
unknown. All survivors must prepare an identical received-retiree set, then
commit with an atomic recheck. Offline/divergent survivors and interrupted
rollout remain explicit and resumable. Prepared successors can catch up from a
pinned peer; unprepared/fork/rekey changes remain refused. Learned removal stops
this Orbit and offers Leave. The actual initiator is retained per retired device;
legacy manual removals name the reporting peer without inventing an actor.

Evidence: [acceptance map](acceptance.md), [commands](commands.md),
[results](results.json), logs, real lifecycle PTY results and local WAN-service
PTY captures. Focused E11–E13 race and final E13 race passed; broader core race
has combined passing evidence after the corrected integration preview rerun.
The full repository/scheduler/workspace/terminal/CLI race rerun passed.
Final quick passed all five PTYs (125 s). Integration, model, design-gate/fault,
36 harness tests, both architecture builds, extracted-package/install/PTY checks
and final gofmt/vet passed. Packages now derive their schema metadata from the
repository and check executable/manifest agreement.

The original full terminal run and corrected reruns cover every former failure.
W16's TUI helper needed a shorter fixture socket with the cache-based TMPDIR;
its three-device rerun passed (343.200 s). Final review also reproduced/fixed a
stale pending-removal retry that waited on survivors before requiring new local
confirmation. The new regression verifies needs_review, unchanged membership,
cleanup release and acceptance of a fresh reviewed operation. No uninterrupted
successful make check/full terminal rerun is claimed; interrupted checks are
not passes.

Final installation and all three read-only host verification helpers passed.
PC and laptop run 2.3.0/schema 15 with identity, roots, membership and history
preserved. Their ordinary-file manifests were empty; working-byte preservation
has separate real-PTY and fault evidence. The Pi executes the updated arm64
binary natively and remains unconfigured/stopped. Private prior-binary and
consistent metadata backups are retained on each host. Default owner installs
were not changed. Changes remain uncommitted.

Limitations: no Undo Leave; retired IDs cannot rejoin, requiring fresh identity
enrollment in separate state while preserving other memberships. Removal needs
all survivors reachable with matching accepted retiree metadata and upgraded
support. Notification on an offline removed device waits for peer contact.
Already admitted requests may finish under the existing per-request retirement
rule. New retiree history arriving at a survivor after another committed can
require reviewed recovery of preserved groups/files; retry cannot expand an
irrevocable retirement snapshot. No distributed lock/consensus/rollback is added.
Maximum-size artifact/scale experiments, package container transaction emulation,
a new physical WAN fault campaign and owner Leave/removal walkthrough are
unexecuted. Existing content retention/budget rules still apply.
