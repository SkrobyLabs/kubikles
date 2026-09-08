package velero

import (
	"context"
	"fmt"

	"kubikles/pkg/resourceactions"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type TransferProvider struct{ Download bool }

func (p TransferProvider) resource() string {
	if p.Download {
		return "datadownloads"
	}
	return "datauploads"
}
func (p TransferProvider) Actions(group, resource string) []resourceactions.Action {
	if group != "velero.io" || resource != p.resource() {
		return nil
	}
	return []resourceactions.Action{{ID: "velero." + p.resource() + ".cancel", Label: "Cancel data transfer…", Description: "Request cancellation of this volume's data transfer. The parent backup or restore may become incomplete; other transfers are not canceled.", Modes: []resourceactions.Mode{{ID: "transfer", Label: "This transfer"}}}}
}
func transferState(obj *unstructured.Unstructured) (bool, error) {
	mover, _, _ := unstructured.NestedString(obj.Object, "spec", "datamover")
	if mover != "" && mover != "velero" {
		return false, fmt.Errorf("cancellation is only supported for Velero's built-in data mover")
	}
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	cancel, _, _ := unstructured.NestedBool(obj.Object, "spec", "cancel")
	switch phase {
	case "Canceled", "Cancelled", "Canceling":
		return true, nil
	case "Completed", "Failed":
		return false, fmt.Errorf("data transfer has already finished")
	case "", "New", "Accepted", "Prepared", "InProgress":
		return cancel, nil
	default:
		return false, fmt.Errorf("unsupported data transfer phase %q", phase)
	}
}
func (p TransferProvider) Prepare(ctx context.Context, client dynamic.Interface, ref resourceactions.ResourceRef, id, mode string) (resourceactions.Plan, error) {
	plan := resourceactions.Plan{Source: ref, ActionID: id, Mode: mode, Targets: []resourceactions.Target{}, Summary: "Set the cancellation request on this transfer. Velero's data mover handles cleanup and reports completion through the transfer status."}
	if ref.Version != "v2alpha1" {
		return plan, fmt.Errorf("data transfer cancellation requires the Velero v2alpha1 API")
	}
	obj, err := resourceactions.ReadSource(ctx, client, ref)
	if err != nil {
		return plan, err
	}
	pending, err := transferState(obj)
	if err != nil {
		return plan, err
	}
	target := resourceactions.ObjectTarget(ref.Context, ref.Resource, obj)
	target.Pending = pending
	plan.Targets = append(plan.Targets, target)
	return plan, nil
}
func (TransferProvider) Mutation(_ resourceactions.Plan, _ resourceactions.Target, obj *unstructured.Unstructured) (resourceactions.Mutation, error) {
	pending, err := transferState(obj)
	if err != nil {
		return resourceactions.Mutation{}, err
	}
	if pending {
		return resourceactions.Mutation{Pending: true}, nil
	}
	return resourceactions.Mutation{Operations: resourceactions.SetField(obj, true, "spec", "cancel")}, nil
}
