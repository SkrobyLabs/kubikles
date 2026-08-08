//go:build helm

package helm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/cli-runtime/pkg/resource"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type acceleratorPullerFake struct {
	ref    string
	result *registry.PullResult
	err    error
}

type acceleratorHostileDriver struct {
	driver.Driver
	queryErr  error
	deleteErr error
}

func (d acceleratorHostileDriver) Query(labels map[string]string) ([]*release.Release, error) {
	if d.queryErr != nil {
		return nil, d.queryErr
	}
	return d.Driver.Query(labels)
}

func (d acceleratorHostileDriver) Delete(key string) (*release.Release, error) {
	if d.deleteErr != nil {
		return nil, d.deleteErr
	}
	return d.Driver.Delete(key)
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
	if got := countLeaves(prepared.values); got != 6 {
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
		"architecture": func(manifest string) string {
			return strings.Replace(manifest, "kubernetes.io/arch: \"amd64\"", "kubernetes.io/arch: \"arm64\"", 1)
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
	stored := &release.Release{Name: prepared.request.ReleaseName, Namespace: prepared.request.ReleaseNamespace, Version: 1, Chart: prepared.chart, Manifest: prepared.manifest, Config: prepared.values, Info: &release.Info{Status: release.StatusPendingInstall}}
	if err := config.Releases.Create(stored); err != nil {
		t.Fatal(err)
	}
	storageName := acceleratorStorageName(prepared.request.ReleaseName)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: storageName, Namespace: prepared.request.ReleaseNamespace, UID: "storage-uid", ResourceVersion: "7"}}
	client := k8sfake.NewSimpleClientset(secret)
	created := make([]AcceleratorDeletionIdentity, 0, len(prepared.resources))
	for _, identity := range prepared.resources {
		created = append(created, AcceleratorDeletionIdentity{Resource: identity, UID: types.UID("uid-" + identity.Name)})
	}
	receipt := &AcceleratorOwnershipReceipt{request: prepared.request, renderHash: prepared.renderHash, storageName: storageName, storageUID: "storage-uid", storageResourceVersion: "7", created: created}
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

func TestConfigureAcceleratorOwnedUninstallExactFlags(t *testing.T) {
	configured := configureAcceleratorOwnedUninstall(acceleratorTestActionConfig())
	if configured == nil || !configured.DisableHooks || configured.KeepHistory || !configured.Wait || configured.Timeout != 45*time.Second || configured.DeletionPropagation != "foreground" || configured.IgnoreNotFound || configured.DryRun {
		t.Fatalf("unsafe owned uninstall flags: %#v", configured)
	}
}

type acceleratorSweepTestFixture struct {
	prepared        *AcceleratorPreparedRelease
	config          *action.Configuration
	job             *batchv1.Job
	secret, storage *corev1.Secret
	account         *corev1.ServiceAccount
	role            *rbacv1.ClusterRole
	binding         *rbacv1.ClusterRoleBinding
	pod             *corev1.Pod
}

func newAcceleratorSweepTestFixture(t *testing.T) *acceleratorSweepTestFixture {
	t.Helper()
	loaded, _ := acceleratorTestChart(t)
	prepared, failure := prepareAcceleratorRelease(loaded, acceleratorTestRequest())
	if failure != AcceleratorOK {
		t.Fatal(failure)
	}
	config := acceleratorTestActionConfig()
	stored := &release.Release{Name: prepared.request.ReleaseName, Namespace: prepared.request.ReleaseNamespace, Version: 1, Chart: prepared.chart, Config: prepared.values, Manifest: prepared.manifest, Info: &release.Info{Status: release.StatusPendingInstall}}
	if err := config.Releases.Create(stored); err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, identity := range prepared.resources {
		names[identity.Kind] = identity.Name
	}
	metadata := func(name, namespace string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID("uid-" + name), Labels: acceleratorLabels(prepared.request.ReleaseName), Annotations: map[string]string{"meta.helm.sh/release-name": prepared.request.ReleaseName, "meta.helm.sh/release-namespace": prepared.request.ReleaseNamespace}}
	}
	immutable, automount, controller := true, false, true
	jobMeta := metadata(names["Job"], prepared.request.ReleaseNamespace)
	jobMeta.Labels["kubikles.io/workload-session-id"] = prepared.request.WorkloadSession
	jobMeta.Annotations["kubikles.io/build-version"] = prepared.request.BuildVersion
	one, zero, ttl, grace, user, mode, expiration := int32(1), int32(0), int32(3600), int64(30), int64(65532), int32(292), int64(3600)
	runNonRoot, allowEscalation, readOnly := true, false, true
	podSpec := corev1.PodSpec{
		ServiceAccountName: names["ServiceAccount"], AutomountServiceAccountToken: &automount, RestartPolicy: corev1.RestartPolicyNever, TerminationGracePeriodSeconds: &grace,
		NodeSelector:    map[string]string{"kubernetes.io/arch": "amd64"},
		SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: &runNonRoot, RunAsUser: &user, RunAsGroup: &user, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
		Containers: []corev1.Container{{Name: "accelerator", Image: prepared.request.ImageRepository + "@" + prepared.request.ImageDigest, ImagePullPolicy: corev1.PullIfNotPresent,
			Env:             []corev1.EnvVar{{Name: "KUBIKLES_ACCELERATOR_CREATOR_VERIFIER", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: names["Secret"]}, Key: "creatorVerifier"}}}},
			SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &allowEscalation, ReadOnlyRootFilesystem: &readOnly, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
			Resources:       corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: apiresource.MustParse("100m"), corev1.ResourceMemory: apiresource.MustParse("128Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: apiresource.MustParse("1"), corev1.ResourceMemory: apiresource.MustParse("512Mi")}},
			VolumeMounts:    []corev1.VolumeMount{{Name: "serviceaccount", MountPath: "/var/run/secrets/kubernetes.io/serviceaccount", ReadOnly: true}}}},
		Volumes: []corev1.Volume{{Name: "serviceaccount", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{DefaultMode: &mode, Sources: []corev1.VolumeProjection{
			{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{Path: "token", ExpirationSeconds: &expiration}},
			{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "kube-root-ca.crt"}, Items: []corev1.KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}}},
			{DownwardAPI: &corev1.DownwardAPIProjection{Items: []corev1.DownwardAPIVolumeFile{{Path: "namespace", FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}}}},
		}}}}},
	}
	templateMeta := metav1.ObjectMeta{Labels: acceleratorLabels(prepared.request.ReleaseName), Annotations: map[string]string{"kubikles.io/build-version": prepared.request.BuildVersion}}
	templateMeta.Labels["kubikles.io/workload-session-id"] = prepared.request.WorkloadSession
	job := &batchv1.Job{ObjectMeta: jobMeta, Spec: batchv1.JobSpec{Completions: &one, Parallelism: &one, BackoffLimit: &zero, TTLSecondsAfterFinished: &ttl, Template: corev1.PodTemplateSpec{ObjectMeta: templateMeta, Spec: *podSpec.DeepCopy()}}, Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}}
	secret := &corev1.Secret{ObjectMeta: metadata(names["Secret"], prepared.request.ReleaseNamespace), Immutable: &immutable, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{"creatorVerifier": []byte(prepared.request.CreatorVerifier)}}
	account := &corev1.ServiceAccount{ObjectMeta: metadata(names["ServiceAccount"], prepared.request.ReleaseNamespace), AutomountServiceAccountToken: &automount}
	role := &rbacv1.ClusterRole{ObjectMeta: metadata(names["ClusterRole"], ""), Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get", "list", "watch"}}}}
	binding := &rbacv1.ClusterRoleBinding{ObjectMeta: metadata(names["ClusterRoleBinding"], ""), RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: names["ClusterRole"]}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: names["ServiceAccount"], Namespace: prepared.request.ReleaseNamespace}}}
	podMeta := metadata("completed-pod", prepared.request.ReleaseNamespace)
	delete(podMeta.Annotations, "meta.helm.sh/release-name")
	delete(podMeta.Annotations, "meta.helm.sh/release-namespace")
	podMeta.Labels["kubikles.io/workload-session-id"] = prepared.request.WorkloadSession
	podMeta.Annotations["kubikles.io/build-version"] = prepared.request.BuildVersion
	podMeta.OwnerReferences = []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller, BlockOwnerDeletion: &controller}}
	pod := &corev1.Pod{ObjectMeta: podMeta, Spec: *podSpec.DeepCopy(), Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}
	applyKubernetes132LivePodTuple(&pod.Spec)
	storageSecret := &corev1.Secret{ObjectMeta: metadata(acceleratorStorageName(prepared.request.ReleaseName), prepared.request.ReleaseNamespace)}
	return &acceleratorSweepTestFixture{prepared: prepared, config: config, job: job, secret: secret, account: account, role: role, binding: binding, pod: pod, storage: storageSecret}
}

