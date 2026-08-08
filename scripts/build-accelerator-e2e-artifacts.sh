#!/usr/bin/env bash
set -euo pipefail
umask 077
export LC_ALL=C

fail() { echo "accelerator-e2e-artifacts: $1" >&2; exit 1; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root/scripts/lib/accelerator-e2e-kind.sh"
source "$root/scripts/publish-accelerator-release.sh"

[ "$(accelerator_e2e_reuse_mode)" = reuse ] || fail reuse-boundary
[ "${BUILD_VERSION-}" = v0.0.0 ] || fail build-version
[ "${ACCELERATOR_E2E_OFFLINE-}" = 1 ] || fail offline-mode
[[ "${KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS-}" =~ ^([1-9]|[12][0-9]|30)$ ]] || fail reconnect-grace
artifact_root="${ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT-}"
[[ "$artifact_root" = /* ]] || fail artifact-root
[ -d "$artifact_root" ] || fail artifact-root
[ -z "$(find "$artifact_root" -mindepth 1 -maxdepth 1 -print -quit)" ] || fail artifact-root-not-empty
accelerator_e2e_validate_reused_fixture || fail fixture

for cached in \
  'docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e' \
  'node:20.19.5-bookworm-slim@sha256:9e70124bd00f47dd023e349cd587132ae61892acc0e47ed641416c3e18f401c3' \
  'golang:1.25.12-bookworm@sha256:ea341baa9bd5ba6784f6d7161ace70544349a6242d54d34a0fbfd2c4d51c9d58' \
  'gcr.io/distroless/static-debian12:nonroot@sha256:f5b485ea962d9bd1186b2f6b3a061191539b905b82ec395de78cbfae51f20e35' \
  'curlimages/curl:8.17.0@sha256:935d9100e9ba842cdb060de42472c7ca90cfe9a7c96e4dacb55e79e560b3ff40'; do
  docker image inspect "$cached" >/dev/null 2>&1 || fail missing-offline-image
done

commit="$(git -C "$root" rev-parse HEAD)"
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || fail source-commit
epoch="$(git -C "$root" show -s --format=%ct "$commit")"
[[ "$epoch" =~ ^[1-9][0-9]*$ ]] || fail source-epoch
layout="$artifact_root/image-layout"
chart="$artifact_root/kubikles-accelerator-0.0.0.tgz"
chart_layout="$artifact_root/chart-layout"
publication="$artifact_root/publication"
build_repository="$KUBIKLES_ACCELERATOR_E2E_REGISTRY/skrobylabs/kubikles-accelerator"
build_ref="$build_repository:v0.0.0-build"
acceptance_build_ref="$build_repository:v0.0.0-acceptance-build"
registry_port="${KUBIKLES_ACCELERATOR_E2E_REGISTRY##*:}"
[[ "$registry_port" =~ ^[1-9][0-9]{0,4}$ ]] && [ "$((10#$registry_port))" -le 65535 ] || fail registry-port
daemon_registry="127.0.0.1:$registry_port"
daemon_build_repository="$daemon_registry/skrobylabs/kubikles-accelerator"
work="$artifact_root/offline-build"
source_root="$work/source"
distroless='gcr.io/distroless/static-debian12:nonroot@sha256:f5b485ea962d9bd1186b2f6b3a061191539b905b82ec395de78cbfae51f20e35'
base_tag="${KUBIKLES_ACCELERATOR_E2E_KIND_NAME#kubikles-a60a-}"
local_base="$daemon_registry/skrobylabs/a60a-distroless:$base_tag"
client_base="$KUBIKLES_ACCELERATOR_E2E_REGISTRY/skrobylabs/a60a-distroless:$base_tag"
cleanup_base() { docker image rm "$local_base" >/dev/null 2>&1 || true; }
trap cleanup_base EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir -m 700 "$layout"
mkdir -m 700 -p "$source_root"

git -C "$root" archive "$commit" | tar -x -C "$source_root" || fail source-archive
(cd "$source_root/frontend" && npm ci --offline --include=optional --no-audit --no-fund && test -s ../build/appicon.svg && rm -f src/assets/images/appicon.svg && cp ../build/appicon.svg src/assets/images/appicon.svg && test -s src/assets/images/appicon.svg && BUILD_VERSION=v0.0.0 npm run build) >"$artifact_root/frontend-build.log" 2>&1 || fail offline-frontend-build
ldflags="-s -w -buildid= -X main.BuildVersion=v0.0.0 -X main.GitCommit=$commit -X main.GitDirty=false -X main.acceleratorBuildIdentity=kubikles-accelerator-build-identity:v0.0.0|$commit|false"
for architecture in amd64; do
  binary="$work/$architecture/kubikles-accelerator"
  mkdir -m 700 -p "$(dirname "$binary")"
  (cd "$source_root" && GOTOOLCHAIN=go1.25.12 GOPROXY=off SOURCE_DATE_EPOCH="$epoch" CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" \
    go build -trimpath -buildvcs=false -tags 'headless accelerator' -ldflags "$ldflags" -o "$binary" .) >"$artifact_root/go-build-$architecture.log" 2>&1 || fail offline-go-build
  touch -d "@$epoch" "$binary"
  (cd "$root" && go run ./scripts/cmd/inspect-accelerator-binary "$binary" "$architecture" v0.0.0 "$commit" false) || fail binary-inspection
done
# The acceptance runtime is deliberately a second binary and image. The
# release descriptor above remains evidence for the untagged production image.
acceptance_binary="$work/acceptance/amd64/kubikles-accelerator-acceptance"
mkdir -m 700 -p "$(dirname "$acceptance_binary")"
(cd "$source_root" && GOTOOLCHAIN=go1.25.12 GOPROXY=off SOURCE_DATE_EPOCH="$epoch" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -buildvcs=false -tags 'headless accelerator accelerator_e2e' -ldflags "$ldflags" -o "$acceptance_binary" .) >"$artifact_root/go-build-acceptance-amd64.log" 2>&1 || fail offline-go-build
touch -d "@$epoch" "$acceptance_binary"
go version -m "$acceptance_binary" | grep -F $'\tbuild\t-tags=headless,accelerator,accelerator_e2e' >/dev/null || fail acceptance-build-tags
cleanup_directory offline-source "$source_root" || fail source-cleanup

base_id="$(docker image inspect --format '{{.Id}}' "$distroless")" || fail distroless-inspection
[[ "$base_id" =~ ^sha256:[0-9a-f]{64}$ ]] || fail distroless-identity
base_architecture="$(docker image inspect --format '{{.Architecture}}' "$distroless")" || fail distroless-inspection
[ "$base_architecture" = amd64 ] || fail distroless-identity
docker image tag "$distroless" "$local_base" || fail distroless-alias
[ "$(docker image inspect --format '{{.Id}}' "$local_base")" = "$base_id" ] || fail distroless-alias
docker image push "$local_base" >"$artifact_root/distroless-mirror.log" 2>&1 || fail distroless-mirror
base_digest="$(oras resolve --plain-http "$client_base")" || fail distroless-mirror
[[ "$base_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail distroless-mirror

(
  cd "$root"
  for architecture in amd64; do
    context="$work/$architecture"
    cat >"$context/Dockerfile" <<EOF
FROM --platform=linux/$base_architecture $daemon_registry/skrobylabs/a60a-distroless@$base_digest
ARG BUILD_VERSION
ARG GIT_COMMIT
LABEL org.opencontainers.image.title="kubikles-accelerator" org.opencontainers.image.version="\$BUILD_VERSION" org.opencontainers.image.revision="\$GIT_COMMIT"
COPY --chown=65532:65532 kubikles-accelerator /kubikles-accelerator
USER 65532:65532
ENTRYPOINT ["/kubikles-accelerator"]
EOF
    SOURCE_DATE_EPOCH="$epoch" docker buildx build --network=none --pull=false --file "$context/Dockerfile" --platform "linux/$architecture" --provenance=false --sbom=false \
      --build-arg BUILD_VERSION=v0.0.0 --build-arg "GIT_COMMIT=$commit" \
      --output "type=registry,name=$daemon_build_repository:v0.0.0-$architecture,registry.insecure=true,oci-mediatypes=false,rewrite-timestamp=true" "$context" || exit 1
  done
  oras manifest index create --plain-http "$build_ref" v0.0.0-amd64 || exit 1
  acceptance_context="$work/acceptance/amd64"
  cat >"$acceptance_context/Dockerfile" <<EOF
FROM --platform=linux/$base_architecture $daemon_registry/skrobylabs/a60a-distroless@$base_digest
ARG BUILD_VERSION
ARG GIT_COMMIT
LABEL org.opencontainers.image.title="kubikles-accelerator-acceptance" org.opencontainers.image.version="\$BUILD_VERSION" org.opencontainers.image.revision="\$GIT_COMMIT"
ENV KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS=$KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS
COPY --chown=65532:65532 kubikles-accelerator-acceptance /kubikles-accelerator
USER 65532:65532
ENTRYPOINT ["/kubikles-accelerator"]
EOF
  SOURCE_DATE_EPOCH="$epoch" docker buildx build --network=none --pull=false --file "$acceptance_context/Dockerfile" --platform linux/amd64 --provenance=false --sbom=false \
    --build-arg BUILD_VERSION=v0.0.0 --build-arg "GIT_COMMIT=$commit" \
    --output "type=registry,name=$daemon_build_repository:v0.0.0-acceptance-amd64,registry.insecure=true,oci-mediatypes=false,rewrite-timestamp=true" "$acceptance_context" || exit 1
  oras manifest index create --plain-http "$acceptance_build_ref" v0.0.0-acceptance-amd64 || exit 1
  oras cp --from-plain-http --to-oci-layout "$build_ref" "$layout:v0.0.0" || exit 1
  go run ./internal/acceleratoracceptance/cmd/accelerator-e2e-oci "$layout" || exit 1
) >"$artifact_root/build.log" 2>&1 || fail offline-build
cleanup_base
trap - EXIT INT TERM
cleanup_directory offline-build "$work" || fail build-cleanup

(cd "$root" && go run ./scripts/accelerator-release inspect-oci "$layout" v0.0.0 "$commit" "$artifact_root/image-evidence.json") || fail image-inspection
(cd "$root" && go run ./scripts/accelerator-release package-chart deploy/charts/kubikles-accelerator "$chart" 0.0.0 v0.0.0 "$epoch") || fail chart-package
(cd "$root" && go run ./scripts/accelerator-release package-chart-oci "$chart" deploy/charts/kubikles-accelerator "$chart_layout" 0.0.0 v0.0.0 "$epoch" >/dev/null) || fail chart-layout
(cd "$root" && go run ./scripts/accelerator-release inspect-chart "$chart" deploy/charts/kubikles-accelerator 0.0.0 v0.0.0 "$epoch") || fail chart-inspection

export RUNNER_TEMP="$artifact_root"
init_client_configs
LOCAL_SOURCE_STATE="$(cd "$root" && { git diff --binary HEAD; git diff --binary --cached HEAD; } | sha256sum | cut -d' ' -f1)"
(cd "$root" && publish_registry "$KUBIKLES_ACCELERATOR_E2E_REGISTRY/skrobylabs" true "$layout" "$chart" "$chart_layout" v0.0.0 0.0.0 "$commit" "$publication" "$epoch" absent "$artifact_root/release-absent") || fail registry-publication

install -m 600 "$publication/kubikles-accelerator-release-v0.0.0.json" "$artifact_root/kubikles-accelerator-release-v0.0.0.json"
install -m 600 "$publication/kubikles-accelerator-release-v0.0.0.json.sha256" "$artifact_root/kubikles-accelerator-release-v0.0.0.json.sha256"
jq --arg version v0.0.0 '. + {inspections: [.platforms[] | {architecture:.architecture,binaryBuildVersion:$version,imageBuildVersion:$version}]}' "$artifact_root/image-evidence.json" >"$artifact_root/acceptance-image-evidence.json"
chmod 600 "$artifact_root/acceptance-image-evidence.json"

image_digest="$(cd "$root" && go run ./scripts/accelerator-release field "$artifact_root/kubikles-accelerator-release-v0.0.0.json" image-digest)"
acceptance_image_digest="$(oras resolve --plain-http "$acceptance_build_ref")"
chart_digest="$(cd "$root" && go run ./scripts/accelerator-release field "$artifact_root/kubikles-accelerator-release-v0.0.0.json" chart-digest)"
[ "$(oras resolve --plain-http "$KUBIKLES_ACCELERATOR_E2E_REGISTRY/skrobylabs/kubikles-accelerator:v0.0.0")" = "$image_digest" ] || fail image-readback
[ "$acceptance_image_digest" != "$image_digest" ] || fail acceptance-image-not-distinct
[ "$(oras resolve --plain-http "$KUBIKLES_ACCELERATOR_E2E_REGISTRY/skrobylabs/helm/kubikles-accelerator:0.0.0")" = "$chart_digest" ] || fail chart-readback
host_arch="$(go env GOARCH)"
[ "$host_arch" = amd64 ] || fail host-architecture
selected_digest="$(jq -r --arg arch "$host_arch" '.platforms[] | select(.architecture == $arch and .os == "linux") | .manifestDigest' "$artifact_root/acceptance-image-evidence.json")"
[[ "$selected_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail selected-manifest
jq -n --arg image "$image_digest" --arg acceptance "$acceptance_image_digest" --arg chart "$chart_digest" --arg host "$host_arch" --arg selected "$selected_digest" --argjson grace "$KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS" \
  '{registryImageDigest:$image,acceptanceImageDigest:$acceptance,registryChartDigest:$chart,chartVersion:"0.0.0",chartAppVersion:"v0.0.0",runtimeBuildVersion:"v0.0.0",hostArchitecture:$host,selectedManifestDigest:$selected,reconnectGraceSeconds:$grace}' >"$artifact_root/acceptance-artifact-metadata.json"
chmod 600 "$artifact_root/acceptance-artifact-metadata.json"
find "$artifact_root" -type d -exec chmod 700 {} +
find "$artifact_root" -type f -exec chmod 600 {} +

(cd "$root" && go run ./internal/acceleratoracceptance/cmd/accelerator-e2e-artifact "$artifact_root") || fail acceptance-validation
logout_clients
cleanup_directory client-config "$CLIENT_CONFIG_ROOT" || fail client-config-cleanup
CLIENT_CONFIG_ROOT=''
echo 'accelerator-e2e-artifacts: passed' >&2
