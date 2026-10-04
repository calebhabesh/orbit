# T07 — Status, attention, and diagnostics

State: **complete**, 2026-10-04. All five planned T07 integration and CLI tests passed twice (0.935s).
Final serial `make check` and `make test-race` exited 0. The uncached packet race run passed (2.294s).
Full repository `make test-race` passed across all packages (178.420s for `tests/terminal`).
Detail observation gap repaired (`Direct` bool on `PeerProgressSummary`) and baseline test `TestTerminalT00DetailObservation` passes.
Physical cross-host LAN/Tailscale, native boot/logout, and P17 actual owner evidence remain unexecuted/outstanding. P/O history and relocation changes are preserved.

Production changes implement truthful observed copy status, persistent attention items, and actionable diagnostics:
1. **Qualified Status and Truthful Observations**:
   - `orbit status` reports named folders, local capture/pending work, actionable attention items, observed device-copy state/freshness (`Saved`, `Stored`, `Applied`, `Direct`, `Online`, `Availability`, `LastContact`), and daemon/startup facts.
   - Preserves working bytes versus durable capture, direct versus indirect reports, stored versus applied, and historical receipt versus current availability.
   - Offline, stale, unknown, and content-pending observations remain visible. A stored receipt from an intermediary forwarder (e.g., VPS) cannot claim a final device has applied the version.
   - No false "Ready" or "global-synchronized" claim when mixed/pending/blocked states exist.
2. **Bounded Aggregate and Attention Control Queries**:
   - Bounded queries (`Query{Kind: "status"}`, `Query{Kind: "attention"}`, `Query{Kind: "doctor"}`) with deterministic pagination via `Limit` and `Cursor`.
   - Fast cancellation support via `context.Context`; long queries abort cleanly.
   - Read-only guarantee: status and attention inspection never scan the filesystem, run GC, or mutate membership as a hidden read side-effect. Large folders do not become complete in-memory UI inventories.
3. **Actionable Persistent Attention Triage Across Client Close/Restart**:
   - Captures and persists actionable attention items across daemon restart/stopped state:
     - `CONFLICT` (competing content versions)
     - `STRUCTURAL_CONFLICT` (name collision / directory file divergence)
     - `ROOT_UNAVAILABLE` (missing local directory)
     - `STALE_ROOT` (dev/inode mismatch)
     - `FOLDER_PAUSED` (operator pause)
     - `BLOCKED_PATH` (unsupported socket/FIFO or unreadable permission)
     - `EXHAUSTED_WORK` (durable task exhausted retry budget)
     - `DISK_BUDGET` / `METADATA_BUDGET` (storage quota/budget exceeded)
     - `AWAITING_APPROVAL` (pending enrollment request)
     - `INCOMPLETE_SETUP` (pending onboarding mutation)
     - `MEMBERSHIP_FORK` (competing concurrent membership revisions)
     - `OFFLINE` (peer without contact for >24h)
   - Safe lifecycle pruning (`PruneTerminalEnrollmentRequests`, `PruneFinishedTasks`) cannot erase pending attention.
4. **Actionable Diagnostics (`orbit doctor`)**:
   - Comprehensively evaluates daemon local-control lifecycle, TLS identity certificates and file permissions, state directory and database permissions, root availability and inode validation, storage budgets and free space reserve, network reachability (loopback advertised address with remote peers, peer contact freshness > 24h, Tailscale CLI reporting), folder approvals and membership forks, active membership limits, service/systemd tooling, protocol compatibility, and pending recovery items.
   - Proposes safe corrective commands (`remediation`) without silently altering host settings (e.g. systemd lingering or network configuration).
   - Distinguishes OK, WARN, and FAIL states accurately.
5. **Adapter and Transport Parity**:
   - Full live/stopped equivalence via `controlclient.Client.WithController`.
   - `--json` emits clean, frozen contract representations (`tc.Result`) with backward-compatible fields (`daemon_running`, `setup`, `folders`, `attention`).
   - Human rendering sanitizes output via `EscapeTerminal` to prevent ANSI terminal escape spoofing.

Commands, intermediate failures and limitations are in [commands](commands.md).
Gate dispositions are in [results](results.json), with task hashes and dirty-tree
provenance in [manifest](manifest.json). Next eligible work is **T08 — conflicts, history and restore** (and T09).

### What an offline device's last stored receipt actually establishes

An offline device's last stored receipt establishes only that *at the specific time the receipt was recorded*, that device durably committed the chunks and metadata to its local storage.
It does NOT prove that:
- The device is currently reachable or online.
- The version was successfully applied to its working directory (`Applied=false`).
- The device has not modified, deleted, or diverged the file offline since that contact.
- Other peer devices or forwarders have received or verified the content.
Furthermore, a receipt recorded by a relaying intermediary or VPS forwarder confirms durable forwarding reception on that node alone; it cannot be promoted to claim that the final destination device received or applied the edit.
This is the worker's explanation, not P17's unaided owner explanation. Outstanding P17 actual owner use and unaided explanation work remains outstanding.
