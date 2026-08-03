#!/bin/sh
set -eu

image=${ACCELERATOR_IMAGE:?ACCELERATOR_IMAGE is required}
: "${BUILD_VERSION:?BUILD_VERSION is required}"
: "${GIT_COMMIT:?GIT_COMMIT is required}"
: "${GIT_DIRTY:?GIT_DIRTY is required}"
: "${SOURCE_DATE_EPOCH:?SOURCE_DATE_EPOCH is required}"

base='gcr.io/distroless/static-debian12:nonroot@sha256:f5b485ea962d9bd1186b2f6b3a061191539b905b82ec395de78cbfae51f20e35'
curl_image='curlimages/curl:8.17.0@sha256:935d9100e9ba842cdb060de42472c7ca90cfe9a7c96e4dacb55e79e560b3ff40'
raw_token='AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'
creator_verifier='rAY0e5uoqjCBVWihMVgIpKLuPEHE5OCyhzVZh3X93EM'
tmp=$(mktemp -d)
clean_context="$tmp/clean-context"
cid=
control_cid=
first_image=
second_image=
version_image=
commit_image=
first_repro_cid=
second_repro_cid=
version_cid=
commit_cid=
offline=false
build_cache_flags=--no-cache
offline_build_flags=

if [ "${ACCELERATOR_E2E_OFFLINE:-0}" = 1 ]; then
    [ "${KUBIKLES_ACCELERATOR_E2E_REUSE:-0}" = 1 ] || { echo "accelerator image validation failed: offline reuse boundary is incomplete" >&2; exit 1; }
    [ "$BUILD_VERSION" = v0.0.0 ] || { echo "accelerator image validation failed: offline BuildVersion must be v0.0.0" >&2; exit 1; }
    offline=true
    build_cache_flags=
    offline_build_flags='--network=none --pull=false'
fi

fail() { echo "accelerator image validation failed: $*" >&2; exit 1; }
cleanup() {
    [ -z "$cid" ] || docker rm -f "$cid" >/dev/null 2>&1 || true
    [ -z "$control_cid" ] || docker rm -f "$control_cid" >/dev/null 2>&1 || true
    [ -z "$first_image" ] || docker image rm -f "$first_image" >/dev/null 2>&1 || true
    [ -z "$second_image" ] || docker image rm -f "$second_image" >/dev/null 2>&1 || true
    [ -z "$version_image" ] || docker image rm -f "$version_image" >/dev/null 2>&1 || true
    [ -z "$commit_image" ] || docker image rm -f "$commit_image" >/dev/null 2>&1 || true
    [ -z "$first_repro_cid" ] || docker rm -f "$first_repro_cid" >/dev/null 2>&1 || true
    [ -z "$second_repro_cid" ] || docker rm -f "$second_repro_cid" >/dev/null 2>&1 || true
    [ -z "$version_cid" ] || docker rm -f "$version_cid" >/dev/null 2>&1 || true
    [ -z "$commit_cid" ] || docker rm -f "$commit_cid" >/dev/null 2>&1 || true
    [ "$offline" = false ] || docker image rm -f "$image" >/dev/null 2>&1 || true
    rm -rf "$tmp"
}
trap cleanup EXIT INT TERM

# Reproducibility builds must not inherit ignored generated frontend files from
# the host checkout. The Dockerfile is responsible for deriving appicon.svg
# from its tracked canonical source.
mkdir "$clean_context"
git ls-files -z | tar --null --files-from=- -cf - | tar -xf - -C "$clean_context"
test -s "$clean_context/build/appicon.svg" || fail "tracked canonical app icon is missing"
test ! -e "$clean_context/frontend/src/assets/images/appicon.svg" || fail "clean Docker context unexpectedly contains generated app icon"

