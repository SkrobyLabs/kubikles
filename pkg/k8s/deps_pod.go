// Code split from dependencies.go; see that file for the graph types and entry point.
package k8s

import (
	"context"
	"fmt"
	"strconv"

	"kubikles/pkg/debug"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
)

// getPodDependencies resolves dependencies for a Pod
func (c *Client) getPodDependencies(cs kubernetes.Interface, cache *resourceCache, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	pod, err := cs.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	podID := nodeID("Pod", namespace, name)
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
		Name:      name,
		Namespace: namespace,
		Status:    string(pod.Status.Phase),
		Metadata:  podMeta,
	})

	// Resolve owner references (upward)
	c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, namespace, pod.OwnerReferences)

	// Resolve volume dependencies (downward)
	c.resolvePodVolumes(cs, graph, nodeMap, podID, name, namespace, pod.Spec.Volumes)

	// Resolve container env references
	c.resolvePodContainerRefs(cs, graph, nodeMap, podID, namespace, pod.Spec.Containers)
	c.resolvePodContainerRefs(cs, graph, nodeMap, podID, namespace, pod.Spec.InitContainers)

	// Find Services that select this Pod (and their Ingresses)
	c.findServicesSelectingPod(cache, graph, nodeMap, namespace, pod.Labels, podID)

	// Resolve ServiceAccount (skip "default" as it's not interesting)
	saName := pod.Spec.ServiceAccountName
	if saName != "" && saName != "default" {
		saID := nodeID("ServiceAccount", namespace, saName)
		c.addNode(graph, nodeMap, DependencyNode{
			ID:        saID,
			Kind:      "ServiceAccount",
			Name:      saName,
			Namespace: namespace,
		})
		c.addEdge(graph, podID, saID, "uses")
	}

	return graph, nil
}

// resolveOwnerRefs traverses owner references upward
func (c *Client) resolveOwnerRefs(cs kubernetes.Interface, cache *resourceCache, graph *DependencyGraph, nodeMap map[string]bool, childID, namespace string, refs []metav1.OwnerReference) {
	for _, ref := range refs {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}

		ownerID := nodeID(ref.Kind, namespace, ref.Name)

		// Fetch owner resource to get metadata (replicas, etc.)
		var metadata map[string]string
		var ownerRefs []metav1.OwnerReference

		switch ref.Kind {
		case "Deployment":
			if deploy, err := cs.AppsV1().Deployments(namespace).Get(cache.ctx, ref.Name, metav1.GetOptions{}); err == nil {
				metadata = map[string]string{
					"replicas": fmt.Sprintf("%d/%d", deploy.Status.ReadyReplicas, deploy.Status.Replicas),
				}
			}
		case "StatefulSet":
			if sts, err := cs.AppsV1().StatefulSets(namespace).Get(cache.ctx, ref.Name, metav1.GetOptions{}); err == nil {
				metadata = map[string]string{
					"replicas": fmt.Sprintf("%d/%d", sts.Status.ReadyReplicas, sts.Status.Replicas),
				}
			}
		case "DaemonSet":
			if ds, err := cs.AppsV1().DaemonSets(namespace).Get(cache.ctx, ref.Name, metav1.GetOptions{}); err == nil {
				metadata = map[string]string{
					"replicas": fmt.Sprintf("%d/%d", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled),
				}
			}
		case "ReplicaSet":
			// Use O(1) name-indexed lookup
			if rs, err := cache.getReplicaSetByName(namespace, ref.Name); err == nil && rs != nil {
				metadata = map[string]string{
					"replicas": fmt.Sprintf("%d/%d", rs.Status.ReadyReplicas, rs.Status.Replicas),
				}
				ownerRefs = rs.OwnerReferences
			}
		case "Job":
			// Use O(1) name-indexed lookup
			if job, err := cache.getJobByName(namespace, ref.Name); err == nil && job != nil {
				completions := int32(1)
				if job.Spec.Completions != nil {
					completions = *job.Spec.Completions
				}
				metadata = map[string]string{
					"completions": fmt.Sprintf("%d/%d", job.Status.Succeeded, completions),
				}
				ownerRefs = job.OwnerReferences
			}
		case "CronJob":
			// CronJobs don't have replica-like metadata
		default:
			// CRD/unknown owner type — resolve via dynamic client
			if gvr, err := cache.resolveGVR(ref.APIVersion, ref.Kind); err == nil {
				if obj, err := cache.getCRDResource(gvr, namespace, ref.Name); err == nil {
					metadata = extractCRDMetadata(obj)
					// Extract owner references from the unstructured resource for recursive resolution
					ownerRefs = extractOwnerRefs(obj)
				} else {
					debug.LogK8s("CRD owner fetch failed, using ownerRef info only", map[string]interface{}{
						"kind": ref.Kind, "name": ref.Name, "apiVersion": ref.APIVersion, "error": err.Error(),
					})
				}
			} else {
				debug.LogK8s("GVR resolution failed for CRD owner", map[string]interface{}{
					"kind": ref.Kind, "apiVersion": ref.APIVersion, "error": err.Error(),
				})
			}
		}

		c.addNode(graph, nodeMap, DependencyNode{
			ID:        ownerID,
			Kind:      ref.Kind,
			Name:      ref.Name,
			Namespace: namespace,
			Metadata:  metadata,
		})
		c.addEdge(graph, ownerID, childID, "owns")

		// Recursively resolve parent's owner refs
		if len(ownerRefs) > 0 {
			c.resolveOwnerRefs(cs, cache, graph, nodeMap, ownerID, namespace, ownerRefs)
		}
	}
}

