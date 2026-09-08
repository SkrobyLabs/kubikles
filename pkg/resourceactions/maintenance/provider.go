// Package maintenance implements documented operator spec controls. Each operator
// registers its resource types and field paths explicitly.
package maintenance

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"kubikles/pkg/resourceactions"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type Provider struct {
	Group          string
	Resource       string
	Versions       []string
	PauseField     string
	PausedValue    interface{}
	RunningValue   interface{}
	PodAnnotations []string
	SidecarMode    bool
}

func (p Provider) Actions(group, resource string) []resourceactions.Action {
	if group != p.Group || resource != p.Resource {
		return nil
	}
	prefix := p.Group + "." + p.Resource + "."
	modes := []resourceactions.Mode{{ID: "resource", Label: "This resource"}}
	actions := []resourceactions.Action{}
	if p.PauseField != "" {
		actions = append(actions, resourceactions.Action{ID: prefix + "pause", Label: "Pause reconciliation…", Description: "Pause operator management of this resource. Existing workloads keep running; operator deletion behavior is unchanged.", Modes: modes}, resourceactions.Action{ID: prefix + "resume", Label: "Resume reconciliation…", Description: "Resume operator management so the desired configuration can be applied to its workloads.", Modes: modes})
	}
	if len(p.PodAnnotations) > 0 {
		actions = append(actions, resourceactions.Action{ID: prefix + "restart", Label: "Restart managed workloads…", Description: "Update the operator's pod template to request a workload rollout. Availability during the rollout depends on the workload's replicas and update strategy.", Modes: modes})
	}
	return actions
}
func (p Provider) operation(id string) string {
	return strings.TrimPrefix(id, p.Group+"."+p.Resource+".")
}
func (p Provider) paused(obj *unstructured.Unstructured) bool {
	v, _, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec", p.PauseField)
	return p.PauseField != "" && reflect.DeepEqual(v, p.PausedValue)
}
func (p Provider) validateRestart(obj *unstructured.Unstructured) error {
	if p.paused(obj) {
		return fmt.Errorf("resume reconciliation before requesting a rollout")
	}
	if p.SidecarMode {
		mode, _, _ := unstructured.NestedString(obj.Object, "spec", "mode")
		if mode == "sidecar" {
			return fmt.Errorf("sidecar collectors require restarting their owning application workloads")
		}
	}
	return nil
}
func (p Provider) Prepare(ctx context.Context, client dynamic.Interface, ref resourceactions.ResourceRef, id, mode string) (resourceactions.Plan, error) {
	plan := resourceactions.Plan{Source: ref, ActionID: id, Mode: mode, Targets: []resourceactions.Target{}}
	supported := false
	for _, v := range p.Versions {
		if ref.Version == v {
			supported = true
		}
	}
	if !supported {
		return plan, fmt.Errorf("API version %s is not supported for this action", ref.Version)
	}
	obj, err := resourceactions.ReadSource(ctx, client, ref)
	if err != nil {
		return plan, err
	}
	target := resourceactions.ObjectTarget(ref.Context, ref.Resource, obj)
	switch p.operation(id) {
	case "pause":
		target.Pending = p.paused(obj)
		plan.Summary = "The operator will stop reconciling this resource. Existing workloads will continue running."
	case "resume":
		target.Pending = !p.paused(obj)
		plan.Summary = "The operator will resume reconciliation and may update managed workloads to match their desired configuration."
	case "restart":
		if err := p.validateRestart(obj); err != nil {
			return plan, err
		}
		plan.Summary = "The operator will propagate the restart request to its managed pod templates. Completion follows the workload's configured update strategy."
	default:
		return plan, fmt.Errorf("unsupported maintenance action")
	}
	plan.Targets = append(plan.Targets, target)
	return plan, nil
}
func (p Provider) Mutation(plan resourceactions.Plan, _ resourceactions.Target, obj *unstructured.Unstructured) (resourceactions.Mutation, error) {
	var fields []string
	var value interface{}
	switch p.operation(plan.ActionID) {
	case "pause":
		if p.paused(obj) {
			return resourceactions.Mutation{Pending: true}, nil
		}
		fields = []string{"spec", p.PauseField}
		value = p.PausedValue
	case "resume":
		if !p.paused(obj) {
			return resourceactions.Mutation{Pending: true}, nil
		}
		fields = []string{"spec", p.PauseField}
		value = p.RunningValue
	case "restart":
		if err := p.validateRestart(obj); err != nil {
			return resourceactions.Mutation{}, err
		}
		fields = append(append([]string{}, p.PodAnnotations...), "kubikles.io/restart-request")
		existing, _, _ := unstructured.NestedString(obj.Object, fields...)
		if existing == plan.RequestID {
			return resourceactions.Mutation{Pending: true}, nil
		}
		value = plan.RequestID
	default:
		return resourceactions.Mutation{}, fmt.Errorf("unsupported maintenance action")
	}
	return resourceactions.Mutation{Operations: resourceactions.SetField(obj, value, fields...)}, nil
}
