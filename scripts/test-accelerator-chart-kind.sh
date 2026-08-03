#!/usr/bin/env bash
set -euo pipefail
umask 077

accelerator_chart_kind_utc_seconds() {
  local timestamp="$1" normalized seconds
  [[ "$timestamp" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?Z$ ]] || return 1
  normalized="${timestamp:0:19}Z"
  if seconds="$(LC_ALL=C date -u -d "$normalized" +%s 2>/dev/null)"; then
    printf '%s\n' "$seconds"
    return 0
  fi
  LC_ALL=C date -j -u -f '%Y-%m-%dT%H:%M:%SZ' "$normalized" +%s 2>/dev/null
}

accelerator_chart_kind_listener_started_seconds() {
  local log_file="$1" records count timestamp
  records="$(LC_ALL=C grep -E '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?Z [0-9]{4}/[0-9]{2}/[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2} Server mode: listening on http://127\.0\.0\.1:8080$' "$log_file" || true)"
  count="$(printf '%s\n' "$records" | sed '/^$/d' | wc -l | tr -d '[:space:]')"
  [ "$count" = 1 ] || return 1
  timestamp="${records%% *}"
  accelerator_chart_kind_utc_seconds "$timestamp"
}

accelerator_chart_kind_pod_projection_is_exact() {
  local pod_json="$1" image="$2" verifier="$3"
  jq -e --arg image "$image" --arg verifier "$verifier" '(.spec.containers[0].name == "accelerator") and (.spec.containers[0].image == $image) and (.spec.containers[0].imagePullPolicy == "IfNotPresent") and (.spec.containers[0].securityContext == {allowPrivilegeEscalation:false,readOnlyRootFilesystem:true,capabilities:{drop:["ALL"]}}) and (.spec.containers[0].resources == {requests:{cpu:"100m",memory:"128Mi"},limits:{cpu:"1",memory:"512Mi"}}) and (.spec.containers[0].env == [{name:"KUBIKLES_ACCELERATOR_CREATOR_VERIFIER",valueFrom:{secretKeyRef:{name:$verifier,key:"creatorVerifier"}}}]) and (.spec.containers[0].volumeMounts == [{name:"serviceaccount",mountPath:"/var/run/secrets/kubernetes.io/serviceaccount",readOnly:true}]) and (.spec.volumes == [{name:"serviceaccount",projected:{defaultMode:292,sources:[{serviceAccountToken:{path:"token",expirationSeconds:3600}},{configMap:{name:"kube-root-ca.crt",items:[{key:"ca.crt",path:"ca.crt"}]}},{downwardAPI:{items:[{path:"namespace",fieldRef:{apiVersion:"v1",fieldPath:"metadata.namespace"}}]}}]}}])' "$pod_json" >/dev/null
}

accelerator_chart_kind_redact_stream() {
  local verifier="${1-}" token="${2-}" expressions
  expressions='s/[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}/[REDACTED_TOKEN]/g'
  [ -z "$verifier" ] || expressions="$expressions;s|$verifier|[REDACTED_VERIFIER]|g"
  [ -z "$token" ] || expressions="$expressions;s|$token|[REDACTED_TOKEN]|g"
  LC_ALL=C sed -E "$expressions"
}

accelerator_chart_kind_can_i_result() {
  local kubeconfig_file="${1-}" user="${2-}" namespace="${3-}" verb="${4-}" stderr_file="${5-}"
  local output status
  [ "$#" -eq 5 ] && [ -n "$kubeconfig_file" ] && [ -n "$user" ] && [ -n "$namespace" ] && [ -n "$verb" ] && [ -n "$stderr_file" ] || return 1
  if output="$(KUBECONFIG="$kubeconfig_file" kubectl auth can-i "$verb" secrets --as="$user" -n "$namespace" 2>"$stderr_file")"; then
    status=0
  else
    status=$?
  fi
  case "$status:$output" in
    0:yes|1:no) printf '%s\n' "$output" ;;
    *) return 1 ;;
  esac
}

accelerator_chart_kind_secret_mutation_attribution() {
  [ "$#" -eq 2 ] || return 1
  case "$1:$2" in
    no:no) printf '%s\n' chart-denied ;;
    yes:yes) printf '%s\n' default-grant ;;
    *) return 1 ;;
  esac
}

