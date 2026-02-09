package issuedetector

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func networkingRules() []Rule {
	return []Rule{
		&ruleNET001{baseRule: baseRule{id: "NET001", name: "Missing IngressClass", description: "Ingress references an IngressClass that doesn't exist", severity: SeverityWarning, category: CategoryNetworking, requires: []string{"ingresses", "ingressclasses"}}},
		&ruleNET002{baseRule: baseRule{id: "NET002", name: "Service No Matching Pods", description: "Service selector matches zero running pods", severity: SeverityWarning, category: CategoryNetworking, requires: []string{"services", "pods"}}},
		&ruleNET003{baseRule: baseRule{id: "NET003", name: "Service Port Mismatch", description: "Service targetPort not found on any matched pod's containers", severity: SeverityWarning, category: CategoryNetworking, requires: []string{"services", "pods"}}},
		&ruleNET004{baseRule: baseRule{id: "NET004", name: "Ingress Backend Service Missing", description: "Ingress backend references a non-existent Service", severity: SeverityCritical, category: CategoryNetworking, requires: []string{"ingresses", "services"}}},
		&ruleNET005{baseRule: baseRule{id: "NET005", name: "Endpoints Not Ready", description: "Service has Endpoints but none are ready", severity: SeverityWarning, category: CategoryNetworking, requires: []string{"services", "endpoints"}}},
		&ruleNET006{baseRule: baseRule{id: "NET006", name: "Duplicate Ingress Host", description: "Multiple Ingresses claim the same host, causing routing conflicts", severity: SeverityCritical, category: CategoryNetworking, requires: []string{"ingresses"}}},
		&ruleNET007{baseRule: baseRule{id: "NET007", name: "Service ExternalName Dangling", description: "ExternalName Service has no externalName set", severity: SeverityWarning, category: CategoryNetworking, requires: []string{"services"}}},
		&ruleNET008{baseRule: baseRule{id: "NET008", name: "LoadBalancer Pending", description: "LoadBalancer Service has no external IP assigned", severity: SeverityWarning, category: CategoryNetworking, requires: []string{"services"}}},
		&ruleNET009{baseRule: baseRule{id: "NET009", name: "NodePort Service", description: "Service uses NodePort type", severity: SeverityInfo, category: CategoryNetworking, requires: []string{"services"}}},
	}
}

// NET001: Ingress references IngressClass that doesn't exist
type ruleNET001 struct{ baseRule }

func (r *ruleNET001) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	ingresses := cache.Ingresses()
	classes := cache.IngressClasses()
	classNames := make(map[string]bool)
	for _, ic := range classes {
		classNames[ic.Name] = true
	}

	var findings []Finding
	for _, ing := range ingresses {
		className := ""
		if ing.Spec.IngressClassName != nil {
			className = *ing.Spec.IngressClassName
		}
		if className == "" {
			continue
		}
		if !classNames[className] {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Ingress", Name: ing.Name, Namespace: ing.Namespace},
				fmt.Sprintf("Ingress '%s' references IngressClass '%s' which does not exist", ing.Name, className),
				"Create the IngressClass or update the Ingress to use an existing one",
				map[string]string{"ingressClass": className},
			))
		}
	}
	return findings, nil
}

// NET002: Service selector matches zero running pods
type ruleNET002 struct{ baseRule }

func (r *ruleNET002) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	services := cache.Services()
	pods := cache.Pods()
	var findings []Finding

	for _, svc := range services {
		if len(svc.Spec.Selector) == 0 {
			continue // headless/no-selector services
		}
		if svc.Spec.Type == v1.ServiceTypeExternalName {
			continue
		}

		sel := labels.SelectorFromSet(svc.Spec.Selector)
		matched := 0
		for _, pod := range pods {
			if pod.Namespace != svc.Namespace {
				continue
			}
			if sel.Matches(labels.Set(pod.Labels)) && pod.Status.Phase == v1.PodRunning {
				matched++
			}
		}

		if matched == 0 {
			selectorStr := formatSelector(svc.Spec.Selector)
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Service", Name: svc.Name, Namespace: svc.Namespace},
				fmt.Sprintf("Service '%s' selector (%s) matches no running pods", svc.Name, selectorStr),
				"Check that pods with matching labels are running in the same namespace",
				map[string]string{"selector": selectorStr},
			))
		}
	}
	return findings, nil
}

// NET003: Service targetPort not exposed on matched pods
type ruleNET003 struct{ baseRule }

