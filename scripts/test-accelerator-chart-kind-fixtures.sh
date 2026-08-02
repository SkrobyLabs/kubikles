#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=test-accelerator-chart-kind.sh
source "$root/scripts/test-accelerator-chart-kind.sh"

tmp="$(mktemp -d "${TMPDIR:-/tmp}/kubikles-accelerator-chart-fixtures.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

printf '%s\n' \
  'verifier=w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM' \
  'creator=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8' \
  'serviceaccount=eyJhbGciOiJSUzI1NiIsImtpZCI6ImZpeHR1cmUifQ.eyJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6Zml4dHVyZSJ9.fixture_signature_value' |
  accelerator_chart_kind_redact_stream 'w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM' 'AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8' >"$tmp/redacted"
printf '%s\n' \
  'verifier=[REDACTED_VERIFIER]' \
  'creator=[REDACTED_TOKEN]' \
  'serviceaccount=[REDACTED_TOKEN]' >"$tmp/expected-redacted"
cmp -s "$tmp/expected-redacted" "$tmp/redacted" || {
  echo "accelerator-chart-kind-fixtures: diagnostic credential redaction mismatch" >&2
  exit 1
}

accelerator_chart_kind_write_config "$tmp/kind.yaml" '0.0.0.0'
printf '%s\n' \
  'kind: Cluster' \
  'apiVersion: kind.x-k8s.io/v1alpha4' \
  'networking:' \
  '  apiServerAddress: "0.0.0.0"' >"$tmp/expected-kind.yaml"
cmp -s "$tmp/expected-kind.yaml" "$tmp/kind.yaml" || {
  echo "accelerator-chart-kind-fixtures: Kind config is not exact" >&2
  exit 1
}

printf '%s\n' \
  'apiVersion: v1' \
  'kind: Config' \
  'current-context: kind-smoke' \
  'clusters:' \
  '- name: kind-smoke' \
  '  cluster:' \
  '    server: https://0.0.0.0:41001' \
  '- name: untouched' \
  '  cluster:' \
  '    server: https://192.0.2.20:6443' \
  'contexts:' \
  '- name: kind-smoke' \
  '  context:' \
  '    cluster: kind-smoke' \
  '    user: kind-smoke' \
  'users:' \
  '- name: kind-smoke' \
  '  user: {}' >"$tmp/kubeconfig"
cp "$tmp/kubeconfig" "$tmp/expected-kubeconfig"
accelerator_chart_kind_rewrite_kubeconfig "$tmp/kubeconfig" '0.0.0.0' 'host.docker.internal'
kubectl config view --raw --kubeconfig "$tmp/kubeconfig" -o json >"$tmp/rewritten.json"
jq -e '(.clusters[] | select(.name == "kind-smoke").cluster) == {server:"https://host.docker.internal:41001","tls-server-name":"localhost"} and (.clusters[] | select(.name == "untouched").cluster) == {server:"https://192.0.2.20:6443"}' "$tmp/rewritten.json" >/dev/null || {
  echo "accelerator-chart-kind-fixtures: kubeconfig endpoint rewrite is not exact" >&2
  exit 1
}

for fixture in wrong-bind malformed-port; do
  server='https://127.0.0.1:41001'
  [ "$fixture" = malformed-port ] && server='https://0.0.0.0:not-a-port'
  sed "s#https://0.0.0.0:41001#$server#" "$tmp/expected-kubeconfig" >"$tmp/$fixture-kubeconfig"
  cp "$tmp/$fixture-kubeconfig" "$tmp/$fixture-before"
  if accelerator_chart_kind_rewrite_kubeconfig "$tmp/$fixture-kubeconfig" '0.0.0.0' 'host.docker.internal' >/dev/null 2>&1; then
    echo "accelerator-chart-kind-fixtures: $fixture endpoint accepted" >&2
    exit 1
  fi
  cmp -s "$tmp/$fixture-before" "$tmp/$fixture-kubeconfig" || {
    echo "accelerator-chart-kind-fixtures: $fixture endpoint mutated on rejection" >&2
    exit 1
  }
