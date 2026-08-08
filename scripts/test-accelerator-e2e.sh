#!/usr/bin/env bash
set -euo pipefail
umask 077
export LC_ALL=C

milliseconds() { date +%s%3N; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root/scripts/lib/accelerator-e2e-kind.sh"
total_started="$(milliseconds)"
timing_phase=fixture
timing_failure_emitted=false
emit_timing_failure() {
  [ "$timing_failure_emitted" = false ] || return 0
  timing_failure_emitted=true
  local now total fixture=0 artifact=0 focused=0 composed=0 validation=0 cleanup=0
  now="$(milliseconds)"; total="$((now - total_started))"
  case "$timing_phase" in
    fixture) fixture="$total" ;; artifact) artifact="$total" ;; focused) focused="$total" ;;
    composed) composed="$total" ;; validation) validation="$total" ;; cleanup) cleanup="$total" ;;
  esac
  # This is intentionally a fixed JSON schema; it never copies the failure
  # detail, shell command, environment, path, or raw child output.
  printf '{"schemaVersion":1,"fixtureMs":%s,"artifactMs":%s,"focusedMs":%s,"composedMs":%s,"validationMs":%s,"cleanupMs":%s,"completedTotalMs":%s,"lastActivePhase":"%s","focusedCase":"","focusedSubphase":"none","composedCase":"","composedStage":"none","failureCode":"command"}\n' "$fixture" "$artifact" "$focused" "$composed" "$validation" "$cleanup" "$total" "$timing_phase" >&2
}
fail() { emit_timing_failure; echo "accelerator-e2e: $1" >&2; exit 1; }

[ -z "${KUBIKLES_ACCELERATOR_E2E_ACTIVE-}" ] || fail recursive-invocation
execution_architecture="$(accelerator_e2e_execution_architecture)" || fail execution-architecture
export KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE="$execution_architecture"
export KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS="${KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS:-5}"
[[ "$KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS" =~ ^([1-9]|[12][0-9]|30)$ ]] || fail reconnect-grace
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
timing_phase=artifact
"$root/scripts/build-accelerator-e2e-artifacts.sh" || fail artifact-fixture
# Focused desktop owners receive an immutable manifest and one compiled test
# binary.  They still allocate their own namespace, HOME, kubeconfig copy and
# cleanup state; none of those mutable values are shared here.
go build -o "$fixture_root/accelerator-e2e-shared-fixture" ./internal/acceleratoracceptance/cmd/accelerator-e2e-shared-fixture || fail shared-fixture-helper-build
go test -c -tags=helm,accelerator_provision_kind,accelerator_e2e -o "$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT/desktop-kind.test" ./pkg/acceleratorprovision || fail desktop-test-build
chmod 700 "$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT/desktop-kind.test"
chart_archive="$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT/kubikles-accelerator-0.0.0.tgz"
test -s "$chart_archive" || fail shared-chart
chart_hash="$(sha256sum "$chart_archive" | cut -d ' ' -f1)"
desktop_hash="$(sha256sum "$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT/desktop-kind.test" | cut -d ' ' -f1)"
artifact_metadata="$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT/acceptance-artifact-metadata.json"
jq -n --arg root "$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT" --arg execution "$execution_architecture" --arg chart "$chart_archive" --arg chart_hash "$chart_hash" --arg test_binary "$ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT/desktop-kind.test" --arg test_hash "$desktop_hash" --arg production "$(jq -er .registryImageDigest "$artifact_metadata")" --arg acceptance "$(jq -er .acceptanceImageDigest "$artifact_metadata")" --arg chart_digest "$(jq -er .registryChartDigest "$artifact_metadata")" --argjson grace "$KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS" \
  '{buildVersion:"v0.0.0",executionArchitecture:$execution,reconnectGraceSeconds:$grace,artifactRoot:$root,productionImageDigest:$production,acceptanceImageDigest:$acceptance,chartDigest:$chart_digest,chartArchive:$chart,chartArchiveSha256:$chart_hash,desktopTestBinary:$test_binary,desktopTestBinarySha256:$test_hash}' >"$fixture_root/shared-fixture.json"
