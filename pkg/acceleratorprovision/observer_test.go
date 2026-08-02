package acceleratorprovision

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

type scriptedWorkloadAPI struct {
	mu          sync.Mutex
	jobs        *batchv1.JobList
	jobsErr     error
	pods        *corev1.PodList
	podsErr     error
	jobWatch    *watch.RaceFreeFakeWatcher
	podWatch    *watch.RaceFreeFakeWatcher
	jobWatches  int
	podWatches  int
	jobWatchRV  string
	jobSelector string
	podWatchRV  string
	podSelector string
	jobLists    int
	podLists    int
}

func (a *scriptedWorkloadAPI) ListJobs(_ context.Context, _ string, selector string) (*batchv1.JobList, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.jobLists++
	a.jobSelector = selector
	if a.jobs == nil {
		return nil, a.jobsErr
	}
	return a.jobs.DeepCopy(), a.jobsErr
}
func (a *scriptedWorkloadAPI) ListPods(_ context.Context, _ string, selector string) (*corev1.PodList, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.podLists++
	a.podSelector = selector
	if a.pods == nil {
		return nil, a.podsErr
	}
	return a.pods.DeepCopy(), a.podsErr
}
func (a *scriptedWorkloadAPI) WatchJob(_ context.Context, _, _ string, rv string) (watch.Interface, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.jobWatches++
	a.jobWatchRV = rv
	return a.jobWatch, nil
}
func (a *scriptedWorkloadAPI) WatchPods(_ context.Context, _, _ string, rv string) (watch.Interface, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.podWatches++
	a.podWatchRV = rv
	return a.podWatch, nil
}

func TestObserveExactWorkloadUsable(t *testing.T) {
	attempt := observerAttempt()
	job := observedJob(attempt)
	pod := observedPod(attempt, job)
	api := &scriptedWorkloadAPI{jobs: &batchv1.JobList{Items: []batchv1.Job{*job}}, pods: &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "11"}, Items: []corev1.Pod{*pod}}}
	gotJob, gotPod, reason := (KubernetesObserver{API: api}).Observe(context.Background(), attempt, nil, func() UnavailableReason { return "" })
	if reason != "" || gotJob != (ObjectIdentity{Name: job.Name, UID: string(job.UID)}) || gotPod != (ObjectIdentity{Name: pod.Name, UID: string(pod.UID)}) {
		t.Fatalf("unexpected result: %#v %#v %s", gotJob, gotPod, reason)
	}
	if api.podSelector != "kubikles.io/workload-session-id="+attempt.Session {
		t.Fatal("observer used a broad Pod selector")
	}

	t.Run("resource version watches", func(t *testing.T) {
		jobWatch, podWatch := watch.NewRaceFreeFake(), watch.NewRaceFreeFake()
		watchAPI := &scriptedWorkloadAPI{
			jobs: &batchv1.JobList{ListMeta: metav1.ListMeta{ResourceVersion: "19"}},
			pods: &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "20"}}, jobWatch: jobWatch, podWatch: podWatch,
		}
		go func() {
			jobWatch.Add(observedJob(attempt))
			podWatch.Add(observedPod(attempt, observedJob(attempt)))
		}()
		gotJob, gotPod, reason := (KubernetesObserver{API: watchAPI}).Observe(context.Background(), attempt, nil, func() UnavailableReason { return "" })
		if reason != "" || gotJob.UID != "job-uid" || gotPod.UID != "pod-uid" || watchAPI.jobWatchRV != "19" || watchAPI.podWatchRV != "20" || watchAPI.jobSelector != attempt.JobName {
			t.Fatalf("watch result %#v %#v %s rv=%s/%s selector=%q", gotJob, gotPod, reason, watchAPI.jobWatchRV, watchAPI.podWatchRV, watchAPI.jobSelector)
		}
	})

	t.Run("correlation rejects", func(t *testing.T) {
		mutations := map[string]func(*batchv1.Job, *corev1.Pod){
			"wrong job uid": func(job *batchv1.Job, _ *corev1.Pod) { job.UID = "" },
			"wrong session": func(_ *batchv1.Job, pod *corev1.Pod) { pod.Labels["kubikles.io/workload-session-id"] = "other" },
			"wrong build":   func(_ *batchv1.Job, pod *corev1.Pod) { pod.Annotations["kubikles.io/build-version"] = "v9.9.9" },
			"wrong image": func(_ *batchv1.Job, pod *corev1.Pod) {
				pod.Spec.Containers[0].Image = "example.invalid/wrong@" + attempt.ImageDigest
			},
			"wrong owner": func(_ *batchv1.Job, pod *corev1.Pod) { pod.OwnerReferences[0].UID = "other" },
			"deleting":    func(_ *batchv1.Job, pod *corev1.Pod) { now := metav1.Now(); pod.DeletionTimestamp = &now },
			"two containers": func(_ *batchv1.Job, pod *corev1.Pod) {
				pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: "extra"})
			},
		}
		for name, mutate := range mutations {
			t.Run(name, func(t *testing.T) {
				changedJob, changedPod := job.DeepCopy(), pod.DeepCopy()
				mutate(changedJob, changedPod)
				_, _, gotReason := (KubernetesObserver{API: &scriptedWorkloadAPI{jobs: &batchv1.JobList{Items: []batchv1.Job{*changedJob}}, pods: &corev1.PodList{Items: []corev1.Pod{*changedPod}}}}).Observe(context.Background(), attempt, nil, func() UnavailableReason { return "" })
				if gotReason != PodFailed && gotReason != JobFailed {
					t.Fatalf("reason=%s", gotReason)
				}
			})
		}
		secondPod := pod.DeepCopy()
		secondPod.Name, secondPod.UID = "accelerator-pod-2", "pod-uid-2"
		_, _, gotReason := (KubernetesObserver{API: &scriptedWorkloadAPI{jobs: &batchv1.JobList{Items: []batchv1.Job{*job}}, pods: &corev1.PodList{Items: []corev1.Pod{*pod, *secondPod}}}}).Observe(context.Background(), attempt, nil, func() UnavailableReason { return "" })
		if gotReason != PodFailed {
			t.Fatalf("multiple pods reason=%s", gotReason)
		}
	})

	t.Run("receipt job uid rejects same-name replacement", func(t *testing.T) {
		replacementJob := job.DeepCopy()
		replacementJob.UID = "replacement-job-uid"
		replacementPod := observedPod(attempt, replacementJob)
		_, _, gotReason := (KubernetesObserver{API: &scriptedWorkloadAPI{
			jobs: &batchv1.JobList{Items: []batchv1.Job{*replacementJob}},
			pods: &corev1.PodList{Items: []corev1.Pod{*replacementPod}},
		}}).Observe(context.Background(), attempt, nil, func() UnavailableReason { return "" })
		if gotReason != JobFailed {
			t.Fatalf("same-name replacement reason=%s", gotReason)
		}
	})

	t.Run("watch rejects same-name replacement", func(t *testing.T) {
		jobWatch, podWatch := watch.NewRaceFreeFake(), watch.NewRaceFreeFake()
		watchAPI := &scriptedWorkloadAPI{
			jobs: &batchv1.JobList{ListMeta: metav1.ListMeta{ResourceVersion: "30"}},
			pods: &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "31"}}, jobWatch: jobWatch, podWatch: podWatch,
		}
		replacement := job.DeepCopy()
		replacement.UID = "replacement-job-uid"
		go jobWatch.Add(replacement)
		_, _, gotReason := (KubernetesObserver{API: watchAPI}).Observe(context.Background(), attempt, nil, func() UnavailableReason { return "" })
		if gotReason != JobFailed {
			t.Fatalf("watch replacement reason=%s", gotReason)
		}
	})
}

