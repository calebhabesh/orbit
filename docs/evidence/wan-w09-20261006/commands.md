# W09 commands and results

Executed in the existing dirty checkout on 2026-10-06; one Linux development host.
Manifest and initial-source hashes retain the original P/O/T/W00–W08 provenance.

| Command | Actual result / log |
| --- | --- |
| `go list -m -json github.com/quic-go/quic-go@latest github.com/pion/ice/v4@latest` | selected v0.63.0 / v4.4.6; tagged API/license audit in dependency.md |
| `go mod download -json github.com/quic-go/quic-go@v0.63.0 github.com/pion/ice/v4@v4.4.6`; pinned `go get`; `go mod tidy` | installed pinned stack; `modules.json`, go.mod/go.sum; no dependency downgrade |
| `go test -list '^TestWANW09' ./internal/... ./tests/... ./cmd/filesync/...` | **15 tests / five packages**; `logs/discovery-final.log` |
| `go test -race ./internal/network ./internal/protocol ./internal/replication ./internal/config -run '^TestWANW09' -count=2 -v` | passing native transport, packet, strict fixture, configuration, admission, header/body deadlines, memory/cancel/close; `logs/focused-race-final.log` |
| `go test -race ./internal/network -run '^TestWANW09HTTP3ResponseHeaderAndWriteBounds$' -count=1 -v` | additional actual 16-KiB response-header refusal, 15-second response-header and 30-second response-write expiration; `logs/response-bounds-final.log` |
| `go test -race ./internal/replication -run '^TestWANW09' -count=2 -v` | final loss/duplicate assertions and additional unavailable-UDP→pinned-TCP test; pass 19.489s, `logs/replication-final.log` |
| `GOFLAGS=-race go test -race ./tests/terminal -run '^TestWANW09' -count=2 -v` (at that time only Local-only QUIC journey existed) | both production-binary repetitions pass, 81.669s; `logs/binary-quic-final.log`; not credited as running the later collision test twice |
| Collision/W08 binary command below | actual optional UDP/TCP collision→relay, retained TCP-only binary sync and mandatory manual collision; pass 132.528s; `logs/binary-collision-compat.log` |
| `go test -race ./internal/config ./internal/network ./internal/protocol ./internal/replication ./internal/control/... ./internal/terminal ./cmd/filesync -count=1` | final compatibility execution; `logs/compatibility-race-final.log` |
| `GOFLAGS=-race go test -race ./tests/terminal -run '^TestWANW07' -count=1 -v` | actual PTY compatibility; `logs/pty-compat-final.log` |
| `make fmt-check vet test test-integration test-model test-faults test-harness build build-arm64 package` | focused Make aggregate; `logs/aggregate-final.log`; ordinary Make caches are labeled in the log |
| Adapter cross-build commands below | pinned Pion/QUIC/adapter test binaries compile for both architectures; `logs/adapter-cross-build-final.log` (successful commands emit no text) |
| Service cross-build commands below | both service builds pass; same cross-build log |
| `python3 scripts/terminal_package_test.py --dist dist` | extracted amd64/arm64 archives / amd64 deb/rpm/completions and bare PTY checks pass; `logs/package-extract-final.log` |
| Independent Python cryptography Ed25519 fixture from seed `bytes(range(32))` | new `schemas/fixtures/lan-quic-v1` canonical/signature verified by Go; original LAN/TCP fixtures unchanged |

Exact binary/cross-build commands:

```sh
GOFLAGS=-race go test -race ./tests/terminal -run '^TestWANW09BinaryOptional|^TestWANW08' -count=1 -v
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c ./internal/network -o /tmp/orbit-w09-network-amd64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -c ./internal/network -o /tmp/orbit-w09-network-arm64
CGO_ENABLED=0 go build -o /tmp/orbit-w09-net-amd64 ./cmd/orbit-net
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /tmp/orbit-w09-net-arm64 ./cmd/orbit-net
```

Final preservation/documentation/format checks are recorded in `preservation.json`
and `logs/docs-links.log`. No full `make check` rerun is claimed for this intermediate
packet: W07's existing full aggregate remains preserved. No actual ICE, NAT matrix,
physical WAN/Pi/operated default profile or native T13 lifecycle is executed here.

## Failed / superseded runs (no acceptance credit)

- `logs/quic-first.log`: hello fixture advertised inventory 256 instead of the
  frozen maximum 128. `logs/quic-second.log`: unknown-member fixture expected
  HTTP 401 instead of the frozen authorization HTTP 403. The corrected fixture
  passes with unchanged production peer semantics in `quic-third.log` and final logs.
- `logs/binary-quic-first.log` and `binary-quic-second.log`: settings were rejected
  as INVALID_ENCODING, leaving no direct listener. The signed-message decoder
  requires every field, which is unsuitable for additive local configuration.
  Direct settings now retain W08 required fields and accept optional UDP fields;
  duplicate/unknown/null/malformed values still fail. The legacy regression test
  and two final production-binary repetitions pass. These failures are not W11
  route/roaming failures and do not overwrite W08's retained W11 failed evidence.
- Repaired the older optional-port fixture's listen-only JSON so it exercises a
  real bind collision with valid configuration. W08 TCP collision now explicitly
  disables UDP; W09 occupies both TCP and UDP. Earlier historical logs are untouched;
  final passing reproductions are in `binary-collision-compat.log`.
- Initial license collection assumed LICENSE / LICENSE.txt; qpack uses LICENSE.md.
  Corrected collection includes full tagged licenses in NOTICE. A license whitespace
  check then removed only trailing whitespace/new blank EOF in the added notices.
- Earlier compatibility/build/focused logs are supporting checks preceding final
  fixture/config refinements; final named logs above take precedence.
