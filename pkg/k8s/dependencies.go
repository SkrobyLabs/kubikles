package k8s

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DependencyNode represents a node in the dependency graph
type DependencyNode struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Status    string `json:"status,omitempty"`
}

// DependencyEdge represents an edge (relationship) in the dependency graph
type DependencyEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"` // "owns", "uses", "selects"
}

// DependencyGraph contains the full dependency information
type DependencyGraph struct {
	Nodes []DependencyNode `json:"nodes"`
	Edges []DependencyEdge `json:"edges"`
}

// GetResourceDependencies resolves all dependencies for a given resource
func (c *Client) GetResourceDependencies(contextName, resourceType, namespace, name string) (*DependencyGraph, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	graph := &DependencyGraph{
		Nodes: []DependencyNode{},
		Edges: []DependencyEdge{},
	}

	nodeMap := make(map[string]bool) // Track added nodes by ID

	switch resourceType {
	case "pod":
		return c.getPodDependencies(cs, namespace, name, graph, nodeMap)
	case "deployment":
		return c.getDeploymentDependencies(cs, contextName, namespace, name, graph, nodeMap)
	case "statefulset":
		return c.getStatefulSetDependencies(cs, contextName, namespace, name, graph, nodeMap)
	case "daemonset":
		return c.getDaemonSetDependencies(cs, contextName, namespace, name, graph, nodeMap)
	case "replicaset":
		return c.getReplicaSetDependencies(cs, contextName, namespace, name, graph, nodeMap)
	case "job":
		return c.getJobDependencies(cs, contextName, namespace, name, graph, nodeMap)
	case "cronjob":
		return c.getCronJobDependencies(cs, contextName, namespace, name, graph, nodeMap)
	case "pvc":
		return c.getPVCDependencies(cs, namespace, name, graph, nodeMap)
	case "pv":
		return c.getPVDependencies(cs, name, graph, nodeMap)
	case "configmap":
		return c.getConfigMapDependencies(cs, contextName, namespace, name, graph, nodeMap)
	case "secret":
		return c.getSecretDependencies(cs, contextName, namespace, name, graph, nodeMap)
	case "service":
		return c.getServiceDependencies(cs, contextName, namespace, name, graph, nodeMap)
	case "ingress":
		return c.getIngressDependencies(cs, contextName, namespace, name, graph, nodeMap)
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

func nodeID(kind, namespace, name string) string {
	if namespace == "" {
		return fmt.Sprintf("%s/%s", kind, name)
	}
	return fmt.Sprintf("%s/%s/%s", kind, namespace, name)
}

func (c *Client) addNode(graph *DependencyGraph, nodeMap map[string]bool, node DependencyNode) {
	if !nodeMap[node.ID] {
		graph.Nodes = append(graph.Nodes, node)
		nodeMap[node.ID] = true
	}
}

func (c *Client) addEdge(graph *DependencyGraph, source, target, relation string) {
	graph.Edges = append(graph.Edges, DependencyEdge{
		Source:   source,
		Target:   target,
		Relation: relation,
	})
}

// getPodDependencies resolves dependencies for a Pod
func (c *Client) getPodDependencies(cs kubernetes.Interface, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	pod, err := cs.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	podID := nodeID("Pod", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        podID,
		Kind:      "Pod",
		Name:      name,
		Namespace: namespace,
		Status:    string(pod.Status.Phase),
	})

	// Resolve owner references (upward)
	c.resolveOwnerRefs(cs, graph, nodeMap, podID, namespace, pod.OwnerReferences)

	// Resolve volume dependencies (downward)
	c.resolvePodVolumes(cs, graph, nodeMap, podID, namespace, pod.Spec.Volumes)

	// Resolve container env references
	c.resolvePodContainerRefs(graph, nodeMap, podID, namespace, pod.Spec.Containers)
	c.resolvePodContainerRefs(graph, nodeMap, podID, namespace, pod.Spec.InitContainers)

	return graph, nil
}

// resolveOwnerRefs traverses owner references upward
func (c *Client) resolveOwnerRefs(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, childID, namespace string, refs []metav1.OwnerReference) {
	for _, ref := range refs {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}

		ownerID := nodeID(ref.Kind, namespace, ref.Name)
		c.addNode(graph, nodeMap, DependencyNode{
			ID:        ownerID,
			Kind:      ref.Kind,
			Name:      ref.Name,
			Namespace: namespace,
		})
		c.addEdge(graph, ownerID, childID, "owns")

		// Recursively resolve parent's owner refs
		switch ref.Kind {
		case "ReplicaSet":
			rs, err := cs.AppsV1().ReplicaSets(namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
			if err == nil {
				c.resolveOwnerRefs(cs, graph, nodeMap, ownerID, namespace, rs.OwnerReferences)
			}
		case "Job":
			job, err := cs.BatchV1().Jobs(namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
			if err == nil {
				c.resolveOwnerRefs(cs, graph, nodeMap, ownerID, namespace, job.OwnerReferences)
			}
		}
	}
}

// resolvePodVolumes resolves PVC, ConfigMap, Secret volume references
func (c *Client) resolvePodVolumes(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, podID, namespace string, volumes []v1.Volume) {
	for _, vol := range volumes {
		// PVC references
		if vol.PersistentVolumeClaim != nil {
			pvcName := vol.PersistentVolumeClaim.ClaimName
			pvcID := nodeID("PersistentVolumeClaim", namespace, pvcName)

			pvc, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
			status := "Unknown"
			if err == nil {
				status = string(pvc.Status.Phase)
			}

			c.addNode(graph, nodeMap, DependencyNode{
				ID:        pvcID,
				Kind:      "PersistentVolumeClaim",
				Name:      pvcName,
				Namespace: namespace,
				Status:    status,
			})
			c.addEdge(graph, podID, pvcID, "uses")

			// Resolve PV from PVC
			if err == nil && pvc.Spec.VolumeName != "" {
				c.resolvePVFromPVC(cs, graph, nodeMap, pvcID, pvc.Spec.VolumeName, pvc.Spec.StorageClassName)
			}
		}

		// ConfigMap volume references
		if vol.ConfigMap != nil {
			cmName := vol.ConfigMap.Name
			cmID := nodeID("ConfigMap", namespace, cmName)
			c.addNode(graph, nodeMap, DependencyNode{
				ID:        cmID,
				Kind:      "ConfigMap",
				Name:      cmName,
				Namespace: namespace,
			})
			c.addEdge(graph, podID, cmID, "uses")
		}

		// Secret volume references
		if vol.Secret != nil {
			secretName := vol.Secret.SecretName
			secretID := nodeID("Secret", namespace, secretName)
			c.addNode(graph, nodeMap, DependencyNode{
				ID:        secretID,
				Kind:      "Secret",
				Name:      secretName,
				Namespace: namespace,
			})
			c.addEdge(graph, podID, secretID, "uses")
		}
	}
}

// resolvePVFromPVC adds PV and StorageClass nodes from a PVC
func (c *Client) resolvePVFromPVC(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, pvcID, pvName string, storageClassName *string) {
	pvID := nodeID("PersistentVolume", "", pvName)

	pv, err := cs.CoreV1().PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
	status := "Unknown"
	if err == nil {
		status = string(pv.Status.Phase)
	}

	c.addNode(graph, nodeMap, DependencyNode{
		ID:     pvID,
		Kind:   "PersistentVolume",
		Name:   pvName,
		Status: status,
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
func (c *Client) resolvePodContainerRefs(graph *DependencyGraph, nodeMap map[string]bool, podID, namespace string, containers []v1.Container) {
	for _, container := range containers {
		// envFrom references
		for _, envFrom := range container.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				cmID := nodeID("ConfigMap", namespace, envFrom.ConfigMapRef.Name)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:        cmID,
					Kind:      "ConfigMap",
					Name:      envFrom.ConfigMapRef.Name,
					Namespace: namespace,
				})
				c.addEdge(graph, podID, cmID, "uses")
			}
			if envFrom.SecretRef != nil {
				secretID := nodeID("Secret", namespace, envFrom.SecretRef.Name)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:        secretID,
					Kind:      "Secret",
					Name:      envFrom.SecretRef.Name,
					Namespace: namespace,
				})
				c.addEdge(graph, podID, secretID, "uses")
			}
		}

		// Individual env var references
		for _, env := range container.Env {
			if env.ValueFrom != nil {
				if env.ValueFrom.ConfigMapKeyRef != nil {
					cmID := nodeID("ConfigMap", namespace, env.ValueFrom.ConfigMapKeyRef.Name)
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        cmID,
						Kind:      "ConfigMap",
						Name:      env.ValueFrom.ConfigMapKeyRef.Name,
						Namespace: namespace,
					})
					c.addEdge(graph, podID, cmID, "uses")
				}
				if env.ValueFrom.SecretKeyRef != nil {
					secretID := nodeID("Secret", namespace, env.ValueFrom.SecretKeyRef.Name)
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        secretID,
						Kind:      "Secret",
						Name:      env.ValueFrom.SecretKeyRef.Name,
						Namespace: namespace,
					})
					c.addEdge(graph, podID, secretID, "uses")
				}
			}
		}
	}
}

