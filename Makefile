VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
DATE    ?= $(shell date -u +%Y-%m-%d)
LDFLAGS := -s -w -X github.com/Ervisio/ErvisioRepoCLI/internal/build.Version=$(VERSION) -X github.com/Ervisio/ErvisioRepoCLI/internal/build.Date=$(DATE)

.PHONY: help build install vet fmt fmt-check lint check clean

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-10s %s\n", $$1, $$2}'

build: ## Build ./eg
	go build -trimpath -ldflags "$(LDFLAGS)" -o eg ./cmd/eg

install: ## Install eg into $GOBIN
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/eg

vet: ## go vet
	go vet ./...

fmt: ## Format the code
	gofmt -w .

fmt-check: ## Fail when gofmt would change files
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

lint: fmt-check vet ## gofmt, go vet and golangci-lint (when installed)
	@if command -v golangci-lint >/dev/null; then golangci-lint run ./...; \
	elif [ -x "$$HOME/go/bin/golangci-lint" ]; then $$HOME/go/bin/golangci-lint run ./...; \
	else echo "golangci-lint not installed, skipped"; fi

check: build lint ## Build and lint

clean: ## Remove build output
	rm -rf eg dist
