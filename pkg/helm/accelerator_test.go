//go:build helm

package helm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/cli-runtime/pkg/resource"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

type acceleratorPullerFake struct {
	ref    string
	result *registry.PullResult
	err    error
}

func (f *acceleratorPullerFake) Pull(ref string, _ ...registry.PullOption) (*registry.PullResult, error) {
	f.ref = ref
	return f.result, f.err
}

type acceleratorRegistryMismatchProbe struct {
	mu                 sync.Mutex
	base               http.RoundTripper
	target             *url.URL
	headRequests       int
	getRequests        int
	nonHTTPSRequests   int
	nonGHCRRequests    int
	authorizedRequests int
	requestsWithNoTTL  int
}

func (p *acceleratorRegistryMismatchProbe) RoundTrip(request *http.Request) (*http.Response, error) {
	p.mu.Lock()
	if request == nil || request.URL == nil || request.URL.Scheme != "https" {
		p.nonHTTPSRequests++
	}
	if request == nil || request.URL == nil || request.URL.Host != "ghcr.io" {
		p.nonGHCRRequests++
	}
	if request != nil && request.Header.Get("Authorization") != "" {
		p.authorizedRequests++
	}
	if request == nil {
		p.requestsWithNoTTL++
	} else if _, ok := request.Context().Deadline(); !ok {
		p.requestsWithNoTTL++
	}
	if request != nil {
		switch request.Method {
		case http.MethodHead:
			p.headRequests++
		case http.MethodGet:
			p.getRequests++
		}
	}
	p.mu.Unlock()
	if request == nil || request.URL == nil {
		return nil, errors.New("nil registry request")
	}
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	clone.URL = &urlCopy
	clone.URL.Scheme, clone.URL.Host, clone.Host = p.target.Scheme, p.target.Host, ""
	return p.base.RoundTrip(clone)
}

func (p *acceleratorRegistryMismatchProbe) snapshot() (int, int, int, int, int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.headRequests, p.getRequests, p.nonHTTPSRequests, p.nonGHCRRequests, p.authorizedRequests, p.requestsWithNoTTL
}

func TestPullAcceleratorChartPublicSDKRejectsFixtureDigestMismatch(t *testing.T) {
	request := acceleratorTestRequest()
	mismatchedManifest := []byte(`{"schemaVersion":2,"config":{"mediaType":"application/vnd.cncf.helm.config.v1+json","digest":"sha256:` + strings.Repeat("c", 64) + `","size":2},"layers":[]}`)
	expectedPath := "/v2/skrobylabs/helm/kubikles-accelerator/manifests/" + request.ChartDigest
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, incoming *http.Request) {
		if incoming.URL.Path != expectedPath {
			http.NotFound(response, incoming)
			return
		}
		response.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		response.Header().Set("Content-Length", strconv.Itoa(len(mismatchedManifest)))
		// The registry advertises the pinned digest while deliberately returning
		// different bytes, so ORAS verifies and rejects the content identity.
		response.Header().Set("Docker-Content-Digest", request.ChartDigest)
		if incoming.Method == http.MethodGet {
			_, _ = response.Write(mismatchedManifest)
		}
	}))
	defer fixture.Close()
	target, err := url.Parse(fixture.URL)
	if err != nil {
		t.Fatal(err)
	}
	trustedTransport := fixture.Client().Transport.(*http.Transport)
	probe := &acceleratorRegistryMismatchProbe{target: target}
	previousTransport := acceleratorRegistryTransportForTest
	acceleratorRegistryTransportForTest = func(base http.RoundTripper) http.RoundTripper {
		transport, ok := base.(*http.Transport)
		if !ok {
			t.Fatalf("registry transport type %T", base)
		}
		clone := transport.Clone()
		clone.TLSClientConfig = trustedTransport.TLSClientConfig.Clone()
		probe.base = clone
		return probe
	}
	defer func() { acceleratorRegistryTransportForTest = previousTransport }()

	loaded, err := PullAcceleratorChart(context.Background(), AcceleratorChartRequest{
		Reference: request.ChartReference, Digest: request.ChartDigest, BuildVersion: request.BuildVersion,
	})
	if loaded != nil || !errors.Is(err, ErrAcceleratorIntegrity) || err.Error() != ErrAcceleratorIntegrity.Error() {
		t.Fatalf("public pull chart=%#v error=%v", loaded, err)
	}
	source, err := os.ReadFile("accelerator.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "registry.ClientOptWriter(io.Discard)") {
		t.Fatal("public SDK pull no longer discards Helm registry output")
	}
	head, get, nonHTTPS, nonGHCR, authorized, noTTL := probe.snapshot()
	if get == 0 || nonHTTPS != 0 || nonGHCR != 0 || authorized != 0 || noTTL != 0 {
		t.Fatalf("SDK path head=%d get=%d nonHTTPS=%d nonGHCR=%d authorized=%d noTTL=%d", head, get, nonHTTPS, nonGHCR, authorized, noTTL)
	}
}