inspect_config() { docker image inspect "$1" --format '{{json .Config}}'; }
config=$(inspect_config "$image")
case "$config" in *'"User":"65532:65532"'*) ;; *) fail "image user is not 65532:65532";; esac
case "$config" in *'"Entrypoint":["/kubikles-accelerator"]'*) ;; *) fail "image entrypoint is not exact";; esac
case "$(docker image inspect "$image" --format '{{json .Config.Cmd}}')" in null|[]) ;; *) fail "image command must be empty";; esac
case "$(docker image inspect "$image" --format '{{json .Config.ExposedPorts}}')" in null|{}) ;; *) fail "image exposes a port";; esac
case "$(docker image inspect "$image" --format '{{json .Config.Volumes}}')" in null|{}) ;; *) fail "image declares a volume";; esac
case "$(docker image inspect "$image" --format '{{json .Config.Healthcheck}}')" in null) ;; *) fail "image declares a healthcheck";; esac
case "$config" in *'"HOME='*|*'"XDG_'*|*'"TMPDIR='*) fail "image declares runtime storage environment";; esac
labels=$(docker image inspect "$image" --format '{{json .Config.Labels}}')
case "$labels" in *"org.opencontainers.image.version\":\"$BUILD_VERSION"*) ;; *) fail "version label mismatch";; esac
case "$labels" in *"org.opencontainers.image.revision\":\"$GIT_COMMIT"*) ;; *) fail "revision label mismatch";; esac

if [ "$offline" = true ]; then
    for cached in \
        'docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e' \
        'node:20.19.5-bookworm-slim@sha256:9e70124bd00f47dd023e349cd587132ae61892acc0e47ed641416c3e18f401c3' \
        'golang:1.25.12-bookworm@sha256:ea341baa9bd5ba6784f6d7161ace70544349a6242d54d34a0fbfd2c4d51c9d58' \
        "$base" "$curl_image"; do
        docker image inspect "$cached" >/dev/null 2>&1 || fail "offline cached image is missing"
    done
else
    docker pull "$base" >/dev/null
    docker pull "$curl_image" >/dev/null
