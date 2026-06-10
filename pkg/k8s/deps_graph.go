// Code split from dependencies.go; see that file for the graph types and entry point.
package k8s

import (
	"strconv"

	"kubikles/pkg/debug"
)

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
		debug.LogK8s("addNode: Added new node", map[string]interface{}{"id": node.ID, "kind": node.Kind})
	} else if node.Metadata != nil {
		// Update existing node with metadata if it was added without it
		for i, existing := range graph.Nodes {
			if existing.ID == node.ID && existing.Metadata == nil {
				graph.Nodes[i].Metadata = node.Metadata
				// Also update status if the new node has it and the old one doesn't
				if existing.Status == "" && node.Status != "" {
					graph.Nodes[i].Status = node.Status
				}
				debug.LogK8s("addNode: Updated existing node with metadata", map[string]interface{}{"id": node.ID})
				break
			}
		}
	} else {
		debug.LogK8s("addNode: Skipped duplicate node", map[string]interface{}{"id": node.ID})
	}
}

func (c *Client) addEdge(graph *DependencyGraph, source, target, relation string) {
	graph.Edges = append(graph.Edges, DependencyEdge{
		Source:   source,
		Target:   target,
		Relation: relation,
	})
}
