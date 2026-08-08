#!/usr/bin/env bash
# Execute every gate stage with private command fakes; no daemon or cluster is used.
set -euo pipefail
umask 077
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"; chmod 700 "$tmp"
bin="$tmp/bin"; mkdir "$bin"; chmod 700 "$bin"
trap 'rm -rf "$tmp"' EXIT
for tool in dirname mktemp chmod rm grep touch mkdir test cat; do ln -s "$(command -v "$tool")" "$bin/$tool"; done

fake() { printf '%s\n' '#!/usr/bin/bash' "$2" >"$bin/$1"; chmod 700 "$bin/$1"; }
run() {
  local scenario="$1"
  shift
  rm -rf "$tmp/state"
  mkdir "$tmp/state"
  /usr/bin/env \
    -u KUBIKLES_ACCELERATOR_E2E_REUSE \
    -u KUBIKLES_ACCELERATOR_E2E_KIND_NAME \
    -u KUBIKLES_ACCELERATOR_E2E_KUBECONFIG \
    -u KUBIKLES_ACCELERATOR_E2E_NAMESPACE \
    -u KUBIKLES_ACCELERATOR_E2E_REGISTRY \
    -u KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE \
    PATH="$bin" HARNESS_STATE="$tmp/state" HARNESS_FAIL="$scenario" /usr/bin/bash "$@"
}
expect() {
  local expected="$1" scenario="$2"
  set +e; output="$(run "$scenario" "$root/scripts/test-accelerator-00-kind.sh" 2>&1)"; rc=$?; set -e
  [ "$rc" -ne 0 ] && [ "$output" = "accelerator-00-kind:$expected" ] || { printf 'harness:%s:%s\n' "$expected" "$output" >&2; exit 1; }
  [ ! -e "$tmp/state/cluster" ] && [ ! -e "$tmp/state/image" ] && [ ! -e "$tmp/state/test-copied" ] && [ ! -e "$tmp/state/config-copied" ] || { printf 'harness:%s:cleanup\n' "$expected" >&2; exit 1; }
}
fake kubectl 'case "${HARNESS_FAIL:-}" in remote-daemon) echo "The connection to the server fixed.invalid was refused - did you specify the right host or port?" >&2; exit 1;; arm64-fallback) echo "dial tcp fixed: i/o timeout" >&2; exit 1;; runner-test) echo "Unable to connect to the server: dial tcp fixed: connect: no route to host" >&2; exit 1;; test-build|internal-kubeconfig|test-copy|config-copy) echo "Unable to connect to the server: dial tcp fixed: connect: connection refused" >&2; exit 1;; tls) echo "x509 certificate failure" >&2; exit 1;; auth) echo "Unauthorized credentials" >&2; exit 1;; http-ready) echo "Error from server (InternalError): readiness backend connection refused" >&2; exit 1;; malformed) echo "error loading config" >&2; exit 1;; deadline) echo "context deadline exceeded" >&2; exit 1;; *) exit 0;; esac'
fake docker 'case "$1:$2" in info:--format) case "${HARNESS_FAIL:-}" in unsupported-architecture) echo riscv64;; arm64-fallback) echo aarch64; echo arm64 > "$HARNESS_STATE/expected-arch";; *) echo amd64; echo amd64 > "$HARNESS_STATE/expected-arch";; esac;; info:*) [ "${HARNESS_FAIL:-}" != daemon ] || exit 1;; build:*) [ "${HARNESS_FAIL:-}" != image-build ] || exit 1; touch "$HARNESS_STATE/image"; echo image-build >> "$HARNESS_STATE/log";; image:inspect) [ -f "$HARNESS_STATE/image" ];; image:rm) rm -f "$HARNESS_STATE/image"; echo image-delete >> "$HARNESS_STATE/log";; cp:*) case "$2" in *accelerator00-kind.test) [ "${HARNESS_FAIL:-}" != test-copy ] || exit 1; touch "$HARNESS_STATE/test-copied";; *internal-kubeconfig) [ "${HARNESS_FAIL:-}" != config-copy ] || exit 1; touch "$HARNESS_STATE/config-copied";; *) exit 2;; esac;; exec:*) [ "${HARNESS_FAIL:-}" != runner-test ] || exit 1; [ -f "$HARNESS_STATE/test-copied" ] && [ -f "$HARNESS_STATE/config-copied" ] && [ "${10}" = -test.count=1 ] && [ "${11}" = -test.timeout=8m ] && [ "${12}" = -test.run ] && [ "${13}" = "^TestAccelerator00ExactPodLoopbackKind$" ] || exit 2; echo selected-runner-test >> "$HARNESS_STATE/log";; esac'
fake go 'case "$1:$2" in build:*) [ "${HARNESS_FAIL:-}" != build ] || exit 1; [ "${GOOS:-}" = linux ] && [ "${CGO_ENABLED:-}" = 0 ] && [ "${GOARCH:-}" = "$(cat "$HARNESS_STATE/expected-arch")" ] || exit 2; touch "$3"; echo fixture-build >> "$HARNESS_STATE/log";; test:-c) [ "${HARNESS_FAIL:-}" != test-build ] || exit 1; [ "$#" -eq 6 ] && [ "$3" = -tags=accelerator_kind ] && [ "$4" = -o ] && [ "$6" = ./pkg/k8s ] && [ "${GOOS:-}" = linux ] && [ "${CGO_ENABLED:-}" = 0 ] && [ "${GOARCH:-}" = "$(cat "$HARNESS_STATE/expected-arch")" ] || exit 2; touch "$5"; echo runner-build >> "$HARNESS_STATE/log";; test:*) expected=(test -tags=accelerator_kind -count=1 -timeout=8m ./pkg/k8s -run "^TestAccelerator00ExactPodLoopbackKind$"); [ "$#" -eq "${#expected[@]}" ] || exit 2; for ((i=0; i<$#; i++)); do position=$((i+1)); actual="${!position}"; [ "$actual" = "${expected[i]}" ] || exit 2; printf "%s\\n" "$actual" >> "$HARNESS_STATE/test-args"; done; [ "${HARNESS_FAIL:-}" != test ] || exit 1; [ -f "$HARNESS_STATE/image" ] && [ -f "$HARNESS_STATE/loaded" ] || exit 2; echo selected-test >> "$HARNESS_STATE/log";; esac'
expect missing-kind ''
fake kind 'case "$1:$2" in get:kubeconfig) [ "${HARNESS_FAIL:-}" != internal-kubeconfig ] || exit 1; echo internal-config; touch "$HARNESS_STATE/internal-config";; get:*) true;; create:*) [ "${HARNESS_FAIL:-}" != cluster-create ] || exit 1; [ "$7" = --wait ] && [ "$8" = 60s ] || exit 2; touch "$6"; touch "$HARNESS_STATE/cluster"; echo cluster-create >> "$HARNESS_STATE/log";; load:*) [ "${HARNESS_FAIL:-}" != image-load ] || exit 1; [ -f "$HARNESS_STATE/image" ] && [ -f "$HARNESS_STATE/cluster" ]; touch "$HARNESS_STATE/loaded"; echo image-load >> "$HARNESS_STATE/log";; delete:*) rm -f "$HARNESS_STATE/cluster" "$HARNESS_STATE/test-copied" "$HARNESS_STATE/config-copied"; echo cluster-delete >> "$HARNESS_STATE/log";; esac'
for tool in kubectl docker go; do
  mv "$bin/$tool" "$bin/$tool.off"; expect "missing-$tool" ''; mv "$bin/$tool.off" "$bin/$tool"