func (r *ruleNET003) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	services := cache.Services()
	pods := cache.Pods()
	var findings []Finding

	for _, svc := range services {
		if len(svc.Spec.Selector) == 0 || svc.Spec.Type == v1.ServiceTypeExternalName {
			continue
		}

		sel := labels.SelectorFromSet(svc.Spec.Selector)
		var matchedPods []v1.Pod
		for _, pod := range pods {
			if pod.Namespace == svc.Namespace && sel.Matches(labels.Set(pod.Labels)) {
				matchedPods = append(matchedPods, pod)
			}
		}
		if len(matchedPods) == 0 {
			continue // NET002 covers this
		}

		for _, port := range svc.Spec.Ports {
			targetPort := port.TargetPort
			if targetPort.IntValue() == 0 && targetPort.String() == "" {
				continue
			}

			portFound := false
			for _, pod := range matchedPods {
				for _, c := range pod.Spec.Containers {
					for _, cp := range c.Ports {
						if targetPort.IntValue() > 0 {
							if int(cp.ContainerPort) == targetPort.IntValue() {
								portFound = true
							}
						} else if cp.Name == targetPort.String() {
							portFound = true
						}
					}
				}
				if portFound {
					break
				}
			}

			if !portFound {
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "Service", Name: svc.Name, Namespace: svc.Namespace},
					fmt.Sprintf("Service '%s' port '%s' (targetPort: %s) not found on any matched pod containers",
						svc.Name, port.Name, targetPort.String()),
					"Ensure pods expose the port referenced by the Service",
					map[string]string{"targetPort": targetPort.String(), "servicePort": port.Name},
				))
			}
		}
	}
	return findings, nil
}

// NET004: Ingress backend references non-existent Service
type ruleNET004 struct{ baseRule }

func (r *ruleNET004) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	ingresses := cache.Ingresses()
	services := cache.Services()
	svcIndex := make(map[string]bool) // "ns/name" -> true
	for _, svc := range services {
		svcIndex[svc.Namespace+"/"+svc.Name] = true
	}

	var findings []Finding
	for _, ing := range ingresses {
		// Check default backend
		if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
			key := ing.Namespace + "/" + ing.Spec.DefaultBackend.Service.Name
			if !svcIndex[key] {
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "Ingress", Name: ing.Name, Namespace: ing.Namespace},
					fmt.Sprintf("Ingress '%s' default backend references non-existent Service '%s'",
						ing.Name, ing.Spec.DefaultBackend.Service.Name),
					"Create the Service or update the Ingress backend",
					map[string]string{"missingService": ing.Spec.DefaultBackend.Service.Name},
				))
			}
		}

		// Check rule backends
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service == nil {
					continue
				}
				key := ing.Namespace + "/" + path.Backend.Service.Name
				if !svcIndex[key] {
					findings = append(findings, makeFinding(r,
						ResourceRef{Kind: "Ingress", Name: ing.Name, Namespace: ing.Namespace},
						fmt.Sprintf("Ingress '%s' path '%s' references non-existent Service '%s'",
							ing.Name, path.Path, path.Backend.Service.Name),
						"Create the Service or update the Ingress backend",
						map[string]string{"missingService": path.Backend.Service.Name, "path": path.Path},
					))
				}
			}
		}
	}
	return findings, nil
}

// NET005: Service has Endpoints but none are ready
type ruleNET005 struct{ baseRule }

func (r *ruleNET005) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	services := cache.Services()
	endpoints := cache.Endpoints()
	epIndex := make(map[string]v1.Endpoints) // "ns/name" -> Endpoints
	for _, ep := range endpoints {
		epIndex[ep.Namespace+"/"+ep.Name] = ep
	}

	var findings []Finding
	for _, svc := range services {
		if len(svc.Spec.Selector) == 0 || svc.Spec.Type == v1.ServiceTypeExternalName {
			continue
		}

		ep, ok := epIndex[svc.Namespace+"/"+svc.Name]
		if !ok {
			continue // No endpoints object at all - different issue
		}

		hasAddresses := false
		hasReady := false
		for _, subset := range ep.Subsets {
			if len(subset.NotReadyAddresses) > 0 {
				hasAddresses = true
			}
			if len(subset.Addresses) > 0 {
				hasAddresses = true
				hasReady = true
			}
		}

		if hasAddresses && !hasReady {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Service", Name: svc.Name, Namespace: svc.Namespace},
				fmt.Sprintf("Service '%s' has endpoints but none are ready", svc.Name),
				"Check that backing pods are healthy and passing readiness probes",
				nil,
			))
		}
	}
	return findings, nil
}

// NET006: Multiple Ingresses claim the same host with overlapping paths
type ruleNET006 struct{ baseRule }

