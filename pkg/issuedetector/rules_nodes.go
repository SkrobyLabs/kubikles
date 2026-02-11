package issuedetector

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
)

func nodeRules() []Rule {
	return []Rule{
		&ruleNODE001{baseRule: baseRule{id: "NODE001", name: "Node Not Ready", description: "Node has Ready condition that is not True", severity: SeverityCritical, category: CategoryWorkloads, requires: []string{"nodes"}}},
		&ruleNODE002{baseRule: baseRule{id: "NODE002", name: "Node Disk Pressure", description: "Node is reporting DiskPressure condition", severity: SeverityWarning, category: CategoryWorkloads, requires: []string{"nodes"}}},
		&ruleNODE003{baseRule: baseRule{id: "NODE003", name: "Node Memory Pressure", description: "Node is reporting MemoryPressure condition", severity: SeverityWarning, category: CategoryWorkloads, requires: []string{"nodes"}}},
	}
}

// NODE001: Node has Ready condition != True
type ruleNODE001 struct{ baseRule }

func (r *ruleNODE001) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	nodes := cache.Nodes()
	var findings []Finding

	for _, node := range nodes {
		for _, cond := range node.Status.Conditions {
			if cond.Type == v1.NodeReady && cond.Status != v1.ConditionTrue {
				reason := cond.Reason
				if reason == "" {
					reason = "unknown"
				}
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "Node", Name: node.Name},
					fmt.Sprintf("Node '%s' is not Ready (status=%s, reason=%s)", node.Name, cond.Status, reason),
					"Check node conditions, kubelet logs, and system resources on the node",
					map[string]string{
						"status":  string(cond.Status),
						"reason":  reason,
						"message": cond.Message,
					},
				))
				break
			}
		}
	}
	return findings, nil
}

// NODE002: Node reporting DiskPressure
type ruleNODE002 struct{ baseRule }

func (r *ruleNODE002) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	nodes := cache.Nodes()
	var findings []Finding

	for _, node := range nodes {
		for _, cond := range node.Status.Conditions {
			if cond.Type == v1.NodeDiskPressure && cond.Status == v1.ConditionTrue {
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "Node", Name: node.Name},
					fmt.Sprintf("Node '%s' has DiskPressure — pods may be evicted", node.Name),
					"Free disk space on the node, clean up unused images, or expand storage",
					map[string]string{
						"reason":  cond.Reason,
						"message": cond.Message,
					},
				))
				break
			}
		}
	}
	return findings, nil
}

// NODE003: Node reporting MemoryPressure
type ruleNODE003 struct{ baseRule }

func (r *ruleNODE003) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	nodes := cache.Nodes()
	var findings []Finding

	for _, node := range nodes {
		for _, cond := range node.Status.Conditions {
			if cond.Type == v1.NodeMemoryPressure && cond.Status == v1.ConditionTrue {
				details := map[string]string{
					"reason":  cond.Reason,
					"message": cond.Message,
				}
				// Include allocatable memory if available
				if alloc, ok := node.Status.Allocatable[v1.ResourceMemory]; ok {
					details["allocatableMemory"] = alloc.String()
				}
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "Node", Name: node.Name},
					fmt.Sprintf("Node '%s' has MemoryPressure — pods may be evicted", node.Name),
					"Review pod memory requests/limits, consider adding nodes or increasing node memory",
					details,
				))
				break
			}
		}
	}
	return findings, nil
}
