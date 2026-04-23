.PHONY: fmt vet lint build run clean test test-e2e build-ui ui-dev build-all

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet

build-ui:
	cd frontend && npm install --legacy-peer-deps --cache /tmp/npm-cache && npx vite build
	touch internal/handler/static/.gitkeep

ui-dev:
	cd frontend && npx vite --host 0.0.0.0

VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildTime=$(BUILD_TIME)

build: lint
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/server ./cmd/server

build-all: build-ui build

run: build
	./bin/server serve -c config.yaml

dev:
	go run ./cmd/server -c config.yaml

clean:
	rm -f bin/server ai-switch

test:
	go test ./...

test-e2e:
	go test ./tests/e2e/ -v -short

test-e2e-full:
	go test ./tests/e2e/ -v

test-all: test test-e2e

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  fmt         Run go fmt"
	@echo "  vet         Run go vet"
	@echo "  lint        Run fmt + vet"
	@echo "  build-ui    Build frontend (Vue/Vite)"
	@echo "  ui-dev      Start frontend dev server with HMR"
	@echo "  build       Lint and build Go binary"
	@echo "  build-all   Build frontend + Go binary"
	@echo "  run         Build and run the server"
	@echo "  dev         Run without building (go run)"
	@echo "  clean       Remove the binary"
	@echo "  test        Run unit tests"
	@echo "  test-e2e    Run E2E protocol tests (no real CLI needed)"
	@echo "  test-e2e-full  Run all E2E tests including real CLI tests"
	@echo "  test-all    Run unit + E2E tests"
	@echo "  help        Show this help"
