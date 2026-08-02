//go:build accelerator_provision_kind && helm

package acceleratorprovision

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type kindRegistryTransport struct {
	base   http.RoundTripper
	target *url.URL
}

func (t kindRegistryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Scheme != "https" || request.URL.Host != "ghcr.io" {
		return nil, errors.New("unexpected production registry request")
	}
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	clone.URL = &urlCopy
	clone.URL.Scheme, clone.URL.Host, clone.Host = t.target.Scheme, t.target.Host, ""
	return t.base.RoundTrip(clone)
}

type recordingSequenceEntropy struct {
	mu   sync.Mutex
	next byte
	data []byte
}

func (r *recordingSequenceEntropy) Read(buffer []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := range buffer {
		buffer[index] = r.next
		r.data = append(r.data, r.next)
		r.next++
	}
	return len(buffer), nil
}

func TestAcceleratorDesktopProvisionKind(t *testing.T) {
	chartDigest := requiredKindEnv(t, "ACCELERATOR_PROVISION_KIND_CHART_DIGEST")
	imageDigest := requiredKindEnv(t, "ACCELERATOR_PROVISION_KIND_IMAGE_DIGEST")
	sentinel := requiredKindEnv(t, "ACCELERATOR_PROVISION_KIND_SENTINEL")
	registryURL, err := url.Parse(requiredKindEnv(t, "ACCELERATOR_PROVISION_KIND_REGISTRY_TLS_URL"))
	if err != nil || registryURL.Scheme != "https" || registryURL.Host == "" {
		t.Fatal("invalid TLS registry fixture URL")
	}
	certificate, err := os.ReadFile(requiredKindEnv(t, "ACCELERATOR_PROVISION_KIND_REGISTRY_CA"))
	if err != nil {
		t.Fatal("read TLS registry fixture CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certificate) {
		t.Fatal("parse TLS registry fixture CA")
	}
	restoreTransport := helm.SetAcceleratorRegistryTransportForTest(func(base http.RoundTripper) http.RoundTripper {
		transport := base.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		return kindRegistryTransport{base: transport, target: registryURL}
	})
	defer restoreTransport()
	k8sClient, err := k8s.NewClient()
	if err != nil {
		t.Fatal("desktop context fixture")
	}
	if k8sClient.GetCurrentContext() == "" {
		t.Fatal("empty current context")
	}
	helmClient := helm.NewClient()
	// The smoke deliberately exercises the same production adapter used by the
	// dormant desktop constructor; only its network transport is fixture-bound.
	service := New(desktopContexts{client: k8sClient}, &helmChartInstaller{client: helmClient}, KubernetesObserver{})
	entropy := &recordingSequenceEntropy{}
	service.entropy = entropy
	request := kindRequest(k8sClient.GetCurrentContext(), chartDigest, imageDigest)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	result := service.Provision(ctx, request)
	if result.Availability != Available || result.Workload == nil || result.Cleanup != CleanupNotNeeded {
		t.Fatalf("success provisioning outcome availability=%s reason=%s cleanup=%s", result.Availability, result.Reason, result.Cleanup)
	}
	workload := result.Workload
	if workload.ContextName != request.ContextName || workload.ReleaseNamespace != "default" || workload.ReleaseName != "kubikles-accelerator-"+workload.WorkloadSessionID ||
		workload.Job.Name == "" || workload.Job.UID == "" || workload.Pod.Name == "" || workload.Pod.UID == "" ||
		workload.BuildVersion != "v1.2.3" || workload.ImageDigest != imageDigest || workload.ChartDigest != chartDigest {
		t.Fatalf("handle mismatch: %#v", workload)
	}
	assertKindReleaseObjects(t, ctx, k8sClient, workload)
	successRelease, err := helmClient.GetRelease(request.ContextName, "default", workload.ReleaseName)
	if err != nil {
		t.Fatal("successful release was not retained")
	}
	sentinelRelease, err := helmClient.GetRelease(request.ContextName, "default", sentinel)
	if err != nil {
		t.Fatal("sentinel release missing after success")
	}

	failing := request
	failing.Resolution.Release.ImageReference = imageRepository + "@sha256:" + strings.Repeat("e", 64)
	failureCtx, failureCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer failureCancel()
	failed := service.Provision(failureCtx, failing)
	if failed.Availability != Unavailable || failed.Reason != ImagePullFailed || failed.Cleanup != CleanupSucceeded || failed.Workload != nil {
		t.Fatalf("failed attempt outcome availability=%s reason=%s cleanup=%s", failed.Availability, failed.Reason, failed.Cleanup)
	}
	if _, err := helmClient.GetRelease(request.ContextName, "default", "kubikles-accelerator-505152535455565758595a5b5c5d5e5f"); err == nil {
		t.Fatal("failed release remained installed")
	}
	if _, err := helmClient.GetRelease(request.ContextName, "default", workload.ReleaseName); err != nil {
		t.Fatal("rollback removed successful release")
	}
	if _, err := helmClient.GetRelease(request.ContextName, "default", sentinel); err != nil {
		t.Fatal("rollback removed sentinel release")
	}

	entropy.mu.Lock()
	recorded := append([]byte(nil), entropy.data...)
	entropy.mu.Unlock()
	if len(recorded) != 96 {
		t.Fatalf("entropy bytes=%d", len(recorded))
	}
	rawTokens := []string{base64.RawURLEncoding.EncodeToString(recorded[:32]), base64.RawURLEncoding.EncodeToString(recorded[48:80])}
	failedSession := hex.EncodeToString(recorded[80:96])
	assertFailedKindObjectsGone(t, ctx, k8sClient, "default", "kubikles-accelerator-"+failedSession, failedSession)
	secrets, err := k8sClient.SnapshotCurrentContext(request.ContextName)
	if err != nil {
		t.Fatal("resnapshot")
	}
	secretList, err := secrets.Clientset().CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal("secret scan")
	}
	encoded, err := json.Marshal(secretList)
	if err != nil {
		t.Fatal("secret encode")
	}
	releaseState, err := json.Marshal([]interface{}{successRelease, sentinelRelease})
	if err != nil {
		t.Fatal("retained release state encode")
	}
	successOutput, _ := json.Marshal(result)
	failedOutput, _ := json.Marshal(failed)
	formatted := []string{
		string(encoded), string(releaseState), string(successOutput), string(failedOutput),
		strings.Join([]string{workload.ReleaseName, workload.WorkloadSessionID, workload.Job.Name, workload.Pod.Name}, " "),
	}
	for _, output := range formatted {
		for _, token := range rawTokens {
			if strings.Contains(output, token) {
				t.Fatal("raw creator token escaped into Kubernetes or handle metadata")
			}
		}
	}
}

