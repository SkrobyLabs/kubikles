// Code split from tools.go; see that file for the package overview.
package tools

import (
	"fmt"
	"sort"
	"strings"

	"kubikles/pkg/k8s"
)

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
