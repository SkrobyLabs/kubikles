package k8s

import (
	"context"
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// DiffRequest specifies two resources to compare
type DiffRequest struct {
	SourceContext   string   `json:"sourceContext"`
	SourceNamespace string   `json:"sourceNamespace"`
	SourceKind      string   `json:"sourceKind"`
	SourceName      string   `json:"sourceName"`
	TargetContext   string   `json:"targetContext"`
	TargetNamespace string   `json:"targetNamespace"`
	TargetKind      string   `json:"targetKind"`
	TargetName      string   `json:"targetName"`
	IgnoreFields    []string `json:"ignoreFields"`
}

// DiffResult contains the comparison output
type DiffResult struct {
	SourceYAML   string       `json:"sourceYaml"`
	TargetYAML   string       `json:"targetYaml"`
	UnifiedDiff  string       `json:"unifiedDiff"`
	HasChanges   bool         `json:"hasChanges"`
	ChangeCount  int          `json:"changeCount"`
	SourceExists bool         `json:"sourceExists"`
	TargetExists bool         `json:"targetExists"`
	Changes      []DiffChange `json:"changes"`
}

// DiffChange represents a structured change between resources
type DiffChange struct {
	Type string `json:"type"` // "added", "removed", "changed"
	Path string `json:"path"` // JSON path to the changed field
	Old  string `json:"old,omitempty"`
	New  string `json:"new,omitempty"`
}

// Default fields to ignore in diff (volatile/auto-generated)
var defaultIgnoreFields = []string{
	"metadata.resourceVersion",
	"metadata.uid",
	"metadata.generation",
	"metadata.creationTimestamp",
	"metadata.managedFields",
	"metadata.selfLink",
	"metadata.annotations.kubectl.kubernetes.io/last-applied-configuration",
	"status",
}

// DiffResources compares two Kubernetes resources
func (c *Client) DiffResources(req DiffRequest) (*DiffResult, error) {
	result := &DiffResult{
		SourceExists: true,
		TargetExists: true,
	}

	// Set default ignore fields if not specified
	ignoreFields := req.IgnoreFields
	if len(ignoreFields) == 0 {
		ignoreFields = defaultIgnoreFields
	}

	// Fetch source resource
	sourceYAML, err := c.fetchResourceYAMLForDiff(req.SourceContext, req.SourceKind, req.SourceNamespace, req.SourceName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			result.SourceExists = false
			sourceYAML = ""
		} else {
			return nil, fmt.Errorf("failed to fetch source: %w", err)
		}
	}

	// Fetch target resource
	targetYAML, err := c.fetchResourceYAMLForDiff(req.TargetContext, req.TargetKind, req.TargetNamespace, req.TargetName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			result.TargetExists = false
			targetYAML = ""
		} else {
			return nil, fmt.Errorf("failed to fetch target: %w", err)
		}
	}

	// Normalize YAML (remove ignored fields)
	sourceNorm := normalizeYAMLForDiff(sourceYAML, ignoreFields)
	targetNorm := normalizeYAMLForDiff(targetYAML, ignoreFields)

	result.SourceYAML = sourceNorm
	result.TargetYAML = targetNorm

	// Generate unified diff
	result.UnifiedDiff = generateUnifiedDiffOutput(
		sourceNorm, targetNorm,
		formatResourceLabel(req.SourceContext, req.SourceNamespace, req.SourceKind, req.SourceName),
		formatResourceLabel(req.TargetContext, req.TargetNamespace, req.TargetKind, req.TargetName),
	)

	// Check for changes
	result.HasChanges = sourceNorm != targetNorm
	if result.HasChanges {
		result.Changes = computeStructuredChanges(sourceNorm, targetNorm)
		result.ChangeCount = len(result.Changes)
	}

	return result, nil
}

