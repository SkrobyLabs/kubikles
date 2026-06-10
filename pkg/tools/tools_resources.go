// Code split from tools.go; see that file for the package overview.
package tools

import (
	"fmt"
	"sort"
	"strings"

	"kubikles/pkg/k8s"

	v1 "k8s.io/api/core/v1"
)

// --- Tool implementations ---

func toolGetPodLogs(client *k8s.Client, args map[string]interface{}) (string, bool) {
	ns := strArg(args, "namespace")
	pod := strArg(args, "pod")
	container := strArg(args, "container")
	previous := boolArg(args, "previous", false)
	if ns == "" || pod == "" {
		return "namespace and pod are required", true
	}

	logs, err := client.GetPodLogs(ns, pod, container, false, previous, "")
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
