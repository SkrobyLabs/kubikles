package acceleratorprovision

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"kubikles/pkg/agent"
	"kubikles/pkg/server"
)

func exactWorkloadFixture() (*ProvisionedWorkload, *batchv1.Job, *corev1.Pod) {
	workload := &ProvisionedWorkload{
		ContextName: "selected", ReleaseNamespace: "team-a", ReleaseName: "release-a", WorkloadSessionID: "workload-session",
		Job: ObjectIdentity{Name: "job-a", UID: "job-uid"}, Pod: ObjectIdentity{Name: "pod-a", UID: "pod-uid"}, BuildVersion: "v1.2.3",
		ImageDigest: "sha256:" + strings.Repeat("a", 64), ChartDigest: "sha256:" + strings.Repeat("b", 64),
	}
	labels := map[string]string{"kubikles.io/workload-session-id": workload.WorkloadSessionID, "app.kubernetes.io/instance": workload.ReleaseName, "app.kubernetes.io/managed-by": "Helm"}
	annotations := map[string]string{"kubikles.io/build-version": workload.BuildVersion}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: workload.Job.Name, Namespace: workload.ReleaseNamespace, UID: types.UID(workload.Job.UID), Labels: labels, Annotations: annotations},
		Spec:       batchv1.JobSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"kubikles.io/workload-session-id": workload.WorkloadSessionID}, Annotations: annotations}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "accelerator", Image: imageRepository + "@" + workload.ImageDigest}}}}},
	}
	controller := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: workload.Pod.Name, Namespace: workload.ReleaseNamespace, UID: types.UID(workload.Pod.UID), Labels: labels, Annotations: annotations, OwnerReferences: []metav1.OwnerReference{{Kind: "Job", Name: workload.Job.Name, UID: types.UID(workload.Job.UID), Controller: &controller}}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "accelerator", Image: imageRepository + "@" + workload.ImageDigest}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "accelerator", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}},
	}
	return workload, job, pod
}

func TestResumeUsesRealKubernetesValidationCauses(t *testing.T) {
	tests := []struct {
		name      string
		failure   error
		wantSleep bool
	}{
		{name: "timeout", failure: apierrors.NewTimeoutError("raw-timeout", 1), wantSleep: true},
		{name: "too many requests", failure: apierrors.NewTooManyRequests("raw-throttle", 1), wantSleep: true},
		{name: "internal", failure: apierrors.NewInternalError(errors.New("raw-internal")), wantSleep: true},
		{name: "service unavailable", failure: apierrors.NewServiceUnavailable("raw-unavailable"), wantSleep: true},
		{name: "gateway timeout", failure: &apierrors.StatusError{ErrStatus: metav1.Status{Code: 504}}, wantSleep: true},
		{name: "unauthorized", failure: apierrors.NewUnauthorized("raw-unauthorized")},
		{name: "forbidden", failure: apierrors.NewForbidden(schema.GroupResource{Resource: "jobs"}, "job-a", errors.New("raw-forbidden"))},
		{name: "not found", failure: apierrors.NewNotFound(schema.GroupResource{Resource: "jobs"}, "job-a")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &fakeResumeClock{now: time.Now()}
			prior, workload := resumableSessionFixture(t, clock)
			client := workload.snapshot.Clientset().(*fake.Clientset)
			var failed bool
			client.PrependReactor("get", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
				if !failed {
					failed = true
					return true, nil, test.failure
				}
				return false, nil, nil
			})
			reconnector := NewReconnector("v1.2.3")
			var tunnelStarts int
			reconnector.connector.startTunnel = func(context.Context, ContextSnapshot, string, string) (tunnel, error) {
				tunnelStarts++
				return nil, errors.New("raw terminal tunnel failure")
			}
			result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
			if test.wantSleep {
				if len(clock.sleeps) != 1 || tunnelStarts != 1 || result.Reason != ResumeTransportUnavailable {
					t.Fatalf("result=%#v sleeps=%v starts=%d", result, clock.sleeps, tunnelStarts)
				}
			} else if len(clock.sleeps) != 0 || tunnelStarts != 0 || result.Reason != ResumeWorkloadTerminalOrChanged {
				t.Fatalf("result=%#v sleeps=%v starts=%d", result, clock.sleeps, tunnelStarts)
			}
		})
	}
}

func TestResumeEarlyCloseDetailed429RevalidationRetriesEndToEnd(t *testing.T) {
	clock := &fakeResumeClock{now: time.Now()}
	prior, workload := resumableSessionFixture(t, clock)
	client := workload.snapshot.Clientset().(*fake.Clientset)
	var gets int
	client.PrependReactor("get", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
		gets++
		if gets == 1 {
			return true, nil, apierrors.NewTooManyRequests("raw-throttle", 1)
		}
		return false, nil, nil
	})
	reconnector := NewReconnector("v1.2.3")
	attempts := 0
	reconnector.attempt = func(_ context.Context, _ connectorLease, expectation connectionExpectation) (*ConnectedSession, *connectAttemptFailure) {
		attempts++
		if attempts == 1 {
			expectation.accepted(2)
			return nil, &connectAttemptFailure{phase: attemptPublication, earlyClose: true}
		}
		if expectation.generationFloor != 2 {
			t.Fatalf("generation floor=%d want=2", expectation.generationFloor)
		}
		order := []string{}
		return newConnectedSession(workload.connectorState.receipt, server.AuthenticatedAcceleratorInfo{Capabilities: agent.V1Capabilities()}, connectedIdentity{sessionID: "session-a", instanceID: "instance-a", generation: 3}, newRecordedSessionSocket(&order), newRecordedTunnel(&order), clock), nil
	}
	result := reconnector.Resume(context.Background(), ResumeRequest{Prior: prior, Workload: workload})
	if result.Session == nil || attempts != 2 || gets != 1 || !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Second}) {
		t.Fatalf("result=%#v attempts=%d gets=%d sleeps=%v", result, attempts, gets, clock.sleeps)
	}
	if err := result.Session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func validateFixture(workload *ProvisionedWorkload, objects ...runtime.Object) UnavailableReason {
	snapshot := fakeSnapshot{identity: "snapshot-id", namespace: workload.ReleaseNamespace, client: fake.NewSimpleClientset(objects...)}
	return revalidateWorkload(context.Background(), workload, snapshot)
}

