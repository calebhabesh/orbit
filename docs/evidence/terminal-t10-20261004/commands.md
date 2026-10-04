# T10 actual commands and results

Run from `<repo>`, 2026-10-04, on the existing dirty tree at
`86ae55280b22a5839258d3cca40210c6e2613025`. No commit/reset/stash or delegation.
Commands below are actual executions; logs retain output. Shell `time` measures
observed elapsed duration. An initial `/usr/bin/time -p` wrapper was unavailable
(exit 127) and ran no tests; it was replaced by the shell builtin.

## Discovery and final packet checks

```sh
go test ./internal/terminal ./tests/terminal -list '^TestTerminalT10'
go test -count=2 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT10'
GOFLAGS=-race go test -race -count=1 -v ./internal/terminal ./tests/terminal -run '^TestTerminalT10'
GOFLAGS=-race go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT10RealPTYOnboarding$'
```

[Discovery](logs/discovery.log): nine ordinary top-level tests (five view/adapter,
four production/process). [Final repeated run](logs/packet-final.log): all nine
passed twice without skips; terminal process suite 127.308s, command 127.750s.
[Packet race](logs/packet-race-final.log): all nine passed without race warnings,
terminal process suite 73.957s, command 74.940s. `GOFLAGS=-race` instruments the
actual CLI/daemon children built inside the process test. The additional final
PTY race confirmation passed (72.020s), recorded in [its log](logs/pty-race-confirmation.log).

## Relevant regression and broad checks

```sh
go test -count=1 -v ./cmd/filesync/... ./internal/control/... \
  ./internal/controlclient/... ./internal/repository/... ./internal/workspace/... \
  ./tests/terminal -run '^TestTerminalT09|^TestTerminalT04|^TestTerminalT05MembershipPinAndSequentialGate|^TestTerminalT07'
go test -count=1 ./cmd/filesync/...
python3 -m unittest discover -s scripts/validation -p 'test_terminal_vt.py'
python3 -m py_compile scripts/terminal_onboarding_pty_test.py scripts/terminal_vt.py
make check
make test-race
git diff --check
```

[Relevant regression](logs/regressions.log) passed (42.413s), including T09 actual
PTY restoration, T04 CLI/interrupted joining and the owning T05 sequential
membership/stale-revision gate. [Explicit CLI tests](logs/cli-final.log) passed
(0.108s). [Independent VT tests](logs/vt-final.log): five passed. Python compile
and diff checks exited 0. [Final make check](logs/make-check-final.log) passed
(89.000s), including vet, tests, model/fault/harness, static amd64/arm64 builds and
packaging. Full `make test-race` passed (284.660s; terminal suite 281.501s), retained
in [its log](logs/make-test-race.log).
`make check` and `make test-race` were serialized because packaging checks write
`dist`; packet/process fixtures have distinct private roots, ports and binaries.

## Sanitized actual-binary PTY artifacts

```sh
python3 scripts/terminal_onboarding_pty_test.py --binary bin/filesync \
  --output docs/evidence/terminal-t10-20261004/transcripts
```

[Transcript run](logs/pty-transcripts.log) passed (63.040s). The documented
`make test-terminal-onboarding-pty` target invokes this same runner; the target
itself was not separately invoked. The runner's onboarding/sharing/approval/management mutations use keyboard controls; typed
read-only queries assert identities, operation/request/attempt, membership/root
state and actual history. Assertions compare published bytes and a captured
SHA-256 after all clients exit. Frames are sanitized by the existing marked-root
campaign recorder. Incoming capabilities are masked; outgoing reveal is not used
in retained artifacts. Private invitation files are exclusively created at mode
0600 and removed with the validated fixture roots.

The runner creates two new mode-0700 marked roots and copies the binary into
each. It initializes finite runtime settings with fresh nonloopback listener
ports, starts only owned child daemons, and validates marker/canonical path/
process birth/executable/state argv before signals or cleanup. The receiver is
closed/reopened and its actual daemon gracefully restarted while approval is
pending. Inviter restart between folder journeys exercises persisted state and
resets its real process-local admission bucket. Startup-error daemons have an
empty fixture PATH, preventing any real host service/unit/lingering change.

## Intermediate corrections retained

- Initial T09 regression failed because the new folder-detail title omitted its
  existing `Inspect` label. The actual screen title was corrected; the regression
  passed subsequently. The initial output is recorded in the implementation turn.
- Initial T10 PTY checks reached completed joining/verified transfer but narrow
  readiness text was below identity details. Readiness now precedes long IDs.
  [First logged run](logs/packet-first.log) also exposed missing VT margin/autowrap
  semantics; independent oracle tests now cover them.
- [Second logged run](logs/packet-second.log) selected the original folder while
  the harness assumed selection had reset after creating another folder. The
  harness now selects from the actual keyset page while honoring preserved
  identity; no application selection reset was added.
- A draft fork assertion expected a rejected candidate to create a persisted fork
  marker. The production fixture now performs rejection plus the documented
  conservative `MEMBERSHIP_FORK` pause, then checks persistent attention/readiness.
- [Earlier instrumented run](logs/packet-race.log) pressed pause after the detail
  title appeared but before its query loaded. Final harness waits for actual
  loaded folder, device, membership and request state. It does not retry mutations
  blindly or weaken byte/identity/controller assertions.
- A draft verification-code renderer derived a label from the request ID. It was
  corrected before accepted proof: the owning canonical transcript/status supplies
  the persisted code in the existing Result requests vocabulary. The two actual
  devices' exact codes must match. Existing nested join fields stay unchanged.
- Intermediate passing runs are retained in packet-third/count2/final-count2 logs;
  they are superseded by final discovered/repeated/race evidence.

Final artifact audit initially found synthetic fixture roots split by hard-wrapped
screen cells, which escaped ordinary full-string redaction. Retained frames and
the T10 recorder now redact these known roots across wrapping; no underlying
assertion or frame was omitted. Python compilation, JSON/Markdown validation
(nine documents, 117 local links), nine-test discovery, final-log/race-warning
checks, exact task-file fingerprints and sanitized-frame audit passed.

## Unexecuted or retained limits

Physical LAN/Tailscale and native laptop/Pi/VPS; packaged bare entry, boot/login/
logout/unattended behavior; VM reset/power loss and P17 actual owner use/unaided
explanation remain unexecuted T12/T13 work. Local nonloopback processes establish
transport/control integration, not cross-host reachability or unattended operation.
Retirement/unregister are conservative previews linked to existing procedures.
Reviewed editor/conflict/history/restore screens remain T11. No personal folder,
existing VPS workload, firewall/VPN policy or host lingering setting was modified.
