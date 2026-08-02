package acceleratorprovision

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func drainObserverReceipt(job *batchv1.Job, client *k8sfake.Clientset) *workloadReceipt {
	return &workloadReceipt{
		releaseNamespace:  job.Namespace,
		releaseName:       job.Labels["app.kubernetes.io/instance"],
		workloadSessionID: job.Labels["kubikles.io/workload-session-id"],
		job:               ObjectIdentity{Name: job.Name, UID: string(job.UID)},
		buildVersion:      job.Annotations["kubikles.io/build-version"],
		imageDigest:       "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		snapshot:          fakeSnapshot{identity: "identity", namespace: job.Namespace, client: client},
	}
}

func drainObserverJob() *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "accelerator", Namespace: "default", UID: "job-uid", ResourceVersion: "1",
			Labels:      map[string]string{"app.kubernetes.io/instance": "release", "app.kubernetes.io/managed-by": "Helm", "kubikles.io/workload-session-id": "202122232425262728292a2b2c2d2e2f"},
			Annotations: map[string]string{"kubikles.io/build-version": "v1.2.3"},
		},
		Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: imageRepository + "@sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}}},
	}
}

func TestDrainObserverClosedOutcomes(t *testing.T) {
	t.Run("complete", func(t *testing.T) {
		job := drainObserverJob()
		job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
		client := k8sfake.NewSimpleClientset(job)
		if got := observeExactDrainJob(context.Background(), drainObserverReceipt(job, client), make(chan struct{})); got != DrainComplete {
			t.Fatalf("status=%s", got)
		}
	})
	t.Run("failed", func(t *testing.T) {
		job := drainObserverJob()
		job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
		client := k8sfake.NewSimpleClientset(job)
		if got := observeExactDrainJob(context.Background(), drainObserverReceipt(job, client), make(chan struct{})); got != DrainFailed {
			t.Fatalf("status=%s", got)
		}
	})
	t.Run("not found", func(t *testing.T) {
		job := drainObserverJob()
		client := k8sfake.NewSimpleClientset()
		if got := observeExactDrainJob(context.Background(), drainObserverReceipt(job, client), make(chan struct{})); got != DrainNotFound {
			t.Fatalf("status=%s", got)
		}
	})
	t.Run("replaced", func(t *testing.T) {
		job := drainObserverJob()
		replacement := job.DeepCopy()
		replacement.UID = "replacement"
		client := k8sfake.NewSimpleClientset(replacement)
		if got := observeExactDrainJob(context.Background(), drainObserverReceipt(job, client), make(chan struct{})); got != DrainChanged {
			t.Fatalf("status=%s", got)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		job := drainObserverJob()
		client := k8sfake.NewSimpleClientset(job)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got := observeExactDrainJob(ctx, drainObserverReceipt(job, client), make(chan struct{})); got != DrainTimedOut {
			t.Fatalf("status=%s", got)
		}
	})
	t.Run("force", func(t *testing.T) {
		job := drainObserverJob()
		client := k8sfake.NewSimpleClientset(job)
		force := make(chan struct{})
		close(force)
		if got := observeExactDrainJob(context.Background(), drainObserverReceipt(job, client), force); got != DrainEscalated {
			t.Fatalf("status=%s", got)
		}
	})
}

func TestDrainObserverSimultaneousTerminalPriorityAndSettlement(t *testing.T) {
	job := drainObserverJob()
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	for _, test := range []struct {
		name   string
		ctx    context.Context
		force  <-chan struct{}
		status DrainObservationStatus
	}{
		{
			name: "force beats deadline and completion",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			force: func() <-chan struct{} {
				force := make(chan struct{})
				close(force)
				return force
			}(),
			status: DrainEscalated,
		},
		{
			name: "deadline beats completion",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			force:  make(chan struct{}),
			status: DrainTimedOut,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := k8sfake.NewSimpleClientset(job.DeepCopy())
			settlements := 0
			settled := DrainSkipped
			got := observeExactDrainJobSettled(test.ctx, drainObserverReceipt(job, client), test.force, func(status DrainObservationStatus) DrainObservationStatus {
				settlements++
				settled = status
				return status
			})
			if got != test.status || settled != test.status || settlements != 1 {
				t.Fatalf("status=%s settled=%s settlements=%d want=%s", got, settled, settlements, test.status)
			}
		})
	}
}

func TestDrainObserverWatchCompletionAndReadFailure(t *testing.T) {
	job := drainObserverJob()
	client := k8sfake.NewSimpleClientset(job)
	stream := watch.NewRaceFreeFake()
	watchStarted := make(chan struct{})
	client.PrependWatchReactor("jobs", func(k8stesting.Action) (bool, watch.Interface, error) {
		close(watchStarted)
		return true, stream, nil
	})
	result := make(chan DrainObservationStatus, 1)
	go func() {
		result <- observeExactDrainJob(context.Background(), drainObserverReceipt(job, client), make(chan struct{}))
	}()
	<-watchStarted
	complete := job.DeepCopy()
	complete.ResourceVersion = "2"
	complete.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	stream.Modify(complete)
	if got := <-result; got != DrainComplete {
		t.Fatalf("status=%s", got)
	}

	errorClient := k8sfake.NewSimpleClientset(job)
	errorClient.PrependWatchReactor("jobs", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, nil, context.Canceled
	})
	if got := observeExactDrainJob(context.Background(), drainObserverReceipt(job, errorClient), make(chan struct{})); got != DrainReadError {
		t.Fatalf("status=%s", got)
	}
}
