#!/usr/bin/env bash
set -euo pipefail

fail() { echo "accelerator-desktop-resume-scope: $1" >&2; exit 1; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base="f1bd21fc47329f90efd9b6362f337aea19030dab"
cd "$root"
git merge-base --is-ancestor "$base" HEAD || fail "base-not-ancestor"
git diff --check "$base...HEAD" || fail "diff-check"

while IFS= read -r changed; do
  case "$changed" in
    Makefile|accelerator_reconnect_desktop.go|accelerator_reconnect_desktop_test.go|pkg/acceleratorprovision/*|pkg/k8s/accelerator_tunnel.go|pkg/k8s/accelerator_tunnel_test.go|scripts/test-accelerator-desktop-resume-kind.sh|scripts/check-accelerator-desktop-connector-scope.sh|scripts/check-accelerator-desktop-resume-scope.sh) ;;
    *) fail "out-of-scope-file:$changed" ;;
  esac
done < <(git diff --name-only "$base...HEAD")

added="$(git diff --unified=0 "$base...HEAD" -- pkg/acceleratorprovision ':(exclude)pkg/acceleratorprovision/*_test.go' accelerator_reconnect_desktop.go | sed -n 's/^+//p' | grep -v '^+++' || true)"
if grep -Eq '"os/exec"|exec\.Command|kubectl|Uninstall|DeleteRelease|NewApp|emit|router' <<<"$added"; then
  fail "forbidden-production-operation"
fi
if git diff --name-only "$base...HEAD" | grep -Eq '(^App|app_|frontend/|router|deploy/charts/|pkg/server/)'; then
  fail "forbidden-surface"
fi
