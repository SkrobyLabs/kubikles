#!/usr/bin/env bash
set -euo pipefail
umask 077

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$root/scripts/lib/accelerator-e2e-kind.sh"

fail() { echo "accelerator-e2e-kind-test: $1" >&2; exit 1; }

clear_reuse() {
  unset KUBIKLES_ACCELERATOR_E2E_REUSE KUBIKLES_ACCELERATOR_E2E_KIND_NAME \
    KUBIKLES_ACCELERATOR_E2E_KUBECONFIG KUBIKLES_ACCELERATOR_E2E_NAMESPACE \
    KUBIKLES_ACCELERATOR_E2E_REGISTRY KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE
}

clear_reuse
[ "$(accelerator_e2e_normalize_architecture x86_64)" = amd64 ] || fail normalize-x86
[ "$(accelerator_e2e_normalize_architecture aarch64)" = arm64 ] || fail normalize-arm
if accelerator_e2e_normalize_architecture s390x >/dev/null 2>&1; then fail unsupported-architecture-accepted; fi
[ "$(accelerator_e2e_reuse_mode)" = standalone ] || fail standalone-mode
KUBIKLES_ACCELERATOR_E2E_REUSE=1
if accelerator_e2e_reuse_mode >/dev/null 2>&1; then fail partial-reuse-accepted; fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/accelerator-e2e-helper-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
touch "$tmp/kubeconfig"
chmod 600 "$tmp/kubeconfig"
export KUBIKLES_ACCELERATOR_E2E_REUSE=1
export KUBIKLES_ACCELERATOR_E2E_KIND_NAME=kubikles-a60a-test
export KUBIKLES_ACCELERATOR_E2E_KUBECONFIG="$tmp/kubeconfig"
export KUBIKLES_ACCELERATOR_E2E_NAMESPACE=kubikles-a60a-001
export KUBIKLES_ACCELERATOR_E2E_REGISTRY=127.0.0.1:49152
export KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE=arm64
[ "$(accelerator_e2e_reuse_mode)" = reuse ] || fail reuse-mode

for invalid in '../escape' 'UPPER' 'a_b' ''; do
  KUBIKLES_ACCELERATOR_E2E_NAMESPACE="$invalid"
  if accelerator_e2e_reuse_mode >/dev/null 2>&1; then fail invalid-namespace-accepted; fi
done
KUBIKLES_ACCELERATOR_E2E_NAMESPACE=kubikles-a60a-001

[ "$(accelerator_e2e_allocate_case_namespace kubikles-a60a A60A-001-BASELINE)" = kubikles-a60a-001-baseline ] || fail namespace-allocation
if accelerator_e2e_allocate_case_namespace kubikles-a60a A60A-001-BASELINE/escape >/dev/null 2>&1; then fail unsafe-case-accepted; fi

mkdir "$tmp/bin"
cat >"$tmp/bin/kind" <<'EOF'
#!/usr/bin/env bash
printf 'kind %s\n' "$*" >>"$FAKE_LOG"
case "$1:$2" in
  'get:clusters') printf '%s\n' "$KUBIKLES_ACCELERATOR_E2E_KIND_NAME" ;;
esac
EOF
cat >"$tmp/bin/docker" <<'EOF'
#!/usr/bin/env bash
printf 'docker %s\n' "$*" >>"$FAKE_LOG"
case "$1:$2" in 'container:inspect') exit 1;; esac
case "$1:$2" in
  'info:--format') printf 'aarch64\n' ;;
  'image:inspect') if [[ "$*" = *'kindest/node:v1.32.2'*'--format'* ]]; then printf 'arm64\n'; fi ;;
