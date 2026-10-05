# T11 — TUI everyday management

T11 is complete for the approved local production-interface acceptance bar.
Seven tests passed twice; packet race with instrumented child binaries,
final PTY race confirmation, explicit CLI tests, relevant regressions,
`make check`, and full `make test-race` passed. [Actual commands/results](commands.md),
[discovery](logs/discovery.log), [results](results.json), [manifest](manifest.json),
and [sanitized PTY frames](transcripts/results.json) retain the evidence.

The keyboard everyday interface provides comprehensive status, navigation, and recovery flows:
- **Overview & Attention:** Displays qualified observations (local readiness, root availability, membership health, degraded sync states), unread attention badges, and shortcut-driven navigation across folders, conflicts, and devices.
- **Folder Inspection & Path Navigation:** Exact folder status reviews, draft path input, live versus stopped daemon detection, and bounded path discovery across 7-item pages.
- **Conflicts & Content Recovery:** Exact concurrent head reviews, canonical diff and external editor invocation with raw/cooked terminal restoration, non-zero editor exit handling, stale editor session rejection with fresh review renewal, and exact select versus keep-copies resolution.
- **Deleted Files & Provenance Restore:** Historical tombstone inspection, non-destructive restore to original path, and separate safe copy restoration.
- **Storage & Maintenance Previews:** Read-only budget previews, cleanup candidate estimates, and narrow root-unavailable handling that safely blocks mutations without mass deletion.
- **Lifecycle & Daemon Continuation:** Clean terminal mode restoration on exit, with background daemons continuing bidirectional capture, synchronization, and transfer after TUI sessions terminate.

All seven scenarios were verified under real two-daemon PTY execution:
1. Real offline/reconnect concurrent heads
2. Canonical diff and editor failed exit recovery
3. Stale editor refusal and fresh review renewal
4. Exact select and keep copies
5. Delete history restore and separate copy
6. Storage preview narrow root block
7. Terminal restoration and daemon transfer after quit

Invariants verified and preserved:
- **I03–I05:** Concurrent heads survive until explicit resolution; no stored receipt without durable content and metadata.
- **I06–I07:** Partial or corrupt content is never published; recovery preserves protected versions and surfaces ambiguity.
- **I11:** Unavailable roots and incomplete scans never author mass deletions.
- **I13–I14:** Bounded resource limits and memory budgets; forwarding preserves author ancestry.
- **I16:** Restore and resolution operations are idempotent and reject stale attempts.
- **I18–I20:** Corruption yields unavailable or verified repair; full UI/CLI parity with qualified progress reporting.
- **I25–I28:** Editor sessions require staged hash verification before atomic commit; periodic lifecycle record pruning honors bounds.

Limitations remain explicit:
- Native laptop/Pi/VPS, physical LAN/Tailscale reachability, packaged bare entry/adoption, boot/login/logout/unattended validation, and VM reset/power loss remain T12/T13.
- **P17 actual owner use and unaided explanation remain outstanding.**
- Next packet: **T12 — Terminal cross-host packaging and lifecycle validation**.
