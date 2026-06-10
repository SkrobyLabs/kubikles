// Code split from dependencies.go; see that file for the graph types and entry point.
package k8s

import (
	"context"
	"fmt"
	"strconv"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// getPVCDependencies resolves dependencies for a PVC
func (c *Client) getPVCDependencies(cs kubernetes.Interface, cache *resourceCache, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	pvc, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pvc: %w", err)
	}

	pvcID := nodeID("PersistentVolumeClaim", namespace, name)
	// Get capacity from status (actual) or spec (requested)
	var pvcMeta map[string]string
	if capacity, ok := pvc.Status.Capacity[v1.ResourceStorage]; ok {
		pvcMeta = map[string]string{"capacity": capacity.String()}
	} else if req, ok := pvc.Spec.Resources.Requests[v1.ResourceStorage]; ok {
		pvcMeta = map[string]string{"capacity": req.String()}
	}
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        pvcID,
		Kind:      "PersistentVolumeClaim",
		Name:      name,
		Namespace: namespace,
		Status:    string(pvc.Status.Phase),
		Metadata:  pvcMeta,
	})

	// Find pods using this PVC (using cache)
	pods, err := cache.getPods(namespace)
	if err == nil {
		for _, pod := range pods {
			if podUsesPVC(&pod, name) {
				podID := nodeID("Pod", namespace, pod.Name)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:        podID,
					Kind:      "Pod",
					Name:      pod.Name,
					Namespace: namespace,
					Status:    string(pod.Status.Phase),
				})
				c.addEdge(graph, podID, pvcID, "uses")

				// Add pod's owner refs
				c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, namespace, pod.OwnerReferences)
				// Find Services selecting this pod (and their Ingresses)
				c.findServicesSelectingPod(cache, graph, nodeMap, namespace, pod.Labels, podID)
			}
		}
	}

	// Resolve PV
	if pvc.Spec.VolumeName != "" {
		c.resolvePVFromPVC(cs, graph, nodeMap, pvcID, pvc.Spec.VolumeName, pvc.Spec.StorageClassName)
	}

	return graph, nil
}

// getPVDependencies resolves dependencies for a PV
func (c *Client) getPVDependencies(cs kubernetes.Interface, cache *resourceCache, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	pv, err := cs.CoreV1().PersistentVolumes().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pv: %w", err)
	}

	pvID := nodeID("PersistentVolume", "", name)
	var pvMeta map[string]string
	if capacity, ok := pv.Spec.Capacity[v1.ResourceStorage]; ok {
		pvMeta = map[string]string{"capacity": capacity.String()}
	}
	c.addNode(graph, nodeMap, DependencyNode{
		ID:       pvID,
		Kind:     "PersistentVolume",
		Name:     name,
		Status:   string(pv.Status.Phase),
		Metadata: pvMeta,
	})

	// Find bound PVC
	if pv.Spec.ClaimRef != nil {
		pvcNamespace := pv.Spec.ClaimRef.Namespace
		pvcName := pv.Spec.ClaimRef.Name
		pvcID := nodeID("PersistentVolumeClaim", pvcNamespace, pvcName)

		pvc, err := cs.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
		status := "Unknown"
		if err == nil {
			status = string(pvc.Status.Phase)
		}

		c.addNode(graph, nodeMap, DependencyNode{
			ID:        pvcID,
			Kind:      "PersistentVolumeClaim",
			Name:      pvcName,
			Namespace: pvcNamespace,
			Status:    status,
		})
		c.addEdge(graph, pvcID, pvID, "binds")

		// Find pods using this PVC (using cache)
		if err == nil {
			pods, err := cache.getPods(pvcNamespace)
			if err == nil {
				for _, pod := range pods {
					if podUsesPVC(&pod, pvcName) {
						podID := nodeID("Pod", pvcNamespace, pod.Name)
						c.addNode(graph, nodeMap, DependencyNode{
							ID:        podID,
							Kind:      "Pod",
							Name:      pod.Name,
							Namespace: pvcNamespace,
							Status:    string(pod.Status.Phase),
						})
						c.addEdge(graph, podID, pvcID, "uses")
						c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, pvcNamespace, pod.OwnerReferences)
						// Find Services selecting this pod (and their Ingresses)
						c.findServicesSelectingPod(cache, graph, nodeMap, pvcNamespace, pod.Labels, podID)
					}
				}
			}
		}
	}

	// StorageClass
	if pv.Spec.StorageClassName != "" {
		scID := nodeID("StorageClass", "", pv.Spec.StorageClassName)
		c.addNode(graph, nodeMap, DependencyNode{
			ID:   scID,
			Kind: "StorageClass",
			Name: pv.Spec.StorageClassName,
		})
		c.addEdge(graph, pvID, scID, "uses")
	}

	return graph, nil
}

