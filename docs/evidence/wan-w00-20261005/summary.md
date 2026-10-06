# W00 baseline — 2026-10-05

State: **complete**. `make check` and final documentation/provenance checks passed.
WAN runtime implementation remains unstarted.

The [inventory](../../implementation/wan-baseline.md) records production commands,
listeners, schemas, persistent identities/pins, manual peers, invitations, durable
setup/service behavior, source ownership, safe harness requirements and every
W packet's relevant inherited prerequisite. [Manifest](manifest.json) records
initial clean revision and every original tracked file hash; no snapshot was used.
[Commands](commands.md) and logs retain actual executions.

Executed focused checks passed: uncached CLI/control/replication; real T04 CLI
create/invite/review/join/delayed approval/interrupted receiver restart and two-way
bytes; T05 production three-process forwarding and changed-address failure followed
by explicit address-only recovery; W00 addressless invitation rejection and bounded
unusable endpoint, including race. Both identities and a preexisting captured
immutable head, verified managed content and local bytes survive the W00 failure.
The successful T05 fixture retains identities/membership and original authors.

`make check` passed: terminal/extracted-package, format/vet, CLI/internal/model,
integration, fault/design-gate and 12 Python safety/VT tests, amd64/arm64 builds
and all package formats. Its ordinary cached tests are distinguished from the
explicit uncached commands. Documentation links/anchors, ordered packet rows,
nonzero selected test discovery and preservation of every original tracked file
outside the three declared W documentation edits passed; `git diff --check` passed.

These are single-host nonloopback sockets, not a physical LAN or native WAN
campaign. No automatic discovery, relay, v3 enrollment, QUIC/ICE, NAT emulator,
namespace faults, deployed default service or privileged host operation was run.
Full release/VM/native/resource checks are not rerun without a new relevant concern.
Current T13 native login/logout/unattended boot remains outstanding; its successful
ordinary three-host Tailscale evidence is retained. P17 personal use/explanation
remains deferred. No existing host service/pilot/root was a fault target.

Next eligible: **W01**. Close WG1–WG3 with actual TLS/stream and independent
admission experiments, freeze additive contracts/fixtures and WG5 model, preserve
v1 histories/membership and v2 manual enrollment. W00's inventory is the handoff;
all W01–W17 production acceptance and remaining gates stay pending.
