.PHONY: fmt vet lint build run clean test build-ui ui-dev build-all

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

build: lint
	GOPROXY=https://goproxy.cn,direct go build -o bin/server ./cmd/server

build-all: build-ui build

run: build
	./bin/server -c config.yaml

dev:
	GOPROXY=https://goproxy.cn,direct go run ./cmd/server -c config.yaml

clean:
	rm -f bin/server ai-switch

test:
	go test ./...

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
	@echo "  test        Run tests"
	@echo "  help        Show this help"