func (f *acceleratorSweepTestFixture) client(objects ...runtime.Object) *k8sfake.Clientset {
	if objects == nil {
		objects = []runtime.Object{f.job, f.secret, f.account, f.role, f.binding, f.pod, f.storage}
	}
	return k8sfake.NewSimpleClientset(objects...)
}

func (f *acceleratorSweepTestFixture) inspect(client *k8sfake.Clientset) (*AcceleratorSweepCandidate, AcceleratorSweepProofStatus) {
	return inspectAcceleratorSweepCandidateWith(context.Background(), f.config, client, f.prepared.request.ReleaseNamespace, f.prepared.request.ReleaseName)
}

func (f *acceleratorSweepTestFixture) ownedReceipt() *AcceleratorOwnershipReceipt {
	created := make([]AcceleratorDeletionIdentity, 0, 5)
	objects := map[string]metav1.Object{"Job": f.job, "Secret": f.secret, "ServiceAccount": f.account, "ClusterRole": f.role, "ClusterRoleBinding": f.binding}
	for _, identity := range f.prepared.resources {
		created = append(created, AcceleratorDeletionIdentity{Resource: identity, UID: objects[identity.Kind].GetUID()})
	}
	return &AcceleratorOwnershipReceipt{request: f.prepared.request, renderHash: f.prepared.renderHash, storageName: f.storage.Name, storageUID: f.storage.UID, storageResourceVersion: f.storage.ResourceVersion, created: created}
}

func TestOwnedFinalProofRejectsEveryStoredContractMutationBeforeRun(t *testing.T) {
	mutations := map[string]func(*release.Release){
		"status":        func(r *release.Release) { r.Info.Status = release.StatusDeployed },
		"revision":      func(r *release.Release) { r.Version = 2 },
		"chart name":    func(r *release.Release) { r.Chart.Metadata.Name = "other" },
		"chart type":    func(r *release.Release) { r.Chart.Metadata.Type = "library" },
		"chart version": func(r *release.Release) { r.Chart.Metadata.Version = "v1.2.3" },
		"app version":   func(r *release.Release) { r.Chart.Metadata.AppVersion = "1.2.3" },
		"hook":          func(r *release.Release) { r.Hooks = []*release.Hook{{Name: "hostile"}} },
		"config":        func(r *release.Release) { r.Config["extra"] = true },
		"manifest":      func(r *release.Release) { r.Manifest += "\n# changed" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := newAcceleratorSweepTestFixture(t)
			fixture.storage.ResourceVersion = "7"
			stored, err := fixture.config.Releases.Get(fixture.prepared.request.ReleaseName, 1)
			if err != nil {
				t.Fatal(err)
			}
			mutate(stored)
			_ = fixture.config.Releases.Update(stored)
			pod := AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "v1", Kind: "Pod", Namespace: fixture.pod.Namespace, Name: fixture.pod.Name}, UID: fixture.pod.UID}
			status := uninstallOwnedAcceleratorReleaseWith(context.Background(), fixture.config, fixture.client(), fixture.prepared, fixture.ownedReceipt(), pod)
			if status != AcceleratorOwnedCleanupOwnershipChanged {
				t.Fatalf("status=%s", status)
			}
			if _, err := fixture.config.Releases.Get(fixture.prepared.request.ReleaseName, 1); err != nil {
				t.Fatalf("invalid final proof reached Run: %v", err)
			}
		})
	}
}