done

for address in '' '127.0.0.1:6443' 'https://127.0.0.1' '999.0.0.1' $'127.0.0.1\nnetworking: {}'; do
  if accelerator_chart_kind_validate_api_server_address "$address"; then
    echo "accelerator-chart-kind-fixtures: unsafe bind address accepted" >&2
    exit 1
  fi
done
for host in '' '-host.docker.internal' 'host.docker.internal:6443' 'https://host.docker.internal' 'host.docker.internal/path' $'host.docker.internal\ntls-server-name: unsafe'; do
  if accelerator_chart_kind_validate_api_server_host "$host"; then
    echo "accelerator-chart-kind-fixtures: unsafe endpoint host accepted" >&2
    exit 1
  fi
done
accelerator_chart_kind_validate_api_server_address '127.0.0.1'
accelerator_chart_kind_validate_api_server_address '0.0.0.0'
accelerator_chart_kind_validate_api_server_host '127.0.0.1'
accelerator_chart_kind_validate_api_server_host 'host.docker.internal'

mkdir "$tmp/bin"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'printf "%s\\n" "$FIXTURE_KUBECTL_OUTPUT"' \
  'exit "$FIXTURE_KUBECTL_STATUS"' >"$tmp/bin/kubectl"
chmod 700 "$tmp/bin/kubectl"
real_path="$PATH"
PATH="$tmp/bin:$PATH"
FIXTURE_KUBECTL_OUTPUT=yes FIXTURE_KUBECTL_STATUS=0 accelerator_chart_kind_can_i_result fixture-kubeconfig fixture-user fixture-namespace get "$tmp/can-i.err" >"$tmp/can-i.out"
[ "$(cat "$tmp/can-i.out")" = yes ] || {
  echo "accelerator-chart-kind-fixtures: allowed authorization output not preserved" >&2
  exit 1
}
FIXTURE_KUBECTL_OUTPUT=no FIXTURE_KUBECTL_STATUS=1 accelerator_chart_kind_can_i_result fixture-kubeconfig fixture-user fixture-namespace create "$tmp/can-i.err" >"$tmp/can-i.out"
[ "$(cat "$tmp/can-i.out")" = no ] || {
  echo "accelerator-chart-kind-fixtures: denied authorization output not preserved" >&2
  exit 1
}
for fixture in 'yes 1' 'no 0' 'error 1'; do
  read -r output status <<<"$fixture"
  if FIXTURE_KUBECTL_OUTPUT="$output" FIXTURE_KUBECTL_STATUS="$status" accelerator_chart_kind_can_i_result fixture-kubeconfig fixture-user fixture-namespace create "$tmp/can-i.err" >"$tmp/can-i.out"; then
    echo "accelerator-chart-kind-fixtures: inconsistent authorization result accepted" >&2
    exit 1
  fi
done
PATH="$real_path"
[ "$(accelerator_chart_kind_secret_mutation_attribution no no)" = chart-denied ] || {
  echo "accelerator-chart-kind-fixtures: chart denial attribution mismatch" >&2
  exit 1
}
[ "$(accelerator_chart_kind_secret_mutation_attribution yes yes)" = default-grant ] || {
  echo "accelerator-chart-kind-fixtures: default grant attribution mismatch" >&2
  exit 1
}
for fixture in 'no yes' 'yes no' 'invalid no'; do
  read -r before after <<<"$fixture"
  if accelerator_chart_kind_secret_mutation_attribution "$before" "$after" >/dev/null 2>&1; then
    echo "accelerator-chart-kind-fixtures: inconsistent authorization attribution accepted" >&2
    exit 1
  fi
done

