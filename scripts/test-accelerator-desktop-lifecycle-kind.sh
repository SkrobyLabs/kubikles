#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"$root/scripts/check-accelerator-desktop-lifecycle-scope.sh"
ACCELERATOR_LIFECYCLE_KIND=1 exec "$root/scripts/test-accelerator-desktop-provision-kind.sh"
