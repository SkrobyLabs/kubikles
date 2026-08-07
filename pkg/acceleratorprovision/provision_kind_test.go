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
	"sync/atomic"
	"testing"
	"time"

	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/agent"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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
	malformed := requiredKindEnv(t, "ACCELERATOR_PROVISION_KIND_MALFORMED")
	registryURL, err := url.Parse(requiredKindEnv(t, "ACCELERATOR_PROVISION_KIND_REGISTRY_TLS_URL"))
	if err != nil || (registryURL.Scheme != "https" && registryURL.Scheme != "http") || registryURL.Host == "" {
		t.Fatal("invalid TLS registry fixture URL")
	}
	pool := x509.NewCertPool()
	if registryURL.Scheme == "https" {
		certificate, readErr := os.ReadFile(requiredKindEnv(t, "ACCELERATOR_PROVISION_KIND_REGISTRY_CA"))
		if readErr != nil {
			t.Fatal("read TLS registry fixture CA")
		}
		if !pool.AppendCertsFromPEM(certificate) {
			t.Fatal("parse TLS registry fixture CA")
		}
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
	if os.Getenv("ACCELERATOR_LIFECYCLE_KIND") == "1" {
		exerciseLifecycleKind(t, service, helmClient, k8sClient, request, sentinel, malformed)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	result := service.Provision(ctx, request)
	if result.Availability != Available || result.Workload == nil || result.Cleanup != CleanupNotNeeded {
		t.Fatalf("success provisioning outcome availability=%s reason=%s cleanup=%s", result.Availability, result.Reason, result.Cleanup)
	}
	workload := result.Workload
	if workload.ContextName != request.ContextName || workload.ReleaseNamespace != kindNamespace() || workload.ReleaseName != "kubikles-accelerator-"+workload.WorkloadSessionID ||
		workload.Job.Name == "" || workload.Job.UID == "" || workload.Pod.Name == "" || workload.Pod.UID == "" ||
		workload.BuildVersion != kindBuildVersion() || workload.ImageDigest != imageDigest || workload.ChartDigest != chartDigest {
		t.Fatalf("handle mismatch: %#v", workload)
	}
	assertKindReleaseObjects(t, ctx, k8sClient, workload)
	if os.Getenv("ACCELERATOR_DISPOSAL_KIND") == "1" {
		exerciseDisposalKind(t, service, helmClient, k8sClient, request, workload, sentinel, malformed)
		return
	}
	if os.Getenv("ACCELERATOR_CONNECT_KIND") == "1" || os.Getenv("ACCELERATOR_RESUME_KIND") == "1" {
		var wrongWorkload, replacementWorkload *ProvisionedWorkload
		if os.Getenv("ACCELERATOR_RESUME_KIND") != "1" {
			wrongAttempt := service.Provision(ctx, request)
			replacementAttempt := service.Provision(ctx, request)
			if wrongAttempt.Availability != Available || wrongAttempt.Workload == nil || replacementAttempt.Availability != Available || replacementAttempt.Workload == nil {
				t.Fatalf("separate connector workloads unavailable wrong=%s/%s/%s replacement=%s/%s/%s", wrongAttempt.Availability, wrongAttempt.Reason, wrongAttempt.Cleanup, replacementAttempt.Availability, replacementAttempt.Reason, replacementAttempt.Cleanup)
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
	successRelease, err := helmClient.GetRelease(request.ContextName, kindNamespace(), workload.ReleaseName)
	if err != nil {
		t.Fatal("successful release was not retained")
	}
	sentinelRelease, err := helmClient.GetRelease(request.ContextName, kindNamespace(), sentinel)
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
	if _, err := helmClient.GetRelease(request.ContextName, kindNamespace(), "kubikles-accelerator-"+failedSession); err == nil {
		t.Fatal("failed release remained installed")
	}
	if _, err := helmClient.GetRelease(request.ContextName, kindNamespace(), workload.ReleaseName); err != nil {
		t.Fatal("rollback removed successful release")
	}
	if _, err := helmClient.GetRelease(request.ContextName, kindNamespace(), sentinel); err != nil {
		t.Fatal("rollback removed sentinel release")
	}

	rawTokens := make([]string, 0, len(recorded)/48)
	for offset := 0; offset < len(recorded); offset += 48 {
		rawTokens = append(rawTokens, base64.RawURLEncoding.EncodeToString(recorded[offset:offset+32]))
	}
	assertFailedKindObjectsGone(t, ctx, k8sClient, kindNamespace(), "kubikles-accelerator-"+failedSession, failedSession)
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

type kindStaticResolver struct{ resolution acceleratorrelease.Resolution }

func (r kindStaticResolver) Resolve(context.Context) acceleratorrelease.Resolution {
	return r.resolution
}

type kindRecordingDisposer struct {
	service *DisposalService
	sweeps  atomic.Int32
}

func (d *kindRecordingDisposer) SweepInert(ctx context.Context, snapshot ContextSnapshot) SweepResult {
	d.sweeps.Add(1)
	return d.service.SweepInert(ctx, snapshot)
}
func (d *kindRecordingDisposer) DisposeNow(ctx context.Context, workload *ProvisionedWorkload) DisposalResult {
	return d.service.DisposeNow(ctx, workload)
}
func (d *kindRecordingDisposer) DrainAndDispose(ctx context.Context, workload *ProvisionedWorkload) DisposalResult {
	return d.service.DrainAndDispose(ctx, workload)
}
func assertKindOneAcceleratorJob(t *testing.T, client *k8s.Client, workload *ProvisionedWorkload) {
	t.Helper()
	snapshot, err := client.SnapshotCurrentContext(workload.ContextName)
	if err != nil {
		t.Fatal("Browser Job snapshot")
	}
	jobs, err := snapshot.Clientset().BatchV1().Jobs(workload.ReleaseNamespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=kubikles-accelerator"})
	if err != nil || len(jobs.Items) != 1 || jobs.Items[0].Name != workload.Job.Name || string(jobs.Items[0].UID) != workload.Job.UID {
		t.Fatal("browser-owned demand created or replaced a Job")
	}
}

func assertKindJobNonterminal(t *testing.T, client *k8s.Client, workload *ProvisionedWorkload) {
	t.Helper()
	snapshot, err := client.SnapshotCurrentContext(workload.ContextName)
	if err != nil {
		t.Fatal("Browser grace Job snapshot")
	}
	job, err := snapshot.Clientset().BatchV1().Jobs(workload.ReleaseNamespace).Get(context.Background(), workload.Job.Name, metav1.GetOptions{})
	if err != nil || string(job.UID) != workload.Job.UID {
		t.Fatal("Browser grace lost the exact Job")
	}
	for _, condition := range job.Status.Conditions {
		if (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) && condition.Status == corev1.ConditionTrue {
			t.Fatal("Browser reconnect did not cancel zero-client grace")
		}
	}
}

func waitKindReleaseGone(t *testing.T, helmClient *helm.Client, workload *ProvisionedWorkload, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := helmClient.GetRelease(workload.ContextName, workload.ReleaseNamespace, workload.ReleaseName); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Browser-owned Helm release remained")
}

func waitKindTunnelDone(t *testing.T, active tunnel, timeout time.Duration) {
	t.Helper()
	if active == nil {
		return
	}
	select {
	case <-active.Done():
	case <-time.After(timeout):
		t.Fatal("Browser-owned tunnel remained open after cleanup")
	}
}

func exerciseKindCrashTTLSweep(t *testing.T, service *Service, helmClient *helm.Client, client *k8s.Client, request Request) {
	t.Helper()
	provisionCtx, provisionCancel := context.WithTimeout(context.Background(), 150*time.Second)
	provisioned := service.Provision(provisionCtx, request)
	provisionCancel()
	if provisioned.Workload == nil {
		t.Fatalf("crash fixture provision reason=%s", provisioned.Reason)
	}
	orphan := provisioned.Workload
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 30*time.Second)
	connected := NewConnector(kindBuildVersion()).Connect(connectCtx, orphan)
	connectCancel()
	if connected.Session == nil || connected.Availability != Available {
		t.Fatal("crash fixture creator connect")
	}
	connected.Session.tunnel.Stop()
	_ = connected.Session.tunnel.Wait(context.Background())
	snapshot, err := client.SnapshotCurrentContext(request.ContextName)
	if err != nil {
		t.Fatal("crash fixture snapshot")
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		job, getErr := snapshot.Clientset().BatchV1().Jobs(orphan.ReleaseNamespace).Get(context.Background(), orphan.Job.Name, metav1.GetOptions{})
		if getErr != nil || string(job.UID) != orphan.Job.UID {
			t.Fatal("crash fixture exact Job disappeared")
		}
		complete := false
		for _, condition := range job.Status.Conditions {
			complete = complete || ((condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) && condition.Status == corev1.ConditionTrue)
		}
		if complete {
			one := int32(1)
			job.Spec.TTLSecondsAfterFinished = &one
			if _, err = snapshot.Clientset().BatchV1().Jobs(orphan.ReleaseNamespace).Update(context.Background(), job, metav1.UpdateOptions{}); err != nil {
				t.Fatal("crash fixture TTL patch")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("crash fixture Job did not exit through server grace")
		}
		time.Sleep(time.Second)
	}
	deadline = time.Now().Add(45 * time.Second)
	for {
		_, jobErr := snapshot.Clientset().BatchV1().Jobs(orphan.ReleaseNamespace).Get(context.Background(), orphan.Job.Name, metav1.GetOptions{})
		_, podErr := snapshot.Clientset().CoreV1().Pods(orphan.ReleaseNamespace).Get(context.Background(), orphan.Pod.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(jobErr) && apierrors.IsNotFound(podErr) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("crash fixture TTL did not remove Job and Pod")
		}
		time.Sleep(500 * time.Millisecond)
	}
	if _, err = helmClient.GetRelease(request.ContextName, orphan.ReleaseNamespace, orphan.ReleaseName); err != nil {
		t.Fatal("crash fixture unexpectedly removed Helm release")
	}

	disposer := &kindRecordingDisposer{service: NewDisposalService(service)}
	coordinator := newCoordinator(desktopContexts{client: client}, kindStaticResolver{resolution: request.Resolution}, service, NewConnector(kindBuildVersion()), NewReconnector(kindBuildVersion()), disposer, processResumeClock{})
	demand := coordinator.AcquireSecretDemand(context.Background(), request.ContextName)
	if !demand.Accepted || demand.Lease == nil {
		t.Fatal("post-crash demand rejected")
	}
	coordinator.Enable(request.ContextName, request.NamespaceOverride)
	_ = waitKindCoordinatorSession(t, coordinator, demand.Lease, request.ContextName, 180*time.Second)
	if disposer.sweeps.Load() != 1 {
		t.Fatal("post-crash first demand omitted one inert sweep")
	}
	if _, err = helmClient.GetRelease(request.ContextName, orphan.ReleaseNamespace, orphan.ReleaseName); err == nil {
		t.Fatal("post-crash sweep retained inert release")
	}
	fresh := kindCoordinatorWorkload(coordinator)
	if fresh == nil || fresh == orphan || fresh.ReleaseName == orphan.ReleaseName {
		t.Fatal("post-crash sweep did not create one fresh workload")
	}
	demand.Lease.Close()
	coordinator.FenceContextSwitch(request.ContextName)
	coordinator.ContextSwitched(request.ContextName, true)
	waitKindReleaseGone(t, helmClient, fresh, 2*time.Minute)
	assertFailedKindObjectsGone(t, context.Background(), client, fresh.ReleaseNamespace, fresh.ReleaseName, fresh.WorkloadSessionID)
	coordinator.Close(context.Background())
}

func exerciseLifecycleKind(t *testing.T, service *Service, helmClient *helm.Client, client *k8s.Client, request Request, sentinel, malformed string) {
	t.Helper()
	contextName := request.ContextName
	disposer := &kindRecordingDisposer{service: NewDisposalService(service)}
	coordinator := newCoordinator(desktopContexts{client: client}, kindStaticResolver{resolution: request.Resolution}, service, NewConnector(kindBuildVersion()), NewReconnector(kindBuildVersion()), disposer, processResumeClock{})
	pollDone := make(chan struct{})
	pollFailure := make(chan string, 1)
	go pollKindKubectlPortForward(pollDone, pollFailure)
	defer close(pollDone)

	directSnapshot, err := client.SnapshotCurrentContext(contextName)
	if err != nil {
		t.Fatal("lifecycle direct snapshot")
	}
	directTripwire := func() {
		if _, listErr := directSnapshot.Clientset().CoreV1().Secrets(kindNamespace()).List(context.Background(), metav1.ListOptions{Limit: 1}); listErr != nil {
			t.Fatal("direct Secret tripwire unavailable")
		}
	}
	directTripwire()
	first := coordinator.AcquireSecretDemand(context.Background(), contextName)
	if !first.Accepted || first.Lease == nil {
		t.Fatalf("first demand=%#v", first)
	}
	if snapshot := coordinator.Snapshot(contextName); snapshot.State != CoordinatorDirectOnly || snapshot.Enabled || kindCoordinatorWorkload(coordinator) != nil || disposer.sweeps.Load() != 0 {
		t.Fatalf("disabled demand started lifecycle: %#v", snapshot)
	}
	coordinator.Enable(contextName, request.NamespaceOverride)
	active := waitKindCoordinatorSession(t, coordinator, first.Lease, contextName, 180*time.Second)
	firstIdentity := active.Identity()
	workload := kindCoordinatorWorkload(coordinator)
	if workload == nil || disposer.sweeps.Load() != 1 {
		t.Fatal("first demand omitted sweep/workload/credential")
	}
	credential := workload.credential
	if credential == nil {
		t.Fatal("first demand omitted creator credential")
	}
	directTripwire()

	lost := first.Lease.Changes()
	if err = active.socket.Close(); err != nil {
		t.Fatal("force lifecycle transport drop")
	}
	select {
	case <-lost:
	case <-time.After(10 * time.Second):
		t.Fatal("transport loss did not revoke availability")
	}
	resumed := waitKindCoordinatorSession(t, coordinator, first.Lease, contextName, 45*time.Second)
	resumedIdentity := resumed.Identity()
	assertKindSameWorkloadHigherGeneration(t, firstIdentity, resumedIdentity)

	first.Lease.Close()
	waitKindCoordinatorState(t, coordinator, contextName, CoordinatorActive, 10*time.Second)
	returned := coordinator.AcquireSecretDemand(context.Background(), contextName)
	if !returned.Accepted || returned.Lease == nil {
		t.Fatal("enabled connection demand rejected")
	}
	reused := waitKindCoordinatorSession(t, coordinator, returned.Lease, contextName, 45*time.Second)
	if reused.Identity() != resumedIdentity {
		t.Fatal("zero-demand enabled connection did not retain its active session")
	}
	if disposer.sweeps.Load() != 1 || kindCoordinatorWorkload(coordinator) != workload {
		t.Fatal("enabled connection return created a new workload or sweep")
	}

	returned.Lease.Close()
	coordinator.Disable(contextName)
	waitKindCoordinatorState(t, coordinator, contextName, CoordinatorDirectOnly, 5*time.Minute)
	if credential.withCreatorAuthorization(context.Background(), func(context.Context, creatorAuthorizationLease) error { return nil }) == nil {
		t.Fatal("final lifecycle drain retained creator credential")
	}
	if _, err = helmClient.GetRelease(contextName, workload.ReleaseNamespace, workload.ReleaseName); err == nil {
		t.Fatal("final lifecycle drain retained Helm release")
	}
	assertFailedKindObjectsGone(t, context.Background(), client, workload.ReleaseNamespace, workload.ReleaseName, workload.WorkloadSessionID)
	for _, retained := range []string{sentinel, malformed} {
		if _, err = helmClient.GetRelease(contextName, kindNamespace(), retained); err != nil {
			t.Fatalf("lifecycle cleanup removed sentinel %s", retained)
		}
	}
	directTripwire()
	select {
	case process := <-pollFailure:
		t.Fatalf("coordinator invoked kubectl port-forward: %s", process)
	default:
	}
}

func kindCoordinatorWorkload(coordinator *Coordinator) *ProvisionedWorkload {
	coordinator.mu.Lock()
	slot := coordinator.slots[coordinator.currentEpoch]
	coordinator.mu.Unlock()
	if slot == nil {
		return nil
	}
	slot.mu.Lock()
	defer slot.mu.Unlock()
	return slot.workload
}

func waitKindCoordinatorSession(t *testing.T, coordinator *Coordinator, demand *SecretDemandLease, contextName string, timeout time.Duration) *ConnectedSession {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if lease, ok := demand.TrySession(); ok {
			session := lease.Session()
			lease.Close()
			return session
		}
		if snapshot := coordinator.Snapshot(contextName); snapshot.State == CoordinatorUnavailable || snapshot.State == CoordinatorClosed {
			t.Fatalf("coordinator session state=%s", snapshot.State)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("coordinator session timeout state=%s", coordinator.Snapshot(contextName).State)
	return nil
}

func waitKindCoordinatorState(t *testing.T, coordinator *Coordinator, contextName string, state CoordinatorState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if coordinator.Snapshot(contextName).State == state {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("coordinator state=%s want=%s", coordinator.Snapshot(contextName).State, state)
}

func assertKindSameWorkloadHigherGeneration(t *testing.T, before, after SessionIdentity) {
	t.Helper()
	if after.Pod != before.Pod || after.Job != before.Job || after.WorkloadSessionID != before.WorkloadSessionID || after.BuildVersion != before.BuildVersion || after.ImageDigest != before.ImageDigest || after.ChartDigest != before.ChartDigest || after.InstanceID != before.InstanceID || after.SessionID != before.SessionID || after.Generation <= before.Generation {
		t.Fatal("lifecycle Resume changed workload identity or did not increase generation")
	}
}

func exerciseDisposalKind(t *testing.T, service *Service, helmClient *helm.Client, client *k8s.Client, request Request, workload *ProvisionedWorkload, sentinel, malformed string) {
	t.Helper()
	preflightSnapshot, preflightErr := client.SnapshotCurrentContext(request.ContextName)
	if preflightErr != nil {
		t.Fatal("sweep proof preflight snapshot")
	}
	if stage := helmClient.AcceleratorSweepProofStageForTest(context.Background(), preflightSnapshot.RESTConfig(), preflightSnapshot.Namespace(), workload.ReleaseName); stage != "stored_ok" {
		t.Fatalf("sweep proof preflight stage=%s", stage)
	}
	connect := func(candidate *ProvisionedWorkload) *ConnectedSession {
		connectCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result := NewConnector(kindBuildVersion()).Connect(connectCtx, candidate)
		if result.Availability != Available || result.Session == nil {
			t.Fatalf("disposal fixture connect reason=%s", result.Reason)
		}
		return result.Session
	}
	assertGone := func(candidate *ProvisionedWorkload) {
		if _, err := helmClient.GetRelease(request.ContextName, candidate.ReleaseNamespace, candidate.ReleaseName); err == nil {
			t.Fatalf("disposed release remained: %s", candidate.ReleaseName)
		}
		assertFailedKindObjectsGone(t, context.Background(), client, candidate.ReleaseNamespace, candidate.ReleaseName, candidate.WorkloadSessionID)
		if _, err := helmClient.GetRelease(request.ContextName, kindNamespace(), sentinel); err != nil {
			t.Fatal("disposal removed sentinel release")
		}
		if _, err := helmClient.GetRelease(request.ContextName, kindNamespace(), malformed); err != nil {
			t.Fatal("disposal removed malformed Accelerator-shaped release")
		}
	}

	_ = connect(workload)
	credential := workload.credential
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 4*time.Minute)
	drained := NewDisposalService(service).DrainAndDispose(drainCtx, workload)
	drainCancel()
	if drained.Quiescence != QuiescenceSucceeded || drained.Credential != CredentialDestroyed || drained.Observation != DrainComplete || drained.Ownership != OwnershipProven || drained.Uninstall != UninstallSucceeded || drained.Disappearance != DisappearanceSucceeded {
		t.Fatalf("drain requested=%s effective=%s observation=%s quiescence=%s credential=%s ownership=%s uninstall=%s disappearance=%s", drained.Requested, drained.Effective, drained.Observation, drained.Quiescence, drained.Credential, drained.Ownership, drained.Uninstall, drained.Disappearance)
	}
	if credential.withCreatorAuthorization(context.Background(), func(context.Context, creatorAuthorizationLease) error { return nil }) == nil {
		t.Fatal("drain retained creator credential")
	}
	assertGone(workload)

	immediateCtx, immediateCancel := context.WithTimeout(context.Background(), 150*time.Second)
	immediateProvision := service.Provision(immediateCtx, request)
	immediateCancel()
	if immediateProvision.Workload == nil {
		t.Fatalf("immediate provision reason=%s", immediateProvision.Reason)
	}
	immediate := immediateProvision.Workload
	_ = connect(immediate)
	started := time.Now()
	now := NewDisposalService(service).DisposeNow(context.Background(), immediate)
	if time.Since(started) >= agent.AcceleratorIdleReconnectGrace || now.Observation != DrainSkipped || now.Uninstall != UninstallSucceeded {
		t.Fatalf("immediate result=%#v elapsed=%s", now, time.Since(started))
	}
	assertGone(immediate)

	orphanCtx, orphanCancel := context.WithTimeout(context.Background(), 150*time.Second)
	orphanProvision := service.Provision(orphanCtx, request)
	orphanCancel()
	if orphanProvision.Workload == nil {
		t.Fatalf("orphan provision reason=%s", orphanProvision.Reason)
	}
	orphan := orphanProvision.Workload
	session := connect(orphan)
	if err := session.Close(context.Background()); err != nil {
		t.Fatal("orphan session close")
	}
	snapshot, err := client.SnapshotCurrentContext(request.ContextName)
	if err != nil {
		t.Fatal("orphan snapshot")
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		job, getErr := snapshot.Clientset().BatchV1().Jobs(orphan.ReleaseNamespace).Get(context.Background(), orphan.Job.Name, metav1.GetOptions{})
		if getErr != nil {
			t.Fatal("orphan job disappeared before TTL patch")
		}
		complete := false
		for _, condition := range job.Status.Conditions {
			complete = complete || condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue
		}
		if complete {
			if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished != 3600 {
				t.Fatal("chart TTL contract changed")
			}
			one := int32(1)
			job.Spec.TTLSecondsAfterFinished = &one
			if _, err = snapshot.Clientset().BatchV1().Jobs(orphan.ReleaseNamespace).Update(context.Background(), job, metav1.UpdateOptions{}); err != nil {
				t.Fatal("test-only TTL patch")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("orphan job did not complete")
		}
		time.Sleep(time.Second)
	}
	deadline = time.Now().Add(45 * time.Second)
	for {
		_, jobErr := snapshot.Clientset().BatchV1().Jobs(orphan.ReleaseNamespace).Get(context.Background(), orphan.Job.Name, metav1.GetOptions{})
		_, podErr := snapshot.Clientset().CoreV1().Pods(orphan.ReleaseNamespace).Get(context.Background(), orphan.Pod.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(jobErr) && apierrors.IsNotFound(podErr) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test TTL did not remove Job and Pod")
		}
		time.Sleep(500 * time.Millisecond)
	}
	orphanName := orphan.ReleaseName
	orphanProvision.Workload = nil
	orphan = nil
	activeCtx, activeCancel := context.WithTimeout(context.Background(), 150*time.Second)
	activeProvision := service.Provision(activeCtx, request)
	activeCancel()
	if activeProvision.Workload == nil {
		t.Fatalf("active sweep sentinel provision reason=%s", activeProvision.Reason)
	}
	active := activeProvision.Workload
	_ = connect(active)
	sweepCtx, sweepCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	sweep := NewDisposalService(service).SweepInert(sweepCtx, snapshot)
	sweepCancel()
	cleaned, retained, malformedRetained := false, false, false
	for _, candidate := range sweep.Candidates {
		cleaned = cleaned || candidate.ReleaseName == orphanName && candidate.Status == SweepCleaned
		retained = retained || candidate.ReleaseName == active.ReleaseName && candidate.Status == SweepActiveOrAmbiguous
		malformedRetained = malformedRetained || candidate.ReleaseName == malformed && candidate.Status == SweepUnsupportedMalformed
	}
	if sweep.Status != SweepCompleted || !cleaned || !retained || !malformedRetained {
		orphanStage := helmClient.AcceleratorSweepProofStageForTest(context.Background(), snapshot.RESTConfig(), snapshot.Namespace(), orphanName)
		activeStage := helmClient.AcceleratorSweepProofStageForTest(context.Background(), snapshot.RESTConfig(), snapshot.Namespace(), active.ReleaseName)
		t.Fatalf("sweep status=%s candidates=%v orphan-stage=%s active-stage=%s", sweep.Status, sweep.Candidates, orphanStage, activeStage)
	}
	if _, err = helmClient.GetRelease(request.ContextName, kindNamespace(), orphanName); err == nil {
		t.Fatal("orphan release remained")
	}
	if _, err = helmClient.GetRelease(request.ContextName, kindNamespace(), active.ReleaseName); err != nil {
		t.Fatal("sweep removed active Accelerator")
	}
	if _, err = helmClient.GetRelease(request.ContextName, kindNamespace(), sentinel); err != nil {
		t.Fatal("sweep removed sentinel")
	}
	if _, err = helmClient.GetRelease(request.ContextName, kindNamespace(), malformed); err != nil {
		t.Fatal("sweep removed malformed Accelerator-shaped release")
	}
	_ = NewDisposalService(service).DisposeNow(context.Background(), active)
}

func exerciseConnectorKind(t *testing.T, ctx context.Context, client *k8s.Client, workload, wrong, replacementWorkload *ProvisionedWorkload) {
	t.Helper()
	connector := NewConnector(kindBuildVersion())
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
		reconnector := NewReconnector(kindBuildVersion())
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
	wrongResult := NewConnector(kindBuildVersion()).Connect(ctx, wrong)
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
	pods := snapshot.Clientset().CoreV1().Pods(replacementWorkload.ReleaseNamespace)
	deletionState := "present"
	err = wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		_, getErr := pods.Get(ctx, replacementWorkload.Pod.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			deletionState = "deleted"
			return true, nil
		}
		if getErr != nil {
			deletionState = "read-unavailable"
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("original Pod deletion did not complete (state=%s)", deletionState)
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
	replacement, err = pods.Create(ctx, replacement, metav1.CreateOptions{})
	if err != nil || replacement.Name != replacementWorkload.Pod.Name || string(replacement.UID) == replacementWorkload.Pod.UID {
		t.Fatal("same-name replacement Pod fixture failed")
	}
	readinessState := "not-observed"
	err = wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 90*time.Second, true, func(ctx context.Context) (bool, error) {
		current, getErr := pods.Get(ctx, replacementWorkload.Pod.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			readinessState = "not-found"
			return false, nil
		}
		if getErr != nil {
			readinessState = "read-unavailable"
			return false, nil
		}
		ready, state := replacementPodReadiness(current)
		readinessState = state
		return ready, nil
	})
	if err != nil {
		t.Fatalf("replacement Pod did not become Running and Ready (state=%s)", readinessState)
	}
	replacementConnector := NewConnector(kindBuildVersion())
	replacementConnector.observeEndpoint = func(string) { t.Fatal("replacement reached tunnel/authentication") }
	replacementResult := replacementConnector.Connect(ctx, replacementWorkload)
	if replacementResult.Availability != Unavailable || replacementResult.Session != nil || (replacementResult.Reason != WorkloadUnavailable && replacementResult.Reason != WorkloadChanged) {
		t.Fatal("connector followed a replacement Pod")
	}
}

func replacementPodReadiness(pod *corev1.Pod) (bool, string) {
	if pod == nil {
		return false, "not-observed"
	}
	if pod.Status.Phase != corev1.PodRunning {
		switch pod.Status.Phase {
		case corev1.PodPending:
			return false, "pending"
		case corev1.PodSucceeded:
			return false, "succeeded"
		case corev1.PodFailed:
			return false, "failed"
		default:
			return false, "phase-unknown"
		}
	}
	if len(pod.Status.ContainerStatuses) != 1 {
		return false, "container-status-count"
	}
	if !pod.Status.ContainerStatuses[0].Ready {
		return false, "container-not-ready"
	}
	return true, "ready"
}

func TestReplacementPodReadiness(t *testing.T) {
	tests := []struct {
		name      string
		pod       *corev1.Pod
		wantReady bool
		wantState string
	}{
		{name: "not observed", wantState: "not-observed"},
		{name: "pending", pod: &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}}, wantState: "pending"},
		{name: "missing container status", pod: &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}, wantState: "container-status-count"},
		{name: "container not ready", pod: &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Ready: false}}}}, wantState: "container-not-ready"},
		{name: "ready", pod: &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Ready: true}}}}, wantReady: true, wantState: "ready"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ready, state := replacementPodReadiness(test.pod)
			if ready != test.wantReady || state != test.wantState {
				t.Fatalf("readiness=%t state=%s", ready, state)
			}
		})
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
			BuildVersion: kindBuildVersion(), SourceCommit: strings.Repeat("c", 40), DescriptorSHA256: strings.Repeat("d", 64),
			ImageReference: imageRepository + "@" + imageDigest, ChartReference: chartRepository + "@" + chartDigest,
		},
	}}
}

func kindBuildVersion() string {
	if value := os.Getenv("BUILD_VERSION"); value != "" {
		return value
	}
	return "v1.2.3"
}

func kindNamespace() string {
	if value := os.Getenv("KUBIKLES_ACCELERATOR_E2E_NAMESPACE"); value != "" {
		return value
	}
	return "default"
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
