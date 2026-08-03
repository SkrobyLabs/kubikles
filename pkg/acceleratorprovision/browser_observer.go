package acceleratorprovision

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const browserObservationReadTimeout = 5 * time.Second

// observeExactBrowserTerminal has no desktop lifetime deadline. Each exact
// read is bounded and ambiguous/replacement state can only retry or be
// escalated by the existing one-shot force channel.
func observeExactBrowserTerminal(ctx context.Context, receipt *workloadReceipt, force <-chan struct{}) DrainObservationStatus {
	return observeExactBrowserTerminalWithHooks(ctx, receipt, force, nil, nil)
}

func observeExactBrowserTerminalWithHooks(ctx context.Context, receipt *workloadReceipt, force <-chan struct{}, exactJob, ambiguity func()) DrainObservationStatus {
	if ctx == nil || receipt == nil || receipt.snapshot == nil || receipt.snapshot.Clientset() == nil || receipt.job.Name == "" || receipt.job.UID == "" || receipt.pod.Name == "" || receipt.pod.UID == "" {
		return DrainReadError
	}
	seenExactJob, identityAmbiguous := false, false
	latchAmbiguity := func() {
		if identityAmbiguous {
			return
		}
		identityAmbiguous = true
		if ambiguity != nil {
			ambiguity()
		}
	}
	for {
		select {
		case <-force:
			return DrainEscalated
		default:
		}
		if !identityAmbiguous {
			readCtx, cancel := context.WithTimeout(ctx, browserObservationReadTimeout)
			job, err := receipt.snapshot.Clientset().BatchV1().Jobs(receipt.releaseNamespace).Get(readCtx, receipt.job.Name, metav1.GetOptions{})
			cancel()
			if err == nil {
				status := classifyDrainJob(job, receipt)
				switch status {
				case DrainComplete, DrainFailed:
					return status
				case DrainChanged:
					latchAmbiguity()
				default:
					if !seenExactJob {
						seenExactJob = true
						if exactJob != nil {
							exactJob()
						}
					}
				}
			} else if apierrors.IsNotFound(err) && seenExactJob {
				settled, ambiguous := browserPodSettled(ctx, receipt)
				if ambiguous {
					latchAmbiguity()
				} else if settled {
					return DrainNotFound
				}
			}
		}
		select {
		case <-force:
			return DrainEscalated
		case <-ctx.Done():
			return DrainEscalated
		case <-time.After(CleanupPollInterval):
		}
	}
}

func browserPodSettled(ctx context.Context, receipt *workloadReceipt) (settled, ambiguous bool) {
	readCtx, cancel := context.WithTimeout(ctx, browserObservationReadTimeout)
	pod, err := receipt.snapshot.Clientset().CoreV1().Pods(receipt.releaseNamespace).Get(readCtx, receipt.pod.Name, metav1.GetOptions{})
	cancel()
	capturedPresent, capturedTerminal := false, false
	if err == nil {
		if !exactBrowserPod(pod, receipt) {
			return false, true
		}
		capturedPresent = true
		capturedTerminal = pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
	} else if !apierrors.IsNotFound(err) {
		return false, false
	}
	listCtx, cancelList := context.WithTimeout(ctx, browserObservationReadTimeout)
	selector := labels.Set{"kubikles.io/workload-session-id": receipt.workloadSessionID}.AsSelector().String()
	pods, listErr := receipt.snapshot.Clientset().CoreV1().Pods(receipt.releaseNamespace).List(listCtx, metav1.ListOptions{LabelSelector: selector})
	cancelList()
	if listErr != nil {
		return false, false
	}
	for index := range pods.Items {
		candidate := &pods.Items[index]
		if !exactBrowserPod(candidate, receipt) {
			return false, true
		}
		capturedPresent = true
		capturedTerminal = candidate.Status.Phase == corev1.PodSucceeded || candidate.Status.Phase == corev1.PodFailed
	}
	return !capturedPresent || capturedTerminal, false
}

func exactBrowserPod(pod *corev1.Pod, receipt *workloadReceipt) bool {
	if pod == nil || string(pod.UID) != receipt.pod.UID || pod.Name != receipt.pod.Name || pod.Namespace != receipt.releaseNamespace || pod.Labels["kubikles.io/workload-session-id"] != receipt.workloadSessionID || pod.Labels["app.kubernetes.io/instance"] != receipt.releaseName || pod.Labels["app.kubernetes.io/managed-by"] != "Helm" || pod.Annotations["kubikles.io/build-version"] != receipt.buildVersion || len(pod.Spec.Containers) != 1 || pod.Spec.Containers[0].Image != imageRepository+"@"+receipt.imageDigest {
		return false
	}
	controllers := 0
	for _, owner := range pod.OwnerReferences {
		if owner.Controller != nil && *owner.Controller {
			controllers++
			if owner.Kind != "Job" || owner.Name != receipt.job.Name || string(owner.UID) != receipt.job.UID {
				return false
			}
		}
	}
	return controllers == 1
}
