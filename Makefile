# Makefile for Kubikles
# Cross-platform: works on Windows (MSYS/Git Bash), macOS, and Linux

.PHONY: help dev build build-release build-lite build-release-lite build-windows-amd64 build-windows-arm64 build-mac build-mac-arm build-linux-amd64 build-linux-arm64 build-appimage build-all install-wails install-deps setup setup-quick install-frontend install-hooks clean test test-frontend test-watch typecheck lint lint-go lint-fix fmt profile build-pgo cluster-up cluster-down cluster-status cluster-load install-kind appicon analyze-size install-gsa generate

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
	@echo "  build              Build for current platform (includes Helm)"
	@echo "  build-release      Build optimized portable executable (includes Helm)"
	@echo "  build-lite         Build for current platform WITHOUT Helm (smaller binary)"
	@echo "  build-release-lite Build optimized portable WITHOUT Helm (smaller binary)"
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
	@echo "  typecheck          Run TypeScript type checking (tsc --noEmit)"
	@echo ""
	@echo "Linting:"
	@echo "  lint               Run all linters"
	@echo "  lint-go            Run Go linter (golangci-lint)"
	@echo "  lint-fix           Run Go linter with auto-fix"
	@echo "  fmt                Format Go code (gofmt + goimports)"
	@echo ""
	@echo "Profile-Guided Optimization:"
	@echo "  profile            Collect CPU profile for PGO optimization"
	@echo "  build-pgo          Build with PGO optimization"
	@echo "  build-mac-arm-pgo  Build Apple Silicon release with PGO"
	@echo "  clean-pgo          Remove PGO profile files"
	@echo ""
	@echo "Local Test Cluster:"
	@echo "  cluster-up         Start a local K8s cluster (kind or minikube)"
	@echo "  cluster-down       Stop and delete the local cluster"
	@echo "  cluster-status     Show cluster status"
	@echo "  cluster-load       Load sample resources into cluster"
	@echo "  install-kind       Install kind (Kubernetes IN Docker)"
	@echo ""
	@echo "Analysis:"
	@echo "  analyze-size       Analyze binary size (produces HTML + text reports)"
	@echo "  install-gsa        Install go-size-analyzer (gsa) tool"
	@echo ""
	@echo "Code Generation:"
	@echo "  generate           Regenerate dispatch_gen.go (go generate)"
	@echo ""
	@echo "Utilities:"
	@echo "  clean              Remove build artifacts"
	@echo ""

# OS Detection - check for Windows (works in MSYS/Git Bash)
ifeq ($(OS),Windows_NT)
    DETECTED_OS := Windows
else
    DETECTED_OS := $(shell uname -s)
endif

# Find wails - check PATH first, then common Go bin locations
WAILS := $(shell command -v wails 2>/dev/null || echo "$(HOME)/go/bin/wails")
ifeq (,$(wildcard $(WAILS)))
    $(error wails not found. Run 'make install-wails' or add ~/go/bin to PATH)
endif
PGO_FILE := default.pgo

# Build tags: default builds include Helm; "lite" builds exclude it for smaller binaries
BUILD_TAGS := helm

# Version info from git
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo "")
GIT_DIRTY := $(shell git diff --quiet 2>/dev/null && echo "false" || echo "true")
VERSION_LDFLAGS := -X main.GitCommit=$(GIT_COMMIT) -X main.GitDirty=$(GIT_DIRTY)
BUILD_FLAGS := -trimpath -ldflags "-s -w $(VERSION_LDFLAGS)"

# Generate app icon PNG from SVG source (Wails generates icon.ico from this)
build/appicon.png: build/appicon.svg
	magick -background none $< -resize 1024x1024 $@

appicon: build/appicon.png

dev:
	$(WAILS) dev -tags "debugcluster $(BUILD_TAGS)"

build: appicon
	$(WAILS) build -tags "$(BUILD_TAGS)" -ldflags "$(VERSION_LDFLAGS)"

