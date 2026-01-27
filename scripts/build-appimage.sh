#!/usr/bin/env bash
# Build AppImage for Kubikles
# Creates a portable Linux executable that bundles GTK/WebKit dependencies

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_ROOT/build"
APPDIR="$BUILD_DIR/Kubikles.AppDir"
ARCH="${ARCH:-x86_64}"
APPIMAGETOOL="$BUILD_DIR/appimagetool-$ARCH.AppImage"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[OK]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Download appimagetool if not present
download_appimagetool() {
    if [[ -x "$APPIMAGETOOL" ]]; then
        info "appimagetool already present"
        return 0
    fi

    info "Downloading appimagetool..."
    curl -L -o "$APPIMAGETOOL" \
        "https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-$ARCH.AppImage"
    chmod +x "$APPIMAGETOOL"
    success "appimagetool downloaded"
}

# Build the wails binary if not present
build_binary() {
    local binary="$BUILD_DIR/bin/kubikles"

    if [[ ! -f "$binary" ]]; then
        info "Building Kubikles binary..."
        cd "$PROJECT_ROOT"

        # Use wails from GOPATH if not in PATH
        local wails_cmd="wails"
        if ! command -v wails &>/dev/null; then
            wails_cmd="$(go env GOPATH)/bin/wails"
        fi

        $wails_cmd build -platform linux/amd64 -trimpath -ldflags "-s -w"
        success "Binary built"
    else
        info "Binary already exists at $binary"
    fi
}

# Create AppDir structure
create_appdir() {
    info "Creating AppDir structure..."

    rm -rf "$APPDIR"
    mkdir -p "$APPDIR/usr/bin"
    mkdir -p "$APPDIR/usr/lib"
    mkdir -p "$APPDIR/usr/share/icons/hicolor/256x256/apps"

    # Copy binary
    cp "$BUILD_DIR/bin/kubikles" "$APPDIR/usr/bin/"

    # Copy desktop file
    cp "$BUILD_DIR/appimage/kubikles.desktop" "$APPDIR/"

    # Copy icon (square app icon, not the landscape logo)
    cp "$PROJECT_ROOT/build/appicon.png" \
        "$APPDIR/usr/share/icons/hicolor/256x256/apps/kubikles.png"
    cp "$PROJECT_ROOT/build/appicon.png" \
        "$APPDIR/kubikles.png"

    # Create AppRun script
    cat > "$APPDIR/AppRun" << 'EOF'
#!/bin/bash
SELF=$(readlink -f "$0")
HERE=${SELF%/*}
export PATH="${HERE}/usr/bin:${PATH}"
export LD_LIBRARY_PATH="${HERE}/usr/lib:${LD_LIBRARY_PATH}"
exec "${HERE}/usr/bin/kubikles" "$@"
EOF
    chmod +x "$APPDIR/AppRun"

    success "AppDir created"
}

# Bundle required libraries (optional - for more portable AppImage)
bundle_libs() {
    info "Bundling shared libraries..."

    local binary="$APPDIR/usr/bin/kubikles"
    local lib_dir="$APPDIR/usr/lib"

    # Get list of required libraries (excluding core system libs)
    # We bundle GTK/WebKit related libs for portability
    local libs_to_bundle=(
        "libwebkit2gtk"
        "libjavascriptcoregtk"
        "libsoup"
    )

    # This is a simplified approach - for full portability you might want to use
    # linuxdeploy or similar tools that handle library bundling more comprehensively

    for lib_pattern in "${libs_to_bundle[@]}"; do
        for lib in $(ldd "$binary" 2>/dev/null | grep "$lib_pattern" | awk '{print $3}'); do
            if [[ -f "$lib" ]]; then
                cp -n "$lib" "$lib_dir/" 2>/dev/null || true
            fi
        done
    done

    success "Libraries bundled (basic set)"
    echo ""
    info "Note: For maximum portability, consider using linuxdeploy instead:"
    echo "  https://github.com/linuxdeploy/linuxdeploy"
}

# Create the AppImage
create_appimage() {
    info "Creating AppImage..."

    cd "$BUILD_DIR"

    # Extract appimagetool if FUSE is not available (common in containers/CI)
    if ! "$APPIMAGETOOL" --version &>/dev/null 2>&1; then
        info "Extracting appimagetool (FUSE not available)..."
        "$APPIMAGETOOL" --appimage-extract &>/dev/null
        ./squashfs-root/AppRun "$APPDIR" "Kubikles-$ARCH.AppImage"
        rm -rf squashfs-root
    else
        "$APPIMAGETOOL" "$APPDIR" "Kubikles-$ARCH.AppImage"
    fi

    chmod +x "Kubikles-$ARCH.AppImage"

    success "AppImage created: $BUILD_DIR/Kubikles-$ARCH.AppImage"
}

# Main
main() {
    echo ""
    echo "=========================================="
    echo "  Kubikles AppImage Builder"
    echo "=========================================="
    echo ""

    cd "$PROJECT_ROOT"

    download_appimagetool
    build_binary
    create_appdir

    # Skip library bundling by default (WebKit is complex)
    # Users on target systems need GTK3 + WebKit2GTK runtime libs
    # Uncomment for experimental lib bundling:
    # bundle_libs

    create_appimage

    echo ""
    echo "=========================================="
    success "Build complete!"
    echo "=========================================="
    echo ""
    echo "Output: $BUILD_DIR/Kubikles-$ARCH.AppImage"
    echo ""
    echo "To run:"
    echo "  ./build/Kubikles-$ARCH.AppImage"
    echo ""
    echo "Note: Target systems need GTK3 and WebKit2GTK runtime libraries."
    echo "See README.md for installation instructions."
    echo ""
}

main "$@"