image='example.invalid/kubikles-accelerator@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'
verifier='accelerator-smoke-kubikles-accelerator-verifier'
pod_prefix='{"spec":{"containers":[{"name":"accelerator","image":"'"$image"'","imagePullPolicy":"IfNotPresent","securityContext":{"allowPrivilegeEscalation":false,"readOnlyRootFilesystem":true,"capabilities":{"drop":["ALL"]}},"resources":{"requests":{"cpu":"100m","memory":"128Mi"},"limits":{"cpu":"1","memory":"512Mi"}},"env":[{"name":"KUBIKLES_ACCELERATOR_CREATOR_VERIFIER","valueFrom":{"secretKeyRef":{"name":"'"$verifier"'","key":"creatorVerifier"}}}],"volumeMounts":[{"name":"serviceaccount","mountPath":"/var/run/secrets/kubernetes.io/serviceaccount","readOnly":true}]}],"volumes":[{"name":"serviceaccount","projected":{"defaultMode":292,"sources":[{"serviceAccountToken":{"path":"token","expirationSeconds":3600}},{"configMap":{"name":"kube-root-ca.crt","items":[{"key":"ca.crt","path":"ca.crt"}]}},{"downwardAPI":{"items":[{"path":"namespace","fieldRef":{'
pod_suffix='"fieldPath":"metadata.namespace"}}]}}]}}]}}'

printf '%s%s%s\n' "$pod_prefix" '"apiVersion":"v1",' "$pod_suffix" >"$tmp/admitted-pod.json"
accelerator_chart_kind_pod_projection_is_exact "$tmp/admitted-pod.json" "$image" "$verifier"

printf '%s%s\n' "$pod_prefix" "$pod_suffix" >"$tmp/pre-admission-pod.json"
if accelerator_chart_kind_pod_projection_is_exact "$tmp/pre-admission-pod.json" "$image" "$verifier"; then
  echo "accelerator-chart-kind-fixtures: projection accepted missing defaulted apiVersion" >&2
  exit 1
fi

printf '%s\n' '2026-08-02T00:00:00.123456789Z 2026/08/02 00:00:00 Server mode: listening on http://127.0.0.1:8080' >"$tmp/listener.log"
listener_seconds="$(accelerator_chart_kind_listener_started_seconds "$tmp/listener.log")"
[ "$listener_seconds" = 1785628800 ] || {
  echo "accelerator-chart-kind-fixtures: listener timestamp parse mismatch" >&2
  exit 1
}
finished_seconds="$(accelerator_chart_kind_utc_seconds '2026-08-02T00:02:00Z')"
[ "$(( finished_seconds - listener_seconds ))" = 120 ] || {
  echo "accelerator-chart-kind-fixtures: listener-to-completion interval mismatch" >&2
  exit 1
}

if accelerator_chart_kind_listener_started_seconds /dev/null >/dev/null 2>&1; then
  echo "accelerator-chart-kind-fixtures: missing listener record accepted" >&2
  exit 1
fi

printf '%s\n' \
  '2026-08-02T00:00:00Z 2026/08/02 00:00:00 Server mode: listening on http://127.0.0.1:8080' \
  '2026-08-02T00:00:01Z 2026/08/02 00:00:01 Server mode: listening on http://127.0.0.1:8080' >"$tmp/multiple.log"
if accelerator_chart_kind_listener_started_seconds "$tmp/multiple.log" >/dev/null 2>&1; then
  echo "accelerator-chart-kind-fixtures: ambiguous listener record accepted" >&2
  exit 1
fi

printf '%s\n' 'not-a-timestamp 2026/08/02 00:00:00 Server mode: listening on http://127.0.0.1:8080' >"$tmp/malformed.log"
if accelerator_chart_kind_listener_started_seconds "$tmp/malformed.log" >/dev/null 2>&1; then
  echo "accelerator-chart-kind-fixtures: malformed listener timestamp accepted" >&2
  exit 1
fi

[ "$finished_seconds" = 1785628920 ] || {
  echo "accelerator-chart-kind-fixtures: completion timestamp parse mismatch" >&2
  exit 1
}

echo "accelerator-chart-kind-fixtures: passed" >&2
