// Package tools implements K8s tool definitions and execution logic.
// It is consumed by the MCP server (pkg/mcp) for JSON-RPC dispatch,
// and can be used directly by future API-based AI providers.
package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"kubikles/pkg/k8s"

	v1 "k8s.io/api/core/v1"
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
					"kind":      map[string]interface{}{"type": "string", "description": "Resource kind (e.g. Pod, Deployment, Service, Node, Namespace, or a CRD kind)"},
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

// --- Tool implementations ---

func toolGetPodLogs(client *k8s.Client, args map[string]interface{}) (string, bool) {
	ns := strArg(args, "namespace")
	pod := strArg(args, "pod")
	container := strArg(args, "container")
	if ns == "" || pod == "" {
		return "namespace and pod are required", true
	}

	logs, err := client.GetPodLogs(ns, pod, container, false, false, "")
	if err != nil {
		return fmt.Sprintf("Error getting logs: %v", err), true
	}

	if logs == "" {
		return "(no logs)", false
	}

	tailLines := intArg(args, "tail_lines", 100)
	lines := strings.Split(logs, "\n")
	if len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}

	return truncate(strings.Join(lines, "\n"), MaxPodLogChars), false
}

func toolGetResourceYaml(client *k8s.Client, args map[string]interface{}) (string, bool) {
	kind := strings.ToLower(strArg(args, "kind"))
	name := strArg(args, "name")
	ns := strArg(args, "namespace")

	if kind == "" || name == "" {
		return "kind and name are required", true
	}

	var yamlStr string
	var err error

	switch kind {
	case "pod", "pods":
		yamlStr, err = client.GetPodYaml(ns, name)
	case "deployment", "deployments":
		yamlStr, err = client.GetDeploymentYaml(ns, name)
	case "statefulset", "statefulsets":
		yamlStr, err = client.GetStatefulSetYaml(ns, name)
	case "daemonset", "daemonsets":
		yamlStr, err = client.GetDaemonSetYaml(ns, name)
	case "replicaset", "replicasets":
		yamlStr, err = client.GetReplicaSetYaml(ns, name)
	case "job", "jobs":
		yamlStr, err = client.GetJobYaml(ns, name)
	case "cronjob", "cronjobs":
		yamlStr, err = client.GetCronJobYaml(ns, name)
	case "service", "services":
		yamlStr, err = client.GetServiceYaml(ns, name)
	case "ingress", "ingresses":
		yamlStr, err = client.GetIngressYaml(ns, name)
	case "ingressclass", "ingressclasses":
		yamlStr, err = client.GetIngressClassYaml(name)
	case "configmap", "configmaps":
		yamlStr, err = client.GetConfigMapYaml(ns, name)
	case "secret", "secrets":
		yamlStr, err = client.GetSecretYaml(ns, name)
		if err == nil {
			yamlStr = redactSecretYaml(yamlStr)
		}
	case "node", "nodes":
		yamlStr, err = client.GetNodeYaml(name)
	case "persistentvolumeclaim", "pvc", "pvcs":
		yamlStr, err = client.GetPVCYaml(ns, name)
	case "persistentvolume", "pv", "pvs":
		yamlStr, err = client.GetPVYaml(name)
	case "storageclass", "storageclasses":
		yamlStr, err = client.GetStorageClassYaml(name)
	case "serviceaccount", "serviceaccounts":
		yamlStr, err = client.GetServiceAccountYaml(ns, name)
	case "role", "roles":
		yamlStr, err = client.GetRoleYaml(ns, name)
	case "clusterrole", "clusterroles":
		yamlStr, err = client.GetClusterRoleYaml(name)
	case "rolebinding", "rolebindings":
		yamlStr, err = client.GetRoleBindingYaml(ns, name)
	case "clusterrolebinding", "clusterrolebindings":
		yamlStr, err = client.GetClusterRoleBindingYaml(name)
	case "networkpolicy", "networkpolicies":
		yamlStr, err = client.GetNetworkPolicyYaml(ns, name)
	case "hpa", "horizontalpodautoscaler", "horizontalpodautoscalers":
		yamlStr, err = client.GetHPAYaml(ns, name)
	case "pdb", "poddisruptionbudget", "poddisruptionbudgets":
		yamlStr, err = client.GetPDBYaml(ns, name)
	case "event", "events":
		yamlStr, err = client.GetEventYAML(ns, name)
	default:
		group := strArg(args, "group")
		version := strArg(args, "version")
		resource := strArg(args, "resource")
		if group != "" && version != "" && resource != "" {
			ctx := client.GetCurrentContext()
			yamlStr, err = client.GetCustomResourceYaml(ctx, group, version, resource, ns, name)
		} else {
			return fmt.Sprintf("Unsupported resource kind: %s. For custom resources, provide group, version, and resource params.", strArg(args, "kind")), true
		}
	}

	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}
	return truncate(yamlStr, MaxYAMLChars), false
}

