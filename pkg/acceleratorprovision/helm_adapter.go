package acceleratorprovision

import (
	"context"
	"time"

	"kubikles/pkg/helm"

	helmchart "helm.sh/helm/v3/pkg/chart"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

type helmPreparedChart struct {
	prepared *helm.AcceleratorPreparedRelease
	attempt  chartAttempt
}

type helmOwnedRelease struct {
	receipt    *helm.AcceleratorOwnershipReceipt
	jobUID     types.UID
	unresolved bool
}

type acceleratorHelmClient interface {
	InstallAcceleratorRelease(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease) (*helm.AcceleratorOwnershipReceipt, helm.AcceleratorFailure, bool)
	InspectAcceleratorOwnership(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease, *helm.AcceleratorOwnershipReceipt) bool
	DeleteOwnedAcceleratorResources(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease, *helm.AcceleratorOwnershipReceipt) helm.AcceleratorFailure
	PurgeOwnedAcceleratorRelease(context.Context, *rest.Config, *helm.AcceleratorPreparedRelease, *helm.AcceleratorOwnershipReceipt) helm.AcceleratorFailure
}

type helmChartInstaller struct {
	client    acceleratorHelmClient
	pullChart func(context.Context, helm.AcceleratorChartRequest) (*helmchart.Chart, error)
}

func (h *helmChartInstaller) Prepare(ctx context.Context, attempt chartAttempt, boundary func() UnavailableReason) (*preparedChart, UnavailableReason) {
	if h == nil || h.client == nil {
		return nil, ChartPullFailed
	}
	request := helmRequest(attempt)
	pullChart := h.pullChart
	if pullChart == nil {
		pullChart = helm.PullAcceleratorChart
	}
	loaded, err := pullChart(ctx, helm.AcceleratorChartRequest{Reference: request.ChartReference, Digest: request.ChartDigest, BuildVersion: request.BuildVersion})
	if err != nil {
		if err == helm.ErrAcceleratorIntegrity {
			return nil, ChartIntegrityFailed
		}
		return nil, ChartPullFailed
	}
	if reason := boundary(); reason != "" {
		return nil, reason
	}
	prepared, failure := helm.RenderAcceleratorRelease(loaded, request)
	if failure != helm.AcceleratorOK {
		return nil, mapHelmFailure(failure)
	}
	return &preparedChart{jobName: prepared.JobName(), implementation: &helmPreparedChart{prepared: prepared, attempt: attempt}}, ""
}

func (h *helmChartInstaller) Install(ctx context.Context, snapshot ContextSnapshot, prepared *preparedChart) (*ownedRelease, UnavailableReason, bool) {
	implementation, ok := prepared.implementation.(*helmPreparedChart)
	if !ok || implementation == nil || implementation.prepared == nil || snapshot == nil {
		return nil, InstallFailed, false
	}
	receipt, failure, ownershipUnproven := h.client.InstallAcceleratorRelease(ctx, snapshot.RESTConfig(), implementation.prepared)
	if receipt != nil {
		_, jobUID, valid := createdAcceleratorResources(receipt)
		if !valid {
			return &ownedRelease{implementation: &helmOwnedRelease{receipt: receipt, unresolved: len(receipt.UnresolvedResources()) != 0}}, InstallFailed, false
		}
		owned := &ownedRelease{implementation: &helmOwnedRelease{receipt: receipt, jobUID: jobUID, unresolved: len(receipt.UnresolvedResources()) != 0}, jobUID: string(jobUID)}
		if failure == helm.AcceleratorOK && jobUID == "" {
			return owned, InstallFailed, false
		}
		return owned, mapHelmFailure(failure), false
	}
	if failure == helm.AcceleratorConflict {
		return nil, ReleaseConflict, false
	}
	// An install error with no exact record is deliberately not ownership.
	return nil, mapHelmFailure(failure), ownershipUnproven
}

func (h *helmChartInstaller) Cleanup(ctx context.Context, snapshot ContextSnapshot, prepared *preparedChart, owned *ownedRelease) CleanupStatus {
	implementation, preparedOK := prepared.implementation.(*helmPreparedChart)
	receipt, ownedOK := owned.implementation.(*helmOwnedRelease)
	if !preparedOK || !ownedOK || implementation == nil || receipt == nil || implementation.prepared == nil || receipt.receipt == nil || snapshot == nil {
		return CleanupOwnershipUnproven
	}
	if !h.client.InspectAcceleratorOwnership(ctx, snapshot.RESTConfig(), implementation.prepared, receipt.receipt) {
		return CleanupOwnershipUnproven
	}
	captured, jobUID, ok := createdAcceleratorResources(receipt.receipt)
	if !ok {
		return CleanupFailed
	}
	if receipt.jobUID != "" && receipt.jobUID != jobUID {
		return CleanupOwnershipUnproven
	}
	return h.cleanupVerifiedAcceleratorResources(ctx, snapshot, prepared, receipt.receipt, captured, jobUID, receipt.unresolved || len(receipt.receipt.UnresolvedResources()) != 0)
}

func (h *helmChartInstaller) cleanupVerifiedAcceleratorResources(ctx context.Context, snapshot ContextSnapshot, prepared *preparedChart, receipt *helm.AcceleratorOwnershipReceipt, captured []capturedAcceleratorResource, jobUID types.UID, unresolved bool) CleanupStatus {
	implementation, preparedOK := prepared.implementation.(*helmPreparedChart)
	if !preparedOK || implementation == nil || implementation.prepared == nil || receipt == nil {
		return CleanupOwnershipUnproven
	}
	request := helmRequest(implementation.attempt)
	if jobUID != "" {
		var jobResource *capturedAcceleratorResource
		for i := range captured {
			if captured[i].identity.Kind == "Job" {
				jobResource = &captured[i]
				break
			}
		}
		if jobResource == nil {
			return CleanupOwnershipUnproven
		}
		if status := suspendAcceleratorJob(ctx, snapshot, *jobResource); status != CleanupSucceeded {
			return status
		}
	}
	barrier, ok := establishAcceleratorPodBarrier(ctx, snapshot, jobUID, request.WorkloadSession)
	if !ok {
		return CleanupFailed
	}
	defer barrier.stop()
	if failure := h.client.DeleteOwnedAcceleratorResources(ctx, snapshot.RESTConfig(), implementation.prepared, receipt); failure != helm.AcceleratorOK {
		return CleanupFailed
	}
	if !waitAcceleratorResourcesGone(ctx, snapshot, captured, jobUID, request.WorkloadSession, barrier) {
		return CleanupFailed
	}
	if unresolved {
		return CleanupOwnershipUnproven
	}
	if failure := h.client.PurgeOwnedAcceleratorRelease(ctx, snapshot.RESTConfig(), implementation.prepared, receipt); failure != helm.AcceleratorOK {
		return CleanupFailed
	}
	return CleanupSucceeded
}

func suspendAcceleratorJob(ctx context.Context, snapshot ContextSnapshot, captured capturedAcceleratorResource) CleanupStatus {
	if snapshot == nil || snapshot.Clientset() == nil || captured.identity.Kind != "Job" || captured.uid == "" {
		return CleanupOwnershipUnproven
	}
	jobs := snapshot.Clientset().BatchV1().Jobs(captured.identity.Namespace)
	job, err := jobs.Get(ctx, captured.identity.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return CleanupSucceeded
	}
	if err != nil {
		return CleanupFailed
	}
	if job.UID != captured.uid {
		return CleanupOwnershipUnproven
	}
	if job.DeletionTimestamp != nil {
		return CleanupFailed
	}
	if jobHasSuspendedCondition(job) {
		// Job conditions have no observed generation. A pre-existing True
		// condition cannot prove that the controller observed the current
		// suspend value, so only a post-update/watch acknowledgement is causal.
		return CleanupFailed
	}
	if job.Spec.Suspend != nil && *job.Spec.Suspend {
		return waitAcceleratorJobSuspended(ctx, snapshot, captured, job)
	}
	suspend := true
	updated := job.DeepCopy()
	updated.Spec.Suspend = &suspend
	updated, err = jobs.Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		if apierrors.IsConflict(err) {
			latest, getErr := jobs.Get(ctx, captured.identity.Name, metav1.GetOptions{})
			if getErr == nil && latest.UID != captured.uid {
				return CleanupOwnershipUnproven
			}
		}
		return CleanupFailed
	}
	if updated.UID != captured.uid {
		return CleanupOwnershipUnproven
	}
	return waitAcceleratorJobSuspended(ctx, snapshot, captured, updated)
}

