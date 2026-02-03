package tools

import (
	"strings"
	"testing"
)

func TestAllToolDefs_Count(t *testing.T) {
	defs := AllToolDefs()

	// Verify expected number of tools (16: 12 original + 4 diagnostic tools)
	if len(defs) != 16 {
		t.Errorf("expected 16 tools, got %d", len(defs))
	}
}

func TestAllToolDefs_RequiredFields(t *testing.T) {
	defs := AllToolDefs()

	for _, def := range defs {
		if def.Name == "" {
			t.Error("tool definition missing name")
		}
		if def.Description == "" {
			t.Errorf("tool %q missing description", def.Name)
		}
		if def.InputSchema == nil {
			t.Errorf("tool %q missing input schema", def.Name)
		}

		// Check schema is an object with properties
		schema, ok := def.InputSchema.(map[string]interface{})
		if !ok {
			t.Errorf("tool %q input schema is not a map", def.Name)
			continue
		}

		schemaType, _ := schema["type"].(string)
		if schemaType != "object" {
			t.Errorf("tool %q schema type is %q, expected 'object'", def.Name, schemaType)
		}
	}
}

func TestAllToolDefs_ExpectedTools(t *testing.T) {
	defs := AllToolDefs()

	expectedTools := []string{
		"get_pod_logs",
		"get_resource_yaml",
		"list_resources",
		"get_events",
		"describe_resource",
		"list_crds",
		"list_custom_resources",
		"get_custom_resource_yaml",
		"get_cluster_metrics",
		"get_pod_metrics",
		"get_namespace_summary",
		"get_resource_dependencies",
		// Diagnostic tools
		"get_flow_timeline",
		"get_multi_pod_logs",
		"diff_resources",
		"check_rbac_access",
	}

	toolMap := make(map[string]bool)
	for _, def := range defs {
		toolMap[def.Name] = true
	}

	for _, expected := range expectedTools {
		if !toolMap[expected] {
			t.Errorf("expected tool %q not found", expected)
		}
	}
}

func TestAllToolDefs_GetPodLogsSchema(t *testing.T) {
	defs := AllToolDefs()

	var podLogsDef *ToolDef
	for i := range defs {
		if defs[i].Name == "get_pod_logs" {
			podLogsDef = &defs[i]
			break
		}
	}

	if podLogsDef == nil {
		t.Fatal("get_pod_logs not found")
	}

	schema := podLogsDef.InputSchema.(map[string]interface{})
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("get_pod_logs missing required field")
	}

	// Should require namespace and pod
	hasNamespace := false
	hasPod := false
	for _, r := range required {
		if r == "namespace" {
			hasNamespace = true
		}
		if r == "pod" {
			hasPod = true
		}
	}

	if !hasNamespace {
		t.Error("get_pod_logs should require 'namespace'")
	}
	if !hasPod {
		t.Error("get_pod_logs should require 'pod'")
	}
}

func TestCallTool_UnknownTool(t *testing.T) {
	// CallTool with unknown tool should return error
	result, isErr := CallTool(nil, "unknown_tool", nil)

	if !isErr {
		t.Error("expected error for unknown tool")
	}
	if !strings.Contains(result, "Unknown tool") {
		t.Errorf("expected 'Unknown tool' in result, got %q", result)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "no truncation needed",
			input:    "short string",
			maxLen:   100,
			expected: "short string",
		},
		{
			name:     "exact length",
			input:    "12345",
			maxLen:   5,
			expected: "12345",
		},
		{
			name:     "needs truncation",
			input:    "this is a long string that needs truncation",
			maxLen:   10,
			expected: "this is a \n... [truncated]",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestStrArg(t *testing.T) {
	args := map[string]interface{}{
		"str_key":   "value",
		"int_key":   42,
		"float_key": 3.14,
		"nil_key":   nil,
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"str_key", "value"},
		{"int_key", ""},        // non-string type returns empty
		{"float_key", ""},      // non-string type returns empty
		{"nil_key", ""},        // nil returns empty
		{"missing_key", ""},    // missing key returns empty
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := strArg(args, tt.key)
			if result != tt.expected {
				t.Errorf("strArg(%v, %q) = %q, want %q", args, tt.key, result, tt.expected)
			}
		})
	}
}

func TestIntArg(t *testing.T) {
	args := map[string]interface{}{
		"float_key": 42.0,
		"int_key":   100,
		"str_key":   "not an int",
		"nil_key":   nil,
	}

	tests := []struct {
		key         string
		defaultVal  int
		expected    int
	}{
		{"float_key", 0, 42},        // float64 converted to int
		{"int_key", 0, 100},         // int returned directly
		{"str_key", 5, 5},           // non-numeric returns default
		{"nil_key", 10, 10},         // nil returns default
		{"missing_key", 25, 25},     // missing returns default
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := intArg(args, tt.key, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("intArg(%v, %q, %d) = %d, want %d", args, tt.key, tt.defaultVal, result, tt.expected)
			}
		})
	}
}

func TestNormalizeKind(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"pods", "pod"},
		{"deployments", "deployment"},
		{"statefulsets", "statefulset"},
		{"daemonsets", "daemonset"},
		{"replicasets", "replicaset"},
		{"jobs", "job"},
		{"cronjobs", "cronjob"},
		{"services", "service"},
		{"ingresses", "ingress"},
		{"configmaps", "configmap"},
		{"secrets", "secret"},
		{"nodes", "node"},
		{"namespaces", "namespace"},
		{"pvcs", "pvc"},
		{"persistentvolumeclaims", "pvc"},
		{"pvs", "pv"},
		{"persistentvolumes", "pv"},
		{"storageclasses", "storageclass"},
		{"hpas", "hpa"},
		{"horizontalpodautoscalers", "hpa"},
		{"pdbs", "pdb"},
		{"poddisruptionbudgets", "pdb"},
		{"serviceaccounts", "serviceaccount"},
		{"networkpolicies", "networkpolicy"},
		{"ingressclasses", "ingressclass"},
		// Already singular
		{"pod", "pod"},
		{"deployment", "deployment"},
		// Unknown kind passed through
		{"customkind", "customkind"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeKind(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeKind(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRedactSecretYaml(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "redacts data section",
			input: `apiVersion: v1
kind: Secret
metadata:
  name: my-secret
data:
  username: dXNlcm5hbWU=
  password: cGFzc3dvcmQ=
type: Opaque`,
			expected: `apiVersion: v1
kind: Secret
metadata:
  name: my-secret
data:
  username: [REDACTED]
  password: [REDACTED]
type: Opaque`,
		},
		{
			name: "redacts stringData section",
			input: `apiVersion: v1
kind: Secret
metadata:
  name: my-secret
stringData:
  api-key: supersecret
type: Opaque`,
			expected: `apiVersion: v1
kind: Secret
metadata:
  name: my-secret
stringData:
  api-key: [REDACTED]
type: Opaque`,
		},
		{
			name: "no data section",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
data:
  key: value`,
			expected: `apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
data:
  key: [REDACTED]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactSecretYaml(tt.input)
			if result != tt.expected {
				t.Errorf("redactSecretYaml() mismatch:\ngot:\n%s\n\nwant:\n%s", result, tt.expected)
			}
		})
	}
}
