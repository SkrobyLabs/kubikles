#!/usr/bin/env bash
set -euo pipefail
umask 077
export LC_ALL=C

fail() { printf 'accelerator-supply-chain: %s\n' "$1" >&2; exit 1; }
require() { command -v "$1" >/dev/null 2>&1 || fail "$1-required"; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
for tool in go docker jq syft trivy helm oras; do require "$tool"; done
[ "$(syft version -o json | jq -r .version)" = 1.44.0 ] || fail syft-version
[ "$(trivy --version | sed -n 's/^Version: //p')" = 0.70.0 ] || fail trivy-version
docker info >/dev/null 2>&1 || fail docker-daemon
[[ "$(docker buildx version)" =~ ^github\.com/docker/buildx[[:space:]]v0\.36\.0[[:space:]]df28b0a0b6a44453a87bd53c438432f4120962c9$ ]] || fail buildx-version
buildkit_image='moby/buildkit@sha256:2f5adac4ecd194d9f8c10b7b5d7bceb5186853db1b26e5abd3a657af0b7e26ec'
docker image inspect "$buildkit_image" >/dev/null 2>&1 || fail buildkit-image-missing

version="${BUILD_VERSION:-v0.0.0}"
[[ "$version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || fail build-version
commit="$(git -C "$root" rev-parse HEAD)"
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || fail source-commit
git -C "$root" diff --quiet && git -C "$root" diff --cached --quiet || fail dirty-source
epoch="$(git -C "$root" show -s --format=%ct "$commit")"
[[ "$epoch" =~ ^[1-9][0-9]*$ ]] || fail source-epoch

trivy_cache="${TRIVY_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/trivy}"
metadata="$trivy_cache/db/metadata.json"
[ -f "$metadata" ] || fail trivy-db-missing
db_updated="$(jq -er '.UpdatedAt | select(type=="string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T"))' "$metadata")" || fail trivy-db-metadata
now_epoch="$(date -u +%s)"
db_epoch="$(date -u -d "$db_updated" +%s 2>/dev/null || date -u -j -f '%Y-%m-%dT%H:%M:%SZ' "$db_updated" +%s 2>/dev/null)" || fail trivy-db-metadata
[ "$db_epoch" -le "$now_epoch" ] && [ $((now_epoch-db_epoch)) -le 86400 ] || fail trivy-db-stale

work="$(mktemp -d "${TMPDIR:-/tmp}/kubikles-accelerator-supply-chain.XXXXXX")"
chmod 700 "$work"
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  find "$work" -type f -delete 2>/dev/null || status=1
  find "$work" -depth -type d -empty -delete 2>/dev/null || status=1
  [ ! -e "$work" ] || status=1
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

prepared="${ACCELERATOR_PREPARED_ROOT:-$work/prepared}"
if [ -n "${ACCELERATOR_PREPARED_ROOT:-}" ]; then
  [[ "$prepared" = /* ]] && [ -d "$prepared" ] || fail prepared-root
  [ "$(jq -er .buildVersion "$prepared/source.json")" = "$version" ] || fail prepared-version
  [ "$(jq -er .sourceCommit "$prepared/source.json")" = "$commit" ] || fail prepared-source
  [ "$(jq -er .sourceEpoch "$prepared/source.json")" = "$epoch" ] || fail prepared-epoch
else
  BUILD_VERSION="$version" SOURCE_COMMIT="$commit" SOURCE_DATE_EPOCH="$epoch" ACCELERATOR_PREPARED_OUTPUT="$prepared" \
    "$root/scripts/publish-accelerator-release.sh" prepare >/dev/null || fail prepare
fi
image_digest="$(jq -er '.imageDigest | select(test("^sha256:[0-9a-f]{64}$"))' "$prepared/image-evidence.json")" || fail image-evidence
amd64_digest="$(jq -er '.platforms[] | select(.os=="linux" and .architecture=="amd64") | .manifestDigest' "$prepared/image-evidence.json")" || fail image-evidence
arm64_digest="$(jq -er '.platforms[] | select(.os=="linux" and .architecture=="arm64") | .manifestDigest' "$prepared/image-evidence.json")" || fail image-evidence
chart_digest="$(jq -er '.chartDigest | select(test("^sha256:[0-9a-f]{64}$"))' "$prepared/source.json")" || fail chart-evidence
[ "$image_digest" != "$amd64_digest" ] && [ "$image_digest" != "$arm64_digest" ] && [ "$amd64_digest" != "$arm64_digest" ] || fail platform-evidence

chart_version="${version#v}"
chart="$prepared/kubikles-accelerator-$chart_version.tgz"
platform_source() {
  local architecture="$1" expected_digest="$2"
  local output="$work/oci-linux-$architecture"
  mkdir -m 0700 "$output"
  oras cp --from-oci-layout "$prepared/image-layout:$version" --to-oci-layout "$output:$architecture" --platform "linux/$architecture" >/dev/null 2>&1 || fail "platform-copy-$architecture"
  [ "$(jq -er '.manifests | select(length==1) | .[0].digest' "$output/index.json")" = "$expected_digest" ] || fail "platform-digest-$architecture"
  find "$output" -type d -exec chmod 0700 {} +
  find "$output" -type f -exec chmod 0600 {} +
  printf 'oci-dir:%s\n' "$output"
}
amd64_source="$(platform_source amd64 "$amd64_digest")"
arm64_source="$(platform_source arm64 "$arm64_digest")"
generate_sbom() {
  local artifact="$1" source="$2" platform="$3" digest="$4" name="$5" output="$6"
  local raw="$work/raw-$artifact.json"
  local -a platform_args=()
  [ -z "$platform" ] || platform_args=(--platform "$platform")
  syft scan "$source" "${platform_args[@]}" --source-name "$name" --source-version "$version" --quiet --output "spdx-json=$raw" || fail "syft-$artifact"
  go run "$root/scripts/accelerator-supply-chain/cmd/accelerator-supply-chain" normalize-spdx "$raw" "$output" "$name" "$digest" "$commit" "$epoch" || fail "normalize-$artifact"
  chmod 600 "$output"
}
amd64_sbom="$prepared/kubikles-accelerator-image-linux-amd64-$version.spdx.json"
arm64_sbom="$prepared/kubikles-accelerator-image-linux-arm64-$version.spdx.json"
chart_sbom="$prepared/kubikles-accelerator-chart-$version.spdx.json"
generate_sbom image-linux-amd64 "$amd64_source" linux/amd64 "$amd64_digest" kubikles-accelerator-image-linux-amd64 "$amd64_sbom"
generate_sbom image-linux-arm64 "$arm64_source" linux/arm64 "$arm64_digest" kubikles-accelerator-image-linux-arm64 "$arm64_sbom"
generate_sbom chart "file:$chart" '' "$chart_digest" kubikles-accelerator-chart "$chart_sbom"

for artifact in image-linux-amd64 image-linux-arm64 chart; do
  first="$prepared/kubikles-accelerator-$artifact-$version.spdx.json"
  [ "$artifact" != chart ] || first="$chart_sbom"
  second="$work/repeat-$artifact.spdx.json"
  case "$artifact" in
    image-linux-amd64) generate_sbom "$artifact-repeat" "$amd64_source" linux/amd64 "$amd64_digest" kubikles-accelerator-image-linux-amd64 "$second";;
    image-linux-arm64) generate_sbom "$artifact-repeat" "$arm64_source" linux/arm64 "$arm64_digest" kubikles-accelerator-image-linux-arm64 "$second";;
    chart) generate_sbom "$artifact-repeat" "file:$chart" '' "$chart_digest" kubikles-accelerator-chart "$second";;
  esac
  cmp "$first" "$second" >/dev/null || fail "sbom-nondeterministic-$artifact"
done

scan_sbom() {
  local artifact="$1" sbom="$2"
  local output="$work/trivy-$artifact.json"
  trivy sbom --cache-dir "$trivy_cache" --skip-db-update --offline-scan --scanners vuln --severity CRITICAL,HIGH,MEDIUM,LOW --format json --output "$output" "$sbom" >/dev/null 2>&1 || fail "trivy-$artifact"
}
scan_sbom image-linux-amd64 "$amd64_sbom"
scan_sbom image-linux-arm64 "$arm64_sbom"
scan_sbom chart "$chart_sbom"
trivy fs --cache-dir "$trivy_cache" --skip-db-update --offline-scan --scanners vuln --severity CRITICAL,HIGH,MEDIUM,LOW --format json --output "$work/trivy-source.json" "$root" >/dev/null 2>&1 || fail trivy-source

findings="$work/findings.json"
jq -n --arg updated "$db_updated" \
  --slurpfile amd "$work/trivy-image-linux-amd64.json" --slurpfile arm "$work/trivy-image-linux-arm64.json" \
  --slurpfile chart "$work/trivy-chart.json" --slurpfile source "$work/trivy-source.json" '
  def rows($doc;$artifact;$type): [$doc[0].Results[]? | select($type=="" or (.Type|ascii_downcase|contains($type))) | .Vulnerabilities[]? |
    (.PkgIdentifier.PURL // "") as $purl |
    if ($purl|startswith("pkg:")) then
      {id:.VulnerabilityID,purl:$purl,artifact:$artifact,severity:(.Severity|ascii_upcase)}
    else error("vulnerability purl invalid") end];
  {schemaVersion:1,dbUpdatedAt:$updated,
   artifacts:["image-linux-amd64","image-linux-arm64","chart","source-go","source-npm"],
   findings:(rows($amd;"image-linux-amd64";"")+rows($arm;"image-linux-arm64";"")+rows($chart;"chart";"")+rows($source;"source-go";"gomod")+rows($source;"source-npm";"npm") | unique_by([.id,.purl,.artifact]) | sort_by([.artifact,.id,.purl]))}' > "$findings" || fail trivy-normalize
policy_result="$work/policy-result.json"
go run "$root/scripts/accelerator-supply-chain/cmd/accelerator-supply-chain" policy "$findings" "$root/security/accelerator-vulnerability-exceptions.json" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$policy_result" || fail vulnerability-policy

bundle="$work/kubikles-accelerator-prepared-$version.tar"
checksum="$bundle.sha256"
go run "$root/scripts/accelerator-supply-chain/cmd/accelerator-supply-chain" bundle-create "$prepared" "$bundle" "$checksum" "$version" "$commit" "$epoch" || fail bundle-create
verified="$work/verified"
go run "$root/scripts/accelerator-supply-chain/cmd/accelerator-supply-chain" bundle-verify "$bundle" "$checksum" "$verified" "$version" "$commit" || fail bundle-verify
cmp "$prepared/image-evidence.json" "$verified/image-evidence.json" >/dev/null || fail bundle-readback
cp "$checksum" "$work/tampered.sha256"
printf 'x' >> "$work/tampered.sha256"
if go run "$root/scripts/accelerator-supply-chain/cmd/accelerator-supply-chain" bundle-verify "$bundle" "$work/tampered.sha256" "$work/tampered-output" "$version" "$commit" >/dev/null 2>&1; then fail bundle-tamper; fi
if [ -n "${ACCELERATOR_SUPPLY_CHAIN_OUTPUT:-}" ]; then
  [[ "$ACCELERATOR_SUPPLY_CHAIN_OUTPUT" = /* ]] && [ ! -e "$ACCELERATOR_SUPPLY_CHAIN_OUTPUT" ] || fail supply-chain-output
  mkdir -m 0700 "$ACCELERATOR_SUPPLY_CHAIN_OUTPUT"
  install -m 0600 "$bundle" "$ACCELERATOR_SUPPLY_CHAIN_OUTPUT/$(basename "$bundle")"
  install -m 0600 "$checksum" "$ACCELERATOR_SUPPLY_CHAIN_OUTPUT/$(basename "$checksum")"
fi

 (cd "$root" && go test -race ./scripts/accelerator-supply-chain/...) || fail helper-tests
go run "$root/scripts/accelerator-supply-chain/cmd/accelerator-supply-chain" validate-contract "$root" || fail workflow-contract
(cd "$root" && go test ./scripts/accelerator-release) || fail release-contract
jq -e '(.critical + .high)==(.excepted|length)' "$policy_result" >/dev/null || fail vulnerability-result
printf 'accelerator-supply-chain: passed\n'
