package acceleratorprovision

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
)

// observeExactDrainJob observes only the captured Job name and UID. It never
// lists or follows a same-name replacement.
func observeExactDrainJob(ctx context.Context, receipt *workloadReceipt, force <-chan struct{}) DrainObservationStatus {
	return observeExactDrainJobSettled(ctx, receipt, force, func(status DrainObservationStatus) DrainObservationStatus { return status })
}

// observeExactDrainJobSettled gives force priority over deadline, and deadline
// priority over a simultaneously readable watch event. Every terminal decision
// is settled under the workload lifecycle mutex before it is returned.
func observeExactDrainJobSettled(ctx context.Context, receipt *workloadReceipt, force <-chan struct{}, settle func(DrainObservationStatus) DrainObservationStatus) DrainObservationStatus {
	if settle == nil {
		settle = func(status DrainObservationStatus) DrainObservationStatus { return status }
	}
	finish := func(status DrainObservationStatus) DrainObservationStatus { return settle(status) }
	if ctx == nil || receipt == nil || receipt.snapshot == nil || receipt.snapshot.Clientset() == nil || receipt.job.Name == "" || receipt.job.UID == "" {
		return finish(DrainReadError)
	}
	jobs := receipt.snapshot.Clientset().BatchV1().Jobs(receipt.releaseNamespace)
	for {
		if status := drainObservationStopPriority(ctx, force); status != "" {
			return finish(status)
		}
		job, err := jobs.Get(ctx, receipt.job.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return finish(DrainNotFound)
		}
		if err != nil {
			if ctx.Err() != nil {
				return finish(DrainTimedOut)
			}
			return finish(DrainReadError)
		}
		if status := classifyDrainJob(job, receipt); status != "" {
			if priority := drainObservationStopPriority(ctx, force); priority != "" {
				return finish(priority)
			}
			return finish(status)
		}
		stream, err := jobs.Watch(ctx, metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("metadata.name", receipt.job.Name).String(), ResourceVersion: job.ResourceVersion, AllowWatchBookmarks: true})
		if err != nil {
			return finish(DrainReadError)
		}
		relist := false
		for !relist {
			select {
			case <-force:
				stream.Stop()
				return finish(DrainEscalated)
			case <-ctx.Done():
				stream.Stop()
				if priority := drainObservationStopPriority(ctx, force); priority != "" {
					return finish(priority)
				}
				return finish(DrainTimedOut)
			case event, open := <-stream.ResultChan():
				if priority := drainObservationStopPriority(ctx, force); priority != "" {
					stream.Stop()
					return finish(priority)
				}
				if !open {
					relist = true
					continue
				}
				if event.Type == watch.Bookmark {
					continue
				}
				if event.Type == watch.Error {
					if watchExpired(event.Object) {
						relist = true
						continue
					}
					stream.Stop()
					return finish(DrainReadError)
				}
				changed, ok := event.Object.(*batchv1.Job)
				if !ok || changed.Name != receipt.job.Name || changed.Namespace != receipt.releaseNamespace {
					continue
				}
				if event.Type == watch.Deleted {
					stream.Stop()
					return finish(DrainNotFound)
				}
				if status := classifyDrainJob(changed, receipt); status != "" {
					stream.Stop()
					return finish(status)
				}
			}
		}
		stream.Stop()
	}
}

func drainObservationStopPriority(ctx context.Context, force <-chan struct{}) DrainObservationStatus {
	select {
	case <-force:
		return DrainEscalated
	default:
	}
	if ctx != nil && ctx.Err() != nil {
		return DrainTimedOut
	}
	return ""
}

func classifyDrainJob(job *batchv1.Job, receipt *workloadReceipt) DrainObservationStatus {
	if job == nil || string(job.UID) != receipt.job.UID || job.DeletionTimestamp != nil || job.Labels["kubikles.io/workload-session-id"] != receipt.workloadSessionID || job.Labels["app.kubernetes.io/instance"] != receipt.releaseName || job.Labels["app.kubernetes.io/managed-by"] != "Helm" || job.Annotations["kubikles.io/build-version"] != receipt.buildVersion || len(job.Spec.Template.Spec.Containers) != 1 || job.Spec.Template.Spec.Containers[0].Image != acceleratorWorkloadImageRepository()+"@"+receipt.imageDigest {
		return DrainChanged
	}
	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case batchv1.JobComplete:
			return DrainComplete
		case batchv1.JobFailed:
			return DrainFailed
		}
	}
	return ""
}