func TestRevalidateProvisionedWorkloadExact(t *testing.T) {
	workload, job, pod := exactWorkloadFixture()
	if reason := validateFixture(workload, job, pod); reason != "" {
		t.Fatalf("exact workload rejected: %s", reason)
	}
	tests := []struct {
		name   string
		mutate func(*ProvisionedWorkload, *batchv1.Job, *corev1.Pod)
	}{
		{"job uid", func(_ *ProvisionedWorkload, j *batchv1.Job, _ *corev1.Pod) { j.UID = "replacement" }},
		{"job name", func(_ *ProvisionedWorkload, j *batchv1.Job, _ *corev1.Pod) { j.Name = "other" }},
		{"job session", func(_ *ProvisionedWorkload, j *batchv1.Job, _ *corev1.Pod) {
			j.Labels["kubikles.io/workload-session-id"] = "other"
		}},
		{"job release", func(_ *ProvisionedWorkload, j *batchv1.Job, _ *corev1.Pod) {
			j.Labels["app.kubernetes.io/instance"] = "other"
		}},
		{"job build", func(_ *ProvisionedWorkload, j *batchv1.Job, _ *corev1.Pod) {
			j.Annotations["kubikles.io/build-version"] = "v1.2.4"
		}},
		{"job template build", func(_ *ProvisionedWorkload, j *batchv1.Job, _ *corev1.Pod) {
			j.Spec.Template.Annotations["kubikles.io/build-version"] = "v1.2.4"
		}},
		{"job image", func(_ *ProvisionedWorkload, j *batchv1.Job, _ *corev1.Pod) {
			j.Spec.Template.Spec.Containers[0].Image = "foreign"
		}},
		{"job deleting", func(_ *ProvisionedWorkload, j *batchv1.Job, _ *corev1.Pod) {
			now := metav1.NewTime(time.Now())
			j.DeletionTimestamp = &now
		}},
		{"job complete", func(_ *ProvisionedWorkload, j *batchv1.Job, _ *corev1.Pod) {
			j.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
		}},
		{"job failed", func(_ *ProvisionedWorkload, j *batchv1.Job, _ *corev1.Pod) { j.Status.Failed = 1 }},
		{"pod uid", func(_ *ProvisionedWorkload, _ *batchv1.Job, p *corev1.Pod) { p.UID = "replacement" }},
		{"pod owner", func(_ *ProvisionedWorkload, _ *batchv1.Job, p *corev1.Pod) { p.OwnerReferences[0].UID = "other" }},
		{"pod session", func(_ *ProvisionedWorkload, _ *batchv1.Job, p *corev1.Pod) {
			p.Labels["kubikles.io/workload-session-id"] = "other"
		}},
		{"pod build", func(_ *ProvisionedWorkload, _ *batchv1.Job, p *corev1.Pod) {
			p.Annotations["kubikles.io/build-version"] = "v1.2.4"
		}},
		{"pod image", func(_ *ProvisionedWorkload, _ *batchv1.Job, p *corev1.Pod) { p.Spec.Containers[0].Image = "foreign" }},
		{"pod deleting", func(_ *ProvisionedWorkload, _ *batchv1.Job, p *corev1.Pod) {
			now := metav1.NewTime(time.Now())
			p.DeletionTimestamp = &now
		}},
		{"pod phase", func(_ *ProvisionedWorkload, _ *batchv1.Job, p *corev1.Pod) { p.Status.Phase = corev1.PodPending }},
		{"second container", func(_ *ProvisionedWorkload, _ *batchv1.Job, p *corev1.Pod) {
			p.Spec.Containers = append(p.Spec.Containers, corev1.Container{Name: "extra"})
		}},
		{"not running", func(_ *ProvisionedWorkload, _ *batchv1.Job, p *corev1.Pod) {
			p.Status.ContainerStatuses[0].State.Running = nil
		}},
		{"not ready", func(_ *ProvisionedWorkload, _ *batchv1.Job, p *corev1.Pod) {
			p.Status.ContainerStatuses[0].Ready = false
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate, candidateJob, candidatePod := exactWorkloadFixture()
			test.mutate(candidate, candidateJob, candidatePod)
			if reason := validateFixture(candidate, candidateJob, candidatePod); reason == "" {
				t.Fatal("mismatch accepted")
			}
		})
	}
}