// extractCRDMetadata extracts useful metadata from an unstructured CRD resource.
func extractCRDMetadata(obj *unstructured.Unstructured) map[string]string {
	meta := make(map[string]string)

	// Extract replicas if present (common in controllers like ArgoCD Rollout)
	if spec, ok := obj.Object["spec"].(map[string]interface{}); ok {
		if replicas, ok := spec["replicas"]; ok {
			meta["replicas"] = fmt.Sprintf("%v", replicas)
		}
	}

	// Extract status phase/conditions if present
	if status, ok := obj.Object["status"].(map[string]interface{}); ok {
		if phase, ok := status["phase"].(string); ok {
			meta["status"] = phase
		}
		// Check for ready replicas (common CRD pattern)
		if readyReplicas, ok := status["readyReplicas"]; ok {
			if replicas, exists := meta["replicas"]; exists {
				meta["replicas"] = fmt.Sprintf("%v/%s", readyReplicas, replicas)
			}
		}
		// Check for health status (ArgoCD pattern)
		if health, ok := status["health"].(map[string]interface{}); ok {
			if healthStatus, ok := health["status"].(string); ok {
				meta["status"] = healthStatus
			}
		}
	}

	return meta
}

// extractOwnerRefs extracts owner references from an unstructured resource.
func extractOwnerRefs(obj *unstructured.Unstructured) []metav1.OwnerReference {
	refs := obj.GetOwnerReferences()
	if len(refs) == 0 {
		return nil
	}
	return refs
}

// resolvePodVolumes resolves PVC, ephemeral, ConfigMap, Secret and projected volume references
func (c *Client) resolvePodVolumes(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, podID, podName, namespace string, volumes []v1.Volume) {
	debug.LogK8s("resolvePodVolumes called", map[string]interface{}{"volumeCount": len(volumes), "podID": podID})
	for _, vol := range volumes {
		// PVC references — direct claims and generic ephemeral volumes
		// (the ephemeral volume controller creates a PVC named <pod>-<volume>)
		pvcName := ""
		if vol.PersistentVolumeClaim != nil {
			pvcName = vol.PersistentVolumeClaim.ClaimName
		} else if vol.Ephemeral != nil {
			pvcName = podName + "-" + vol.Name
		}
		if pvcName != "" {
			pvcID := nodeID("PersistentVolumeClaim", namespace, pvcName)
			debug.LogK8s("Processing PVC volume", map[string]interface{}{"pvcName": pvcName})

			pvc, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
			status := "Unknown"
			var pvcMeta map[string]string
			if err == nil {
				status = string(pvc.Status.Phase)
				debug.LogK8s("PVC fetched successfully", map[string]interface{}{"pvcName": pvcName, "status": status})
				// Get capacity from status (actual) or spec (requested)
				if capacity, ok := pvc.Status.Capacity[v1.ResourceStorage]; ok {
					pvcMeta = map[string]string{"capacity": capacity.String()}
				} else if req, ok := pvc.Spec.Resources.Requests[v1.ResourceStorage]; ok {
					pvcMeta = map[string]string{"capacity": req.String()}
				}
			} else {
				debug.LogK8s("PVC fetch error", map[string]interface{}{"pvcName": pvcName, "error": err.Error()})
			}

			c.addNode(graph, nodeMap, DependencyNode{
				ID:        pvcID,
				Kind:      "PersistentVolumeClaim",
				Name:      pvcName,
				Namespace: namespace,
				Status:    status,
				Metadata:  pvcMeta,
			})
			c.addEdge(graph, podID, pvcID, "uses")
			debug.LogK8s("Added PVC node", map[string]interface{}{"pvcID": pvcID, "nodeCount": len(graph.Nodes)})

			// Resolve PV from PVC
			if err == nil && pvc.Spec.VolumeName != "" {
				c.resolvePVFromPVC(cs, graph, nodeMap, pvcID, pvc.Spec.VolumeName, pvc.Spec.StorageClassName)
			}
		}

		// ConfigMap volume references
		if vol.ConfigMap != nil {
			c.addConfigMapNode(cs, graph, nodeMap, podID, namespace, vol.ConfigMap.Name)
		}

		// Secret volume references
		if vol.Secret != nil {
			c.addSecretNode(cs, graph, nodeMap, podID, namespace, vol.Secret.SecretName)
		}

		// Projected volume references (configMap/secret sources)
		if vol.Projected != nil {
			for _, src := range vol.Projected.Sources {
				if src.ConfigMap != nil {
					c.addConfigMapNode(cs, graph, nodeMap, podID, namespace, src.ConfigMap.Name)
				}
				if src.Secret != nil {
					c.addSecretNode(cs, graph, nodeMap, podID, namespace, src.Secret.Name)
				}
			}
		}
	}
}

