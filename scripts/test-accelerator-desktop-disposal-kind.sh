#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"$root/scripts/check-accelerator-desktop-disposal-scope.sh"
ACCELERATOR_DISPOSAL_KIND=1 "$root/scripts/test-accelerator-desktop-provision-kind.sh"
echo "accelerator-desktop-disposal-kind: passed" >&2
