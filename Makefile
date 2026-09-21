GO ?= go
VERSION ?= dev
GOFLAGS ?=

.PHONY: build build-arm64 check fmt-check test test-race test-integration test-model test-faults vet clean

build:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags '-X main.version=$(VERSION)' -o bin/filesync ./cmd/filesync

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -trimpath -ldflags '-X main.version=$(VERSION)' -o bin/filesync-linux-arm64 ./cmd/filesync

fmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))" || (gofmt -l $$(find . -name '*.go' -not -path './.git/*'); exit 1)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./internal/...

test-race:
	$(GO) test -race ./...

test-integration:
	$(GO) test ./tests/integration/...

test-model:
	@echo 'model tests begin in P02 (no model exists yet)'

test-faults:
	@echo 'fault tests begin in P01/P03 and require disposable roots (none exist yet)'

check: fmt-check vet test test-integration build build-arm64

clean:
	rm -f bin/filesync bin/filesync-linux-arm64
