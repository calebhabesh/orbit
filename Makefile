GO ?= go
VERSION ?= 2.3.0
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "release")
DATE ?= 2026-10-09
LDFLAGS ?= -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GOFLAGS ?=

.PHONY: quick trial-install trial-reset trial-fresh trial-status ci-fast check-core test-short test-race-core build build-arm64 build-orbit-net package-orbit-net test-orbit-net-rehearsal check fmt-check test test-race test-integration test-model test-faults test-harness test-terminal-pty test-terminal-onboarding-pty test-terminal-everyday-pty test-terminal-keys-pty test-terminal test-terminal-release test-terminal-packages test-terminal-package-transactions test-legacy-browser vet clean package demo

build:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/orbit ./cmd/orbit

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/orbit-linux-arm64 ./cmd/orbit

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
	$(GO) test ./cmd/orbit/... ./internal/... ./model/...

test-race: build
	$(GO) test -race -timeout=60m ./...

# Race detector over everything except tests/terminal, which has its own
# race target (test-terminal-release) and dominates wall time.
test-race-core: build
	$(GO) test -race -timeout=30m ./cmd/... ./internal/... ./model/... ./tests/integration/... ./tests/designgates/... ./tests/faults/...

# Fast inner loop: skips tests that call testing.Short().
test-short:
	$(GO) test -short ./cmd/orbit/... ./internal/... ./model/...

# Trial inner loop (about a minute): build, vet and short tests of the touched
# packages plus the affected PTY suites. Not a release gate; see scripts/quick_check.sh.
quick:
	scripts/quick_check.sh

# Disposable orbit-trial instance on local, laptop and rpi (TRIAL_HOSTS to override).
trial-install:
	scripts/trial/trial.sh install

trial-reset:
	scripts/trial/trial.sh reset

trial-fresh:
	scripts/trial/trial.sh fresh

trial-status:
	scripts/trial/trial.sh status

# Push/PR gate. Target: a few minutes. Full suite runs in full.yml.
ci-fast: fmt-check vet build build-arm64 test-short
	$(GO) test -short ./tests/integration/...

test-integration: build
	$(GO) test ./tests/integration/...

test-model:
	$(GO) test -count=1 -v ./model/...

test-faults:
	$(GO) test -count=1 -v ./tests/designgates/... ./tests/faults/...

test-harness:
	python3 -m unittest discover -s scripts/validation -p 'test_*.py'

test-terminal-pty: build
	python3 scripts/terminal_pty_test.py --binary bin/orbit

test-terminal-onboarding-pty: build
	python3 scripts/terminal_onboarding_pty_test.py --binary bin/orbit

test-terminal-keys-pty: build
	python3 scripts/terminal_keys_pty_test.py --binary bin/orbit

test-terminal-everyday-pty: build
	python3 scripts/terminal_everyday_pty_test.py --binary bin/orbit

.PHONY: test-terminal-participation-pty
test-terminal-participation-pty: build
	python3 scripts/terminal_participation_pty_test.py --binary bin/orbit

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

# Everything in check except the two slow terminal suites (run as separate CI jobs).
check-core: fmt-check vet test test-integration test-model test-faults test-harness build build-arm64 package

check: test-terminal test-terminal-packages fmt-check vet test test-integration test-model test-faults test-harness build build-arm64 package

clean:
	rm -rf bin/orbit bin/orbit-linux-amd64 bin/orbit-linux-arm64 bin/orbit-net bin/orbit-net-linux-amd64 bin/orbit-net-linux-arm64 dist/
