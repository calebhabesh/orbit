# T04 commands and actual outcomes

Commands ran from `<repo>`, on revision
`86ae55280b22a5839258d3cca40210c6e2613025` plus the preserved dirty tree.
No commits, dependencies, personal-folder faults, native service changes or host
network-policy changes were performed. Only disposable owned child processes
were stopped/killed; pending-phase SIGKILL uses the explicit marker validator.

- `go test ./... -list '^TestTerminalT04'`: 19 ordinary tests discovered, final
  [discovery](logs/discovery-final.log). The original discovery had 16 tests;
  issue-page, expired-preparation and actual CLI PTY coverage expanded the group.
- `go test -count=2 -v ./tests/terminal -run '^TestTerminalT04'`: final full
  [ordinary log](logs/ordinary-final.log); passed twice (38 top-level executions); outcome is in results.json.
- `go test -count=2 -v ./tests/terminal -run '^TestTerminalT04Interactive'`:
  passed twice; [actual CLI PTY](logs/pty.log).
- `go test -count=2 -v ./tests/terminal -run '^TestTerminalT04ExpiredPrepared'`:
  passed twice with accepted/unsent subcases; [expiry refinement](logs/expiry-refinement.log).
- `make check`: final serial [log](logs/make-check.log), followed by
  `make test-race`: final serial [log](logs/make-test-race.log). Both exited 0 on final runtime source. Gates writing packages never
  overlap. Packet CLI tests build their own binary beneath a private test root.
- `git diff --check`, local Markdown link/anchor validation and final source /
  environment / process inventory are recorded in results and manifest.

Intermediate checks were retained, not relabeled as final passes:
`draft-tests*.log`, `compatibility*.log`, `setup-compatibility*.log`,
`check-draft.log`, `check-final.log`, `check-refined-final.log`,
`packet-tests.log`, `packet-final.log`, `packet-refined-final.log`,
`cli-draft-2.log`, `race-final.log` and `build.log`.
Initial build/type mistakes are visible in their logs. Polling exceeded the
existing five-request/minute limiter; persisted 25s status cadence and 60s throttle
backoff fix that. Join mutation copied its intent before private capability
scrubbing to retain caller replay input. `COMMITTED` publication journals are
recognized as final. Approved requester endpoints now feed reverse pulls at the
scheduler cadence. Legacy script fixtures prepare private reviewed requests and
keep original file/key/alias assertions. An amd64-only implicit `Nlink` conversion
failed arm64 packaging and was replaced by an explicit `uint64` conversion.
The legacy unit fixture now initializes finite limits before onboarding.

Earlier draft commands launched two CLI daemons before returning an input error,
and Go removed their temporary directories. Exact PIDs 1050381 and 1050440 were
identified by executable, full selected state argv and corresponding open lock
before graceful SIGTERM; [cleanup](logs/cleanup-draft.log). This was cleanup of
known session children, not fault injection into an existing workload.

Full T04 tests contain testkit markers, production TLS/HTTP/control calls and real
CLI binaries/processes. The SIGKILL targets the acknowledged receiving child;
other phase interruptions use named hooks and state close/reopen. PTY uses Linux
`pty.openpty` and actual CLI prompts, without a rendering snapshot oracle.
Native hosts, physical LAN/Tailscale, native user-service lifecycle, VM abrupt
reset and owner use/explanation were not executed for this packet.
