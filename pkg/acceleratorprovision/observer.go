package acceleratorprovision

import (
	"context"
	"fmt"
	"strings"
	"time"

	"kubikles/pkg/debug"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

type WorkloadAPI interface {
	ListJobs(context.Context, string, string) (*batchv1.JobList, error)
	ListPods(context.Context, string, string) (*corev1.PodList, error)
	WatchJob(context.Context, string, string, string) (watch.Interface, error)
	WatchPods(context.Context, string, string, string) (watch.Interface, error)
}

type snapshotWorkloadAPI struct {
	snapshot ContextSnapshot
}

func (a snapshotWorkloadAPI) ListJobs(ctx context.Context, namespace, name string) (*batchv1.JobList, error) {
	return a.snapshot.Clientset().BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("metadata.name", name).String()})
}
func (a snapshotWorkloadAPI) ListPods(ctx context.Context, namespace, selector string) (*corev1.PodList, error) {
	return a.snapshot.Clientset().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
}
func (a snapshotWorkloadAPI) WatchJob(ctx context.Context, namespace, name, resourceVersion string) (watch.Interface, error) {
	return a.snapshot.Clientset().BatchV1().Jobs(namespace).Watch(ctx, metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("metadata.name", name).String(), ResourceVersion: resourceVersion, AllowWatchBookmarks: true})
}
func (a snapshotWorkloadAPI) WatchPods(ctx context.Context, namespace, selector, resourceVersion string) (watch.Interface, error) {
	return a.snapshot.Clientset().CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{LabelSelector: selector, ResourceVersion: resourceVersion, AllowWatchBookmarks: true})
}

// KubernetesObserver uses initial exact reads followed by resourceVersion-aware
// watches. Session labels narrow Pod reads; the Job controller UID remains the
// ownership proof.
type KubernetesObserver struct {
	API WorkloadAPI
}

const observerRelistBackoff = 25 * time.Millisecond

func (o KubernetesObserver) Observe(ctx context.Context, attempt chartAttempt, snapshot ContextSnapshot, boundary func() UnavailableReason) (ObjectIdentity, ObjectIdentity, UnavailableReason) {
	api := o.API
	if api == nil {
		if snapshot == nil || snapshot.Clientset() == nil {
			return ObjectIdentity{}, ObjectIdentity{}, PermissionDenied
		}
		api = snapshotWorkloadAPI{snapshot: snapshot}
	}
	selector := "kubikles.io/workload-session-id=" + attempt.Session
	relisting := false
	for {
		if relisting {
			timer := time.NewTimer(observerRelistBackoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ObjectIdentity{}, ObjectIdentity{}, contextReason(ctx)
			case <-timer.C:
			}
			if reason := boundary(); reason != "" {
				return ObjectIdentity{}, ObjectIdentity{}, reason
			}
		}
		if reason := boundary(); reason != "" {
			return ObjectIdentity{}, ObjectIdentity{}, reason
		}
		jobs, err := api.ListJobs(ctx, attempt.Namespace, attempt.JobName)
		if err != nil {
			return ObjectIdentity{}, ObjectIdentity{}, observerErrorReason(ctx, err, JobFailed)
		}
		if jobs == nil || len(jobs.Items) > 1 {
			return ObjectIdentity{}, ObjectIdentity{}, JobFailed
		}
		var job *batchv1.Job
		if len(jobs.Items) == 1 {
			job = jobs.Items[0].DeepCopy()
		}
		pods, err := api.ListPods(ctx, attempt.Namespace, selector)
		if err != nil {
			return ObjectIdentity{}, ObjectIdentity{}, observerErrorReason(ctx, err, PodFailed)
		}
		podMap := make(map[string]*corev1.Pod, len(pods.Items))
		for i := range pods.Items {
			pod := pods.Items[i].DeepCopy()
			podMap[podWatchKey(pod)] = pod
		}
		if jobID, podID, reason, done := evaluateWorkload(job, podMap, attempt); done {
			if reason == "" {
				reason = boundary()
			}
			return jobID, podID, reason
		}
		// Watch from the exact collection list RV. This closes the lost-create
		// window between observing absence and starting the field-selected watch.
		jobWatch, err := api.WatchJob(ctx, attempt.Namespace, attempt.JobName, jobs.ResourceVersion)
		if err != nil {
			return ObjectIdentity{}, ObjectIdentity{}, observerErrorReason(ctx, err, JobFailed)
		}
		podWatch, err := api.WatchPods(ctx, attempt.Namespace, selector, pods.ResourceVersion)
		if err != nil {
			jobWatch.Stop()
			return ObjectIdentity{}, ObjectIdentity{}, observerErrorReason(ctx, err, PodFailed)
		}
		relist, jobID, podID, reason := observeWatchLoop(ctx, boundary, attempt, job, podMap, jobWatch, podWatch)
		jobWatch.Stop()
		podWatch.Stop()
		if !relist {
			return jobID, podID, reason
		}
		relisting = true
	}
}