// fetchResourceYAMLForDiff fetches a resource as YAML from a specific context
func (c *Client) fetchResourceYAMLForDiff(contextName, kind, namespace, name string) (string, error) {
	ctx := context.Background()

	// Use dynamic client for flexibility
	dc, err := c.getDynamicClientForContext(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get dynamic client: %w", err)
	}

	// Map kind to GVR
	gvr, namespaced := kindToGVRForDiff(kind)

	var obj interface{}
	if namespaced && namespace != "" {
		obj, err = dc.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	} else {
		obj, err = dc.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}

	if err != nil {
		return "", err
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// kindToGVRForDiff maps a kind to its GroupVersionResource
func kindToGVRForDiff(kind string) (schema.GroupVersionResource, bool) {
	// Map of common kinds to their GVR and whether they're namespaced
	mapping := map[string]struct {
		gvr        schema.GroupVersionResource
		namespaced bool
	}{
		"pod":                   {schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, true},
		"deployment":            {schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, true},
		"service":               {schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, true},
		"configmap":             {schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, true},
		"secret":                {schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, true},
		"ingress":               {schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}, true},
		"statefulset":           {schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, true},
		"daemonset":             {schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, true},
		"replicaset":            {schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, true},
		"job":                   {schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, true},
		"cronjob":               {schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, true},
		"persistentvolumeclaim": {schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, true},
		"persistentvolume":      {schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}, false},
		"serviceaccount":        {schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}, true},
		"namespace":             {schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, false},
		"node":                  {schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}, false},
		"clusterrole":           {schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}, false},
		"clusterrolebinding":    {schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}, false},
		"role":                  {schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}, true},
		"rolebinding":           {schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}, true},
		"networkpolicy":         {schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}, true},
		"storageclass":          {schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}, false},
		"hpa":                   {schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}, true},
		"pdb":                   {schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}, true},
	}

	kindLower := strings.ToLower(kind)
	if m, ok := mapping[kindLower]; ok {
		return m.gvr, m.namespaced
	}

	// Default: assume core v1 namespaced resource
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: strings.ToLower(kind) + "s"}, true
}

// normalizeYAMLForDiff removes ignored fields from YAML
func normalizeYAMLForDiff(yamlStr string, ignoreFields []string) string {
	if yamlStr == "" {
		return ""
	}

	// Parse YAML into map
	var obj map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &obj); err != nil {
		return yamlStr // Return original if parsing fails
	}

	// Remove ignored fields
	for _, field := range ignoreFields {
		removeNestedField(obj, strings.Split(field, "."))
	}

	// Re-serialize
	result, err := yaml.Marshal(obj)
	if err != nil {
		return yamlStr
	}

	return string(result)
}

// removeNestedField removes a nested field from a map
func removeNestedField(obj map[string]interface{}, path []string) {
	if len(path) == 0 {
		return
	}

	if len(path) == 1 {
		delete(obj, path[0])
		return
	}

	// Navigate to parent
	if child, ok := obj[path[0]].(map[string]interface{}); ok {
		removeNestedField(child, path[1:])
	}
}

// formatResourceLabel creates a label for diff header
func formatResourceLabel(context, namespace, kind, name string) string {
	parts := []string{}
	if context != "" {
		parts = append(parts, context)
	}
	if namespace != "" {
		parts = append(parts, namespace)
	}
	parts = append(parts, kind, name)
	return strings.Join(parts, "/")
}

// generateUnifiedDiffOutput creates a unified diff between two strings
func generateUnifiedDiffOutput(source, target, sourceLabel, targetLabel string) string {
	if source == target {
		return ""
	}

	dmp := diffmatchpatch.New()

	// Use line-mode diff for better results with YAML
	a, b, lineArray := dmp.DiffLinesToChars(source, target)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)
	diffs = dmp.DiffCleanupSemantic(diffs)

	// Build unified diff output
	var result strings.Builder
	result.WriteString(fmt.Sprintf("--- %s\n", sourceLabel))
	result.WriteString(fmt.Sprintf("+++ %s\n", targetLabel))

	// Convert diffs to unified format
	lineNum := 1
	for _, diff := range diffs {
		lines := strings.Split(diff.Text, "\n")
		// Remove empty last element if present
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}

		for _, line := range lines {
			switch diff.Type {
			case diffmatchpatch.DiffEqual:
				result.WriteString(fmt.Sprintf(" %s\n", line))
				lineNum++
			case diffmatchpatch.DiffDelete:
				result.WriteString(fmt.Sprintf("-%s\n", line))
			case diffmatchpatch.DiffInsert:
				result.WriteString(fmt.Sprintf("+%s\n", line))
				lineNum++
			}
		}
	}

	return result.String()
}

