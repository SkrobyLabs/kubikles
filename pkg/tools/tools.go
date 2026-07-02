// Package tools implements K8s tool definitions and execution logic.
// It is consumed by the MCP server (pkg/mcp) for JSON-RPC dispatch,
// and can be used directly by future API-based AI providers.
package tools

import (
	"fmt"

	"kubikles/pkg/k8s"
)

// ToolDef describes a single tool's name, description, and JSON Schema input.
type ToolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// AllToolDefs returns the full set of K8s tool definitions.
func AllToolDefs() []ToolDef {
	return []ToolDef{
		{
			Name:        "get_pod_logs",
			Description: "Get recent logs from a Kubernetes pod",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"namespace":  map[string]interface{}{"type": "string", "description": "Pod namespace"},
					"pod":        map[string]interface{}{"type": "string", "description": "Pod name"},
					"container":  map[string]interface{}{"type": "string", "description": "Container name (optional, defaults to first container)"},
					"tail_lines": map[string]interface{}{"type": "integer", "description": "Number of lines to return (default 100)"},
					"previous":   map[string]interface{}{"type": "boolean", "description": "Return logs from previous terminated container instance (default false)"},
				},
				"required": []string{"namespace", "pod"},
			},
		},
		{
			Name:        "get_resource_yaml",
			Description: "Get the YAML manifest of a Kubernetes resource. For custom resources (CRDs), provide group, version, and resource params.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"kind":      map[string]interface{}{"type": "string", "description": "Resource kind (e.g. Pod, Deployment, Service, ConfigMap, Secret, Node, Namespace, or a CRD kind)"},
					"name":      map[string]interface{}{"type": "string", "description": "Resource name"},
					"namespace": map[string]interface{}{"type": "string", "description": "Namespace (required for namespaced resources)"},
					"group":     map[string]interface{}{"type": "string", "description": "API group for custom resources (e.g. traefik.io)"},
					"version":   map[string]interface{}{"type": "string", "description": "API version for custom resources (e.g. v1alpha1)"},
					"resource":  map[string]interface{}{"type": "string", "description": "Plural resource name for custom resources (e.g. ingressroutes)"},
				},
				"required": []string{"kind", "name"},
			},
		},
		{
			Name:        "list_resources",
			Description: "List Kubernetes resources of a given kind, returning a concise summary table. For custom resources (CRDs), provide group, version, and resource params.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"kind":      map[string]interface{}{"type": "string", "description": "Resource kind. Built-in kinds are supported natively (e.g. Pod, Deployment, StatefulSet, Service, Node, Namespace, ConfigMap, Secret, Ingress, NetworkPolicy, Role, ClusterRole, RoleBinding, ClusterRoleBinding, StorageClass, IngressClass, PDB, HPA, PVC, PV); pass group/version/resource only for CRDs"},
					"namespace": map[string]interface{}{"type": "string", "description": "Namespace to list from (optional; omit for cluster-scoped resources or all namespaces)"},
					"group":     map[string]interface{}{"type": "string", "description": "API group for custom resources (e.g. traefik.io)"},
					"version":   map[string]interface{}{"type": "string", "description": "API version for custom resources (e.g. v1alpha1)"},
					"resource":  map[string]interface{}{"type": "string", "description": "Plural resource name for custom resources (e.g. ingressroutes)"},
				},
				"required": []string{"kind"},
			},
		},
		{
			Name:        "get_events",
			Description: "Get recent Kubernetes events for a namespace",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"namespace":       map[string]interface{}{"type": "string", "description": "Namespace to get events from (empty for all namespaces)"},
					"involved_object": map[string]interface{}{"type": "string", "description": "Filter by involved object in 'Kind/Name' format (e.g. 'Pod/my-pod')"},
				},
			},
		},
		{
			Name:        "describe_resource",
			Description: "Get a concise description of a Kubernetes resource including status, conditions, labels, and key details. For custom resources (CRDs), provide group, version, and resource params.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"kind":      map[string]interface{}{"type": "string", "description": "Resource kind (e.g. Pod, Deployment, Service, or a CRD kind)"},
					"name":      map[string]interface{}{"type": "string", "description": "Resource name"},
					"namespace": map[string]interface{}{"type": "string", "description": "Namespace (required for namespaced resources)"},
					"group":     map[string]interface{}{"type": "string", "description": "API group for custom resources (e.g. traefik.io)"},
					"version":   map[string]interface{}{"type": "string", "description": "API version for custom resources (e.g. v1alpha1)"},
					"resource":  map[string]interface{}{"type": "string", "description": "Plural resource name for custom resources (e.g. ingressroutes)"},
				},
				"required": []string{"kind", "name"},
			},
		},
		{
			Name:        "list_crds",
			Description: "List all CustomResourceDefinitions in the cluster. Returns CRD names, groups, versions, kinds, scope, and plural names needed for querying instances.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "list_custom_resources",
			Description: "List instances of a custom resource (CRD). Use list_crds first to discover available CRDs and their group/version/resource values.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"group":     map[string]interface{}{"type": "string", "description": "API group (e.g. traefik.io, cert-manager.io)"},
					"version":   map[string]interface{}{"type": "string", "description": "API version (e.g. v1alpha1, v1)"},
					"resource":  map[string]interface{}{"type": "string", "description": "Plural resource name (e.g. ingressroutes, certificates)"},
					"namespace": map[string]interface{}{"type": "string", "description": "Namespace (optional; omit for cluster-scoped resources)"},
				},
				"required": []string{"group", "version", "resource"},
			},
		},
		{
			Name:        "get_custom_resource_yaml",
			Description: "Get the YAML manifest of a specific custom resource instance",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"group":     map[string]interface{}{"type": "string", "description": "API group"},
					"version":   map[string]interface{}{"type": "string", "description": "API version"},
					"resource":  map[string]interface{}{"type": "string", "description": "Plural resource name"},
					"name":      map[string]interface{}{"type": "string", "description": "Resource instance name"},
					"namespace": map[string]interface{}{"type": "string", "description": "Namespace (optional; omit for cluster-scoped)"},
				},
				"required": []string{"group", "version", "resource", "name"},
			},
		},
		{
			Name:        "get_cluster_metrics",
			Description: "Get cluster-wide CPU, memory, and pod usage summary with a per-node breakdown table",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "get_pod_metrics",
			Description: "Get per-pod CPU and memory usage. Filterable by namespace, sortable by cpu, memory, or name.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"namespace": map[string]interface{}{"type": "string", "description": "Filter by namespace (optional; omit for all namespaces)"},
					"sort_by":   map[string]interface{}{"type": "string", "description": "Sort field: cpu, memory, or name (default: cpu)"},
				},
			},
		},
		{
			Name:        "get_namespace_summary",
			Description: "Get resource counts for a namespace (pods, deployments, services, etc.)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"namespace": map[string]interface{}{"type": "string", "description": "Namespace to summarize"},
				},
				"required": []string{"namespace"},
			},
		},
		{
			Name:        "get_resource_dependencies",
			Description: "Get the dependency graph for a resource showing what it owns, selects, and uses",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"kind":      map[string]interface{}{"type": "string", "description": "Resource kind (e.g. Deployment, Service, StatefulSet)"},
					"name":      map[string]interface{}{"type": "string", "description": "Resource name"},
					"namespace": map[string]interface{}{"type": "string", "description": "Namespace (optional for cluster-scoped resources)"},
				},
				"required": []string{"kind", "name"},
			},
		},
		// Diagnostic tools
		{
			Name:        "get_flow_timeline",
			Description: "Get an event timeline showing the lifecycle of a Kubernetes resource including creation, updates, scaling events, and related events",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"resource_type": map[string]interface{}{"type": "string", "description": "Resource type (pod, deployment, statefulset, daemonset, job, service, configmap, secret)"},
					"namespace":     map[string]interface{}{"type": "string", "description": "Namespace of the resource"},
					"name":          map[string]interface{}{"type": "string", "description": "Name of the resource"},
				},
				"required": []string{"resource_type", "namespace", "name"},
			},
		},
		{
			Name:        "get_multi_pod_logs",
			Description: "Get aggregated logs from multiple pods matching a label selector, useful for viewing logs across replicas of a deployment or service",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"namespace":      map[string]interface{}{"type": "string", "description": "Namespace to search for pods"},
					"label_selector": map[string]interface{}{"type": "string", "description": "Label selector to match pods (e.g. 'app=nginx,tier=frontend')"},
					"container":      map[string]interface{}{"type": "string", "description": "Container name (optional, defaults to all containers)"},
					"tail_lines":     map[string]interface{}{"type": "integer", "description": "Number of lines to return per pod (default 50)"},
					"since_seconds":  map[string]interface{}{"type": "integer", "description": "Only return logs newer than this many seconds (default 300)"},
				},
				"required": []string{"namespace", "label_selector"},
			},
		},
		{
			Name:        "diff_resources",
			Description: "Compare two Kubernetes resources and show the differences. Can compare resources across different contexts/clusters.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source_context":   map[string]interface{}{"type": "string", "description": "Source cluster context (optional, defaults to current)"},
					"source_namespace": map[string]interface{}{"type": "string", "description": "Source resource namespace"},
					"source_kind":      map[string]interface{}{"type": "string", "description": "Source resource kind (e.g. Deployment, ConfigMap)"},
					"source_name":      map[string]interface{}{"type": "string", "description": "Source resource name"},
					"target_context":   map[string]interface{}{"type": "string", "description": "Target cluster context (optional, defaults to current)"},
					"target_namespace": map[string]interface{}{"type": "string", "description": "Target resource namespace"},
					"target_kind":      map[string]interface{}{"type": "string", "description": "Target resource kind"},
					"target_name":      map[string]interface{}{"type": "string", "description": "Target resource name"},
				},
				"required": []string{"source_namespace", "source_kind", "source_name", "target_namespace", "target_kind", "target_name"},
			},
		},
		{
			Name:        "check_rbac_access",
			Description: "Check what RBAC permissions a subject (user, group, or service account) has. Returns allowed actions with the rules that grant them.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"subject_kind":      map[string]interface{}{"type": "string", "description": "Subject type: User, Group, or ServiceAccount"},
					"subject_name":      map[string]interface{}{"type": "string", "description": "Name of the subject"},
					"subject_namespace": map[string]interface{}{"type": "string", "description": "Namespace (required for ServiceAccount)"},
					"target_namespace":  map[string]interface{}{"type": "string", "description": "Namespace to check permissions in (optional, empty for cluster-wide)"},
					"resource":          map[string]interface{}{"type": "string", "description": "Filter by specific resource type (e.g. pods, deployments)"},
					"verb":              map[string]interface{}{"type": "string", "description": "Filter by specific verb (e.g. get, list, create, delete)"},
				},
				"required": []string{"subject_kind", "subject_name"},
			},
		},
		{
			Name:        "run_command",
			Description: "Execute a shell command. Only commands matching the user-configured allowlist of safe prefixes (e.g. kubectl get, helm list) will run. Commands are executed directly without shell interpretation. Disallowed commands are rejected with an error.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{"type": "string", "description": "The full command to run (e.g. 'kubectl get pods -n default'). Must match an allowed command prefix."},
				},
				"required": []string{"command"},
			},
		},
	}
}

