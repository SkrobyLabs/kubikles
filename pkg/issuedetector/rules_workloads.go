package issuedetector

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
)

func workloadRules() []Rule {
	return []Rule{
		&ruleWRK001{baseRule: baseRule{id: "WRK001", name: "No Resource Limits", description: "Containers missing CPU or memory limits", severity: SeverityWarning, category: CategoryWorkloads, requires: []string{"pods"}}},
		&ruleWRK002{baseRule: baseRule{id: "WRK002", name: "CrashLooping Pods", description: "Pod containers restarting frequently with CrashLoopBackOff", severity: SeverityCritical, category: CategoryWorkloads, requires: []string{"pods"}}},
		&ruleWRK003{baseRule: baseRule{id: "WRK003", name: "Replica Mismatch", description: "Deployment desired replicas differ from available replicas", severity: SeverityWarning, category: CategoryWorkloads, requires: []string{"deployments"}}},
		&ruleWRK004{baseRule: baseRule{id: "WRK004", name: "HPA Target Missing", description: "HPA targets a non-existent workload", severity: SeverityWarning, category: CategoryWorkloads, requires: []string{"hpas", "deployments", "statefulsets"}}},
		&ruleWRK005{baseRule: baseRule{id: "WRK005", name: "OOMKilled Pods", description: "Container was terminated due to out-of-memory", severity: SeverityCritical, category: CategoryWorkloads, requires: []string{"pods"}}},
		&ruleWRK006{baseRule: baseRule{id: "WRK006", name: "Pending Pods", description: "Pod stuck in Pending state", severity: SeverityCritical, category: CategoryWorkloads, requires: []string{"pods"}}},
		&ruleWRK007{baseRule: baseRule{id: "WRK007", name: "ImagePullBackOff", description: "Container has image pull failure", severity: SeverityCritical, category: CategoryWorkloads, requires: []string{"pods"}}},
		&ruleWRK008{baseRule: baseRule{id: "WRK008", name: "Missing Health Probes", description: "Containers without liveness and readiness probes", severity: SeverityWarning, category: CategoryWorkloads, requires: []string{"pods"}}},
		&ruleWRK009{baseRule: baseRule{id: "WRK009", name: "StatefulSet Replica Mismatch", description: "StatefulSet desired replicas differ from ready replicas", severity: SeverityWarning, category: CategoryWorkloads, requires: []string{"statefulsets"}}},
		&ruleWRK010{baseRule: baseRule{id: "WRK010", name: "DaemonSet Not Fully Scheduled", description: "DaemonSet desired pods differ from ready pods", severity: SeverityWarning, category: CategoryWorkloads, requires: []string{"daemonsets"}}},
		&ruleWRK011{baseRule: baseRule{id: "WRK011", name: "Evicted Pods", description: "Pod was evicted from node", severity: SeverityInfo, category: CategoryWorkloads, requires: []string{"pods"}}},
		&ruleWRK012{baseRule: baseRule{id: "WRK012", name: "Single-Replica Deployment", description: "Deployment has only one replica with no high availability", severity: SeverityInfo, category: CategoryWorkloads, requires: []string{"deployments"}}},
		&ruleWRK013{baseRule: baseRule{id: "WRK013", name: "Requests Exceed Node Capacity", description: "Container requests exceed the allocatable capacity of every node in the cluster", severity: SeverityWarning, category: CategoryWorkloads, requires: []string{"pods", "nodes"}}},
		&ruleWRK014{baseRule: baseRule{id: "WRK014", name: "Missing Pod Anti-Affinity for HA", description: "Multi-replica workload without pod anti-affinity may schedule all replicas on the same node", severity: SeverityInfo, category: CategoryWorkloads, requires: []string{"deployments", "statefulsets"}}},
	}
}

// WRK001: Containers without resource limits
type ruleWRK001 struct{ baseRule }

func (r *ruleWRK001) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		// Skip completed pods
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		var noLimits []string
		for _, c := range pod.Spec.Containers {
			if c.Resources.Limits.Cpu().IsZero() && c.Resources.Limits.Memory().IsZero() {
				noLimits = append(noLimits, c.Name)
			}
		}

		if len(noLimits) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' has containers without resource limits: %s", pod.Name, strings.Join(noLimits, ", ")),
				"Set CPU and memory limits on all containers to prevent resource starvation",
				map[string]string{"containers": strings.Join(noLimits, ",")},
			))
		}
	}
	return findings, nil
}