done
expect daemon-unavailable daemon
expect unsupported-architecture unsupported-architecture
expect build build
expect image-build image-build
expect cluster-create cluster-create
expect image-load image-load
expect api-ready tls
expect api-ready auth
expect api-ready http-ready
expect api-ready malformed
expect api-ready deadline
expect test test
expect test runner-test
expect test-build test-build
expect internal-kubeconfig internal-kubeconfig
expect test-copy test-copy
expect config-copy config-copy
expect_go_reject() {
  set +e; PATH="$bin" HARNESS_STATE="$tmp/state" HARNESS_FAIL='' go "$@" >/dev/null 2>&1; rc=$?; set -e
  [ "$rc" -ne 0 ] || { echo 'harness:wrong-go-selection' >&2; exit 1; }
}
expect_go_reject test -tags=wrong -count=1 -timeout=8m ./pkg/k8s -run '^TestAccelerator00ExactPodLoopbackKind$'
expect_go_reject test -tags=accelerator_kind -count=1 -timeout=8m ./pkg/k8s -run '^TestWrong$'
# The successful fake run reaches cleanup; its in-gate verification confirms
# the owned cluster/image are absent without exposing command output.
set +e; output="$(run '' "$root/scripts/test-accelerator-00-kind.sh" 2>&1)"; rc=$?; set -e
[ "$rc" -eq 0 ] && [ -z "$output" ] || { echo 'harness:success-cleanup' >&2; exit 1; }
[ ! -e "$tmp/state/cluster" ] && [ ! -e "$tmp/state/image" ] && grep -Fx fixture-build "$tmp/state/log" >/dev/null && grep -Fx image-build "$tmp/state/log" >/dev/null && grep -Fx image-load "$tmp/state/log" >/dev/null && grep -Fx selected-test "$tmp/state/log" >/dev/null && grep -Fx cluster-delete "$tmp/state/log" >/dev/null && grep -Fx image-delete "$tmp/state/log" >/dev/null && grep -Fx -- -tags=accelerator_kind "$tmp/state/test-args" >/dev/null && grep -Fx -- '-run' "$tmp/state/test-args" >/dev/null && grep -Fx -- '^TestAccelerator00ExactPodLoopbackKind$' "$tmp/state/test-args" >/dev/null || { echo 'harness:stateful-cleanup' >&2; exit 1; }
# When the daemon loopback is outside this network namespace, the same tagged
# test is compiled and executed inside the owned node. Cleanup remains exact.
set +e; output="$(run remote-daemon "$root/scripts/test-accelerator-00-kind.sh" 2>&1)"; rc=$?; set -e
[ "$rc" -eq 0 ] && [ -z "$output" ] && [ ! -e "$tmp/state/cluster" ] && [ ! -e "$tmp/state/image" ] && [ ! -e "$tmp/state/test-copied" ] && [ ! -e "$tmp/state/config-copied" ] && grep -Fx runner-build "$tmp/state/log" >/dev/null && grep -Fx selected-runner-test "$tmp/state/log" >/dev/null && grep -Fx cluster-delete "$tmp/state/log" >/dev/null && grep -Fx image-delete "$tmp/state/log" >/dev/null || { echo 'harness:remote-daemon' >&2; exit 1; }
# The owned-node path preserves Docker architecture normalization for arm64.
set +e; output="$(run arm64-fallback "$root/scripts/test-accelerator-00-kind.sh" 2>&1)"; rc=$?; set -e
[ "$rc" -eq 0 ] && [ -z "$output" ] && [ ! -e "$tmp/state/cluster" ] && [ ! -e "$tmp/state/image" ] && [ ! -e "$tmp/state/test-copied" ] && [ ! -e "$tmp/state/config-copied" ] && grep -Fx runner-build "$tmp/state/log" >/dev/null && grep -Fx selected-runner-test "$tmp/state/log" >/dev/null || { echo 'harness:arm64-fallback' >&2; exit 1; }
# A successful mocked run also exercises the forced-cleanup path and preserves
# the private, fixed diagnostic rather than command output.
set +e; output="$(ACCELERATOR00_FORCE_CLEANUP_FAILURE=1 run '' "$root/scripts/test-accelerator-00-kind.sh" 2>&1)"; rc=$?; set -e
[ "$rc" -ne 0 ] && [ "$output" = 'accelerator-00-kind:cleanup-failed' ] || { echo 'harness:cleanup' >&2; exit 1; }
