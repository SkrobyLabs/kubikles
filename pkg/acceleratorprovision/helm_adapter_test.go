//go:build helm

package acceleratorprovision

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kubikles/pkg/helm"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

func TestCleanupCapturesExactUIDsAndIgnoresReplacements(t *testing.T) {
	prepared, request := adapterPreparedRelease(t)
	jobUID := types.UID("old-job")
	captured := make([]capturedAcceleratorResource, 0, len(prepared.ResourceIdentities()))
	for _, identity := range prepared.ResourceIdentities() {
		uid := types.UID("old-" + strings.ToLower(identity.Kind))
		if identity.Kind == "Job" {
			uid = jobUID
		}
		captured = append(captured, capturedAcceleratorResource{identity: identity, uid: uid})
	}

	replacements := acceleratorLiveObjects(prepared.ResourceIdentities(), request, "replacement-")
	controller := true
	unrelated := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "unrelated", Namespace: request.ReleaseNamespace, UID: "unrelated-pod",
		Labels:          map[string]string{"kubikles.io/workload-session-id": request.WorkloadSession},
		OwnerReferences: []metav1.OwnerReference{{Kind: "Job", UID: "different-job", Controller: &controller}},
	}}
	replacements = append(replacements, unrelated)
	replacementSnapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: fake.NewSimpleClientset(replacements...)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	barrier, ok := establishAcceleratorPodBarrier(ctx, replacementSnapshot, jobUID, request.WorkloadSession)
	if !ok || !waitAcceleratorResourcesGone(ctx, replacementSnapshot, captured, jobUID, request.WorkloadSession, barrier) {
		t.Fatal("same-name replacement or unrelated label match was followed")
	}
	barrier.stop()

	t.Run("controller owned pod must disappear", func(t *testing.T) {
		ownedPod := unrelated.DeepCopy()
		ownedPod.Name, ownedPod.UID, ownedPod.OwnerReferences[0].UID = "owned", "owned-pod", jobUID
		podClient := fake.NewSimpleClientset(ownedPod)
		podSnapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: podClient}
		barrier, ok := establishAcceleratorPodBarrier(context.Background(), podSnapshot, jobUID, request.WorkloadSession)
		if !ok {
			t.Fatal("pod barrier")
		}
		defer barrier.stop()
		result := make(chan bool, 1)
		waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
		defer waitCancel()
		go func() {
			result <- waitAcceleratorResourcesGone(waitCtx, podSnapshot, captured, jobUID, request.WorkloadSession, barrier)
		}()
		select {
		case <-result:
			t.Fatal("returned while original controller-owned Pod remained")
		case <-time.After(30 * time.Millisecond):
		}
		if err := podClient.CoreV1().Pods(request.ReleaseNamespace).Delete(context.Background(), ownedPod.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatal(err)
		}
		select {
		case ok := <-result:
			if !ok {
				t.Fatal("wait failed after exact Pod disappeared")
			}
		case <-time.After(time.Second):
			t.Fatal("wait did not finish")
		}
	})

	t.Run("late controller Pod is covered by pre-delete watch barrier", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		snapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: client}
		barrier, ok := establishAcceleratorPodBarrier(context.Background(), snapshot, jobUID, request.WorkloadSession)
		if !ok {
			t.Fatal("pod barrier")
		}
		defer barrier.stop()
		controller := true
		late := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "late", Namespace: request.ReleaseNamespace, UID: "late-pod",
			Labels:          map[string]string{"kubikles.io/workload-session-id": request.WorkloadSession},
			OwnerReferences: []metav1.OwnerReference{{Kind: "Job", UID: jobUID, Controller: &controller}},
		}}
		if _, err := client.CoreV1().Pods(request.ReleaseNamespace).Create(context.Background(), late, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		result := make(chan bool, 1)
		go func() {
			result <- waitAcceleratorResourcesGone(context.Background(), snapshot, nil, jobUID, request.WorkloadSession, barrier)
		}()
		select {
		case <-result:
			t.Fatal("returned while late controller Pod remained")
		case <-time.After(30 * time.Millisecond):
		}
		if err := client.CoreV1().Pods(request.ReleaseNamespace).Delete(context.Background(), late.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatal(err)
		}
		select {
		case ok := <-result:
			if !ok {
				t.Fatal("wait failed")
			}
		case <-time.After(time.Second):
			t.Fatal("wait did not finish")
		}
		actions := client.Actions()
		watchIndex, createIndex := -1, -1
		for index, action := range actions {
			if action.GetVerb() == "watch" && watchIndex < 0 {
				watchIndex = index
			}
			if action.GetVerb() == "create" && createIndex < 0 {
				createIndex = index
			}
		}
		if watchIndex < 0 || createIndex < 0 || watchIndex > createIndex {
			t.Fatalf("watch barrier was not established before late create: %#v", actions)
		}
	})

	t.Run("no Job receipt refuses any session Pod", func(t *testing.T) {
		podSnapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: fake.NewSimpleClientset(unrelated)}
		if _, ok := establishAcceleratorPodBarrier(context.Background(), podSnapshot, "", request.WorkloadSession); ok {
			t.Fatal("accepted a session Pod without created Job authority")
		}
	})
}

