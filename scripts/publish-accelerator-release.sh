#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'accelerator publication: %s\n' "$1" >&2; exit 1; }
readonly CHART_SOURCE=deploy/charts/kubikles-accelerator
readonly BUILDKIT_IMAGE='moby/buildkit@sha256:2f5adac4ecd194d9f8c10b7b5d7bceb5186853db1b26e5abd3a657af0b7e26ec'

CLIENT_CONFIG_ROOT=''
ORAS_CONFIG=''
HELM_REGISTRY_CONFIG=''
GH_CONFIG_DIR=''
REGISTRY_AUTHENTICATED=false
LOCAL_SOURCE_STATE=''
RUN_CONTAINER=''
RUN_NETWORK=''
RUN_BUILDER=''
RUN_TMP=''
MODE_TMP=''
GH_TOKEN_VALUE=''
OWNED_CACHE_ROOT=''
SUCCESS_MESSAGE=''
HOSTILE_REGISTRY_TOKEN=''
HOSTILE_HELM_TOKEN=''
HOSTILE_VERIFIER=''
HOSTILE_RAW_TOKEN=''

capture_gh_token() {
  GH_TOKEN_VALUE=${GH_TOKEN:-}
  unset GH_TOKEN
}

github_cli() {
  [ -n "$GH_TOKEN_VALUE" ] || fail "GH_TOKEN is required for GitHub API access"
  GH_TOKEN="$GH_TOKEN_VALUE" gh "$@"
}

resource_state() {
  local kind=$1 name=$2
  local error_file status
  error_file=$(mktemp "${TMPDIR:-/tmp}/accelerator-cleanup-inspect.XXXXXX") || return 1
  case $kind in
    container) docker container inspect "$name" >/dev/null 2>"$error_file"; status=$? ;;
    builder) docker buildx inspect "$name" >/dev/null 2>"$error_file"; status=$? ;;
    network) docker network inspect "$name" >/dev/null 2>"$error_file"; status=$? ;;
    *) rm -f "$error_file"; return 1 ;;
  esac
  if [ "$status" -eq 0 ]; then
    rm -f "$error_file"
    printf 'present\n'
    return 0
  fi
  if grep -Eiq '(no such (object|container|network)|network .+ not found|no builder .+ found|failed to find instance)' "$error_file"; then
    rm -f "$error_file"
    printf 'absent\n'
    return 0
  fi
  rm -f "$error_file"
  return 1
}

remove_resource() {
  local kind=$1 name=$2
  case $kind in
    container) docker rm -f "$name" >/dev/null 2>&1 ;;
    builder) docker buildx rm "$name" >/dev/null 2>&1 ;;
    network) docker network rm "$name" >/dev/null 2>&1 ;;
    *) return 1 ;;
  esac
}

cleanup_resource() {
  local kind=$1 name=$2 state
  [ -n "$name" ] || return 0
  if ! state=$(resource_state "$kind" "$name"); then
    printf 'accelerator publication: could not inspect disposable %s %s during cleanup\n' "$kind" "$name" >&2
    return 1
  fi
  if [ "$state" = present ] && ! remove_resource "$kind" "$name"; then
    printf 'accelerator publication: could not remove disposable %s %s\n' "$kind" "$name" >&2
    return 1
  fi
  if ! state=$(resource_state "$kind" "$name"); then
    printf 'accelerator publication: could not verify disposable %s %s cleanup\n' "$kind" "$name" >&2
    return 1
  fi
  if [ "$state" != absent ]; then
    printf 'accelerator publication: disposable %s %s remains after cleanup\n' "$kind" "$name" >&2
    return 1
  fi
  return 0
}

cleanup_directory() {
  local label=$1 path=$2
  [ -n "$path" ] || return 0
  if ! rm -rf -- "$path"; then
    printf 'accelerator publication: could not remove disposable %s directory\n' "$label" >&2
    return 1
  fi
  if [ -e "$path" ] || [ -L "$path" ]; then
    printf 'accelerator publication: disposable %s directory remains after cleanup\n' "$label" >&2
    return 1
  fi
  return 0
}

cleanup_all() {
  local failed=0
  logout_clients || { printf 'accelerator publication: client logout failed during cleanup\n' >&2; failed=1; }
  cleanup_resource builder "$RUN_BUILDER" || failed=1
  cleanup_resource container "$RUN_CONTAINER" || failed=1
  cleanup_resource network "$RUN_NETWORK" || failed=1
  cleanup_directory run-temporary "$RUN_TMP" || failed=1
  cleanup_directory mode-temporary "$MODE_TMP" || failed=1
  cleanup_directory client-config "$CLIENT_CONFIG_ROOT" || failed=1
  cleanup_directory build-cache "$OWNED_CACHE_ROOT" || failed=1
  [ "$failed" -eq 0 ]
}

cleanup_on_exit() {
  local status=$?
  trap - EXIT INT TERM
  if ! cleanup_all; then
    [ "$status" -ne 0 ] || status=1
  elif [ "$status" -eq 0 ] && [ -n "$SUCCESS_MESSAGE" ]; then
    printf '%s\n' "$SUCCESS_MESSAGE"
  fi
  exit "$status"
}

cleanup_on_signal() {
  local status=$1
  trap - EXIT INT TERM
  cleanup_all || printf 'accelerator publication: cleanup failed while handling signal\n' >&2
  exit "$status"
}

init_client_configs() {
  CLIENT_CONFIG_ROOT=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/accelerator-client-config.XXXXXX")
  chmod 700 "$CLIENT_CONFIG_ROOT"
  ORAS_CONFIG="$CLIENT_CONFIG_ROOT/oras/config.json"
  HELM_REGISTRY_CONFIG="$CLIENT_CONFIG_ROOT/helm/registry.json"
  GH_CONFIG_DIR="$CLIENT_CONFIG_ROOT/gh"
  mkdir -p "$(dirname "$ORAS_CONFIG")" "$(dirname "$HELM_REGISTRY_CONFIG")" "$GH_CONFIG_DIR"
  chmod 700 "$(dirname "$ORAS_CONFIG")" "$(dirname "$HELM_REGISTRY_CONFIG")" "$GH_CONFIG_DIR"
  printf '{"auths":{}}\n' > "$ORAS_CONFIG"
  printf '{"auths":{}}\n' > "$HELM_REGISTRY_CONFIG"
  : > "$GH_CONFIG_DIR/config.yml"
  chmod 600 "$ORAS_CONFIG" "$HELM_REGISTRY_CONFIG" "$GH_CONFIG_DIR/config.yml"
  export GH_CONFIG_DIR
}

logout_clients() {
  if [ "$REGISTRY_AUTHENTICATED" = true ]; then
    oras logout --registry-config "$ORAS_CONFIG" ghcr.io >/dev/null 2>&1 || true
    helm registry logout --registry-config "$HELM_REGISTRY_CONFIG" ghcr.io >/dev/null 2>&1 || true
  fi
}

normalize() { go run ./scripts/accelerator-release normalize "$1" >/dev/null; }

oras_plain_flags() {
  if [ "$1" = true ]; then printf '%s\n' --plain-http; fi
}