// getConfigMapDependencies resolves dependencies for a ConfigMap
func (c *Client) getConfigMapDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	cm, err := cs.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap: %w", err)
	}

	cmID := nodeID("ConfigMap", namespace, name)
	keyCount := len(cm.Data) + len(cm.BinaryData)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        cmID,
		Kind:      "ConfigMap",
		Name:      name,
		Namespace: namespace,
		Metadata:  map[string]string{"keys": strconv.Itoa(keyCount)},
	})

	// Find pods using this ConfigMap
	c.findPodsUsingConfigMap(cs, cache, graph, nodeMap, namespace, name, cmID)

	return graph, nil
}

// getSecretDependencies resolves dependencies for a Secret
func (c *Client) getSecretDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	secret, err := cs.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	secretID := nodeID("Secret", namespace, name)
	keyCount := len(secret.Data) + len(secret.StringData)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        secretID,
		Kind:      "Secret",
		Name:      name,
		Namespace: namespace,
		Metadata:  map[string]string{"keys": strconv.Itoa(keyCount)},
	})

	// Find pods using this Secret
	c.findPodsUsingSecret(cs, cache, graph, nodeMap, namespace, name, secretID)

	return graph, nil
}

// getServiceDependencies resolves dependencies for a Service
func (c *Client) getServiceDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	svc, err := cs.CoreV1().Services(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	svcID := nodeID("Service", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        svcID,
		Kind:      "Service",
		Name:      name,
		Namespace: namespace,
	})

	// Find pods matching selector (using cache)
	if len(svc.Spec.Selector) > 0 {
		pods, err := cache.getPods(namespace)
		if err == nil {
			for _, pod := range pods {
				if matchesSelector(pod.Labels, svc.Spec.Selector) {
					podID := nodeID("Pod", namespace, pod.Name)
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        podID,
						Kind:      "Pod",
						Name:      pod.Name,
						Namespace: namespace,
						Status:    string(pod.Status.Phase),
					})
					c.addEdge(graph, svcID, podID, "selects")

					// Add pod's owner refs
					c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, namespace, pod.OwnerReferences)
				}
			}
		}
	}

	// Find Ingresses that route to this Service
	c.findIngressesForService(cache, graph, nodeMap, namespace, name, svcID)

	return graph, nil
}

// findIngressesForService finds all Ingresses that route to a given Service
func (c *Client) findIngressesForService(cache *resourceCache, graph *DependencyGraph, nodeMap map[string]bool, namespace, serviceName, svcID string) {
	ingresses, err := cache.getIngresses(namespace)
	if err != nil {
		return
	}

	for _, ingress := range ingresses {
		routesToService := false

		// Check default backend
		if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
			if ingress.Spec.DefaultBackend.Service.Name == serviceName {
				routesToService = true
			}
		}

		// Check rules
		if !routesToService {
			for _, rule := range ingress.Spec.Rules {
				if rule.HTTP == nil {
					continue
				}
				for _, path := range rule.HTTP.Paths {
					if path.Backend.Service != nil && path.Backend.Service.Name == serviceName {
						routesToService = true
						break
					}
				}
				if routesToService {
					break
				}
			}
		}

		if routesToService {
			ingressID := nodeID("Ingress", namespace, ingress.Name)
			c.addNode(graph, nodeMap, DependencyNode{
				ID:        ingressID,
				Kind:      "Ingress",
				Name:      ingress.Name,
				Namespace: namespace,
			})
			c.addEdge(graph, ingressID, svcID, "routes-to")

			// Also resolve IngressClass if set
			if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName != "" {
				icName := *ingress.Spec.IngressClassName
				icID := nodeID("IngressClass", "", icName)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:   icID,
					Kind: "IngressClass",
					Name: icName,
				})
				c.addEdge(graph, ingressID, icID, "uses")
			}
		}
	}
}