func TestOwnedDisappearanceWaitGetsExactPodAndNeverFollowsReplacement(t *testing.T) {
	const namespace = "default"
	podIdentity := ObjectIdentity{Name: "owned-pod", UID: "owned-pod-uid"}
	for _, test := range []struct {
		name       string
		object     runtime.Object
		apiError   error
		want       DisappearanceStatus
		deadline   bool
		wantPolls  int
		wantPodGET int
	}{
		{name: "pod absent", want: DisappearanceSucceeded, wantPodGET: 1},
		{name: "same UID through injected deadline", object: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podIdentity.Name, Namespace: namespace, UID: types.UID(podIdentity.UID)}}, want: DisappearanceResourcesRemaining, deadline: true, wantPolls: 1, wantPodGET: 1},
		{name: "replacement UID", object: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podIdentity.Name, Namespace: namespace, UID: "replacement"}}, want: DisappearanceUIDReplaced, wantPodGET: 1},
		{name: "Pod API error", apiError: errors.New("hostile Pod GET Bearer secret-token"), want: DisappearanceResourcesRemaining, deadline: true, wantPolls: 1, wantPodGET: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var objects []runtime.Object
			if test.object != nil {
				objects = append(objects, test.object)
			}
			client := fake.NewSimpleClientset(objects...)
			podGETs := 0
			client.PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
				get := action.(k8stesting.GetAction)
				if get.GetName() != podIdentity.Name {
					t.Fatalf("followed Pod name %q", get.GetName())
				}
				podGETs++
				if test.apiError != nil {
					return true, nil, test.apiError
				}
				return false, nil, nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			if test.deadline {
				ctx, cancel = context.WithDeadline(context.Background(), time.Unix(0, 0))
			}
			defer cancel()
			polls := 0
			pollWait := func(waitCtx context.Context, interval time.Duration) bool {
				polls++
				if interval != CleanupPollInterval {
					t.Fatalf("poll interval=%s", interval)
				}
				if test.deadline && waitCtx.Err() != context.DeadlineExceeded {
					t.Fatalf("wait context error=%v", waitCtx.Err())
				}
				return realDisposalPollWait(waitCtx, interval)
			}
			got := waitDisposedAcceleratorResources(ctx, fakeSnapshot{identity: "identity", namespace: namespace, client: client}, &helm.AcceleratorOwnershipReceipt{}, podIdentity, pollWait)
			if got != test.want || polls != test.wantPolls || podGETs != test.wantPodGET {
				t.Fatalf("status=%s polls=%d Pod GETs=%d", got, polls, podGETs)
			}
		})
	}
}

func TestMapHelmSuccessHasNoUnavailableReason(t *testing.T) {
	if reason := mapHelmFailure(helm.AcceleratorOK); reason != "" {
		t.Fatalf("successful install mapped to %q", reason)
	}
}

