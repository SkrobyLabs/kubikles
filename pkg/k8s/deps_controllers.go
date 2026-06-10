// Code split from dependencies.go; see that file for the graph types and entry point.
package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// getDeploymentDependencies resolves dependencies for a Deployment
func (c *Client) getDeploymentDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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
		Metadata: map[string]string{
			"replicas": fmt.Sprintf("%d/%d", deploy.Status.ReadyReplicas, deploy.Status.Replicas),
		},
	})

	// Find owned ReplicaSets (using cache)
	rsList, err := cache.getReplicaSets(namespace)
	if err == nil {
		for _, rs := range rsList {
			for _, ref := range rs.OwnerReferences {
				if ref.Kind == "Deployment" && ref.Name == name {
					rsID := nodeID("ReplicaSet", namespace, rs.Name)
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        rsID,
						Kind:      "ReplicaSet",
						Name:      rs.Name,
						Namespace: namespace,
						Metadata: map[string]string{
							"replicas": fmt.Sprintf("%d/%d", rs.Status.ReadyReplicas, rs.Status.Replicas),
						},
					})
					c.addEdge(graph, deployID, rsID, "owns")

					// Find pods owned by this ReplicaSet
					c.findOwnedPods(cs, cache, graph, nodeMap, rsID, namespace, rs.Name, "ReplicaSet")
				}
			}
		}
	}

	// Resolve Services that select this deployment's pods
	c.findSelectingServices(cache, graph, nodeMap, namespace, deploy.Spec.Selector.MatchLabels)

	return graph, nil
}

// getStatefulSetDependencies resolves dependencies for a StatefulSet
func (c *Client) getStatefulSetDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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
		Metadata: map[string]string{
			"replicas": fmt.Sprintf("%d/%d", sts.Status.ReadyReplicas, sts.Status.Replicas),
		},
	})

	// Find owned Pods
	c.findOwnedPods(cs, cache, graph, nodeMap, stsID, namespace, name, "StatefulSet")

	// Resolve Services
	c.findSelectingServices(cache, graph, nodeMap, namespace, sts.Spec.Selector.MatchLabels)

	return graph, nil
}

// getDaemonSetDependencies resolves dependencies for a DaemonSet
func (c *Client) getDaemonSetDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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
		Metadata: map[string]string{
			"replicas": fmt.Sprintf("%d/%d", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled),
		},
	})

	// Find owned Pods
	c.findOwnedPods(cs, cache, graph, nodeMap, dsID, namespace, name, "DaemonSet")

	// Resolve Services
	c.findSelectingServices(cache, graph, nodeMap, namespace, ds.Spec.Selector.MatchLabels)

	return graph, nil
}

// getReplicaSetDependencies resolves dependencies for a ReplicaSet
func (c *Client) getReplicaSetDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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
		Metadata: map[string]string{
			"replicas": fmt.Sprintf("%d/%d", rs.Status.ReadyReplicas, rs.Status.Replicas),
		},
	})

	// Resolve owner (Deployment)
	c.resolveOwnerRefs(cs, cache, graph, nodeMap, rsID, namespace, rs.OwnerReferences)

	// Find owned Pods
	c.findOwnedPods(cs, cache, graph, nodeMap, rsID, namespace, name, "ReplicaSet")

	// Find Services that select these pods (and their Ingresses)
	if rs.Spec.Selector != nil {
		c.findSelectingServices(cache, graph, nodeMap, namespace, rs.Spec.Selector.MatchLabels)
	}

	return graph, nil
}

// getJobDependencies resolves dependencies for a Job
func (c *Client) getJobDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
	job, err := cs.BatchV1().Jobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	jobID := nodeID("Job", namespace, name)
	completions := int32(1)
	if job.Spec.Completions != nil {
		completions = *job.Spec.Completions
	}
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        jobID,
		Kind:      "Job",
		Name:      name,
		Namespace: namespace,
		Metadata: map[string]string{
			"completions": fmt.Sprintf("%d/%d", job.Status.Succeeded, completions),
		},
	})

	// Resolve owner (CronJob)
	c.resolveOwnerRefs(cs, cache, graph, nodeMap, jobID, namespace, job.OwnerReferences)

	// Find owned Pods
	c.findOwnedPods(cs, cache, graph, nodeMap, jobID, namespace, name, "Job")

	// Find Services that select job pods (and their Ingresses)
	if job.Spec.Selector != nil {
		c.findSelectingServices(cache, graph, nodeMap, namespace, job.Spec.Selector.MatchLabels)
	}

	return graph, nil
}

// getCronJobDependencies resolves dependencies for a CronJob
func (c *Client) getCronJobDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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

	// Find owned Jobs (using cache)
	jobList, err := cache.getJobs(namespace)
	if err == nil {
		for _, job := range jobList {
			for _, ref := range job.OwnerReferences {
				if ref.Kind == "CronJob" && ref.Name == name {
					jobID := nodeID("Job", namespace, job.Name)
					completions := int32(1)
					if job.Spec.Completions != nil {
						completions = *job.Spec.Completions
					}
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        jobID,
						Kind:      "Job",
						Name:      job.Name,
						Namespace: namespace,
						Metadata: map[string]string{
							"completions": fmt.Sprintf("%d/%d", job.Status.Succeeded, completions),
						},
					})
					c.addEdge(graph, cronJobID, jobID, "owns")

					// Find pods owned by this Job
					c.findOwnedPods(cs, cache, graph, nodeMap, jobID, namespace, job.Name, "Job")
				}
			}
		}
	}

	return graph, nil
}