chmod 600 "$fixture_root/shared-fixture.json"
"$fixture_root/accelerator-e2e-shared-fixture" "$fixture_root/shared-fixture.json" || fail shared-fixture
export ACCELERATOR_ACCEPTANCE_SHARED_FIXTURE="$fixture_root/shared-fixture.json"
export ACCELERATOR_ACCEPTANCE_SHARED_FIXTURE_VALIDATOR="$fixture_root/accelerator-e2e-shared-fixture"
test "$(jq -r . "$fixture_root/tripwire-count.json")" = 0 || fail external-network
ACCELERATOR_IMAGE_DIGEST="$(jq -er '.acceptanceImageDigest | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))' "$artifact_metadata")" || fail artifact-metadata
test "$(jq -er '.registryImageDigest | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))' "$artifact_metadata")" != "$ACCELERATOR_IMAGE_DIGEST" || fail artifact-metadata
test "$(jq -er --argjson grace "$KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS" '.reconnectGraceSeconds | select(type == "number" and . == $grace)' "$artifact_metadata")" = "$KUBIKLES_ACCELERATOR_E2E_RECONNECT_GRACE_SECONDS" || fail artifact-metadata
test "$(jq -er --arg execution "$execution_architecture" '.executionArchitecture | select(. == $execution)' "$artifact_metadata")" = "$execution_architecture" || fail artifact-metadata
ACCELERATOR_IMAGE_VERSION="$(jq -er '.runtimeBuildVersion | select(. == "v0.0.0")' "$artifact_metadata")" || fail artifact-metadata
export ACCELERATOR_IMAGE_REPOSITORY="$KUBIKLES_ACCELERATOR_E2E_REGISTRY/skrobylabs/kubikles-accelerator"
export ACCELERATOR_IMAGE_DIGEST ACCELERATOR_IMAGE_VERSION
accelerator_e2e_preload_acceptance_image "$ACCELERATOR_IMAGE_DIGEST" || fail acceptance-image-preload
artifact_finished="$(milliseconds)"
export ACCELERATOR_ACCEPTANCE_ROOT="$root"
export ACCELERATOR_ACCEPTANCE_CONTRACT="$root/test/accelerator/acceptance-v1.json"
export ACCELERATOR_ACCEPTANCE_PROGRESS_REPORT="$fixture_root/focused-report.json"
export ACCELERATOR_ACCEPTANCE_FINAL_REPORT="$fixture_root/final-report.json"
export ACCELERATOR_ACCEPTANCE_TARGET_TIMEOUT_SECONDS=3600

go build -o "$fixture_root/accelerator-e2e-runner" ./internal/acceleratoracceptance/cmd/accelerator-e2e-runner || fail runner-build
"$fixture_root/accelerator-e2e-runner" || fail focused-cases
focused_finished="$(milliseconds)"
timing_phase=composed
test "$(jq -r . "$fixture_root/tripwire-count.json")" = 0 || fail external-network

export KUBIKLES_ACCELERATOR_E2E_NAMESPACE="kubikles-a60a-composed-${RANDOM}"
ACCELERATOR_ACCEPTANCE_COMPOSED_KIND=1 "$root/scripts/test-accelerator-desktop-provision-kind.sh" || fail composed-cases
composed_finished="$(milliseconds)"
timing_phase=validation
accelerator_e2e_audit_case_cleanup kubikles-a60a-sentinel || fail composed-cleanup
test -s "$ACCELERATOR_ACCEPTANCE_FINAL_REPORT" || fail composed-report-missing
test "$(stat -c '%a' "$ACCELERATOR_ACCEPTANCE_FINAL_REPORT" 2>/dev/null || stat -f '%Lp' "$ACCELERATOR_ACCEPTANCE_FINAL_REPORT" 2>/dev/null)" = 600 || fail composed-report-mode
validation_started="$(milliseconds)"
export ACCELERATOR_ACCEPTANCE_FIXTURE_MS="$((fixture_finished - total_started))"
export ACCELERATOR_ACCEPTANCE_ARTIFACT_MS="$((artifact_finished - fixture_finished))"
export ACCELERATOR_ACCEPTANCE_FOCUSED_MS="$((focused_finished - artifact_finished))"
export ACCELERATOR_ACCEPTANCE_COMPOSED_MS="$((composed_finished - focused_finished))"
export ACCELERATOR_ACCEPTANCE_VALIDATION_MS=0
export ACCELERATOR_ACCEPTANCE_TOTAL_MS="$((validation_started - total_started))"
"$fixture_root/accelerator-e2e-runner" validate-report || fail composed-report-invalid
test "$(jq -r . "$fixture_root/tripwire-count.json")" = 0 || fail external-network
echo 'accelerator-e2e: passed' >&2
