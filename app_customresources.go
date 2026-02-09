package main

import (
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// =============================================================================
// Custom Resources
// =============================================================================

func (a *App) ListCRDs() ([]apiextensionsv1.CustomResourceDefinition, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ListCRDs called", map[string]interface{}{"context": currentContext})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCRDs(currentContext)
}

func (a *App) GetCRDYaml(name string) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetCRDYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCRDYaml(currentContext, name)
}

func (a *App) UpdateCRDYaml(name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("UpdateCRDYaml called", map[string]interface{}{"name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCRDYaml(currentContext, name, yamlContent)
}

func (a *App) DeleteCRD(name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteCRD called", map[string]interface{}{"context": currentContext, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCRD(currentContext, name)
}

// GetCRDPrinterColumns returns the additional printer columns for a CRD
func (a *App) GetCRDPrinterColumns(crdName string) ([]k8s.PrinterColumn, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetCRDPrinterColumns called", map[string]interface{}{"context": currentContext, "crdName": crdName})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCRDPrinterColumns(currentContext, crdName)
}

// Custom Resource instance operations (dynamic)
func (a *App) ListCustomResources(group, version, resource, namespace string) ([]map[string]interface{}, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("ListCustomResources called", map[string]interface{}{"context": currentContext, "group": group, "version": version, "resource": resource, "namespace": namespace})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCustomResources(currentContext, group, version, resource, namespace)
}

func (a *App) GetCustomResourceYaml(group, version, resource, namespace, name string) (string, error) {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("GetCustomResourceYaml called", map[string]interface{}{"group": group, "version": version, "resource": resource, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCustomResourceYaml(currentContext, group, version, resource, namespace, name)
}

func (a *App) UpdateCustomResourceYaml(group, version, resource, namespace, name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("UpdateCustomResourceYaml called", map[string]interface{}{"group": group, "version": version, "resource": resource, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCustomResourceYaml(currentContext, group, version, resource, namespace, name, yamlContent)
}

func (a *App) DeleteCustomResource(group, version, resource, namespace, name string) error {
	currentContext := a.GetCurrentContext()
	debug.LogK8s("DeleteCustomResource called", map[string]interface{}{"context": currentContext, "group": group, "version": version, "resource": resource, "namespace": namespace, "name": name})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCustomResource(currentContext, group, version, resource, namespace, name)
}