esac
exit 0
EOF
cat >"$tmp/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
printf 'kubectl %s\n' "$*" >>"$FAKE_LOG"
case "$*" in
  'config current-context') printf 'kind-kubikles-a60a-test\n' ;;
  *'get --raw=/readyz'*) printf 'ok\n' ;;
  *'get nodes -o json'*) printf '{"apiVersion":"v1","kind":"List","items":[{"metadata":{"name":"kubikles-a60a-test-control-plane","labels":{"kubernetes.io/arch":"arm64"}},"status":{"nodeInfo":{"architecture":"arm64"}}}]}\n' ;;
  *'get clusterroles,clusterrolebindings -l app.kubernetes.io/name=kubikles-accelerator,app.kubernetes.io/component=accelerator,app.kubernetes.io/part-of=kubikles,app.kubernetes.io/managed-by=Helm -o json'*) printf '{"items":[]}\n' ;;
  *'get clusterrolebindings -l app.kubernetes.io/name=kubikles-accelerator,app.kubernetes.io/component=accelerator,app.kubernetes.io/part-of=kubikles,app.kubernetes.io/managed-by=Helm -o json'*) printf '{"apiVersion":"v1","kind":"List","items":[{"metadata":{"name":"owned-binding","annotations":{"meta.helm.sh/release-namespace":"kubikles-a60a-001","meta.helm.sh/release-name":"kubikles-accelerator-0123456789abcdef0123456789abcdef"}}},{"metadata":{"name":"foreign-binding","annotations":{"meta.helm.sh/release-namespace":"foreign","meta.helm.sh/release-name":"kubikles-accelerator-0123456789abcdef0123456789abcdef"}}}]}\n' ;;
  *'get clusterroles -l app.kubernetes.io/name=kubikles-accelerator,app.kubernetes.io/component=accelerator,app.kubernetes.io/part-of=kubikles,app.kubernetes.io/managed-by=Helm -o json'*) printf '{"apiVersion":"v1","kind":"List","items":[{"metadata":{"name":"owned-role","annotations":{"meta.helm.sh/release-namespace":"kubikles-a60a-001","meta.helm.sh/release-name":"kubikles-accelerator-0123456789abcdef0123456789abcdef"}}},{"metadata":{"name":"foreign-role","annotations":{"meta.helm.sh/release-namespace":"foreign","meta.helm.sh/release-name":"kubikles-accelerator-0123456789abcdef0123456789abcdef"}}}]}\n' ;;
  *'get namespace kubikles-a60a-sentinel'*) exit 0 ;;
  *'get namespace kubikles-a60a-001'*) printf 'Error from server (NotFound)\n' >&2; exit 1 ;;