fi
base_layers=$(docker image inspect "$base" --format '{{join .RootFS.Layers " "}}')
image_layers=$(docker image inspect "$image" --format '{{join .RootFS.Layers " "}}')
case "$image_layers" in "$base_layers "*) ;; *) fail "final image rootfs does not retain exact pinned-base layer prefix";; esac
image_tail=${image_layers#"$base_layers "}
[ "$(printf '%s\n' "$image_tail" | awk '{print NF}')" -eq 1 ] || fail "final image must add exactly one layer"

cid=$(docker create "$image")
docker cp "$cid:/kubikles-accelerator" "$tmp/kubikles-accelerator"
docker rm "$cid" >/dev/null
cid=
docker image save "$image" -o "$tmp/image.tar"
last_layer=$(tar -xOf "$tmp/image.tar" manifest.json | grep -o '"blobs/sha256/[0-9a-f]*"' | tail -n 1 | tr -d '"')
[ -n "$last_layer" ] || fail "image archive has no final layer"
tar -xOf "$tmp/image.tar" "$last_layer" | tar -tf - | LC_ALL=C sort >"$tmp/added.files"
added=$(tr '\n' ' ' <"$tmp/added.files")
test "$added" = 'kubikles-accelerator ' || fail "final image adds files other than /kubikles-accelerator: $added"
for forbidden in bin/sh bin/bash usr/bin/apt usr/bin/dpkg usr/bin/kubectl usr/bin/helm usr/bin/docker root .kube home data; do
    if grep -Eq "(^|/)$forbidden(/|$)" "$tmp/added.files"; then fail "final image adds forbidden path $forbidden"; fi
done
go run ./scripts/cmd/inspect-accelerator-binary "$tmp/kubikles-accelerator" "$(go env GOARCH)" "$BUILD_VERSION" "$GIT_COMMIT" "$GIT_DIRTY"

if [ "$offline" = false ]; then
repro_nonce=$(basename "$tmp" | tr -cd 'a-zA-Z0-9')
first_image="${image}-repro-${repro_nonce}-one"
second_image="${image}-repro-${repro_nonce}-two"
build_product() {
	repro=$1
	archive=$2
	product_version=$3
	product_commit=$4
	SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH" docker buildx build $build_cache_flags $offline_build_flags --output type=docker,rewrite-timestamp=true -f "$clean_context/Dockerfile.accelerator" -t "$repro" --build-arg BUILD_VERSION="$product_version" --build-arg GIT_COMMIT="$product_commit" --build-arg GIT_DIRTY="$GIT_DIRTY" --build-arg SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH" "$clean_context" >/dev/null
	if [ -n "$archive" ]; then
		docker image save "$repro" -o "$archive"
	fi
}
build_product "$first_image" "$tmp/first.oci.tar" "$BUILD_VERSION" "$GIT_COMMIT"
build_product "$second_image" "$tmp/second.oci.tar" "$BUILD_VERSION" "$GIT_COMMIT"
first_repro_cid=$(docker create "$first_image")
docker cp "$first_repro_cid:/kubikles-accelerator" "$tmp/$(basename "$first_image")"
docker rm "$first_repro_cid" >/dev/null
first_repro_cid=
second_repro_cid=$(docker create "$second_image")
docker cp "$second_repro_cid:/kubikles-accelerator" "$tmp/$(basename "$second_image")"
docker rm "$second_repro_cid" >/dev/null
second_repro_cid=
product_hash=$(sha256sum "$tmp/kubikles-accelerator" | awk '{print $1}')
first_hash=$(sha256sum "$tmp/$(basename "$first_image")" | awk '{print $1}')
second_hash=$(sha256sum "$tmp/$(basename "$second_image")" | awk '{print $1}')
test "$product_hash" = "$first_hash" || fail "product and fresh executable hashes differ"
test "$first_hash" = "$second_hash" || fail "fresh executable hashes differ"
for field in Id RootFS Config; do
    case "$field" in
        Id) template='{{.Id}}' ;;
        RootFS) template='{{json .RootFS}}' ;;
        Config) template='{{json .Config}}' ;;
    esac
    left=$(docker image inspect "$first_image" --format "$template")
    right=$(docker image inspect "$second_image" --format "$template")
    test "$left" = "$right" || fail "fresh images differ in $field"
done
go run ./scripts/cmd/inspect-accelerator-oci "$tmp/first.oci.tar" "$tmp/second.oci.tar"

# T7 mutation evidence uses complete product-image builds, not synthetic Go
# fixtures. Version changes must flow through Browser marker, binary, and OCI
# label identity. Commit-only changes must affect binary/revision identity while
# leaving the Browser output and ordinary compressed frontend bytes unchanged.
version_mutated="${BUILD_VERSION}-completion-mutation"
case "$GIT_COMMIT" in
	0*) commit_mutated="1${GIT_COMMIT#?}" ;;
	*) commit_mutated="0${GIT_COMMIT#?}" ;;
esac
test "$commit_mutated" != "$GIT_COMMIT" || fail "could not derive a controlled commit mutation"
version_image="${image}-repro-${repro_nonce}-version"
commit_image="${image}-repro-${repro_nonce}-commit"
build_product "$version_image" "" "$version_mutated" "$GIT_COMMIT"
build_product "$commit_image" "" "$BUILD_VERSION" "$commit_mutated"

