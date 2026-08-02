package chart_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

const (
	validVerifier = "w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM"
	validDigest   = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func chartDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal("resolve chart directory")
	}
	return dir
}

func render(t *testing.T, release, namespace string, values string) []byte {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Fatal("Helm 3 is required for accelerator chart tests; install Helm 3 and rerun make test-accelerator-chart")
	}
	tmp := t.TempDir()
	file := filepath.Join(tmp, "values.yaml")
	if err := os.WriteFile(file, []byte(values), 0600); err != nil {
		t.Fatal("write test values")
	}
	cmd := exec.Command("helm", "template", release, chartDir(t), "--namespace", namespace, "--values", file)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal("helm template failed for valid redacted test values")
	}
	return out
}

func lint(t *testing.T, values string, wantOK bool) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Fatal("Helm 3 is required for accelerator chart tests; install Helm 3 and rerun make test-accelerator-chart")
	}
	tmp := t.TempDir()
	file := filepath.Join(tmp, "values.yaml")
	if err := os.WriteFile(file, []byte(values), 0600); err != nil {
		t.Fatal("write test values")
	}
	err := exec.Command("helm", "lint", "--strict", chartDir(t), "--values", file).Run()
	if (err == nil) != wantOK {
		t.Fatal("unexpected helm lint status")
	}
}

func values(extra string) string {
	return "image:\n  repository: example.invalid/kubikles-accelerator\n  digest: " + validDigest + "\n  version: v1.2.3\naccelerator:\n  workloadSessionId: desktop-session-1\nauth:\n  creatorVerifier: " + validVerifier + "\n" + extra
}

func objects(t *testing.T, manifest []byte) []map[string]any {
	t.Helper()
	var result []map[string]any
	for _, doc := range bytes.Split(manifest, []byte("\n---")) {
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}
		var object map[string]any
		if err := yaml.Unmarshal(doc, &object); err != nil {
			t.Fatal("decode rendered object")
		}
		result = append(result, object)
	}
	return result
}

func find(t *testing.T, objects []map[string]any, kind string) map[string]any {
	t.Helper()
	for _, object := range objects {
		if object["kind"] == kind {
			return object
		}
	}
	t.Fatalf("missing %s", kind)
	return nil
}
func nested(object map[string]any, keys ...string) any {
	var value any = object
	for _, key := range keys {
		m, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = m[key]
	}
	return value
}

