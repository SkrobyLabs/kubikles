package velero

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"kubikles/pkg/resourceactions"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

type ScheduleProvider struct{}

func (ScheduleProvider) Actions(group, resource string) []resourceactions.Action {
	if group != "velero.io" || resource != "schedules" {
		return nil
	}
	modes := []resourceactions.Mode{{ID: "schedule", Label: "This schedule"}}
	return []resourceactions.Action{
		{ID: "velero.schedule.backup", Label: "Back up now…", Description: "Create a new backup from this schedule's template, including its resource filters and storage settings. This can also run a paused schedule once.", Modes: modes},
		{ID: "velero.schedule.pause", Label: "Pause schedule…", Description: "Stop new scheduled backups. Backups already running will continue.", Modes: modes},
		{ID: "velero.schedule.resume", Label: "Resume schedule…", Description: "Resume scheduled backups. A backup may run immediately if one is due, according to the schedule's skipImmediately setting.", Modes: modes},
	}
}
func scheduleFingerprint(obj *unstructured.Unstructured) string {
	b, _ := json.Marshal([]interface{}{obj.Object["spec"], obj.GetLabels(), obj.GetAnnotations()})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (ScheduleProvider) Prepare(ctx context.Context, client dynamic.Interface, ref resourceactions.ResourceRef, id, mode string) (resourceactions.Plan, error) {
	plan := resourceactions.Plan{Source: ref, ActionID: id, Mode: mode, Targets: []resourceactions.Target{}}
	if ref.Version != "v1" {
		return plan, fmt.Errorf("schedule actions require the Velero v1 API")
	}
	obj, err := resourceactions.ReadSource(ctx, client, ref)
	if err != nil {
		return plan, err
	}
	target := resourceactions.ObjectTarget(ref.Context, ref.Resource, obj)
	paused, _, _ := unstructured.NestedBool(obj.Object, "spec", "paused")
	switch id {
	case "velero.schedule.pause":
		target.Pending = paused
		plan.Summary = "Set this schedule to paused. Its schedule expression and backup template remain unchanged."
	case "velero.schedule.resume":
		target.Pending = !paused
		plan.Summary = "Resume this schedule using its existing expression and backup template."
	case "velero.schedule.backup":
		template, found, _ := unstructured.NestedMap(obj.Object, "spec", "template")
		if !found || len(template) == 0 {
			return plan, fmt.Errorf("schedule has no backup template")
		}
		plan.Fingerprint = scheduleFingerprint(obj)
		plan.Summary = "Create one new Backup from the current schedule template. Repeating this same request will reuse that backup; Velero reports its progress through the Backup status."
	}
	plan.Targets = append(plan.Targets, target)
	return plan, nil
}

// Mirrors Velero's BackupBuilder.FromSchedule, preserving filters, storage,
// template metadata overrides and the optional schedule owner reference.
// https://github.com/velero-io/velero/blob/main/pkg/builder/backup_builder.go
func (ScheduleProvider) Mutation(plan resourceactions.Plan, _ resourceactions.Target, obj *unstructured.Unstructured) (resourceactions.Mutation, error) {
	if plan.ActionID != "velero.schedule.backup" {
		paused, _, _ := unstructured.NestedBool(obj.Object, "spec", "paused")
		want := plan.ActionID == "velero.schedule.pause"
		if paused == want {
			return resourceactions.Mutation{Pending: true}, nil
		}
		return resourceactions.Mutation{Operations: resourceactions.SetField(obj, want, "spec", "paused")}, nil
	}
	if scheduleFingerprint(obj) != plan.Fingerprint {
		return resourceactions.Mutation{}, fmt.Errorf("schedule changed; refresh the preview")
	}
	template, _, err := unstructured.NestedMap(obj.Object, "spec", "template")
	if err != nil {
		return resourceactions.Mutation{}, err
	}
	metadata, _ := template["metadata"].(map[string]interface{})
	labels := obj.GetLabels()
	annotations := obj.GetAnnotations()
	if override, ok := metadata["labels"].(map[string]interface{}); ok {
		labels = stringMap(override)
	}
	if override, ok := metadata["annotations"].(map[string]interface{}); ok {
		annotations = stringMap(override)
	}
	if labels == nil {
		labels = map[string]string{}
	}
	labels["velero.io/schedule-name"] = obj.GetName()
	backup := newBackup(obj, plan.RequestID, template)
	backup.SetLabels(labels)
	backup.SetAnnotations(annotations)
	refs, _, _ := unstructured.NestedBool(obj.Object, "spec", "useOwnerReferencesInBackup")
	if refs {
		controller := true
		backup.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: obj.GetAPIVersion(), Kind: "Schedule", Name: obj.GetName(), UID: types.UID(plan.Source.UID), Controller: &controller}})
	}
	return resourceactions.Mutation{Create: &resourceactions.Creation{Resource: schema.GroupVersionResource{Group: "velero.io", Version: "v1", Resource: "backups"}, Object: backup}}, nil
}
func stringMap(values map[string]interface{}) map[string]string {
	result := map[string]string{}
	for k, v := range values {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}
func newBackup(source *unstructured.Unstructured, requestID string, spec map[string]interface{}) *unstructured.Unstructured {
	prefix := source.GetName()
	if len(prefix) > 200 {
		prefix = prefix[:200]
	}
	prefix = strings.TrimRight(prefix, "-.")
	return &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "velero.io/v1", "kind": "Backup", "metadata": map[string]interface{}{"name": prefix + "-manual-" + requestID, "namespace": source.GetNamespace()}, "spec": spec}}
}