type prepareMutationTripwire struct{ calls []string }

func (f *prepareMutationTripwire) InstallAcceleratorRelease(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease) (*helm.AcceleratorOwnershipReceipt, helm.AcceleratorFailure, bool) {
	f.calls = append(f.calls, "install")
	return nil, helm.AcceleratorInstall, false
}
func (f *prepareMutationTripwire) InspectAcceleratorOwnership(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease, *helm.AcceleratorOwnershipReceipt) bool {
	f.calls = append(f.calls, "inspect")
	return false
}
func (f *prepareMutationTripwire) DeleteOwnedAcceleratorResources(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease, *helm.AcceleratorOwnershipReceipt) helm.AcceleratorFailure {
	f.calls = append(f.calls, "delete")
	return helm.AcceleratorCleanup
}
func (f *prepareMutationTripwire) PurgeOwnedAcceleratorRelease(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease, *helm.AcceleratorOwnershipReceipt) helm.AcceleratorFailure {
	f.calls = append(f.calls, "purge")
	return helm.AcceleratorCleanup
}

func TestEnsureDefaultAcceleratorNamespaceCreatesMissingNamespace(t *testing.T) {
	client := fake.NewSimpleClientset()
	snapshot := fakeSnapshot{identity: "identity", namespace: DefaultAcceleratorNamespace, client: client}

	if reason := ensureDefaultAcceleratorNamespace(context.Background(), snapshot); reason != "" {
		t.Fatalf("reason=%s", reason)
	}
	created, err := client.CoreV1().Namespaces().Get(context.Background(), DefaultAcceleratorNamespace, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantLabels := map[string]string{
		"app.kubernetes.io/name":       "kubikles-app",
		"app.kubernetes.io/part-of":    "kubikles",
		"app.kubernetes.io/managed-by": "kubikles",
	}
	for key, want := range wantLabels {
		if created.Labels[key] != want {
			t.Fatalf("label %s=%q want=%q", key, created.Labels[key], want)
		}
	}
}

func TestInstallEnsuresDefaultAcceleratorNamespaceBeforeHelm(t *testing.T) {
	client := fake.NewSimpleClientset()
	operations := &prepareMutationTripwire{}
	installer := &helmChartInstaller{client: operations}
	prepared := &preparedChart{implementation: &helmPreparedChart{
		prepared: &helm.AcceleratorPreparedRelease{},
		attempt:  chartAttempt{Namespace: DefaultAcceleratorNamespace},
	}}
	snapshot := fakeSnapshot{identity: "identity", namespace: DefaultAcceleratorNamespace, client: client}

	owned, reason, unproven := installer.Install(context.Background(), snapshot, prepared)
	if owned != nil || reason != InstallFailed || unproven {
		t.Fatalf("owned=%#v reason=%s unproven=%v", owned, reason, unproven)
	}
	if strings.Join(operations.calls, ",") != "install" {
		t.Fatalf("Helm calls=%v", operations.calls)
	}
	if _, err := client.CoreV1().Namespaces().Get(context.Background(), DefaultAcceleratorNamespace, metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureDefaultAcceleratorNamespaceLeavesExistingAndCustomNamespacesAlone(t *testing.T) {
	existing := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: DefaultAcceleratorNamespace}}
	client := fake.NewSimpleClientset(existing)
	if reason := ensureDefaultAcceleratorNamespace(context.Background(), fakeSnapshot{identity: "identity", namespace: DefaultAcceleratorNamespace, client: client}); reason != "" {
		t.Fatalf("existing reason=%s", reason)
	}
	if actions := client.Actions(); len(actions) != 1 || actions[0].GetVerb() != "get" {
		t.Fatalf("existing actions=%#v", actions)
	}

	customClient := fake.NewSimpleClientset()
	if reason := ensureDefaultAcceleratorNamespace(context.Background(), fakeSnapshot{identity: "identity", namespace: "team-a", client: customClient}); reason != "" {
		t.Fatalf("custom reason=%s", reason)
	}
	if len(customClient.Actions()) != 0 {
		t.Fatalf("custom namespace actions=%#v", customClient.Actions())
	}
}

func TestEnsureDefaultAcceleratorNamespaceMapsCreatePermissionFailure(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, DefaultAcceleratorNamespace, errors.New("create denied"))
	})
	snapshot := fakeSnapshot{identity: "identity", namespace: DefaultAcceleratorNamespace, client: client}

	if reason := ensureDefaultAcceleratorNamespace(context.Background(), snapshot); reason != PermissionDenied {
		t.Fatalf("reason=%s", reason)
	}
}