func TestValuesSchemaRejectsInvalidInputs(t *testing.T) {
	lint(t, values(""), true)
	for _, repository := range []string{
		"registry.example:1/team/kubikles-accelerator",
		"registry.example:65535/team/kubikles-accelerator",
		"localhost:5000/kubikles-accelerator",
		"library/kubikles-accelerator",
		strings.Repeat("a", 63) + ".example/team/kubikles-accelerator",
	} {
		lint(t, strings.Replace(values(""), "example.invalid/kubikles-accelerator", repository, 1), true)
	}
	for _, invalid := range []string{
		"",
		strings.Replace(values(""), "image:\n  repository: example.invalid/kubikles-accelerator\n  digest: "+validDigest+"\n  version: v1.2.3\n", "", 1),
		strings.Replace(values(""), "accelerator:\n  workloadSessionId: desktop-session-1\n", "", 1),
		strings.Replace(values(""), "auth:\n  creatorVerifier: "+validVerifier+"\n", "", 1),
		strings.Replace(values(""), "  repository: example.invalid/kubikles-accelerator\n", "", 1),
		strings.Replace(values(""), "  digest: "+validDigest+"\n", "", 1),
		strings.Replace(values(""), "  version: v1.2.3\n", "", 1),
		strings.Replace(values(""), "  workloadSessionId: desktop-session-1\n", "", 1),
		strings.Replace(values(""), "  creatorVerifier: "+validVerifier+"\n", "", 1),
		values("token: raw-token\n"),
		values("credentials: {}\n"),
		values("rbac: {}\n"),
		values("lifecycle: {}\n"),
		values("security: {}\n"),
		values("resources: {}\n"),
		values("extraEnv: []\n"),
		values("sidecars: []\n"),
		strings.Replace(values(""), "  version: v1.2.3\n", "  version: v1.2.3\n  tag: latest\n", 1),
		strings.Replace(values(""), "  version: v1.2.3\n", "  version: v1.2.3\n  pullPolicy: Always\n", 1),
		strings.Replace(values(""), "  workloadSessionId: desktop-session-1\n", "  workloadSessionId: desktop-session-1\n  ttlSecondsAfterFinished: 1\n", 1),
		strings.Replace(values(""), "  workloadSessionId: desktop-session-1\n", "  workloadSessionId: desktop-session-1\n  serviceAccountName: existing\n", 1),
		strings.Replace(values(""), "  workloadSessionId: desktop-session-1\n", "  workloadSessionId: desktop-session-1\n  privileged: true\n", 1),
		strings.Replace(values(""), "  creatorVerifier: "+validVerifier+"\n", "  creatorVerifier: "+validVerifier+"\n  rawToken: raw-token\n", 1),
		strings.Replace(values(""), "  creatorVerifier: "+validVerifier+"\n", "  creatorVerifier: "+validVerifier+"\n  existingSecret: external\n", 1),
		strings.Replace(values(""), "creatorVerifier: "+validVerifier, "creatorVerifier: invalid", 1),
		strings.Replace(values(""), "creatorVerifier: "+validVerifier, "creatorVerifier: "+validVerifier[:42]+"B", 1), // non-canonical base64url tail
		strings.Replace(values(""), "creatorVerifier: "+validVerifier, "creatorVerifier: "+validVerifier+"=", 1),
		strings.Replace(values(""), "repository: example.invalid/kubikles-accelerator", "repository: https://example.invalid/kubikles-accelerator", 1),
		strings.Replace(values(""), "repository: example.invalid/kubikles-accelerator", "repository: example.invalid/kubikles-accelerator:latest", 1),
		strings.Replace(values(""), "repository: example.invalid/kubikles-accelerator", "repository: example.invalid/kubikles-accelerator@sha256:deadbeef", 1),
		strings.Replace(values(""), "repository: example.invalid/kubikles-accelerator", "repository: example.invalid/bad repo", 1),
		strings.Replace(values(""), "repository: example.invalid/kubikles-accelerator", "repository: registry..invalid/kubikles-accelerator", 1),
		strings.Replace(values(""), "repository: example.invalid/kubikles-accelerator", "repository: registry.-invalid/kubikles-accelerator", 1),
		strings.Replace(values(""), "repository: example.invalid/kubikles-accelerator", "repository: registry-.invalid/kubikles-accelerator", 1),
		strings.Replace(values(""), "repository: example.invalid/kubikles-accelerator", "repository: "+strings.Repeat("a", 64)+".example/team/kubikles-accelerator", 1),
		strings.Replace(values(""), "repository: example.invalid/kubikles-accelerator", "repository: registry.example:0/team/kubikles-accelerator", 1),
		strings.Replace(values(""), "repository: example.invalid/kubikles-accelerator", "repository: registry.example:65536/team/kubikles-accelerator", 1),
		strings.Replace(values(""), "digest: "+validDigest, "digest: sha256:"+strings.Repeat("A", 64), 1),
		strings.Replace(values(""), "version: v1.2.3", "version: "+strings.Repeat("x", 129), 1),
		strings.Replace(values(""), "version: v1.2.3", "version: ''", 1),
		strings.Replace(values(""), "workloadSessionId: desktop-session-1", "workloadSessionId: ''", 1),
		strings.Replace(values(""), "workloadSessionId: desktop-session-1", "workloadSessionId: "+strings.Repeat("x", 64), 1),
		strings.Replace(values(""), "workloadSessionId: desktop-session-1", "workloadSessionId: bad/session", 1),
		"image: []\naccelerator:\n  workloadSessionId: desktop-session-1\nauth:\n  creatorVerifier: " + validVerifier + "\n",
		"image:\n  repository: example.invalid/kubikles-accelerator\n  digest: " + validDigest + "\n  version: v1.2.3\naccelerator: false\nauth:\n  creatorVerifier: " + validVerifier + "\n",
		"image:\n  repository: example.invalid/kubikles-accelerator\n  digest: " + validDigest + "\n  version: v1.2.3\naccelerator:\n  workloadSessionId: desktop-session-1\nauth: []\n",
		strings.Replace(values(""), "repository: example.invalid/kubikles-accelerator", "repository: false", 1),
		strings.Replace(values(""), "digest: "+validDigest, "digest: 1", 1),
		strings.Replace(values(""), "version: v1.2.3", "version: []", 1),
		strings.Replace(values(""), "workloadSessionId: desktop-session-1", "workloadSessionId: 1", 1),
		strings.Replace(values(""), "creatorVerifier: "+validVerifier, "creatorVerifier: 1", 1),
		strings.Replace(values(""), "creatorVerifier: "+validVerifier, "creatorVerifier:\n  rawToken: ignored", 1),
	} {
		lint(t, invalid, false)
	}
}

