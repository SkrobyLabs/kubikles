#!/usr/bin/env bash
set -euo pipefail

fail() { echo "accelerator-desktop-connector-scope: $1" >&2; exit 1; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base="b88c370ff69bef16853a31a142ba924510ed64b6"
cd "$root"
git merge-base --is-ancestor "$base" HEAD || fail "base-not-ancestor"
git diff --check "$base...HEAD" || fail "diff-check"

while IFS= read -r changed; do
  case "$changed" in
    Makefile|accelerator_connect_desktop.go|accelerator_connect_desktop_test.go|pkg/acceleratorprovision/*|pkg/k8s/accelerator_tunnel.go|pkg/k8s/accelerator_tunnel_test.go|scripts/test-accelerator-desktop-connector-kind.sh|scripts/check-accelerator-desktop-connector-scope.sh|scripts/check-accelerator-desktop-provision-scope.sh) ;;
    *) fail "out-of-scope-file:$changed" ;;
  esac
done < <(git diff --name-only "$base...HEAD")

added="$(git diff --unified=0 "$base...HEAD" -- pkg/acceleratorprovision ':(exclude)pkg/acceleratorprovision/*_test.go' pkg/k8s/accelerator_tunnel.go accelerator_connect_desktop.go | sed -n 's/^+//p' | grep -v '^+++' || true)"
if grep -Eq '"os/exec"|exec\.Command|kubectl|localhost|0\.0\.0\.0|\[::\]|reconnect|Uninstall|DeleteRelease' <<<"$added"; then
  fail "forbidden-production-operation"
fi
if git diff --name-only "$base...HEAD" | grep -Eq '(^App|app_|frontend/|router|deploy/charts/|scripts/(publish|sign))'; then
  fail "forbidden-surface"
fi
