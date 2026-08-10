//go:build helm

package helm

// Strict Accelerator helpers are intentionally separate from the UI client's
// upgrade and subprocess OCI paths.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"kubikles/pkg/debug"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote/auth"
	"sigs.k8s.io/yaml"
)

var ErrAcceleratorUnavailable = errors.New("accelerator provisioning unavailable")
var ErrAcceleratorIntegrity = errors.New("accelerator chart integrity failed")
var errAcceleratorExistingResource = errors.New("accelerator fresh install encountered existing resource")
var errAcceleratorCreateFailed = errors.New("accelerator resource create failed")
var errAcceleratorCreateConflict = errors.New("accelerator resource create conflict")
var errAcceleratorCreatePermission = errors.New("accelerator resource create permission denied")
var errAcceleratorRenderMismatch = errors.New("accelerator live render did not match prepared release")

var acceleratorDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var acceleratorVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?$`)
var acceleratorRegistryReference = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?::[0-9]+)?(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+(?::[A-Za-z0-9_][A-Za-z0-9._-]{0,127}|@sha256:[0-9a-f]{64})$`)
var acceleratorSession = regexp.MustCompile(`^[0-9a-f]{32}$`)
var acceleratorVerifier = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

const (
	acceleratorChartRepository = "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator@"
	acceleratorImageRepository = "ghcr.io/skrobylabs/kubikles-accelerator"
	acceleratorRegistryTimeout = 60 * time.Second
	acceleratorSchemaSHA256    = "ed7e27cb666c4fd24bfe45379286a4628c4165dd433f5fc8cc5d43db2f2311dc"
)

type AcceleratorChartRequest struct {
	Reference, Digest, BuildVersion string
	AllowVersionMismatch            bool
}

type acceleratorContextTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t acceleratorContextTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Scheme != "https" {
		return nil, ErrAcceleratorIntegrity
	}
	return t.base.RoundTrip(request.Clone(t.ctx))
}

type acceleratorRegistryPuller interface {
	Pull(string, ...registry.PullOption) (*registry.PullResult, error)
}

var acceleratorRegistryTransportForTest = func(transport http.RoundTripper) http.RoundTripper { return transport }

// PullAcceleratorChart pulls an anonymous, digest-pinned OCI chart entirely in
// memory. Helm's Pull has no context argument, so the request transport binds
// every SDK request to the caller's bounded context.
func PullAcceleratorChart(ctx context.Context, request AcceleratorChartRequest) (*chart.Chart, error) {
	if !validAcceleratorChartRequest(request) {
		logAcceleratorChartFailure(request, "validate_request", ErrAcceleratorIntegrity, nil)
		return nil, ErrAcceleratorIntegrity
	}
	bound, cancel := context.WithTimeout(ctx, acceleratorRegistryTimeout)
	defer cancel()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	httpClient := &http.Client{Timeout: acceleratorRegistryTimeout, Transport: acceleratorContextTransport{ctx: bound, base: acceleratorRegistryTransportForTest(transport)}}
	authorizer := auth.Client{Client: httpClient, Credential: func(context.Context, string) (auth.Credential, error) {
		return auth.EmptyCredential, nil
	}}
	client, err := registry.NewClient(
		registry.ClientOptWriter(io.Discard),
		registry.ClientOptHTTPClient(httpClient),
		registry.ClientOptAuthorizer(authorizer),
	)
	if err != nil {
		logAcceleratorChartFailure(request, "create_registry_client", err, nil)
		return nil, ErrAcceleratorUnavailable
	}
	return pullAcceleratorChart(bound, client, request)
}

func validAcceleratorChartRequest(request AcceleratorChartRequest) bool {
	reference := strings.TrimPrefix(request.Reference, "oci://")
	if request.BuildVersion == "" || len(request.BuildVersion) > 128 || !acceleratorRegistryReference.MatchString(reference) {
		return false
	}
	if request.Digest == "" {
		return !strings.Contains(request.Reference, "@")
	}
	return acceleratorDigest.MatchString(request.Digest) && strings.HasSuffix(request.Reference, "@"+request.Digest)
}

func pullAcceleratorChart(ctx context.Context, puller acceleratorRegistryPuller, request AcceleratorChartRequest) (*chart.Chart, error) {
	if !validAcceleratorChartRequest(request) {
		logAcceleratorChartFailure(request, "validate_request", ErrAcceleratorIntegrity, nil)
		return nil, ErrAcceleratorIntegrity
	}
	result, err := puller.Pull(request.Reference, registry.PullOptWithChart(true), registry.PullOptWithProv(false))
	if err != nil {
		logAcceleratorChartFailure(request, "registry_pull", err, ctx.Err())
		if errors.Is(err, content.ErrMismatchedDigest) {
			return nil, ErrAcceleratorIntegrity
		}
		if ctx.Err() != nil {
			return nil, ErrAcceleratorUnavailable
		}
		return nil, ErrAcceleratorUnavailable
	}
	if result == nil || result.Manifest == nil || (request.Digest != "" && result.Manifest.Digest != request.Digest) || result.Chart == nil || result.Chart.Meta == nil || result.Config == nil {
		logAcceleratorChartFailure(request, "validate_registry_response", errors.New("registry response is incomplete or does not match the requested digest"), nil)
		return nil, ErrAcceleratorIntegrity
	}
	if !exactAcceleratorManifestLayers(result.Manifest.Data) {
		logAcceleratorChartFailure(request, "validate_manifest", errors.New("registry manifest has unexpected chart layers"), nil)
		return nil, ErrAcceleratorIntegrity
	}
	loaded, err := loadAcceleratorArchive(result.Chart.Data, request)
	if err != nil {
		logAcceleratorChartFailure(request, "load_and_validate_chart", err, nil)
		return nil, err
	}
	return loaded, nil
}

func logAcceleratorChartFailure(request AcceleratorChartRequest, stage string, err, contextErr error) {
	details := map[string]interface{}{
		"reference": request.Reference,
		"digest":    request.Digest,
		"stage":     stage,
	}
	if err != nil {
		details["error"] = err.Error()
	}
	if contextErr != nil {
		details["contextError"] = contextErr.Error()
	}
	debug.LogHelm("Accelerator chart pull failed", details)
}

