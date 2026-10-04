# T04 — Reviewed setup and resumable joining

All 19 ordinary T04 tests passed twice (38 executions). Final `make check`
passed, including amd64/arm64 packaging; subsequent serial `make test-race` passed.
T04 is complete and establishes M1. Next eligible packet: T05.
Actual commands and final dispositions are recorded in [commands](commands.md)
and [results](results.json).

Implementation adds bounded descriptor-rooted recursive review, private resumable
preview state, root/tree/plan generation and expiry checks, conservative capacity
admission and atomic operation/job/review admission. Create/adopt/join use shared
live controls; jobs survive prompt exit, delayed approval and daemon restart.
Exact signed requests and inviter identity remain durable; expired preparation
recovers prior acceptance by possession status or requires an explicitly new
attempt. Bootstrap capture precedes remote file history/publication; scan,
content, publication and conflicts independently qualify Ready.

The actual CLI scenario uses two independent daemon installations on one Linux
host's nonloopback IPv4. It creates/invites/reviews/joins/approves, kills only the
marked receiving child after acknowledged pending admission, restarts it, checks
stable request/attempt/device identity, preserves initial local bytes and verifies
ordinary edits in both pull directions. After stopping owners, exact heads and
managed-content digests are checked. A separate real CLI PTY case corrects an
invalid finite input, edits review, retains names, adopts existing bytes and proves
client exit leaves the daemon running. Other phase interruptions use production
hooks and close/reopen, not SIGKILL at every boundary.

Preserved failures include draft compile errors, invitation polling throttling,
caller-owned join intent mutation, false pending publication due to phase spelling,
missing reverse endpoint persistence, migration of script fixtures to explicit
review files, and an arm64 `Nlink` type error. Those defects were corrected before final validation.
Two exact leaked draft CLI children were gracefully stopped after validating their
process executable/argv/open lock; no personal service was touched.

Physical cross-host LAN/Tailscale, native laptop/Pi/VPS execution, boot/logout/
login/unattended guarantees, abrupt reset/power loss, TUI behavior, additional-folder/
offline/third-peer rollout and actual owner pilot/explanation remain unexecuted or
assigned to later packets. No dependencies or schema/protocol identity changed.
Existing relocation and historical P/O evidence were preserved. P17 owner use and
unaided explanation remain outstanding.

Worker explanation: enrollment admits the reviewed identity; local capture saves
working bytes; missing verified content/publication/conflicts govern Ready. These
are distinct observations. Restart keeps the operation and authenticated attempt,
and a stale root never silently approves different files. Capacity observations
are conservative admission checks, not reservations against unrelated writers.
This explanation is not owner evidence.