func TestEnsureDefaultAcceleratorNamespaceContinuesWhenClusterScopedLookupIsForbidden(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, DefaultAcceleratorNamespace, errors.New("get denied"))
	})
	client.PrependReactor("create", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, DefaultAcceleratorNamespace, errors.New("create denied"))
	})
	snapshot := fakeSnapshot{identity: "identity", namespace: DefaultAcceleratorNamespace, client: client}

	if reason := ensureDefaultAcceleratorNamespace(context.Background(), snapshot); reason != "" {
		t.Fatalf("reason=%s", reason)
	}
	if actions := client.Actions(); len(actions) != 2 || actions[0].GetVerb() != "get" || actions[1].GetVerb() != "create" {
		t.Fatalf("actions=%#v", actions)
	}
}

func TestPublicPullIntegrityFailureStopsServiceBeforeMutation(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	contexts := &fakeContexts{current: "ctx", snapshot: fakeSnapshot{identity: "identity", namespace: "default", client: clientset}}
	mutations := &prepareMutationTripwire{}
	pullCalls := 0
	installer := &helmChartInstaller{
		client: mutations,
		pullChart: func(_ context.Context, request helm.AcceleratorChartRequest) (*chart.Chart, error) {
			pullCalls++
			digest := "sha256:" + strings.Repeat("b", 64)
			if request.Reference != chartRepository+"@"+digest || request.Digest != digest || request.BuildVersion != "v1.2.3" {
				t.Fatalf("unexpected public pull request: %#v", request)
			}
			return nil, helm.ErrAcceleratorIntegrity
		},
	}
	observer := &fakeObserver{}
	service := New(contexts, installer, observer)
	service.entropy = bytes.NewReader(vectorEntropy())
	result := service.Provision(context.Background(), validRequest())
	if result.Availability != Unavailable || result.Reason != ChartIntegrityFailed || result.Cleanup != CleanupNotNeeded || result.Workload != nil {
		t.Fatalf("result=%#v", result)
	}
	if pullCalls != 1 || len(mutations.calls) != 0 || observer.calls.Load() != 0 || len(clientset.Actions()) != 0 {
		t.Fatalf("pulls=%d Helm mutations=%v observer=%d Kubernetes actions=%#v", pullCalls, mutations.calls, observer.calls.Load(), clientset.Actions())
	}
}

type cleanupOrderHelm struct {
	calls         []string
	storageExists bool
	resourcesGone bool
	purgeFailure  helm.AcceleratorFailure
}

func (f *cleanupOrderHelm) InstallAcceleratorRelease(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease) (*helm.AcceleratorOwnershipReceipt, helm.AcceleratorFailure, bool) {
	return nil, helm.AcceleratorInstall, false
}
func (f *cleanupOrderHelm) InspectAcceleratorOwnership(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease, *helm.AcceleratorOwnershipReceipt) bool {
	f.calls = append(f.calls, "inspect")
	return f.storageExists
}
func (f *cleanupOrderHelm) DeleteOwnedAcceleratorResources(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease, *helm.AcceleratorOwnershipReceipt) helm.AcceleratorFailure {
	f.calls = append(f.calls, "delete")
	if !f.storageExists {
		return helm.AcceleratorCleanup
	}
	f.resourcesGone = true
	return helm.AcceleratorOK
}
func (f *cleanupOrderHelm) PurgeOwnedAcceleratorRelease(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease, *helm.AcceleratorOwnershipReceipt) helm.AcceleratorFailure {
	f.calls = append(f.calls, "purge")
	if !f.storageExists || !f.resourcesGone || f.purgeFailure != helm.AcceleratorOK {
		return helm.AcceleratorCleanup
	}
	f.storageExists = false
	return helm.AcceleratorOK
}

