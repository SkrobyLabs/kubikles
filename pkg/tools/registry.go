package tools

// ToolMeta provides additional metadata about a tool for UI and categorization.
type ToolMeta struct {
	// RelevantViews lists the UI views where this tool is most useful
	// (e.g., "pods", "deployments", "metrics-overview")
	RelevantViews []string `json:"relevantViews,omitempty"`

	// RelevantActions lists the UI actions/tabs where this tool is most useful
	// (e.g., "details", "yaml", "logs", "deps")
	RelevantActions []string `json:"relevantActions,omitempty"`

	// Category groups tools by functionality
	// (e.g., "metrics", "resources", "debugging", "crds")
	Category string `json:"category,omitempty"`

	// IsDangerous indicates if this tool can modify cluster state
	IsDangerous bool `json:"isDangerous,omitempty"`

	// SafetyNote provides guidance for dangerous tools
	SafetyNote string `json:"safetyNote,omitempty"`
}

// ToolRegistry holds tool definitions and their metadata.
type ToolRegistry struct {
	tools    []ToolDef
	metadata map[string]ToolMeta
}

// NewToolRegistry creates a new tool registry with the default tool definitions.
func NewToolRegistry() *ToolRegistry {
	r := &ToolRegistry{
		tools:    AllToolDefs(),
		metadata: make(map[string]ToolMeta),
	}
	r.initMetadata()
	return r
}

// initMetadata populates the metadata for all built-in tools.
func (r *ToolRegistry) initMetadata() {
	r.metadata["get_pod_logs"] = ToolMeta{
		RelevantViews:   []string{"pods", "jobs"},
		RelevantActions: []string{"logs"},
		Category:        "debugging",
	}

	r.metadata["get_resource_yaml"] = ToolMeta{
		RelevantViews:   []string{"pods", "deployments", "statefulsets", "daemonsets", "services", "configmaps", "secrets"},
		RelevantActions: []string{"yaml"},
		Category:        "resources",
	}

	r.metadata["list_resources"] = ToolMeta{
		RelevantViews:   []string{"pods", "deployments", "statefulsets", "daemonsets", "replicasets", "jobs", "cronjobs", "services", "ingresses", "configmaps", "secrets", "nodes", "namespaces", "pvcs", "pvs", "hpas", "serviceaccounts"},
		RelevantActions: []string{},
		Category:        "resources",
	}

	r.metadata["get_events"] = ToolMeta{
		RelevantViews:   []string{"events", "pods", "deployments", "nodes"},
		RelevantActions: []string{"details"},
		Category:        "debugging",
	}

	r.metadata["describe_resource"] = ToolMeta{
		RelevantViews:   []string{"pods", "deployments", "statefulsets", "daemonsets", "services", "nodes"},
		RelevantActions: []string{"details"},
		Category:        "resources",
	}

	r.metadata["list_crds"] = ToolMeta{
		RelevantViews:   []string{"crds"},
		RelevantActions: []string{},
		Category:        "crds",
	}

	r.metadata["list_custom_resources"] = ToolMeta{
		RelevantViews:   []string{"crds"},
		RelevantActions: []string{},
		Category:        "crds",
	}

	r.metadata["get_custom_resource_yaml"] = ToolMeta{
		RelevantViews:   []string{"crds"},
		RelevantActions: []string{"yaml"},
		Category:        "crds",
	}

	r.metadata["get_cluster_metrics"] = ToolMeta{
		RelevantViews:   []string{"metrics-overview", "nodes"},
		RelevantActions: []string{"details"},
		Category:        "metrics",
	}

	r.metadata["get_pod_metrics"] = ToolMeta{
		RelevantViews:   []string{"metrics-overview", "pods", "deployments", "statefulsets", "daemonsets"},
		RelevantActions: []string{"details"},
		Category:        "metrics",
	}

	r.metadata["get_namespace_summary"] = ToolMeta{
		RelevantViews:   []string{"namespaces"},
		RelevantActions: []string{"details"},
		Category:        "resources",
	}

	r.metadata["get_resource_dependencies"] = ToolMeta{
		RelevantViews:   []string{"deployments", "statefulsets", "services", "ingresses", "pvcs"},
		RelevantActions: []string{"deps", "details"},
		Category:        "resources",
	}

	// Diagnostic tools
	r.metadata["get_flow_timeline"] = ToolMeta{
		RelevantViews:   []string{"pods", "deployments", "statefulsets", "daemonsets", "jobs", "services", "flow-timeline"},
		RelevantActions: []string{"details"},
		Category:        "diagnostics",
	}

	r.metadata["get_multi_pod_logs"] = ToolMeta{
		RelevantViews:   []string{"pods", "deployments", "statefulsets", "daemonsets", "jobs", "multi-log-viewer"},
		RelevantActions: []string{"logs"},
		Category:        "diagnostics",
	}

	r.metadata["diff_resources"] = ToolMeta{
		RelevantViews:   []string{"deployments", "statefulsets", "configmaps", "secrets", "services", "resource-diff"},
		RelevantActions: []string{"yaml", "details"},
		Category:        "diagnostics",
	}

	r.metadata["check_rbac_access"] = ToolMeta{
		RelevantViews:   []string{"serviceaccounts", "pods", "rbac-checker"},
		RelevantActions: []string{"details"},
		Category:        "diagnostics",
	}
}