func observeWatchLoop(ctx context.Context, boundary func() UnavailableReason, attempt chartAttempt, job *batchv1.Job, pods map[string]*corev1.Pod, jobWatch, podWatch watch.Interface) (bool, ObjectIdentity, ObjectIdentity, UnavailableReason) {
	ticker := time.NewTicker(ContextPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false, ObjectIdentity{}, ObjectIdentity{}, contextReason(ctx)
		case <-ticker.C:
			if reason := boundary(); reason != "" {
				return false, ObjectIdentity{}, ObjectIdentity{}, reason
			}
		case event, ok := <-jobWatch.ResultChan():
			if !ok {
				return true, ObjectIdentity{}, ObjectIdentity{}, ""
			}
			if event.Type == watch.Error {
				if watchExpired(event.Object) {
					return true, ObjectIdentity{}, ObjectIdentity{}, ""
				}
				return false, ObjectIdentity{}, ObjectIdentity{}, observerErrorReason(ctx, apierrors.FromObject(event.Object), JobFailed)
			}
			changed, ok := event.Object.(*batchv1.Job)
			if !ok || changed.Name != attempt.JobName || changed.Namespace != attempt.Namespace {
				continue
			}
			if event.Type == watch.Deleted {
				return false, ObjectIdentity{}, ObjectIdentity{}, JobFailed
			}
			job = changed.DeepCopy()
		case event, ok := <-podWatch.ResultChan():
			if !ok {
				return true, ObjectIdentity{}, ObjectIdentity{}, ""
			}
			if event.Type == watch.Error {
				if watchExpired(event.Object) {
					return true, ObjectIdentity{}, ObjectIdentity{}, ""
				}
				return false, ObjectIdentity{}, ObjectIdentity{}, observerErrorReason(ctx, apierrors.FromObject(event.Object), PodFailed)
			}
			changed, ok := event.Object.(*corev1.Pod)
			if !ok || changed.Namespace != attempt.Namespace || changed.Labels["kubikles.io/workload-session-id"] != attempt.Session {
				continue
			}
			key := podWatchKey(changed)
			if event.Type == watch.Deleted {
				delete(pods, key)
			} else {
				pods[key] = changed.DeepCopy()
			}
		}
		if jobID, podID, reason, done := evaluateWorkload(job, pods, attempt); done {
			if reason == "" {
				reason = boundary()
			}
			return false, jobID, podID, reason
		}
	}
}

func podWatchKey(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}
	if pod.UID != "" {
		return string(pod.UID)
	}
	return pod.Name
}

func watchExpired(object runtime.Object) bool {
	status, ok := object.(*metav1.Status)
	return ok && (status.Reason == metav1.StatusReasonExpired || status.Code == 410)
}