# Prints exactly "present DIGEST" or "absent". Only a registry's explicit
# manifest-not-found response is absence; every other error is fatal.
classify_registry_ref() {
  local ref=$1 plain=$2 output error_file status
  error_file=$(mktemp "${TMPDIR:-/tmp}/accelerator-resolve.XXXXXX")
  if [ "$plain" = true ]; then
    if output=$(oras resolve --registry-config "$ORAS_CONFIG" --plain-http "$ref" 2>"$error_file"); then status=0; else status=$?; fi
  else
    if output=$(oras resolve --registry-config "$ORAS_CONFIG" "$ref" 2>"$error_file"); then status=0; else status=$?; fi
  fi
  if [ "$status" -eq 0 ]; then
    rm -f "$error_file"
    [[ "$output" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "registry returned a non-canonical digest"
    printf 'present %s\n' "$output"
    return
  fi
  if grep -Eiq '(^|[^a-z])(manifest unknown|manifest not found|not found)([^a-z]|$)' "$error_file" && ! grep -Eiq '(unauthorized|authentication|required|denied|forbidden|permission|timeout|connection|certificate|tls|server error|status code: 5[0-9][0-9]|invalid|parse)' "$error_file"; then
    rm -f "$error_file"
    printf 'absent\n'
    return
  fi
  rm -f "$error_file"
  fail "registry reference could not be classified authoritatively"
}

state_kind() { printf '%s\n' "${1%% *}"; }
state_digest() { [ "$(state_kind "$1")" = present ] && printf '%s\n' "${1#present }"; }

oras_copy_from_layout() {
  local source=$1 destination=$2 plain=$3
  if [ "$plain" = true ]; then
    oras cp --to-registry-config "$ORAS_CONFIG" --from-oci-layout --to-plain-http "$source" "$destination" >/dev/null
  else
    oras cp --to-registry-config "$ORAS_CONFIG" --from-oci-layout "$source" "$destination" >/dev/null
  fi
}

oras_copy_to_layout() {
  local source=$1 destination=$2 plain=$3
  if [ "$plain" = true ]; then
    oras cp --from-registry-config "$ORAS_CONFIG" --from-plain-http --to-oci-layout "$source" "$destination" >/dev/null
  else
    oras cp --from-registry-config "$ORAS_CONFIG" --to-oci-layout "$source" "$destination" >/dev/null
  fi
}

wrap_single_platform_oci_layout() {
  local layout=$1 tag=$2 descriptor manifest_digest wrapped
  descriptor=$(oras manifest fetch --oci-layout --format go-template --template '{{ .mediaType }} {{ .digest }}' "$layout:$tag") || fail "built image manifest is unavailable"
  [[ "$descriptor" =~ ^application/vnd\.oci\.image\.manifest\.v1\+json\ (sha256:[0-9a-f]{64})$ ]] || fail "built OCI layout root is not one image manifest"
  manifest_digest=${BASH_REMATCH[1]}
  oras manifest index create --oci-layout "$layout:$tag" "$manifest_digest" >/dev/null || fail "built image index could not be created"
  wrapped=$(oras manifest fetch --oci-layout --format go-template --template '{{ .mediaType }} {{ .digest }}' "$layout:$tag") || fail "built image index is unavailable"
  [[ "$wrapped" =~ ^application/vnd\.oci\.image\.index\.v1\+json\ sha256:[0-9a-f]{64}$ ]] || fail "built OCI layout root is not one image index"
}

fetch_chart_manifest() {
  local ref=$1 plain=$2 destination=$3
  if [ "$plain" = true ]; then
    oras manifest fetch --registry-config "$ORAS_CONFIG" --plain-http --output "$destination" "$ref" >/dev/null
  else
    oras manifest fetch --registry-config "$ORAS_CONFIG" --output "$destination" "$ref" >/dev/null
  fi
}

fetch_registry_blob() {
  local repository=$1 digest=$2 plain=$3 destination=$4
  if [ "$plain" = true ]; then
    oras blob fetch --registry-config "$ORAS_CONFIG" --plain-http --output "$destination" "$repository@$digest" >/dev/null
  else
    oras blob fetch --registry-config "$ORAS_CONFIG" --output "$destination" "$repository@$digest" >/dev/null
  fi
}

helm_pull_digest() {
  local repository=$1 digest=$2 plain=$3 destination=$4
  if [ "$plain" = true ]; then
    helm pull --registry-config "$HELM_REGISTRY_CONFIG" "oci://$repository@$digest" --plain-http --destination "$destination" >/dev/null
  else
    helm pull --registry-config "$HELM_REGISTRY_CONFIG" "oci://$repository@$digest" --destination "$destination" >/dev/null
  fi
  # Helm writes pulled archives as 0644 even when the process umask is 077.
  find "$destination" -maxdepth 1 -type f -name '*.tgz' -exec chmod 600 {} +
}

single_chart_archive() {
  local directory=$1
  local -a archives=()
  mapfile -t archives < <(find "$directory" -maxdepth 1 -type f -name '*.tgz' -print)
  [ "${#archives[@]}" -eq 1 ] || fail "digest pull did not produce exactly one chart package"
  printf '%s\n' "${archives[0]}"
}

recheck_source() {
  local version=$1 commit=$2
  if [ "${PUBLICATION_AUTHORITY:-local}" = github ]; then
    [ "$(go run ./scripts/accelerator-release verify-git . "$version" "$commit")" = "$commit" ] || fail "release source identity changed"
  else
    [ "$(git rev-parse HEAD)" = "$commit" ] || fail "local source commit changed"
    [ -n "$LOCAL_SOURCE_STATE" ] || fail "local source preflight state is missing"
    [ "$( { git diff --binary HEAD; git diff --binary --cached HEAD; } | sha256sum | cut -d' ' -f1)" = "$LOCAL_SOURCE_STATE" ] || fail "local tracked source changed"
  fi
}

validate_release_metadata() {
  local version=$1 metadata=$2 tag name draft prerelease expected_prerelease=false
  if [[ "$version" == *-* ]]; then expected_prerelease=true; fi
  IFS=$'\t' read -r tag name draft prerelease <<< "$metadata"
  [ "$tag" = "$version" ] || fail "GitHub Release tag differs from exact BuildVersion"
  [ "$name" = "$version" ] || fail "GitHub Release name differs from exact BuildVersion"
  [ "$draft" = false ] || fail "GitHub Release must not be a draft"
  [ "$prerelease" = "$expected_prerelease" ] || fail "GitHub Release prerelease classification differs from BuildVersion"
}

github_release_state() {
  local version=$1 destination=$2 response error_file metadata
  mkdir -p "$destination"; chmod 700 "$destination"
  [ "$(github_cli api repos/SkrobyLabs/kubikles --jq .full_name 2>/dev/null)" = 'SkrobyLabs/kubikles' ] || fail "GitHub repository access is required before Release classification"
  error_file=$(mktemp "${TMPDIR:-/tmp}/accelerator-release-state.XXXXXX")
  if response=$(github_cli api "repos/SkrobyLabs/kubikles/releases/tags/$version" --jq '([.tag_name, .name, .draft, .prerelease] | @tsv), (.assets[].name)' 2>"$error_file"); then
    rm -f "$error_file"
    metadata=$(printf '%s\n' "$response" | sed -n '1p')
    validate_release_metadata "$version" "$metadata"
    printf '%s\n' "$response" | sed '1d' > "$destination/assets.txt"
    go run ./scripts/accelerator-release verify-release-assets "$version" "$destination/assets.txt"
    printf 'existing\n'
    return
  fi
  if grep -Eq '\(HTTP 404\)$' "$error_file"; then
    rm -f "$error_file"
    printf 'absent\n'
    return
  fi
  rm -f "$error_file"
  fail "GitHub Release could not be classified authoritatively"
}

remote_tag_commit() {
  local version=$1 commit
  commit=$(github_cli api "repos/SkrobyLabs/kubikles/commits/$version" --jq .sha 2>/dev/null) || fail "GitHub tag source commit could not be resolved"
  [[ "$commit" =~ ^[0-9a-f]{40}$ ]] || fail "GitHub tag resolved to a non-canonical commit"
  printf '%s\n' "$commit"
}

recheck_remote_tag() {
  local version=$1 commit=$2
  if [ "${PUBLICATION_AUTHORITY:-local}" = github ]; then
    [ "$(remote_tag_commit "$version")" = "$commit" ] || fail "GitHub tag source moved immediately before registry write"
  fi
}

write_registry_artifact() {
  local version=$1 commit=$2 source=$3 destination=$4 plain=$5
  recheck_remote_tag "$version" "$commit"
  oras_copy_from_layout "$source" "$destination" "$plain"
}

verify_finalized_release() {
  local version=$1 directory=$2
  [ -f "$directory/assets.txt" ] || fail "finalized Release asset evidence is missing"
  go run ./scripts/accelerator-release verify-release-assets "$version" "$directory/assets.txt"
}

recheck_release_state() {
  local version=$1 expected=$2 scratch=$3 state
  if [ "${PUBLICATION_AUTHORITY:-local}" != github ]; then
    [ "$expected" = absent ] || fail "local finalized Release cannot change during publication"
    return
  fi
  rm -rf "$scratch"; mkdir -p "$scratch"; chmod 700 "$scratch"
  state=$(github_release_state "$version" "$scratch")
  [ "$state" = "$expected" ] || fail "GitHub Release finalization state changed during publication"
}

read_back_image() {
  local repository=$1 digest=$2 plain=$3 version=$4 commit=$5 destination=$6
  mkdir -p "$destination/layout"
  oras_copy_to_layout "$repository@$digest" "$destination/layout:$version" "$plain"
  go run ./scripts/accelerator-release inspect-oci "$destination/layout" "$version" "$commit" "$destination/evidence.json"
}

read_back_chart() {
  local repository=$1 digest=$2 plain=$3 package=$4 chart_version=$5 version=$6 epoch=$7 destination=$8
  mkdir -p "$destination/package"
  fetch_chart_manifest "$repository@$digest" "$plain" "$destination/manifest.json"
  local config_digest
  config_digest=$(go run ./scripts/accelerator-release chart-config-digest "$destination/manifest.json" "$digest")
  fetch_registry_blob "$repository" "$config_digest" "$plain" "$destination/config.json"
  go run ./scripts/accelerator-release inspect-chart-manifest "$destination/manifest.json" "$destination/config.json" "$package" "$CHART_SOURCE" "$chart_version" "$version" "$epoch" "$digest"
  helm_pull_digest "$repository" "$digest" "$plain" "$destination/package"
  local archive
  archive=$(single_chart_archive "$destination/package")
  cmp "$package" "$archive" >/dev/null || fail "digest-pulled chart bytes differ"
  go run ./scripts/accelerator-release inspect-chart "$archive" "$CHART_SOURCE" "$chart_version" "$version" "$epoch"
}

publish_registry() {
  [ "$#" -eq 12 ] || fail "internal publication argument mismatch"
  local registry=$1 plain=$2 layout=$3 package=$4 chart_layout=$5 version=$6 chart_version=$7 commit=$8 output=$9 epoch=${10} release_state=${11} release_dir=${12}
  local image_repo="$registry/kubikles-accelerator" chart_repo="$registry/helm/kubikles-accelerator"
  local image_ref="$image_repo:$version" chart_ref="$chart_repo:$chart_version"
  local local_image_evidence="$output/local-image-evidence.json" local_image_digest image_state chart_state image_digest chart_digest
  mkdir -p "$output"; chmod 700 "$output"
  recheck_source "$version" "$commit"
  go run ./scripts/accelerator-release inspect-oci "$layout" "$version" "$commit" "$local_image_evidence"
  go run ./scripts/accelerator-release inspect-chart "$package" "$CHART_SOURCE" "$chart_version" "$version" "$epoch"
  local_image_digest=$(sed -n 's/.*"imageDigest": "\([^"]*\)".*/\1/p' "$local_image_evidence")
  [[ "$local_image_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "validated image evidence has no canonical index digest"

  # Classify and deeply compare the entire immutable set before the first write.
  image_state=$(classify_registry_ref "$image_ref" "$plain")
  chart_state=$(classify_registry_ref "$chart_ref" "$plain")
  if [ "$(state_kind "$image_state")" = present ]; then
    image_digest=$(state_digest "$image_state")
    [ "$image_digest" = "$local_image_digest" ] || fail "existing image conflicts with immutable release content"
    read_back_image "$image_repo" "$image_digest" "$plain" "$version" "$commit" "$output/preflight-image"
    cmp "$local_image_evidence" "$output/preflight-image/evidence.json" >/dev/null || fail "existing image evidence conflicts"
  fi
  if [ "$(state_kind "$chart_state")" = present ]; then
    chart_digest=$(state_digest "$chart_state")
    read_back_chart "$chart_repo" "$chart_digest" "$plain" "$package" "$chart_version" "$version" "$epoch" "$output/preflight-chart"
  fi
  if [ "$release_state" = existing ]; then
    [ "$(state_kind "$image_state")" = present ] && [ "$(state_kind "$chart_state")" = present ] || fail "finalized Release refers to an incomplete registry set"
    verify_finalized_release "$version" "$release_dir"
  elif [ "$release_state" != absent ]; then
    fail "invalid Release finalization state"
  fi

  if [ "$(state_kind "$image_state")" = absent ]; then
    recheck_source "$version" "$commit"
    recheck_release_state "$version" "$release_state" "$output/recheck-release-image"
    [ "$(classify_registry_ref "$image_ref" "$plain")" = absent ] || fail "image reference changed immediately before write"
    [ "$(classify_registry_ref "$chart_ref" "$plain")" = "$chart_state" ] || fail "chart reference changed immediately before image write"
    write_registry_artifact "$version" "$commit" "$layout:$version" "$image_ref" "$plain"
    image_digest=$(state_digest "$(classify_registry_ref "$image_ref" "$plain")")
    [ "$image_digest" = "$local_image_digest" ] || fail "published image digest differs from validated layout"
  else
    image_digest=$(state_digest "$image_state")
  fi

  if [ "$(state_kind "$chart_state")" = absent ]; then
    recheck_source "$version" "$commit"
    recheck_release_state "$version" "$release_state" "$output/recheck-release-chart"
    [ "$(classify_registry_ref "$image_ref" "$plain")" = "present $image_digest" ] || fail "image reference changed immediately before chart write"
    [ "$(classify_registry_ref "$chart_ref" "$plain")" = absent ] || fail "chart reference changed immediately before write"
    write_registry_artifact "$version" "$commit" "$chart_layout:$chart_version" "$chart_ref" "$plain"
    chart_digest=$(state_digest "$(classify_registry_ref "$chart_ref" "$plain")")
  else
    chart_digest=$(state_digest "$chart_state")
  fi
  [[ "$chart_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "registry returned invalid chart digest"

  # Completion evidence comes only from clean digest reads.
  read_back_image "$image_repo" "$image_digest" "$plain" "$version" "$commit" "$output/clean-image"
  cmp "$local_image_evidence" "$output/clean-image/evidence.json" >/dev/null || fail "clean image pull evidence differs"
  read_back_chart "$chart_repo" "$chart_digest" "$plain" "$package" "$chart_version" "$version" "$epoch" "$output/clean-chart"
  if [ "$release_state" = existing ]; then
    verify_finalized_release "$version" "$release_dir"
  fi
}

write_local_asset_contract() {
  local version=$1 directory=$2
  mkdir -p "$directory"; chmod 700 "$directory"
  printf '%s\n' \
    Kubikles-linux-amd64.zip Kubikles-macos-amd64.zip Kubikles-macos-arm64.zip \
    Kubikles-windows-amd64.zip Kubikles-windows-arm64.zip > "$directory/assets.txt"
}

expect_publication_failure() {
  local label=$1 log=$2 status
  shift 2
  set +e
  (set -e; "$@") >"$log" 2>&1
  status=$?
  set -e
  [ "$status" -ne 0 ] || fail "$label was accepted"
}

hostile_secret() {
  local label=$1 seed=$2
  printf '%s' "$label:$seed" | sha256sum | cut -d' ' -f1
}

exercise_hostile_credentials() {
  local registry=$1 chart_digest=$2 version=$3 private=$4
  local registry_auth helm_auth chart_archive chart_repo="$registry/helm/kubikles-accelerator"
  mkdir -m 0700 "$private" "$private/gh" "$private/pull"
  registry_auth=$(printf 'fixture:%s' "$HOSTILE_REGISTRY_TOKEN" | base64 | tr -d '\n')
  helm_auth=$(printf 'fixture:%s' "$HOSTILE_HELM_TOKEN" | base64 | tr -d '\n')
  printf '{"auths":{"%s":{"auth":"%s"}}}\n' "$registry" "$registry_auth" > "$private/oras.json"
  printf '{"auths":{"%s":{"auth":"%s"}}}\n' "$registry" "$helm_auth" > "$private/helm.json"
  printf 'example.invalid:\n    oauth_token: %s\n' "$HOSTILE_RAW_TOKEN" > "$private/gh/hosts.yml"
  printf 'image:\n  reference: %s/kubikles-accelerator:%s\n  version: %s\naccelerator:\n  workloadSessionId: credential-probe\n  allowVersionMismatch: false\nauth:\n  creatorVerifier: %s\nownership:\n  installationId: 101112131415161718191a1b1c1d1e1f\n' \
    "$registry" "$version" "$version" "$HOSTILE_VERIFIER" > "$private/render-values.yaml"
  chmod 600 "$private/oras.json" "$private/helm.json" "$private/gh/hosts.yml" "$private/render-values.yaml"

  if ! (ORAS_CONFIG="$private/oras.json" classify_registry_ref "$chart_repo@$chart_digest" true) >"$private/oras-probe.log" 2>&1; then
    fail "credential-bearing ORAS probe failed"
  fi
  if ! HELM_REGISTRY_CONFIG="$private/helm.json" helm_pull_digest "$chart_repo" "$chart_digest" true "$private/pull" >"$private/helm-pull.log" 2>&1; then
    fail "credential-bearing Helm pull probe failed"
  fi
  chart_archive=$(single_chart_archive "$private/pull")
  if ! HOSTILE_VERIFIER="$HOSTILE_VERIFIER" helm template credential-probe "$chart_archive" --values "$private/render-values.yaml" >"$private/rendered.yaml" 2>"$private/helm-render.log"; then
    fail "credential-bearing Helm render probe failed"
  fi
  if GH_CONFIG_DIR="$private/gh" GH_TOKEN="$HOSTILE_RAW_TOKEN" bash -c 'test -s "$GH_CONFIG_DIR/hosts.yml" && test -n "$GH_TOKEN"; exit 23' >"$private/github-failure.log" 2>&1; then
    fail "credential-bearing GitHub failure probe unexpectedly succeeded"
  fi
  cleanup_directory credential-fixture "$private" || fail "private credential fixture cleanup failed"
}

assert_no_hostile_leaks() {
  local root=$1 hostile
  for hostile in "$HOSTILE_REGISTRY_TOKEN" "$HOSTILE_HELM_TOKEN" "$HOSTILE_VERIFIER" "$HOSTILE_RAW_TOKEN"; do
    [ -n "$hostile" ] || fail "secret-safety fixture is incomplete"
    if grep -R -F "$hostile" "$root" >/dev/null 2>&1; then
      fail "hostile secret leaked to publication logs, metadata, or artifacts"
    fi
  done
}

local_test() {
  local version=v0.0.0 chart_version=0.0.0 commit epoch run registry layout package chart_layout port bind_host client_host endpoint
  local -a cache_args=() offline_build_flags=() builder_args=()
  local canonical_image canonical_chart collision_state after_image after_chart finalized
  if [ "${ACCELERATOR_E2E_OFFLINE:-0}" = 1 ]; then
    [ "${KUBIKLES_ACCELERATOR_E2E_REUSE:-0}" = 1 ] && [ "${BUILD_VERSION:-}" = v0.0.0 ] || fail "offline reuse boundary is incomplete"
    [ -n "${KUBIKLES_ACCELERATOR_E2E_KIND_NAME:-}" ] && [ -n "${KUBIKLES_ACCELERATOR_E2E_KUBECONFIG:-}" ] && [ -n "${KUBIKLES_ACCELERATOR_E2E_NAMESPACE:-}" ] && [ -n "${KUBIKLES_ACCELERATOR_E2E_REGISTRY:-}" ] || fail "offline reuse boundary is incomplete"
    offline_build_flags=(--network=none --pull=false)
  fi
  commit=$(git rev-parse HEAD); [[ "$commit" =~ ^[0-9a-f]{40}$ ]] || fail "source commit must be a full lowercase hash"
  HOSTILE_REGISTRY_TOKEN=$(hostile_secret oras "$commit:$RANDOM")
  HOSTILE_HELM_TOKEN=$(hostile_secret helm "$commit:$RANDOM")
  HOSTILE_VERIFIER="$(hostile_secret verifier "$commit:$RANDOM" | cut -c1-42)A"
  HOSTILE_RAW_TOKEN=$(hostile_secret github "$commit:$RANDOM")
  epoch=$(git show -s --format=%ct "$commit"); normalize "$version"
  LOCAL_SOURCE_STATE=$( { git diff --binary HEAD; git diff --binary --cached HEAD; } | sha256sum | cut -d' ' -f1)
  trap cleanup_on_exit EXIT
  trap 'cleanup_on_signal 130' INT
  trap 'cleanup_on_signal 143' TERM
  init_client_configs
  run="kubikles-accelerator-release-$RANDOM-$$"
  RUN_TMP=$(mktemp -d "${TMPDIR:-/tmp}/kubikles-accelerator-release.XXXXXX"); chmod 700 "$RUN_TMP"
  OWNED_CACHE_ROOT="$RUN_TMP/owned-cache"; mkdir -m 0700 "$OWNED_CACHE_ROOT"
  local tmp=$RUN_TMP
  if [ "${ACCELERATOR_E2E_OFFLINE:-0}" = 1 ]; then
    RUN_CONTAINER=''; RUN_NETWORK=''; RUN_BUILDER=''
    registry="$KUBIKLES_ACCELERATOR_E2E_REGISTRY/publication-$RANDOM-$$"
    curl --noproxy '*' --fail --silent --show-error "http://$KUBIKLES_ACCELERATOR_E2E_REGISTRY/v2/" >/dev/null || fail "reused registry is unreachable"
    layout="$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT/image-layout"
    package="$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT/kubikles-accelerator-0.0.0.tgz"
    chart_layout="$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT/chart-layout"
    go run ./internal/acceleratoracceptance/cmd/accelerator-e2e-artifact "$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT" || fail "offline artifact evidence is invalid"
  else
    RUN_CONTAINER="$run-registry"; RUN_NETWORK="$run-network"; RUN_BUILDER="$run-builder"
    docker buildx create --name "$RUN_BUILDER" --driver docker-container --driver-opt "image=$BUILDKIT_IMAGE" >/dev/null
    docker buildx inspect "$RUN_BUILDER" --bootstrap >/dev/null
    docker network create "$RUN_NETWORK" >/dev/null
    bind_host=${ACCELERATOR_LOCAL_REGISTRY_BIND_HOST:-127.0.0.1}
    client_host=${ACCELERATOR_LOCAL_REGISTRY_CLIENT_HOST:-$bind_host}
    [[ "$bind_host" =~ ^(127\.0\.0\.1|::1)$ ]] || fail "disposable registry bind host must be loopback"
    endpoint=$(docker context inspect "$(docker context show)" --format '{{.Endpoints.docker.Host}}')
    if [[ "$endpoint" != unix://* ]] && [ -z "${ACCELERATOR_LOCAL_REGISTRY_CLIENT_HOST:-}" ]; then
      fail "remote Docker requires explicit ACCELERATOR_LOCAL_REGISTRY_CLIENT_HOST"
    fi
    docker run --detach --rm --name "$RUN_CONTAINER" --network "$RUN_NETWORK" --tmpfs /var/lib/registry:rw,noexec,nosuid,nodev --publish "$bind_host::5000" registry:2.8.3@sha256:a3d8aaa63ed8681a604f1dea0aa03f100d5895b6a58ace528858a7b332415373 >/dev/null
    [ "$(docker inspect -f '{{(index (index .HostConfig.PortBindings "5000/tcp") 0).HostIp}}' "$RUN_CONTAINER")" = "$bind_host" ] || fail "registry was not bound to the configured loopback host"
    port=$(docker inspect -f '{{(index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort}}' "$RUN_CONTAINER")
    [[ "$port" =~ ^[1-9][0-9]*$ ]] || fail "registry published port is invalid"
    registry="$client_host:$port"
    for _ in $(seq 1 30); do curl --fail --silent --show-error "http://$registry/v2/" >/dev/null 2>&1 && break; sleep 0.2; done
    curl --fail --silent --show-error "http://$registry/v2/" >/dev/null || fail "configured registry client endpoint is unreachable; VM-backed Unix-socket Docker may require ACCELERATOR_LOCAL_REGISTRY_CLIENT_HOST=host.docker.internal"
    layout="$tmp/image-layout"; package="$tmp/kubikles-accelerator-$chart_version.tgz"; chart_layout="$tmp/chart-layout"; mkdir -p "$layout"
    if [ -n "${ACCELERATOR_LOCAL_BUILD_CACHE:-}" ]; then
      if [ -f "$ACCELERATOR_LOCAL_BUILD_CACHE/index.json" ]; then
        cache_args=(--cache-from "type=local,src=$ACCELERATOR_LOCAL_BUILD_CACHE")
      else
        mkdir -p "$ACCELERATOR_LOCAL_BUILD_CACHE"; chmod 700 "$ACCELERATOR_LOCAL_BUILD_CACHE"
        cache_args=(--cache-to "type=local,dest=$ACCELERATOR_LOCAL_BUILD_CACHE,mode=max")
      fi
    fi
    builder_args=(--builder "$RUN_BUILDER")
    SOURCE_DATE_EPOCH="$epoch" docker buildx build "${builder_args[@]}" "${offline_build_flags[@]}" --file Dockerfile.accelerator --platform linux/amd64 --provenance=false --sbom=false --build-arg "BUILD_VERSION=$version" --build-arg "GIT_COMMIT=$commit" --build-arg GIT_DIRTY=false --build-arg "SOURCE_DATE_EPOCH=$epoch" --output "type=oci,dest=$layout,tar=false,rewrite-timestamp=true,name=$registry/kubikles-accelerator:$version" "${cache_args[@]}" .
    wrap_single_platform_oci_layout "$layout" "$version"
    go run ./scripts/accelerator-release package-chart "$CHART_SOURCE" "$package" "$chart_version" "$version" "$epoch"
    go run ./scripts/accelerator-release package-chart-oci "$package" "$CHART_SOURCE" "$chart_layout" "$chart_version" "$version" "$epoch" >/dev/null
  fi
  go run ./scripts/accelerator-release inspect-chart "$package" "$CHART_SOURCE" "$chart_version" "$version" "$epoch"
  go run ./scripts/accelerator-release inspect-oci "$layout" "$version" "$commit" "$tmp/pre-image-evidence.json"
  publish_registry "$registry" true "$layout" "$package" "$chart_layout" "$version" "$chart_version" "$commit" "$tmp/first" "$epoch" absent "$tmp/release-absent"
  canonical_image=$(classify_registry_ref "$registry/kubikles-accelerator:$version" true)
  canonical_chart=$(classify_registry_ref "$registry/helm/kubikles-accelerator:$chart_version" true)
  publish_registry "$registry" true "$layout" "$package" "$chart_layout" "$version" "$chart_version" "$commit" "$tmp/second" "$epoch" absent "$tmp/release-absent"
  cmp "$tmp/first/clean-image/evidence.json" "$tmp/second/clean-image/evidence.json" >/dev/null || fail "exact rerun image evidence changed bytes"
  [ "$(classify_registry_ref "$registry/kubikles-accelerator:$version" true)" = "$canonical_image" ] && [ "$(classify_registry_ref "$registry/helm/kubikles-accelerator:$chart_version" true)" = "$canonical_chart" ] || fail "exact rerun mutated registry state"

  # Matching partial sets continue by writing only the absent member.
  oras_copy_from_layout "$layout:$version" "$registry/image-only/kubikles-accelerator:$version" true
  publish_registry "$registry/image-only" true "$layout" "$package" "$chart_layout" "$version" "$chart_version" "$commit" "$tmp/image-only" "$epoch" absent "$tmp/release-absent"
  [ "$(classify_registry_ref "$registry/image-only/kubikles-accelerator:$version" true)" = "$canonical_image" ] || fail "matching image-only continuation changed image evidence"
  oras_copy_from_layout "$chart_layout:$chart_version" "$registry/chart-only/helm/kubikles-accelerator:$chart_version" true
  publish_registry "$registry/chart-only" true "$layout" "$package" "$chart_layout" "$version" "$chart_version" "$commit" "$tmp/chart-only" "$epoch" absent "$tmp/release-absent"
  [ "$(state_digest "$(classify_registry_ref "$registry/chart-only/helm/kubikles-accelerator:$chart_version" true)")" = "$(state_digest "$canonical_chart")" ] || fail "matching chart-only continuation changed chart evidence"

  finalized="$tmp/finalized"; write_local_asset_contract "$version" "$finalized"
  publish_registry "$registry" true "$layout" "$package" "$chart_layout" "$version" "$chart_version" "$commit" "$tmp/finalized-rerun" "$epoch" existing "$finalized"

  # Independent image and chart collisions use isolated repository prefixes.
  printf 'conflict' > "$tmp/conflicting.bin"
  (cd "$tmp" && oras push --registry-config "$ORAS_CONFIG" --plain-http "$registry/image-collision/kubikles-accelerator:$version" 'conflicting.bin:application/octet-stream' >/dev/null)
  collision_state=$(classify_registry_ref "$registry/image-collision/kubikles-accelerator:$version" true)
  expect_publication_failure "conflicting image" "$tmp/image-collision.log" publish_registry "$registry/image-collision" true "$layout" "$package" "$chart_layout" "$version" "$chart_version" "$commit" "$tmp/image-collision" "$epoch" absent "$tmp/release-absent"
  [ "$(classify_registry_ref "$registry/image-collision/kubikles-accelerator:$version" true)" = "$collision_state" ] && [ "$(classify_registry_ref "$registry/image-collision/helm/kubikles-accelerator:$chart_version" true)" = absent ] || fail "image collision caused a registry mutation"

  (cd "$tmp" && oras push --registry-config "$ORAS_CONFIG" --plain-http "$registry/chart-collision/helm/kubikles-accelerator:$chart_version" 'conflicting.bin:application/octet-stream' >/dev/null)
  collision_state=$(classify_registry_ref "$registry/chart-collision/helm/kubikles-accelerator:$chart_version" true)
  expect_publication_failure "conflicting chart" "$tmp/chart-collision.log" publish_registry "$registry/chart-collision" true "$layout" "$package" "$chart_layout" "$version" "$chart_version" "$commit" "$tmp/chart-collision" "$epoch" absent "$tmp/release-absent"
  [ "$(classify_registry_ref "$registry/chart-collision/kubikles-accelerator:$version" true)" = absent ] && [ "$(classify_registry_ref "$registry/chart-collision/helm/kubikles-accelerator:$chart_version" true)" = "$collision_state" ] || fail "chart collision caused a registry mutation"

  # A conflicting finalized asset set must remain zero-write.
  for collision in assets; do
    cp -R "$finalized" "$tmp/finalized-$collision"
    printf 'unexpected.zip\n' >> "$tmp/finalized-$collision/assets.txt"
    expect_publication_failure "finalized $collision collision" "$tmp/finalized-$collision.log" publish_registry "$registry" true "$layout" "$package" "$chart_layout" "$version" "$chart_version" "$commit" "$tmp/finalized-$collision-output" "$epoch" existing "$tmp/finalized-$collision"
    [ "$(classify_registry_ref "$registry/kubikles-accelerator:$version" true)" = "$canonical_image" ] && [ "$(classify_registry_ref "$registry/helm/kubikles-accelerator:$chart_version" true)" = "$canonical_chart" ] || fail "finalized collision mutated registry state"
  done

  # Network/auth-like failures are never classified as absence.
  if (classify_registry_ref '127.0.0.1:1/missing:v0.0.0' true) >"$tmp/unreachable.log" 2>&1; then fail "unreachable registry was classified as absent"; fi
  exercise_hostile_credentials "$registry" "$(state_digest "$canonical_chart")" "$version" "$tmp/private-credential-fixture"
  [ ! -e "$tmp/private-credential-fixture" ] || fail "private credential fixture remains after exercise"
  assert_no_hostile_leaks "$tmp"
  if find "$CLIENT_CONFIG_ROOT" -type f ! -perm 600 -print -quit | grep -q .; then fail "publication client config permissions are too broad"; fi
  if find "$CLIENT_CONFIG_ROOT" -type d ! -perm 700 -print -quit | grep -q .; then fail "publication client config directory permissions are too broad"; fi
  if find "$tmp" -type f \( -name '*evidence*.json' -o -name '*.sha256' -o -name '*.log' -o -name '*.tgz' \) ! -perm 600 -print -quit | grep -q .; then fail "publication evidence permissions are too broad"; fi
  after_image=$(classify_registry_ref "$registry/kubikles-accelerator:$version" true); after_chart=$(classify_registry_ref "$registry/helm/kubikles-accelerator:$chart_version" true)
  [ "$after_image" = "$canonical_image" ] && [ "$after_chart" = "$canonical_chart" ] || fail "local matrix changed canonical registry evidence"
  SUCCESS_MESSAGE='accelerator publication local-test: passed'
}

login_ghcr() {
  [ -n "$GH_TOKEN_VALUE" ] || fail "GH_TOKEN is required for authenticated GHCR verification"
  local actor=${GITHUB_ACTOR:-token}
  printf '%s' "$GH_TOKEN_VALUE" | oras login --registry-config "$ORAS_CONFIG" ghcr.io --username "$actor" --password-stdin >/dev/null
  printf '%s' "$GH_TOKEN_VALUE" | helm registry login --registry-config "$HELM_REGISTRY_CONFIG" ghcr.io --username "$actor" --password-stdin >/dev/null
  REGISTRY_AUTHENTICATED=true
}

verify_ghcr() {
  capture_gh_token
  [ -n "${BUILD_VERSION:-}" ] || fail "BUILD_VERSION=vX.Y.Z is required"; normalize "$BUILD_VERSION"
  [ -n "$GH_TOKEN_VALUE" ] || fail "GH_TOKEN is required for authenticated production verification"
  trap cleanup_on_exit EXIT
  trap 'cleanup_on_signal 130' INT
  trap 'cleanup_on_signal 143' TERM
  init_client_configs
  local tmp image_ref image_repo image_digest chart_ref chart_repo chart_digest chart_version commit remote_commit epoch chart_archive image_state chart_state release_state config_digest
  MODE_TMP=$(mktemp -d "${TMPDIR:-/tmp}/kubikles-ghcr-verify.XXXXXX"); chmod 700 "$MODE_TMP"
  local tmp=$MODE_TMP
  login_ghcr
  release_state=$(github_release_state "$BUILD_VERSION" "$tmp/release")
  [ "$release_state" = existing ] || fail "matching finalized GitHub Release is required"
  image_repo=ghcr.io/skrobylabs/kubikles-accelerator
  chart_repo=ghcr.io/skrobylabs/helm/kubikles-accelerator
  chart_version=${BUILD_VERSION#v}
  image_ref="$image_repo:$BUILD_VERSION"
  chart_ref="$chart_repo:$chart_version"
  commit=$(go run ./scripts/accelerator-release verify-git . "$BUILD_VERSION")
  [ "$(go run ./scripts/accelerator-release verify-git . "$BUILD_VERSION" "$commit")" = "$commit" ] || fail "local release source authority differs"
  remote_commit=$(remote_tag_commit "$BUILD_VERSION"); [ "$remote_commit" = "$commit" ] || fail "GitHub tag source commit differs"
  epoch=$(git show -s --format=%ct "$commit"); [[ "$epoch" =~ ^[1-9][0-9]*$ ]] || fail "release commit epoch is invalid"
  image_state=$(classify_registry_ref "$image_ref" false); [ "$(state_kind "$image_state")" = present ] || fail "GHCR image tag is missing"
  image_digest=$(state_digest "$image_state")
  chart_state=$(classify_registry_ref "$chart_ref" false); [ "$(state_kind "$chart_state")" = present ] || fail "GHCR chart tag is missing"
  chart_digest=$(state_digest "$chart_state")
  read_back_image "$image_repo" "$image_digest" false "$BUILD_VERSION" "$commit" "$tmp/image"
  mkdir -p "$tmp/chart/package"; fetch_chart_manifest "$chart_repo@$chart_digest" false "$tmp/chart/manifest.json"
  config_digest=$(go run ./scripts/accelerator-release chart-config-digest "$tmp/chart/manifest.json" "$chart_digest")
  fetch_registry_blob "$chart_repo" "$config_digest" false "$tmp/chart/config.json"
  helm_pull_digest "$chart_repo" "$chart_digest" false "$tmp/chart/package"
  chart_archive=$(single_chart_archive "$tmp/chart/package")
  go run ./scripts/accelerator-release inspect-chart-manifest "$tmp/chart/manifest.json" "$tmp/chart/config.json" "$chart_archive" "$CHART_SOURCE" "$chart_version" "$BUILD_VERSION" "$epoch" "$chart_digest"
  go run ./scripts/accelerator-release inspect-chart "$chart_archive" "$CHART_SOURCE" "$chart_version" "$BUILD_VERSION" "$epoch"
  helm template release "$chart_archive" --namespace release-verification --set-string image.reference="$image_ref" --set-string image.version="$BUILD_VERSION" --set-string accelerator.workloadSessionId=release-verification --set-string auth.creatorVerifier=w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM --set-string ownership.installationId=101112131415161718191a1b1c1d1e1f >/dev/null
  SUCCESS_MESSAGE="accelerator production verification: passed for $BUILD_VERSION"
}

publish_ghcr() {
  capture_gh_token
  [ -n "${BUILD_VERSION:-}" ] || fail "BUILD_VERSION is required"
  [ -n "${SOURCE_COMMIT:-}" ] || fail "SOURCE_COMMIT is required"
  [ -n "${SOURCE_DATE_EPOCH:-}" ] || fail "SOURCE_DATE_EPOCH is required"
  normalize "$BUILD_VERSION"
  trap cleanup_on_exit EXIT
  trap 'cleanup_on_signal 130' INT
  trap 'cleanup_on_signal 143' TERM
  init_client_configs
  local chart_version=${BUILD_VERSION#v} layout package chart_layout work release_state remote_commit
  MODE_TMP=$(mktemp -d "${RUNNER_TEMP:-/tmp}/kubikles-ghcr-publish.XXXXXX"); chmod 700 "$MODE_TMP"; work=$MODE_TMP
  login_ghcr
  [ "$(go run ./scripts/accelerator-release verify-git . "$BUILD_VERSION" "$SOURCE_COMMIT")" = "$SOURCE_COMMIT" ] || fail "release source differs"
  remote_commit=$(remote_tag_commit "$BUILD_VERSION"); [ "$remote_commit" = "$SOURCE_COMMIT" ] || fail "GitHub tag source differs from workflow preflight"
  [ "$(git show -s --format=%ct "$SOURCE_COMMIT")" = "$SOURCE_DATE_EPOCH" ] || fail "release epoch differs from verified source commit"
  release_state=$(github_release_state "$BUILD_VERSION" "$work/release-state")
  layout="$work/image-layout"; package="$work/kubikles-accelerator-$chart_version.tgz"; chart_layout="$work/chart-layout"; mkdir -p "$layout"
  SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH" docker buildx build --file Dockerfile.accelerator --platform linux/amd64 --provenance=false --sbom=false --build-arg "BUILD_VERSION=$BUILD_VERSION" --build-arg "GIT_COMMIT=$SOURCE_COMMIT" --build-arg GIT_DIRTY=false --build-arg "SOURCE_DATE_EPOCH=$SOURCE_DATE_EPOCH" --output "type=oci,dest=$layout,tar=false,rewrite-timestamp=true,name=ghcr.io/skrobylabs/kubikles-accelerator:$BUILD_VERSION" .
  wrap_single_platform_oci_layout "$layout" "$BUILD_VERSION"
  go run ./scripts/accelerator-release package-chart "$CHART_SOURCE" "$package" "$chart_version" "$BUILD_VERSION" "$SOURCE_DATE_EPOCH"
  go run ./scripts/accelerator-release package-chart-oci "$package" "$CHART_SOURCE" "$chart_layout" "$chart_version" "$BUILD_VERSION" "$SOURCE_DATE_EPOCH" >/dev/null
  PUBLICATION_AUTHORITY=github publish_registry ghcr.io/skrobylabs false "$layout" "$package" "$chart_layout" "$BUILD_VERSION" "$chart_version" "$SOURCE_COMMIT" "$work/publication" "$SOURCE_DATE_EPOCH" "$release_state" "$work/release-state"
}

prepare_release() {
  [ -n "${BUILD_VERSION:-}" ] || fail "BUILD_VERSION is required"
  [ -n "${SOURCE_COMMIT:-}" ] || fail "SOURCE_COMMIT is required"
  [ -n "${SOURCE_DATE_EPOCH:-}" ] || fail "SOURCE_DATE_EPOCH is required"
  [ -n "${ACCELERATOR_PREPARED_OUTPUT:-}" ] || fail "ACCELERATOR_PREPARED_OUTPUT is required"
  normalize "$BUILD_VERSION"
  [[ "$SOURCE_COMMIT" =~ ^[0-9a-f]{40}$ ]] || fail "SOURCE_COMMIT must be a full lowercase hash"
  [ "$(git rev-parse HEAD)" = "$SOURCE_COMMIT" ] || fail "prepared source differs from HEAD"
  [ "$(git show -s --format=%ct "$SOURCE_COMMIT")" = "$SOURCE_DATE_EPOCH" ] || fail "prepared source epoch differs"
  [ ! -e "$ACCELERATOR_PREPARED_OUTPUT" ] || fail "prepared output must be absent"
  mkdir -m 0700 "$ACCELERATOR_PREPARED_OUTPUT"
  trap cleanup_on_exit EXIT
  trap 'cleanup_on_signal 130' INT
  trap 'cleanup_on_signal 143' TERM
  local chart_version=${BUILD_VERSION#v} layout package chart_layout chart_digest
  local -a builder_args=()
  if [ -n "${ACCELERATOR_BUILDER:-}" ]; then builder_args=(--builder "$ACCELERATOR_BUILDER"); fi
  layout="$ACCELERATOR_PREPARED_OUTPUT/image-layout"
  package="$ACCELERATOR_PREPARED_OUTPUT/kubikles-accelerator-$chart_version.tgz"
  chart_layout="$ACCELERATOR_PREPARED_OUTPUT/chart-layout"
  mkdir -m 0700 "$layout"
  SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH" docker buildx build "${builder_args[@]}" --file Dockerfile.accelerator --platform linux/amd64 --provenance=false --sbom=false --build-arg "BUILD_VERSION=$BUILD_VERSION" --build-arg "GIT_COMMIT=$SOURCE_COMMIT" --build-arg GIT_DIRTY=false --build-arg "SOURCE_DATE_EPOCH=$SOURCE_DATE_EPOCH" --output "type=oci,dest=$layout,tar=false,rewrite-timestamp=true,name=ghcr.io/skrobylabs/kubikles-accelerator:$BUILD_VERSION" .
  wrap_single_platform_oci_layout "$layout" "$BUILD_VERSION"
  go run ./scripts/accelerator-release package-chart "$CHART_SOURCE" "$package" "$chart_version" "$BUILD_VERSION" "$SOURCE_DATE_EPOCH"
  chart_digest=$(go run ./scripts/accelerator-release package-chart-oci "$package" "$CHART_SOURCE" "$chart_layout" "$chart_version" "$BUILD_VERSION" "$SOURCE_DATE_EPOCH")
  [[ "$chart_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "prepared chart digest is invalid"
  go run ./scripts/accelerator-release inspect-oci "$layout" "$BUILD_VERSION" "$SOURCE_COMMIT" "$ACCELERATOR_PREPARED_OUTPUT/image-evidence.json"
  go run ./scripts/accelerator-release inspect-chart "$package" "$CHART_SOURCE" "$chart_version" "$BUILD_VERSION" "$SOURCE_DATE_EPOCH"
  jq -n --arg version "$BUILD_VERSION" --arg commit "$SOURCE_COMMIT" --arg chart "$chart_digest" --argjson epoch "$SOURCE_DATE_EPOCH" \
    '{schemaVersion:1,buildVersion:$version,sourceCommit:$commit,sourceEpoch:$epoch,chartDigest:$chart}' > "$ACCELERATOR_PREPARED_OUTPUT/source.json"
  find "$ACCELERATOR_PREPARED_OUTPUT" -type d -exec chmod 0700 {} +
  find "$ACCELERATOR_PREPARED_OUTPUT" -type f -exec chmod 0600 {} +
  SUCCESS_MESSAGE="accelerator release preparation: passed for $BUILD_VERSION"
}

publish_existing_ghcr() {
  capture_gh_token
  [ -n "${BUILD_VERSION:-}" ] || fail "BUILD_VERSION is required"
  [ -n "${SOURCE_COMMIT:-}" ] || fail "SOURCE_COMMIT is required"
  [ -n "${SOURCE_DATE_EPOCH:-}" ] || fail "SOURCE_DATE_EPOCH is required"
  [ -n "${ACCELERATOR_PREPARED_ROOT:-}" ] || fail "ACCELERATOR_PREPARED_ROOT is required"
  normalize "$BUILD_VERSION"
  [ -d "$ACCELERATOR_PREPARED_ROOT/image-layout" ] && [ -d "$ACCELERATOR_PREPARED_ROOT/chart-layout" ] || fail "prepared release layout is incomplete"
  [ -f "$ACCELERATOR_PREPARED_ROOT/source.json" ] && [ -f "$ACCELERATOR_PREPARED_ROOT/image-evidence.json" ] || fail "prepared release evidence is incomplete"
  [ "$(jq -r .buildVersion "$ACCELERATOR_PREPARED_ROOT/source.json")" = "$BUILD_VERSION" ] || fail "prepared BuildVersion differs"
  [ "$(jq -r .sourceCommit "$ACCELERATOR_PREPARED_ROOT/source.json")" = "$SOURCE_COMMIT" ] || fail "prepared source differs"
  [ "$(jq -r .sourceEpoch "$ACCELERATOR_PREPARED_ROOT/source.json")" = "$SOURCE_DATE_EPOCH" ] || fail "prepared epoch differs"
  trap cleanup_on_exit EXIT
  trap 'cleanup_on_signal 130' INT
  trap 'cleanup_on_signal 143' TERM
  init_client_configs
  local chart_version=${BUILD_VERSION#v} package layout chart_layout work release_state remote_commit
  MODE_TMP=$(mktemp -d "${RUNNER_TEMP:-/tmp}/kubikles-ghcr-publish-existing.XXXXXX"); chmod 700 "$MODE_TMP"; work=$MODE_TMP
  package="$ACCELERATOR_PREPARED_ROOT/kubikles-accelerator-$chart_version.tgz"
  layout="$ACCELERATOR_PREPARED_ROOT/image-layout"
  chart_layout="$ACCELERATOR_PREPARED_ROOT/chart-layout"
  [ -f "$package" ] || fail "prepared chart is missing"
  go run ./scripts/accelerator-release inspect-oci "$layout" "$BUILD_VERSION" "$SOURCE_COMMIT" "$work/verified-image.json"
  go run ./scripts/accelerator-release inspect-chart "$package" "$CHART_SOURCE" "$chart_version" "$BUILD_VERSION" "$SOURCE_DATE_EPOCH"
  cmp "$ACCELERATOR_PREPARED_ROOT/image-evidence.json" "$work/verified-image.json" >/dev/null || fail "prepared image evidence differs"
  rm -f "$work/verified-image.json"
  login_ghcr
  [ "$(go run ./scripts/accelerator-release verify-git . "$BUILD_VERSION" "$SOURCE_COMMIT")" = "$SOURCE_COMMIT" ] || fail "release source differs"
  remote_commit=$(remote_tag_commit "$BUILD_VERSION"); [ "$remote_commit" = "$SOURCE_COMMIT" ] || fail "GitHub tag source differs from workflow preflight"
  release_state=$(github_release_state "$BUILD_VERSION" "$work/release-state")
  PUBLICATION_AUTHORITY=github publish_registry ghcr.io/skrobylabs false "$layout" "$package" "$chart_layout" "$BUILD_VERSION" "$chart_version" "$SOURCE_COMMIT" "$work/publication" "$SOURCE_DATE_EPOCH" "$release_state" "$work/release-state"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  [ "$#" -eq 1 ] || fail "usage: publish-accelerator-release.sh local-test|prepare|publish-existing|ghcr|verify-ghcr"
  case $1 in local-test) local_test;; prepare) prepare_release;; publish-existing) publish_existing_ghcr;; ghcr) publish_ghcr;; verify-ghcr) verify_ghcr;; *) fail "unknown publication mode";; esac
fi