func TestObserveTerminalStates(t *testing.T) {
	attempt := observerAttempt()
	tests := map[string]struct {
		mutate func(*batchv1.Job, *corev1.Pod)
		want   UnavailableReason
	}{
		"job failed": {mutate: func(job *batchv1.Job, _ *corev1.Pod) {
			job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
		}, want: JobFailed},
		"job complete": {mutate: func(job *batchv1.Job, _ *corev1.Pod) {
			job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
		}, want: JobFailed},
		"pod failed":    {mutate: func(_ *batchv1.Job, pod *corev1.Pod) { pod.Status.Phase = corev1.PodFailed }, want: PodFailed},
		"pod succeeded": {mutate: func(_ *batchv1.Job, pod *corev1.Pod) { pod.Status.Phase = corev1.PodSucceeded }, want: PodFailed},
		"terminated": {mutate: func(_ *batchv1.Job, pod *corev1.Pod) {
			pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}
		}, want: PodFailed},
	}
	for _, waitingReason := range []string{"ErrImagePull", "ImagePullBackOff", "InvalidImageName", "CreateContainerConfigError", "CreateContainerError", "RunContainerError"} {
		reason := waitingReason
		tests[waitingReason] = struct {
			mutate func(*batchv1.Job, *corev1.Pod)
			want   UnavailableReason
		}{mutate: func(_ *batchv1.Job, pod *corev1.Pod) {
			pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}}
		}, want: ImagePullFailed}
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			job := observedJob(attempt)
			pod := observedPod(attempt, job)
			test.mutate(job, pod)
			_, _, got := (KubernetesObserver{API: &scriptedWorkloadAPI{jobs: &batchv1.JobList{Items: []batchv1.Job{*job}}, pods: &corev1.PodList{Items: []corev1.Pod{*pod}}}}).Observe(context.Background(), attempt, nil, func() UnavailableReason { return "" })
			if got != test.want {
				t.Fatalf("reason=%s want=%s", got, test.want)
			}
		})
	}

	for _, statusErr := range []error{
		apierrors.NewForbidden(schema.GroupResource{Group: "batch", Resource: "jobs"}, attempt.JobName, errors.New("secret status")),
		apierrors.NewUnauthorized("secret status"),
	} {
		_, _, reason := (KubernetesObserver{API: &scriptedWorkloadAPI{jobsErr: statusErr}}).Observe(context.Background(), attempt, nil, func() UnavailableReason { return "" })
		if reason != PermissionDenied {
			t.Fatalf("permission reason=%s", reason)
		}
	}

	job := observedJob(attempt)
	pod := observedPod(attempt, job)
	calls := 0
	_, _, reason := (KubernetesObserver{API: &scriptedWorkloadAPI{jobs: &batchv1.JobList{Items: []batchv1.Job{*job}}, pods: &corev1.PodList{Items: []corev1.Pod{*pod}}}}).Observe(context.Background(), attempt, nil, func() UnavailableReason {
		calls++
		if calls > 1 {
			return ContextChanged
		}
		return ""
	})
	if reason != ContextChanged {
		t.Fatalf("final context did not win: %s", reason)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, reason = (KubernetesObserver{API: &scriptedWorkloadAPI{}}).Observe(ctx, attempt, nil, func() UnavailableReason { return contextReason(ctx) })
	if reason != Cancelled {
		t.Fatalf("cancel reason=%s", reason)
	}
	deadlineCtx, deadlineCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer deadlineCancel()
	_, _, reason = (KubernetesObserver{API: &scriptedWorkloadAPI{}}).Observe(deadlineCtx, attempt, nil, func() UnavailableReason { return contextReason(deadlineCtx) })
	if reason != TimedOut {
		t.Fatalf("timeout reason=%s", reason)
	}
}