// ingressPathKey groups ingresses by (host, path, pathType).
// Only ingresses that share all of these represent a real routing conflict.
// Different paths on the same host is a valid nginx-ingress merge pattern.
type ingressPathKey struct {
	host, path, pathType string
}

type ingressRef struct {
	ns, name string
}

func (r *ruleNET006) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	ingresses := cache.Ingresses()

	// Map (host, path, pathType) → list of ingresses claiming it
	pathOwners := make(map[ingressPathKey][]ingressRef)
	for _, ing := range ingresses {
		ref := ingressRef{ns: ing.Namespace, name: ing.Name}

		for _, rule := range ing.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}
			if rule.HTTP == nil {
				// Rule with no HTTP block — use empty path
				key := ingressPathKey{host: host, path: "", pathType: ""}
				pathOwners[key] = append(pathOwners[key], ref)
				continue
			}
			for _, p := range rule.HTTP.Paths {
				pt := "Prefix"
				if p.PathType != nil {
					pt = string(*p.PathType)
				}
				key := ingressPathKey{host: host, path: p.Path, pathType: pt}
				pathOwners[key] = append(pathOwners[key], ref)
			}
		}
	}

	// Deduplicate findings per ingress+host pair (an ingress might have multiple
	// colliding paths on the same host — report once per ingress per host).
	type findingKey struct {
		ns, name, host string
	}
	seen := make(map[findingKey]bool)

	var findings []Finding
	for key, refs := range pathOwners {
		if len(refs) < 2 {
			continue
		}
		for _, ref := range refs {
			fk := findingKey{ns: ref.ns, name: ref.name, host: key.host}
			if seen[fk] {
				continue
			}
			seen[fk] = true

			others := make([]string, 0, len(refs)-1)
			for _, o := range refs {
				if o != ref {
					others = append(others, o.ns+"/"+o.name)
				}
			}
			f := makeFinding(r,
				ResourceRef{Kind: "Ingress", Name: ref.name, Namespace: ref.ns},
				fmt.Sprintf("Ingress '%s' has conflicting path '%s %s' on host '%s' with %s",
					ref.name, key.pathType, key.path, key.host, strings.Join(others, ", ")),
				"Remove the duplicate path or merge the Ingress rules into a single Ingress",
				map[string]string{"host": key.host, "path": key.path, "pathType": key.pathType, "conflicts": strings.Join(others, ", ")},
			)
			f.GroupKey = fmt.Sprintf("%s %s %s", key.host, key.pathType, key.path)
			findings = append(findings, f)
		}
	}
	return findings, nil
}

// NET007: ExternalName Service with no externalName set
type ruleNET007 struct{ baseRule }

func (r *ruleNET007) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	services := cache.Services()
	var findings []Finding

	for _, svc := range services {
		if svc.Spec.Type != v1.ServiceTypeExternalName {
			continue
		}
		if svc.Spec.ExternalName == "" {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Service", Name: svc.Name, Namespace: svc.Namespace},
				fmt.Sprintf("Service '%s' is type ExternalName but has no externalName set", svc.Name),
				"Set the externalName field to the target DNS hostname",
				nil,
			))
		}
	}
	return findings, nil
}

// NET008: LoadBalancer Service with no external IP assigned
type ruleNET008 struct{ baseRule }

func (r *ruleNET008) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	services := cache.Services()
	var findings []Finding

	for _, svc := range services {
		if svc.Spec.Type != v1.ServiceTypeLoadBalancer {
			continue
		}
		if len(svc.Status.LoadBalancer.Ingress) == 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Service", Name: svc.Name, Namespace: svc.Namespace},
				fmt.Sprintf("Service '%s' is type LoadBalancer but has no external IP assigned", svc.Name),
				"Check cloud provider integration, service annotations, and load balancer quotas",
				nil,
			))
		}
	}
	return findings, nil
}

// NET009: NodePort Service (informational)
type ruleNET009 struct{ baseRule }

func (r *ruleNET009) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	services := cache.Services()
	var findings []Finding

	for _, svc := range services {
		if svc.Spec.Type != v1.ServiceTypeNodePort {
			continue
		}
		if svc.Namespace == "kube-system" {
			continue
		}
		findings = append(findings, makeFinding(r,
			ResourceRef{Kind: "Service", Name: svc.Name, Namespace: svc.Namespace},
			fmt.Sprintf("Service '%s' uses NodePort — consider using LoadBalancer or Ingress for production", svc.Name),
			"Use LoadBalancer type or Ingress for external traffic in production environments",
			nil,
		))
	}
	return findings, nil
}

func formatSelector(sel map[string]string) string {
	var parts []string
	for k, v := range sel {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}
