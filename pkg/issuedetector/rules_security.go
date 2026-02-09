package issuedetector

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
)

func securityRules() []Rule {
	return []Rule{
		&ruleSEC001{baseRule: baseRule{id: "SEC001", name: "Running as Root", description: "Container running as root user or without security context", severity: SeverityWarning, category: CategorySecurity, requires: []string{"pods"}}},
		&ruleSEC002{baseRule: baseRule{id: "SEC002", name: "Default ServiceAccount", description: "Pod using the default ServiceAccount", severity: SeverityInfo, category: CategorySecurity, requires: []string{"pods"}}},
		&ruleSEC003{baseRule: baseRule{id: "SEC003", name: "Privileged Containers", description: "Container running in privileged mode", severity: SeverityWarning, category: CategorySecurity, requires: []string{"pods"}}},
		&ruleSEC004{baseRule: baseRule{id: "SEC004", name: "Host Network/PID/IPC", description: "Pod using host namespaces", severity: SeverityWarning, category: CategorySecurity, requires: []string{"pods"}}},
		&ruleSEC005{baseRule: baseRule{id: "SEC005", name: "Writable Root Filesystem", description: "Container without read-only root filesystem", severity: SeverityInfo, category: CategorySecurity, requires: []string{"pods"}}},
		&ruleSEC006{baseRule: baseRule{id: "SEC006", name: "Privilege Escalation Allowed", description: "Container allows privilege escalation", severity: SeverityWarning, category: CategorySecurity, requires: []string{"pods"}}},
		&ruleSEC007{baseRule: baseRule{id: "SEC007", name: "Capability Additions", description: "Container adds dangerous Linux capabilities", severity: SeverityWarning, category: CategorySecurity, requires: []string{"pods"}}},
	}
}

// SEC001: Container running as root
type ruleSEC001 struct{ baseRule }

func (r *ruleSEC001) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		var rootContainers []string
		for _, c := range pod.Spec.Containers {
			runAsRoot := false

			if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil {
				if *c.SecurityContext.RunAsUser == 0 {
					runAsRoot = true
				}
			} else if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsUser != nil {
				if *pod.Spec.SecurityContext.RunAsUser == 0 {
					runAsRoot = true
				}
			}
			// Note: we don't flag missing securityContext as root - that's too noisy.
			// Only explicit runAsUser=0.

			if runAsRoot {
				rootContainers = append(rootContainers, c.Name)
			}
		}

		if len(rootContainers) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' has containers running as root (UID 0): %s", pod.Name, strings.Join(rootContainers, ", ")),
				"Set runAsNonRoot: true or specify a non-zero runAsUser in the security context",
				map[string]string{"containers": strings.Join(rootContainers, ",")},
			))
		}
	}
	return findings, nil
}

// SEC002: Pod using default ServiceAccount
type ruleSEC002 struct{ baseRule }

func (r *ruleSEC002) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		saName := pod.Spec.ServiceAccountName
		if saName == "" || saName == "default" {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' is using the 'default' ServiceAccount", pod.Name),
				"Create a dedicated ServiceAccount with minimal RBAC permissions",
				nil,
			))
		}
	}
	return findings, nil
}

// SEC003: Privileged containers
type ruleSEC003 struct{ baseRule }

func (r *ruleSEC003) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		var privileged []string
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				privileged = append(privileged, c.Name)
			}
		}

		if len(privileged) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' has privileged containers: %s", pod.Name, strings.Join(privileged, ", ")),
				"Remove privileged mode unless absolutely required; use specific capabilities instead",
				map[string]string{"containers": strings.Join(privileged, ",")},
			))
		}
	}
	return findings, nil
}

// SEC004: Host Network/PID/IPC
type ruleSEC004 struct{ baseRule }

func (r *ruleSEC004) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		var hostNS []string
		if pod.Spec.HostNetwork {
			hostNS = append(hostNS, "hostNetwork")
		}
		if pod.Spec.HostPID {
			hostNS = append(hostNS, "hostPID")
		}
		if pod.Spec.HostIPC {
			hostNS = append(hostNS, "hostIPC")
		}

		if len(hostNS) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' uses host namespaces: %s", pod.Name, strings.Join(hostNS, ", ")),
				"Avoid host namespace sharing unless required; it breaks container isolation",
				map[string]string{"hostNamespaces": strings.Join(hostNS, ",")},
			))
		}
	}
	return findings, nil
}

// SEC005: Writable root filesystem
type ruleSEC005 struct{ baseRule }

func (r *ruleSEC005) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		var writable []string
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext == nil || c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
				writable = append(writable, c.Name)
			}
		}

		if len(writable) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' has containers with writable root filesystem: %s", pod.Name, strings.Join(writable, ", ")),
				"Set readOnlyRootFilesystem: true and use emptyDir or volume mounts for writable paths",
				map[string]string{"containers": strings.Join(writable, ",")},
			))
		}
	}
	return findings, nil
}

// SEC006: Privilege escalation allowed
type ruleSEC006 struct{ baseRule }

func (r *ruleSEC006) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		var escalation []string
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext == nil || c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
				escalation = append(escalation, c.Name)
			}
		}

		if len(escalation) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' has containers that allow privilege escalation: %s", pod.Name, strings.Join(escalation, ", ")),
				"Set allowPrivilegeEscalation: false in the container security context",
				map[string]string{"containers": strings.Join(escalation, ",")},
			))
		}
	}
	return findings, nil
}

// SEC007: Capability additions
type ruleSEC007 struct{ baseRule }

// dangerousCapabilities is the set of Linux capabilities considered dangerous.
var dangerousCapabilities = map[v1.Capability]bool{
	"SYS_ADMIN":  true,
	"NET_ADMIN":  true,
	"SYS_PTRACE": true,
	"SYS_RAWIO":  true,
	"NET_RAW":    true,
	"ALL":        true,
}

func (r *ruleSEC007) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		for _, c := range pod.Spec.Containers {
			if c.SecurityContext == nil || c.SecurityContext.Capabilities == nil {
				continue
			}

			var dangerous []string
			for _, cap := range c.SecurityContext.Capabilities.Add {
				if dangerousCapabilities[cap] {
					dangerous = append(dangerous, string(cap))
				}
			}

			if len(dangerous) > 0 {
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
					fmt.Sprintf("Pod '%s' container '%s' adds dangerous capabilities: %s", pod.Name, c.Name, strings.Join(dangerous, ", ")),
					"Remove unnecessary capabilities; follow the principle of least privilege",
					map[string]string{"container": c.Name, "capabilities": strings.Join(dangerous, ",")},
				))
			}
		}
	}
	return findings, nil
}