func exactAcceleratorManifestLayers(data []byte) bool {
	var manifest struct {
		SchemaVersion int `json:"schemaVersion"`
		Config        struct {
			MediaType string `json:"mediaType"`
		} `json:"config"`
		Layers []struct {
			MediaType string `json:"mediaType"`
		} `json:"layers"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.SchemaVersion != 2 || manifest.Config.MediaType != registry.ConfigMediaType || len(manifest.Layers) != 1 {
		return false
	}
	return manifest.Layers[0].MediaType == registry.ChartLayerMediaType
}

func loadAcceleratorArchive(data []byte, request AcceleratorChartRequest) (*chart.Chart, error) {
	loaded, err := loader.LoadArchive(bytes.NewReader(data))
	if err != nil {
		return nil, ErrAcceleratorIntegrity
	}
	return validateAcceleratorChart(loaded, request)
}

func validateAcceleratorChart(loaded *chart.Chart, request AcceleratorChartRequest) (*chart.Chart, error) {
	if request.BuildVersion == "" || loaded == nil || loaded.Metadata == nil || loaded.Metadata.Name != "kubikles-accelerator" || loaded.Metadata.Type != "application" || !exactAcceleratorSchema(loaded.Schema) {
		return nil, ErrAcceleratorIntegrity
	}
	if !request.AllowVersionMismatch && (!acceleratorVersion.MatchString(request.BuildVersion) || loaded.Metadata.Version != strings.TrimPrefix(request.BuildVersion, "v") || loaded.Metadata.AppVersion != request.BuildVersion) {
		return nil, ErrAcceleratorIntegrity
	}
	return loaded, nil
}

func exactAcceleratorSchema(schema []byte) bool {
	digest := sha256.Sum256(schema)
	return hex.EncodeToString(digest[:]) == acceleratorSchemaSHA256
}

func validAcceleratorReleaseRequest(request AcceleratorReleaseRequest) bool {
	return validAcceleratorChartRequest(AcceleratorChartRequest{request.ChartReference, request.ChartDigest, request.BuildVersion, request.AllowVersionMismatch}) &&
		validAcceleratorImageReference(effectiveAcceleratorImageReference(request)) &&
		acceleratorSession.MatchString(request.WorkloadSession) && acceleratorVerifier.MatchString(request.CreatorVerifier) &&
		request.ReleaseName == "kubikles-accelerator-"+request.WorkloadSession && request.ReleaseNamespace != ""
}

// PrepareAcceleratorRelease performs exact pull, schema validation, client-only
// render, and strict five-object validation before any cluster mutation.
func (c *Client) PrepareAcceleratorRelease(ctx context.Context, request AcceleratorReleaseRequest) (*AcceleratorPreparedRelease, AcceleratorFailure) {
	if !validAcceleratorReleaseRequest(request) {
		return nil, AcceleratorIntegrity
	}
	loaded, err := PullAcceleratorChart(ctx, AcceleratorChartRequest{request.ChartReference, request.ChartDigest, request.BuildVersion, request.AllowVersionMismatch})
	if err != nil {
		if errors.Is(err, ErrAcceleratorIntegrity) {
			return nil, AcceleratorIntegrity
		}
		return nil, AcceleratorPull
	}
	return RenderAcceleratorRelease(loaded, request)
}

// RenderAcceleratorRelease validates and renders an already digest-verified
// in-memory chart. It exists so callers can settle context cancellation between
// the pull and render boundaries without rereading kubeconfig.
func RenderAcceleratorRelease(loaded *chart.Chart, request AcceleratorReleaseRequest) (*AcceleratorPreparedRelease, AcceleratorFailure) {
	return prepareAcceleratorRelease(loaded, request)
}

func prepareAcceleratorRelease(loaded *chart.Chart, request AcceleratorReleaseRequest) (*AcceleratorPreparedRelease, AcceleratorFailure) {
	if !validAcceleratorReleaseRequest(request) {
		return nil, AcceleratorIntegrity
	}
	if _, err := validateAcceleratorChart(loaded, AcceleratorChartRequest{request.ChartReference, request.ChartDigest, request.BuildVersion, request.AllowVersionMismatch}); err != nil {
		return nil, AcceleratorIntegrity
	}
	values := acceleratorValues(request)
	if err := chartutil.ValidateAgainstSchema(loaded, values); err != nil {
		return nil, AcceleratorIntegrity
	}
	actionConfig := &action.Configuration{}
	install := action.NewInstall(actionConfig)
	install.ClientOnly = true
	install.DryRun = true
	install.DryRunOption = "client"
	install.DisableHooks = true
	install.Replace = false
	install.ReleaseName = request.ReleaseName
	install.Namespace = request.ReleaseNamespace
	install.IncludeCRDs = false
	install.SkipCRDs = true
	install.SkipSchemaValidation = false
	rendered, err := install.Run(loaded, values)
	if err != nil || rendered == nil || rendered.Manifest == "" {
		return nil, AcceleratorRender
	}
	resources, jobName, verifierName, ok := validateAcceleratorManifest(rendered.Manifest, request)
	if !ok {
		return nil, AcceleratorRender
	}
	hash, ok := acceleratorReleaseHash(rendered.Manifest, values)
	if !ok {
		return nil, AcceleratorRender
	}
	return &AcceleratorPreparedRelease{
		request: request, chart: loaded, values: values, manifest: rendered.Manifest, renderHash: hash,
		resources: resources, jobName: jobName, verifierName: verifierName,
	}, AcceleratorOK
}

func acceleratorValues(request AcceleratorReleaseRequest) map[string]interface{} {
	return map[string]interface{}{
		"image":       map[string]interface{}{"reference": effectiveAcceleratorImageReference(request), "version": request.BuildVersion, "architecture": acceleratorRuntimeArchitecture()},
		"accelerator": map[string]interface{}{"workloadSessionId": request.WorkloadSession, "allowVersionMismatch": request.AllowVersionMismatch},
		"auth":        map[string]interface{}{"creatorVerifier": request.CreatorVerifier},
	}
}

func effectiveAcceleratorImageReference(request AcceleratorReleaseRequest) string {
	if request.ImageReference != "" {
		return request.ImageReference
	}
	if request.ImageRepository != "" && request.ImageDigest != "" {
		return request.ImageRepository + "@" + request.ImageDigest
	}
	return ""
}

func validAcceleratorImageReference(reference string) bool {
	return len(reference) <= 512 && acceleratorRegistryReference.MatchString(reference)
}

func acceleratorFullname(releaseName string) string {
	name := releaseName + "-kubikles-accelerator"
	if len(name) > 63 {
		name = name[:63]
	}
	return strings.TrimSuffix(name, "-")
}

func acceleratorClusterName(namespace, releaseName string) string {
	base := strings.ToLower(strings.ReplaceAll(namespace+"-"+releaseName+"-accelerator", "_", "-"))
	if len(base) > 54 {
		base = base[:54]
	}
	base = strings.TrimSuffix(base, "-")
	digest := sha256.Sum256([]byte(namespace + "/" + releaseName))
	return base + "-" + hex.EncodeToString(digest[:])[:8]
}

func acceleratorLabels(releaseName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": "kubikles-accelerator", "app.kubernetes.io/instance": releaseName,
		"app.kubernetes.io/component": "accelerator", "app.kubernetes.io/part-of": "kubikles", "app.kubernetes.io/managed-by": "Helm",
	}
}

func validateAcceleratorManifest(manifest string, request AcceleratorReleaseRequest) ([]AcceleratorResourceIdentity, string, string, bool) {
	parts := releaseutil.SplitManifests(manifest)
	if len(parts) != 5 || strings.Contains(manifest, request.CreatorVerifier) {
		return nil, "", "", false
	}
	baseName := acceleratorFullname(request.ReleaseName)
	clusterName := acceleratorClusterName(request.ReleaseNamespace, request.ReleaseName)
	verifierName := baseName + "-verifier"
	expected := map[string]AcceleratorResourceIdentity{
		"batch/v1/Job":      {APIVersion: "batch/v1", Kind: "Job", Namespace: request.ReleaseNamespace, Name: baseName},
		"v1/ServiceAccount": {APIVersion: "v1", Kind: "ServiceAccount", Namespace: request.ReleaseNamespace, Name: baseName},
		"v1/Secret":         {APIVersion: "v1", Kind: "Secret", Namespace: request.ReleaseNamespace, Name: verifierName},
		"rbac.authorization.k8s.io/v1/ClusterRole":        {APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: clusterName},
		"rbac.authorization.k8s.io/v1/ClusterRoleBinding": {APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBinding", Name: clusterName},
	}
	seen := make(map[string]bool, len(expected))
	objects := make(map[string]*unstructured.Unstructured, len(expected))
	for _, document := range parts {
		var object map[string]interface{}
		if yaml.Unmarshal([]byte(document), &object) != nil {
			return nil, "", "", false
		}
		u := &unstructured.Unstructured{Object: object}
		key := u.GetAPIVersion() + "/" + u.GetKind()
		identity, exists := expected[key]
		if !exists || seen[key] || u.GetName() != identity.Name || (u.GetNamespace() != "" && u.GetNamespace() != identity.Namespace) {
			return nil, "", "", false
		}
		annotations := u.GetAnnotations()
		if annotations["helm.sh/hook"] != "" || annotations["helm.sh/resource-policy"] != "" {
			return nil, "", "", false
		}
		labels := u.GetLabels()
		wantLabels := acceleratorLabels(request.ReleaseName)
		if key == "batch/v1/Job" {
			wantLabels["kubikles.io/workload-session-id"] = request.WorkloadSession
		}
		if !equalStringMap(labels, wantLabels) || !exactAcceleratorMetadata(u, key == "batch/v1/Job") {
			return nil, "", "", false
		}
		seen[key], objects[key] = true, u
	}
	for key := range expected {
		if !seen[key] {
			return nil, "", "", false
		}
	}
	if !validateAcceleratorJob(objects["batch/v1/Job"], request, baseName, verifierName) ||
		!validateAcceleratorServiceAccount(objects["v1/ServiceAccount"]) ||
		!validateAcceleratorSecret(objects["v1/Secret"], request) ||
		!validateAcceleratorRBAC(objects, request, baseName, clusterName) {
		return nil, "", "", false
	}
	resources := make([]AcceleratorResourceIdentity, 0, len(expected))
	for _, identity := range expected {
		resources = append(resources, identity)
	}
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Kind != resources[j].Kind {
			return resources[i].Kind < resources[j].Kind
		}
		return resources[i].Name < resources[j].Name
	})
	return resources, baseName, verifierName, true
}

func exactMapKeys(object map[string]interface{}, keys ...string) bool {
	if len(object) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return false
		}
	}
	return true
}

func exactAcceleratorMetadata(object *unstructured.Unstructured, job bool) bool {
	if object == nil {
		return false
	}
	metadata, found, err := unstructured.NestedMap(object.Object, "metadata")
	if err != nil || !found {
		return false
	}
	if job {
		return exactMapKeys(metadata, "name", "labels", "annotations")
	}
	return exactMapKeys(metadata, "name", "labels")
}

func equalStringMap(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

func nestedString(object map[string]interface{}, fields ...string) string {
	value, _, _ := unstructured.NestedString(object, fields...)
	return value
}

func validateAcceleratorJob(job *unstructured.Unstructured, request AcceleratorReleaseRequest, baseName, verifierName string) bool {
	if job == nil || job.GetAnnotations()["kubikles.io/build-version"] != request.BuildVersion {
		return false
	}
	object := job.Object
	spec, specFound, _ := unstructured.NestedMap(object, "spec")
	template, templateFound, _ := unstructured.NestedMap(spec, "template")
	templateMetadata, metadataFound, _ := unstructured.NestedMap(template, "metadata")
	podSpec, podSpecFound, _ := unstructured.NestedMap(template, "spec")
	if !exactMapKeys(object, "apiVersion", "kind", "metadata", "spec") || !specFound || !exactMapKeys(spec, "completions", "parallelism", "backoffLimit", "ttlSecondsAfterFinished", "template") ||
		!templateFound || !exactMapKeys(template, "metadata", "spec") || !metadataFound || !exactMapKeys(templateMetadata, "labels", "annotations") || !podSpecFound ||
		!exactMapKeys(podSpec, "serviceAccountName", "automountServiceAccountToken", "restartPolicy", "terminationGracePeriodSeconds", "nodeSelector", "securityContext", "containers", "volumes") {
		return false
	}
	templateLabels, _, _ := unstructured.NestedStringMap(object, "spec", "template", "metadata", "labels")
	wantTemplateLabels := acceleratorLabels(request.ReleaseName)
	wantTemplateLabels["kubikles.io/workload-session-id"] = request.WorkloadSession
	templateAnnotations, _, _ := unstructured.NestedStringMap(object, "spec", "template", "metadata", "annotations")
	nodeSelector, nodeSelectorFound, _ := unstructured.NestedStringMap(object, "spec", "template", "spec", "nodeSelector")
	containers, found, _ := unstructured.NestedSlice(object, "spec", "template", "spec", "containers")
	if !found || len(containers) != 1 || !equalStringMap(templateLabels, wantTemplateLabels) || !equalStringMap(templateAnnotations, map[string]string{"kubikles.io/build-version": request.BuildVersion}) || !nodeSelectorFound || !equalStringMap(nodeSelector, map[string]string{"kubernetes.io/arch": acceleratorRuntimeArchitecture()}) || nestedString(object, "spec", "template", "spec", "serviceAccountName") != baseName ||
		!nestedInt64Equals(object, 1, "spec", "completions") || !nestedInt64Equals(object, 1, "spec", "parallelism") || !nestedInt64Equals(object, 0, "spec", "backoffLimit") || !nestedInt64Equals(object, 3600, "spec", "ttlSecondsAfterFinished") ||
		!nestedBoolEquals(object, false, "spec", "template", "spec", "automountServiceAccountToken") || nestedString(object, "spec", "template", "spec", "restartPolicy") != "Never" || !nestedInt64Equals(object, 30, "spec", "template", "spec", "terminationGracePeriodSeconds") ||
		!validPodSecurityContext(object) || !validProjectedServiceAccountVolume(object) {
		return false
	}
	container, ok := containers[0].(map[string]interface{})
	if !ok || !exactMapKeys(container, "name", "image", "imagePullPolicy", "env", "securityContext", "resources", "volumeMounts") || nestedString(container, "image") != effectiveAcceleratorImageReference(request) || nestedString(container, "name") != "accelerator" || nestedString(container, "imagePullPolicy") != "IfNotPresent" || !validContainerSecurityContext(container) || !validContainerResources(container) || !validServiceAccountMount(container) {
		return false
	}
	env, found, _ := unstructured.NestedSlice(container, "env")
	if !found || len(env) != 1 {
		return false
	}
	envVar, ok := env[0].(map[string]interface{})
	valueFrom, valueFromFound, _ := unstructured.NestedMap(envVar, "valueFrom")
	secretKeyRef, secretRefFound, _ := unstructured.NestedMap(valueFrom, "secretKeyRef")
	return ok && exactMapKeys(envVar, "name", "valueFrom") && valueFromFound && exactMapKeys(valueFrom, "secretKeyRef") && secretRefFound && exactMapKeys(secretKeyRef, "name", "key") && nestedString(envVar, "name") == "KUBIKLES_ACCELERATOR_CREATOR_VERIFIER" &&
		nestedString(envVar, "valueFrom", "secretKeyRef", "name") == verifierName && nestedString(envVar, "valueFrom", "secretKeyRef", "key") == "creatorVerifier"
}

func nestedInt64Equals(object map[string]interface{}, want int64, fields ...string) bool {
	got, found, err := unstructured.NestedFieldCopy(object, fields...)
	if err != nil || !found {
		return false
	}
	switch value := got.(type) {
	case int64:
		return value == want
	case int:
		return int64(value) == want
	case float64:
		return value == float64(want)
	case json.Number:
		parsed, err := value.Int64()
		return err == nil && parsed == want
	default:
		return false
	}
}

func nestedBoolEquals(object map[string]interface{}, want bool, fields ...string) bool {
	got, found, err := unstructured.NestedBool(object, fields...)
	return err == nil && found && got == want
}

func validPodSecurityContext(object map[string]interface{}) bool {
	value, found, err := unstructured.NestedMap(object, "spec", "template", "spec", "securityContext")
	seccomp, seccompFound, _ := unstructured.NestedMap(value, "seccompProfile")
	return err == nil && found && exactMapKeys(value, "runAsNonRoot", "runAsUser", "runAsGroup", "seccompProfile") && seccompFound && exactMapKeys(seccomp, "type") && nestedBoolEquals(value, true, "runAsNonRoot") && nestedInt64Equals(value, 65532, "runAsUser") && nestedInt64Equals(value, 65532, "runAsGroup") && nestedString(value, "seccompProfile", "type") == "RuntimeDefault"
}

func validContainerSecurityContext(container map[string]interface{}) bool {
	value, found, err := unstructured.NestedMap(container, "securityContext")
	if err != nil || !found || len(value) != 3 || !nestedBoolEquals(value, false, "allowPrivilegeEscalation") || !nestedBoolEquals(value, true, "readOnlyRootFilesystem") {
		return false
	}
	capabilities, capabilitiesFound, _ := unstructured.NestedMap(value, "capabilities")
	return exactMapKeys(value, "allowPrivilegeEscalation", "readOnlyRootFilesystem", "capabilities") && capabilitiesFound && exactMapKeys(capabilities, "drop") && nestedStringSliceEquals(value, []string{"ALL"}, "capabilities", "drop")
}

func validContainerResources(container map[string]interface{}) bool {
	value, found, err := unstructured.NestedMap(container, "resources")
	if err != nil || !found || len(value) != 2 {
		return false
	}
	requests, requestsFound, _ := unstructured.NestedStringMap(value, "requests")
	limits, limitsFound, _ := unstructured.NestedStringMap(value, "limits")
	return exactMapKeys(value, "requests", "limits") && requestsFound && limitsFound && equalStringMap(requests, map[string]string{"cpu": "100m", "memory": "128Mi"}) && equalStringMap(limits, map[string]string{"cpu": "1", "memory": "512Mi"})
}

func validServiceAccountMount(container map[string]interface{}) bool {
	mounts, found, err := unstructured.NestedSlice(container, "volumeMounts")
	if err != nil || !found || len(mounts) != 1 {
		return false
	}
	mount, ok := mounts[0].(map[string]interface{})
	return ok && len(mount) == 3 && nestedString(mount, "name") == "serviceaccount" && nestedString(mount, "mountPath") == "/var/run/secrets/kubernetes.io/serviceaccount" && nestedBoolEquals(mount, true, "readOnly")
}

func validProjectedServiceAccountVolume(object map[string]interface{}) bool {
	volumes, found, err := unstructured.NestedSlice(object, "spec", "template", "spec", "volumes")
	if err != nil || !found || len(volumes) != 1 {
		return false
	}
	volume, ok := volumes[0].(map[string]interface{})
	projected, projectedFound, _ := unstructured.NestedMap(volume, "projected")
	if !ok || !exactMapKeys(volume, "name", "projected") || !projectedFound || !exactMapKeys(projected, "defaultMode", "sources") || nestedString(volume, "name") != "serviceaccount" || !nestedInt64Equals(volume, 292, "projected", "defaultMode") {
		return false
	}
	sources, found, err := unstructured.NestedSlice(volume, "projected", "sources")
	if err != nil || !found || len(sources) != 3 {
		return false
	}
	token, tokenOK := sources[0].(map[string]interface{})
	ca, caOK := sources[1].(map[string]interface{})
	namespace, namespaceOK := sources[2].(map[string]interface{})
	tokenProjection, tokenFound, _ := unstructured.NestedMap(token, "serviceAccountToken")
	caProjection, caFound, _ := unstructured.NestedMap(ca, "configMap")
	downwardProjection, downwardFound, _ := unstructured.NestedMap(namespace, "downwardAPI")
	if !tokenOK || !caOK || !namespaceOK || !exactMapKeys(token, "serviceAccountToken") || !exactMapKeys(ca, "configMap") || !exactMapKeys(namespace, "downwardAPI") || !tokenFound || !exactMapKeys(tokenProjection, "path", "expirationSeconds") || !caFound || !exactMapKeys(caProjection, "name", "items") || !downwardFound || !exactMapKeys(downwardProjection, "items") || nestedString(token, "serviceAccountToken", "path") != "token" || !nestedInt64Equals(token, 3600, "serviceAccountToken", "expirationSeconds") || nestedString(ca, "configMap", "name") != "kube-root-ca.crt" || !validSingleProjectionItem(ca, "configMap", "ca.crt", "ca.crt") {
		return false
	}
	items, namespaceFound, _ := unstructured.NestedSlice(namespace, "downwardAPI", "items")
	if !namespaceFound || len(items) != 1 {
		return false
	}
	item, ok := items[0].(map[string]interface{})
	fieldRef, fieldRefFound, _ := unstructured.NestedMap(item, "fieldRef")
	return ok && exactMapKeys(item, "path", "fieldRef") && fieldRefFound && exactMapKeys(fieldRef, "fieldPath") && nestedString(item, "path") == "namespace" && nestedString(item, "fieldRef", "fieldPath") == "metadata.namespace"
}

func validSingleProjectionItem(object map[string]interface{}, source, key, path string) bool {
	items, found, _ := unstructured.NestedSlice(object, source, "items")
	if !found || len(items) != 1 {
		return false
	}
	item, ok := items[0].(map[string]interface{})
	return ok && len(item) == 2 && nestedString(item, "key") == key && nestedString(item, "path") == path
}

func validateAcceleratorSecret(secret *unstructured.Unstructured, request AcceleratorReleaseRequest) bool {
	if secret == nil || !exactMapKeys(secret.Object, "apiVersion", "kind", "metadata", "type", "immutable", "data") || nestedString(secret.Object, "type") != "Opaque" {
		return false
	}
	immutable, found, _ := unstructured.NestedBool(secret.Object, "immutable")
	data, foundData, _ := unstructured.NestedStringMap(secret.Object, "data")
	want := base64.StdEncoding.EncodeToString([]byte(request.CreatorVerifier))
	return found && immutable && foundData && len(data) == 1 && data["creatorVerifier"] == want
}

func validateAcceleratorServiceAccount(account *unstructured.Unstructured) bool {
	return account != nil && exactMapKeys(account.Object, "apiVersion", "kind", "metadata", "automountServiceAccountToken") && nestedBoolEquals(account.Object, false, "automountServiceAccountToken")
}

func validateAcceleratorRBAC(objects map[string]*unstructured.Unstructured, request AcceleratorReleaseRequest, baseName, clusterName string) bool {
	role := objects["rbac.authorization.k8s.io/v1/ClusterRole"]
	binding := objects["rbac.authorization.k8s.io/v1/ClusterRoleBinding"]
	if role == nil || binding == nil {
		return false
	}
	rules, _, _ := unstructured.NestedSlice(role.Object, "rules")
	if len(rules) != 1 {
		return false
	}
	rule, ok := rules[0].(map[string]interface{})
	if !exactMapKeys(role.Object, "apiVersion", "kind", "metadata", "rules") || !ok || !exactMapKeys(rule, "apiGroups", "resources", "verbs") || !nestedStringSliceEquals(rule, []string{""}, "apiGroups") || !nestedStringSliceEquals(rule, []string{"secrets"}, "resources") || !nestedStringSliceEquals(rule, []string{"get", "list", "watch"}, "verbs") {
		return false
	}
	roleRef, roleRefFound, _ := unstructured.NestedMap(binding.Object, "roleRef")
	if !exactMapKeys(binding.Object, "apiVersion", "kind", "metadata", "roleRef", "subjects") || !roleRefFound || !exactMapKeys(roleRef, "apiGroup", "kind", "name") || nestedString(binding.Object, "roleRef", "apiGroup") != "rbac.authorization.k8s.io" || nestedString(binding.Object, "roleRef", "kind") != "ClusterRole" || nestedString(binding.Object, "roleRef", "name") != clusterName {
		return false
	}
	subjects, _, _ := unstructured.NestedSlice(binding.Object, "subjects")
	if len(subjects) != 1 {
		return false
	}
	subject, ok := subjects[0].(map[string]interface{})
	return ok && exactMapKeys(subject, "kind", "name", "namespace") && nestedString(subject, "kind") == "ServiceAccount" && nestedString(subject, "name") == baseName && nestedString(subject, "namespace") == request.ReleaseNamespace
}

func nestedStringSliceEquals(object map[string]interface{}, want []string, fields ...string) bool {
	values, found, _ := unstructured.NestedStringSlice(object, fields...)
	if !found || len(values) != len(want) {
		return false
	}
	for i := range want {
		if values[i] != want[i] {
			return false
		}
	}
	return true
}

func acceleratorReleaseHash(manifest string, values map[string]interface{}) (string, bool) {
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", false
	}
	hash := sha256.New()
	_, _ = io.WriteString(hash, manifest)
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(encoded)
	return hex.EncodeToString(hash.Sum(nil)), true
}

type acceleratorRESTClientGetter struct {
	config    *rest.Config
	namespace string
}

// acceleratorCreateOnlyKubeClient deliberately turns Helm's existing-object
// fallback into a conflict.  Helm 3.19 accepts ownership-valid objects and
// calls Update; fresh Accelerator installs may only create.
type acceleratorCreateOnlyKubeClient struct{ kube.Interface }

func (acceleratorCreateOnlyKubeClient) Update(kube.ResourceList, kube.ResourceList, bool) (*kube.Result, error) {
	return nil, errAcceleratorExistingResource
}
func (acceleratorCreateOnlyKubeClient) UpdateThreeWayMerge(kube.ResourceList, kube.ResourceList, bool) (*kube.Result, error) {
	return nil, errAcceleratorExistingResource
}

type acceleratorAttemptRecorder struct {
	mu                     sync.Mutex
	storageCreated         bool
	storageAttempted       bool
	storageName            string
	storageUID             types.UID
	storageResourceVersion string
	created                []AcceleratorDeletionIdentity
	unresolved             []AcceleratorResourceIdentity
	inFlight               *AcceleratorResourceIdentity
	requireStorageIdentity bool
}

func (r *acceleratorAttemptRecorder) recordStorage(name string, uid types.UID, resourceVersion string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storageCreated = true
	r.storageName = name
	if uid != "" {
		r.storageUID = uid
	}
	if resourceVersion != "" {
		r.storageResourceVersion = resourceVersion
	}
}

func (r *acceleratorAttemptRecorder) recordStorageAttempt() {
	r.mu.Lock()
	r.storageAttempted = true
	r.mu.Unlock()
}

func (r *acceleratorAttemptRecorder) ownershipUnproven(failure AcceleratorFailure) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.storageAttempted && !r.storageCreated && failure != AcceleratorConflict
}

func (r *acceleratorAttemptRecorder) recordCreated(identity AcceleratorResourceIdentity, uid types.UID) {
	if uid == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inFlight = nil
	r.created = append(r.created, AcceleratorDeletionIdentity{Resource: identity, UID: uid})
}

func (r *acceleratorAttemptRecorder) recordCreateAttempt(identity AcceleratorResourceIdentity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := identity
	r.inFlight = &copy
}

func (r *acceleratorAttemptRecorder) recordCreateRejected() {
	r.mu.Lock()
	r.inFlight = nil
	r.mu.Unlock()
}

func (r *acceleratorAttemptRecorder) recordCreateUnresolved() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inFlight != nil {
		r.unresolved = append(r.unresolved, *r.inFlight)
		r.inFlight = nil
	}
}

func (r *acceleratorAttemptRecorder) receipt(prepared *AcceleratorPreparedRelease) *AcceleratorOwnershipReceipt {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.storageCreated || prepared == nil || (r.requireStorageIdentity && (r.storageUID == "" || r.storageResourceVersion == "")) {
		return nil
	}
	return &AcceleratorOwnershipReceipt{
		request: prepared.request, renderHash: prepared.renderHash, storageName: r.storageName,
		storageUID: r.storageUID, storageResourceVersion: r.storageResourceVersion,
		created:    append([]AcceleratorDeletionIdentity(nil), r.created...),
		unresolved: append([]AcceleratorResourceIdentity(nil), r.unresolved...),
	}
}

type acceleratorAttemptDriver struct {
	driver.Driver
	recorder *acceleratorAttemptRecorder
	prepared *AcceleratorPreparedRelease
}

func (d acceleratorAttemptDriver) Create(key string, stored *release.Release) error {
	if proveAcceleratorOwnership(stored, d.prepared) == nil {
		return errAcceleratorRenderMismatch
	}
	d.recorder.recordStorageAttempt()
	if err := d.Driver.Create(key, stored); err != nil {
		return err
	}
	d.recorder.recordStorage(key, "", "")
	return nil
}

// Revision one is immutable for this dedicated path. Helm's status/description
// rewrites are intentionally acknowledged without mutating the storage record.
func (d acceleratorAttemptDriver) Update(string, *release.Release) error { return nil }

type acceleratorReceiptSecretClient struct {
	typedcorev1.SecretInterface
	recorder *acceleratorAttemptRecorder
}

func (c acceleratorReceiptSecretClient) Create(ctx context.Context, secret *corev1.Secret, options metav1.CreateOptions) (*corev1.Secret, error) {
	c.recorder.recordStorageAttempt()
	created, err := c.SecretInterface.Create(ctx, secret, options)
	if err == nil && created != nil {
		c.recorder.recordStorage(created.Name, created.UID, created.ResourceVersion)
	}
	return created, err
}

type acceleratorReceiptKubeClient struct {
	kube.Interface
	recorder *acceleratorAttemptRecorder
	allowed  map[string]AcceleratorResourceIdentity
}

func (c acceleratorReceiptKubeClient) Create(resources kube.ResourceList) (*kube.Result, error) {
	result := &kube.Result{}
	for _, info := range resources {
		if info == nil || info.Mapping == nil {
			return nil, errAcceleratorCreateFailed
		}
		gvk := info.Mapping.GroupVersionKind
		key := acceleratorResourceKey(gvk.GroupVersion().String(), gvk.Kind, info.Namespace, info.Name)
		identity, ok := c.allowed[key]
		if !ok {
			return nil, errAcceleratorCreateFailed
		}
		c.recorder.recordCreateAttempt(identity)
		created, err := c.Interface.Create(kube.ResourceList{info})
		if err != nil {
			if acceleratorCreateErrorIsDefinitive(err) {
				c.recorder.recordCreateRejected()
			} else {
				c.recorder.recordCreateUnresolved()
			}
			switch {
			case apierrors.IsAlreadyExists(err):
				return nil, errAcceleratorCreateConflict
			case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
				return nil, errAcceleratorCreatePermission
			default:
				return nil, errAcceleratorCreateFailed
			}
		}
		accessor, accessorErr := meta.Accessor(info.Object)
		if accessorErr != nil || accessor.GetUID() == "" {
			c.recorder.recordCreateUnresolved()
			return nil, errAcceleratorCreateFailed
		}
		c.recorder.recordCreated(identity, accessor.GetUID())
		if created != nil {
			result.Created = append(result.Created, created.Created...)
		} else {
			result.Created = append(result.Created, info)
		}
	}
	return result, nil
}

func acceleratorCreateErrorIsDefinitive(err error) bool {
	switch apierrors.ReasonForError(err) {
	case metav1.StatusReasonAlreadyExists, metav1.StatusReasonForbidden, metav1.StatusReasonUnauthorized,
		metav1.StatusReasonInvalid, metav1.StatusReasonBadRequest, metav1.StatusReasonConflict:
		return true
	default:
		return false
	}
}

type acceleratorUIDDeleteKubeClient struct {
	kube.Interface
	uids map[string]types.UID
}

func acceleratorResourceKey(apiVersion, kind, namespace, name string) string {
	return apiVersion + "/" + kind + "/" + namespace + "/" + name
}
func (c acceleratorUIDDeleteKubeClient) Delete(resources kube.ResourceList) (*kube.Result, []error) {
	return c.delete(resources, metav1.DeletePropagationBackground)
}
func (c acceleratorUIDDeleteKubeClient) DeleteWithPropagationPolicy(resources kube.ResourceList, policy metav1.DeletionPropagation) (*kube.Result, []error) {
	return c.delete(resources, policy)
}
func (c acceleratorUIDDeleteKubeClient) delete(resources kube.ResourceList, policy metav1.DeletionPropagation) (*kube.Result, []error) {
	result := &kube.Result{}
	var errs []error
	for _, info := range resources {
		if info == nil || info.Mapping == nil {
			errs = append(errs, ErrAcceleratorUnavailable)
			continue
		}
		gvk := info.Mapping.GroupVersionKind
		uid, ok := c.uids[acceleratorResourceKey(gvk.GroupVersion().String(), gvk.Kind, info.Namespace, info.Name)]
		if !ok || uid == "" {
			errs = append(errs, ErrAcceleratorUnavailable)
			continue
		}
		preconditions := &metav1.Preconditions{UID: &uid}
		opts := &metav1.DeleteOptions{PropagationPolicy: &policy, Preconditions: preconditions}
		if _, err := resource.NewHelper(info.Client, info.Mapping).DeleteWithOptions(info.Namespace, info.Name, opts); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, err)
			continue
		}
		result.Deleted = append(result.Deleted, info)
	}
	if len(errs) != 0 {
		return nil, errs
	}
	return result, nil
}

func (g acceleratorRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return rest.CopyConfig(g.config), nil
}
func (g acceleratorRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	client, err := discovery.NewDiscoveryClientForConfig(rest.CopyConfig(g.config))
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(client), nil
}
func (g acceleratorRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient), nil
}
func (g acceleratorRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return acceleratorStaticClientConfig{config: g.config, namespace: g.namespace}
}

type acceleratorStaticClientConfig struct {
	config    *rest.Config
	namespace string
}

func (c acceleratorStaticClientConfig) RawConfig() (clientcmdapi.Config, error) {
	return clientcmdapi.Config{}, nil
}
func (c acceleratorStaticClientConfig) ClientConfig() (*rest.Config, error) {
	return rest.CopyConfig(c.config), nil
}
func (c acceleratorStaticClientConfig) Namespace() (string, bool, error) {
	return c.namespace, true, nil
}
func (c acceleratorStaticClientConfig) ConfigAccess() clientcmd.ConfigAccess { return nil }

func acceleratorActionConfig(ctx context.Context, config *rest.Config, namespace string) (*action.Configuration, error) {
	if config == nil || namespace == "" {
		return nil, ErrAcceleratorUnavailable
	}
	bound := rest.CopyConfig(config)
	baseWrap := bound.WrapTransport
	bound.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		if baseWrap != nil {
			rt = baseWrap(rt)
		}
		return acceleratorContextTransport{ctx: ctx, base: rt}
	}
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(acceleratorRESTClientGetter{config: bound, namespace: namespace}, namespace, "secret", func(string, ...interface{}) {}); err != nil {
		return nil, ErrAcceleratorUnavailable
	}
	return actionConfig, nil
}

// InstallAcceleratorRelease preflights exact absence and performs only a fresh
// revision-one install. A receipt is returned after an install error only when
// the exact stored release proves this attempt owns it.
func (c *Client) InstallAcceleratorRelease(ctx context.Context, config *rest.Config, prepared *AcceleratorPreparedRelease) (*AcceleratorOwnershipReceipt, AcceleratorFailure, bool) {
	if prepared == nil || prepared.chart == nil || prepared.renderHash == "" || !validAcceleratorReleaseRequest(prepared.request) {
		return nil, AcceleratorIntegrity, false
	}
	actionConfig, err := acceleratorActionConfig(ctx, config, prepared.request.ReleaseNamespace)
	if err != nil {
		return nil, AcceleratorPermission, false
	}
	recorder := &acceleratorAttemptRecorder{requireStorageIdentity: true}
	getter, ok := actionConfig.RESTClientGetter.(acceleratorRESTClientGetter)
	if !ok {
		return nil, AcceleratorPermission, false
	}
	clientset, err := kubernetes.NewForConfig(getter.config)
	if err != nil {
		return nil, AcceleratorPermission, false
	}
	actionConfig.Releases.Driver = driver.NewSecrets(acceleratorReceiptSecretClient{SecretInterface: clientset.CoreV1().Secrets(prepared.request.ReleaseNamespace), recorder: recorder})
	receipt, failure := installAcceleratorWithActionConfig(ctx, actionConfig, prepared, recorder)
	return receipt, failure, receipt == nil && recorder.ownershipUnproven(failure)
}

func installAcceleratorWithActionConfig(ctx context.Context, actionConfig *action.Configuration, prepared *AcceleratorPreparedRelease, supplied ...*acceleratorAttemptRecorder) (*AcceleratorOwnershipReceipt, AcceleratorFailure) {
	if actionConfig == nil || prepared == nil {
		return nil, AcceleratorIntegrity
	}
	get := action.NewGet(actionConfig)
	if existing, getErr := get.Run(prepared.request.ReleaseName); getErr == nil || existing != nil {
		return nil, AcceleratorConflict
	} else if !errors.Is(getErr, driver.ErrReleaseNotFound) {
		return nil, acceleratorFailureForError(getErr, AcceleratorInstall)
	}
	if failure := preflightAcceleratorResources(actionConfig, prepared); failure != AcceleratorOK {
		return nil, failure
	}
	recorder := &acceleratorAttemptRecorder{}
	if len(supplied) == 1 && supplied[0] != nil {
		recorder = supplied[0]
	}
	actionConfig.Releases.Driver = acceleratorAttemptDriver{Driver: actionConfig.Releases.Driver, recorder: recorder, prepared: prepared}
	// Run is synchronous. Request cancellation is carried by the private REST
	// transport above, avoiding Helm's RunWithContext background install.
	allowed := make(map[string]AcceleratorResourceIdentity, len(prepared.resources))
	for _, identity := range prepared.resources {
		allowed[acceleratorResourceKey(identity.APIVersion, identity.Kind, identity.Namespace, identity.Name)] = identity
	}
	actionConfig.KubeClient = acceleratorReceiptKubeClient{Interface: acceleratorCreateOnlyKubeClient{Interface: actionConfig.KubeClient}, recorder: recorder, allowed: allowed}
	install := configureAcceleratorInstall(ctx, actionConfig, prepared)
	installed, runErr := install.Run(prepared.chart, prepared.values)
	receipt := recorder.receipt(prepared)
	if runErr == nil && receipt != nil && proveAcceleratorOwnership(installed, prepared) != nil {
		return receipt, AcceleratorOK
	}
	if receipt != nil {
		return receipt, acceleratorFailureForError(runErr, AcceleratorInstall)
	}
	if runErr == nil {
		return nil, AcceleratorConflict
	}
	return nil, acceleratorFailureForError(runErr, AcceleratorInstall)
}

func preflightAcceleratorResources(actionConfig *action.Configuration, prepared *AcceleratorPreparedRelease) AcceleratorFailure {
	resources, err := actionConfig.KubeClient.Build(bytes.NewBufferString(prepared.manifest), false)
	if err != nil {
		return acceleratorFailureForError(err, AcceleratorInstall)
	}
	for _, info := range resources {
		if info == nil || info.Mapping == nil {
			return AcceleratorInstall
		}
		_, err := resource.NewHelper(info.Client, info.Mapping).Get(info.Namespace, info.Name)
		if err == nil {
			return AcceleratorConflict
		}
		if !apierrors.IsNotFound(err) {
			return acceleratorFailureForError(err, AcceleratorInstall)
		}
	}
	return AcceleratorOK
}

func configureAcceleratorInstall(ctx context.Context, actionConfig *action.Configuration, prepared *AcceleratorPreparedRelease) *action.Install {
	install := action.NewInstall(actionConfig)
	install.Namespace = prepared.request.ReleaseNamespace
	install.ReleaseName = prepared.request.ReleaseName
	install.CreateNamespace = false
	install.Replace = false
	install.Force = false
	install.Atomic = false
	install.Wait = false
	install.WaitForJobs = false
	install.DisableHooks = true
	install.SkipCRDs = true
	install.IncludeCRDs = false
	install.TakeOwnership = false
	install.Timeout = remainingAcceleratorTimeout(ctx)
	return install
}

func remainingAcceleratorTimeout(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 {
			return remaining
		}
	}
	return time.Nanosecond
}

func proveAcceleratorOwnership(stored *release.Release, prepared *AcceleratorPreparedRelease) *AcceleratorOwnershipReceipt {
	if stored == nil || prepared == nil || stored.Name != prepared.request.ReleaseName || stored.Namespace != prepared.request.ReleaseNamespace || stored.Version != 1 {
		return nil
	}
	hash, ok := acceleratorReleaseHash(stored.Manifest, stored.Config)
	if !ok || hash != prepared.renderHash {
		return nil
	}
	return &AcceleratorOwnershipReceipt{request: prepared.request, renderHash: prepared.renderHash}
}

func acceleratorFailureForError(err error, fallback AcceleratorFailure) AcceleratorFailure {
	if errors.Is(err, errAcceleratorRenderMismatch) {
		return AcceleratorRender
	}
	if errors.Is(err, errAcceleratorExistingResource) || errors.Is(err, errAcceleratorCreateConflict) {
		return AcceleratorConflict
	}
	if errors.Is(err, errAcceleratorCreatePermission) || apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return AcceleratorPermission
	}
	if apierrors.IsAlreadyExists(err) || errors.Is(err, driver.ErrReleaseExists) {
		return AcceleratorConflict
	}
	return fallback
}

// InspectAcceleratorOwnership rechecks the exact release immediately before
// destructive cleanup. A different or replaced record is never uninstalled.
func (c *Client) InspectAcceleratorOwnership(ctx context.Context, config *rest.Config, prepared *AcceleratorPreparedRelease, receipt *AcceleratorOwnershipReceipt) bool {
	if prepared == nil || receipt == nil || receipt.renderHash != prepared.renderHash || receipt.request.ReleaseName != prepared.request.ReleaseName || receipt.request.ReleaseNamespace != prepared.request.ReleaseNamespace || receipt.storageName != acceleratorStorageName(prepared.request.ReleaseName) || receipt.storageUID == "" || receipt.storageResourceVersion == "" || !validReceiptCreatedResources(prepared, receipt) {
		return false
	}
	actionConfig, err := acceleratorActionConfig(ctx, config, prepared.request.ReleaseNamespace)
	if err != nil {
		return false
	}
	clientset, err := acceleratorClientset(ctx, config)
	if err != nil {
		return false
	}
	return inspectAcceleratorOwnershipWith(ctx, actionConfig, clientset, prepared, receipt)
}

func inspectAcceleratorOwnershipWith(ctx context.Context, actionConfig *action.Configuration, clientset kubernetes.Interface, prepared *AcceleratorPreparedRelease, receipt *AcceleratorOwnershipReceipt) bool {
	return inspectAcceleratorOwnershipStatusWith(ctx, actionConfig, clientset, prepared, receipt) == AcceleratorOwnedCleanupSucceeded
}

func inspectAcceleratorOwnershipStatusWith(ctx context.Context, actionConfig *action.Configuration, clientset kubernetes.Interface, prepared *AcceleratorPreparedRelease, receipt *AcceleratorOwnershipReceipt) AcceleratorOwnedCleanupStatus {
	if actionConfig == nil || actionConfig.Releases == nil || clientset == nil || prepared == nil || receipt == nil || receipt.renderHash != prepared.renderHash || receipt.request.ReleaseName != prepared.request.ReleaseName || receipt.request.ReleaseNamespace != prepared.request.ReleaseNamespace || receipt.storageName != acceleratorStorageName(prepared.request.ReleaseName) || receipt.storageUID == "" || receipt.storageResourceVersion == "" || !validReceiptCreatedResources(prepared, receipt) {
		return AcceleratorOwnedCleanupOwnershipChanged
	}
	history, err := actionConfig.Releases.History(prepared.request.ReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return AcceleratorOwnedCleanupAlreadyGone
	}
	if err != nil || len(history) != 1 {
		return AcceleratorOwnedCleanupOwnershipChanged
	}
	if _, stage := proveClosedAcceleratorRelease(history[0], prepared.request.ReleaseNamespace, prepared.request.ReleaseName, prepared); stage != "" {
		return AcceleratorOwnedCleanupOwnershipChanged
	}
	stored, err := clientset.CoreV1().Secrets(prepared.request.ReleaseNamespace).Get(ctx, receipt.storageName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return AcceleratorOwnedCleanupAlreadyGone
	}
	if err != nil || stored.UID != receipt.storageUID || stored.ResourceVersion != receipt.storageResourceVersion {
		return AcceleratorOwnedCleanupOwnershipChanged
	}
	return AcceleratorOwnedCleanupSucceeded
}

func acceleratorStorageName(releaseName string) string {
	return "sh.helm.release.v1." + releaseName + ".v1"
}

func validReceiptCreatedResources(prepared *AcceleratorPreparedRelease, receipt *AcceleratorOwnershipReceipt) bool {
	allowed := make(map[string]struct{}, len(prepared.resources))
	for _, identity := range prepared.resources {
		allowed[acceleratorResourceKey(identity.APIVersion, identity.Kind, identity.Namespace, identity.Name)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(receipt.created))
	for _, item := range receipt.created {
		key := acceleratorResourceKey(item.Resource.APIVersion, item.Resource.Kind, item.Resource.Namespace, item.Resource.Name)
		if item.UID == "" {
			return false
		}
		if _, ok := allowed[key]; !ok {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
	}
	for _, identity := range receipt.unresolved {
		key := acceleratorResourceKey(identity.APIVersion, identity.Kind, identity.Namespace, identity.Name)
		if _, ok := allowed[key]; !ok {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}

func acceleratorClientset(ctx context.Context, config *rest.Config) (kubernetes.Interface, error) {
	if config == nil {
		return nil, ErrAcceleratorUnavailable
	}
	bound := rest.CopyConfig(config)
	baseWrap := bound.WrapTransport
	bound.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		if baseWrap != nil {
			rt = baseWrap(rt)
		}
		return acceleratorContextTransport{ctx: ctx, base: rt}
	}
	return kubernetes.NewForConfig(bound)
}

// DeleteOwnedAcceleratorResources deletes only the successful Create prefix
// recorded by this attempt. Revision-one storage remains as evidence.
func (c *Client) DeleteOwnedAcceleratorResources(ctx context.Context, config *rest.Config, prepared *AcceleratorPreparedRelease, receipt *AcceleratorOwnershipReceipt) AcceleratorFailure {
	if !c.InspectAcceleratorOwnership(ctx, config, prepared, receipt) {
		return AcceleratorConflict
	}
	actionConfig, err := acceleratorActionConfig(ctx, config, prepared.request.ReleaseNamespace)
	if err != nil {
		return AcceleratorPermission
	}
	resources, err := actionConfig.KubeClient.Build(bytes.NewBufferString(prepared.manifest), false)
	if err != nil {
		return acceleratorFailureForError(err, AcceleratorCleanup)
	}
	byKey := make(map[string]*resource.Info, len(resources))
	for _, info := range resources {
		if info == nil || info.Mapping == nil {
			return AcceleratorConflict
		}
		gvk := info.Mapping.GroupVersionKind
		byKey[acceleratorResourceKey(gvk.GroupVersion().String(), gvk.Kind, info.Namespace, info.Name)] = info
	}
	for _, created := range receipt.CreatedResources() {
		identity := created.Resource
		key := acceleratorResourceKey(identity.APIVersion, identity.Kind, identity.Namespace, identity.Name)
		info := byKey[key]
		if info == nil {
			return AcceleratorConflict
		}
		deleter := acceleratorUIDDeleteKubeClient{Interface: actionConfig.KubeClient, uids: map[string]types.UID{key: created.UID}}
		policy := metav1.DeletePropagationBackground
		if identity.Kind == "Job" {
			policy = metav1.DeletePropagationForeground
		}
		if _, errs := deleter.DeleteWithPropagationPolicy(kube.ResourceList{info}, policy); len(errs) != 0 {
			return acceleratorFailureForError(errs[0], AcceleratorCleanup)
		}
	}
	return AcceleratorOK
}

// PurgeOwnedAcceleratorRelease removes only the exact immutable revision-one
// storage object, after callers have proved all attempt-created resources gone.
func (c *Client) PurgeOwnedAcceleratorRelease(ctx context.Context, config *rest.Config, prepared *AcceleratorPreparedRelease, receipt *AcceleratorOwnershipReceipt) AcceleratorFailure {
	if !c.InspectAcceleratorOwnership(ctx, config, prepared, receipt) {
		return AcceleratorConflict
	}
	clientset, err := acceleratorClientset(ctx, config)
	if err != nil {
		return AcceleratorPermission
	}
	uid, resourceVersion := receipt.storageUID, receipt.storageResourceVersion
	options := metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &resourceVersion}}
	if err := clientset.CoreV1().Secrets(prepared.request.ReleaseNamespace).Delete(ctx, receipt.storageName, options); err != nil && !apierrors.IsNotFound(err) {
		return acceleratorFailureForError(err, AcceleratorCleanup)
	}
	return AcceleratorOK
}
