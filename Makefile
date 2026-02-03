# Makefile for Kubikles

.PHONY: help dev build build-release build-windows-amd64 build-windows-arm64 build-mac build-mac-arm build-linux-amd64 build-linux-arm64 build-appimage build-all install-wails install-deps setup setup-quick install-frontend install-hooks clean test test-frontend test-watch profile build-pgo

.DEFAULT_GOAL := help

help:
	@echo ""
	@echo "Kubikles - Kubernetes Desktop Client"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Setup:"
	@echo "  setup              Full setup (system deps, Go, Node, Wails, npm, hooks)"
	@echo "  setup-quick        Setup without system dependencies"
	@echo "  install-frontend   Install frontend npm dependencies only"
	@echo "  install-wails      Install Wails CLI tool"
	@echo "  install-hooks      Install git pre-commit hooks"
	@echo ""
	@echo "Development:"
	@echo "  dev                Start development server with hot-reload"
	@echo ""
	@echo "Build:"
	@echo "  build              Build for current platform"
	@echo "  build-release      Build optimized portable executable"
	@echo "  build-mac          Build for macOS (amd64)"
	@echo "  build-mac-arm      Build for macOS (Apple Silicon)"
	@echo "  build-windows-amd64  Build for Windows (amd64)"
	@echo "  build-windows-arm64  Build for Windows (arm64)"
	@echo "  build-linux-amd64  Build for Linux (amd64)"
	@echo "  build-linux-arm64  Build for Linux (arm64)"
	@echo "  build-appimage     Build Linux AppImage"
	@echo "  build-all          Build for all platforms"
	@echo ""
	@echo "Headless Server (no GUI):"
	@echo "  build-headless     Build headless server for current platform"
	@echo "  build-headless-linux-amd64  Build headless for Linux amd64"
	@echo "  build-headless-linux-arm64  Build headless for Linux arm64"
	@echo "  build-headless-all Build headless for all Linux platforms"
	@echo ""
	@echo "Testing:"
	@echo "  test               Run all tests"
	@echo "  test-frontend      Run frontend tests"
	@echo "  test-watch         Run frontend tests in watch mode"
	@echo ""
	@echo "Profile-Guided Optimization:"
	@echo "  profile            Collect CPU profile for PGO optimization"
	@echo "  build-pgo          Build with PGO optimization"
	@echo "  build-mac-arm-pgo  Build Apple Silicon release with PGO"
	@echo "  clean-pgo          Remove PGO profile files"
	@echo ""
	@echo "Utilities:"
	@echo "  clean              Remove build artifacts"
	@echo ""

# Ensure GOPATH/bin is in PATH
GOPATH := $(shell go env GOPATH)
WAILS := $(GOPATH)/bin/wails
PGO_FILE := default.pgo
UNAME_S := $(shell uname -s)

# Version info from git
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo "")
GIT_DIRTY := $(shell git diff --quiet 2>/dev/null && echo "false" || echo "true")
VERSION_LDFLAGS := -X main.GitCommit=$(GIT_COMMIT) -X main.GitDirty=$(GIT_DIRTY)
BUILD_FLAGS := -trimpath -ldflags "-s -w $(VERSION_LDFLAGS)"

dev:
	$(WAILS) dev

build:
	$(WAILS) build -ldflags "$(VERSION_LDFLAGS)"

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

# Build portable Linux AppImage (bundles into single executable)
build-appimage:
	@./scripts/build-appimage.sh

# Build all platforms
build-all: clean build-windows-amd64 build-windows-arm64 build-mac build-mac-arm build-linux-amd64 build-linux-arm64

# ===========================================
# Headless Server Builds (no Wails/GUI dependencies)
# ===========================================
# These builds create a minimal server-only binary without GUI dependencies.
# Ideal for running on headless servers or in containers.

# Build headless server for current platform
build-headless:
	@echo "Building headless server..."
	@cd frontend && npm run build 2>&1 | grep -v "WARNING\|nesting\|css-syntax-error\|invalid-@nest" || true
	@mkdir -p build/bin
	CGO_ENABLED=0 go build -tags headless $(BUILD_FLAGS) -o build/bin/kubikles-server
	@echo "Built build/bin/kubikles-server"

# Build headless for Linux AMD64 (static binary for containers)
build-headless-linux-amd64:
	@echo "Building headless server for Linux AMD64..."
	@cd frontend && npm run build 2>&1 | grep -v "WARNING\|nesting\|css-syntax-error\|invalid-@nest" || true
	@mkdir -p build/bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags headless $(BUILD_FLAGS) -o build/bin/kubikles-server-linux-amd64
	@echo "Built build/bin/kubikles-server-linux-amd64"

# Build headless for Linux ARM64
build-headless-linux-arm64:
	@echo "Building headless server for Linux ARM64..."
	@cd frontend && npm run build 2>&1 | grep -v "WARNING\|nesting\|css-syntax-error\|invalid-@nest" || true
	@mkdir -p build/bin
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags headless $(BUILD_FLAGS) -o build/bin/kubikles-server-linux-arm64
	@echo "Built build/bin/kubikles-server-linux-arm64"

# Build headless for all Linux platforms
build-headless-all: build-headless-linux-amd64 build-headless-linux-arm64

install-wails:
	go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Install all dependencies (cross-platform setup script)
install-deps: setup

# Full setup: system deps, Go, Node, Wails, frontend, hooks
setup:
	@./scripts/setup.sh

# Setup without system dependencies (if you already have GTK/WebKit)
setup-quick:
	@./scripts/setup.sh --skip-system-deps

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
