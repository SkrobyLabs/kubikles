package main

import (
	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"

	"fmt"

	batchv1 "k8s.io/api/batch/v1"
)

// =============================================================================
// Jobs & CronJobs
// =============================================================================

func (a *App) ListJobs(requestId, namespace string) ([]batchv1.Job, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ListJobs called", map[string]interface{}{"context": currentContext, "namespace": namespace})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListJobsWithContext(ctx, currentContext, namespace, a.listProgressCallback("jobs"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListJobs(currentContext, namespace)
}

func (a *App) GetJobYaml(namespace, name string) (string, error) {
	debug.LogK8s("GetJobYaml called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetJobYaml(namespace, name)
}

func (a *App) UpdateJobYaml(namespace, name, yamlContent string) error {
	debug.LogK8s("UpdateJobYaml called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateJobYaml(namespace, name, yamlContent)
}

func (a *App) DeleteJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteJob called", map[string]interface{}{"context": currentContext, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteJob(currentContext, namespace, name)
}

func (a *App) ListCronJobs(requestId, namespace string) ([]batchv1.CronJob, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ListCronJobs called", map[string]interface{}{"context": currentContext, "namespace": namespace})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListCronJobsWithContext(ctx, currentContext, namespace, a.listProgressCallback("cronjobs"))
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListCronJobs(currentContext, namespace)
}

func (a *App) GetCronJobYaml(namespace, name string) (string, error) {
	debug.LogK8s("GetCronJobYaml called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCronJobYaml(namespace, name)
}

func (a *App) UpdateCronJobYaml(namespace, name, yamlContent string) error {
	debug.LogK8s("UpdateCronJobYaml called", map[string]interface{}{"namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCronJobYaml(namespace, name, yamlContent)
}

func (a *App) DeleteCronJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteCronJob called", map[string]interface{}{"context": currentContext, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCronJob(currentContext, namespace, name)
}

func (a *App) TriggerCronJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("TriggerCronJob called", map[string]interface{}{"context": currentContext, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.TriggerCronJob(currentContext, namespace, name)
}

func (a *App) SuspendCronJob(namespace, name string, suspend bool) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("SuspendCronJob called", map[string]interface{}{"context": currentContext, "namespace": namespace, "name": name, "suspend": suspend})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.SuspendCronJob(currentContext, namespace, name, suspend)
}
