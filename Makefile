BINARY_NAME=bin/jira-mcp
VERSION=1.1.0
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

.PHONY: all build test test-coverage install clean run

all: build test

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/jira-mcp
	@echo "Build complete: $(BINARY_NAME)"

test:
	go test -v -race ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"

install: build
	@mkdir -p $(HOME)/.local/bin
	install -m 755 $(BINARY_NAME) $(HOME)/.local/bin/jira-mcp
	@echo "Installed to $(HOME)/.local/bin/jira-mcp"

clean:
	rm -rf bin coverage.out coverage.html
	@echo "Cleaned build artifacts"

run: build
	./$(BINARY_NAME)
