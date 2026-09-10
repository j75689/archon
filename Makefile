.DEFAULT_GOAL := help

BINDIR := bin
BIN    := $(BINDIR)/archon
PKG    := ./cmd/archon

export CGO_ENABLED := 0

.PHONY: help build test vet fmt fmt-check tidy check sync install clean all

help: ## show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: $(BIN) ## compile archon into bin/archon

$(BIN):
	@mkdir -p $(BINDIR)
	go build -trimpath -o $(BIN) $(PKG)

test: ## run all tests
	go test ./...

vet: ## run go vet
	go vet ./...

fmt: ## format Go sources
	gofmt -w .

fmt-check: ## fail if gofmt would change files
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)

tidy: ## sync go.mod and go.sum
	go mod tidy

check: build ## fail if docs/ARCHITECTURE.md is stale
	$(BIN) check

sync: build ## rewrite the architecture diagram from the import graph
	$(BIN) sync

install: ## install archon into GOPATH/bin or GOBIN
	go install $(PKG)

all: fmt-check vet test build check ## fmt, vet, test, build, and doc check

clean: ## remove bin/
	rm -rf $(BINDIR)
