#!/bin/sh
# compress-dist.sh - Pre-compress frontend/dist assets with gzip for smaller Go binary.
# Run AFTER vite build, BEFORE go build.
# POSIX-compatible (works on macOS, Linux, Windows/MSYS/Git Bash).

set -e

# Determine DIST_DIR based on current working directory
# Script can be called from project root (frontend/dist) or frontend dir (dist)
if [ $# -gt 0 ]; then
  DIST_DIR="$1"
elif [ -d "dist" ]; then
  # Running from frontend directory
  DIST_DIR="dist"
elif [ -d "frontend/dist" ]; then
  # Running from project root
  DIST_DIR="frontend/dist"
else
  echo "Error: dist directory not found in current directory or frontend/dist" >&2
  exit 1
fi

if [ ! -d "$DIST_DIR" ]; then
  echo "Error: dist directory not found: $DIST_DIR" >&2
  exit 1
fi

# Remove stats.html if present (build analysis artifact, not needed in binary)
rm -f "$DIST_DIR/stats.html"

# Gzip compressible text assets (skip fonts — woff/woff2 are already compressed).
# Keep index.html uncompressed — Wails requires it in the embed.FS for startup
# validation. The middleware serves the .gz variant for all other files.
find "$DIST_DIR" -type f \( -name "*.js" -o -name "*.css" -o -name "*.html" -o -name "*.svg" \) | while read -r file; do
  gzip -9 -k "$file"            # -k keeps the original
  if [ "$(basename "$file")" != "index.html" ]; then
    rm "$file"                   # remove original for everything except index.html
  fi
done

# Count compressed files for reporting
total=$(find "$DIST_DIR" -type f -name "*.gz" | wc -l | tr -d ' ')
echo "compress-dist: gzipped $total files in $DIST_DIR"
