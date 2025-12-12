# Makefile for Kubikles

.PHONY: dev build build-release build-windows-amd64 build-windows-arm64 build-mac build-mac-arm build-linux-amd64 build-linux-arm64 build-all install-wails install-deps install-frontend clean

# Ensure GOPATH/bin is in PATH
GOPATH := $(shell go env GOPATH)
WAILS := $(GOPATH)/bin/wails
BUILD_FLAGS := -trimpath -ldflags "-s -w"
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
	@echo "Done! Run 'make dev' to start development server."
else ifeq ($(OS),Windows_NT)
	@echo "Installing dependencies for Windows..."
	@command -v go >/dev/null 2>&1 || { echo "Go not found. Install from https://go.dev/dl/"; exit 1; }
	@command -v node >/dev/null 2>&1 || { echo "Node.js not found. Install from https://nodejs.org/"; exit 1; }
	@echo "Installing Wails..."
	@go install github.com/wailsapp/wails/v2/cmd/wails@latest
	@echo "Installing frontend dependencies..."
	@cd frontend && npm install
	@echo "Done! Run 'make dev' to start development server."
else
	@echo "Installing dependencies for Linux..."
	@command -v go >/dev/null 2>&1 || { echo "Go not found. Install from https://go.dev/dl/"; exit 1; }
	@command -v node >/dev/null 2>&1 || { echo "Node.js not found. Install from https://nodejs.org/"; exit 1; }
	@echo "Installing Wails..."
	@go install github.com/wailsapp/wails/v2/cmd/wails@latest
	@echo "Installing frontend dependencies..."
	@cd frontend && npm install
	@echo "Note: For cross-compiling to Windows, install mingw-w64"
	@echo "Done! Run 'make dev' to start development server."
endif

# Install frontend dependencies only
install-frontend:
	cd frontend && npm install

clean:
	rm -rf build/bin/*