// Helper functions

func (c *Client) findOwnedPods(cs kubernetes.Interface, cache *resourceCache, graph *DependencyGraph, nodeMap map[string]bool, ownerID, namespace, ownerName, ownerKind string) {
	pods, err := cache.getPods(namespace)
	if err != nil {
		return
	}

	for _, pod := range pods {
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == ownerKind && ref.Name == ownerName {
				podID := nodeID("Pod", namespace, pod.Name)
				// Calculate total restart count
				var restarts int32
				for _, cs := range pod.Status.ContainerStatuses {
					restarts += cs.RestartCount
				}
				podMeta := make(map[string]string)
				if restarts > 0 {
					podMeta["restarts"] = strconv.Itoa(int(restarts))
				}
				c.addNode(graph, nodeMap, DependencyNode{
					ID:        podID,
					Kind:      "Pod",
					Name:      pod.Name,
					Namespace: namespace,
					Status:    string(pod.Status.Phase),
					Metadata:  podMeta,
				})
				c.addEdge(graph, ownerID, podID, "owns")

				// Resolve pod's downward dependencies (volumes, configs)
				c.resolvePodVolumes(cs, graph, nodeMap, podID, pod.Name, namespace, pod.Spec.Volumes)
				c.resolvePodContainerRefs(cs, graph, nodeMap, podID, namespace, pod.Spec.Containers)
				c.resolvePodContainerRefs(cs, graph, nodeMap, podID, namespace, pod.Spec.InitContainers)
			}
		}
	}
}

func (c *Client) findSelectingServices(cache *resourceCache, graph *DependencyGraph, nodeMap map[string]bool, namespace string, podLabels map[string]string) {
	if len(podLabels) == 0 {
		return
	}

	services, err := cache.getServices(namespace)
	if err != nil {
		return
	}

	for _, svc := range services {
		if len(svc.Spec.Selector) > 0 && matchesSelector(podLabels, svc.Spec.Selector) {
			svcID := nodeID("Service", namespace, svc.Name)
			c.addNode(graph, nodeMap, DependencyNode{
				ID:        svcID,
				Kind:      "Service",
				Name:      svc.Name,
				Namespace: namespace,
			})
			// Link the Service to the pods already in the graph that its
			// selector actually matches (deterministic, all matching pods)
			pods, err := cache.getPods(namespace)
			if err == nil {
				for _, pod := range pods {
					podID := nodeID("Pod", namespace, pod.Name)
					if nodeMap[podID] && matchesSelector(pod.Labels, svc.Spec.Selector) {
						c.addEdge(graph, svcID, podID, "selects")
					}
				}
			}

			// Find Ingresses that route to this Service
			c.findIngressesForService(cache, graph, nodeMap, namespace, svc.Name, svcID)
		}
	}
}