func TestObserveCleanWatchClosureBacksOffBeforeRelist(t *testing.T) {
	attempt := observerAttempt()
	jobWatch, podWatch := watch.NewRaceFreeFake(), watch.NewRaceFreeFake()
	jobWatch.Stop()
	podWatch.Stop()
	api := &scriptedWorkloadAPI{
		jobs:     &batchv1.JobList{ListMeta: metav1.ListMeta{ResourceVersion: "10"}},
		pods:     &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "11"}},
		jobWatch: jobWatch, podWatch: podWatch,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, _, reason := (KubernetesObserver{API: api}).Observe(ctx, attempt, nil, func() UnavailableReason {
		if ctx.Err() != nil {
			return contextReason(ctx)
		}
		return ""
	})
	if reason != TimedOut {
		t.Fatalf("reason=%s", reason)
	}
	api.mu.Lock()
	jobLists, podLists := api.jobLists, api.podLists
	api.mu.Unlock()
	if elapsed := time.Since(started); elapsed < 60*time.Millisecond || jobLists > 4 || podLists > 4 {
		t.Fatalf("clean watch closure hot-looped: elapsed=%s lists=%d/%d", elapsed, jobLists, podLists)
	}
}

func observerAttempt() chartAttempt {
	return chartAttempt{
		ReleaseName: "kubikles-accelerator-202122232425262728292a2b2c2d2e2f", Namespace: "default",
		Session: "202122232425262728292a2b2c2d2e2f", BuildVersion: "v1.2.3",
		ImageRepository: imageRepository, ImageDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		JobName: "kubikles-accelerator-202122232425262728292a2b2c2d2e2f-kubikles", JobUID: "job-uid",
	}
}

func observedJob(attempt chartAttempt) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: attempt.JobName, Namespace: attempt.Namespace, UID: types.UID("job-uid"), ResourceVersion: "10", Labels: map[string]string{
			"kubikles.io/workload-session-id": attempt.Session, "app.kubernetes.io/instance": attempt.ReleaseName, "app.kubernetes.io/managed-by": "Helm",
		}, Annotations: map[string]string{"kubikles.io/build-version": attempt.BuildVersion}},
		Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"kubikles.io/workload-session-id": attempt.Session}, Annotations: map[string]string{"kubikles.io/build-version": attempt.BuildVersion}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "accelerator", Image: attempt.ImageRepository + "@" + attempt.ImageDigest}}}}},
	}
}

func observedPod(attempt chartAttempt, job *batchv1.Job) *corev1.Pod {
	controller := true
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "accelerator-pod", Namespace: attempt.Namespace, UID: types.UID("pod-uid"), ResourceVersion: "12", Labels: map[string]string{
			"kubikles.io/workload-session-id": attempt.Session, "app.kubernetes.io/instance": attempt.ReleaseName, "app.kubernetes.io/managed-by": "Helm",
		}, Annotations: map[string]string{"kubikles.io/build-version": attempt.BuildVersion}, OwnerReferences: []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "Job", Name: job.Name, UID: job.UID, Controller: &controller}}},
		Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "accelerator", Image: attempt.ImageRepository + "@" + attempt.ImageDigest}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "accelerator", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()}}}}},
	}
}