func TestOwnedFinalLiveProofRejectsChartControlledMutationBeforeRun(t *testing.T) {
	mutations := map[string]func(*acceleratorSweepTestFixture){
		"job template label": func(f *acceleratorSweepTestFixture) { delete(f.job.Spec.Template.Labels, "app.kubernetes.io/name") },
		"job template annotation": func(f *acceleratorSweepTestFixture) {
			delete(f.job.Spec.Template.Annotations, "kubikles.io/build-version")
		},
		"job completions":      func(f *acceleratorSweepTestFixture) { *f.job.Spec.Completions = 2 },
		"job parallelism":      func(f *acceleratorSweepTestFixture) { *f.job.Spec.Parallelism = 2 },
		"job backoff":          func(f *acceleratorSweepTestFixture) { *f.job.Spec.BackoffLimit = 1 },
		"job ttl":              func(f *acceleratorSweepTestFixture) { *f.job.Spec.TTLSecondsAfterFinished = 1 },
		"service account name": func(f *acceleratorSweepTestFixture) { f.job.Spec.Template.Spec.ServiceAccountName = "other" },
		"automount":            func(f *acceleratorSweepTestFixture) { *f.job.Spec.Template.Spec.AutomountServiceAccountToken = true },
		"restart policy": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
		},
		"termination grace": func(f *acceleratorSweepTestFixture) { *f.job.Spec.Template.Spec.TerminationGracePeriodSeconds = 29 },
		"host network":      func(f *acceleratorSweepTestFixture) { f.job.Spec.Template.Spec.HostNetwork = true },
		"Job template node name": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.NodeName = "kind-control-plane"
		},
		"Job template node selector": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.NodeSelector["kubernetes.io/arch"] = "arm64"
		},
		"Job template priority": func(f *acceleratorSweepTestFixture) {
			priority := int32(0)
			f.job.Spec.Template.Spec.Priority = &priority
		},
		"Job template preemption": func(f *acceleratorSweepTestFixture) {
			preemption := corev1.PreemptLowerPriority
			f.job.Spec.Template.Spec.PreemptionPolicy = &preemption
		},
		"Job template default tolerations": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Tolerations = acceleratorKubernetes132DefaultTolerations()
		},
		"init container": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.InitContainers = []corev1.Container{{Name: "extra"}}
		},
		"pod run as non-root": func(f *acceleratorSweepTestFixture) { *f.job.Spec.Template.Spec.SecurityContext.RunAsNonRoot = false },
		"pod run as user":     func(f *acceleratorSweepTestFixture) { *f.job.Spec.Template.Spec.SecurityContext.RunAsUser = 1 },
		"pod run as group":    func(f *acceleratorSweepTestFixture) { *f.job.Spec.Template.Spec.SecurityContext.RunAsGroup = 1 },
		"pod seccomp": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.SecurityContext.SeccompProfile.Type = corev1.SeccompProfileTypeUnconfined
		},
		"container name":  func(f *acceleratorSweepTestFixture) { f.job.Spec.Template.Spec.Containers[0].Name = "other" },
		"container image": func(f *acceleratorSweepTestFixture) { f.job.Spec.Template.Spec.Containers[0].Image = "other" },
		"image pull policy": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Containers[0].ImagePullPolicy = corev1.PullAlways
		},
		"container command": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Containers[0].Command = []string{"other"}
		},
		"container port": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{ContainerPort: 1}}
		},
		"env name":    func(f *acceleratorSweepTestFixture) { f.job.Spec.Template.Spec.Containers[0].Env[0].Name = "other" },
		"env literal": func(f *acceleratorSweepTestFixture) { f.job.Spec.Template.Spec.Containers[0].Env[0].Value = "other" },
		"env secret": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Containers[0].Env[0].ValueFrom.SecretKeyRef.Name = "other"
		},
		"env key": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Containers[0].Env[0].ValueFrom.SecretKeyRef.Key = "other"
		},
		"allow escalation": func(f *acceleratorSweepTestFixture) {
			*f.job.Spec.Template.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation = true
		},
		"writable root": func(f *acceleratorSweepTestFixture) {
			*f.job.Spec.Template.Spec.Containers[0].SecurityContext.ReadOnlyRootFilesystem = false
		},
		"capability drop": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Containers[0].SecurityContext.Capabilities.Drop = nil
		},
		"resource request": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = apiresource.MustParse("101m")
		},
		"resource limit": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceMemory] = apiresource.MustParse("513Mi")
		},
		"mount name": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name = "other"
		},
		"mount path": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath = "/other"
		},
		"mount read-only": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Containers[0].VolumeMounts[0].ReadOnly = false
		},
		"volume name":            func(f *acceleratorSweepTestFixture) { f.job.Spec.Template.Spec.Volumes[0].Name = "other" },
		"projected default mode": func(f *acceleratorSweepTestFixture) { *f.job.Spec.Template.Spec.Volumes[0].Projected.DefaultMode = 420 },
		"token path": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Volumes[0].Projected.Sources[0].ServiceAccountToken.Path = "other"
		},
		"token expiry": func(f *acceleratorSweepTestFixture) {
			*f.job.Spec.Template.Spec.Volumes[0].Projected.Sources[0].ServiceAccountToken.ExpirationSeconds = 1
		},
		"root ca name": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Volumes[0].Projected.Sources[1].ConfigMap.Name = "other"
		},
		"root ca key": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Volumes[0].Projected.Sources[1].ConfigMap.Items[0].Key = "other"
		},
		"namespace projection path": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Volumes[0].Projected.Sources[2].DownwardAPI.Items[0].Path = "other"
		},
		"namespace field": func(f *acceleratorSweepTestFixture) {
			f.job.Spec.Template.Spec.Volumes[0].Projected.Sources[2].DownwardAPI.Items[0].FieldRef.FieldPath = "metadata.name"
		},
		"verifier value":       func(f *acceleratorSweepTestFixture) { f.secret.Data["creatorVerifier"] = []byte("other") },
		"pod build annotation": func(f *acceleratorSweepTestFixture) { f.pod.Annotations["kubikles.io/build-version"] = "v9.9.9" },
		"pod image":            func(f *acceleratorSweepTestFixture) { f.pod.Spec.Containers[0].Image = "other" },
		"pod nonzero priority": func(f *acceleratorSweepTestFixture) {
			priority := int32(1)
			f.pod.Spec.Priority = &priority
		},
		"pod priority class": func(f *acceleratorSweepTestFixture) {
			priority := int32(0)
			f.pod.Spec.Priority = &priority
			f.pod.Spec.PriorityClassName = "hostile"
		},
		"pod missing priority": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.Priority = nil
		},
		"pod missing preemption": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.PreemptionPolicy = nil
		},
		"pod wrong preemption": func(f *acceleratorSweepTestFixture) {
			preemption := corev1.PreemptNever
			f.pod.Spec.PreemptionPolicy = &preemption
		},
		"pod missing default tolerations": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.Tolerations = nil
		},
		"pod partial default tolerations": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.Tolerations = acceleratorKubernetes132DefaultTolerations()[:1]
		},
		"pod wrong default toleration key": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.Tolerations[0].Key = "hostile"
		},
		"pod wrong default toleration effect": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.Tolerations[0].Effect = corev1.TaintEffectNoSchedule
		},
		"pod wrong default toleration operator": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.Tolerations[0].Operator = corev1.TolerationOpEqual
		},
		"pod wrong default toleration value": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.Tolerations[0].Value = "hostile"
		},
		"pod nil default toleration seconds": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.Tolerations[0].TolerationSeconds = nil
		},
		"pod altered default toleration": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.Tolerations = acceleratorKubernetes132DefaultTolerations()
			seconds := int64(299)
			f.pod.Spec.Tolerations[0].TolerationSeconds = &seconds
		},
		"pod extra toleration": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.Tolerations = append(acceleratorKubernetes132DefaultTolerations(), corev1.Toleration{Key: "hostile", Operator: corev1.TolerationOpExists})
		},
		"pod node selector": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.NodeSelector = map[string]string{"hostile": "true"}
		},
		"pod empty node name": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.NodeName = ""
		},
		"pod env reference": func(f *acceleratorSweepTestFixture) {
			f.pod.Spec.Containers[0].Env[0].ValueFrom.SecretKeyRef.Name = "other"
		},
		"pod volume":      func(f *acceleratorSweepTestFixture) { f.pod.Spec.Volumes[0].Name = "other" },
		"pod owner api":   func(f *acceleratorSweepTestFixture) { f.pod.OwnerReferences[0].APIVersion = "v1" },
		"pod owner block": func(f *acceleratorSweepTestFixture) { *f.pod.OwnerReferences[0].BlockOwnerDeletion = false },
		"service account": func(f *acceleratorSweepTestFixture) { *f.account.AutomountServiceAccountToken = true },
		"service account secret": func(f *acceleratorSweepTestFixture) {
			f.account.Secrets = []corev1.ObjectReference{{Name: "unexpected"}}
		},
		"service account image pull secret": func(f *acceleratorSweepTestFixture) {
			f.account.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "unexpected"}}
		},
		"role api group":        func(f *acceleratorSweepTestFixture) { f.role.Rules[0].APIGroups = []string{"apps"} },
		"role aggregation rule": func(f *acceleratorSweepTestFixture) { f.role.AggregationRule = &rbacv1.AggregationRule{} },
		"role non-resource URL": func(f *acceleratorSweepTestFixture) { f.role.Rules[0].NonResourceURLs = []string{"/healthz"} },
		"binding subject":       func(f *acceleratorSweepTestFixture) { f.binding.Subjects[0].Name = "other" },
		"binding subject API group": func(f *acceleratorSweepTestFixture) {
			f.binding.Subjects[0].APIGroup = rbacv1.GroupName
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := newAcceleratorSweepTestFixture(t)
			fixture.storage.ResourceVersion = "7"
			mutate(fixture)
			client := fixture.client()
			pod := AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "v1", Kind: "Pod", Namespace: fixture.pod.Namespace, Name: fixture.pod.Name}, UID: fixture.pod.UID}
			if status := uninstallOwnedAcceleratorReleaseWith(context.Background(), fixture.config, client, fixture.prepared, fixture.ownedReceipt(), pod); status != AcceleratorOwnedCleanupOwnershipChanged {
				t.Fatalf("status=%s", status)
			}
			if _, err := fixture.config.Releases.Get(fixture.prepared.request.ReleaseName, 1); err != nil {
				t.Fatalf("live mutation reached Helm Run: %v", err)
			}
		})
	}
}

func acceleratorKubernetes132DefaultTolerations() []corev1.Toleration {
	seconds := int64(300)
	return []corev1.Toleration{
		{Key: corev1.TaintNodeNotReady, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute, TolerationSeconds: &seconds},
		{Key: corev1.TaintNodeUnreachable, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute, TolerationSeconds: &seconds},
	}
}

func applyKubernetes132LivePodTuple(spec *corev1.PodSpec) {
	priority := int32(0)
	preemption := corev1.PreemptLowerPriority
	spec.Priority = &priority
	spec.PreemptionPolicy = &preemption
	spec.Tolerations = acceleratorKubernetes132DefaultTolerations()
	spec.NodeName = "kind-control-plane"
}