func TestPullAcceleratorChartByDigestSDKOnly(t *testing.T) {
	loaded, archive := acceleratorTestChart(t)
	request := acceleratorTestRequest()
	chartRequest := AcceleratorChartRequest{Reference: request.ChartReference, Digest: request.ChartDigest, BuildVersion: request.BuildVersion}
	manifest, _ := json.Marshal(map[string]interface{}{
		"schemaVersion": 2,
		"config":        map[string]string{"mediaType": registry.ConfigMediaType, "digest": "sha256:" + strings.Repeat("c", 64)},
		"layers":        []map[string]string{{"mediaType": registry.ChartLayerMediaType, "digest": "sha256:" + strings.Repeat("d", 64)}},
	})
	puller := &acceleratorPullerFake{result: &registry.PullResult{
		Manifest: &registry.DescriptorPullSummary{Digest: request.ChartDigest, Data: manifest},
		Config:   &registry.DescriptorPullSummary{Digest: "sha256:" + strings.Repeat("c", 64)},
		Chart:    &registry.DescriptorPullSummaryWithMeta{DescriptorPullSummary: registry.DescriptorPullSummary{Data: archive}, Meta: loaded.Metadata},
	}}
	got, err := pullAcceleratorChart(context.Background(), puller, chartRequest)
	if err != nil || got.Metadata.Name != "kubikles-accelerator" || puller.ref != request.ChartReference {
		t.Fatalf("pull mismatch: chart=%#v err=%v ref=%q", got, err, puller.ref)
	}

	mutations := []struct {
		name string
		edit func(*registry.PullResult)
	}{
		{name: "manifest digest", edit: func(result *registry.PullResult) { result.Manifest.Digest = "sha256:" + strings.Repeat("e", 64) }},
		{name: "extra layer", edit: func(result *registry.PullResult) {
			var body map[string]interface{}
			_ = json.Unmarshal(result.Manifest.Data, &body)
			body["layers"] = append(body["layers"].([]interface{}), map[string]interface{}{"mediaType": registry.ChartLayerMediaType})
			result.Manifest.Data, _ = json.Marshal(body)
		}},
		{name: "missing schema", edit: func(result *registry.PullResult) {
			bad, _ := acceleratorTestChart(t)
			bad.Schema = nil
			result.Chart.Data = acceleratorArchive(t, bad)
		}},
		{name: "wrong metadata", edit: func(result *registry.PullResult) {
			bad, _ := acceleratorTestChart(t)
			bad.Metadata.Name = "other"
			result.Chart.Data = acceleratorArchive(t, bad)
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			copyResult := *puller.result
			manifestCopy, chartCopy := *puller.result.Manifest, *puller.result.Chart
			manifestCopy.Data = append([]byte(nil), manifestCopy.Data...)
			chartCopy.Data = append([]byte(nil), chartCopy.Data...)
			copyResult.Manifest, copyResult.Chart = &manifestCopy, &chartCopy
			mutation.edit(&copyResult)
			_, gotErr := pullAcceleratorChart(context.Background(), &acceleratorPullerFake{result: &copyResult}, chartRequest)
			if !errors.Is(gotErr, ErrAcceleratorIntegrity) {
				t.Fatalf("error=%v", gotErr)
			}
		})
	}
	for _, bad := range []AcceleratorChartRequest{
		{Reference: "http://ghcr.io/skrobylabs/helm/kubikles-accelerator@" + request.ChartDigest, Digest: request.ChartDigest, BuildVersion: request.BuildVersion},
		{Reference: acceleratorChartRepository + "v1.2.3", Digest: request.ChartDigest, BuildVersion: request.BuildVersion},
		{Reference: request.ChartReference, Digest: request.ChartDigest, BuildVersion: "v01.2.3"},
	} {
		if _, gotErr := pullAcceleratorChart(context.Background(), &acceleratorPullerFake{}, bad); !errors.Is(gotErr, ErrAcceleratorIntegrity) {
			t.Fatalf("accepted malformed request: %#v", bad)
		}
	}
	source, err := os.ReadFile("accelerator.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"\"os/exec\"", ".locateOCIChart(", ".UpgradeRelease("} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("strict path contains forbidden operation %q", forbidden)
		}
	}
}