// getDeploymentDependencies resolves dependencies for a Deployment
func (c *Client) getDeploymentDependencies(cs kubernetes.Interface, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	deploy, err := cs.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	deployID := nodeID("Deployment", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        deployID,
		Kind:      "Deployment",
		Name:      name,
		Namespace: namespace,
	})

	// Find owned ReplicaSets
	rsList, err := cs.AppsV1().ReplicaSets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err == nil {
		for _, rs := range rsList.Items {
			for _, ref := range rs.OwnerReferences {
				if ref.Kind == "Deployment" && ref.Name == name {
					rsID := nodeID("ReplicaSet", namespace, rs.Name)
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        rsID,
						Kind:      "ReplicaSet",
						Name:      rs.Name,
						Namespace: namespace,
					})
					c.addEdge(graph, deployID, rsID, "owns")

					// Find pods owned by this ReplicaSet
					c.findOwnedPods(cs, graph, nodeMap, rsID, namespace, rs.Name, "ReplicaSet")
				}
			}
		}
	}

	// Resolve Services that select this deployment's pods
	c.findSelectingServices(cs, graph, nodeMap, namespace, deploy.Spec.Selector.MatchLabels)

	return graph, nil
}

// getStatefulSetDependencies resolves dependencies for a StatefulSet
func (c *Client) getStatefulSetDependencies(cs kubernetes.Interface, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	sts, err := cs.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get statefulset: %w", err)
	}

	stsID := nodeID("StatefulSet", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        stsID,
		Kind:      "StatefulSet",
		Name:      name,
		Namespace: namespace,
	})

	// Find owned Pods
	c.findOwnedPods(cs, graph, nodeMap, stsID, namespace, name, "StatefulSet")

	// Resolve Services
	c.findSelectingServices(cs, graph, nodeMap, namespace, sts.Spec.Selector.MatchLabels)

	return graph, nil
}

