.PHONY: fmt vet lint build run clean test

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet

build: lint
	GOPROXY=https://goproxy.cn,direct go build -o bin/server ./cmd/server

run: build
	./bin/server -c config.yaml

dev:
	GOPROXY=https://goproxy.cn,direct go run ./cmd/server -c config.yaml

clean:
	rm -rf bin/server

test:
	go test ./...

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  fmt     Run go fmt"
	@echo "  vet     Run go vet"
	@echo "  lint    Run fmt + vet"
	@echo "  build   Lint and build the binary to bin/server"
	@echo "  run     Build and run the server"
	@echo "  dev     Run without building (go run)"
	@echo "  clean   Remove the binary"
	@echo "  test    Run tests"
	@echo "  help    Show this help"
	@echo ""
	@echo "Options:"
	@echo "  -c, -config <path>   Config file path (default: config.yaml)"
	@echo "  -h, -help            Show help"
