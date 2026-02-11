package k8s

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

var kindToResource = map[string]string{
	"Pod":                            "pods",
	"Deployment":                     "deployments",
	"StatefulSet":                    "statefulsets",
	"DaemonSet":                      "daemonsets",
	"ReplicaSet":                     "replicasets",
	"Job":                            "jobs",
	"CronJob":                        "cronjobs",
	"Service":                        "services",
	"Ingress":                        "ingresses",
	"ConfigMap":                      "configmaps",
	"Secret":                         "secrets",
	"PersistentVolumeClaim":          "persistentvolumeclaims",
	"PersistentVolume":               "persistentvolumes",
	"StorageClass":                   "storageclasses",
	"ServiceAccount":                 "serviceaccounts",
	"Role":                           "roles",
	"ClusterRole":                    "clusterroles",
	"RoleBinding":                    "rolebindings",
	"ClusterRoleBinding":             "clusterrolebindings",
	"NetworkPolicy":                  "networkpolicies",
	"Namespace":                      "namespaces",
	"Node":                           "nodes",
	"Endpoints":                      "endpoints",
	"EndpointSlice":                  "endpointslices",
	"HorizontalPodAutoscaler":        "horizontalpodautoscalers",
	"PodDisruptionBudget":            "poddisruptionbudgets",
	"ResourceQuota":                  "resourcequotas",
	"LimitRange":                     "limitranges",
	"ValidatingWebhookConfiguration": "validatingwebhookconfigurations",
	"MutatingWebhookConfiguration":   "mutatingwebhookconfigurations",
	"PriorityClass":                  "priorityclasses",
	"Lease":                          "leases",
	"CSIDriver":                      "csidrivers",
	"CSINode":                        "csinodes",
	"IngressClass":                   "ingressclasses",
}

// ApplyYAML creates a resource from YAML content using the dynamic client
func (c *Client) ApplyYAML(contextName, yamlContent string) error {
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get dynamic client: %w", err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Parse YAML into unstructured object
	var obj map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &obj); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	unstructuredObj := &unstructured.Unstructured{Object: obj}

	// Extract apiVersion and kind
	apiVersion := unstructuredObj.GetAPIVersion()
	kind := unstructuredObj.GetKind()
	namespace := unstructuredObj.GetNamespace()

	if apiVersion == "" || kind == "" {
		return fmt.Errorf("YAML must contain apiVersion and kind")
	}

	// Parse apiVersion into group and version
	var group, version string
	if g, v, found := strings.Cut(apiVersion, "/"); found {
		group = g
		version = v
	} else {
		group = ""
		version = apiVersion
	}

	// Get resource name (plural form)
	resource, ok := kindToResource[kind]
	if !ok {
		// Fallback: lowercase the kind and add 's'
		resource = strings.ToLower(kind) + "s"
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	// Create the resource
	if namespace != "" {
		_, err = dc.Resource(gvr).Namespace(namespace).Create(ctx, unstructuredObj, metav1.CreateOptions{})
	} else {
		_, err = dc.Resource(gvr).Create(ctx, unstructuredObj, metav1.CreateOptions{})
	}

	if err != nil {
		return fmt.Errorf("failed to create %s: %w", kind, err)
	}

	return nil
}

// ============================================================================
// Prometheus Integration
// ============================================================================

// PrometheusInfo contains detected Prometheus endpoint information