func TestRenderHasExactObjectSetAndJobLifecycle(t *testing.T) {
	first := render(t, "release", "test-ns", values(""))
	second := render(t, "release", "test-ns", values(""))
	if !bytes.Equal(first, second) {
		t.Fatal("render is not deterministic")
	}
	got := objects(t, first)
	if len(got) != 5 {
		t.Fatalf("object count = %d, want 5", len(got))
	}
	want := map[string]string{"Job": "batch/v1", "Secret": "v1", "ServiceAccount": "v1", "ClusterRole": "rbac.authorization.k8s.io/v1", "ClusterRoleBinding": "rbac.authorization.k8s.io/v1"}
	for _, object := range got {
		kind, api := object["kind"].(string), object["apiVersion"].(string)
		if want[kind] != api {
			t.Fatalf("unexpected object %s", kind)
		}
		delete(want, kind)
	}
	if len(want) != 0 {
		t.Fatal("missing required objects")
	}
	job := find(t, got, "Job")
	spec := nested(job, "spec").(map[string]any)
	for key, wantValue := range map[string]float64{"completions": 1, "parallelism": 1, "backoffLimit": 0, "ttlSecondsAfterFinished": 3600} {
		if spec[key] != wantValue {
			t.Fatalf("job %s is not fixed", key)
		}
	}
	pod := nested(job, "spec", "template", "spec").(map[string]any)
	if pod["serviceAccountName"] != "release-kubikles-accelerator" || pod["initContainers"] != nil || len(pod["containers"].([]any)) != 1 {
		t.Fatal("pod has an unexpected service account or container set")
	}
	if pod["restartPolicy"] != "Never" || spec["activeDeadlineSeconds"] != nil {
		t.Fatal("job lifecycle is incorrect")
	}
	container := nested(job, "spec", "template", "spec", "containers").([]any)[0].(map[string]any)
	if container["name"] != "accelerator" || nested(container, "resources", "requests", "cpu") != "100m" || nested(container, "resources", "requests", "memory") != "128Mi" || nested(container, "resources", "limits", "cpu") != "1" || nested(container, "resources", "limits", "memory") != "512Mi" {
		t.Fatal("container identity or resources are not fixed")
	}
	for _, forbidden := range []string{"command", "args", "ports", "livenessProbe", "readinessProbe", "startupProbe"} {
		if container[forbidden] != nil {
			t.Fatalf("forbidden container field %s", forbidden)
		}
	}
}