func (c *Client) findPodsUsingConfigMap(cs kubernetes.Interface, cache *resourceCache, graph *DependencyGraph, nodeMap map[string]bool, namespace, cmName, cmID string) {
	pods, err := cache.getPods(namespace)
	if err != nil {
		return
	}

	for _, pod := range pods {
		usesConfigMap := false

		// Check volumes (plain configMap and projected sources)
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil && vol.ConfigMap.Name == cmName {
				usesConfigMap = true
				break
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.ConfigMap != nil && src.ConfigMap.Name == cmName {
						usesConfigMap = true
						break
					}
				}
				if usesConfigMap {
					break
				}
			}
		}

		// Check containers
		if !usesConfigMap {
			for _, container := range append(pod.Spec.Containers, pod.Spec.InitContainers...) {
				for _, envFrom := range container.EnvFrom {
					if envFrom.ConfigMapRef != nil && envFrom.ConfigMapRef.Name == cmName {
						usesConfigMap = true
						break
					}
				}
				if usesConfigMap {
					break
				}
				for _, env := range container.Env {
					if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil && env.ValueFrom.ConfigMapKeyRef.Name == cmName {
						usesConfigMap = true
						break
					}
				}
				if usesConfigMap {
					break
				}
			}
		}

		if usesConfigMap {
			podID := nodeID("Pod", namespace, pod.Name)
			c.addNode(graph, nodeMap, DependencyNode{
				ID:        podID,
				Kind:      "Pod",
				Name:      pod.Name,
				Namespace: namespace,
				Status:    string(pod.Status.Phase),
			})
			c.addEdge(graph, podID, cmID, "uses")
			c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, namespace, pod.OwnerReferences)
			// Find Services selecting this pod (and their Ingresses)
			c.findServicesSelectingPod(cache, graph, nodeMap, namespace, pod.Labels, podID)
		}
	}
}

func (c *Client) findPodsUsingSecret(cs kubernetes.Interface, cache *resourceCache, graph *DependencyGraph, nodeMap map[string]bool, namespace, secretName, secretID string) {
	pods, err := cache.getPods(namespace)
	if err != nil {
		return
	}

	for _, pod := range pods {
		usesSecret := false

		// Check volumes (plain secret and projected sources)
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil && vol.Secret.SecretName == secretName {
				usesSecret = true
				break
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.Secret != nil && src.Secret.Name == secretName {
						usesSecret = true
						break
					}
				}
				if usesSecret {
					break
				}
			}
		}

		// Check containers
		if !usesSecret {
			for _, container := range append(pod.Spec.Containers, pod.Spec.InitContainers...) {
				for _, envFrom := range container.EnvFrom {
					if envFrom.SecretRef != nil && envFrom.SecretRef.Name == secretName {
						usesSecret = true
						break
					}
				}
				if usesSecret {
					break
				}
				for _, env := range container.Env {
					if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil && env.ValueFrom.SecretKeyRef.Name == secretName {
						usesSecret = true
						break
					}
				}
				if usesSecret {
					break
				}
			}
		}

		if usesSecret {
			podID := nodeID("Pod", namespace, pod.Name)
			c.addNode(graph, nodeMap, DependencyNode{
				ID:        podID,
				Kind:      "Pod",
				Name:      pod.Name,
				Namespace: namespace,
				Status:    string(pod.Status.Phase),
			})
			c.addEdge(graph, podID, secretID, "uses")
			c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, namespace, pod.OwnerReferences)
			// Find Services selecting this pod (and their Ingresses)
			c.findServicesSelectingPod(cache, graph, nodeMap, namespace, pod.Labels, podID)
		}
	}
}

// podUsesPVC reports whether a pod mounts the claim directly or through a
// generic ephemeral volume (which creates a PVC named <pod>-<volume>)
func podUsesPVC(pod *v1.Pod, claimName string) bool {
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == claimName {
			return true
		}
		if vol.Ephemeral != nil && pod.Name+"-"+vol.Name == claimName {
			return true
		}
	}
	return false
}

func matchesSelector(labels, selector map[string]string) bool {
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

// matchesLabelSelector evaluates a full LabelSelector, honoring both
// matchLabels and matchExpressions (In/NotIn/Exists/DoesNotExist)
func matchesLabelSelector(podLabels map[string]string, selector *metav1.LabelSelector) bool {
	if selector == nil {
		return false
	}
	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return false
	}
	return sel.Matches(labels.Set(podLabels))
}