func TestAcceleratorReleaseRequestNeverFormatsOrSerializesVerifier(t *testing.T) {
	request := acceleratorTestRequest()
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
		if got := fmt.Sprintf(verb, request); strings.Contains(got, request.CreatorVerifier) {
			t.Fatalf("%s leaked verifier: %q", verb, got)
		}
	}
	if _, err := json.Marshal(request); err == nil {
		t.Fatal("request JSON serialization unexpectedly succeeded")
	}
}

func TestRenderAcceleratorReleaseExact(t *testing.T) {
	loaded, _ := acceleratorTestChart(t)
	request := acceleratorTestRequest()
	prepared, failure := prepareAcceleratorRelease(loaded, request)
	if failure != AcceleratorOK || prepared == nil || prepared.JobName() == "" || len(prepared.ResourceIdentities()) != 5 || len(prepared.renderHash) != 64 {
		t.Fatalf("prepare failure=%s prepared=%#v", failure, prepared)
	}
	if got := countLeaves(prepared.values); got != 5 {
		t.Fatalf("values leaves=%d", got)
	}
	if strings.Contains(prepared.manifest, request.CreatorVerifier) || strings.Count(prepared.manifest, "dzJnTHJYTk5JTEdETG1SRHl6bTJzQW12UnNkdV9mYlFwem1yeVBLLWhsTQ==") != 1 {
		t.Fatal("verifier isolation mismatch")
	}
	mutations := map[string]func(string) string{
		"extra": func(manifest string) string {
			return manifest + "\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: extra\n"
		},
		"kind": func(manifest string) string { return strings.Replace(manifest, "kind: Job", "kind: Deployment", 1) },
		"session": func(manifest string) string {
			return strings.Replace(manifest, request.WorkloadSession, strings.Repeat("f", 32), 1)
		},
		"build": func(manifest string) string { return strings.Replace(manifest, request.BuildVersion, "v9.9.9", 1) },
		"image": func(manifest string) string {
			return strings.Replace(manifest, request.ImageDigest, "sha256:"+strings.Repeat("e", 64), 1)
		},
		"secret ref": func(manifest string) string {
			return strings.Replace(manifest, prepared.verifierName, "wrong-verifier", 1)
		},
		"hook": func(manifest string) string {
			return strings.Replace(manifest, "annotations:\n", "annotations:\n    helm.sh/hook: pre-install\n", 1)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			if _, _, _, ok := validateAcceleratorManifest(mutate(prepared.manifest), request); ok {
				t.Fatal("mutation accepted")
			}
		})
	}
	badValues := request
	badValues.ImageRepository = "example.invalid/other"
	if prepared, failure := prepareAcceleratorRelease(loaded, badValues); prepared != nil || failure != AcceleratorIntegrity {
		t.Fatalf("accepted caller-selected repository: %#v %s", prepared, failure)
	}

	t.Run("schema is the exact closed chart contract", func(t *testing.T) {
		bad := *loaded
		bad.Schema = []byte(`{"type":"object","additionalProperties":true}`)
		if prepared, failure := prepareAcceleratorRelease(&bad, request); prepared != nil || failure != AcceleratorIntegrity {
			t.Fatalf("accepted permissive schema: %#v %s", prepared, failure)
		}
	})

	for name, addition := range map[string]string{
		"job active deadline": "  activeDeadlineSeconds: 60\n",
		"pod host network":    "      hostNetwork: true\n",
		"container command":   "          command: [sh]\n",
		"container args":      "          args: [unexpected]\n",
	} {
		t.Run("reject extra "+name, func(t *testing.T) {
			manifest := prepared.manifest
			switch name {
			case "job active deadline":
				manifest = strings.Replace(manifest, "spec:\n  completions:", "spec:\n"+addition+"  completions:", 1)
			case "pod host network":
				manifest = strings.Replace(manifest, "    spec:\n      serviceAccountName:", "    spec:\n"+addition+"      serviceAccountName:", 1)
			default:
				manifest = strings.Replace(manifest, "          image:", addition+"          image:", 1)
			}
			if _, _, _, ok := validateAcceleratorManifest(manifest, request); ok {
				t.Fatal("extra field accepted")
			}
		})
	}
}