func applyKubernetes132CompletedLiveMutations(fixture *acceleratorSweepTestFixture) {
	manual, suspend, serviceLinks := false, false, true
	completionMode, replacementPolicy := batchv1.NonIndexedCompletion, batchv1.TerminatingOrFailed
	fixture.job.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"batch.kubernetes.io/controller-uid": string(fixture.job.UID)}}
	fixture.job.Spec.ManualSelector = &manual
	fixture.job.Spec.CompletionMode = &completionMode
	fixture.job.Spec.Suspend = &suspend
	fixture.job.Spec.PodReplacementPolicy = &replacementPolicy
	for _, spec := range []*corev1.PodSpec{&fixture.job.Spec.Template.Spec, &fixture.pod.Spec} {
		spec.DeprecatedServiceAccount = spec.ServiceAccountName
		spec.DNSPolicy = corev1.DNSClusterFirst
		spec.SchedulerName = corev1.DefaultSchedulerName
		spec.EnableServiceLinks = &serviceLinks
		spec.Containers[0].TerminationMessagePath = corev1.TerminationMessagePathDefault
		spec.Containers[0].TerminationMessagePolicy = corev1.TerminationMessageReadFile
		optional := false
		spec.Containers[0].Env[0].ValueFrom.SecretKeyRef.Optional = &optional
		spec.Volumes[0].Projected.Sources[2].DownwardAPI.Items[0].FieldRef.APIVersion = "v1"
	}
	applyKubernetes132LivePodTuple(&fixture.pod.Spec)
	for _, metadata := range []*metav1.ObjectMeta{&fixture.job.Spec.Template.ObjectMeta, &fixture.pod.ObjectMeta} {
		metadata.Labels["batch.kubernetes.io/controller-uid"] = string(fixture.job.UID)
		metadata.Labels["batch.kubernetes.io/job-name"] = fixture.job.Name
		metadata.Labels["controller-uid"] = string(fixture.job.UID)
		metadata.Labels["job-name"] = fixture.job.Name
	}
}

func TestOwnedFinalProofAcceptsKubernetes132CompletedLivePod(t *testing.T) {
	for _, reverseTolerations := range []bool{false, true} {
		name := "admission order"
		if reverseTolerations {
			name = "reversed admission order"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newAcceleratorSweepTestFixture(t)
			fixture.storage.ResourceVersion = "7"
			applyKubernetes132CompletedLiveMutations(fixture)
			if reverseTolerations {
				fixture.pod.Spec.Tolerations[0], fixture.pod.Spec.Tolerations[1] = fixture.pod.Spec.Tolerations[1], fixture.pod.Spec.Tolerations[0]
			}
			client := fixture.client()
			if candidate, status := fixture.inspect(client); candidate == nil || status != AcceleratorSweepEligible {
				t.Fatalf("shared live proof status=%s candidate=%#v", status, candidate)
			}
			pod := AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "v1", Kind: "Pod", Namespace: fixture.pod.Namespace, Name: fixture.pod.Name}, UID: fixture.pod.UID}
			if status := uninstallOwnedAcceleratorReleaseWith(context.Background(), fixture.config, client, fixture.prepared, fixture.ownedReceipt(), pod); status != AcceleratorOwnedCleanupSucceeded {
				t.Fatalf("owned final proof status=%s", status)
			}
		})
	}
}

func TestSweepCandidateRequiresExactStoredAndLiveContract(t *testing.T) {
	fixture := newAcceleratorSweepTestFixture(t)
	candidate, status := fixture.inspect(fixture.client())
	if status != AcceleratorSweepEligible || candidate == nil {
		stored, _ := fixture.config.Releases.Get(fixture.prepared.request.ReleaseName, 1)
		closed, stage := proveClosedAcceleratorRelease(stored, fixture.prepared.request.ReleaseNamespace, fixture.prepared.request.ReleaseName, nil)
		names := map[string]string{}
		for _, identity := range closed.resources {
			names[identity.Kind] = identity.Name
		}
		for kind, object := range map[string]metav1.Object{"Job": fixture.job, "Secret": fixture.secret, "ServiceAccount": fixture.account, "ClusterRole": fixture.role, "ClusterRoleBinding": fixture.binding} {
			if !acceleratorClosedLiveObjectContract(object, closed, names) {
				t.Logf("closed stored stage=%s live kind=%s rejected", stage, kind)
			}
		}
		if !acceleratorClosedPodContract(fixture.pod, closed, closed.jobName, fixture.job.UID) {
			t.Log("closed pod rejected")
		}
		t.Fatalf("status=%s candidate=%#v", status, candidate)
	}
	replacement := fixture.job.DeepCopy()
	replacement.UID = "replacement"
	client := fixture.client(replacement, fixture.secret, fixture.account, fixture.role, fixture.binding, fixture.pod, fixture.storage)
	if _, status = fixture.inspect(client); status != AcceleratorSweepActiveOrAmbiguous {
		t.Fatalf("replacement status=%s", status)
	}
}

func TestSweepLiveContractAcceptsOnlyKnownKubernetesDefaultsAndGeneratedFields(t *testing.T) {
	fixture := newAcceleratorSweepTestFixture(t)
	manual, suspend, serviceLinks := false, false, true
	completionMode, replacementPolicy := batchv1.NonIndexedCompletion, batchv1.TerminatingOrFailed
	fixture.job.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"batch.kubernetes.io/controller-uid": string(fixture.job.UID)}}
	fixture.job.Spec.ManualSelector = &manual
	fixture.job.Spec.CompletionMode = &completionMode
	fixture.job.Spec.Suspend = &suspend
	fixture.job.Spec.PodReplacementPolicy = &replacementPolicy
	for _, spec := range []*corev1.PodSpec{&fixture.job.Spec.Template.Spec, &fixture.pod.Spec} {
		spec.DeprecatedServiceAccount = spec.ServiceAccountName
		spec.DNSPolicy = corev1.DNSClusterFirst
		spec.SchedulerName = corev1.DefaultSchedulerName
		spec.EnableServiceLinks = &serviceLinks
		spec.Containers[0].TerminationMessagePath = corev1.TerminationMessagePathDefault
		spec.Containers[0].TerminationMessagePolicy = corev1.TerminationMessageReadFile
		optional := false
		spec.Containers[0].Env[0].ValueFrom.SecretKeyRef.Optional = &optional
		spec.Volumes[0].Projected.Sources[2].DownwardAPI.Items[0].FieldRef.APIVersion = "v1"
	}
	for _, metadata := range []*metav1.ObjectMeta{&fixture.job.Spec.Template.ObjectMeta, &fixture.pod.ObjectMeta} {
		metadata.Labels["batch.kubernetes.io/controller-uid"] = string(fixture.job.UID)
		metadata.Labels["batch.kubernetes.io/job-name"] = fixture.job.Name
		metadata.Labels["controller-uid"] = string(fixture.job.UID)
		metadata.Labels["job-name"] = fixture.job.Name
	}
	if candidate, status := fixture.inspect(fixture.client()); status != AcceleratorSweepEligible || candidate == nil {
		t.Fatalf("status=%s candidate=%#v", status, candidate)
	}
}

func TestSweepFailedJobAndFailedPodAreTerminalAndEligible(t *testing.T) {
	fixture := newAcceleratorSweepTestFixture(t)
	fixture.job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
	fixture.pod.Status.Phase = corev1.PodFailed
	if candidate, status := fixture.inspect(fixture.client()); status != AcceleratorSweepEligible || candidate == nil {
		t.Fatalf("status=%s candidate=%#v", status, candidate)
	}
}