version_cid=$(docker create "$version_image")
docker cp "$version_cid:/kubikles-accelerator" "$tmp/version-mutated-accelerator"
docker rm "$version_cid" >/dev/null
version_cid=
commit_cid=$(docker create "$commit_image")
docker cp "$commit_cid:/kubikles-accelerator" "$tmp/commit-mutated-accelerator"
docker rm "$commit_cid" >/dev/null
commit_cid=
version_hash=$(sha256sum "$tmp/version-mutated-accelerator" | awk '{print $1}')
commit_hash=$(sha256sum "$tmp/commit-mutated-accelerator" | awk '{print $1}')
test "$version_hash" != "$first_hash" || fail "BuildVersion mutation did not change executable bytes"
test "$commit_hash" != "$first_hash" || fail "commit-only mutation did not change executable bytes"
test "$(docker image inspect "$version_image" --format '{{.Id}}')" != "$(docker image inspect "$first_image" --format '{{.Id}}')" || fail "BuildVersion mutation did not change image identity"
test "$(docker image inspect "$commit_image" --format '{{.Id}}')" != "$(docker image inspect "$first_image" --format '{{.Id}}')" || fail "commit-only mutation did not change image identity"
test "$(docker image inspect "$version_image" --format '{{index .Config.Labels "org.opencontainers.image.version"}}')" = "$version_mutated" || fail "BuildVersion mutation label mismatch"
test "$(docker image inspect "$version_image" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}')" = "$GIT_COMMIT" || fail "BuildVersion mutation changed revision label"
test "$(docker image inspect "$commit_image" --format '{{index .Config.Labels "org.opencontainers.image.version"}}')" = "$BUILD_VERSION" || fail "commit-only mutation changed version label"
test "$(docker image inspect "$commit_image" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}')" = "$commit_mutated" || fail "commit-only mutation revision label mismatch"
go run ./scripts/cmd/inspect-accelerator-binary "$tmp/version-mutated-accelerator" "$(go env GOARCH)" "$version_mutated" "$GIT_COMMIT" "$GIT_DIRTY"
go run ./scripts/cmd/inspect-accelerator-binary "$tmp/commit-mutated-accelerator" "$(go env GOARCH)" "$BUILD_VERSION" "$commit_mutated" "$GIT_DIRTY"

build_asset_evidence() {
	asset_dir=$1
	asset_version=$2
	asset_commit=$3
	mkdir "$asset_dir"
	SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH" docker buildx build $offline_build_flags --target accelerator-assets --output "type=local,dest=$asset_dir" -f "$clean_context/Dockerfile.accelerator" --build-arg BUILD_VERSION="$asset_version" --build-arg GIT_COMMIT="$asset_commit" --build-arg GIT_DIRTY="$GIT_DIRTY" --build-arg SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH" "$clean_context" >/dev/null
}
baseline_assets="$tmp/assets-baseline"
version_assets="$tmp/assets-version"
commit_assets="$tmp/assets-commit"
build_asset_evidence "$baseline_assets" "$BUILD_VERSION" "$GIT_COMMIT"
build_asset_evidence "$version_assets" "$version_mutated" "$GIT_COMMIT"
build_asset_evidence "$commit_assets" "$BUILD_VERSION" "$commit_mutated"

ordinary_gzip_manifest() {
	asset_dir=$1
	(
		cd "$asset_dir/frontend/dist"
		find . -type f -name '*.gz' -exec sha256sum {} \; | LC_ALL=C sort
	)
}
ordinary_gzip_manifest "$baseline_assets" >"$tmp/ordinary-baseline.sha256"
ordinary_gzip_manifest "$version_assets" >"$tmp/ordinary-version.sha256"
ordinary_gzip_manifest "$commit_assets" >"$tmp/ordinary-commit.sha256"
test -s "$tmp/ordinary-baseline.sha256" || fail "ordinary frontend produced no gzip hash evidence"
cmp "$tmp/ordinary-baseline.sha256" "$tmp/ordinary-version.sha256" >/dev/null || fail "ordinary gzip hashes changed with BuildVersion"
cmp "$tmp/ordinary-baseline.sha256" "$tmp/ordinary-commit.sha256" >/dev/null || fail "ordinary gzip hashes changed with commit-only mutation"

