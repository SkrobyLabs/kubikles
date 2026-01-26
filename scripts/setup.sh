#!/usr/bin/env bash
# Cross-platform setup script for Kubikles development environment
# Supports: Arch/CachyOS, Debian/Ubuntu, Fedora, macOS

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[OK]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Detect OS and package manager
detect_os() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        OS="macos"
        PKG_MGR="brew"
    elif [[ -f /etc/os-release ]]; then
        . /etc/os-release
        case "$ID" in
            arch|cachyos|manjaro|endeavouros|garuda)
                OS="arch"
                PKG_MGR="pacman"
                ;;
            debian|ubuntu|pop|linuxmint|elementary)
                OS="debian"
                PKG_MGR="apt"
                ;;
            fedora|rhel|centos|rocky|almalinux)
                OS="fedora"
                PKG_MGR="dnf"
                ;;
            opensuse*|suse*)
                OS="suse"
                PKG_MGR="zypper"
                ;;
            *)
                OS="unknown"
                PKG_MGR="unknown"
                ;;
        esac
    else
        OS="unknown"
        PKG_MGR="unknown"
    fi
    info "Detected OS: $OS (package manager: $PKG_MGR)"
}

# Check if a command exists
has_cmd() {
    command -v "$1" &>/dev/null
}

# Install system dependencies based on OS
install_system_deps() {
    info "Installing system dependencies for Wails..."

    case "$PKG_MGR" in
        pacman)
            # Arch-based: CachyOS, Manjaro, EndeavourOS, etc.
            local pkgs="gtk3 webkit2gtk base-devel"
            info "Installing: $pkgs"
            sudo pacman -S --needed --noconfirm $pkgs
            ;;
        apt)
            # Debian/Ubuntu-based
            sudo apt-get update
            # Check for webkit2gtk-4.1 (Ubuntu 24.04+) vs 4.0 (older)
            if apt-cache show libwebkit2gtk-4.1-dev &>/dev/null; then
                info "Using webkit2gtk-4.1 (modern)"
                sudo apt-get install -y build-essential libgtk-3-dev libwebkit2gtk-4.1-dev
                warn "Note: Build with -tags webkit2_41 flag"
            else
                info "Using webkit2gtk-4.0"
                sudo apt-get install -y build-essential libgtk-3-dev libwebkit2gtk-4.0-dev
            fi
            ;;
        dnf)
            # Fedora/RHEL-based
            sudo dnf install -y gtk3-devel webkit2gtk4.1-devel gcc-c++
            ;;
        zypper)
            # openSUSE
            sudo zypper install -y gtk3-devel webkit2gtk3-devel gcc-c++
            ;;
        brew)
            # macOS - no system deps needed, but ensure Xcode CLI tools
            if ! xcode-select -p &>/dev/null; then
                info "Installing Xcode Command Line Tools..."
                xcode-select --install
                echo "Please complete the Xcode CLI tools installation and re-run this script."
                exit 0
            fi
            ;;
        *)
            error "Unsupported package manager: $PKG_MGR"
            echo ""
            echo "Please manually install:"
            echo "  - GTK3 development headers"
            echo "  - WebKit2GTK development headers (4.0 or 4.1)"
            echo "  - GCC/build tools"
            echo ""
            echo "Then re-run this script."
            exit 1
            ;;
    esac
    success "System dependencies installed"
}

# Install Go
install_go() {
    if has_cmd go; then
        local go_version=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | head -1)
        success "Go already installed: $go_version"
        return 0
    fi

    info "Installing Go..."
    case "$PKG_MGR" in
        pacman)
            sudo pacman -S --needed --noconfirm go
            ;;
        apt)
            # Use the golang package or suggest manual install for latest
            if apt-cache show golang-go &>/dev/null; then
                sudo apt-get install -y golang-go
            else
                error "Go not found in apt repositories."
                echo "Install from https://go.dev/dl/ or use a PPA"
                exit 1
            fi
            ;;
        dnf)
            sudo dnf install -y golang
            ;;
        zypper)
            sudo zypper install -y go
            ;;
        brew)
            brew install go
            ;;
        *)
            error "Please install Go manually from https://go.dev/dl/"
            exit 1
            ;;
    esac
    success "Go installed"
}

# Install Node.js
install_node() {
    if has_cmd node; then
        local node_version=$(node --version)
        success "Node.js already installed: $node_version"
        return 0
    fi

    info "Installing Node.js..."
    case "$PKG_MGR" in
        pacman)
            sudo pacman -S --needed --noconfirm nodejs npm
            ;;
        apt)
            # Install from NodeSource for recent version, or use distro package
            if apt-cache show nodejs &>/dev/null; then
                sudo apt-get install -y nodejs npm
            else
                error "Node.js not found. Install from https://nodejs.org/"
                exit 1
            fi
            ;;
        dnf)
            sudo dnf install -y nodejs npm
            ;;
        zypper)
            sudo zypper install -y nodejs npm
            ;;
        brew)
            brew install node
            ;;
        *)
            error "Please install Node.js manually from https://nodejs.org/"
            exit 1
            ;;
    esac
    success "Node.js installed"
}

# Setup Go environment
setup_go_env() {
    local gopath=$(go env GOPATH 2>/dev/null || echo "$HOME/go")
    local gobin="$gopath/bin"

    # Check if GOPATH/bin is in PATH
    if [[ ":$PATH:" != *":$gobin:"* ]]; then
        warn "GOPATH/bin not in PATH"
        echo ""
        echo "Add this to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
        echo ""
        echo "  export PATH=\"\$PATH:$gobin\""
        echo ""

        # Temporarily add for this session
        export PATH="$PATH:$gobin"
        info "Added to PATH for this session"
    fi
}

# Install Wails CLI
install_wails() {
    setup_go_env

    if has_cmd wails; then
        local wails_version=$(wails version 2>/dev/null | head -1)
        success "Wails already installed: $wails_version"
        return 0
    fi

    info "Installing Wails CLI..."
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
    success "Wails installed"
}

# Install frontend dependencies
install_frontend() {
    if [[ -d "frontend" ]]; then
        info "Installing frontend dependencies..."
        cd frontend
        npm install
        cd ..
        success "Frontend dependencies installed"
    else
        warn "No frontend directory found, skipping npm install"
    fi
}

# Install git hooks
install_hooks() {
    if [[ -d ".githooks" ]]; then
        info "Installing git hooks..."
        cp .githooks/* .git/hooks/ 2>/dev/null || true
        chmod +x .git/hooks/* 2>/dev/null || true
        success "Git hooks installed"
    fi
}

# Run wails doctor to verify setup
run_doctor() {
    echo ""
    info "Running 'wails doctor' to verify setup..."
    echo ""
    wails doctor || true
    echo ""
}

# Main
main() {
    echo ""
    echo "=========================================="
    echo "  Kubikles Development Setup"
    echo "=========================================="
    echo ""

    detect_os
    echo ""

    # Check for --skip-system-deps flag
    if [[ "$1" != "--skip-system-deps" ]]; then
        install_system_deps
    else
        info "Skipping system dependencies (--skip-system-deps)"
    fi

    install_go
    install_node
    install_wails
    install_frontend
    install_hooks
    run_doctor

    echo ""
    echo "=========================================="
    success "Setup complete!"
    echo "=========================================="
    echo ""
    echo "Next steps:"
    echo "  make dev          # Start development server"
    echo "  make build        # Build for current platform"
    echo "  make profile      # Build with profiling for PGO"
    echo ""
}

main "$@"
