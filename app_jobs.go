package main

import (
	"kubikles/pkg/k8s"

	"fmt"

	batchv1 "k8s.io/api/batch/v1"
)

// =============================================================================
// Jobs & CronJobs
// =============================================================================

func (a *App) ListJobs(requestId, namespace string) ([]batchv1.Job, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ListJobs called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListJobsWithContext(ctx, currentContext, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListJobs(currentContext, namespace)
}

func (a *App) GetJobYaml(namespace, name string) (string, error) {
	a.logDebug("GetJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetJobYaml(namespace, name)
}

func (a *App) UpdateJobYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateJobYaml(namespace, name, yamlContent)
}

func (a *App) DeleteJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteJob called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteJob(currentContext, namespace, name)
}

func (a *App) ListCronJobs(requestId, namespace string) ([]batchv1.CronJob, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ListCronJobs called: context=%s, ns=%s", currentContext, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListCronJobsWithContext(ctx, currentContext, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListCronJobs(currentContext, namespace)
}

func (a *App) GetCronJobYaml(namespace, name string) (string, error) {
	a.logDebug("GetCronJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCronJobYaml(namespace, name)
}

func (a *App) UpdateCronJobYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateCronJobYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCronJobYaml(namespace, name, yamlContent)
}

func (a *App) DeleteCronJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteCronJob called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCronJob(currentContext, namespace, name)
}

func (a *App) TriggerCronJob(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("TriggerCronJob called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.TriggerCronJob(currentContext, namespace, name)
}

func (a *App) SuspendCronJob(namespace, name string, suspend bool) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("SuspendCronJob called: context=%s, ns=%s, name=%s, suspend=%v", currentContext, namespace, name, suspend)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.SuspendCronJob(currentContext, namespace, name, suspend)
}