func observerErrorReason(ctx context.Context, err error, fallback UnavailableReason) UnavailableReason {
	if ctx != nil && ctx.Err() != nil {
		return contextReason(ctx)
	}
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return PermissionDenied
	}
	return fallback
}

func evaluateWorkload(job *batchv1.Job, podMap map[string]*corev1.Pod, attempt chartAttempt) (ObjectIdentity, ObjectIdentity, UnavailableReason, bool) {
	if job != nil {
		if !validObservedJob(job, attempt) {
			logObservedWorkloadFailure("validate_job", JobFailed, attempt, job, nil)
			return ObjectIdentity{}, ObjectIdentity{}, JobFailed, true
		}
		if reason := jobState(job); reason != "" {
			logObservedWorkloadFailure("job_state", reason, attempt, job, nil)
			return ObjectIdentity{}, ObjectIdentity{}, reason, true
		}
	}
	pods := make([]*corev1.Pod, 0, len(podMap))
	for _, pod := range podMap {
		pods = append(pods, pod)
	}
	if len(pods) > 1 {
		logObservedWorkloadFailure("multiple_pods", PodFailed, attempt, job, nil)
		return ObjectIdentity{}, ObjectIdentity{}, PodFailed, true
	}
	if len(pods) == 0 {
		return ObjectIdentity{}, ObjectIdentity{}, "", false
	}
	pod := pods[0]
	if job == nil {
		if reason := podState(pod); reason != "" {
			return ObjectIdentity{}, ObjectIdentity{}, reason, true
		}
		return ObjectIdentity{}, ObjectIdentity{}, "", false
	}
	if !validObservedPod(pod, job, attempt) {
		logObservedWorkloadFailure("validate_pod", PodFailed, attempt, job, pod)
		return ObjectIdentity{}, ObjectIdentity{}, PodFailed, true
	}
	if reason := podState(pod); reason != "" {
		logObservedWorkloadFailure("pod_state", reason, attempt, job, pod)
		return ObjectIdentity{}, ObjectIdentity{}, reason, true
	}
	if usablePod(pod) {
		return ObjectIdentity{Name: job.Name, UID: string(job.UID)}, ObjectIdentity{Name: pod.Name, UID: string(pod.UID)}, "", true
	}
	return ObjectIdentity{}, ObjectIdentity{}, "", false
}

func validObservedJob(job *batchv1.Job, attempt chartAttempt) bool {
	expectedImage := effectiveAttemptImageReference(attempt)
	if job == nil || attempt.JobUID == "" || job.Name != attempt.JobName || job.Namespace != attempt.Namespace || string(job.UID) != attempt.JobUID || job.DeletionTimestamp != nil ||
		job.Labels["kubikles.io/workload-session-id"] != attempt.Session || job.Labels["app.kubernetes.io/instance"] != attempt.ReleaseName || job.Labels["app.kubernetes.io/managed-by"] != "Helm" ||
		job.Annotations["kubikles.io/build-version"] != attempt.BuildVersion || job.Spec.Template.Labels["kubikles.io/workload-session-id"] != attempt.Session ||
		job.Spec.Template.Annotations["kubikles.io/build-version"] != attempt.BuildVersion || len(job.Spec.Template.Spec.Containers) != 1 || expectedImage == "" {
		return false
	}
	return job.Spec.Template.Spec.Containers[0].Image == expectedImage
}

func validObservedPod(pod *corev1.Pod, job *batchv1.Job, attempt chartAttempt) bool {
	expectedImage := effectiveAttemptImageReference(attempt)
	if pod == nil || pod.Namespace != attempt.Namespace || pod.Name == "" || pod.UID == "" || pod.DeletionTimestamp != nil ||
		pod.Labels["kubikles.io/workload-session-id"] != attempt.Session || pod.Labels["app.kubernetes.io/instance"] != attempt.ReleaseName || pod.Labels["app.kubernetes.io/managed-by"] != "Helm" ||
		pod.Annotations["kubikles.io/build-version"] != attempt.BuildVersion || len(pod.Spec.Containers) != 1 || expectedImage == "" || pod.Spec.Containers[0].Image != expectedImage {
		return false
	}
	for _, owner := range pod.OwnerReferences {
		if owner.Controller != nil && *owner.Controller && owner.Kind == "Job" && owner.Name == job.Name && owner.UID == job.UID {
			return true
		}
	}
	return false
}

