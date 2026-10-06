# W07 commands and results

All commands run from `<repo>` on one Linux development host.
The initial dirty source/evidence is recorded in `initial-status.txt`; W00–W06 and
P/O/T provenance is preserved. No host firewall/user-service/personal-root action.

| Command | Actual result / artifact |
| --- | --- |
| `git status --short`; read WAN plan/status/W07/W06 evidence and owning specifications | Prerequisites W06/WG1–WG3 inspected; no delegation |
| `python3 /home/owner/dotfiles/.agents/skills/ui-ux-pro-max/scripts/search.py 'keyboard focus form error preserve input asynchronous' --domain ux -n 3` | Keyboard focus/error retention guidance; existing terminal stack retained |
| `go test ./internal/terminal ./internal/controlclient ./cmd/filesync` | Passed after initial integration |
| `go test -list '^TestWANW07' ./internal/terminal ./tests/terminal` | Five model/control-presentation tests plus one real production-binary PTY test discovered; `logs/discovery.log` |
| `go test -race ./internal/terminal ./internal/controlclient ./cmd/filesync -count=1` | Passed; `logs/focused-race-final.log` (controlclient has no standalone test files) |
| `go test ./tests/terminal -run '^TestWANW07' -count=1 -v` | Initial attempts retained separately in `logs/pty-first.log` through subsequent numbered logs; failures receive no acceptance credit |
| `ORBIT_W07_PTY_EVIDENCE=<repo>/docs/evidence/wan-w07-20261005/pty go test ./tests/terminal -run '^TestWANW07' -count=1 -v` | Passed in 142.840 seconds; `logs/pty-thirteenth.log`, `pty/results.json` and sanitized PTY frames; runner requires empty evidence output and omits deliberate revealed transfer codes |
| `make check` | First aggregate failed solely at the new test's incorrect byte-array digest oracle (existing JSON uses hex); `logs/make-check.log`. Final aggregate passed all targets (terminal campaign 925.129 seconds); `logs/make-check-final.log` |

The canonical protocol/control-contract/model suite also passes:
`go test ./internal/protocol ./internal/control/terminalcontract ./model -count=1`
(`logs/contracts-model.log`). Owning-document relative file links and
`git diff --check` pass; source preservation checks all 5,050 initial files
(`logs/source-preservation.log`).

Initial failures and corrections:

- Fixture used top-level `network` with a copied `filesync` executable; corrected
  to the existing `orbit network` dispatch. Profile parent required owner-only
  permissions, and an encrypted-transit text assertion needed to account for wrapping.
- Expired v3 input previously reached its historical certificate validation first,
  reporting identity mismatch. TUI fresh-input expiry is now classified before
  that check. No network/mutation occurs on rejected input; PTY and focused model
  checks cover it.
- The first transfer reached both byte oracles but the test used `id` instead of
  the existing `version` JSON field. Corrected the oracle, retaining failed output.
- Rapid second-folder request hit the real historical enrollment limiter before
  submission. A longer wait established `EXPIRED_ATTEMPT`, retaining the same
  prepared identity and refusing implicit renewal. Restart-only isolation produced a generic temporary block and one second-file
  timeout, retained without acceptance credit. The final normal second-folder
  journey instead leaves both daemons/routes alive and allows 65 seconds for the
  historical enrollment bucket to refill; it passes verified transfer and does
  not change a rate limit, transcript lifetime or identity. Native generation/route
  change recovery remains W11 work.

Real PTYs run production binaries built without race instrumentation. The focused
race checks instrument the Go terminal model/shared-client/CLI package tests.
Hosted/native WAN, direct/QUIC/ICE/roaming, Pi resource, T13 login/logout/boot and
P17 owner use/explanation remain outside these local commands.

Further fixture corrections retained in numbered logs: folder lists are ordered
by their actual control page, rather than assuming Notes sorts first; revealed
code prefixes can wrap across lines and are checked with whitespace removed;
legacy list digests use the existing hex JSON encoding. Explicit transfer screens
are excluded from saved frames and any failure output is redacted. The first
aggregate's digest-oracle failure is uncredited; no production revocation failure
was established by that incorrectly typed test.
