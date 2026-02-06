package main

import (
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"

	"kubikles/pkg/k8s"
)

// =============================================================================
// Network Resources
// =============================================================================

func (a *App) ListNetworkPolicies(requestId, namespace string) ([]networkingv1.NetworkPolicy, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListNetworkPoliciesWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListNetworkPolicies(namespace)
}

func (a *App) GetNetworkPolicyYaml(namespace, name string) (string, error) {
	a.logDebug("GetNetworkPolicyYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetNetworkPolicyYaml(namespace, name)
}

func (a *App) UpdateNetworkPolicyYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateNetworkPolicyYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateNetworkPolicyYaml(namespace, name, yamlContent)
}

func (a *App) DeleteNetworkPolicy(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteNetworkPolicy called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteNetworkPolicy(currentContext, namespace, name)
}

// HorizontalPodAutoscaler operations (namespaced)
func (a *App) ListHPAs(requestId, namespace string) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListHPAsWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListHPAs(namespace)
}

func (a *App) GetHPAYaml(namespace, name string) (string, error) {
	a.logDebug("GetHPAYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetHPAYaml(namespace, name)
}

func (a *App) UpdateHPAYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateHPAYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateHPAYaml(namespace, name, yamlContent)
}

func (a *App) DeleteHPA(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteHPA called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteHPA(currentContext, namespace, name)
}

// PodDisruptionBudget operations (namespaced)
func (a *App) ListPDBs(requestId, namespace string) ([]policyv1.PodDisruptionBudget, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListPDBsWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListPDBs(namespace)
}

func (a *App) GetPDBYaml(namespace, name string) (string, error) {
	a.logDebug("GetPDBYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPDBYaml(namespace, name)
}

func (a *App) UpdatePDBYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdatePDBYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdatePDBYaml(namespace, name, yamlContent)
}

func (a *App) DeletePDB(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeletePDB called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeletePDB(currentContext, namespace, name)
}

// ResourceQuota operations (namespaced)
func (a *App) ListResourceQuotas(requestId, namespace string) ([]v1.ResourceQuota, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListResourceQuotasWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListResourceQuotas(namespace)
}

func (a *App) GetResourceQuotaYaml(namespace, name string) (string, error) {
	a.logDebug("GetResourceQuotaYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetResourceQuotaYaml(namespace, name)
}

func (a *App) UpdateResourceQuotaYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateResourceQuotaYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateResourceQuotaYaml(namespace, name, yamlContent)
}

func (a *App) DeleteResourceQuota(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteResourceQuota called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteResourceQuota(currentContext, namespace, name)
}

// LimitRange operations (namespaced)
func (a *App) ListLimitRanges(requestId, namespace string) ([]v1.LimitRange, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListLimitRangesWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListLimitRanges(namespace)
}

func (a *App) GetLimitRangeYaml(namespace, name string) (string, error) {
	a.logDebug("GetLimitRangeYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetLimitRangeYaml(namespace, name)
}

func (a *App) UpdateLimitRangeYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateLimitRangeYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateLimitRangeYaml(namespace, name, yamlContent)
}

func (a *App) DeleteLimitRange(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteLimitRange called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteLimitRange(currentContext, namespace, name)
}

func (a *App) ListEndpoints(requestId, namespace string) ([]v1.Endpoints, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListEndpointsWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListEndpoints(namespace)
}

func (a *App) GetEndpointsYaml(namespace, name string) (string, error) {
	a.logDebug("GetEndpointsYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetEndpointsYaml(namespace, name)
}

func (a *App) UpdateEndpointsYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateEndpointsYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateEndpointsYaml(namespace, name, yamlContent)
}

func (a *App) DeleteEndpoints(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteEndpoints called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteEndpoints(currentContext, namespace, name)
}

// EndpointSlice operations (namespaced, discovery.k8s.io/v1)
func (a *App) ListEndpointSlices(requestId, namespace string) ([]discoveryv1.EndpointSlice, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	if requestId != "" {
		ctx, seq := a.listRequestManager.StartRequest(requestId)
		defer a.listRequestManager.CompleteRequest(requestId, seq)

		result, err := a.k8sClient.ListEndpointSlicesWithContext(ctx, namespace)
		if err == k8s.ErrRequestCancelled {
			return nil, nil
		}
		return result, err
	}
	return a.k8sClient.ListEndpointSlices(namespace)
}

func (a *App) GetEndpointSliceYaml(namespace, name string) (string, error) {
	a.logDebug("GetEndpointSliceYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetEndpointSliceYaml(namespace, name)
}

func (a *App) UpdateEndpointSliceYaml(namespace, name, yamlContent string) error {
	a.logDebug("UpdateEndpointSliceYaml called: ns=%s, name=%s", namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateEndpointSliceYaml(namespace, name, yamlContent)
}

func (a *App) DeleteEndpointSlice(namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteEndpointSlice called: context=%s, ns=%s, name=%s", currentContext, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteEndpointSlice(currentContext, namespace, name)
}
