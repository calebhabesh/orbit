# W06 commands

Executed from `<repo>` on 2026-10-05. Output is retained in
`logs/`. All process/PTY fixtures create and validate `.filesync-disposable` roots.
No personal folder, cloud credentials, external operator or existing VPS was used.

- Repository/owning-document reads and `git status --short` establish the dirty
  W00–W05/P/O/T baseline. No reset, stash, commit or removal of unrelated work.
- `go test -list '^TestWANW06' ./internal/... ./tests/... ./cmd/filesync/...`:
  discovery in `logs/discovery.log`; later added recovery/generation cases are
  listed in the final discovery record.
- `go test ./internal/config ./internal/control/... ./cmd/filesync -count=1`:
  `logs/internal-cli.log`; final control/CLI run `logs/internal-cli-final.log`.
- `go test -race ./internal/control/... ./internal/config ./internal/network ./cmd/filesync -count=1`:
  `logs/race-final.log`. Later policy-generation/recovery changes use
  `go test -race ./internal/config ./internal/control/... ./cmd/filesync -count=1`,
  recorded in `logs/control-final-race.log`.
- `go test ./tests/terminal -run '^TestWANW06' -count=1 -v`:
  `logs/w06-cli-final.log` (initial two-case pass),
  `logs/w06-cli-expanded.log` (second-folder retryable quota surfaced), and
  `logs/w06-cli-expanded-final.log` (expanded three-case pass).
- `go test ./tests/terminal -run '^TestWANW06BinaryLocal' -count=1 -v`:
  `logs/w06-local.log`; guided PTY discovery/execution with
  `-run '^TestWANW06BinaryGuided'` is in `logs/w06-guided.log`.
- `go test -race ./tests/terminal -run '^TestWANW06' -count=1 -v`:
  `logs/w06-cli-race.log` and final `logs/w06-cli-final-race.log`. The test/service
  process is instrumented; the fixture's ordinary production CLI binaries are
  built by `go build` and are not independently race-instrumented.
- `go test ./internal/protocol ./internal/control/terminalcontract -count=1`:
  `logs/contracts.log`; unchanged canonical network/v2/v3 fixtures and additive
  control contracts.
- `make check`: first aggregate `logs/make-check.log` exhausted the default ten
  minutes while W05 third-device coverage ran; it is failed/uncredited. The target
  now specifies `go test -timeout=30m ./tests/terminal/...`. The next aggregate in `logs/make-check-final.log` passed the full terminal
  campaign (784.945s), packages, formatting, vet and internal checks, then found
  a manual integration fixture still relying on the former fresh-install default.
  `make check` on the repaired final tree is in `logs/make-check-complete.log`:
  exit 0; every target passes, including the full terminal, package, integration,
  model, fault and Python harness checks.
- `go vet ./...`: `logs/vet-final.log`.
- `git diff --check`: `logs/diff-check.log`.
- Python relative-link check over the edited owning WAN/control/operation/persistence
  documents: all relative file targets existed. Final check retained in
  `logs/docs-links.log`; anchors are not claimed independently validated.

Repairs found during execution: the legacy membership-export API expects a query
parameter; the typed request result differs from the legacy request-list JSON;
the CLI's old default explicitly passed five minutes, preventing the new routed
cadence from taking effect; second-folder admission can be quota-delayed while
retaining its accepted request. The final tests assert those actual behaviors.

Final compatibility work:

- `make build`, then `go test ./tests/integration -run '^TestOrbitPairing_CLI' -count=1 -v`:
  `logs/compat-build.log` and `logs/manual-compat-final.log` pass. These fixtures
  explicitly review Manual mode; fresh Automatic and its missing-profile behavior
  remain covered by W06. The earlier failing transfer transcript in
  `logs/manual-compat.log` is sanitized, preserving its incorrect-version label
  while removing the disposable capability/encoded private transfer.
- `go test -race ./internal/config ./internal/control/... ./cmd/filesync -count=1`:
  `logs/control-compat-final-race.log` passes after retained-alias integration.
- `go test -race ./tests/terminal -run '^TestWANW06' -count=1 -v`:
  `logs/w06-compat-final-race.log` passes, now asserting the retained `invite create`
  command's explicit v3 label as well as the named invitation workflow. Manual
  invitation envelope version 1 continues to use its historical v2 transfer label;
  neither canonical v2 signed bytes nor device identity changes.