func assertFailedKindObjectsGone(t *testing.T, ctx context.Context, client *k8s.Client, namespace, releaseName, session string) {
	t.Helper()
	snapshot, err := client.SnapshotCurrentContext(client.GetCurrentContext())
	if err != nil {
		t.Fatal("snapshot for failed cleanup assertions")
	}
	baseName := releaseName + "-kubikles-accelerator"
	if len(baseName) > 63 {
		baseName = baseName[:63]
	}
	baseName = strings.TrimSuffix(baseName, "-")
	clusterBase := strings.ToLower(strings.ReplaceAll(namespace+"-"+releaseName+"-accelerator", "_", "-"))
	if len(clusterBase) > 54 {
		clusterBase = clusterBase[:54]
	}
	clusterBase = strings.TrimSuffix(clusterBase, "-")
	digest := sha256.Sum256([]byte(namespace + "/" + releaseName))
	clusterName := clusterBase + "-" + hex.EncodeToString(digest[:])[:8]
	selector := "app.kubernetes.io/instance=" + releaseName

	if _, err := snapshot.Clientset().BatchV1().Jobs(namespace).Get(ctx, baseName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("failed Job identity remained")
	}
	if list, err := snapshot.Clientset().BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector}); err != nil || len(list.Items) != 0 {
		t.Fatal("failed Job set remained")
	}
	if _, err := snapshot.Clientset().CoreV1().ServiceAccounts(namespace).Get(ctx, baseName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("failed ServiceAccount identity remained")
	}
	if list, err := snapshot.Clientset().CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector}); err != nil || len(list.Items) != 0 {
		t.Fatal("failed ServiceAccount set remained")
	}
	if _, err := snapshot.Clientset().CoreV1().Secrets(namespace).Get(ctx, baseName+"-verifier", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("failed Secret identity remained")
	}
	if list, err := snapshot.Clientset().CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector}); err != nil || len(list.Items) != 0 {
		t.Fatal("failed Secret set remained")
	}
	if _, err := snapshot.Clientset().RbacV1().ClusterRoles().Get(ctx, clusterName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("failed ClusterRole identity remained")
	}
	if list, err := snapshot.Clientset().RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{LabelSelector: selector}); err != nil || len(list.Items) != 0 {
		t.Fatal("failed ClusterRole set remained")
	}
	if _, err := snapshot.Clientset().RbacV1().ClusterRoleBindings().Get(ctx, clusterName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("failed ClusterRoleBinding identity remained")
	}
	if list, err := snapshot.Clientset().RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{LabelSelector: selector}); err != nil || len(list.Items) != 0 {
		t.Fatal("failed ClusterRoleBinding set remained")
	}
	if pods, err := snapshot.Clientset().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "kubikles.io/workload-session-id=" + session}); err != nil || len(pods.Items) != 0 {
		t.Fatal("failed session Pods remained")
	}
}

func requiredKindEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("missing mandatory kind fixture %s", name)
	}
	return value
}

func kindRequest(contextName, chartDigest, imageDigest string) Request {
	return Request{ContextName: contextName, Resolution: acceleratorrelease.Resolution{
		Availability: acceleratorrelease.Available, Source: acceleratorrelease.SourceNetwork,
		Release: acceleratorrelease.VerifiedRelease{
			BuildVersion: "v1.2.3", SourceCommit: strings.Repeat("c", 40), DescriptorSHA256: strings.Repeat("d", 64),
			ImageReference: imageRepository + "@" + imageDigest, ChartReference: chartRepository + "@" + chartDigest,
		},
	}}
}

func assertKindReleaseObjects(t *testing.T, ctx context.Context, client *k8s.Client, workload *ProvisionedWorkload) {
	t.Helper()
	snapshot, err := client.SnapshotCurrentContext(workload.ContextName)
	if err != nil {
		t.Fatal("snapshot for live assertions")
	}
	selector := "app.kubernetes.io/instance=" + workload.ReleaseName
	jobs, err := snapshot.Clientset().BatchV1().Jobs(workload.ReleaseNamespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil || len(jobs.Items) != 1 || string(jobs.Items[0].UID) != workload.Job.UID {
		t.Fatal("exact Job identity mismatch")
	}
	accounts, err := snapshot.Clientset().CoreV1().ServiceAccounts(workload.ReleaseNamespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil || len(accounts.Items) != 1 {
		t.Fatal("exact ServiceAccount set mismatch")
	}
	secrets, err := snapshot.Clientset().CoreV1().Secrets(workload.ReleaseNamespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil || len(secrets.Items) != 1 || secrets.Items[0].Immutable == nil || !*secrets.Items[0].Immutable {
		t.Fatal("exact immutable verifier Secret mismatch")
	}
	roles, err := snapshot.Clientset().RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil || len(roles.Items) != 1 {
		t.Fatal("exact ClusterRole set mismatch")
	}
	bindings, err := snapshot.Clientset().RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil || len(bindings.Items) != 1 {
		t.Fatal("exact ClusterRoleBinding set mismatch")
	}
}