// GetAllTools returns all registered tool definitions.
func (r *ToolRegistry) GetAllTools() []ToolDef {
	return r.tools
}

// GetMeta returns the metadata for a tool by name.
// Returns an empty ToolMeta if the tool is not found.
func (r *ToolRegistry) GetMeta(name string) ToolMeta {
	return r.metadata[name]
}

// GetToolsForView returns tool names relevant to a specific UI view.
func (r *ToolRegistry) GetToolsForView(view string) []string {
	var result []string
	for name, meta := range r.metadata {
		for _, v := range meta.RelevantViews {
			if v == view {
				result = append(result, name)
				break
			}
		}
	}
	return result
}

// GetToolsForAction returns tool names relevant to a specific UI action/tab.
func (r *ToolRegistry) GetToolsForAction(action string) []string {
	var result []string
	for name, meta := range r.metadata {
		for _, a := range meta.RelevantActions {
			if a == action {
				result = append(result, name)
				break
			}
		}
	}
	return result
}

// BuildViewMapping returns a map of view names to relevant tool names.
func (r *ToolRegistry) BuildViewMapping() map[string][]string {
	viewMap := make(map[string][]string)
	for name, meta := range r.metadata {
		for _, view := range meta.RelevantViews {
			viewMap[view] = append(viewMap[view], name)
		}
	}
	return viewMap
}

// BuildActionMapping returns a map of action names to relevant tool names.
func (r *ToolRegistry) BuildActionMapping() map[string][]string {
	actionMap := make(map[string][]string)
	for name, meta := range r.metadata {
		for _, action := range meta.RelevantActions {
			actionMap[action] = append(actionMap[action], name)
		}
	}
	return actionMap
}

// ToolDiscoveryResponse is the API response for tool discovery.
type ToolDiscoveryResponse struct {
	Tools         []ToolDef           `json:"tools"`
	ViewMapping   map[string][]string `json:"viewMapping"`
	ActionMapping map[string][]string `json:"actionMapping"`
}

// GetDiscoveryResponse builds the full discovery response.
func (r *ToolRegistry) GetDiscoveryResponse() ToolDiscoveryResponse {
	return ToolDiscoveryResponse{
		Tools:         r.tools,
		ViewMapping:   r.BuildViewMapping(),
		ActionMapping: r.BuildActionMapping(),
	}
}

// DefaultToolRegistry is the global default tool registry.
var DefaultToolRegistry = NewToolRegistry()