func TestRenderPinsRBACCredentialsAndHardening(t *testing.T) {
	manifest := render(t, "release", "test-ns", values(""))
	if bytes.Contains(manifest, []byte(validVerifier)) || bytes.Count(manifest, []byte("creatorVerifier")) != 2 {
		t.Fatal("verifier must only be encoded in its Secret and referenced once")
	}
	result := objects(t, manifest)
	for _, object := range result {
		metadata := nested(object, "metadata").(map[string]any)
		labels := metadata["labels"].(map[string]any)
		if labels["app.kubernetes.io/name"] != "kubikles-accelerator" || labels["app.kubernetes.io/instance"] != "release" || labels["app.kubernetes.io/component"] != "accelerator" || labels["app.kubernetes.io/part-of"] != "kubikles" || labels["app.kubernetes.io/managed-by"] != "Helm" {
			t.Fatalf("object %s lacks exact ownership labels", object["kind"])
		}
	}
	role := find(t, result, "ClusterRole")
	rules := nested(role, "rules").([]any)
	if len(rules) != 1 {
		t.Fatal("expected exactly one RBAC rule")
	}
	binding := find(t, result, "ClusterRoleBinding")
	if nested(binding, "roleRef", "apiGroup") != "rbac.authorization.k8s.io" || nested(binding, "roleRef", "kind") != "ClusterRole" || nested(binding, "roleRef", "name") != nested(role, "metadata", "name") {
		t.Fatal("ClusterRoleBinding roleRef is not exact")
	}
	subjects := nested(binding, "subjects").([]any)
	if len(subjects) != 1 || nested(subjects[0].(map[string]any), "kind") != "ServiceAccount" || nested(subjects[0].(map[string]any), "name") != "release-kubikles-accelerator" || nested(subjects[0].(map[string]any), "namespace") != "test-ns" {
		t.Fatal("ClusterRoleBinding subject is not exact")
	}
	rule := rules[0].(map[string]any)
	if strings.Join(stringSlice(rule["apiGroups"]), ",") != "" || strings.Join(stringSlice(rule["resources"]), ",") != "secrets" || strings.Join(stringSlice(rule["verbs"]), ",") != "get,list,watch" {
		t.Fatal("RBAC rule differs from fixed Secret read rule")
	}
	secret := find(t, result, "Secret")
	if secret["type"] != "Opaque" || secret["immutable"] != true {
		t.Fatal("verifier Secret must be immutable Opaque")
	}
	data := nested(secret, "data").(map[string]any)
	if len(data) != 1 {
		t.Fatal("verifier Secret has unexpected keys")
	}
	decoded, _ := base64.StdEncoding.DecodeString(data["creatorVerifier"].(string))
	if string(decoded) != validVerifier {
		t.Fatal("verifier data mismatch")
	}
	job := find(t, result, "Job")
	pod := nested(job, "spec", "template", "spec").(map[string]any)
	if pod["automountServiceAccountToken"] != false || len(pod["volumes"].([]any)) != 1 {
		t.Fatal("projected service account identity is not exact")
	}
	container := nested(job, "spec", "template", "spec", "containers").([]any)[0].(map[string]any)
	if container["image"] != "example.invalid/kubikles-accelerator@"+validDigest || container["imagePullPolicy"] != "IfNotPresent" {
		t.Fatal("image is not digest pinned")
	}
	if len(container["env"].([]any)) != 1 {
		t.Fatal("credential environment is not exact")
	}
	if len(container["volumeMounts"].([]any)) != 1 || nested(container, "volumeMounts") == nil || nested(container, "volumeMounts").([]any)[0].(map[string]any)["name"] != "serviceaccount" || nested(container, "volumeMounts").([]any)[0].(map[string]any)["mountPath"] != "/var/run/secrets/kubernetes.io/serviceaccount" {
		t.Fatal("service account mount is not exact")
	}
	env := container["env"].([]any)[0].(map[string]any)
	if env["name"] != "KUBIKLES_ACCELERATOR_CREATOR_VERIFIER" || nested(env, "valueFrom", "secretKeyRef", "name") != "release-kubikles-accelerator-verifier" || nested(env, "valueFrom", "secretKeyRef", "key") != "creatorVerifier" || env["value"] != nil {
		t.Fatal("credential is not isolated in its exact Secret key reference")
	}
	if nested(container, "securityContext", "readOnlyRootFilesystem") != true || nested(container, "securityContext", "allowPrivilegeEscalation") != false || strings.Join(stringSlice(nested(container, "securityContext", "capabilities", "drop")), ",") != "ALL" {
		t.Fatal("container hardening is incorrect")
	}
	if nested(job, "spec", "template", "spec", "securityContext", "runAsUser") != float64(65532) || nested(job, "spec", "template", "spec", "securityContext", "runAsGroup") != float64(65532) || nested(job, "spec", "template", "spec", "securityContext", "runAsNonRoot") != true || nested(job, "spec", "template", "spec", "securityContext", "seccompProfile", "type") != "RuntimeDefault" {
		t.Fatal("pod user is not fixed")
	}
	if nested(job, "spec", "template", "spec", "terminationGracePeriodSeconds") != float64(30) || nested(job, "metadata", "labels", "kubikles.io/workload-session-id") != "desktop-session-1" || nested(job, "metadata", "annotations", "kubikles.io/build-version") != "v1.2.3" {
		t.Fatal("workload identity or termination contract is incorrect")
	}
	if nested(job, "spec", "template", "metadata", "labels", "kubikles.io/workload-session-id") != "desktop-session-1" || nested(job, "spec", "template", "metadata", "annotations", "kubikles.io/build-version") != "v1.2.3" {
		t.Fatal("Pod template workload correlation is not exact")
	}
	volume := nested(job, "spec", "template", "spec", "volumes").([]any)[0].(map[string]any)
	if volume["name"] != "serviceaccount" || nested(volume, "projected", "defaultMode") != float64(292) || len(nested(volume, "projected", "sources").([]any)) != 3 || nested(container, "volumeMounts").([]any)[0].(map[string]any)["readOnly"] != true {
		t.Fatal("projected service account contents or mode is not exact")
	}
	sources := nested(volume, "projected", "sources").([]any)
	downwardItems := nested(sources[2].(map[string]any), "downwardAPI", "items").([]any)
	if nested(sources[0].(map[string]any), "serviceAccountToken", "path") != "token" || nested(sources[0].(map[string]any), "serviceAccountToken", "expirationSeconds") != float64(3600) || nested(sources[1].(map[string]any), "configMap", "name") != "kube-root-ca.crt" || nested(sources[1].(map[string]any), "configMap", "items").([]any)[0].(map[string]any)["key"] != "ca.crt" || nested(sources[1].(map[string]any), "configMap", "items").([]any)[0].(map[string]any)["path"] != "ca.crt" || len(downwardItems) != 1 || downwardItems[0].(map[string]any)["path"] != "namespace" || nested(downwardItems[0].(map[string]any), "fieldRef", "fieldPath") != "metadata.namespace" {
		t.Fatal("projected service account sources are not exact")
	}
	for _, object := range result {
		if object["kind"] == "Role" || object["kind"] == "RoleBinding" || object["kind"] == "NetworkPolicy" {
			t.Fatal("forbidden object rendered")
		}
	}
}