func waitAcceleratorJobSuspended(ctx context.Context, snapshot ContextSnapshot, captured capturedAcceleratorResource, job *batchv1.Job) CleanupStatus {
	const retryDelay = 25 * time.Millisecond
	for {
		if job == nil || job.UID != captured.uid {
			return CleanupOwnershipUnproven
		}
		if job.DeletionTimestamp != nil {
			return CleanupFailed
		}
		if jobSuspended(job) {
			return CleanupSucceeded
		}
		watcher, err := snapshot.Clientset().BatchV1().Jobs(captured.identity.Namespace).Watch(ctx, metav1.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("metadata.name", captured.identity.Name).String(), ResourceVersion: job.ResourceVersion, AllowWatchBookmarks: true,
		})
		if err != nil {
			return CleanupFailed
		}
		relist := false
		for !relist {
			select {
			case <-ctx.Done():
				watcher.Stop()
				return CleanupFailed
			case event, open := <-watcher.ResultChan():
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
					watcher.Stop()
					return CleanupFailed
				}
				changed, typed := event.Object.(*batchv1.Job)
				if !typed || changed.Name != captured.identity.Name || changed.Namespace != captured.identity.Namespace {
					continue
				}
				if changed.UID != captured.uid {
					watcher.Stop()
					return CleanupOwnershipUnproven
				}
				if event.Type == watch.Deleted {
					watcher.Stop()
					return CleanupFailed
				}
				job = changed.DeepCopy()
				if jobSuspended(job) {
					watcher.Stop()
					return CleanupSucceeded
				}
			}
		}
		watcher.Stop()
		timer := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return CleanupFailed
		case <-timer.C:
		}
		listed, err := snapshot.Clientset().BatchV1().Jobs(captured.identity.Namespace).List(ctx, metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("metadata.name", captured.identity.Name).String()})
		if err != nil || len(listed.Items) > 1 {
			return CleanupFailed
		}
		if len(listed.Items) == 0 {
			return CleanupFailed
		}
		job = listed.Items[0].DeepCopy()
	}
}

