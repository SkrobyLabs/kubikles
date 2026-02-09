package main

import (
	"fmt"

	"kubikles/pkg/debug"
)

// =============================================================================
// Ingress Forwarding
// =============================================================================

// GetIngressForwardState returns the current ingress forward state
func (a *App) GetIngressForwardState() IngressForwardState {
	debug.LogPortforward("GetIngressForwardState called", nil)
	if a.ingressForwardManager == nil {
		return IngressForwardState{Active: false, Status: "stopped"}
	}
	return a.ingressForwardManager.GetState()
}

// DetectIngressController finds the ingress controller in the cluster
func (a *App) DetectIngressController() (*IngressController, error) {
	debug.LogPortforward("DetectIngressController called", nil)
	if a.ingressForwardManager == nil {
		return nil, fmt.Errorf("ingress forward manager not initialized")
	}
	return a.ingressForwardManager.DetectIngressController()
}

// CollectIngressHostnames collects all unique hostnames from ingresses
func (a *App) CollectIngressHostnames(namespaces []string) ([]string, error) {
	debug.LogPortforward("CollectIngressHostnames called", map[string]interface{}{"namespaces": namespaces})
	if a.ingressForwardManager == nil {
		return nil, fmt.Errorf("ingress forward manager not initialized")
	}
	return a.ingressForwardManager.CollectIngressHostnames(namespaces)
}

// StartIngressForward starts ingress forwarding with the given controller
func (a *App) StartIngressForward(controller IngressController, namespaces []string) error {
	debug.LogPortforward("StartIngressForward called", map[string]interface{}{"controllerNamespace": controller.Namespace, "controllerName": controller.Name, "namespaces": namespaces})
	if a.ingressForwardManager == nil {
		return fmt.Errorf("ingress forward manager not initialized")
	}
	return a.ingressForwardManager.Start(&controller, namespaces)
}

// StopIngressForward stops ingress forwarding and cleans up hosts file
func (a *App) StopIngressForward() error {
	debug.LogPortforward("StopIngressForward called", nil)
	if a.ingressForwardManager == nil {
		return fmt.Errorf("ingress forward manager not initialized")
	}
	return a.ingressForwardManager.Stop()
}

// RefreshIngressHostnames re-collects hostnames and updates the hosts file
func (a *App) RefreshIngressHostnames(namespaces []string) error {
	debug.LogPortforward("RefreshIngressHostnames called", map[string]interface{}{"namespaces": namespaces})
	if a.ingressForwardManager == nil {
		return fmt.Errorf("ingress forward manager not initialized")
	}
	return a.ingressForwardManager.RefreshHostnames(namespaces)
}

// GetManagedHosts returns the currently managed hosts file entries
func (a *App) GetManagedHosts() ([]string, error) {
	debug.LogPortforward("GetManagedHosts called", nil)
	if a.ingressForwardManager == nil {
		return nil, fmt.Errorf("ingress forward manager not initialized")
	}
	entries, err := a.ingressForwardManager.GetManagedHosts()
	if err != nil {
		return nil, err
	}
	hostnames := make([]string, len(entries))
	for i, e := range entries {
		hostnames[i] = e.Hostname
	}
	return hostnames, nil
}

// GetPodPorts returns the container ports for a pod
func (a *App) GetPodPorts(namespace, podName string) ([]int32, error) {
	currentContext := a.GetCurrentContext()
	debug.LogPortforward("GetPodPorts called", map[string]interface{}{"context": currentContext, "namespace": namespace, "pod": podName})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetPodContainerPorts(currentContext, namespace, podName)
}

// GetServicePorts returns the ports exposed by a service
func (a *App) GetServicePorts(namespace, serviceName string) ([]int32, error) {
	currentContext := a.GetCurrentContext()
	debug.LogPortforward("GetServicePorts called", map[string]interface{}{"context": currentContext, "namespace": namespace, "service": serviceName})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetServicePorts(currentContext, namespace, serviceName)
}
