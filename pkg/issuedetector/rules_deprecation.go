package issuedetector

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
)

func deprecationRules() []Rule {
	return []Rule{
		&ruleDEP001{baseRule: baseRule{id: "DEP001", name: "Deprecated Endpoints Resources", description: "Endpoints objects exist (use EndpointSlices instead)", severity: SeverityWarning, category: CategoryDeprecation, requires: []string{"endpoints"}}},
		&ruleDEP002{baseRule: baseRule{id: "DEP002", name: "Deprecated Ingress Class Annotation", description: "Ingress uses deprecated kubernetes.io/ingress.class annotation", severity: SeverityWarning, category: CategoryDeprecation, requires: []string{"ingresses"}}},
		&ruleDEP003{baseRule: baseRule{id: "DEP003", name: "Deprecated Node Topology Labels", description: "Node uses deprecated failure-domain.beta.kubernetes.io labels", severity: SeverityWarning, category: CategoryDeprecation, requires: []string{"nodes"}}},
		&ruleDEP004{baseRule: baseRule{id: "DEP004", name: "Deprecated Seccomp Annotations", description: "Pod uses deprecated seccomp annotations instead of securityContext field", severity: SeverityWarning, category: CategoryDeprecation, requires: []string{"pods"}}},
		&ruleDEP005{baseRule: baseRule{id: "DEP005", name: "Deprecated AppArmor Annotations", description: "Pod uses deprecated AppArmor annotations instead of securityContext field", severity: SeverityWarning, category: CategoryDeprecation, requires: []string{"pods"}}},
	}
}

// DEP001: Deprecated Endpoints resources (should use EndpointSlices)
type ruleDEP001 struct{ baseRule }

func (r *ruleDEP001) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	endpoints := cache.Endpoints()

	count := 0
	for _, ep := range endpoints {
		if ep.Namespace == "kube-system" || ep.Namespace == "kube-public" || ep.Namespace == "kube-node-lease" {
			continue
		}
		count++
	}

	if count == 0 {
		return nil, nil
	}

	var findings []Finding
	findings = append(findings, makeFinding(r,
		ResourceRef{Kind: "Endpoints", Name: fmt.Sprintf("(%d resources)", count)},
		fmt.Sprintf("Found %d Endpoints objects (excluding system namespaces). The Endpoints API is deprecated in favor of EndpointSlices", count),
		"Migrate to EndpointSlice API for better scalability and dual-stack support",
		nil,
	))
	return findings, nil
}

// DEP002: Deprecated kubernetes.io/ingress.class annotation
type ruleDEP002 struct{ baseRule }

func (r *ruleDEP002) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	ingresses := cache.Ingresses()
	var findings []Finding

	for _, ing := range ingresses {
		if _, ok := ing.Annotations["kubernetes.io/ingress.class"]; ok {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Ingress", Name: ing.Name, Namespace: ing.Namespace},
				fmt.Sprintf("Ingress '%s' uses deprecated annotation kubernetes.io/ingress.class", ing.Name),
				"Use spec.ingressClassName field instead of the annotation",
				nil,
			))
		}
	}
	return findings, nil
}

// DEP003: Deprecated failure-domain.beta.kubernetes.io topology labels
type ruleDEP003 struct{ baseRule }

func (r *ruleDEP003) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	nodes := cache.Nodes()
	var findings []Finding

	deprecatedLabels := []string{
		"failure-domain.beta.kubernetes.io/zone",
		"failure-domain.beta.kubernetes.io/region",
	}

	for _, node := range nodes {
		var found []string
		for _, label := range deprecatedLabels {
			if _, ok := node.Labels[label]; ok {
				found = append(found, label)
			}
		}
		if len(found) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Node", Name: node.Name},
				fmt.Sprintf("Node '%s' has deprecated topology labels: %s", node.Name, strings.Join(found, ", ")),
				"Use topology.kubernetes.io/zone and topology.kubernetes.io/region instead",
				nil,
			))
		}
	}
	return findings, nil
}

// DEP004: Deprecated seccomp annotations
type ruleDEP004 struct{ baseRule }

func (r *ruleDEP004) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		var found []string
		for key := range pod.Annotations {
			if key == "seccomp.security.alpha.kubernetes.io/pod" ||
				strings.HasPrefix(key, "container.seccomp.security.alpha.kubernetes.io/") {
				found = append(found, key)
			}
		}
		if len(found) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' uses deprecated seccomp annotations: %s", pod.Name, strings.Join(found, ", ")),
				"Use spec.securityContext.seccompProfile or container-level securityContext.seccompProfile instead",
				nil,
			))
		}
	}
	return findings, nil
}

// DEP005: Deprecated AppArmor annotations
type ruleDEP005 struct{ baseRule }

func (r *ruleDEP005) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		var found []string
		for key := range pod.Annotations {
			if strings.HasPrefix(key, "container.apparmor.security.beta.kubernetes.io/") {
				found = append(found, key)
			}
		}
		if len(found) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' uses deprecated AppArmor annotations: %s", pod.Name, strings.Join(found, ", ")),
				"Use spec.securityContext.appArmorProfile or container-level securityContext.appArmorProfile instead",
				nil,
			))
		}
	}
	return findings, nil
}
