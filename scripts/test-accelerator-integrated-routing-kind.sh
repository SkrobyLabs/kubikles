#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { echo "accelerator-integrated-routing-kind: $1" >&2; exit 1; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root/scripts/lib/accelerator-e2e-kind.sh"
reuse_mode="$(accelerator_e2e_reuse_mode)" || fail "invalid-reuse-boundary"

command -v git >/dev/null 2>&1 || fail "missing-git"
test -z "$(git -C "$root" status --porcelain=v1 --untracked-files=all)" || fail "clean-worktree-required"
if [ "$reuse_mode" = standalone ]; then
  source_image="${ACCELERATOR_PROVISION_KIND_IMAGE:-kubikles-accelerator:provision-kind}"
  for tool in docker go; do command -v "$tool" >/dev/null 2>&1 || fail "missing-$tool"; done
  docker image inspect "$source_image" >/dev/null 2>&1 || fail "missing-image-$source_image"
  version="$(docker image inspect "$source_image" --format '{{index .Config.Labels "org.opencontainers.image.version"}}' 2>/dev/null)" || fail "source-image-version-unavailable"
  test "$version" = v1.2.3 || fail "source-image-v1.2.3-required"
  revision="$(docker image inspect "$source_image" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' 2>/dev/null)" || fail "source-image-revision-unavailable"
  head_revision="$(git -C "$root" rev-parse HEAD 2>/dev/null)" || fail "checkout-revision-unavailable"
  test "$revision" = "$head_revision" || fail "source-image-revision-mismatch"
  image_os="$(docker image inspect "$source_image" --format '{{.Os}}' 2>/dev/null)" || fail "source-image-os-unavailable"
  test "$image_os" = linux || fail "source-image-linux-required"
  image_arch="$(docker image inspect "$source_image" --format '{{.Architecture}}' 2>/dev/null)" || fail "source-image-architecture-unavailable"
  host_arch="$(go env GOARCH 2>/dev/null)" || fail "host-goarch-unavailable"
  test -n "$host_arch" && test "$image_arch" = "$host_arch" || fail "source-image-host-goarch-required"
else
  test "${BUILD_VERSION-}" = v0.0.0 || fail "reuse-build-version"
  accelerator_e2e_validate_reused_fixture || fail "reuse-fixture"
fi

ACCELERATOR_INTEGRATED_ROUTING_KIND=1 exec "$root/scripts/test-accelerator-desktop-provision-kind.sh"
