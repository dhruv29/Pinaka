# Common tasks. Everything here is also runnable by hand; see README.md.

BINARY := pinaka
GOFLAGS := -trimpath -buildvcs=false
LDFLAGS := -s -w -buildid=

.PHONY: all build test vet fmt check

all: check build

build: ## Build a reproducible binary
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/pinaka

test: ## Unit tests with the race detector
	go test ./... -race -cover

vet:
	go vet ./...

fmt: ## Fail if any file is not gofmt-formatted
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run gofmt -w on the files above" && exit 1)

check: fmt vet test ## Everything CI runs
