# T05 — Additional-folder sharing and rollout

State: **complete**, 2026-10-04. All six ordinary tests passed twice (197.981s).
Final serial `make check` and `make test-race` exited 0. The additional uncached
T05 race run also instrumented CLI/daemon children and passed (119.791s).
Physical cross-host LAN/Tailscale, native boot/logout and P17 actual owner evidence
remain unexecuted/outstanding. P/O history and relocation changes are preserved.

Production changes implement the previously frozen `share` operation, restricted
known-ID/key invitations, private distinct scoped attempts and ordinary T04
receiving-root review/approval. Membership retrieval verifies the presented TLS
member key and returns one exact-predecessor successor; reconciliation installs
only additive membership chains before the existing exact-revision data gate.
The scheduler retains wire error codes. Live/stopped endpoint refresh reuses the
saved peer certificate, persists the address atomically and is adopted by the
existing runtime target/client reload. CLI/TUI retain the common control seam.

Six ordinary tests cross actual authenticated control/enrollment/peer APIs and
three daemon processes on fresh local nonloopback addresses. Final checks verify
same key/two folders, retained independent attempts/replay, receiver review and
preexisting bytes, denied unshared inventory, offline rollout with daemon restart,
two sequential missing revisions, both explicit B/C pull directions, A-authored
content forwarded via B with A offline, address refresh and actual connection
work errors, unchanged identities/revisions, and exact verified heads/content.
Additional checks cover wrong target ID/key, forged member certificate, stale
competing approvals, explicit predecessor fork and separate-root reviewed recovery
preserving the old paused group's heads/bytes. Generic revocation/expiry/replay,
independent membership models and conservative retirement regressions are rerun.

Commands, intermediate failures and limitations are in [commands](commands.md).
Gate dispositions are in [results](results.json), with task hashes and dirty-tree
provenance in [manifest](manifest.json). Next eligible work is **T06 — commands
and context**.

A single DeviceID/key may be an active member of several unrelated folders. Each
folder's immutable membership chain and scoped attempt separately authorize its
data; names and addresses locate a member but never grant global trust. Additive
rollout preserves existing key bindings and checks each committed predecessor.
A mismatch blocks file exchange; a fork needs reviewed recovery, not last-writer
selection. Retiring an offline peer does not make its unknown bytes safe.
This is the worker's explanation, not P17's unaided owner explanation.
