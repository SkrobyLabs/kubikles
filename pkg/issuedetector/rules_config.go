package issuedetector

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
)

func configRules() []Rule {
	return []Rule{
		&ruleCFG001{baseRule: baseRule{id: "CFG001", name: "Unreferenced ConfigMap", description: "ConfigMap not used by any workload", severity: SeverityInfo, category: CategoryConfig, requires: []string{"configmaps", "pods", "deployments", "statefulsets", "daemonsets"}}},
		&ruleCFG002{baseRule: baseRule{id: "CFG002", name: "Missing Image Tag", description: "Container image uses :latest or no tag", severity: SeverityWarning, category: CategoryConfig, requires: []string{"pods"}}},
		&ruleCFG003{baseRule: baseRule{id: "CFG003", name: "Unreferenced Secret", description: "Secret not used by any Pod or ServiceAccount", severity: SeverityInfo, category: CategoryConfig, requires: []string{"secrets", "pods", "serviceaccounts"}}},
		&ruleCFG004{baseRule: baseRule{id: "CFG004", name: "No Resource Requests", description: "Containers missing CPU or memory requests", severity: SeverityWarning, category: CategoryConfig, requires: []string{"pods"}}},
	}
}

// CFG001: ConfigMap not referenced by any workload
type ruleCFG001 struct{ baseRule }

func (r *ruleCFG001) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	configmaps := cache.ConfigMaps()
	pods := cache.Pods()

	// Build set of ConfigMaps referenced by pods (volumes + envFrom + env.valueFrom)
	usedCMs := make(map[string]bool)
	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}
		// Volume references
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				usedCMs[pod.Namespace+"/"+vol.ConfigMap.Name] = true
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.ConfigMap != nil {
						usedCMs[pod.Namespace+"/"+src.ConfigMap.Name] = true
					}
				}
			}
		}
		// Container envFrom and env valueFrom
		for _, c := range append(pod.Spec.Containers, pod.Spec.InitContainers...) {
			for _, ef := range c.EnvFrom {
				if ef.ConfigMapRef != nil {
					usedCMs[pod.Namespace+"/"+ef.ConfigMapRef.Name] = true
				}
			}
			for _, e := range c.Env {
				if e.ValueFrom != nil && e.ValueFrom.ConfigMapKeyRef != nil {
					usedCMs[pod.Namespace+"/"+e.ValueFrom.ConfigMapKeyRef.Name] = true
				}
			}
		}
	}

	// Skip system ConfigMaps
	systemPrefixes := []string{"kube-", "extension-apiserver-"}

	var findings []Finding
	for _, cm := range configmaps {
		// Skip system namespaces
		if cm.Namespace == "kube-system" || cm.Namespace == "kube-public" || cm.Namespace == "kube-node-lease" {
			continue
		}
		// Skip system ConfigMaps
		skip := false
		for _, prefix := range systemPrefixes {
			if strings.HasPrefix(cm.Name, prefix) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		key := cm.Namespace + "/" + cm.Name
		if !usedCMs[key] {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "ConfigMap", Name: cm.Name, Namespace: cm.Namespace},
				fmt.Sprintf("ConfigMap '%s' is not referenced by any running Pod", cm.Name),
				"Delete the ConfigMap if it's no longer needed",
				nil,
			))
		}
	}
	return findings, nil
}

// CFG002: Container image uses :latest or has no tag
type ruleCFG002 struct{ baseRule }

func (r *ruleCFG002) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		var badImages []string
		for _, c := range pod.Spec.Containers {
			image := c.Image
			if isLatestOrUntagged(image) {
				badImages = append(badImages, fmt.Sprintf("%s(%s)", c.Name, image))
			}
		}

		if len(badImages) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' uses :latest or untagged images: %s", pod.Name, strings.Join(badImages, ", ")),
				"Pin container images to specific tags or digests for reproducible deployments",
				map[string]string{"containers": strings.Join(badImages, ",")},
			))
		}
	}
	return findings, nil
}

// CFG003: Secret not referenced by any Pod or ServiceAccount
type ruleCFG003 struct{ baseRule }

func (r *ruleCFG003) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	secrets := cache.Secrets()
	pods := cache.Pods()
	serviceAccounts := cache.ServiceAccounts()

	// Build set of Secrets referenced by pods (volumes, projected volumes, envFrom, env.valueFrom)
	usedSecrets := make(map[string]bool)
	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}
		// Volume references
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				usedSecrets[pod.Namespace+"/"+vol.Secret.SecretName] = true
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.Secret != nil {
						usedSecrets[pod.Namespace+"/"+src.Secret.Name] = true
					}
				}
			}
		}
		// Container envFrom and env valueFrom
		for _, c := range append(pod.Spec.Containers, pod.Spec.InitContainers...) {
			for _, ef := range c.EnvFrom {
				if ef.SecretRef != nil {
					usedSecrets[pod.Namespace+"/"+ef.SecretRef.Name] = true
				}
			}
			for _, e := range c.Env {
				if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
					usedSecrets[pod.Namespace+"/"+e.ValueFrom.SecretKeyRef.Name] = true
				}
			}
		}
	}

	// Secrets referenced by ServiceAccounts (imagePullSecrets, secrets list)
	for _, sa := range serviceAccounts {
		for _, ips := range sa.ImagePullSecrets {
			usedSecrets[sa.Namespace+"/"+ips.Name] = true
		}
		for _, s := range sa.Secrets {
			usedSecrets[sa.Namespace+"/"+s.Name] = true
		}
	}

	var findings []Finding
	for _, secret := range secrets {
		// Skip system namespaces
		if secret.Namespace == "kube-system" || secret.Namespace == "kube-public" || secret.Namespace == "kube-node-lease" {
			continue
		}
		// Skip service-account-token secrets and helm release secrets
		if secret.Type == v1.SecretTypeServiceAccountToken || secret.Type == "helm.sh/release.v1" {
			continue
		}

		key := secret.Namespace + "/" + secret.Name
		if !usedSecrets[key] {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Secret", Name: secret.Name, Namespace: secret.Namespace},
				fmt.Sprintf("Secret '%s' is not referenced by any running Pod or ServiceAccount", secret.Name),
				"Delete the Secret if it's no longer needed",
				nil,
			))
		}
	}
	return findings, nil
}

// CFG004: Containers without resource requests
type ruleCFG004 struct{ baseRule }

func (r *ruleCFG004) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	pods := cache.Pods()
	var findings []Finding

	for _, pod := range pods {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		var noRequests []string
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests.Cpu().IsZero() && c.Resources.Requests.Memory().IsZero() {
				noRequests = append(noRequests, c.Name)
			}
		}

		if len(noRequests) > 0 {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: pod.Namespace},
				fmt.Sprintf("Pod '%s' has containers without resource requests: %s", pod.Name, strings.Join(noRequests, ", ")),
				"Set CPU and memory requests for proper scheduling and resource allocation",
				map[string]string{"containers": strings.Join(noRequests, ",")},
			))
		}
	}
	return findings, nil
}

// isLatestOrUntagged checks if an image reference uses :latest or has no tag.
func isLatestOrUntagged(image string) bool {
	// Images with @ are digest-pinned, always fine
	if strings.Contains(image, "@") {
		return false
	}
	// Check for :latest
	if strings.HasSuffix(image, ":latest") {
		return true
	}
	// No tag at all (no colon after the last slash)
	lastSlash := strings.LastIndex(image, "/")
	afterSlash := image
	if lastSlash >= 0 {
		afterSlash = image[lastSlash+1:]
	}
	return !strings.Contains(afterSlash, ":")
}