// getDaemonSetDependencies resolves dependencies for a DaemonSet
func (c *Client) getDaemonSetDependencies(cs kubernetes.Interface, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	ds, err := cs.AppsV1().DaemonSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get daemonset: %w", err)
	}

	dsID := nodeID("DaemonSet", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        dsID,
		Kind:      "DaemonSet",
		Name:      name,
		Namespace: namespace,
	})

	// Find owned Pods
	c.findOwnedPods(cs, graph, nodeMap, dsID, namespace, name, "DaemonSet")

	// Resolve Services
	c.findSelectingServices(cs, graph, nodeMap, namespace, ds.Spec.Selector.MatchLabels)

	return graph, nil
}

// getReplicaSetDependencies resolves dependencies for a ReplicaSet
func (c *Client) getReplicaSetDependencies(cs kubernetes.Interface, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	rs, err := cs.AppsV1().ReplicaSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get replicaset: %w", err)
	}

	rsID := nodeID("ReplicaSet", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        rsID,
		Kind:      "ReplicaSet",
		Name:      name,
		Namespace: namespace,
	})

	// Resolve owner (Deployment)
	c.resolveOwnerRefs(cs, graph, nodeMap, rsID, namespace, rs.OwnerReferences)

	// Find owned Pods
	c.findOwnedPods(cs, graph, nodeMap, rsID, namespace, name, "ReplicaSet")

	return graph, nil
}

