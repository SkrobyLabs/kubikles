//go:build accelerator_provision_kind && helm

package acceleratorprovision

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/agent"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"

	corev1 "k8s.io/api/core/v1"
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
	if os.Getenv("ACCELERATOR_CONNECT_KIND") == "1" || os.Getenv("ACCELERATOR_RESUME_KIND") == "1" {
		var wrongWorkload, replacementWorkload *ProvisionedWorkload
		if os.Getenv("ACCELERATOR_RESUME_KIND") != "1" {
			wrongAttempt := service.Provision(ctx, request)
			replacementAttempt := service.Provision(ctx, request)
			if wrongAttempt.Availability != Available || wrongAttempt.Workload == nil || replacementAttempt.Availability != Available || replacementAttempt.Workload == nil {
				t.Fatal("separate connector workloads were not provisioned")
			}
			wrongWorkload, replacementWorkload = wrongAttempt.Workload, replacementAttempt.Workload
			assertKindReleaseObjects(t, ctx, k8sClient, wrongWorkload)
			assertKindReleaseObjects(t, ctx, k8sClient, replacementWorkload)
		}
		exerciseConnectorKind(t, ctx, k8sClient, workload, wrongWorkload, replacementWorkload)
		assertKindReleaseObjects(t, ctx, k8sClient, workload)
		for _, retained := range []*ProvisionedWorkload{wrongWorkload, replacementWorkload} {
			if retained == nil {
				continue
			}
			if _, err := helmClient.GetRelease(request.ContextName, retained.ReleaseNamespace, retained.ReleaseName); err != nil {
				t.Fatalf("connector failure removed retained release %s", retained.ReleaseName)
			}
		}
	}
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
	entropy.mu.Lock()
	recorded := append([]byte(nil), entropy.data...)
	entropy.mu.Unlock()
	expectedEntropyBytes := 96
	if os.Getenv("ACCELERATOR_CONNECT_KIND") == "1" && os.Getenv("ACCELERATOR_RESUME_KIND") != "1" {
		expectedEntropyBytes = 192
	}
	if len(recorded) != expectedEntropyBytes {
		t.Fatalf("entropy bytes=%d", len(recorded))
	}
	failedSession := hex.EncodeToString(recorded[len(recorded)-16:])
	if _, err := helmClient.GetRelease(request.ContextName, "default", "kubikles-accelerator-"+failedSession); err == nil {
		t.Fatal("failed release remained installed")
	}
	if _, err := helmClient.GetRelease(request.ContextName, "default", workload.ReleaseName); err != nil {
		t.Fatal("rollback removed successful release")
	}
	if _, err := helmClient.GetRelease(request.ContextName, "default", sentinel); err != nil {
		t.Fatal("rollback removed sentinel release")
	}

	rawTokens := make([]string, 0, len(recorded)/48)
	for offset := 0; offset < len(recorded); offset += 48 {
		rawTokens = append(rawTokens, base64.RawURLEncoding.EncodeToString(recorded[offset:offset+32]))
	}
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

