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
	$(GO) test ./internal/... ./model/...

test-race:
	$(GO) test -race ./...

test-integration:
	$(GO) test ./tests/integration/...

test-model:
	$(GO) test -count=1 -v ./model/...

test-faults:
	$(GO) test -count=1 -v ./tests/designgates/... ./tests/faults/...

check: fmt-check vet test test-integration build build-arm64

clean:
	rm -f bin/filesync bin/filesync-linux-arm64
