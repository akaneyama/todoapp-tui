.PHONY: all build build-all test vet clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "1.0.0")
LDFLAGS := -s -w -X main.version=$(VERSION)
OUTPUT_DIR := bin

all: test build

build:
	@echo "==> Building server and client for host OS..."
	@mkdir -p $(OUTPUT_DIR)
	go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/server ./cmd/server
	go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/client ./cmd/client

test:
	@echo "==> Running tests..."
	go test -count=1 ./...

vet:
	@echo "==> Running go vet..."
	go vet ./...

clean:
	@echo "==> Cleaning build artifacts..."
	@rm -rf $(OUTPUT_DIR) dist

build-all: test
	@echo "==> Cross-compiling for Linux, macOS, and Windows..."
	@mkdir -p $(OUTPUT_DIR)
	# Linux amd64 & arm64
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-server-linux-amd64 ./cmd/server
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-client-linux-amd64 ./cmd/client
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-server-linux-arm64 ./cmd/server
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-client-linux-arm64 ./cmd/client
	# macOS (Darwin) amd64 & arm64 (Apple Silicon)
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-server-darwin-amd64 ./cmd/server
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-client-darwin-amd64 ./cmd/client
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-server-darwin-arm64 ./cmd/server
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-client-darwin-arm64 ./cmd/client
	# Windows amd64 & arm64
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-server-windows-amd64.exe ./cmd/server
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-client-windows-amd64.exe ./cmd/client
	GOOS=windows GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-server-windows-arm64.exe ./cmd/server
	GOOS=windows GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(OUTPUT_DIR)/todoapp-client-windows-arm64.exe ./cmd/client
	@echo "==> Cross-compile completed in $(OUTPUT_DIR)/"