func TestRenderClusterNamesAreNamespaceBoundAndStable(t *testing.T) {
	first := objects(t, render(t, "same-release", "first-ns", values("")))
	again := objects(t, render(t, "same-release", "first-ns", values("")))
	second := objects(t, render(t, "same-release", "second-ns", values("")))
	firstRole := nested(find(t, first, "ClusterRole"), "metadata", "name").(string)
	againRole := nested(find(t, again, "ClusterRole"), "metadata", "name").(string)
	secondRole := nested(find(t, second, "ClusterRole"), "metadata", "name").(string)
	if firstRole != againRole || firstRole == secondRole || len(firstRole) > 63 || len(secondRole) > 63 {
		t.Fatal("cluster-scoped names are not stable, bounded, and namespace-specific")
	}
	for _, tc := range []struct {
		objects   []map[string]any
		namespace string
		role      string
	}{{first, "first-ns", firstRole}, {second, "second-ns", secondRole}} {
		binding := find(t, tc.objects, "ClusterRoleBinding")
		if nested(binding, "metadata", "name") != tc.role || nested(binding, "roleRef", "name") != tc.role {
			t.Fatal("cluster binding name or role reference is not exact")
		}
		subjects := nested(binding, "subjects").([]any)
		if len(subjects) != 1 || nested(subjects[0].(map[string]any), "name") != "same-release-kubikles-accelerator" || nested(subjects[0].(map[string]any), "namespace") != tc.namespace {
			t.Fatal("cluster binding subject is not exact")
		}
	}
}