func TestInstallAcceleratorReleaseFreshOnly(t *testing.T) {
	loaded, _ := acceleratorTestChart(t)
	prepared, failure := prepareAcceleratorRelease(loaded, acceleratorTestRequest())
	if failure != AcceleratorOK {
		t.Fatal(failure)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	actionConfig := acceleratorTestActionConfig()
	configured := configureAcceleratorInstall(ctx, actionConfig, prepared)
	if configured.CreateNamespace || configured.Replace || configured.Force || configured.Atomic || configured.Wait || configured.WaitForJobs || !configured.DisableHooks || !configured.SkipCRDs || configured.TakeOwnership || configured.Namespace != prepared.request.ReleaseNamespace || configured.ReleaseName != prepared.request.ReleaseName {
		t.Fatalf("unsafe install flags: %#v", configured)
	}
	receipt, failure := installAcceleratorWithActionConfig(ctx, actionConfig, prepared)
	if failure != AcceleratorOK || receipt == nil {
		t.Fatalf("install failure=%s receipt=%#v", failure, receipt)
	}
	stored, err := actionConfig.Releases.Get(prepared.request.ReleaseName, 1)
	if err != nil || proveAcceleratorOwnership(stored, prepared) == nil {
		t.Fatal("stored release was not exact revision-one ownership")
	}

	for _, status := range []release.Status{release.StatusDeployed, release.StatusFailed, release.StatusUninstalled, release.StatusPendingInstall} {
		t.Run(status.String(), func(t *testing.T) {
			config := acceleratorTestActionConfig()
			existing := &release.Release{Name: prepared.request.ReleaseName, Namespace: prepared.request.ReleaseNamespace, Version: 1, Info: &release.Info{Status: status}}
			if err := config.Releases.Create(existing); err != nil {
				t.Fatal(err)
			}
			gotReceipt, gotFailure := installAcceleratorWithActionConfig(ctx, config, prepared)
			if gotReceipt != nil || gotFailure != AcceleratorConflict {
				t.Fatalf("existing release adopted: %#v %s", gotReceipt, gotFailure)
			}
			if _, err := config.Releases.Get(prepared.request.ReleaseName, 1); err != nil {
				t.Fatal("existing release was altered")
			}
		})
	}
	mismatch := *stored
	mismatch.Manifest += "\n# mismatch"
	if proveAcceleratorOwnership(&mismatch, prepared) != nil {
		t.Fatal("mismatched manifest proved ownership")
	}
	mismatch = *stored
	mismatch.Version = 2
	if proveAcceleratorOwnership(&mismatch, prepared) != nil {
		t.Fatal("revision two proved ownership")
	}
}

func TestInstallRejectsLiveRerenderMismatchBeforeStorage(t *testing.T) {
	loaded, _ := acceleratorTestChart(t)
	loaded.Templates = append(loaded.Templates, &chart.File{Name: "templates/live-only.yaml", Data: []byte(`{{- if .Capabilities.APIVersions.Has "evil.example/v1" }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: live-only
{{- end }}`)})
	prepared, failure := prepareAcceleratorRelease(loaded, acceleratorTestRequest())
	if failure != AcceleratorOK {
		t.Fatalf("client-only preparation failed: %s", failure)
	}
	config := acceleratorTestActionConfig()
	config.Capabilities.APIVersions = append(config.Capabilities.APIVersions, "evil.example/v1")
	recorder := &acceleratorAttemptRecorder{}
	receipt, failure := installAcceleratorWithActionConfig(context.Background(), config, prepared, recorder)
	if receipt != nil || failure != AcceleratorRender {
		t.Fatalf("live mismatch receipt=%#v failure=%s", receipt, failure)
	}
	if recorder.ownershipUnproven(failure) {
		t.Fatal("live mismatch reached the storage Create boundary")
	}
	if _, err := config.Releases.Get(prepared.request.ReleaseName, 1); !errors.Is(err, driver.ErrReleaseNotFound) {
		t.Fatalf("live mismatch created storage: %v", err)
	}
}

type acceleratorPrefixCreateKube struct {
	kube.Interface
	calls  int
	failAt int
}

func (c *acceleratorPrefixCreateKube) Create(resources kube.ResourceList) (*kube.Result, error) {
	c.calls++
	if c.calls == c.failAt {
		return nil, apierrors.NewAlreadyExists(schema.GroupResource{Group: "batch", Resource: "jobs"}, "racing")
	}
	for _, info := range resources {
		accessor, err := meta.Accessor(info.Object)
		if err != nil {
			return nil, err
		}
		accessor.SetUID(types.UID(fmt.Sprintf("created-%d", c.calls)))
	}
	return &kube.Result{Created: resources}, nil
}

func TestInstallAcceleratorReceiptRecordsOnlySuccessfulCreatePrefix(t *testing.T) {
	loaded, _ := acceleratorTestChart(t)
	prepared, failure := prepareAcceleratorRelease(loaded, acceleratorTestRequest())
	if failure != AcceleratorOK {
		t.Fatal(failure)
	}
	recorder := &acceleratorAttemptRecorder{}
	recorder.recordStorage(acceleratorStorageName(prepared.request.ReleaseName), "storage", "1")
	prefix := &acceleratorPrefixCreateKube{Interface: acceleratorTestActionConfig().KubeClient, failAt: 2}
	allowed := make(map[string]AcceleratorResourceIdentity)
	resources := make(kube.ResourceList, 0, len(prepared.resources))
	for _, identity := range prepared.resources {
		key := acceleratorResourceKey(identity.APIVersion, identity.Kind, identity.Namespace, identity.Name)
		allowed[key] = identity
		gvk := schema.FromAPIVersionAndKind(identity.APIVersion, identity.Kind)
		resources = append(resources, &resource.Info{Name: identity.Name, Namespace: identity.Namespace, Mapping: &meta.RESTMapping{GroupVersionKind: gvk}, Object: &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": identity.APIVersion, "kind": identity.Kind, "metadata": map[string]interface{}{"name": identity.Name}}}})
	}
	creator := acceleratorReceiptKubeClient{Interface: prefix, recorder: recorder, allowed: allowed}
	if _, err := creator.Create(resources); !errors.Is(err, errAcceleratorCreateConflict) {
		t.Fatalf("create error=%v", err)
	}
	receipt := recorder.receipt(prepared)
	if receipt == nil {
		t.Fatal("storage-backed receipt missing")
	}
	created := receipt.CreatedResources()
	if len(created) != 1 || created[0].UID != "created-1" || len(receipt.UnresolvedResources()) != 0 {
		t.Fatalf("created prefix=%#v", created)
	}
	for _, item := range created {
		if item.Resource.Kind == "Job" {
			t.Fatal("receipt invented a Job not created by the prefix")
		}
	}
}

func TestAcceleratorOwnershipIsExactRevisionAndStorageIdentity(t *testing.T) {
	loaded, _ := acceleratorTestChart(t)
	prepared, failure := prepareAcceleratorRelease(loaded, acceleratorTestRequest())
	if failure != AcceleratorOK {
		t.Fatal(failure)
	}
	config := acceleratorTestActionConfig()
	stored := &release.Release{Name: prepared.request.ReleaseName, Namespace: prepared.request.ReleaseNamespace, Version: 1, Manifest: prepared.manifest, Config: prepared.values, Info: &release.Info{Status: release.StatusPendingInstall}}
	if err := config.Releases.Create(stored); err != nil {
		t.Fatal(err)
	}
	storageName := acceleratorStorageName(prepared.request.ReleaseName)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: storageName, Namespace: prepared.request.ReleaseNamespace, UID: "storage-uid", ResourceVersion: "7"}}
	client := k8sfake.NewSimpleClientset(secret)
	receipt := &AcceleratorOwnershipReceipt{request: prepared.request, renderHash: prepared.renderHash, storageName: storageName, storageUID: "storage-uid", storageResourceVersion: "7"}
	if !inspectAcceleratorOwnershipWith(context.Background(), config, client, prepared, receipt) {
		t.Fatal("exact revision-one receipt rejected")
	}

	replacement := *receipt
	replacement.storageUID = "replacement"
	if inspectAcceleratorOwnershipWith(context.Background(), config, client, prepared, &replacement) {
		t.Fatal("replacement storage UID accepted")
	}
	revision2 := *stored
	revision2.Version = 2
	if err := config.Releases.Create(&revision2); err != nil {
		t.Fatal(err)
	}
	if inspectAcceleratorOwnershipWith(context.Background(), config, client, prepared, receipt) {
		t.Fatal("revision two insertion accepted")
	}
}

type acceleratorRawCreateKube struct {
	kube.Interface
	err error
}

func (c acceleratorRawCreateKube) Create(kube.ResourceList) (*kube.Result, error) {
	return nil, c.err
}

type acceleratorAcceptedLostCreateKube struct {
	kube.Interface
	err error
}

func (c acceleratorAcceptedLostCreateKube) Create(resources kube.ResourceList) (*kube.Result, error) {
	for _, info := range resources {
		accessor, err := meta.Accessor(info.Object)
		if err != nil {
			return nil, err
		}
		accessor.SetUID("server-created")
	}
	return nil, c.err
}

func TestAcceleratorCreateErrorAmbiguityClassification(t *testing.T) {
	definitive := map[string]error{
		"already exists": apierrors.NewAlreadyExists(schema.GroupResource{Resource: "jobs"}, "job"),
		"forbidden":      apierrors.NewForbidden(schema.GroupResource{Resource: "jobs"}, "job", errors.New("denied")),
		"unauthorized":   apierrors.NewUnauthorized("denied"),
		"invalid":        apierrors.NewInvalid(schema.GroupKind{Group: "batch", Kind: "Job"}, "job", field.ErrorList{field.Invalid(field.NewPath("spec"), nil, "invalid")}),
		"bad request":    apierrors.NewBadRequest("bad"),
		"conflict":       apierrors.NewConflict(schema.GroupResource{Resource: "jobs"}, "job", errors.New("conflict")),
	}
	ambiguous := map[string]error{
		"eof":              io.EOF,
		"connection reset": &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET},
		"cancelled":        context.Canceled,
		"deadline":         context.DeadlineExceeded,
		"timeout":          apierrors.NewTimeoutError("timeout", 1),
		"server timeout":   apierrors.NewServerTimeout(schema.GroupResource{Resource: "jobs"}, "create", 1),
		"internal":         apierrors.NewInternalError(errors.New("internal")),
		"service down":     apierrors.NewServiceUnavailable("down"),
		"bad gateway":      apierrors.NewGenericServerResponse(http.StatusBadGateway, http.MethodPost, schema.GroupResource{Resource: "jobs"}, "job", "gateway", 0, true),
		"unknown":          errors.New("unknown transport outcome"),
	}
	for name, err := range definitive {
		t.Run("definitive "+name, func(t *testing.T) {
			if !acceleratorCreateErrorIsDefinitive(err) {
				t.Fatal("definitive rejection classified ambiguous")
			}
		})
	}
	for name, err := range ambiguous {
		t.Run("ambiguous "+name, func(t *testing.T) {
			if acceleratorCreateErrorIsDefinitive(err) {
				t.Fatal("unknown create outcome classified definitive")
			}
		})
	}
}

