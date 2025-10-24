# BSync Server Makefile

# Variables
BINARY_NAME=bsync-server
BUILD_DIR=bin
CMD_DIR=cmd/server
GO=go
GOFLAGS=-v

# Platform-specific variables
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Version info
VERSION ?= 1.0.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# LDFLAGS for version info
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

.PHONY: all build clean test run deps help build-linux build-windows build-darwin

# Default target
all: clean deps build

# Build the application
build:
	@echo "Building $(BINARY_NAME) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for Linux
build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	@echo "Linux build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

# Build for Windows
build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
	@echo "Windows build complete: $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe"

# Build for macOS
build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./$(CMD_DIR)
	@echo "macOS build complete: $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64"

# Build for all platforms
build-all: build-linux build-windows build-darwin
	@echo "All platform builds complete"

# Install dependencies
deps:
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy
	@echo "Dependencies installed"

# Run tests
test:
	@echo "Running tests..."
	$(GO) test -v ./...

# Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

# Run with custom flags
run-dev:
	@echo "Running $(BINARY_NAME) in development mode..."
	./$(BUILD_DIR)/$(BINARY_NAME) --log-level=debug --host=0.0.0.0 --port=8090

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	@echo "Clean complete"

# Show help
help:
	@echo "BSync Server - Makefile Help"
	@echo ""
	@echo "Available targets:"
	@echo "  make build          - Build for current platform"
	@echo "  make build-linux    - Build for Linux (amd64)"
	@echo "  make build-windows  - Build for Windows (amd64)"
	@echo "  make build-darwin   - Build for macOS (amd64)"
	@echo "  make build-all      - Build for all platforms"
	@echo "  make deps           - Download and install dependencies"
	@echo "  make test           - Run tests"
	@echo "  make run            - Build and run the application"
	@echo "  make run-dev        - Run in development mode"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make help           - Show this help message"
	@echo ""
	@echo "Environment variables:"
	@echo "  GOOS                - Target operating system (default: current OS)"
	@echo "  GOARCH              - Target architecture (default: current arch)"
	@echo "  VERSION             - Application version (default: 1.0.0)"