func jobSuspended(job *batchv1.Job) bool {
	return job != nil && job.Spec.Suspend != nil && *job.Spec.Suspend && jobHasSuspendedCondition(job)
}

func jobHasSuspendedCondition(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobSuspended && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func helmRequest(attempt chartAttempt) helm.AcceleratorReleaseRequest {
	return helm.AcceleratorReleaseRequest{
		ChartReference: attempt.ChartReference, ChartDigest: attempt.ChartDigest, BuildVersion: attempt.BuildVersion,
		ImageRepository: attempt.ImageRepository, ImageDigest: attempt.ImageDigest, WorkloadSession: attempt.Session,
		CreatorVerifier: attempt.Verifier, ReleaseName: attempt.ReleaseName, ReleaseNamespace: attempt.Namespace,
	}
}

func mapHelmFailure(failure helm.AcceleratorFailure) UnavailableReason {
	switch failure {
	case helm.AcceleratorOK:
		return ""
	case helm.AcceleratorPull:
		return ChartPullFailed
	case helm.AcceleratorIntegrity:
		return ChartIntegrityFailed
	case helm.AcceleratorRender:
		return RenderFailed
	case helm.AcceleratorConflict:
		return ReleaseConflict
	case helm.AcceleratorPermission:
		return PermissionDenied
	default:
		return InstallFailed
	}
}

type capturedAcceleratorResource struct {
	identity helm.AcceleratorResourceIdentity
	uid      types.UID
}

func createdAcceleratorResources(receipt *helm.AcceleratorOwnershipReceipt) ([]capturedAcceleratorResource, types.UID, bool) {
	if receipt == nil {
		return nil, "", false
	}
	created := receipt.CreatedResources()
	captured := make([]capturedAcceleratorResource, 0, len(created))
	var jobUID types.UID
	seen := make(map[string]struct{}, len(created))
	for _, item := range created {
		identity, uid := item.Resource, item.UID
		key := identity.APIVersion + "/" + identity.Kind + "/" + identity.Namespace + "/" + identity.Name
		if uid == "" {
			return nil, "", false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, "", false
		}
		seen[key] = struct{}{}
		if identity.Kind == "Job" {
			if jobUID != "" {
				return nil, "", false
			}
			jobUID = uid
		}
		captured = append(captured, capturedAcceleratorResource{identity: identity, uid: uid})
	}
	return captured, jobUID, true
}

func getAcceleratorObject(ctx context.Context, snapshot ContextSnapshot, identity helm.AcceleratorResourceIdentity) (metav1.Object, error) {
	client := snapshot.Clientset()
	switch identity.Kind {
	case "Job":
		return client.BatchV1().Jobs(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
	case "ServiceAccount":
		return client.CoreV1().ServiceAccounts(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
	case "Secret":
		return client.CoreV1().Secrets(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
	case "ClusterRole":
		return client.RbacV1().ClusterRoles().Get(ctx, identity.Name, metav1.GetOptions{})
	case "ClusterRoleBinding":
		return client.RbacV1().ClusterRoleBindings().Get(ctx, identity.Name, metav1.GetOptions{})
	default:
		return nil, apierrors.NewNotFound(corev1.Resource("unknown"), identity.Name)
	}
}

type acceleratorPodBarrier struct {
	watcher watch.Interface
}

func (b *acceleratorPodBarrier) stop() {
	if b != nil && b.watcher != nil {
		b.watcher.Stop()
	}
}

func establishAcceleratorPodBarrier(ctx context.Context, snapshot ContextSnapshot, jobUID types.UID, session string) (*acceleratorPodBarrier, bool) {
	if snapshot == nil || snapshot.Clientset() == nil || session == "" {
		return nil, false
	}
	selector := "kubikles.io/workload-session-id=" + session
	pods, err := snapshot.Clientset().CoreV1().Pods(snapshot.Namespace()).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil || (jobUID == "" && len(pods.Items) != 0) {
		return nil, false
	}
	barrier := &acceleratorPodBarrier{}
	if jobUID != "" {
		barrier.watcher, err = snapshot.Clientset().CoreV1().Pods(snapshot.Namespace()).Watch(ctx, metav1.ListOptions{LabelSelector: selector, ResourceVersion: pods.ResourceVersion, AllowWatchBookmarks: true})
		if err != nil {
			return nil, false
		}
	}
	return barrier, true
}

func waitAcceleratorResourcesGone(ctx context.Context, snapshot ContextSnapshot, captured []capturedAcceleratorResource, jobUID types.UID, session string, barrier *acceleratorPodBarrier) bool {
	const retryDelay = 25 * time.Millisecond
	selector := "kubikles.io/workload-session-id=" + session
	for {
		allGone := true
		for _, resource := range captured {
			object, err := getAcceleratorObject(ctx, snapshot, resource.identity)
			if err == nil && object.GetUID() == resource.uid {
				allGone = false
			} else if err != nil && !apierrors.IsNotFound(err) {
				return false
			}
		}
		pods, err := snapshot.Clientset().CoreV1().Pods(snapshot.Namespace()).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return false
		}
		if jobUID == "" && len(pods.Items) != 0 {
			return false
		}
		if jobUID != "" {
			for i := range pods.Items {
				for _, owner := range pods.Items[i].OwnerReferences {
					if owner.Controller != nil && *owner.Controller && owner.UID == jobUID {
						allGone = false
					}
				}
			}
		}
		if allGone {
			return true
		}
		var events <-chan watch.Event
		if barrier != nil && barrier.watcher != nil {
			events = barrier.watcher.ResultChan()
		}
		timer := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case _, open := <-events:
			timer.Stop()
			if !open {
				barrier.watcher.Stop()
				barrier.watcher = nil
				backoff := time.NewTimer(retryDelay)
				select {
				case <-ctx.Done():
					backoff.Stop()
					return false
				case <-backoff.C:
				}
				watcher, err := snapshot.Clientset().CoreV1().Pods(snapshot.Namespace()).Watch(ctx, metav1.ListOptions{LabelSelector: selector, ResourceVersion: pods.ResourceVersion, AllowWatchBookmarks: true})
				if err != nil {
					return false
				}
				barrier.watcher = watcher
			}
		case <-timer.C:
		}
	}
}
