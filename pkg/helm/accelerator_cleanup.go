//go:build helm

package helm

import (
	"context"
	"crypto/sha256"
	"time"

	"helm.sh/helm/v3/pkg/action"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const AcceleratorOwnedCleanupTimeout = 45 * time.Second

// UninstallOwnedAcceleratorRelease is the dedicated SDK cleanup path for an
// exact revision-one Accelerator receipt. The final ownership proof and Run
// occur synchronously through the same captured action configuration.
func (c *Client) UninstallOwnedAcceleratorRelease(ctx context.Context, config *rest.Config, prepared *AcceleratorPreparedRelease, receipt *AcceleratorOwnershipReceipt, pod AcceleratorDeletionIdentity) AcceleratorOwnedCleanupStatus {
	if ctx == nil || prepared == nil || receipt == nil {
		return AcceleratorOwnedCleanupOwnershipChanged
	}
	actionConfig, err := acceleratorActionConfig(ctx, config, prepared.request.ReleaseNamespace)
	if err != nil {
		return AcceleratorOwnedCleanupFailed
	}
	clientset, err := acceleratorClientset(ctx, config)
	if err != nil {
		return AcceleratorOwnedCleanupFailed
	}
	return uninstallOwnedAcceleratorReleaseWith(ctx, actionConfig, clientset, prepared, receipt, pod)
}

func uninstallOwnedAcceleratorReleaseWith(ctx context.Context, actionConfig *action.Configuration, clientset kubernetes.Interface, prepared *AcceleratorPreparedRelease, receipt *AcceleratorOwnershipReceipt, pod AcceleratorDeletionIdentity) AcceleratorOwnedCleanupStatus {
	if actionConfig == nil || clientset == nil || prepared == nil || receipt == nil {
		return AcceleratorOwnedCleanupOwnershipChanged
	}
	// This is intentionally the final check immediately before mutation.
	storedStatus := inspectAcceleratorOwnershipStatusWith(ctx, actionConfig, clientset, prepared, receipt)
	if storedStatus == AcceleratorOwnedCleanupAlreadyGone {
		return storedStatus
	}
	if storedStatus != AcceleratorOwnedCleanupSucceeded {
		return AcceleratorOwnedCleanupOwnershipChanged
	}
	if !proveAcceleratorLiveOwnership(ctx, clientset, prepared, receipt, pod) {
		return AcceleratorOwnedCleanupOwnershipChanged
	}
	uninstall := configureAcceleratorOwnedUninstall(actionConfig)
	if uninstall == nil {
		return AcceleratorOwnedCleanupFailed
	}
	if _, err := uninstall.Run(prepared.request.ReleaseName); err != nil {
		return AcceleratorOwnedCleanupFailed
	}
	return AcceleratorOwnedCleanupSucceeded
}

func proveAcceleratorLiveOwnership(ctx context.Context, client kubernetes.Interface, prepared *AcceleratorPreparedRelease, receipt *AcceleratorOwnershipReceipt, pod AcceleratorDeletionIdentity) bool {
	if client == nil || prepared == nil || receipt == nil || len(receipt.created) != 5 || len(receipt.unresolved) != 0 || !validReceiptCreatedResources(prepared, receipt) {
		return false
	}
	storedProof := acceleratorClosedReleaseProof{request: prepared.request, resources: prepared.resources, jobName: prepared.jobName, renderHash: prepared.renderHash, verifierHash: sha256.Sum256([]byte(prepared.request.CreatorVerifier))}
	var jobUID types.UID
	names := make(map[string]string, len(receipt.created))
	for _, captured := range receipt.created {
		names[captured.Resource.Kind] = captured.Resource.Name
	}
	for _, captured := range receipt.created {
		if captured.Resource.Kind == "Job" {
			jobUID = captured.UID
		}
		object, err := getAcceleratorLiveObject(ctx, client, captured.Resource)
		if apierrors.IsNotFound(err) && captured.Resource.Kind == "Job" {
			continue
		}
		if err != nil || object.GetUID() != captured.UID || !acceleratorClosedLiveObjectContract(object, storedProof, names) {
			return false
		}
	}
	if pod.Resource.Name == "" || pod.UID == "" {
		return false
	}
	observed, err := client.CoreV1().Pods(pod.Resource.Namespace).Get(ctx, pod.Resource.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return true
	}
	if err != nil || observed.UID != pod.UID || !acceleratorClosedPodContract(observed, storedProof, prepared.jobName, jobUID) {
		return false
	}
	return true
}

func sameStrings(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	seen := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		seen[value] = struct{}{}
	}
	for _, value := range expected {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return true
}

func acceleratorHelmMetadata(object metav1.Object, request AcceleratorReleaseRequest) bool {
	if object == nil || object.GetDeletionTimestamp() != nil {
		return false
	}
	labels, annotations := object.GetLabels(), object.GetAnnotations()
	wantLabels := acceleratorLabels(request.ReleaseName)
	wantAnnotations := map[string]string{"meta.helm.sh/release-name": request.ReleaseName, "meta.helm.sh/release-namespace": request.ReleaseNamespace}
	if _, ok := object.(*batchv1.Job); ok {
		wantLabels["kubikles.io/workload-session-id"] = request.WorkloadSession
		wantAnnotations["kubikles.io/build-version"] = request.BuildVersion
	}
	return equalStringMap(labels, wantLabels) && equalStringMap(annotations, wantAnnotations)
}

func getAcceleratorLiveObject(ctx context.Context, client kubernetes.Interface, identity AcceleratorResourceIdentity) (metav1.Object, error) {
	switch identity.Kind {
	case "Job":
		return client.BatchV1().Jobs(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
	case "Pod":
		return client.CoreV1().Pods(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
	case "Secret":
		return client.CoreV1().Secrets(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
	case "ServiceAccount":
		return client.CoreV1().ServiceAccounts(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
	case "ClusterRole":
		return client.RbacV1().ClusterRoles().Get(ctx, identity.Name, metav1.GetOptions{})
	case "ClusterRoleBinding":
		return client.RbacV1().ClusterRoleBindings().Get(ctx, identity.Name, metav1.GetOptions{})
	default:
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "accelerator"}, identity.Name)
	}
}

func configureAcceleratorOwnedUninstall(actionConfig *action.Configuration) *action.Uninstall {
	if actionConfig == nil {
		return nil
	}
	uninstall := action.NewUninstall(actionConfig)
	uninstall.DisableHooks = true
	uninstall.KeepHistory = false
	uninstall.Wait = true
	uninstall.Timeout = AcceleratorOwnedCleanupTimeout
	uninstall.DeletionPropagation = "foreground"
	uninstall.IgnoreNotFound = false
	return uninstall
}
