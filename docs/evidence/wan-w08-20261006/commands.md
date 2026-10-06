# W08 commands and results

Commands executed in the existing dirty checkout on 2026-10-06. Initial revision,
file hashes and status are retained; final source hashes live in `manifest.json`.
Logs include unsuccessful attempts. Only the following passing runs establish acceptance.

| Command | Result / artifact |
| --- | --- |
| `go test -list '^TestWANW08' ./internal/... ./tests/... ./cmd/filesync/...` | 15 tests, four packages; `logs/discovery-final.log` |
| `go test -race ./internal/network -run '^TestWANW08' -count=3 -v` | pass; `logs/network-race-final.log` |
| `go test -race ./internal/network ./internal/protocol ./internal/replication -run '^TestWANW08' -count=1 -v` | pass; one expected namespace skip, separately executed below; `logs/focused-race-final.log` |
| `go test -race ./internal/network ./internal/protocol ./internal/replication ./internal/config ./internal/control/... ./internal/terminal ./cmd/filesync -count=1` | all pass; `logs/compatibility-race-current.log` |
| `GOFLAGS=-race go test -race ./tests/terminal -run '^TestWANW08' -count=2 -v` | both final journeys twice, children instrumented, 173.308s; `logs/binary-race-fresh-final.log` |
| `go test ./tests/terminal -run '^TestWANW0[678]' -count=1 -v` | W06/W07 compatibility and earlier W08 fixture pass, 324.337s; `logs/binary-pty-final.log`; W08 final acceptance uses the later named journeys |
| `make fmt-check vet test test-integration test-model test-faults test-harness build build-arm64 package` | all pass; `logs/aggregate-focused-final.log` |
| `go test ./cmd/filesync ./internal/control/... -count=1` | final legacy Local-only error-code compatibility; `logs/legacy-control-final.log` |
| `make build build-arm64 package` | final package build; `logs/final-sanity-packages.log` |
| `CGO_ENABLED=0 go build -o /tmp/orbit-w08-net-amd64 ./cmd/orbit-net` | pass |
| `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /tmp/orbit-w08-net-arm64 ./cmd/orbit-net` | pass |
| `make fmt-check vet`; `python3 -m py_compile scripts/wan_direct_namespace_test.py` | pass; `logs/fmt-vet-final.log` |
| Independent Python `cryptography` Ed25519 fixture generation from seed `bytes(range(32))` | canonical/signature fixture matches Go; `schemas/fixtures/lan-v1` |
| Initial SHA-256 preservation audit; relative documentation-link checker; `git diff --check` | pass; `preservation.json`, `logs/docs-links.log`, `logs/final-sanity.log` |

## Isolated public TCP/IPv6

Executed `go test -c -race ./internal/replication -o /tmp/orbit-w08-public-tests`,
then the following Python wrapper (only new private marked temporary roots):

```python
import os, tempfile, pathlib, subprocess, shutil
root = pathlib.Path(tempfile.mkdtemp(prefix='orbit-w08-namespace-'))
root.chmod(0o700)
(root / '.filesync-disposable').write_text('W08 disposable namespace fixture\n')
try:
    result = subprocess.run([
        'unshare', '--user', '--map-root-user', '--net', 'python3',
        'scripts/wan_direct_namespace_test.py',
        '--test-binary', '/tmp/orbit-w08-public-tests', '--root', str(root),
        '--parent-namespace', os.readlink('/proc/self/ns/net')])
    if (root / 'network.json').exists():
        shutil.copyfile(root / 'network.json',
            'docs/evidence/wan-w08-20261006/namespace.json')
    if result.returncode:
        raise SystemExit(result.returncode)
finally:
    assert root.resolve() == root and (root / '.filesync-disposable').is_file()
    shutil.rmtree(root)
```

Pass: `logs/public-namespace-final-fixed.log`. `namespace.json` records actual
isolated dummy-interface setup and addresses. No external route, NAT emulation,
host namespace/link/firewall modification or VPS workload was involved. A same-host
invocation with the current namespace is refused before `ip`; a new user namespace
without a new network namespace is also refused by ownership checks. Both refusal
results are in `logs/namespace-refusal.log`.

## Failed or superseded work

- Initial compile failures: unavailable guessed Unix ancillary parser (replaced by
  native-endian IP_PKTINFO index decoding), unused import, wrong fixture helper signature.
- `logs/focused-race-first.log`: invalid all-zero fixture IDs and shared httptest
  certificate mistakenly used as a wrong-pin oracle; fixed with nonzero IDs and
  independently generated Ed25519 server certificate. No failed test receives credit.
- `logs/empty-lease-red.log`: 20 lookups instead of one; `logs/empty-lease-green.log`
  and final focused tests pass after caching empty signed leases.
- `logs/network-bounds-final.log`: library dial goroutines had not yet quiesced;
  complete candidate/TLS attempts are now manager-owned and joined. Final repeated
  resource checks pass and use a finite post-close library-quiescence bound.
- `logs/binary-first.log` through `binary-third.log`: one-second restart/collision
  fixture quota/route failures. Five-second `binary-fourth.log` passes once, while
  `logs/binary-race-repeat-final.log` fails one of two repetitions. These combined
  lifecycle/restart fixtures are uncredited; W11 owns repair/tuning. Final W08 separates
  known-peer Local-only LAN from fresh occupied-port onboarding and passes both twice.
- First namespace run followed failed compilation and could not find its test binary.
  A later parent `/proc` check was permission-denied under mapped credentials;
  `NS_GET_USERNS` ownership validation replaces it and final/public/refusal runs pass.

The first documentation checker incorrectly treated historical `file://` URI links
as filesystem-relative links (`logs/docs-links-first.log`). The corrected relative
file check passes; historical URI references/evidence remain untouched.

Full `make check`, native internet/WAN/NAT/Pi, IPv6-only multicast, deployed hosted
profiles/operator readiness and T13 native lifecycle checks remain unexecuted here.
Prior W07 full aggregate and P/O/T evidence are retained unchanged.
