# Foundation packets

Read [plan](../implementation-plan.md), [architecture](../architecture.md), and each packet's named contracts. Every packet uses the completion record defined in the plan.

## P00 — Reproducible project skeleton

**Depends on:** none. **Requirements:** S01, S20–S22.

Read operations and verification. Select a currently supported Go toolchain and maintained SQLite driver through official documentation and actual Linux amd64/arm64 build/runtime checks. Record driver linkage/cross-compilation tradeoffs. Create module, minimal CLI/agent entry point, configuration loader, migration runner skeleton, test harness directories, build targets and CI. Do not create empty implementations for all future modules.

Add state-directory ownership/permissions checks and exclusive-agent locking. Establish deterministic clock/randomness injection where behavior requires it, and disposable-root validation for tests. Record dependency licenses and design attribution.

**Acceptance:** clean checkout builds on the development host; CI runs real checks; SQLite persists across restart; incompatible schema refusal is exercised; second process cannot own the same state; an arm64 executable runs on available arm64 hardware/emulation with the environment stated. Mark real-device execution pending if not yet accessible rather than claiming it.

**Evidence:** toolchain/driver decision, commands and CI result in P00 status. **Explain:** why driver choice affects deployment, and why a compiled binary alone does not prove target compatibility.

## P01 — Architecture experiments and contract freeze

**Depends on:** P00. **Requirements:** S05–S13, S19. **Invariants:** I04, I07–I12, I15.

Read protocol/persistence. Implement small disposable experiments for D1–D5 from architecture. Focus D1 on real filesystem behavior: editor overwrite vs save-by-rename, open descriptor across replacement, path swaps, same-filesystem staging, and crash gaps. Prototype metadata/content retention interleavings in a simple model; no full transfer engine yet.

Produce `docs/design-gates.md` with one section per gate: attempted counterexamples, commands/traces, proposed implementation rule, supported assumptions and rejected alternatives. Freeze root/path policy, directory projection, author sequencing, retirement snapshot artifacts, canonical membership encoding and GC pin rules. Wire field details may complete in P02/P05 before interoperability fixtures are frozen.

**Acceptance:** every gate has executable evidence and a specific written outcome. Unsafe races are not dismissed as “unlikely.” Any discovery requiring a weaker approved guarantee is surfaced to the owner; independent safe work may continue. Baseline design refinements are made in their owning documents.

**Evidence:** reproducible experiment sources/traces and updated specifications. **Explain:** a concrete publication race a pre-rename stat check cannot eliminate.

## P02 — Independent causal model and production history module

**Depends on:** P01 D2/D3/D5 outcomes. **Requirements:** S07–S09. **Invariants:** I01–I04, I12, I15, I16.

Build the test-only DAG oracle and production vector-based history module separately. Define immutable envelopes, parent validation, counter ranges, deterministic heads, reviewed resolution plans and structural conflict representation. Add golden wire/domain fixtures including empty files, executable-only edits, tombstones, equal-byte conflicts and three authors. Keep transport, SQL and filesystem code outside both causal implementations.

**Acceptance:** bounded exhaustive cases and deterministic generated schedules agree with the independent oracle; every causal fixture in protocol passes; malformed ancestry/duplicate identities/counter overflow are rejected; same-author stale basis is explicitly exercised. Demonstrate the oracle catches an intentionally introduced comparison defect, then remove it. Freeze normalized comparison output and record enumeration bounds.

**Evidence:** fixture IDs, seeds and minimized traces. **Explain:** why receiving a version does not mean an ordinary edit should causally supersede it.

## P03 — Durable repository and immutable content

**Depends on:** P00, P02. **Requirements:** S01, S07, S10–S12. **Invariants:** I01, I05–I08, I20.

Implement migrations for the architecture's logical records, immutable chunk installation, streaming manifests, whole-file verification, durable local version creation and pending remote metadata. Pin SQLite configuration on every relevant connection. Add named fault hooks at object and SQL boundaries. Implement content pins and budget accounting now; actual GC remains off until P10.

Separate metadata-known, content-ready and working-applied queries. Include zero-byte files, repeated chunk hashes, malformed lengths, duplicate object installation and disk-full failures. Choose a safe consistent backup mechanism; identity rollback restrictions must be visible in recovery documentation.

**Acceptance:** kill/restart around every object/version commit boundary; protected referenced chunks exist and hash correctly after recovery; counter/version atomicity holds; no premature durable receipt is possible through the public interface. Orphan objects are identified without deleting referenced content. SQLite busy/checkpoint errors do not leave false readiness.

**Evidence:** schema diagram, boundary table and fault results. **Explain:** why writing bytes, verifying bytes and durably committing a version are different milestones.

## P04 — Workspace capture and journaled publication

**Depends on:** P01 D1/D5, P03. **Requirements:** S06, S09, S11–S13. **Invariants:** I04, I06, I07, I09, I11, I12, I17.

Implement safe root registration, reserved staging area, supported-object validation, full scan, stable capture and working basis. Implement journaled application of supplied verified versions without networking. Add structural conflict handling, executable mode policy, unavailable-root pause, complete-subtree deletion inference, deletion preview/generation tokens and recovery-before-scan ordering.

Support a minimal local control/CLI surface sufficient to register, scan, inspect and apply test versions. Implement mass-deletion detection using configurable absolute AND/OR ratio rules frozen in this packet, with fixtures for both small and large folders. Show candidates before creating distributable tombstones.

**Acceptance:** editor/race scenarios from D1, directory cases from D5, root replacement/unavailability, invalid paths/symlink swaps and kill-at-each-publication-boundary pass under declared assumptions. Repeated scan after apply/restart produces no spurious version. Existing protected bytes survive blocked/ambiguous operations. Confirm unsupported long-lived-writer cases are documented without overclaiming.

**Evidence:** filesystem support contract and actual recovery matrix. **Explain:** why equivalent histories do not guarantee identical working trees during conflicts.
