#!/usr/bin/env bash
set -euo pipefail

fail() { echo "accelerator-desktop-lifecycle-scope: $1" >&2; exit 1; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base="57a8deee84359c44bb98d91f695ccae347052083"
cd "$root"
"$root/scripts/check-accelerator-desktop-disposal-scope.sh" || fail "disposal-predecessor"
git merge-base --is-ancestor "$base" HEAD || fail "base-not-ancestor"
git diff --check "$base...HEAD" || fail "diff-check"

while IFS= read -r changed; do
  case "$changed" in
    Makefile|app.go|app_accelerator_runtime.go|app_context.go|accelerator_lifecycle_contract.go|accelerator_lifecycle_desktop.go|accelerator_lifecycle_stub.go|accelerator_lifecycle_test.go|accelerator_lifecycle_stub_test.go|pkg/acceleratorprovision/coordinator*.go|pkg/acceleratorprovision/connector.go|pkg/acceleratorprovision/connector_test.go|pkg/acceleratorprovision/reconnector.go|pkg/acceleratorprovision/reconnector_test.go|pkg/acceleratorprovision/resume_retry.go|pkg/acceleratorprovision/session.go|pkg/acceleratorprovision/disposer.go|pkg/acceleratorprovision/provision_kind_test.go|scripts/test-accelerator-desktop-provision-kind.sh|scripts/test-accelerator-desktop-lifecycle-kind.sh|scripts/check-accelerator-desktop-lifecycle-scope.sh|scripts/check-accelerator-desktop-disposal-scope.sh) ;;
    *) fail "out-of-scope-file:$changed" ;;
  esac
done < <(git diff --name-only "$base...HEAD")

added="$(git diff --unified=0 "$base...HEAD" -- '*.go' ':(exclude)*_test.go' | sed -n 's/^+//p' | grep -v '^+++' || true)"
if grep -Eq 'exec\.Command|kubectl|\.Delete\(|UninstallHelmRelease|AgentRouter.*Supports|emitEvent|runtime\.Events|frontend|keyring|Browser' <<<"$added"; then
  fail "forbidden-production-operation"
fi
if grep -Eq 'ListSecretsMetadata|GetSecretData|GetSecretYaml|CancelListRequest|SubscribeSecretWatcher|UnsubscribeSecretWatcher' <<<"$added"; then
  fail "secret-routing-out-of-scope"
fi
if git diff --name-only "$base...HEAD" | grep -Eq '(^frontend/|^pkg/server/|^deploy/charts/|^pkg/helm/|^pkg/acceleratorrelease/|^accelerator_browser|^browser_)'; then
  fail "forbidden-surface"
fi

echo "accelerator-desktop-lifecycle-scope: passed" >&2
