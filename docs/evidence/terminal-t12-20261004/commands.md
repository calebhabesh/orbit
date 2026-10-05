# Actual commands and results

Executed on the dirty development tree; output files retain intermediate failures.

The successful final gates are listed in results.json. Earlier nonzero runs are retained.

- `go test -count=2 -v ./tests/terminal -run '^TestTerminalT12'`: exit 0, 22.441s; [transcript](transcripts/packet.txt).
- `make package`: exit 0, 2.389s; [transcript](transcripts/package.txt).
- `python3 scripts/terminal_package_test.py --dist dist --containers --output docs/evidence/terminal-t12-20261004/package-results.json`: exit 1, 0.194s; [transcript](transcripts/extracted-packages.txt).
- `env GOFLAGS=-race go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT12'`: exit 0, 39.6s; [transcript](transcripts/packet-race.txt).
- `python3 scripts/terminal_package_test.py --dist dist --containers --output docs/evidence/terminal-t12-20261004/package-results.json`: exit 1, 12.999s; [transcript](transcripts/extracted-packages-retry.txt).
- `go test -count=1 -v ./cmd/filesync ./tests/integration ./internal/repository ./internal/controlclient -run 'Test(Relocation|OrbitMigration|OrbitRecovery|OrbitSchemaRollback|OrbitStoppedMetadata|P15|TerminalT02|TerminalT05)'`: exit 0, 4.745s; [transcript](transcripts/compatibility.txt).
- `make package`: exit 0, 1.38s; [transcript](transcripts/package-directories.txt).
- `python3 scripts/terminal_package_test.py --dist dist --containers --output docs/evidence/terminal-t12-20261004/package-results.json`: exit 1, 15.074s; [transcript](transcripts/extracted-packages-directories.txt).
- `make demo`: exit 0, 0.604s; [transcript](transcripts/demo.txt).
- `make package`: exit 0, 1.383s; [transcript](transcripts/package-rpm-header.txt).
- `python3 scripts/terminal_package_test.py --dist dist --containers --emulate-arm64 --output docs/evidence/terminal-t12-20261004/package-results.json`: exit 1, 0.261s; [transcript](transcripts/extracted-packages-rpm-header.txt).
- `python3 scripts/terminal_package_test.py --dist dist --containers --emulate-arm64 --output docs/evidence/terminal-t12-20261004/package-results.json`: exit 1, 0.093s; [transcript](transcripts/extracted-packages-emulated.txt).
- `python3 scripts/terminal_package_test.py --dist dist --containers --emulate-arm64 --output docs/evidence/terminal-t12-20261004/package-results.json`: exit 1, 15.65s; [transcript](transcripts/extracted-packages-emulated-retry.txt).
- `make package`: exit 0, 1.409s; [transcript](transcripts/package-rpm-offsets.txt).
- `python3 scripts/terminal_package_test.py --dist dist --containers --emulate-arm64 --output docs/evidence/terminal-t12-20261004/package-results.json`: exit 1, 15.722s; [transcript](transcripts/extracted-packages-rpm-offsets.txt).
- `make package`: exit 0, 1.443s; [transcript](transcripts/package-rpm-v4.txt).
- `python3 scripts/terminal_package_test.py --dist dist --containers --emulate-arm64 --output docs/evidence/terminal-t12-20261004/package-results.json`: exit 1, 15.615s; [transcript](transcripts/extracted-packages-rpm-v4.txt).
- `docker run --rm --label orbit.disposable=t12-rpm-diagnostic --mount type=bind,source=<repo>/dist,target=/packages,readonly fedora:43 sh -c 'mkdir -m 700 /tmp/orbit-t12; touch /tmp/orbit-t12/.filesync-disposable; rpm -vv -U --replacepkgs --nosignature /packages/filesync-1.0.0-1.x86_64.rpm'`: exit 1, 0.273s; [transcript](transcripts/rpm-diagnostic.txt).
- `docker run --rm --label orbit.disposable=t12-rpm-query --mount type=bind,source=<repo>/dist,target=/packages,readonly fedora:43 sh -c 'mkdir -m 700 /tmp/orbit-t12; touch /tmp/orbit-t12/.filesync-disposable; rpm -vv -qp --nodigest --nosignature /packages/filesync-1.0.0-1.x86_64.rpm'`: exit 0, 0.275s; [transcript](transcripts/rpm-query.txt).
- `make package`: exit 0, 1.439s; [transcript](transcripts/package-rpm-digests.txt).
- `docker run --rm --label orbit.disposable=t12-rpm-digests --mount type=bind,source=<repo>/dist,target=/packages,readonly fedora:43 sh -c 'mkdir -m 700 /tmp/orbit-t12; touch /tmp/orbit-t12/.filesync-disposable; rpm -vv -U --replacepkgs --nosignature /packages/filesync-1.0.0-1.x86_64.rpm'`: exit 0, 0.417s; [transcript](transcripts/rpm-digests-diagnostic.txt).
- `make package`: exit 0, 1.402s; [transcript](transcripts/package-lifecycle.txt).
- `python3 scripts/terminal_package_test.py --dist dist --containers --emulate-arm64 --output docs/evidence/terminal-t12-20261004/package-results.json`: exit 1, 16.836s; [transcript](transcripts/extracted-packages-final.txt).
- `python3 scripts/terminal_package_test.py --dist dist --containers --emulate-arm64 --output docs/evidence/terminal-t12-20261004/package-results.json`: exit 0, 16.609s; [transcript](transcripts/extracted-packages-hash-oracle.txt).
- `go test -count=2 -v ./tests/terminal -run '^TestTerminalT12'`: exit 1, 0.454s; [transcript](transcripts/packet-final.txt).
- `make check`: exit 2, 275.298s; [transcript](transcripts/check.txt).
- `go test -count=2 -v ./tests/terminal -run '^TestTerminalT12'`: exit 1, 22.709s; [transcript](transcripts/packet-final-retry.txt).
- `go test -count=2 -v ./tests/terminal -run '^TestTerminalT12'`: exit 0, 22.7s; [transcript](transcripts/packet-final-fixed.txt).
- `go test ./tests/terminal -list '^TestTerminalT12'`: exit 0, 0.22s; [transcript](transcripts/discovery.txt).
- `go test -count=1 -v ./internal/replication -run '^TestAuthenticationAuthorizationAndCompatibilityMatrix$'`: exit 0, 0.216s; [transcript](transcripts/peer-compatibility.txt).
- `go test -count=1 ./cmd/filesync`: exit 0, 0.531s; [transcript](transcripts/cli-final.txt).
- `env GOFLAGS=-race go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT12'`: exit 0, 42.155s; [transcript](transcripts/packet-race-final.txt).
- `git diff --check`: exit 0, 0.008s; [transcript](transcripts/source-checks.txt).
- `make check`: exit 0, 356.253s; [transcript](transcripts/check-final.txt).
- `make test-race`: exit 0, 320.85s; [transcript](transcripts/test-race-final.txt).
- `git diff --check`: exit 0, 0.007s; [transcript](transcripts/whitespace-final.txt).
- `make package`: exit 0, 1.331s; [transcript](transcripts/packages-final.txt).
- `go test -count=1 ./tests/integration -run '^(TestP14|TestP15EmbeddedUI|TestOrbitSession)'`: exit 0, 0.772s; [transcript](transcripts/legacy-browser-final.txt).
- `python3 scripts/terminal_package_test.py --dist dist --containers --emulate-arm64 --output docs/evidence/terminal-t12-20261004/package-results.json --pty-output docs/evidence/terminal-t12-20261004/pty`: exit 0, 16.786s; [transcript](transcripts/package-acceptance-final.txt).
- `make fmt-check`: exit 0, 0.047s; [transcript](transcripts/formatting-final.txt).
- `git diff --check`: exit 0, 0.007s; [transcript](transcripts/diff-final.txt).