func toolListResources(client *k8s.Client, args map[string]interface{}) (string, bool) {
	kind := strings.ToLower(strArg(args, "kind"))
	ns := strArg(args, "namespace")

	if kind == "" {
		return "kind is required", true
	}

	switch kind {
	case "pod", "pods":
		items, err := client.ListPods(ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-10s %-10s", "NAME", "NAMESPACE", "STATUS", "AGE"))
		for _, p := range items {
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-10s %-10s", p.Name, p.Namespace, string(p.Status.Phase), age(p.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "deployment", "deployments":
		items, err := client.ListDeployments(ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-12s %-10s", "NAME", "NAMESPACE", "READY", "AGE"))
		for _, d := range items {
			replicas := int32(1)
			if d.Spec.Replicas != nil {
				replicas = *d.Spec.Replicas
			}
			ready := fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, replicas)
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-12s %-10s", d.Name, d.Namespace, ready, age(d.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "service", "services":
		items, err := client.ListServices(ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-15s %-20s %-10s", "NAME", "NAMESPACE", "TYPE", "CLUSTER-IP", "AGE"))
		for _, s := range items {
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-15s %-20s %-10s", s.Name, s.Namespace, string(s.Spec.Type), s.Spec.ClusterIP, age(s.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "node", "nodes":
		items, err := client.ListNodes()
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-10s %-10s", "NAME", "STATUS", "AGE"))
		for _, n := range items {
			status := "NotReady"
			for _, c := range n.Status.Conditions {
				if c.Type == "Ready" && c.Status == "True" {
					status = "Ready"
				}
			}
			lines = append(lines, fmt.Sprintf("%-50s %-10s %-10s", n.Name, status, age(n.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "namespace", "namespaces":
		items, err := client.ListNamespaces()
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-10s %-10s", "NAME", "STATUS", "AGE"))
		for _, n := range items {
			lines = append(lines, fmt.Sprintf("%-50s %-10s %-10s", n.Name, string(n.Status.Phase), age(n.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "configmap", "configmaps":
		items, err := client.ListConfigMaps(ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-10s %-10s", "NAME", "NAMESPACE", "DATA", "AGE"))
		for _, cm := range items {
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-10d %-10s", cm.Name, cm.Namespace, len(cm.Data), age(cm.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "secret", "secrets":
		items, err := client.ListSecrets(ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-25s %-10s %-10s", "NAME", "NAMESPACE", "TYPE", "DATA", "AGE"))
		for _, s := range items {
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-25s %-10d %-10s", s.Name, s.Namespace, string(s.Type), len(s.Data), age(s.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "statefulset", "statefulsets":
		items, err := client.ListStatefulSets("", ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-12s %-10s", "NAME", "NAMESPACE", "READY", "AGE"))
		for _, s := range items {
			replicas := int32(0)
			if s.Spec.Replicas != nil {
				replicas = *s.Spec.Replicas
			}
			ready := fmt.Sprintf("%d/%d", s.Status.ReadyReplicas, replicas)
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-12s %-10s", s.Name, s.Namespace, ready, age(s.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "daemonset", "daemonsets":
		items, err := client.ListDaemonSets("", ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-12s %-10s", "NAME", "NAMESPACE", "READY", "AGE"))
		for _, d := range items {
			ready := fmt.Sprintf("%d/%d", d.Status.NumberReady, d.Status.DesiredNumberScheduled)
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-12s %-10s", d.Name, d.Namespace, ready, age(d.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "replicaset", "replicasets":
		items, err := client.ListReplicaSets("", ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-12s %-10s", "NAME", "NAMESPACE", "READY", "AGE"))
		for _, r := range items {
			replicas := int32(0)
			if r.Spec.Replicas != nil {
				replicas = *r.Spec.Replicas
			}
			ready := fmt.Sprintf("%d/%d", r.Status.ReadyReplicas, replicas)
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-12s %-10s", r.Name, r.Namespace, ready, age(r.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "job", "jobs":
		items, err := client.ListJobs("", ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-12s %-10s", "NAME", "NAMESPACE", "STATUS", "AGE"))
		for _, j := range items {
			status := "Running"
			if j.Status.Succeeded > 0 {
				status = "Complete"
			} else if j.Status.Failed > 0 {
				status = "Failed"
			}
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-12s %-10s", j.Name, j.Namespace, status, age(j.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "cronjob", "cronjobs":
		items, err := client.ListCronJobs("", ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-20s %-10s", "NAME", "NAMESPACE", "SCHEDULE", "AGE"))
		for _, cj := range items {
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-20s %-10s", cj.Name, cj.Namespace, cj.Spec.Schedule, age(cj.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "ingress", "ingresses":
		items, err := client.ListIngresses(ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-30s %-10s", "NAME", "NAMESPACE", "HOSTS", "AGE"))
		for _, ing := range items {
			var hosts []string
			for _, r := range ing.Spec.Rules {
				if r.Host != "" {
					hosts = append(hosts, r.Host)
				}
			}
			hostStr := strings.Join(hosts, ",")
			if hostStr == "" {
				hostStr = "*"
			}
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-30s %-10s", ing.Name, ing.Namespace, hostStr, age(ing.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "persistentvolumeclaim", "pvc", "pvcs":
		items, err := client.ListPVCs("", ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-10s %-15s %-10s", "NAME", "NAMESPACE", "STATUS", "VOLUME", "AGE"))
		for _, pvc := range items {
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-10s %-15s %-10s", pvc.Name, pvc.Namespace, string(pvc.Status.Phase), pvc.Spec.VolumeName, age(pvc.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "persistentvolume", "pv", "pvs":
		items, err := client.ListPVs("")
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-10s %-15s %-10s", "NAME", "STATUS", "CAPACITY", "AGE"))
		for _, pv := range items {
			cap := ""
			if storage, ok := pv.Spec.Capacity["storage"]; ok {
				cap = storage.String()
			}
			lines = append(lines, fmt.Sprintf("%-50s %-10s %-15s %-10s", pv.Name, string(pv.Status.Phase), cap, age(pv.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "hpa", "horizontalpodautoscaler", "horizontalpodautoscalers":
		items, err := client.ListHPAs(ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-20s %-10s", "NAME", "NAMESPACE", "REFERENCE", "AGE"))
		for _, h := range items {
			ref := fmt.Sprintf("%s/%s", h.Spec.ScaleTargetRef.Kind, h.Spec.ScaleTargetRef.Name)
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-20s %-10s", h.Name, h.Namespace, ref, age(h.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "serviceaccount", "serviceaccounts":
		items, err := client.ListServiceAccounts(ns)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-10s", "NAME", "NAMESPACE", "AGE"))
		for _, sa := range items {
			lines = append(lines, fmt.Sprintf("%-50s %-20s %-10s", sa.Name, sa.Namespace, age(sa.CreationTimestamp.Time)))
		}
		return strings.Join(lines, "\n"), false

	case "event", "events":
		return toolGetEvents(client, args)

	default:
		group := strArg(args, "group")
		version := strArg(args, "version")
		resource := strArg(args, "resource")
		if group != "" && version != "" && resource != "" {
			return toolListCustomResources(client, map[string]interface{}{
				"group":     group,
				"version":   version,
				"resource":  resource,
				"namespace": ns,
			})
		}
		return fmt.Sprintf("Unsupported resource kind for listing: %s. For custom resources, provide group, version, and resource params.", strArg(args, "kind")), true
	}
}

func toolGetEvents(client *k8s.Client, args map[string]interface{}) (string, bool) {
	ns := strArg(args, "namespace")
	involvedObj := strArg(args, "involved_object")

	events, err := client.ListEvents(ns)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}

	if involvedObj != "" {
		parts := strings.SplitN(involvedObj, "/", 2)
		if len(parts) == 2 {
			filterKind := strings.ToLower(parts[0])
			filterName := parts[1]
			var filtered []v1.Event
			for _, e := range events {
				if strings.ToLower(e.InvolvedObject.Kind) == filterKind &&
					e.InvolvedObject.Name == filterName {
					filtered = append(filtered, e)
				}
			}
			events = filtered
		}
	}

	if len(events) == 0 {
		return "No events found", false
	}

	sort.Slice(events, func(i, j int) bool {
		return eventTime(events[i]).After(eventTime(events[j]))
	})

	limit := 30
	if len(events) < limit {
		limit = len(events)
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%-8s %-10s %-15s %-25s %s", "AGE", "TYPE", "REASON", "OBJECT", "MESSAGE"))
	for _, e := range events[:limit] {
		obj := fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name)
		lines = append(lines, fmt.Sprintf("%-8s %-10s %-15s %-25s %s", age(eventTime(e)), e.Type, e.Reason, obj, e.Message))
	}
	return strings.Join(lines, "\n"), false
}

func toolDescribeResource(client *k8s.Client, args map[string]interface{}) (string, bool) {
	kind := strings.ToLower(strArg(args, "kind"))
	name := strArg(args, "name")
	ns := strArg(args, "namespace")

	if kind == "" || name == "" {
		return "kind and name are required", true
	}

	resourceType := NormalizeKind(kind)

	yamlResult, isErr := toolGetResourceYaml(client, args)
	if isErr {
		return yamlResult, true
	}

	eventsResult, _ := toolGetEvents(client, map[string]interface{}{
		"namespace":       ns,
		"involved_object": fmt.Sprintf("%s/%s", capitalize(resourceType), name),
	})

	graph, graphErr := client.GetResourceDependencies("", resourceType, ns, name)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== %s: %s", capitalize(resourceType), name))
	if ns != "" {
		sb.WriteString(fmt.Sprintf(" (namespace: %s)", ns))
	}
	sb.WriteString(" ===\n\n")

	sb.WriteString("--- Manifest ---\n")
	sb.WriteString(truncate(yamlResult, 4000))
	sb.WriteString("\n\n")

	if graphErr == nil && graph != nil && len(graph.Nodes) > 0 {
		sb.WriteString("--- Related Resources ---\n")
		for _, edge := range graph.Edges {
			for _, node := range graph.Nodes {
				if node.ID == edge.Target {
					status := ""
					if node.Status != "" {
						status = fmt.Sprintf(" [%s]", node.Status)
					}
					sb.WriteString(fmt.Sprintf("  %s %s %s/%s%s\n", edge.Relation, node.Kind, node.Namespace, node.Name, status))
					break
				}
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("--- Events ---\n")
	sb.WriteString(eventsResult)

	return truncate(sb.String(), MaxDescribeChars), false
}

// --- Metrics & dependency tool implementations ---

func toolGetClusterMetrics(client *k8s.Client, args map[string]interface{}) (string, bool) {
	result, err := client.GetNodeMetrics()
	if err != nil {
		return fmt.Sprintf("Error getting node metrics: %v", err), true
	}
	if !result.Available {
		msg := "Metrics API not available"
		if result.Error != "" {
			msg += ": " + result.Error
		}
		return msg, true
	}
	if len(result.Metrics) == 0 {
		return "No node metrics found", false
	}

	var totalCPU, totalMem, totalCPUCap, totalMemCap int64
	var totalCPUReq, totalMemReq, totalCPUComm, totalMemComm int64
	var totalPods, totalPodCap int64
	for _, n := range result.Metrics {
		totalCPU += n.CPUUsage
		totalMem += n.MemoryUsage
		totalCPUCap += n.CPUCapacity
		totalMemCap += n.MemCapacity
		totalCPUReq += n.CPURequested
		totalMemReq += n.MemRequested
		totalCPUComm += n.CPUCommitted
		totalMemComm += n.MemCommitted
		totalPods += n.PodCount
		totalPodCap += n.PodCapacity
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Cluster: %d nodes, %d/%d pods (%d%%)\n",
		len(result.Metrics), totalPods, totalPodCap, pct(totalPods, totalPodCap)))
	sb.WriteString(fmt.Sprintf("CPU:  usage %s/%s (%d%%), reserved %s (%d%%), committed %s (%d%%)\n",
		formatCPUCompact(totalCPU), formatCPUCompact(totalCPUCap), pct(totalCPU, totalCPUCap),
		formatCPUCompact(totalCPUReq), pct(totalCPUReq, totalCPUCap),
		formatCPUCompact(totalCPUComm), pct(totalCPUComm, totalCPUCap)))
	sb.WriteString(fmt.Sprintf("Mem:  usage %s/%s (%d%%), reserved %s (%d%%), committed %s (%d%%)\n",
		formatBytesCompact(totalMem), formatBytesCompact(totalMemCap), pct(totalMem, totalMemCap),
		formatBytesCompact(totalMemReq), pct(totalMemReq, totalMemCap),
		formatBytesCompact(totalMemComm), pct(totalMemComm, totalMemCap)))

	sb.WriteString(fmt.Sprintf("\n%-30s %10s %5s %10s %5s %10s %10s %s\n",
		"NODE", "CPU-USE", "CPU%", "MEM-USE", "MEM%", "CPU-CAP", "MEM-CAP", "PODS"))
	for _, n := range result.Metrics {
		sb.WriteString(fmt.Sprintf("%-30s %10s %4d%% %10s %4d%% %10s %10s %d/%d\n",
			n.Name,
			formatCPUCompact(n.CPUUsage), pct(n.CPUUsage, n.CPUCapacity),
			formatBytesCompact(n.MemoryUsage), pct(n.MemoryUsage, n.MemCapacity),
			formatCPUCompact(n.CPUCapacity), formatBytesCompact(n.MemCapacity),
			n.PodCount, n.PodCapacity))
	}

	return truncate(sb.String(), MaxMetricsChars), false
}

func toolGetPodMetrics(client *k8s.Client, args map[string]interface{}) (string, bool) {
	result, err := client.GetPodMetrics()
	if err != nil {
		return fmt.Sprintf("Error getting pod metrics: %v", err), true
	}
	if !result.Available {
		msg := "Metrics API not available"
		if result.Error != "" {
			msg += ": " + result.Error
		}
		return msg, true
	}
	if len(result.Metrics) == 0 {
		return "No pod metrics found", false
	}

	metrics := result.Metrics

	ns := strArg(args, "namespace")
	if ns != "" {
		var filtered []k8s.PodMetrics
		for _, m := range metrics {
			if m.Namespace == ns {
				filtered = append(filtered, m)
			}
		}
		metrics = filtered
		if len(metrics) == 0 {
			return fmt.Sprintf("No pod metrics found in namespace %q", ns), false
		}
	}

	sortBy := strArg(args, "sort_by")
	switch sortBy {
	case "memory":
		sort.Slice(metrics, func(i, j int) bool { return metrics[i].MemoryUsage > metrics[j].MemoryUsage })
	case "name":
		sort.Slice(metrics, func(i, j int) bool { return metrics[i].Name < metrics[j].Name })
	default:
		sort.Slice(metrics, func(i, j int) bool { return metrics[i].CPUUsage > metrics[j].CPUUsage })
	}

	if len(metrics) > 50 {
		metrics = metrics[:50]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-50s %-20s %10s %10s %10s %10s %s\n",
		"POD", "NAMESPACE", "CPU", "CPU-REQ", "MEM", "MEM-REQ", "NODE"))
	for _, m := range metrics {
		cpuReq := "-"
		if m.CPURequested > 0 {
			cpuReq = formatCPUCompact(m.CPURequested)
		}
		memReq := "-"
		if m.MemRequested > 0 {
			memReq = formatBytesCompact(m.MemRequested)
		}
		sb.WriteString(fmt.Sprintf("%-50s %-20s %10s %10s %10s %10s %s\n",
			m.Name, m.Namespace,
			formatCPUCompact(m.CPUUsage), cpuReq,
			formatBytesCompact(m.MemoryUsage), memReq,
			m.NodeName))
	}

	return truncate(sb.String(), MaxPodMetricsChars), false
}

func toolGetNamespaceSummary(client *k8s.Client, args map[string]interface{}) (string, bool) {
	ns := strArg(args, "namespace")
	if ns == "" {
		return "namespace is required", true
	}

	counts, err := client.GetNamespaceResourceCounts(ns)
	if err != nil {
		return fmt.Sprintf("Error getting namespace summary: %v", err), true
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Namespace %q resource counts:\n", ns))
	sb.WriteString(fmt.Sprintf("  Pods: %d, Deployments: %d, StatefulSets: %d, DaemonSets: %d\n",
		counts.Pods, counts.Deployments, counts.StatefulSets, counts.DaemonSets))
	sb.WriteString(fmt.Sprintf("  ReplicaSets: %d, Jobs: %d, CronJobs: %d\n",
		counts.ReplicaSets, counts.Jobs, counts.CronJobs))
	sb.WriteString(fmt.Sprintf("  Services: %d, Ingresses: %d\n",
		counts.Services, counts.Ingresses))
	sb.WriteString(fmt.Sprintf("  ConfigMaps: %d, Secrets: %d, PVCs: %d\n",
		counts.ConfigMaps, counts.Secrets, counts.PVCs))

	return sb.String(), false
}

func toolGetResourceDependencies(client *k8s.Client, args map[string]interface{}) (string, bool) {
	kind := strings.ToLower(strArg(args, "kind"))
	name := strArg(args, "name")
	ns := strArg(args, "namespace")

	if kind == "" || name == "" {
		return "kind and name are required", true
	}

	resourceType := NormalizeKind(kind)

	graph, err := client.GetResourceDependencies("", resourceType, ns, name)
	if err != nil {
		return fmt.Sprintf("Error getting dependencies: %v", err), true
	}
	if graph == nil || len(graph.Nodes) == 0 {
		return fmt.Sprintf("No dependencies found for %s %q", capitalize(resourceType), name), false
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Dependencies for %s %q", capitalize(resourceType), name))
	if ns != "" {
		sb.WriteString(fmt.Sprintf(" (ns: %s)", ns))
	}
	sb.WriteString(":\n")

	for _, edge := range graph.Edges {
		for _, node := range graph.Nodes {
			if node.ID == edge.Target {
				status := ""
				if node.Status != "" {
					status = fmt.Sprintf(" [%s]", node.Status)
				}
				nsPart := ""
				if node.Namespace != "" {
					nsPart = node.Namespace + "/"
				}
				sb.WriteString(fmt.Sprintf("  %s %s %s%s%s\n", edge.Relation, node.Kind, nsPart, node.Name, status))
				break
			}
		}
	}

	return truncate(sb.String(), MaxDependenciesChars), false
}

// --- CRD tool implementations ---

func toolListCRDs(client *k8s.Client, args map[string]interface{}) (string, bool) {
	ctx := client.GetCurrentContext()
	crds, err := client.ListCRDs(ctx)
	if err != nil {
		return fmt.Sprintf("Error listing CRDs: %v", err), true
	}
	if len(crds) == 0 {
		return "No CustomResourceDefinitions found", false
	}

	sort.Slice(crds, func(i, j int) bool {
		if crds[i].Spec.Group != crds[j].Spec.Group {
			return crds[i].Spec.Group < crds[j].Spec.Group
		}
		return crds[i].Name < crds[j].Name
	})

	var lines []string
	lines = append(lines, fmt.Sprintf("%-60s %-30s %-10s %-25s %-12s %-25s", "NAME", "GROUP", "VERSION", "KIND", "SCOPE", "PLURAL"))
	for _, crd := range crds {
		version := ""
		for _, v := range crd.Spec.Versions {
			if v.Served {
				version = v.Name
				break
			}
		}
		scope := string(crd.Spec.Scope)
		kind := crd.Spec.Names.Kind
		plural := crd.Spec.Names.Plural
		lines = append(lines, fmt.Sprintf("%-60s %-30s %-10s %-25s %-12s %-25s", crd.Name, crd.Spec.Group, version, kind, scope, plural))
	}
	return truncate(strings.Join(lines, "\n"), MaxCRDListChars), false
}

func toolListCustomResources(client *k8s.Client, args map[string]interface{}) (string, bool) {
	group := strArg(args, "group")
	version := strArg(args, "version")
	resource := strArg(args, "resource")
	ns := strArg(args, "namespace")

	if group == "" || version == "" || resource == "" {
		return "group, version, and resource are required", true
	}

	ctx := client.GetCurrentContext()
	items, err := client.ListCustomResources(ctx, group, version, resource, ns)
	if err != nil {
		return fmt.Sprintf("Error listing custom resources: %v", err), true
	}
	if len(items) == 0 {
		return fmt.Sprintf("No %s resources found", resource), false
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%-50s %-20s %-10s", "NAME", "NAMESPACE", "AGE"))
	for _, item := range items {
		name := nestedString(item, "metadata", "name")
		namespace := nestedString(item, "metadata", "namespace")
		createdAt := nestedString(item, "metadata", "creationTimestamp")
		ageStr := "<unknown>"
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			ageStr = age(t)
		}
		lines = append(lines, fmt.Sprintf("%-50s %-20s %-10s", name, namespace, ageStr))
	}
	return truncate(strings.Join(lines, "\n"), MaxCustomResourceListChars), false
}

func toolGetCustomResourceYaml(client *k8s.Client, args map[string]interface{}) (string, bool) {
	group := strArg(args, "group")
	version := strArg(args, "version")
	resource := strArg(args, "resource")
	name := strArg(args, "name")
	ns := strArg(args, "namespace")

	if group == "" || version == "" || resource == "" || name == "" {
		return "group, version, resource, and name are required", true
	}

	ctx := client.GetCurrentContext()
	yamlStr, err := client.GetCustomResourceYaml(ctx, group, version, resource, ns, name)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}
	return truncate(yamlStr, MaxYAMLChars), false
}

// nestedString extracts a string from a nested map path.
func nestedString(obj map[string]interface{}, keys ...string) string {
	current := obj
	for i, key := range keys {
		val, ok := current[key]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			if s, ok := val.(string); ok {
				return s
			}
			return ""
		}
		if m, ok := val.(map[string]interface{}); ok {
			current = m
		} else {
			return ""
		}
	}
	return ""
}

// redactSecretYaml replaces the values under `data:` and `stringData:` blocks
// with [REDACTED] to prevent leaking secrets to the AI provider.
var secretDataLineRe = regexp.MustCompile(`^(\s+\S+:\s)(.+)$`)

func redactSecretYaml(yaml string) string {
	lines := strings.Split(yaml, "\n")
	inDataBlock := false
	dataIndent := 0
	var out []string

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "data:" || trimmed == "stringData:" {
			inDataBlock = true
			dataIndent = len(line) - len(strings.TrimLeft(line, " "))
			out = append(out, line)
			continue
		}

		if inDataBlock {
			if trimmed == "" || strings.HasPrefix(strings.TrimSpace(trimmed), "#") {
				out = append(out, line)
				continue
			}
			lineIndent := len(line) - len(strings.TrimLeft(line, " "))
			if lineIndent > dataIndent {
				if m := secretDataLineRe.FindStringSubmatch(line); m != nil {
					out = append(out, m[1]+"[REDACTED]")
				} else {
					out = append(out, line)
				}
				continue
			}
			inDataBlock = false
		}

		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// --- Helpers ---

func age(t time.Time) string {
	if t.IsZero() {
		return "<unknown>"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func eventTime(e v1.Event) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	if !e.EventTime.IsZero() {
		return e.EventTime.Time
	}
	return e.CreationTimestamp.Time
}

func formatBytesCompact(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
	)
	switch {
	case bytes >= tb:
		return fmt.Sprintf("%.1fTB", float64(bytes)/float64(tb))
	case bytes >= gb:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func formatCPUCompact(millicores int64) string {
	if millicores >= 1000 {
		return fmt.Sprintf("%.1fcores", float64(millicores)/1000.0)
	}
	return fmt.Sprintf("%dm", millicores)
}

func pct(usage, capacity int64) int {
	if capacity == 0 {
		return 0
	}
	return int(usage * 100 / capacity)
}

// NormalizeKind maps plural/variant resource kind strings to their canonical singular form.
// sync: frontend/src/components/layout/AIPanel.jsx:kindToViewName
func NormalizeKind(kind string) string {
	switch kind {
	case "pods":
		return "pod"
	case "deployments":
		return "deployment"
	case "statefulsets":
		return "statefulset"
	case "daemonsets":
		return "daemonset"
	case "replicasets":
		return "replicaset"
	case "jobs":
		return "job"
	case "cronjobs":
		return "cronjob"
	case "services":
		return "service"
	case "ingresses":
		return "ingress"
	case "configmaps":
		return "configmap"
	case "secrets":
		return "secret"
	case "nodes":
		return "node"
	case "namespaces":
		return "namespace"
	case "pvcs", "persistentvolumeclaims":
		return "pvc"
	case "pvs", "persistentvolumes":
		return "pv"
	case "storageclasses":
		return "storageclass"
	case "hpas", "horizontalpodautoscalers":
		return "hpa"
	case "pdbs", "poddisruptionbudgets":
		return "pdb"
	case "serviceaccounts":
		return "serviceaccount"
	case "networkpolicies":
		return "networkpolicy"
	case "ingressclasses":
		return "ingressclass"
	default:
		return kind
	}
}

// --- Diagnostic tool implementations ---

func toolGetFlowTimeline(client *k8s.Client, args map[string]interface{}) (string, bool) {
	resourceType := strArg(args, "resource_type")
	namespace := strArg(args, "namespace")
	name := strArg(args, "name")

	if resourceType == "" || namespace == "" || name == "" {
		return "resource_type, namespace, and name are required", true
	}

	req := k8s.FlowTimelineRequest{
		ResourceType:    resourceType,
		Namespace:       namespace,
		Name:            name,
		DurationMinutes: 30,
		MaxEntries:      100,
		IncludeLogs:     false,
	}

	entries, err := client.GetFlowTimeline(req)
	if err != nil {
		return fmt.Sprintf("Error getting flow timeline: %v", err), true
	}

	if len(entries) == 0 {
		return fmt.Sprintf("No timeline events found for %s/%s in namespace %s", resourceType, name, namespace), false
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Timeline for %s/%s (namespace: %s)\n\n", resourceType, name, namespace))

	for _, entry := range entries {
		ts := entry.Timestamp.Format(time.RFC3339)
		icon := getTimelineIcon(entry.EntryType, entry.Severity)
		sb.WriteString(fmt.Sprintf("%s %s **%s** [%s]: %s\n", ts, icon, entry.Kind, entry.Severity, entry.Message))
		if entry.Details != "" {
			sb.WriteString(fmt.Sprintf("   Details: %s\n", entry.Details))
		}
		if entry.ResourceRef != "" {
			sb.WriteString(fmt.Sprintf("   Resource: %s\n", entry.ResourceRef))
		}
		sb.WriteString("\n")
	}

	return truncate(sb.String(), MaxToolOutputChars), false
}

func getTimelineIcon(entryType, severity string) string {
	if severity == "warning" || severity == "error" {
		return "[!]"
	}
	switch entryType {
	case "event":
		return "[E]"
	case "log":
		return "[L]"
	case "change":
		return "[C]"
	default:
		return "[.]"
	}
}

func toolGetMultiPodLogs(client *k8s.Client, args map[string]interface{}) (string, bool) {
	namespace := strArg(args, "namespace")
	labelSelectorStr := strArg(args, "label_selector")
	container := strArg(args, "container")
	tailLines := int64(intArg(args, "tail_lines", 50))
	sinceSeconds := int64(intArg(args, "since_seconds", 300))

	if namespace == "" || labelSelectorStr == "" {
		return "namespace and label_selector are required", true
	}

	// Parse label selector string to map
	labelSelector := make(map[string]string)
	pairs := strings.Split(labelSelectorStr, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			labelSelector[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	if len(labelSelector) == 0 {
		return "invalid label_selector format, use 'key=value,key2=value2'", true
	}

	req := k8s.MultiLogRequest{
		Namespace:     namespace,
		LabelSelector: labelSelector,
		Container:     container,
		TailLines:     tailLines,
		SinceSeconds:  sinceSeconds,
		Follow:        false,
		Timestamps:    true,
	}

	entries, err := client.GetMultiPodLogsBatch(req)
	if err != nil {
		return fmt.Sprintf("Error getting multi-pod logs: %v", err), true
	}

	if len(entries) == 0 {
		return fmt.Sprintf("No logs found for pods matching '%s' in namespace %s", labelSelectorStr, namespace), false
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Logs from pods matching '%s' (namespace: %s)\n\n", labelSelectorStr, namespace))

	// Group by pod for cleaner output
	podGroups := make(map[string][]k8s.MultiLogEntry)
	for _, entry := range entries {
		key := entry.PodName
		podGroups[key] = append(podGroups[key], entry)
	}

	for podName, podEntries := range podGroups {
		sb.WriteString(fmt.Sprintf("## Pod: %s\n", podName))
		for _, entry := range podEntries {
			ts := entry.Timestamp.Format("15:04:05")
			sb.WriteString(fmt.Sprintf("[%s] [%s] %s\n", ts, entry.Container, entry.Message))
		}
		sb.WriteString("\n")
	}

	return truncate(sb.String(), MaxToolOutputChars), false
}

func toolDiffResources(client *k8s.Client, args map[string]interface{}) (string, bool) {
	req := k8s.DiffRequest{
		SourceContext:   strArg(args, "source_context"),
		SourceNamespace: strArg(args, "source_namespace"),
		SourceKind:      strArg(args, "source_kind"),
		SourceName:      strArg(args, "source_name"),
		TargetContext:   strArg(args, "target_context"),
		TargetNamespace: strArg(args, "target_namespace"),
		TargetKind:      strArg(args, "target_kind"),
		TargetName:      strArg(args, "target_name"),
	}

	if req.SourceNamespace == "" || req.SourceKind == "" || req.SourceName == "" ||
		req.TargetNamespace == "" || req.TargetKind == "" || req.TargetName == "" {
		return "source and target namespace, kind, and name are required", true
	}

	result, err := client.DiffResources(req)
	if err != nil {
		return fmt.Sprintf("Error comparing resources: %v", err), true
	}

	var sb strings.Builder
	sourceLabel := fmt.Sprintf("%s/%s/%s", req.SourceNamespace, req.SourceKind, req.SourceName)
	targetLabel := fmt.Sprintf("%s/%s/%s", req.TargetNamespace, req.TargetKind, req.TargetName)

	if req.SourceContext != "" {
		sourceLabel = req.SourceContext + "/" + sourceLabel
	}
	if req.TargetContext != "" {
		targetLabel = req.TargetContext + "/" + targetLabel
	}

	sb.WriteString("# Resource Diff\n\n")
	sb.WriteString(fmt.Sprintf("**Source**: %s (exists: %v)\n", sourceLabel, result.SourceExists))
	sb.WriteString(fmt.Sprintf("**Target**: %s (exists: %v)\n\n", targetLabel, result.TargetExists))

	if !result.HasChanges {
		sb.WriteString("**No differences found** - resources are identical\n")
		return sb.String(), false
	}

	sb.WriteString(fmt.Sprintf("**Changes found**: %d\n\n", result.ChangeCount))

	// Show structured changes
	if len(result.Changes) > 0 {
		sb.WriteString("## Summary of Changes\n\n")
		for _, change := range result.Changes {
			switch change.Type {
			case "added":
				sb.WriteString(fmt.Sprintf("+ **%s**: %s\n", change.Path, change.New))
			case "removed":
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", change.Path, change.Old))
			case "changed":
				sb.WriteString(fmt.Sprintf("~ **%s**: %s -> %s\n", change.Path, change.Old, change.New))
			}
		}
		sb.WriteString("\n")
	}

	// Show unified diff if not too long
	if result.UnifiedDiff != "" && len(result.UnifiedDiff) < 5000 {
		sb.WriteString("## Unified Diff\n\n```diff\n")
		sb.WriteString(result.UnifiedDiff)
		sb.WriteString("```\n")
	}

	return truncate(sb.String(), MaxToolOutputChars), false
}

func toolCheckRBACAccess(client *k8s.Client, args map[string]interface{}) (string, bool) {
	subjectKind := strArg(args, "subject_kind")
	subjectName := strArg(args, "subject_name")
	subjectNamespace := strArg(args, "subject_namespace")
	namespace := strArg(args, "target_namespace")
	resource := strArg(args, "resource")
	verb := strArg(args, "verb")

	if subjectKind == "" || subjectName == "" {
		return "subject_kind and subject_name are required", true
	}

	// Normalize and validate subject kind
	switch strings.ToLower(subjectKind) {
	case "user":
		subjectKind = "User"
	case "group":
		subjectKind = "Group"
	case "serviceaccount", "sa":
		subjectKind = "ServiceAccount"
	default:
		return "subject_kind must be User, Group, or ServiceAccount", true
	}

	if subjectKind == "ServiceAccount" && subjectNamespace == "" {
		return "subject_namespace is required for ServiceAccount", true
	}

	// If no specific resource/verb provided, check common ones
	if resource == "" || verb == "" {
		return checkMultipleRBACPermissions(client, subjectKind, subjectName, subjectNamespace, namespace)
	}

	req := k8s.RBACCheckRequest{
		SubjectKind:      subjectKind,
		SubjectName:      subjectName,
		SubjectNamespace: subjectNamespace,
		Namespace:        namespace,
		Resource:         resource,
		Verb:             verb,
	}

	result, err := client.CheckRBACAccess(req)
	if err != nil {
		return fmt.Sprintf("Error checking RBAC access: %v", err), true
	}

	var sb strings.Builder

	// Format subject
	subjectDesc := fmt.Sprintf("%s '%s'", subjectKind, subjectName)
	if subjectKind == "ServiceAccount" {
		subjectDesc = fmt.Sprintf("ServiceAccount '%s/%s'", subjectNamespace, subjectName)
	}

	sb.WriteString(fmt.Sprintf("# RBAC Check: %s\n\n", subjectDesc))
	sb.WriteString(fmt.Sprintf("**Action**: %s %s", verb, resource))
	if namespace != "" {
		sb.WriteString(fmt.Sprintf(" in namespace '%s'", namespace))
	} else {
		sb.WriteString(" (cluster-wide)")
	}
	sb.WriteString("\n\n")

	if result.Allowed {
		sb.WriteString("**Result**: ALLOWED\n")
	} else {
		sb.WriteString("**Result**: DENIED\n")
	}

	if result.Reason != "" {
		sb.WriteString(fmt.Sprintf("**Reason**: %s\n", result.Reason))
	}

	// Show permission chain
	if len(result.Chain) > 0 {
		sb.WriteString("\n## Permission Chain\n\n")
		for _, link := range result.Chain {
			status := "denies"
			if link.Grants {
				status = "grants"
			}
			sb.WriteString(fmt.Sprintf("- %s '%s' %s via rule: %s\n",
				link.Kind, link.Name, status, link.Rule))
		}
	}

	return sb.String(), false
}

// checkMultipleRBACPermissions checks common permissions when no specific resource/verb is provided
func checkMultipleRBACPermissions(client *k8s.Client, subjectKind, subjectName, subjectNamespace, namespace string) (string, bool) {
	var sb strings.Builder

	subjectDesc := fmt.Sprintf("%s '%s'", subjectKind, subjectName)
	if subjectKind == "ServiceAccount" {
		subjectDesc = fmt.Sprintf("ServiceAccount '%s/%s'", subjectNamespace, subjectName)
	}

	sb.WriteString(fmt.Sprintf("# RBAC Permissions Summary: %s\n\n", subjectDesc))
	if namespace != "" {
		sb.WriteString(fmt.Sprintf("**Namespace**: %s\n\n", namespace))
	} else {
		sb.WriteString("**Scope**: Cluster-wide\n\n")
	}

	// Check common resource/verb combinations
	checks := []struct {
		resource string
		verb     string
	}{
		{"pods", "get"},
		{"pods", "list"},
		{"pods", "create"},
		{"pods", "delete"},
		{"pods/log", "get"},
		{"pods/exec", "create"},
		{"deployments", "get"},
		{"deployments", "list"},
		{"deployments", "create"},
		{"deployments", "update"},
		{"services", "get"},
		{"services", "list"},
		{"configmaps", "get"},
		{"configmaps", "list"},
		{"secrets", "get"},
		{"secrets", "list"},
	}

	sb.WriteString("| Resource | Verb | Allowed |\n")
	sb.WriteString("|----------|------|---------|")

	for _, check := range checks {
		req := k8s.RBACCheckRequest{
			SubjectKind:      subjectKind,
			SubjectName:      subjectName,
			SubjectNamespace: subjectNamespace,
			Namespace:        namespace,
			Resource:         check.resource,
			Verb:             check.verb,
		}

		result, err := client.CheckRBACAccess(req)
		status := "?"
		if err == nil {
			if result.Allowed {
				status = "Yes"
			} else {
				status = "No"
			}
		}
		sb.WriteString(fmt.Sprintf("\n| %s | %s | %s |", check.resource, check.verb, status))
	}

	sb.WriteString("\n")
	return truncate(sb.String(), MaxToolOutputChars), false
}
