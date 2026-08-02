#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { echo "accelerator-desktop-provision-kind: $1" >&2; exit 1; }

for tool in kind docker kubectl helm oras go curl openssl; do
  command -v "$tool" >/dev/null 2>&1 || fail "missing-$tool"
done
docker info >/dev/null 2>&1 || fail "docker-daemon-unavailable"
kind version 2>/dev/null | grep -F 'kind v0.32.0 ' >/dev/null || fail "kind-v0.32.0-required"
helm version --short 2>/dev/null | grep -F 'v3.21.3+' >/dev/null || fail "helm-v3.21.3-required"
oras version 2>/dev/null | grep -F 'Version:        1.3.3' >/dev/null || fail "oras-v1.3.3-required"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"$root/scripts/check-accelerator-desktop-provision-scope.sh"
source_image="${ACCELERATOR_PROVISION_KIND_IMAGE:-kubikles-accelerator:provision-kind}"
docker image inspect "$source_image" >/dev/null 2>&1 || fail "missing-image-$source_image"
docker image inspect kindest/node:v1.32.2 >/dev/null 2>&1 || fail "missing-kindest-node-v1.32.2"
docker image inspect registry:2.8.3 >/dev/null 2>&1 || fail "missing-registry-v2.8.3"
test -f "$root/deploy/charts/kubikles-accelerator/Chart.yaml" || fail "missing-chart-source"

tmp="$(mktemp -d "${TMPDIR:-/tmp}/kubikles-provision-kind.XXXXXX")"
cluster="kubikles-provision-$RANDOM-$RANDOM"
registry_container="${cluster}-registry"
chart_registry_container="${cluster}-chart-registry"
kubeconfig="$tmp/kubeconfig"
test_home="$tmp/home"
sentinel="kubikles-provision-sentinel"
malformed="kubikles-accelerator-ffffffffffffffffffffffffffffffff"
cluster_created=false
registry_created=false
chart_registry_created=false
cleanup_failed=false