baseline_browser="$baseline_assets/frontend/dist/accelerator-browser"
version_browser="$version_assets/frontend/dist/accelerator-browser"
commit_browser="$commit_assets/frontend/dist/accelerator-browser"
for relative in assets/browser.js assets/browser.css; do
	baseline_browser_hash=$(sha256sum "$baseline_browser/$relative" | awk '{print $1}')
	test "$baseline_browser_hash" = "$(sha256sum "$version_browser/$relative" | awk '{print $1}')" || fail "$relative changed with BuildVersion"
	test "$baseline_browser_hash" = "$(sha256sum "$commit_browser/$relative" | awk '{print $1}')" || fail "$relative changed with commit-only mutation"
done
baseline_marker_hash=$(sha256sum "$baseline_browser/.kubikles-browser-v1.json" | awk '{print $1}')
version_marker_hash=$(sha256sum "$version_browser/.kubikles-browser-v1.json" | awk '{print $1}')
commit_marker_hash=$(sha256sum "$commit_browser/.kubikles-browser-v1.json" | awk '{print $1}')
test "$baseline_marker_hash" != "$version_marker_hash" || fail "Browser marker did not change with BuildVersion"
test "$baseline_marker_hash" = "$commit_marker_hash" || fail "Browser marker changed with commit-only mutation"
grep -F "\"buildVersion\":\"$BUILD_VERSION\"" "$baseline_browser/.kubikles-browser-v1.json" >/dev/null || fail "baseline Browser marker version mismatch"
grep -F "\"buildVersion\":\"$version_mutated\"" "$version_browser/.kubikles-browser-v1.json" >/dev/null || fail "mutated Browser marker version mismatch"
else
    [ -n "${ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT:-}" ] || fail "offline artifact root is missing"
    go run ./internal/acceleratoracceptance/cmd/accelerator-e2e-artifact "$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT" || fail "offline artifact evidence is invalid"
fi

mkdir -p "$tmp/serviceaccount"
chmod 0755 "$tmp" "$tmp/serviceaccount"
printf '%s' "$raw_token" >"$tmp/serviceaccount/token"
printf '%s\n' default >"$tmp/serviceaccount/namespace"
openssl req -x509 -newkey rsa:2048 -keyout "$tmp/key.pem" -out "$tmp/serviceaccount/ca.crt" -days 1 -nodes -subj /CN=kubikles-test >/dev/null 2>&1 || fail "openssl could not create test CA"
chmod 0444 "$tmp/serviceaccount/token" "$tmp/serviceaccount/namespace" "$tmp/serviceaccount/ca.crt"
serviceaccount_bind="$tmp/serviceaccount"
mounted_token=$(docker run --rm --read-only --user 65532:65532 --entrypoint /bin/cat -v "$serviceaccount_bind:/var/run/secrets/kubernetes.io/serviceaccount:ro" "$curl_image" /var/run/secrets/kubernetes.io/serviceaccount/token 2>/dev/null) || mounted_token=
if [ "$mounted_token" != "$raw_token" ]; then
    container_id=$(cat /etc/hostname 2>/dev/null || true)
    merged_dir=$(docker inspect "$container_id" --format '{{.GraphDriver.Data.MergedDir}}' 2>/dev/null || true)
    if [ -n "$merged_dir" ] && [ "${tmp#/}" != "$tmp" ]; then
        serviceaccount_bind="$merged_dir$tmp/serviceaccount"
        mounted_token=$(docker run --rm --read-only --user 65532:65532 --entrypoint /bin/cat -v "$serviceaccount_bind:/var/run/secrets/kubernetes.io/serviceaccount:ro" "$curl_image" /var/run/secrets/kubernetes.io/serviceaccount/token 2>/dev/null) || mounted_token=
    fi