// CallTool dispatches a tool call to the appropriate implementation.
// Returns the text result and whether it's an error.
func CallTool(client *k8s.Client, name string, args map[string]interface{}) (string, bool) {
	switch name {
	case "get_pod_logs":
		return toolGetPodLogs(client, args)
	case "get_resource_yaml":
		return toolGetResourceYaml(client, args)
	case "list_resources":
		return toolListResources(client, args)
	case "get_events":
		return toolGetEvents(client, args)
	case "describe_resource":
		return toolDescribeResource(client, args)
	case "list_crds":
		return toolListCRDs(client, args)
	case "list_custom_resources":
		return toolListCustomResources(client, args)
	case "get_custom_resource_yaml":
		return toolGetCustomResourceYaml(client, args)
	case "get_cluster_metrics":
		return toolGetClusterMetrics(client, args)
	case "get_pod_metrics":
		return toolGetPodMetrics(client, args)
	case "get_namespace_summary":
		return toolGetNamespaceSummary(client, args)
	case "get_resource_dependencies":
		return toolGetResourceDependencies(client, args)
	case "get_flow_timeline":
		return toolGetFlowTimeline(client, args)
	case "get_multi_pod_logs":
		return toolGetMultiPodLogs(client, args)
	case "diff_resources":
		return toolDiffResources(client, args)
	case "check_rbac_access":
		return toolCheckRBACAccess(client, args)
	case "run_command":
		prefixes := AllowedCommandPrefixes
		if prefixes == nil {
			prefixes = []string{} // no fallback — empty means nothing allowed
		}
		return toolRunCommand(args, prefixes)
	default:
		return fmt.Sprintf("Unknown tool: %s", name), true
	}
}

// --- Arg helpers ---

func strArg(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func boolArg(args map[string]interface{}, key string, defaultVal bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultVal
}

func intArg(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... [truncated]"
}