func TestVerifierExistsOnlyInImmutableSecret(t *testing.T) {
	manifest := render(t, "release", "test-ns", values(""))
	encoded := base64.StdEncoding.EncodeToString([]byte(validVerifier))
	rawToken := "raw-token-fixture-must-never-render"
	digest := sha256.Sum256([]byte(validVerifier))
	derivedMarkers := []string{hex.EncodeToString(digest[:]), hex.EncodeToString(digest[:8])}
	if bytes.Contains(manifest, []byte(validVerifier)) || bytes.Contains(manifest, []byte(rawToken)) || bytes.Count(manifest, []byte(encoded)) != 1 {
		t.Fatal("credential material escaped its exact transport field")
	}
	for _, marker := range derivedMarkers {
		if bytes.Contains(manifest, []byte(marker)) {
			t.Fatal("verifier-derived marker escaped into the rendered corpus")
		}
	}

	result := objects(t, manifest)
	secret := find(t, result, "Secret")
	if secret["type"] != "Opaque" || secret["immutable"] != true || len(nested(secret, "data").(map[string]any)) != 1 || nested(secret, "data", "creatorVerifier") != encoded {
		t.Fatal("verifier is not isolated in the exact immutable Secret field")
	}
	job := find(t, result, "Job")
	env := nested(job, "spec", "template", "spec", "containers").([]any)[0].(map[string]any)["env"].([]any)
	if len(env) != 1 || nested(env[0].(map[string]any), "valueFrom", "secretKeyRef", "name") != "release-kubikles-accelerator-verifier" || nested(env[0].(map[string]any), "valueFrom", "secretKeyRef", "key") != "creatorVerifier" || env[0].(map[string]any)["value"] != nil {
		t.Fatal("Job does not have the one exact verifier Secret reference")
	}

	err := filepath.WalkDir(chartDir(t), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.Contains(filepath.ToSlash(path), "/tests/") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, forbidden := range append([]string{validVerifier, encoded, rawToken}, derivedMarkers...) {
			if bytes.Contains(content, []byte(forbidden)) {
				t.Fatal("credential fixture or derived marker exists in chart source corpus")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal("scan chart source corpus")
	}
}

func TestRenderForbidsLifecycleSecurityAndReferenceEscapes(t *testing.T) {
	result := objects(t, render(t, "release", "test-ns", values("")))
	job := find(t, result, "Job")
	spec := nested(job, "spec").(map[string]any)
	pod := nested(job, "spec", "template", "spec").(map[string]any)
	container := nested(job, "spec", "template", "spec", "containers").([]any)[0].(map[string]any)
	if len(spec) != 5 || len(pod) != 7 || len(container) != 7 || len(nested(pod, "securityContext").(map[string]any)) != 4 || len(nested(container, "securityContext").(map[string]any)) != 3 {
		t.Fatal("Job, Pod, or container contains an unapproved field")
	}
	for _, key := range []string{"activeDeadlineSeconds", "selector"} {
		if spec[key] != nil {
			t.Fatalf("forbidden Job field %s", key)
		}
	}
	for _, key := range []string{"hostNetwork", "hostPID", "hostIPC", "hostname", "subdomain", "nodeName", "nodeSelector", "affinity", "tolerations", "imagePullSecrets", "runtimeClassName", "priorityClassName"} {
		if pod[key] != nil {
			t.Fatalf("forbidden Pod field %s", key)
		}
	}
	for _, key := range []string{"command", "args", "ports", "lifecycle", "livenessProbe", "readinessProbe", "startupProbe", "workingDir", "stdin", "tty"} {
		if container[key] != nil {
			t.Fatalf("forbidden container field %s", key)
		}
	}
	if nested(container, "securityContext", "privileged") != nil || nested(container, "securityContext", "procMount") != nil || nested(pod, "securityContext", "fsGroup") != nil || nested(pod, "securityContext", "supplementalGroups") != nil {
		t.Fatal("forbidden security escape rendered")
	}
	if nested(job, "metadata", "name") != "release-kubikles-accelerator" || pod["serviceAccountName"] != "release-kubikles-accelerator" || nested(find(t, result, "ServiceAccount"), "metadata", "name") != "release-kubikles-accelerator" || nested(find(t, result, "Secret"), "metadata", "name") != "release-kubikles-accelerator-verifier" {
		t.Fatal("namespaced resource names are not exact")
	}
}

func TestRenderEscapesValuesAndPreservesWorkloadIDBounds(t *testing.T) {
	manifest := string(render(t, "release", "test-ns", strings.Replace(values(""), "version: v1.2.3", "version: 'v1: quoted'", 1)))
	if !strings.Contains(manifest, `kubikles.io/build-version: "v1: quoted"`) {
		t.Fatal("build version was not YAML escaped")
	}
	maxID := "a" + strings.Repeat("x", 61) + "z"
	lint(t, strings.Replace(values(""), "workloadSessionId: desktop-session-1", "workloadSessionId: "+maxID, 1), true)
}

func stringSlice(value any) []string {
	values := value.([]any)
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = value.(string)
	}
	return result
}
