# W01 validation commands

All runs in repository root. Local disposable test roots/listeners only.
Initial reads, dependency/API/license audit and fixture generator are summarized
in dependency.md and summary.md. Earlier failing relay reproduction retained
in transport-first-failure.log.

```sh
go test -list ^TestWANW01 ./internal/... ./model/... ./tests/... ./cmd/filesync/...
```

Exit 0; [discovery.log](discovery.log).

```sh
go test -count=1 -timeout 60s -run ^TestWANW01 -v ./internal/network ./internal/protocol ./internal/replication ./internal/control/terminalcontract ./model
```

Exit 0; [focused.log](focused.log).

```sh
go test -race -count=1 -timeout 90s -run ^TestWANW01 -v ./internal/network ./internal/protocol ./internal/replication ./internal/control/terminalcontract ./model
```

Exit 0; [focused-race.log](focused-race.log).

```sh
go test -count=1 ./cmd/filesync/... ./internal/control/... ./internal/replication/... ./internal/protocol/... ./tests/terminal -run TestTerminalT0[1345]|TestWAN|TestTerminalT01|TestEnrollment|TestReplication|TestClient|TestIdentity|TestControl
```

Exit 0; [compatibility.log](compatibility.log).

```sh
go test -count=1 ./internal/... ./model/... ./cmd/filesync/...
```

Exit 0; [all-unit.log](all-unit.log).

```sh
go vet ./...
```

Exit 0; [vet.log](vet.log).

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...
```

Exit 0; [compile-amd64.log](compile-amd64.log).

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...
```

Exit 0; [compile-arm64.log](compile-arm64.log).

Final after contract/model updates:

```sh
go test -race -count=1 -timeout 90s -run ^TestWANW01 -v ./internal/network ./internal/protocol ./internal/replication ./internal/control/terminalcontract ./model
```

Exit 0; [final-focused-race.log](final-focused-race.log).

```sh
go test -list ^TestWANW01 ./internal/network ./internal/protocol ./internal/replication ./internal/control/terminalcontract ./model
```

Exit 0; [final-discovery.log](final-discovery.log).

```sh
go test -race -count=1 -run ^TestWANW01 -v ./model
```

Exit 0; [final-model-race.log](final-model-race.log).

```sh
go vet ./...
```

Exit 0; [final-vet.log](final-vet.log).

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...
```

Exit 0; [final-compile-amd64.log](final-compile-amd64.log).

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...
```

Exit 0; [final-compile-arm64.log](final-compile-arm64.log).

```sh
python3 docs/evidence/wan-w01-20261005/check_docs.py
```

Exit 0; [docs.log](docs.log).

```sh
git diff --check
```

Exit 0; [whitespace.log](whitespace.log).

```sh
python3 docs/evidence/wan-w01-20261005/generate_fixtures.py
```

Exit 0; independent fixture regeneration preserves all 25 hashes.

Final signed-intent fixture/API refinement re-executed the final-focused-race command (exit 0) and final-discovery (32 tests). Logs above are the final executions.

```sh
go vet ./...
```

Exit 0; [post-contract-vet.log](post-contract-vet.log).

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...
```

Exit 0; [post-contract-amd64.log](post-contract-amd64.log).

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...
```

Exit 0; [post-contract-arm64.log](post-contract-arm64.log).

```sh
python3 docs/evidence/wan-w01-20261005/check_docs.py
```

Exit 0; [post-contract-docs.log](post-contract-docs.log).

```sh
git diff --check
```

Exit 0; [post-contract-whitespace.log](post-contract-whitespace.log).

