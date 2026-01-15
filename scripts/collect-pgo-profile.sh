#!/bin/bash
# collect-pgo-profile.sh
# Collects a CPU profile for Profile-Guided Optimization (PGO)
#
# This script:
# 1. Starts the app with pprof enabled
# 2. Waits for user to exercise the app
# 3. Captures CPU profile on exit
# 4. Converts to PGO format (default.pgo)

set -e

PROFILE_DURATION=${PROFILE_DURATION:-30}
PPROF_PORT=${PPROF_PORT:-6060}
PGO_FILE="default.pgo"
CPU_PROFILE="cpu.pprof"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Find the built app - macOS uses .app bundle, others use direct binary
if [ -d "build/bin/Kubikles.app" ]; then
    APP_BINARY="build/bin/Kubikles.app/Contents/MacOS/Kubikles"
elif [ -f "build/bin/Kubikles" ]; then
    APP_BINARY="build/bin/Kubikles"
elif [ -f "build/bin/kubikles" ]; then
    APP_BINARY="build/bin/kubikles"
else
    echo "Error: Could not find built application in build/bin/"
    echo "Run 'make profile' to build first."
    exit 1
fi

echo -e "${GREEN}Starting Kubikles with profiling...${NC}"
echo "Binary: $APP_BINARY"
echo ""

# Start the profiled app in background
PPROF_PORT=$PPROF_PORT "$APP_BINARY" &
APP_PID=$!

# Wait for pprof to be ready
sleep 2

# Check if app started successfully
if ! kill -0 $APP_PID 2>/dev/null; then
    echo "Error: App failed to start"
    exit 1
fi

echo -e "${GREEN}App running (PID: $APP_PID)${NC}"
echo -e "${YELLOW}pprof available at: http://localhost:$PPROF_PORT/debug/pprof/${NC}"
echo ""
echo "Use the app normally. When done, press Enter to capture profile..."
echo "(Or wait ${PROFILE_DURATION}s for automatic capture)"
echo ""

# Start profile collection in background
(
    sleep 2  # Let app settle
    echo "Collecting CPU profile for ${PROFILE_DURATION} seconds..."
    curl -s "http://localhost:$PPROF_PORT/debug/pprof/profile?seconds=$PROFILE_DURATION" > $CPU_PROFILE
    echo -e "${GREEN}Profile saved to $CPU_PROFILE${NC}"
) &
PROFILE_PID=$!

# Wait for user input or profile completion
read -t $((PROFILE_DURATION + 5)) -p "" || true

# Stop the app gracefully
echo ""
echo "Stopping app..."
kill $APP_PID 2>/dev/null || true
wait $APP_PID 2>/dev/null || true

# Wait for profile collection to complete
wait $PROFILE_PID 2>/dev/null || true

# Check if profile was captured
if [ ! -f "$CPU_PROFILE" ] || [ ! -s "$CPU_PROFILE" ]; then
    echo "Warning: CPU profile not captured or empty"
    echo "You can manually capture with:"
    echo "  curl -o $CPU_PROFILE 'http://localhost:$PPROF_PORT/debug/pprof/profile?seconds=30'"
    exit 1
fi

# Convert to PGO format (Go 1.21+ uses pprof directly as default.pgo)
cp $CPU_PROFILE $PGO_FILE

echo ""
echo -e "${GREEN}=== PGO Profile Ready ===${NC}"
echo "Profile saved to: $PGO_FILE"
echo "Profile size: $(du -h $PGO_FILE | cut -f1)"
echo ""
echo "To build with PGO optimization:"
echo "  make build-pgo"
echo "  make build-mac-arm-pgo  # For Apple Silicon release"
echo ""
