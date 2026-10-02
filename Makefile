GO ?= go
VERSION ?= 1.0.0
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "release")
DATE ?= 2026-10-01
LDFLAGS ?= -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GOFLAGS ?=

.PHONY: build build-arm64 check fmt-check test test-race test-integration test-model test-faults test-harness vet clean package demo

build:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/filesync ./cmd/filesync
	ln -sf filesync bin/orbit

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/filesync-linux-arm64 ./cmd/filesync

package: build build-arm64
	$(GO) run ./scripts/build_packages.go

demo: build
	$(GO) run ./scripts/local_demo.go --quick

fmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))" || (gofmt -l $$(find . -name '*.go' -not -path './.git/*'); exit 1)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./internal/... ./model/...

test-race: build
	$(GO) test -race ./...

test-integration: build
	$(GO) test ./tests/integration/...

test-model:
	$(GO) test -count=1 -v ./model/...

test-faults:
	$(GO) test -count=1 -v ./tests/designgates/... ./tests/faults/...

test-harness:
	python3 -m unittest discover -s scripts/validation -p 'test_*.py'

check: fmt-check vet test test-integration test-model test-faults test-harness build build-arm64 package

clean:
	rm -rf bin/filesync bin/orbit bin/filesync-linux-arm64 dist/
