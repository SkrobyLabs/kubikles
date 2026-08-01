#!/bin/sh
set -eu

fail() { echo "accelerator image validation failed: $*" >&2; exit 1; }

if ! command -v docker >/dev/null 2>&1; then
    fail "Docker is required: install Docker Engine with Buildx and start its daemon."
fi
if ! docker info >/dev/null 2>&1; then
    fail "Docker daemon is unavailable: start Docker Engine and retry."
fi
if ! docker buildx version >/dev/null 2>&1; then
    fail "Docker Buildx is required: install/enable the Buildx plugin and retry."
fi
