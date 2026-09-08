package velero

import (
	"context"
	"fmt"

	"kubikles/pkg/resourceactions"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type BackupProvider struct{}

func (BackupProvider) Actions(group, resource string) []resourceactions.Action {
	if group != "velero.io" || resource != "backups" {
		return nil
	}
	return []resourceactions.Action{{ID: "velero.backup.repeat", Label: "Run backup again…", Description: "Create a new backup using this backup's resource filters and storage settings. This captures current cluster data and leaves the original backup unchanged.", Modes: []resourceactions.Mode{{ID: "backup", Label: "This backup configuration"}}}}
}
func completedBackup(obj *unstructured.Unstructured) bool {
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	switch phase {
	case "Completed", "PartiallyFailed", "Failed", "FailedValidation":
		return true
	}
	return false
}
func (BackupProvider) Prepare(ctx context.Context, client dynamic.Interface, ref resourceactions.ResourceRef, id, mode string) (resourceactions.Plan, error) {
	plan := resourceactions.Plan{Source: ref, ActionID: id, Mode: mode, Targets: []resourceactions.Target{}, Summary: "Create a separate Backup request. Existing backup data is retained. Any configuration errors in the original backup must be corrected before a repeat can succeed."}
	if ref.Version != "v1" {
		return plan, fmt.Errorf("backup actions require the Velero v1 API")
	}
	obj, err := resourceactions.ReadSource(ctx, client, ref)
	if err != nil {
		return plan, err
	}
	if !completedBackup(obj) {
		return plan, fmt.Errorf("wait until the original backup has finished before running it again")
	}
	spec, found, _ := unstructured.NestedMap(obj.Object, "spec")
	if !found || len(spec) == 0 {
		return plan, fmt.Errorf("backup has no specification")
	}
	plan.Fingerprint = scheduleFingerprint(obj)
	plan.Targets = append(plan.Targets, resourceactions.ObjectTarget(ref.Context, ref.Resource, obj))
	return plan, nil
}
func (BackupProvider) Mutation(plan resourceactions.Plan, _ resourceactions.Target, obj *unstructured.Unstructured) (resourceactions.Mutation, error) {
	if !completedBackup(obj) || scheduleFingerprint(obj) != plan.Fingerprint {
		return resourceactions.Mutation{}, fmt.Errorf("backup changed; refresh the preview")
	}
	spec, _, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return resourceactions.Mutation{}, err
	}
	backup := newBackup(obj, plan.RequestID, spec)
	// Runtime metadata and owner references from the completed backup do not
	// belong on a new standalone request. Its full BackupSpec is retained.
	return resourceactions.Mutation{Create: &resourceactions.Creation{Resource: schema.GroupVersionResource{Group: "velero.io", Version: "v1", Resource: "backups"}, Object: backup}}, nil
}
