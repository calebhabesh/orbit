# T08 — Conflicts, history and restore

State: **complete**. T06/T07 prerequisites and TG4, terminal UX/architecture,
protocol, persistence, operations and verification contracts were read.

Nine ordinary T08 tests were discovered and all passed twice without skips.
The uncached packet race run passed all nine without race warnings. Explicit
CLI/control/client/workspace/repository/model regressions, `make check`, full
`make test-race` and `git diff --check` exited 0. Full race `tests/terminal`
passed in 190.639s. Exact commands/exits/timings are in
[commands](commands.md), [validation](validation.json), [results](results.json)
and the associated logs.

## Implemented behavior and evidence

- Authenticated raw exact-version reads and uploads share live/stopped ownership,
  capability checks, cancellation and identity validation. Reads verify the whole
  manifest before emission, retain one chunk buffer and response pins, reject
  unavailable/range substitutions, and release state ownership before a stopped
  read returns an error or closes. A large live read survives racing GC.
- Bounded SQL conflict/structural/history/Deleted pages preserve generation-bound
  cursors and expose actual source candidates/content states. Content review binds
  stable registration/membership, exact heads and current named working stat/hash;
  ordinary unchanged scans do not invalidate it. Missing reviewed inputs cannot
  silently resolve currently known heads.
- Select, keep-copies, staged manual merge, original-path restore and separate-copy
  recovery use typed shared controls. New causal versions and replay effects commit
  together. Lost-response/partial-copy tests assert stable identities and effects.
  Completed replay returns its recorded result without reapplying old bytes after
  later capture. Pending replay refuses publication after newer/competing heads.
  Explicit destination plans reject collisions; publication uses existing guards.
- Private durable editor sessions admit exports/result/one immutable upload,
  reopen verified sources sequentially, retain session pins and private results,
  and survive inspection/restart. Direct quoted tool argv runs with a Linux file-size
  limit. Real configured tool failure/crash retains the candidate without staging
  or resolving it. Interrupted staging remains budgeted and nonauthoritative.
  Renewal revalidates original review; expiry/cancel preserve recovery candidates;
  explicit freshly reviewed discard removes only known private paths.
- Real daemon-backed CLI commands exercise select, keep-copies, history, restore,
  external exact export, configured editor and a 6,000,000-byte streamed merge.
  Tests compare exact parents/source provenance and working bytes/hashes. Other
  production-interface checks cover pending/expired/corrupt refusal, uncaptured
  edits, new versions during review, active read/GC interaction, persistent recovery
  attention, structural pages, interrupted copy replay, renewal/expiry and discard.

## Limits and handoff

Tools are trusted owner programs; file-size admission is not a host sandbox.
Results default to the largest reviewed source size with a 1 MiB minimum.
One immutable upload is admitted per session. Unknown auxiliary files prevent
complete discard and remain inspectable. Structural conflicts require separate
path reviews; there is no cross-path atomic visibility or semantic auto-merge.
Unavailable content has no unverified peer-fetch action or fixed deletion timer.

RSS observations in logs include Linux fork/exec high-water effects and the synthetic
fixture. One fixed large workload does not establish a scaling curve or throughput
claim. No SIGKILL at every T08 durable boundary or power-loss claim is made.
Interactive PTY editor suspend/resume belongs to T11/T13. Physical LAN/Tailscale,
native boot/logout/login/unattended, and P17 actual owner use/unaided explanation
remain unexecuted here. Historical P/O evidence and existing unrelated/relocation
changes are preserved. Next eligible packet: **T09**.

Worker explanation: restored bytes come from the selected historical version,
which is provenance. The new restore's parents are the reviewed current heads,
so current ancestry is retained and later arrivals can remain concurrent. File
mtime/display clocks can disagree, be reset or reflect delayed delivery; they
cannot prove causal dominance or select a safe winner. This is worker explanation,
not P17 unaided owner evidence.
