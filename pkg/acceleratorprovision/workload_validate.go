package acceleratorprovision

import (
	"context"
	"errors"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type workloadValidationResult struct {
	reason UnavailableReason
	cause  error
}

func (r workloadValidationResult) Error() string {
	return "accelerator workload validation unavailable"
}
func (r workloadValidationResult) Unwrap() error { return r.cause }

// revalidateWorkload performs direct GETs only; it never lists or discovers a
// replacement Pod.
func revalidateWorkload(ctx context.Context, w *ProvisionedWorkload, s ContextSnapshot) UnavailableReason {
	return revalidateWorkloadDetailed(ctx, w, s).reason
}

func revalidateWorkloadDetailed(ctx context.Context, w *ProvisionedWorkload, s ContextSnapshot) workloadValidationResult {
	if ctx == nil || w == nil || s == nil || s.Clientset() == nil {
		return workloadValidationResult{reason: ContextUnavailable}
	}
	if s.Identity() == "" || s.Namespace() != w.ReleaseNamespace || w.ContextName == "" || w.ReleaseNamespace == "" || w.ReleaseName == "" || w.Job.Name == "" || w.Job.UID == "" || w.Pod.Name == "" || w.Pod.UID == "" || w.WorkloadSessionID == "" || w.BuildVersion == "" || w.ImageDigest == "" || w.ChartDigest == "" {
		return workloadValidationResult{reason: ContextUnavailable}
	}
	job, err := s.Clientset().BatchV1().Jobs(w.ReleaseNamespace).Get(ctx, w.Job.Name, metav1.GetOptions{})
	if err != nil {
		return workloadValidationResult{reason: JobFailed, cause: err}
	}
	pod, err := s.Clientset().CoreV1().Pods(w.ReleaseNamespace).Get(ctx, w.Pod.Name, metav1.GetOptions{})
	if err != nil {
		return workloadValidationResult{reason: PodFailed, cause: err}
	}
	if !validExactJob(job, w) || !validExactPod(pod, w) {
		return workloadValidationResult{reason: ContextChanged}
	}
	return workloadValidationResult{}
}

func (r workloadValidationResult) Is(target error) bool { return errors.Is(r.cause, target) }
func validExactJob(j *batchv1.Job, w *ProvisionedWorkload) bool {
	if j == nil || j.Name != w.Job.Name || j.Namespace != w.ReleaseNamespace || string(j.UID) != w.Job.UID || j.DeletionTimestamp != nil ||
		j.Annotations["kubikles.io/build-version"] != w.BuildVersion || j.Labels["kubikles.io/workload-session-id"] != w.WorkloadSessionID ||
		j.Labels["app.kubernetes.io/instance"] != w.ReleaseName || j.Labels["app.kubernetes.io/managed-by"] != "Helm" ||
		j.Spec.Template.Annotations["kubikles.io/build-version"] != w.BuildVersion || j.Spec.Template.Labels["kubikles.io/workload-session-id"] != w.WorkloadSessionID ||
		len(j.Spec.Template.Spec.InitContainers) != 0 || len(j.Spec.Template.Spec.EphemeralContainers) != 0 || len(j.Spec.Template.Spec.Containers) != 1 || j.Spec.Template.Spec.Containers[0].Image != imageRepository+"@"+w.ImageDigest ||
		j.Status.CompletionTime != nil || j.Status.Failed != 0 || j.Status.Succeeded != 0 {
		return false
	}
	for _, condition := range j.Status.Conditions {
		if condition.Status == corev1.ConditionTrue && (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) {
			return false
		}
	}
	return true
}
func validExactPod(p *corev1.Pod, w *ProvisionedWorkload) bool {
	if p == nil || p.Name != w.Pod.Name || p.Namespace != w.ReleaseNamespace || string(p.UID) != w.Pod.UID || p.DeletionTimestamp != nil ||
		p.Labels["kubikles.io/workload-session-id"] != w.WorkloadSessionID || p.Labels["app.kubernetes.io/instance"] != w.ReleaseName || p.Labels["app.kubernetes.io/managed-by"] != "Helm" ||
		p.Annotations["kubikles.io/build-version"] != w.BuildVersion || p.Status.Phase != corev1.PodRunning || len(p.Spec.InitContainers) != 0 || len(p.Spec.EphemeralContainers) != 0 || len(p.Status.InitContainerStatuses) != 0 || len(p.Status.EphemeralContainerStatuses) != 0 || len(p.Spec.Containers) != 1 || len(p.Status.ContainerStatuses) != 1 {
		return false
	}
	controllers := 0
	owned := false
	for _, o := range p.OwnerReferences {
		if o.Controller != nil && *o.Controller {
			controllers++
			owned = o.Kind == "Job" && o.Name == w.Job.Name && string(o.UID) == w.Job.UID
		}
	}
	st := p.Status.ContainerStatuses[0]
	return controllers == 1 && owned && st.Name == p.Spec.Containers[0].Name && st.State.Running != nil && st.Ready && p.Spec.Containers[0].Image == imageRepository+"@"+w.ImageDigest
}