// getJobDependencies resolves dependencies for a Job
func (c *Client) getJobDependencies(cs kubernetes.Interface, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	job, err := cs.BatchV1().Jobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	jobID := nodeID("Job", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        jobID,
		Kind:      "Job",
		Name:      name,
		Namespace: namespace,
	})

	// Resolve owner (CronJob)
	c.resolveOwnerRefs(cs, graph, nodeMap, jobID, namespace, job.OwnerReferences)

	// Find owned Pods
	c.findOwnedPods(cs, graph, nodeMap, jobID, namespace, name, "Job")

	return graph, nil
}

// getCronJobDependencies resolves dependencies for a CronJob
func (c *Client) getCronJobDependencies(cs kubernetes.Interface, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	_, err := cs.BatchV1().CronJobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get cronjob: %w", err)
	}

	cronJobID := nodeID("CronJob", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        cronJobID,
		Kind:      "CronJob",
		Name:      name,
		Namespace: namespace,
	})

	// Find owned Jobs
	jobList, err := cs.BatchV1().Jobs(namespace).List(context.TODO(), metav1.ListOptions{})
	if err == nil {
		for _, job := range jobList.Items {
			for _, ref := range job.OwnerReferences {
				if ref.Kind == "CronJob" && ref.Name == name {
					jobID := nodeID("Job", namespace, job.Name)
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        jobID,
						Kind:      "Job",
						Name:      job.Name,
						Namespace: namespace,
					})
					c.addEdge(graph, cronJobID, jobID, "owns")

					// Find pods owned by this Job
					c.findOwnedPods(cs, graph, nodeMap, jobID, namespace, job.Name, "Job")
				}
			}
		}
	}

	return graph, nil
}

// getPVCDependencies resolves dependencies for a PVC
func (c *Client) getPVCDependencies(cs kubernetes.Interface, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	pvc, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pvc: %w", err)
	}

	pvcID := nodeID("PersistentVolumeClaim", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        pvcID,
		Kind:      "PersistentVolumeClaim",
		Name:      name,
		Namespace: namespace,
		Status:    string(pvc.Status.Phase),
	})

	// Find pods using this PVC
	pods, err := cs.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			for _, vol := range pod.Spec.Volumes {
				if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == name {
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
					c.resolveOwnerRefs(cs, graph, nodeMap, podID, namespace, pod.OwnerReferences)
				}
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
func (c *Client) getPVDependencies(cs kubernetes.Interface, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	pv, err := cs.CoreV1().PersistentVolumes().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pv: %w", err)
	}

	pvID := nodeID("PersistentVolume", "", name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:     pvID,
		Kind:   "PersistentVolume",
		Name:   name,
		Status: string(pv.Status.Phase),
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

		// Find pods using this PVC
		if err == nil {
			pods, err := cs.CoreV1().Pods(pvcNamespace).List(context.TODO(), metav1.ListOptions{})
			if err == nil {
				for _, pod := range pods.Items {
					for _, vol := range pod.Spec.Volumes {
						if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == pvcName {
							podID := nodeID("Pod", pvcNamespace, pod.Name)
							c.addNode(graph, nodeMap, DependencyNode{
								ID:        podID,
								Kind:      "Pod",
								Name:      pod.Name,
								Namespace: pvcNamespace,
								Status:    string(pod.Status.Phase),
							})
							c.addEdge(graph, podID, pvcID, "uses")
							c.resolveOwnerRefs(cs, graph, nodeMap, podID, pvcNamespace, pod.OwnerReferences)
						}
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
func (c *Client) getConfigMapDependencies(cs kubernetes.Interface, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	_, err := cs.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap: %w", err)
	}

	cmID := nodeID("ConfigMap", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        cmID,
		Kind:      "ConfigMap",
		Name:      name,
		Namespace: namespace,
	})

	// Find pods using this ConfigMap
	c.findPodsUsingConfigMap(cs, graph, nodeMap, namespace, name, cmID)

	return graph, nil
}

// getSecretDependencies resolves dependencies for a Secret
func (c *Client) getSecretDependencies(cs kubernetes.Interface, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	_, err := cs.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	secretID := nodeID("Secret", namespace, name)
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        secretID,
		Kind:      "Secret",
		Name:      name,
		Namespace: namespace,
	})

	// Find pods using this Secret
	c.findPodsUsingSecret(cs, graph, nodeMap, namespace, name, secretID)

	return graph, nil
}

// getServiceDependencies resolves dependencies for a Service
func (c *Client) getServiceDependencies(cs kubernetes.Interface, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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

	// Find pods matching selector
	if len(svc.Spec.Selector) > 0 {
		pods, err := cs.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
		if err == nil {
			for _, pod := range pods.Items {
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
					c.resolveOwnerRefs(cs, graph, nodeMap, podID, namespace, pod.OwnerReferences)
				}
			}
		}
	}

	// Find Ingresses that route to this Service
	c.findIngressesForService(cs, graph, nodeMap, namespace, name, svcID)

	return graph, nil
}

// findIngressesForService finds all Ingresses that route to a given Service
func (c *Client) findIngressesForService(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, namespace, serviceName, svcID string) {
	ingresses, err := cs.NetworkingV1().Ingresses(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, ingress := range ingresses.Items {
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
		}
	}
}

// Helper functions

func (c *Client) findOwnedPods(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, ownerID, namespace, ownerName, ownerKind string) {
	pods, err := cs.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, pod := range pods.Items {
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == ownerKind && ref.Name == ownerName {
				podID := nodeID("Pod", namespace, pod.Name)
				c.addNode(graph, nodeMap, DependencyNode{
					ID:        podID,
					Kind:      "Pod",
					Name:      pod.Name,
					Namespace: namespace,
					Status:    string(pod.Status.Phase),
				})
				c.addEdge(graph, ownerID, podID, "owns")

				// Resolve pod's downward dependencies (volumes, configs)
				c.resolvePodVolumes(cs, graph, nodeMap, podID, namespace, pod.Spec.Volumes)
				c.resolvePodContainerRefs(graph, nodeMap, podID, namespace, pod.Spec.Containers)
				c.resolvePodContainerRefs(graph, nodeMap, podID, namespace, pod.Spec.InitContainers)
			}
		}
	}
}