// WRK002: CrashLooping pods
type ruleWRK002 struct{ baseRule }

func (r *ruleWRK002) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 5 && cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
					fmt.Sprintf("Pod '%s' container '%s' is CrashLooping (%d restarts)",
						pod.Name, cs.Name, cs.RestartCount),
					"Check container logs for error details: kubectl logs <pod> -c <container>",
					map[string]string{
						"container":    cs.Name,
						"restartCount": fmt.Sprintf("%d", cs.RestartCount),
					},
				))
			}
		}
	}
	return findings, nil
}

// WRK003: Deployment replica mismatch
type ruleWRK003 struct{ baseRule }

func (r *ruleWRK003) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	deployments := cache.Deployments()
	var findings []Finding

	for _, dep := range deployments {
		desired := int32(1)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}
		available := dep.Status.AvailableReplicas

		if desired > 0 && available != desired {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Deployment", Name: dep.Name, Namespace: dep.Namespace},
				fmt.Sprintf("Deployment '%s' wants %d replicas but only %d are available",
					dep.Name, desired, available),
				"Check pod events and logs for scheduling or startup failures",
				map[string]string{
					"desired":   fmt.Sprintf("%d", desired),
					"available": fmt.Sprintf("%d", available),
				},
			))
		}
	}
	return findings, nil
}

// WRK004: HPA targets non-existent workload
type ruleWRK004 struct{ baseRule }

func (r *ruleWRK004) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	hpas := cache.HPAs()
	deployments := cache.Deployments()
	statefulsets := cache.StatefulSets()

	// Build lookup index
	depIndex := make(map[string]bool)
	for _, d := range deployments {
		depIndex[d.Namespace+"/"+d.Name] = true
	}
	stsIndex := make(map[string]bool)
	for _, s := range statefulsets {
		stsIndex[s.Namespace+"/"+s.Name] = true
	}

	var findings []Finding
	for _, hpa := range hpas {
		ref := hpa.Spec.ScaleTargetRef
		key := hpa.Namespace + "/" + ref.Name

		found := false
		switch ref.Kind {
		case "Deployment":
			found = depIndex[key]
		case "StatefulSet":
			found = stsIndex[key]
		default:
			continue // Unknown kind, skip
		}

		if !found {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "HPA", Name: hpa.Name, Namespace: hpa.Namespace},
				fmt.Sprintf("HPA '%s' targets %s '%s' which does not exist",
					hpa.Name, ref.Kind, ref.Name),
				"Create the target workload or update the HPA target reference",
				map[string]string{"targetKind": ref.Kind, "targetName": ref.Name},
			))
		}
	}
	return findings, nil
}

// WRK005: OOMKilled pods
type ruleWRK005 struct{ baseRule }

func (r *ruleWRK005) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
					fmt.Sprintf("Pod '%s' container '%s' was OOMKilled (exit code %d)",
						pod.Name, cs.Name, cs.LastTerminationState.Terminated.ExitCode),
					"Increase memory limits or optimize application memory usage",
					map[string]string{
						"container":    cs.Name,
						"restartCount": fmt.Sprintf("%d", cs.RestartCount),
					},
				))
			}
		}
	}
	return findings, nil
}

// WRK006: Pending pods
type ruleWRK006 struct{ baseRule }

func (r *ruleWRK006) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase != v1.PodPending {
			continue
		}

		// Skip pods owned by Jobs (they might legitimately be pending)
		ownedByJob := false
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "Job" {
				ownedByJob = true
				break
			}
		}
		if ownedByJob {
			continue
		}

		findings = append(findings, makeFinding(r,
			ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
			fmt.Sprintf("Pod '%s' is stuck in Pending state", pod.Name),
			"Check events for scheduling failures, resource constraints, or node affinity issues",
			nil,
		))
	}
	return findings, nil
}

// WRK007: ImagePullBackOff
type ruleWRK007 struct{ baseRule }

func (r *ruleWRK007) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil &&
				(cs.State.Waiting.Reason == "ImagePullBackOff" || cs.State.Waiting.Reason == "ErrImagePull") {
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
					fmt.Sprintf("Pod '%s' container '%s' has image pull failure: %s",
						pod.Name, cs.Name, cs.Image),
					"Verify image name/tag exists, check registry credentials and network access",
					map[string]string{
						"container": cs.Name,
						"image":     cs.Image,
						"reason":    cs.State.Waiting.Reason,
					},
				))
			}
		}
	}
	return findings, nil
}