func TestCleanupRetainsStorageUntilDeletionProofThenPurges(t *testing.T) {
	prepared, request := adapterPreparedRelease(t)
	attempt := chartAttempt{ReleaseName: request.ReleaseName, Namespace: request.ReleaseNamespace, Session: request.WorkloadSession}
	wrappedPrepared := &preparedChart{jobName: prepared.JobName(), implementation: &helmPreparedChart{prepared: prepared, attempt: attempt}}
	owned := &ownedRelease{implementation: &helmOwnedRelease{receipt: &helm.AcceleratorOwnershipReceipt{}}}
	ops := &cleanupOrderHelm{storageExists: true}
	installer := &helmChartInstaller{client: ops}
	snapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: fake.NewSimpleClientset()}
	if got := installer.Cleanup(context.Background(), snapshot, wrappedPrepared, owned); got != CleanupSucceeded {
		t.Fatalf("cleanup=%s calls=%v", got, ops.calls)
	}
	if strings.Join(ops.calls, ",") != "inspect,delete,purge" || ops.storageExists {
		t.Fatalf("storage purge ordering=%v retained=%v", ops.calls, ops.storageExists)
	}

	controller := true
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "unproven", Namespace: request.ReleaseNamespace, Labels: map[string]string{"kubikles.io/workload-session-id": request.WorkloadSession}, OwnerReferences: []metav1.OwnerReference{{Kind: "Job", UID: "unknown", Controller: &controller}}}}
	ops = &cleanupOrderHelm{storageExists: true}
	installer.client = ops
	snapshot.client = fake.NewSimpleClientset(pod)
	if got := installer.Cleanup(context.Background(), snapshot, wrappedPrepared, owned); got != CleanupFailed || !ops.storageExists || strings.Join(ops.calls, ",") != "inspect" {
		t.Fatalf("unsafe no-Job cleanup=%s calls=%v retained=%v", got, ops.calls, ops.storageExists)
	}

	owned = &ownedRelease{implementation: &helmOwnedRelease{receipt: &helm.AcceleratorOwnershipReceipt{}, unresolved: true}}
	ops = &cleanupOrderHelm{storageExists: true}
	installer.client = ops
	snapshot.client = fake.NewSimpleClientset()
	if got := installer.Cleanup(context.Background(), snapshot, wrappedPrepared, owned); got != CleanupOwnershipUnproven || !ops.storageExists || strings.Join(ops.calls, ",") != "inspect,delete" {
		t.Fatalf("ambiguous Create cleanup=%s calls=%v retained=%v", got, ops.calls, ops.storageExists)
	}
}