func (c *Client) findSelectingServices(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, namespace string, podLabels map[string]string) {
	if len(podLabels) == 0 {
		return
	}

	services, err := cs.CoreV1().Services(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, svc := range services.Items {
		if len(svc.Spec.Selector) > 0 && matchesSelector(podLabels, svc.Spec.Selector) {
			svcID := nodeID("Service", namespace, svc.Name)
			c.addNode(graph, nodeMap, DependencyNode{
				ID:        svcID,
				Kind:      "Service",
				Name:      svc.Name,
				Namespace: namespace,
			})
			// Service selects the workload's pods - find a pod to link to
			for id := range nodeMap {
				if len(id) > 4 && id[:4] == "Pod/" {
					c.addEdge(graph, svcID, id, "selects")
					break
				}
			}

			// Find Ingresses that route to this Service
			c.findIngressesForService(cs, graph, nodeMap, namespace, svc.Name, svcID)
		}
	}
}

func (c *Client) findPodsUsingConfigMap(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, namespace, cmName, cmID string) {
	pods, err := cs.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, pod := range pods.Items {
		usesConfigMap := false

		// Check volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil && vol.ConfigMap.Name == cmName {
				usesConfigMap = true
				break
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
			c.resolveOwnerRefs(cs, graph, nodeMap, podID, namespace, pod.OwnerReferences)
		}
	}
}

func (c *Client) findPodsUsingSecret(cs kubernetes.Interface, graph *DependencyGraph, nodeMap map[string]bool, namespace, secretName, secretID string) {
	pods, err := cs.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, pod := range pods.Items {
		usesSecret := false

		// Check volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil && vol.Secret.SecretName == secretName {
				usesSecret = true
				break
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
			c.resolveOwnerRefs(cs, graph, nodeMap, podID, namespace, pod.OwnerReferences)
		}
	}
}

func matchesSelector(labels, selector map[string]string) bool {
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

// getIngressDependencies resolves dependencies for an Ingress
func (c *Client) getIngressDependencies(cs kubernetes.Interface, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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
