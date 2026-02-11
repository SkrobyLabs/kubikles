package k8s

import (
	"context"
	"fmt"
	"log"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListServices(namespace string) ([]v1.Service, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	services, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return services.Items, nil
}

// ListServicesWithContext lists services with cancellation support
func (c *Client) ListServicesWithContext(ctx context.Context, namespace string) ([]v1.Service, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	services, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return services.Items, nil
}

// ListServicesForContext lists services for a specific kubeconfig context
func (c *Client) ListServicesForContext(contextName, namespace string) ([]v1.Service, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	services, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return services.Items, nil
}

func (c *Client) GetServiceYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	service, err := cs.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	service.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(service)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateServiceYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var service v1.Service
	if err := yaml.Unmarshal([]byte(yamlContent), &service); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().Services(namespace).Update(ctx, &service, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteService(contextName, namespace, name string) error {
	log.Printf("Deleting service: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Ingress operations

func (c *Client) GetServiceBackingPods(contextName, namespace, serviceName string) ([]string, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Get the service
	svc, err := cs.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s: %w", serviceName, err)
	}

	// Build label selector from service selector
	if len(svc.Spec.Selector) == 0 {
		return nil, fmt.Errorf("service %s has no selector", serviceName)
	}

	selectorParts := make([]string, 0, len(svc.Spec.Selector))
	for k, v := range svc.Spec.Selector {
		selectorParts = append(selectorParts, k+"="+v)
	}
	selector := strings.Join(selectorParts, ",")

	// Find pods matching selector
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Return names of running pods
	result := make([]string, 0, len(pods.Items))
	for _, pod := range pods.Items {
		if pod.Status.Phase == v1.PodRunning {
			result = append(result, pod.Name)
		}
	}

	return result, nil
}

// GetPodContainerPorts returns the container ports for a pod
func (c *Client) GetPodContainerPorts(contextName, namespace, podName string) ([]int32, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	pod, err := cs.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s: %w", podName, err)
	}

	// Calculate total ports for pre-allocation
	totalPorts := 0
	for _, container := range pod.Spec.Containers {
		totalPorts += len(container.Ports)
	}
	ports := make([]int32, 0, totalPorts)
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			ports = append(ports, port.ContainerPort)
		}
	}

	return ports, nil
}

// GetServicePorts returns the ports exposed by a service
func (c *Client) GetServicePorts(contextName, namespace, serviceName string) ([]int32, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	svc, err := cs.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s: %w", serviceName, err)
	}

	ports := make([]int32, 0, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		ports = append(ports, port.Port)
	}

	return ports, nil
}

// ==================== RBAC / Access Control ====================

// ServiceAccount operations
