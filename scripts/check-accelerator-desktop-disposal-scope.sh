#!/usr/bin/env bash
set -euo pipefail

fail() { echo "accelerator-desktop-disposal-scope: $1" >&2; exit 1; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base="8786785f55f269ca72804e322177542016407702"
disposal_head="57a8deee84359c44bb98d91f695ccae347052083"
cd "$root"
"$root/scripts/check-accelerator-desktop-provision-scope.sh" || fail "provision-predecessor"
"$root/scripts/check-accelerator-desktop-connector-scope.sh" || fail "connector-predecessor"
"$root/scripts/check-accelerator-desktop-resume-scope.sh" || fail "resume-predecessor"
git merge-base --is-ancestor "$base" HEAD || fail "base-not-ancestor"
git merge-base --is-ancestor "$disposal_head" HEAD || fail "disposal-head-not-ancestor"
range="$base...$disposal_head"
git diff --check "$range" || fail "diff-check"

while IFS= read -r changed; do
  case "$changed" in
    Makefile|accelerator_dispose_desktop.go|accelerator_dispose_desktop_stub.go|pkg/acceleratorprovision/*|pkg/helm/accelerator.go|pkg/helm/accelerator_cleanup.go|pkg/helm/accelerator_sweep.go|pkg/helm/accelerator_stub.go|pkg/helm/accelerator_test.go|pkg/helm/accelerator_types.go|pkg/helm/accelerator_kind_seam.go|scripts/test-accelerator-desktop-provision-kind.sh|scripts/test-accelerator-desktop-disposal-kind.sh|scripts/check-accelerator-desktop-provision-scope.sh|scripts/check-accelerator-desktop-connector-scope.sh|scripts/check-accelerator-desktop-resume-scope.sh|scripts/check-accelerator-desktop-disposal-scope.sh) ;;
    *) fail "out-of-scope-file:$changed" ;;
  esac
done < <(git diff --name-only "$range")

added="$(git diff --unified=0 "$range" -- pkg/acceleratorprovision ':(exclude)pkg/acceleratorprovision/*_test.go' pkg/helm/accelerator.go pkg/helm/accelerator_cleanup.go pkg/helm/accelerator_sweep.go accelerator_dispose_desktop.go accelerator_dispose_desktop_stub.go | sed -n 's/^+//p' | grep -v '^+++' || true)"
if grep -Eq 'exec\.Command|kubectl|\.Delete\(|DeleteOwnedAcceleratorResources|PurgeOwnedAcceleratorRelease|NewApp|emit|router|frontend' <<<"$added"; then
  fail "forbidden-production-operation"
fi
if git diff --name-only "$range" | grep -Eq '(^app\.go$|^app_|^frontend/|^pkg/server/|^deploy/charts/)'; then
  fail "forbidden-surface"
fi
