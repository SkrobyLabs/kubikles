#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'accelerator-supply-chain verification: %s\n' "$1" >&2; exit 1; }
[ -n "${BUILD_VERSION:-}" ] || fail build-version-required
go run ./scripts/accelerator-release normalize "$BUILD_VERSION" >/dev/null || fail build-version-invalid
[ -n "${GH_TOKEN:-}" ] || fail github-token-required
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/kubikles-accelerator-supply-verify.XXXXXX")"
chmod 700 "$work"
trap 'find "$work" -type f -delete 2>/dev/null; find "$work" -depth -type d -empty -delete 2>/dev/null' EXIT
GH_TOKEN_VALUE="$GH_TOKEN"; unset GH_TOKEN
github_cli() { GH_TOKEN="$GH_TOKEN_VALUE" gh "$@"; }
if [ -n "${ACCELERATOR_SUPPLY_CHAIN_ASSET_ROOT:-}" ]; then
  [[ "$ACCELERATOR_SUPPLY_CHAIN_ASSET_ROOT" = /* ]] && [ -d "$ACCELERATOR_SUPPLY_CHAIN_ASSET_ROOT" ] || fail asset-root
  find "$ACCELERATOR_SUPPLY_CHAIN_ASSET_ROOT" -maxdepth 1 -type f -exec cp {} "$work/" \;
else
  github_cli release download "$BUILD_VERSION" --repo SkrobyLabs/kubikles --dir "$work" --pattern "kubikles-accelerator-release-$BUILD_VERSION.json" --pattern "kubikles-accelerator-release-$BUILD_VERSION.json.sha256" --pattern "kubikles-accelerator-*.spdx.json" >/dev/null || fail release-assets
fi
descriptor="$work/kubikles-accelerator-release-$BUILD_VERSION.json"
go run "$root/scripts/accelerator-release" verify-descriptor "$descriptor" "$descriptor.sha256" || fail descriptor
[ "$(find "$work" -maxdepth 1 -name '*.spdx.json' -type f | wc -l | tr -d ' ')" = 2 ] || fail sbom-assets
printf 'accelerator-supply-chain verification: passed for %s\n' "$BUILD_VERSION"