// findServicesSelectingPod finds all Services that select a given Pod
func (c *Client) findServicesSelectingPod(cache *resourceCache, graph *DependencyGraph, nodeMap map[string]bool, namespace string, podLabels map[string]string, podID string) {
	if len(podLabels) == 0 {
		return
	}

	services, err := cache.getServices(namespace)
	if err != nil {
		return
	}

	for _, svc := range services {
		if len(svc.Spec.Selector) > 0 && matchesSelector(podLabels, svc.Spec.Selector) {
			svcID := nodeID("Service", namespace, svc.Name)
			c.addNode(graph, nodeMap, DependencyNode{
				ID:        svcID,
				Kind:      "Service",
				Name:      svc.Name,
				Namespace: namespace,
			})
			c.addEdge(graph, svcID, podID, "selects")

			// Find Ingresses that route to this Service
			c.findIngressesForService(cache, graph, nodeMap, namespace, svc.Name, svcID)
		}
	}
}

// getEndpointsDependencies resolves dependencies for Endpoints
func (c *Client) getEndpointsDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	endpoints, err := cs.CoreV1().Endpoints(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints: %w", err)
	}

	endpointsID := nodeID("Endpoints", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        endpointsID,
		Kind:      "Endpoints",
		Name:      name,
		Namespace: namespace,
	})

	// Endpoints typically share the same name as their Service
	_, err = cs.CoreV1().Services(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err == nil {
		svcID := nodeID("Service", namespace, name)
		c.addNode(graph, nodeMap, DependencyNode{
			ID:        svcID,
			Kind:      "Service",
			Name:      name,
			Namespace: namespace,
		})
		c.addEdge(graph, svcID, endpointsID, "owns")

		// Find Ingresses that route to this Service
		c.findIngressesForService(cache, graph, nodeMap, namespace, name, svcID)
	}

	// Find Pods referenced by this Endpoints object
	for _, subset := range endpoints.Subsets {
		// Ready addresses
		for _, addr := range subset.Addresses {
			if addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
				podName := addr.TargetRef.Name
				podNamespace := namespace
				if addr.TargetRef.Namespace != "" {
					podNamespace = addr.TargetRef.Namespace
				}

				// Use cached pod lookup instead of individual API call
				pod, _ := cache.getPodByName(podNamespace, podName)
				status := "Unknown"
				if pod != nil {
					status = string(pod.Status.Phase)
				}

				podID := nodeID("Pod", podNamespace, podName)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:        podID,
					Kind:      "Pod",
					Name:      podName,
					Namespace: podNamespace,
					Status:    status,
				})
				c.addEdge(graph, endpointsID, podID, "references")

				// Resolve pod's owner refs
				if pod != nil {
					c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, podNamespace, pod.OwnerReferences)
				}
			}
		}

		// Not ready addresses
		for _, addr := range subset.NotReadyAddresses {
			if addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
				podName := addr.TargetRef.Name
				podNamespace := namespace
				if addr.TargetRef.Namespace != "" {
					podNamespace = addr.TargetRef.Namespace
				}

				// Use cached pod lookup instead of individual API call
				pod, _ := cache.getPodByName(podNamespace, podName)
				status := "NotReady"
				if pod != nil {
					status = string(pod.Status.Phase)
				}

				podID := nodeID("Pod", podNamespace, podName)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:        podID,
					Kind:      "Pod",
					Name:      podName,
					Namespace: podNamespace,
					Status:    status,
				})
				c.addEdge(graph, endpointsID, podID, "references")

				if pod != nil {
					c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, podNamespace, pod.OwnerReferences)
				}
			}
		}
	}

	return graph, nil
}