// computeStructuredChanges identifies structured changes between YAML documents
func computeStructuredChanges(source, target string) []DiffChange {
	var changes []DiffChange

	var sourceObj, targetObj map[string]interface{}
	if err := yaml.Unmarshal([]byte(source), &sourceObj); err != nil {
		return changes
	}
	if err := yaml.Unmarshal([]byte(target), &targetObj); err != nil {
		return changes
	}

	// Compare recursively
	compareObjectsRecursive(sourceObj, targetObj, "", &changes)

	return changes
}

// compareObjectsRecursive recursively compares two objects and records differences
func compareObjectsRecursive(source, target map[string]interface{}, path string, changes *[]DiffChange) {
	if source == nil {
		source = make(map[string]interface{})
	}
	if target == nil {
		target = make(map[string]interface{})
	}

	// Find removed and changed keys
	for key, sourceVal := range source {
		currentPath := key
		if path != "" {
			currentPath = path + "." + key
		}

		targetVal, exists := target[key]
		if !exists {
			*changes = append(*changes, DiffChange{
				Type: "removed",
				Path: currentPath,
				Old:  formatValue(sourceVal),
			})
			continue
		}

		// Compare values based on type
		compareValues(sourceVal, targetVal, currentPath, changes)
	}

	// Find added keys
	for key, targetVal := range target {
		currentPath := key
		if path != "" {
			currentPath = path + "." + key
		}

		if _, exists := source[key]; !exists {
			*changes = append(*changes, DiffChange{
				Type: "added",
				Path: currentPath,
				New:  formatValue(targetVal),
			})
		}
	}
}

// compareValues compares two values of any type and records differences
func compareValues(sourceVal, targetVal interface{}, path string, changes *[]DiffChange) {
	sourceMap, sourceIsMap := sourceVal.(map[string]interface{})
	targetMap, targetIsMap := targetVal.(map[string]interface{})
	sourceArr, sourceIsArr := sourceVal.([]interface{})
	targetArr, targetIsArr := targetVal.([]interface{})

	if sourceIsMap && targetIsMap {
		// Both are maps - recurse
		compareObjectsRecursive(sourceMap, targetMap, path, changes)
	} else if sourceIsArr && targetIsArr {
		// Both are arrays - compare elements
		compareArrays(sourceArr, targetArr, path, changes)
	} else if formatValue(sourceVal) != formatValue(targetVal) {
		// Different types or different scalar values
		*changes = append(*changes, DiffChange{
			Type: "changed",
			Path: path,
			Old:  formatValue(sourceVal),
			New:  formatValue(targetVal),
		})
	}
}

// compareArrays compares two arrays and records differences
func compareArrays(source, target []interface{}, path string, changes *[]DiffChange) {
	maxLen := len(source)
	if len(target) > maxLen {
		maxLen = len(target)
	}

	for i := 0; i < maxLen; i++ {
		elemPath := fmt.Sprintf("%s[%d]", path, i)

		if i >= len(source) {
			// Element added in target
			*changes = append(*changes, DiffChange{
				Type: "added",
				Path: elemPath,
				New:  formatValue(target[i]),
			})
		} else if i >= len(target) {
			// Element removed from target
			*changes = append(*changes, DiffChange{
				Type: "removed",
				Path: elemPath,
				Old:  formatValue(source[i]),
			})
		} else {
			// Both have this index - compare
			compareValues(source[i], target[i], elemPath, changes)
		}
	}
}

// formatValue converts a value to string for display
func formatValue(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	switch val := v.(type) {
	case string:
		return val
	case []interface{}:
		return fmt.Sprintf("[%d items]", len(val))
	case map[string]interface{}:
		return fmt.Sprintf("{%d keys}", len(val))
	default:
		return fmt.Sprintf("%v", val)
	}
}