func TestSweepSecondProofMissingReplacementAndAPIErrorMatrix(t *testing.T) {
	targets := []string{"job", "verifier secret", "service account", "cluster role", "cluster role binding", "pod", "Helm storage"}
	operations := []string{"missing", "replacement", "API error"}
	type resolvedTarget struct {
		object    runtime.Object
		gvr       schema.GroupVersionResource
		resource  string
		namespace string
		name      string
		podList   bool
		storage   bool
	}
	resolve := func(f *acceleratorSweepTestFixture, name string) resolvedTarget {
		switch name {
		case "job":
			return resolvedTarget{object: f.job, gvr: batchv1.SchemeGroupVersion.WithResource("jobs"), resource: "jobs", namespace: f.job.Namespace, name: f.job.Name}
		case "verifier secret":
			return resolvedTarget{object: f.secret, gvr: corev1.SchemeGroupVersion.WithResource("secrets"), resource: "secrets", namespace: f.secret.Namespace, name: f.secret.Name}
		case "service account":
			return resolvedTarget{object: f.account, gvr: corev1.SchemeGroupVersion.WithResource("serviceaccounts"), resource: "serviceaccounts", namespace: f.account.Namespace, name: f.account.Name}
		case "cluster role":
			return resolvedTarget{object: f.role, gvr: rbacv1.SchemeGroupVersion.WithResource("clusterroles"), resource: "clusterroles", name: f.role.Name}
		case "cluster role binding":
			return resolvedTarget{object: f.binding, gvr: rbacv1.SchemeGroupVersion.WithResource("clusterrolebindings"), resource: "clusterrolebindings", name: f.binding.Name}
		case "pod":
			return resolvedTarget{object: f.pod, gvr: corev1.SchemeGroupVersion.WithResource("pods"), resource: "pods", namespace: f.pod.Namespace, name: f.pod.Name, podList: true}
		case "Helm storage":
			return resolvedTarget{object: f.storage, gvr: corev1.SchemeGroupVersion.WithResource("secrets"), resource: "secrets", namespace: f.storage.Namespace, name: f.storage.Name, storage: true}
		default:
			panic(name)
		}
	}
	for _, targetName := range targets {
		for _, operation := range operations {
			t.Run(targetName+"/"+operation, func(t *testing.T) {
				fixture := newAcceleratorSweepTestFixture(t)
				client := fixture.client()
				candidate, status := fixture.inspect(client)
				if status != AcceleratorSweepEligible || candidate == nil {
					t.Fatal(status)
				}
				target := resolve(fixture, targetName)
				switch operation {
				case "missing":
					if err := client.Tracker().Delete(target.gvr, target.namespace, target.name); err != nil {
						t.Fatal(err)
					}
				case "replacement":
					replacement := target.object.DeepCopyObject()
					replacement.(metav1.Object).SetUID("replacement")
					if err := client.Tracker().Update(target.gvr, replacement, target.namespace); err != nil {
						t.Fatal(err)
					}
				case "API error":
					if target.podList {
						client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
							return true, nil, errors.New("raw Kubernetes API Bearer secret-token")
						})
					} else {
						client.PrependReactor("get", target.resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
							get, ok := action.(k8stesting.GetAction)
							if !ok || get.GetName() != target.name {
								return false, nil, nil
							}
							return true, nil, errors.New("raw Kubernetes API Bearer secret-token")
						})
					}
				}
				want := AcceleratorSweepOwnershipChanged
				if target.storage && operation == "missing" {
					want = AcceleratorSweepAlreadyGone
				}
				if got := uninstallAcceleratorSweepCandidateWith(context.Background(), fixture.config, client, candidate); got != want {
					t.Fatalf("status=%s want=%s", got, want)
				}
				if _, err := fixture.config.Releases.Get(fixture.prepared.request.ReleaseName, 1); err != nil {
					t.Fatalf("second-proof change reached Helm Run: %v", err)
				}
			})
		}
	}
}

func TestForgedSweepCandidateIsRejectedBeforeIO(t *testing.T) {
	fixture := newAcceleratorSweepTestFixture(t)
	client := fixture.client()
	var actions int
	client.PrependReactor("*", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		actions++
		return false, nil, nil
	})
	status := uninstallAcceleratorSweepCandidateWith(context.Background(), fixture.config, client, &AcceleratorSweepCandidate{name: fixture.prepared.request.ReleaseName, namespace: fixture.prepared.request.ReleaseNamespace})
	if status != AcceleratorSweepOwnershipChanged || actions != 0 {
		t.Fatalf("status=%s actions=%d", status, actions)
	}
}

func TestSweepAbsenceWaitNeverFollowsReplacementUID(t *testing.T) {
	storage := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "storage", Namespace: "default", UID: "replacement"}}
	client := k8sfake.NewSimpleClientset(storage)
	gets := 0
	client.PrependReactor("get", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		gets++
		return false, nil, nil
	})
	candidate := &AcceleratorSweepCandidate{storage: AcceleratorStorageIdentity{Namespace: "default", Name: "storage", UID: "original"}, authority: trustedAcceleratorSweepAuthority}
	if !waitAcceleratorSweepCandidateGoneWith(context.Background(), client, candidate) || gets != 1 {
		t.Fatalf("replacement followed: gets=%d", gets)
	}
}

func TestSweepExactUIDAbsenceMatrixForEveryCapturedIdentity(t *testing.T) {
	fixture := newAcceleratorSweepTestFixture(t)
	type target struct {
		name    string
		object  runtime.Object
		item    AcceleratorDeletionIdentity
		storage bool
	}
	targets := []target{
		{name: "job", object: fixture.job, item: AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "batch/v1", Kind: "Job", Namespace: fixture.job.Namespace, Name: fixture.job.Name}, UID: fixture.job.UID}},
		{name: "verifier secret", object: fixture.secret, item: AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "v1", Kind: "Secret", Namespace: fixture.secret.Namespace, Name: fixture.secret.Name}, UID: fixture.secret.UID}},
		{name: "service account", object: fixture.account, item: AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "v1", Kind: "ServiceAccount", Namespace: fixture.account.Namespace, Name: fixture.account.Name}, UID: fixture.account.UID}},
		{name: "cluster role", object: fixture.role, item: AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: fixture.role.Name}, UID: fixture.role.UID}},
		{name: "cluster role binding", object: fixture.binding, item: AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBinding", Name: fixture.binding.Name}, UID: fixture.binding.UID}},
		{name: "pod", object: fixture.pod, item: AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "v1", Kind: "Pod", Namespace: fixture.pod.Namespace, Name: fixture.pod.Name}, UID: fixture.pod.UID}},
		{name: "Helm storage", object: fixture.storage, item: AcceleratorDeletionIdentity{Resource: AcceleratorResourceIdentity{APIVersion: "v1", Kind: "Secret", Namespace: fixture.storage.Namespace, Name: fixture.storage.Name}, UID: fixture.storage.UID}, storage: true},
	}
	for _, captured := range targets {
		captured := captured
		t.Run(captured.name, func(t *testing.T) {
			candidate := func() *AcceleratorSweepCandidate {
				value := &AcceleratorSweepCandidate{storage: AcceleratorStorageIdentity{Namespace: "default", Name: "absent-storage", UID: "absent-storage"}, authority: trustedAcceleratorSweepAuthority}
				if captured.storage {
					value.storage = AcceleratorStorageIdentity{Namespace: captured.item.Resource.Namespace, Name: captured.item.Resource.Name, UID: captured.item.UID}
				} else if captured.item.Resource.Kind == "Pod" {
					value.pods = []AcceleratorDeletionIdentity{captured.item}
				} else {
					value.resources = []AcceleratorDeletionIdentity{captured.item}
				}
				return value
			}
			t.Run("absent", func(t *testing.T) {
				if !waitAcceleratorSweepCandidateGoneWith(context.Background(), k8sfake.NewSimpleClientset(), candidate()) {
					t.Fatal("absent exact identity remained pending")
				}
			})
			t.Run("same UID remains", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if waitAcceleratorSweepCandidateGoneWith(ctx, k8sfake.NewSimpleClientset(captured.object.DeepCopyObject()), candidate()) {
					t.Fatal("same UID was reported absent")
				}
			})
			t.Run("replacement is not followed", func(t *testing.T) {
				replacement := captured.object.DeepCopyObject()
				replacement.(metav1.Object).SetUID("replacement")
				if !waitAcceleratorSweepCandidateGoneWith(context.Background(), k8sfake.NewSimpleClientset(replacement), candidate()) {
					t.Fatal("replacement UID was followed")
				}
			})
			t.Run("API error remains", func(t *testing.T) {
				client := k8sfake.NewSimpleClientset()
				client.PrependReactor("get", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("hostile API payload Bearer secret-token")
				})
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if waitAcceleratorSweepCandidateGoneWith(ctx, client, candidate()) {
					t.Fatal("API error was reported absent")
				}
			})
		})
	}
}

