# Operational correctness packets

Read [persistence](../persistence.md), [operations](../operations.md), and [verification](../verification.md).

## P10 — Finite storage and safe content cleanup

**Depends on:** P09 and D4 outcome. **Requirements:** S14, S17. **Invariants:** I05, I07, I10, I13, I15.

Implement storage accounting for all filesystems involved, durable reservations/pins, retention preview, configurable content expiry and crash-safe GC. Retain accepted envelope/tombstone metadata in v1; enforce visible metadata admission limits. Suspend unsafe membership-dependent cleanup during configuration changes. Preserve head/conflict content, pending-publication fallback, retained history and active work as specified.

Use a separate simple reference-set oracle to check GC candidates under generated receive/serve/restore/GC interleavings. Test physical unlink versus metadata intent/finalization gaps, orphan discovery, pinned corrupt content and shared chunks. Do not infer safety from age or an aggregate reference counter alone without validating transactions/races.

**Acceptance:** interrupted GC loses no protected object; expired history is visibly unavailable while current reconciliation succeeds; long-offline peers do not resurrect deletions; storage exhaustion pauses safely with actionable diagnostics; full-file staging is budgeted despite chunk reuse; clock jumps do not accelerate protected expiry. Demonstrate bytes reclaimed with retained current/conflict hashes unchanged.

**Evidence:** GC state machine, reference-set comparisons and finite-budget experiment. **Explain:** why historical content can expire while its version/tombstone metadata remains necessary.

## P11 — Integrity diagnosis and peer-assisted repair

**Depends on:** P10. **Requirements:** S15. **Invariants:** I06, I09, I10, I18.

Implement verified reads, explicit integrity scans, quarantine, affected-version accounting, current peer-availability lookup and repair through existing authorized transfer operations. Repair reconstructs the exact expected bytes and leaves causal identity unchanged. Distinguish expired historical content, missing protected content, corrupt content and temporary peer unavailability.

Make integrity scans bounded and cancelable. Pin content during checking/repair so GC cannot race it. Diagnose repeated corrupt responses without creating infinite retries or logging contents.

**Acceptance:** injected corruption of a chunk shared by multiple versions diagnoses all affected versions; another authorized replica repairs it correctly; no remaining copy yields explicit unavailable state; repair through an unauthorized folder is rejected; scan/repair restart and GC interleavings preserve required pins. No success receipt is newly issued for known corrupt content.

**Evidence:** corruption/repair demonstration and unavailable-content fixture. **Explain:** why a historical receipt cannot prove a peer still has good bytes today.

## P12 — Bounded continuous operation

**Depends on:** P11. **Requirements:** S10, S13, S17, S18. **Invariants:** I11, I13, I17, I20.

Implement watcher hints with periodic reconciliation and periodic full-content scanning. Add durable work recovery, coalescing, fair folder/peer/file scheduling, bandwidth caps, retry classification, queue bounds and graceful shutdown. Apply measured Pi/laptop resource profiles to baseline limits. Resource accounting includes in-flight hashing/staging rather than only network buffers.

Test notification loss/overflow, constant file mutation, equal-size timestamp-preserving edits, prolonged network failure, repeated failures on one path and unrelated successful work. Retries yield to other work; no busy-loop logging or goroutine leaks. Root failure pauses relevant folder operations.

**Acceptance:** bounded RSS/open descriptors/queue growth under sustained backlog; large-file progress amid small edits; canceled work stops promptly and restarts safely; notification loss recovered by scanning; repeated watcher feedback creates no authored versions; CPU idle behavior measured. All configured limits have validation and user-facing outcomes.

**Evidence:** fairness/resource experiments with explicit hardware/workload and updated defaults. **Explain:** why watchers improve latency but cannot replace reconciliation scans.

## P13 — Complete operator control and diagnostics

**Depends on:** P12. **Requirements:** S05, S13, S16, S19–S21. **Invariants:** I09, I15, I16, I19, I20.

Finish the CLI/control operations listed in operations, including storage/membership previews, repair, doctor, support export and maintenance commands. Add authenticated loopback control with strict Host/Origin checks, browser bootstrap/session/CSRF flow and stable JSON output. Specify idempotency-record lifetime and expired replay handling. Explain partial operations rather than displaying generic success.

Create runbooks for full disk, root unavailable, retired/lost device, corrupt content, stuck conflict, expired history, incompatible versions and database recovery. Logs/support export are bounded and sanitized; export stays local. Instrument metadata/content/network byte counts and operation phases for later benchmarks.

**Acceptance:** browser cross-origin mutation and unauthenticated sensitive reads fail; CLI mutations use the same operations as browser clients; support export contains no keys/tokens/content and defaults to path redaction; every stable error category includes a tested next action. Removal of folder registration and uninstall paths do not emit replicated deletes.

**Evidence:** control contract fixtures, security tests and operator runbooks. **Explain:** how stored, applied, offline, unknown and conflicted states differ for the same path.
