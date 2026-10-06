# W07 — TUI onboarding and relay milestone

Run date: 2026-10-05. **W07 complete. Production integration, focused acceptance and full
`make check` pass.**

The keyboard TUI uses the existing authenticated shared controls for measured
create/join review, explicit device/folder approval and durable setup progress.
Fresh setup reviews Automatic and service metadata visibility; Local network only
is available before confirmation, and existing installations retain reviewed
policy. Address/port/concurrency settings are under Advanced. Tab/Shift-Tab retain
visible focus, Ctrl-N changes connection choice, and Esc returns to editable review.
No presentation-owned transport, membership authority or receipt is introduced.

Private v2/v3 paste and files share the existing invitation contract. Deliberate
versioned reveal/private save and exact revocation use shared controls. Expired
fresh input is classified before historical certificate checks, without any
network/mutation. Wrong pins, root-review errors and asynchronous polling retain
safe inputs; request/folder selection survives reordered pages and late responses
cannot resurrect an abandoned view. Accepted retries retain operation/attempt.

Connection details (`N`) report desired/active policy, cached service readiness
and actual dated direct/relay observations separately from local capture,
stored/applied copies, conflicts and membership. A relay route never marks files
complete. Interface exit restores terminal modes and leaves daemon work running.

Executed focused validation:

- Five discovered `TestWANW07` model/presentation tests and the existing terminal,
  shared-client and CLI package checks pass under race instrumentation.
- The production-binary keyboard journey passes in **142.840 seconds**, with
  independently selected signed local-development profile/TLS trust and no direct
  listener or manual address setup. It verifies actual bytes in both directions,
  an edit to an existing file, exact version/head/hash/author agreement, saved
  inviter certificate/pin and unchanged device identities.
- The same real PTY journey verifies exact cross-device approval/code, delayed
  client exit and actual daemon restart with the same operation/attempt/request/
  root; second-folder consent/approval and verified bytes; durable revocation;
  masked wrapped v3 paste, wrong pin, expiry, existing-root/unsupported-object
  correction, Back/Edit, narrow/colorless resize and terminal restoration.
- Actual marked service shutdown preserves new-folder drafts and daemon local
  capture. Fresh Automatic with missing profile and Local-only selected before
  confirmation both keep local capture after quitting the interface.
- Full `make check` passes: terminal campaign (925.129 seconds), packages,
  formatting, vet, internal/CLI tests, integration, models, design gates/faults,
  all twelve Python validation-harness checks and release build/package targets.
- Canonical protocol/control-contract/model checks, relative documentation file
  links, initial source preservation and `git diff --check` pass.

[Commands](commands.md), [manifest](manifest.json), [PTY results](pty/results.json),
[focused race](logs/focused-race-final.log), [passing journey](logs/pty-thirteenth.log)
and [aggregate log](logs/make-check-final.log) retain provenance and actual results.
The Go fixture/model race tests are instrumented; launched production binaries
are ordinary builds. Saved frames omit deliberate private transfer output.

Initial failures remain recorded without acceptance credit. Fixture corrections
include command dispatch/private-parent permissions, existing `version` and hex
`Digest` JSON fields, actual folder-page order and wrapped transfer labels. The
expired-v3 error classification repair has a focused regression and passing PTY.
The first aggregate failed solely at its incorrectly typed new digest oracle;
its otherwise completed terminal checks are retained, without aggregate credit.

Limits: one Linux development host, explicitly trusted development services and
private marked disposable roots. This establishes local production-control/PTY
acceptance, rather than hosted/default operation or physical WAN/NAT/Pi evidence.
Rapid extra proof requests can exhaust the historical process-local enrollment
bucket and leave an unsent prepared transcript expired; Retry never renews it.
The successful additional-folder journey allows 65 seconds for that bucket to
refill while both daemons/routes keep running. Restart-only experiments produced
transient setup blocks and one second-file timeout and remain uncredited;
W11 still owns native generation/route-change timing and recovery evidence.
Direct/LAN discovery, QUIC/ICE/STUN, roaming, expanded diagnostics, operated
profiles and distribution remain later packets. T13 native login/logout/boot
checks are incomplete; P17 owner use/explanation remains deferred. Existing
P/O/T/W00–W06 source/evidence is preserved.

Next: **W08 — LAN discovery and reachable direct candidates** is the next
sequential packet.
