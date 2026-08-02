#!/usr/bin/env bash
set -euo pipefail

fail() { echo "accelerator-desktop-provision-scope: $1" >&2; exit 1; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base="4f8fd00b0febb63db0c9f79feaa8350b92946b9f"
cd "$root"
git rev-parse --verify "$base^{commit}" >/dev/null || fail "missing-base"
git merge-base --is-ancestor "$base" HEAD || fail "base-not-ancestor"
test "$(git merge-base "$base" HEAD)" = "$base" || fail "unexpected-merge-base"

# Deliberately inspect the committed feature range, not the caller's working
# tree: a developer's unrelated local edits must neither be accepted as this
# plan's output nor cause this bounded ownership proof to fail.
range="$base...HEAD"
git diff --check "$range" || fail "diff-check"

while IFS= read -r path; do
  case "$path" in
    Makefile|accelerator_provision_desktop.go|accelerator_provision_desktop_test.go|pkg/acceleratorprovision/*|pkg/helm/accelerator.go|pkg/helm/accelerator_stub.go|pkg/helm/accelerator_test.go|pkg/helm/accelerator_types.go|pkg/helm/accelerator_kind_seam.go|pkg/k8s/accelerator_context.go|pkg/k8s/accelerator_context_test.go|scripts/test-accelerator-desktop-provision-kind.sh|scripts/check-accelerator-desktop-provision-scope.sh) ;;
    *) fail "out-of-scope-file:$path" ;;
  esac
done < <(git diff --name-only "$range")

added="$(git diff --unified=0 "$range" -- pkg/acceleratorprovision pkg/helm/accelerator.go | sed -n 's/^+//p' | grep -v '^+++' || true)"
if grep -Eq '"os/exec"|exec\.Command|kubectl|helm (install|upgrade|pull)|UpgradeRelease|locateOCIChart|search' <<<"$added"; then
  fail "forbidden-production-operation"
fi
if git diff --name-only "$range" | grep -Eq '(^App|app_|frontend/|router|tunnel|deploy/charts/|scripts/(publish|sign)|.*release.*publication)'; then
  fail "forbidden-surface"
fi
