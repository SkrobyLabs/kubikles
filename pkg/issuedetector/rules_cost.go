package issuedetector

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
)

func costRules() []Rule {
	return []Rule{
		&ruleCOST001{baseRule: baseRule{id: "COST001", name: "LoadBalancer Without Endpoints", description: "LoadBalancer Service has no backing endpoints, incurring cost without serving traffic", severity: SeverityWarning, category: CategoryNetworking, requires: []string{"services", "endpoints"}}},
	}
}

// COST001: LoadBalancer Service with zero ready endpoints
type ruleCOST001 struct{ baseRule }

func (r *ruleCOST001) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	services := cache.Services()
	endpoints := cache.Endpoints()

	epIndex := make(map[string]v1.Endpoints) // "ns/name" -> Endpoints
	for _, ep := range endpoints {
		epIndex[ep.Namespace+"/"+ep.Name] = ep
	}

	var findings []Finding
	for _, svc := range services {
		if svc.Spec.Type != v1.ServiceTypeLoadBalancer {
			continue
		}

		key := svc.Namespace + "/" + svc.Name
		ep, exists := epIndex[key]

		hasReadyAddresses := false
		if exists {
			for _, subset := range ep.Subsets {
				if len(subset.Addresses) > 0 {
					hasReadyAddresses = true
					break
				}
			}
		}

		if !hasReadyAddresses {
			desc := fmt.Sprintf("LoadBalancer Service '%s' has no ready endpoints — it may be incurring cloud provider costs without serving traffic", svc.Name)
			if !exists {
				desc = fmt.Sprintf("LoadBalancer Service '%s' has no Endpoints object — it may be incurring cloud provider costs without serving traffic", svc.Name)
			}
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Service", Name: svc.Name, Namespace: svc.Namespace},
				desc,
				"Consider converting to ClusterIP or removing if unused",
				nil,
			))
		}
	}
	return findings, nil
}
