# Makefile for Kubikles

.PHONY: dev build build-release build-windows-amd64 build-windows-arm64 build-mac build-mac-arm build-linux-amd64 build-linux-arm64 build-all install-wails install-deps install-frontend install-hooks clean test test-frontend test-watch profile build-pgo

# Ensure GOPATH/bin is in PATH
GOPATH := $(shell go env GOPATH)
WAILS := $(GOPATH)/bin/wails
BUILD_FLAGS := -trimpath -ldflags "-s -w"
PGO_FILE := default.pgo
UNAME_S := $(shell uname -s)

dev:
	$(WAILS) dev

build:
	$(WAILS) build

# Build optimized portable executable for current platform
build-release:
	$(WAILS) build $(BUILD_FLAGS)

# Build portable Windows executables (requires mingw-w64: brew install mingw-w64)
build-windows-amd64:
	$(WAILS) build -platform windows/amd64 $(BUILD_FLAGS) -o Kubikles-amd64.exe

build-windows-arm64:
	$(WAILS) build -platform windows/arm64 $(BUILD_FLAGS) -o Kubikles-arm64.exe

# Build portable macOS executable
build-mac:
	$(WAILS) build -platform darwin/amd64 $(BUILD_FLAGS) -o Kubikles-amd64

# Build portable macOS ARM executable (Apple Silicon)
build-mac-arm:
	$(WAILS) build -platform darwin/arm64 $(BUILD_FLAGS) -o Kubikles-arm64

# Build portable Linux executables
build-linux-amd64:
	$(WAILS) build -platform linux/amd64 $(BUILD_FLAGS) -o Kubikles-linux-amd64

build-linux-arm64:
	$(WAILS) build -platform linux/arm64 $(BUILD_FLAGS) -o Kubikles-linux-arm64

# Build all platforms
build-all: clean build-windows-amd64 build-windows-arm64 build-mac build-mac-arm build-linux-amd64 build-linux-arm64

install-wails:
	go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Install all dependencies (platform-aware)
install-deps:
ifeq ($(UNAME_S),Darwin)
	@echo "Installing dependencies for macOS..."
	@command -v brew >/dev/null 2>&1 || { echo "Homebrew not found. Install from https://brew.sh"; exit 1; }
	@command -v go >/dev/null 2>&1 || brew install go
	@command -v node >/dev/null 2>&1 || brew install node
	@brew list mingw-w64 >/dev/null 2>&1 || brew install mingw-w64
	@echo "Installing Wails..."
	@go install github.com/wailsapp/wails/v2/cmd/wails@latest
	@echo "Installing frontend dependencies..."
	@cd frontend && npm install
	@echo "Installing git hooks..."
	@$(MAKE) install-hooks
	@echo "Done! Run 'make dev' to start development server."
else ifeq ($(OS),Windows_NT)
	@echo "Installing dependencies for Windows..."
	@command -v go >/dev/null 2>&1 || { echo "Go not found. Install from https://go.dev/dl/"; exit 1; }
	@command -v node >/dev/null 2>&1 || { echo "Node.js not found. Install from https://nodejs.org/"; exit 1; }
	@echo "Installing Wails..."
	@go install github.com/wailsapp/wails/v2/cmd/wails@latest
	@echo "Installing frontend dependencies..."
	@cd frontend && npm install
	@echo "Installing git hooks..."
	@$(MAKE) install-hooks
	@echo "Done! Run 'make dev' to start development server."
else
	@echo "Installing dependencies for Linux..."
	@command -v go >/dev/null 2>&1 || { echo "Go not found. Install from https://go.dev/dl/"; exit 1; }
	@command -v node >/dev/null 2>&1 || { echo "Node.js not found. Install from https://nodejs.org/"; exit 1; }
	@echo "Installing Wails..."
	@go install github.com/wailsapp/wails/v2/cmd/wails@latest
	@echo "Installing frontend dependencies..."
	@cd frontend && npm install
	@echo "Installing git hooks..."
	@$(MAKE) install-hooks
	@echo "Note: For cross-compiling to Windows, install mingw-w64"
	@echo "Done! Run 'make dev' to start development server."
endif

# Install frontend dependencies only
install-frontend:
	cd frontend && npm install

# Install git hooks from tracked .githooks directory
install-hooks:
	@echo "Installing git hooks..."
	@cp .githooks/* .git/hooks/
	@chmod +x .git/hooks/*
	@echo "Git hooks installed successfully."

clean:
	rm -rf build/bin/*

# Run all tests
test: test-frontend

# Run frontend tests
test-frontend:
	cd frontend && npm test

# Run frontend tests in watch mode
test-watch:
	cd frontend && npm run test:watch

# ===========================================
# Profile-Guided Optimization (PGO)
# ===========================================
# PGO improves performance by 5-15% by optimizing hot paths
# Profile is architecture-specific (arm64 profile works on all Apple Silicon)

# Build with profiling enabled and capture a CPU profile
# Usage: make profile
#   1. App launches with pprof on port 6060
#   2. Use the app normally for 30-60 seconds (typical operations)
#   3. Press Ctrl+C to stop and save profile
profile:
	@echo "Building with profiling enabled..."
	$(WAILS) build -tags "profiling"
	@echo ""
	@echo "=== PGO Profile Collection ==="
	@echo "1. App will start with pprof enabled on port 6060"
	@echo "2. Use the app normally for 30-60 seconds:"
	@echo "   - Switch contexts, browse resources"
	@echo "   - Open logs, terminals"
	@echo "   - View dependency graphs"
	@echo "3. Press Ctrl+C when done"
	@echo ""
	@echo "Starting profile collection..."
	@./scripts/collect-pgo-profile.sh

# Build with PGO optimization (requires profile from 'make profile')
build-pgo:
	@if [ ! -f "$(PGO_FILE)" ]; then \
		echo "Error: $(PGO_FILE) not found. Run 'make profile' first to generate it."; \
		exit 1; \
	fi
	@echo "Building with PGO optimization from $(PGO_FILE)..."
	$(WAILS) build $(BUILD_FLAGS) -tags pgo

# Build optimized release for Apple Silicon with PGO
build-mac-arm-pgo:
	@if [ ! -f "$(PGO_FILE)" ]; then \
		echo "Error: $(PGO_FILE) not found. Run 'make profile' first."; \
		exit 1; \
	fi
	@echo "Building Apple Silicon release with PGO..."
	GOFLAGS="-pgo=$(PGO_FILE)" $(WAILS) build -platform darwin/arm64 $(BUILD_FLAGS) -o Kubikles-arm64-pgo

# Clean PGO profile
clean-pgo:
	rm -f $(PGO_FILE) cpu.pprof