# Build optimized portable executable for current platform
build-release: appicon
	$(WAILS) build -tags "$(BUILD_TAGS)" $(BUILD_FLAGS)

# Build WITHOUT Helm for smaller binary (lite variant)
build-lite: appicon
	$(WAILS) build -ldflags "$(VERSION_LDFLAGS)"

# Build optimized portable WITHOUT Helm (lite variant)
build-release-lite: appicon
	$(WAILS) build $(BUILD_FLAGS)

# Build portable Windows executables (requires mingw-w64 on non-Windows: brew install mingw-w64)
build-windows-amd64: appicon
	$(WAILS) build -platform windows/amd64 -tags "$(BUILD_TAGS)" $(BUILD_FLAGS) -o Kubikles-amd64.exe

build-windows-arm64: appicon
	$(WAILS) build -platform windows/arm64 -tags "$(BUILD_TAGS)" $(BUILD_FLAGS) -o Kubikles-arm64.exe

# Build portable macOS executable
build-mac: appicon
	$(WAILS) build -platform darwin/amd64 -tags "$(BUILD_TAGS)" $(BUILD_FLAGS) -o Kubikles-amd64

# Build portable macOS ARM executable (Apple Silicon)
build-mac-arm: appicon
	$(WAILS) build -platform darwin/arm64 -tags "$(BUILD_TAGS)" $(BUILD_FLAGS) -o Kubikles-arm64

# Build portable Linux executables
build-linux-amd64: appicon
	$(WAILS) build -platform linux/amd64 -tags "$(BUILD_TAGS)" $(BUILD_FLAGS) -o Kubikles-linux-amd64

build-linux-arm64: appicon
	$(WAILS) build -platform linux/arm64 -tags "$(BUILD_TAGS)" $(BUILD_FLAGS) -o Kubikles-linux-arm64

# Build portable Linux AppImage (bundles into single executable) - Unix only
build-appimage:
ifeq ($(DETECTED_OS),Windows)
	@echo "AppImage builds are only supported on Linux"
else
	@./scripts/build-appimage.sh
endif

# Build all platforms (clean+appicon first, then platform builds in parallel)
build-all: clean appicon
	$(MAKE) build-windows-amd64 build-windows-arm64 build-mac build-mac-arm build-linux-amd64 build-linux-arm64 -j6

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
	CGO_ENABLED=0 go build -tags "headless $(BUILD_TAGS)" $(BUILD_FLAGS) -o build/bin/kubikles-server
	@echo "Built build/bin/kubikles-server"

# Build headless for Linux AMD64 (static binary for containers)
build-headless-linux-amd64:
	@echo "Building headless server for Linux AMD64..."
	@cd frontend && npm run build 2>&1 | grep -v "WARNING\|nesting\|css-syntax-error\|invalid-@nest" || true
	@mkdir -p build/bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags "headless $(BUILD_TAGS)" $(BUILD_FLAGS) -o build/bin/kubikles-server-linux-amd64
	@echo "Built build/bin/kubikles-server-linux-amd64"

# Build headless for Linux ARM64
build-headless-linux-arm64:
	@echo "Building headless server for Linux ARM64..."
	@cd frontend && npm run build 2>&1 | grep -v "WARNING\|nesting\|css-syntax-error\|invalid-@nest" || true
	@mkdir -p build/bin
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags "headless $(BUILD_TAGS)" $(BUILD_FLAGS) -o build/bin/kubikles-server-linux-arm64
	@echo "Built build/bin/kubikles-server-linux-arm64"

# Build headless for all Linux platforms
build-headless-all: build-headless-linux-amd64 build-headless-linux-arm64

install-wails:
	go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Install all dependencies (cross-platform setup script)
install-deps: setup

# Full setup: system deps, Go, Node, Wails, frontend, hooks
setup:
ifeq ($(DETECTED_OS),Windows)
	@echo "On Windows, please ensure you have installed:"
	@echo "  1. Go: https://go.dev/dl/"
	@echo "  2. Node.js: https://nodejs.org/"
	@echo "  3. Then run: make install-wails"
	@echo "  4. Then run: make install-frontend"