func logObservedWorkloadFailure(stage string, reason UnavailableReason, attempt chartAttempt, job *batchv1.Job, pod *corev1.Pod) {
	details := map[string]interface{}{
		"stage":          stage,
		"reason":         reason,
		"namespace":      attempt.Namespace,
		"release":        attempt.ReleaseName,
		"expectedJob":    attempt.JobName,
		"expectedJobUID": attempt.JobUID,
		"expectedImage":  effectiveAttemptImageReference(attempt),
	}
	if job != nil {
		details["job"] = job.Name
		details["jobUID"] = string(job.UID)
		details["jobActive"] = job.Status.Active
		details["jobFailed"] = job.Status.Failed
		details["jobSucceeded"] = job.Status.Succeeded
		details["jobConditions"] = observedJobConditions(job)
		if len(job.Spec.Template.Spec.Containers) == 1 {
			details["jobImage"] = job.Spec.Template.Spec.Containers[0].Image
		}
	}
	if pod != nil {
		details["pod"] = pod.Name
		details["podUID"] = string(pod.UID)
		details["podPhase"] = pod.Status.Phase
		if len(pod.Spec.Containers) == 1 {
			details["podImage"] = pod.Spec.Containers[0].Image
		}
		if len(pod.Status.ContainerStatuses) == 1 {
			status := pod.Status.ContainerStatuses[0]
			details["containerReady"] = status.Ready
			switch {
			case status.State.Waiting != nil:
				details["containerState"] = "waiting"
				details["containerReason"] = status.State.Waiting.Reason
			case status.State.Running != nil:
				details["containerState"] = "running"
			case status.State.Terminated != nil:
				details["containerState"] = "terminated"
				details["containerReason"] = status.State.Terminated.Reason
				details["containerExitCode"] = status.State.Terminated.ExitCode
			}
		}
	}
	debug.LogK8s("Accelerator workload observation failed", details)
}

func observedJobConditions(job *batchv1.Job) []string {
	if job == nil || len(job.Status.Conditions) == 0 {
		return nil
	}
	conditions := make([]string, 0, len(job.Status.Conditions))
	for _, condition := range job.Status.Conditions {
		conditions = append(conditions, fmt.Sprintf("%s=%s reason=%s message=%s", condition.Type, condition.Status, condition.Reason, condition.Message))
	}
	return conditions
}

func jobState(job *batchv1.Job) UnavailableReason {
	if job.DeletionTimestamp != nil {
		return JobFailed
	}
	for _, condition := range job.Status.Conditions {
		if condition.Status == corev1.ConditionTrue && (condition.Type == batchv1.JobFailed || condition.Type == batchv1.JobComplete) {
			return JobFailed
		}
	}
	return ""
}

func usablePod(pod *corev1.Pod) bool {
	if pod == nil || pod.Status.Phase != corev1.PodRunning || len(pod.Status.ContainerStatuses) != 1 {
		return false
	}
	status := pod.Status.ContainerStatuses[0]
	return status.Name == pod.Spec.Containers[0].Name && status.State.Running != nil && status.Ready
}

func podState(pod *corev1.Pod) UnavailableReason {
	if pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		return PodFailed
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Terminated != nil {
			return PodFailed
		}
		if status.State.Waiting != nil {
			switch strings.TrimSpace(status.State.Waiting.Reason) {
			case "ErrImagePull", "ImagePullBackOff", "InvalidImageName", "CreateContainerConfigError", "CreateContainerError", "RunContainerError":
				return ImagePullFailed
			}
		}
	}
	return ""
}