// getPriorityClassDependencies resolves dependencies for a PriorityClass
func (c *Client) getPriorityClassDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	_, err := cs.SchedulingV1().PriorityClasses().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get priorityclass: %w", err)
	}

	pcID := nodeID("PriorityClass", "", name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:   pcID,
		Kind: "PriorityClass",
		Name: name,
	})

	// Find all pods using this PriorityClass (cluster-wide with caching)
	pods, err := cache.getAllPods()
	if err == nil {
		for _, pod := range pods {
			if pod.Spec.PriorityClassName == name {
				podID := nodeID("Pod", pod.Namespace, pod.Name)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:        podID,
					Kind:      "Pod",
					Name:      pod.Name,
					Namespace: pod.Namespace,
					Status:    string(pod.Status.Phase),
				})
				c.addEdge(graph, podID, pcID, "uses")

				// Resolve pod's owner refs
				c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, pod.Namespace, pod.OwnerReferences)
			}
		}
	}

	return graph, nil
}

// getNetworkPolicyDependencies resolves dependencies for a NetworkPolicy
func (c *Client) getNetworkPolicyDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	np, err := cs.NetworkingV1().NetworkPolicies(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get networkpolicy: %w", err)
	}

	npID := nodeID("NetworkPolicy", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        npID,
		Kind:      "NetworkPolicy",
		Name:      name,
		Namespace: namespace,
	})

	// Find pods that this policy applies to (via podSelector)
	if np.Spec.PodSelector.MatchLabels != nil || np.Spec.PodSelector.MatchExpressions != nil {
		pods, err := cache.getPods(namespace)
		if err == nil {
			for _, pod := range pods {
				if matchesLabelSelector(pod.Labels, &np.Spec.PodSelector) {
					podID := nodeID("Pod", namespace, pod.Name)
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        podID,
						Kind:      "Pod",
						Name:      pod.Name,
						Namespace: namespace,
						Status:    string(pod.Status.Phase),
					})
					c.addEdge(graph, npID, podID, "applies-to")

					// Resolve pod's owner refs
					c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, namespace, pod.OwnerReferences)
				}
			}
		}
	}

	return graph, nil
}

// getIngressDependencies resolves dependencies for an Ingress
func (c *Client) getIngressDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	ingress, err := cs.NetworkingV1().Ingresses(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ingress: %w", err)
	}

	ingressID := nodeID("Ingress", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        ingressID,
		Kind:      "Ingress",
		Name:      name,
		Namespace: namespace,
	})

	// Resolve IngressClass
	if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName != "" {
		icName := *ingress.Spec.IngressClassName
		icID := nodeID("IngressClass", "", icName)
		c.addNode(graph, nodeMap, DependencyNode{
			ID:   icID,
			Kind: "IngressClass",
			Name: icName,
		})
		c.addEdge(graph, ingressID, icID, "uses")
	}

	// Resolve TLS secrets
	for _, tls := range ingress.Spec.TLS {
		if tls.SecretName != "" {
			secretID := nodeID("Secret", namespace, tls.SecretName)
			c.addNode(graph, nodeMap, DependencyNode{
				ID:        secretID,
				Kind:      "Secret",
				Name:      tls.SecretName,
				Namespace: namespace,
			})
			c.addEdge(graph, ingressID, secretID, "uses")
		}
	}

	// Resolve default backend service
	if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
		svcName := ingress.Spec.DefaultBackend.Service.Name
		svcID := nodeID("Service", namespace, svcName)
		c.addNode(graph, nodeMap, DependencyNode{
			ID:        svcID,
			Kind:      "Service",
			Name:      svcName,
			Namespace: namespace,
		})
		c.addEdge(graph, ingressID, svcID, "routes-to")
	}

	// Resolve backend services from rules
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service != nil {
				svcName := path.Backend.Service.Name
				svcID := nodeID("Service", namespace, svcName)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:        svcID,
					Kind:      "Service",
					Name:      svcName,
					Namespace: namespace,
				})
				c.addEdge(graph, ingressID, svcID, "routes-to")
			}
		}
	}

	return graph, nil
}