func exerciseConnectorKind(t *testing.T, ctx context.Context, client *k8s.Client, workload, wrong, replacementWorkload *ProvisionedWorkload) {
	t.Helper()
	connector := NewConnector("v1.2.3")
	var privateEndpoint string
	connector.observeEndpoint = func(endpoint string) { privateEndpoint = endpoint }
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	pollDone := make(chan struct{})
	pollFailure := make(chan string, 1)
	go pollKindKubectlPortForward(pollDone, pollFailure)
	result := connector.Connect(connectCtx, workload)
	cancel()
	if result.Availability != Available || result.Session == nil || result.Reason != "" {
		t.Fatalf("connector unavailable: availability=%s reason=%s", result.Availability, result.Reason)
	}
	host, port, err := net.SplitHostPort(privateEndpoint)
	if err != nil || host != "127.0.0.1" || port == "" || port == "0" {
		t.Fatal("connector did not use one OS-assigned IPv4-loopback endpoint")
	}
	identity := result.Session.Identity()
	if identity.Job != workload.Job || identity.Pod != workload.Pod || identity.BuildVersion != workload.BuildVersion || identity.InstanceID == "" || identity.SessionID == "" || identity.Generation != 1 {
		t.Fatal("connected session identity mismatch")
	}
	if capabilities := result.Session.Capabilities(); !reflect.DeepEqual(capabilities, agent.V1Capabilities()) {
		t.Fatal("connected capability snapshot mismatch")
	}
	if os.Getenv("ACCELERATOR_RESUME_KIND") == "1" {
		priorTunnel := result.Session.tunnel
		if err = result.Session.socket.Close(); err != nil {
			t.Fatal("force creator transport drop")
		}
		select {
		case <-result.Session.Done():
		case <-time.After(10 * time.Second):
			t.Fatal("creator transport drop was not observed")
		}
		if result.Session.EndReason() != SessionPeerClosed {
			t.Fatalf("forced drop reason=%s", result.Session.EndReason())
		}
		resumeCtx, resumeCancel := context.WithTimeout(ctx, 30*time.Second)
		reconnector := NewReconnector("v1.2.3")
		var resumeEndpoint string
		reconnector.connector.observeEndpoint = func(endpoint string) { resumeEndpoint = endpoint }
		resumed := reconnector.Resume(resumeCtx, ResumeRequest{Prior: result.Session, Workload: workload})
		resumeCancel()
		if resumed.Availability != Available || resumed.Session == nil || resumed.Reason != "" {
			t.Fatalf("resume unavailable: availability=%s reason=%s", resumed.Availability, resumed.Reason)
		}
		resumedIdentity := resumed.Session.Identity()
		if resumedIdentity.Pod != identity.Pod || resumedIdentity.Job != identity.Job || resumedIdentity.WorkloadSessionID != identity.WorkloadSessionID || resumedIdentity.BuildVersion != identity.BuildVersion || resumedIdentity.ImageDigest != identity.ImageDigest || resumedIdentity.ChartDigest != identity.ChartDigest || resumedIdentity.InstanceID != identity.InstanceID || resumedIdentity.SessionID != identity.SessionID || resumedIdentity.Generation <= identity.Generation {
			t.Fatal("resumed session did not retain exact identity with a higher generation")
		}
		resumeHost, resumePort, splitErr := net.SplitHostPort(resumeEndpoint)
		if resumed.Session.tunnel == priorTunnel || splitErr != nil || resumeHost != "127.0.0.1" || resumePort == "" || resumePort == "0" {
			t.Fatal("resume did not establish a new private exact-Pod tunnel")
		}
		resumeSafeOutputs := []interface{}{resumed, resumed.Session, resumed.Session.Identity(), ResumeRequest{Prior: result.Session, Workload: workload}}
		resumeUnsafe := []string{string(workload.credential.encoded[:]), workload.credential.verifier, resumeEndpoint, "Authorization"}
		for _, value := range resumeSafeOutputs {
			for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
				assertNoCredentialCorpus(t, fmt.Sprintf(verb, value), resumeUnsafe)
			}
			encoded, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				t.Fatal("resume safe output encode")
			}
			assertNoCredentialCorpus(t, string(encoded), resumeUnsafe)
		}
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err = resumed.Session.Close(closeCtx); err != nil {
			closeCancel()
			t.Fatal("resumed session close failed")
		}
		closeCancel()
		if resumed.Session.EndReason() != SessionClosed {
			t.Fatal("resumed session did not explicitly close")
		}
	} else {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err = result.Session.Close(closeCtx); err != nil {
			closeCancel()
			t.Fatal("connector close failed")
		}
		closeCancel()
		if result.Session.EndReason() != SessionClosed {
			t.Fatal("connector did not publish closed end reason")
		}
	}
	close(pollDone)
	select {
	case process := <-pollFailure:
		t.Fatalf("connector/reconnector invoked kubectl port-forward: %s", process)
	default:
	}
	if os.Getenv("ACCELERATOR_RESUME_KIND") == "1" {
		return
	}

	wrongCredential, err := generateCreatorCredential(bytes.NewReader(bytes.Repeat([]byte{0xff}, 48)))
	if err != nil {
		t.Fatal("wrong-token fixture")
	}
	wrong.credential = wrongCredential
	wrongResult := NewConnector("v1.2.3").Connect(ctx, wrong)
	if wrongResult.Availability != Unavailable || wrongResult.Reason != AcceleratorUnavailable || wrongResult.Session != nil {
		t.Fatal("wrong creator token did not fail closed")
	}

	snapshot, err := client.SnapshotCurrentContext(replacementWorkload.ContextName)
	if err != nil {
		t.Fatal("snapshot before Pod replacement")
	}
	originalPod, err := snapshot.Clientset().CoreV1().Pods(replacementWorkload.ReleaseNamespace).Get(ctx, replacementWorkload.Pod.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal("read exact Pod for replacement fence")
	}
	zero := int64(0)
	if err = snapshot.Clientset().CoreV1().Pods(replacementWorkload.ReleaseNamespace).Delete(ctx, replacementWorkload.Pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero}); err != nil {
		t.Fatal("delete exact Pod for replacement fence")
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_, getErr := snapshot.Clientset().CoreV1().Pods(replacementWorkload.ReleaseNamespace).Get(ctx, replacementWorkload.Pod.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	replacement := originalPod.DeepCopy()
	replacement.ResourceVersion = ""
	replacement.UID = ""
	replacement.CreationTimestamp = metav1.Time{}
	replacement.DeletionTimestamp = nil
	replacement.DeletionGracePeriodSeconds = nil
	replacement.ManagedFields = nil
	replacement.Finalizers = nil
	replacement.GenerateName = ""
	replacement.Status = corev1.PodStatus{}
	replacement.Spec.NodeName = ""
	replacement, err = snapshot.Clientset().CoreV1().Pods(replacementWorkload.ReleaseNamespace).Create(ctx, replacement, metav1.CreateOptions{})
	if err != nil || replacement.Name != replacementWorkload.Pod.Name || string(replacement.UID) == replacementWorkload.Pod.UID {
		t.Fatal("same-name replacement Pod fixture failed")
	}
	readyReplacement := false
	for time.Now().Before(deadline) {
		current, getErr := snapshot.Clientset().CoreV1().Pods(replacementWorkload.ReleaseNamespace).Get(ctx, replacementWorkload.Pod.Name, metav1.GetOptions{})
		if getErr == nil && current.Status.Phase == corev1.PodRunning && len(current.Status.ContainerStatuses) == 1 && current.Status.ContainerStatuses[0].Ready {
			readyReplacement = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !readyReplacement {
		t.Fatal("replacement Pod did not become Running and Ready")
	}
	replacementConnector := NewConnector("v1.2.3")
	replacementConnector.observeEndpoint = func(string) { t.Fatal("replacement reached tunnel/authentication") }
	replacementResult := replacementConnector.Connect(ctx, replacementWorkload)
	if replacementResult.Availability != Unavailable || replacementResult.Session != nil || (replacementResult.Reason != WorkloadUnavailable && replacementResult.Reason != WorkloadChanged) {
		t.Fatal("connector followed a replacement Pod")
	}
}

func pollKindKubectlPortForward(done <-chan struct{}, failure chan<- string) {
	check := func() bool {
		output, err := exec.Command("ps", "-axo", "command").Output()
		if err != nil {
			return false
		}
		for _, line := range strings.Split(string(output), "\n") {
			if strings.Contains(line, "kubectl") && strings.Contains(line, "port-forward") {
				select {
				case failure <- line:
				default:
				}
				return true
			}
		}
		return false
	}
	if check() {
		return
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if check() {
				return
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
