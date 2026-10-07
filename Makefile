GO ?= go
VERSION ?= 1.0.0
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "release")
DATE ?= 2026-10-01
LDFLAGS ?= -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GOFLAGS ?=

.PHONY: build build-arm64 build-orbit-net package-orbit-net test-orbit-net-rehearsal check fmt-check test test-race test-integration test-model test-faults test-harness test-terminal-pty test-terminal-onboarding-pty test-terminal-everyday-pty test-terminal test-terminal-release test-terminal-packages test-terminal-package-transactions test-legacy-browser vet clean package demo

build:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/filesync ./cmd/filesync
	ln -sf filesync bin/orbit

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/filesync-linux-arm64 ./cmd/filesync

package: build build-arm64
	$(GO) run ./scripts/build_packages.go

# Operator-only connection service; never part of end-user packages.
build-orbit-net:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/orbit-net ./cmd/orbit-net

package-orbit-net:
	$(GO) run ./scripts/build_orbit_net.go

test-orbit-net-rehearsal: build build-orbit-net
	$(GO) test -count=1 -v ./tests/terminal -run '^TestWANW13'

demo: build
	$(GO) run ./scripts/local_demo.go --quick

fmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))" || (gofmt -l $$(find . -name '*.go' -not -path './.git/*'); exit 1)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./cmd/filesync/... ./internal/... ./model/...

test-race: build
	$(GO) test -race -timeout=60m ./...

test-integration: build
	$(GO) test ./tests/integration/...

test-model:
	$(GO) test -count=1 -v ./model/...

test-faults:
	$(GO) test -count=1 -v ./tests/designgates/... ./tests/faults/...

test-harness:
	python3 -m unittest discover -s scripts/validation -p 'test_*.py'

test-terminal-pty: build
	python3 scripts/terminal_pty_test.py --binary bin/filesync

test-terminal-onboarding-pty: build
	python3 scripts/terminal_onboarding_pty_test.py --binary bin/filesync

test-terminal-everyday-pty: build
	python3 scripts/terminal_everyday_pty_test.py --binary bin/filesync

test-legacy-browser: build
	$(GO) test ./tests/integration -run '^(TestP14|TestP15EmbeddedUI|TestOrbitSession)'

test-terminal:
	$(GO) test -timeout=45m ./tests/terminal/...

test-terminal-release:
	GOFLAGS=-race $(GO) test -race -count=2 -v ./tests/terminal -run '^TestTerminalT13'

test-terminal-packages: package
	python3 scripts/terminal_package_test.py --dist dist

test-terminal-package-transactions: package
	python3 scripts/terminal_package_test.py --dist dist --containers --emulate-arm64

check: test-terminal test-terminal-packages fmt-check vet test test-integration test-model test-faults test-harness build build-arm64 package

clean:
	rm -rf bin/filesync bin/orbit bin/filesync-linux-arm64 bin/orbit-net bin/orbit-net-linux-amd64 bin/orbit-net-linux-arm64 dist/