// getIngressClassDependencies resolves dependencies for an IngressClass
func (c *Client) getIngressClassDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	_, err := cs.NetworkingV1().IngressClasses().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ingressclass: %w", err)
	}

	icID := nodeID("IngressClass", "", name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:   icID,
		Kind: "IngressClass",
		Name: name,
	})

	// Find all Ingresses using this IngressClass (cluster-wide with caching)
	ingresses, err := cache.getAllIngresses()
	if err == nil {
		for _, ingress := range ingresses {
			if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == name {
				ingressID := nodeID("Ingress", ingress.Namespace, ingress.Name)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:        ingressID,
					Kind:      "Ingress",
					Name:      ingress.Name,
					Namespace: ingress.Namespace,
				})
				c.addEdge(graph, ingressID, icID, "uses")
			}
		}
	}

	return graph, nil
}

// getStorageClassDependencies resolves dependencies for a StorageClass
func (c *Client) getStorageClassDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	_, err := cs.StorageV1().StorageClasses().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get storageclass: %w", err)
	}

	scID := nodeID("StorageClass", "", name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:   scID,
		Kind: "StorageClass",
		Name: name,
	})

	// Find all PVs using this StorageClass (cluster-wide)
	pvs, err := cs.CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err == nil {
		for _, pv := range pvs.Items {
			if pv.Spec.StorageClassName == name {
				pvID := nodeID("PersistentVolume", "", pv.Name)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:     pvID,
					Kind:   "PersistentVolume",
					Name:   pv.Name,
					Status: string(pv.Status.Phase),
				})
				c.addEdge(graph, pvID, scID, "uses")

				// If PV is bound, show the PVC
				if pv.Spec.ClaimRef != nil {
					pvcID := nodeID("PersistentVolumeClaim", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        pvcID,
						Kind:      "PersistentVolumeClaim",
						Name:      pv.Spec.ClaimRef.Name,
						Namespace: pv.Spec.ClaimRef.Namespace,
					})
					c.addEdge(graph, pvcID, pvID, "binds")
				}
			}
		}
	}

	// Find all PVCs using this StorageClass directly (cluster-wide)
	pvcs, err := cs.CoreV1().PersistentVolumeClaims("").List(context.TODO(), metav1.ListOptions{})
	if err == nil {
		for _, pvc := range pvcs.Items {
			if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName == name {
				pvcID := nodeID("PersistentVolumeClaim", pvc.Namespace, pvc.Name)
				if !nodeMap[pvcID] {
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        pvcID,
						Kind:      "PersistentVolumeClaim",
						Name:      pvc.Name,
						Namespace: pvc.Namespace,
						Status:    string(pvc.Status.Phase),
					})
					c.addEdge(graph, pvcID, scID, "uses")
				}
			}
		}
	}

	return graph, nil
}

// getServiceAccountDependencies resolves dependencies for a ServiceAccount
func (c *Client) getServiceAccountDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	_, err := cs.CoreV1().ServiceAccounts(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get serviceaccount: %w", err)
	}

	saID := nodeID("ServiceAccount", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        saID,
		Kind:      "ServiceAccount",
		Name:      name,
		Namespace: namespace,
	})

	// Find all Pods using this ServiceAccount (using cache)
	pods, err := cache.getPods(namespace)
	if err == nil {
		for _, pod := range pods {
			podSA := pod.Spec.ServiceAccountName
			if podSA == "" {
				podSA = "default"
			}
			if podSA == name {
				podID := nodeID("Pod", namespace, pod.Name)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:        podID,
					Kind:      "Pod",
					Name:      pod.Name,
					Namespace: namespace,
					Status:    string(pod.Status.Phase),
				})
				c.addEdge(graph, podID, saID, "uses")

				// Resolve pod's owner refs
				c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, namespace, pod.OwnerReferences)
				// Find Services selecting this pod
				c.findServicesSelectingPod(cache, graph, nodeMap, namespace, pod.Labels, podID)
			}
		}
	}

	return graph, nil
}

