#!/usr/bin/env bash
set -euo pipefail

fail() { echo "accelerator-desktop-resume-scope: $1" >&2; exit 1; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base="f1bd21fc47329f90efd9b6362f337aea19030dab"
resume_head="8786785f55f269ca72804e322177542016407702"
cd "$root"
git merge-base --is-ancestor "$base" HEAD || fail "base-not-ancestor"
git merge-base --is-ancestor "$resume_head" HEAD || fail "resume-head-not-ancestor"
range="$base...$resume_head"
git diff --check "$range" || fail "diff-check"

while IFS= read -r changed; do
  case "$changed" in
    Makefile|accelerator_reconnect_desktop.go|accelerator_reconnect_desktop_test.go|pkg/acceleratorprovision/*|pkg/k8s/accelerator_tunnel.go|pkg/k8s/accelerator_tunnel_test.go|scripts/test-accelerator-desktop-resume-kind.sh|scripts/check-accelerator-desktop-connector-scope.sh|scripts/check-accelerator-desktop-resume-scope.sh) ;;
    *) fail "out-of-scope-file:$changed" ;;
  esac
done < <(git diff --name-only "$range")

added="$(git diff --unified=0 "$range" -- pkg/acceleratorprovision ':(exclude)pkg/acceleratorprovision/*_test.go' accelerator_reconnect_desktop.go | sed -n 's/^+//p' | grep -v '^+++' || true)"
if grep -Eq '"os/exec"|exec\.Command|kubectl|Uninstall|DeleteRelease|NewApp|emit|router' <<<"$added"; then
  fail "forbidden-production-operation"
fi
if git diff --name-only "$range" | grep -Eq '(^App|app_|frontend/|router|deploy/charts/|pkg/server/)'; then
  fail "forbidden-surface"
fi