func TestCleanupSuspendsExactReceiptJobBeforeDeletion(t *testing.T) {
	_, request := adapterPreparedRelease(t)
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: request.ReleaseNamespace, UID: "job-uid", ResourceVersion: "7"}}
	client := fake.NewSimpleClientset(job)
	watcher := watch.NewRaceFreeFake()
	watchStarted := make(chan struct{})
	client.PrependWatchReactor("jobs", func(k8stesting.Action) (bool, watch.Interface, error) {
		close(watchStarted)
		return true, watcher, nil
	})
	go func() {
		<-watchStarted
		acknowledged := job.DeepCopy()
		suspended := true
		acknowledged.Spec.Suspend = &suspended
		acknowledged.ResourceVersion = "9"
		acknowledged.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobSuspended, Status: corev1.ConditionTrue}}
		watcher.Modify(acknowledged)
	}()
	snapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: client}
	captured := capturedAcceleratorResource{identity: helm.AcceleratorResourceIdentity{APIVersion: "batch/v1", Kind: "Job", Namespace: request.ReleaseNamespace, Name: job.Name}, uid: job.UID}
	if got := suspendAcceleratorJob(context.Background(), snapshot, captured); got != CleanupSucceeded {
		t.Fatalf("suspend status=%s", got)
	}
	updated, err := client.BatchV1().Jobs(request.ReleaseNamespace).Get(context.Background(), job.Name, metav1.GetOptions{})
	if err != nil || updated.Spec.Suspend == nil || !*updated.Spec.Suspend || updated.UID != job.UID {
		t.Fatalf("job was not UID-bound suspended: %#v err=%v", updated, err)
	}

	t.Run("late UID-owned Pod must still disappear", func(t *testing.T) {
		controller := true
		late := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "late-after-suspend", Namespace: request.ReleaseNamespace, UID: "late-after-suspend",
			Labels:          map[string]string{"kubikles.io/workload-session-id": request.WorkloadSession},
			OwnerReferences: []metav1.OwnerReference{{Kind: "Job", UID: job.UID, Controller: &controller}},
		}}
		if _, err := client.CoreV1().Pods(request.ReleaseNamespace).Create(context.Background(), late, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		barrier, ok := establishAcceleratorPodBarrier(context.Background(), snapshot, job.UID, request.WorkloadSession)
		if !ok {
			t.Fatal("post-suspend Pod barrier")
		}
		defer barrier.stop()
		result := make(chan bool, 1)
		go func() {
			result <- waitAcceleratorResourcesGone(context.Background(), snapshot, nil, job.UID, request.WorkloadSession, barrier)
		}()
		select {
		case <-result:
			t.Fatal("returned while late UID-owned Pod remained")
		case <-time.After(30 * time.Millisecond):
		}
		if err := client.CoreV1().Pods(request.ReleaseNamespace).Delete(context.Background(), late.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatal(err)
		}
		if ok := <-result; !ok {
			t.Fatal("late Pod disappearance failed")
		}
	})

	t.Run("same-name replacement blocks every delete", func(t *testing.T) {
		replacement := job.DeepCopy()
		replacement.UID = "replacement-uid"
		replacement.ResourceVersion = "10"
		replacementClient := fake.NewSimpleClientset(replacement)
		replacementSnapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: replacementClient}
		if got := suspendAcceleratorJob(context.Background(), replacementSnapshot, captured); got != CleanupOwnershipUnproven {
			t.Fatalf("replacement suspend status=%s", got)
		}
		for _, action := range replacementClient.Actions() {
			if action.GetVerb() == "update" || action.GetVerb() == "delete" {
				t.Fatalf("replacement was mutated: %#v", replacementClient.Actions())
			}
		}
	})

	t.Run("preexisting suspended state fails closed without update", func(t *testing.T) {
		already := job.DeepCopy()
		suspended := true
		already.Spec.Suspend = &suspended
		already.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobSuspended, Status: corev1.ConditionTrue}}
		alreadyClient := fake.NewSimpleClientset(already)
		alreadySnapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: alreadyClient}
		if got := suspendAcceleratorJob(context.Background(), alreadySnapshot, captured); got != CleanupFailed {
			t.Fatalf("preexisting suspended status=%s", got)
		}
		for _, action := range alreadyClient.Actions() {
			if action.GetVerb() == "update" || action.GetVerb() == "delete" {
				t.Fatalf("preexisting suspended Job was mutated: %#v", alreadyClient.Actions())
			}
		}
	})

	t.Run("missing controller acknowledgement is bounded failure", func(t *testing.T) {
		pending := job.DeepCopy()
		pending.ResourceVersion = "11"
		pendingClient := fake.NewSimpleClientset(pending)
		pendingClient.PrependWatchReactor("jobs", func(k8stesting.Action) (bool, watch.Interface, error) {
			return true, watch.NewRaceFreeFake(), nil
		})
		pendingSnapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: pendingClient}
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
		defer cancel()
		if got := suspendAcceleratorJob(ctx, pendingSnapshot, captured); got != CleanupFailed {
			t.Fatalf("unacknowledged suspend status=%s", got)
		}
		for _, action := range pendingClient.Actions() {
			if action.GetVerb() == "delete" {
				t.Fatalf("unacknowledged Job was deleted: %#v", pendingClient.Actions())
			}
		}
	})

	t.Run("patch conflict deletes nothing", func(t *testing.T) {
		conflictClient := fake.NewSimpleClientset(job.DeepCopy())
		conflictClient.PrependReactor("update", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "batch", Resource: "jobs"}, job.Name, errors.New("conflict"))
		})
		conflictSnapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: conflictClient}
		if got := suspendAcceleratorJob(context.Background(), conflictSnapshot, captured); got != CleanupFailed {
			t.Fatalf("conflict suspend status=%s", got)
		}
		for _, action := range conflictClient.Actions() {
			if action.GetVerb() == "delete" {
				t.Fatalf("conflicted Job was deleted: %#v", conflictClient.Actions())
			}
		}
	})

	t.Run("watch failure deletes nothing", func(t *testing.T) {
		watchClient := fake.NewSimpleClientset(job.DeepCopy())
		watchClient.PrependWatchReactor("jobs", func(k8stesting.Action) (bool, watch.Interface, error) {
			return true, nil, errors.New("watch failed")
		})
		watchSnapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: watchClient}
		if got := suspendAcceleratorJob(context.Background(), watchSnapshot, captured); got != CleanupFailed {
			t.Fatalf("watch failure status=%s", got)
		}
		for _, action := range watchClient.Actions() {
			if action.GetVerb() == "delete" {
				t.Fatalf("watch-failed Job was deleted: %#v", watchClient.Actions())
			}
		}
	})
}