func TestAcceleratorAcceptedResponseLostRemainsUnresolved(t *testing.T) {
	loaded, _ := acceleratorTestChart(t)
	prepared, failure := prepareAcceleratorRelease(loaded, acceleratorTestRequest())
	if failure != AcceleratorOK {
		t.Fatal(failure)
	}
	identity := prepared.resources[0]
	gvk := schema.FromAPIVersionAndKind(identity.APIVersion, identity.Kind)
	info := &resource.Info{Name: identity.Name, Namespace: identity.Namespace, Mapping: &meta.RESTMapping{GroupVersionKind: gvk}, Object: &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": identity.APIVersion, "kind": identity.Kind, "metadata": map[string]interface{}{"name": identity.Name}}}}
	recorder := &acceleratorAttemptRecorder{}
	recorder.recordStorage(acceleratorStorageName(prepared.request.ReleaseName), "storage", "1")
	creator := acceleratorReceiptKubeClient{
		Interface: acceleratorAcceptedLostCreateKube{Interface: acceleratorTestActionConfig().KubeClient, err: io.EOF},
		recorder:  recorder,
		allowed:   map[string]AcceleratorResourceIdentity{acceleratorResourceKey(identity.APIVersion, identity.Kind, identity.Namespace, identity.Name): identity},
	}
	if _, err := creator.Create(kube.ResourceList{info}); !errors.Is(err, errAcceleratorCreateFailed) {
		t.Fatalf("create error=%v", err)
	}
	receipt := recorder.receipt(prepared)
	if receipt == nil || len(receipt.UnresolvedResources()) != 1 || len(receipt.CreatedResources()) != 0 {
		t.Fatalf("ambiguous receipt=%#v", receipt)
	}
}

func TestAcceleratorRawErrorCorpusNeverPersists(t *testing.T) {
	rawRegistry := "registry-body-T11-secret"
	request := acceleratorTestRequest()
	chartRequest := AcceleratorChartRequest{Reference: request.ChartReference, Digest: request.ChartDigest, BuildVersion: request.BuildVersion}
	if _, err := pullAcceleratorChart(context.Background(), &acceleratorPullerFake{err: errors.New(rawRegistry)}, chartRequest); !errors.Is(err, ErrAcceleratorUnavailable) || strings.Contains(err.Error(), rawRegistry) {
		t.Fatalf("registry error escaped: %v", err)
	}

	loaded, _ := acceleratorTestChart(t)
	prepared, failure := prepareAcceleratorRelease(loaded, request)
	if failure != AcceleratorOK {
		t.Fatal(failure)
	}
	rawCreate := "kubernetes-status-T11-secret"
	identity := prepared.resources[0]
	gvk := schema.FromAPIVersionAndKind(identity.APIVersion, identity.Kind)
	info := &resource.Info{Name: identity.Name, Namespace: identity.Namespace, Mapping: &meta.RESTMapping{GroupVersionKind: gvk}, Object: &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": identity.APIVersion, "kind": identity.Kind, "metadata": map[string]interface{}{"name": identity.Name}}}}
	recorder := &acceleratorAttemptRecorder{}
	recorder.recordStorageAttempt()
	if !recorder.ownershipUnproven(AcceleratorPermission) || recorder.ownershipUnproven(AcceleratorConflict) {
		t.Fatal("ambiguous storage classification did not preserve conflict boundary")
	}
	creator := acceleratorReceiptKubeClient{
		Interface: acceleratorRawCreateKube{Interface: acceleratorTestActionConfig().KubeClient, err: errors.New(rawCreate)},
		recorder:  recorder,
		allowed:   map[string]AcceleratorResourceIdentity{acceleratorResourceKey(identity.APIVersion, identity.Kind, identity.Namespace, identity.Name): identity},
	}
	if _, err := creator.Create(kube.ResourceList{info}); !errors.Is(err, errAcceleratorCreateFailed) || strings.Contains(err.Error(), rawCreate) {
		t.Fatalf("Kubernetes error escaped: %v", err)
	}

	rawHelm := "helm-failure-manifest-value-T11-secret"
	config := acceleratorTestActionConfig()
	stored := &release.Release{Name: request.ReleaseName, Namespace: request.ReleaseNamespace, Version: 1, Manifest: prepared.manifest, Config: prepared.values, Info: &release.Info{Status: release.StatusPendingInstall, Description: "Initial install underway"}}
	if err := config.Releases.Create(stored); err != nil {
		t.Fatal(err)
	}
	config.Releases.Driver = acceleratorAttemptDriver{Driver: config.Releases.Driver, recorder: recorder}
	failed := *stored
	failed.Info = &release.Info{Status: release.StatusFailed, Description: rawHelm}
	if err := config.Releases.Update(&failed); err != nil {
		t.Fatal(err)
	}
	decoded, err := config.Releases.Get(request.ReleaseName, 1)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Info.Status != release.StatusPendingInstall || strings.Contains(string(encoded), rawHelm) {
		t.Fatalf("raw Helm failure persisted in decoded release state: %s", encoded)
	}

	hostileValues := request
	hostileValues.CreatorVerifier = rawHelm
	if got, failure := prepareAcceleratorRelease(loaded, hostileValues); got != nil || failure != AcceleratorIntegrity {
		t.Fatal("hostile value was accepted")
	}
	hostileChart, _ := acceleratorTestChart(t)
	hostileChart.Templates = append(hostileChart.Templates, &chart.File{Name: "templates/hostile.yaml", Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: hostile\ndata:\n  raw: " + rawHelm + "\n")})
	if got, failure := prepareAcceleratorRelease(hostileChart, request); got != nil || failure != AcceleratorRender {
		t.Fatalf("hostile manifest failure=%s prepared=%#v", failure, got)
	}
}

func acceleratorTestRequest() AcceleratorReleaseRequest {
	chartDigest := "sha256:" + strings.Repeat("b", 64)
	return AcceleratorReleaseRequest{
		ChartReference: acceleratorChartRepository + chartDigest, ChartDigest: chartDigest,
		BuildVersion: "v1.2.3", ImageRepository: acceleratorImageRepository,
		ImageDigest: "sha256:" + strings.Repeat("a", 64), WorkloadSession: "202122232425262728292a2b2c2d2e2f",
		CreatorVerifier: "w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM",
		ReleaseName:     "kubikles-accelerator-202122232425262728292a2b2c2d2e2f", ReleaseNamespace: "default",
	}
}

func acceleratorTestChart(t *testing.T) (*chart.Chart, []byte) {
	t.Helper()
	loaded, err := loader.Load(filepath.Join("..", "..", "deploy", "charts", "kubikles-accelerator"))
	if err != nil {
		t.Fatal(err)
	}
	loaded.Metadata.Version = "1.2.3"
	loaded.Metadata.AppVersion = "v1.2.3"
	return loaded, acceleratorArchive(t, loaded)
}

func acceleratorArchive(t *testing.T, loaded *chart.Chart) []byte {
	t.Helper()
	path, err := chartutil.Save(loaded, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func acceleratorTestActionConfig() *action.Configuration {
	memoryDriver := driver.NewMemory()
	memoryDriver.SetNamespace("default")
	return &action.Configuration{
		Releases: storage.Init(memoryDriver), KubeClient: &fake.PrintingKubeClient{Out: io.Discard, LogOutput: io.Discard},
		Capabilities: chartutil.DefaultCapabilities.Copy(), Log: func(string, ...interface{}) {},
	}
}

func countLeaves(value interface{}) int {
	switch typed := value.(type) {
	case map[string]interface{}:
		total := 0
		for _, child := range typed {
			total += countLeaves(child)
		}
		return total
	default:
		return 1
	}
}
