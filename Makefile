# Makefile for gh-aw-fleet
# A GitHub Actions Workflow orchestration tool.
# Run 'make help' for target descriptions.

.PHONY: build test vet fmt fmt-check lint ci tidy clean help

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
	golangci-lint run ./...

ci: fmt-check vet lint test

tidy:
	go mod tidy

clean:
	rm -f gh-aw-fleet gh-aw-fleet.exe

help:
	@echo "Targets:"
	@echo "  build      — produce ./gh-aw-fleet binary"
	@echo "  test       — run go test ./..."
	@echo "  vet        — run go vet ./..."
	@echo "  fmt        — format code with gofmt"
	@echo "  fmt-check  — check if code needs formatting"
	@echo "  lint       — run golangci-lint"
	@echo "  ci         — run fmt-check, vet, lint, test"
	@echo "  tidy       — run go mod tidy"
	@echo "  clean      — remove built binaries"