func TestCleanupRejectsStaleSuspendedConditionWithoutMutation(t *testing.T) {
	prepared, request := adapterPreparedRelease(t)
	attempt := chartAttempt{ReleaseName: request.ReleaseName, Namespace: request.ReleaseNamespace, Session: request.WorkloadSession}
	wrappedPrepared := &preparedChart{jobName: prepared.JobName(), implementation: &helmPreparedChart{prepared: prepared, attempt: attempt}}
	receipt := &helm.AcceleratorOwnershipReceipt{}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: prepared.JobName(), Namespace: request.ReleaseNamespace, UID: "job-uid", ResourceVersion: "21"},
		Status:     batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobSuspended, Status: corev1.ConditionTrue}}},
	}
	client := fake.NewSimpleClientset(job)
	snapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: client}
	captured := []capturedAcceleratorResource{{identity: helm.AcceleratorResourceIdentity{APIVersion: "batch/v1", Kind: "Job", Namespace: request.ReleaseNamespace, Name: job.Name}, uid: job.UID}}
	ops := &cleanupOrderHelm{storageExists: true}
	installer := &helmChartInstaller{client: ops}
	got := installer.cleanupVerifiedAcceleratorResources(context.Background(), snapshot, wrappedPrepared, receipt, captured, job.UID, false)
	if got != CleanupFailed || !ops.storageExists || len(ops.calls) != 0 {
		t.Fatalf("stale condition cleanup=%s calls=%v retained=%v", got, ops.calls, ops.storageExists)
	}
	for _, action := range client.Actions() {
		if action.GetVerb() == "update" || action.GetVerb() == "delete" {
			t.Fatalf("stale condition caused mutation: %#v", client.Actions())
		}
	}
}

