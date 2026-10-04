# T01 commands and observations

Executed from `<repo>`, 2026-10-03. Provenance and toolchain
are in [manifest.json](manifest.json). The working tree includes earlier
planning, T00 and relocation changes; no commit or dependency migration was made.

Read AGENTS.md, implementation/P/O/terminal status and plans, terminal UX/
architecture, T00 evidence, scope/glossary, protocol, persistence, operations,
verification and current source/strict codec/state lock. Initial inspection:

```sh
git status --short
git rev-parse HEAD
go version
uname -srmo
findmnt -T . -n -o FSTYPE,OPTIONS
rg --files internal/control schemas tests/designgates
```

The initial tracked diff SHA-256 and full dirty status are in the manifest.
T01 changes are listed/hashed separately; P/O evidence and relocation sources
were preserved. Tests use generated identities, httptest listeners and a
`t.TempDir()` root with `.filesync-disposable` for the existing lock experiment.
No existing services, personal roots, VPN or firewall were touched.

Early incremental package checks passed after adding the contract codec, then
the typed transcript, fixtures and experiments. The initial targeted run found
11 T01 tests before the remaining wire/join/upload/exit tests were added; those
intermediate checks are not the final coverage claim. Final discovery and runs:

```sh
go test ./... -list '^TestTerminalT01' > docs/evidence/terminal-t01-20261003/transcripts/discovery.txt 2>&1
go test -count=2 -v ./... -run '^TestTerminalT01' > docs/evidence/terminal-t01-20261003/transcripts/targeted.txt 2>&1
go test -race -count=1 -v ./internal/control/terminalcontract ./internal/protocol ./tests/designgates -run '^TestTerminalT01' > docs/evidence/terminal-t01-20261003/transcripts/targeted-race.txt 2>&1
```

Final discovery: **17 top-level tests**, 8 contract, 2 protocol, 7 design gates.
Final twice-run group: **34 passes**, no skipped T01 tests. Targeted race group:
17 passes with no race report. Packages with zero T01 matches establish no
packet coverage; the three nonzero groups above do. The expected wrong-pin TLS
handshake rejection may appear in the prototype server log and is asserted as
a negative case, not a failing test. No production T00 assertion was weakened.

An initial `make check` passed during incremental work. After the final Go/test
changes, rerun the broad gates serially; both share packaging outputs:

```sh
make check > docs/evidence/terminal-t01-20261003/transcripts/make-check.txt 2>&1
make test-race > docs/evidence/terminal-t01-20261003/transcripts/make-test-race.txt 2>&1
python3 docs/evidence/terminal-t01-20261003/validate_docs.py > docs/evidence/terminal-t01-20261003/transcripts/docs.txt 2>&1
git diff --check
```

Final serial broad gates both exited 0; documentation validation and
`git diff --check` passed. A draft link-validation run found the not-yet-created
results.json evidence file; the rerun passed after writing it. Final results are
recorded in results.json and summary.md.
`make check` includes fmt/vet, internal tests, integration/model/design/fault
suites, Python safety tests, amd64/arm64 builds and packaging. `make test-race`
uses `-race ./...`, including CLI tests. It does not execute opt-in T00 failures.
The documentation validator checks local links/anchors, fences and whitespace
for the eleven T01 owning/evidence documents; it is preserved beside this record.

The TLS experiment creates two fresh httptest listeners and uses a trusted
certificate plus VerifyConnection SPKI checking without verification bypass.
The peer prototype requires a client certificate, enrollment mounts no data
routes. Signature tests use fresh Ed25519 keys; independent canonical golden
bytes were generated using Python `struct.pack` big-endian uint64/uint32,
explicit byte fields and UTF-8 lengths. Model admission launches 16 contenders
and expects exactly one fresh request; a same-record retry consumes no new use.
Restart tests encode/decode all planned phases and preserve recorded effects.
The 8 MiB stream experiment hashes incrementally with a 1 MiB transfer buffer;
its oracle separately allocates expected bytes after streaming. This is not
an RSS benchmark or production merge execution.

Unexecuted for T01: actual terminal endpoint/adapter, schema/fsync/SIGKILL join
recovery, real root-swap traversal, requester status/approval over production
handlers, real external editor/GC races, PTYs, native service login/logout,
LAN/Tailscale application enrollment, native package adoption/rollback and
personal use/explanation. These stay with T02–T13 and P17. No fault was injected
against personal data; ordinary broad disposable fault tests retain their own
pre-existing safety checks and limitations.