accelerator_chart_kind_validate_api_server_address() {
  local address="${1-}" octet
  local -a octets
  [ "$#" -eq 1 ] || return 1
  [[ "$address" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || return 1
  IFS=. read -r -a octets <<<"$address"
  [ "${#octets[@]}" -eq 4 ] || return 1
  for octet in "${octets[@]}"; do
    [[ "$octet" == 0 || "$octet" =~ ^[1-9][0-9]{0,2}$ ]] || return 1
    [ "$((10#$octet))" -le 255 ] || return 1
  done
}

accelerator_chart_kind_validate_api_server_host() {
  local host="${1-}" label
  local -a labels
  [ "$#" -eq 1 ] && [ -n "$host" ] && [ "${#host}" -le 253 ] || return 1
  if [[ "$host" =~ ^[0-9.]+$ ]]; then
    accelerator_chart_kind_validate_api_server_address "$host"
    return
  fi
  [[ "$host" =~ ^[A-Za-z0-9.-]+$ ]] || return 1
  IFS=. read -r -a labels <<<"$host"
  [ "${#labels[@]}" -gt 0 ] || return 1
  for label in "${labels[@]}"; do
    [ -n "$label" ] && [ "${#label}" -le 63 ] || return 1
    [[ "$label" =~ ^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$ ]] || return 1
  done
}

accelerator_chart_kind_write_config() {
  local config_file="${1-}" address="${2-}"
  [ "$#" -eq 2 ] && [ -n "$config_file" ] || return 1
  accelerator_chart_kind_validate_api_server_address "$address" || return 1
  printf '%s\n' \
    'kind: Cluster' \
    'apiVersion: kind.x-k8s.io/v1alpha4' \
    'networking:' \
    "  apiServerAddress: \"$address\"" >"$config_file"
}

accelerator_chart_kind_rewrite_kubeconfig() {
  local kubeconfig_file="${1-}" bind_address="${2-}" endpoint_host="${3-}"
  local before current_context context_count cluster_name cluster_count server prefix port endpoint after
  [ "$#" -eq 3 ] && [ -f "$kubeconfig_file" ] || return 1
  accelerator_chart_kind_validate_api_server_address "$bind_address" || return 1
  accelerator_chart_kind_validate_api_server_host "$endpoint_host" || return 1
  before="$(kubectl config view --raw --kubeconfig "$kubeconfig_file" -o json 2>/dev/null)" || return 1
  current_context="$(jq -er '.["current-context"] | select(type == "string" and length > 0)' <<<"$before")" || return 1
  context_count="$(jq -er --arg name "$current_context" '[.contexts[] | select(.name == $name)] | length' <<<"$before")" || return 1
  [ "$context_count" = 1 ] || return 1
  cluster_name="$(jq -er --arg name "$current_context" '.contexts[] | select(.name == $name) | .context.cluster | select(type == "string" and length > 0)' <<<"$before")" || return 1
  cluster_count="$(jq -er --arg name "$cluster_name" '[.clusters[] | select(.name == $name)] | length' <<<"$before")" || return 1
  [ "$cluster_count" = 1 ] || return 1
  server="$(jq -er --arg name "$cluster_name" '.clusters[] | select(.name == $name) | .cluster.server | select(type == "string")' <<<"$before")" || return 1
  prefix="https://$bind_address:"
  [[ "$server" == "$prefix"* ]] || return 1
  port="${server#"$prefix"}"
  [[ "$port" =~ ^[1-9][0-9]{0,4}$ ]] && [ "$((10#$port))" -le 65535 ] || return 1
  endpoint="https://$endpoint_host:$port"
  kubectl config set-cluster "$cluster_name" --kubeconfig "$kubeconfig_file" --server "$endpoint" --tls-server-name localhost >/dev/null 2>&1 || return 1
  after="$(kubectl config view --raw --kubeconfig "$kubeconfig_file" -o json 2>/dev/null)" || return 1
  jq -e --arg name "$cluster_name" --arg endpoint "$endpoint" '([.clusters[] | select(.name == $name)] | length) == 1 and (.clusters[] | select(.name == $name) | .cluster.server) == $endpoint and (.clusters[] | select(.name == $name) | .cluster["tls-server-name"]) == "localhost"' <<<"$after" >/dev/null || return 1
  [ "$(jq -cS --arg name "$cluster_name" '[.clusters[] | select(.name != $name)]' <<<"$before")" = "$(jq -cS --arg name "$cluster_name" '[.clusters[] | select(.name != $name)]' <<<"$after")" ] || return 1
}

if [ "${BASH_SOURCE[0]}" != "$0" ]; then
  return 0
fi

# No NetworkPolicy is rendered: managed API-server and DNS egress endpoints are
# cluster-specific. The chart deliberately creates no Service or public port.
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root/scripts/lib/accelerator-e2e-kind.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/kubikles-accelerator-chart.XXXXXX")"
cluster="kubikles-accelerator-${RANDOM}-${RANDOM}"
namespace="accelerator-smoke-${RANDOM}"
release="accelerator-smoke"
secondary_namespace="accelerator-smoke-other"
reuse_mode="$(accelerator_e2e_reuse_mode)" || { echo 'accelerator-chart-kind: invalid-reuse-boundary' >&2; exit 1; }
kubeconfig="$tmp/kubeconfig"
kind_config="$tmp/kind.yaml"
job="${release}-kubikles-accelerator"
sa="system:serviceaccount:${namespace}:${job}"
pod=""
creator_verifier='w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM'
preload_image=""
port_forward_pid=""
cluster_created=false
cluster_create_attempted=false
cluster_absent_at_start=false
namespace_created=false
secondary_namespace_created=false
release_installed=false
cleanup_failed=false
fail() { echo "accelerator-chart-kind: $1" >&2; exit 1; }
capture_failure_diagnostics() {
  local diagnostic_pod="$pod"
  [ "$cluster_created" = true ] && [ "$namespace_created" = true ] || return 0
  KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get job "$job" -o json 2>&1 | accelerator_chart_kind_redact_stream "$creator_verifier" "${raw_token-}" >"$tmp/failure-job.json" || true
  KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get pods -l job-name="$job" -o json 2>&1 | accelerator_chart_kind_redact_stream "$creator_verifier" "${raw_token-}" >"$tmp/failure-pods.json" || true
  KUBECONFIG="$kubeconfig" kubectl -n "$namespace" describe pods -l job-name="$job" 2>&1 | accelerator_chart_kind_redact_stream "$creator_verifier" "${raw_token-}" >"$tmp/failure-pod-describe" || true
  KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get events --sort-by=.metadata.creationTimestamp 2>&1 | accelerator_chart_kind_redact_stream "$creator_verifier" "${raw_token-}" >"$tmp/failure-events" || true
  docker exec "${cluster}-control-plane" crictl images -o json 2>&1 | accelerator_chart_kind_redact_stream "$creator_verifier" "${raw_token-}" >"$tmp/failure-node-images.json" || true
  if [ -z "$diagnostic_pod" ]; then
    diagnostic_pod="$(KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get pods -l job-name="$job" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  fi
  [ -n "$diagnostic_pod" ] || return 0
  KUBECONFIG="$kubeconfig" kubectl -n "$namespace" logs "$diagnostic_pod" -c accelerator --timestamps 2>&1 | accelerator_chart_kind_redact_stream "$creator_verifier" "${raw_token-}" >"$tmp/failure-current.log" || true
  KUBECONFIG="$kubeconfig" kubectl -n "$namespace" logs "$diagnostic_pod" -c accelerator --previous --timestamps 2>&1 | accelerator_chart_kind_redact_stream "$creator_verifier" "${raw_token-}" >"$tmp/failure-previous.log" || true
}
cleanup() {
  local status=$?
  if [ "$status" -ne 0 ]; then
    capture_failure_diagnostics || true
  fi
  rm -f "$tmp/values.yaml"
  if [ -n "$preload_image" ]; then
    docker image rm "$preload_image" >"$tmp/preload-tag-cleanup" 2>&1 || cleanup_failed=true
  fi
  if [ -n "$port_forward_pid" ] && kill -0 "$port_forward_pid" >/dev/null 2>&1; then
    kill "$port_forward_pid" >/dev/null 2>&1 || cleanup_failed=true
    wait "$port_forward_pid" >/dev/null 2>&1 || true
  fi
  if [ "$release_installed" = true ]; then
    KUBECONFIG="$kubeconfig" helm uninstall "$release" -n "$namespace" >"$tmp/uninstall-cleanup" 2>&1 || cleanup_failed=true
  fi
  if [ "$secondary_namespace_created" = true ]; then
    KUBECONFIG="$kubeconfig" kubectl delete namespace "$secondary_namespace" --wait=true --timeout=45s >"$tmp/secondary-namespace-cleanup" 2>&1 || cleanup_failed=true
    secondary_namespace_created=false
  fi
  if [ "$namespace_created" = true ]; then
    KUBECONFIG="$kubeconfig" kubectl delete namespace "$namespace" --wait=true --timeout=45s >"$tmp/namespace-cleanup" 2>&1 || cleanup_failed=true
    namespace_created=false
  fi
  if { [ "$cluster_created" = true ] || [ "$cluster_create_attempted" = true ]; } && [ "$cluster_absent_at_start" = true ]; then
    if ! kind get clusters >"$tmp/kind-existing" 2>&1; then
      cleanup_failed=true
    elif grep -Fx "$cluster" "$tmp/kind-existing" >/dev/null; then
      kind delete cluster --name "$cluster" --kubeconfig "$kubeconfig" >"$tmp/kind-cleanup" 2>&1 || cleanup_failed=true
    fi
  fi
  if [ "$status" -ne 0 ] || [ "$cleanup_failed" = true ]; then
    echo "accelerator-chart-kind: preserving diagnostics at $tmp" >&2
    [ "$status" -ne 0 ] && exit "$status"
    exit 1
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

for tool in helm kind kubectl docker curl jq; do command -v "$tool" >/dev/null 2>&1 || fail "missing-$tool; install it and rerun make test-accelerator-chart-kind"; done
helm version --short 2>/dev/null | grep -Eq '^v3\.' || fail "Helm 3 is required and must be usable"
docker info >/dev/null 2>&1 || fail "Docker daemon unavailable"
if [ "$reuse_mode" = reuse ]; then
  command -v sha256sum >/dev/null 2>&1 || fail "missing-sha256sum"
  command -v oras >/dev/null 2>&1 || fail "missing-oras"
  accelerator_e2e_validate_reused_fixture || fail "reuse-fixture"
  cluster="$KUBIKLES_ACCELERATOR_E2E_KIND_NAME"
  namespace="$KUBIKLES_ACCELERATOR_E2E_NAMESPACE"
  secondary_namespace="${namespace}-other"
  [ "${#secondary_namespace}" -le 63 ] || fail "reuse-secondary-namespace"
  release="a60a-${namespace##*-}"
  job="${release}-kubikles-accelerator"
  sa="system:serviceaccount:${namespace}:${job}"
  accelerator_e2e_prepare_child_kubeconfig "$kubeconfig" "$namespace" || fail "reuse-kubeconfig"
  cluster_created=true
fi
api_server_address="${ACCELERATOR_KIND_API_SERVER_ADDRESS-127.0.0.1}"
api_server_host="${ACCELERATOR_KIND_API_SERVER_HOST-$api_server_address}"
accelerator_chart_kind_validate_api_server_address "$api_server_address" || fail "ACCELERATOR_KIND_API_SERVER_ADDRESS must be a canonical IPv4 address"
accelerator_chart_kind_validate_api_server_host "$api_server_host" || fail "ACCELERATOR_KIND_API_SERVER_HOST must be a safe IPv4 address or DNS hostname"
accelerator_chart_kind_write_config "$kind_config" "$api_server_address" || fail "kind-config-write"
: "${ACCELERATOR_IMAGE_REPOSITORY:?ACCELERATOR_IMAGE_REPOSITORY must identify the immutable Accelerator image repository}"
: "${ACCELERATOR_IMAGE_DIGEST:?ACCELERATOR_IMAGE_DIGEST must be sha256:<64 lowercase hex>}"
: "${ACCELERATOR_IMAGE_VERSION:?ACCELERATOR_IMAGE_VERSION must equal the Accelerator BuildVersion>}"
[[ "$ACCELERATOR_IMAGE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "ACCELERATOR_IMAGE_DIGEST must be sha256:<64 lowercase hex>"
image="$ACCELERATOR_IMAGE_REPOSITORY@$ACCELERATOR_IMAGE_DIGEST"
if [ "$reuse_mode" = reuse ]; then
  [ "$ACCELERATOR_IMAGE_REPOSITORY" = "$KUBIKLES_ACCELERATOR_E2E_REGISTRY/skrobylabs/kubikles-accelerator" ] || fail "reuse-image-repository"
  oras manifest fetch --plain-http --output "$tmp/image-index.json" "$image" 2>"$tmp/image-index.stderr" || fail "reuse-image-index"
  [ "sha256:$(sha256sum "$tmp/image-index.json" | cut -d ' ' -f 1)" = "$ACCELERATOR_IMAGE_DIGEST" ] || fail "reuse-image-index-digest"
  jq -e --arg digest "$ACCELERATOR_IMAGE_DIGEST" '(.schemaVersion == 2) and ([.manifests[].platform | .os + "/" + .architecture] | sort) == ["linux/amd64","linux/arm64"]' "$tmp/image-index.json" >/dev/null || fail "reuse-image-platforms"
else
  docker image inspect "$image" >"$tmp/image-inspect" 2>&1 || fail "immutable local image $image is unavailable; build/load the completed Accelerator image by digest"
  [ "$(docker image inspect "$image" --format '{{index .Config.Labels "org.opencontainers.image.version"}}')" = "$ACCELERATOR_IMAGE_VERSION" ] || fail "image BuildVersion label mismatch"
  host_image_id="$(docker image inspect "$image" --format '{{.Id}}')"
  host_image_labels="$(docker image inspect "$image" --format '{{json .Config.Labels}}')"
fi

if [ "$reuse_mode" = standalone ]; then
  kind get clusters >"$tmp/kind-before-create" 2>&1 || fail "kind-list-before-create"
  grep -Fx "$cluster" "$tmp/kind-before-create" >/dev/null && fail "generated-cluster-name-already-exists"
  cluster_absent_at_start=true
fi
jq -n --arg repository "$ACCELERATOR_IMAGE_REPOSITORY" --arg digest "$ACCELERATOR_IMAGE_DIGEST" --arg version "$ACCELERATOR_IMAGE_VERSION" --arg verifier "$creator_verifier" '{image:{repository:$repository,digest:$digest,version:$version},accelerator:{workloadSessionId:"smoke-session-1"},auth:{creatorVerifier:$verifier}}' >"$tmp/values.yaml"
if [ "$reuse_mode" = standalone ]; then
  cluster_create_attempted=true
  kind create cluster --name "$cluster" --config "$kind_config" --kubeconfig "$kubeconfig" >"$tmp/kind-create" 2>&1 || fail "kind-create"
  cluster_created=true
  chmod 600 "$kubeconfig"
  accelerator_chart_kind_rewrite_kubeconfig "$kubeconfig" "$api_server_address" "$api_server_host" || fail "kubeconfig-api-server-rewrite"
fi
ready_deadline=$((SECONDS + 90))
ready=false
while [ "$SECONDS" -lt "$ready_deadline" ]; do
  if KUBECONFIG="$kubeconfig" kubectl --request-timeout=2s get --raw=/readyz >"$tmp/readyz" 2>&1 && grep -Fx ok "$tmp/readyz" >/dev/null; then
    ready=true
    break
  fi
  sleep 1
done
[ "$ready" = true ] || fail "api-server-readyz"
if [ "$reuse_mode" = standalone ]; then
  # Kind's Docker-archive loader does not preserve a digest-qualified localhost
  # repository name. Load the same image through a unique test-only tag, then add
  # the admitted digest reference to the node's containerd image store.
  preload_image="${ACCELERATOR_IMAGE_REPOSITORY}:kind-preload-${cluster#kubikles-accelerator-}"
  docker tag "$image" "$preload_image" || fail "kind-preload-tag"
  kind load docker-image "$preload_image" --name "$cluster" >"$tmp/kind-load" 2>&1 || fail "kind-load-image"
  docker exec "${cluster}-control-plane" ctr -n k8s.io images tag "$preload_image" "$image" >"$tmp/kind-image-alias" 2>&1 || fail "kind-digest-image-alias"
  docker exec "${cluster}-control-plane" crictl inspecti "$image" >"$tmp/node-image-inspect.json" 2>"$tmp/node-image-inspect.stderr" || fail "kind-digest-image-inspect"
  jq -e --arg id "$host_image_id" --argjson labels "$host_image_labels" '(.status.id == $id) and (.info.imageSpec.config.Labels == $labels)' "$tmp/node-image-inspect.json" >/dev/null || fail "kind-image-identity-or-labels"
  docker image rm "$preload_image" >"$tmp/preload-tag-remove" 2>&1 || fail "kind-preload-tag-remove"
  preload_image=""
fi
KUBECONFIG="$kubeconfig" kubectl create namespace "$namespace" >"$tmp/namespace" 2>&1 || fail "namespace-create"
namespace_created=true
KUBECONFIG="$kubeconfig" kubectl create namespace "$secondary_namespace" >"$tmp/secondary-namespace" 2>&1 || fail "secondary-namespace-create"
secondary_namespace_created=true
for ns in "$namespace" "$secondary_namespace"; do KUBECONFIG="$kubeconfig" kubectl -n "$ns" create secret generic admin-owned --from-literal=value=redacted >"$tmp/secret" 2>&1 || fail "admin-secret-create"; done
for verb in create update patch delete deletecollection; do accelerator_chart_kind_can_i_result "$kubeconfig" "$sa" "$namespace" "$verb" "$tmp/default-secret-$verb.stderr" >"$tmp/default-secret-$verb" || fail "default-secret-$verb-authorization-review"; done
KUBECONFIG="$kubeconfig" helm upgrade --install "$release" "$root/deploy/charts/kubikles-accelerator" -n "$namespace" -f "$tmp/values.yaml" --wait >"$tmp/install" 2>&1 || fail "helm-install"
release_installed=true
rm -f "$tmp/values.yaml"
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get job "$job" -o json >"$tmp/live-job.json" || fail "read-live-job-json"
jq -e '(.spec.completions == 1) and (.spec.parallelism == 1) and (.spec.backoffLimit == 0) and (.spec.ttlSecondsAfterFinished == 3600) and ((.spec | has("activeDeadlineSeconds")) | not)' "$tmp/live-job.json" >/dev/null || fail "live-job-lifecycle-is-not-exact"
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get serviceaccount "$job" -o json >"$tmp/serviceaccount.json" || fail "read-serviceaccount-json"
jq -e '.automountServiceAccountToken == false and ((.secrets // []) | length == 0)' "$tmp/serviceaccount.json" >/dev/null || fail "serviceaccount-token-automount-or-legacy-secret"
for ns in "$namespace" "$secondary_namespace"; do for verb in get list watch; do result="$(accelerator_chart_kind_can_i_result "$kubeconfig" "$sa" "$ns" "$verb" "$tmp/secret-$verb-$ns.stderr")" || fail "secret-$verb-$ns-authorization-review"; [ "$result" = yes ] || fail "missing-secret-$verb-$ns"; done; done
for verb in create update patch delete deletecollection; do
  result="$(accelerator_chart_kind_can_i_result "$kubeconfig" "$sa" "$namespace" "$verb" "$tmp/chart-secret-$verb.stderr")" || fail "chart-secret-$verb-authorization-review"
  default_result="$(cat "$tmp/default-secret-$verb")"
  attribution="$(accelerator_chart_kind_secret_mutation_attribution "$default_result" "$result")" || fail "chart-expanded-secret-$verb"
  if [ "$attribution" = default-grant ]; then
    printf 'accelerator-chart-kind: default Kubernetes RBAC grants %s secrets; chart ownership is verified from its exact live role and binding\n' "$verb" | tee -a "$tmp/default-rbac-grants" >&2
  fi
done
role="$(KUBECONFIG="$kubeconfig" kubectl get clusterrole -l "app.kubernetes.io/instance=$release" -o jsonpath='{.items[0].metadata.name}')"
KUBECONFIG="$kubeconfig" kubectl get clusterrole "$role" -o jsonpath='{.rules}' >"$tmp/rules" || fail "read-role"
KUBECONFIG="$kubeconfig" kubectl get clusterrole "$role" -o json >"$tmp/role.json" || fail "read-role-json"
jq -e '.rules == [{apiGroups:[""],resources:["secrets"],verbs:["get","list","watch"]}]' "$tmp/role.json" >/dev/null || fail "role-is-not-exact-secret-read-rbac"
binding="$(KUBECONFIG="$kubeconfig" kubectl get clusterrolebinding -l "app.kubernetes.io/instance=$release" -o jsonpath='{.items[0].metadata.name}')"
[ -n "$binding" ] || fail "missing-clusterrolebinding"
KUBECONFIG="$kubeconfig" kubectl get clusterrolebinding "$binding" -o json >"$tmp/binding.json" || fail "read-binding-json"
jq -e --arg role "$role" --arg namespace "$namespace" --arg job "$job" '.roleRef == {apiGroup:"rbac.authorization.k8s.io",kind:"ClusterRole",name:$role} and .subjects == [{kind:"ServiceAccount",name:$job,namespace:$namespace}]' "$tmp/binding.json" >/dev/null || fail "binding-is-not-exact-serviceaccount-role"
pod_deadline=$((SECONDS + 30))
while [ "$SECONDS" -lt "$pod_deadline" ]; do
  if ! pod="$(KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get pod -l job-name="$job" -o jsonpath='{.items[0].metadata.name}' 2>"$tmp/pod-query")"; then
    pod=""
  fi
  [ -n "$pod" ] && break
  sleep 1
done
[ -n "$pod" ] || fail "missing-job-pod"
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" wait --for=condition=Ready "pod/$pod" --timeout=90s >"$tmp/pod-ready" 2>&1 || fail "pod-start"
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get pod "$pod" -o json >"$tmp/pod.json" || fail "read-pod-json"
jq -e --arg job "$job" '(.spec.restartPolicy == "Never") and (.spec.automountServiceAccountToken == false) and (.spec.serviceAccountName == $job) and (.spec.securityContext == {runAsNonRoot:true,runAsUser:65532,runAsGroup:65532,seccompProfile:{type:"RuntimeDefault"}}) and (.spec.initContainers | not) and (.spec.containers | length == 1) and (.spec.volumes | length == 1)' "$tmp/pod.json" >/dev/null || fail "pod-hardening-baseline"
# Preserve the admitted object for the canonical API-defaulted check while reusing the render-shape assertion for every other fixed field.
mv "$tmp/pod.json" "$tmp/admitted-pod.json"
jq 'del(.spec.volumes[0].projected.sources[2].downwardAPI.items[0].fieldRef.apiVersion)' "$tmp/admitted-pod.json" >"$tmp/pod.json" || fail "normalize-admitted-pod-for-static-field-checks"
jq -e --arg image "$image" --arg verifier "${job}-verifier" '(.spec.containers[0].name == "accelerator") and (.spec.containers[0].image == $image) and (.spec.containers[0].imagePullPolicy == "IfNotPresent") and (.spec.containers[0].securityContext == {allowPrivilegeEscalation:false,readOnlyRootFilesystem:true,capabilities:{drop:["ALL"]}}) and (.spec.containers[0].resources == {requests:{cpu:"100m",memory:"128Mi"},limits:{cpu:"1",memory:"512Mi"}}) and (.spec.containers[0].env == [{name:"KUBIKLES_ACCELERATOR_CREATOR_VERIFIER",valueFrom:{secretKeyRef:{name:$verifier,key:"creatorVerifier"}}}]) and (.spec.containers[0].volumeMounts == [{name:"serviceaccount",mountPath:"/var/run/secrets/kubernetes.io/serviceaccount",readOnly:true}]) and (.spec.volumes == [{name:"serviceaccount",projected:{defaultMode:292,sources:[{serviceAccountToken:{path:"token",expirationSeconds:3600}},{configMap:{name:"kube-root-ca.crt",items:[{key:"ca.crt",path:"ca.crt"}]}},{downwardAPI:{items:[{path:"namespace",fieldRef:{fieldPath:"metadata.namespace"}}]}}]}}])' "$tmp/pod.json" >/dev/null || fail "pod-container-identity-projection-or-credentials"
accelerator_chart_kind_pod_projection_is_exact "$tmp/admitted-pod.json" "$image" "${job}-verifier" || fail "admitted-pod-projection-is-not-canonical"
mv "$tmp/admitted-pod.json" "$tmp/pod.json"
# The raw token remains local, is never passed to Helm/Kubernetes, and is sent as an HTTP header from stdin.
raw_token='AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8'
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" port-forward "pod/$pod" :8080 >"$tmp/port-forward" 2>&1 & port_forward_pid=$!
for _ in $(seq 1 30); do
  kill -0 "$port_forward_pid" >/dev/null 2>&1 || fail "port-forward-exited"
  port="$(sed -nE 's/^Forwarding from 127\.0\.0\.1:([0-9]+) -> 8080$/\1/p' "$tmp/port-forward" | head -n 1)"
  [ -n "${port:-}" ] && break
  sleep 1
done
[ -n "${port:-}" ] || fail "port-forward-did-not-bind"
endpoint="http://127.0.0.1:$port/api/accelerator-info"
status="$(curl --silent --show-error --connect-timeout 2 --max-time 5 -o "$tmp/missing" -w '%{http_code}' "$endpoint" 2>"$tmp/curl-missing" || true)"
[ "$status" = 401 ] || fail "missing-auth-status-$status"
status="$(curl --silent --show-error --connect-timeout 2 --max-time 5 -H 'Authorization: Bearer wrong' -o "$tmp/wrong" -w '%{http_code}' "$endpoint" 2>"$tmp/curl-wrong" || true)"
[ "$status" = 401 ] || fail "wrong-auth-status-$status"
status="$(printf '%s\n' "Authorization: Bearer $raw_token" | curl --silent --show-error --connect-timeout 2 --max-time 5 -H @- -o "$tmp/info" -w '%{http_code}' "$endpoint" 2>"$tmp/curl-valid" || true)"
[ "$status" = 200 ] || fail "valid-auth-status-$status"
jq -e --arg version "$ACCELERATOR_IMAGE_VERSION" 'keys == ["build","capabilities","capabilityDiagnostics","instanceId","runtime"] and .runtime == "accelerator" and (.build | keys == ["buildVersion","commit","dirty"] and .buildVersion == $version and (.commit | type == "string") and (.dirty | type == "boolean")) and (.instanceId | type == "string" and length > 0) and .capabilities == ["secrets.list","secrets.detail","secrets.watch"] and (.capabilityDiagnostics | type == "array")' "$tmp/info" >/dev/null || fail "accelerator-info-contract"
kill "$port_forward_pid" >/dev/null 2>&1 || fail "port-forward-stop"; wait "$port_forward_pid" >/dev/null 2>&1 || true; port_forward_pid=""
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" wait --for=condition=complete "job/$job" --timeout=150s >"$tmp/job-complete" 2>&1 || fail "natural-job-completion"
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get job "$job" -o json >"$tmp/job.json" || fail "read-completed-job"
jq -e '.status.succeeded == 1 and .status.failed != 1' "$tmp/job.json" >/dev/null || fail "job-did-not-succeed"
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get pod "$pod" -o json >"$tmp/completed-pod.json" || fail "read-completed-pod"
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" logs "$pod" -c accelerator --timestamps >"$tmp/accelerator.log" 2>"$tmp/log-read" || fail "read-accelerator-log"
jq -e '(.status.containerStatuses | length == 1) and (.status.containerStatuses[0].name == "accelerator") and (.status.containerStatuses[0].state.terminated.exitCode == 0) and (.status.containerStatuses[0].restartCount == 0) and (.status.containerStatuses[0].state.terminated.finishedAt | type == "string")' "$tmp/completed-pod.json" >/dev/null || fail "pod-did-not-exit-cleanly"
finished_time="$(jq -r '.status.containerStatuses[0].state.terminated.finishedAt' "$tmp/completed-pod.json")"
finished_seconds="$(accelerator_chart_kind_utc_seconds "$finished_time")" || fail "malformed-container-finished-time"
listener_started_seconds="$(accelerator_chart_kind_listener_started_seconds "$tmp/accelerator.log")" || fail "missing-malformed-or-ambiguous-listener-log-record"
elapsed_from_listener="$(( finished_seconds - listener_started_seconds ))"
[ "$elapsed_from_listener" -ge 115 ] && [ "$elapsed_from_listener" -le 125 ] || fail "natural-exit-after-listener-ready-not-consistent-with-exact-120-second-grace"
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" patch job "$job" --type merge -p '{"spec":{"ttlSecondsAfterFinished":1}}' >"$tmp/ttl-patch" 2>&1 || fail "ttl-patch"
job_gone=false; pod_gone=false
for _ in $(seq 1 45); do
  KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get job "$job" >"$tmp/job-get" 2>&1 && job_gone=false || { grep -F '(NotFound)' "$tmp/job-get" >/dev/null && job_gone=true || fail "ttl-job-query"; }
  KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get pod "$pod" >"$tmp/pod-get" 2>&1 && pod_gone=false || { grep -F '(NotFound)' "$tmp/pod-get" >/dev/null && pod_gone=true || fail "ttl-pod-query"; }
  [ "$job_gone" = true ] && [ "$pod_gone" = true ] && break
  sleep 1
done
[ "$job_gone" = true ] && [ "$pod_gone" = true ] || fail "ttl-did-not-delete-job-and-pod"
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get secret "${job}-verifier" >/dev/null || fail "TTL removed verifier"
KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get serviceaccount "$job" >/dev/null || fail "TTL removed serviceaccount"
KUBECONFIG="$kubeconfig" kubectl get clusterrole "$role" >/dev/null || fail "TTL removed clusterrole"
KUBECONFIG="$kubeconfig" kubectl get clusterrolebinding "$binding" >/dev/null || fail "TTL removed clusterrolebinding"
KUBECONFIG="$kubeconfig" helm uninstall "$release" -n "$namespace" >"$tmp/uninstall" 2>&1 || fail "helm-uninstall"; release=""
release_installed=false
for resource in "secret/${job}-verifier" "serviceaccount/${job}"; do KUBECONFIG="$kubeconfig" kubectl -n "$namespace" get "$resource" >"$tmp/residual" 2>&1 && fail "residual-$resource" || grep -F '(NotFound)' "$tmp/residual" >/dev/null || fail "residual-query-$resource"; done
for resource in "clusterrole/$role" "clusterrolebinding/$binding"; do KUBECONFIG="$kubeconfig" kubectl get "$resource" >"$tmp/residual" 2>&1 && fail "residual-$resource" || grep -F '(NotFound)' "$tmp/residual" >/dev/null || fail "residual-query-$resource"; done
echo "accelerator-chart-kind: passed" >&2
