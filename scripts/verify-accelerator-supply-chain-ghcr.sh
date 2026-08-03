#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'accelerator-supply-chain verification: %s\n' "$1" >&2; exit 1; }
[ -n "${BUILD_VERSION:-}" ] || fail build-version-required
[[ "$BUILD_VERSION" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || fail build-version-invalid
[ -n "${GH_TOKEN:-}" ] || fail github-token-required
command -v gh >/dev/null 2>&1 || fail gh-required
[ "$(gh --version | sed -n '1p')" = 'gh version 2.97.0 (2026-07-31)' ] || fail gh-version
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
commit="$(go run "$root/scripts/accelerator-release" verify-git "$root" "$BUILD_VERSION")" || fail source-identity
identity="https://github.com/SkrobyLabs/kubikles/.github/workflows/release.yml@refs/tags/$BUILD_VERSION"
work="$(mktemp -d "${TMPDIR:-/tmp}/kubikles-accelerator-supply-verify.XXXXXX")"
chmod 700 "$work"
trap 'find "$work" -type f -delete 2>/dev/null; find "$work" -depth -type d -empty -delete 2>/dev/null' EXIT
GH_TOKEN_VALUE="$GH_TOKEN"; unset GH_TOKEN
github_cli() { GH_TOKEN="$GH_TOKEN_VALUE" gh "$@"; }
if [ -n "${ACCELERATOR_SUPPLY_CHAIN_ASSET_ROOT:-}" ]; then
  [[ "$ACCELERATOR_SUPPLY_CHAIN_ASSET_ROOT" = /* ]] && [ -d "$ACCELERATOR_SUPPLY_CHAIN_ASSET_ROOT" ] || fail asset-root
  find "$ACCELERATOR_SUPPLY_CHAIN_ASSET_ROOT" -maxdepth 1 -type f -exec cp {} "$work/" \;
else
  github_cli release download "$BUILD_VERSION" --repo SkrobyLabs/kubikles --dir "$work" --pattern "kubikles-accelerator-release-$BUILD_VERSION.json" --pattern "kubikles-accelerator-release-$BUILD_VERSION.json.sha256" --pattern "kubikles-accelerator-*.spdx.json" --pattern "kubikles-accelerator-attestations-$BUILD_VERSION.jsonl" >/dev/null || fail release-assets
fi
descriptor="$work/kubikles-accelerator-release-$BUILD_VERSION.json"
go run "$root/scripts/accelerator-release" verify-descriptor "$descriptor" "$descriptor.sha256" || fail descriptor
image="$(go run "$root/scripts/accelerator-release" field "$descriptor" image-reference)"
chart="$(go run "$root/scripts/accelerator-release" field "$descriptor" chart-reference)"; chart="${chart#oci://}"
for subject in "$image" "$chart"; do
  github_cli attestation verify "oci://$subject" --repo SkrobyLabs/kubikles --signer-workflow SkrobyLabs/kubikles/.github/workflows/release.yml --cert-identity "$identity" --source-ref "refs/tags/$BUILD_VERSION" --source-digest "$commit" >/dev/null || fail attestation-identity
done
[ "$(find "$work" -maxdepth 1 -name '*.spdx.json' -type f | wc -l | tr -d ' ')" = 3 ] || fail sbom-assets
[ "$(wc -l < "$work/kubikles-accelerator-attestations-$BUILD_VERSION.jsonl" | tr -d ' ')" = 6 ] || fail attestation-bundles
printf 'accelerator-supply-chain verification: passed for %s\n' "$BUILD_VERSION"