fi
test "$mounted_token" = "$raw_token" || fail "fake ServiceAccount bind token mismatch"
control_cid=$(docker run -d --read-only --user 65532:65532 -v "$serviceaccount_bind:/var/run/secrets/kubernetes.io/serviceaccount:ro" "$image" --fixture-control)
test "$(docker wait "$control_cid")" -eq 1 || fail "filesystem control container did not reject arguments"
baseline_diff=$(docker diff "$control_cid" | LC_ALL=C sort)
docker rm "$control_cid" >/dev/null
control_cid=
cid=$(docker run -d --read-only --user 65532:65532 -v "$serviceaccount_bind:/var/run/secrets/kubernetes.io/serviceaccount:ro" -e KUBERNETES_SERVICE_HOST=127.0.0.1 -e KUBERNETES_SERVICE_PORT=1 -e KUBIKLES_ACCELERATOR_CREATOR_VERIFIER="$creator_verifier" "$image")
for attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    if docker run --rm --network "container:$cid" "$curl_image" -fsS --connect-timeout 1 --max-time 3 http://127.0.0.1:8080/livez >"$tmp/livez" 2>/dev/null; then break; fi
    sleep 1
done
test "$(cat "$tmp/livez")" = live || fail "loopback /livez did not become ready"
for path in / /index.html /missing-static; do
	status=$(docker run --rm --network "container:$cid" "$curl_image" -sS -o /dev/null -w '%{http_code}' --connect-timeout 1 --max-time 3 "http://127.0.0.1:8080$path") || fail "request to $path failed"
	test "$status" = 404 || fail "Accelerator static route $path = $status, want 404"
done
docker run --rm --network "container:$cid" "$curl_image" -fsS --connect-timeout 1 --max-time 3 -H "Authorization: Bearer $raw_token" http://127.0.0.1:8080/api/accelerator-info >"$tmp/info"
grep -F '"runtime":"accelerator"' "$tmp/info" >/dev/null || fail "authenticated info runtime mismatch"
grep -F "\"buildVersion\":\"$BUILD_VERSION\"" "$tmp/info" >/dev/null || fail "authenticated info version mismatch"
grep -F "\"commit\":\"$GIT_COMMIT\"" "$tmp/info" >/dev/null || fail "authenticated info commit mismatch"
grep -F "\"dirty\":$GIT_DIRTY" "$tmp/info" >/dev/null || fail "authenticated info dirty mismatch"
test "$(docker inspect "$cid" --format '{{.HostConfig.ReadonlyRootfs}}')" = true || fail "container is not read-only"
test "$(docker inspect "$cid" --format '{{len .Mounts}}')" -eq 1 || fail "container has an unexpected mount"
test "$(docker inspect "$cid" --format '{{range .Mounts}}{{if eq .Destination "/var/run/secrets/kubernetes.io/serviceaccount"}}{{.RW}}{{end}}{{end}}')" = false || fail "fake ServiceAccount bind is not read-only"
docker top "$cid" -eo pid,user,group | awk 'NR > 1 { if ($2 != "65532" || $3 != "65532") exit 1; seen=1 } END { exit !seen }' || fail "runtime process is not UID/GID 65532"
docker stop -t 10 "$cid" >/dev/null || fail "SIGTERM shutdown did not complete"
test "$(docker inspect "$cid" --format '{{.State.ExitCode}}')" = 0 || fail "SIGTERM exit code is not zero"
test "$(docker inspect "$cid" --format '{{.State.OOMKilled}}')" = false || fail "runtime was OOM-killed"
runtime_diff=$(docker diff "$cid" | LC_ALL=C sort)
test "$runtime_diff" = "$baseline_diff" || fail "process wrote to its container filesystem"
if ! logs=$(docker logs "$cid" 2>&1); then
	fail "could not retrieve runtime logs"
fi
case "$logs" in *"$raw_token"*|*"$creator_verifier"*) fail "runtime logs leaked a test secret";; esac
docker rm "$cid" >/dev/null
cid=
echo "accelerator image validation passed: $image"
