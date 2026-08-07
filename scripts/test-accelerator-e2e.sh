#!/usr/bin/env bash
set -euo pipefail
umask 077
export LC_ALL=C

fail() { echo "accelerator-e2e: $1" >&2; exit 1; }
milliseconds() { date +%s%3N; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root/scripts/lib/accelerator-e2e-kind.sh"
total_started="$(milliseconds)"

[ -z "${KUBIKLES_ACCELERATOR_E2E_ACTIVE-}" ] || fail recursive-invocation
accelerator_e2e_preflight "$root" || fail preflight

fixture_root=""
fixture_state=""
cleanup_failed=false
tripwire_pid=""
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  set +e
  if [ -n "$tripwire_pid" ]; then
    kill -TERM "$tripwire_pid" >/dev/null 2>&1 || cleanup_failed=true
    wait "$tripwire_pid" >/dev/null 2>&1 || cleanup_failed=true
    tripwire_pid=""
  fi
  if [ -n "$fixture_state" ]; then
    accelerator_e2e_cleanup_owned_fixture "$fixture_state" || cleanup_failed=true
  fi
  if "$cleanup_failed"; then
    echo 'accelerator-e2e: cleanup-failed' >&2
    status=1
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/kubikles-accelerator-e2e.XXXXXX")"
chmod 700 "$fixture_root"
fixture_state="$(mktemp -d "${TMPDIR:-/tmp}/kubikles-accelerator-e2e-state.XXXXXX")"
accelerator_e2e_create_owned_fixture "$fixture_state" "$fixture_root" || fail fixture-create

export BUILD_VERSION=v0.0.0
export ACCELERATOR_E2E_OFFLINE=1
export KUBIKLES_ACCELERATOR_E2E_NAMESPACE="a60a-${RANDOM}-${RANDOM}"
export ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT="$fixture_root/artifacts"
mkdir -m 700 "$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT"
go build -o "$fixture_root/accelerator-e2e-tripwire" ./internal/acceleratoracceptance/cmd/accelerator-e2e-tripwire || fail tripwire-build
"$fixture_root/accelerator-e2e-tripwire" "$fixture_root/tripwire-address.json" "$fixture_root/tripwire-count.json" &
tripwire_pid=$!
for _ in $(seq 1 100); do test -s "$fixture_root/tripwire-address.json" && break; sleep 0.05; done
test -s "$fixture_root/tripwire-address.json" || fail tripwire-start
tripwire_address="$(jq -r . "$fixture_root/tripwire-address.json")"
[[ "$tripwire_address" =~ ^127\.0\.0\.1:[1-9][0-9]{0,4}$ ]] || fail tripwire-address
export HTTP_PROXY="http://$tripwire_address" HTTPS_PROXY="http://$tripwire_address" ALL_PROXY="http://$tripwire_address"
export http_proxy="$HTTP_PROXY" https_proxy="$HTTPS_PROXY" all_proxy="$ALL_PROXY"
export NO_PROXY="127.0.0.1,localhost,host.docker.internal"
export no_proxy="$NO_PROXY"
fixture_finished="$(milliseconds)"
"$root/scripts/build-accelerator-e2e-artifacts.sh" || fail artifact-fixture
artifact_finished="$(milliseconds)"
test "$(jq -r . "$fixture_root/tripwire-count.json")" = 0 || fail external-network
artifact_metadata="$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT/acceptance-artifact-metadata.json"
ACCELERATOR_IMAGE_DIGEST="$(jq -er '.registryImageDigest | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))' "$artifact_metadata")" || fail artifact-metadata
ACCELERATOR_IMAGE_VERSION="$(jq -er '.runtimeBuildVersion | select(. == "v0.0.0")' "$artifact_metadata")" || fail artifact-metadata
export ACCELERATOR_IMAGE_REPOSITORY="$KUBIKLES_ACCELERATOR_E2E_REGISTRY/skrobylabs/kubikles-accelerator"
export ACCELERATOR_IMAGE_DIGEST ACCELERATOR_IMAGE_VERSION
export ACCELERATOR_ACCEPTANCE_ROOT="$root"
export ACCELERATOR_ACCEPTANCE_CONTRACT="$root/test/accelerator/acceptance-v1.json"
export ACCELERATOR_ACCEPTANCE_PROGRESS_REPORT="$fixture_root/focused-report.json"
export ACCELERATOR_ACCEPTANCE_FINAL_REPORT="$fixture_root/final-report.json"
export ACCELERATOR_ACCEPTANCE_TARGET_TIMEOUT_SECONDS=3600

go build -o "$fixture_root/accelerator-e2e-runner" ./internal/acceleratoracceptance/cmd/accelerator-e2e-runner || fail runner-build
"$fixture_root/accelerator-e2e-runner" || fail focused-cases
focused_finished="$(milliseconds)"
test "$(jq -r . "$fixture_root/tripwire-count.json")" = 0 || fail external-network

export KUBIKLES_ACCELERATOR_E2E_NAMESPACE="kubikles-a60a-composed-${RANDOM}"
ACCELERATOR_ACCEPTANCE_COMPOSED_KIND=1 "$root/scripts/test-accelerator-desktop-provision-kind.sh" || fail composed-cases
composed_finished="$(milliseconds)"
accelerator_e2e_audit_case_cleanup kubikles-a60a-sentinel || fail composed-cleanup
test -s "$ACCELERATOR_ACCEPTANCE_FINAL_REPORT" || fail composed-report-missing
test "$(stat -c '%a' "$ACCELERATOR_ACCEPTANCE_FINAL_REPORT" 2>/dev/null || stat -f '%Lp' "$ACCELERATOR_ACCEPTANCE_FINAL_REPORT" 2>/dev/null)" = 600 || fail composed-report-mode
validation_started="$(milliseconds)"
export ACCELERATOR_ACCEPTANCE_FIXTURE_MS="$((fixture_finished - total_started))"
export ACCELERATOR_ACCEPTANCE_ARTIFACT_MS="$((artifact_finished - fixture_finished))"
export ACCELERATOR_ACCEPTANCE_FOCUSED_MS="$((focused_finished - artifact_finished))"
export ACCELERATOR_ACCEPTANCE_COMPOSED_MS="$((composed_finished - focused_finished))"
export ACCELERATOR_ACCEPTANCE_VALIDATION_MS="$((milliseconds - validation_started))"
export ACCELERATOR_ACCEPTANCE_TOTAL_MS="$((milliseconds - total_started))"
"$fixture_root/accelerator-e2e-runner" validate-report || fail composed-report-invalid
test "$(jq -r . "$fixture_root/tripwire-count.json")" = 0 || fail external-network
echo 'accelerator-e2e: passed' >&2