func TestSweepLiveContractAndInertnessHostileMatrix(t *testing.T) {
	mutations := map[string]func(*acceleratorSweepTestFixture){
		"job metadata":             func(f *acceleratorSweepTestFixture) { delete(f.job.Labels, "app.kubernetes.io/managed-by") },
		"job spec":                 func(f *acceleratorSweepTestFixture) { f.job.Spec.Template.Spec.ServiceAccountName = "other" },
		"secret metadata":          func(f *acceleratorSweepTestFixture) { delete(f.secret.Annotations, "meta.helm.sh/release-name") },
		"secret spec":              func(f *acceleratorSweepTestFixture) { f.secret.Immutable = nil },
		"service account metadata": func(f *acceleratorSweepTestFixture) { delete(f.account.Labels, "app.kubernetes.io/instance") },
		"service account spec":     func(f *acceleratorSweepTestFixture) { yes := true; f.account.AutomountServiceAccountToken = &yes },
		"service account secret": func(f *acceleratorSweepTestFixture) {
			f.account.Secrets = []corev1.ObjectReference{{Name: "unexpected"}}
		},
		"service account image pull secret": func(f *acceleratorSweepTestFixture) {
			f.account.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "unexpected"}}
		},
		"role metadata":         func(f *acceleratorSweepTestFixture) { f.role.Annotations["helm.sh/resource-policy"] = "keep" },
		"role rules":            func(f *acceleratorSweepTestFixture) { f.role.Rules[0].Verbs = []string{"get"} },
		"role aggregation rule": func(f *acceleratorSweepTestFixture) { f.role.AggregationRule = &rbacv1.AggregationRule{} },
		"role non-resource URL": func(f *acceleratorSweepTestFixture) { f.role.Rules[0].NonResourceURLs = []string{"/healthz"} },
		"binding metadata":      func(f *acceleratorSweepTestFixture) { delete(f.binding.Annotations, "meta.helm.sh/release-namespace") },
		"binding role":          func(f *acceleratorSweepTestFixture) { f.binding.RoleRef.Name = "other" },
		"binding subject API group": func(f *acceleratorSweepTestFixture) {
			f.binding.Subjects[0].APIGroup = rbacv1.GroupName
		},
		"job active":     func(f *acceleratorSweepTestFixture) { f.job.Status.Conditions = nil },
		"pod pending":    func(f *acceleratorSweepTestFixture) { f.pod.Status.Phase = corev1.PodPending },
		"pod running":    func(f *acceleratorSweepTestFixture) { f.pod.Status.Phase = corev1.PodRunning },
		"pod unknown":    func(f *acceleratorSweepTestFixture) { f.pod.Status.Phase = corev1.PodUnknown },
		"pod release":    func(f *acceleratorSweepTestFixture) { f.pod.Labels["app.kubernetes.io/instance"] = "other" },
		"pod controller": func(f *acceleratorSweepTestFixture) { f.pod.OwnerReferences[0].UID = "other" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := newAcceleratorSweepTestFixture(t)
			mutate(fixture)
			if _, status := fixture.inspect(fixture.client()); status != AcceleratorSweepActiveOrAmbiguous {
				t.Fatalf("status=%s", status)
			}
			if _, err := fixture.config.Releases.Get(fixture.prepared.request.ReleaseName, 1); err != nil {
				t.Fatalf("live mutation reached Helm Run: %v", err)
			}
		})
	}
	t.Run("absent job and pod is inert", func(t *testing.T) {
		fixture := newAcceleratorSweepTestFixture(t)
		client := fixture.client(fixture.secret, fixture.account, fixture.role, fixture.binding, fixture.storage)
		if _, status := fixture.inspect(client); status != AcceleratorSweepEligible {
			t.Fatalf("status=%s", status)
		}
	})
	t.Run("absent job with nonterminal pod is active", func(t *testing.T) {
		fixture := newAcceleratorSweepTestFixture(t)
		fixture.pod.Status.Phase = corev1.PodRunning
		client := fixture.client(fixture.secret, fixture.account, fixture.role, fixture.binding, fixture.pod, fixture.storage)
		if _, status := fixture.inspect(client); status != AcceleratorSweepActiveOrAmbiguous {
			t.Fatalf("status=%s", status)
		}
	})
	t.Run("ambiguous second pod", func(t *testing.T) {
		fixture := newAcceleratorSweepTestFixture(t)
		second := fixture.pod.DeepCopy()
		second.Name, second.UID, second.OwnerReferences[0].UID = "second", "second", "other"
		client := fixture.client(fixture.job, fixture.secret, fixture.account, fixture.role, fixture.binding, fixture.pod, second, fixture.storage)
		if _, status := fixture.inspect(client); status != AcceleratorSweepActiveOrAmbiguous {
			t.Fatalf("status=%s", status)
		}
	})
	t.Run("pod list error", func(t *testing.T) {
		fixture := newAcceleratorSweepTestFixture(t)
		client := fixture.client()
		client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("raw verifier stored error")
		})
		if _, status := fixture.inspect(client); status != AcceleratorSweepActiveOrAmbiguous {
			t.Fatalf("status=%s", status)
		}
	})
}

func TestSweepAbsentJobDerivesTerminalPodControllerCorrelation(t *testing.T) {
	for _, phase := range []corev1.PodPhase{corev1.PodSucceeded, corev1.PodFailed} {
		t.Run(string(phase), func(t *testing.T) {
			fixture := newAcceleratorSweepTestFixture(t)
			fixture.pod.Status.Phase = phase
			owner := fixture.pod.OwnerReferences[0]
			fixture.pod.Labels["batch.kubernetes.io/controller-uid"] = string(owner.UID)
			fixture.pod.Labels["batch.kubernetes.io/job-name"] = owner.Name
			fixture.pod.Labels["controller-uid"] = string(owner.UID)
			fixture.pod.Labels["job-name"] = owner.Name
			client := fixture.client(fixture.secret, fixture.account, fixture.role, fixture.binding, fixture.pod, fixture.storage)
			candidate, status := fixture.inspect(client)
			if status != AcceleratorSweepEligible || candidate == nil || len(candidate.resources) != 4 || len(candidate.pods) != 1 || candidate.pods[0].UID != fixture.pod.UID {
				t.Fatalf("status=%s candidate=%#v", status, candidate)
			}
		})
	}

	for _, test := range []struct {
		name string
		edit func(*acceleratorSweepTestFixture)
	}{
		{name: "nonterminal", edit: func(f *acceleratorSweepTestFixture) { f.pod.Status.Phase = corev1.PodRunning }},
		{name: "owner name mismatch", edit: func(f *acceleratorSweepTestFixture) { f.pod.OwnerReferences[0].Name = "other" }},
		{name: "empty owner UID", edit: func(f *acceleratorSweepTestFixture) { f.pod.OwnerReferences[0].UID = "" }},
		{name: "ambiguous owner", edit: func(f *acceleratorSweepTestFixture) {
			owner := f.pod.OwnerReferences[0]
			f.pod.OwnerReferences = append(f.pod.OwnerReferences, owner)
		}},
		{name: "batch controller UID label mismatch", edit: func(f *acceleratorSweepTestFixture) { f.pod.Labels["batch.kubernetes.io/controller-uid"] = "other" }},
		{name: "legacy controller UID label mismatch", edit: func(f *acceleratorSweepTestFixture) { f.pod.Labels["controller-uid"] = "other" }},
		{name: "batch Job name label mismatch", edit: func(f *acceleratorSweepTestFixture) { f.pod.Labels["batch.kubernetes.io/job-name"] = "other" }},
		{name: "legacy Job name label mismatch", edit: func(f *acceleratorSweepTestFixture) { f.pod.Labels["job-name"] = "other" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAcceleratorSweepTestFixture(t)
			test.edit(fixture)
			client := fixture.client(fixture.secret, fixture.account, fixture.role, fixture.binding, fixture.pod, fixture.storage)
			if _, status := fixture.inspect(client); status != AcceleratorSweepActiveOrAmbiguous {
				t.Fatalf("status=%s", status)
			}
		})
	}

	t.Run("multiple terminal Pods require one absent Job UID", func(t *testing.T) {
		fixture := newAcceleratorSweepTestFixture(t)
		second := fixture.pod.DeepCopy()
		second.Name, second.UID = "second", "second-pod-uid"
		second.OwnerReferences[0].UID = "other-job-uid"
		client := fixture.client(fixture.secret, fixture.account, fixture.role, fixture.binding, fixture.pod, second, fixture.storage)
		if _, status := fixture.inspect(client); status != AcceleratorSweepActiveOrAmbiguous {
			t.Fatalf("status=%s", status)
		}
	})
}

