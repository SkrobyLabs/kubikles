package main

import (
	"fmt"

	"kubikles/pkg/k8s"
)

// =============================================================================
// DIAGNOSTIC TOOLKIT - Flow Timeline, Resource Diff, RBAC Checker
// =============================================================================

// GetFlowTimeline returns a unified timeline of events and logs for a resource and its dependencies
func (a *App) GetFlowTimeline(resourceType, namespace, name string, durationMinutes int, includeLogs bool) ([]k8s.FlowTimelineEntry, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("no Kubernetes client available")
	}

	req := k8s.FlowTimelineRequest{
		ResourceType:    resourceType,
		Namespace:       namespace,
		Name:            name,
		DurationMinutes: durationMinutes,
		IncludeLogs:     includeLogs,
		MaxEntries:      200,
	}

	return a.k8sClient.GetFlowTimeline(req)
}

// DiffResources compares two Kubernetes resources and returns a structured diff
func (a *App) DiffResources(sourceContext, sourceNamespace, sourceKind, sourceName, targetContext, targetNamespace, targetKind, targetName string, ignoreFields []string) (*k8s.DiffResult, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("no Kubernetes client available")
	}

	req := k8s.DiffRequest{
		SourceContext:   sourceContext,
		SourceNamespace: sourceNamespace,
		SourceKind:      sourceKind,
		SourceName:      sourceName,
		TargetContext:   targetContext,
		TargetNamespace: targetNamespace,
		TargetKind:      targetKind,
		TargetName:      targetName,
		IgnoreFields:    ignoreFields,
	}

	return a.k8sClient.DiffResources(req)
}

// CheckRBACAccess checks if a subject has permission to perform an action
func (a *App) CheckRBACAccess(subjectKind, subjectName, subjectNamespace, verb, resource, resourceName, namespace, apiGroup string) (*k8s.RBACCheckResult, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("no Kubernetes client available")
	}

	req := k8s.RBACCheckRequest{
		SubjectKind:      subjectKind,
		SubjectName:      subjectName,
		SubjectNamespace: subjectNamespace,
		Verb:             verb,
		Resource:         resource,
		ResourceName:     resourceName,
		Namespace:        namespace,
		APIGroup:         apiGroup,
	}

	return a.k8sClient.CheckRBACAccess(req)
}

// GetMultiPodLogs fetches logs from multiple pods matching criteria (batch mode)
func (a *App) GetMultiPodLogs(namespace string, labelSelector map[string]string, podNames []string, container string, tailLines int64, sinceSeconds int64) ([]k8s.MultiLogEntry, error) {
	fmt.Printf("[GetMultiPodLogs] Called: ns=%s labels=%v pods=%v container=%s tail=%d since=%d\n",
		namespace, labelSelector, podNames, container, tailLines, sinceSeconds)

	if a.k8sClient == nil {
		return nil, fmt.Errorf("no Kubernetes client available")
	}

	req := k8s.MultiLogRequest{
		Namespace:     namespace,
		LabelSelector: labelSelector,
		PodNames:      podNames,
		Container:     container,
		TailLines:     tailLines,
		SinceSeconds:  sinceSeconds,
		Follow:        false,
		Timestamps:    true,
	}

	result, err := a.k8sClient.GetMultiPodLogsBatch(req)
	if err != nil {
		fmt.Printf("[GetMultiPodLogs] Error: %v\n", err)
		return nil, err
	}
	fmt.Printf("[GetMultiPodLogs] Returned %d log entries\n", len(result))
	return result, nil
}