else
	@./scripts/setup.sh
endif

# Setup without system dependencies (if you already have GTK/WebKit)
setup-quick:
ifeq ($(DETECTED_OS),Windows)
	@echo "On Windows, run: make install-wails && make install-frontend"
else
	@./scripts/setup.sh --skip-system-deps
endif

# Install frontend dependencies only
install-frontend:
	cd frontend && npm install

# Install git hooks from tracked .githooks directory
install-hooks:
	@echo "Installing git hooks..."
	@mkdir -p .git/hooks
	@cp .githooks/* .git/hooks/
	@chmod +x .git/hooks/* 2>/dev/null || true
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

# Run TypeScript type checking (zero errors = gate passes)
typecheck:
	cd frontend && npm run typecheck

# ===========================================
# Linting
# ===========================================

# Run all linters
lint: lint-go

# Run Go linter (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
lint-go:
	golangci-lint run ./...

# Run Go linter with auto-fix
lint-fix:
	golangci-lint run --fix ./...

# Format Go code
fmt:
	gofmt -w .
	goimports -w .

# Regenerate code (dispatch_gen.go for server-mode method dispatch)
generate:
	go generate ./...

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
	$(WAILS) build -tags "profiling" -skipbindings
	@echo ""
	@echo "=== PGO Profile Collection ==="
	@echo "1. App will start with pprof enabled on port 6060"
	@echo "2. Use the app normally for 30-60 seconds:"
	@echo "   - Switch contexts, browse resources"
	@echo "   - Open logs, terminals"
	@echo "   - View dependency graphs"
	@echo "3. Press Ctrl+C when done"
	@echo ""
ifeq ($(DETECTED_OS),Windows)
	@echo "Profile collection scripts are Unix-only. Run the app manually."
else
	@echo "Starting profile collection..."
	@./scripts/collect-pgo-profile.sh
endif

# Build with PGO optimization (requires profile from 'make profile')
build-pgo:
	@if [ ! -f "$(PGO_FILE)" ]; then \
		echo "Error: $(PGO_FILE) not found. Run 'make profile' first to generate it."; \
		exit 1; \
	fi
	@echo "Building with PGO optimization from $(PGO_FILE)..."
	$(WAILS) build $(BUILD_FLAGS) -tags "pgo $(BUILD_TAGS)"

# Build optimized release for Apple Silicon with PGO (macOS only)
build-mac-arm-pgo:
ifeq ($(DETECTED_OS),Windows)
	@echo "This target is only available on macOS"
else
	@if [ ! -f "$(PGO_FILE)" ]; then \
		echo "Error: $(PGO_FILE) not found. Run 'make profile' first."; \
		exit 1; \
	fi
	@echo "Building Apple Silicon release with PGO..."
	GOFLAGS="-pgo=$(PGO_FILE)" $(WAILS) build -platform darwin/arm64 $(BUILD_FLAGS) -o Kubikles-arm64-pgo
endif

# Clean PGO profile
clean-pgo:
	rm -f $(PGO_FILE) cpu.pprof

# =============================================================================
# Local Test Cluster
# =============================================================================

CLUSTER_NAME := kubikles-dev
# Find kind - check PATH first, then ~/go/bin
KIND := $(shell command -v kind 2>/dev/null || (test -x "$(HOME)/go/bin/kind" && echo "$(HOME)/go/bin/kind"))

# Find gsa (go-size-analyzer) - check PATH first, then ~/go/bin
GSA := $(shell command -v gsa 2>/dev/null || (test -x "$(HOME)/go/bin/gsa" && echo "$(HOME)/go/bin/gsa"))


# Install kind if not present (requires Docker to run)
install-kind:
	@if [ -n "$(KIND)" ]; then \
		echo "kind is already installed: $$($(KIND) version)"; \
	else \
		echo "Installing kind..."; \
		CGO_ENABLED=0 go install sigs.k8s.io/kind@latest; \
		echo "kind installed to ~/go/bin/kind"; \
		echo "Note: kind requires Docker to create clusters."; \
	fi

# Start local cluster - prefers kind (requires Docker), falls back to minikube
cluster-up:
	@if [ -n "$(KIND)" ]; then \
		if ! docker info >/dev/null 2>&1; then \
			echo "Error: Docker is not running (required for kind)"; \
			echo ""; \
			echo "Install Docker via one of:"; \
			echo "  - Docker Desktop: https://docker.com/products/docker-desktop"; \
			echo "  - OrbStack (macOS): https://orbstack.dev"; \
			echo "  - Colima (macOS): brew install colima && colima start"; \
			echo ""; \
			echo "Or use minikube with a VM driver."; \
			exit 1; \
		fi; \
		echo "Starting kind cluster '$(CLUSTER_NAME)'..."; \
		if $(KIND) get clusters 2>/dev/null | grep -q "^$(CLUSTER_NAME)$$"; then \
			echo "Cluster already exists. Use 'make cluster-down' to remove it first."; \
		else \
			$(KIND) create cluster --name $(CLUSTER_NAME) --wait 60s && \
			echo "" && \
			echo "Cluster ready! Context set to: kind-$(CLUSTER_NAME)" && \
			kubectl cluster-info --context kind-$(CLUSTER_NAME); \
		fi; \
	elif command -v minikube >/dev/null 2>&1; then \
		echo "Starting minikube cluster '$(CLUSTER_NAME)'..."; \
		if minikube status -p $(CLUSTER_NAME) 2>/dev/null | grep -q "Running"; then \
			echo "Cluster already running."; \
		else \
			minikube start -p $(CLUSTER_NAME) --memory=2048 --cpus=2 && \
			echo "" && \
			echo "Cluster ready! Context set to: $(CLUSTER_NAME)"; \
		fi; \
	else \
		echo "Error: Neither kind nor minikube found."; \
		echo "Install kind with: make install-kind (requires Docker)"; \
		echo "Or install minikube from: https://minikube.sigs.k8s.io/"; \
		exit 1; \
	fi

# Stop and delete local cluster
cluster-down:
	@if [ -n "$(KIND)" ] && $(KIND) get clusters 2>/dev/null | grep -q "^$(CLUSTER_NAME)$$"; then \
		echo "Deleting kind cluster '$(CLUSTER_NAME)'..."; \
		$(KIND) delete cluster --name $(CLUSTER_NAME); \
	elif command -v minikube >/dev/null 2>&1 && minikube status -p $(CLUSTER_NAME) >/dev/null 2>&1; then \
		echo "Deleting minikube cluster '$(CLUSTER_NAME)'..."; \
		minikube delete -p $(CLUSTER_NAME); \
	else \
		echo "No cluster '$(CLUSTER_NAME)' found."; \
	fi

# Show cluster status
cluster-status:
	@echo "=== Cluster Status ==="
	@if [ -n "$(KIND)" ] && $(KIND) get clusters 2>/dev/null | grep -q "^$(CLUSTER_NAME)$$"; then \
		echo "Kind cluster '$(CLUSTER_NAME)' exists"; \
		kubectl cluster-info --context kind-$(CLUSTER_NAME) 2>/dev/null || echo "  (not reachable)"; \
	elif command -v minikube >/dev/null 2>&1; then \
		minikube status -p $(CLUSTER_NAME) 2>/dev/null || echo "Minikube cluster '$(CLUSTER_NAME)' not found"; \
	else \
		echo "No cluster tools (kind/minikube) found"; \
	fi
	@echo ""
	@echo "=== Current Context ==="
	@kubectl config current-context 2>/dev/null || echo "No context set"
	@echo ""
	@echo "=== Namespaces ==="
	@kubectl get namespaces 2>/dev/null || echo "Cannot reach cluster"

# Load sample resources for testing
cluster-load:
	@echo "Creating sample resources in cluster..."
	@kubectl create namespace demo --dry-run=client -o yaml | kubectl apply -f -
	@kubectl create namespace staging --dry-run=client -o yaml | kubectl apply -f -
	@echo ""
	@echo "Creating sample deployment..."
	@kubectl create deployment nginx --image=nginx:alpine -n demo --dry-run=client -o yaml | kubectl apply -f -
	@kubectl scale deployment nginx -n demo --replicas=3
	@echo ""
	@echo "Creating sample service..."
	@kubectl expose deployment nginx -n demo --port=80 --dry-run=client -o yaml | kubectl apply -f -
	@echo ""
	@echo "Creating sample configmap..."
	@kubectl create configmap app-config -n demo --from-literal=env=development --from-literal=debug=true --dry-run=client -o yaml | kubectl apply -f -
	@echo ""
	@echo "Creating sample secret..."
	@kubectl create secret generic app-secret -n demo --from-literal=api-key=test123 --dry-run=client -o yaml | kubectl apply -f -
	@echo ""
	@echo "Creating sample cronjob..."
	@kubectl create cronjob hello -n demo --image=busybox --schedule="*/5 * * * *" -- echo "Hello from CronJob" --dry-run=client -o yaml | kubectl apply -f -
	@echo ""
	@echo "=== Resources Created ==="
	@kubectl get all,configmaps,secrets -n demo
	@echo ""
	@echo "Sample resources loaded! Run 'make dev' to test Kubikles."

