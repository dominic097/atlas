BINARY  := atlas
PKG     := github.com/MsysTechnologiesllc/aziron-atlas
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
  -X main.Version=$(VERSION) \
  -X main.Commit=$(COMMIT) \
  -X main.Date=$(DATE)

.PHONY: help build install test vet fmt tidy run clean

help: ## list targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n",$$1,$$2}'

build: ## build the atlas binary
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/atlas

install: ## go install into GOBIN
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/atlas

test: ## run unit tests
	go test ./...

vet: ## go vet
	go vet ./...

fmt: ## gofmt
	gofmt -w .

tidy: ## tidy modules
	go mod tidy

run: build ## build then print status
	./bin/$(BINARY) status

clean: ## remove build artifacts
	rm -rf bin