esac
EOF
cat >"$tmp/bin/curl" <<'EOF'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >>"$FAKE_LOG"
if [ "${FAKE_CURL_HOST_DOCKER_ONLY:-0}" = 1 ] && [[ "$*" = *'http://127.0.0.1:'* ]]; then exit 1; fi
exit 0
EOF
chmod 700 "$tmp/bin/"*
export FAKE_LOG="$tmp/fake.log"
[ "$(PATH="$tmp/bin:$PATH" accelerator_e2e_execution_architecture)" = arm64 ] || fail forced-execution-architecture
KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE=amd64
export KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE
[ "$(PATH="$tmp/bin:$PATH" accelerator_e2e_execution_architecture)" = amd64 ] || fail forced-amd64-architecture
KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE=invalid
export KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE
if PATH="$tmp/bin:$PATH" accelerator_e2e_execution_architecture >/dev/null 2>&1; then fail invalid-forced-architecture-accepted; fi
unset KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE
[ "$(PATH="$tmp/bin:$PATH" accelerator_e2e_execution_architecture)" = arm64 ] || fail daemon-execution-architecture
export KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE=arm64
[ "$(PATH="$tmp/bin:$PATH" accelerator_e2e_kind_image_architecture)" = arm64 ] || fail kind-image-architecture
unset ACCELERATOR_PROVISION_REGISTRY_HOST
[ "$(PATH="$tmp/bin:$PATH" accelerator_e2e_select_registry_host 49152)" = 127.0.0.1 ] || fail loopback-registry-selection
FAKE_CURL_HOST_DOCKER_ONLY=1
export FAKE_CURL_HOST_DOCKER_ONLY
[ "$(PATH="$tmp/bin:$PATH" accelerator_e2e_select_registry_host 49152)" = host.docker.internal ] || fail docker-desktop-registry-selection
unset FAKE_CURL_HOST_DOCKER_ONLY
ACCELERATOR_PROVISION_REGISTRY_HOST=host.docker.internal
export ACCELERATOR_PROVISION_REGISTRY_HOST
[ "$(PATH="$tmp/bin:$PATH" accelerator_e2e_select_registry_host 49152)" = host.docker.internal ] || fail explicit-registry-selection
ACCELERATOR_PROVISION_REGISTRY_HOST=unsafe.example
export ACCELERATOR_PROVISION_REGISTRY_HOST
if PATH="$tmp/bin:$PATH" accelerator_e2e_select_registry_host 49152 >/dev/null 2>&1; then fail unsafe-registry-host-accepted; fi
unset ACCELERATOR_PROVISION_REGISTRY_HOST
PATH="$tmp/bin:$PATH" accelerator_e2e_validate_reused_fixture || fail reuse-validation
PATH="$tmp/bin:$PATH" accelerator_e2e_audit_case_cleanup kubikles-a60a-sentinel || fail cleanup-audit
PATH="$tmp/bin:$PATH" accelerator_e2e_delete_case_cluster_scope kubikles-a60a-001 || fail cluster-scope-cleanup
grep -Fx 'kubectl delete clusterrolebindings owned-binding --wait=true --timeout=30s' "$FAKE_LOG" >/dev/null || fail owned-binding-cleanup
grep -Fx 'kubectl delete clusterroles owned-role --wait=true --timeout=30s' "$FAKE_LOG" >/dev/null || fail owned-role-cleanup
if grep -F 'delete clusterrolebindings foreign-binding' "$FAKE_LOG" >/dev/null || grep -F 'delete clusterroles foreign-role' "$FAKE_LOG" >/dev/null; then fail foreign-cluster-scope-cleanup; fi

state="$tmp/state"
owned_root="$(mktemp -d "${TMPDIR:-/tmp}/kubikles-accelerator-e2e.test.XXXXXX")"
mkdir "$state"
printf '%s\n' kubikles-a60a-1-2-3 >"$state/kind-name"
printf '%s\n' kubikles-a60a-1-2-3-registry >"$state/registry-container"
printf '%s\n' "$owned_root" >"$state/temp-root"
touch "$state/cluster-owned" "$state/registry-owned"
PATH="$tmp/bin:$PATH" accelerator_e2e_cleanup_owned_fixture "$state" || fail owned-cleanup
PATH="$tmp/bin:$PATH" accelerator_e2e_cleanup_owned_fixture "$state" || fail idempotent-cleanup
[ ! -e "$owned_root" ] || fail temp-root-remained
[ "$(grep -Fc 'docker rm -f kubikles-a60a-1-2-3-registry' "$FAKE_LOG")" -eq 1 ] || fail registry-cleanup-count
[ "$(grep -Fc 'kind delete cluster --name kubikles-a60a-1-2-3' "$FAKE_LOG")" -eq 1 ] || fail cluster-cleanup-count
cluster_cleanup_line="$(grep -nF 'kind delete cluster --name kubikles-a60a-1-2-3' "$FAKE_LOG" | cut -d: -f1)"
registry_cleanup_line="$(grep -nF 'docker rm -f kubikles-a60a-1-2-3-registry' "$FAKE_LOG" | cut -d: -f1)"
[ "$cluster_cleanup_line" -lt "$registry_cleanup_line" ] || fail cleanup-order

mkdir "$tmp/empty-bin"
if PATH="$tmp/empty-bin" accelerator_e2e_preflight "$root" >/dev/null 2>&1; then fail missing-preflight-accepted; fi

echo 'accelerator-e2e-kind-test: passed' >&2
