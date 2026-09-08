package main

import (
	"fmt"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
	"kubikles/pkg/resourceactions"
)

func (a *App) GetResourceActions(group, resource string) []resourceactions.Action {
	return k8s.ResourceActions(group, resource)
}
func (a *App) PrepareResourceAction(ref resourceactions.ResourceRef, actionID, mode string) (resourceactions.Plan, error) {
	if a.k8sClient == nil {
		return resourceactions.Plan{}, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.PrepareResourceAction(ref, actionID, mode)
}
func (a *App) ExecuteResourceAction(plan resourceactions.Plan) ([]resourceactions.Result, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	debug.LogK8s("Executing operator action", map[string]interface{}{"context": plan.Source.Context, "action": plan.ActionID, "namespace": plan.Source.Namespace, "resource": plan.Source.Name, "targets": len(plan.Targets), "requestId": plan.RequestID})
	results, err := a.k8sClient.ExecuteResourceAction(plan)
	for _, result := range results {
		debug.LogK8s("Operator action result", map[string]interface{}{"context": plan.Source.Context, "action": plan.ActionID, "target": result.Target.Name, "namespace": result.Target.Namespace, "status": result.Status, "error": result.Error, "requestId": plan.RequestID})
	}
	if err != nil {
		debug.LogK8s("Operator action rejected", map[string]interface{}{"action": plan.ActionID, "error": err.Error(), "requestId": plan.RequestID})
	}
	return results, err
}