// WRK008: Missing health probes
type ruleWRK008 struct{ baseRule }

func (r *ruleWRK008) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		// Skip completed/failed pods
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		var noProbes []string
		for _, c := range pod.Spec.Containers {
			// Only check regular containers (init containers are in pod.Spec.InitContainers)
			if c.LivenessProbe == nil && c.ReadinessProbe == nil {
				noProbes = append(noProbes, c.Name)
			}
		}

		if len(noProbes) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' has containers without health probes: %s", pod.Name, strings.Join(noProbes, ", ")),
				"Add liveness and readiness probes for reliable health monitoring",
				map[string]string{"containers": strings.Join(noProbes, ",")},
			))
		}
	}
	return findings, nil
}

// WRK009: StatefulSet replica mismatch
type ruleWRK009 struct{ baseRule }

func (r *ruleWRK009) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	statefulsets := cache.StatefulSets()
	var findings []Finding

	for _, sts := range statefulsets {
		desired := int32(1)
		if sts.Spec.Replicas != nil {
			desired = *sts.Spec.Replicas
		}
		ready := sts.Status.ReadyReplicas

		if desired > 0 && ready != desired {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "StatefulSet", Name: sts.Name, Namespace: sts.Namespace},
				fmt.Sprintf("StatefulSet '%s' wants %d replicas but only %d are ready",
					sts.Name, desired, ready),
				"Check pod events and logs for scheduling or startup failures",
				map[string]string{
					"desired": fmt.Sprintf("%d", desired),
					"ready":   fmt.Sprintf("%d", ready),
				},
			))
		}
	}
	return findings, nil
}

// WRK010: DaemonSet not fully scheduled
type ruleWRK010 struct{ baseRule }

func (r *ruleWRK010) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	daemonsets := cache.DaemonSets()
	var findings []Finding

	for _, ds := range daemonsets {
		desired := ds.Status.DesiredNumberScheduled
		ready := ds.Status.NumberReady

		if desired != ready {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "DaemonSet", Name: ds.Name, Namespace: ds.Namespace},
				fmt.Sprintf("DaemonSet '%s' wants %d pods but only %d are ready",
					ds.Name, desired, ready),
				"Check node taints, tolerations, and pod resource requirements",
				map[string]string{
					"desired": fmt.Sprintf("%d", desired),
					"ready":   fmt.Sprintf("%d", ready),
				},
			))
		}
	}
	return findings, nil
}

// WRK011: Evicted pods
type ruleWRK011 struct{ baseRule }

func (r *ruleWRK011) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodFailed && pod.Status.Reason == "Evicted" {
			msg := pod.Status.Message
			if msg == "" {
				msg = "no message"
			}
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' was evicted: %s", pod.Name, msg),
				"Review node resource pressure; set appropriate resource requests and limits",
				map[string]string{"message": msg},
			))
		}
	}
	return findings, nil
}

// WRK012: Single-replica deployment
type ruleWRK012 struct{ baseRule }

func (r *ruleWRK012) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	deployments := cache.Deployments()
	var findings []Finding

	skipNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}

	for _, dep := range deployments {
		if skipNamespaces[dep.Namespace] {
			continue
		}

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		if replicas == 1 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Deployment", Name: dep.Name, Namespace: dep.Namespace},
				fmt.Sprintf("Deployment '%s' has only 1 replica — no high availability", dep.Name),
				"Consider increasing replicas for production workloads",
				nil,
			))
		}
	}
	return findings, nil
}

// WRK013: Requests exceed node capacity — container can never be scheduled
type ruleWRK013 struct{ baseRule }

