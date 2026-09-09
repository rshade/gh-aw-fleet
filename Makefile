# Makefile for gh-aw-fleet
# A GitHub Actions Workflow orchestration tool.
# Run 'make help' for target descriptions.

# Tool versions are pinned once, in mise.toml. GOLANGCI_LINT_VERSION is read
# back out of that file rather than duplicated here, and the `lint` target
# compares it against the golangci-lint actually on PATH before running --
# mise's shims do not reliably win PATH, so reading the pin is not enough on
# its own. See the Toolchain note in AGENTS.md.
GOLANGCI_LINT_VERSION := $(shell awk -F'"' '/^golangci-lint =/{print $$2}' mise.toml)

.PHONY: build test vet fmt fmt-check lint ci drift tidy clean help ensure

build:
	go build -o gh-aw-fleet .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	gofmt -l . | diff -u /dev/null -

lint:
	@have=$$(golangci-lint version --short 2>/dev/null || \
		golangci-lint --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1); \
	if [ -z "$$have" ]; then \
		echo "golangci-lint not found on PATH; mise.toml pins $(GOLANGCI_LINT_VERSION)." >&2; \
		echo "Run: make ensure" >&2; exit 1; \
	fi; \
	[ "$$have" = "$(GOLANGCI_LINT_VERSION)" ] || { \
		echo "golangci-lint $$have is on PATH but mise.toml pins $(GOLANGCI_LINT_VERSION)." >&2; \
		echo "Try: mise exec -- make lint" >&2; exit 1; }
	golangci-lint run ./...

drift:
	@./scripts/check-toolchain-drift.sh

ci: drift fmt-check vet lint test

tidy:
	go mod tidy

clean:
	rm -f gh-aw-fleet gh-aw-fleet.exe

ensure:
	@command -v mise >/dev/null 2>&1 || \
		(echo "mise not found. Install: https://mise.jdx.dev/installing-mise.html"; exit 1)
	mise install
	@echo "Toolchain ready: go, golangci-lint $(GOLANGCI_LINT_VERSION), node"

help:
	@echo "Targets:"
	@echo "  build      — produce ./gh-aw-fleet binary"
	@echo "  test       — run go test ./..."
	@echo "  vet        — run go vet ./..."
	@echo "  fmt        — format code with gofmt"
	@echo "  fmt-check  — check if code needs formatting"
	@echo "  lint       — run golangci-lint"
	@echo "  drift      — check go.mod matches mise.toml"
	@echo "  ci         — run drift, fmt-check, vet, lint, test"
	@echo "  tidy       — run go mod tidy"
	@echo "  clean      — remove built binaries"
	@echo "  ensure     — install pinned tools from mise.toml"
