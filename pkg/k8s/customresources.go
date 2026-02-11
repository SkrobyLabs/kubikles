package k8s

import (
	"fmt"
	"log"
	"path/filepath"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/yaml"
)

func (c *Client) getApiExtensionsClientForContext(contextName string) (*apiextensionsclientset.Clientset, error) {
	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}

	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := configLoader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config for context %s: %w", contextName, err)
	}

	return apiextensionsclientset.NewForConfig(config)
}

// CustomResourceDefinition operations (cluster-scoped)
func (c *Client) ListCRDs(contextName string) ([]apiextensionsv1.CustomResourceDefinition, error) {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get apiextensions client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	crds, err := cs.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return crds.Items, nil
}

func (c *Client) GetCRDYaml(contextName, name string) (string, error) {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get apiextensions client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	crd, err := cs.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	crd.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(crd)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateCRDYaml(contextName, name, yamlContent string) error {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get apiextensions client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal([]byte(yamlContent), &crd); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.ApiextensionsV1().CustomResourceDefinitions().Update(ctx, &crd, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteCRD(contextName, name string) error {
	log.Printf("Deleting CRD: context=%s, name=%s", contextName, name)
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get apiextensions client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, name, metav1.DeleteOptions{})
}

// PrinterColumn represents an additional printer column from a CRD
type PrinterColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`        // string, integer, number, boolean, date
	JSONPath    string `json:"jsonPath"`    // JSONPath expression to extract value
	Description string `json:"description"` // Optional description
	Priority    int32  `json:"priority"`    // 0 = always show, higher = hide in narrow views
}

// GetCRDPrinterColumns returns the additional printer columns for a CRD
func (c *Client) GetCRDPrinterColumns(contextName, crdName string) ([]PrinterColumn, error) {
	cs, err := c.getApiExtensionsClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get apiextensions client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	crd, err := cs.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get CRD %s: %w", crdName, err)
	}

	var columns []PrinterColumn

	// Find the served version's printer columns
	for _, version := range crd.Spec.Versions {
		if version.Served {
			for _, col := range version.AdditionalPrinterColumns {
				columns = append(columns, PrinterColumn{
					Name:        col.Name,
					Type:        col.Type,
					JSONPath:    col.JSONPath,
					Description: col.Description,
					Priority:    col.Priority,
				})
			}
			break // Use the first served version
		}
	}

	return columns, nil
}

// getDynamicClientForContext returns a dynamic client for a given context
func (c *Client) getDynamicClientForContext(contextName string) (dynamic.Interface, error) {
	// If context is empty, use the client's current context (not kubeconfig's default)
	if contextName == "" {
		c.mu.RLock()
		contextName = c.currentContext
		c.mu.RUnlock()
	}

	home := homedir.HomeDir()
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}

	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := configLoader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config for context %s: %w", contextName, err)
	}

	return dynamic.NewForConfig(config)
}

// CustomResourceInfo represents metadata about a custom resource instance
type CustomResourceInfo struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace,omitempty"`
	CreationTimestamp string `json:"creationTimestamp"`
	UID               string `json:"uid"`
}

// ListCustomResources lists instances of a custom resource
func (c *Client) ListCustomResources(contextName, group, version, resource, namespace string) ([]map[string]interface{}, error) {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get dynamic client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	var list *unstructured.UnstructuredList
	if namespace != "" {
		list, err = dc.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	} else {
		list, err = dc.Resource(gvr).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	// Convert to slice of maps for easier JSON serialization
	result := make([]map[string]interface{}, len(list.Items))
	for i, item := range list.Items {
		result[i] = item.Object
	}
	return result, nil
}

// GetCustomResourceYaml gets a custom resource instance as YAML
func (c *Client) GetCustomResourceYaml(contextName, group, version, resource, namespace, name string) (string, error) {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get dynamic client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	var obj *unstructured.Unstructured
	if namespace != "" {
		obj, err = dc.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	} else {
		obj, err = dc.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}
	if err != nil {
		return "", err
	}

	// Remove managed fields
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")

	yamlBytes, err := yaml.Marshal(obj.Object)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

// UpdateCustomResourceYaml updates a custom resource instance from YAML
func (c *Client) UpdateCustomResourceYaml(contextName, group, version, resource, namespace, name, yamlContent string) error {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get dynamic client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	var obj map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &obj); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	unstructuredObj := &unstructured.Unstructured{Object: obj}

	if namespace != "" {
		_, err = dc.Resource(gvr).Namespace(namespace).Update(ctx, unstructuredObj, metav1.UpdateOptions{})
	} else {
		_, err = dc.Resource(gvr).Update(ctx, unstructuredObj, metav1.UpdateOptions{})
	}
	return err
}

// DeleteCustomResource deletes a custom resource instance
func (c *Client) DeleteCustomResource(contextName, group, version, resource, namespace, name string) error {
	log.Printf("Deleting custom resource: context=%s, gvr=%s/%s/%s, ns=%s, name=%s", contextName, group, version, resource, namespace, name)
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get dynamic client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	if namespace != "" {
		return dc.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	}
	return dc.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
}

// GetCustomResourceEvents returns events related to a custom resource instance
func (c *Client) GetCustomResourceEvents(contextName, group, version, resource, namespace, name, kind string) ([]map[string]interface{}, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Use field selector to filter events by involvedObject
	fieldSelector := fmt.Sprintf("involvedObject.name=%s", name)
	if kind != "" {
		fieldSelector += fmt.Sprintf(",involvedObject.kind=%s", kind)
	}

	events, err := cs.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, err
	}

	// Build expected apiVersion for matching
	expectedAPIVersion := version
	if group != "" {
		expectedAPIVersion = group + "/" + version
	}

	// Filter by apiVersion match and convert to maps
	var result []map[string]interface{}
	for _, event := range events.Items {
		// Check if the involvedObject's apiVersion matches the CRD's group/version
		if event.InvolvedObject.APIVersion != expectedAPIVersion {
			continue
		}

		eventMap := map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":              event.Name,
				"namespace":         event.Namespace,
				"uid":               string(event.UID),
				"creationTimestamp": event.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
			},
			"type":           event.Type,
			"reason":         event.Reason,
			"message":        event.Message,
			"count":          event.Count,
			"firstTimestamp": event.FirstTimestamp.Format("2006-01-02T15:04:05Z"),
			"lastTimestamp":  event.LastTimestamp.Format("2006-01-02T15:04:05Z"),
			"involvedObject": map[string]interface{}{
				"kind":       event.InvolvedObject.Kind,
				"name":       event.InvolvedObject.Name,
				"namespace":  event.InvolvedObject.Namespace,
				"uid":        string(event.InvolvedObject.UID),
				"apiVersion": event.InvolvedObject.APIVersion,
			},
		}
		result = append(result, eventMap)
	}

	if result == nil {
		result = []map[string]interface{}{}
	}

	return result, nil
}

// --- Port Forwarding Support ---

// GetRestConfigForContext returns the REST config for a specific context