// addConfigMapNode adds a ConfigMap node (with key-count metadata) and a "uses" edge from the pod
func (c *Client) addConfigMapNode(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, podID, namespace, cmName string) {
	cmID := nodeID("ConfigMap", namespace, cmName)
	var cmMeta map[string]string
	if cm, err := cs.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{}); err == nil {
		keyCount := len(cm.Data) + len(cm.BinaryData)
		cmMeta = map[string]string{"keys": strconv.Itoa(keyCount)}
	}
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        cmID,
		Kind:      "ConfigMap",
		Name:      cmName,
		Namespace: namespace,
		Metadata:  cmMeta,
	})
	c.addEdge(graph, podID, cmID, "uses")
}

// addSecretNode adds a Secret node (with key-count metadata) and a "uses" edge from the pod
func (c *Client) addSecretNode(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, podID, namespace, secretName string) {
	secretID := nodeID("Secret", namespace, secretName)
	var secretMeta map[string]string
	if secret, err := cs.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{}); err == nil {
		keyCount := len(secret.Data) + len(secret.StringData)
		secretMeta = map[string]string{"keys": strconv.Itoa(keyCount)}
	}
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        secretID,
		Kind:      "Secret",
		Name:      secretName,
		Namespace: namespace,
		Metadata:  secretMeta,
	})
	c.addEdge(graph, podID, secretID, "uses")
}

// resolvePVFromPVC adds PV and StorageClass nodes from a PVC
func (c *Client) resolvePVFromPVC(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, pvcID, pvName string, storageClassName *string) {
	pvID := nodeID("PersistentVolume", "", pvName)

	pv, err := cs.CoreV1().PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
	status := "Unknown"
	var pvMeta map[string]string
	if err == nil {
		status = string(pv.Status.Phase)
		if capacity, ok := pv.Spec.Capacity[v1.ResourceStorage]; ok {
			pvMeta = map[string]string{"capacity": capacity.String()}
		}
	}

	c.addNode(graph, nodeMap, DependencyNode{
		ID:       pvID,
		Kind:     "PersistentVolume",
		Name:     pvName,
		Status:   status,
		Metadata: pvMeta,
	})
	c.addEdge(graph, pvcID, pvID, "binds")

	// StorageClass from PVC spec or PV
	scName := ""
	if storageClassName != nil && *storageClassName != "" {
		scName = *storageClassName
	} else if err == nil && pv.Spec.StorageClassName != "" {
		scName = pv.Spec.StorageClassName
	}

	if scName != "" {
		scID := nodeID("StorageClass", "", scName)
		c.addNode(graph, nodeMap, DependencyNode{
			ID:   scID,
			Kind: "StorageClass",
			Name: scName,
		})
		c.addEdge(graph, pvID, scID, "uses")
	}
}

// resolvePodContainerRefs resolves ConfigMap/Secret references from container env vars
func (c *Client) resolvePodContainerRefs(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, podID, namespace string, containers []v1.Container) {
	for _, container := range containers {
		// envFrom references
		for _, envFrom := range container.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				c.addConfigMapNode(cs, graph, nodeMap, podID, namespace, envFrom.ConfigMapRef.Name)
			}
			if envFrom.SecretRef != nil {
				c.addSecretNode(cs, graph, nodeMap, podID, namespace, envFrom.SecretRef.Name)
			}
		}

		// Individual env var references
		for _, env := range container.Env {
			if env.ValueFrom != nil {
				if env.ValueFrom.ConfigMapKeyRef != nil {
					c.addConfigMapNode(cs, graph, nodeMap, podID, namespace, env.ValueFrom.ConfigMapKeyRef.Name)
				}
				if env.ValueFrom.SecretKeyRef != nil {
					c.addSecretNode(cs, graph, nodeMap, podID, namespace, env.ValueFrom.SecretKeyRef.Name)
				}
			}
		}
	}
}
