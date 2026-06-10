package k8s

import (
	"context"
	"fmt"
	"strconv"
	"time"
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

// DependencyNode represents a node in the dependency graph
type DependencyNode struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Status    string `json:"status,omitempty"`
	// Summary node fields
	IsSummary      bool   `json:"isSummary,omitempty"`
	RemainingCount int    `json:"remainingCount,omitempty"`
	ParentID       string `json:"parentId,omitempty"` // For expansion context
	// Metadata for node charms (e.g., replica counts)
	Metadata map[string]string `json:"metadata,omitempty"`
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

const DefaultNodeLimit = 5
const ExpansionLimit = 10

// GetResourceDependencies resolves all dependencies for a given resource
func (c *Client) GetResourceDependencies(contextName, resourceType, namespace, name string) (*DependencyGraph, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Get dynamic client for CRD owner resolution (best-effort, nil is handled gracefully)
	dc, _ := c.getDynamicClientForContext(contextName)

	// Create context with timeout to prevent UI hangs on slow clusters
	ctx, cancel := context.WithTimeout(context.Background(), dependencyAPITimeout)
	defer cancel()

	graph := &DependencyGraph{
		Nodes: make([]DependencyNode, 0, 32),
		Edges: make([]DependencyEdge, 0, 32),
	}

	nodeMap := make(map[string]bool, 32)   // Track added nodes by ID, pre-sized for typical graph
	cache := newResourceCache(ctx, cs, dc) // Request-scoped cache for List() results

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

// ExpandDependencyNode returns additional nodes when a summary node is expanded
// summaryNodeID format: "summary:parentID:kind"
// Returns up to ExpansionLimit (10) nodes starting from the given offset
func (c *Client) ExpandDependencyNode(contextName, resourceType, namespace, resourceName, summaryNodeID string, offset int) (*DependencyGraph, error) {
	// First, get the full graph without aggregation
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}

	// Get dynamic client for CRD owner resolution (best-effort, nil is handled gracefully)
	dc, _ := c.getDynamicClientForContext(contextName)

	// Create context with timeout to prevent UI hangs on slow clusters
	ctx, cancel := context.WithTimeout(context.Background(), dependencyAPITimeout)
	defer cancel()

	graph := &DependencyGraph{
		Nodes: make([]DependencyNode, 0, 32),
		Edges: make([]DependencyEdge, 0, 32),
	}
	nodeMap := make(map[string]bool, 32)
	cache := newResourceCache(ctx, cs, dc) // Request-scoped cache with timeout

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
