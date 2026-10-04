# T05 commands and observations

Working directory: `<repo>`. Revision
`86ae55280b22a5839258d3cca40210c6e2613025` plus the existing dirty T00–T04,
relocation and planning tree. Go `go1.27.1-X:nodwarf5 linux/amd64`, Linux
`7.2.8-arch1-2 x86_64`. No delegation, dependency or schema increment.

Commands are actually executed; logs omit capabilities, private request payloads
and credentials. Test roots are new marker-protected temporary directories.

| Command | Actual result / record |
| --- | --- |
| `go test ./internal/replication ./internal/control ./internal/repository` | Exit 0; 32.492/6.130/1.463s |
| `go test ./cmd/filesync ./internal/control` | Exit 0; 0.044/0.369s |
| `go test ./tests/terminal -list '^TestTerminalT05'` | Exit 0; six ordinary top-level tests; [discovery](discovery.log) |
| `go test ./tests/terminal -run '^TestTerminalT05' -count=1 -v` during initial development | Initial compile exit 1: nonexistent pause/manifest field names; corrected to owning workspace Pause/Manifest.Digest. Three initial focused API tests then passed in 13.115s |
| `go test ./tests/terminal -run '^TestTerminalT05ThreeProcess' -count=1 -v` during initial development | Wrong membership export HTTP method produced HTML instead of JSON; corrected to actual POST/query contract. A following compile used a nonexistent request type and was corrected. Process attempt then failed at second request due to real admission limiter; [retained log](process-development.log) |
| same process command after fixture lifecycle correction | Exit 0, 83.754s; [development pass](process-development-2.log) |
| `go test ./tests/terminal -run '^TestTerminalT05(Competing\|Targeted)' -count=1 -v` | Exit 0, 0.072s; [approval/key checks](approval-development.log) |
| `go test -count=1 ./model/... ./internal/scheduler ./internal/repository ./internal/replication ./internal/control ./cmd/filesync` | Exit 0; model 0.469s, scheduler 7.562s, repository 1.613s, replication 32.591s, control 6.193s, CLI 0.051s |
| `go test ./tests/terminal -run '^TestTerminalT05' -count=2 -v` before strengthened forwarding assertions | Exit 0, all six twice, 193.594s; [earlier packet](packet.log) |
| `go test -count=1 -v ./tests/terminal -run '^TestTerminalT03(WrongScopeChallengeAndRevocation\|ReviewedApprovalStaleAndReplay\|ExpiredAndRevokedPending\|AdmissionCapsAndChangedReplay)$'` | Exit 0; four top-level/nine leaves, 0.209s; [enrollment regressions](enrollment-regressions.log) |
| `go test -count=1 -v ./model/... ./internal/repository -run 'Membership\|Retirement\|Retired\|Revival'` | Exit 0; four model/three repository tests; [membership regressions](membership-regressions.log) |
| `go vet ./tests/terminal ./cmd/filesync` | Exit 0; [scoped vet](scoped-vet.log) |
| initial `make check` | Exit 2: existing endpoint integration expected an empty certificate to fail even for an already configured pair; [retained failure](make-check.log) |
| `go test -count=1 -v ./tests/integration -run '^TestOrbitEndpoints_PersistenceAndValidation$'` | Exit 0; 0.014s. Revised test rejects an unknown pair without certificate and asserts existing-pair address refresh preserves its exact anchor; [regression](endpoint-regression.log) |
| `go test ./tests/terminal -run '^TestTerminalT05' -count=2 -v` with final forwarding assertions | Exit 0; all six twice, 197.981s; [final packet](packet-final.log) |
| final serial `make check` | Exit 0; unit/model/integration (73.043s), design/fault/harness checks, amd64/arm64 and all release packages; [final broad gate](make-check-final.log) |
| `python3 docs/evidence/terminal-t05-20261004/validate_docs.py` and `git diff --check` | Exit 0; 13 documents, local links/anchors, balanced fences and no trailing whitespace; [documentation validation](documentation-validation.log) |
| serial `make test-race` | Exit 0; no race report; integration 108.474s, terminal 186.259s; [broad race](make-test-race.log) |
| `GOFLAGS=-race go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT05'` | Exit 0; all six, 119.791s, including race-instrumented CLI/daemon children; [child race](packet-child-race.log) |

The real limiter remains five requests/minute/IP, burst five. The process fixture
restarts A between enrollment journeys, verifying durable records and resetting
the process-local admission bucket. An immediate subsequent same-IP preparation
can spend the remaining quota before submission; an unsent transcript that later
expires needs explicit new review. The fixture does not bypass the limiter or
claim an unlimited immediate enrollment rate. Twelve-second-per-token admission
and persisted joining backoff are existing product bounds, not benchmark claims.

Retirement remains the explicit survivor maintenance procedure. Automatic rollout
accepts only additive enrollment and never silently rekeys/removes a member.
Fork recovery retains old paused state/history and creates a new reviewed group;
it does not merge DAGs or infer completeness for uncaptured/offline bytes.
Older closed peer decoders reject new predecessor fields safely; exact-agreement
legacy data remains compatible. New automatic rollout requires T05-capable peers.

Cross-host LAN/Tailscale, native boot/logout/login, physical power-cut and P17
actual owner use/unaided explanation are **unexecuted** here. Ordinary named
commands/full adapter parity, qualified persistent attention and TUI journeys
remain T06/T07/T10. No personal service, folder, VPN or firewall was modified.
