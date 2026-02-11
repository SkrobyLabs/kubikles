package k8s

import (
	"context"
	"fmt"
	"log"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListNetworkPolicies(namespace string) ([]networkingv1.NetworkPolicy, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// ListNetworkPoliciesForContext lists network policies for a specific kubeconfig context
func (c *Client) ListNetworkPoliciesForContext(contextName, namespace string) ([]networkingv1.NetworkPolicy, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListNetworkPoliciesWithContext(ctx context.Context, namespace string) ([]networkingv1.NetworkPolicy, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetNetworkPolicyYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	policy, err := cs.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	policy.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateNetworkPolicyYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var policy networkingv1.NetworkPolicy
	if err := yaml.Unmarshal([]byte(yamlContent), &policy); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.NetworkingV1().NetworkPolicies(namespace).Update(ctx, &policy, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteNetworkPolicy(contextName, namespace, name string) error {
	log.Printf("Deleting network policy: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// HorizontalPodAutoscaler operations (namespaced)
func (c *Client) ListHPAs(namespace string) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListHPAsWithContext(ctx context.Context, namespace string) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

// ListHPAsForContext lists HPAs for a specific kubeconfig context
func (c *Client) ListHPAsForContext(contextName, namespace string) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetHPAYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	hpa, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	hpa.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(hpa)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateHPAYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var hpa autoscalingv2.HorizontalPodAutoscaler
	if err := yaml.Unmarshal([]byte(yamlContent), &hpa); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Update(ctx, &hpa, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteHPA(contextName, namespace, name string) error {
	log.Printf("Deleting HPA: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// PodDisruptionBudget operations (namespaced)
func (c *Client) ListPDBs(namespace string) ([]policyv1.PodDisruptionBudget, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListPDBsWithContext(ctx context.Context, namespace string) ([]policyv1.PodDisruptionBudget, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetPDBYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pdb, err := cs.PolicyV1().PodDisruptionBudgets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	pdb.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(pdb)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePDBYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var pdb policyv1.PodDisruptionBudget
	if err := yaml.Unmarshal([]byte(yamlContent), &pdb); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.PolicyV1().PodDisruptionBudgets(namespace).Update(ctx, &pdb, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePDB(contextName, namespace, name string) error {
	log.Printf("Deleting PDB: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.PolicyV1().PodDisruptionBudgets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ResourceQuota operations (namespaced)
func (c *Client) ListResourceQuotas(namespace string) ([]v1.ResourceQuota, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListResourceQuotasWithContext(ctx context.Context, namespace string) ([]v1.ResourceQuota, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetResourceQuotaYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	quota, err := cs.CoreV1().ResourceQuotas(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	quota.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(quota)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateResourceQuotaYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var quota v1.ResourceQuota
	if err := yaml.Unmarshal([]byte(yamlContent), &quota); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().ResourceQuotas(namespace).Update(ctx, &quota, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteResourceQuota(contextName, namespace, name string) error {
	log.Printf("Deleting resource quota: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().ResourceQuotas(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// LimitRange operations (namespaced)
func (c *Client) ListLimitRanges(namespace string) ([]v1.LimitRange, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.CoreV1().LimitRanges(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListLimitRangesWithContext(ctx context.Context, namespace string) ([]v1.LimitRange, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.CoreV1().LimitRanges(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetLimitRangeYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	lr, err := cs.CoreV1().LimitRanges(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	lr.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(lr)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateLimitRangeYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var lr v1.LimitRange
	if err := yaml.Unmarshal([]byte(yamlContent), &lr); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().LimitRanges(namespace).Update(ctx, &lr, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteLimitRange(contextName, namespace, name string) error {
	log.Printf("Deleting limit range: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().LimitRanges(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Endpoints operations (namespaced)
func (c *Client) ListEndpoints(namespace string) ([]v1.Endpoints, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.CoreV1().Endpoints(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListEndpointsWithContext(ctx context.Context, namespace string) ([]v1.Endpoints, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.CoreV1().Endpoints(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetEndpointsYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	ep, err := cs.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	ep.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(ep)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateEndpointsYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var ep v1.Endpoints
	if err := yaml.Unmarshal([]byte(yamlContent), &ep); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().Endpoints(namespace).Update(ctx, &ep, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteEndpoints(contextName, namespace, name string) error {
	log.Printf("Deleting endpoints: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Endpoints(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// EndpointSlice operations (namespaced, discovery.k8s.io/v1)
func (c *Client) ListEndpointSlices(namespace string) ([]discoveryv1.EndpointSlice, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	list, err := cs.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListEndpointSlicesWithContext(ctx context.Context, namespace string) ([]discoveryv1.EndpointSlice, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	list, err := cs.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetEndpointSliceYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	eps, err := cs.DiscoveryV1().EndpointSlices(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	eps.ManagedFields = nil
	yamlBytes, err := yaml.Marshal(eps)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateEndpointSliceYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var eps discoveryv1.EndpointSlice
	if err := yaml.Unmarshal([]byte(yamlContent), &eps); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.DiscoveryV1().EndpointSlices(namespace).Update(ctx, &eps, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteEndpointSlice(contextName, namespace, name string) error {
	log.Printf("Deleting endpointslice: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.DiscoveryV1().EndpointSlices(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ValidatingWebhookConfiguration operations (cluster-scoped)