func (r *ruleWRK013) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	nodes := cache.Nodes()
	if len(nodes) == 0 {
		return nil, nil // no node data available, skip
	}

	// Find the maximum allocatable CPU and memory across all nodes
	var maxCPU, maxMem int64
	for _, node := range nodes {
		if cpu := node.Status.Allocatable.Cpu().MilliValue(); cpu > maxCPU {
			maxCPU = cpu
		}
		if mem := node.Status.Allocatable.Memory().Value(); mem > maxMem {
			maxMem = mem
		}
	}
	if maxCPU == 0 && maxMem == 0 {
		return nil, nil // nodes have no allocatable resources reported
	}

	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase != v1.PodRunning && pod.Status.Phase != v1.PodPending {
			continue
		}

		for _, c := range pod.Spec.Containers {
			cpuReq := c.Resources.Requests.Cpu().MilliValue()
			memReq := c.Resources.Requests.Memory().Value()

			if cpuReq == 0 && memReq == 0 {
				continue
			}

			exceedsCPU := maxCPU > 0 && cpuReq > maxCPU
			exceedsMem := maxMem > 0 && memReq > maxMem

			if exceedsCPU || exceedsMem {
				var detail string
				if exceedsCPU && exceedsMem {
					detail = fmt.Sprintf("requests %dm CPU (max node: %dm) and %s memory (max node: %s)",
						cpuReq, maxCPU, c.Resources.Requests.Memory().String(), fmtBytes(maxMem))
				} else if exceedsCPU {
					detail = fmt.Sprintf("requests %dm CPU (max node: %dm)",
						cpuReq, maxCPU)
				} else {
					detail = fmt.Sprintf("requests %s memory (max node: %s)",
						c.Resources.Requests.Memory().String(), fmtBytes(maxMem))
				}

				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
					fmt.Sprintf("Pod '%s' container '%s' %s — exceeds every node's capacity",
						pod.Name, c.Name, detail),
					"Resource requests exceed the largest node's allocatable capacity. The pod cannot be scheduled. Reduce requests or add larger nodes",
					map[string]string{
						"container":  c.Name,
						"cpuReq":     fmt.Sprintf("%dm", cpuReq),
						"memReq":     c.Resources.Requests.Memory().String(),
						"maxNodeCPU": fmt.Sprintf("%dm", maxCPU),
						"maxNodeMem": fmtBytes(maxMem),
					},
				))
			}
		}
	}
	return findings, nil
}

// WRK014: Missing Pod Anti-Affinity for HA workloads
type ruleWRK014 struct{ baseRule }

func (r *ruleWRK014) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	deployments := cache.Deployments()
	statefulsets := cache.StatefulSets()
	var findings []Finding

	skipNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}

	for _, dep := range deployments {
		if skipNamespaces[dep.Namespace] {
			continue
		}
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas <= 1 {
			continue
		}

		affinity := dep.Spec.Template.Spec.Affinity
		if affinity != nil && affinity.PodAntiAffinity != nil {
			if len(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0 ||
				len(affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0 {
				continue
			}
		}

		findings = append(findings, makeFinding(r,
			ResourceRef{Kind: "Deployment", Name: dep.Name, Namespace: dep.Namespace},
			fmt.Sprintf("Deployment '%s' has %d replicas but no pod anti-affinity — all replicas may land on the same node",
				dep.Name, replicas),
			"Add podAntiAffinity to spread replicas across nodes for high availability",
			map[string]string{
				"replicas": fmt.Sprintf("%d", replicas),
			},
		))
	}

	for _, sts := range statefulsets {
		if skipNamespaces[sts.Namespace] {
			continue
		}
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		if replicas <= 1 {
			continue
		}

		affinity := sts.Spec.Template.Spec.Affinity
		if affinity != nil && affinity.PodAntiAffinity != nil {
			if len(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0 ||
				len(affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0 {
				continue
			}
		}

		findings = append(findings, makeFinding(r,
			ResourceRef{Kind: "StatefulSet", Name: sts.Name, Namespace: sts.Namespace},
			fmt.Sprintf("StatefulSet '%s' has %d replicas but no pod anti-affinity — all replicas may land on the same node",
				sts.Name, replicas),
			"Add podAntiAffinity to spread replicas across nodes for high availability",
			map[string]string{
				"replicas": fmt.Sprintf("%d", replicas),
			},
		))
	}
	return findings, nil
}

func fmtBytes(b int64) string {
	const gi = 1024 * 1024 * 1024
	if b >= gi && b%gi == 0 {
		return fmt.Sprintf("%dGi", b/gi)
	}
	const mi = 1024 * 1024
	if b >= mi && b%mi == 0 {
		return fmt.Sprintf("%dMi", b/mi)
	}
	return fmt.Sprintf("%d", b)
}