cleanup() {
	status=$?
	trap - EXIT INT TERM
  set +e
  if "$registry_created"; then
    docker rm -f "$registry_container" >/dev/null 2>&1
    docker container inspect "$registry_container" >/dev/null 2>&1 && cleanup_failed=true
  fi
  if "$chart_registry_created"; then
    docker rm -f "$chart_registry_container" >/dev/null 2>&1
    docker container inspect "$chart_registry_container" >/dev/null 2>&1 && cleanup_failed=true
  fi
  if "$cluster_created"; then
    kind delete cluster --name "$cluster" >/dev/null 2>&1
    kind get clusters 2>/dev/null | grep -Fx "$cluster" >/dev/null && cleanup_failed=true
  fi
  docker image rm "127.0.0.1:${registry_port:-1}/skrobylabs/kubikles-accelerator:v1.2.3" >/dev/null 2>&1 || true
  docker image rm "ghcr.io/skrobylabs/kubikles-accelerator:provision-kind" >/dev/null 2>&1 || true
  rm -rf "$tmp"
  if "$cleanup_failed"; then
    echo "accelerator-desktop-provision-kind: cleanup-failed" >&2
    status=1
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

docker run --detach --rm --name "$registry_container" --publish 127.0.0.1:0:5000 registry:2.8.3 >"$tmp/registry-id" 2>"$tmp/registry-start" || fail "registry-start"
registry_created=true
registry_port="$(docker inspect "$registry_container" --format '{{(index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort}}')"
[[ "$registry_port" =~ ^[1-9][0-9]{0,4}$ ]] || fail "registry-port"
registry_daemon="127.0.0.1:$registry_port"
registry_host="${ACCELERATOR_PROVISION_REGISTRY_HOST:-host.docker.internal}"
registry="$registry_host:$registry_port"
registry_ready=false
for _ in $(seq 1 50); do
  if curl --noproxy '*' --fail --silent --show-error "http://$registry/v2/" >/dev/null 2>&1; then
    registry_ready=true
    break
  fi
  sleep 0.1
done
"$registry_ready" || fail "registry-not-ready"

mkdir -p "$tmp/chart-registry-certs"
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=host.docker.internal' -addext 'subjectAltName=DNS:localhost,DNS:host.docker.internal,IP:127.0.0.1' \
  -keyout "$tmp/chart-registry-certs/key.pem" -out "$tmp/chart-registry-certs/cert.pem" >"$tmp/chart-registry-cert" 2>&1 || fail "chart-registry-cert"
docker create --name "$chart_registry_container" --publish 127.0.0.1:0:5000 \
  --env REGISTRY_HTTP_ADDR=0.0.0.0:5000 --env REGISTRY_HTTP_TLS_CERTIFICATE=/cert.pem --env REGISTRY_HTTP_TLS_KEY=/key.pem \
  registry:2.8.3 >"$tmp/chart-registry-id" 2>"$tmp/chart-registry-create" || fail "chart-registry-create"
chart_registry_created=true
docker cp "$tmp/chart-registry-certs/cert.pem" "$chart_registry_container:/cert.pem" >"$tmp/chart-registry-cert-copy" 2>&1 || fail "chart-registry-cert-copy"
docker cp "$tmp/chart-registry-certs/key.pem" "$chart_registry_container:/key.pem" >"$tmp/chart-registry-key-copy" 2>&1 || fail "chart-registry-key-copy"
docker start "$chart_registry_container" >"$tmp/chart-registry-start" 2>&1 || fail "chart-registry-start"
chart_registry_port="$(docker inspect "$chart_registry_container" --format '{{(index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort}}')"
[[ "$chart_registry_port" =~ ^[1-9][0-9]{0,4}$ ]] || fail "chart-registry-port"
chart_registry="host.docker.internal:$chart_registry_port"
chart_registry_ready=false
for _ in $(seq 1 50); do
  if curl --noproxy '*' --cacert "$tmp/chart-registry-certs/cert.pem" --fail --silent --show-error "https://$chart_registry/v2/" >/dev/null 2>&1; then
    chart_registry_ready=true
    break
  fi
  sleep 0.1
done
if ! "$chart_registry_ready"; then
  docker logs "$chart_registry_container" >&2 || true
  fail "chart-registry-not-ready"
fi

image_daemon_ref="$registry_daemon/skrobylabs/kubikles-accelerator:v1.2.3"
image_client_ref="$registry/skrobylabs/kubikles-accelerator:v1.2.3"
docker tag "$source_image" "$image_daemon_ref" || fail "image-tag"
docker push "$image_daemon_ref" >"$tmp/image-push" 2>&1 || fail "image-push"
image_digest="$(oras resolve --plain-http "$image_client_ref" 2>"$tmp/image-resolve-error")" || fail "image-resolve"
[[ "$image_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "image-digest"

helm package "$root/deploy/charts/kubikles-accelerator" --version 1.2.3 --app-version v1.2.3 --destination "$tmp" >"$tmp/chart-package" 2>&1 || fail "chart-package"
chart_archive="$tmp/kubikles-accelerator-1.2.3.tgz"
test -s "$chart_archive" || fail "chart-archive"
helm push "$chart_archive" "oci://$chart_registry/skrobylabs/helm" --insecure-skip-tls-verify >"$tmp/chart-push" 2>&1 || fail "chart-push"
chart_ref="$chart_registry/skrobylabs/helm/kubikles-accelerator:1.2.3"
chart_digest="$(oras resolve --insecure "$chart_ref" 2>"$tmp/chart-resolve-error")" || fail "chart-resolve"
[[ "$chart_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "chart-digest"

cat >"$tmp/kind.yaml" <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry]
      config_path = "/etc/containerd/certs.d"
    [plugins."io.containerd.cri.v1.images".registry]
      config_path = "/etc/containerd/certs.d"
EOF
kind create cluster --name "$cluster" --image kindest/node:v1.32.2 --config "$tmp/kind.yaml" --kubeconfig "$kubeconfig" --wait 90s >"$tmp/kind-create" 2>&1 || fail "kind-create"
cluster_created=true
api_host="${ACCELERATOR_PROVISION_KIND_API_HOST:-host.docker.internal}"
cluster_name="$(KUBECONFIG="$kubeconfig" kubectl config view -o jsonpath='{.contexts[0].context.cluster}')"
server="$(KUBECONFIG="$kubeconfig" kubectl config view -o "jsonpath={.clusters[?(@.name=='$cluster_name')].cluster.server}")"
api_port="${server##*:}"
[[ "$api_port" =~ ^[1-9][0-9]{0,4}$ ]] || fail "kind-api-port"
KUBECONFIG="$kubeconfig" kubectl config set-cluster "$cluster_name" --server="https://$api_host:$api_port" --tls-server-name=localhost >/dev/null 2>&1 || fail "kind-api-rewrite"
docker network connect kind "$registry_container" >/dev/null 2>&1 || fail "registry-network"
node="${cluster}-control-plane"
docker exec "$node" mkdir -p /etc/containerd/certs.d/ghcr.io >/dev/null 2>&1 || fail "registry-mirror-directory"
docker exec "$node" sh -c 'cat > /etc/containerd/certs.d/ghcr.io/hosts.toml' <<EOF || fail "registry-mirror-config"
server = "https://ghcr.io"
[host."http://$registry_container:5000"]
  capabilities = ["pull", "resolve"]
EOF
kind_image_ref="ghcr.io/skrobylabs/kubikles-accelerator:provision-kind"
docker tag "$source_image" "$kind_image_ref" || fail "kind-image-tag"
kind load docker-image --name "$cluster" "$kind_image_ref" >"$tmp/kind-image-load" 2>&1 || fail "kind-image-load"
docker exec "$node" ctr --namespace k8s.io images tag "$kind_image_ref" "ghcr.io/skrobylabs/kubikles-accelerator@$image_digest" >"$tmp/kind-image-digest-tag" 2>&1 || fail "kind-image-digest-tag"
docker exec "$node" crictl inspecti "ghcr.io/skrobylabs/kubikles-accelerator@$image_digest" >"$tmp/kind-image-inspect" 2>&1 || fail "kind-image-digest-missing"

api_ready=false
for _ in $(seq 1 60); do
  if KUBECONFIG="$kubeconfig" kubectl --request-timeout=2s get --raw=/readyz 2>/dev/null | grep -Fx ok >/dev/null; then
    api_ready=true
    break
  fi
  sleep 0.5
done
"$api_ready" || fail "kind-api-unreachable"

mkdir -p "$test_home/.kube"
cp "$kubeconfig" "$test_home/.kube/config"
current_context="$(KUBECONFIG="$kubeconfig" kubectl config current-context)"
test -n "$current_context" || fail "current-context"
configured_namespace="$(KUBECONFIG="$kubeconfig" kubectl config view -o "jsonpath={.contexts[?(@.name=='$current_context')].context.namespace}")"
test -z "$configured_namespace" || fail "kind-context-namespace-must-be-empty"

mkdir -p "$tmp/sentinel/templates"
cat >"$tmp/sentinel/Chart.yaml" <<'EOF'
apiVersion: v2
name: sentinel
type: application
version: 0.1.0
EOF
cat >"$tmp/sentinel/templates/configmap.yaml" <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
data:
  retained: "true"
EOF
KUBECONFIG="$kubeconfig" helm install "$sentinel" "$tmp/sentinel" --namespace default >"$tmp/sentinel-install" 2>&1 || fail "sentinel-install"
KUBECONFIG="$kubeconfig" helm install "$malformed" "$tmp/sentinel" --namespace default >"$tmp/malformed-install" 2>&1 || fail "malformed-install"

go_test_timeout=5m
if [[ "${ACCELERATOR_DISPOSAL_KIND:-0}" == "1" || "${ACCELERATOR_LIFECYCLE_KIND:-0}" == "1" ]]; then
  go_test_timeout=9m
fi
(cd "$root" && HOME="$test_home" KUBECONFIG="$kubeconfig" \
  ACCELERATOR_PROVISION_KIND_CHART="$chart_archive" \
  ACCELERATOR_PROVISION_KIND_CHART_DIGEST="$chart_digest" \
  ACCELERATOR_PROVISION_KIND_REGISTRY_TLS_URL="https://$chart_registry" \
  ACCELERATOR_PROVISION_KIND_REGISTRY_CA="$tmp/chart-registry-certs/cert.pem" \
  ACCELERATOR_PROVISION_KIND_IMAGE_DIGEST="$image_digest" \
  ACCELERATOR_PROVISION_KIND_SENTINEL="$sentinel" \
  ACCELERATOR_PROVISION_KIND_MALFORMED="$malformed" \
  go test -v -tags=helm,accelerator_provision_kind -count=1 -timeout="$go_test_timeout" ./pkg/acceleratorprovision -run '^TestAcceleratorDesktopProvisionKind$') >"$tmp/go-test" 2>&1 || {
  sed -E \
    -e 's/[A-Za-z0-9_-]{43}/[REDACTED_43]/g' \
    -e 's#Bearer registry-secret-T11#[REDACTED_TEST_CORPUS]#g' \
    -e 's#/secret/kubeconfig/path-T11#[REDACTED_TEST_CORPUS]#g' \
    -e 's#raw-(registry|helm|kubernetes-status|cleanup)-error-T11#[REDACTED_TEST_CORPUS]#g' \
    -e 's#raw helm (manifest|value) T11#[REDACTED_TEST_CORPUS]#g' \
    "$tmp/go-test" >&2
  fail "go-service-test"
}

while IFS= read -r sensitive; do
  if grep -Fq -- "$sensitive" "$tmp/go-test"; then
    fail "go-service-output-sensitive"
  fi
done <<'EOF'
AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8
MDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSElKS0xNTk8
w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM
66-MHiCJFjcS0tgtsrGBc-19KNhsQvlV7hXKV3AF2Mo
Bearer registry-secret-T11
/secret/kubeconfig/path-T11
raw-registry-error-T11
raw-helm-error-T11
raw-kubernetes-status-T11
raw-cleanup-error-T11
raw helm manifest T11
raw helm value T11
EOF

echo "accelerator-desktop-provision-kind: passed" >&2
