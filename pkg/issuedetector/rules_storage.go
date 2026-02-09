package issuedetector

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
)

func storageRules() []Rule {
	return []Rule{
		&ruleSTR001{baseRule: baseRule{id: "STR001", name: "Orphan PVCs", description: "PVC not mounted by any Pod", severity: SeverityInfo, category: CategoryStorage, requires: []string{"pvcs", "pods"}}},
		&ruleSTR002{baseRule: baseRule{id: "STR002", name: "PV Released/Failed", description: "PersistentVolume in Released or Failed phase", severity: SeverityWarning, category: CategoryStorage, requires: []string{"pvs"}}},
		&ruleSTR003{baseRule: baseRule{id: "STR003", name: "PVC Pending", description: "PVC stuck in Pending state with no PV bound", severity: SeverityWarning, category: CategoryStorage, requires: []string{"pvcs"}}},
	}
}

// STR001: PVCs not mounted by any pod
type ruleSTR001 struct{ baseRule }

func (r *ruleSTR001) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pvcs := cache.PVCs()
	pods := cache.Pods()

	// Build set of PVCs referenced by running/pending pods
	usedPVCs := make(map[string]bool) // "ns/name"
	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				usedPVCs[pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}

	var findings []Finding
	for _, pvc := range pvcs {
		key := pvc.Namespace + "/" + pvc.Name
		if !usedPVCs[key] {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "PVC", Name: pvc.Name, Namespace: pvc.Namespace},
				fmt.Sprintf("PVC '%s' is not mounted by any running Pod", pvc.Name),
				"Delete the PVC if it's no longer needed, or mount it in a workload",
				map[string]string{"phase": string(pvc.Status.Phase)},
			))
		}
	}
	return findings, nil
}

// STR002: PVs in Released or Failed phase
type ruleSTR002 struct{ baseRule }

func (r *ruleSTR002) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pvs := cache.PVs()
	var findings []Finding

	for _, pv := range pvs {
		if pv.Status.Phase == v1.VolumeReleased || pv.Status.Phase == v1.VolumeFailed {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "PV", Name: pv.Name},
				fmt.Sprintf("PersistentVolume '%s' is in '%s' phase", pv.Name, pv.Status.Phase),
				"Reclaim or delete the PersistentVolume to free storage resources",
				map[string]string{"phase": string(pv.Status.Phase)},
			))
		}
	}
	return findings, nil
}

// STR003: PVC stuck in Pending state
type ruleSTR003 struct{ baseRule }

func (r *ruleSTR003) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pvcs := cache.PVCs()
	var findings []Finding

	for _, pvc := range pvcs {
		if pvc.Status.Phase == v1.ClaimPending {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "PVC", Name: pvc.Name, Namespace: pvc.Namespace},
				fmt.Sprintf("PVC '%s' is in Pending state — no PersistentVolume bound", pvc.Name),
				"Check StorageClass provisioner, PV availability, and access modes",
				map[string]string{"phase": string(pvc.Status.Phase)},
			))
		}
	}
	return findings, nil
}
