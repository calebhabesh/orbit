# Product and release packets

Read [scope](../portfolio-scope.md), [operations](../operations.md), and [verification](../verification.md). A polished interface does not waive an earlier failed invariant.

## P14 — Focused embedded web interface

**Depends on:** P13. **Requirements:** S08, S16, S18. **Invariants:** I16, I19.

Build React/TypeScript/Vite static assets embedded in the Go binary. Use the existing control contract; no second engine or runtime Node service. Implement only folders/devices/pending work, files/history, and conflicts. Include root/storage warnings, qualified per-peer progress, unavailable historical content and reviewed conflict/restore flows.

Provide keyboard access, visible focus, clear labels, escaped long/unusual filenames, accessible errors, responsive layout and safe loading/empty/stale states. Start with ordinary polling; real-time streams are optional only if justified. A browser closing must not stop synchronization.

**Acceptance:** end-to-end enrollment/status, conflict selection/manual-merge workflow, stale-resolution rejection and restore use the same operations as CLI; no access without local session; filenames cannot inject markup; mobile-width and keyboard operation usable; embedded build works without Node installed at runtime. Pending/failed operations never render as successful completion.

**Evidence:** screenshots and automated control/UI flows. **Explain:** which decisions live in the engine and why the UI cannot independently decide a conflict winner.

## P15 — Linux packaging and lifecycle

**Depends on:** P14. **Requirements:** S20, S21. **Invariants:** I08, I20.

Package for actual Linux amd64/arm64 devices, with checksums, version metadata, license notices and reproducible build instructions. Add systemd user-service templates, documented configuration/state locations and explicit root permissions. Avoid automatic privileged firewall/network changes.

Implement/test upgrade preflight, consistent backup, migration, health checks, compatible binary rollback and safe recovery after database rollback. Document new identity requirements after old-state restoration. Preserve user roots and state on ordinary uninstall. Recheck dependency support/security using official sources at release time.

**Acceptance:** clean installation and service restart on target architectures; embedded UI available without Node; unsupported schema/protocol fails clearly; interrupted migration recovers as specified; restore from old metadata cannot knowingly reuse causal identity/counters; uninstall preserves data. Release binary excludes or disables destructive test hooks.

**Evidence:** install/upgrade/recovery/uninstall runbooks and executed target checks. **Explain:** why copying a live SQLite main file may not create a consistent backup and why identity rollback is a protocol concern.

## P16 — Reproducible failure campaign and local demo

**Depends on:** P15; builds on tests from every earlier packet. **Requirements:** S11, S22. **Invariants:** I01–I20.

Consolidate the independent model, deterministic process hooks, adversarial protocol/path cases, GC/membership tests and resource tests into repeatable targets. Add a safe local multi-process demo with automatic disposable setup/cleanup and no external cloud credentials. Preserve failing seeds/minimized traces. Run race/fuzz/static checks appropriate to the actual stack.

Run controlled abrupt-reset experiments in disposable environments and publish their narrower assumptions separately from process-kill results. Audit the failure matrix for both pre-operation and post-operation crash gaps; a hook only after a step is insufficient to cover all states.

**Acceptance:** every invariant has a passing named check or an explicitly unresolved release blocker; all required scenario-matrix rows have evidence; harness refuses unsafe paths; a clean checkout reproduces the local demo and deterministic failure subset. No blanket “all failures safe” claim replaces individual outcomes.

**Evidence:** fault report with environment, exact commands, seeds, fault boundaries, results and limitations. **Explain:** which findings establish algorithmic correctness, process recovery, and abrupt-reset behavior respectively.

## P17 — Three-host pilot, measurements and case study

**Depends on:** P16. **Requirements:** S02, S03, S22.

Use the laptop, Pi and Oracle VPS with a dedicated non-sensitive shared folder. Record actual architecture, storage, OS, network route and versions; inventory provided hardware through read-only checks when access is available. Set up only project-specific paths/services, preserving unrelated VPS workloads.

Demonstrate normal sync, three independent offline edits, reviewed resolution with late arrivals, A→VPS→B without A/B overlap, interrupted/resumed transfer and historical restore. Under the owner's 2026-10-04 scope amendment, personal use and comprehensive owner review follow delivery. Preserve the prepared pilot; any later personal-use record reports actual duration, normal edits, offline/reconnect and restart. Keep destructive faults in disposable environments.

Execute benchmark matrix against fair full-file baseline, publish raw results and workload generators, then write the case study, architecture diagram, short demo and evidence-backed resume bullets. Include negative results, limits, one difficult bug and attributed design references. Finish README with current installation/demo commands only after they work.

**Acceptance:** all three actual hosts participate with verified content; forwarding and status semantics demonstrated; real pilot and synthetic benchmarks labeled separately; reproduction works from documented commands; claims link to evidence. If hardware/cloud access is unavailable, record a concrete blocker and finish independent artifacts without substituting three containers for three-host proof.

**Evidence:** release manifest, separately labeled automated pilot preparation, raw benchmark runs, demo and case study. **Deferred owner review:** product story, causal-resolution walkthrough, recovery trace, GC tradeoff and measured bottleneck.