// getHPADependencies resolves dependencies for a HorizontalPodAutoscaler
func (c *Client) getHPADependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	hpa, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get hpa: %w", err)
	}

	hpaID := nodeID("HorizontalPodAutoscaler", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        hpaID,
		Kind:      "HorizontalPodAutoscaler",
		Name:      name,
		Namespace: namespace,
	})

	// Resolve the scale target (Deployment, StatefulSet, ReplicaSet, etc.)
	targetKind := hpa.Spec.ScaleTargetRef.Kind
	targetName := hpa.Spec.ScaleTargetRef.Name
	targetID := nodeID(targetKind, namespace, targetName)

	c.addNode(graph, nodeMap, DependencyNode{
		ID:        targetID,
		Kind:      targetKind,
		Name:      targetName,
		Namespace: namespace,
	})
	c.addEdge(graph, hpaID, targetID, "scales")

	// Resolve the target's dependencies based on kind
	switch targetKind {
	case "Deployment":
		deploy, err := cs.AppsV1().Deployments(namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
		if err == nil {
			// Find ReplicaSets owned by this deployment (using cache)
			rsList, err := cache.getReplicaSets(namespace)
			if err == nil {
				for _, rs := range rsList {
					for _, ref := range rs.OwnerReferences {
						if ref.Kind == "Deployment" && ref.Name == targetName {
							rsID := nodeID("ReplicaSet", namespace, rs.Name)
							c.addNode(graph, nodeMap, DependencyNode{
								ID:        rsID,
								Kind:      "ReplicaSet",
								Name:      rs.Name,
								Namespace: namespace,
							})
							c.addEdge(graph, targetID, rsID, "owns")
							c.findOwnedPods(cs, cache, graph, nodeMap, rsID, namespace, rs.Name, "ReplicaSet")
						}
					}
				}
			}
			c.findSelectingServices(cache, graph, nodeMap, namespace, deploy.Spec.Selector.MatchLabels)
		}
	case "StatefulSet":
		sts, err := cs.AppsV1().StatefulSets(namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
		if err == nil {
			c.findOwnedPods(cs, cache, graph, nodeMap, targetID, namespace, targetName, "StatefulSet")
			c.findSelectingServices(cache, graph, nodeMap, namespace, sts.Spec.Selector.MatchLabels)
		}
	case "ReplicaSet":
		rs, err := cs.AppsV1().ReplicaSets(namespace).Get(context.TODO(), targetName, metav1.GetOptions{})
		if err == nil {
			c.findOwnedPods(cs, cache, graph, nodeMap, targetID, namespace, targetName, "ReplicaSet")
			if rs.Spec.Selector != nil {
				c.findSelectingServices(cache, graph, nodeMap, namespace, rs.Spec.Selector.MatchLabels)
			}
		}
	}

	return graph, nil
}

// getPDBDependencies resolves dependencies for a PodDisruptionBudget
func (c *Client) getPDBDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	pdb, err := cs.PolicyV1().PodDisruptionBudgets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pdb: %w", err)
	}

	pdbID := nodeID("PodDisruptionBudget", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        pdbID,
		Kind:      "PodDisruptionBudget",
		Name:      name,
		Namespace: namespace,
	})

	// Find pods matching the PDB's selector (using cache)
	if pdb.Spec.Selector != nil {
		pods, err := cache.getPods(namespace)
		if err == nil {
			for _, pod := range pods {
				if matchesLabelSelector(pod.Labels, pdb.Spec.Selector) {
					podID := nodeID("Pod", namespace, pod.Name)
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        podID,
						Kind:      "Pod",
						Name:      pod.Name,
						Namespace: namespace,
						Status:    string(pod.Status.Phase),
					})
					c.addEdge(graph, pdbID, podID, "protects")

					// Resolve pod's owner refs
					c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, namespace, pod.OwnerReferences)
					// Find Services selecting this pod
					c.findServicesSelectingPod(cache, graph, nodeMap, namespace, pod.Labels, podID)
				}
			}
		}
	}

	return graph, nil
}
