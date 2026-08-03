#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test -f "$root/pkg/acceleratorprovision/provision_kind_test.go" &&
  grep -Fq 'func exerciseBrowserLifecycleKind' "$root/pkg/acceleratorprovision/provision_kind_test.go" || {
  echo "accelerator-browser-lifecycle-kind: missing-browser-substitute" >&2
  exit 1
}
ACCELERATOR_BROWSER_LIFECYCLE_KIND=1 exec "$root/scripts/test-accelerator-desktop-provision-kind.sh"
