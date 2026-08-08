#!/usr/bin/env bash

# Shared, test-only ownership helpers for Accelerator 60A. Callers retain
# set -e/-u policy and provide their own fixed failure classification.

accelerator_e2e_normalize_architecture() {
  [ "$#" -eq 1 ] || return 1
  case "$1" in
    amd64|x86_64) printf '%s\n' amd64 ;;
    arm64|aarch64) printf '%s\n' arm64 ;;
    *) return 1 ;;
  esac
}

accelerator_e2e_execution_architecture() {
  [ "$#" -eq 0 ] || return 1
  if [ -n "${KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE-}" ]; then
    case "$KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE" in
      amd64|arm64) printf '%s\n' "$KUBIKLES_ACCELERATOR_E2E_EXECUTION_ARCHITECTURE" ;;
      *) return 1 ;;
    esac
    return
  fi
  accelerator_e2e_normalize_architecture "$(docker info --format '{{.Architecture}}' 2>/dev/null)"
}

accelerator_e2e_kind_image_architecture() {
  [ "$#" -eq 0 ] || return 1
  accelerator_e2e_normalize_architecture "$(docker image inspect kindest/node:v1.32.2 --format '{{.Architecture}}' 2>/dev/null)"
}

accelerator_e2e_reuse_mode() {
  local key present=0
  for key in \
    KUBIKLES_ACCELERATOR_E2E_REUSE \
    KUBIKLES_ACCELERATOR_E2E_KIND_NAME \
    KUBIKLES_ACCELERATOR_E2E_KUBECONFIG \
    KUBIKLES_ACCELERATOR_E2E_NAMESPACE \
    KUBIKLES_ACCELERATOR_E2E_REGISTRY; do
    [ -z "${!key-}" ] || present=$((present + 1))
  done
  if [ "$present" -eq 0 ]; then
    printf '%s\n' standalone
    return 0
  fi
  [ "$present" -eq 5 ] || return 1
  [ "$KUBIKLES_ACCELERATOR_E2E_REUSE" = 1 ] || return 1
  [[ "$KUBIKLES_ACCELERATOR_E2E_KIND_NAME" =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$ ]] || return 1
  [[ "$KUBIKLES_ACCELERATOR_E2E_NAMESPACE" =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$ ]] || return 1
  [[ "$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" = /* ]] || return 1
  [ -f "$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" ] || return 1
  [ "$(stat -c '%a' "$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" 2>/dev/null || stat -f '%Lp' "$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" 2>/dev/null)" = 600 ] || return 1
  [[ "$KUBIKLES_ACCELERATOR_E2E_REGISTRY" =~ ^(127\.0\.0\.1|host\.docker\.internal):([1-9][0-9]{0,4})$ ]] || return 1
  [ "$((10#${BASH_REMATCH[2]}))" -le 65535 ] || return 1
  printf '%s\n' reuse
}

accelerator_e2e_allocate_case_namespace() {
  local prefix="${1-}" case_id="${2-}" suffix namespace
  [ "$#" -eq 2 ] || return 1
  [[ "$prefix" =~ ^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$ ]] || return 1
  [[ "$case_id" =~ ^A60A-([0-9]{3})-([A-Z][A-Z0-9-]*)$ ]] || return 1
  suffix="${BASH_REMATCH[1]}-${BASH_REMATCH[2],,}"
  namespace="$prefix-$suffix"
  [ "${#namespace}" -le 63 ] || return 1
  [[ "$namespace" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] || return 1
  printf '%s\n' "$namespace"
}

accelerator_e2e_preflight() {
  local root tool go_version node_version npm_version kubectl_version execution_architecture
  root="${1-}"
  [ "$#" -eq 1 ] && [ -d "$root" ] || return 1
  for tool in go node npm docker helm oras kind kubectl curl jq git; do
    command -v "$tool" >/dev/null 2>&1 || return 1
  done
  go_version="$(go version 2>/dev/null)"
  [[ "$go_version" =~ ^go\ version\ go1\.25\.12\ linux/(amd64|arm64)$ ]] || return 1
  node_version="$(node --version 2>/dev/null)"; [ "$node_version" = v22.23.2 ] || return 1
  npm_version="$(npm --version 2>/dev/null)"; [ "$npm_version" = 10.9.8 ] || return 1
  docker info >/dev/null 2>&1 || return 1
  [ "$(docker buildx version 2>/dev/null)" = 'github.com/docker/buildx v0.36.0 df28b0a0b6a44453a87bd53c438432f4120962c9' ] || return 1
  [ "$(helm version --short 2>/dev/null)" = 'v3.21.3+g1ad6e68' ] || return 1
  [ "$(oras version 2>/dev/null | sed -n 's/^Version:[[:space:]]*//p')" = 1.3.3 ] || return 1
  [ "$(oras version 2>/dev/null | sed -n 's/^Git commit:[[:space:]]*//p')" = 210747c29c1d38732b3194878dfd8b5a6b9ad7eb ] || return 1
  kind version 2>/dev/null | grep -F 'kind v0.32.0 ' >/dev/null || return 1
  kubectl_version="$(kubectl version --client -o json 2>/dev/null | jq -r '.clientVersion.gitVersion')"
  [ "$kubectl_version" = v1.36.3 ] || return 1
  execution_architecture="$(accelerator_e2e_execution_architecture)" || return 1
  [ "$(accelerator_e2e_kind_image_architecture)" = "$execution_architecture" ] || return 1
  docker image inspect registry:2.8.3 >/dev/null 2>&1 || return 1
  [ -x "$root/scripts/publish-accelerator-release.sh" ] || return 1
  [ -d "$root/scripts/cmd/inspect-accelerator-binary" ] || return 1
}

accelerator_e2e_validate_execution_node() {
  local execution_architecture nodes
  [ "$#" -eq 0 ] && [ "$(accelerator_e2e_reuse_mode)" = reuse ] || return 1
  execution_architecture="$(accelerator_e2e_execution_architecture)" || return 1
  nodes="$(KUBECONFIG="$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" kubectl get nodes -o json 2>/dev/null)" || return 1
  jq -e --arg execution "$execution_architecture" '
    .apiVersion == "v1" and .kind == "List" and (.items | length) == 1 and
    .items[0].status.nodeInfo.architecture == $execution and
    .items[0].metadata.labels["kubernetes.io/arch"] == $execution
  ' <<<"$nodes" >/dev/null
}

accelerator_e2e_validate_reused_fixture() {
  local current
  [ "$(accelerator_e2e_reuse_mode)" = reuse ] || return 1
  kind get clusters 2>/dev/null | grep -Fx "$KUBIKLES_ACCELERATOR_E2E_KIND_NAME" >/dev/null || return 1
  current="$(KUBECONFIG="$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" kubectl config current-context 2>/dev/null)" || return 1
  [ -n "$current" ] || return 1
  KUBECONFIG="$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" kubectl --request-timeout=2s get --raw=/readyz 2>/dev/null | grep -Fx ok >/dev/null || return 1
  accelerator_e2e_validate_execution_node || return 1
  curl --noproxy '*' --fail --silent --show-error "http://$KUBIKLES_ACCELERATOR_E2E_REGISTRY/v2/" >/dev/null 2>&1 || return 1
}

accelerator_e2e_prepare_child_kubeconfig() {
  local destination="${1-}" namespace="${2-}" context
  [ "$#" -eq 2 ] && [ "$(accelerator_e2e_reuse_mode)" = reuse ] || return 1
  [[ "$destination" = /* ]] && [ -n "$namespace" ] || return 1
  cp "$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" "$destination" || return 1
  chmod 600 "$destination" || return 1
  context="$(KUBECONFIG="$destination" kubectl config current-context 2>/dev/null)" || return 1
  [ -n "$context" ] || return 1
  KUBECONFIG="$destination" kubectl config set-context "$context" --namespace="$namespace" >/dev/null 2>&1 || return 1
}

accelerator_e2e_select_registry_host() {
  local port="${1-}" configured="${ACCELERATOR_PROVISION_REGISTRY_HOST-}" host
  local -a candidates
  [ "$#" -eq 1 ] || return 1
  [[ "$port" =~ ^[1-9][0-9]{0,4}$ ]] && [ "$((10#$port))" -le 65535 ] || return 1
  if [ -n "$configured" ]; then
    case "$configured" in 127.0.0.1|host.docker.internal) ;; *) return 1;; esac
    candidates=("$configured")
  else
    # Native Linux exposes loopback-published ports on the host, while a
    # mounted Docker Desktop socket exposes them through its VM gateway.
    candidates=(127.0.0.1 host.docker.internal)
  fi
  for _ in $(seq 1 100); do
    for host in "${candidates[@]}"; do
      if curl --noproxy '*' --fail --silent --show-error "http://$host:$port/v2/" >/dev/null 2>&1; then
        printf '%s\n' "$host"
        return 0
      fi
    done
    sleep 0.1
  done
  return 1
}

accelerator_e2e_create_owned_fixture() {
  local state="${1-}" root="${2-}" nonce cluster registry_container registry_port registry_host registry_endpoint tmp kubeconfig node api_server kube_cluster execution_architecture
  [ "$#" -eq 2 ] && [ -d "$state" ] && [ -d "$root" ] || return 1
  [ ! -e "$state/cluster-owned" ] && [ ! -e "$state/registry-owned" ] || return 1
  execution_architecture="$(accelerator_e2e_execution_architecture)" || return 1
  [ "$(accelerator_e2e_kind_image_architecture)" = "$execution_architecture" ] || return 1
  nonce="$$-$RANDOM-$RANDOM"
  cluster="kubikles-a60a-$nonce"
  registry_container="${cluster}-registry"
  tmp="$root/fixture"
  kubeconfig="$tmp/kubeconfig"
  mkdir -m 700 "$tmp" || return 1
  printf '%s\n' "$cluster" >"$state/kind-name"
  printf '%s\n' "$registry_container" >"$state/registry-container"
  printf '%s\n' "$root" >"$state/temp-root"
  docker container inspect "$registry_container" >/dev/null 2>&1 && return 1
  kind get clusters 2>/dev/null | grep -Fx "$cluster" >/dev/null && return 1
  : >"$state/registry-owned"
  docker run --detach --rm --name "$registry_container" --publish 127.0.0.1:0:5000 registry:2.8.3 >"$tmp/registry-id" 2>"$tmp/registry-start" || return 1
  registry_port="$(docker inspect "$registry_container" --format '{{(index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort}}')" || return 1
  [[ "$registry_port" =~ ^[1-9][0-9]{0,4}$ ]] && [ "$((10#$registry_port))" -le 65535 ] || return 1
  registry_host="$(accelerator_e2e_select_registry_host "$registry_port")" || return 1
  registry_endpoint="$registry_host:$registry_port"
  cat >"$tmp/kind.yaml" <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry]
      config_path = "/etc/containerd/certs.d"
    [plugins."io.containerd.cri.v1.images".registry]
      config_path = "/etc/containerd/certs.d"
EOF
  : >"$state/cluster-owned"
  kind create cluster --name "$cluster" --image kindest/node:v1.32.2 --config "$tmp/kind.yaml" --kubeconfig "$kubeconfig" --wait 90s >"$tmp/kind-create" 2>&1 || return 1
  chmod 600 "$kubeconfig" || return 1
  if [ "$registry_host" = host.docker.internal ]; then
    api_server="$(KUBECONFIG="$kubeconfig" kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' 2>/dev/null)" || return 1
    kube_cluster="$(KUBECONFIG="$kubeconfig" kubectl config view --minify -o jsonpath='{.contexts[0].context.cluster}' 2>/dev/null)" || return 1
    [[ "$api_server" =~ ^https://127\.0\.0\.1:([1-9][0-9]{0,4})$ ]] || return 1
    [ -n "$kube_cluster" ] || return 1
    KUBECONFIG="$kubeconfig" kubectl config set-cluster "$kube_cluster" \
      --server="https://host.docker.internal:${BASH_REMATCH[1]}" --tls-server-name=localhost >/dev/null 2>&1 || return 1
  fi
  docker network connect kind "$registry_container" >/dev/null 2>&1 || return 1
  registry_port="$(docker inspect "$registry_container" --format '{{(index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort}}')" || return 1
  [[ "$registry_port" =~ ^[1-9][0-9]{0,4}$ ]] && [ "$((10#$registry_port))" -le 65535 ] || return 1
  registry_endpoint="$registry_host:$registry_port"
  local ready=false
  for _ in $(seq 1 100); do
    if curl --noproxy '*' --fail --silent --show-error "http://$registry_endpoint/v2/" >/dev/null 2>&1; then
      ready=true
      break
    fi
    sleep 0.1
  done
  [ "$ready" = true ] || return 1
  node="${cluster}-control-plane"
  docker exec "$node" mkdir -p "/etc/containerd/certs.d/$registry_endpoint" >/dev/null 2>&1 || return 1
  docker exec -i "$node" sh -c "cat > '/etc/containerd/certs.d/$registry_endpoint/hosts.toml'" <<EOF || return 1
server = "http://$registry_endpoint"
[host."http://$registry_container:5000"]
  capabilities = ["pull", "resolve"]
EOF
  docker exec "$node" mkdir -p /etc/containerd/certs.d/ghcr.io >/dev/null 2>&1 || return 1
  docker exec -i "$node" sh -c "cat > /etc/containerd/certs.d/ghcr.io/hosts.toml" <<EOF || return 1
server = "https://ghcr.io"
[host."http://$registry_container:5000"]
  capabilities = ["pull", "resolve"]
EOF
  ready=false
  for _ in $(seq 1 150); do
    if KUBECONFIG="$kubeconfig" kubectl --request-timeout=2s get --raw=/readyz 2>/dev/null | grep -Fx ok >/dev/null; then
      ready=true
      break
    fi
    sleep 0.2
  done
  [ "$ready" = true ] || return 1
  KUBECONFIG="$kubeconfig" kubectl create namespace kubikles-a60a-sentinel >"$tmp/sentinel-namespace" 2>&1 || return 1
  KUBECONFIG="$kubeconfig" kubectl -n kubikles-a60a-sentinel create configmap foreign-sentinel --from-literal=retained=true >"$tmp/sentinel-create" 2>&1 || return 1
  export KUBIKLES_ACCELERATOR_E2E_REUSE=1
  export KUBIKLES_ACCELERATOR_E2E_KIND_NAME="$cluster"
  export KUBIKLES_ACCELERATOR_E2E_KUBECONFIG="$kubeconfig"
  export KUBIKLES_ACCELERATOR_E2E_NAMESPACE="kubikles-a60a-$RANDOM"
  export KUBIKLES_ACCELERATOR_E2E_REGISTRY="$registry_endpoint"
  accelerator_e2e_validate_reused_fixture
}

accelerator_e2e_audit_case_cleanup() {
  local sentinel="${1-}" output cluster_scope cleared=false
  [ "$#" -eq 1 ] && [[ "$sentinel" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] || return 1
  accelerator_e2e_validate_reused_fixture || return 1
  if output="$(KUBECONFIG="$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" kubectl get namespace "$KUBIKLES_ACCELERATOR_E2E_NAMESPACE" 2>&1)"; then
    return 1
  fi
  grep -F '(NotFound)' <<<"$output" >/dev/null || return 1
  KUBECONFIG="$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" kubectl get namespace "$sentinel" >/dev/null 2>&1 || return 1
  for _ in $(seq 1 100); do
    if cluster_scope="$(KUBECONFIG="$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" kubectl get clusterroles,clusterrolebindings \
      -l 'app.kubernetes.io/name=kubikles-accelerator,app.kubernetes.io/component=accelerator,app.kubernetes.io/part-of=kubikles,app.kubernetes.io/managed-by=Helm' \
      -o json 2>/dev/null)" &&
      jq -e --arg namespace "$KUBIKLES_ACCELERATOR_E2E_NAMESPACE" \
        '([.items[]? | select(.metadata.annotations["meta.helm.sh/release-namespace"] == $namespace)] | length) == 0' \
        <<<"$cluster_scope" >/dev/null; then
      cleared=true
      break
    fi
    sleep 0.1
  done
  "$cleared" || return 1
}

accelerator_e2e_delete_case_cluster_scope() {
  local namespace="${1-}" selector resource objects names name
  [ "$#" -eq 1 ] && [ "$(accelerator_e2e_reuse_mode)" = reuse ] || return 1
  [ "$namespace" = "$KUBIKLES_ACCELERATOR_E2E_NAMESPACE" ] || return 1
  selector='app.kubernetes.io/name=kubikles-accelerator,app.kubernetes.io/component=accelerator,app.kubernetes.io/part-of=kubikles,app.kubernetes.io/managed-by=Helm'
  for resource in clusterrolebindings clusterroles; do
    objects="$(KUBECONFIG="$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" kubectl get "$resource" -l "$selector" -o json 2>/dev/null)" || return 1
    jq -e '.apiVersion == "v1" and .kind == "List" and (.items | type == "array")' <<<"$objects" >/dev/null || return 1
    names="$(jq -r --arg namespace "$namespace" '.items[] | select(.metadata.annotations["meta.helm.sh/release-namespace"] == $namespace and (.metadata.annotations["meta.helm.sh/release-name"] | test("^kubikles-accelerator-[0-9a-f]{32}$"))) | .metadata.name' <<<"$objects")" || return 1
    while IFS= read -r name; do
      [ -z "$name" ] && continue
      [[ "$name" =~ ^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$ ]] || return 1
      KUBECONFIG="$KUBIKLES_ACCELERATOR_E2E_KUBECONFIG" kubectl delete "$resource" "$name" --wait=true --timeout=30s >/dev/null 2>&1 || return 1
    done <<<"$names"
  done
}

accelerator_e2e_cleanup_owned_fixture() {
  local state="${1-}" cluster registry_container temp_root temp_parent temp_name expected_parent failed=0
  [ "$#" -eq 1 ] || return 1
  [ -e "$state" ] || return 0
  [ -d "$state" ] || return 1
  if [ -f "$state/cluster-owned" ]; then
    IFS= read -r cluster <"$state/kind-name" || return 1
    [[ "$cluster" =~ ^kubikles-a60a-[0-9]+-[0-9]+-[0-9]+$ ]] || return 1
    kind delete cluster --name "$cluster" >/dev/null 2>&1 || true
    kind get clusters 2>/dev/null | grep -Fx "$cluster" >/dev/null && failed=1
    rm -f "$state/cluster-owned"
  fi
  if [ -f "$state/registry-owned" ]; then
    IFS= read -r registry_container <"$state/registry-container" || return 1
    [[ "$registry_container" =~ ^kubikles-a60a-[0-9]+-[0-9]+-[0-9]+-registry$ ]] || return 1
    docker rm -f "$registry_container" >/dev/null 2>&1 || true
    docker container inspect "$registry_container" >/dev/null 2>&1 && failed=1
    rm -f "$state/registry-owned"
  fi
  if [ -f "$state/temp-root" ]; then
    IFS= read -r temp_root <"$state/temp-root" || return 1
    temp_parent="${temp_root%/*}"
    temp_name="${temp_root##*/}"
    expected_parent="$(cd "${TMPDIR:-/tmp}" && pwd -P)" || return 1
    [ "$temp_parent" = "$expected_parent" ] || return 1
    [[ "$temp_name" =~ ^kubikles-(accelerator-e2e|a60a-smoke)\.[A-Za-z0-9.-]+$ ]] || return 1
    rm -rf -- "$temp_root"
    [ ! -e "$temp_root" ] || failed=1
    rm -f "$state/temp-root"
  fi
  rm -f "$state/kind-name" "$state/registry-container"
  rmdir "$state" 2>/dev/null || failed=1
  [ "$failed" -eq 0 ]
}