# =============================================================================
# Binary Size Analysis
# =============================================================================
# Uses go-size-analyzer (gsa) to produce per-package size breakdowns.
# Install: make install-gsa (requires internet or pre-cached Go modules)

# Install gsa if not present
install-gsa:
	@if [ -n "$(GSA)" ]; then \
		echo "gsa is already installed: $(GSA)"; \
	else \
		echo "Installing go-size-analyzer (gsa)..."; \
		go install github.com/Zxilly/go-size-analyzer/cmd/gsa@latest; \
		echo "gsa installed to ~/go/bin/gsa"; \
		echo "Note: Under airgap, pre-cache the module in your Go module cache."; \
	fi

# Analyze binary size - auto-detects binary path based on OS
# Override with: BINARY=path/to/binary make analyze-size
analyze-size:
ifndef BINARY
ifeq ($(DETECTED_OS),Darwin)
	$(eval BINARY := $(wildcard build/bin/kubikles.app/Contents/MacOS/kubikles))
else ifeq ($(DETECTED_OS),Windows)
	$(eval BINARY := $(or $(wildcard build/bin/Kubikles-amd64.exe),$(wildcard build/bin/Kubikles-arm64.exe)))
else
	$(eval BINARY := $(or $(wildcard build/bin/Kubikles-linux-amd64),$(wildcard build/bin/Kubikles-linux-arm64),$(wildcard build/bin/kubikles-server)))
endif
endif
	@if [ -z "$(BINARY)" ]; then \
		echo "Error: No binary found. Build first with 'make build' or specify: BINARY=path make analyze-size"; \
		exit 1; \
	fi
	@if [ ! -f "$(BINARY)" ]; then \
		echo "Error: Binary not found at $(BINARY)"; \
		exit 1; \
	fi
	@if [ -z "$(GSA)" ]; then \
		echo "Error: gsa not found. Install with: make install-gsa"; \
		exit 1; \
	fi
	@mkdir -p build
	@echo "Analyzing binary: $(BINARY)"
	@echo "Binary size: $$(du -h "$(BINARY)" | cut -f1)"
	@echo ""
	@echo "Generating HTML report -> build/size-report.html"
	@$(GSA) --format html -o build/size-report.html "$(BINARY)"
	@echo "Generating text report -> build/size-report.txt"
	@$(GSA) --format text -o build/size-report.txt "$(BINARY)"
	@echo ""
	@echo "=== Top Packages by Size ==="
	@head -40 build/size-report.txt
	@echo ""
	@echo "Full reports: build/size-report.html (interactive), build/size-report.txt (text)"

