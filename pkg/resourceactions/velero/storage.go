// Package velero implements Velero maintenance requests.
package velero

import (
	"context"
	"fmt"

	"kubikles/pkg/resourceactions"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type StorageProvider struct{}

func (StorageProvider) Actions(group, resource string) []resourceactions.Action {
	if group != "velero.io" || resource != "backupstoragelocations" {
		return nil
	}
	return []resourceactions.Action{{ID: "velero.storage.revalidate", Label: "Revalidate storage…", Description: "Make this backup storage location eligible for another Velero validation check.", Modes: []resourceactions.Mode{{ID: "location", Label: "This location"}}}}
}
func (StorageProvider) Prepare(ctx context.Context, client dynamic.Interface, ref resourceactions.ResourceRef, id, mode string) (resourceactions.Plan, error) {
	plan := resourceactions.Plan{Source: ref, ActionID: id, Mode: mode, Targets: []resourceactions.Target{}, Summary: "Reset the last validation timestamp in one guarded patch. Velero will check the location on its next validation poll; the configured validation frequency stays unchanged."}
	if ref.Version != "v1" {
		return plan, fmt.Errorf("storage validation requires the Velero v1 API")
	}
	obj, err := resourceactions.ReadSource(ctx, client, ref)
	if err != nil {
		return plan, err
	}
	target := resourceactions.ObjectTarget(ref.Context, ref.Resource, obj)
	value, _, _ := unstructured.NestedFieldNoCopy(obj.Object, "status", "lastValidationTime")
	target.Pending = value == nil
	plan.Targets = append(plan.Targets, target)
	return plan, nil
}

// Velero's IsReadyToValidate treats a nil LastValidationTime as requiring a
// validation, even with periodic validation disabled. The controller polls this
// predicate, so a spec edit or an artificial status phase is unnecessary.
// https://github.com/velero-io/velero/blob/main/internal/storage/storagelocation.go
func (StorageProvider) Mutation(_ resourceactions.Plan, _ resourceactions.Target, obj *unstructured.Unstructured) (resourceactions.Mutation, error) {
	value, _, _ := unstructured.NestedFieldNoCopy(obj.Object, "status", "lastValidationTime")
	if value == nil {
		return resourceactions.Mutation{Pending: true}, nil
	}
	// Velero v1 resources do not expose a separate status subresource.
	return resourceactions.Mutation{Operations: []map[string]interface{}{{"op": "remove", "path": "/status/lastValidationTime"}}}, nil
}