func TestSweepDisappearanceUsesExactInjectedPollAndCleanupBudget(t *testing.T) {
	start := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	elapsed := time.Duration(0)
	polls := 0
	storage := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "storage", Namespace: "default", UID: "storage-uid"}}
	client := k8sfake.NewSimpleClientset(storage)
	candidate := &AcceleratorSweepCandidate{storage: AcceleratorStorageIdentity{Namespace: storage.Namespace, Name: storage.Name, UID: storage.UID}, authority: trustedAcceleratorSweepAuthority}
	got := waitAcceleratorSweepCandidateGoneWithPoll(context.Background(), client, candidate, func() time.Time { return start.Add(elapsed) }, func(_ context.Context, interval time.Duration) bool {
		if interval != 250*time.Millisecond {
			t.Fatalf("poll interval=%s", interval)
		}
		polls++
		elapsed += interval
		return true
	})
	if got || elapsed != AcceleratorOwnedCleanupTimeout || polls != int(AcceleratorOwnedCleanupTimeout/(250*time.Millisecond)) {
		t.Fatalf("gone=%v elapsed=%s polls=%d", got, elapsed, polls)
	}

	t.Run("disappearance is observed only after exact polls", func(t *testing.T) {
		gets, elapsed := 0, time.Duration(0)
		client := k8sfake.NewSimpleClientset()
		client.PrependReactor("get", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
			gets++
			if gets < 3 {
				return true, storage.DeepCopy(), nil
			}
			return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, storage.Name)
		})
		if !waitAcceleratorSweepCandidateGoneWithPoll(context.Background(), client, candidate, func() time.Time { return start.Add(elapsed) }, func(_ context.Context, interval time.Duration) bool {
			if interval != 250*time.Millisecond {
				t.Fatalf("poll interval=%s", interval)
			}
			elapsed += interval
			return true
		}) || gets != 3 || elapsed != 500*time.Millisecond {
			t.Fatalf("gets=%d elapsed=%s", gets, elapsed)
		}
	})
}

func TestSweepHostileHelmAndKubernetesErrorsReachClosedMappings(t *testing.T) {
	hostile := []string{
		"raw Helm history creatorVerifier=w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM",
		"Bearer first-proof-secret-token",
		"raw second-proof manifest value",
		"raw Helm uninstall cleanup token",
		"raw disappearance Kubernetes status",
	}
	var logs bytes.Buffer
	prior := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prior)

	statuses := make([]AcceleratorSweepProofStatus, 0, 4)
	t.Run("Helm history", func(t *testing.T) {
		fixture := newAcceleratorSweepTestFixture(t)
		fixture.config.Releases.Driver = acceleratorHostileDriver{Driver: fixture.config.Releases.Driver, queryErr: errors.New(hostile[0])}
		_, status := fixture.inspect(fixture.client())
		statuses = append(statuses, status)
		if status != AcceleratorSweepUnsupportedMalformed {
			t.Fatalf("status=%s", status)
		}
	})

	t.Run("Kubernetes first proof", func(t *testing.T) {
		fixture := newAcceleratorSweepTestFixture(t)
		client := fixture.client()
		client.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if action.(k8stesting.GetAction).GetName() == fixture.storage.Name {
				return true, nil, errors.New(hostile[1])
			}
			return false, nil, nil
		})
		_, status := fixture.inspect(client)
		statuses = append(statuses, status)
		if status != AcceleratorSweepUnsupportedMalformed {
			t.Fatalf("status=%s", status)
		}
	})

	t.Run("Kubernetes second proof", func(t *testing.T) {
		fixture := newAcceleratorSweepTestFixture(t)
		client := fixture.client()
		candidate, status := fixture.inspect(client)
		if status != AcceleratorSweepEligible {
			t.Fatal(status)
		}
		client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New(hostile[2])
		})
		status = uninstallAcceleratorSweepCandidateWith(context.Background(), fixture.config, client, candidate)
		statuses = append(statuses, status)
		if status != AcceleratorSweepOwnershipChanged {
			t.Fatalf("status=%s", status)
		}
		if _, err := fixture.config.Releases.Get(fixture.prepared.request.ReleaseName, 1); err != nil {
			t.Fatalf("second-proof error reached Helm Run: %v", err)
		}
	})

	t.Run("Helm cleanup Run", func(t *testing.T) {
		fixture := newAcceleratorSweepTestFixture(t)
		client := fixture.client()
		candidate, status := fixture.inspect(client)
		if status != AcceleratorSweepEligible {
			t.Fatal(status)
		}
		fixture.config.Releases.Driver = acceleratorHostileDriver{Driver: fixture.config.Releases.Driver, deleteErr: errors.New(hostile[3])}
		status = uninstallAcceleratorSweepCandidateWith(context.Background(), fixture.config, client, candidate)
		statuses = append(statuses, status)
		if status != AcceleratorSweepCleanupFailed {
			t.Fatalf("status=%s", status)
		}
	})

	t.Run("Kubernetes disappearance", func(t *testing.T) {
		storage := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "storage", Namespace: "default", UID: "storage-uid"}}
		client := k8sfake.NewSimpleClientset(storage)
		client.PrependReactor("get", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New(hostile[4])
		})
		candidate := &AcceleratorSweepCandidate{storage: AcceleratorStorageIdentity{Namespace: storage.Namespace, Name: storage.Name, UID: storage.UID}, authority: trustedAcceleratorSweepAuthority}
		if waitAcceleratorSweepCandidateGoneWithPoll(context.Background(), client, candidate, time.Now, func(context.Context, time.Duration) bool { return false }) {
			t.Fatal("Kubernetes error was mapped to disappearance")
		}
	})

	encoded, err := json.Marshal(struct {
		Statuses []AcceleratorSweepProofStatus `json:"statuses"`
	}{Statuses: statuses})
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{fmt.Sprint(statuses), fmt.Sprintf("%#v", statuses), string(encoded), logs.String()} {
		for _, raw := range hostile {
			if strings.Contains(output, raw) {
				t.Fatal("hostile Helm/Kubernetes corpus escaped a closed sweep mapping")
			}
		}
	}
	if logs.Len() != 0 {
		t.Fatal("sweep mapping emitted a standard-library log entry")
	}
}

