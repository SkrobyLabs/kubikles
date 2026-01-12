package k8s

import (
	"context"
	"fmt"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// dependencyAPITimeout is the timeout for Kubernetes API calls during dependency resolution.
// This prevents UI hangs when clusters are slow or unresponsive.
const dependencyAPITimeout = 30 * time.Second

// Relation type lookups - package level to avoid allocation per call.
// childToParentRelations: edge goes FROM child TO parent (child uses/binds parent)
// parentToChildRelations: edge goes FROM parent TO child (parent owns/selects child)
var (
	childToParentRelations = map[string]bool{
		"uses":       true,
		"binds":      true,
		"references": true,
	}
	parentToChildRelations = map[string]bool{
		"owns":       true,
		"selects":    true,
		"routes-to":  true,
		"scales":     true,
		"protects":   true,
		"applies-to": true,
	}
)

// resourceCache provides request-scoped caching for Kubernetes resources
// to avoid duplicate API calls within a single GetResourceDependencies call
type resourceCache struct {
	pods         map[string][]v1.Pod               // namespace -> pods
	services     map[string][]v1.Service           // namespace -> services
	ingresses    map[string][]networkingv1.Ingress // namespace -> ingresses
	replicaSets  map[string][]appsv1.ReplicaSet    // namespace -> replicasets
	jobs         map[string][]batchv1.Job          // namespace -> jobs
	// Cluster-wide caches for cluster-scoped resource queries
	allPods            []v1.Pod
	allPodsCached      bool
	allIngresses       []networkingv1.Ingress
	allIngressesCached bool
	ctx                context.Context // Request context with timeout
	cs                 kubernetes.Interface
}

func newResourceCache(ctx context.Context, cs kubernetes.Interface) *resourceCache {
	return &resourceCache{
		pods:        make(map[string][]v1.Pod),
		services:    make(map[string][]v1.Service),
		ingresses:   make(map[string][]networkingv1.Ingress),
		replicaSets: make(map[string][]appsv1.ReplicaSet),
		jobs:        make(map[string][]batchv1.Job),
		ctx:         ctx,
		cs:          cs,
	}
}

func (rc *resourceCache) getPods(namespace string) ([]v1.Pod, error) {
	if cached, ok := rc.pods[namespace]; ok {
		return cached, nil
	}
	list, err := rc.cs.CoreV1().Pods(namespace).List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.pods[namespace] = list.Items
	return list.Items, nil
}

func (rc *resourceCache) getServices(namespace string) ([]v1.Service, error) {
	if cached, ok := rc.services[namespace]; ok {
		return cached, nil
	}
	list, err := rc.cs.CoreV1().Services(namespace).List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.services[namespace] = list.Items
	return list.Items, nil
}

func (rc *resourceCache) getIngresses(namespace string) ([]networkingv1.Ingress, error) {
	if cached, ok := rc.ingresses[namespace]; ok {
		return cached, nil
	}
	list, err := rc.cs.NetworkingV1().Ingresses(namespace).List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.ingresses[namespace] = list.Items
	return list.Items, nil
}

func (rc *resourceCache) getReplicaSets(namespace string) ([]appsv1.ReplicaSet, error) {
	if cached, ok := rc.replicaSets[namespace]; ok {
		return cached, nil
	}
	list, err := rc.cs.AppsV1().ReplicaSets(namespace).List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.replicaSets[namespace] = list.Items
	return list.Items, nil
}

func (rc *resourceCache) getJobs(namespace string) ([]batchv1.Job, error) {
	if cached, ok := rc.jobs[namespace]; ok {
		return cached, nil
	}
	list, err := rc.cs.BatchV1().Jobs(namespace).List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.jobs[namespace] = list.Items
	return list.Items, nil
}

// getAllPods returns all pods cluster-wide with caching
// Used for cluster-scoped resources like PriorityClass
func (rc *resourceCache) getAllPods() ([]v1.Pod, error) {
	if rc.allPodsCached {
		return rc.allPods, nil
	}
	list, err := rc.cs.CoreV1().Pods("").List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.allPods = list.Items
	rc.allPodsCached = true
	return list.Items, nil
}

// getAllIngresses returns all ingresses cluster-wide with caching
// Used for cluster-scoped resources like IngressClass
func (rc *resourceCache) getAllIngresses() ([]networkingv1.Ingress, error) {
	if rc.allIngressesCached {
		return rc.allIngresses, nil
	}
	list, err := rc.cs.NetworkingV1().Ingresses("").List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.allIngresses = list.Items
	rc.allIngressesCached = true
	return list.Items, nil
}

// DependencyNode represents a node in the dependency graph
type DependencyNode struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Status    string `json:"status,omitempty"`
	// Summary node fields
	IsSummary      bool `json:"isSummary,omitempty"`
	RemainingCount int  `json:"remainingCount,omitempty"`
	ParentID       string `json:"parentId,omitempty"` // For expansion context
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

// limiterKey is a struct key for NodeLimiter maps (avoids string concat allocations)
type limiterKey struct{ parentID, kind string }

// NodeLimiter tracks how many nodes of each type have been added per parent
type NodeLimiter struct {
	counts  map[limiterKey]int
	limit   int
	skipped map[limiterKey][]DependencyNode
}

const DefaultNodeLimit = 5
const ExpansionLimit = 10

func newNodeLimiter(limit int) *NodeLimiter {
	return &NodeLimiter{
		counts:  make(map[limiterKey]int),
		limit:   limit,
		skipped: make(map[limiterKey][]DependencyNode),
	}
}

func (l *NodeLimiter) canAdd(parentID, kind string) bool {
	return l.counts[limiterKey{parentID, kind}] < l.limit
}

func (l *NodeLimiter) add(parentID, kind string) {
	l.counts[limiterKey{parentID, kind}]++
}

func (l *NodeLimiter) skip(parentID, kind string, node DependencyNode) {
	key := limiterKey{parentID, kind}
	l.skipped[key] = append(l.skipped[key], node)
}

func (l *NodeLimiter) getSkippedCount(parentID, kind string) int {
	return len(l.skipped[limiterKey{parentID, kind}])
}

// GetResourceDependencies resolves all dependencies for a given resource
func (c *Client) GetResourceDependencies(contextName, resourceType, namespace, name string) (*DependencyGraph, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Create context with timeout to prevent UI hangs on slow clusters
	ctx, cancel := context.WithTimeout(context.Background(), dependencyAPITimeout)
	defer cancel()

	graph := &DependencyGraph{
		Nodes: make([]DependencyNode, 0, 32),
		Edges: make([]DependencyEdge, 0, 32),
	}

	nodeMap := make(map[string]bool, 32) // Track added nodes by ID, pre-sized for typical graph
	cache := newResourceCache(ctx, cs)    // Request-scoped cache for List() results

	var result *DependencyGraph

	switch resourceType {
	case "pod":
		result, err = c.getPodDependencies(cs, cache, namespace, name, graph, nodeMap)
	case "deployment":
		result, err = c.getDeploymentDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "statefulset":
		result, err = c.getStatefulSetDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "daemonset":
		result, err = c.getDaemonSetDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "replicaset":
		result, err = c.getReplicaSetDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "job":
		result, err = c.getJobDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "cronjob":
		result, err = c.getCronJobDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "pvc":
		result, err = c.getPVCDependencies(cs, cache, namespace, name, graph, nodeMap)
	case "pv":
		result, err = c.getPVDependencies(cs, cache, name, graph, nodeMap)
	case "configmap":
		result, err = c.getConfigMapDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "secret":
		result, err = c.getSecretDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "service":
		result, err = c.getServiceDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "ingress":
		result, err = c.getIngressDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "endpoints":
		result, err = c.getEndpointsDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "priorityclass":
		result, err = c.getPriorityClassDependencies(cs, cache, contextName, name, graph, nodeMap)
	case "networkpolicy":
		result, err = c.getNetworkPolicyDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "ingressclass":
		result, err = c.getIngressClassDependencies(cs, cache, contextName, name, graph, nodeMap)
	case "storageclass":
		result, err = c.getStorageClassDependencies(cs, cache, contextName, name, graph, nodeMap)
	case "serviceaccount":
		result, err = c.getServiceAccountDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "hpa":
		result, err = c.getHPADependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	case "pdb":
		result, err = c.getPDBDependencies(cs, cache, contextName, namespace, name, graph, nodeMap)
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	if err != nil {
		return nil, err
	}

	// Determine the root node ID (the queried resource)
	rootID := ""
	if len(result.Nodes) > 0 {
		rootID = result.Nodes[0].ID // First node added is always the queried resource
	}

	// Apply node limiting/aggregation
	return aggregateGraph(result, DefaultNodeLimit, rootID), nil
}

// aggregateGraph limits nodes per type per parent and creates summary nodes for overflow
// explicitRootID is the ID of the queried resource (should always be treated as root)
func aggregateGraph(graph *DependencyGraph, limit int, explicitRootID string) *DependencyGraph {
	if graph == nil {
		return nil
	}

	nodeCount := len(graph.Nodes)
	edgeCount := len(graph.Edges)

	// Build node lookup - pre-sized, stores indices to avoid copying structs
	nodeByID := make(map[string]int, nodeCount) // nodeID -> index in graph.Nodes
	for i := range graph.Nodes {
		nodeByID[graph.Nodes[i].ID] = i
	}

	// Estimate avg edges per node for pre-allocation
	avgEdges := 2
	if nodeCount > 0 {
		avgEdges = (edgeCount / nodeCount) + 1
	}

	// Build edge relationships using indices (8 bytes) instead of struct copies (48 bytes)
	// This reduces memory by ~6x for edge storage
	incomingEdgeIdx := make(map[string][]int, nodeCount)
	outgoingEdgeIdx := make(map[string][]int, nodeCount)
	for i, edge := range graph.Edges {
		if incomingEdgeIdx[edge.Target] == nil {
			incomingEdgeIdx[edge.Target] = make([]int, 0, avgEdges)
		}
		incomingEdgeIdx[edge.Target] = append(incomingEdgeIdx[edge.Target], i)
		if outgoingEdgeIdx[edge.Source] == nil {
			outgoingEdgeIdx[edge.Source] = make([]int, 0, avgEdges)
		}
		outgoingEdgeIdx[edge.Source] = append(outgoingEdgeIdx[edge.Source], i)
	}

	// Identify "connector" nodes - nodes that connect different parts of the graph
	// A connector node has incoming edges from different node KINDS
	// These nodes must be kept visible to maintain graph connectivity
	connectorNodes := make(map[string]bool, nodeCount/4) // estimate ~25% are connectors

	// Reuse a single map for kind counting (cleared between iterations)
	kindSet := make(map[string]bool, 8) // typically <8 different kinds

	for nodeID, edgeIndices := range incomingEdgeIdx {
		// Clear and reuse kindSet instead of allocating new map
		for k := range kindSet {
			delete(kindSet, k)
		}
		for _, ei := range edgeIndices {
			edge := &graph.Edges[ei]
			if idx, ok := nodeByID[edge.Source]; ok {
				kindSet[graph.Nodes[idx].Kind] = true
			}
		}
		if len(kindSet) >= 2 {
			connectorNodes[nodeID] = true
		}
	}

	// Also mark nodes that have outgoing edges to different kinds as connectors
	for nodeID, edgeIndices := range outgoingEdgeIdx {
		for k := range kindSet {
			delete(kindSet, k)
		}
		for _, ei := range edgeIndices {
			edge := &graph.Edges[ei]
			if idx, ok := nodeByID[edge.Target]; ok {
				kindSet[graph.Nodes[idx].Kind] = true
			}
		}
		if len(kindSet) >= 2 {
			connectorNodes[nodeID] = true
		}
	}

	// Use explicit root if provided, otherwise find node with no incoming edges
	rootID := explicitRootID
	if rootID == "" {
		for _, node := range graph.Nodes {
			if len(incomingEdgeIdx[node.ID]) == 0 {
				rootID = node.ID
				break
			}
		}
	}

	// Determine primary parent for each node (for grouping purposes)
	// Uses package-level childToParentRelations and parentToChildRelations maps
	primaryParent := make(map[string]string, nodeCount)
	for nodeID := range nodeByID {
		if nodeID == rootID {
			continue
		}

		parent := ""

		// First check outgoing edges for "uses/binds" type relations (child → parent)
		for _, ei := range outgoingEdgeIdx[nodeID] {
			edge := &graph.Edges[ei]
			if childToParentRelations[edge.Relation] {
				parent = edge.Target
				break
			}
		}

		// Check incoming edges for "owns/selects/routes-to" type relations (parent → child)
		if parent == "" {
			for _, ei := range incomingEdgeIdx[nodeID] {
				edge := &graph.Edges[ei]
				if parentToChildRelations[edge.Relation] {
					parent = edge.Source
					break
				}
			}
		}

		// Fallback: any incoming edge source (but NOT if we'd create a cycle)
		if parent == "" && len(incomingEdgeIdx[nodeID]) > 0 {
			candidate := graph.Edges[incomingEdgeIdx[nodeID][0]].Source
			// Avoid cycles: don't set parent if candidate's parent would be this node
			if primaryParent[candidate] != nodeID {
				parent = candidate
			}
		}

		primaryParent[nodeID] = parent
	}

	// Build new graph with limits applied - pre-allocate based on input size
	// Most nodes will be kept (limited only when exceeding threshold)
	newGraph := &DependencyGraph{
		Nodes: make([]DependencyNode, 0, nodeCount),
		Edges: make([]DependencyEdge, 0, edgeCount),
	}
	keptNodes := make(map[string]bool, nodeCount)
	hiddenNodes := make(map[string]bool, nodeCount/4) // Track nodes that were aggregated away

	// Always keep root node
	if rootID != "" {
		if rootIdx, ok := nodeByID[rootID]; ok {
			newGraph.Nodes = append(newGraph.Nodes, graph.Nodes[rootIdx])
			keptNodes[rootID] = true
		}
	}

	// Process nodes level by level, starting from root's children
	// This ensures parents are processed before children
	type groupKey struct {
		parentID string
		kind     string
	}

	processedNodes := make(map[string]bool, nodeCount)
	processedNodes[rootID] = true

	// Keep processing until no more nodes to process
	// Use indices instead of copying full DependencyNode structs (~112 bytes each)
	for {
		// Find nodes whose parent has been processed (either kept or hidden)
		nodeIdxByGroup := make(map[groupKey][]int)
		for i := range graph.Nodes {
			node := &graph.Nodes[i]
			if processedNodes[node.ID] {
				continue
			}
			parentID := primaryParent[node.ID]
			if parentID == "" {
				parentID = rootID // Treat as direct child of root
			}
			// Only process if parent has been processed
			if !processedNodes[parentID] && parentID != rootID {
				continue
			}
			key := groupKey{parentID: parentID, kind: node.Kind}
			nodeIdxByGroup[key] = append(nodeIdxByGroup[key], i)
		}

		if len(nodeIdxByGroup) == 0 {
			break // No more nodes to process
		}

		// Process each group
		for key, nodeIndices := range nodeIdxByGroup {
			// If parent was hidden, hide all children too (they're implicitly aggregated
			// under their parent's summary node)
			if hiddenNodes[key.parentID] {
				for _, ni := range nodeIndices {
					node := &graph.Nodes[ni]
					hiddenNodes[node.ID] = true
					processedNodes[node.ID] = true
				}
				continue
			}

			// Parent is visible, apply normal aggregation
			if len(nodeIndices) <= limit {
				// Keep all nodes
				for _, ni := range nodeIndices {
					node := &graph.Nodes[ni]
					newGraph.Nodes = append(newGraph.Nodes, *node)
					keptNodes[node.ID] = true
					processedNodes[node.ID] = true
				}
			} else {
				// Separate connector nodes from regular nodes (using indices)
				var connectorIdx, regularIdx []int
				for _, ni := range nodeIndices {
					if connectorNodes[graph.Nodes[ni].ID] {
						connectorIdx = append(connectorIdx, ni)
					} else {
						regularIdx = append(regularIdx, ni)
					}
				}

				// Always keep all connector nodes
				for _, ni := range connectorIdx {
					node := &graph.Nodes[ni]
					newGraph.Nodes = append(newGraph.Nodes, *node)
					keptNodes[node.ID] = true
					processedNodes[node.ID] = true
				}

				// Fill remaining slots with regular nodes
				remainingSlots := limit - len(connectorIdx)
				if remainingSlots < 0 {
					remainingSlots = 0
				}

				keptRegular := 0
				for i := 0; i < remainingSlots && i < len(regularIdx); i++ {
					node := &graph.Nodes[regularIdx[i]]
					newGraph.Nodes = append(newGraph.Nodes, *node)
					keptNodes[node.ID] = true
					processedNodes[node.ID] = true
					keptRegular++
				}

				// Mark remaining regular nodes as hidden
				for i := keptRegular; i < len(regularIdx); i++ {
					node := &graph.Nodes[regularIdx[i]]
					hiddenNodes[node.ID] = true
					processedNodes[node.ID] = true
				}

				// Add summary node for the hidden nodes
				remaining := len(regularIdx) - keptRegular
				if remaining > 0 {
					summaryID := "summary:" + key.parentID + ":" + key.kind
					summaryNode := DependencyNode{
						ID:             summaryID,
						Kind:           key.kind,
						Name:           "+" + strconv.Itoa(remaining) + " more " + key.kind + "s",
						IsSummary:      true,
						RemainingCount: remaining,
						ParentID:       key.parentID,
					}
					newGraph.Nodes = append(newGraph.Nodes, summaryNode)
					keptNodes[summaryID] = true
				}
			}
		}
	}

	// Rebuild edges for kept nodes
	// Use a struct key for O(1) edge deduplication (avoids string allocation)
	type edgeKey struct{ src, tgt, rel string }
	edgeSet := make(map[edgeKey]bool, edgeCount)

	for _, edge := range graph.Edges {
		sourceKept := keptNodes[edge.Source]
		targetKept := keptNodes[edge.Target]

		if sourceKept && targetKept {
			key := edgeKey{edge.Source, edge.Target, edge.Relation}
			if !edgeSet[key] {
				newGraph.Edges = append(newGraph.Edges, edge)
				edgeSet[key] = true
			}
		} else if sourceKept && !targetKept {
			// Check if there's a summary node for the target's group
			if targetIdx, ok := nodeByID[edge.Target]; ok {
				parentID := primaryParent[edge.Target]
				if parentID == "" {
					parentID = "_root_"
				}
				summaryID := "summary:" + parentID + ":" + graph.Nodes[targetIdx].Kind
				if keptNodes[summaryID] {
					key := edgeKey{edge.Source, summaryID, edge.Relation}
					if !edgeSet[key] {
						newGraph.Edges = append(newGraph.Edges, DependencyEdge{
							Source:   edge.Source,
							Target:   summaryID,
							Relation: edge.Relation,
						})
						edgeSet[key] = true
					}
				}
			}
		}
	}

	return newGraph
}

// nodeID builds a unique identifier for a dependency node.
// Uses string concatenation instead of fmt.Sprintf for 3-5x speedup.
func nodeID(kind, namespace, name string) string {
	if namespace == "" {
		return kind + "/" + name
	}
	return kind + "/" + namespace + "/" + name
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
func (c *Client) getPodDependencies(cs kubernetes.Interface, cache *resourceCache, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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
	c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, namespace, pod.OwnerReferences)

	// Resolve volume dependencies (downward)
	c.resolvePodVolumes(cs, graph, nodeMap, podID, namespace, pod.Spec.Volumes)

	// Resolve container env references
	c.resolvePodContainerRefs(graph, nodeMap, podID, namespace, pod.Spec.Containers)
	c.resolvePodContainerRefs(graph, nodeMap, podID, namespace, pod.Spec.InitContainers)

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
			// Use cache for ReplicaSet lookup
			rsList, err := cache.getReplicaSets(namespace)
			if err == nil {
				for _, rs := range rsList {
					if rs.Name == ref.Name {
						c.resolveOwnerRefs(cs, cache, graph, nodeMap, ownerID, namespace, rs.OwnerReferences)
						break
					}
				}
			}
		case "Job":
			// Use cache for Job lookup
			jobList, err := cache.getJobs(namespace)
			if err == nil {
				for _, job := range jobList {
					if job.Name == ref.Name {
						c.resolveOwnerRefs(cs, cache, graph, nodeMap, ownerID, namespace, job.OwnerReferences)
						break
					}
				}
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
	c.addNode(graph, nodeMap, DependencyNode{
		ID:        jobID,
		Kind:      "Job",
		Name:      name,
		Namespace: namespace,
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
					c.addNode(graph, nodeMap, DependencyNode{
						ID:        jobID,
						Kind:      "Job",
						Name:      job.Name,
						Namespace: namespace,
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

// getPVCDependencies resolves dependencies for a PVC
func (c *Client) getPVCDependencies(cs kubernetes.Interface, cache *resourceCache, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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

	// Find pods using this PVC (using cache)
	pods, err := cache.getPods(namespace)
	if err == nil {
		for _, pod := range pods {
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
					c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, namespace, pod.OwnerReferences)
					// Find Services selecting this pod (and their Ingresses)
					c.findServicesSelectingPod(cache, graph, nodeMap, namespace, pod.Labels, podID)
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
func (c *Client) getPVDependencies(cs kubernetes.Interface, cache *resourceCache, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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

		// Find pods using this PVC (using cache)
		if err == nil {
			pods, err := cache.getPods(pvcNamespace)
			if err == nil {
				for _, pod := range pods {
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
							c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, pvcNamespace, pod.OwnerReferences)
							// Find Services selecting this pod (and their Ingresses)
							c.findServicesSelectingPod(cache, graph, nodeMap, pvcNamespace, pod.Labels, podID)
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
func (c *Client) getConfigMapDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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
	c.findPodsUsingConfigMap(cs, cache, graph, nodeMap, namespace, name, cmID)

	return graph, nil
}

// getSecretDependencies resolves dependencies for a Secret
func (c *Client) getSecretDependencies(cs kubernetes.Interface, cache *resourceCache, contextName, namespace, name string, graph *DependencyGraph, nodeMap map[string]bool) (*DependencyGraph, error) {
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
			// Service selects the workload's pods - find a pod to link to
			for id := range nodeMap {
				if len(id) > 4 && id[:4] == "Pod/" {
					c.addEdge(graph, svcID, id, "selects")
					break
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
			c.resolveOwnerRefs(cs, cache, graph, nodeMap, podID, namespace, pod.OwnerReferences)
			// Find Services selecting this pod (and their Ingresses)
			c.findServicesSelectingPod(cache, graph, nodeMap, namespace, pod.Labels, podID)
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

				pod, podErr := cs.CoreV1().Pods(podNamespace).Get(context.TODO(), podName, metav1.GetOptions{})
				status := "Unknown"
				if podErr == nil {
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
				if podErr == nil {
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

				pod, podErr := cs.CoreV1().Pods(podNamespace).Get(context.TODO(), podName, metav1.GetOptions{})
				status := "NotReady"
				if podErr == nil {
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

				if podErr == nil {
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
				if matchesSelector(pod.Labels, np.Spec.PodSelector.MatchLabels) {
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
				if matchesSelector(pod.Labels, pdb.Spec.Selector.MatchLabels) {
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

// ExpandDependencyNode returns additional nodes when a summary node is expanded
// summaryNodeID format: "summary:parentID:kind"
// Returns up to ExpansionLimit (10) nodes starting from the given offset
func (c *Client) ExpandDependencyNode(contextName, resourceType, namespace, resourceName, summaryNodeID string, offset int) (*DependencyGraph, error) {
	// First, get the full graph without aggregation
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Create context with timeout to prevent UI hangs on slow clusters
	ctx, cancel := context.WithTimeout(context.Background(), dependencyAPITimeout)
	defer cancel()

	graph := &DependencyGraph{
		Nodes: make([]DependencyNode, 0, 32),
		Edges: make([]DependencyEdge, 0, 32),
	}
	nodeMap := make(map[string]bool, 32)
	cache := newResourceCache(ctx, cs) // Request-scoped cache with timeout

	var fullGraph *DependencyGraph

	switch resourceType {
	case "pod":
		fullGraph, err = c.getPodDependencies(cs, cache, namespace, resourceName, graph, nodeMap)
	case "deployment":
		fullGraph, err = c.getDeploymentDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "statefulset":
		fullGraph, err = c.getStatefulSetDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "daemonset":
		fullGraph, err = c.getDaemonSetDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "replicaset":
		fullGraph, err = c.getReplicaSetDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "job":
		fullGraph, err = c.getJobDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "cronjob":
		fullGraph, err = c.getCronJobDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "configmap":
		fullGraph, err = c.getConfigMapDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "secret":
		fullGraph, err = c.getSecretDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "service":
		fullGraph, err = c.getServiceDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "pvc":
		fullGraph, err = c.getPVCDependencies(cs, cache, namespace, resourceName, graph, nodeMap)
	case "pv":
		fullGraph, err = c.getPVDependencies(cs, cache, resourceName, graph, nodeMap)
	case "storageclass":
		fullGraph, err = c.getStorageClassDependencies(cs, cache, contextName, resourceName, graph, nodeMap)
	case "ingress":
		fullGraph, err = c.getIngressDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "ingressclass":
		fullGraph, err = c.getIngressClassDependencies(cs, cache, contextName, resourceName, graph, nodeMap)
	case "endpoints":
		fullGraph, err = c.getEndpointsDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "networkpolicy":
		fullGraph, err = c.getNetworkPolicyDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "priorityclass":
		fullGraph, err = c.getPriorityClassDependencies(cs, cache, contextName, resourceName, graph, nodeMap)
	case "serviceaccount":
		fullGraph, err = c.getServiceAccountDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "hpa":
		fullGraph, err = c.getHPADependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	case "pdb":
		fullGraph, err = c.getPDBDependencies(cs, cache, contextName, namespace, resourceName, graph, nodeMap)
	default:
		return nil, fmt.Errorf("unsupported resource type for expansion: %s", resourceType)
	}

	if err != nil {
		return nil, err
	}

	// Parse summaryNodeID: "summary:parentID:kind"
	// e.g., "summary:Deployment/default/nginx:Pod"
	parts := splitSummaryID(summaryNodeID)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid summary node ID format: %s", summaryNodeID)
	}
	parentID := parts[1]
	targetKind := parts[2]

	// Build node lookup (uses package-level childToParentRelations)
	nodeByID := make(map[string]DependencyNode, len(fullGraph.Nodes))
	for _, node := range fullGraph.Nodes {
		nodeByID[node.ID] = node
	}

	// Find all children of the parent with matching kind
	// Need to consider edge direction: some relations have parent as source, others have parent as target
	var matchingNodes []DependencyNode
	seenNodes := make(map[string]bool, ExpansionLimit*2) // Pre-size for typical expansion

	for _, edge := range fullGraph.Edges {
		var childID string
		if childToParentRelations[edge.Relation] {
			// For "uses/binds/references": source is child, target is parent
			if edge.Target == parentID {
				childID = edge.Source
			}
		} else {
			// For "owns/selects/routes-to": source is parent, target is child
			if edge.Source == parentID {
				childID = edge.Target
			}
		}

		if childID != "" && !seenNodes[childID] {
			if node, ok := nodeByID[childID]; ok && node.Kind == targetKind {
				matchingNodes = append(matchingNodes, node)
				seenNodes[childID] = true
			}
		}
	}

	// Apply offset and limit to get the expansion batch
	// offset typically starts at DefaultNodeLimit (5), then increases by ExpansionLimit (10)
	expandedGraph := &DependencyGraph{
		Nodes: make([]DependencyNode, 0, ExpansionLimit),
		Edges: make([]DependencyEdge, 0, ExpansionLimit*2),
	}

	endIdx := offset + ExpansionLimit
	if endIdx > len(matchingNodes) {
		endIdx = len(matchingNodes)
	}

	expandedNodeIDs := make(map[string]bool, ExpansionLimit)
	for i := offset; i < endIdx; i++ {
		expandedGraph.Nodes = append(expandedGraph.Nodes, matchingNodes[i])
		expandedNodeIDs[matchingNodes[i].ID] = true
	}

	// Also include children of the expanded nodes (e.g., PVCs under PVs)
	// This ensures the graph remains connected
	for _, edge := range fullGraph.Edges {
		var childID string
		if childToParentRelations[edge.Relation] {
			// For "uses/binds/references": check if target is an expanded node
			if expandedNodeIDs[edge.Target] {
				childID = edge.Source
			}
		} else {
			// For "owns/selects/routes-to": check if source is an expanded node
			if expandedNodeIDs[edge.Source] {
				childID = edge.Target
			}
		}

		// Add child node if not already added
		if childID != "" && !expandedNodeIDs[childID] {
			if childNode, ok := nodeByID[childID]; ok {
				expandedGraph.Nodes = append(expandedGraph.Nodes, childNode)
				expandedNodeIDs[childID] = true
			}
		}
	}

	// Copy all relevant edges between expanded nodes and their relationships
	for _, edge := range fullGraph.Edges {
		sourceIncluded := expandedNodeIDs[edge.Source] || edge.Source == parentID
		targetIncluded := expandedNodeIDs[edge.Target] || edge.Target == parentID

		if sourceIncluded && targetIncluded {
			expandedGraph.Edges = append(expandedGraph.Edges, edge)
		}
	}

	// If there are more nodes after this batch, add a new summary node
	remaining := len(matchingNodes) - endIdx
	if remaining > 0 {
		newSummaryID := "summary:" + parentID + ":" + targetKind
		expandedGraph.Nodes = append(expandedGraph.Nodes, DependencyNode{
			ID:             newSummaryID,
			Kind:           targetKind,
			Name:           "+" + strconv.Itoa(remaining) + " more " + targetKind + "s",
			IsSummary:      true,
			RemainingCount: remaining,
			ParentID:       parentID,
		})
	}

	return expandedGraph, nil
}

// splitSummaryID splits a summary node ID into its parts
// "summary:Deployment/default/nginx:Pod" -> ["summary", "Deployment/default/nginx", "Pod"]
func splitSummaryID(id string) []string {
	// Split by first colon to get "summary", then by last colon to separate parent from kind
	if len(id) < 8 || id[:8] != "summary:" {
		return nil
	}
	rest := id[8:] // After "summary:"
	// Find the last colon (kind is always a simple word without colons)
	lastColon := -1
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == ':' {
			lastColon = i
			break
		}
	}
	if lastColon < 0 {
		return nil
	}
	return []string{"summary", rest[:lastColon], rest[lastColon+1:]}
}