func TestCleanupWatchClosureUsesBoundedBackoffBeforeRelist(t *testing.T) {
	_, request := adapterPreparedRelease(t)
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: request.ReleaseNamespace, UID: "job-uid"}}
	client := fake.NewSimpleClientset(job)
	watchCalls := 0
	client.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		watchCalls++
		watcher := watch.NewRaceFreeFake()
		if watchCalls == 1 {
			watcher.Stop()
		}
		return true, watcher, nil
	})
	snapshot := fakeSnapshot{identity: "identity", namespace: request.ReleaseNamespace, client: client}
	barrier, ok := establishAcceleratorPodBarrier(context.Background(), snapshot, "job-uid", request.WorkloadSession)
	if !ok {
		t.Fatal("barrier")
	}
	defer barrier.stop()
	go func() {
		time.Sleep(5 * time.Millisecond)
		_ = client.BatchV1().Jobs(request.ReleaseNamespace).Delete(context.Background(), job.Name, metav1.DeleteOptions{})
	}()
	started := time.Now()
	captured := []capturedAcceleratorResource{{identity: helm.AcceleratorResourceIdentity{APIVersion: "batch/v1", Kind: "Job", Namespace: request.ReleaseNamespace, Name: job.Name}, uid: job.UID}}
	if !waitAcceleratorResourcesGone(context.Background(), snapshot, captured, job.UID, request.WorkloadSession, barrier) {
		t.Fatal("wait failed")
	}
	if elapsed := time.Since(started); elapsed < 20*time.Millisecond || watchCalls < 2 {
		t.Fatalf("clean closure hot-looped: elapsed=%s watchCalls=%d", elapsed, watchCalls)
	}
}

func adapterPreparedRelease(t *testing.T) (*helm.AcceleratorPreparedRelease, helm.AcceleratorReleaseRequest) {
	t.Helper()
	loaded, err := loader.Load(filepath.Join("..", "..", "deploy", "charts", "kubikles-accelerator"))
	if err != nil {
		t.Fatal(err)
	}
	loaded.Metadata.Version, loaded.Metadata.AppVersion = "1.2.3", "v1.2.3"
	chartDigest := "sha256:" + strings.Repeat("b", 64)
	request := helm.AcceleratorReleaseRequest{
		InstallationID: "101112131415161718191a1b1c1d1e1f",
		ChartReference: "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator@" + chartDigest, ChartDigest: chartDigest,
		BuildVersion: "v1.2.3", ImageRepository: imageRepository, ImageDigest: "sha256:" + strings.Repeat("a", 64),
		WorkloadSession: "202122232425262728292a2b2c2d2e2f", CreatorVerifier: "w2gLrXNNILGDLmRDyzm2sAmvRsdu_fbQpzmryPK-hlM",
		ReleaseName: "kubikles-accelerator-202122232425262728292a2b2c2d2e2f", ReleaseNamespace: "default",
	}
	prepared, failure := helm.RenderAcceleratorRelease(loaded, request)
	if failure != helm.AcceleratorOK {
		t.Fatal(failure)
	}
	return prepared, request
}

func acceleratorLiveObjects(identities []helm.AcceleratorResourceIdentity, request helm.AcceleratorReleaseRequest, uidPrefix string) []runtime.Object {
	objects := make([]runtime.Object, 0, len(identities))
	metadata := func(identity helm.AcceleratorResourceIdentity) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: identity.Name, Namespace: identity.Namespace, UID: types.UID(uidPrefix + strings.ToLower(identity.Kind)),
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "Helm", "app.kubernetes.io/instance": request.ReleaseName},
			Annotations: map[string]string{"meta.helm.sh/release-name": request.ReleaseName, "meta.helm.sh/release-namespace": request.ReleaseNamespace}}
	}
	for _, identity := range identities {
		meta := metadata(identity)
		switch identity.Kind {
		case "Job":
			meta.UID = "old-job"
			if uidPrefix != "old-" {
				meta.UID = types.UID(uidPrefix + "job")
			}
			meta.Labels["kubikles.io/workload-session-id"] = request.WorkloadSession
			objects = append(objects, &batchv1.Job{ObjectMeta: meta})
		case "ServiceAccount":
			objects = append(objects, &corev1.ServiceAccount{ObjectMeta: meta})
		case "Secret":
			objects = append(objects, &corev1.Secret{ObjectMeta: meta})
		case "ClusterRole":
			objects = append(objects, &rbacv1.ClusterRole{ObjectMeta: meta})
		case "ClusterRoleBinding":
			objects = append(objects, &rbacv1.ClusterRoleBinding{ObjectMeta: meta})
		}
	}
	return objects
}