func TestSweepSecondProofMutationRaces(t *testing.T) {
	mutations := map[string]func(*acceleratorSweepTestFixture, *k8sfake.Clientset){
		"revision": func(f *acceleratorSweepTestFixture, _ *k8sfake.Clientset) {
			stored, _ := f.config.Releases.Get(f.prepared.request.ReleaseName, 1)
			next := *stored
			next.Version = 2
			_ = f.config.Releases.Create(&next)
		},
		"config": func(f *acceleratorSweepTestFixture, _ *k8sfake.Clientset) {
			stored, _ := f.config.Releases.Get(f.prepared.request.ReleaseName, 1)
			stored.Config["extra"] = true
			_ = f.config.Releases.Update(stored)
		},
		"manifest": func(f *acceleratorSweepTestFixture, _ *k8sfake.Clientset) {
			stored, _ := f.config.Releases.Get(f.prepared.request.ReleaseName, 1)
			stored.Manifest += "\n# changed"
			_ = f.config.Releases.Update(stored)
		},
		"storage uid": func(f *acceleratorSweepTestFixture, c *k8sfake.Clientset) {
			changed := f.storage.DeepCopy()
			changed.UID = "other"
			_ = c.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), changed, changed.Namespace)
		},
		"live uid": func(f *acceleratorSweepTestFixture, c *k8sfake.Clientset) {
			changed := f.secret.DeepCopy()
			changed.UID = "other"
			_ = c.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), changed, changed.Namespace)
		},
		"live ownership": func(f *acceleratorSweepTestFixture, c *k8sfake.Clientset) {
			changed := f.secret.DeepCopy()
			changed.Annotations["meta.helm.sh/release-name"] = "other"
			_ = c.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), changed, changed.Namespace)
		},
		"terminal active": func(f *acceleratorSweepTestFixture, c *k8sfake.Clientset) {
			changed := f.job.DeepCopy()
			changed.Status.Conditions = nil
			_ = c.Tracker().Update(batchv1.SchemeGroupVersion.WithResource("jobs"), changed, changed.Namespace)
		},
		"add active pod": func(f *acceleratorSweepTestFixture, c *k8sfake.Clientset) {
			changed := f.pod.DeepCopy()
			changed.Name, changed.UID, changed.Status.Phase = "active", "active", corev1.PodRunning
			_ = c.Tracker().Create(corev1.SchemeGroupVersion.WithResource("pods"), changed, changed.Namespace)
		},
		"ttl delete": func(f *acceleratorSweepTestFixture, c *k8sfake.Clientset) {
			_ = c.Tracker().Delete(batchv1.SchemeGroupVersion.WithResource("jobs"), f.job.Namespace, f.job.Name)
			_ = c.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), f.pod.Namespace, f.pod.Name)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := newAcceleratorSweepTestFixture(t)
			client := fixture.client()
			candidate, status := fixture.inspect(client)
			if status != AcceleratorSweepEligible {
				t.Fatal(status)
			}
			mutate(fixture, client)
			if got := uninstallAcceleratorSweepCandidateWith(context.Background(), fixture.config, client, candidate); got != AcceleratorSweepOwnershipChanged {
				t.Fatalf("status=%s", got)
			}
			if _, err := fixture.config.Releases.Get(fixture.prepared.request.ReleaseName, 1); err != nil {
				t.Fatal("mutation removed release")
			}
		})
	}
	t.Run("already gone", func(t *testing.T) {
		fixture := newAcceleratorSweepTestFixture(t)
		client := fixture.client()
		candidate, _ := fixture.inspect(client)
		_, _ = fixture.config.Releases.Delete(fixture.prepared.request.ReleaseName, 1)
		if got := uninstallAcceleratorSweepCandidateWith(context.Background(), fixture.config, client, candidate); got != AcceleratorSweepAlreadyGone {
			t.Fatalf("status=%s", got)
		}
	})
	t.Run("unchanged only", func(t *testing.T) {
		fixture := newAcceleratorSweepTestFixture(t)
		client := fixture.client()
		candidate, _ := fixture.inspect(client)
		if got := uninstallAcceleratorSweepCandidateWith(context.Background(), fixture.config, client, candidate); got != AcceleratorSweepEligible {
			t.Fatalf("status=%s", got)
		}
		if _, err := fixture.config.Releases.Get(fixture.prepared.request.ReleaseName, 1); !errors.Is(err, driver.ErrReleaseNotFound) {
			t.Fatalf("release remained: %v", err)
		}
	})
}

func TestConfigureAcceleratorSweepListIsNamespaceBoundedAndComplete(t *testing.T) {
	configured := configureAcceleratorSweepList(acceleratorTestActionConfig())
	if configured == nil || configured.All || configured.AllNamespaces || configured.StateMask != action.ListAll|action.ListUnknown || configured.Limit != 101 || configured.Offset != 0 || configured.ByDate || configured.SortReverse {
		t.Fatalf("unsafe sweep list flags: %#v", configured)
	}
}

func TestSweepStructuralStatusMatchesSecretDriverPersistence(t *testing.T) {
	loaded, _ := acceleratorTestChart(t)
	prepared, failure := prepareAcceleratorRelease(loaded, acceleratorTestRequest())
	if failure != AcceleratorOK {
		t.Fatal(failure)
	}

	t.Run("secret driver retains pending-install and proves", func(t *testing.T) {
		config := acceleratorTestActionConfig()
		client := k8sfake.NewSimpleClientset()
		config.Releases = storage.Init(driver.NewSecrets(client.CoreV1().Secrets(prepared.request.ReleaseNamespace)))
		if receipt, got := installAcceleratorWithActionConfig(context.Background(), config, prepared); receipt == nil || got != AcceleratorOK {
			t.Fatalf("install=%s receipt=%#v", got, receipt)
		}
		stored, err := config.Releases.Get(prepared.request.ReleaseName, 1)
		if err != nil {
			t.Fatal(err)
		}
		request, resources, jobName, hash, ok := proveStoredSweepRelease(stored, prepared.request.ReleaseNamespace, prepared.request.ReleaseName)
		if stored.Info == nil || stored.Info.Status != release.StatusPendingInstall || !ok {
			t.Fatalf("secret-backed release rejected: status=%v", stored.Info)
		}
		if request.WorkloadSession != prepared.request.WorkloadSession || len(resources) != 5 || jobName != prepared.JobName() || hash == "" {
			t.Fatal("incomplete sweep proof")
		}
	})

	t.Run("memory driver aliases deployed state and is rejected", func(t *testing.T) {
		config := acceleratorTestActionConfig()
		if receipt, got := installAcceleratorWithActionConfig(context.Background(), config, prepared); receipt == nil || got != AcceleratorOK {
			t.Fatalf("install=%s receipt=%#v", got, receipt)
		}
		stored, err := config.Releases.Get(prepared.request.ReleaseName, 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, _, stage := proveStoredSweepReleaseStage(stored, prepared.request.ReleaseNamespace, prepared.request.ReleaseName); stored.Info == nil || stored.Info.Status != release.StatusDeployed || stage != "status_deployed" {
			t.Fatalf("memory-backed status=%v stage=%s", stored.Info, stage)
		}
		if _, _, _, _, ok := proveStoredSweepRelease(stored, prepared.request.ReleaseNamespace, prepared.request.ReleaseName); ok {
			t.Fatal("memory-only deployed status satisfied the secret-backed storage contract")
		}
	})
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

type acceleratorCanceledDeadlineContext struct {
	context.Context
	deadline time.Time
	done     <-chan struct{}
}

func (c acceleratorCanceledDeadlineContext) Deadline() (time.Time, bool) { return c.deadline, true }
func (c acceleratorCanceledDeadlineContext) Done() <-chan struct{}       { return c.done }
func (acceleratorCanceledDeadlineContext) Err() error                    { return context.Canceled }

func TestAcceleratorDetachedSweepCleanupContextUsesLesserDeadlineAndDropsCancellation(t *testing.T) {
	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	canceled := make(chan struct{})
	close(canceled)
	for _, test := range []struct {
		name      string
		remaining time.Duration
		want      time.Duration
	}{
		{name: "cleanup cap", remaining: 90 * time.Second, want: 45 * time.Second},
		{name: "outer remainder", remaining: 30 * time.Second, want: 30 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			outer := acceleratorCanceledDeadlineContext{Context: context.Background(), deadline: now.Add(test.remaining), done: canceled}
			var got time.Duration
			cleanup, cancel := acceleratorDetachedSweepCleanupContext(outer, func() time.Time { return now }, func(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
				got = duration
				if parent.Err() != nil || parent.Done() != nil {
					t.Fatalf("detached parent retained cancellation: err=%v done=%v", parent.Err(), parent.Done())
				}
				return context.WithCancel(parent)
			})
			defer cancel()
			if got != test.want {
				t.Fatalf("duration=%s want=%s", got, test.want)
			}
			if cleanup.Err() != nil {
				t.Fatalf("cleanup context inherited caller cancellation: %v", cleanup.Err())
			}
		})
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
