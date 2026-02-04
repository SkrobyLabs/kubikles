package main

import (
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"kubikles/pkg/k8s"
)

// =============================================================================
// Custom Resources
// =============================================================================

func (a *App) ListCRDs() ([]apiextensionsv1.CustomResourceDefinition, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ListCRDs called: context=%s", currentContext)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCRDs(currentContext)
}

func (a *App) GetCRDYaml(name string) (string, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetCRDYaml called: name=%s", name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCRDYaml(currentContext, name)
}

func (a *App) UpdateCRDYaml(name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("UpdateCRDYaml called: name=%s", name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCRDYaml(currentContext, name, yamlContent)
}

func (a *App) DeleteCRD(name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteCRD called: context=%s, name=%s", currentContext, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCRD(currentContext, name)
}

// GetCRDPrinterColumns returns the additional printer columns for a CRD
func (a *App) GetCRDPrinterColumns(crdName string) ([]k8s.PrinterColumn, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetCRDPrinterColumns called: context=%s, crdName=%s", currentContext, crdName)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCRDPrinterColumns(currentContext, crdName)
}

// Custom Resource instance operations (dynamic)
func (a *App) ListCustomResources(group, version, resource, namespace string) ([]map[string]interface{}, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("ListCustomResources called: context=%s, gvr=%s/%s/%s, ns=%s", currentContext, group, version, resource, namespace)
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.ListCustomResources(currentContext, group, version, resource, namespace)
}

func (a *App) GetCustomResourceYaml(group, version, resource, namespace, name string) (string, error) {
	currentContext := a.GetCurrentContext()
	a.logDebug("GetCustomResourceYaml called: gvr=%s/%s/%s, ns=%s, name=%s", group, version, resource, namespace, name)
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetCustomResourceYaml(currentContext, group, version, resource, namespace, name)
}

func (a *App) UpdateCustomResourceYaml(group, version, resource, namespace, name, yamlContent string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("UpdateCustomResourceYaml called: gvr=%s/%s/%s, ns=%s, name=%s", group, version, resource, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.UpdateCustomResourceYaml(currentContext, group, version, resource, namespace, name, yamlContent)
}

func (a *App) DeleteCustomResource(group, version, resource, namespace, name string) error {
	currentContext := a.GetCurrentContext()
	a.logDebug("DeleteCustomResource called: context=%s, gvr=%s/%s/%s, ns=%s, name=%s", currentContext, group, version, resource, namespace, name)
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteCustomResource(currentContext, group, version, resource, namespace, name)
}
