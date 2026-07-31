#!/usr/bin/env bash
set -euo pipefail
umask 077
export LC_ALL=C

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/kubikles-accelerator00.XXXXXX")"; chmod 700 "$tmp"
cluster="kubikles-accelerator00-${$}-${RANDOM}"
image="kubikles-accelerator00:${$}-${RANDOM}"
cluster_owned=0
image_owned=0
stage() { echo "accelerator-00-kind:$1" >&2; exit 1; }
can_fallback_from_api_failure() {
  local diagnostic="$1"
  if grep -Eqi '^Error from server|http status|status code|readiness|readyz|x509|certificate|tls|unauthorized|forbidden|authentication|authorization|credentials|error loading config|invalid configuration|malformed|unable to parse|error converting yaml|the server has asked for the client to provide credentials' "$diagnostic"; then
    return 1
  fi
  grep -Eqi '^The connection to the server .+ was refused - did you specify the right host or port\?$|^(Unable to connect to the server: )?dial tcp .+: connect: connection refused$|^(Unable to connect to the server: )?dial tcp .+: connect: (no route to host|network is unreachable)$|^(Unable to connect to the server: )?dial tcp .+: i/o timeout$|^(Unable to connect to the server: )?dial tcp .+: connect: connection timed out$' "$diagnostic"
}
cleanup() {
  local rc=0
  if [ "$cluster_owned" -eq 1 ]; then
    # Ownership is claimed before creation, so a partially-created target is
    # cleaned too. Absence is an acceptable outcome for the unique target.
    kind delete cluster --name "$cluster" >"$tmp/cleanup-kind" 2>&1 || true
    kind get clusters >"$tmp/verify-cluster" 2>&1 || rc=1
    grep -Fx "$cluster" "$tmp/verify-cluster" >/dev/null && rc=1
  fi
  if [ "$image_owned" -eq 1 ]; then
    docker image rm "$image" >"$tmp/cleanup-image" 2>&1 || true
    docker image inspect "$image" >"$tmp/verify-image" 2>&1 && rc=1
  fi
  [ "${ACCELERATOR00_FORCE_CLEANUP_FAILURE:-}" != 1 ] || rc=1
  rm -rf "$tmp" || rc=1
  if [ "$rc" -ne 0 ]; then echo "accelerator-00-kind:cleanup-failed" >&2; fi
  return "$rc"
}
trap 'rc=$?; trap - EXIT INT TERM; cleanup || exit 1; exit "$rc"' EXIT
trap 'exit 1' INT TERM

for tool in kind kubectl docker go; do command -v "$tool" >/dev/null 2>&1 || stage "missing-${tool}"; done
docker info >/dev/null 2>&1 || stage daemon-unavailable
if kind get clusters 2>/dev/null | grep -Fx "$cluster" >/dev/null; then stage ownership-collision; fi
if docker image inspect "$image" >"$tmp/image-collision" 2>&1; then stage ownership-collision; fi
case "$(docker info --format '{{.Architecture}}' 2>"$tmp/architecture")" in
  amd64|x86_64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) stage unsupported-architecture ;;
esac
GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -o "$tmp/accelerator00-echo" "$root/testdata/accelerator00/echo" >"$tmp/build" 2>&1 || stage build
image_owned=1
docker build -t "$image" -f "$root/testdata/accelerator00/Dockerfile" "$tmp" >"$tmp/image" 2>&1 || stage image-build
cluster_owned=1
kind create cluster --name "$cluster" --kubeconfig "$tmp/kubeconfig" --wait 60s >"$tmp/cluster" 2>&1 || stage cluster-create
chmod 600 "$tmp/kubeconfig"
kind load docker-image "$image" --name "$cluster" >"$tmp/load" 2>&1 || stage image-load
if KUBECONFIG="$tmp/kubeconfig" kubectl --request-timeout=5s get --raw=/readyz >"$tmp/api-ready" 2>&1; then
  KUBECONFIG="$tmp/kubeconfig" ACCELERATOR00_KIND_IMAGE="$image" go test -tags=accelerator_kind -count=1 -timeout=8m ./pkg/k8s -run '^TestAccelerator00ExactPodLoopbackKind$' >"$tmp/test" 2>&1 || stage test
elif can_fallback_from_api_failure "$tmp/api-ready"; then
  # A mounted Docker socket can belong to a daemon in another network
  # namespace, making its loopback-published API port unreachable here. Run
  # the same tagged test binary in the owned control-plane container so the
  # gate remains live without exposing or rewriting an API address.
  GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go test -c -tags=accelerator_kind -o "$tmp/accelerator00-kind.test" ./pkg/k8s >"$tmp/test-build" 2>&1 || stage test-build
  kind get kubeconfig --internal --name "$cluster" >"$tmp/internal-kubeconfig" 2>"$tmp/internal-config" || stage internal-kubeconfig
  chmod 600 "$tmp/internal-kubeconfig"
  docker cp "$tmp/accelerator00-kind.test" "$cluster-control-plane:/accelerator00-kind.test" >"$tmp/test-copy" 2>&1 || stage test-copy
  docker cp "$tmp/internal-kubeconfig" "$cluster-control-plane:/accelerator00-kubeconfig" >"$tmp/config-copy" 2>&1 || stage config-copy
  docker exec -e KUBECONFIG=/accelerator00-kubeconfig -e ACCELERATOR00_KIND_IMAGE="$image" "$cluster-control-plane" /accelerator00-kind.test -test.count=1 -test.timeout=8m -test.run '^TestAccelerator00ExactPodLoopbackKind$' >"$tmp/test" 2>&1 || stage test
else
  stage api-ready
fi
